package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestNewMockDB(t *testing.T) {
	mock := NewMockDB()
	
	if mock == nil {
		t.Fatal("NewMockDB() returned nil")
	}
	
	if mock.QueryCalls == nil {
		t.Error("QueryCalls should be initialized")
	}
	
	if mock.ExecCalls == nil {
		t.Error("ExecCalls should be initialized")
	}
	
	if mock.QueryRowCalls == nil {
		t.Error("QueryRowCalls should be initialized")
	}
}

func TestMockDatabase_Query(t *testing.T) {
	mock := NewMockDB()
	
	// Set up mock function
	expectedRows := &sql.Rows{}
	mock.QueryFunc = func(query string, args ...interface{}) (*sql.Rows, error) {
		return expectedRows, nil
	}
	
	rows, err := mock.Query("SELECT * FROM test", "arg1")
	
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	if rows != expectedRows {
		t.Error("Query() did not return expected rows")
	}
	
	if len(mock.QueryCalls) != 1 {
		t.Errorf("Expected 1 QueryCall, got %d", len(mock.QueryCalls))
	}
	
	if mock.QueryCalls[0].Query != "SELECT * FROM test" {
		t.Errorf("Query not recorded correctly: %s", mock.QueryCalls[0].Query)
	}
}

func TestMockDatabase_Query_Error(t *testing.T) {
	mock := NewMockDB()
	expectedErr := errors.New("query error")
	
	mock.QueryFunc = func(query string, args ...interface{}) (*sql.Rows, error) {
		return nil, expectedErr
	}
	
	_, err := mock.Query("SELECT * FROM test")
	
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestMockDatabase_QueryRow(t *testing.T) {
	mock := NewMockDB()
	
	mock.QueryRowFunc = func(query string, args ...interface{}) *sql.Row {
		return nil
	}
	
	row := mock.QueryRow("SELECT * FROM test WHERE id = ?", 1)
	
	if row != nil {
		t.Error("Expected nil row")
	}
	
	if len(mock.QueryRowCalls) != 1 {
		t.Errorf("Expected 1 QueryRowCall, got %d", len(mock.QueryRowCalls))
	}
}

func TestMockDatabase_Exec(t *testing.T) {
	mock := NewMockDB()
	
	mockResult := &MockResult{
		lastInsertId: 123,
		rowsAffected: 1,
	}
	
	mock.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		return mockResult, nil
	}
	
	result, err := mock.Exec("INSERT INTO test VALUES (?)", "value")
	
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	if result != mockResult {
		t.Error("Exec() did not return expected result")
	}
	
	if len(mock.ExecCalls) != 1 {
		t.Errorf("Expected 1 ExecCall, got %d", len(mock.ExecCalls))
	}
	
	lastID, _ := result.LastInsertId()
	if lastID != 123 {
		t.Errorf("Expected LastInsertId 123, got %d", lastID)
	}
	
	rowsAff, _ := result.RowsAffected()
	if rowsAff != 1 {
		t.Errorf("Expected RowsAffected 1, got %d", rowsAff)
	}
}

func TestMockDatabase_Begin(t *testing.T) {
	mock := NewMockDB()
	
	mock.BeginFunc = func() (*sql.Tx, error) {
		return nil, nil
	}
	
	_, err := mock.Begin()
	
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	if mock.BeginCalls != 1 {
		t.Errorf("Expected 1 BeginCall, got %d", mock.BeginCalls)
	}
}

func TestMockDatabase_ImplementsInterface(t *testing.T) {
	var _ DatabaseInterface = (*MockDatabase)(nil)
	
	// This test ensures MockDatabase implements DatabaseInterface
	// If it doesn't, compilation will fail
}

func TestMockResult_LastInsertId(t *testing.T) {
	result := &MockResult{lastInsertId: 42}
	
	id, err := result.LastInsertId()
	
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	if id != 42 {
		t.Errorf("Expected LastInsertId 42, got %d", id)
	}
}

func TestMockResult_RowsAffected(t *testing.T) {
	result := &MockResult{rowsAffected: 5}
	
	rows, err := result.RowsAffected()
	
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	if rows != 5 {
		t.Errorf("Expected RowsAffected 5, got %d", rows)
	}
}
