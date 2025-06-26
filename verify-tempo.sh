#!/bin/bash

echo "=== Verifying Tempo Tools Integration ==="

# Step 1: Clean and build
echo "1. Building mcp-grafana..."
rm -f ./dist/mcp-grafana
go build -o dist/mcp-grafana ./cmd/mcp-grafana

if [ $? -ne 0 ]; then
    echo "✗ Build failed!"
    exit 1
fi
echo "✓ Build successful!"

# Step 2: Verify tempo is in enabled tools
echo -e "\n2. Checking default enabled tools:"
./dist/mcp-grafana --help | grep -A2 "enabled-tools" | grep -o "tempo" && echo "✓ Tempo is in default enabled tools" || echo "✗ Tempo is NOT in default enabled tools"

# Step 3: Verify disable-tempo flag exists
echo -e "\n3. Checking for --disable-tempo flag:"
./dist/mcp-grafana --help | grep "disable-tempo" && echo "✓ --disable-tempo flag exists" || echo "✗ --disable-tempo flag NOT found"

# Step 4: List all Tempo tools
echo -e "\n4. Tempo tools available:"
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | \
    ./dist/mcp-grafana 2>/dev/null | \
    jq -r '.result.tools[] | select(.name | contains("tempo")) | .name' | \
    sort | \
    while read tool; do echo "   ✓ $tool"; done

# Step 5: Count tools
echo -e "\n5. Tool statistics:"
TOTAL=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | ./dist/mcp-grafana 2>/dev/null | jq '.result.tools | length')
TEMPO=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | ./dist/mcp-grafana 2>/dev/null | jq '.result.tools[] | select(.name | contains("tempo"))' | wc -l)

echo "   Total tools: $TOTAL"
echo "   Tempo tools: $TEMPO"

# Step 6: Test disabling tempo
echo -e "\n6. Testing --disable-tempo flag:"
TEMPO_DISABLED=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | ./dist/mcp-grafana --disable-tempo 2>/dev/null | jq '.result.tools[] | select(.name | contains("tempo"))' | wc -l)
echo "   Tempo tools with --disable-tempo: $TEMPO_DISABLED"

if [ "$TEMPO" -gt 0 ] && [ "$TEMPO_DISABLED" -eq 0 ]; then
    echo "   ✓ --disable-tempo flag works correctly"
else
    echo "   ✗ --disable-tempo flag not working"
fi

echo -e "\n=== Summary ==="
if [ "$TEMPO" -eq 4 ]; then
    echo "✓ All 4 Tempo tools are available:"
    echo "  • search_tempo_traces"
    echo "  • get_tempo_trace"
    echo "  • list_tempo_tag_names"
    echo "  • list_tempo_tag_values"
    echo -e "\n✓ Tempo integration is working correctly!"
else
    echo "✗ Expected 4 Tempo tools, but found $TEMPO"
    echo "✗ Tempo integration needs to be checked"
fi