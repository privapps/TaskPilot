package services

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"taskpilot/db"
	"taskpilot/models"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRESTJobTimeoutCreateUpdateGetAndSchema(t *testing.T) {
	_, jobService, _ := newSchedulerTestHarness(t)
	apiServer := NewAPIServer(jobService, 8080)

	createRequest := httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(
		`{"name":"REST timeout","command":"echo test","directory":".","schedule":"* * * * *","schedule_type":"cron","timeout_minutes":null}`,
	))
	createResponse := httptest.NewRecorder()
	apiServer.handleJobs(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", createResponse.Code, http.StatusCreated, createResponse.Body.String())
	}

	var created models.Job
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created job: %v", err)
	}
	if created.TimeoutMinutes != nil {
		t.Fatalf("created timeout_minutes = %d, want null", *created.TimeoutMinutes)
	}
	var createdPayload map[string]interface{}
	if err := json.Unmarshal(createResponse.Body.Bytes(), &createdPayload); err != nil {
		t.Fatalf("decode created job payload: %v", err)
	}
	if value, exists := createdPayload["timeout_minutes"]; !exists || value != nil {
		t.Errorf("created response timeout_minutes = %v (present %t), want null", value, exists)
	}

	explicitCreateRequest := httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(
		`{"name":"REST timeout set","command":"echo test","directory":".","schedule":"* * * * *","schedule_type":"cron","timeout_minutes":10}`,
	))
	explicitCreateResponse := httptest.NewRecorder()
	apiServer.handleJobs(explicitCreateResponse, explicitCreateRequest)
	if explicitCreateResponse.Code != http.StatusCreated {
		t.Fatalf("explicit create status = %d, want %d: %s", explicitCreateResponse.Code, http.StatusCreated, explicitCreateResponse.Body.String())
	}
	var explicitlyCreated models.Job
	if err := json.Unmarshal(explicitCreateResponse.Body.Bytes(), &explicitlyCreated); err != nil {
		t.Fatalf("decode explicitly created job: %v", err)
	}
	if explicitlyCreated.TimeoutMinutes == nil || *explicitlyCreated.TimeoutMinutes != 10 {
		t.Fatalf("explicitly created timeout_minutes = %v, want 10", explicitlyCreated.TimeoutMinutes)
	}

	updateRequest := httptest.NewRequest(http.MethodPut, "/api/jobs/"+created.ID, strings.NewReader(
		`{"name":"REST timeout","command":"echo test","directory":".","schedule":"* * * * *","schedule_type":"cron","timeout_minutes":25}`,
	))
	updateResponse := httptest.NewRecorder()
	apiServer.handleJobByID(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d: %s", updateResponse.Code, http.StatusOK, updateResponse.Body.String())
	}

	var updated models.Job
	if err := json.Unmarshal(updateResponse.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated job: %v", err)
	}
	if updated.TimeoutMinutes == nil || *updated.TimeoutMinutes != 25 {
		t.Fatalf("updated timeout_minutes = %v, want 25", updated.TimeoutMinutes)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/jobs/"+created.ID, nil)
	getResponse := httptest.NewRecorder()
	apiServer.handleJobByID(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d: %s", getResponse.Code, http.StatusOK, getResponse.Body.String())
	}
	var fetched models.Job
	if err := json.Unmarshal(getResponse.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode fetched job: %v", err)
	}
	if fetched.TimeoutMinutes == nil || *fetched.TimeoutMinutes != 25 {
		t.Fatalf("fetched timeout_minutes = %v, want 25", fetched.TimeoutMinutes)
	}

	invalidUpdate := httptest.NewRequest(http.MethodPut, "/api/jobs/"+created.ID, strings.NewReader(
		`{"name":"REST timeout","command":"echo test","directory":".","schedule":"* * * * *","schedule_type":"cron","timeout_minutes":0}`,
	))
	invalidUpdateResponse := httptest.NewRecorder()
	apiServer.handleJobByID(invalidUpdateResponse, invalidUpdate)
	if invalidUpdateResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid update status = %d, want %d", invalidUpdateResponse.Code, http.StatusBadRequest)
	}
	if !strings.Contains(invalidUpdateResponse.Body.String(), "timeout_minutes must be between 1 and 1440") {
		t.Errorf("invalid update response %q does not contain the timeout validation error", invalidUpdateResponse.Body.String())
	}
	stored, err := jobService.GetJobByID(created.ID)
	if err != nil {
		t.Fatalf("GetJobByID() failed: %v", err)
	}
	if stored.TimeoutMinutes == nil || *stored.TimeoutMinutes != 25 {
		t.Errorf("invalid REST update persisted timeout_minutes = %v, want 25", stored.TimeoutMinutes)
	}

	clearRequest := httptest.NewRequest(http.MethodPut, "/api/jobs/"+created.ID, strings.NewReader(
		`{"name":"REST timeout","command":"echo test","directory":".","schedule":"* * * * *","schedule_type":"cron","timeout_minutes":null}`,
	))
	clearResponse := httptest.NewRecorder()
	apiServer.handleJobByID(clearResponse, clearRequest)
	if clearResponse.Code != http.StatusOK {
		t.Fatalf("clear timeout status = %d, want %d: %s", clearResponse.Code, http.StatusOK, clearResponse.Body.String())
	}
	var cleared models.Job
	if err := json.Unmarshal(clearResponse.Body.Bytes(), &cleared); err != nil {
		t.Fatalf("decode cleared job: %v", err)
	}
	if cleared.TimeoutMinutes != nil {
		t.Errorf("cleared timeout_minutes = %d, want null", *cleared.TimeoutMinutes)
	}

	invalidCreate := httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(
		`{"name":"Non-integer timeout","command":"echo test","schedule":"* * * * *","timeout_minutes":1.5}`,
	))
	invalidCreateResponse := httptest.NewRecorder()
	apiServer.handleJobs(invalidCreateResponse, invalidCreate)
	if invalidCreateResponse.Code != http.StatusBadRequest {
		t.Fatalf("non-integer create status = %d, want %d", invalidCreateResponse.Code, http.StatusBadRequest)
	}
	if !strings.Contains(invalidCreateResponse.Body.String(), "timeout_minutes") {
		t.Errorf("non-integer create response %q does not describe the invalid timeout field", invalidCreateResponse.Body.String())
	}
	jobs, err := jobService.GetJobs()
	if err != nil {
		t.Fatalf("GetJobs() failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("invalid REST create left %d jobs, want 2", len(jobs))
	}

	swaggerResponse := httptest.NewRecorder()
	apiServer.handleSwagger(swaggerResponse, httptest.NewRequest(http.MethodGet, "/api/swagger", nil))
	if swaggerResponse.Code != http.StatusOK {
		t.Fatalf("swagger status = %d, want %d", swaggerResponse.Code, http.StatusOK)
	}
	var openAPI map[string]interface{}
	if err := json.Unmarshal(swaggerResponse.Body.Bytes(), &openAPI); err != nil {
		t.Fatalf("decode swagger schema: %v", err)
	}
	components := openAPI["components"].(map[string]interface{})
	schemas := components["schemas"].(map[string]interface{})
	jobSchema := schemas["Job"].(map[string]interface{})
	properties := jobSchema["properties"].(map[string]interface{})
	timeoutSchema, exists := properties["timeout_minutes"].(map[string]interface{})
	if !exists {
		t.Fatal("REST job schema does not document timeout_minutes")
	}
	if timeoutSchema["type"] != "integer" || timeoutSchema["nullable"] != true ||
		timeoutSchema["minimum"] != float64(1) || timeoutSchema["maximum"] != float64(1440) {
		t.Errorf("timeout_minutes schema = %#v, want nullable integer from 1 to 1440", timeoutSchema)
	}
}

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
	rows := sqlmock.NewRows([]string{"id", "name", "command", "directory", "schedule", "sound_file", "on_success_cmd", "last_result", "status", "schedule_type", "paused", "run_at", "delay_minutes", "last_run_at", "next_run_at", "last_scheduled_at", "disable_macos_sleep_prevention", "timeout_minutes"}).
		AddRow(jobID, jobName, jobCommand, "/tmp", "* * * * *", "", "", "", "idle", "cron", false, nil, nil, nil, nil, nil, false, nil)
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
	rows := sqlmock.NewRows([]string{"id", "name", "command", "directory", "schedule", "sound_file", "on_success_cmd", "last_result", "status", "schedule_type", "paused", "run_at", "delay_minutes", "last_run_at", "next_run_at", "last_scheduled_at", "disable_macos_sleep_prevention", "timeout_minutes"}).
		AddRow(jobID, jobName, "echo test", "/tmp", "* * * * *", "", "", "", "idle", "cron", false, nil, nil, nil, nil, nil, false, nil)
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

func TestAPIServer_HandleSystemTimezones_OK(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	apiServer := NewAPIServer(service, 8080)

	req := httptest.NewRequest(http.MethodGet, "/api/system/timezones", nil)
	w := httptest.NewRecorder()

	apiServer.handleSystemTimezones(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp TimezonesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// current timezone must be non-empty
	if resp.Current == "" {
		t.Error("Expected non-empty current timezone")
	}

	// list must contain well-known entries
	found := false
	for _, tz := range resp.Timezones {
		if tz == "UTC" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected UTC in timezones list")
	}

	if len(resp.Timezones) < 100 {
		t.Errorf("Expected at least 100 timezones, got %d", len(resp.Timezones))
	}
}

func TestAPIServer_HandleSystemTimezones_MethodNotAllowed(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	apiServer := NewAPIServer(service, 8080)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/system/timezones", nil)
		w := httptest.NewRecorder()
		apiServer.handleSystemTimezones(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, w.Code)
		}
	}
}

func TestAPIServer_HandleSystemTimezones_ValidIANA(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	wrapper := &db.Database{DB: sqlDB}
	service := NewJobServiceWithDB(wrapper)
	apiServer := NewAPIServer(service, 8080)

	req := httptest.NewRequest(http.MethodGet, "/api/system/timezones", nil)
	w := httptest.NewRecorder()
	apiServer.handleSystemTimezones(w, req)

	var resp TimezonesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Spot-check a handful of well-known IANA names are present
	must := []string{"America/New_York", "America/Los_Angeles", "Europe/London", "Asia/Tokyo", "Australia/Sydney", "UTC"}
	tzSet := make(map[string]bool, len(resp.Timezones))
	for _, tz := range resp.Timezones {
		tzSet[tz] = true
	}
	for _, want := range must {
		if !tzSet[want] {
			t.Errorf("Expected %q in timezones list", want)
		}
	}
}
