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

## MCP Testing

### Model Context Protocol (MCP)

TaskPilot implements the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) to allow AI agents (like GitHub Copilot) to manage jobs.

#### AI Safety & Constraints
- **Naming Enforcement**: All jobs created via MCP are automatically prefixed with `ai_` to distinguish them from user-created jobs.
- **Sound Disabled**: MCP-created jobs have the `sound_file` property cleared to ensure silent background execution.

#### Connecting via Copilot
To use TaskPilot with GitHub Copilot CLI or IDE, use the provided `mcp-local` proxy:

```bash
# In your Copilot configuration
# command: /path/to/taskpilot/mcp-local/mcp-local
# env: TASKPILOT_PORT=41327
```

For more information on the MCP implementation, see `services/mcp_server.go`.

### Test Scripts

- **`test-mcp-http.sh`** - Tests all MCP methods via direct HTTP POST
- **`test-mcp-local.sh`** - Tests all MCP methods via the mcp-local stdio proxy

### Running MCP Tests

```bash
# HTTP tests (direct API calls)
TASKPILOT_PORT=41327 ./scripts/test-mcp-http.sh

# mcp-local tests (stdio proxy)
TASKPILOT_PORT=41327 ./scripts/test-mcp-local.sh
```

**Note:** Set `TASKPILOT_PORT` to match your TaskPilot API server port (check startup logs or defaults settings).

### Requirements

- **jq** - JSON processor (`brew install jq` or `apt-get install jq`)
- **curl** - HTTP client (included on most systems)
- **mcp-local** - For stdio proxy tests (build from `mcp-local/`)

For detailed information, see [scripts/README.md](scripts/README.md).

## License

Copyright © 2026 TaskPilot Team
