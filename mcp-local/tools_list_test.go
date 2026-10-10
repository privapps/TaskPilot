package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestStaticToolsListDocumentsTimeoutMinutes(t *testing.T) {
	content, err := os.ReadFile("tools_list.json")
	if err != nil {
		t.Fatalf("read tools_list.json: %v", err)
	}

	var response struct {
		Result struct {
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
		} `json:"result"`
	}
	if err := json.Unmarshal(content, &response); err != nil {
		t.Fatalf("decode tools_list.json: %v", err)
	}

	for _, toolName := range []string{"jobs_create", "jobs_update"} {
		t.Run(toolName, func(t *testing.T) {
			for _, tool := range response.Result.Tools {
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
			t.Fatalf("%s tool missing from tools_list.json", toolName)
		})
	}
}
