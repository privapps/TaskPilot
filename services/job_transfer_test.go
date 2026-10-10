package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"taskpilot/db"
	"taskpilot/models"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func newJobTransferTestService(t *testing.T) (*sql.DB, *JobService) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

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
			disable_macos_sleep_prevention BOOLEAN DEFAULT 0,
			timeout_minutes INTEGER
		)`,
		`CREATE TABLE defaults (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			working_directory TEXT,
			sound_file TEXT,
			on_success_cmd TEXT,
			api_port INTEGER DEFAULT 8080,
			default_timeout_minutes INTEGER NOT NULL DEFAULT 10
		)`,
	}
	for _, statement := range schema {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("create test schema: %v", err)
		}
	}

	return sqlDB, NewJobServiceWithDB(&db.Database{DB: sqlDB})
}

func newTransferTestJob(name, id string, timeoutMinutes *int) models.Job {
	job := models.NewTestJob(
		models.WithTitle(name),
		models.WithID(id),
		models.WithPaused(true),
	)
	job.TimeoutMinutes = timeoutMinutes
	return job
}

func writeJobTransferTestFile(t *testing.T, contents []byte) string {
	t.Helper()
	path := filepath.Join(".", "job_transfer_test_"+uuid.NewString()+".json")
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatalf("write import file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
	})
	return path
}

func setJobTransferTestDefault(t *testing.T, sqlDB *sql.DB, timeoutMinutes int) {
	t.Helper()
	_, err := sqlDB.Exec(
		`INSERT OR REPLACE INTO defaults (id, working_directory, sound_file, on_success_cmd, api_port, default_timeout_minutes)
		 VALUES (1, ?, ?, ?, ?, ?)`,
		".", "", "", 8080, timeoutMinutes,
	)
	if err != nil {
		t.Fatalf("set test default: %v", err)
	}
}

func TestJobServiceExportImportRoundTripsTimeouts(t *testing.T) {
	sourceDB, sourceService := newJobTransferTestService(t)
	targetDB, targetService := newJobTransferTestService(t)

	timeout := 45
	withTimeout := newTransferTestJob("with timeout", "with-timeout", &timeout)
	if _, err := sourceService.CreateJob(withTimeout); err != nil {
		t.Fatalf("create source job with timeout: %v", err)
	}
	withoutTimeout := newTransferTestJob("without timeout", "without-timeout", nil)
	if _, err := sourceService.CreateJob(withoutTimeout); err != nil {
		t.Fatalf("create source job without timeout: %v", err)
	}
	minimumTimeout := minJobTimeoutMinutes
	if _, err := sourceService.CreateJob(newTransferTestJob("minimum timeout", "minimum-timeout", &minimumTimeout)); err != nil {
		t.Fatalf("create source job with minimum timeout: %v", err)
	}
	maximumTimeout := maxJobTimeoutMinutes
	if _, err := sourceService.CreateJob(newTransferTestJob("maximum timeout", "maximum-timeout", &maximumTimeout)); err != nil {
		t.Fatalf("create source job with maximum timeout: %v", err)
	}
	setJobTransferTestDefault(t, sourceDB, 37)

	exportPath := filepath.Join(".", "job_transfer_test_"+uuid.NewString()+".json")
	t.Cleanup(func() {
		_ = os.Remove(exportPath)
	})
	if err := sourceService.ExportJobs(exportPath); err != nil {
		t.Fatalf("export jobs: %v", err)
	}

	var exported struct {
		Jobs     []map[string]json.RawMessage `json:"jobs"`
		Defaults struct {
			DefaultTimeoutMinutes int `json:"default_timeout_minutes"`
		} `json:"defaults"`
	}
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read exported jobs: %v", err)
	}
	if err := json.Unmarshal(data, &exported); err != nil {
		t.Fatalf("decode exported jobs: %v", err)
	}
	if exported.Defaults.DefaultTimeoutMinutes != 37 {
		t.Errorf("exported default timeout = %d, want 37", exported.Defaults.DefaultTimeoutMinutes)
	}

	exportedTimeouts := make(map[string]string, len(exported.Jobs))
	for _, job := range exported.Jobs {
		var id string
		if err := json.Unmarshal(job["id"], &id); err != nil {
			t.Fatalf("decode exported job ID: %v", err)
		}
		value, exists := job["timeout_minutes"]
		if !exists {
			t.Errorf("exported job %q omitted timeout_minutes", id)
			continue
		}
		exportedTimeouts[id] = string(value)
	}
	if exportedTimeouts["with-timeout"] != "45" {
		t.Errorf("exported timeout = %q, want 45", exportedTimeouts["with-timeout"])
	}
	if exportedTimeouts["without-timeout"] != "null" {
		t.Errorf("exported unset timeout = %q, want null", exportedTimeouts["without-timeout"])
	}
	if exportedTimeouts["minimum-timeout"] != "1" {
		t.Errorf("exported minimum timeout = %q, want 1", exportedTimeouts["minimum-timeout"])
	}
	if exportedTimeouts["maximum-timeout"] != "1440" {
		t.Errorf("exported maximum timeout = %q, want 1440", exportedTimeouts["maximum-timeout"])
	}

	if err := targetService.ImportJobs(exportPath); err != nil {
		t.Fatalf("import exported jobs: %v", err)
	}
	imported, err := targetService.GetJobByID("with-timeout")
	if err != nil {
		t.Fatalf("get imported job: %v", err)
	}
	if imported.TimeoutMinutes == nil || *imported.TimeoutMinutes != 45 {
		t.Errorf("imported timeout = %v, want 45", imported.TimeoutMinutes)
	}
	importedUnset, err := targetService.GetJobByID("without-timeout")
	if err != nil {
		t.Fatalf("get imported job without timeout: %v", err)
	}
	if importedUnset.TimeoutMinutes != nil {
		t.Errorf("imported unset timeout = %v, want nil", *importedUnset.TimeoutMinutes)
	}
	for id, want := range map[string]int{"minimum-timeout": minJobTimeoutMinutes, "maximum-timeout": maxJobTimeoutMinutes} {
		importedBoundary, err := targetService.GetJobByID(id)
		if err != nil {
			t.Fatalf("get imported boundary job %q: %v", id, err)
		}
		if importedBoundary.TimeoutMinutes == nil || *importedBoundary.TimeoutMinutes != want {
			t.Errorf("imported timeout for %q = %v, want %d", id, importedBoundary.TimeoutMinutes, want)
		}
	}
	var importedDefault int
	if err := targetDB.QueryRow(`SELECT default_timeout_minutes FROM defaults WHERE id = 1`).Scan(&importedDefault); err != nil {
		t.Fatalf("read imported default timeout: %v", err)
	}
	if importedDefault != 37 {
		t.Errorf("imported default timeout = %d, want 37", importedDefault)
	}
}

func TestJobServiceImportJobsAcceptsLegacyTimeoutFieldsAsUnset(t *testing.T) {
	sqlDB, service := newJobTransferTestService(t)
	legacy := `{
		"version":"1.0",
		"exported_at":"2026-09-29T00:00:00Z",
		"jobs":[{
			"id":"legacy-job",
			"name":"legacy",
			"command":"echo legacy",
			"directory":".",
			"schedule":"0 * * * *",
			"schedule_type":"cron",
			"paused":true
		}],
		"defaults":{
			"working_directory":".",
			"sound_file":"",
			"on_success_cmd":"",
			"api_port":8080
		}
	}`
	if err := service.ImportJobs(writeJobTransferTestFile(t, []byte(legacy))); err != nil {
		t.Fatalf("import legacy export: %v", err)
	}

	job, err := service.GetJobByID("legacy-job")
	if err != nil {
		t.Fatalf("get legacy job: %v", err)
	}
	if job.TimeoutMinutes != nil {
		t.Errorf("legacy job timeout = %v, want nil", *job.TimeoutMinutes)
	}
	var importedDefault int
	if err := sqlDB.QueryRow(`SELECT default_timeout_minutes FROM defaults WHERE id = 1`).Scan(&importedDefault); err != nil {
		t.Fatalf("read imported legacy default: %v", err)
	}
	if importedDefault != 10 {
		t.Errorf("legacy default timeout = %d, want built-in 10", importedDefault)
	}
}

func TestJobServiceImportJobsRejectsInvalidTimeoutsWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		jobJSON    string
		defaultTo  int
		defaultKey string
	}{
		{name: "job timeout zero", jobJSON: `"timeout_minutes":0`, defaultTo: 66},
		{name: "job timeout negative", jobJSON: `"timeout_minutes":-1`, defaultTo: 66},
		{name: "job timeout above maximum", jobJSON: `"timeout_minutes":1441`, defaultTo: 66},
		{name: "job timeout fractional", jobJSON: `"timeout_minutes":1.5`, defaultTo: 66},
		{name: "default timeout zero", jobJSON: `"timeout_minutes":55`, defaultTo: 0},
		{name: "default timeout zero with case variation", jobJSON: `"timeout_minutes":55`, defaultTo: 0, defaultKey: "Default_Timeout_Minutes"},
		{name: "default timeout negative", jobJSON: `"timeout_minutes":55`, defaultTo: -1},
		{name: "default timeout above maximum", jobJSON: `"timeout_minutes":55`, defaultTo: 1441},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sqlDB, service := newJobTransferTestService(t)
			initialTimeout := 31
			existing := newTransferTestJob("existing", "existing-job", &initialTimeout)
			if _, err := service.CreateJob(existing); err != nil {
				t.Fatalf("create existing job: %v", err)
			}
			setJobTransferTestDefault(t, sqlDB, 24)

			importData := fmt.Sprintf(`{
				"version":"1.0",
				"exported_at":"2026-09-29T00:00:00Z",
				"jobs":[{
					"id":"existing-job",
					"name":"changed",
					"command":"echo changed",
					"directory":".",
					"schedule":"0 * * * *",
					"schedule_type":"cron",
					"paused":true,
					%s
				},{
					"id":"new-job",
					"name":"new",
					"command":"echo new",
					"directory":".",
					"schedule":"0 * * * *",
					"schedule_type":"cron",
					"paused":true
				}],
				"defaults":{
					"working_directory":".",
					"sound_file":"",
					"on_success_cmd":"",
					"api_port":8080,
					"%s":%d
				}
			}`, test.jobJSON, testDefaultTimeoutKey(test.defaultKey), test.defaultTo)
			importErr := service.ImportJobs(writeJobTransferTestFile(t, []byte(importData)))
			if importErr == nil {
				t.Fatal("ImportJobs() succeeded for an invalid timeout")
			}

			gotExisting, err := service.GetJobByID("existing-job")
			if err != nil {
				t.Fatalf("get existing job after rejected import: %v", err)
			}
			if gotExisting.Name != "existing" {
				t.Errorf("existing job name = %q, want unchanged name %q", gotExisting.Name, "existing")
			}
			if gotExisting.TimeoutMinutes == nil || *gotExisting.TimeoutMinutes != initialTimeout {
				t.Errorf("existing job timeout = %v, want unchanged value %d", gotExisting.TimeoutMinutes, initialTimeout)
			}
			if _, err := service.GetJobByID("new-job"); err == nil {
				t.Error("invalid import partially inserted a new job")
			}
			var storedDefault int
			if err := sqlDB.QueryRow(`SELECT default_timeout_minutes FROM defaults WHERE id = 1`).Scan(&storedDefault); err != nil {
				t.Fatalf("read default after rejected import: %v", err)
			}
			if storedDefault != 24 {
				t.Errorf("default timeout = %d, want unchanged value 24", storedDefault)
			}

			if strings.Contains(test.name, "job timeout") && !strings.Contains(importErr.Error(), "timeout_minutes") {
				t.Errorf("invalid job timeout error = %q, want timeout_minutes validation", importErr)
			}
			if strings.Contains(test.name, "default timeout") && !strings.Contains(importErr.Error(), "default_timeout_minutes") {
				t.Errorf("invalid default timeout error = %q, want default_timeout_minutes validation", importErr)
			}
		})
	}
}

func testDefaultTimeoutKey(key string) string {
	if key == "" {
		return "default_timeout_minutes"
	}
	return key
}

func TestJobServiceDuplicateJobCopiesTimeout(t *testing.T) {
	_, service := newJobTransferTestService(t)
	timeout := 73
	original := newTransferTestJob("original", "original-job", &timeout)
	if _, err := service.CreateJob(original); err != nil {
		t.Fatalf("create original job: %v", err)
	}

	duplicate, err := service.DuplicateJob(original.ID)
	if err != nil {
		t.Fatalf("duplicate job: %v", err)
	}
	if duplicate.ID == original.ID {
		t.Fatal("duplicate reused the original job ID")
	}
	if duplicate.TimeoutMinutes == nil || *duplicate.TimeoutMinutes != timeout {
		t.Errorf("duplicate timeout = %v, want %d", duplicate.TimeoutMinutes, timeout)
	}
	persisted, err := service.GetJobByID(duplicate.ID)
	if err != nil {
		t.Fatalf("get duplicated job: %v", err)
	}
	if persisted.TimeoutMinutes == nil || *persisted.TimeoutMinutes != timeout {
		t.Errorf("persisted duplicate timeout = %v, want %d", persisted.TimeoutMinutes, timeout)
	}
}
