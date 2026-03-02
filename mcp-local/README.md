# TaskPilot MCP Local Server

A lightweight stdio-to-HTTP proxy that enables stdio-based MCP clients (GitHub Copilot, VS Code extensions) to communicate with TaskPilot's HTTP MCP server.

## Overview

This proxy acts as a bridge between:
- **Stdio-based MCP clients** (read/write JSON-RPC via stdin/stdout)
- **TaskPilot's HTTP MCP server** (POST /api/mcp)

It transparently forwards JSON-RPC 2.0 messages without interpreting the protocol, allowing any MCP client to use Task Pilot's 9 job management tools.

## Requirements

**TaskPilot must be running** with the API server enabled for mcp-local to work. The proxy connects to the HTTP endpoint and forwards requests.

**⚠️ CRITICAL: Port Configuration**

TaskPilot's default API port in the defaults database is often **NOT 8080**. You MUST check the actual port:

1. **Check startup logs**: Look for "Starting API server on port XXXXX"
2. **Settings/Defaults**: API Port configuration (common values: 8080, 41327)
3. **Test connection**: `curl http://localhost:PORT/api/mcp`

**If you get "Request timed out" errors**, the port is wrong! Use the `--url` flag with the correct port.

## Installation

Build the executable:

```bash
cd mcp-local
go build
```

This creates the `mcp-local` binary in the current directory.

## Usage

### Basic Usage

Start the proxy (connects to default `http://localhost:8080`):

```bash
./mcp-local
```

### Custom API URL

Using command-line flag:

```bash
./mcp-local --url=http://localhost:9090
```

Using environment variable:

```bash
export TASKPILOT_API_URL=http://localhost:9090
./mcp-local
```

**Configuration precedence:** `--url` flag > `TASKPILOT_API_URL` env > default (`http://localhost:8080`)

## MCP Client Configuration

### GitHub Copilot

**⚠️ IMPORTANT:** Most users need to specify the port! Run this first:

```bash
# Check TaskPilot's actual port from startup logs or Settings
# Common values: 8080, 41327

# Quick test to find the correct port:
curl http://localhost:41327/api/mcp -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
```

Add to your GitHub Copilot MCP settings (`~/.copilot/mcp-config.json` or VS Code settings):

**For port 41327 (most common):**

```json
{
  "mcpServers": {
    "taskpilot": {
      "type": "stdio",
      "command": "/absolute/path/to/TaskPilot/mcp-local/mcp-local",
      "args": ["--url", "http://localhost:41327"]
    }
  }
}
```

**For default port 8080 (rare):**

```json
{
  "mcpServers": {
    "taskpilot": {
      "type": "stdio",
      "command": "/absolute/path/to/TaskPilot/mcp-local/mcp-local",
      "args": []
    }
  }
}
```

**💡 Auto-setup:** Use the setup script:

```bash
./scripts/setup-copilot-mcp.sh
```

### VS Code MCP Extension

Add to your VS Code MCP extension configuration:

```json
{
  "mcp.servers": {
    "taskpilot": {
      "command": "/path/to/TaskPilot/mcp-local/mcp-local",
      "args": []
    }
  }
}
```

## Available Tools

The proxy provides access to all 9 TaskPilot MCP tools:

1. **jobs_create** - Create a new job (auto-adds `ai_` prefix)
2. **jobs_get** - Get job by ID
3. **jobs_list** - List all jobs
4. **jobs_update** - Update job properties (maintains `ai_` prefix)
5. **jobs_delete** - Delete a job
6. **jobs_execute** - Run a job immediately
7. **jobs_pause** - Disable job scheduling
8. **jobs_resume** - Enable job scheduling
9. **jobs_history** - Query execution history (with filters)

All tools automatically enforce the `ai_` prefix for job names created through MCP.

### Protocol Methods (Not Listed as Tools)

These are called directly as MCP protocol methods, not through `tools/call`:

- **initialize** - Establish MCP connection
- **tools/list** - List available tools
- **system/time** - Get server's current time (for debugging/time sync)

**Why system/time isn't in the tools list:**  
It's a protocol-level method like `initialize`, not a job management tool. You can call it directly:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"system/time","params":{}}' | ./mcp-local --url http://localhost:41327
# Returns: {"jsonrpc":"2.0","id":1,"result":{"timestamp":1772347181,"iso8601":"2026-02-28T22:39:41-08:00"}}
```

## Troubleshooting

### "Request timed out" Error (Most Common Issue)

**Symptom:** GitHub Copilot shows: `✗ jobs_list - MCP error -32001: Request timed out`

**Root Cause:** Port mismatch! Your config is using the wrong port.

**Solution:**

1. **Find TaskPilot's actual port** (check startup logs):
   ```
   Starting API server on port 41327 (MCP endpoint: http://localhost:41327/api/mcp)
   ```

2. **Test connection to correct port:**
   ```bash
   curl -X POST http://localhost:41327/api/mcp \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
   ```
   
   If this works, but Copilot times out → config has wrong port!

3. **Fix your GitHub Copilot config** to use correct port:
   ```json
   {
     "mcpServers": {
       "taskpilot": {
         "type": "stdio",
         "command": "/path/to/mcp-local",
         "args": ["--url", "http://localhost:41327"]
       }
     }
   }
   ```

4. **Restart VS Code** after config change

5. **Verify in VS Code:**
   - Command Palette → "MCP: Show Server Status"
   - Should show: `✓ taskpilot Connected`

### Copilot CLI Hangs (stderr Pollution)

**Symptom:** Command-line `copilot` tool hangs indefinitely:
```bash
copilot --model gpt-4.1 --allow-all-tools -p 'use taskpilot mcp tool to fetch all jobs'
# Hangs with no response
```

**Root Cause:** mcp-local logs to stderr (`[mcp-local] 2026/02/28...`), which mixes with stdout JSON-RPC messages. The copilot CLI waits for clean JSON but receives log prefixes first.

**Solution:** Use the wrapper script that redirects stderr to a log file:

1. **Use mcp-local-wrapper.sh** in your config:
   ```json
   {
     "mcpServers": {
       "taskpilot": {
         "command": "/Users/yourname/workplace/pp/ai/TaskPilot/mcp-local/mcp-local-wrapper.sh",
         "args": ["--url", "http://localhost:41327"]
       }
     }
   }
   ```

2. **Check logs** (if needed):
   ```bash
   tail -f ~/.taskpilot-mcp.log
   ```

3. **Test the wrapper:**
   ```bash
   echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | \
     ./mcp-local/mcp-local-wrapper.sh --url http://localhost:41327
   ```
   Should output clean JSON with no `[mcp-local]` log messages.

**Why this happens:** stdio transport requires strict separation:
- **stdout** = JSON-RPC protocol only
- **stderr** = logging/debugging

VS Code GitHub Copilot extension tolerates stderr output, but the CLI tool does not.

See [COPILOT_CLI_FIX.md](../COPILOT_CLI_FIX.md) for detailed explanation.

### "API server not available" Error

**Cause:** TaskPilot application is not running or API server is disabled.

**Solution:**
1. Start TaskPilot application
2. Check TaskPilot startup logs for "Starting API server on port XXXXX"
3. Verify the port matches what mcp-local is connecting to
4. If using custom port, pass `--url=http://localhost:<port>` to mcp-local

### Port Mismatch

**Cause:** TaskPilot is using a different API port than the default 8080.

**Solution:** 
- Check TaskPilot's startup logs: "Starting API server on port XXXXX"
- Check TaskPilot Settings → Defaults → API Port (e.g., 41327)
- Update mcp-local connection:
  - CLI: `./mcp-local --url=http://localhost:41327`
  - Env: `export TASKPILOT_API_URL=http://localhost:41327`
  - GitHub Copilot config: `"args": ["--url=http://localhost:41327"]`

### Connection Timeout

**Cause:** Network issues or TaskPilot is overloaded.

**Solution:**
- Check TaskPilot application is responsive
- Check network connectivity (firewall, localhost access)
- Restart TaskPilot if needed

### Logging

All diagnostic logs are written to **stderr** (not stdout, which is reserved for JSON-RPC protocol).

To see logs:
```bash
./mcp-local 2> mcp-local.log
```

Or in MCP client configurations, stderr is usually captured in client logs.

## Architecture

```
┌────────────────┐              ┌─────────────────────┐
│  MCP Client    │              │  TaskPilot (Wails)  │
│  (Copilot/VSC) │              │                     │
├────────────────┤   HTTP POST  ├─────────────────────┤
│  stdio         ├─────────────▶│  MCPServer          │
│  JSON-RPC      │◀─────────────┤  (9 tools)          │
│                │   JSON-RPC   │                     │
└────────────────┘              │  JobService         │
        ▲                       │  SQLite DB          │
        │                       └─────────────────────┘
        │
┌───────┴────────┐
│  mcp-local     │
│  (proxy)       │
│  this binary   │
└────────────────┘
```

**Key Points:**
- No database access (avoids lock contention)
- Multiple mcp-local instances can run simultaneously
- Single source of truth: TaskPilot's MCPServer
- ~140 lines of Go code

## License

Same as TaskPilot project.
