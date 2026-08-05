package services

import (
	"database/sql"
	"fmt"

	"taskpilot/db"
	"taskpilot/models"
)

const jobSelectColumns = `j.id, j.name, j.command, j.directory, j.schedule, j.sound_file, j.on_success_cmd, j.last_result, j.status,
	COALESCE(j.schedule_type, 'cron'), COALESCE(j.paused, 0), j.run_at, j.delay_minutes, j.last_run_at, j.next_run_at, j.last_scheduled_at,
	COALESCE(j.disable_macos_sleep_prevention, 0)`

const jobSelectColumnsByID = `id, name, command, directory, schedule, sound_file, on_success_cmd, last_result, status,
	COALESCE(schedule_type, 'cron'), COALESCE(paused, 0), run_at, delay_minutes, last_run_at, next_run_at, last_scheduled_at,
	COALESCE(disable_macos_sleep_prevention, 0)`

const historySelectColumns = `id, job_id, output, exit_code,
	CAST(timestamp AS INTEGER) as timestamp,
	duration_ms,
	scheduled_at,
	COALESCE(trigger_type, '')`

type rowScanner interface {
	Scan(dest ...interface{}) error
}

// JobPersistence is the deep persistence module for jobs and execution history.
// Callers work with models and state transitions rather than SQL row shape.
type JobPersistence struct {
	db db.DatabaseInterface
}

func NewJobPersistence(database db.DatabaseInterface) *JobPersistence {
	return &JobPersistence{db: database}
}

func (p *JobPersistence) ListJobs() ([]models.Job, error) {
	rows, err := p.db.Query(`SELECT ` + jobSelectColumns + ` FROM jobs j ORDER BY j.name`)
	if err != nil {
		return nil, fmt.Errorf("failed to query jobs: %w", err)
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating jobs: %w", err)
	}
	return jobs, nil
}

func (p *JobPersistence) GetJob(id string) (models.Job, error) {
	row := p.db.QueryRow(`SELECT `+jobSelectColumnsByID+` FROM jobs WHERE id = ?`, id)
	if row == nil {
		return models.Job{}, fmt.Errorf("job with ID %s not found", id)
	}

	job, err := scanJob(row)
	if err == sql.ErrNoRows {
		return models.Job{}, fmt.Errorf("job with ID %s not found", id)
	}
	if err != nil {
		return models.Job{}, fmt.Errorf("failed to get job: %w", err)
	}
	return job, nil
}

func (p *JobPersistence) InsertJob(job models.Job) error {
	_, err := p.db.Exec(`INSERT INTO jobs (id, name, command, directory, schedule, sound_file, on_success_cmd, last_result, status, schedule_type, paused, run_at, delay_minutes, last_run_at, next_run_at, last_scheduled_at, disable_macos_sleep_prevention)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, jobArgs(job)...)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}
	return nil
}

func (p *JobPersistence) UpdateJob(job models.Job) error {
	result, err := p.db.Exec(`UPDATE jobs SET name = ?, command = ?, directory = ?, schedule = ?, sound_file = ?, on_success_cmd = ?, last_result = ?, status = ?,
		schedule_type = ?, paused = ?, run_at = ?, delay_minutes = ?, last_run_at = ?, next_run_at = ?, last_scheduled_at = ?, disable_macos_sleep_prevention = ? WHERE id = ?`,
		job.Name, job.Command, job.Directory, job.Schedule, job.SoundFile, job.OnSuccessCmd, job.LastResult, job.Status,
		job.ScheduleType, job.Paused, job.RunAt, job.DelayMinutes, job.LastRunAt, job.NextRunAt, job.LastScheduledAt, job.DisableMacosSleepPrevention, job.ID)
	if err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("job with ID %s not found", job.ID)
	}
	return nil
}

func (p *JobPersistence) DeleteJob(id string) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM history WHERE job_id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete job history: %w", err)
	}

	result, err := tx.Exec(`DELETE FROM jobs WHERE id = ?`, id)
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
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (p *JobPersistence) ImportJobs(jobs []models.Job) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`INSERT INTO jobs (id, name, command, directory, schedule, sound_file, on_success_cmd, last_result, status, schedule_type, paused, run_at, delay_minutes, last_run_at, next_run_at, last_scheduled_at, disable_macos_sleep_prevention)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		delay_minutes=excluded.delay_minutes,
		last_run_at=excluded.last_run_at,
		next_run_at=excluded.next_run_at,
		last_scheduled_at=excluded.last_scheduled_at,
		disable_macos_sleep_prevention=excluded.disable_macos_sleep_prevention`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, job := range jobs {
		if _, err := stmt.Exec(jobArgs(job)...); err != nil {
			return fmt.Errorf("failed to import job %s: %w", job.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (p *JobPersistence) ListJobHistoryPage(jobID string, limit, offset int) ([]models.History, error) {
	rows, err := p.db.Query(`SELECT `+historySelectColumns+` FROM history
		WHERE job_id = ?
		ORDER BY timestamp DESC, id DESC
		LIMIT ? OFFSET ?`, jobID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	return scanHistoryRows(rows)
}

func (p *JobPersistence) ListHistory(jobID *string, orderAsc bool) ([]models.History, error) {
	orderDir := "DESC"
	if orderAsc {
		orderDir = "ASC"
	}
	rows, err := p.db.Query(`SELECT `+historySelectColumns+` FROM history
		WHERE (? IS NULL OR job_id = ?)
		ORDER BY timestamp `+orderDir, jobID, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	return scanHistoryRows(rows)
}

func (p *JobPersistence) DeleteHistory(historyID string) error {
	result, err := p.db.Exec(`DELETE FROM history WHERE id = ?`, historyID)
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

func (p *JobPersistence) DeleteAllJobHistory(jobID string) error {
	if _, err := p.db.Exec(`DELETE FROM history WHERE job_id = ?`, jobID); err != nil {
		return fmt.Errorf("failed to delete job history: %w", err)
	}
	return nil
}

func (p *JobPersistence) DeleteAllHistory() error {
	if _, err := p.db.Exec(`DELETE FROM history`); err != nil {
		return fmt.Errorf("failed to delete all history: %w", err)
	}
	return nil
}

func (p *JobPersistence) RecordHistory(history models.History) error {
	if history.ID == "" {
		return fmt.Errorf("history ID is required")
	}
	_, err := p.db.Exec(`INSERT INTO history (id, job_id, output, exit_code, timestamp, duration_ms, scheduled_at, trigger_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		history.ID, history.JobID, history.Output, history.ExitCode, history.Timestamp,
		history.DurationMs, history.ScheduledAt, history.TriggerType)
	if err != nil {
		return fmt.Errorf("failed to create history entry: %w", err)
	}
	return nil
}

func (p *JobPersistence) SetNextRun(jobID string, nextRunAt *int64) error {
	_, err := p.db.Exec(`UPDATE jobs SET next_run_at = ? WHERE id = ?`, nextRunAt, jobID)
	return err
}

func (p *JobPersistence) ClaimScheduledExecution(job models.Job, scheduledAt int64, nextRunAt *int64, pauseOnClaim bool) (bool, error) {
	result, err := p.db.Exec(`UPDATE jobs
		SET status = ?, next_run_at = ?, last_scheduled_at = ?,
		    paused = CASE WHEN ? = 1 THEN 1 ELSE paused END
		WHERE id = ? AND paused = 0 AND next_run_at = ?`,
		"running", nextRunAt, scheduledAt, boolToInt(pauseOnClaim), job.ID, scheduledAt)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func (p *JobPersistence) SkipScheduledExecution(job models.Job, scheduledAt int64, nextRunAt *int64, pauseJob bool) (bool, error) {
	result, err := p.db.Exec(`UPDATE jobs
		SET status = ?, last_result = ?, next_run_at = ?, last_scheduled_at = ?,
		    paused = CASE WHEN ? = 1 THEN 1 ELSE paused END
		WHERE id = ? AND paused = 0 AND next_run_at = ?`,
		"idle", "missed", nextRunAt, scheduledAt, boolToInt(pauseJob), job.ID, scheduledAt)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func scanJob(scanner rowScanner) (models.Job, error) {
	var job models.Job
	var runAt, delayMinutes, lastRunAt, nextRunAt, lastScheduledAt sql.NullInt64
	err := scanner.Scan(
		&job.ID, &job.Name, &job.Command, &job.Directory, &job.Schedule,
		&job.SoundFile, &job.OnSuccessCmd, &job.LastResult, &job.Status,
		&job.ScheduleType, &job.Paused, &runAt, &delayMinutes, &lastRunAt,
		&nextRunAt, &lastScheduledAt, &job.DisableMacosSleepPrevention,
	)
	if err != nil {
		return models.Job{}, err
	}
	job.RunAt = nullableInt64(runAt)
	if delayMinutes.Valid {
		value := int(delayMinutes.Int64)
		job.DelayMinutes = &value
	}
	job.LastRunAt = nullableInt64(lastRunAt)
	job.NextRunAt = nullableInt64(nextRunAt)
	job.LastScheduledAt = nullableInt64(lastScheduledAt)
	return job, nil
}

func scanHistoryRows(rows *sql.Rows) ([]models.History, error) {
	defer rows.Close()
	var history []models.History
	for rows.Next() {
		var entry models.History
		var durationMs, timestamp, scheduledAt sql.NullInt64
		if err := rows.Scan(&entry.ID, &entry.JobID, &entry.Output, &entry.ExitCode,
			&timestamp, &durationMs, &scheduledAt, &entry.TriggerType); err != nil {
			return nil, fmt.Errorf("failed to scan history: %w", err)
		}
		if timestamp.Valid {
			entry.Timestamp = timestamp.Int64
		}
		if durationMs.Valid {
			entry.DurationMs = durationMs.Int64
		}
		entry.ScheduledAt = nullableInt64(scheduledAt)
		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating history: %w", err)
	}
	return history, nil
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func jobArgs(job models.Job) []interface{} {
	return []interface{}{
		job.ID, job.Name, job.Command, job.Directory, job.Schedule,
		job.SoundFile, job.OnSuccessCmd, job.LastResult, job.Status,
		job.ScheduleType, job.Paused, job.RunAt, job.DelayMinutes,
		job.LastRunAt, job.NextRunAt, job.LastScheduledAt,
		job.DisableMacosSleepPrevention,
	}
}
