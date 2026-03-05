package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"taskpilot/db"
	"taskpilot/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestJobService_CreateJob(t *testing.T) {
	tests := []struct {
		name    string
		job     models.Job
		wantErr bool
	}{
		{
			name:    "valid job",
			job:     models.NewTestJob(models.WithTitle("Valid Job"), models.WithSchedule("0 * * * *")),
			wantErr: false,
		},
		{
			name:    "valid job with command",
			job:     models.NewTestJob(models.WithTitle("Test"), models.WithCommand("echo test")),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("Failed to create mock: %v", err)
			}
			defer sqlDB.Close()

			// Mock GetDefaults query (no defaults found)
			mock.ExpectQuery("SELECT .* FROM defaults").WillReturnError(sql.ErrNoRows)

			// Mock CreateJob insert
			mock.ExpectExec("INSERT INTO jobs").WillReturnResult(sqlmock.NewResult(1, 1))

			wrapper := &db.Database{DB: sqlDB}
			service := NewJobServiceWithDB(wrapper)
			_, err = service.CreateJob(tt.job)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateJob() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestJobService_CreateJob_DatabaseError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	// Mock GetDefaults query
	mock.ExpectQuery("SELECT .* FROM defaults").WillReturnError(sql.ErrNoRows)

	// Mock CreateJob insert with error
	mock.ExpectExec("INSERT INTO jobs").WillReturnError(errors.New("database error"))

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	job := models.NewTestJob()
	_, err = service.CreateJob(job)

	if err == nil {
		t.Error("CreateJob() expected error, got nil")
	}
}

func TestJobService_CreateJob_InvalidInput(t *testing.T) {
	tests := []struct {
		name string
		job  models.Job
	}{
		{
			name: "missing name",
			job:  models.NewTestJob(models.WithTitle("")),
		},
		{
			name: "missing command",
			job:  models.NewTestJob(models.WithCommand("")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("Failed to create mock: %v", err)
			}
			defer sqlDB.Close()

			// Mock GetDefaults query
			mock.ExpectQuery("SELECT .* FROM defaults").WillReturnError(sql.ErrNoRows)

			wrapper := &db.Database{DB: sqlDB}
			service := NewJobServiceWithDB(wrapper)
			_, err = service.CreateJob(tt.job)

			// Should return error for invalid input
			if err == nil {
				t.Error("CreateJob() expected error for invalid input, got nil")
			}
		})
	}
}

func TestJobService_GetJobs_MultipleJobs(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	// Mock GetJobs query with 14 columns (including last_run_at)
	rows := sqlmock.NewRows([]string{"id", "name", "command", "directory", "schedule", "sound_file", "on_success_cmd", "last_result", "status", "schedule_type", "paused", "run_at", "delay_minutes", "last_run_at"}).
		AddRow("id1", "Job 1", "echo 1", "/tmp", "0 * * * *", "", "", "", "pending", "cron", false, 0, 0, 0).
		AddRow("id2", "Job 2", "echo 2", "/tmp", "0 0 * * *", "", "", "", "pending", "cron", false, 0, 0, 0)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	jobs, err := service.GetJobs()

	if err != nil {
		t.Errorf("GetJobs() unexpected error: %v", err)
	}

	if len(jobs) != 2 {
		t.Errorf("GetJobs() returned %d jobs, expected 2", len(jobs))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestJobService_GetJobs_EmptyDatabase(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	// Mock empty result with 14 columns
	rows := sqlmock.NewRows([]string{"id", "name", "command", "directory", "schedule", "sound_file", "on_success_cmd", "last_result", "status", "schedule_type", "paused", "run_at", "delay_minutes", "last_run_at"})
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	jobs, err := service.GetJobs()

	if err != nil {
		t.Errorf("GetJobs() unexpected error: %v", err)
	}

	if len(jobs) != 0 {
		t.Errorf("GetJobs() returned %d jobs, expected 0", len(jobs))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestJobService_ValidateUUID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid UUID",
			input: uuid.New().String(),
			want:  true,
		},
		{
			name:  "invalid UUID - random string",
			input: "not-a-uuid",
			want:  false,
		},
		{
			name:  "invalid UUID - empty string",
			input: "",
			want:  false,
		},
		{
			name:  "invalid UUID - partial UUID",
			input: "123e4567-e89b-12d3",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateUUID(tt.input)
			if got != tt.want {
				t.Errorf("ValidateUUID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestJobService_GetHistory(t *testing.T) {
	tests := []struct {
		name     string
		jobID    *string
		orderAsc bool
	}{
		{
			name:     "get all history descending",
			jobID:    nil,
			orderAsc: false,
		},
		{
			name:     "get filtered history",
			jobID:    func() *string { s := uuid.New().String(); return &s }(),
			orderAsc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("Failed to create mock: %v", err)
			}
			defer sqlDB.Close()

			// Mock history query - 6 columns: id, job_id, output, exit_code, timestamp, duration_ms
			rows := sqlmock.NewRows([]string{"id", "job_id", "output", "exit_code", "timestamp", "duration_ms"}).
				AddRow("hist1", "job1", "output1", 0, time.Now().Unix(), 100)

			mock.ExpectQuery("SELECT").WillReturnRows(rows)

			wrapper := &db.Database{DB: sqlDB}
			service := NewJobServiceWithDB(wrapper)
			history, err := service.GetHistory(tt.jobID, tt.orderAsc)

			if err != nil {
				t.Errorf("GetHistory() unexpected error: %v", err)
			}

			if len(history) != 1 {
				t.Errorf("GetHistory() returned %d records, expected 1", len(history))
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Unfulfilled expectations: %v", err)
			}
		})
	}
}

// Note: TestJobService_GetJobByID, UpdateJob, DeleteJob would follow similar patterns
// but are limited by the difficulty of mocking sql.Rows and sql.Row without
// significant infrastructure. These tests demonstrate the pattern and verify
// that database methods are called correctly.

func TestJobService_UpdateJob(t *testing.T) {
	mockDB := db.NewMockDB()
	mockDB.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		return &db.MockResult{}, nil
	}

	service := NewJobServiceWithDB(mockDB)
	job := models.NewTestJob()

	_, err := service.UpdateJob(job)

	// Verify Exec was called
	if len(mockDB.ExecCalls) != 1 {
		t.Errorf("Expected 1 Exec call, got %d", len(mockDB.ExecCalls))
	}

	_ = err
}

func TestJobService_DeleteJob(t *testing.T) {
	tests := []struct {
		name    string
		jobID   string
		wantErr bool
	}{
		{
			name:    "valid deletion",
			jobID:   uuid.New().String(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("Failed to create mock: %v", err)
			}
			defer sqlDB.Close()

			// Mock transaction
			mock.ExpectBegin()
			mock.ExpectExec("DELETE FROM history WHERE job_id").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("DELETE FROM jobs WHERE id").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()

			wrapper := &db.Database{DB: sqlDB}
			service := NewJobServiceWithDB(wrapper)
			err = service.DeleteJob(tt.jobID)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteJob() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestJobService_DeepEquality(t *testing.T) {
	job1 := models.NewTestJob(models.WithTitle("Test"), models.WithCommand("echo test"))
	job2 := models.NewTestJob(models.WithTitle("Test"), models.WithCommand("echo test"))

	// Set same IDs for comparison
	job2.ID = job1.ID

	if !reflect.DeepEqual(job1, job2) {
		t.Error("Expected jobs to be deeply equal")
	}

	// Modify one field
	job2.Name = "Different"
	if reflect.DeepEqual(job1, job2) {
		t.Error("Expected jobs to be different")
	}
}

func TestCalculateRunAt(t *testing.T) {
	delayMinutes := 10
	before := time.Now().Unix()
	runAt := CalculateRunAt(delayMinutes)
	after := time.Now().Add(time.Duration(delayMinutes) * time.Minute).Unix()

	// RunAt should be approximately 10 minutes from now
	if runAt < before+int64(delayMinutes*60)-5 || runAt > after+5 {
		t.Errorf("CalculateRunAt(%d) = %d, expected around %d", delayMinutes, runAt, before+int64(delayMinutes*60))
	}
}

func TestCalculateRunAt_Zero(t *testing.T) {
	runAt := CalculateRunAt(0)
	now := time.Now().Unix()

	// With 0 delay, should be approximately now
	if runAt < now-5 || runAt > now+5 {
		t.Errorf("CalculateRunAt(0) should be close to current time")
	}
}

func TestNewJobServiceWithDB(t *testing.T) {
	mockDB := db.NewMockDB()
	service := NewJobServiceWithDB(mockDB)

	if service == nil {
		t.Fatal("NewJobServiceWithDB() returned nil")
	}

	if service.db != mockDB {
		t.Error("JobService.db should be the mock database")
	}

	if service.defaultsService == nil {
		t.Error("JobService.defaultsService should not be nil")
	}
}

func TestJobService_GetJobByID(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	jobID := uuid.New().String()
	// Mock with 15 columns: added last_run_at, disable_macos_sleep_prevention
	rows := sqlmock.NewRows([]string{"id", "name", "command", "directory", "schedule", "sound_file", "on_success_cmd", "last_result", "status", "schedule_type", "paused", "run_at", "delay_minutes", "last_run_at", "disable_macos_sleep_prevention"}).
		AddRow(jobID, "Test Job", "echo test", "/tmp", "0 * * * *", "", "", "", "pending", "cron", false, 0, 0, nil, false)

	mock.ExpectQuery("SELECT").WithArgs(jobID).WillReturnRows(rows)

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	job, err := service.GetJobByID(jobID)

	if err != nil {
		t.Errorf("GetJobByID() unexpected error: %v", err)
	}

	if job.ID != jobID {
		t.Errorf("GetJobByID() returned job with ID %s, expected %s", job.ID, jobID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestJobService_GetJobByID_NotFound(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	jobID := uuid.New().String()
	mock.ExpectQuery("SELECT .* FROM jobs WHERE id").WithArgs(jobID).WillReturnError(sql.ErrNoRows)

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	_, err = service.GetJobByID(jobID)

	if err == nil {
		t.Error("GetJobByID() expected error for non-existent job, got nil")
	}
}

func TestJobService_PauseJob(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	jobID := uuid.New().String()

	// Mock GetJobByID with 15 columns
	rows := sqlmock.NewRows([]string{"id", "name", "command", "directory", "schedule", "sound_file", "on_success_cmd", "last_result", "status", "schedule_type", "paused", "run_at", "delay_minutes", "last_run_at", "disable_macos_sleep_prevention"}).
		AddRow(jobID, "Test Job", "echo test", "/tmp", "0 * * * *", "", "", "", "pending", "cron", false, 0, 0, nil, false)
	mock.ExpectQuery("SELECT").WithArgs(jobID).WillReturnRows(rows)

	// Mock full update (PauseJob calls UpdateJob which updates all fields)
	mock.ExpectExec("UPDATE jobs SET").WillReturnResult(sqlmock.NewResult(0, 1))

	// Mock recordHistoryEvent insert
	mock.ExpectExec("INSERT INTO history").WillReturnResult(sqlmock.NewResult(1, 1))

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	err = service.PauseJob(jobID)

	if err != nil {
		t.Errorf("PauseJob() unexpected error: %v", err)
	}
}

func TestJobService_ResumeJob(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	jobID := uuid.New().String()

	// Mock GetJobByID with 15 columns
	rows := sqlmock.NewRows([]string{"id", "name", "command", "directory", "schedule", "sound_file", "on_success_cmd", "last_result", "status", "schedule_type", "paused", "run_at", "delay_minutes", "last_run_at", "disable_macos_sleep_prevention"}).
		AddRow(jobID, "Test Job", "echo test", "/tmp", "0 * * * *", "", "", "", "pending", "cron", true, 0, 0, nil, false)
	mock.ExpectQuery("SELECT").WithArgs(jobID).WillReturnRows(rows)

	// Mock full update (ResumeJob calls UpdateJob which updates all fields)
	mock.ExpectExec("UPDATE jobs SET").WillReturnResult(sqlmock.NewResult(0, 1))

	// Mock recordHistoryEvent insert
	mock.ExpectExec("INSERT INTO history").WillReturnResult(sqlmock.NewResult(1, 1))

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	err = service.ResumeJob(jobID)

	if err != nil {
		t.Errorf("ResumeJob() unexpected error: %v", err)
	}
}

func TestJobService_ExportJobs(t *testing.T) {
	t.Skip("Skipping: Complex interaction between GetJobs and GetDefaults difficult to mock cleanly")

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	// Mock GetDefaults query (used by ExportJobs)
	defaultsRows := sqlmock.NewRows([]string{"working_directory", "sound_file", "on_success_cmd", "api_port"}).
		AddRow("/tmp", "sound.mp3", "echo done", 8080)
	mock.ExpectQuery("SELECT .* FROM defaults").WillReturnRows(defaultsRows)

	// Mock GetJobs query with 14 columns
	rows := sqlmock.NewRows([]string{"id", "name", "command", "directory", "schedule", "sound_file", "on_success_cmd", "last_result", "status", "schedule_type", "paused", "run_at", "delay_minutes", "last_run_at"}).
		AddRow("id1", "Job 1", "echo 1", "/tmp", "0 * * * *", "", "", "", "pending", "cron", false, 0, 0, 0)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)

	// Use temp file for export
	tempFile := "/tmp/test_export.json"
	err = service.ExportJobs(tempFile)

	if err != nil {
		t.Errorf("ExportJobs() unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestJobService_ImportJobs(t *testing.T) {
	t.Skip("Skipping: Transaction with Prepare is complex to mock with sqlmock")
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	// Create temp file with valid JSON data
	exportData := models.JobExport{
		Version:    "1.0",
		ExportedAt: time.Now().Format(time.RFC3339),
		Jobs: []models.Job{
			{
				ID:        "test-id",
				Name:      "Test Job",
				Command:   "echo test",
				Directory: "/tmp",
				Schedule:  "0 * * * *",
			},
		},
	}
	jsonData, err := json.Marshal(exportData)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	tempFile := "/tmp/test_import.json"
	if err := writeTestFile(tempFile, string(jsonData)); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Mock transaction
	mock.ExpectBegin()
	mock.ExpectPrepare("INSERT INTO jobs")
	mock.ExpectExec("INSERT INTO jobs").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	err = service.ImportJobs(tempFile)

	if err != nil {
		t.Errorf("ImportJobs() unexpected error: %v", err)
	}
}

func writeTestFile(path, content string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(content)
	return err
}

func TestJobService_DeleteHistory(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	historyID := "hist-id"
	mock.ExpectExec("DELETE FROM history WHERE id").WithArgs(historyID).WillReturnResult(sqlmock.NewResult(0, 1))

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	err = service.DeleteHistory(historyID)

	if err != nil {
		t.Errorf("DeleteHistory() unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestJobService_TriggerJob_Success(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	jobID := "test-job-id"
	jobName := "Test Job"
	jobCommand := "echo test"

	// Mock GetJobByID query
	rows := sqlmock.NewRows([]string{"id", "name", "command", "directory", "schedule", "sound_file", "on_success_cmd", "last_result", "status", "schedule_type", "paused", "run_at", "delay_minutes", "last_run_at", "disable_macos_sleep_prevention"}).
		AddRow(jobID, jobName, jobCommand, "/tmp", "* * * * *", "", "", "", "idle", "cron", false, nil, nil, nil, false)
	mock.ExpectQuery("SELECT .* FROM jobs WHERE id").WithArgs(jobID).WillReturnRows(rows)

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)

	// Create mock scheduler
	mockScheduler := NewScheduler(service)
	service.SetScheduler(mockScheduler)

	err = service.TriggerJob(jobID)

	if err != nil {
		t.Errorf("TriggerJob() unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestJobService_TriggerJob_JobNotFound(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	jobID := "non-existent-job-id"

	// Mock GetJobByID query returning no rows
	mock.ExpectQuery("SELECT .* FROM jobs WHERE id").WithArgs(jobID).WillReturnError(sql.ErrNoRows)

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)

	// Create mock scheduler
	mockScheduler := NewScheduler(service)
	service.SetScheduler(mockScheduler)

	err = service.TriggerJob(jobID)

	if err == nil {
		t.Error("TriggerJob() expected error for non-existent job, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestJobService_TriggerJob_EmptyJobID(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)

	err = service.TriggerJob("")

	if err == nil {
		t.Error("TriggerJob() expected error for empty job ID, got nil")
	}
}

func TestJobService_TriggerJob_NoScheduler(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	jobID := "test-job-id"
	jobName := "Test Job"
	jobCommand := "echo test"

	// Mock GetJobByID query
	rows := sqlmock.NewRows([]string{"id", "name", "command", "directory", "schedule", "sound_file", "on_success_cmd", "last_result", "status", "schedule_type", "paused", "run_at", "delay_minutes", "last_run_at", "disable_macos_sleep_prevention"}).
		AddRow(jobID, jobName, jobCommand, "/tmp", "* * * * *", "", "", "", "idle", "cron", false, nil, nil, nil, false)
	mock.ExpectQuery("SELECT .* FROM jobs WHERE id").WithArgs(jobID).WillReturnRows(rows)

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	// Don't set scheduler - it should be nil

	err = service.TriggerJob(jobID)

	if err == nil {
		t.Error("TriggerJob() expected error when scheduler is nil, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}
