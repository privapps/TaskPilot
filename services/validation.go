package services

import (
	"fmt"

	"taskpilot/models"
)

const (
	minJobTimeoutMinutes = 1
	maxJobTimeoutMinutes = 1440
)

func validateTimeoutMinutes(timeoutMinutes *int) error {
	if timeoutMinutes == nil {
		return nil
	}
	if *timeoutMinutes < minJobTimeoutMinutes || *timeoutMinutes > maxJobTimeoutMinutes {
		return fmt.Errorf("timeout_minutes must be between %d and %d minutes", minJobTimeoutMinutes, maxJobTimeoutMinutes)
	}
	return nil
}

func validateDefaultTimeoutMinutes(timeoutMinutes int) error {
	if timeoutMinutes < minJobTimeoutMinutes || timeoutMinutes > maxJobTimeoutMinutes {
		return fmt.Errorf("default_timeout_minutes must be between %d and %d minutes", minJobTimeoutMinutes, maxJobTimeoutMinutes)
	}
	return nil
}

// validateCronExpression validates a cron expression using the same parser as the scheduler
func validateCronExpression(expr string) error {
	return NewScheduleSemantics().validateCron(expr)
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
	_, err := NewScheduleSemantics().Normalize(job)
	return err
}

// validateDatetimeSchedule checks that the schedule field is a parseable ISO 8601 datetime
// and that the scheduled time is not in the past.
func validateDatetimeSchedule(schedule string) error {
	job := models.Job{ScheduleType: models.ScheduleTypeDatetime, Schedule: schedule}
	_, err := NewScheduleSemantics().Normalize(job)
	return err
}
