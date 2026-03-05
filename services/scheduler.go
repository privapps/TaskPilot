package services

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"

	"taskpilot/db"
	"taskpilot/models"

	"github.com/robfig/cron/v3"
)

// MacOSWakeLeadTime is how far before a scheduled cron fire time we wake the Mac.
const MacOSWakeLeadTime = 2 * time.Minute

type Scheduler struct {
	cron       *cron.Cron
	jobService *JobService
	entryMap   map[string]cron.EntryID // maps job ID to cron entry ID
	stopTicker chan bool               // channel to stop the one-time job ticker
	wakeMap    map[string]time.Time   // maps job ID to registered pmset wake time
}

func NewScheduler(jobService *JobService) *Scheduler {
	return &Scheduler{
		cron:       cron.New(),
		jobService: jobService,
		entryMap:   make(map[string]cron.EntryID),
		stopTicker: make(chan bool),
		wakeMap:    make(map[string]time.Time),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	// Load all jobs and schedule them
	jobs, err := s.jobService.GetJobs()
	if err != nil {
		return fmt.Errorf("failed to load jobs: %w", err)
	}

	for _, job := range jobs {
		// Skip paused jobs
		if job.Paused {
			log.Printf("Skipping paused job: %s", job.Name)
			continue
		}

		if err := s.ScheduleJob(job); err != nil {
			log.Printf("Failed to schedule job %s: %v", job.Name, err)
		}
	}

	s.cron.Start()
	log.Println("Scheduler started")

	// Register macOS wake events for all scheduled cron jobs (task 4.1)
	for jobID, entryID := range s.entryMap {
		entry := s.cron.Entry(entryID)
		if !entry.Next.IsZero() {
			// Find the job to check opt-out flag
			jobs, err := s.jobService.GetJobs()
			if err == nil {
				for _, j := range jobs {
					if j.ID == jobID {
						s.registerWakeEvent(j, entry.Next)
						break
					}
				}
			}
		}
	}

	// Start ticker for checking one-time jobs
	go s.checkOneTimeJobs(ctx)

	// Keep running until context is cancelled
	<-ctx.Done()
	s.Stop()
	return nil
}

func (s *Scheduler) Stop() {
	// Non-blocking send to stopTicker (in case Start was never called)
	select {
	case s.stopTicker <- true:
	default:
	}

	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Println("Scheduler stopped")
}

func (s *Scheduler) ScheduleJob(job models.Job) error {
	// Remove existing schedule if any
	if entryID, exists := s.entryMap[job.ID]; exists {
		s.cron.Remove(entryID)
		delete(s.entryMap, job.ID)
	}

	// For immediate execution, run once in a goroutine
	if job.ScheduleType == models.ScheduleTypeImmediate {
		log.Printf("Executing immediate job: %s", job.Name)
		go s.runImmediateJob(job)
		return nil
	}

	// For one-time jobs (delay or datetime), don't use cron - rely on ticker
	if job.ScheduleType == models.ScheduleTypeDelay || job.ScheduleType == models.ScheduleTypeDatetime {
		log.Printf("Scheduled one-time job: %s to run at %v", job.Name, job.RunAt)
		return nil
	}

	// For cron jobs, use the cron scheduler.
	// entryID is declared before AddFunc so the closure can capture it by reference
	// and read it at fire time (by which point AddFunc has returned and assigned it).
	var entryID cron.EntryID
	entryID, err := s.cron.AddFunc(job.Schedule, func() {
		// Detect if the system was sleeping and caused the cron to fire late.
		// entry.Prev is set to the scheduled fire time by the cron library before
		// starting this goroutine, so it reflects when the job *should* have started.
		if entry := s.cron.Entry(entryID); !entry.Prev.IsZero() {
			if delay := time.Since(entry.Prev); delay > 2*time.Minute {
				log.Printf("WARNING: Job '%s' fired %v late (scheduled: %v, actual: %v). System may have been sleeping.",
					job.Name, delay.Round(time.Second),
					entry.Prev.Format("15:04:05"), time.Now().Format("15:04:05"))
			}
		}
		s.runJob(job)
	})
	if err != nil {
		return fmt.Errorf("failed to schedule job: %w", err)
	}

	s.entryMap[job.ID] = entryID
	// Register macOS wake event for the first scheduled fire time (task 3.5)
	if entry := s.cron.Entry(entryID); !entry.Next.IsZero() {
		s.registerWakeEvent(job, entry.Next)
	}
	log.Printf("Scheduled cron job: %s with schedule: %s", job.Name, job.Schedule)
	return nil
}

func (s *Scheduler) UnscheduleJob(jobID string) {
	s.cancelWakeEvent(jobID) // cancel any pending macOS wake event (task 3.6)
	if entryID, exists := s.entryMap[jobID]; exists {
		s.cron.Remove(entryID)
		delete(s.entryMap, jobID)
		log.Printf("Unscheduled job: %s", jobID)
	}
}

// RunJobImmediately triggers a job to execute immediately without affecting its schedule
// This bypasses the paused check since it's an explicit user action
func (s *Scheduler) RunJobImmediately(job models.Job) {
	log.Printf("Triggering immediate execution of job: %s", job.Name)
	go s.runJobImmediate(job)
}

// checkOneTimeJobs checks for one-time jobs that need to be executed
func (s *Scheduler) checkOneTimeJobs(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	log.Printf("Starting one-time job checker")

	for {
		select {
		case <-ticker.C:
			jobs, err := s.jobService.GetJobs()
			if err != nil {
				log.Printf("Failed to load jobs for one-time check: %v", err)
				continue
			}

			now := time.Now().Unix()
			log.Printf("Checking one-time jobs. Current time: %d", now)

			for _, job := range jobs {
				// Skip paused jobs
				if job.Paused {
					continue
				}

				// Log all one-time jobs for debugging
				if job.ScheduleType == models.ScheduleTypeDelay || job.ScheduleType == models.ScheduleTypeDatetime {
					runAtStr := "nil"
					if job.RunAt != nil {
						runAtStr = fmt.Sprintf("%d (in %d seconds)", *job.RunAt, *job.RunAt-now)
					}
					log.Printf("One-time job '%s': type=%s, run_at=%s, status=%s, paused=%v",
						job.Name, job.ScheduleType, runAtStr, job.Status, job.Paused)
				}

				// Check if this is a one-time job that should run now
				if (job.ScheduleType == models.ScheduleTypeDelay || job.ScheduleType == models.ScheduleTypeDatetime) &&
					job.RunAt != nil && *job.RunAt <= now && job.Status != "running" {

					// Skip stale jobs: scheduled more than 5 minutes ago and never ran.
					// A 5-minute window tolerates brief app restarts; anything older is
					// considered missed and should not be retroactively executed.
					if now-*job.RunAt > 5*60 && job.LastRunAt == nil {
						log.Printf("Skipping stale one-time job '%s': scheduled %ds ago, never ran", job.Name, now-*job.RunAt)
						continue
					}

					log.Printf("Triggering one-time job: %s (run_at=%d, now=%d)", job.Name, *job.RunAt, now)

					// Update status to "running" immediately to prevent duplicate triggers
					job.Status = "running"
					if _, err := s.jobService.updateJob(job, false); err != nil {
						log.Printf("Failed to update job status before execution: %v", err)
						continue
					}

					go s.runJob(job)
				}
			}
		case <-s.stopTicker:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) runJob(job models.Job) {
	// Prevent execution of paused jobs
	if job.Paused {
		log.Printf("Skipping execution of paused job: %s", job.Name)
		return
	}

	log.Printf("Executing job: %s", job.Name)

	// Update status to running
	job.Status = "running"
	if _, err := s.jobService.updateJob(job, false); err != nil {
		log.Printf("Failed to update job status: %v", err)
	}

	// Execute the command (wrap with caffeinate on macOS to prevent sleep mid-run)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", wrapWithCaffeinate(job.Command, job.DisableMacosSleepPrevention))
	if job.Directory != "" {
		cmd.Dir = job.Directory
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// Prepare output and exit code
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			output += fmt.Sprintf("\nError: %v", err)
		}
	}

	// Update job status
	if exitCode == 0 {
		job.Status = "idle"
		job.LastResult = "success"

		// Play sound if specified
		if job.SoundFile != "" {
			go s.playSound(job.SoundFile)
		}

		// Execute on_success_cmd if specified
		if job.OnSuccessCmd != "" {
			go s.executeOnSuccess(job.OnSuccessCmd)
		}
	} else {
		job.Status = "idle"
		job.LastResult = "failed"
	}

	// For one-time jobs, pause them after execution so they don't run again
	if job.ScheduleType == models.ScheduleTypeDelay || job.ScheduleType == models.ScheduleTypeDatetime {
		job.Paused = true
		log.Printf("Pausing one-time job after execution: %s", job.Name)
	}

	// Update last run timestamp
	now := time.Now().Unix()
	job.LastRunAt = &now

	if _, err := s.jobService.updateJob(job, false); err != nil {
		log.Printf("Failed to update job after execution: %v", err)
	}

	// Create history entry
	s.createHistory(job.ID, output, exitCode, startTime, duration)

	// Re-register macOS wake event for the next scheduled fire time (task 3.7)
	if job.ScheduleType == models.ScheduleTypeCron {
		if entryID, exists := s.entryMap[job.ID]; exists {
			if entry := s.cron.Entry(entryID); !entry.Next.IsZero() {
				s.registerWakeEvent(job, entry.Next)
			}
		}
	}

	log.Printf("Job %s completed with exit code %d in %v", job.Name, exitCode, duration)
}
// This is used for manual triggers where the user explicitly wants to run the job
func (s *Scheduler) runJobImmediate(job models.Job) {
	log.Printf("Executing job immediately (bypassing pause check): %s", job.Name)

	// Update status to running
	job.Status = "running"
	if _, err := s.jobService.updateJob(job, false); err != nil {
		log.Printf("Failed to update job status: %v", err)
	}

	// Execute the command (wrap with caffeinate on macOS to prevent sleep mid-run)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", wrapWithCaffeinate(job.Command, job.DisableMacosSleepPrevention))
	if job.Directory != "" {
		cmd.Dir = job.Directory
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// Prepare output and exit code
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			output += fmt.Sprintf("\nError: %v", err)
		}
	}

	// Update job status
	if exitCode == 0 {
		job.Status = "idle"
		job.LastResult = "success"

		// Play sound if specified
		if job.SoundFile != "" {
			go s.playSound(job.SoundFile)
		}

		// Execute on_success_cmd if specified
		if job.OnSuccessCmd != "" {
			go s.executeOnSuccess(job.OnSuccessCmd)
		}
	} else {
		job.Status = "idle"
		job.LastResult = "failed"
	}

	// For one-time jobs, pause them after execution so they don't run again
	if job.ScheduleType == models.ScheduleTypeDelay || job.ScheduleType == models.ScheduleTypeDatetime {
		job.Paused = true
		log.Printf("Pausing one-time job after execution: %s", job.Name)
	}

	// Update last run timestamp
	now := time.Now().Unix()
	job.LastRunAt = &now

	if _, err := s.jobService.updateJob(job, false); err != nil {
		log.Printf("Failed to update job after execution: %v", err)
	}

	// Create history entry
	s.createHistory(job.ID, output, exitCode, startTime, duration)

	log.Printf("Job %s completed with exit code %d in %v", job.Name, exitCode, duration)
}

func (s *Scheduler) runImmediateJob(job models.Job) {
	// Immediate jobs execute once immediately
	log.Printf("Executing immediate job: %s", job.Name)

	// Update status to running
	job.Status = "running"
	if _, err := s.jobService.updateJob(job, false); err != nil {
		log.Printf("Failed to update job status: %v", err)
	}

	// Execute the command (wrap with caffeinate on macOS to prevent sleep mid-run)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", wrapWithCaffeinate(job.Command, job.DisableMacosSleepPrevention))
	if job.Directory != "" {
		cmd.Dir = job.Directory
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// Prepare output and exit code
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			output += fmt.Sprintf("\nError: %v", err)
		}
	}

	// Mark immediate jobs as completed after execution
	if exitCode == 0 {
		job.Status = "completed"
		job.LastResult = "success"

		// Play sound if specified
		if job.SoundFile != "" {
			go s.playSound(job.SoundFile)
		}

		// Execute on_success_cmd if specified
		if job.OnSuccessCmd != "" {
			go s.executeOnSuccess(job.OnSuccessCmd)
		}
	} else {
		job.Status = "failed"
		job.LastResult = "failed"
	}

	// Immediate jobs don't reschedule - mark as paused
	job.Paused = true

	// Update last run timestamp
	now := time.Now().Unix()
	job.LastRunAt = &now

	if _, err := s.jobService.updateJob(job, false); err != nil {
		log.Printf("Failed to update job after execution: %v", err)
	}

	// Create history entry
	s.createHistory(job.ID, output, exitCode, startTime, duration)

	log.Printf("Immediate job %s completed with exit code %d in %v", job.Name, exitCode, duration)
}

func (s *Scheduler) executeOnSuccess(cmd string) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	execCmd := exec.CommandContext(ctx, "sh", "-c", cmd)
	if err := execCmd.Run(); err != nil {
		log.Printf("Failed to execute on_success_cmd: %v", err)
	}
}

func (s *Scheduler) playSound(soundFile string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use afplay on macOS to play the sound
	cmd := exec.CommandContext(ctx, "afplay", soundFile)
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to play sound %s: %v", soundFile, err)
	} else {
		log.Printf("Played sound: %s", soundFile)
	}
}

func (s *Scheduler) createHistory(jobID string, output string, exitCode int, startTime time.Time, duration time.Duration) {
	query := `INSERT INTO history (id, job_id, output, exit_code, timestamp, duration_ms) VALUES (?, ?, ?, ?, ?, ?)`

	durationMs := duration.Milliseconds()
	timestamp := startTime.Unix() // record when the job started, not when it completed

	if _, err := db.DB.Exec(query, generateHistoryID(), jobID, output, exitCode, timestamp, durationMs); err != nil {
		log.Printf("Failed to create history entry: %v", err)
	}
}

func generateHistoryID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// wrapWithCaffeinate wraps cmd with `caffeinate -s --` on macOS to assert a
// PreventSystemSleep power assertion for the duration of the job. Falls back to
// the original command if caffeinate is not on PATH or the opt-out flag is set.
func wrapWithCaffeinate(cmd string, disabled bool) string {
	if disabled || runtime.GOOS != "darwin" {
		return cmd
	}
	if _, err := exec.LookPath("caffeinate"); err != nil {
		log.Printf("WARNING: caffeinate not found on PATH, running job without sleep prevention: %v", err)
		return cmd
	}
	return "caffeinate -s -- sh -c " + shellQuote(cmd)
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	// Replace ' with '\'' and wrap the whole thing in single quotes
	result := "'"
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			result += "'\\''"
		} else {
			result += string(s[i])
		}
	}
	return result + "'"
}

// registerWakeEvent schedules a macOS pmset wake event before nextFire so the
// system is awake when the cron timer fires. No-op on non-darwin or when opted out.
func (s *Scheduler) registerWakeEvent(job models.Job, nextFire time.Time) {
	if runtime.GOOS != "darwin" || job.DisableMacosSleepPrevention {
		return
	}

	wakeTime := nextFire.Add(-MacOSWakeLeadTime)
	if wakeTime.Before(time.Now()) {
		// Wake time already passed — nothing to schedule
		return
	}

	// pmset expects: MM/dd/yy HH:mm:ss
	wakeStr := wakeTime.Format("01/02/06 15:04:05")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pmset", "schedule", "wake", wakeStr)
	if err := cmd.Run(); err != nil {
		log.Printf("WARNING: Failed to register pmset wake for job '%s' at %s: %v", job.Name, wakeStr, err)
		return
	}

	s.wakeMap[job.ID] = wakeTime
	log.Printf("Registered macOS wake event for job '%s' at %s (fires at %s)",
		job.Name, wakeStr, nextFire.Format("15:04:05"))
}

// cancelWakeEvent cancels a previously registered pmset wake event for a job.
func (s *Scheduler) cancelWakeEvent(jobID string) {
	wakeTime, exists := s.wakeMap[jobID]
	if !exists || runtime.GOOS != "darwin" {
		return
	}

	wakeStr := wakeTime.Format("01/02/06 15:04:05")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pmset", "cancel", "wake", wakeStr)
	if err := cmd.Run(); err != nil {
		// Cancel failures are non-fatal — duplicate wake events are harmless
		log.Printf("Note: pmset cancel wake for job %s returned: %v", jobID, err)
	}

	delete(s.wakeMap, jobID)
}
