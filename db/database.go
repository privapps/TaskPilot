package db

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// DatabaseInterface defines the database operations needed by services
type DatabaseInterface interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Exec(query string, args ...interface{}) (sql.Result, error)
	Begin() (*sql.Tx, error)
}

// Database wraps sql.DB and implements DatabaseInterface
type Database struct {
	*sql.DB
}

// Query executes a query that returns rows
func (d *Database) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return d.DB.Query(query, args...)
}

// QueryRow executes a query that returns at most one row
func (d *Database) QueryRow(query string, args ...interface{}) *sql.Row {
	return d.DB.QueryRow(query, args...)
}

// Exec executes a query without returning rows
func (d *Database) Exec(query string, args ...interface{}) (sql.Result, error) {
	return d.DB.Exec(query, args...)
}

// Begin starts a new transaction
func (d *Database) Begin() (*sql.Tx, error) {
	return d.DB.Begin()
}

var DB *sql.DB

func Initialize() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	appDir := filepath.Join(homeDir, ".taskpilot")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return err
	}

	dbPath := filepath.Join(appDir, "taskpilot.db")
	log.Printf("Database path: %s", dbPath)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	DB = db
	return migrate()
}

func migrate() error {
	jobsTable := `CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		command TEXT NOT NULL,
		directory TEXT,
		schedule TEXT NOT NULL,
		sound_file TEXT,
		on_success_cmd TEXT,
		last_result TEXT,
		status TEXT DEFAULT 'idle',
		schedule_type TEXT DEFAULT 'cron',
		paused BOOLEAN DEFAULT 0,
		run_at INTEGER,
		delay_minutes INTEGER,
		last_run_at INTEGER
	)`

	if _, err := DB.Exec(jobsTable); err != nil {
		return err
	}

	historyTable := `CREATE TABLE IF NOT EXISTS history (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		output TEXT,
		exit_code INTEGER,
		timestamp INTEGER,
		duration_ms INTEGER,
		FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
	)`

	if _, err := DB.Exec(historyTable); err != nil {
		return err
	}

	// Add duration_ms column if it doesn't exist (for existing databases)
	_, _ = DB.Exec(`ALTER TABLE history ADD COLUMN duration_ms INTEGER`)

	// Migration for new scheduling columns
	_, _ = DB.Exec(`ALTER TABLE jobs ADD COLUMN schedule_type TEXT DEFAULT 'cron'`)
	_, _ = DB.Exec(`ALTER TABLE jobs ADD COLUMN paused BOOLEAN DEFAULT 0`)
	_, _ = DB.Exec(`ALTER TABLE jobs ADD COLUMN run_at INTEGER`)
	_, _ = DB.Exec(`ALTER TABLE jobs ADD COLUMN delay_minutes INTEGER`)

	// Set schedule_type='cron' for existing jobs that don't have it set
	_, _ = DB.Exec(`UPDATE jobs SET schedule_type = 'cron' WHERE schedule_type IS NULL OR schedule_type = ''`)

	// Add last_run_at column for tracking last execution time
	_, _ = DB.Exec(`ALTER TABLE jobs ADD COLUMN last_run_at INTEGER`)

	// Add disable_macos_sleep_prevention column for per-job opt-out of macOS sleep prevention
	_, _ = DB.Exec(`ALTER TABLE jobs ADD COLUMN disable_macos_sleep_prevention BOOLEAN DEFAULT 0`)

	// Backfill last_run_at from history table for existing jobs
	_, _ = DB.Exec(`
		UPDATE jobs 
		SET last_run_at = (
			SELECT MAX(timestamp) 
			FROM history 
			WHERE history.job_id = jobs.id
		) 
		WHERE last_run_at IS NULL AND EXISTS (
			SELECT 1 FROM history WHERE history.job_id = jobs.id
		)
	`)

	// Create defaults table with singleton constraint
	defaultsTable := `CREATE TABLE IF NOT EXISTS defaults (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		working_directory TEXT,
		sound_file TEXT,
		on_success_cmd TEXT,
		api_port INTEGER DEFAULT 8080
	)`

	if _, err := DB.Exec(defaultsTable); err != nil {
		return err
	}

	// Add api_port column for existing databases
	_, _ = DB.Exec(`ALTER TABLE defaults ADD COLUMN api_port INTEGER DEFAULT 8080`)

	log.Println("Database tables created successfully")
	return nil
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
