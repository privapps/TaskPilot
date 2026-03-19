package services

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildLaunchAgentPlist(t *testing.T) {
	plist := buildLaunchAgentPlist("/Applications/TaskPilot.app/Contents/MacOS/TaskPilot", 9090, "/tmp/out.log", "/tmp/err.log")

	for _, snippet := range []string{
		launchAgentLabel,
		"--no-gui-with-port",
		"<string>9090</string>",
		"<key>KeepAlive</key>",
		"<string>/tmp/out.log</string>",
		"<string>/tmp/err.log</string>",
	} {
		if !strings.Contains(plist, snippet) {
			t.Fatalf("expected plist to contain %q", snippet)
		}
	}
}

func TestExtractLaunchAgentPort(t *testing.T) {
	plist := buildLaunchAgentPlist("/tmp/taskpilot", 8181, "/tmp/out.log", "/tmp/err.log")
	if got := extractLaunchAgentPort(plist); got != 8181 {
		t.Fatalf("extractLaunchAgentPort() = %d, want 8181", got)
	}
}

func TestEscapeXML(t *testing.T) {
	got := escapeXML(`TaskPilot & "Scheduler" <Daemon>`)
	want := "TaskPilot &amp; &quot;Scheduler&quot; &lt;Daemon&gt;"
	if got != want {
		t.Fatalf("escapeXML() = %q, want %q", got, want)
	}
}

func TestShouldRunEmbeddedScheduler(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		status LaunchAgentStatus
		err    error
		want   bool
	}{
		{name: "non-darwin always runs embedded", goos: "linux", want: true},
		{name: "darwin with loaded daemon skips embedded", goos: "darwin", status: LaunchAgentStatus{Loaded: true}, want: false},
		{name: "darwin without loaded daemon runs embedded", goos: "darwin", status: LaunchAgentStatus{Loaded: false}, want: true},
		{name: "darwin status error falls back to embedded", goos: "darwin", err: errTestLaunchAgent, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRunEmbeddedScheduler(tt.goos, tt.status, tt.err); got != tt.want {
				t.Fatalf("shouldRunEmbeddedScheduler() = %v, want %v", got, tt.want)
			}
		})
	}
}

var errTestLaunchAgent = errors.New("launch agent error")
