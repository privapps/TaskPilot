package services

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"taskpilot/db"
	"taskpilot/models"
)

const testTimeoutMinute = 50 * time.Millisecond
const testTimeoutMinutes = 10

func TestKidsEvents45MinuteTimeoutSmoke(t *testing.T) {
	const optInEnv = "TASKPILOT_RUN_KIDS_EVENTS_TIMEOUT_SMOKE"
	if os.Getenv(optInEnv) != "1" {
		t.Skipf("set %s=1 to run this 5+ minute integration smoke test", optInEnv)
	}
	if !isUnixLikeOS() {
		t.Skip("shell execution smoke test requires a Unix-like OS")
	}

	_, jobService, _ := newSchedulerTestHarness(t)
	timeoutMinutes := 45
	job := models.NewTestJob(
		models.WithTitle("Kids_Events"),
		models.WithCommand("sleep 305"),
		models.WithDirectory("."),
		models.WithPaused(true),
	)
	job.TimeoutMinutes = &timeoutMinutes
	job.DisableMacosSleepPrevention = true

	created, err := jobService.CreateJob(job)
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}
	if !created.Paused {
		t.Fatal("CreateJob() returned an unpaused job, want paused")
	}
	stored, err := jobService.GetJobByID(created.ID)
	if err != nil {
		t.Fatalf("GetJobByID() before trigger failed: %v", err)
	}
	if !stored.Paused {
		t.Fatal("persisted job is not paused")
	}
	if stored.TimeoutMinutes == nil || *stored.TimeoutMinutes != timeoutMinutes {
		t.Fatalf("persisted timeout_minutes = %v, want %d", stored.TimeoutMinutes, timeoutMinutes)
	}

	if err := jobService.TriggerJob(created.ID); err != nil {
		t.Fatalf("TriggerJob() failed: %v", err)
	}
	history := waitForJobHistory(t, jobService, created.ID, 306*time.Second)
	if history.ExitCode != 0 {
		t.Errorf("history exit code = %d, want 0; output: %q", history.ExitCode, history.Output)
	}
	if history.DurationMs <= int64((300*time.Second)/time.Millisecond) {
		t.Errorf("history duration = %v, want more than 300 seconds", time.Duration(history.DurationMs)*time.Millisecond)
	}
	if strings.Contains(strings.ToLower(history.Output), "timed out") {
		t.Errorf("successful history unexpectedly reports a timeout: %q", history.Output)
	}
	if history.TriggerType != triggerTypeManual {
		t.Errorf("history trigger_type = %q, want %q", history.TriggerType, triggerTypeManual)
	}

	stored, err = jobService.GetJobByID(created.ID)
	if err != nil {
		t.Fatalf("GetJobByID() after run failed: %v", err)
	}
	if stored.LastResult != "success" {
		t.Errorf("last_result = %q, want success", stored.LastResult)
	}
}

func TestJobTimeoutUsesPersistedPerJobLimit(t *testing.T) {
	if !isUnixLikeOS() {
		t.Skip("shell execution tests require a Unix shell")
	}

	_, jobService, scheduler := newSchedulerTestHarness(t)
	scheduler.timeoutMinuteDuration = testTimeoutMinute

	timeoutMinutes := 1
	job := models.NewTestJob(
		models.WithTitle("per-job-timeout"),
		models.WithCommand("sleep 5"),
		models.WithDirectory("."),
		models.WithSchedule("* * * * *"),
	)
	job.TimeoutMinutes = &timeoutMinutes

	created, err := jobService.CreateJob(job)
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}
	var logs synchronizedTestBuffer
	previousLogOutput := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousLogOutput)

	if err := jobService.TriggerJob(created.ID); err != nil {
		t.Fatalf("TriggerJob() failed: %v", err)
	}
	waitForLogMessage(t, &logs, "Job per-job-timeout completed with exit code 124")

	historyEntries, err := jobService.GetJobHistory(created.ID, 10)
	if err != nil {
		t.Fatalf("GetJobHistory() failed: %v", err)
	}
	if len(historyEntries) == 0 {
		t.Fatal("job run did not persist history")
	}
	history := historyEntries[0]
	if history.ExitCode != 124 {
		t.Errorf("history exit code = %d, want 124", history.ExitCode)
	}
	if !strings.Contains(history.Output, "job timed out after 1 minutes") {
		t.Errorf("history output %q does not report the per-job timeout", history.Output)
	}
	if time.Duration(history.DurationMs)*time.Millisecond >= time.Second {
		t.Errorf("history duration = %v, want the configured one-minute test duration", time.Duration(history.DurationMs)*time.Millisecond)
	}

	stored, err := jobService.GetJobByID(created.ID)
	if err != nil {
		t.Fatalf("GetJobByID() failed: %v", err)
	}
	if stored.TimeoutMinutes == nil || *stored.TimeoutMinutes != 1 {
		t.Errorf("stored timeout_minutes = %v, want 1", stored.TimeoutMinutes)
	}
	if stored.LastResult != "failed" {
		t.Errorf("last_result = %q, want failed", stored.LastResult)
	}
}

func TestJobServiceValidatesTimeoutMinutesOnCreateAndUpdate(t *testing.T) {
	invalidTimeouts := []int{0, -1, 1441}

	for _, timeoutMinutes := range invalidTimeouts {
		t.Run(fmt.Sprintf("create_%d", timeoutMinutes), func(t *testing.T) {
			_, jobService, _ := newSchedulerTestHarness(t)
			job := models.NewTestJob(models.WithTitle("invalid-timeout-create"))
			job.TimeoutMinutes = &timeoutMinutes

			if _, err := jobService.CreateJob(job); err == nil || !strings.Contains(err.Error(), "timeout_minutes must be between 1 and 1440") {
				t.Fatalf("CreateJob() error = %v, want timeout range validation error", err)
			}
			jobs, err := jobService.GetJobs()
			if err != nil {
				t.Fatalf("GetJobs() failed: %v", err)
			}
			if len(jobs) != 0 {
				t.Errorf("CreateJob() persisted invalid job: %+v", jobs)
			}
		})

		t.Run(fmt.Sprintf("update_%d", timeoutMinutes), func(t *testing.T) {
			_, jobService, _ := newSchedulerTestHarness(t)
			job, err := jobService.CreateJob(models.NewTestJob(models.WithTitle("invalid-timeout-update")))
			if err != nil {
				t.Fatalf("CreateJob() failed: %v", err)
			}
			job.TimeoutMinutes = &timeoutMinutes

			if _, err := jobService.UpdateJob(job); err == nil || !strings.Contains(err.Error(), "timeout_minutes must be between 1 and 1440") {
				t.Fatalf("UpdateJob() error = %v, want timeout range validation error", err)
			}
			stored, err := jobService.GetJobByID(job.ID)
			if err != nil {
				t.Fatalf("GetJobByID() failed: %v", err)
			}
			if stored.TimeoutMinutes != nil {
				t.Errorf("invalid update persisted timeout_minutes = %d", *stored.TimeoutMinutes)
			}
		})
	}
}

func TestJobServiceAcceptsTimeoutMinuteBounds(t *testing.T) {
	_, jobService, _ := newSchedulerTestHarness(t)
	maximum := 1440
	job := models.NewTestJob(models.WithTitle("maximum-timeout"))
	job.TimeoutMinutes = &maximum

	created, err := jobService.CreateJob(job)
	if err != nil {
		t.Fatalf("CreateJob() rejected the maximum timeout: %v", err)
	}
	if created.TimeoutMinutes == nil || *created.TimeoutMinutes != 1440 {
		t.Fatalf("created timeout_minutes = %v, want 1440", created.TimeoutMinutes)
	}

	minimum := 1
	created.TimeoutMinutes = &minimum
	if _, err := jobService.UpdateJob(created); err != nil {
		t.Fatalf("UpdateJob() rejected the minimum timeout: %v", err)
	}
	stored, err := jobService.GetJobByID(created.ID)
	if err != nil {
		t.Fatalf("GetJobByID() failed: %v", err)
	}
	if stored.TimeoutMinutes == nil || *stored.TimeoutMinutes != 1 {
		t.Errorf("updated timeout_minutes = %v, want 1", stored.TimeoutMinutes)
	}
}

func TestJobTimeoutPersistsThroughImportUpsert(t *testing.T) {
	_, jobService, _ := newSchedulerTestHarness(t)
	initial := models.NewTestJob(models.WithTitle("imported-timeout"))
	initial.TimeoutMinutes = intPointer(5)
	created, err := jobService.CreateJob(initial)
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}

	imported := created
	imported.TimeoutMinutes = intPointer(1440)
	if err := jobService.persistence.ImportJobs([]models.Job{imported}); err != nil {
		t.Fatalf("ImportJobs() failed: %v", err)
	}
	stored, err := jobService.GetJobByID(created.ID)
	if err != nil {
		t.Fatalf("GetJobByID() failed: %v", err)
	}
	if stored.TimeoutMinutes == nil || *stored.TimeoutMinutes != 1440 {
		t.Errorf("imported timeout_minutes = %v, want 1440", stored.TimeoutMinutes)
	}
}

func TestTimedOutJobKillsDescendantsAndPersistsPartialOutput(t *testing.T) {
	if !isUnixLikeOS() {
		t.Skip("process-group termination is Unix-specific")
	}

	_, jobService, scheduler := newSchedulerTestHarness(t)
	scheduler.timeoutMinuteDuration = testTimeoutMinute

	childPIDFile := filepath.Join(t.TempDir(), "child.pid")
	grandchildPIDFile := filepath.Join(t.TempDir(), "grandchild.pid")
	command := fmt.Sprintf(
		`(trap '' TERM; (trap '' TERM; while :; do sleep 5; done) & grandchild=$!; printf '%%s' "$grandchild" > %s; wait) & child=$!; printf '%%s' "$child" > %s; while [ ! -s %s ]; do sleep 0.01; done; printf 'partial stdout'; printf 'partial stderr' >&2; wait`,
		shellQuote(grandchildPIDFile),
		shellQuote(childPIDFile),
		shellQuote(grandchildPIDFile),
	)
	job := models.NewTestJob(
		models.WithTitle("timeout-process-tree"),
		models.WithCommand(command),
		models.WithDirectory("."),
	)
	insertSchedulerTestJob(t, jobService.db.(*db.Database).DB, job)

	var logs synchronizedTestBuffer
	previousLogOutput := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousLogOutput)

	started := time.Now()
	scheduler.RunJobImmediately(job)
	history := waitForJobHistory(t, jobService, job.ID, time.Duration(testTimeoutMinutes)*testTimeoutMinute+time.Second)
	elapsed := time.Since(started)
	executionDuration := time.Duration(history.DurationMs) * time.Millisecond

	if executionDuration < time.Duration(testTimeoutMinutes)*testTimeoutMinute-100*time.Millisecond {
		t.Fatalf("job returned before its timeout: duration %v", executionDuration)
	}
	if executionDuration >= 10*time.Second || elapsed >= 10*time.Second {
		t.Fatalf("timed-out job took %v to return (history duration %v), want under 10 seconds", elapsed, executionDuration)
	}
	waitForLogMessage(t, &logs, "Job timeout-process-tree completed")
	if history.ExitCode != 124 {
		t.Errorf("history exit code = %d, want 124", history.ExitCode)
	}
	for _, output := range []string{"partial stdout", "STDERR:\npartial stderr", "job timed out after 10 minutes"} {
		if !strings.Contains(history.Output, output) {
			t.Errorf("history output %q does not contain %q", history.Output, output)
		}
	}

	stored, err := jobService.GetJobByID(job.ID)
	if err != nil {
		t.Fatalf("GetJobByID() failed: %v", err)
	}
	if stored.LastResult != "failed" {
		t.Errorf("last_result = %q, want failed", stored.LastResult)
	}

	for _, pidFile := range []string{childPIDFile, grandchildPIDFile} {
		pid, err := readProcessID(pidFile)
		if err != nil {
			t.Fatalf("read descendant PID from %s: %v", pidFile, err)
		}
		if processIsRunning(pid) {
			t.Errorf("descendant process %d is still running", pid)
		}
	}

	logOutput := logs.String()
	for _, message := range []string{
		"Executing job: timeout-process-tree (manual) with effective timeout 10 minutes",
		"timed out after 10 minutes",
	} {
		if !strings.Contains(logOutput, message) {
			t.Errorf("daemon log %q does not contain %q", logOutput, message)
		}
	}
}

func TestJobTimeoutAppliesToEveryTrigger(t *testing.T) {
	if !isUnixLikeOS() {
		t.Skip("shell execution tests require a Unix shell")
	}

	tests := []struct {
		name        string
		triggerType string
		trigger     func(*testing.T, *Scheduler, models.Job)
	}{
		{
			name:        "immediate",
			triggerType: triggerTypeImmediate,
			trigger: func(t *testing.T, scheduler *Scheduler, job models.Job) {
				if err := scheduler.ScheduleJob(job); err != nil {
					t.Fatalf("ScheduleJob() failed: %v", err)
				}
			},
		},
		{
			name:        "scheduled one-time",
			triggerType: triggerTypeScheduled,
			trigger: func(t *testing.T, scheduler *Scheduler, job models.Job) {
				if err := scheduler.processDueJobs(time.Now()); err != nil {
					t.Fatalf("processDueJobs() failed: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, jobService, scheduler := newSchedulerTestHarness(t)
			scheduler.timeoutMinuteDuration = testTimeoutMinute

			jobOptions := []models.TestJobOption{
				models.WithTitle("timeout-" + test.name),
				models.WithCommand("sleep 5"),
				models.WithDirectory("."),
			}
			if test.name == "immediate" {
				jobOptions = append(jobOptions, models.WithScheduleType(models.ScheduleTypeImmediate))
			} else {
				runAt := time.Now().Add(-time.Second).Unix()
				jobOptions = append(jobOptions,
					models.WithScheduleType(models.ScheduleTypeDatetime),
					models.WithRunAt(runAt),
				)
			}
			job := models.NewTestJob(jobOptions...)
			perJobTimeoutMinutes := 1
			job.TimeoutMinutes = &perJobTimeoutMinutes
			if test.triggerType == triggerTypeScheduled {
				runAt := *job.RunAt
				job.NextRunAt = &runAt
			}
			insertSchedulerTestJob(t, jobService.db.(*db.Database).DB, job)

			test.trigger(t, scheduler, job)
			history := waitForJobHistory(t, jobService, job.ID, time.Duration(testTimeoutMinutes)*testTimeoutMinute+time.Second)
			if history.ExitCode != 124 {
				t.Errorf("history exit code = %d, want 124", history.ExitCode)
			}
			if history.TriggerType != test.triggerType {
				t.Errorf("trigger type = %q, want %q", history.TriggerType, test.triggerType)
			}
			if !strings.Contains(history.Output, "job timed out after 1 minutes") {
				t.Errorf("history output %q does not report the timeout", history.Output)
			}
		})
	}
}

func TestFastJobHistoryIsUnchangedWithTimeoutEnabled(t *testing.T) {
	if !isUnixLikeOS() {
		t.Skip("shell execution tests require a Unix shell")
	}

	_, jobService, _ := newSchedulerTestHarness(t)
	commands := &timeoutTestCommandRunner{called: make(chan string, 1)}
	sounds := &timeoutTestSoundPlayer{called: make(chan string, 1)}
	scheduler := NewSchedulerWithAdapters(jobService, ShellJobExecutor{}, commands, sounds, recordingWakeEvents{})
	scheduler.timeoutMinuteDuration = testTimeoutMinute
	jobService.SetScheduler(scheduler)
	job := models.NewTestJob(
		models.WithTitle("fast-job"),
		models.WithCommand("printf stdout; printf stderr >&2; exit 0"),
		models.WithDirectory("."),
	)
	job.OnSuccessCmd = "echo after"
	job.SoundFile = "success.wav"
	insertSchedulerTestJob(t, jobService.db.(*db.Database).DB, job)

	scheduler.RunJobImmediately(job)
	history := waitForJobHistory(t, jobService, job.ID, 500*time.Millisecond)
	if history.ExitCode != 0 {
		t.Errorf("history exit code = %d, want 0", history.ExitCode)
	}
	if history.Output != "stdout\nSTDERR:\nstderr" {
		t.Errorf("history output = %q, want unchanged stdout/stderr capture", history.Output)
	}
	if strings.Contains(history.Output, "timed out") {
		t.Errorf("successful history unexpectedly reports a timeout: %q", history.Output)
	}

	stored, err := jobService.GetJobByID(job.ID)
	if err != nil {
		t.Fatalf("GetJobByID() failed: %v", err)
	}
	if stored.LastResult != "success" {
		t.Errorf("last_result = %q, want success", stored.LastResult)
	}
	if stored.Status != "idle" {
		t.Errorf("status = %q, want idle", stored.Status)
	}
	if called := receiveTestSignal(t, commands.called, "on-success command"); called != "echo after" {
		t.Errorf("on-success command = %q, want echo after", called)
	}
	if played := receiveTestSignal(t, sounds.called, "success sound"); played != "success.wav" {
		t.Errorf("sound = %q, want success.wav", played)
	}
}

func TestSchedulerDefaultTimeoutIsTenMinutes(t *testing.T) {
	_, jobService, _ := newSchedulerTestHarness(t)
	deadlines := make(chan time.Time, 1)
	scheduler := NewSchedulerWithAdapters(
		jobService,
		timeoutTestDeadlineExecutor{deadlines: deadlines},
		recordingCommandRunner{},
		recordingSoundPlayer{},
		recordingWakeEvents{},
	)
	jobService.SetScheduler(scheduler)

	job := models.NewTestJob(models.WithDirectory("."))
	insertSchedulerTestJob(t, jobService.db.(*db.Database).DB, job)
	scheduler.RunJobImmediately(job)

	var deadline time.Time
	select {
	case deadline = <-deadlines:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not start the job")
	}
	remaining := time.Until(deadline)
	if remaining <= 9*time.Minute || remaining > 10*time.Minute {
		t.Errorf("default execution deadline has %v remaining, want about 10 minutes", remaining)
	}

	waitForJobHistory(t, jobService, job.ID, 100*time.Millisecond)
}

type timeoutTestDeadlineExecutor struct {
	deadlines chan<- time.Time
}

func (e timeoutTestDeadlineExecutor) Execute(ctx context.Context, _ models.Job) ExecutionResult {
	deadline, _ := ctx.Deadline()
	e.deadlines <- deadline
	return ExecutionResult{ExitCode: 0, StartedAt: time.Now()}
}

type timeoutTestCommandRunner struct {
	called chan string
}

func (r *timeoutTestCommandRunner) Run(_ context.Context, command string) error {
	r.called <- command
	return nil
}

type timeoutTestSoundPlayer struct {
	called chan string
}

func (p *timeoutTestSoundPlayer) Play(_ context.Context, soundFile string) error {
	p.called <- soundFile
	return nil
}

func receiveTestSignal(t *testing.T, signal <-chan string, description string) string {
	t.Helper()

	select {
	case value := <-signal:
		return value
	case <-time.After(time.Second):
		t.Fatalf("scheduler did not invoke %s", description)
		return ""
	}
}

type synchronizedTestBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedTestBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(value)
}

func (b *synchronizedTestBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func waitForLogMessage(t *testing.T, logs *synchronizedTestBuffer, message string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logs.String(), message) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("daemon log did not contain %q: %s", message, logs.String())
}

func waitForJobHistory(t *testing.T, jobService *JobService, jobID string, settleDelay time.Duration) models.History {
	t.Helper()

	time.Sleep(settleDelay)
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		history, err := jobService.GetJobHistory(jobID, 10)
		if err != nil {
			lastErr = err
			time.Sleep(25 * time.Millisecond)
			continue
		}
		if len(history) > 0 {
			return history[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("GetJobHistory() did not succeed before deadline: %v", lastErr)
	}
	t.Fatal("scheduler did not persist a history entry")
	return models.History{}
}

func readProcessID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func processIsRunning(pid int) bool {
	output, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	status := strings.TrimSpace(string(output))
	return status != "" && !strings.HasPrefix(status, "Z")
}

func intPointer(value int) *int {
	return &value
}

func isUnixLikeOS() bool {
	switch runtime.GOOS {
	case "aix", "darwin", "dragonfly", "freebsd", "illumos", "linux", "netbsd", "openbsd", "solaris":
		return true
	default:
		return false
	}
}
