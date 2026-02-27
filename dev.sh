#!/usr/bin/env bash
set -euo pipefail

PID_FILE=".dev.pid"

# Kill previous instance if running.
if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE")
    if kill -0 "$OLD_PID" 2>/dev/null; then
        echo "Stopping previous instance (PID $OLD_PID)..."
        kill "$OLD_PID"
        sleep 1
    fi
    rm -f "$PID_FILE"
fi

echo "Building..."
make build

echo "Starting alertstoopenclaw..."
./alertstoopenclaw &
echo $! > "$PID_FILE"
echo "Started with PID $(cat "$PID_FILE")"
