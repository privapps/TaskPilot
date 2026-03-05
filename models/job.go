package models

// ScheduleType constants for different scheduling modes
const (
	ScheduleTypeCron      = "cron"
	ScheduleTypeDelay     = "delay"
	ScheduleTypeDatetime  = "datetime"
	ScheduleTypeImmediate = "immediate"
)

// Job represents a scheduled task
type Job struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Command      string `json:"command"`
	Directory    string `json:"directory"`
	Schedule     string `json:"schedule"`
	SoundFile    string `json:"sound_file"`
	OnSuccessCmd string `json:"on_success_cmd"`
	LastResult   string `json:"last_result"`
	Status       string `json:"status"`
	ScheduleType string `json:"schedule_type"`
	Paused       bool   `json:"paused"`
	RunAt                      *int64 `json:"run_at,omitempty"`                        // Unix timestamp for one-time execution
	DelayMinutes               *int   `json:"delay_minutes,omitempty"`                 // Delay in minutes for delay-based scheduling
	LastRunAt                  *int64 `json:"last_run_at,omitempty"`                   // Unix timestamp of most recent execution
	DisableMacosSleepPrevention bool  `json:"disable_macos_sleep_prevention,omitempty"` // Skip caffeinate wrap and pmset wake on macOS
}

// History represents a job execution record
type History struct {
	ID         string `json:"id"`
	JobID      string `json:"job_id"`
	Output     string `json:"output"`
	ExitCode   int    `json:"exit_code"`
	Timestamp  int64  `json:"timestamp"`
	DurationMs int64  `json:"duration_ms"`
}

// Defaults represents default values for job fields
type Defaults struct {
	WorkingDirectory string `json:"working_directory"`
	SoundFile        string `json:"sound_file"`
	OnSuccessCmd     string `json:"on_success_cmd"`
	APIPort          int    `json:"api_port"`
}

// JobExport represents the structure of an exported job file
type JobExport struct {
	Version    string    `json:"version"`
	ExportedAt string    `json:"exported_at"`
	Jobs       []Job     `json:"jobs"`
	Defaults   *Defaults `json:"defaults,omitempty"`
}
