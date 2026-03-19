package services

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const launchAgentLabel = "com.taskpilot.daemon"

type LaunchAgentStatus struct {
	Label     string `json:"label"`
	PlistPath string `json:"plist_path"`
	Installed bool   `json:"installed"`
	Loaded    bool   `json:"loaded"`
	Port      int    `json:"port"`
}

func ShouldRunEmbeddedScheduler() (bool, error) {
	if runtime.GOOS != "darwin" {
		return true, nil
	}

	status, err := GetLaunchAgentStatus()
	return shouldRunEmbeddedScheduler(runtime.GOOS, status, err), err
}

func InstallLaunchAgent(port int) (LaunchAgentStatus, error) {
	status, err := GetLaunchAgentStatus()
	if err != nil {
		return LaunchAgentStatus{}, err
	}

	executablePath, err := os.Executable()
	if err != nil {
		return LaunchAgentStatus{}, fmt.Errorf("failed to resolve executable path: %w", err)
	}
	executablePath, err = filepath.Abs(executablePath)
	if err != nil {
		return LaunchAgentStatus{}, fmt.Errorf("failed to canonicalize executable path: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return LaunchAgentStatus{}, fmt.Errorf("failed to resolve home directory: %w", err)
	}

	logDir := filepath.Join(homeDir, ".taskpilot", "logs")
	if err := os.MkdirAll(filepath.Dir(status.PlistPath), 0755); err != nil {
		return LaunchAgentStatus{}, fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return LaunchAgentStatus{}, fmt.Errorf("failed to create daemon log directory: %w", err)
	}

	stdoutPath := filepath.Join(logDir, "daemon.out.log")
	stderrPath := filepath.Join(logDir, "daemon.err.log")
	plistContent := buildLaunchAgentPlist(executablePath, port, stdoutPath, stderrPath)
	if err := os.WriteFile(status.PlistPath, []byte(plistContent), 0644); err != nil {
		return LaunchAgentStatus{}, fmt.Errorf("failed to write launch agent plist: %w", err)
	}

	_ = runLaunchctl("unload", "-w", status.PlistPath)
	if err := runLaunchctl("load", "-w", status.PlistPath); err != nil {
		return LaunchAgentStatus{}, fmt.Errorf("failed to load launch agent: %w", err)
	}

	status, err = GetLaunchAgentStatus()
	if err != nil {
		return LaunchAgentStatus{}, err
	}
	status.Port = port
	return status, nil
}

func UninstallLaunchAgent() error {
	status, err := GetLaunchAgentStatus()
	if err != nil {
		return err
	}

	if status.Installed {
		_ = runLaunchctl("unload", "-w", status.PlistPath)
	}

	if err := os.Remove(status.PlistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove launch agent plist: %w", err)
	}

	return nil
}

func GetLaunchAgentStatus() (LaunchAgentStatus, error) {
	if runtime.GOOS != "darwin" {
		return LaunchAgentStatus{}, fmt.Errorf("launch agent support is only available on macOS")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return LaunchAgentStatus{}, fmt.Errorf("failed to resolve home directory: %w", err)
	}

	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", launchAgentLabel+".plist")
	_, statErr := os.Stat(plistPath)
	installed := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return LaunchAgentStatus{}, fmt.Errorf("failed to inspect launch agent plist: %w", statErr)
	}

	port := 0
	if installed {
		if data, err := os.ReadFile(plistPath); err == nil {
			port = extractLaunchAgentPort(string(data))
		}
	}

	return LaunchAgentStatus{
		Label:     launchAgentLabel,
		PlistPath: plistPath,
		Installed: installed,
		Loaded:    isLaunchAgentLoaded(),
		Port:      port,
	}, nil
}

func buildLaunchAgentPlist(executablePath string, port int, stdoutPath string, stderrPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>--no-gui-with-port</string>
		<string>%d</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, escapeXML(launchAgentLabel), escapeXML(executablePath), port, escapeXML(stdoutPath), escapeXML(stderrPath))
}

func isLaunchAgentLoaded() bool {
	return runLaunchctl("list", launchAgentLabel) == nil
}

func runLaunchctl(args ...string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("launchctl is only available on macOS")
	}

	cmd := exec.Command("launchctl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return err
		}
		return fmt.Errorf("%s: %w", message, err)
	}
	return nil
}

func extractLaunchAgentPort(plist string) int {
	const marker = "<string>--no-gui-with-port</string>"
	index := strings.Index(plist, marker)
	if index < 0 {
		return 0
	}

	rest := plist[index+len(marker):]
	start := strings.Index(rest, "<string>")
	end := strings.Index(rest, "</string>")
	if start < 0 || end < 0 || end <= start+len("<string>") {
		return 0
	}

	value := rest[start+len("<string>") : end]
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return port
}

func escapeXML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}

func shouldRunEmbeddedScheduler(goos string, status LaunchAgentStatus, err error) bool {
	if goos != "darwin" {
		return true
	}
	if err != nil {
		return true
	}
	return !status.Loaded
}
