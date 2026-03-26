#!/bin/bash
# Quick test script: kill old agents, clean test dir, rebuild, launch
set -e
TEST_DIR="${1:-/tmp/tui-v2-test}"

# Kill any running test agents
pkill -f "lingtai run $TEST_DIR" 2>/dev/null || true

# Clean
rm -rf "$TEST_DIR"

# Rebuild
cd "$(dirname "$0")/.."
go build -o bin/lingtai .

echo "Launching: bin/lingtai $TEST_DIR"
exec ./bin/lingtai "$TEST_DIR"
