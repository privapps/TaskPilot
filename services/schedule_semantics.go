package services

import (
	"fmt"
	"time"

	"taskpilot/models"

	"github.com/robfig/cron/v3"
)

const scheduleTimeGrace = time.Minute

// ScheduleSemantics is the deep module for interpreting a job's schedule.
// It owns normalization, validation, and the facts needed by the scheduler.
type ScheduleSemantics struct {
	parser cron.Parser
	now    func() time.Time
}

func NewScheduleSemantics() *ScheduleSemantics {
	return NewScheduleSemanticsWithClock(time.Now)
}

func NewScheduleSemanticsWithClock(now func() time.Time) *ScheduleSemantics {
	return &ScheduleSemantics{
		parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
		now:    now,
	}
}

// Normalize validates a job schedule and fills in the canonical fields needed
// by persistence and scheduling. Non-schedule job fields are preserved.
func (s *ScheduleSemantics) Normalize(job models.Job) (models.Job, error) {
	if job.ScheduleType == "" {
		job.ScheduleType = models.ScheduleTypeCron
	}

	if job.ScheduleType == models.ScheduleTypeImmediate && job.Schedule == "" {
		job.Schedule = models.ScheduleTypeImmediate
	}

	switch job.ScheduleType {
	case models.ScheduleTypeCron:
		if err := s.validateCron(job.Schedule); err != nil {
			return models.Job{}, fmt.Errorf("invalid cron expression: %v", err)
		}
	case models.ScheduleTypeDatetime:
		if job.RunAt != nil {
			if time.Unix(*job.RunAt, 0).Before(s.now().Add(-scheduleTimeGrace)) {
				return models.Job{}, fmt.Errorf("scheduled datetime is in the past; use a future datetime")
			}
			break
		}

		runAt, err := s.parseDatetime(job.Schedule)
		if err != nil {
			return models.Job{}, err
		}
		job.RunAt = &runAt
	case models.ScheduleTypeDelay:
		if job.DelayMinutes == nil || *job.DelayMinutes <= 0 {
			return models.Job{}, fmt.Errorf("delay_minutes must be a positive number")
		}
		runAt := s.RunAtForDelay(*job.DelayMinutes)
		job.RunAt = &runAt
	case models.ScheduleTypeImmediate:
		// Immediate jobs do not need a future execution time.
	default:
		return models.Job{}, fmt.Errorf("invalid schedule type: %s", job.ScheduleType)
	}

	return job, nil
}

// NextRun calculates the first execution time after now for a normalized job.
func (s *ScheduleSemantics) NextRun(job models.Job, now time.Time) (*int64, error) {
	switch job.ScheduleType {
	case models.ScheduleTypeCron, "":
		schedule, err := s.parser.Parse(job.Schedule)
		if err != nil {
			return nil, fmt.Errorf("failed to parse cron schedule: %w", err)
		}
		next := schedule.Next(now)
		if next.IsZero() {
			return nil, nil
		}
		return int64Ptr(next.Unix()), nil
	case models.ScheduleTypeDelay, models.ScheduleTypeDatetime:
		if job.RunAt == nil {
			return nil, fmt.Errorf("one-time job %s is missing run_at", job.Name)
		}
		return int64Ptr(*job.RunAt), nil
	case models.ScheduleTypeImmediate:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported schedule type: %s", job.ScheduleType)
	}
}

// FollowingRun calculates the next recurring execution after a claimed run.
func (s *ScheduleSemantics) FollowingRun(job models.Job, scheduledAt int64) (*int64, error) {
	if job.ScheduleType != models.ScheduleTypeCron && job.ScheduleType != "" {
		return nil, nil
	}

	schedule, err := s.parser.Parse(job.Schedule)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cron schedule: %w", err)
	}
	next := schedule.Next(time.Unix(scheduledAt, 0))
	if next.IsZero() {
		return nil, nil
	}
	return int64Ptr(next.Unix()), nil
}

func (s *ScheduleSemantics) MisfireGrace(job models.Job) time.Duration {
	if job.ScheduleType == models.ScheduleTypeDelay || job.ScheduleType == models.ScheduleTypeDatetime {
		return oneTimeMisfireGrace
	}
	return cronMisfireGrace
}

func (s *ScheduleSemantics) Supports(scheduleType string) bool {
	switch scheduleType {
	case "", models.ScheduleTypeCron, models.ScheduleTypeDelay, models.ScheduleTypeDatetime, models.ScheduleTypeImmediate:
		return true
	default:
		return false
	}
}

func (s *ScheduleSemantics) IsOneTime(job models.Job) bool {
	return job.ScheduleType == models.ScheduleTypeDelay || job.ScheduleType == models.ScheduleTypeDatetime
}

func (s *ScheduleSemantics) RunAtForDelay(delayMinutes int) int64 {
	return s.now().Add(time.Duration(delayMinutes) * time.Minute).Unix()
}

func (s *ScheduleSemantics) validateCron(expr string) error {
	_, err := s.parser.Parse(expr)
	return err
}

func (s *ScheduleSemantics) parseDatetime(schedule string) (int64, error) {
	if schedule == "" {
		return 0, fmt.Errorf("datetime job requires a datetime in the schedule field (e.g. '2026-03-05T14:30:00')")
	}

	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
	}
	for _, format := range formats {
		if parsed, err := time.ParseInLocation(format, schedule, time.Local); err == nil {
			if parsed.Before(s.now().Add(-scheduleTimeGrace)) {
				return 0, fmt.Errorf("scheduled datetime '%s' is in the past; use a future datetime", schedule)
			}
			return parsed.Unix(), nil
		}
	}

	return 0, fmt.Errorf("invalid datetime format '%s': use ISO 8601 (e.g. '2026-03-05T14:30:00')", schedule)
}
