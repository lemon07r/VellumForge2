#!/bin/bash
set -e

echo "========================================="
echo "VellumForge2 Comprehensive Test Suite"
echo "========================================="
echo ""

# Test 1: Basic Generation
echo "[TEST 1] Basic generation (1 subtopic, 1 prompt)..."
timeout 120 ./bin/vellumforge2 run --config config.test.toml --verbose > /dev/null 2>&1 && echo "✓ PASSED" || echo "✗ FAILED"
echo ""

# Test 2: Judge functionality
echo "[TEST 2] Judge evaluation..."
timeout 300 ./bin/vellumforge2 run --config config.judge-test.toml --verbose > /dev/null 2>&1 && echo "✓ PASSED" || echo "✗ FAILED"
echo ""

# Test 3: Verify logs not empty
echo "[TEST 3] Session logs populated..."
LATEST=$(ls -t output/ | head -1)
if [ -s "output/$LATEST/session.log" ]; then
    echo "✓ PASSED - Log file has content"
else
    echo "✗ FAILED - Log file is empty"
fi
echo ""

# Test 4: Verify dataset format
echo "[TEST 4] Dataset schema validation..."
LATEST=$(ls -t output/ | head -1)
if jq -e '.prompt and .chosen and .rejected' "output/$LATEST/dataset.jsonl" > /dev/null 2>&1; then
    echo "✓ PASSED - Dataset has required fields"
else
    echo "✗ FAILED - Dataset missing fields"
fi
echo ""

# Test 5: HF Upload (dry run - check if it starts without errors)
echo "[TEST 5] HF Upload (checking upload starts)..."
timeout 180 ./bin/vellumforge2 run --config config.hftest.toml --upload-to-hf --verbose > /dev/null 2>&1 && echo "✓ PASSED" || echo "✗ FAILED"
echo ""

echo "========================================="
echo "Test Suite Complete!"
echo "========================================="
echo ""
echo "Check output/ directory for generated datasets"
echo "Check session logs in each output/YYYYMMDD_HHMMSS/ folder"
