package services

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"taskpilot/db"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAPIServer_HandleTriggerJob_Success(t *testing.T) {
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

	// Mock job status update (happens during execution)
	mock.ExpectExec("UPDATE jobs").WillReturnResult(sqlmock.NewResult(1, 1))

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	scheduler := NewScheduler(service)
	service.SetScheduler(scheduler)

	apiServer := NewAPIServer(service, 8080)

	req := httptest.NewRequest(http.MethodPost, "/api/jobs/"+jobID+"/trigger", nil)
	w := httptest.NewRecorder()

	apiServer.handleTriggerJob(w, req, jobID)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["message"] != "Job triggered successfully" {
		t.Errorf("Expected success message, got %s", response["message"])
	}
}

func TestAPIServer_HandleTriggerJob_NotFound(t *testing.T) {
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
	scheduler := NewScheduler(service)
	service.SetScheduler(scheduler)

	apiServer := NewAPIServer(service, 8080)

	req := httptest.NewRequest(http.MethodPost, "/api/jobs/"+jobID+"/trigger", nil)
	w := httptest.NewRecorder()

	apiServer.handleTriggerJob(w, req, jobID)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestAPIServer_HandleJobByID_TriggerRoute(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	jobID := "test-job-id"
	jobName := "Test Job"

	// Mock GetJobByID query
	rows := sqlmock.NewRows([]string{"id", "name", "command", "directory", "schedule", "sound_file", "on_success_cmd", "last_result", "status", "schedule_type", "paused", "run_at", "delay_minutes", "last_run_at", "disable_macos_sleep_prevention"}).
		AddRow(jobID, jobName, "echo test", "/tmp", "* * * * *", "", "", "", "idle", "cron", false, nil, nil, nil, false)
	mock.ExpectQuery("SELECT .* FROM jobs WHERE id").WithArgs(jobID).WillReturnRows(rows)

	// Mock job status update
	mock.ExpectExec("UPDATE jobs").WillReturnResult(sqlmock.NewResult(1, 1))

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	scheduler := NewScheduler(service)
	service.SetScheduler(scheduler)

	apiServer := NewAPIServer(service, 8080)

	req := httptest.NewRequest(http.MethodPost, "/api/jobs/"+jobID+"/trigger", nil)
	w := httptest.NewRecorder()

	apiServer.handleJobByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestAPIServer_HandleJobByID_TriggerRoute_WrongMethod(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)

	apiServer := NewAPIServer(service, 8080)

	jobID := "test-job-id"
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID+"/trigger", nil)
	w := httptest.NewRecorder()

	apiServer.handleJobByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}
