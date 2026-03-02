#!/bin/bash
# Coverage script for TaskPilot - generates coverage reports

set -e

echo "Generating test coverage report..."

# Run tests with coverage profile
go test -coverprofile=coverage.out ./...

# Calculate overall coverage percentage
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')

echo ""
echo "Overall coverage: $COVERAGE"

# Extract percentage value (remove %)
COVERAGE_NUM=$(echo $COVERAGE | sed 's/%//')

# Check if coverage meets threshold (70%)
THRESHOLD=70.0

if (( $(echo "$COVERAGE_NUM < $THRESHOLD" | bc -l) )); then
  echo "❌ Coverage $COVERAGE is below threshold of ${THRESHOLD}%"
  exit 1
else
  echo "✅ Coverage $COVERAGE meets threshold of ${THRESHOLD}%"
fi

echo ""
echo "To view detailed HTML report, run:"
echo "  go tool cover -html=coverage.out"
