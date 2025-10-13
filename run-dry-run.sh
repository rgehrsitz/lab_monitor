#!/bin/bash
# Helper script to run lab monitor in dry-run mode
# This ensures environment variables are properly passed

set -e

# Check if we're in the right directory
if [ ! -f "config.yaml" ]; then
    echo "Error: Must run from lab_monitor directory"
    exit 1
fi

# Check for required environment variable in dry-run mode
if [ -z "$OPENAI_API_KEY" ]; then
    echo "Error: OPENAI_API_KEY environment variable is not set"
    echo "Please run: export OPENAI_API_KEY='your-key-here'"
    exit 1
fi

echo "=== Running Lab Monitor in DRY-RUN mode ==="
echo "OPENAI_API_KEY: ${OPENAI_API_KEY:0:20}... (set)"
echo ""

go run ./cmd/labmonitor -config config.yaml -dry-run
