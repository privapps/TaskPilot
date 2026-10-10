package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"taskpilot/models"
)

func TestSchedulerResolvesTimeoutFromJobThenGlobalThenBuiltInDefault(t *testing.T) {
	tests := []struct {
		name                 string
		storedDefaultMinutes int
		jobTimeoutMinutes    *int
		wantTimeoutMinutes   int
	}{
		{
			name:                 "per-job value overrides stored default",
			storedDefaultMinutes: 3,
			jobTimeoutMinutes:    intPointer(1),
			wantTimeoutMinutes:   1,
		},
		{
			name:                 "stored default applies without job override",
			storedDefaultMinutes: 2,
			wantTimeoutMinutes:   2,
		},
		{
			name:               "missing stored default uses built-in ten minutes",
			wantTimeoutMinutes: 10,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, jobService, scheduler := newSchedulerTestHarness(t)
			scheduler.timeoutMinuteDuration = testTimeoutMinute
			if test.storedDefaultMinutes > 0 {
				defaults := models.NewTestDefaults()
				defaults.DefaultTimeoutMinutes = test.storedDefaultMinutes
				if _, err := jobService.defaultsService.UpdateDefaults(&defaults); err != nil {
					t.Fatalf("UpdateDefaults() failed: %v", err)
				}
			}

			job := models.NewTestJob(
				models.WithTitle("resolved-timeout"),
				models.WithCommand("sleep 5"),
				models.WithDirectory("."),
			)
			job.TimeoutMinutes = test.jobTimeoutMinutes
			created, err := jobService.CreateJob(job)
			if err != nil {
				t.Fatalf("CreateJob() failed: %v", err)
			}
			stored, err := jobService.GetJobByID(created.ID)
			if err != nil {
				t.Fatalf("GetJobByID() failed: %v", err)
			}
			if test.jobTimeoutMinutes == nil && stored.TimeoutMinutes != nil {
				t.Errorf("CreateJob() copied the global default into timeout_minutes: %d", *stored.TimeoutMinutes)
			}
			if test.jobTimeoutMinutes != nil &&
				(stored.TimeoutMinutes == nil || *stored.TimeoutMinutes != *test.jobTimeoutMinutes) {
				t.Errorf("stored timeout_minutes = %v, want per-job value %d", stored.TimeoutMinutes, *test.jobTimeoutMinutes)
			}

			if err := jobService.TriggerJob(created.ID); err != nil {
				t.Fatalf("TriggerJob() failed: %v", err)
			}
			history := waitForJobHistory(t, jobService, created.ID, 0)
			wantMessage := fmt.Sprintf("job timed out after %d minutes", test.wantTimeoutMinutes)
			if history.ExitCode != jobTimedOutExitCode {
				t.Errorf("history exit code = %d, want %d", history.ExitCode, jobTimedOutExitCode)
			}
			if !strings.Contains(history.Output, wantMessage) {
				t.Errorf("history output %q does not contain %q", history.Output, wantMessage)
			}

			stored, err = jobService.GetJobByID(created.ID)
			if err != nil {
				t.Fatalf("GetJobByID() after run failed: %v", err)
			}
			if test.jobTimeoutMinutes == nil && stored.TimeoutMinutes != nil {
				t.Errorf("running the job copied the global timeout into the job record: %d", *stored.TimeoutMinutes)
			}
		})
	}
}

func TestChangingGlobalTimeoutAppliesToExistingJobOnNextRun(t *testing.T) {
	_, jobService, scheduler := newSchedulerTestHarness(t)
	scheduler.timeoutMinuteDuration = testTimeoutMinute

	job := models.NewTestJob(
		models.WithTitle("existing-job-uses-current-default"),
		models.WithCommand("sleep 5"),
		models.WithDirectory("."),
	)
	created, err := jobService.CreateJob(job)
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}

	defaults := models.NewTestDefaults()
	defaults.DefaultTimeoutMinutes = 2
	if _, err := jobService.defaultsService.UpdateDefaults(&defaults); err != nil {
		t.Fatalf("UpdateDefaults(2) failed: %v", err)
	}
	if err := jobService.TriggerJob(created.ID); err != nil {
		t.Fatalf("TriggerJob() with first default failed: %v", err)
	}
	firstHistory := waitForJobHistory(t, jobService, created.ID, 0)
	if !strings.Contains(firstHistory.Output, "job timed out after 2 minutes") {
		t.Fatalf("first run output %q does not use the stored two-minute default", firstHistory.Output)
	}

	defaults.DefaultTimeoutMinutes = 1
	if _, err := jobService.defaultsService.UpdateDefaults(&defaults); err != nil {
		t.Fatalf("UpdateDefaults(1) failed: %v", err)
	}
	if err := jobService.TriggerJob(created.ID); err != nil {
		t.Fatalf("TriggerJob() after changing the default failed: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var histories []models.History
	for time.Now().Before(deadline) {
		histories, err = jobService.GetJobHistory(created.ID, 10)
		if err == nil && len(histories) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GetJobHistory() failed: %v", err)
	}
	if len(histories) < 2 {
		t.Fatalf("job has %d persisted history entries after two runs, want at least 2", len(histories))
	}
	if !historyContainsTimeout(histories, 2) || !historyContainsTimeout(histories, 1) {
		t.Errorf("persisted run outputs do not show both the old and changed defaults: %#v", histories)
	}

	stored, err := jobService.GetJobByID(created.ID)
	if err != nil {
		t.Fatalf("GetJobByID() failed: %v", err)
	}
	if stored.TimeoutMinutes != nil {
		t.Errorf("global timeout was copied into existing job: %d", *stored.TimeoutMinutes)
	}
}

func historyContainsTimeout(histories []models.History, timeoutMinutes int) bool {
	want := fmt.Sprintf("job timed out after %d minutes", timeoutMinutes)
	for _, history := range histories {
		if strings.Contains(history.Output, want) {
			return true
		}
	}
	return false
}
