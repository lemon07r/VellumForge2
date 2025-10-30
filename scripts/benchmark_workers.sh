#!/bin/bash
# Worker Count Benchmark Script
# Tests different worker counts to find optimal configuration

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CONFIG_FILE="$PROJECT_DIR/config.toml"
BINARY="$PROJECT_DIR/bin/vellumforge2"
RESULTS_DIR="$PROJECT_DIR/output/benchmark_$(date +%Y%m%d_%H%M%S)"

# Worker counts to test
WORKER_COUNTS=(8 12 16 20 24)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}============================================${NC}"
echo -e "${BLUE}  VellumForge2 Worker Count Benchmark${NC}"
echo -e "${BLUE}============================================${NC}"
echo ""
echo "Testing worker counts: ${WORKER_COUNTS[@]}"
echo "Results directory: $RESULTS_DIR"
echo ""

# Create results directory
mkdir -p "$RESULTS_DIR"

# Backup original config
cp "$CONFIG_FILE" "$CONFIG_FILE.backup"
echo -e "${GREEN}✓${NC} Backed up config to config.toml.backup"

# Check if binary exists
if [ ! -f "$BINARY" ]; then
    echo -e "${YELLOW}Building binary...${NC}"
    cd "$PROJECT_DIR"
    go build -o bin/vellumforge2 ./cmd/vellumforge2
    echo -e "${GREEN}✓${NC} Binary built"
fi

# Function to update worker count in config
update_worker_count() {
    local count=$1
    # Use sed to update concurrency value
    sed -i "s/^concurrency = [0-9]\+/concurrency = $count/" "$CONFIG_FILE"
    echo -e "${GREEN}✓${NC} Updated config: concurrency = $count"
}

# Function to extract metrics from session log
extract_metrics() {
    local session_dir=$1
    local log_file="$session_dir/session.log"
    
    if [ ! -f "$log_file" ]; then
        echo "ERROR: Log file not found: $log_file"
        return 1
    fi
    
    # Extract metrics using Python
    python3 << EOF
import json
import sys

log_file = "$log_file"
metrics = {
    'duration': 0,
    'jobs': 0,
    'throughput': 0,
    'avg_chosen': 0,
    'avg_rejected': 0,
    'avg_judge': 0,
    'avg_total': 0,
    'rate_limit_wait': 0,
    'blocking_rate': 0
}

chosen_times = []
rejected_times = []
judge_times = []
total_times = []
rate_limit_waits = []

try:
    with open(log_file, 'r') as f:
        for line in f:
            try:
                data = json.loads(line)
                
                if 'Job processing breakdown' in data.get('msg', ''):
                    chosen_times.append(data.get('chosen_ms', 0))
                    rejected_times.append(data.get('rejected_ms', 0))
                    judge_times.append(data.get('judge_ms', 0))
                    total_times.append(data.get('total_ms', 0))
                
                elif 'rate_limit_wait_ms' in data:
                    rate_limit_waits.append(data.get('rate_limit_wait_ms', 0))
                
                elif 'Generation pipeline completed' in data.get('msg', ''):
                    metrics['duration'] = data.get('duration', 0) / 1e9
                    metrics['jobs'] = data.get('total_prompts', 0)
            except:
                pass
    
    if chosen_times:
        metrics['avg_chosen'] = sum(chosen_times) / len(chosen_times) / 1000
        metrics['avg_rejected'] = sum(rejected_times) / len(rejected_times) / 1000
        metrics['avg_judge'] = sum(judge_times) / len(judge_times) / 1000
        metrics['avg_total'] = sum(total_times) / len(total_times) / 1000
        metrics['throughput'] = len(chosen_times) / (metrics['duration'] / 60) if metrics['duration'] > 0 else 0
    
    if rate_limit_waits:
        metrics['rate_limit_wait'] = sum(rate_limit_waits) / len(rate_limit_waits)
        metrics['blocking_rate'] = sum(1 for w in rate_limit_waits if w > 0) / len(rate_limit_waits) * 100
    
    # Output as JSON
    print(json.dumps(metrics))
    
except Exception as e:
    print(f"Error: {e}", file=sys.stderr)
    sys.exit(1)
EOF
}

# Array to store all results
declare -a RESULTS

# Run benchmarks
echo ""
echo -e "${BLUE}Starting benchmark runs...${NC}"
echo ""

for workers in "${WORKER_COUNTS[@]}"; do
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}Testing with $workers workers${NC}"
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    # Update config
    update_worker_count "$workers"
    
    # Run generation
    echo "Running generation..."
    RUN_OUTPUT=$("$BINARY" run --config "$CONFIG_FILE" 2>&1)
    RUN_STATUS=$?
    
    if [ $RUN_STATUS -ne 0 ]; then
        echo -e "${RED}✗${NC} Run failed with exit code $RUN_STATUS"
        continue
    fi
    
    # Extract session directory from output
    SESSION_DIR=$(echo "$RUN_OUTPUT" | grep -oP 'session_dir=\K[^ ]+' | tail -1)
    
    if [ -z "$SESSION_DIR" ]; then
        echo -e "${RED}✗${NC} Could not find session directory"
        continue
    fi
    
    echo -e "${GREEN}✓${NC} Run completed: $SESSION_DIR"
    
    # Extract metrics
    echo "Extracting metrics..."
    METRICS=$(extract_metrics "$SESSION_DIR")
    
    if [ $? -ne 0 ]; then
        echo -e "${RED}✗${NC} Failed to extract metrics"
        continue
    fi
    
    # Parse metrics
    DURATION=$(echo "$METRICS" | jq -r '.duration')
    THROUGHPUT=$(echo "$METRICS" | jq -r '.throughput')
    AVG_TOTAL=$(echo "$METRICS" | jq -r '.avg_total')
    RATE_LIMIT_WAIT=$(echo "$METRICS" | jq -r '.rate_limit_wait')
    BLOCKING_RATE=$(echo "$METRICS" | jq -r '.blocking_rate')
    
    # Display results
    echo ""
    echo "Results:"
    echo "  Duration:          ${DURATION}s ($(echo "scale=2; $DURATION/60" | bc) min)"
    echo "  Throughput:        ${THROUGHPUT} jobs/min"
    echo "  Avg Time per Job:  ${AVG_TOTAL}s"
    echo "  Rate Limit Wait:   ${RATE_LIMIT_WAIT}ms"
    echo "  Blocking Rate:     ${BLOCKING_RATE}%"
    echo ""
    
    # Store results
    RESULTS+=("$workers|$DURATION|$THROUGHPUT|$AVG_TOTAL|$RATE_LIMIT_WAIT|$BLOCKING_RATE|$SESSION_DIR")
    
    # Copy session to benchmark results
    cp -r "$SESSION_DIR" "$RESULTS_DIR/workers_${workers}"
    
    # Small delay between runs
    sleep 2
done

# Restore original config
mv "$CONFIG_FILE.backup" "$CONFIG_FILE"
echo ""
echo -e "${GREEN}✓${NC} Restored original config"

# Generate comparison report
echo ""
echo -e "${BLUE}============================================${NC}"
echo -e "${BLUE}  Benchmark Results Summary${NC}"
echo -e "${BLUE}============================================${NC}"
echo ""

printf "%-10s %-15s %-18s %-18s %-18s %-15s\n" \
    "Workers" "Duration (min)" "Throughput (/min)" "Avg Job Time (s)" "Rate Wait (ms)" "Blocking %"
echo "─────────────────────────────────────────────────────────────────────────────────────────────"

BEST_THROUGHPUT=0
BEST_WORKERS=0

for result in "${RESULTS[@]}"; do
    IFS='|' read -r workers duration throughput avg_total rate_wait blocking session <<< "$result"
    
    duration_min=$(python3 -c "print(f'{$duration/60:.2f}')")
    
    printf "%-10s %-15s %-18s %-18s %-18s %-15s\n" \
        "$workers" "$duration_min" "$throughput" "$avg_total" "$rate_wait" "$blocking"
    
    # Track best throughput (using Python for float comparison)
    IS_BETTER=$(python3 -c "print(1 if $throughput > $BEST_THROUGHPUT else 0)")
    if [ "$IS_BETTER" = "1" ]; then
        BEST_THROUGHPUT=$throughput
        BEST_WORKERS=$workers
    fi
done

echo ""
echo -e "${GREEN}✓ OPTIMAL CONFIGURATION: $BEST_WORKERS workers${NC}"
echo -e "  Best throughput: $BEST_THROUGHPUT jobs/min"
echo ""
echo "Results saved to: $RESULTS_DIR"

# Generate detailed analysis report
REPORT_FILE="$RESULTS_DIR/benchmark_report.md"
cat > "$REPORT_FILE" << EOFMD
# Worker Count Benchmark Report
**Date:** $(date +"%Y-%m-%d %H:%M:%S")  
**Configuration:** $(basename "$CONFIG_FILE")

## Test Parameters
- Worker counts tested: ${WORKER_COUNTS[@]}
- Test dataset: $(grep -A1 '\[generation\]' "$CONFIG_FILE" | grep num_subtopics | awk '{print $3}') subtopics × $(grep -A2 '\[generation\]' "$CONFIG_FILE" | grep num_prompts_per_subtopic | awk '{print $3}') prompts

## Results Summary

| Workers | Duration | Throughput | Avg Job Time | Rate Wait | Blocking % |
|---------|----------|------------|--------------|-----------|------------|
EOFMD

for result in "${RESULTS[@]}"; do
    IFS='|' read -r workers duration throughput avg_total rate_wait blocking session <<< "$result"
    duration_min=$(python3 -c "print(f'{$duration/60:.2f}')")
    
    echo "| $workers | ${duration_min} min | $throughput jobs/min | ${avg_total}s | ${rate_wait}ms | ${blocking}% |" >> "$REPORT_FILE"
done

cat >> "$REPORT_FILE" << EOFMD

## Recommendation

**Optimal worker count: $BEST_WORKERS workers**
- Best throughput: $BEST_THROUGHPUT jobs/min

## Session Directories

EOFMD

for result in "${RESULTS[@]}"; do
    IFS='|' read -r workers duration throughput avg_total rate_wait blocking session <<< "$result"
    echo "- **${workers} workers:** \`$(basename "$session")\`" >> "$REPORT_FILE"
done

echo ""
echo -e "${GREEN}✓${NC} Detailed report saved to: $REPORT_FILE"
echo ""
