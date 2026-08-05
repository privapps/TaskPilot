package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"taskpilot/models"
)

const (
	// MacOSWakeLeadTime is how far before a scheduled fire time we wake the Mac.
	MacOSWakeLeadTime = 2 * time.Minute

	schedulerPollInterval = 5 * time.Second
	cronMisfireGrace      = 10 * time.Minute
	oneTimeMisfireGrace   = 5 * time.Minute
	skippedJobExitCode    = -2

	triggerTypeEvent     = "event"
	triggerTypeImmediate = "immediate"
	triggerTypeManual    = "manual"
	triggerTypeRecovery  = "recovery"
	triggerTypeScheduled = "scheduled"
	triggerTypeSkipped   = "skipped"
)

type Scheduler struct {
	schedule    *ScheduleSemantics
	persistence *JobPersistence
	jobService  *JobService
	executor    JobExecutor
	commands    CommandRunner
	sound       SoundPlayer
	wakeEvents  WakeEventAdapter
	stopTicker  chan bool
	wakeMap     map[string]time.Time // maps job ID to registered pmset wake time
}

func NewScheduler(jobService *JobService) *Scheduler {
	return NewSchedulerWithAdapters(jobService, ShellJobExecutor{}, ShellCommandRunner{}, SystemSoundPlayer{}, SystemWakeEventAdapter{})
}

func NewSchedulerWithAdapters(jobService *JobService, executor JobExecutor, commands CommandRunner, sound SoundPlayer, wakeEvents WakeEventAdapter) *Scheduler {
	return &Scheduler{
		schedule:    jobService.schedule,
		persistence: jobService.persistence,
		jobService:  jobService,
		executor:    executor,
		commands:    commands,
		sound:       sound,
		wakeEvents:  wakeEvents,
		stopTicker:  make(chan bool, 1),
		wakeMap:     make(map[string]time.Time),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	log.Println("Scheduler started (durable DB mode)")

	if err := s.recoverInterruptedJobs(); err != nil {
		return fmt.Errorf("failed to recover interrupted jobs: %w", err)
	}
	if err := s.reconcileAllJobs(); err != nil {
		return fmt.Errorf("failed to reconcile job schedules: %w", err)
	}
	if err := s.processDueJobs(time.Now()); err != nil {
		return fmt.Errorf("failed to process due jobs: %w", err)
	}

	ticker := time.NewTicker(schedulerPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.reconcileAllJobs(); err != nil {
				log.Printf("Failed to reconcile job schedules: %v", err)
			}
			if err := s.processDueJobs(time.Now()); err != nil {
				log.Printf("Failed to process due jobs: %v", err)
			}
		case <-s.stopTicker:
			log.Println("Scheduler stopped")
			return nil
		case <-ctx.Done():
			log.Println("Scheduler stopped")
			return nil
		}
	}
}

func (s *Scheduler) Stop() {
	select {
	case s.stopTicker <- true:
	default:
	}
}

func (s *Scheduler) ScheduleJob(job models.Job) error {
	s.cancelWakeEvent(job.ID)

	if job.Paused {
		return s.persistNextRun(job.ID, nil)
	}

	if job.ScheduleType == models.ScheduleTypeImmediate {
		log.Printf("Executing immediate job: %s", job.Name)
		go s.runImmediateJob(job)
		return nil
	}

	nextRunAt, err := s.calculateNextRun(job, time.Now())
	if err != nil {
		return err
	}

	if err := s.persistNextRun(job.ID, nextRunAt); err != nil {
		return err
	}

	if nextRunAt != nil {
		s.registerWakeEvent(job, time.Unix(*nextRunAt, 0))
		log.Printf("Scheduled durable job: %s to run at %s", job.Name, time.Unix(*nextRunAt, 0).Format(time.RFC3339))
	} else {
		log.Printf("Cleared next run for job: %s", job.Name)
	}

	return nil
}

func (s *Scheduler) UnscheduleJob(jobID string) {
	s.cancelWakeEvent(jobID)
	if err := s.persistNextRun(jobID, nil); err != nil {
		log.Printf("Failed to unschedule job %s: %v", jobID, err)
		return
	}
	log.Printf("Unscheduled job: %s", jobID)
}

// RunJobImmediately triggers a job to execute immediately without affecting its schedule.
// This bypasses the paused check since it's an explicit user action.
func (s *Scheduler) RunJobImmediately(job models.Job) {
	log.Printf("Triggering immediate execution of job: %s", job.Name)
	go s.runJobInternal(job, nil, triggerTypeManual, false, false)
}

func (s *Scheduler) reconcileAllJobs() error {
	jobs, err := s.jobService.GetJobs()
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if job.Paused || job.ScheduleType == models.ScheduleTypeImmediate {
			if job.NextRunAt != nil {
				if err := s.persistNextRun(job.ID, nil); err != nil {
					log.Printf("Failed to clear next run for paused/immediate job %s: %v", job.Name, err)
				}
			}
			s.cancelWakeEvent(job.ID)
			continue
		}

		if job.NextRunAt == nil {
			if err := s.ScheduleJob(job); err != nil {
				log.Printf("Failed to reconcile schedule for job %s: %v", job.Name, err)
			}
		}
	}

	return nil
}

func (s *Scheduler) recoverInterruptedJobs() error {
	jobs, err := s.jobService.GetJobs()
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if job.Status != "running" {
			continue
		}

		job.Status = "idle"
		job.LastResult = "interrupted"
		if err := s.persistence.UpdateJob(job); err != nil {
			log.Printf("Failed to recover interrupted job %s: %v", job.Name, err)
			continue
		}

		message := "Recovered interrupted job after scheduler restart"
		s.createHistory(job.ID, message, -1, time.Now(), 0, job.LastScheduledAt, triggerTypeRecovery)
		log.Printf("Recovered interrupted job: %s", job.Name)
	}

	return nil
}

func (s *Scheduler) processDueJobs(now time.Time) error {
	jobs, err := s.jobService.GetJobs()
	if err != nil {
		return err
	}

	nowUnix := now.Unix()
	for _, job := range jobs {
		if job.Paused || job.NextRunAt == nil || *job.NextRunAt > nowUnix || job.ScheduleType == models.ScheduleTypeImmediate {
			continue
		}

		scheduledAt := *job.NextRunAt
		if skip, reason := s.shouldSkipLateExecution(job, scheduledAt, now); skip {
			if err := s.skipDueJob(job, scheduledAt, now, reason); err != nil {
				log.Printf("Failed to skip overdue job %s: %v", job.Name, err)
			}
			continue
		}

		nextRunAt, err := s.calculateFollowingRun(job, scheduledAt)
		if err != nil {
			log.Printf("Failed to calculate next run for job %s: %v", job.Name, err)
			continue
		}

		pauseOnClaim := job.ScheduleType == models.ScheduleTypeDelay || job.ScheduleType == models.ScheduleTypeDatetime
		claimed, err := s.claimScheduledExecution(job, scheduledAt, nextRunAt, pauseOnClaim)
		if err != nil {
			log.Printf("Failed to claim job %s: %v", job.Name, err)
			continue
		}
		if !claimed {
			continue
		}

		job.Status = "running"
		job.NextRunAt = nextRunAt
		job.LastScheduledAt = int64Ptr(scheduledAt)
		if pauseOnClaim {
			job.Paused = true
		}

		go s.runScheduledJob(job, scheduledAt)
	}

	return nil
}

func (s *Scheduler) calculateNextRun(job models.Job, now time.Time) (*int64, error) {
	return s.schedule.NextRun(job, now)
}

func (s *Scheduler) calculateFollowingRun(job models.Job, scheduledAt int64) (*int64, error) {
	return s.schedule.FollowingRun(job, scheduledAt)
}

func (s *Scheduler) shouldSkipLateExecution(job models.Job, scheduledAt int64, now time.Time) (bool, string) {
	if !s.schedule.Supports(job.ScheduleType) {
		return false, ""
	}
	delay := now.Sub(time.Unix(scheduledAt, 0))
	if delay <= 0 {
		return false, ""
	}

	grace := s.schedule.MisfireGrace(job)
	if delay <= grace {
		return false, ""
	}

	reason := fmt.Sprintf(
		"Skipped overdue job '%s': scheduled for %s but detected at %s (%v late)",
		job.Name,
		time.Unix(scheduledAt, 0).Format(time.RFC3339),
		now.Format(time.RFC3339),
		delay.Round(time.Second),
	)
	return true, reason
}

func (s *Scheduler) skipDueJob(job models.Job, scheduledAt int64, now time.Time, reason string) error {
	var nextRunAt *int64
	var err error
	if job.ScheduleType == models.ScheduleTypeCron || job.ScheduleType == "" {
		nextRunAt, err = s.calculateNextRun(job, now)
		if err != nil {
			return err
		}
	}

	pauseJob := job.ScheduleType == models.ScheduleTypeDelay || job.ScheduleType == models.ScheduleTypeDatetime
	claimed, err := s.persistence.SkipScheduledExecution(job, scheduledAt, nextRunAt, pauseJob)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	if nextRunAt != nil {
		s.registerWakeEvent(job, time.Unix(*nextRunAt, 0))
	} else {
		s.cancelWakeEvent(job.ID)
	}

	s.createHistory(job.ID, reason, skippedJobExitCode, now, 0, int64Ptr(scheduledAt), triggerTypeSkipped)
	log.Print(reason)
	return nil
}

func (s *Scheduler) claimScheduledExecution(job models.Job, scheduledAt int64, nextRunAt *int64, pauseOnClaim bool) (bool, error) {
	claimed, err := s.persistence.ClaimScheduledExecution(job, scheduledAt, nextRunAt, pauseOnClaim)
	if err != nil {
		return false, err
	}
	if !claimed {
		return false, nil
	}

	if nextRunAt != nil {
		s.registerWakeEvent(job, time.Unix(*nextRunAt, 0))
	} else {
		s.cancelWakeEvent(job.ID)
	}

	return true, nil
}

func (s *Scheduler) persistNextRun(jobID string, nextRunAt *int64) error {
	return s.persistence.SetNextRun(jobID, nextRunAt)
}

func (s *Scheduler) runScheduledJob(job models.Job, scheduledAt int64) {
	s.runJobInternal(job, int64Ptr(scheduledAt), triggerTypeScheduled, true, false)
}

func (s *Scheduler) runImmediateJob(job models.Job) {
	s.runJobInternal(job, nil, triggerTypeImmediate, false, true)
}

func (s *Scheduler) runJobInternal(job models.Job, scheduledAt *int64, triggerType string, alreadyMarkedRunning bool, pauseAfterRun bool) {
	if triggerType == triggerTypeScheduled && scheduledAt == nil && job.Paused {
		log.Printf("Skipping execution of paused job: %s", job.Name)
		return
	}

	if !alreadyMarkedRunning {
		job.Status = "running"
		if err := s.persistence.UpdateJob(job); err != nil {
			log.Printf("Failed to update job status: %v", err)
		}
	}

	log.Printf("Executing job: %s (%s)", job.Name, triggerType)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result := s.executor.Execute(ctx, job)
	output := result.Output
	exitCode := result.ExitCode
	startTime := result.StartedAt
	duration := result.Duration

	if exitCode == 0 {
		if pauseAfterRun {
			job.Status = "completed"
		} else {
			job.Status = "idle"
		}
		job.LastResult = "success"

		if job.SoundFile != "" {
			go s.playSound(job.SoundFile)
		}
		if job.OnSuccessCmd != "" {
			go s.executeOnSuccess(job.OnSuccessCmd)
		}
	} else {
		if pauseAfterRun {
			job.Status = "failed"
		} else {
			job.Status = "idle"
		}
		job.LastResult = "failed"
	}

	if pauseAfterRun {
		job.Paused = true
	}

	now := time.Now().Unix()
	job.LastRunAt = &now

	if err := s.persistence.UpdateJob(job); err != nil {
		log.Printf("Failed to update job after execution: %v", err)
	}

	s.createHistory(job.ID, output, exitCode, startTime, duration, scheduledAt, triggerType)
	log.Printf("Job %s completed with exit code %d in %v", job.Name, exitCode, duration)
}

func (s *Scheduler) executeOnSuccess(cmd string) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	if err := s.commands.Run(ctx, cmd); err != nil {
		log.Printf("Failed to execute on_success_cmd: %v", err)
	}
}

func (s *Scheduler) playSound(soundFile string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.sound.Play(ctx, soundFile); err != nil {
		log.Printf("Failed to play sound %s: %v", soundFile, err)
	} else {
		log.Printf("Played sound: %s", soundFile)
	}
}

func (s *Scheduler) createHistory(jobID string, output string, exitCode int, startTime time.Time, duration time.Duration, scheduledAt *int64, triggerType string) {
	err := s.persistence.RecordHistory(models.History{
		ID:          generateHistoryID(),
		JobID:       jobID,
		Output:      output,
		ExitCode:    exitCode,
		Timestamp:   startTime.Unix(),
		DurationMs:  duration.Milliseconds(),
		ScheduledAt: scheduledAt,
		TriggerType: triggerType,
	})
	if err != nil {
		log.Printf("Failed to create history entry: %v", err)
	}
}

func generateHistoryID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// registerWakeEvent schedules a macOS pmset wake event before nextFire so the
// system is awake when the timer fires. No-op on non-darwin or when opted out.
func (s *Scheduler) registerWakeEvent(job models.Job, nextFire time.Time) {
	if job.DisableMacosSleepPrevention {
		return
	}

	wakeTime := nextFire.Add(-MacOSWakeLeadTime)
	if wakeTime.Before(time.Now()) {
		return
	}

	registered, err := s.wakeEvents.Register(job, wakeTime)
	if err != nil {
		log.Printf("WARNING: Failed to register pmset wake for job '%s' at %s: %v", job.Name, wakeTime.Format("01/02/06 15:04:05"), err)
		return
	}
	if !registered {
		return
	}

	s.wakeMap[job.ID] = wakeTime
	log.Printf("Registered macOS wake event for job '%s' at %s (fires at %s)",
		job.Name, wakeTime.Format("01/02/06 15:04:05"), nextFire.Format(time.RFC3339))
}

// cancelWakeEvent cancels a previously registered pmset wake event for a job.
func (s *Scheduler) cancelWakeEvent(jobID string) {
	wakeTime, exists := s.wakeMap[jobID]
	if !exists {
		return
	}

	if err := s.wakeEvents.Cancel(jobID, wakeTime); err != nil {
		log.Printf("Note: pmset cancel wake for job %s returned: %v", jobID, err)
	}

	delete(s.wakeMap, jobID)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func int64Ptr(v int64) *int64 {
	return &v
}
