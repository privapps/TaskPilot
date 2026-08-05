package services

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"

	"taskpilot/models"
)

type ExecutionResult struct {
	Output    string
	ExitCode  int
	StartedAt time.Time
	Duration  time.Duration
}

// JobExecutor is the internal seam for running a job command.
type JobExecutor interface {
	Execute(ctx context.Context, job models.Job) ExecutionResult
}

// ShellJobExecutor is the production adapter for shell command execution.
type ShellJobExecutor struct{}

func (ShellJobExecutor) Execute(ctx context.Context, job models.Job) ExecutionResult {
	startedAt := time.Now()
	command := exec.CommandContext(ctx, "sh", "-c", wrapWithCaffeinate(job.Command, job.DisableMacosSleepPrevention))
	if job.Directory != "" {
		command.Dir = job.Directory
	}

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			output += fmt.Sprintf("\nError: %v", err)
		}
	}

	return ExecutionResult{
		Output:    output,
		ExitCode:  exitCode,
		StartedAt: startedAt,
		Duration:  time.Since(startedAt),
	}
}

type CommandRunner interface {
	Run(ctx context.Context, command string) error
}

type ShellCommandRunner struct{}

func (ShellCommandRunner) Run(ctx context.Context, command string) error {
	return exec.CommandContext(ctx, "sh", "-c", command).Run()
}

type SoundPlayer interface {
	Play(ctx context.Context, soundFile string) error
}

type SystemSoundPlayer struct{}

func (SystemSoundPlayer) Play(ctx context.Context, soundFile string) error {
	return exec.CommandContext(ctx, "afplay", soundFile).Run()
}

type WakeEventAdapter interface {
	Register(job models.Job, wakeTime time.Time) (bool, error)
	Cancel(jobID string, wakeTime time.Time) error
}

type SystemWakeEventAdapter struct{}

func (SystemWakeEventAdapter) Register(job models.Job, wakeTime time.Time) (bool, error) {
	if runtime.GOOS != "darwin" {
		return false, nil
	}

	wakeStr := wakeTime.Format("01/02/06 15:04:05")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "pmset", "schedule", "wake", wakeStr).Run(); err != nil {
		return false, err
	}
	log.Printf("Registered macOS wake event for job '%s' at %s", job.Name, wakeStr)
	return true, nil
}

func (SystemWakeEventAdapter) Cancel(jobID string, wakeTime time.Time) error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	wakeStr := wakeTime.Format("01/02/06 15:04:05")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "pmset", "cancel", "wake", wakeStr).Run()
}

// wrapWithCaffeinate wraps a job command with macOS sleep prevention when enabled.
func wrapWithCaffeinate(command string, disabled bool) string {
	if disabled || runtime.GOOS != "darwin" {
		return command
	}
	if _, err := exec.LookPath("caffeinate"); err != nil {
		log.Printf("WARNING: caffeinate not found on PATH, running job without sleep prevention: %v", err)
		return command
	}
	return "caffeinate -s -- sh -c " + shellQuote(command)
}

func shellQuote(value string) string {
	result := "'"
	for i := 0; i < len(value); i++ {
		if value[i] == '\'' {
			result += "'\\''"
		} else {
			result += string(value[i])
		}
	}
	return result + "'"
}
