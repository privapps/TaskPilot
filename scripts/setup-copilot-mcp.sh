#!/bin/bash
################################################################################
# setup-copilot-mcp.sh - Configure GitHub Copilot for TaskPilot MCP
#
# This script helps you configure GitHub Copilot to use TaskPilot's MCP server
# via the mcp-local stdio proxy. It auto-detects the correct port from TaskPilot.
#
# Usage: ./scripts/setup-copilot-mcp.sh
################################################################################

set -euo pipefail

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "=========================================="
echo "TaskPilot MCP Setup for GitHub Copilot"
echo "=========================================="
echo ""

# Get the absolute path to TaskPilot directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TASKPILOT_DIR="$(dirname "$SCRIPT_DIR")"
MCP_LOCAL_PATH="$TASKPILOT_DIR/mcp-local/mcp-local"

echo -e "${BLUE}TaskPilot directory:${NC} $TASKPILOT_DIR"
echo ""

# Check if mcp-local binary exists
if [[ ! -f "$MCP_LOCAL_PATH" ]]; then
    echo -e "${RED}Error: mcp-local binary not found at $MCP_LOCAL_PATH${NC}"
    echo ""
    echo "Build it first:"
    echo "  cd $TASKPILOT_DIR/mcp-local"
    echo "  go build -o mcp-local"
    exit 1
fi

echo -e "${GREEN}✓${NC} mcp-local binary found"

# Detect TaskPilot's API port
echo ""
echo -e "${BLUE}Detecting TaskPilot API port...${NC}"
echo ""

DETECTED_PORT=""
for port in 41327 8080 3000 9090; do
    echo -n "  Trying port $port... "
    if curl -s -m 2 -X POST "http://localhost:$port/api/mcp" \
        -H 'Content-Type: application/json' \
        -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
        | grep -q '"result"'; then
        echo -e "${GREEN}✓ Found!${NC}"
        DETECTED_PORT=$port
        break
    else
        echo "✗"
    fi
done

if [[ -z "$DETECTED_PORT" ]]; then
    echo ""
    echo -e "${YELLOW}Warning: Could not auto-detect TaskPilot's API port.${NC}"
    echo ""
    echo "Make sure TaskPilot is running, then check startup logs for:"
    echo "  \"Starting API server on port XXXXX\""
    echo ""
    read -p "Enter API port manually (default: 8080): " MANUAL_PORT
    DETECTED_PORT="${MANUAL_PORT:-8080}"
fi

echo ""
echo -e "${GREEN}✓${NC} Using API port: ${BLUE}$DETECTED_PORT${NC}"

# Generate config
CONFIG_JSON=$(cat <<EOF
{
  "mcpServers": {
    "taskpilot": {
      "type": "stdio",
      "command": "$MCP_LOCAL_PATH",
      "args": ["--url", "http://localhost:$DETECTED_PORT"]
    }
  }
}
EOF
)

echo ""
echo "=========================================="
echo "GitHub Copilot Configuration"
echo "=========================================="
echo ""
echo "Add this to your GitHub Copilot MCP config:"
echo ""
echo -e "${BLUE}Config Location:${NC}"
echo "  • VS Code: Settings → Extensions → GitHub Copilot → MCP Servers"
echo "  • File: ~/.copilot/mcp-config.json (if exists)"
echo ""
echo -e "${BLUE}Config JSON:${NC}"
echo "$CONFIG_JSON"
echo ""

# Offer to save config
read -p "Save this config to copilot-mcp-config.json in current directory? (y/n) " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "$CONFIG_JSON" > "$TASKPILOT_DIR/copilot-mcp-config.json"
    echo -e "${GREEN}✓${NC} Saved to: $TASKPILOT_DIR/copilot-mcp-config.json"
    echo ""
    echo "Copy this file's contents to your GitHub Copilot settings."
fi

echo ""
echo "=========================================="
echo "Next Steps"
echo "=========================================="
echo ""
echo "1. ${BLUE}Add the config${NC} to GitHub Copilot settings in VS Code"
echo "2. ${BLUE}Restart VS Code${NC}"
echo "3. ${BLUE}Test in Copilot Chat:${NC}"
echo "   @taskpilot list all jobs"
echo ""
echo "4. ${BLUE}Verify MCP server status:${NC}"
echo "   Open Command Palette → 'MCP: Show Server Status'"
echo "   Should show: ✓ taskpilot Connected"
echo ""
echo "=========================================="
echo "Troubleshooting"
echo "=========================================="
echo ""
echo "If you get '${RED}Request timed out${NC}' errors:"
echo ""
echo "1. Check TaskPilot is running:"
echo "   ps aux | grep taskpilot"
echo ""
echo "2. Verify API port in TaskPilot startup logs:"
echo "   Look for: 'Starting API server on port XXXXX'"
echo ""
echo "3. Test connection manually:"
echo "   curl -X POST http://localhost:$DETECTED_PORT/api/mcp \\"
echo "     -H 'Content-Type: application/json' \\"
echo "     -d '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{}}'"
echo ""
echo "4. Update config with correct port if needed"
echo ""
echo -e "${GREEN}Setup complete!${NC}"
