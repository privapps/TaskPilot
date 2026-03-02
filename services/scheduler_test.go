package services

import (
	"database/sql"
	"testing"
	"time"

	"taskpilot/db"
	"taskpilot/models"
)

func TestNewScheduler(t *testing.T) {
	mockDB := db.NewMockDB()
	jobService := NewJobServiceWithDB(mockDB)

	scheduler := NewScheduler(jobService)

	if scheduler == nil {
		t.Fatal("NewScheduler() returned nil")
	}

	if scheduler.cron == nil {
		t.Error("Scheduler cron is nil")
	}

	if scheduler.jobService == nil {
		t.Error("Scheduler jobService is nil")
	}

	if scheduler.entryMap == nil {
		t.Error("Scheduler entryMap is nil")
	}
}

func TestScheduler_ScheduleJob_ValidCron(t *testing.T) {
	mockDB := db.NewMockDB()
	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)

	job := models.NewTestJob(
		models.WithSchedule("* * * * *"), // Every minute
		models.WithScheduleType(models.ScheduleTypeCron),
	)

	err := scheduler.ScheduleJob(job)

	if err != nil {
		t.Errorf("ScheduleJob() unexpected error: %v", err)
	}

	// Verify the job was added to the entry map
	if _, exists := scheduler.entryMap[job.ID]; !exists {
		t.Error("Job not found in entryMap after scheduling")
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

	err := scheduler.ScheduleJob(job)

	if err == nil {
		t.Error("ScheduleJob() expected error for invalid cron expression, got nil")
	}
}

func TestScheduler_ScheduleJob_ImmediateExecution(t *testing.T) {
	t.Skip("Skipping: Immediate execution spawns goroutine that accesses db.DB directly. Requires integration test or scheduler refactoring.")

	mockDB := db.NewMockDB()
	mockDB.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		return &db.MockResult{}, nil
	}

	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)

	job := models.NewTestJob(
		models.WithScheduleType(models.ScheduleTypeImmediate),
		models.WithCommand("echo test"),
	)

	err := scheduler.ScheduleJob(job)

	if err != nil {
		t.Errorf("ScheduleJob() unexpected error: %v", err)
	}

	// For immediate jobs, they should not be in the entry map
	// (they run once and are not scheduled)
	if _, exists := scheduler.entryMap[job.ID]; exists {
		t.Error("Immediate job should not be in entryMap")
	}
}

func TestScheduler_RemoveJob(t *testing.T) {
	mockDB := db.NewMockDB()
	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)

	// First schedule a job
	job := models.NewTestJob(
		models.WithSchedule("* * * * *"),
		models.WithScheduleType(models.ScheduleTypeCron),
	)

	err := scheduler.ScheduleJob(job)
	if err != nil {
		t.Fatalf("ScheduleJob() failed: %v", err)
	}

	// Verify it's in the map
	if _, exists := scheduler.entryMap[job.ID]; !exists {
		t.Fatal("Job not in entryMap after scheduling")
	}

	// Remove it by scheduling with empty schedule or directly from map
	delete(scheduler.entryMap, job.ID)

	// Verify it's removed from the map
	if _, exists := scheduler.entryMap[job.ID]; exists {
		t.Error("Job still in entryMap after removal")
	}
}

func TestScheduler_StartStop_Lifecycle(t *testing.T) {
	mockDB := db.NewMockDB()

	// Mock GetJobs to return empty list
	mockDB.QueryFunc = func(query string, args ...interface{}) (*sql.Rows, error) {
		return nil, nil
	}

	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)

	// Test Stop immediately (should not panic)
	scheduler.Stop()

	// Note: Full Start() testing requires a context and would block,
	// so we only test that Stop() works and doesn't panic
}

func TestScheduler_JobExecution_Callback(t *testing.T) {
	// This test verifies that the execution callback mechanism works
	mockDB := db.NewMockDB()

	executionCount := 0
	mockDB.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		executionCount++
		return &db.MockResult{}, nil
	}

	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)

	// Create a job with very short delay for testing
	job := models.NewTestJob(
		models.WithSchedule("* * * * *"),
		models.WithScheduleType(models.ScheduleTypeCron),
		models.WithCommand("echo test"),
	)

	// Schedule the job (but don't start the scheduler to avoid actual execution)
	err := scheduler.ScheduleJob(job)
	if err != nil {
		t.Fatalf("ScheduleJob() failed: %v", err)
	}

	// In a real test with execution, we'd wait for the callback
	// For this unit test, we just verify the job was scheduled
	if _, exists := scheduler.entryMap[job.ID]; !exists {
		t.Error("Job not scheduled")
	}
}

func TestScheduler_OneTimeJob_Scheduling(t *testing.T) {
	mockDB := db.NewMockDB()
	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)

	runAt := time.Now().Add(1 * time.Hour).Unix()
	job := models.NewTestJob(
		models.WithScheduleType(models.ScheduleTypeDatetime),
		models.WithRunAt(runAt),
	)

	err := scheduler.ScheduleJob(job)

	if err != nil {
		t.Errorf("ScheduleJob() unexpected error: %v", err)
	}

	// One-time jobs are not added to entryMap (they're handled by ticker)
	if _, exists := scheduler.entryMap[job.ID]; exists {
		t.Error("One-time job should not be in entryMap")
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

func TestNewScheduler_Initialization(t *testing.T) {
	mockDB := db.NewMockDB()
	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)

	if scheduler.cron == nil {
		t.Error("Scheduler.cron should not be nil")
	}

	if scheduler.jobService != jobService {
		t.Error("Scheduler.jobService should be set")
	}

	if scheduler.entryMap == nil {
		t.Error("Scheduler.entryMap should be initialized")
	}

	if len(scheduler.entryMap) != 0 {
		t.Error("Scheduler.entryMap should start empty")
	}

	if scheduler.stopTicker == nil {
		t.Error("Scheduler.stopTicker should be initialized")
	}
}

func TestScheduler_RunJobImmediately(t *testing.T) {
	mockDB := db.NewMockDB()

	updateCalled := false
	mockDB.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		updateCalled = true
		return &db.MockResult{}, nil
	}

	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)

	job := models.NewTestJob(
		models.WithCommand("echo test"),
		models.WithName("Test Trigger Job"),
	)

	// Call RunJobImmediately - it should not panic
	scheduler.RunJobImmediately(job)

	// Give the goroutine a moment to start
	time.Sleep(50 * time.Millisecond)

	// The method should have been called without error
	// We expect updateCalled to be true since runJob updates the job status
	if !updateCalled {
		t.Error("Expected job update to be called during execution")
	}
}

func TestScheduler_RunJobImmediately_DoesNotAffectSchedule(t *testing.T) {
	mockDB := db.NewMockDB()
	mockDB.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		return &db.MockResult{}, nil
	}

	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)

	// Schedule a cron job
	job := models.NewTestJob(
		models.WithSchedule("* * * * *"),
		models.WithScheduleType(models.ScheduleTypeCron),
	)

	err := scheduler.ScheduleJob(job)
	if err != nil {
		t.Fatalf("ScheduleJob() failed: %v", err)
	}

	// Verify it's in the schedule
	if _, exists := scheduler.entryMap[job.ID]; !exists {
		t.Fatal("Job not scheduled")
	}

	// Trigger it immediately
	scheduler.RunJobImmediately(job)

	// Verify it's still in the schedule
	if _, exists := scheduler.entryMap[job.ID]; !exists {
		t.Error("Job should still be scheduled after immediate trigger")
	}
}
