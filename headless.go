package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // embed IANA timezone database for cross-platform support

	"taskpilot/db"
	"taskpilot/services"
)

// runHeadless starts TaskPilot as a pure REST + SSE service on the given port,
// with no GUI. It blocks until SIGINT or SIGTERM is received, then shuts down
// gracefully. This path has no dependency on the Wails runtime.
func runHeadless(port int) {
	if port <= 0 || port > 65535 {
		fmt.Fprintf(os.Stderr, "taskpilot: invalid port %d (must be 1–65535)\n", port)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Block until SIGINT/SIGTERM, then cancel the context.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("Shutting down TaskPilot headless server…")
		cancel()
	}()

	if err := runHeadlessCtx(ctx, port); err != nil {
		log.Fatal(err)
	}
	log.Println("TaskPilot stopped.")
}

// runHeadlessCtx is the testable core of headless mode.
// It initialises all services, starts them, and blocks until ctx is cancelled.
func runHeadlessCtx(ctx context.Context, port int) error {
	log.Printf("TaskPilot headless mode: starting REST+SSE server on port %d", port)

	if err := db.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()

	jobService := services.NewJobService()
	scheduler := services.NewScheduler(jobService)
	jobService.SetScheduler(scheduler)
	apiServer := services.NewAPIServer(jobService, port)

	go func() {
		if err := scheduler.Start(ctx); err != nil {
			log.Printf("Scheduler error: %v", err)
		}
	}()

	go func() {
		if err := apiServer.Start(ctx); err != nil {
			log.Printf("API server error: %v", err)
		}
	}()

	log.Printf("TaskPilot REST+SSE server running on http://localhost:%d", port)
	log.Printf("MCP endpoint: http://localhost:%d/api/mcp  |  Press Ctrl+C to stop", port)

	<-ctx.Done()

	shutdownCtx := context.Background()
	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("API server shutdown error: %v", err)
	}
	return nil
}
