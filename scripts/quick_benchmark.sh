#!/bin/bash
# Quick Worker Count Benchmark - Tests fewer configurations for faster results

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CONFIG_FILE="$PROJECT_DIR/config.toml"
BINARY="$PROJECT_DIR/bin/vellumforge2"
RESULTS_DIR="$PROJECT_DIR/output/quick_bench_$(date +%Y%m%d_%H%M%S)"

# Quick test: fewer worker counts
WORKER_COUNTS=(8 16 24)

# Parse command line arguments
if [ $# -gt 0 ]; then
    WORKER_COUNTS=($@)
    echo "Testing custom worker counts: ${WORKER_COUNTS[@]}"
fi

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Quick Worker Benchmark${NC}"
echo "Testing: ${WORKER_COUNTS[@]}"
echo ""

mkdir -p "$RESULTS_DIR"
cp "$CONFIG_FILE" "$CONFIG_FILE.benchmark_backup"

# Check if binary exists and is up to date
if [ ! -f "$BINARY" ] || [ "$PROJECT_DIR/cmd/vellumforge2/main.go" -nt "$BINARY" ]; then
    echo "Building binary..."
    cd "$PROJECT_DIR"
    go build -o bin/vellumforge2 ./cmd/vellumforge2
fi

update_worker_count() {
    sed -i "s/^concurrency = [0-9][0-9]*/concurrency = $1/" "$CONFIG_FILE"
}

# Results CSV
RESULTS_CSV="$RESULTS_DIR/results.csv"
echo "workers,duration_sec,throughput,avg_job_time,rate_wait_ms,blocking_pct" > "$RESULTS_CSV"

echo "Workers,Duration,Throughput,Avg Job,Rate Wait,Blocking" > "$RESULTS_DIR/summary.txt"
echo "──────────────────────────────────────────────────────────────────" >> "$RESULTS_DIR/summary.txt"

for workers in "${WORKER_COUNTS[@]}"; do
    echo ""
    echo -e "${YELLOW}Testing $workers workers...${NC}"
    
    update_worker_count "$workers"
    
    # Run and capture output
    "$BINARY" run --config "$CONFIG_FILE" > "$RESULTS_DIR/run_${workers}w.log" 2>&1
    
    # Find session directory
    SESSION_DIR=$(grep -oP 'session_dir=output/\K[^ ]+' "$RESULTS_DIR/run_${workers}w.log" | tail -1)
    SESSION_DIR="$PROJECT_DIR/output/$SESSION_DIR"
    
    if [ ! -d "$SESSION_DIR" ]; then
        echo "Error: Session directory not found"
        continue
    fi
    
    # Quick metric extraction
    METRICS=$(python3 -c "
import json
import sys

chosen, rejected, judge, total = [], [], [], []
rate_waits = []
duration = 0

with open('$SESSION_DIR/session.log') as f:
    for line in f:
        try:
            d = json.loads(line)
            if 'Job processing breakdown' in d.get('msg', ''):
                chosen.append(d.get('chosen_ms', 0))
                rejected.append(d.get('rejected_ms', 0))
                judge.append(d.get('judge_ms', 0))
                total.append(d.get('total_ms', 0))
            elif 'rate_limit_wait_ms' in d:
                rate_waits.append(d.get('rate_limit_wait_ms', 0))
            elif 'Generation pipeline completed' in d.get('msg', ''):
                duration = d.get('duration', 0) / 1e9
        except:
            pass

if total:
    throughput = len(total) / (duration / 60)
    avg_total = sum(total) / len(total) / 1000
    rate_wait = sum(rate_waits) / len(rate_waits) if rate_waits else 0
    blocking = sum(1 for w in rate_waits if w > 0) / len(rate_waits) * 100 if rate_waits else 0
    
    print(f'{duration},{throughput:.2f},{avg_total:.1f},{rate_wait:.0f},{blocking:.1f}')
else:
    sys.exit(1)
")
    
    if [ $? -eq 0 ]; then
        IFS=',' read -r duration throughput avg_total rate_wait blocking <<< "$METRICS"
        duration_min=$(python3 -c "print(f'{$duration / 60:.2f}')")
        
        echo "  Duration: ${duration_min} min | Throughput: ${throughput} jobs/min | Rate Wait: ${rate_wait}ms"
        
        # Save to CSV
        echo "$workers,$duration,$throughput,$avg_total,$rate_wait,$blocking" >> "$RESULTS_CSV"
        echo "$workers,$duration_min min,$throughput/min,${avg_total}s,${rate_wait}ms,${blocking}%" >> "$RESULTS_DIR/summary.txt"
    else
        echo "  Failed to extract metrics"
    fi
done

# Restore config
mv "$CONFIG_FILE.benchmark_backup" "$CONFIG_FILE"

# Find best
echo ""
echo -e "${BLUE}Results Summary:${NC}"
cat "$RESULTS_DIR/summary.txt"
echo ""

BEST_LINE=$(tail -n +2 "$RESULTS_CSV" | sort -t',' -k3 -rn | head -1)
BEST_WORKERS=$(echo "$BEST_LINE" | cut -d',' -f1)
BEST_THROUGHPUT=$(echo "$BEST_LINE" | cut -d',' -f3)

echo -e "${GREEN}✓ OPTIMAL: $BEST_WORKERS workers ($BEST_THROUGHPUT jobs/min)${NC}"
echo ""
echo "Detailed results: $RESULTS_DIR"
