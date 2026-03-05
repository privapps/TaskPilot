package services

import (
	"database/sql"
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
	scheduler       *Scheduler
	defaultsService *DefaultsService
}

func NewJobService() *JobService {
	return &JobService{
		db:              &db.Database{DB: db.DB},
		defaultsService: NewDefaultsService(),
	}
}

func NewJobServiceWithDB(database db.DatabaseInterface) *JobService {
	return &JobService{
		db:              database,
		defaultsService: NewDefaultsServiceWithDB(database),
	}
}

func (s *JobService) SetScheduler(scheduler *Scheduler) {
	s.scheduler = scheduler
}

func (s *JobService) GetJobs() ([]models.Job, error) {
	query := `SELECT 
		j.id, j.name, j.command, j.directory, j.schedule, j.sound_file, j.on_success_cmd, j.last_result, j.status, 
		COALESCE(j.schedule_type, 'cron'), COALESCE(j.paused, 0), j.run_at, j.delay_minutes, j.last_run_at,
		COALESCE(j.disable_macos_sleep_prevention, 0)
		FROM jobs j
		ORDER BY j.name`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query jobs: %w", err)
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		var job models.Job
		var runAt sql.NullInt64
		var delayMinutes sql.NullInt64
		var lastRunAt sql.NullInt64
		err := rows.Scan(&job.ID, &job.Name, &job.Command, &job.Directory, &job.Schedule,
			&job.SoundFile, &job.OnSuccessCmd, &job.LastResult, &job.Status,
			&job.ScheduleType, &job.Paused, &runAt, &delayMinutes, &lastRunAt,
			&job.DisableMacosSleepPrevention)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}
		if runAt.Valid {
			val := runAt.Int64
			job.RunAt = &val
		}
		if delayMinutes.Valid {
			val := int(delayMinutes.Int64)
			job.DelayMinutes = &val
		}
		if lastRunAt.Valid {
			val := lastRunAt.Int64
			job.LastRunAt = &val
		}
		jobs = append(jobs, job)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating jobs: %w", err)
	}

	return jobs, nil
}

func (s *JobService) CreateJob(job models.Job) (models.Job, error) {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}

	if job.Status == "" {
		job.Status = "idle"
	}

	if job.ScheduleType == "" {
		job.ScheduleType = models.ScheduleTypeCron
	}

	// For immediate execution, set schedule to placeholder since it's not used
	if job.ScheduleType == models.ScheduleTypeImmediate && job.Schedule == "" {
		job.Schedule = "immediate"
	}

	// Apply defaults to empty fields
	if err := s.defaultsService.ApplyDefaultsToJob(&job); err != nil {
		log.Printf("Warning: Failed to apply defaults: %v", err)
		// Continue with job creation even if defaults fail
	}

	// Validate job schedule fields
	if err := ValidateJob(job); err != nil {
		return models.Job{}, err
	}

	// Calculate run_at for delay-based scheduling
	if job.ScheduleType == models.ScheduleTypeDelay && job.DelayMinutes != nil {
		runAt := CalculateRunAt(*job.DelayMinutes)
		job.RunAt = &runAt
		log.Printf("CreateJob: Calculated run_at=%d for delay job '%s' (delay_minutes=%d, current_time=%d)",
			runAt, job.Name, *job.DelayMinutes, time.Now().Unix())
	}

	// For datetime jobs, derive run_at from the schedule string if run_at is not already set.
	// The AI (and other clients) naturally pass an ISO 8601 datetime in schedule.
	if job.ScheduleType == models.ScheduleTypeDatetime && job.RunAt == nil && job.Schedule != "" {
		formats := []string{
			time.RFC3339,
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04",
		}
		for _, format := range formats {
			if t, err := time.ParseInLocation(format, job.Schedule, time.Local); err == nil {
				runAt := t.Unix()
				job.RunAt = &runAt
				log.Printf("CreateJob: Parsed run_at=%d from schedule '%s' for datetime job '%s'",
					runAt, job.Schedule, job.Name)
				break
			}
		}
		if job.RunAt == nil {
			return models.Job{}, fmt.Errorf("datetime job requires a valid ISO 8601 datetime in schedule field, got: %s", job.Schedule)
		}
	}

	// Validate datetime run_at is in the future
	if job.ScheduleType == models.ScheduleTypeDatetime && job.RunAt != nil {
		log.Printf("CreateJob: Datetime job '%s' with run_at=%d (current_time=%d, in %d seconds)",
			job.Name, *job.RunAt, time.Now().Unix(), *job.RunAt-time.Now().Unix())
	}

	query := `INSERT INTO jobs (id, name, command, directory, schedule, sound_file, on_success_cmd, last_result, status, schedule_type, paused, run_at, delay_minutes, last_run_at, disable_macos_sleep_prevention) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.Exec(query, job.ID, job.Name, job.Command, job.Directory, job.Schedule,
		job.SoundFile, job.OnSuccessCmd, job.LastResult, job.Status, job.ScheduleType, job.Paused, job.RunAt, job.DelayMinutes, job.LastRunAt, job.DisableMacosSleepPrevention)
	if err != nil {
		return models.Job{}, fmt.Errorf("failed to create job: %w", err)
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
	if err := ValidateJob(job); err != nil {
		return models.Job{}, err
	}
	return s.updateJob(job, true)
}

func (s *JobService) updateJob(job models.Job, notifyScheduler bool) (models.Job, error) {
	if job.ID == "" {
		return models.Job{}, errors.New("job ID is required")
	}

	// Recalculate run_at for delay-based scheduling (similar to CreateJob)
	if job.ScheduleType == models.ScheduleTypeDelay && job.DelayMinutes != nil {
		runAt := CalculateRunAt(*job.DelayMinutes)
		job.RunAt = &runAt
		log.Printf("UpdateJob: Calculated run_at=%d for delay job '%s' (delay_minutes=%d, current_time=%d)",
			runAt, job.Name, *job.DelayMinutes, time.Now().Unix())
	}

	if job.ScheduleType == models.ScheduleTypeDatetime && job.RunAt != nil {
		log.Printf("UpdateJob: Datetime job '%s' with run_at=%d (current_time=%d, in %d seconds)",
			job.Name, *job.RunAt, time.Now().Unix(), *job.RunAt-time.Now().Unix())
	}

	query := `UPDATE jobs SET name = ?, command = ?, directory = ?, schedule = ?, sound_file = ?, on_success_cmd = ?, last_result = ?, status = ?, 
		schedule_type = ?, paused = ?, run_at = ?, delay_minutes = ?, last_run_at = ?, disable_macos_sleep_prevention = ? WHERE id = ?`

	result, err := s.db.Exec(query, job.Name, job.Command, job.Directory, job.Schedule, job.SoundFile, job.OnSuccessCmd, job.LastResult, job.Status,
		job.ScheduleType, job.Paused, job.RunAt, job.DelayMinutes, job.LastRunAt, job.DisableMacosSleepPrevention, job.ID)
	if err != nil {
		return models.Job{}, fmt.Errorf("failed to update job: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return models.Job{}, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return models.Job{}, fmt.Errorf("job with ID %s not found", job.ID)
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

	// Begin transaction for atomic cascade delete
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Safe to call even after commit

	// Delete all history records for this job first
	historyQuery := `DELETE FROM history WHERE job_id = ?`
	if _, err := tx.Exec(historyQuery, id); err != nil {
		return fmt.Errorf("failed to delete job history: %w", err)
	}

	// Then delete the job
	jobQuery := `DELETE FROM jobs WHERE id = ?`
	result, err := tx.Exec(jobQuery, id)
	if err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("job with ID %s not found", id)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (s *JobService) GetJobByID(id string) (models.Job, error) {
	query := `SELECT id, name, command, directory, schedule, sound_file, on_success_cmd, last_result, status,
		COALESCE(schedule_type, 'cron'), COALESCE(paused, 0), run_at, delay_minutes, last_run_at,
		COALESCE(disable_macos_sleep_prevention, 0)
		FROM jobs WHERE id = ?`

	var job models.Job
	var runAt sql.NullInt64
	var delayMinutes sql.NullInt64
	var lastRunAt sql.NullInt64
	err := s.db.QueryRow(query, id).Scan(&job.ID, &job.Name, &job.Command, &job.Directory, &job.Schedule,
		&job.SoundFile, &job.OnSuccessCmd, &job.LastResult, &job.Status,
		&job.ScheduleType, &job.Paused, &runAt, &delayMinutes, &lastRunAt, &job.DisableMacosSleepPrevention)

	if err == sql.ErrNoRows {
		return models.Job{}, fmt.Errorf("job with ID %s not found", id)
	}
	if err != nil {
		return models.Job{}, fmt.Errorf("failed to get job: %w", err)
	}

	if runAt.Valid {
		val := runAt.Int64
		job.RunAt = &val
	}
	if delayMinutes.Valid {
		val := int(delayMinutes.Int64)
		job.DelayMinutes = &val
	}
	if lastRunAt.Valid {
		val := lastRunAt.Int64
		job.LastRunAt = &val
	}

	return job, nil
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
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, job_id, output, exit_code, 
	              CAST(timestamp AS INTEGER) as timestamp, 
	              duration_ms 
	          FROM history 
	          WHERE job_id = ? 
	          ORDER BY id DESC 
	          LIMIT ?`

	rows, err := s.db.Query(query, jobID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	defer rows.Close()

	var history []models.History
	for rows.Next() {
		var h models.History
		var durationMs sql.NullInt64
		var timestamp sql.NullInt64
		err := rows.Scan(&h.ID, &h.JobID, &h.Output, &h.ExitCode, &timestamp, &durationMs)
		if err != nil {
			return nil, fmt.Errorf("failed to scan history: %w", err)
		}
		if timestamp.Valid {
			h.Timestamp = timestamp.Int64
		}
		if durationMs.Valid {
			h.DurationMs = durationMs.Int64
		}
		history = append(history, h)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating history: %w", err)
	}

	return history, nil
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
	orderDir := "DESC"
	if orderAsc {
		orderDir = "ASC"
	}

	query := `SELECT id, job_id, output, exit_code, 
	              CAST(timestamp AS INTEGER) as timestamp, 
	              duration_ms 
	          FROM history 
	          WHERE (? IS NULL OR job_id = ?) 
	          ORDER BY timestamp ` + orderDir

	rows, err := s.db.Query(query, jobID, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	defer rows.Close()

	var history []models.History
	for rows.Next() {
		var h models.History
		var durationMs sql.NullInt64
		var timestamp sql.NullInt64
		err := rows.Scan(&h.ID, &h.JobID, &h.Output, &h.ExitCode, &timestamp, &durationMs)
		if err != nil {
			return nil, fmt.Errorf("failed to scan history: %w", err)
		}
		if timestamp.Valid {
			h.Timestamp = timestamp.Int64
		}
		if durationMs.Valid {
			h.DurationMs = durationMs.Int64
		}
		history = append(history, h)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating history: %w", err)
	}

	return history, nil
}

// DeleteHistory deletes a single history record by ID
func (s *JobService) DeleteHistory(historyID string) error {
	if historyID == "" {
		return errors.New("history ID is required")
	}

	query := `DELETE FROM history WHERE id = ?`
	result, err := s.db.Exec(query, historyID)
	if err != nil {
		return fmt.Errorf("failed to delete history: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("history record with ID %s not found", historyID)
	}

	return nil
}

// CalculateRunAt calculates the run_at timestamp for delay-based scheduling
func CalculateRunAt(delayMinutes int) int64 {
	return time.Now().Add(time.Duration(delayMinutes) * time.Minute).Unix()
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
		// For one-time jobs with past run_at, execute immediately
		if (job.ScheduleType == models.ScheduleTypeDelay || job.ScheduleType == models.ScheduleTypeDatetime) &&
			job.RunAt != nil && *job.RunAt < time.Now().Unix() {
			go s.scheduler.runJob(job)
		} else {
			if err := s.scheduler.ScheduleJob(job); err != nil {
				return fmt.Errorf("failed to reschedule job: %w", err)
			}
		}
	}

	// Record resume event in history
	s.recordHistoryEvent(jobID, "Job resumed")

	return nil
}

// DeleteJobHistory deletes a single history entry
func (s *JobService) DeleteJobHistory(historyID string) error {
	log.Printf("DeleteJobHistory called with historyID='%s' (type: %T, len: %d)", historyID, historyID, len(historyID))

	// Debug: Check what IDs exist in the database
	var existingIDs []string
	rows, err := s.db.Query("SELECT id FROM history LIMIT 10")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				existingIDs = append(existingIDs, id)
			}
		}
		log.Printf("Sample history IDs in database: %v", existingIDs)
	}

	query := `DELETE FROM history WHERE id = ?`
	result, err := s.db.Exec(query, historyID)
	if err != nil {
		log.Printf("DeleteJobHistory failed: %v", err)
		return fmt.Errorf("failed to delete history: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("DeleteJobHistory - failed to get rows affected: %v", err)
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	log.Printf("DeleteJobHistory - rows affected: %d", rowsAffected)

	if rowsAffected == 0 {
		return fmt.Errorf("history entry %s not found", historyID)
	}

	return nil
}

// DeleteAllJobHistory deletes all history entries for a specific job
func (s *JobService) DeleteAllJobHistory(jobID string) error {
	log.Printf("DeleteAllJobHistory called with jobID='%s'", jobID)

	// Check how many entries exist
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM history WHERE job_id = ?", jobID).Scan(&count)
	log.Printf("DeleteAllJobHistory - found %d entries for job_id '%s'", count, jobID)

	query := `DELETE FROM history WHERE job_id = ?`
	result, err := s.db.Exec(query, jobID)
	if err != nil {
		log.Printf("DeleteAllJobHistory failed: %v", err)
		return fmt.Errorf("failed to delete job history: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("DeleteAllJobHistory - deleted %d rows", rowsAffected)

	return nil
}

// DeleteAllHistory deletes all history entries across all jobs
func (s *JobService) DeleteAllHistory() error {
	query := `DELETE FROM history`
	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to delete all history: %w", err)
	}

	return nil
}

// recordHistoryEvent records a pause/resume event in the history
func (s *JobService) recordHistoryEvent(jobID, message string) {
	query := `INSERT INTO history (id, job_id, output, exit_code, timestamp, duration_ms) VALUES (?, ?, ?, ?, ?, ?)`
	id := uuid.New().String()
	timestamp := time.Now().Unix()
	_, _ = s.db.Exec(query, id, jobID, message, 0, timestamp, 0)
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

	// Insert into database
	query := `INSERT INTO jobs (id, name, command, directory, schedule, sound_file, on_success_cmd, last_result, status, schedule_type, paused, run_at, delay_minutes, last_run_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.Exec(query, newJob.ID, newJob.Name, newJob.Command, newJob.Directory, newJob.Schedule,
		newJob.SoundFile, newJob.OnSuccessCmd, newJob.LastResult, newJob.Status, newJob.ScheduleType, newJob.Paused, newJob.RunAt, newJob.DelayMinutes, newJob.LastRunAt)
	if err != nil {
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

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `INSERT INTO jobs (id, name, command, directory, schedule, sound_file, on_success_cmd, last_result, status, schedule_type, paused, run_at, delay_minutes) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		name=excluded.name,
		command=excluded.command,
		directory=excluded.directory,
		schedule=excluded.schedule,
		sound_file=excluded.sound_file,
		on_success_cmd=excluded.on_success_cmd,
		last_result=excluded.last_result,
		status=excluded.status,
		schedule_type=excluded.schedule_type,
		paused=excluded.paused,
		run_at=excluded.run_at,
		delay_minutes=excluded.delay_minutes`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, job := range export.Jobs {
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

		_, err = stmt.Exec(
			job.ID, job.Name, job.Command, job.Directory, job.Schedule,
			job.SoundFile, job.OnSuccessCmd, job.LastResult, job.Status,
			job.ScheduleType, job.Paused, job.RunAt, job.DelayMinutes,
		)
		if err != nil {
			return fmt.Errorf("failed to import job %s: %w", job.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Import defaults if present in the export file
	if export.Defaults != nil {
		_, err := s.defaultsService.UpdateDefaults(export.Defaults)
		if err != nil {
			log.Printf("Warning: Failed to import defaults: %v", err)
			// Continue even if defaults import fails
		}
	}

	// Reschedule jobs if strict scheduling logic is needed, but mostly the scheduler polls DB.
	// If the scheduler relies on in-memory state, we might need to notify it.
	// Assuming scheduler polls or we should trigger reload.
	// If scheduler polls, it picks up changes. If checking DB every minute, it's fine.
	// If needed, s.scheduler.Reload() or similar.
	// Based on earlier context, `scheduler` is injected into `JobService`.
	// Let's check if scheduler has a Reload method or similar, but for now assuming polling or next tick picks up.
	// Actually, `Scheduler` struct isn't fully visible here but `s.scheduler` is available.

	return nil
}
