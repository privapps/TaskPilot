package services

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"

	"taskpilot/db"
	"taskpilot/models"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron       *cron.Cron
	jobService *JobService
	entryMap   map[string]cron.EntryID // maps job ID to cron entry ID
	stopTicker chan bool               // channel to stop the one-time job ticker
}

func NewScheduler(jobService *JobService) *Scheduler {
	return &Scheduler{
		cron:       cron.New(),
		jobService: jobService,
		entryMap:   make(map[string]cron.EntryID),
		stopTicker: make(chan bool),
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

	// For cron jobs, use the cron scheduler
	entryID, err := s.cron.AddFunc(job.Schedule, func() {
		s.runJob(job)
	})
	if err != nil {
		return fmt.Errorf("failed to schedule job: %w", err)
	}

	s.entryMap[job.ID] = entryID
	log.Printf("Scheduled cron job: %s with schedule: %s", job.Name, job.Schedule)
	return nil
}

func (s *Scheduler) UnscheduleJob(jobID string) {
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
					log.Printf("Triggering one-time job: %s (run_at=%d, now=%d)", job.Name, *job.RunAt, now)
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

	// Execute the command
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", job.Command)
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
	s.createHistory(job.ID, output, exitCode, duration)

	log.Printf("Job %s completed with exit code %d in %v", job.Name, exitCode, duration)
}

// runJobImmediate executes a job immediately without checking if it's paused
// This is used for manual triggers where the user explicitly wants to run the job
func (s *Scheduler) runJobImmediate(job models.Job) {
	log.Printf("Executing job immediately (bypassing pause check): %s", job.Name)

	// Update status to running
	job.Status = "running"
	if _, err := s.jobService.updateJob(job, false); err != nil {
		log.Printf("Failed to update job status: %v", err)
	}

	// Execute the command
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", job.Command)
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
	s.createHistory(job.ID, output, exitCode, duration)

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

	// Execute the command
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", job.Command)
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
	s.createHistory(job.ID, output, exitCode, duration)

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

func (s *Scheduler) createHistory(jobID string, output string, exitCode int, duration time.Duration) {
	query := `INSERT INTO history (id, job_id, output, exit_code, timestamp, duration_ms) VALUES (?, ?, ?, ?, ?, ?)`

	durationMs := duration.Milliseconds()
	timestamp := time.Now().Unix()

	if _, err := db.DB.Exec(query, generateHistoryID(), jobID, output, exitCode, timestamp, durationMs); err != nil {
		log.Printf("Failed to create history entry: %v", err)
	}
}

func generateHistoryID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
