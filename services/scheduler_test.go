package services

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"taskpilot/db"
	"taskpilot/models"

	_ "modernc.org/sqlite"
)

func newSchedulerTestHarness(t *testing.T) (*sql.DB, *JobService, *Scheduler) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "taskpilot.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	schema := []string{
		`CREATE TABLE jobs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			command TEXT NOT NULL,
			directory TEXT,
			schedule TEXT NOT NULL,
			sound_file TEXT,
			on_success_cmd TEXT,
			last_result TEXT,
			status TEXT DEFAULT 'idle',
			schedule_type TEXT DEFAULT 'cron',
			paused BOOLEAN DEFAULT 0,
			run_at INTEGER,
			delay_minutes INTEGER,
			last_run_at INTEGER,
			next_run_at INTEGER,
			last_scheduled_at INTEGER,
			disable_macos_sleep_prevention BOOLEAN DEFAULT 0
		)`,
		`CREATE TABLE history (
			id TEXT PRIMARY KEY,
			job_id TEXT NOT NULL,
			output TEXT,
			exit_code INTEGER,
			timestamp INTEGER,
			duration_ms INTEGER,
			scheduled_at INTEGER,
			trigger_type TEXT
		)`,
		`CREATE TABLE defaults (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			working_directory TEXT,
			sound_file TEXT,
			on_success_cmd TEXT,
			api_port INTEGER DEFAULT 8080
		)`,
	}

	for _, stmt := range schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("apply schema: %v", err)
		}
	}

	jobService := NewJobServiceWithDB(&db.Database{DB: sqlDB})
	scheduler := NewScheduler(jobService)
	jobService.SetScheduler(scheduler)

	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return sqlDB, jobService, scheduler
}

func insertSchedulerTestJob(t *testing.T, sqlDB *sql.DB, job models.Job) {
	t.Helper()
	_, err := sqlDB.Exec(
		`INSERT INTO jobs (
			id, name, command, directory, schedule, sound_file, on_success_cmd, last_result,
			status, schedule_type, paused, run_at, delay_minutes, last_run_at, next_run_at,
			last_scheduled_at, disable_macos_sleep_prevention
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Name, job.Command, job.Directory, job.Schedule, job.SoundFile, job.OnSuccessCmd, job.LastResult,
		job.Status, job.ScheduleType, job.Paused, job.RunAt, job.DelayMinutes, job.LastRunAt, job.NextRunAt,
		job.LastScheduledAt, job.DisableMacosSleepPrevention,
	)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
}

func TestNewScheduler(t *testing.T) {
	mockDB := db.NewMockDB()
	jobService := NewJobServiceWithDB(mockDB)

	scheduler := NewScheduler(jobService)

	if scheduler == nil {
		t.Fatal("NewScheduler() returned nil")
	}
	if scheduler.jobService == nil {
		t.Error("Scheduler jobService is nil")
	}
	if scheduler.stopTicker == nil {
		t.Error("Scheduler stopTicker is nil")
	}
	if scheduler.wakeMap == nil {
		t.Error("Scheduler wakeMap is nil")
	}
}

func TestScheduler_ScheduleJob_ValidCron(t *testing.T) {
	_, jobService, scheduler := newSchedulerTestHarness(t)

	job := models.NewTestJob(
		models.WithSchedule("* * * * *"),
		models.WithScheduleType(models.ScheduleTypeCron),
	)
	insertSchedulerTestJob(t, jobService.db.(*db.Database).DB, job)

	if err := scheduler.ScheduleJob(job); err != nil {
		t.Fatalf("ScheduleJob() unexpected error: %v", err)
	}

	stored, err := jobService.GetJobByID(job.ID)
	if err != nil {
		t.Fatalf("GetJobByID() failed: %v", err)
	}
	if stored.NextRunAt == nil {
		t.Fatal("expected next_run_at to be persisted")
	}
	if *stored.NextRunAt <= time.Now().Unix() {
		t.Errorf("expected next_run_at in the future, got %d", *stored.NextRunAt)
	}
}

func TestScheduler_ScheduleJob_InvalidCron(t *testing.T) {
	mockDB := db.NewMockDB()
	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)

	job := models.NewTestJob(
		models.WithSchedule("invalid cron expression"),
		models.WithScheduleType(models.ScheduleTypeCron),
	)

	if err := scheduler.ScheduleJob(job); err == nil {
		t.Error("ScheduleJob() expected error for invalid cron expression, got nil")
	}
}

func TestScheduler_RunJobImmediately_DoesNotAffectSchedule(t *testing.T) {
	_, jobService, scheduler := newSchedulerTestHarness(t)

	nextRunAt := time.Now().Add(30 * time.Minute).Unix()
	job := models.NewTestJob(
		models.WithSchedule("* * * * *"),
		models.WithScheduleType(models.ScheduleTypeCron),
		models.WithCommand("printf manual"),
	)
	job.NextRunAt = &nextRunAt
	insertSchedulerTestJob(t, jobService.db.(*db.Database).DB, job)

	scheduler.RunJobImmediately(job)
	time.Sleep(250 * time.Millisecond)

	stored, err := jobService.GetJobByID(job.ID)
	if err != nil {
		t.Fatalf("GetJobByID() failed: %v", err)
	}
	if stored.NextRunAt == nil {
		t.Fatal("expected next_run_at to remain set")
	}
	if *stored.NextRunAt != nextRunAt {
		t.Errorf("expected next_run_at %d, got %d", nextRunAt, *stored.NextRunAt)
	}
	if stored.LastResult != "success" {
		t.Errorf("expected last_result success, got %q", stored.LastResult)
	}
}

func TestScheduler_SkipOverdueCronJob(t *testing.T) {
	_, jobService, scheduler := newSchedulerTestHarness(t)

	pastDue := time.Now().Add(-3 * time.Hour).Unix()
	job := models.NewTestJob(
		models.WithSchedule("*/5 * * * *"),
		models.WithScheduleType(models.ScheduleTypeCron),
	)
	job.NextRunAt = &pastDue
	insertSchedulerTestJob(t, jobService.db.(*db.Database).DB, job)

	if err := scheduler.processDueJobs(time.Now()); err != nil {
		t.Fatalf("processDueJobs() failed: %v", err)
	}

	stored, err := jobService.GetJobByID(job.ID)
	if err != nil {
		t.Fatalf("GetJobByID() failed: %v", err)
	}
	if stored.LastResult != "missed" {
		t.Errorf("expected last_result missed, got %q", stored.LastResult)
	}
	if stored.NextRunAt == nil || *stored.NextRunAt <= time.Now().Unix() {
		t.Fatalf("expected future next_run_at after skip, got %+v", stored.NextRunAt)
	}

	history, err := jobService.GetJobHistory(job.ID, 10)
	if err != nil {
		t.Fatalf("GetJobHistory() failed: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected skip history entry")
	}
	if history[0].TriggerType != triggerTypeSkipped {
		t.Errorf("expected trigger_type %q, got %q", triggerTypeSkipped, history[0].TriggerType)
	}
	if history[0].ScheduledAt == nil || *history[0].ScheduledAt != pastDue {
		t.Errorf("expected scheduled_at %d, got %+v", pastDue, history[0].ScheduledAt)
	}
}

func TestScheduler_RunDueOneTimeJobPausesAfterExecution(t *testing.T) {
	_, jobService, scheduler := newSchedulerTestHarness(t)

	runAt := time.Now().Add(-30 * time.Second).Unix()
	job := models.NewTestJob(
		models.WithScheduleType(models.ScheduleTypeDatetime),
		models.WithRunAt(runAt),
		models.WithCommand("printf one-time"),
	)
	job.NextRunAt = &runAt
	insertSchedulerTestJob(t, jobService.db.(*db.Database).DB, job)

	if err := scheduler.processDueJobs(time.Now()); err != nil {
		t.Fatalf("processDueJobs() failed: %v", err)
	}

	time.Sleep(250 * time.Millisecond)

	stored, err := jobService.GetJobByID(job.ID)
	if err != nil {
		t.Fatalf("GetJobByID() failed: %v", err)
	}
	if !stored.Paused {
		t.Error("expected one-time job to be paused after execution")
	}
	if stored.Status != "idle" {
		t.Errorf("expected status idle, got %q", stored.Status)
	}
	if stored.LastResult != "success" {
		t.Errorf("expected last_result success, got %q", stored.LastResult)
	}
	if stored.NextRunAt != nil {
		t.Errorf("expected next_run_at cleared, got %+v", stored.NextRunAt)
	}

	history, err := jobService.GetJobHistory(job.ID, 10)
	if err != nil {
		t.Fatalf("GetJobHistory() failed: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected execution history entry")
	}
	if history[0].TriggerType != triggerTypeScheduled {
		t.Errorf("expected trigger_type %q, got %q", triggerTypeScheduled, history[0].TriggerType)
	}
}

func TestGenerateHistoryID(t *testing.T) {
	id1 := generateHistoryID()
	time.Sleep(time.Millisecond)
	id2 := generateHistoryID()

	if id1 == "" {
		t.Error("generateHistoryID() should not return empty string")
	}
	if id1 == id2 {
		t.Error("generateHistoryID() should generate unique IDs")
	}
}

func TestWrapWithCaffeinate_NonDarwin(t *testing.T) {
	original := "echo hello"
	result := wrapWithCaffeinate_forTest(original, false, "linux")
	if result != original {
		t.Errorf("expected original command on linux, got: %s", result)
	}
}

func TestWrapWithCaffeinate_DisabledFlag(t *testing.T) {
	original := "echo hello"
	result := wrapWithCaffeinate_forTest(original, true, "darwin")
	if result != original {
		t.Errorf("expected original command when disabled, got: %s", result)
	}
}

func TestWrapWithCaffeinate_Darwin(t *testing.T) {
	original := "echo hello"
	result := wrapWithCaffeinate_forTest(original, false, "darwin")
	expected := "caffeinate -s -- sh -c 'echo hello'"
	if result != expected {
		t.Errorf("expected caffeinate-wrapped command, got: %s", result)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"echo hello", "'echo hello'"},
		{"it's a test", "'it'\\''s a test'"},
		{"simple", "'simple'"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.expected {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestScheduler_DisableMacosSleepPrevention_NoWakeEvent(t *testing.T) {
	mockDB := db.NewMockDB()
	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)

	job := models.NewTestJob(
		models.WithSchedule("* * * * *"),
		models.WithScheduleType(models.ScheduleTypeCron),
	)
	job.DisableMacosSleepPrevention = true

	scheduler.registerWakeEvent(job, time.Now().Add(10*time.Minute))

	if _, exists := scheduler.wakeMap[job.ID]; exists {
		t.Error("wakeMap should not contain entry for job with DisableMacosSleepPrevention=true")
	}
}

func wrapWithCaffeinate_forTest(cmd string, disabled bool, goos string) string {
	if disabled || goos != "darwin" {
		return cmd
	}
	return "caffeinate -s -- sh -c " + shellQuote(cmd)
}
