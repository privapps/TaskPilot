# TaskPilot

A lightweight, cross-platform desktop job scheduler built with Go and Wails v3.

---

**Spec-driven architecture:**
TaskPilot follows an "openspec" specification-driven development process. All major features, APIs, and integrations are implemented, documented, and tested according to formal specs in the `openspec/` folder (see `openspec/changes`).

---

## Features

- Create, update, and delete scheduled jobs (spec: openspec/changes/initial-setup/specs/job-management/spec.md)
- SQLite database for persistent storage
- Modern UI with Svelte 5 and Tailwind CSS (spec: openspec/changes/initial-setup/specs/project-structure/spec.md)
- Cross-platform support (macOS, Windows, Linux)
- **REST API server** for programmatic access to job management (spec: openspec/changes/rest-api-server/specs/rest-api-server/spec.md)
- Interactive Swagger API documentation
- Job execution history tracking (`/api/jobs/{job_id}/history` endpoint, spec: openspec/changes/api-enhancements/specs/job-history-endpoint/spec.md)
- Default job settings configuration (API server port and more; configurable, restart required)
- All major features and APIs are covered by testable requirements and scenarios in openspec docs

## Tech Stack

- **Backend:** Go 1.22+
- **Frontend:** Svelte 5 + Tailwind CSS
- **Framework:** Wails v2.10.1
- **Database:** SQLite (embedded)

## Prerequisites

- Go 1.22 or higher
- Node.js 18 or higher
- Wails CLI v2

## Installation

1. Clone the repository
2. Install dependencies:

```bash
# Install Go dependencies
go mod tidy

# Install frontend dependencies
cd frontend
npm install
cd ..
```

## Development

To run the application in development mode:

```bash
wails dev
```

This will start the application with hot-reload enabled for both frontend and backend changes.

## Building

To build the application for production:

```bash
wails build
```

The built application will be in the `build/bin` directory.

## Project Structure

```
.
├── main.go                    # Application entry point
├── app.go                     # Main app struct
├── API_DOCUMENTATION.md       # Complete REST API documentation
├── db/                        # Database package
│   └── database.go           # SQLite connection and migrations
├── models/                    # Data models
│   └── job.go                # Job, History, and Defaults structs
├── services/                  # Business logic
│   ├── job_service.go        # Job CRUD operations
│   ├── scheduler.go          # Job scheduling engine
│   ├── defaults_service.go   # Default settings management
│   └── api_server.go         # REST API server (NEW)
└── frontend/                  # Svelte frontend
    ├── src/
    │   ├── App.svelte        # Main UI component
    │   ├── DefaultsModal.svelte  # Settings dialog
    │   └── main.js           # Frontend entry point
    └── wailsjs/              # Generated Go-JS bindings
```

## Architecture

TaskPilot follows a service-based architecture:

- **JobService**: Handles database interactions for job definitions
- **Scheduler**: Manages job execution and scheduling
- **DefaultsService**: Manages default configuration settings
- **APIServer**: REST API server for programmatic access
- **Database**: SQLite with automatic schema migration
- **Frontend**: Reactive UI built with Svelte 5

## REST API

TaskPilot includes a built-in REST API server that enables programmatic access to job management features, **implemented and validated according to openspec specs** (`openspec/changes/rest-api-server/specs/rest-api-server/spec.md`).

### Quick Start

1. The API server starts automatically with the application
2. Default port: `8080` (configurable through Settings → Defaults; restart required for port changes)
3. Access interactive Swagger documentation: `http://localhost:8080/api/swagger`
4. Port changes are validated (spec: port must be 1024-65535) and persisted; invalid entries are rejected, and restart is required for new port to apply

### API Endpoints (spec-conformant)

- `GET /api/jobs` — List all jobs (returns JSON array)
- `POST /api/jobs` — Create a new job (returns created job or validation errors)
- `GET /api/jobs/:id` — Get job details (404 if not found)
- `PUT /api/jobs/:id` — Update a job (validation and error handling)
- `DELETE /api/jobs/:id` — Delete job (cascades to delete history records)
- `GET /api/jobs/:id/history` — Execution history for a specific job, supports `order=asc|desc` (spec-compliant; 404 for non-existent jobs, validated UUID)
- `GET /api/history` — List all execution history
- `GET /api/history/:id` — Get history record
- `DELETE /api/history/:id` — Delete history record

All endpoints, request/response format, and error cases match requirements and scenarios in the openspec docs.

### Example Usage

```bash
# List all jobs
curl http://localhost:8080/api/jobs

# Create a job
curl -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Backup Job",
    "command": "tar -czf backup.tar.gz /data",
    "schedule": "0 2 * * *",
    "schedule_type": "cron"
  }'

# Delete a job (and its history)
curl -X DELETE http://localhost:8080/api/jobs/{job-id}
```

For complete API documentation and formal requirements, see [API_DOCUMENTATION.md](API_DOCUMENTATION.md) and the relevant formal specs in [openspec/changes](openspec/changes).

## Database Schema

### Jobs Table
- `id` (TEXT): Unique job identifier
- `name` (TEXT): Job display name
- `command` (TEXT): Command to execute
- `directory` (TEXT): Working directory
- `schedule` (TEXT): Cron or interval expression
- `sound_file` (TEXT): Optional sound notification
- `on_success_cmd` (TEXT): Command to run on success
- `last_result` (TEXT): Last execution result
- `status` (TEXT): Current status (idle, running, failed)

### History Table
- `id` (INTEGER): Auto-increment ID
- `job_id` (TEXT): Reference to job
- `output` (TEXT): Command output
- `exit_code` (INTEGER): Process exit code
- `timestamp` (DATETIME): Execution time

## Testing

TaskPilot includes a comprehensive unit testing framework with automated test execution integrated into the build process.

### Running Tests

Run all unit tests:

```bash
go test ./...
```

Run tests with verbose output:

```bash
./scripts/test.sh -v
```

Run specific tests:

```bash
./scripts/test.sh -run TestJobService
```

Run tests with coverage:

```bash
./scripts/test.sh --coverage
```

### Coverage Reports

Generate a detailed coverage report:

```bash
./scripts/coverage.sh
```

This will:
- Run all tests with coverage measurement
- Display overall coverage percentage
- Check against the 70% coverage threshold
- Provide instructions for viewing HTML reports

View HTML coverage report:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Build Integration

Tests automatically run before every build:

```bash
wails build  # Tests run first, build fails if tests fail
wails dev    # Tests run before starting dev server
```

This ensures code quality by catching bugs before compilation.

### Test Organization

Tests follow Go conventions:
- Test files use `_test.go` suffix
- Located alongside source files (e.g., `job_service.go` → `job_service_test.go`)
- Table-driven test pattern for multiple scenarios
- Mock database for fast, isolated unit tests

### Coverage Threshold

The project maintains a **70% code coverage** target for the services layer. This pragmatic threshold ensures critical business logic is tested while allowing flexibility for edge cases and exploratory testing.

### Writing Tests

Example test pattern:

```go
func TestJobService_CreateJob(t *testing.T) {
    tests := []struct {
        name    string
        job     models.Job
        wantErr bool
    }{
        {name: "valid job", job: validJob, wantErr: false},
        {name: "invalid job", job: invalidJob, wantErr: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockDB := db.NewMockDB()
            service := NewJobServiceWithDB(mockDB)
            
            _, err := service.CreateJob(tt.job)
            
            if (err != nil) != tt.wantErr {
                t.Errorf("got error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

For more testing best practices, see the existing test files in `services/*_test.go`.

## MCP Server Integration

TaskPilot implements the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/), enabling integration with AI agents (like GitHub Copilot or VS Code extensions) for programmatic job management and automation. You can connect to the MCP server using either HTTP (for direct/SSE clients) or via the local `mcp-local` proxy for stdio-based agents.

### MCP Connection Types

#### 1. HTTP (Direct/SSE)
- The MCP server endpoint is available at: `http://localhost:<API_PORT>/api/mcp`
- Supports JSON-RPC 2.0 over HTTP POST. SSE (server-sent events) are supported for real-time updates to subscribed clients.
- **Default port**: 8080 (but your TaskPilot might use a different port; see below).
- 
**How to Connect:**
```bash
# Test connectivity (replace <API_PORT> with your actual port)
curl -X POST http://localhost:<API_PORT>/api/mcp \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
```
- To consume live events, connect using EventSource/SSE client to `/api/mcp/events` (if your framework supports it).

#### 2. Local Proxy (`mcp-local`) — For Copilot/VS Code/stdio/CLI Agents
- The `mcp-local` binary bridges stdio-based MCP clients and the TaskPilot HTTP server.
- Required for integrating with GitHub Copilot, VS Code MCP extension, or other tools that require a stdio transport.

Build the proxy:
```bash
cd mcp-local
go build
```

Launch (using your API port):
```bash
./mcp-local --url=http://localhost:<API_PORT>
# or set env variable
export TASKPILOT_API_URL=http://localhost:<API_PORT>
./mcp-local
```

#### 3. GitHub Copilot/VS Code Config Example
- **Find your port:** Check TaskPilot startup logs for the actual API port (often NOT `8080`). 

GitHub Copilot (`~/.copilot/mcp-config.json`):
```json
{
  "mcpServers": {
    "taskpilot": {
      "type": "stdio",
      "command": "/absolute/path/to/TaskPilot/mcp-local/mcp-local",
      "args": ["--url", "http://localhost:8080"]
    }
  }
}
```

VS Code example (`settings.json`):
```json
{
  "mcp.servers": {
    "taskpilot": {
      "command": "/absolute/path/to/TaskPilot/mcp-local/mcp-local",
      "args": ["--url", "http://localhost:8080"]
    }
  }
}
```

**Auto-Setup:**
To auto-generate the config and detect your API port:
```bash
./scripts/setup-copilot-mcp.sh
```

#### Troubleshooting
- **Request timed out errors:** Ensure the MCP proxy or client uses the correct TaskPilot API port (not always 8080!).
- **Copilot CLI hangs:** Use the provided wrapper (`mcp-local-wrapper.sh`) to silence stderr logs.
- **See [scripts/README.md](scripts/README.md) and [mcp-local/README.md](mcp-local/README.md) for additional troubleshooting details.**

#### Supported Tools
TaskPilot's MCP supports these 9 tools:
1. jobs_create – Create a new job (with `ai_` prefix enforced)
2. jobs_get – Get job by ID
3. jobs_list – List all jobs (or only AI jobs)
4. jobs_update – Update job (prefix enforced)
5. jobs_delete – Delete job
6. jobs_execute – Run job instantly
7. jobs_pause – Pause job scheduling
8. jobs_resume – Unpause job scheduling
9. jobs_history – View execution logs

**All jobs created via MCP are prefixed `ai_` and are silent by default (sound file is cleared).**

---

## MCP Testing

TaskPilot ships with scripts for fully automated MCP protocol & integration testing.

### Test Scripts

- **`test-mcp-http.sh`** – Test MCP methods via direct HTTP POST
- **`test-mcp-local.sh`** – Test MCP via the mcp-local stdio proxy

### Running MCP Tests

```bash
# HTTP tests (direct API calls)
TASKPILOT_PORT=41327 ./scripts/test-mcp-http.sh

# mcp-local tests (stdio proxy)
TASKPILOT_PORT=41327 ./scripts/test-mcp-local.sh
```

**Note:** Set `TASKPILOT_PORT` to match your TaskPilot API server port (see logs or Defaults Settings page).

### Requirements
- **jq** – for parsing test results (`brew install jq`)
- **curl** – for HTTP requests
- **mcp-local** – for local (stdio) integration (build from `/mcp-local`)

See also:
- [scripts/README.md](scripts/README.md)
- [mcp-local/README.md](mcp-local/README.md)

## License

Copyright © 2026 TaskPilot Team
