package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"taskpilot/models"
	"taskpilot/services"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx             context.Context
	scheduler       *services.Scheduler
	jobService      *services.JobService
	defaultsService *services.DefaultsService
	apiServer       *services.APIServer
}

// NewApp creates a new App application struct
func NewApp(scheduler *services.Scheduler, jobService *services.JobService) *App {
	defaultsService := services.NewDefaultsService()

	// Get API port from defaults (use 8080 if not set)
	defaults, err := defaultsService.GetDefaults()
	apiPort := 8080
	if err == nil && defaults.APIPort > 0 {
		apiPort = defaults.APIPort
	}

	return &App{
		scheduler:       scheduler,
		jobService:      jobService,
		defaultsService: defaultsService,
		apiServer:       services.NewAPIServer(jobService, apiPort),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Start the scheduler in a goroutine
	go func() {
		if err := a.scheduler.Start(ctx); err != nil {
			// Log error but don't crash the app
		}
	}()

	// Start the API server in a goroutine
	go func() {
		if err := a.apiServer.Start(ctx); err != nil {
			// Log error but don't crash the app
			// Application continues without API server if port is in use
		}
	}()

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx := context.Background()
		if err := a.apiServer.Shutdown(shutdownCtx); err != nil {
			// Log error but application is already shutting down
		}
	}()
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return "Hello " + name + ", It's show time!"
}

// ExportJobsWithDialog opens a save dialog and exports jobs to the selected file
func (a *App) ExportJobsWithDialog() error {
	filepath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export Jobs",
		DefaultFilename: "taskpilot-jobs.json",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "JSON Files (*.json)",
				Pattern:     "*.json",
			},
		},
	})
	if err != nil {
		return err
	}

	// User cancelled the dialog
	if filepath == "" {
		return nil
	}

	return a.jobService.ExportJobs(filepath)
}

// ImportJobsWithDialog opens an open dialog and imports jobs from the selected file
func (a *App) ImportJobsWithDialog() error {
	filepath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import Jobs",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "JSON Files (*.json)",
				Pattern:     "*.json",
			},
		},
	})
	if err != nil {
		return err
	}

	// User cancelled the dialog
	if filepath == "" {
		return nil
	}

	return a.jobService.ImportJobs(filepath)
}

// ImportJobsFromFile imports jobs from a specific file path (for use with confirmation dialogs)
func (a *App) ImportJobsFromFile(filepath string) error {
	return a.jobService.ImportJobs(filepath)
}

// PrepareImportWithDialog opens a file dialog and returns the selected filepath and whether it contains defaults
func (a *App) PrepareImportWithDialog() (map[string]interface{}, error) {
	filepath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import Jobs",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "JSON Files (*.json)",
				Pattern:     "*.json",
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// User cancelled the dialog
	if filepath == "" {
		return map[string]interface{}{
			"cancelled":   true,
			"filepath":    "",
			"hasDefaults": false,
		}, nil
	}

	// Check if file contains defaults
	hasDefaults, err := a.CheckImportFileHasDefaults(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to check file: %w", err)
	}

	return map[string]interface{}{
		"cancelled":   false,
		"filepath":    filepath,
		"hasDefaults": hasDefaults,
	}, nil
}

// GetDefaults retrieves the current default settings
func (a *App) GetDefaults() (*models.Defaults, error) {
	return a.defaultsService.GetDefaults()
}

// UpdateDefaults saves or updates the default settings
func (a *App) UpdateDefaults(defaults *models.Defaults) (*models.Defaults, error) {
	return a.defaultsService.UpdateDefaults(defaults)
}

// CheckImportFileHasDefaults checks if an import file contains defaults without importing
func (a *App) CheckImportFileHasDefaults(filepath string) (bool, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	var export models.JobExport
	err = json.Unmarshal(data, &export)
	if err != nil {
		return false, fmt.Errorf("failed to parse file: %w", err)
	}

	return export.Defaults != nil, nil
}

// TriggerJob triggers a job to run immediately
func (a *App) TriggerJob(jobID string) error {
	return a.jobService.TriggerJob(jobID)
}
