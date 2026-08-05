This Product Design Document (PDD) outlines a cross-platform desktop application built with **Go** and **Wails v3**. This stack is ideal for your needs because it combines Go’s powerful system-level execution with a lightweight web-based UI.

---

# Product Design Document: "TaskPilot"

## 1. Executive Summary

**TaskPilot** is a lightweight, cross-platform job scheduler that allows users to automate shell commands and scripts. It features a modern dashboard to manage, monitor, and log tasks with custom post-execution triggers (sound/commands).

## 2. Technical Stack

* **Backend:** Go 1.22+ (for system execution and scheduling).
* **Frontend:** Svelte 5 + Tailwind CSS (via Wails v3).
* **Framework:** **Wails v3** (Native windowing, system tray, and JS-to-Go bindings).
* **Scheduling Engine:** `gocron` (for human-readable intervals) + `robfig/cron` (for complex Cron expressions).
* **Database:** SQLite (embedded) for job persistence and history logs.
* **Audio:** `faiface/beep` (Pure Go audio library, no CGO dependencies).

---

## 3. System Architecture

### A. The "Service" Pattern

Wails v3 uses a service-based architecture. We will define three core Go services:

1. **JobService:** Handles CRUD operations for jobs and saves them to SQLite.
2. **ScheduleService:** Manages the background ticker, starting/stopping jobs based on time.
3. **ExecutionService:** Runs the actual shell commands, captures output, and triggers post-job actions.

### B. Core Data Structure

```go
type Job struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`
    Command     string    `json:"command"`
    Directory   string    `json:"directory"`
    Schedule    string    `json:"schedule"` // e.g., "0 9 * * 1-5" or "1h30m"
    SoundFile   string    `json:"sound_file"`
    OnSuccess   string    `json:"on_success_cmd"`
    LastResult  string    `json:"last_result"`
    Status      string    `json:"status"` // Running, Idle, Failed
}

```

---

## 4. Feature Implementation

### 1. Advanced Scheduling

To support all your requirements, we will implement a dual-mode parser:

* **One-shot:** Use `time.AfterFunc` for "In X hours X min."
* **Recurring:** Use `gocron` for "Every hour" or "Mon-Fri 9 am."
* **Cron:** Allow raw Cron strings for power users.

### 2. Job Execution (The Runner)

The backend will use the `os/exec` package:

```go
func RunCommand(job Job) (string, error) {
    cmd := exec.Command("sh", "-c", job.Command) // Use 'cmd' /c for Windows
    cmd.Dir = job.Directory
    output, err := cmd.CombinedOutput()
    return string(output), err
}

```

### 3. macOS Permissions & Entitlements

Because you are on a Mac, we must handle the **App Sandbox**.

* **Entitlements:** In your `Info.plist` (managed by Wails), we will enable `com.apple.security.files.user-selected.read-write`.
* **Full Disk Access:** Since you want to run commands from *any* directory, you will likely need to grant the compiled `.app` "Full Disk Access" in System Settings.
* **Backgrounding:** We will use Wails v3 **System Tray** features so the app can stay active in the menu bar even when the window is closed.

---

## 5. UI/UX Design (Svelte + Tailwind)

### View 1: Dashboard (The "Job Board")

* **List View:** Cards showing Job Name, Next Run Time, and a "Run Now" manual button.
* **Status Indicators:** Green/Red dots for the last execution status.

### View 2: Job Editor (The "Composer")

* **Path Picker:** A button that opens a native Go dialog (`runtime.OpenDirectoryDialog`) to select the working directory.
* **Schedule Builder:** Simple dropdowns for "Every X" or a calendar picker for specific times.
* **Sound Preview:** A "Play" button next to the sound file path to test the alert.

### View 3: Execution Logs

* A History modal showing execution metadata and Markdown-rendered `stdout` from previous runs.
* History is loaded in pages so older entries can be retrieved without loading the entire log at once.
* Each entry provides copy and delete actions; HTTP/HTTPS links open in the system browser.
* The modal can be maximized for reviewing long output and larger histories.

---

## 6. Post-Job Logic Flow

1. **Execute:** Start process.
2. **Capture:** Stream output to the UI in real-time using Wails Events (`EventsEmit`).
3. **Save:** Log the exit code and output to SQLite.
4. **Trigger A:** If `SoundFile` is set, initialize `beep` and play the MP3/WAV.
5. **Trigger B:** If `OnSuccess` command exists, spawn a secondary `exec.Command`.
