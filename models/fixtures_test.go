package models

import "testing"

func TestNewTestJob(t *testing.T) {
job := NewTestJob()
if job.ID == "" {
t.Error("ID should not be empty")
}
if job.Name == "" {
t.Error("Name should not be empty")
}
}

func TestNewTestDefaults(t *testing.T) {
defaults := NewTestDefaults()
if defaults.WorkingDirectory == "" {
t.Error("WorkingDirectory should not be empty")
}
if defaults.APIPort == 0 {
t.Error("APIPort should not be 0")
}
}

func TestNewTestHistory(t *testing.T) {
history := NewTestHistory("job-123")
if history.JobID != "job-123" {
t.Errorf("Expected JobID job-123, got %s", history.JobID)
}
if history.ID == "" {
t.Error("ID should not be empty")
}
}

func TestScheduleTypeConstants(t *testing.T) {
if ScheduleTypeCron != "cron" {
t.Error("ScheduleTypeCron should be cron")
}
if ScheduleTypeImmediate != "immediate" {
t.Error("ScheduleTypeImmediate should be immediate")
}
}
