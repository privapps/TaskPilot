package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"taskpilot/models"
)

func TestRESTDefaultsCanReadUpdateAndRejectInvalidTimeouts(t *testing.T) {
	_, jobService, _ := newSchedulerTestHarness(t)
	handler := NewAPIServer(jobService, 8080).handler()

	getDefaults := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/defaults", nil))
		return response
	}

	initialResponse := getDefaults()
	if initialResponse.Code != http.StatusOK {
		t.Fatalf("initial GET /api/defaults status = %d, want %d: %s", initialResponse.Code, http.StatusOK, initialResponse.Body.String())
	}
	var initial models.Defaults
	if err := json.Unmarshal(initialResponse.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial defaults: %v", err)
	}
	if initial.DefaultTimeoutMinutes != 10 {
		t.Fatalf("initial default_timeout_minutes = %d, want 10", initial.DefaultTimeoutMinutes)
	}

	updateResponse := httptest.NewRecorder()
	handler.ServeHTTP(updateResponse, httptest.NewRequest(http.MethodPut, "/api/defaults", strings.NewReader(
		`{"working_directory":"/api/default","sound_file":"","on_success_cmd":"","api_port":8080,"default_timeout_minutes":37}`,
	)))
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("PUT /api/defaults status = %d, want %d: %s", updateResponse.Code, http.StatusOK, updateResponse.Body.String())
	}
	var updated models.Defaults
	if err := json.Unmarshal(updateResponse.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated defaults: %v", err)
	}
	if updated.DefaultTimeoutMinutes != 37 {
		t.Fatalf("updated default_timeout_minutes = %d, want 37", updated.DefaultTimeoutMinutes)
	}

	fetchedResponse := getDefaults()
	var fetched models.Defaults
	if fetchedResponse.Code != http.StatusOK {
		t.Fatalf("GET /api/defaults after update status = %d, want %d", fetchedResponse.Code, http.StatusOK)
	}
	if err := json.Unmarshal(fetchedResponse.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode fetched defaults: %v", err)
	}
	if fetched.DefaultTimeoutMinutes != 37 {
		t.Errorf("GET /api/defaults returned timeout %d after update, want 37", fetched.DefaultTimeoutMinutes)
	}

	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, httptest.NewRequest(http.MethodPut, "/api/defaults", strings.NewReader(
		`{"working_directory":"/invalid","api_port":8080,"default_timeout_minutes":1441}`,
	)))
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT /api/defaults status = %d, want %d", invalidResponse.Code, http.StatusBadRequest)
	}
	if !strings.Contains(invalidResponse.Body.String(), "default_timeout_minutes must be between 1 and 1440 minutes") {
		t.Errorf("invalid PUT response %q does not include the default timeout validation error", invalidResponse.Body.String())
	}

	afterInvalidResponse := getDefaults()
	if err := json.Unmarshal(afterInvalidResponse.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode defaults after invalid update: %v", err)
	}
	if fetched.DefaultTimeoutMinutes != 37 || fetched.WorkingDirectory != "/api/default" {
		t.Errorf("invalid update changed stored defaults: %+v", fetched)
	}
}

func TestRESTDefaultsEndpointDocumentsTimeoutRange(t *testing.T) {
	_, jobService, _ := newSchedulerTestHarness(t)
	handler := NewAPIServer(jobService, 8080).handler()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/swagger", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/swagger status = %d, want %d", response.Code, http.StatusOK)
	}

	var openAPI map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &openAPI); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	paths, ok := openAPI["paths"].(map[string]interface{})
	if !ok {
		t.Fatal("OpenAPI document has no paths")
	}
	defaultsPath, ok := paths["/api/defaults"].(map[string]interface{})
	if !ok {
		t.Fatal("OpenAPI document does not describe /api/defaults")
	}
	if _, ok := defaultsPath["get"]; !ok {
		t.Error("OpenAPI document does not describe GET /api/defaults")
	}
	if _, ok := defaultsPath["put"]; !ok {
		t.Error("OpenAPI document does not describe PUT /api/defaults")
	}

	components := openAPI["components"].(map[string]interface{})
	schemas := components["schemas"].(map[string]interface{})
	defaultsSchema := schemas["Defaults"].(map[string]interface{})
	properties := defaultsSchema["properties"].(map[string]interface{})
	timeoutSchema := properties["default_timeout_minutes"].(map[string]interface{})
	if timeoutSchema["type"] != "integer" ||
		timeoutSchema["minimum"] != float64(1) ||
		timeoutSchema["maximum"] != float64(1440) {
		t.Errorf("default_timeout_minutes schema = %#v, want integer from 1 to 1440", timeoutSchema)
	}
}
