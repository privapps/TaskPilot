package services

import (
	"context"
	"testing"
	"time"

	"taskpilot/db"
	"taskpilot/models"
)

func TestShellJobExecutorCapturesOutputAndExitCode(t *testing.T) {
	result := (ShellJobExecutor{}).Execute(context.Background(), models.Job{
		Command: "printf stdout; printf stderr >&2; exit 7",
	})

	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
	if result.Output != "stdout\nSTDERR:\nstderr" {
		t.Fatalf("Output = %q, want combined stdout and stderr", result.Output)
	}
	if result.StartedAt.IsZero() {
		t.Fatal("StartedAt should be set")
	}
	if result.Duration < 0 {
		t.Fatalf("Duration = %v, want non-negative", result.Duration)
	}
}

func TestShellJobExecutorUsesWorkingDirectory(t *testing.T) {
	result := (ShellJobExecutor{}).Execute(context.Background(), models.Job{
		Command:   "pwd",
		Directory: "/tmp",
	})

	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; output=%q", result.ExitCode, result.Output)
	}
	if result.Output == "" {
		t.Fatal("Output should contain the working directory")
	}
}

func TestShellCommandRunnerRunsSuccessCommand(t *testing.T) {
	err := (ShellCommandRunner{}).Run(context.Background(), "exit 0")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
}

type recordingJobExecutor struct {
	result ExecutionResult
	called chan models.Job
}

func (e *recordingJobExecutor) Execute(_ context.Context, job models.Job) ExecutionResult {
	e.called <- job
	return e.result
}

type recordingCommandRunner struct{}

func (recordingCommandRunner) Run(context.Context, string) error {
	return nil
}

type recordingSoundPlayer struct{}

func (recordingSoundPlayer) Play(context.Context, string) error {
	return nil
}

type recordingWakeEvents struct{}

func (recordingWakeEvents) Register(models.Job, time.Time) (bool, error) {
	return false, nil
}

func (recordingWakeEvents) Cancel(string, time.Time) error {
	return nil
}

func TestSchedulerManualExecutionUsesExecutorSeam(t *testing.T) {
	_, jobService, _ := newSchedulerTestHarness(t)
	job := models.NewTestJob(models.WithCommand("this command is never launched"))
	insertSchedulerTestJob(t, jobService.db.(*db.Database).DB, job)

	executor := &recordingJobExecutor{
		result: ExecutionResult{
			Output:    "fake output",
			ExitCode:  0,
			StartedAt: time.Now(),
			Duration:  2 * time.Millisecond,
		},
		called: make(chan models.Job, 1),
	}
	scheduler := NewSchedulerWithAdapters(
		jobService,
		executor,
		recordingCommandRunner{},
		recordingSoundPlayer{},
		recordingWakeEvents{},
	)
	jobService.SetScheduler(scheduler)

	scheduler.RunJobImmediately(job)
	select {
	case calledJob := <-executor.called:
		if calledJob.ID != job.ID {
			t.Fatalf("executor received job %q, want %q", calledJob.ID, job.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not invoke executor")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		stored, err := jobService.GetJobByID(job.ID)
		if err == nil && stored.LastResult == "success" {
			history, err := jobService.GetJobHistory(job.ID, 10)
			if err != nil {
				t.Fatalf("GetJobHistory() failed: %v", err)
			}
			if len(history) != 1 || history[0].Output != "fake output" {
				t.Fatalf("history = %+v, want fake execution output", history)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scheduler did not persist the fake execution result")
}
