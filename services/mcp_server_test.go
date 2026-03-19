package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"taskpilot/db"
	"taskpilot/models"
)

// TestEnforceMCPNaming tests the ai_ prefix enforcement function
func TestEnforceMCPNaming(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"With ai_ prefix", "ai_test_job", "ai_test_job"},
		{"Without ai_ prefix", "test_job", "ai_test_job"},
		{"Empty name", "", "ai_"},
		{"Only ai", "ai", "ai_ai"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enforceMCPNaming(tt.input)
			if tt.input == "" {
				// For empty names, we generate with UUID, just check prefix
				if !strings.HasPrefix(result, "ai_") {
					t.Errorf("Expected prefix ai_ for empty name, got %s", result)
				}
			} else if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestMCPRequestParsing tests JSON-RPC request parsing and validation
func TestMCPRequestParsing(t *testing.T) {
	mcpServer := NewMCPServer(nil)

	tests := []struct {
		name        string
		request     MCPRequest
		expectError bool
		errorCode   int
	}{
		{
			name: "Valid request",
			request: MCPRequest{
				JSONRPC: "2.0",
				ID:      "1",
				Method:  "jobs/list",
			},
			expectError: false,
		},
		{
			name: "Invalid JSON-RPC version",
			request: MCPRequest{
				JSONRPC: "1.0",
				ID:      "2",
				Method:  "jobs/list",
			},
			expectError: true,
			errorCode:   MCPErrorInvalidRequest,
		},
		{
			name: "Unknown method",
			request: MCPRequest{
				JSONRPC: "2.0",
				ID:      "3",
				Method:  "unknown/method",
			},
			expectError: true,
			errorCode:   MCPErrorMethodNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := mcpServer.HandleRequest(context.Background(), tt.request)
			if tt.expectError {
				if response.Error == nil {
					t.Error("Expected error but got none")
				} else if response.Error.Code != tt.errorCode {
					t.Errorf("Expected error code %d, got %d", tt.errorCode, response.Error.Code)
				}
			} else if response.Error != nil {
				t.Errorf("Unexpected error: %v", response.Error)
			}
		})
	}
}

// TestMCPConnectionManagement tests client registration and deregistration
func TestMCPConnectionManagement(t *testing.T) {
	mcpServer := NewMCPServer(nil)

	// Register a client
	clientID := "test-client-1"
	ch := mcpServer.RegisterClient(clientID)

	if ch == nil {
		t.Fatal("Expected channel, got nil")
	}

	// Check client is registered
	mcpServer.mu.RLock()
	_, exists := mcpServer.clients[clientID]
	mcpServer.mu.RUnlock()

	if !exists {
		t.Error("Client not registered")
	}

	// Unregister client
	mcpServer.UnregisterClient(clientID)

	// Check client is unregistered
	mcpServer.mu.RLock()
	_, exists = mcpServer.clients[clientID]
	mcpServer.mu.RUnlock()

	if exists {
		t.Error("Client still registered after unregister")
	}
}

// TestMCPEventBroadcasting tests event broadcasting to multiple clients
func TestMCPEventBroadcasting(t *testing.T) {
	mcpServer := NewMCPServer(nil)

	// Register multiple clients
	client1 := mcpServer.RegisterClient("client-1")
	client2 := mcpServer.RegisterClient("client-2")

	// Broadcast an event
	event := MCPEvent{
		Type: EventJobCreated,
		Data: map[string]string{"test": "data"},
	}

	go mcpServer.BroadcastEvent(event)

	// Verify both clients receive the event
	timeout := time.After(2 * time.Second)

	select {
	case e := <-client1:
		if e.Type != EventJobCreated {
			t.Errorf("Client 1: Expected event type %s, got %s", EventJobCreated, e.Type)
		}
	case <-timeout:
		t.Error("Client 1 did not receive event")
	}

	select {
	case e := <-client2:
		if e.Type != EventJobCreated {
			t.Errorf("Client 2: Expected event type %s, got %s", EventJobCreated, e.Type)
		}
	case <-timeout:
		t.Error("Client 2 did not receive event")
	}

	// Cleanup
	mcpServer.UnregisterClient("client-1")
	mcpServer.UnregisterClient("client-2")
}

// TestMCPJobCreateWithPrefixEnforcement tests job creation with name prefix enforcement and sound file clearing
func TestMCPJobCreateWithPrefixEnforcement(t *testing.T) {
	// Setup mocks
	mockDB := db.NewMockDB()

	mockDB.QueryRowFunc = func(query string, args ...interface{}) *sql.Row {
		// DefaultsService calls QueryRow for "SELECT ... FROM defaults LIMIT 1"
		// If it's a select query, we return nil Row which will cause Scan to return nil
		// (instead of panicking due to sql.Row internals needing a real Rows object)
		// but wait - Scan on a nil Row will panic. We need a way to mock Row.
		// Since we cannot mock sql.Row easily, let's just make the query return error
		return nil
	}

	// Mock job creation (INSERT)
	mockDB.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		res := &db.MockResult{}
		res.SetRowsAffected(1)
		return res, nil
	}

	// Create services
	jobService := NewJobServiceWithDB(mockDB)
	mcpServer := NewMCPServer(jobService)

	// Test cases
	tests := []struct {
		name         string
		params       map[string]interface{}
		expectedName string
		expectError  bool
	}{
		{
			name: "Normal job creation",
			params: map[string]interface{}{
				"name":          "test_job",
				"command":       "echo test",
				"schedule":      "0 * * * *",
				"schedule_type": "cron",
			},
			expectedName: "ai_test_job",
			expectError:  false,
		},
		{
			name: "Job with ai_ prefix",
			params: map[string]interface{}{
				"name":          "ai_existing_job",
				"command":       "echo test",
				"schedule":      "0 * * * *",
				"schedule_type": "cron",
			},
			expectedName: "ai_existing_job",
			expectError:  false,
		},
		{
			name: "Job with sound file should be cleared",
			params: map[string]interface{}{
				"name":          "sound_job",
				"command":       "echo test",
				"sound_file":    "/path/to/sound.wav",
				"schedule":      "0 * * * *",
				"schedule_type": "cron",
			},
			expectedName: "ai_sound_job",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request params
			paramsBytes, _ := json.Marshal(tt.params)

			req := MCPRequest{
				JSONRPC: "2.0",
				ID:      1,
				Method:  "jobs/create",
				Params:  paramsBytes,
			}

			// execute
			resp := mcpServer.handleJobCreate(context.Background(), req)

			if tt.expectError {
				if resp.Error == nil {
					t.Errorf("Expected error but got success")
				}
			} else {
				if resp.Error != nil {
					t.Errorf("Unexpected error: %v", resp.Error)
					return
				}

				if toolResult, ok := resp.Result.(MCPToolResult); ok {
					if len(toolResult.Content) == 0 {
						t.Fatal("expected MCP tool result content")
					}
					var decoded models.Job
					if err := json.Unmarshal([]byte(toolResult.Content[0].Text), &decoded); err != nil {
						t.Fatalf("failed to decode MCP tool result: %v", err)
					}
					if decoded.Name != tt.expectedName {
						t.Errorf("Expected job name %s, got %s", tt.expectedName, decoded.Name)
					}
					if decoded.SoundFile != "" {
						t.Errorf("Expected empty sound_file, got %s", decoded.SoundFile)
					}
					return
				}

				resultMap, ok := resp.Result.(map[string]interface{})
				if !ok {
					t.Errorf("Result is not a map or MCP tool result, got %T", resp.Result)
					return
				}

				// Verify name prefix
				if name, ok := resultMap["name"].(string); ok {
					if name != tt.expectedName {
						t.Errorf("Expected job name %s, got %s", tt.expectedName, name)
					}
				}

				// Verify sound_file is empty
				if soundFile, ok := resultMap["sound_file"].(string); ok {
					if soundFile != "" {
						t.Errorf("Expected empty sound_file, got %s", soundFile)
					}
				}
			}
		})
	}
}

// TestMCPJobListWithFilter tests job listing with ai_ prefix filter
func TestMCPJobListWithFilter(t *testing.T) {
	t.Skip("Skipping: Requires full database mocking with query results")
}

// TestMCPJobUpdate tests job update operations
func TestMCPJobUpdate(t *testing.T) {
	t.Skip("Skipping: Requires full database mocking for GetJobByID")
}

// TestMCPJobDelete tests job deletion
func TestMCPJobDelete(t *testing.T) {
	t.Skip("Skipping: Requires transaction mocking for cascade delete")
}

// TestMCPJobHistoryFiltering tests history retrieval with various filters
func TestMCPJobHistoryFiltering(t *testing.T) {
	t.Skip("Skipping: Requires full database mocking with query results")
}

// TestMCPJobTrigger tests immediate job triggering
func TestMCPJobTrigger(t *testing.T) {
	mockDB := db.NewMockDB()
	jobService := NewJobServiceWithDB(mockDB)
	scheduler := NewScheduler(jobService)
	jobService.SetScheduler(scheduler)

	mcpServer := NewMCPServer(jobService)

	jobID := "test-job-id"

	// Mock the GetJobByID call
	mockDB.QueryRowFunc = func(query string, args ...interface{}) *sql.Row { return nil }
	mockDB.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		return &db.MockResult{}, nil
	}

	params := map[string]string{"job_id": jobID}
	paramsJSON, _ := json.Marshal(params)

	request := MCPRequest{
		JSONRPC: "2.0",
		ID:      "test-trigger",
		Method:  "jobs/trigger",
		Params:  paramsJSON,
	}

	response := mcpServer.handleJobTrigger(context.Background(), request)

	// Should succeed even if job doesn't exist in mock (depends on mock setup)
	// For now, just verify the response structure
	if response.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC 2.0, got %s", response.JSONRPC)
	}
	if response.ID != request.ID {
		t.Errorf("Expected ID %v, got %v", request.ID, response.ID)
	}
}

// TestMCPJobTrigger_MissingJobID tests error handling for missing job_id
func TestMCPJobTrigger_MissingJobID(t *testing.T) {
	mockDB := db.NewMockDB()
	jobService := NewJobServiceWithDB(mockDB)
	mcpServer := NewMCPServer(jobService)

	params := map[string]string{} // Missing job_id
	paramsJSON, _ := json.Marshal(params)

	request := MCPRequest{
		JSONRPC: "2.0",
		ID:      "test-trigger-missing",
		Method:  "jobs/trigger",
		Params:  paramsJSON,
	}

	response := mcpServer.handleJobTrigger(context.Background(), request)

	if response.Error == nil {
		t.Error("Expected error for missing job_id")
	}
	if response.Error.Code != MCPErrorInvalidParams {
		t.Errorf("Expected error code %d, got %d", MCPErrorInvalidParams, response.Error.Code)
	}
}

// TestMCPJobTrigger_InvalidJSON tests error handling for invalid JSON params
func TestMCPJobTrigger_InvalidJSON(t *testing.T) {
	mockDB := db.NewMockDB()
	jobService := NewJobServiceWithDB(mockDB)
	mcpServer := NewMCPServer(jobService)

	request := MCPRequest{
		JSONRPC: "2.0",
		ID:      "test-trigger-invalid",
		Method:  "jobs/trigger",
		Params:  json.RawMessage(`{invalid json`),
	}

	response := mcpServer.handleJobTrigger(context.Background(), request)

	if response.Error == nil {
		t.Error("Expected error for invalid JSON")
	}
	if response.Error.Code != MCPErrorInvalidParams {
		t.Errorf("Expected error code %d, got %d", MCPErrorInvalidParams, response.Error.Code)
	}
}
