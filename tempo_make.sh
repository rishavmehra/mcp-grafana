#!/bin/bash

# Build and test script for Tempo tools

echo "=== Building mcp-grafana with Tempo support ==="

# 1. First, ensure all dependencies are up to date
echo "Updating dependencies..."
go mod tidy

# 2. Build the binary
echo "Building mcp-grafana..."
go build -o dist/mcp-grafana ./cmd/mcp-grafana

# 3. Check if build was successful
if [ $? -eq 0 ]; then
    echo "✓ Build successful!"
else
    echo "✗ Build failed!"
    exit 1
fi

# 4. Start docker services if not running
echo "Starting Docker services..."
docker compose up -d

# 5. Wait for services to be ready
echo "Waiting for services to start..."
sleep 10

# 6. Test the binary with list of enabled tools
echo "Testing mcp-grafana binary..."
./dist/mcp-grafana --help | grep -A 20 "enabled-tools"

# 7. Run in debug mode to see which tools are being loaded
echo "Running in debug mode to check loaded tools..."
timeout 5s ./dist/mcp-grafana --debug --log-level debug 2>&1 | grep -E "(Enabling|Disabling) tools"

# 8. Run integration tests for Tempo
echo "Running Tempo integration tests..."
go test -v -tags integration ./tools -run TestTempoTools

# 9. Check if Tempo is in the datasources
echo "Checking Tempo datasource..."
curl -s http://localhost:3000/api/datasources | jq '.[] | select(.type == "tempo")'

echo "=== Done ==="