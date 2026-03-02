package main

import (
	"embed"
	"log"

	"taskpilot/db"
	"taskpilot/services"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
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
