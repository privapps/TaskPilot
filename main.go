package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"strconv"

	"taskpilot/db"
	"taskpilot/services"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	args := os.Args[1:]

	// --no-gui-with-port <port>: run as a headless REST+SSE service, no GUI.
	if port, ok, err := parseNoGUIPort(args); err != nil {
		fmt.Fprintln(os.Stderr, "taskpilot:", err)
		os.Exit(1)
	} else if ok {
		runHeadless(port)
		return
	}

	if hasFlag(args, "--install-launch-agent") {
		if err := runInstallLaunchAgent(); err != nil {
			fmt.Fprintln(os.Stderr, "taskpilot:", err)
			os.Exit(1)
		}
		return
	}

	if hasFlag(args, "--uninstall-launch-agent") {
		if err := services.UninstallLaunchAgent(); err != nil {
			fmt.Fprintln(os.Stderr, "taskpilot:", err)
			os.Exit(1)
		}
		fmt.Println("TaskPilot launch agent removed.")
		return
	}

	if hasFlag(args, "--launch-agent-status") {
		status, err := services.GetLaunchAgentStatus()
		if err != nil {
			fmt.Fprintln(os.Stderr, "taskpilot:", err)
			os.Exit(1)
		}
		fmt.Printf("label=%s installed=%t loaded=%t port=%d plist=%s\n",
			status.Label, status.Installed, status.Loaded, status.Port, status.PlistPath)
		return
	}

	// Initialize database
	if err := db.Initialize(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// Create JobService
	jobService := services.NewJobService()

	// Create Scheduler
	scheduler := services.NewScheduler(jobService)

	// Link JobService to Scheduler
	jobService.SetScheduler(scheduler)

	// Create an instance of the app structure
	app := NewApp(scheduler, jobService)

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "TaskPilot",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
			jobService,
		},
	})

	if err != nil {
		log.Fatal("Error:", err)
	}
}

// parseNoGUIPort scans args for "--no-gui-with-port <port>".
// Returns (port, true, nil) when the flag is found and valid,
// (0, false, nil) when the flag is absent, or (0, false, err) on bad input.
func parseNoGUIPort(args []string) (int, bool, error) {
	for i, arg := range args {
		if arg == "--no-gui-with-port" {
			if i+1 >= len(args) {
				return 0, false, fmt.Errorf("--no-gui-with-port requires a port number")
			}
			port, err := strconv.Atoi(args[i+1])
			if err != nil {
				return 0, false, fmt.Errorf("invalid port %q: %v", args[i+1], err)
			}
			return port, true, nil
		}
	}
	return 0, false, nil
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func runInstallLaunchAgent() error {
	if err := db.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()

	defaultsService := services.NewDefaultsService()
	defaults, err := defaultsService.GetDefaults()
	if err != nil {
		return fmt.Errorf("failed to read defaults: %w", err)
	}

	port := 8080
	if defaults != nil && defaults.APIPort > 0 {
		port = defaults.APIPort
	}

	status, err := services.InstallLaunchAgent(port)
	if err != nil {
		return err
	}

	fmt.Printf("TaskPilot launch agent installed on port %d (%s)\n", status.Port, status.PlistPath)
	return nil
}
