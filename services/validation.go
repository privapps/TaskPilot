package services

import (
	"fmt"
	"time"

	"taskpilot/models"

	"github.com/robfig/cron/v3"
)

// validateCronExpression validates a cron expression using the same parser as the scheduler
func validateCronExpression(expr string) error {
	// Use the same parser as the scheduler (cron.New() uses standardParser = Minute|Hour|Dom|Month|Dow|Descriptor)
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	_, err := parser.Parse(expr)
	return err
}

// isValidScheduleType checks if the schedule type is one of the valid values (or empty for default)
func isValidScheduleType(scheduleType string) bool {
	// Empty is OK - will default to cron in JobService.CreateJob
	if scheduleType == "" {
		return true
	}
	switch scheduleType {
	case models.ScheduleTypeCron, models.ScheduleTypeDelay, models.ScheduleTypeDatetime, models.ScheduleTypeImmediate:
		return true
	default:
		return false
	}
}

// ValidateJob validates a job's schedule fields before create or update.
// It checks schedule type, cron syntax, and datetime constraints.
func ValidateJob(job models.Job) error {
	if !isValidScheduleType(job.ScheduleType) {
		return fmt.Errorf("invalid schedule type: %s", job.ScheduleType)
	}

	switch job.ScheduleType {
	case models.ScheduleTypeCron, "":
		if err := validateCronExpression(job.Schedule); err != nil {
			return fmt.Errorf("invalid cron expression: %v", err)
		}
	case models.ScheduleTypeDatetime:
		if job.RunAt != nil {
			// run_at provided directly (e.g. from Wails UI) — check it's in the future
			if *job.RunAt <= time.Now().Add(-60*time.Second).Unix() {
				return fmt.Errorf("scheduled datetime is in the past; use a future datetime")
			}
		} else {
			// fall back to parsing the schedule string (e.g. from REST/MCP)
			if err := validateDatetimeSchedule(job.Schedule); err != nil {
				return err
			}
		}
	case models.ScheduleTypeDelay:
		if job.DelayMinutes == nil || *job.DelayMinutes <= 0 {
			return fmt.Errorf("delay_minutes must be a positive number")
		}
	}
	return nil
}

// validateDatetimeSchedule checks that the schedule field is a parseable ISO 8601 datetime
// and that the scheduled time is not in the past.
func validateDatetimeSchedule(schedule string) error {
	if schedule == "" {
		return fmt.Errorf("datetime job requires a datetime in the schedule field (e.g. '2026-03-05T14:30:00')")
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
	}
	for _, format := range formats {
		if t, err := time.ParseInLocation(format, schedule, time.Local); err == nil {
			// Allow a 60-second grace window for clock skew / transmission delay
			if t.Before(time.Now().Add(-60 * time.Second)) {
				return fmt.Errorf("scheduled datetime '%s' is in the past; use a future datetime", schedule)
			}
			return nil
		}
	}
	return fmt.Errorf("invalid datetime format '%s': use ISO 8601 (e.g. '2026-03-05T14:30:00')", schedule)
}
