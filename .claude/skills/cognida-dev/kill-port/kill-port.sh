#!/bin/bash
# Kill process on specified port (Linux/macOS)

PORT=$1

if [ -z "$PORT" ]; then
    echo "Usage: kill-port.sh <port>"
    exit 1
fi

# Find process
PID=$(lsof -ti :$PORT)

if [ -z "$PID" ]; then
    echo "No process found on port $PORT"
    exit 0
fi

echo "Killing process $PID on port $PORT..."
kill -9 $PID

if [ $? -eq 0 ]; then
    echo "Process killed successfully"
else
    echo "Failed to kill process"
    exit 1
fi
