# TaskPilot REST API Documentation

## Overview

TaskPilot provides a REST API server that enables programmatic access to job management and execution history features. The API runs alongside the desktop application and can be accessed via HTTP.

## Configuration

### API Server Port

The API server port can be configured through the application's defaults settings:

1. Open TaskPilot desktop application
2. Go to Settings → Defaults
3. Set the "API Server Port" field (default: 8080)
4. Save settings
5. **Restart the application** for the port change to take effect

Valid port range: 1024-65535

### Base URL

By default, the API is accessible at:
```
http://localhost:8080/api
```

If you change the port, replace `8080` with your configured port number.

## API Documentation

Interactive Swagger documentation is available at:
```
http://localhost:8080/api/swagger
```

This provides a complete OpenAPI 3.0 specification with all endpoint details, request/response schemas, and examples.

## Endpoints

### Jobs

#### List All Jobs
```http
GET /api/jobs
```

Returns an array of all scheduled jobs.

**Response:** HTTP 200
```json
[
  {
    "id": "uuid",
    "name": "Job Name",
    "command": "echo 'hello'",
    "directory": "/path/to/dir",
    "schedule": "*/5 * * * *",
    "status": "idle",
    "schedule_type": "cron",
    "paused": false
  }
]
```

#### Get Job by ID
```http
GET /api/jobs/:id
```

Returns details of a specific job.

**Response:** HTTP 200 (job found) or HTTP 404 (not found)

#### Create Job
```http
POST /api/jobs
Content-Type: application/json
```

Creates a new scheduled job.

**Request Body:**
```json
{
  "name": "My Job",
  "command": "echo 'hello'",
  "directory": "/path/to/working/dir",
  "schedule": "*/5 * * * *",
  "schedule_type": "cron"
}
```

**Required fields:** `name`, `command`, `schedule`

**Response:** HTTP 201 (created) or HTTP 400 (validation error)

#### Update Job
```http
PUT /api/jobs/:id
Content-Type: application/json
```

Updates an existing job. All fields from the create request are supported.

**Response:** HTTP 200 (updated), HTTP 404 (not found), or HTTP 400 (validation error)

#### Delete Job
```http
DELETE /api/jobs/:id
```

Deletes a job and all its associated execution history records (cascade delete).

**Response:** HTTP 204 (deleted) or HTTP 404 (not found)

### History

#### List All History
```http
GET /api/history
GET /api/history?job_id={uuid}
GET /api/history?order=asc
GET /api/history?job_id={uuid}&order=asc
```

Returns all job execution history records with optional filtering and ordering.

**Query Parameters:**
- `job_id` (optional) - Filter history by specific job UUID
- `order` (optional) - Sort order: `asc` (oldest first) or `desc` (newest first, default)

**Response:** HTTP 200
```json
[
  {
    "id": "uuid",
    "job_id": "job-uuid",
    "output": "command output",
    "exit_code": 0,
    "timestamp": 1234567890,
    "duration_ms": 150
  }
]
```

**Error Response:** HTTP 400 (invalid job_id format)

**Examples:**
```bash
# Get all history (newest first)
curl http://localhost:8080/api/history

# Get history for specific job
curl http://localhost:8080/api/history?job_id=abc123-uuid-here

# Get all history in ascending order (oldest first)
curl http://localhost:8080/api/history?order=asc

# Get history for specific job in ascending order
curl http://localhost:8080/api/history?job_id=abc123-uuid-here&order=asc
```

#### Get History for Specific Job
```http
GET /api/jobs/:id/history
GET /api/jobs/:id/history?order=asc
```

Returns all execution history for a specific job with optional ordering.

**Path Parameters:**
- `id` (required) - Job UUID

**Query Parameters:**
- `order` (optional) - Sort order: `asc` (oldest first) or `desc` (newest first, default)

**Response:** HTTP 200 (job exists), HTTP 404 (job not found), HTTP 400 (invalid job ID format)

Returns an array of history records (empty array if job has no history):
```json
[
  {
    "id": "history-uuid",
    "job_id": "job-uuid",
    "output": "command output",
    "exit_code": 0,
    "timestamp": 1234567890,
    "duration_ms": 150
  }
]
```

**Examples:**
```bash
# Get history for specific job (newest first)
curl http://localhost:8080/api/jobs/abc123-uuid-here/history

# Get history for specific job in ascending order
curl http://localhost:8080/api/jobs/abc123-uuid-here/history?order=asc
```

**Python Example:**
```python
import requests

job_id = "abc123-uuid-here"
response = requests.get(f"http://localhost:8080/api/jobs/{job_id}/history?order=asc")

if response.status_code == 200:
    history = response.json()
    for record in history:
        print(f"Execution at {record['timestamp']}: exit code {record['exit_code']}")
elif response.status_code == 404:
    print("Job not found")
elif response.status_code == 400:
    print("Invalid job ID format")
```

#### Get History Record by ID
```http
GET /api/history/:id
```

Returns a specific history record.

**Response:** HTTP 200 (found) or HTTP 404 (not found)

#### Delete History Record
```http
DELETE /api/history/:id
```

Deletes a single history record. The associated job is not affected.

**Response:** HTTP 204 (deleted) or HTTP 404 (not found)

### System

#### Get System Time and Timezone
```http
GET /api/system/time
```

Returns the current server time in multiple formats along with timezone information.

**Response:** HTTP 200
```json
{
  "timestamp": 1735689600,
  "iso8601": "2025-01-01T00:00:00Z",
  "timezone": "America/Los_Angeles",
  "timezone_offset": "-08:00",
  "utc_offset_seconds": -28800
}
```

**Response Fields:**
- `timestamp` - Unix timestamp in seconds since epoch
- `iso8601` - ISO8601 formatted time string (RFC3339)
- `timezone` - IANA timezone name (e.g., "America/Los_Angeles", "UTC", "Europe/London")
- `timezone_offset` - Timezone offset from UTC as string (e.g., "-08:00", "+05:30")
- `utc_offset_seconds` - UTC offset in seconds (negative for west of UTC, positive for east)

**Use Cases:**
- Coordinate scheduled job execution across different timezones
- Display job execution times in user's local timezone
- Verify server timezone configuration
- Calculate time differences for scheduling

**Examples:**
```bash
# Get current server time
curl http://localhost:8080/api/system/time
```

**Python Example:**
```python
import requests
from datetime import datetime

response = requests.get("http://localhost:8080/api/system/time")
if response.status_code == 200:
    time_info = response.json()
    
    # Convert Unix timestamp to datetime
    server_time = datetime.fromtimestamp(time_info['timestamp'])
    print(f"Server time: {server_time}")
    print(f"Server timezone: {time_info['timezone']}")
    print(f"UTC offset: {time_info['timezone_offset']} ({time_info['utc_offset_seconds']} seconds)")
```

## Error Responses

All error responses follow this format:

```json
{
  "error": "Error message description"
}
```

Common HTTP status codes:
- **200 OK** - Successful GET/PUT request
- **201 Created** - Successful POST request
- **204 No Content** - Successful DELETE request
- **400 Bad Request** - Invalid request data or validation error
- **404 Not Found** - Resource not found
- **405 Method Not Allowed** - HTTP method not supported for endpoint
- **500 Internal Server Error** - Server-side error

## Schedule Types

Jobs support multiple schedule types via the `schedule_type` field:

- **`cron`** - Traditional cron expression (e.g., `*/5 * * * *`)
- **`delay`** - Delay in minutes (`delay_minutes` field required)
- **`datetime`** - One-time execution at specific time (`run_at` unix timestamp required)
- **`immediate`** - Execute immediately once

## Examples

### Using cURL

**Create a job:**
```bash
curl -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Backup Job",
    "command": "tar -czf /backup/data.tar.gz /data",
    "directory": "/",
    "schedule": "0 2 * * *",
    "schedule_type": "cron"
  }'
```

**List all jobs:**
```bash
curl http://localhost:8080/api/jobs
```

**Delete a job:**
```bash
curl -X DELETE http://localhost:8080/api/jobs/{job-id}
```

### Using Python

```python
import requests

base_url = "http://localhost:8080/api"

# Create a job
job_data = {
    "name": "Daily Sync",
    "command": "rsync -av /source /dest",
    "directory": "/",
    "schedule": "0 3 * * *",
    "schedule_type": "cron"
}
response = requests.post(f"{base_url}/jobs", json=job_data)
job = response.json()
print(f"Created job: {job['id']}")

# List all jobs
jobs = requests.get(f"{base_url}/jobs").json()
print(f"Total jobs: {len(jobs)}")

# Get execution history
history = requests.get(f"{base_url}/history").json()
for record in history:
    print(f"Job {record['job_id']}: exit code {record['exit_code']}")
```

## CORS Support

The API server includes CORS headers, allowing cross-origin requests from web applications:

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS
Access-Control-Allow-Headers: Content-Type
```

## Data Model

### Job Object

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique identifier (UUID) |
| `name` | string | Job name (required) |
| `command` | string | Shell command to execute (required) |
| `directory` | string | Working directory for execution |
| `schedule` | string | Schedule specification (required) |
| `sound_file` | string | Sound file to play on completion |
| `on_success_cmd` | string | Command to run after successful execution |
| `last_result` | string | Result of last execution |
| `status` | string | Current status (e.g., "idle", "running") |
| `schedule_type` | string | Type of schedule (cron/delay/datetime/immediate) |
| `paused` | boolean | Whether job is paused |
| `run_at` | integer | Unix timestamp for one-time execution |
| `delay_minutes` | integer | Delay in minutes for delay-based scheduling |
| `last_run_at` | integer | Unix timestamp of last execution |

### History Object

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique identifier (UUID) |
| `job_id` | string | Associated job ID |
| `output` | string | Command output (stdout/stderr) |
| `exit_code` | integer | Exit code of command |
| `timestamp` | integer | Unix timestamp of execution |
| `duration_ms` | integer | Execution duration in milliseconds |

## MCP (Model Context Protocol) Endpoint

TaskPilot provides an MCP server endpoint that enables AI agents to control jobs programmatically using a hybrid HTTP/SSE transport model.

### Communication Model

The MCP endpoint uses two communication channels:

1. **SSE Stream (Server → Client)**: Real-time events for job state changes, notifications, and heartbeats
   - Endpoint: `GET /api/mcp`
   - Events: job.created, job.updated, job.deleted, job.executing, job.completed, job.failed, heartbeat

2. **JSON-RPC Requests (Client → Server)**: Synchronous commands with immediate responses
   - Endpoint: `POST /api/mcp`
   - Methods: jobs/create, jobs/get, jobs/list, jobs/update, jobs/delete, jobs/execute, jobs/pause, jobs/resume, jobs/history

This design ensures:
- **Low latency** for command responses (synchronous POST)
- **Real-time updates** for job events (asynchronous SSE)
- **Reliable bidirectional** communication

### Connection Options

TaskPilot supports two ways to connect MCP clients:

#### 1. HTTP Direct (Recommended for Claude Desktop)

Connect directly to the HTTP endpoint for full SSE streaming support.

**Use when:**
- Your MCP client supports HTTP/SSE transport (like Claude Desktop)
- You need real-time job event notifications
- You want the lowest latency

**Configuration:** Point your MCP client to `http://localhost:8080/api/mcp`

#### 2. Stdio Proxy (Recommended for GitHub Copilot, VS Code)

Use the `mcp-local` stdio-to-HTTP proxy for MCP clients that only support stdio transport.

**Use when:**
- Your MCP client only supports stdio transport (like GitHub Copilot, VS Code MCP extensions)
- You need JSON-RPC request/response without streaming events
- You want a simple bridge to the TaskPilot HTTP endpoint

**How it works:**
- `mcp-local` is a lightweight Go binary that reads JSON-RPC from stdin, forwards to `POST /api/mcp`, and writes responses to stdout
- No protocol interpretation - pure transparent proxy
- Requires TaskPilot application running on localhost:8080

**Installation & Configuration:** See [mcp-local/README.md](mcp-local/README.md) for detailed setup instructions including GitHub Copilot integration.

**Quick example:**
```bash
# Run the proxy (connects to http://localhost:8080/api/mcp by default)
./mcp-local

# Or specify a custom API URL
./mcp-local -api-url http://custom-host:9000/api/mcp
```

### MCP Connection Endpoint

#### Establish SSE Connection
```http
GET /api/mcp
```

Establishes a Server-Sent Events connection for receiving real-time job events.

**Response Headers:**
- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`
- `Connection: keep-alive`

**Events Streamed:**

The server streams events in SSE format:
```
data: {"type":"connection.established","data":{"client_id":"...","status":"connected"}}

data: {"type":"job.created","data":{...job object...}}

data: {"type":"heartbeat","data":{"timestamp":1234567890}}
```

**Event Types:**
- `connection.established` - Initial connection acknowledgment with client ID
- `job.created` - A new job was created
- `job.updated` - A job was updated
- `job.deleted` - A job was deleted
- `job.executing` - A job started executing
- `job.completed` - A job finished executing (includes exit code and output)
- `job.failed` - A job execution failed
- `heartbeat` - Keep-alive heartbeat (sent every 30 seconds)

#### Send MCP Requests
```http
POST /api/mcp
```

Sends JSON-RPC 2.0 requests to control jobs. Responses are returned immediately in the HTTP response.

**Request Format:**
```json
{
  "jsonrpc": "2.0",
  "id": "request-1",
  "method": "jobs/create",
  "params": {
    "name": "my_job",
    "command": "echo hello"
  }
}
```

**Response Format:**
```json
{
  "jsonrpc": "2.0",
  "id": "request-1",
  "result": {
    "id": "...",
    "name": "ai_my_job",
    "command": "echo hello",
    ...
  }
}
```

**Error Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "request-1",
  "error": {
    "code": -32602,
    "message": "Invalid parameters: command is required"
  }
}
```

### MCP Methods

TaskPilot implements the Model Context Protocol (MCP) specification. All jobs created or modified through MCP automatically receive an `ai_` prefix on their names.

#### Standard MCP Protocol Methods

##### initialize

Establishes the MCP connection and exchanges capabilities.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {
      "name": "client-name",
      "version": "1.0.0"
    }
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "tools": {}
    },
    "serverInfo": {
      "name": "taskpilot",
      "version": "1.0.0"
    }
  }
}
```

##### tools/list

Lists all available tools.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "2",
  "method": "tools/list",
  "params": {}
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "2",
  "result": {
    "tools": [
      {
        "name": "jobs_create",
        "description": "Create a new scheduled job with automatic ai_ prefix enforcement",
        "inputSchema": { ... }
      },
      ...
    ]
  }
}
```

##### tools/call

Executes a tool by name. This is the standard MCP way to invoke tools.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "3",
  "method": "tools/call",
  "params": {
    "name": "jobs_list",
    "arguments": {
      "filter_ai_prefix": true
    }
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "3",
  "result": [ ...array of jobs... ]
}
```

##### system/time

Returns the server's current time. Useful for debugging, time synchronization checks, and verifying MCP connectivity.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "4",
  "method": "system/time",
  "params": {}
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "4",
  "result": {
    "timestamp": 1772347181,
    "iso8601": "2026-02-28T22:39:41-08:00"
  }
}
```

**Fields:**
- `timestamp` (integer): Unix timestamp (seconds since epoch)
- `iso8601` (string): ISO 8601 formatted datetime with timezone (RFC3339)

**Available Tools:**
- `jobs_create` - Create a new job
- `jobs_get` - Get a specific job by ID
- `jobs_list` - List all jobs
- `jobs_update` - Update a job
- `jobs_delete` - Delete a job
- `jobs_execute` - Execute a job immediately
- `jobs_pause` - Pause a job's schedule
- `jobs_resume` - Resume a paused job
- `jobs_history` - Get execution history

#### Direct Job Methods (Alternative API)

The following methods provide direct access to job operations without using `tools/call`. These are useful for custom integrations that don't use the standard MCP protocol.

##### jobs/create

Creates a new job with automatic `ai_` prefix enforcement.

**Parameters:**
```json
{
  "name": "job_name",           // Optional, auto-prefixed with ai_
  "command": "echo test",       // Required
  "directory": "/path/to/dir",  // Optional
  "schedule": "0 * * * *",      // Optional (cron expression)
  "schedule_type": "cron",      // Optional: cron/delay/datetime/immediate
  "sound_file": "/path/sound",  // Optional
  "on_success_cmd": "notify",   // Optional
  "delay_minutes": 30,          // Optional (for delay type)
  "run_at": 1234567890          // Optional (for datetime type)
}
```

**Example:**
```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "method": "jobs/create",
  "params": {
    "name": "backup",
    "command": "rsync -a /data /backup",
    "schedule": "0 2 * * *"
  }
}
```

**Result:** Job object with name prefixed as `ai_backup`

#### jobs/get

Retrieves a specific job by ID.

**Parameters:**
```json
{
  "id": "job-uuid"  // Required
}
```

**Example:**
```json
{
  "jsonrpc": "2.0",
  "id": "2",
  "method": "jobs/get",
  "params": {
    "id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

**Result:** Job object

#### jobs/list

Lists all jobs, optionally filtered by `ai_` prefix.

**Parameters:**
```json
{
  "filter_ai_prefix": true  // Optional, default: false
}
```

**Example (list all AI-created jobs):**
```json
{
  "jsonrpc": "2.0",
  "id": "3",
  "method": "jobs/list",
  "params": {
    "filter_ai_prefix": true
  }
}
```

**Result:** Array of job objects

#### jobs/update

Updates an existing job. Names are automatically prefixed with `ai_` if missing.

**Parameters:**
```json
{
  "id": "job-uuid",              // Required
  "name": "new_name",            // Optional, auto-prefixed
  "command": "new command",      // Optional
  "directory": "/new/path",      // Optional
  "schedule": "*/5 * * * *",     // Optional
  "schedule_type": "cron",       // Optional
  "paused": false,               // Optional
  "sound_file": "/path",         // Optional
  "on_success_cmd": "cmd",       // Optional
  "delay_minutes": 60,           // Optional
  "run_at": 1234567890           // Optional
}
```

**Example:**
```json
{
  "jsonrpc": "2.0",
  "id": "4",
  "method": "jobs/update",
  "params": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "command": "rsync -avz /data /backup",
    "paused": false
  }
}
```

**Result:** Updated job object

#### jobs/delete

Deletes a job and its execution history.

**Parameters:**
```json
{
  "id": "job-uuid"  // Required
}
```

**Example:**
```json
{
  "jsonrpc": "2.0",
  "id": "5",
  "method": "jobs/delete",
  "params": {
    "id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

**Result:**
```json
{
  "status": "deleted",
  "id": "550e8400-e29b-41d4-a716-446655440000"
}
```

#### jobs/execute

Executes a job immediately.

**Parameters:**
```json
{
  "id": "job-uuid"  // Required
}
```

**Example:**
```json
{
  "jsonrpc": "2.0",
  "id": "6",
  "method": "jobs/execute",
  "params": {
    "id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

**Result:**
```json
{
  "status": "executing",
  "id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Note:** Execution status updates (executing, completed, failed) are streamed via SSE events.

#### jobs/pause

Pauses a job's schedule.

**Parameters:**
```json
{
  "id": "job-uuid"  // Required
}
```

**Example:**
```json
{
  "jsonrpc": "2.0",
  "id": "7",
  "method": "jobs/pause",
  "params": {
    "id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

**Result:** Updated job object with `paused: true`

#### jobs/resume

Resumes a paused job.

**Parameters:**
```json
{
  "id": "job-uuid"  // Required
}
```

**Example:**
```json
{
  "jsonrpc": "2.0",
  "id": "8",
  "method": "jobs/resume",
  "params": {
    "id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

**Result:** Updated job object with `paused: false`

#### jobs/history

Retrieves job execution history with optional filtering.

**Parameters:**
```json
{
  "job_id": "job-uuid",         // Optional, filter by specific job
  "start_date": 1234567890,     // Optional, Unix timestamp
  "end_date": 1234570000,       // Optional, Unix timestamp
  "filter_ai_prefix": true,     // Optional, only AI-created jobs
  "order_asc": false            // Optional, default: false (DESC)
}
```

**Example (get last week's history for AI jobs):**
```json
{
  "jsonrpc": "2.0",
  "id": "9",
  "method": "jobs/history",
  "params": {
    "start_date": 1640000000,
    "end_date": 1640604800,
    "filter_ai_prefix": true
  }
}
```

**Result:** Array of history objects

### MCP Error Codes

Standard JSON-RPC 2.0 error codes:

| Code | Message | Description |
|------|---------|-------------|
| -32700 | Parse error | Invalid JSON in request |
| -32600 | Invalid request | Invalid JSON-RPC format |
| -32601 | Method not found | Unknown method |
| -32602 | Invalid params | Missing or invalid parameters |
| -32603 | Internal error | Server-side error |

### AI Prefix Enforcement

All jobs created or modified through the MCP endpoint automatically enforce the `ai_` prefix:

- **Empty name**: Generates name like `ai_a1b2c3d4`
- **Name without prefix**: `backup` becomes `ai_backup`
- **Name with prefix**: `ai_backup` stays `ai_backup`

This ensures AI-managed jobs are easily identifiable and can be filtered using the `filter_ai_prefix` parameter.

## Configuration for AI Tools

To use TaskPilot with AI assistants that support MCP HTTP transport (like Claude Desktop, VS Code extensions, or custom MCP clients), you need to configure the MCP server settings.

### MCP HTTP Server Configuration

TaskPilot provides an HTTP-based MCP server using Server-Sent Events (SSE) for real-time communication. The endpoint supports:
- **GET** `/api/mcp` - SSE connection for receiving server events
- **POST** `/api/mcp` - JSON-RPC 2.0 requests for controlling jobs

### Claude Desktop

1. Open your Claude Desktop configuration file:
   - macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
   - Windows: `%APPDATA%\Claude\claude_desktop_config.json`
2. Add the TaskPilot MCP server configuration:

```json
{
  "servers": {
    "taskpilot": {
      "type": "http",
      "uri": "http://localhost:8080/api/mcp",
      "version": "1.0.0"
    }
  }
}
```

**With Optional Authentication Headers:**
```json
{
  "servers": {
    "taskpilot": {
      "type": "http",
      "uri": "http://localhost:8080/api/mcp",
      "headers": {
        "Authorization": "Bearer YOUR_API_KEY"
      },
      "version": "1.0.0"
    }
  }
}
```

*Note: TaskPilot currently does not require authentication, but the headers field is available for future use.*

3. Restart Claude Desktop for the configuration to take effect.

### GitHub Copilot (VS Code)

GitHub Copilot's MCP support currently only accepts `stdio` or `local` type servers, not HTTP-based servers. To use TaskPilot with GitHub Copilot, you'll need a stdio bridge wrapper.

**Quick Setup (Recommended):**

Run the automated setup script:
```bash
cd /path/to/TaskPilot
./scripts/setup-copilot-mcp.sh
```

This script will:
- Automatically detect the correct paths
- Create the configuration file at `~/.copilot/mcp-config.json`
- Test the connection
- Back up any existing configuration

**Manual Setup:**

1. **Locate the bridge script** included with TaskPilot:
   ```
   scripts/copilot-mcp-bridge.js
   ```

2. **Configure GitHub Copilot** by creating or editing `~/.copilot/mcp-config.json`:

   ```json
   {
     "mcpServers": {
       "taskpilot": {
         "type": "stdio",
         "command": "node",
         "args": ["/ABSOLUTE/PATH/TO/TaskPilot/scripts/copilot-mcp-bridge.js"],
         "env": {
           "TASKPILOT_PORT": "8080"
         }
       }
     }
   }
   ```

   **Replace** `/ABSOLUTE/PATH/TO/TaskPilot` with the actual path to your TaskPilot installation.
   
   *Tip: An example configuration is available in `scripts/copilot-mcp-config.example.json`*

3. **Restart VS Code** for the configuration to take effect.

**Verify Setup:**
```bash
# Test the bridge manually
echo '{"jsonrpc":"2.0","id":"1","method":"jobs/list","params":{}}' | node scripts/copilot-mcp-bridge.js
```

You should see a JSON response with your jobs list.

In VS Code:
- Open GitHub Copilot Chat
- Try asking: "List all jobs in TaskPilot"
- The bridge script logs to stderr, which you can view in VS Code's Output panel (select "MCP" from the dropdown)

**Important Limitations:**
- GitHub Copilot's MCP integration is still evolving
- Real-time SSE events (job.created, job.completed, etc.) won't work through the stdio bridge
- Only synchronous JSON-RPC requests/responses are supported
- For full functionality with real-time events, consider using Claude Desktop or other tools with native HTTP MCP support

### VS Code (via Cline/Cursor/Roo Code)

If you are using MCP-compatible VS Code extensions (not GitHub Copilot):

**Cline/Roo Code with HTTP Support:**

1. Open the extension's settings (MCP Servers configuration).
2. Add a new MCP server with the following details:
   - **Name**: `taskpilot`
   - **Type**: `http` or `sse`
   - **URL**: `http://localhost:8080/api/mcp`

**Alternative: Manual Configuration File**

Some extensions may use a configuration file (e.g., `~/.config/cline/mcp-servers.json`):

```json
{
  "servers": {
    "taskpilot": {
      "url": "http://localhost:8080/api/mcp"
    }
  }
}
```

*Check your specific extension's documentation for the exact configuration format.*

### Custom Port Configuration

If you changed TaskPilot's API port from the default 8080:

1. Update the `uri` in your MCP configuration to match your configured port:
   ```json
   "uri": "http://localhost:YOUR_PORT/api/mcp"
   ```
2. Ensure TaskPilot is running with the API server enabled.

### Verifying MCP Configuration

To test if your MCP server is accessible:

**1. Test SSE Connection:**
```bash
curl -N http://localhost:8080/api/mcp
```

You should see a connection established event:
```
data: {"type":"connection.established","data":{"client_id":"...","status":"connected"}}
```

**2. Test JSON-RPC Request:**
```bash
curl -X POST http://localhost:8080/api/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":"1","method":"jobs/list","params":{}}'
```

You should receive a JSON-RPC response with a list of jobs.

**3. Check in AI Tool:**

After configuring your AI tool (Claude Desktop, VS Code extension, etc.):
- Restart the application
- Check the tool's MCP server status or logs
- Try asking the AI to "list all jobs" or "create a test job"
- The AI should be able to interact with TaskPilot

### Complete MCP Workflow Example

```javascript
// 1. Establish SSE connection
const eventSource = new EventSource('http://localhost:8080/api/mcp');

eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event:', data.type, data.data);
};

// 2. Create a job
fetch('http://localhost:8080/api/mcp', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    jsonrpc: '2.0',
    id: '1',
    method: 'jobs/create',
    params: {
      name: 'cleanup',
      command: 'rm -rf /tmp/*',
      schedule: '0 3 * * *'
    }
  })
});

// 3. Watch for job.created event via SSE
// 4. Execute job immediately
fetch('http://localhost:8080/api/mcp', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    jsonrpc: '2.0',
    id: '2',
    method: 'jobs/execute',
    params: { id: '<job-id>' }
  })
});

// 5. Watch for job.executing, job.completed events via SSE
```

## Notes

- **Cascade Delete**: Deleting a job automatically deletes all its execution history records
- **Atomic Operations**: Job deletion with history uses database transactions for consistency
- **No Authentication**: Currently, the API does not require authentication. Use appropriate firewall rules if exposing beyond localhost
- **Port Conflicts**: If the configured port is already in use, the API server will fail to start but the desktop application will continue to function normally

## Troubleshooting

### API server not responding

1. Check if the application is running
2. Verify the configured port in Settings → Defaults
3. Check application logs for port conflict errors
4. Try accessing the swagger documentation at `/api/swagger`
5. Ensure firewall allows connections to the configured port

### Port conflict error

If you see "address already in use" in logs:
1. Choose a different port in Settings → Defaults
2. Restart the application
3. Or stop the other application using that port

## Version

API Version: 1.0.0  
TaskPilot Application: Compatible with all versions supporting REST API feature
