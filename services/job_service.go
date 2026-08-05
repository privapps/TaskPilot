package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"taskpilot/db"
	"taskpilot/models"

	"github.com/google/uuid"
)

type JobService struct {
	db              db.DatabaseInterface
	persistence     *JobPersistence
	scheduler       *Scheduler
	defaultsService *DefaultsService
	schedule        *ScheduleSemantics
}

func NewJobService() *JobService {
	database := &db.Database{DB: db.DB}
	return &JobService{
		db:              database,
		persistence:     NewJobPersistence(database),
		defaultsService: NewDefaultsService(),
		schedule:        NewScheduleSemantics(),
	}
}

func NewJobServiceWithDB(database db.DatabaseInterface) *JobService {
	return &JobService{
		db:              database,
		persistence:     NewJobPersistence(database),
		defaultsService: NewDefaultsServiceWithDB(database),
		schedule:        NewScheduleSemantics(),
	}
}

func (s *JobService) SetScheduler(scheduler *Scheduler) {
	s.scheduler = scheduler
}

func (s *JobService) GetJobs() ([]models.Job, error) {
	return s.persistence.ListJobs()
}

func (s *JobService) CreateJob(job models.Job) (models.Job, error) {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}

	if job.Status == "" {
		job.Status = "idle"
	}

	// Apply defaults to empty fields
	if err := s.defaultsService.ApplyDefaultsToJob(&job); err != nil {
		log.Printf("Warning: Failed to apply defaults: %v", err)
		// Continue with job creation even if defaults fail
	}

	// Normalize and validate schedule fields in one place for every caller.
	normalized, err := s.schedule.Normalize(job)
	if err != nil {
		return models.Job{}, err
	}
	job = normalized

	if err := s.persistence.InsertJob(job); err != nil {
		return models.Job{}, err
	}

	// Notify scheduler
	if s.scheduler != nil && !job.Paused {
		if err := s.scheduler.ScheduleJob(job); err != nil {
			return job, fmt.Errorf("job created but failed to schedule: %w", err)
		}
	}

	return job, nil
}

func (s *JobService) UpdateJob(job models.Job) (models.Job, error) {
	normalized, err := s.schedule.Normalize(job)
	if err != nil {
		return models.Job{}, err
	}
	return s.updateJob(normalized, true)
}

func (s *JobService) updateJob(job models.Job, notifyScheduler bool) (models.Job, error) {
	if job.ID == "" {
		return models.Job{}, errors.New("job ID is required")
	}

	if err := s.persistence.UpdateJob(job); err != nil {
		return models.Job{}, err
	}

	// Notify scheduler to reschedule if requested
	if notifyScheduler && s.scheduler != nil {
		// First unschedule the old job
		s.scheduler.UnscheduleJob(job.ID)
		// Then reschedule if not paused
		if !job.Paused {
			if err := s.scheduler.ScheduleJob(job); err != nil {
				return job, fmt.Errorf("job updated but failed to reschedule: %w", err)
			}
		}
	}

	return job, nil
}

func (s *JobService) DeleteJob(id string) error {
	if id == "" {
		return errors.New("job ID is required")
	}

	// Notify scheduler first (before any DB operations)
	if s.scheduler != nil {
		s.scheduler.UnscheduleJob(id)
	}

	if err := s.persistence.DeleteJob(id); err != nil {
		return err
	}

	return nil
}

func (s *JobService) GetJobByID(id string) (models.Job, error) {
	return s.persistence.GetJob(id)
}

func (s *JobService) TriggerJob(jobID string) error {
	if jobID == "" {
		return errors.New("job ID is required")
	}

	// Get the job
	job, err := s.GetJobByID(jobID)
	if err != nil {
		return err // Error already formatted by GetJobByID
	}

	// Check if scheduler is available
	if s.scheduler == nil {
		return errors.New("scheduler not available")
	}

	// Trigger immediate execution
	s.scheduler.RunJobImmediately(job)

	return nil
}

func (s *JobService) GetJobHistory(jobID string, limit int) ([]models.History, error) {
	return s.GetJobHistoryPage(jobID, limit, 0)
}

// GetJobHistoryPage returns a page of execution history records for a job.
// Results are ordered newest first and offset allows the UI to load older pages.
func (s *JobService) GetJobHistoryPage(jobID string, limit int, offset int) ([]models.History, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.persistence.ListJobHistoryPage(jobID, limit, offset)
}

// GetHistory retrieves all job execution history records
// ValidateUUID checks if a string is a valid UUID format
func ValidateUUID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}

// GetHistory retrieves history records with optional job_id filter and ordering
// jobID: optional job_id to filter by (nil means no filter)
// orderAsc: true for ascending order, false for descending (default)
func (s *JobService) GetHistory(jobID *string, orderAsc bool) ([]models.History, error) {
	return s.persistence.ListHistory(jobID, orderAsc)
}

// DeleteHistory deletes a single history record by ID
func (s *JobService) DeleteHistory(historyID string) error {
	if historyID == "" {
		return errors.New("history ID is required")
	}
	return s.persistence.DeleteHistory(historyID)
}

// CalculateRunAt calculates the run_at timestamp for delay-based scheduling
func CalculateRunAt(delayMinutes int) int64 {
	return NewScheduleSemantics().RunAtForDelay(delayMinutes)
}

// PauseJob pauses a job by setting paused=true and removing it from the scheduler
func (s *JobService) PauseJob(jobID string) error {
	job, err := s.GetJobByID(jobID)
	if err != nil {
		return err
	}

	job.Paused = true
	_, err = s.updateJob(job, false)
	if err != nil {
		return err
	}

	// Remove from scheduler
	if s.scheduler != nil {
		s.scheduler.UnscheduleJob(jobID)
	}

	// Record pause event in history
	s.recordHistoryEvent(jobID, "Job paused")

	return nil
}

// ResumeJob resumes a paused job by setting paused=false and adding it back to the scheduler
func (s *JobService) ResumeJob(jobID string) error {
	job, err := s.GetJobByID(jobID)
	if err != nil {
		return err
	}

	job.Paused = false
	_, err = s.updateJob(job, false)
	if err != nil {
		return err
	}

	// Add back to scheduler
	if s.scheduler != nil {
		if err := s.scheduler.ScheduleJob(job); err != nil {
			return fmt.Errorf("failed to reschedule job: %w", err)
		}
	}

	// Record resume event in history
	s.recordHistoryEvent(jobID, "Job resumed")

	return nil
}

// DeleteJobHistory deletes a single history entry
func (s *JobService) DeleteJobHistory(historyID string) error {
	return s.persistence.DeleteHistory(historyID)
}

// DeleteAllJobHistory deletes all history entries for a specific job
func (s *JobService) DeleteAllJobHistory(jobID string) error {
	return s.persistence.DeleteAllJobHistory(jobID)
}

// DeleteAllHistory deletes all history entries across all jobs
func (s *JobService) DeleteAllHistory() error {
	return s.persistence.DeleteAllHistory()
}

// recordHistoryEvent records a pause/resume event in the history
func (s *JobService) recordHistoryEvent(jobID, message string) {
	_ = s.persistence.RecordHistory(models.History{
		ID:          uuid.New().String(),
		JobID:       jobID,
		Output:      message,
		Timestamp:   time.Now().Unix(),
		TriggerType: triggerTypeEvent,
	})
}

// DuplicateJob creates a copy of an existing job (paused by default)
func (s *JobService) DuplicateJob(jobID string) (models.Job, error) {
	originalJob, err := s.GetJobByID(jobID)
	if err != nil {
		return models.Job{}, err
	}

	// Create new job with copied properties
	newJob := originalJob
	newJob.ID = uuid.New().String()
	newJob.Name = originalJob.Name + " (Copy)"
	newJob.Status = "idle"
	newJob.LastResult = ""
	newJob.LastRunAt = nil // Clear last run timestamp for duplicated job
	newJob.NextRunAt = nil
	newJob.LastScheduledAt = nil

	// For immediate execution jobs, keep them unpaused so they execute right away
	// For other jobs, pause by default
	if newJob.ScheduleType == models.ScheduleTypeImmediate {
		newJob.Paused = false
	} else {
		newJob.Paused = true
	}

	// For delay-based jobs, recalculate run_at
	if newJob.ScheduleType == models.ScheduleTypeDelay && newJob.DelayMinutes != nil {
		runAt := CalculateRunAt(*newJob.DelayMinutes)
		newJob.RunAt = &runAt
	}

	if err := s.persistence.InsertJob(newJob); err != nil {
		return models.Job{}, fmt.Errorf("failed to duplicate job: %w", err)
	}

	// If immediate job and not paused, schedule it to run immediately
	if s.scheduler != nil && !newJob.Paused {
		if err := s.scheduler.ScheduleJob(newJob); err != nil {
			return newJob, fmt.Errorf("job duplicated but failed to schedule: %w", err)
		}
	}

	return newJob, nil
}

func (s *JobService) ExportJobs(filepath string) error {
	jobs, err := s.GetJobs()
	if err != nil {
		return fmt.Errorf("failed to get jobs: %w", err)
	}

	// Get defaults to include in export
	defaults, err := s.defaultsService.GetDefaults()
	if err != nil {
		log.Printf("Warning: Failed to get defaults for export: %v", err)
		defaults = nil // Continue without defaults
	}

	export := models.JobExport{
		Version:    "1.0",
		ExportedAt: time.Now().Format(time.RFC3339),
		Jobs:       jobs,
		Defaults:   defaults,
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal jobs: %w", err)
	}

	err = os.WriteFile(filepath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write export file: %w", err)
	}

	return nil
}

func (s *JobService) ImportJobs(filepath string) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read import file: %w", err)
	}

	var export models.JobExport
	err = json.Unmarshal(data, &export)
	if err != nil {
		return fmt.Errorf("failed to unmarshal import file: %w", err)
	}

	if export.Version != "1.0" {
		return fmt.Errorf("unsupported version: %s", export.Version)
	}

	for index, job := range export.Jobs {
		// Ensure defaults if missing in JSON (though usually JSON unmarshals to zero values)
		if job.ID == "" {
			job.ID = uuid.New().String()
		}
		if job.ScheduleType == "" {
			job.ScheduleType = models.ScheduleTypeCron
		}
		if job.Status == "" {
			job.Status = "idle"
		}

		normalized, err := s.schedule.Normalize(job)
		if err != nil {
			return fmt.Errorf("failed to normalize imported job %s: %w", job.Name, err)
		}
		export.Jobs[index] = normalized
	}

	if err := s.persistence.ImportJobs(export.Jobs); err != nil {
		return err
	}

	// Import defaults if present in the export file
	if export.Defaults != nil {
		_, err := s.defaultsService.UpdateDefaults(export.Defaults)
		if err != nil {
			log.Printf("Warning: Failed to import defaults: %v", err)
			// Continue even if defaults import fails
		}
	}

	if s.scheduler != nil {
		for _, job := range export.Jobs {
			if job.Paused {
				s.scheduler.UnscheduleJob(job.ID)
				continue
			}
			if err := s.scheduler.ScheduleJob(job); err != nil {
				return fmt.Errorf("failed to schedule imported job %s: %w", job.Name, err)
			}
		}
	}

	return nil
}
