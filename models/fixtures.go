package models

import (
	"time"

	"github.com/google/uuid"
)

// TestJobOption is a function that modifies a test job
type TestJobOption func(*Job)

// NewTestJob creates a Job with sensible defaults for testing
func NewTestJob(opts ...TestJobOption) Job {
	job := Job{
		ID:           uuid.New().String(),
		Name:         "Test Job",
		Command:      "echo 'test'",
		Directory:    "/tmp",
		Schedule:     "0 * * * *",
		SoundFile:    "",
		OnSuccessCmd: "",
		LastResult:   "",
		Status:       "idle",
		ScheduleType: ScheduleTypeCron,
		Paused:       false,
		RunAt:        nil,
		DelayMinutes: nil,
		LastRunAt:    nil,
	}

	for _, opt := range opts {
		opt(&job)
	}

	return job
}

// WithID sets the job ID
func WithID(id string) TestJobOption {
	return func(j *Job) {
		j.ID = id
	}
}

// WithTitle sets the job name
func WithTitle(name string) TestJobOption {
	return func(j *Job) {
		j.Name = name
	}
}

// WithCommand sets the command
func WithCommand(command string) TestJobOption {
	return func(j *Job) {
		j.Command = command
	}
}

// WithDirectory sets the working directory
func WithDirectory(dir string) TestJobOption {
	return func(j *Job) {
		j.Directory = dir
	}
}

// WithSchedule sets the schedule
func WithSchedule(schedule string) TestJobOption {
	return func(j *Job) {
		j.Schedule = schedule
	}
}

// WithScheduleType sets the schedule type
func WithScheduleType(scheduleType string) TestJobOption {
	return func(j *Job) {
		j.ScheduleType = scheduleType
	}
}

// WithStatus sets the job status
func WithStatus(status string) TestJobOption {
	return func(j *Job) {
		j.Status = status
	}
}

// WithPaused sets the paused state
func WithPaused(paused bool) TestJobOption {
	return func(j *Job) {
		j.Paused = paused
	}
}

// WithRunAt sets the run at timestamp
func WithRunAt(timestamp int64) TestJobOption {
	return func(j *Job) {
		j.RunAt = &timestamp
	}
}

// WithDelayMinutes sets the delay in minutes
func WithDelayMinutes(minutes int) TestJobOption {
	return func(j *Job) {
		j.DelayMinutes = &minutes
	}
}

// NewTestDefaults creates a Defaults struct for testing
func NewTestDefaults() Defaults {
	return Defaults{
		WorkingDirectory: "/home/user",
		SoundFile:        "",
		OnSuccessCmd:     "",
		APIPort:          8080,
	}
}

// NewTestHistory creates a History record for testing
func NewTestHistory(jobID string) History {
	return History{
		ID:         uuid.New().String(),
		JobID:      jobID,
		Output:     "Test output",
		ExitCode:   0,
		Timestamp:  time.Now().Unix(),
		DurationMs: 1000,
	}
}

// TestHistoryOption is a function that modifies a test history
type TestHistoryOption func(*History)

// NewTestHistoryWithOptions creates a History record with custom options
func NewTestHistoryWithOptions(jobID string, opts ...TestHistoryOption) History {
	history := NewTestHistory(jobID)

	for _, opt := range opts {
		opt(&history)
	}

	return history
}

// WithHistoryOutput sets the history output
func WithHistoryOutput(output string) TestHistoryOption {
	return func(h *History) {
		h.Output = output
	}
}

// WithExitCode sets the exit code
func WithExitCode(code int) TestHistoryOption {
	return func(h *History) {
		h.ExitCode = code
	}
}

// WithTimestamp sets the timestamp
func WithTimestamp(timestamp int64) TestHistoryOption {
	return func(h *History) {
		h.Timestamp = timestamp
	}
}
