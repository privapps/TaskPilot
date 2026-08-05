package services

import (
	"testing"
	"time"

	"taskpilot/models"
)

func TestScheduleSemanticsNormalize(t *testing.T) {
	now := time.Date(2026, time.July, 23, 15, 0, 0, 0, time.UTC)
	semantics := NewScheduleSemanticsWithClock(func() time.Time { return now })

	delay := 5
	tests := []struct {
		name      string
		job       models.Job
		wantType  string
		wantRunAt *int64
		wantSched string
		wantErr   bool
	}{
		{name: "defaults empty type to cron", job: models.Job{Schedule: "0 * * * *"}, wantType: models.ScheduleTypeCron, wantSched: "0 * * * *"},
		{name: "normalizes delay run time", job: models.Job{ScheduleType: models.ScheduleTypeDelay, DelayMinutes: &delay}, wantType: models.ScheduleTypeDelay, wantRunAt: int64Ptr(now.Add(5 * time.Minute).Unix())},
		{name: "parses datetime run time", job: models.Job{ScheduleType: models.ScheduleTypeDatetime, Schedule: "2026-07-23T16:30:00Z"}, wantType: models.ScheduleTypeDatetime, wantSched: "2026-07-23T16:30:00Z", wantRunAt: int64Ptr(time.Date(2026, time.July, 23, 16, 30, 0, 0, time.UTC).Unix())},
		{name: "adds immediate placeholder", job: models.Job{ScheduleType: models.ScheduleTypeImmediate}, wantType: models.ScheduleTypeImmediate, wantSched: models.ScheduleTypeImmediate},
		{name: "rejects invalid cron", job: models.Job{ScheduleType: models.ScheduleTypeCron, Schedule: "not cron"}, wantErr: true},
		{name: "rejects non-positive delay", job: models.Job{ScheduleType: models.ScheduleTypeDelay, DelayMinutes: int64ToIntPtr(0)}, wantErr: true},
		{name: "rejects past datetime", job: models.Job{ScheduleType: models.ScheduleTypeDatetime, Schedule: "2026-07-23T13:00:00Z"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := semantics.Normalize(tt.job)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Normalize() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.ScheduleType != tt.wantType {
				t.Errorf("ScheduleType = %q, want %q", got.ScheduleType, tt.wantType)
			}
			if got.Schedule != tt.wantSched {
				t.Errorf("Schedule = %q, want %q", got.Schedule, tt.wantSched)
			}
			if !sameInt64Ptr(got.RunAt, tt.wantRunAt) {
				t.Errorf("RunAt = %v, want %v", got.RunAt, tt.wantRunAt)
			}
		})
	}
}

func TestScheduleSemanticsNextRun(t *testing.T) {
	semantics := NewScheduleSemanticsWithClock(time.Now)
	now := time.Date(2026, time.July, 23, 15, 1, 0, 0, time.UTC)

	next, err := semantics.NextRun(models.Job{ScheduleType: models.ScheduleTypeCron, Schedule: "0 * * * *"}, now)
	if err != nil {
		t.Fatalf("NextRun() unexpected error: %v", err)
	}
	want := time.Date(2026, time.July, 23, 16, 0, 0, 0, time.UTC).Unix()
	if next == nil || *next != want {
		t.Fatalf("NextRun() = %v, want %d", next, want)
	}
}

func int64ToIntPtr(value int) *int {
	return &value
}

func sameInt64Ptr(left, right *int64) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
