# TaskPilot Scripts

This directory contains testing and utility scripts for TaskPilot development.

## GitHub Copilot MCP Setup

**⚠️ IMPORTANT: Common issues:**
- **"Request timed out" in VS Code** → Your config is missing the correct port!
- **Command-line `copilot` hangs** → Use the wrapper to suppress stderr logging

### Automated Setup (Recommended)

Run the setup script to auto-detect your configuration:

```bash
./scripts/setup-copilot-mcp.sh
```

This will:
1. Detect TaskPilot's API port (e.g., 41327, not default 8080)
2. Generate the correct config with absolute paths  
3. Guide you through adding it to GitHub Copilot
4. Verify everything is working

### Manual Configuration

If you prefer manual setup, see `scripts/copilot-mcp-config.example.json` and [mcp-local/README.md](../mcp-local/README.md) for detailed instructions.

**Key points:**
- **VS Code extension:** Use `mcp-local` with `--url` flag specifying the correct port
- **Command-line copilot:** Use `mcp-local-wrapper.sh` to prevent stderr pollution (see [COPILOT_CLI_FIX.md](../COPILOT_CLI_FIX.md))

## Testing Scripts

### Unit Tests

- **test.sh** - Run all unit tests (Unix/macOS)
  ```bash
  ./scripts/test.sh                 # Run all tests
  ./scripts/test.sh -v              # Verbose output
  ./scripts/test.sh -run TestName   # Run specific test
  ./scripts/test.sh --coverage      # With coverage
  ```

- **test.bat** - Run all unit tests (Windows)
  ```cmd
  scripts\test.bat                  # Run all tests
  scripts\test.bat -v               # Verbose output
  scripts\test.bat --coverage       # With coverage
  ```

- **coverage.sh** - Generate detailed coverage reports
  ```bash
  ./scripts/coverage.sh             # Generate and check coverage
  ```

### MCP Integration Tests

- **test-mcp-http.sh** - Test MCP methods via direct HTTP POST to `/api/mcp`
  ```bash
  ./scripts/test-mcp-http.sh                      # Test on default port 8080
  TASKPILOT_PORT=41327 ./scripts/test-mcp-http.sh # Test on custom port
  ```

- **test-mcp-local.sh** - Test MCP methods via mcp-local stdio proxy
  ```bash
  ./scripts/test-mcp-local.sh                      # Test on default port 8080
  TASKPILOT_PORT=41327 ./scripts/test-mcp-local.sh # Test on custom port
  ```

## MCP Bridge Configuration

For GitHub Copilot or VS Code MCP integration, see:
- [mcp-local/README.md](../mcp-local/README.md) - Go-based stdio-to-HTTP proxy for MCP clients

## Requirements

- **Go** - For running unit tests
- **jq** - For MCP integration tests (install via `brew install jq`)
- **curl** - For HTTP-based tests
- **mcp-local binary** - For test-mcp-local.sh (build from `mcp-local/` directory)
