#!/bin/bash
# Test script for TaskPilot - runs all unit tests

set -e  # Exit on error

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo "Running TaskPilot unit tests..."

# Parse command line options
VERBOSE=""
RUN_PATTERN=""
COVER_FLAG=""

while [[ $# -gt 0 ]]; do
  case $1 in
    -v|--verbose)
      VERBOSE="-v"
      shift
      ;;
    -run)
      RUN_PATTERN="-run $2"
      shift 2
      ;;
    -cover|--coverage)
      COVER_FLAG="-cover"
      shift
      ;;
    *)
      echo "Unknown option: $1"
      echo "Usage: $0 [-v|--verbose] [-run pattern] [-cover|--coverage]"
      exit 1
      ;;
  esac
done

# Run tests
if go test $VERBOSE $RUN_PATTERN $COVER_FLAG ./...; then
  echo -e "${GREEN}✓ All tests passed!${NC}"
  exit 0
else
  echo -e "${RED}✗ Tests failed${NC}"
  exit 1
fi
