# Worker Count Benchmarking Guide

## Overview

Two benchmarking scripts to empirically test different worker counts and find the optimal configuration for your system. This tests with the `config.toml` file in your working directory.

## Scripts

### 1. Quick Benchmark (Recommended)
**File:** `scripts/quick_benchmark.sh`  
**Duration:** ~15-20 minutes  
**Tests:** 3 configurations (12, 16, 20 workers)

```bash
# Run with default configurations
./scripts/quick_benchmark.sh

# Or specify custom worker counts
./scripts/quick_benchmark.sh 10 15 20 25
```

### 2. Full Benchmark
**File:** `scripts/benchmark_workers.sh`  
**Duration:** ~30-40 minutes  
**Tests:** 5 configurations (8, 12, 16, 20, 24 workers)

```bash
./scripts/benchmark_workers.sh
```

## Prerequisites

Both scripts require:
- ✅ `jq` 
- ✅ `python3` 

## What Gets Measured

For each worker count, the scripts measure:
- **Duration:** Total time to complete all jobs
- **Throughput:** Jobs processed per minute
- **Avg Job Time:** Average time per job
- **Rate Limit Wait:** Average rate limiter wait time
- **Blocking Rate:** % of API calls that were rate limited

## Output

### Console Output
```
Quick Worker Benchmark
Testing: 12 16 20

Testing 12 workers...
  Duration: 5.25 min | Throughput: 6.10 jobs/min | Rate Wait: 111ms

Testing 16 workers...
  Duration: 4.80 min | Throughput: 6.67 jobs/min | Rate Wait: 245ms

Testing 20 workers...
  Duration: 4.50 min | Throughput: 7.11 jobs/min | Rate Wait: 580ms

Results Summary:
Workers,Duration,Throughput,Avg Job,Rate Wait,Blocking
12,5.25 min,6.10/min,88.7s,111ms,3.0%
16,4.80 min,6.67/min,82.1s,245ms,8.5%
20,4.50 min,7.11/min,76.3s,580ms,15.2%

✓ OPTIMAL: 20 workers (7.11 jobs/min)
```

### Files Generated

Results are saved to `output/quick_bench_YYYYMMDD_HHMMSS/`:
- `results.csv` - Raw data in CSV format
- `summary.txt` - Human-readable summary
- `run_Xw.log` - Full output for each run
- Original session directories are preserved

## Interpreting Results

### Good Configuration ✅
- **Throughput:** Higher is better
- **Rate Wait:** < 500ms
- **Blocking Rate:** < 10%

### Moderate Issues 🟡
- **Rate Wait:** 500-2000ms
- **Blocking Rate:** 10-30%
- Still usable but not optimal

### Severe Problems 🔴
- **Rate Wait:** > 2000ms
- **Blocking Rate:** > 50%
- Too many workers causing rate limit thrashing

## Example Analysis

```
12 workers: 6.10 jobs/min, 111ms wait, 3% blocking   ✅ Good baseline
16 workers: 6.67 jobs/min, 245ms wait, 8.5% blocking ✅ Better throughput
20 workers: 7.11 jobs/min, 580ms wait, 15% blocking  🟡 Best throughput but more limiting
24 workers: 6.80 jobs/min, 1200ms wait, 35% blocking 🔴 Too many workers
```

**In this case:** 20 workers gives best throughput before severe rate limiting kicks in.

## Running a Custom Test

To test specific worker counts that showed promise:

```bash
# Test fine-grained range around optimal
./scripts/quick_benchmark.sh 18 20 22

# Test wider range
./scripts/quick_benchmark.sh 12 15 18 21 24
```

## Tips

1. **Start with quick benchmark** to find the general range
2. **Run fine-grained test** around the optimal point
3. **Consider your priorities:**
   - Maximum throughput? Choose highest jobs/min
   - Minimum rate limiting? Choose lowest blocking rate
   - Balance? Choose sweet spot with <10% blocking

4. **Test at scale:** Results with 32 jobs may differ from 300+ jobs
   - Small datasets have high variance
   - Large datasets show steady-state performance

## Automating the Decision

After benchmarking, update your config:

```bash
# Set optimal worker count
OPTIMAL=20  # From benchmark results
sed -i "s/^concurrency = [0-9]\+/concurrency = $OPTIMAL/" config.toml
```

## Comparing Configurations

To compare before/after optimizations:

```bash
# Run with current settings
./scripts/quick_benchmark.sh 12 16 20

# Modify config (e.g., reduce max_output_tokens)
# Run again
./scripts/quick_benchmark.sh 12 16 20

# Compare the two output directories
```

## Expected Runtimes

For 32 jobs (8 subtopics × 4 prompts):
- **Per configuration:** ~5-6 minutes
- **Quick benchmark (3 configs):** ~15-20 minutes
- **Full benchmark (5 configs):** ~30-40 minutes

For 300 jobs (100 subtopics × 3 prompts):
- **Per configuration:** ~30-50 minutes
- **Quick benchmark:** ~2-3 hours
- **Full benchmark:** ~3-5 hours

## Troubleshooting

### Script fails with "jq: command not found"
```bash
# Install jq
sudo pacman -S jq  # Arch/CachyOS
# or
sudo apt install jq  # Debian/Ubuntu
```

### Script can't find binary
The script automatically builds if needed. If issues persist:
```bash
cd /home/lamim/Development/VellumForge2
go build -o bin/vellumforge2 ./cmd/vellumforge2
```

### Config not restored after failure
Manual restore:
```bash
mv config.toml.benchmark_backup config.toml
```

## Advanced: Scripted Decision

For fully automated optimization:

```bash
#!/bin/bash
# Find and set optimal workers

RESULT=$(./scripts/quick_benchmark.sh 12 16 20 24 | grep "OPTIMAL:" | awk '{print $3}')
sed -i "s/^concurrency = [0-9]\+/concurrency = $RESULT/" config.toml
echo "Set optimal worker count: $RESULT"
```

## Example: Full Optimization Workflow

```bash
# 1. Benchmark current configuration
./scripts/quick_benchmark.sh

# 2. Based on results, test API optimizations
# Edit config.toml: max_output_tokens = 4096

# 3. Re-benchmark with new config
./scripts/quick_benchmark.sh

# 4. Compare results and choose best configuration

# 5. Run full-scale test with optimal settings
# Edit config.toml: num_subtopics = 100
vellumforge2 run --config config.toml
```

## Notes

- **Backups:** Script automatically backs up config before testing
- **Restoration:** Config is restored after completion (or failure)
- **Concurrent runs:** Don't run multiple benchmarks simultaneously
- **API limits:** Ensure you have API quota available
- **Local LLM:** Make sure local server is running if used
