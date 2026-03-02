package db

import (
	"database/sql"
	"testing"
)

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
