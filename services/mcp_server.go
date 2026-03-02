package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"taskpilot/models"
	"time"

	"github.com/google/uuid"
)

// MCPServer handles Model Context Protocol connections and events
type MCPServer struct {
	jobService *JobService
	scheduler  *Scheduler
	clients    map[string]chan MCPEvent
	mu         sync.RWMutex
}

// NewMCPServer creates a new MCP server instance
func NewMCPServer(jobService *JobService) *MCPServer {
	return &MCPServer{
		jobService: jobService,
		clients:    make(map[string]chan MCPEvent),
	}
}

// SetScheduler sets the scheduler for job execution
func (s *MCPServer) SetScheduler(scheduler *Scheduler) {
	s.scheduler = scheduler
}

// MCPEvent represents an event to be sent to MCP clients
type MCPEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// MCP event types
const (
	EventJobCreated   = "job.created"
	EventJobUpdated   = "job.updated"
	EventJobDeleted   = "job.deleted"
	EventJobExecuting = "job.executing"
	EventJobCompleted = "job.completed"
	EventJobFailed    = "job.failed"
)

// MCPRequest represents a JSON-RPC 2.0 request
type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCPResponse represents a JSON-RPC 2.0 response
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPError represents a JSON-RPC 2.0 error
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP error codes
const (
	MCPErrorParseError     = -32700
	MCPErrorInvalidRequest = -32600
	MCPErrorMethodNotFound = -32601
	MCPErrorInvalidParams  = -32602
	MCPErrorInternalError  = -32603
)

// MCPContent represents a piece of content in MCP format
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MCPToolResult represents the result of a tool call in MCP format
type MCPToolResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError"`
}

// wrapMCPResult wraps data in the MCP content format
func wrapMCPResult(data interface{}) (MCPToolResult, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return MCPToolResult{}, err
	}
	return MCPToolResult{
		Content: []MCPContent{
			{
				Type: "text",
				Text: string(jsonBytes),
			},
		},
		IsError: false,
	}, nil
}

// RegisterClient adds a new client connection
func (s *MCPServer) RegisterClient(clientID string) chan MCPEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan MCPEvent, 100)
	s.clients[clientID] = ch
	log.Printf("MCP client registered: %s", clientID)
	return ch
}

// UnregisterClient removes a client connection
func (s *MCPServer) UnregisterClient(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ch, exists := s.clients[clientID]; exists {
		close(ch)
		delete(s.clients, clientID)
		log.Printf("MCP client unregistered: %s", clientID)
	}
}

// BroadcastEvent sends an event to all connected clients
func (s *MCPServer) BroadcastEvent(event MCPEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for clientID, ch := range s.clients {
		select {
		case ch <- event:
			// Event sent successfully
		case <-time.After(1 * time.Second):
			log.Printf("Timeout sending event to client %s", clientID)
		}
	}
}

// SendEventToClient sends an event to a specific client
func (s *MCPServer) SendEventToClient(clientID string, event MCPEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if ch, exists := s.clients[clientID]; exists {
		select {
		case ch <- event:
			// Event sent successfully
		case <-time.After(1 * time.Second):
			log.Printf("Timeout sending event to client %s", clientID)
		}
	}
}

// enforceMCPNaming ensures job names have the ai_ prefix
func enforceMCPNaming(jobName string) string {
	if jobName == "" {
		return "ai_" + uuid.New().String()[:8]
	}
	if !strings.HasPrefix(jobName, "ai_") {
		return "ai_" + jobName
	}
	return jobName
}

// handleInitialize processes the MCP initialize request
func (s *MCPServer) handleInitialize(ctx context.Context, req MCPRequest) MCPResponse {
	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "taskpilot",
				"version": "1.0.0",
			},
		},
	}
}

// handleInitialized processes the MCP initialized notification
func (s *MCPServer) handleInitialized(ctx context.Context, req MCPRequest) MCPResponse {
	// Initialized is a notification, no response needed (but we return empty success)
	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{},
	}
}

// handleToolsList returns the list of available MCP tools
func (s *MCPServer) handleToolsList(ctx context.Context, req MCPRequest) MCPResponse {
	tools := []map[string]interface{}{
		{
			"name":        "jobs_create",
			"description": "Create a new scheduled job with automatic ai_ prefix enforcement",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Job name (will be auto-prefixed with ai_)",
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Command to execute",
					},
					"directory": map[string]interface{}{
						"type":        "string",
						"description": "Working directory for command execution",
					},
					"schedule": map[string]interface{}{
						"type":        "string",
						"description": "Cron expression for scheduling",
					},
					"schedule_type": map[string]interface{}{
						"type":        "string",
						"description": "Schedule type: cron, delay, datetime, or immediate",
					},
				},
				"required": []string{"command"},
			},
		},
		{
			"name":        "jobs_get",
			"description": "Retrieve a specific job by ID",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Job UUID",
					},
				},
				"required": []string{"id"},
			},
		},
		{
			"name":        "jobs_list",
			"description": "List all jobs, optionally filtered by ai_ prefix",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"filter_ai_prefix": map[string]interface{}{
						"type":        "boolean",
						"description": "Filter to show only AI-created jobs",
					},
				},
			},
		},
		{
			"name":        "jobs_update",
			"description": "Update an existing job",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Job UUID",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "New job name (will be auto-prefixed with ai_)",
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "New command",
					},
					"schedule": map[string]interface{}{
						"type":        "string",
						"description": "New schedule (cron expression)",
					},
					"paused": map[string]interface{}{
						"type":        "boolean",
						"description": "Pause/unpause the job",
					},
				},
				"required": []string{"id"},
			},
		},
		{
			"name":        "jobs_delete",
			"description": "Delete a job and its execution history",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Job UUID",
					},
				},
				"required": []string{"id"},
			},
		},
		{
			"name":        "jobs_execute",
			"description": "Execute a job immediately",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Job UUID",
					},
				},
				"required": []string{"id"},
			},
		},
		{
			"name":        "jobs_pause",
			"description": "Pause a job's schedule",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Job UUID",
					},
				},
				"required": []string{"id"},
			},
		},
		{
			"name":        "jobs_resume",
			"description": "Resume a paused job",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Job UUID",
					},
				},
				"required": []string{"id"},
			},
		},
		{
			"name":        "jobs_trigger",
			"description": "Trigger a job to execute immediately without affecting its schedule",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "Job UUID to trigger",
					},
				},
				"required": []string{"job_id"},
			},
		},
		{
			"name":        "jobs_history",
			"description": "Retrieve job execution history with optional filtering",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "Filter by specific job ID",
					},
					"start_date": map[string]interface{}{
						"type":        "integer",
						"description": "Filter by start timestamp (Unix)",
					},
					"end_date": map[string]interface{}{
						"type":        "integer",
						"description": "Filter by end timestamp (Unix)",
					},
					"filter_ai_prefix": map[string]interface{}{
						"type":        "boolean",
						"description": "Show only AI-created jobs",
					},
				},
			},
		},
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"tools": tools,
		},
	}
}

// handleToolsCall executes a tool by name with provided arguments
func (s *MCPServer) handleToolsCall(ctx context.Context, req MCPRequest) MCPResponse {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "Invalid parameters: " + err.Error(),
			},
		}
	}

	if params.Name == "" {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "Tool name is required",
			},
		}
	}

	// Convert arguments back to JSON for reuse with existing handlers
	argsJSON, err := json.Marshal(params.Arguments)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to process arguments: " + err.Error(),
			},
		}
	}

	// Create a new request with the tool-specific method
	toolReq := MCPRequest{
		JSONRPC: req.JSONRPC,
		ID:      req.ID,
		Params:  argsJSON,
	}

	// Route to the appropriate handler based on tool name
	switch params.Name {
	case "jobs_create":
		toolReq.Method = "jobs/create"
		return s.handleJobCreate(ctx, toolReq)
	case "jobs_get":
		toolReq.Method = "jobs/get"
		return s.handleJobGet(ctx, toolReq)
	case "jobs_list":
		toolReq.Method = "jobs/list"
		return s.handleJobList(ctx, toolReq)
	case "jobs_update":
		toolReq.Method = "jobs/update"
		return s.handleJobUpdate(ctx, toolReq)
	case "jobs_delete":
		toolReq.Method = "jobs/delete"
		return s.handleJobDelete(ctx, toolReq)
	case "jobs_execute":
		toolReq.Method = "jobs/execute"
		return s.handleJobExecute(ctx, toolReq)
	case "jobs_pause":
		toolReq.Method = "jobs/pause"
		return s.handleJobPause(ctx, toolReq)
	case "jobs_resume":
		toolReq.Method = "jobs/resume"
		return s.handleJobResume(ctx, toolReq)
	case "jobs_trigger":
		toolReq.Method = "jobs/trigger"
		return s.handleJobTrigger(ctx, toolReq)
	case "jobs_history":
		toolReq.Method = "jobs/history"
		return s.handleJobHistory(ctx, toolReq)
	default:
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorMethodNotFound,
				Message: fmt.Sprintf("Unknown tool: %s", params.Name),
			},
		}
	}
}

// handleSystemTime returns the server's current time
func (s *MCPServer) handleSystemTime(ctx context.Context, req MCPRequest) MCPResponse {
	now := time.Now()
	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"timestamp": now.Unix(),
			"iso8601":   now.Format(time.RFC3339),
		},
	}
}

// Placeholder handler methods - Job operations
func (s *MCPServer) HandleRequest(ctx context.Context, req MCPRequest) MCPResponse {
	// Panic recovery
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in MCP request handler: %v", r)
		}
	}()

	// Validate request
	if req.JSONRPC != "2.0" {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidRequest,
				Message: "Invalid JSON-RPC version, must be 2.0",
			},
		}
	}

	// Handle notifications (requests without ID) - these should not receive responses
	// Per JSON-RPC 2.0 spec, notifications are one-way messages
	if req.ID == nil {
		switch req.Method {
		case "notifications/initialized", "initialized":
			log.Printf("Received initialized notification")
			// No response for notifications
			return MCPResponse{}
		case "notifications/cancelled":
			var params struct {
				RequestID interface{} `json:"requestId"`
				Reason    string      `json:"reason"`
			}
			if err := json.Unmarshal(req.Params, &params); err == nil {
				log.Printf("Request %v cancelled: %s", params.RequestID, params.Reason)
			}
			// No response for notifications
			return MCPResponse{}
		case "notifications/progress":
			// Handle progress notifications silently
			return MCPResponse{}
		default:
			// Unknown notification - log and ignore
			log.Printf("Received unknown notification: %s", req.Method)
			return MCPResponse{}
		}
	}

	// Route to appropriate handler for requests with ID
	switch req.Method {
	case "initialize":
		return s.handleInitialize(ctx, req)
	case "initialized":
		return s.handleInitialized(ctx, req)
	case "tools/list":
		return s.handleToolsList(ctx, req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "system/time":
		return s.handleSystemTime(ctx, req)
	case "jobs/create":
		return s.handleJobCreate(ctx, req)
	case "jobs/get":
		return s.handleJobGet(ctx, req)
	case "jobs/list":
		return s.handleJobList(ctx, req)
	case "jobs/update":
		return s.handleJobUpdate(ctx, req)
	case "jobs/delete":
		return s.handleJobDelete(ctx, req)
	case "jobs/execute":
		return s.handleJobExecute(ctx, req)
	case "jobs/pause":
		return s.handleJobPause(ctx, req)
	case "jobs/resume":
		return s.handleJobResume(ctx, req)
	case "jobs/history":
		return s.handleJobHistory(ctx, req)
	default:
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorMethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

// Placeholder handler methods - Job operations
func (s *MCPServer) handleJobCreate(ctx context.Context, req MCPRequest) MCPResponse {
	var params struct {
		Name         string `json:"name"`
		Command      string `json:"command"`
		Directory    string `json:"directory,omitempty"`
		Schedule     string `json:"schedule,omitempty"`
		ScheduleType string `json:"schedule_type,omitempty"`
		SoundFile    string `json:"sound_file,omitempty"`
		OnSuccessCmd string `json:"on_success_cmd,omitempty"`
		DelayMinutes *int   `json:"delay_minutes,omitempty"`
		RunAt        *int64 `json:"run_at,omitempty"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "Invalid parameters: " + err.Error(),
			},
		}
	}

	// Validate required fields
	if params.Command == "" {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "command is required",
			},
		}
	}

	// Enforce ai_ prefix on job name
	params.Name = enforceMCPNaming(params.Name)

	// Ensure sound_file is empty for MCP created jobs
	// Use "none" to bypass default sound_file application
	params.SoundFile = "none"

	// Create job
	job := models.Job{
		Name:         params.Name,
		Command:      params.Command,
		Directory:    params.Directory,
		Schedule:     params.Schedule,
		ScheduleType: params.ScheduleType,
		SoundFile:    params.SoundFile,
		OnSuccessCmd: params.OnSuccessCmd,
		DelayMinutes: params.DelayMinutes,
		RunAt:        params.RunAt,
	}

	createdJob, err := s.jobService.CreateJob(job)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to create job: " + err.Error(),
			},
		}
	}

	// Ensure sound_file is empty in the response (in case defaults were applied)
	createdJob.SoundFile = ""

	// Broadcast job created event
	s.BroadcastEvent(MCPEvent{
		Type: EventJobCreated,
		Data: createdJob,
	})

	wrappedResult, err := wrapMCPResult(createdJob)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to format response: " + err.Error(),
			},
		}
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  wrappedResult,
	}
}

func (s *MCPServer) handleJobGet(ctx context.Context, req MCPRequest) MCPResponse {
	var params struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "Invalid parameters: " + err.Error(),
			},
		}
	}

	if params.ID == "" {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "id is required",
			},
		}
	}

	job, err := s.jobService.GetJobByID(params.ID)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Job not found: " + err.Error(),
			},
		}
	}

	wrappedResult, err := wrapMCPResult(job)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to format response: " + err.Error(),
			},
		}
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  wrappedResult,
	}
}

func (s *MCPServer) handleJobList(ctx context.Context, req MCPRequest) MCPResponse {
	var params struct {
		FilterAIPrefix bool `json:"filter_ai_prefix,omitempty"`
	}

	// Parse params if provided
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MCPResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &MCPError{
					Code:    MCPErrorInvalidParams,
					Message: "Invalid parameters: " + err.Error(),
				},
			}
		}
	}

	jobs, err := s.jobService.GetJobs()
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to retrieve jobs: " + err.Error(),
			},
		}
	}

	// Filter by ai_ prefix if requested
	if params.FilterAIPrefix {
		filteredJobs := make([]models.Job, 0)
		for _, job := range jobs {
			if strings.HasPrefix(job.Name, "ai_") {
				filteredJobs = append(filteredJobs, job)
			}
		}
		jobs = filteredJobs
	}

	wrappedResult, err := wrapMCPResult(jobs)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to format response: " + err.Error(),
			},
		}
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  wrappedResult,
	}
}

func (s *MCPServer) handleJobUpdate(ctx context.Context, req MCPRequest) MCPResponse {
	var params struct {
		ID           string  `json:"id"`
		Name         *string `json:"name,omitempty"`
		Command      *string `json:"command,omitempty"`
		Directory    *string `json:"directory,omitempty"`
		Schedule     *string `json:"schedule,omitempty"`
		ScheduleType *string `json:"schedule_type,omitempty"`
		SoundFile    *string `json:"sound_file,omitempty"`
		OnSuccessCmd *string `json:"on_success_cmd,omitempty"`
		DelayMinutes *int    `json:"delay_minutes,omitempty"`
		RunAt        *int64  `json:"run_at,omitempty"`
		Paused       *bool   `json:"paused,omitempty"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "Invalid parameters: " + err.Error(),
			},
		}
	}

	if params.ID == "" {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "id is required",
			},
		}
	}

	// Get existing job
	existingJob, err := s.jobService.GetJobByID(params.ID)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Job not found: " + err.Error(),
			},
		}
	}

	// Apply updates
	if params.Name != nil {
		existingJob.Name = enforceMCPNaming(*params.Name)
	}
	if params.Command != nil {
		existingJob.Command = *params.Command
	}
	if params.Directory != nil {
		existingJob.Directory = *params.Directory
	}
	if params.Schedule != nil {
		existingJob.Schedule = *params.Schedule
	}
	if params.ScheduleType != nil {
		existingJob.ScheduleType = *params.ScheduleType
	}
	// Ensure sound_file is always empty for MCP updated jobs
	existingJob.SoundFile = ""
	if params.OnSuccessCmd != nil {
		existingJob.OnSuccessCmd = *params.OnSuccessCmd
	}
	if params.DelayMinutes != nil {
		existingJob.DelayMinutes = params.DelayMinutes
	}
	if params.RunAt != nil {
		existingJob.RunAt = params.RunAt
	}
	if params.Paused != nil {
		existingJob.Paused = *params.Paused
	}

	updatedJob, err := s.jobService.UpdateJob(existingJob)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to update job: " + err.Error(),
			},
		}
	}

	// Ensure sound_file is empty in the response
	updatedJob.SoundFile = ""

	// Broadcast job updated event
	s.BroadcastEvent(MCPEvent{
		Type: EventJobUpdated,
		Data: updatedJob,
	})

	wrappedResult, err := wrapMCPResult(updatedJob)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to format response: " + err.Error(),
			},
		}
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  wrappedResult,
	}
}

func (s *MCPServer) handleJobDelete(ctx context.Context, req MCPRequest) MCPResponse {
	var params struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "Invalid parameters: " + err.Error(),
			},
		}
	}

	if params.ID == "" {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "id is required",
			},
		}
	}

	// Get job before deletion for event broadcasting
	job, err := s.jobService.GetJobByID(params.ID)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Job not found: " + err.Error(),
			},
		}
	}

	err = s.jobService.DeleteJob(params.ID)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to delete job: " + err.Error(),
			},
		}
	}

	// Broadcast job deleted event
	s.BroadcastEvent(MCPEvent{
		Type: EventJobDeleted,
		Data: map[string]interface{}{
			"id":   job.ID,
			"name": job.Name,
		},
	})

	result := map[string]string{"status": "deleted", "id": params.ID}
	wrappedResult, err := wrapMCPResult(result)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to format response: " + err.Error(),
			},
		}
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  wrappedResult,
	}
}

func (s *MCPServer) handleJobExecute(ctx context.Context, req MCPRequest) MCPResponse {
	var params struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "Invalid parameters: " + err.Error(),
			},
		}
	}

	if params.ID == "" {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "id is required",
			},
		}
	}

	job, err := s.jobService.GetJobByID(params.ID)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Job not found: " + err.Error(),
			},
		}
	}

	// Broadcast job executing event
	s.BroadcastEvent(MCPEvent{
		Type: EventJobExecuting,
		Data: map[string]interface{}{
			"id":   job.ID,
			"name": job.Name,
		},
	})

	// Execute job immediately using scheduler
	if s.scheduler != nil {
		go func() {
			// Use a goroutine to avoid blocking the response
			// The scheduler will handle the execution and update job status
			job.ScheduleType = models.ScheduleTypeImmediate
			if err := s.scheduler.ScheduleJob(job); err != nil {
				log.Printf("Failed to execute job %s: %v", job.ID, err)
				s.BroadcastEvent(MCPEvent{
					Type: EventJobFailed,
					Data: map[string]interface{}{
						"id":    job.ID,
						"name":  job.Name,
						"error": err.Error(),
					},
				})
			}
		}()
	}

	result := map[string]string{"status": "executing", "id": params.ID}
	wrappedResult, err := wrapMCPResult(result)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to format response: " + err.Error(),
			},
		}
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  wrappedResult,
	}
}

func (s *MCPServer) handleJobPause(ctx context.Context, req MCPRequest) MCPResponse {
	var params struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "Invalid parameters: " + err.Error(),
			},
		}
	}

	if params.ID == "" {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "id is required",
			},
		}
	}

	job, err := s.jobService.GetJobByID(params.ID)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Job not found: " + err.Error(),
			},
		}
	}

	// Pause the job
	job.Paused = true
	updatedJob, err := s.jobService.UpdateJob(job)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to pause job: " + err.Error(),
			},
		}
	}

	// Broadcast job updated event
	s.BroadcastEvent(MCPEvent{
		Type: EventJobUpdated,
		Data: updatedJob,
	})

	wrappedResult, err := wrapMCPResult(updatedJob)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to format response: " + err.Error(),
			},
		}
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  wrappedResult,
	}
}

func (s *MCPServer) handleJobResume(ctx context.Context, req MCPRequest) MCPResponse {
	var params struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "Invalid parameters: " + err.Error(),
			},
		}
	}

	if params.ID == "" {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "id is required",
			},
		}
	}

	job, err := s.jobService.GetJobByID(params.ID)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Job not found: " + err.Error(),
			},
		}
	}

	// Resume the job
	job.Paused = false
	updatedJob, err := s.jobService.UpdateJob(job)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to resume job: " + err.Error(),
			},
		}
	}

	// Broadcast job updated event
	s.BroadcastEvent(MCPEvent{
		Type: EventJobUpdated,
		Data: updatedJob,
	})

	wrappedResult, err := wrapMCPResult(updatedJob)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to format response: " + err.Error(),
			},
		}
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  wrappedResult,
	}
}

func (s *MCPServer) handleJobTrigger(ctx context.Context, req MCPRequest) MCPResponse {
	var params struct {
		JobID string `json:"job_id"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "Invalid parameters: " + err.Error(),
			},
		}
	}

	if params.JobID == "" {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInvalidParams,
				Message: "job_id is required",
			},
		}
	}

	// Trigger the job
	err := s.jobService.TriggerJob(params.JobID)
	if err != nil {
		errorCode := MCPErrorInternalError
		if strings.Contains(err.Error(), "not found") {
			errorCode = MCPErrorInvalidParams
		}
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    errorCode,
				Message: err.Error(),
			},
		}
	}

	result := map[string]string{
		"status":  "triggered",
		"message": fmt.Sprintf("Job %s triggered successfully", params.JobID),
	}

	wrappedResult, err := wrapMCPResult(result)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to format response: " + err.Error(),
			},
		}
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  wrappedResult,
	}
}

func (s *MCPServer) handleJobHistory(ctx context.Context, req MCPRequest) MCPResponse {
	var params struct {
		JobID          *string `json:"job_id,omitempty"`
		StartDate      *int64  `json:"start_date,omitempty"`
		EndDate        *int64  `json:"end_date,omitempty"`
		FilterAIPrefix bool    `json:"filter_ai_prefix,omitempty"`
		OrderAsc       bool    `json:"order_asc,omitempty"`
	}

	// Parse params if provided
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MCPResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &MCPError{
					Code:    MCPErrorInvalidParams,
					Message: "Invalid parameters: " + err.Error(),
				},
			}
		}
	}

	// Get history
	history, err := s.jobService.GetHistory(params.JobID, params.OrderAsc)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to retrieve history: " + err.Error(),
			},
		}
	}

	// Get all jobs for filtering by ai_ prefix
	var jobMap map[string]models.Job
	if params.FilterAIPrefix {
		jobs, err := s.jobService.GetJobs()
		if err != nil {
			return MCPResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &MCPError{
					Code:    MCPErrorInternalError,
					Message: "Failed to retrieve jobs for filtering: " + err.Error(),
				},
			}
		}
		jobMap = make(map[string]models.Job)
		for _, job := range jobs {
			jobMap[job.ID] = job
		}
	}

	// Apply filters
	filteredHistory := make([]models.History, 0)
	for _, h := range history {
		// Filter by date range
		if params.StartDate != nil && h.Timestamp < *params.StartDate {
			continue
		}
		if params.EndDate != nil && h.Timestamp > *params.EndDate {
			continue
		}

		// Filter by ai_ prefix
		if params.FilterAIPrefix {
			if job, exists := jobMap[h.JobID]; exists {
				if !strings.HasPrefix(job.Name, "ai_") {
					continue
				}
			} else {
				// Job doesn't exist anymore, skip
				continue
			}
		}

		filteredHistory = append(filteredHistory, h)
	}

	wrappedResult, err := wrapMCPResult(filteredHistory)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    MCPErrorInternalError,
				Message: "Failed to format response: " + err.Error(),
			},
		}
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  wrappedResult,
	}
}
