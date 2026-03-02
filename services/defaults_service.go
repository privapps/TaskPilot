package services

import (
	"database/sql"
	"fmt"
	"taskpilot/db"
	"taskpilot/models"
)

type DefaultsService struct {
	db db.DatabaseInterface
}

func NewDefaultsService() *DefaultsService {
	return &DefaultsService{
		db: &db.Database{DB: db.DB},
	}
}

func NewDefaultsServiceWithDB(database db.DatabaseInterface) *DefaultsService {
	return &DefaultsService{
		db: database,
	}
}

// GetDefaults retrieves the current default settings
func (s *DefaultsService) GetDefaults() (*models.Defaults, error) {
	query := `SELECT working_directory, sound_file, on_success_cmd, COALESCE(api_port, 8080) FROM defaults WHERE id = 1`

	defaults := &models.Defaults{}
	row := s.db.QueryRow(query)
	if row == nil {
		// No defaults configured yet, return empty defaults (mock compatibility)
		return defaults, nil
	}

	err := row.Scan(&defaults.WorkingDirectory, &defaults.SoundFile, &defaults.OnSuccessCmd, &defaults.APIPort)

	if err == sql.ErrNoRows {
		// No defaults configured yet, return empty defaults
		return defaults, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get defaults: %w", err)
	}

	return defaults, nil
}

// UpdateDefaults saves or updates the default settings
func (s *DefaultsService) UpdateDefaults(defaults *models.Defaults) (*models.Defaults, error) {
	// Use INSERT OR REPLACE for lazy initialization (creates row if not exists)
	query := `INSERT OR REPLACE INTO defaults (id, working_directory, sound_file, on_success_cmd, api_port) 
	          VALUES (1, ?, ?, ?, ?)`

	_, err := s.db.Exec(query, defaults.WorkingDirectory, defaults.SoundFile, defaults.OnSuccessCmd, defaults.APIPort)
	if err != nil {
		return nil, fmt.Errorf("failed to update defaults: %w", err)
	}

	return defaults, nil
}

// ApplyDefaultsToJob populates empty job fields with default values
func (s *DefaultsService) ApplyDefaultsToJob(job *models.Job) error {
	defaults, err := s.GetDefaults()
	if err != nil {
		return fmt.Errorf("failed to apply defaults: %w", err)
	}

	// Only apply defaults to empty fields
	if job.Directory == "" && defaults.WorkingDirectory != "" {
		job.Directory = defaults.WorkingDirectory
	}

	// If sound file is explicitly set to "none" or "empty", clear it and skip default
	if job.SoundFile == "none" || job.SoundFile == "empty" {
		job.SoundFile = ""
	} else if job.SoundFile == "" && defaults.SoundFile != "" {
		job.SoundFile = defaults.SoundFile
	}

	if job.OnSuccessCmd == "" && defaults.OnSuccessCmd != "" {
		job.OnSuccessCmd = defaults.OnSuccessCmd
	}

	return nil
}
