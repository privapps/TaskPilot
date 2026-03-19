package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// parseNoGUIPort unit tests
// ---------------------------------------------------------------------------

func TestParseNoGUIPort_Absent(t *testing.T) {
	port, ok, err := parseNoGUIPort([]string{"--other-flag", "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("expected flag absent, got port %d", port)
	}
}

func TestParseNoGUIPort_Valid(t *testing.T) {
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"--no-gui-with-port", "9999"}, 9999},
		{[]string{"--no-gui-with-port", "8080"}, 8080},
		{[]string{"--verbose", "--no-gui-with-port", "1234"}, 1234},
	}
	for _, tc := range cases {
		port, ok, err := parseNoGUIPort(tc.args)
		if err != nil {
			t.Errorf("args %v: unexpected error: %v", tc.args, err)
			continue
		}
		if !ok {
			t.Errorf("args %v: expected flag present", tc.args)
			continue
		}
		if port != tc.want {
			t.Errorf("args %v: got port %d, want %d", tc.args, port, tc.want)
		}
	}
}

func TestParseNoGUIPort_MissingValue(t *testing.T) {
	_, _, err := parseNoGUIPort([]string{"--no-gui-with-port"})
	if err == nil {
		t.Error("expected error for missing port value, got nil")
	}
}

func TestParseNoGUIPort_NonNumeric(t *testing.T) {
	_, _, err := parseNoGUIPort([]string{"--no-gui-with-port", "abc"})
	if err == nil {
		t.Error("expected error for non-numeric port, got nil")
	}
}

// ---------------------------------------------------------------------------
// runHeadlessCtx integration test
// ---------------------------------------------------------------------------

// TestRunHeadlessCtx_StartsAndResponds launches the headless server on a free
// port via runHeadlessCtx, probes /api/jobs, then cancels the context and
// waits for clean shutdown.
func TestRunHeadlessCtx_StartsAndResponds(t *testing.T) {
	const port = 19387 // unlikely-to-be-in-use test port

	ctx, cancel := context.WithCancel(context.Background())

	errc := make(chan error, 1)
	go func() {
		errc <- runHeadlessCtx(ctx, port)
	}()

	// Give the server a moment to bind and start.
	time.Sleep(300 * time.Millisecond)

	resp, err := http.Get("http://localhost:19387/api/jobs")
	if err != nil {
		cancel()
		t.Fatalf("GET /api/jobs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Errorf("GET /api/jobs returned status %d, want 200", resp.StatusCode)
	}

	// Trigger shutdown and wait.
	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Errorf("runHeadlessCtx returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("runHeadlessCtx did not shut down in time")
	}
}

