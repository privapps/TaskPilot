package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"taskpilot/db"
	"taskpilot/models"

	_ "modernc.org/sqlite"
)

func newMCPTimeoutTestServer(t *testing.T) (*sql.DB, *MCPServer) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	schema := []string{
		`CREATE TABLE jobs (
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
			disable_macos_sleep_prevention BOOLEAN DEFAULT 0,
			timeout_minutes INTEGER
		)`,
		`CREATE TABLE defaults (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			working_directory TEXT,
			sound_file TEXT,
			on_success_cmd TEXT,
			api_port INTEGER DEFAULT 8080
		)`,
	}
	for _, statement := range schema {
		if _, err := sqlDB.Exec(statement); err != nil {
			_ = sqlDB.Close()
			t.Fatalf("create MCP test schema: %v", err)
		}
	}

	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return sqlDB, NewMCPServer(NewJobServiceWithDB(&db.Database{DB: sqlDB}))
}

func callMCPTool(t *testing.T, server *MCPServer, name string, arguments map[string]interface{}) MCPResponse {
	t.Helper()

	params, err := json.Marshal(map[string]interface{}{
		"name":      name,
		"arguments": arguments,
	})
	if err != nil {
		t.Fatalf("marshal MCP tool arguments: %v", err)
	}
	return server.HandleRequest(context.Background(), MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  params,
	})
}

func decodeMCPJob(t *testing.T, response MCPResponse) models.Job {
	t.Helper()

	var job models.Job
	if err := json.Unmarshal([]byte(mcpToolResultText(t, response)), &job); err != nil {
		t.Fatalf("decode MCP job result: %v", err)
	}
	return job
}

func mcpToolResultText(t *testing.T, response MCPResponse) string {
	t.Helper()

	if response.Error != nil {
		t.Fatalf("MCP request failed: code %d, message %q", response.Error.Code, response.Error.Message)
	}
	result, ok := response.Result.(MCPToolResult)
	if !ok || len(result.Content) == 0 {
		t.Fatalf("MCP result = %#v, want tool result with content", response.Result)
	}
	return result.Content[0].Text
}

func TestMCPCreateReturnsTimeoutMinutes(t *testing.T) {
	_, server := newMCPTimeoutTestServer(t)

	response := callMCPTool(t, server, "jobs_create", map[string]interface{}{
		"name":            "timeout-create",
		"command":         "echo test",
		"directory":       ".",
		"schedule":        "0 9 * * *",
		"schedule_type":   "cron",
		"timeout_minutes": 15,
	})
	created := decodeMCPJob(t, response)

	if created.TimeoutMinutes == nil || *created.TimeoutMinutes != 15 {
		t.Fatalf("created timeout_minutes = %v, want 15", created.TimeoutMinutes)
	}
}

func TestMCPCreateNullTimeoutUsesDefault(t *testing.T) {
	_, server := newMCPTimeoutTestServer(t)
	response := callMCPTool(t, server, "jobs_create", map[string]interface{}{
		"name":            "timeout-create-default",
		"command":         "echo test",
		"directory":       ".",
		"schedule":        "0 9 * * *",
		"schedule_type":   "cron",
		"timeout_minutes": nil,
	})
	created := decodeMCPJob(t, response)
	if created.TimeoutMinutes != nil {
		t.Fatalf("created timeout_minutes = %d, want null", *created.TimeoutMinutes)
	}
}

func TestMCPGetAndListReturnTimeoutMinutes(t *testing.T) {
	_, server := newMCPTimeoutTestServer(t)
	created := decodeMCPJob(t, callMCPTool(t, server, "jobs_create", map[string]interface{}{
		"name":            "timeout-get-list",
		"command":         "echo test",
		"directory":       ".",
		"schedule":        "0 9 * * *",
		"schedule_type":   "cron",
		"timeout_minutes": 25,
	}))

	var fetched map[string]interface{}
	getText := mcpToolResultText(t, callMCPTool(t, server, "jobs_get", map[string]interface{}{
		"id": created.ID,
	}))
	if err := json.Unmarshal([]byte(getText), &fetched); err != nil {
		t.Fatalf("decode MCP get result: %v", err)
	}
	if value, exists := fetched["timeout_minutes"]; !exists || value != float64(25) {
		t.Errorf("jobs_get timeout_minutes = %v (present %t), want 25", value, exists)
	}

	var listed []map[string]interface{}
	listText := mcpToolResultText(t, callMCPTool(t, server, "jobs_list", map[string]interface{}{}))
	if err := json.Unmarshal([]byte(listText), &listed); err != nil {
		t.Fatalf("decode MCP list result: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("jobs_list returned %d jobs, want 1", len(listed))
	}
	if value, exists := listed[0]["timeout_minutes"]; !exists || value != float64(25) {
		t.Errorf("jobs_list timeout_minutes = %v (present %t), want 25", value, exists)
	}
}

func TestMCPUpdateTimeoutMinutesCanSetPreserveAndClear(t *testing.T) {
	_, server := newMCPTimeoutTestServer(t)
	created := decodeMCPJob(t, callMCPTool(t, server, "jobs_create", map[string]interface{}{
		"name":          "timeout-update",
		"command":       "echo test",
		"directory":     ".",
		"schedule":      "0 9 * * *",
		"schedule_type": "cron",
	}))

	set := decodeMCPJob(t, callMCPTool(t, server, "jobs_update", map[string]interface{}{
		"id":              created.ID,
		"timeout_minutes": 30,
	}))
	if set.TimeoutMinutes == nil || *set.TimeoutMinutes != 30 {
		t.Fatalf("updated timeout_minutes = %v, want 30", set.TimeoutMinutes)
	}

	preserved := decodeMCPJob(t, callMCPTool(t, server, "jobs_update", map[string]interface{}{
		"id":      created.ID,
		"command": "echo changed",
	}))
	if preserved.TimeoutMinutes == nil || *preserved.TimeoutMinutes != 30 {
		t.Fatalf("omitted timeout_minutes = %v, want preserved value 30", preserved.TimeoutMinutes)
	}

	cleared := decodeMCPJob(t, callMCPTool(t, server, "jobs_update", map[string]interface{}{
		"id":              created.ID,
		"timeout_minutes": nil,
	}))
	if cleared.TimeoutMinutes != nil {
		t.Fatalf("explicit null timeout_minutes = %d, want cleared", *cleared.TimeoutMinutes)
	}
	var clearedPayload map[string]interface{}
	clearedText := mcpToolResultText(t, callMCPTool(t, server, "jobs_get", map[string]interface{}{
		"id": created.ID,
	}))
	if err := json.Unmarshal([]byte(clearedText), &clearedPayload); err != nil {
		t.Fatalf("decode cleared MCP get result: %v", err)
	}
	if value, exists := clearedPayload["timeout_minutes"]; !exists || value != nil {
		t.Errorf("cleared jobs_get timeout_minutes = %v (present %t), want null", value, exists)
	}
}

func TestMCPTimeoutAcceptsInclusiveBounds(t *testing.T) {
	_, server := newMCPTimeoutTestServer(t)
	created := createMCPJob(t, server, "minimum-timeout", 1)
	updated := decodeMCPJob(t, callMCPTool(t, server, "jobs_update", map[string]interface{}{
		"id":              created.ID,
		"timeout_minutes": 1440,
	}))
	if updated.TimeoutMinutes == nil || *updated.TimeoutMinutes != 1440 {
		t.Fatalf("updated timeout_minutes = %v, want 1440", updated.TimeoutMinutes)
	}
}

func TestMCPTimeoutToolsDescribeNullableRange(t *testing.T) {
	_, server := newMCPTimeoutTestServer(t)
	response := server.HandleRequest(context.Background(), MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	})
	if response.Error != nil {
		t.Fatalf("tools/list failed: %v", response.Error)
	}

	payload, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatalf("marshal tools/list result: %v", err)
	}
	var listed struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]struct {
					Type        json.RawMessage `json:"type"`
					Minimum     int             `json:"minimum"`
					Maximum     int             `json:"maximum"`
					Description string          `json:"description"`
				} `json:"properties"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(payload, &listed); err != nil {
		t.Fatalf("decode tools/list result: %v", err)
	}

	for _, toolName := range []string{"jobs_create", "jobs_update"} {
		t.Run(toolName, func(t *testing.T) {
			for _, tool := range listed.Tools {
				if tool.Name != toolName {
					continue
				}
				timeout, exists := tool.InputSchema.Properties["timeout_minutes"]
				if !exists {
					t.Fatal("timeout_minutes is missing from tool schema")
				}
				var types []string
				if err := json.Unmarshal(timeout.Type, &types); err != nil {
					t.Fatalf("decode timeout_minutes type: %v", err)
				}
				if len(types) != 2 || types[0] != "integer" || types[1] != "null" {
					t.Errorf("timeout_minutes type = %v, want [integer null]", types)
				}
				if timeout.Minimum != 1 || timeout.Maximum != 1440 {
					t.Errorf("timeout_minutes range = %d–%d, want 1–1440", timeout.Minimum, timeout.Maximum)
				}
				if !strings.Contains(timeout.Description, "minutes") || !strings.Contains(timeout.Description, "1440") {
					t.Errorf("timeout_minutes description %q does not document its unit and range", timeout.Description)
				}
				return
			}
			t.Fatalf("%s tool missing from tools/list", toolName)
		})
	}
}

func TestMCPTimeoutRejectsOutOfRangeIntegers(t *testing.T) {
	for _, timeoutMinutes := range []int{0, -1, 1441} {
		t.Run(fmt.Sprintf("%d", timeoutMinutes), func(t *testing.T) {
			t.Run("create", func(t *testing.T) {
				_, server := newMCPTimeoutTestServer(t)
				response := callMCPTool(t, server, "jobs_create", map[string]interface{}{
					"name":            "invalid-timeout-create",
					"command":         "echo test",
					"directory":       ".",
					"schedule":        "0 9 * * *",
					"schedule_type":   "cron",
					"timeout_minutes": timeoutMinutes,
				})
				assertMCPTimeoutValidationError(t, response)

				listed := decodeMCPJobs(t, callMCPTool(t, server, "jobs_list", map[string]interface{}{}))
				if len(listed) != 0 {
					t.Errorf("invalid create left %d jobs, want none", len(listed))
				}
			})

			t.Run("update", func(t *testing.T) {
				_, server := newMCPTimeoutTestServer(t)
				created := createMCPJob(t, server, "valid-timeout-update", 12)
				response := callMCPTool(t, server, "jobs_update", map[string]interface{}{
					"id":              created.ID,
					"timeout_minutes": timeoutMinutes,
				})
				assertMCPTimeoutValidationError(t, response)

				stored := decodeMCPJob(t, callMCPTool(t, server, "jobs_get", map[string]interface{}{
					"id": created.ID,
				}))
				if stored.TimeoutMinutes == nil || *stored.TimeoutMinutes != 12 {
					t.Errorf("invalid update left timeout_minutes = %v, want 12", stored.TimeoutMinutes)
				}
			})
		})
	}
}

func TestMCPTimeoutRejectsFractionalJSON(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		_, server := newMCPTimeoutTestServer(t)
		response := callMCPTool(t, server, "jobs_create", map[string]interface{}{
			"name":            "fractional-timeout-create",
			"command":         "echo test",
			"directory":       ".",
			"schedule":        "0 9 * * *",
			"schedule_type":   "cron",
			"timeout_minutes": 1.5,
		})
		assertMCPInvalidParams(t, response)

		listed := decodeMCPJobs(t, callMCPTool(t, server, "jobs_list", map[string]interface{}{}))
		if len(listed) != 0 {
			t.Errorf("fractional create left %d jobs, want none", len(listed))
		}
	})

	t.Run("update", func(t *testing.T) {
		_, server := newMCPTimeoutTestServer(t)
		created := createMCPJob(t, server, "valid-fractional-update", 12)
		response := callMCPTool(t, server, "jobs_update", map[string]interface{}{
			"id":              created.ID,
			"timeout_minutes": 1.5,
		})
		assertMCPInvalidParams(t, response)

		stored := decodeMCPJob(t, callMCPTool(t, server, "jobs_get", map[string]interface{}{
			"id": created.ID,
		}))
		if stored.TimeoutMinutes == nil || *stored.TimeoutMinutes != 12 {
			t.Errorf("fractional update left timeout_minutes = %v, want 12", stored.TimeoutMinutes)
		}
	})
}

func createMCPJob(t *testing.T, server *MCPServer, name string, timeoutMinutes int) models.Job {
	t.Helper()

	return decodeMCPJob(t, callMCPTool(t, server, "jobs_create", map[string]interface{}{
		"name":            name,
		"command":         "echo test",
		"directory":       ".",
		"schedule":        "0 9 * * *",
		"schedule_type":   "cron",
		"timeout_minutes": timeoutMinutes,
	}))
}

func decodeMCPJobs(t *testing.T, response MCPResponse) []models.Job {
	t.Helper()

	var jobs []models.Job
	if err := json.Unmarshal([]byte(mcpToolResultText(t, response)), &jobs); err != nil {
		t.Fatalf("decode MCP job list: %v", err)
	}
	return jobs
}

func assertMCPTimeoutValidationError(t *testing.T, response MCPResponse) {
	t.Helper()

	assertMCPInvalidParams(t, response)
	if response.Error.Message != "timeout_minutes must be between 1 and 1440 minutes" {
		t.Errorf("validation error = %q, want JobService timeout range message", response.Error.Message)
	}
}

func assertMCPInvalidParams(t *testing.T, response MCPResponse) {
	t.Helper()

	if response.Error == nil {
		t.Fatalf("MCP response = %#v, want invalid-params error", response.Result)
	}
	if response.Error.Code != MCPErrorInvalidParams {
		t.Errorf("MCP error code = %d, want %d", response.Error.Code, MCPErrorInvalidParams)
	}
	if response.Result != nil {
		t.Errorf("MCP response includes success-shaped result %#v alongside error", response.Result)
	}
}
