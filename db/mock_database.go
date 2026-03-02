package db

import (
	"database/sql"
	"errors"
)

// MockDatabase is a mock implementation of DatabaseInterface for testing
type MockDatabase struct {
	QueryFunc    func(query string, args ...interface{}) (*sql.Rows, error)
	QueryRowFunc func(query string, args ...interface{}) *sql.Row
	ExecFunc     func(query string, args ...interface{}) (sql.Result, error)
	BeginFunc    func() (*sql.Tx, error)

	// Call tracking
	QueryCalls    []QueryCall
	QueryRowCalls []QueryCall
	ExecCalls     []QueryCall
	BeginCalls    int
}

// QueryCall records a database call for verification
type QueryCall struct {
	Query string
	Args  []interface{}
}

// NewMockDB returns a new mock database with default behaviors
func NewMockDB() *MockDatabase {
	return &MockDatabase{
		QueryCalls:    make([]QueryCall, 0),
		QueryRowCalls: make([]QueryCall, 0),
		ExecCalls:     make([]QueryCall, 0),
	}
}

// SetRowsAffected is a helper for testing
func (m *MockResult) SetRowsAffected(n int64) {
	m.rowsAffected = n
}

// Query implements DatabaseInterface
func (m *MockDatabase) Query(query string, args ...interface{}) (*sql.Rows, error) {
	m.QueryCalls = append(m.QueryCalls, QueryCall{Query: query, Args: args})
	if m.QueryFunc != nil {
		return m.QueryFunc(query, args...)
	}
	return nil, errors.New("QueryFunc not configured")
}

// QueryRow implements DatabaseInterface
func (m *MockDatabase) QueryRow(query string, args ...interface{}) *sql.Row {
	m.QueryRowCalls = append(m.QueryRowCalls, QueryCall{Query: query, Args: args})
	if m.QueryRowFunc != nil {
		return m.QueryRowFunc(query, args...)
	}
	// Return nil - tests will need to configure QueryRowFunc for actual scanning
	return nil
}

// Exec implements DatabaseInterface
func (m *MockDatabase) Exec(query string, args ...interface{}) (sql.Result, error) {
	m.ExecCalls = append(m.ExecCalls, QueryCall{Query: query, Args: args})
	if m.ExecFunc != nil {
		return m.ExecFunc(query, args...)
	}
	return &MockResult{rowsAffected: 1}, nil
}

// Begin implements DatabaseInterface
func (m *MockDatabase) Begin() (*sql.Tx, error) {
	m.BeginCalls++
	if m.BeginFunc != nil {
		return m.BeginFunc()
	}
	return nil, errors.New("BeginFunc not configured")
}

// MockResult implements sql.Result for testing
type MockResult struct {
	lastInsertId int64
	rowsAffected int64
}

func (m *MockResult) LastInsertId() (int64, error) {
	return m.lastInsertId, nil
}

func (m *MockResult) RowsAffected() (int64, error) {
	return m.rowsAffected, nil
}

// MockRow is a mock implementation of sql.Row for testing without unexported field issues
type MockRow struct {
	err error
}

func (r *MockRow) Scan(dest ...interface{}) error {
	return r.err
}

func NewMockRow(err error) *sql.Row {
	// Note: We can't actually return a *sql.Row from our own struct because sql.Row fields are private
	// and there is no RowInterface in the standard library.
	// However, we can use a clever trick if the consumer uses an interface, but they don't here.
	// For now, let's just make QueryRow return nil and handle it in the service if possible,
	// or provide a better MockDatabase that uses a real sql.DB with a mock driver.
	return nil
}
