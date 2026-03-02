package services

import (
	"database/sql"
	"errors"
	"testing"

	"taskpilot/db"
	"taskpilot/models"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDefaultsService_GetDefaults(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "get existing defaults",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("Failed to create mock: %v", err)
			}
			defer sqlDB.Close()

			// Mock GetDefaults query
			rows := sqlmock.NewRows([]string{"working_directory", "sound_file", "on_success_cmd", "api_port"}).
				AddRow("/tmp", "sound.mp3", "echo done", 8080)
			mock.ExpectQuery("SELECT .* FROM defaults").WillReturnRows(rows)

			wrapper := &db.Database{DB: sqlDB}
			service := NewDefaultsServiceWithDB(wrapper)
			defaults, err := service.GetDefaults()

			if (err != nil) != tt.wantErr {
				t.Errorf("GetDefaults() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && defaults.WorkingDirectory != "/tmp" {
				t.Errorf("GetDefaults() returned wrong working directory: %s", defaults.WorkingDirectory)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestDefaultsService_GetDefaults_NoDefaults(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	// Mock no defaults - GetDefaults returns nil error when sql.ErrNoRows occurs
	mock.ExpectQuery("SELECT .* FROM defaults").WillReturnError(sql.ErrNoRows)

	wrapper := &db.Database{DB: sqlDB}
	service := NewDefaultsServiceWithDB(wrapper)
	defaults, err := service.GetDefaults()

	// When no defaults exist, GetDefaults returns empty defaults with nil error
	if err != nil {
		t.Errorf("GetDefaults() expected nil error, got %v", err)
	}
	
	if defaults == nil {
		t.Error("GetDefaults() should return non-nil defaults even when none exist")
	}
}

func TestDefaultsService_SetDefaults_New(t *testing.T) {
	mockDB := db.NewMockDB()
	mockDB.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		return &db.MockResult{}, nil
	}

	service := NewDefaultsServiceWithDB(mockDB)
	defaults := models.NewTestDefaults()

	_, err := service.UpdateDefaults(&defaults)

	if err != nil {
		t.Errorf("SetDefaults() unexpected error: %v", err)
	}

	// Verify Exec was called
	if len(mockDB.ExecCalls) != 1 {
		t.Errorf("Expected 1 Exec call, got %d", len(mockDB.ExecCalls))
	}
}

func TestDefaultsService_SetDefaults_Update(t *testing.T) {
	mockDB := db.NewMockDB()
	mockDB.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		return &db.MockResult{}, nil
	}

	service := NewDefaultsServiceWithDB(mockDB)
	defaults := models.Defaults{
		WorkingDirectory: "/updated/path",
		SoundFile:        "updated.mp3",
		OnSuccessCmd:     "echo updated",
		APIPort:          9090,
	}

	_, err := service.UpdateDefaults(&defaults)

	if err != nil {
		t.Errorf("SetDefaults() unexpected error: %v", err)
	}

	// Verify Exec was called
	if len(mockDB.ExecCalls) != 1 {
		t.Errorf("Expected 1 Exec call, got %d", len(mockDB.ExecCalls))
	}

	// Verify correct values were passed
	if len(mockDB.ExecCalls) > 0 {
		call := mockDB.ExecCalls[0]
		if len(call.Args) < 4 {
			t.Error("Expected at least 4 arguments to Exec")
		}
	}
}

func TestDefaultsService_SetDefaults_Error(t *testing.T) {
	mockDB := db.NewMockDB()
	mockDB.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		return nil, errors.New("database error")
	}

	service := NewDefaultsServiceWithDB(mockDB)
	defaults := models.NewTestDefaults()

	_, err := service.UpdateDefaults(&defaults)

	if err == nil {
		t.Error("SetDefaults() expected error, got nil")
	}

	// Verify Exec was called
	if len(mockDB.ExecCalls) != 1 {
		t.Errorf("Expected 1 Exec call, got %d", len(mockDB.ExecCalls))
	}
}

func TestNewDefaultsService(t *testing.T) {
	service := NewDefaultsService()

	if service == nil {
		t.Fatal("NewDefaultsService() returned nil")
	}

	if service.db == nil {
		t.Error("DefaultsService.db should not be nil")
	}
}

func TestNewDefaultsServiceWithDB(t *testing.T) {
	mockDB := db.NewMockDB()
	service := NewDefaultsServiceWithDB(mockDB)

	if service == nil {
		t.Fatal("NewDefaultsServiceWithDB() returned nil")
	}

	if service.db != mockDB {
		t.Error("DefaultsService.db should be the mock database")
	}
}

func TestDefaultsService_ApplyDefaultsToJob(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer sqlDB.Close()

	defaults := models.Defaults{
		WorkingDirectory: "/default/path",
		SoundFile:        "default.mp3",
		OnSuccessCmd:     "echo default",
		APIPort:          8080,
	}

	// Mock GetDefaults query
	rows := sqlmock.NewRows([]string{"working_directory", "sound_file", "on_success_cmd", "api_port"}).
		AddRow(defaults.WorkingDirectory, defaults.SoundFile, defaults.OnSuccessCmd, defaults.APIPort)
	mock.ExpectQuery("SELECT .* FROM defaults").WillReturnRows(rows)

	wrapper := &db.Database{DB: sqlDB}
	service := NewDefaultsServiceWithDB(wrapper)

	job := models.NewTestJob(models.WithTitle("Test"), models.WithCommand("echo test"))
	job.Directory = "" // Empty directory should be filled with default

	err = service.ApplyDefaultsToJob(&job)
	if err != nil {
		t.Errorf("ApplyDefaultsToJob() unexpected error: %v", err)
	}

	if job.Directory != defaults.WorkingDirectory {
		t.Errorf("ApplyDefaultsToJob() did not apply default directory, got %s", job.Directory)
	}
}
