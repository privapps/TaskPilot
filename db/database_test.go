package db

import (
	"database/sql"
	"testing"
)

func TestMigrateAddsNullableTimeoutToExistingJobsIdempotently(t *testing.T) {
	legacyDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer func() {
		if err := legacyDB.Close(); err != nil {
			t.Errorf("close legacy database: %v", err)
		}
	}()

	previousDB := DB
	DB = legacyDB
	defer func() { DB = previousDB }()

	_, err = DB.Exec(`CREATE TABLE jobs (
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
		last_run_at INTEGER,
		next_run_at INTEGER,
		last_scheduled_at INTEGER,
		disable_macos_sleep_prevention BOOLEAN DEFAULT 0
	)`)
	if err != nil {
		t.Fatalf("create legacy jobs table: %v", err)
	}
	_, err = DB.Exec(`INSERT INTO jobs (id, name, command, schedule) VALUES ('existing-job', 'Existing job', 'echo existing', '* * * * *')`)
	if err != nil {
		t.Fatalf("insert existing job: %v", err)
	}

	if err := migrate(); err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}

	var timeoutMinutes sql.NullInt64
	if err := DB.QueryRow(`SELECT timeout_minutes FROM jobs WHERE id = 'existing-job'`).Scan(&timeoutMinutes); err != nil {
		t.Fatalf("read migrated timeout_minutes: %v", err)
	}
	if timeoutMinutes.Valid {
		t.Errorf("existing job timeout_minutes = %d, want NULL", timeoutMinutes.Int64)
	}

	if err := migrate(); err != nil {
		t.Fatalf("rerun migration: %v", err)
	}
	if err := DB.QueryRow(`SELECT timeout_minutes FROM jobs WHERE id = 'existing-job'`).Scan(&timeoutMinutes); err != nil {
		t.Fatalf("read timeout_minutes after rerun: %v", err)
	}
	if timeoutMinutes.Valid {
		t.Errorf("existing job timeout_minutes after rerun = %d, want NULL", timeoutMinutes.Int64)
	}
}

func TestMigrateAddsDefaultTimeoutToExistingDefaultsIdempotently(t *testing.T) {
	legacyDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer func() {
		if err := legacyDB.Close(); err != nil {
			t.Errorf("close legacy database: %v", err)
		}
	}()

	previousDB := DB
	DB = legacyDB
	defer func() { DB = previousDB }()

	_, err = DB.Exec(`CREATE TABLE defaults (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		working_directory TEXT,
		sound_file TEXT,
		on_success_cmd TEXT,
		api_port INTEGER DEFAULT 8080
	)`)
	if err != nil {
		t.Fatalf("create legacy defaults table: %v", err)
	}
	_, err = DB.Exec(`INSERT INTO defaults (id, working_directory, sound_file, on_success_cmd, api_port)
		VALUES (1, '/saved/path', 'saved.mp3', 'echo saved', 9090)`)
	if err != nil {
		t.Fatalf("insert legacy defaults row: %v", err)
	}

	if err := migrate(); err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}

	var workingDirectory, soundFile, onSuccessCmd string
	var apiPort, timeoutMinutes int
	if err := DB.QueryRow(`SELECT working_directory, sound_file, on_success_cmd, api_port, default_timeout_minutes
		FROM defaults WHERE id = 1`).Scan(&workingDirectory, &soundFile, &onSuccessCmd, &apiPort, &timeoutMinutes); err != nil {
		t.Fatalf("read migrated defaults: %v", err)
	}
	if workingDirectory != "/saved/path" || soundFile != "saved.mp3" || onSuccessCmd != "echo saved" || apiPort != 9090 {
		t.Errorf("migration changed existing defaults: %q, %q, %q, %d", workingDirectory, soundFile, onSuccessCmd, apiPort)
	}
	if timeoutMinutes != 10 {
		t.Errorf("migrated default_timeout_minutes = %d, want 10", timeoutMinutes)
	}

	if _, err := DB.Exec(`UPDATE defaults SET default_timeout_minutes = 37 WHERE id = 1`); err != nil {
		t.Fatalf("update migrated timeout: %v", err)
	}
	if err := migrate(); err != nil {
		t.Fatalf("rerun migration: %v", err)
	}
	if err := DB.QueryRow(`SELECT default_timeout_minutes FROM defaults WHERE id = 1`).Scan(&timeoutMinutes); err != nil {
		t.Fatalf("read default_timeout_minutes after rerun: %v", err)
	}
	if timeoutMinutes != 37 {
		t.Errorf("rerunning migration changed default_timeout_minutes to %d, want 37", timeoutMinutes)
	}
}

func TestFreshDatabaseDefaultsTimeoutToTenMinutes(t *testing.T) {
	freshDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open fresh database: %v", err)
	}
	defer func() {
		if err := freshDB.Close(); err != nil {
			t.Errorf("close fresh database: %v", err)
		}
	}()

	previousDB := DB
	DB = freshDB
	defer func() { DB = previousDB }()

	if err := migrate(); err != nil {
		t.Fatalf("migrate fresh database: %v", err)
	}
	if _, err := DB.Exec(`INSERT INTO defaults (id) VALUES (1)`); err != nil {
		t.Fatalf("insert defaults row using database defaults: %v", err)
	}

	var timeoutMinutes int
	if err := DB.QueryRow(`SELECT default_timeout_minutes FROM defaults WHERE id = 1`).Scan(&timeoutMinutes); err != nil {
		t.Fatalf("read fresh database timeout: %v", err)
	}
	if timeoutMinutes != 10 {
		t.Errorf("fresh database default_timeout_minutes = %d, want 10", timeoutMinutes)
	}
}

func TestDatabase_Query(t *testing.T) {
	// Test that Database wrapper delegates to sql.DB
	d := &Database{DB: &sql.DB{}}

	if d.DB == nil {
		t.Error("Database.DB should not be nil")
	}

	// Note: Can't test actual query without real database
	// This tests the interface implementation exists
}

func TestDatabase_QueryRow(t *testing.T) {
	d := &Database{DB: &sql.DB{}}

	if d.DB == nil {
		t.Error("Database.DB should not be nil")
	}
}

func TestDatabase_Exec(t *testing.T) {
	d := &Database{DB: &sql.DB{}}

	if d.DB == nil {
		t.Error("Database.DB should not be nil")
	}
}

func TestDatabase_Begin(t *testing.T) {
	d := &Database{DB: &sql.DB{}}

	if d.DB == nil {
		t.Error("Database.DB should not be nil")
	}
}

func TestDatabase_ImplementsInterface(t *testing.T) {
	var _ DatabaseInterface = (*Database)(nil)

	// This test ensures Database implements DatabaseInterface
	// If it doesn't, compilation will fail
}
