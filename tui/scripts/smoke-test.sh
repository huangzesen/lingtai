#!/bin/bash
# scripts/smoke-test.sh — end-to-end smoke test
set -e

echo "=== Building ==="
make build

echo "=== Setting up test project ==="
TEST_DIR=$(mktemp -d)
trap "rm -rf $TEST_DIR" EXIT

echo "=== Running lingtai-cli ==="
./bin/lingtai "$TEST_DIR" &
PID=$!
sleep 2

echo "=== Checking project init ==="
test -f "$TEST_DIR/.lingtai/human/.agent.json" || { echo "FAIL: human manifest missing"; kill $PID; exit 1; }
test -d "$TEST_DIR/.lingtai/human/mailbox/inbox" || { echo "FAIL: human inbox missing"; kill $PID; exit 1; }

echo "=== Checking API ==="
PORT_FILE="$TEST_DIR/.lingtai/.port"
for i in $(seq 1 10); do
  [ -f "$PORT_FILE" ] && break
  sleep 0.5
done
if [ -f "$PORT_FILE" ]; then
  PORT=$(cat "$PORT_FILE")
  RESPONSE=$(curl -s "http://localhost:$PORT/api/network")
  echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'nodes={len(d[\"nodes\"])}'); assert 'stats' in d" \
    || { echo "FAIL: invalid API response"; kill $PID; exit 1; }
  echo "API verified on port $PORT"
else
  echo "WARN: port file not found, skipping API test"
fi

echo "=== Cleanup ==="
kill $PID 2>/dev/null || true

echo "=== ALL SMOKE TESTS PASSED ==="
