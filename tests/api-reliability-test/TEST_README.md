# API Reliability Stress Test

This test package reproduces the streaming reliability issues discovered in VellumForge2 dataset generation under load.

## Purpose

Identify and compare API streaming reliability under realistic stress conditions:
- 100 concurrent workers
- 100 stories generated
- 32,768 max output tokens
- 4,000+ word story targets
- ~30-45 minute duration

## Background

During a 4,000-story generation run with the nahcrof API, 21% of outputs (840 stories) were incomplete mid-sentence cutoffs. This test reproduces those conditions to:
1. Verify the issue is reproducible
2. Compare different API providers
3. Validate that the new output validation catches these issues

## Test Results Summary

| Provider | Success Rate | Incomplete Caught | Dataset Quality |
|----------|--------------|-------------------|-----------------|
| **nahcrof** | 27% | 71 | 27/27 (100%) |
| **chutes** | 96% | 4 | 96/96 (100%) |

**Conclusion:** nahcrof streaming has serious reliability issues under load. chutes is significantly more reliable.

---

## Requirements

- VellumForge2 built from CoT branch
- API keys for providers you want to test
- Python 3.x (for analysis script)
- 30-45 minutes per provider test

---

## Setup

### Step 1: Clone the Repository and Checkout the Branch

```bash
# Clone the VellumForge2 repository
git clone https://github.com/yourusername/VellumForge2.git
cd VellumForge2

# Checkout the branch with API reliability test
git checkout CoT
```

### Step 2: Build VellumForge2

```bash
# Install Go dependencies
make install

# Build the binary (with all tests)
make all
# Or without tests
make build

# Verify binary was created
ls -lh bin/vellumforge2
```

### Step 3: Navigate to Test Directory

```bash
# Change to the test directory
cd tests/api-reliability-test

# Verify test files exist
ls -la
# Should see: README.md, run_test.sh, analyze_results.py, config files, etc.
```

### Step 4: Configure API Keys

Copy the example env files and add your API keys:

```bash
# For nahcrof
cp .env.nahcrof.example .env.nahcrof
nano .env.nahcrof  # or use your preferred editor
# Set: API_KEY=your_nahcrof_api_key_here

# For chutes
cp .env.chutes.example .env.chutes
nano .env.chutes  # or use your preferred editor
# Set: API_KEY=your_chutes_api_key_here

# Verify keys are set (should NOT show "example")
cat .env.nahcrof
cat .env.chutes
```

**Security Note:** Never commit `.env.nahcrof` or `.env.chutes` files - they contain your API keys!

---

## Running the Test

### Test Both Providers (Recommended)

```bash
# Make sure you're in the VellumForge2 root directory
cd /path/to/VellumForge2

# Run the test script
./tests/api-reliability-test/run_test.sh

# OR if you're already in the test directory:
# cd tests/api-reliability-test
# ../../bin/vellumforge2 run --config config.nahcrof.toml --env-file .env.nahcrof
```

This will:
1. Test nahcrof (~5-15 minutes depending on failures)
2. Test chutes (~10-15 minutes)
3. Analyze and compare results
4. Display summary

**Expected Duration:** 5~20 minutes total

### Test Single Provider

```bash
# From VellumForge2 root directory

# Test only nahcrof
./tests/api-reliability-test/run_test.sh nahcrof

# Test only chutes
./tests/api-reliability-test/run_test.sh chutes
```

### Manual Test Run (Alternative Method)

If the script doesn't work, you can run tests manually:

```bash
# From VellumForge2 root directory

# Test nahcrof
./bin/vellumforge2 run \
  --config tests/api-reliability-test/config.nahcrof.toml \
  --env-file tests/api-reliability-test/.env.nahcrof

# Test chutes
./bin/vellumforge2 run \
  --config tests/api-reliability-test/config.chutes.toml \
  --env-file tests/api-reliability-test/.env.chutes

# Analyze results
python3 tests/api-reliability-test/analyze_results.py
```

---

## Understanding the Results

### Success Rate

Percentage of stories that completed with proper endings (terminal punctuation).

- **High (>90%)**: Reliable API, use for production
- **Medium (50-90%)**: Some issues, monitor closely
- **Low (<50%)**: Significant reliability problems

### Incomplete Caught

Number of outputs that were cut off mid-sentence and caught by validation:
- These would have polluted the training dataset without validation
- High number indicates API reliability issues

### Dataset Quality

Percentage of saved records that are complete:
- Should always be 100% with the new validation
- Lower means incomplete outputs made it through (validation bug)

---

## Test Configuration Details

### Stress Conditions
```toml
num_subtopics = 10
num_prompts_per_subtopic = 10
concurrency = 100              # High load
max_output_tokens = 32768      # Large outputs
chosen_generation = "...at least 24,000 characters, or 4,000 words..."
```

### Why These Conditions?

- **100 workers**: Stresses API rate limiting and concurrency handling
- **32k tokens**: Tests maximum output handling
- **4,000 word target**: Forces long generations that expose streaming issues
- **100 stories**: Enough volume to see patterns

Small tests (8 stories, 16 workers) won't reproduce the issues!

---

## Interpreting Results

### If You See High Incomplete Rates

**nahcrof results (27% success, 71 incomplete):**
```
✗ 73% FAILURE RATE
✗ Most outputs cut off mid-sentence
✗ Missing finish_reason in responses
→ API streaming implementation has bugs
→ Recommendation: Switch providers or disable streaming
```

**Example incomplete output:**
```
"...their voice amplified"  (19,753 chars, no punctuation)
```

### If You See High Success Rates

**chutes results (96% success, 4 incomplete):**
```
✓ 96% SUCCESS RATE
✓ Properly completes outputs
✓ Returns finish_reason correctly
→ Reliable streaming implementation
→ Safe for production use
```

---

## What the Validation Catches

The new validation (commits 67bb2a7, 65f6b99) detects:

1. **Missing finish_reason** (API didn't complete properly)
2. **No terminal punctuation** (`.!?"'`)
3. **Lowercase last word** (mid-sentence cutoff)

**Example detections:**
```
✗ "...their voice amplified"     → Last word lowercase, no punctuation
✗ "...turn away refugees"         → Mid-sentence
✗ "...A loop"                     → Incomplete thought
✓ "...happily ever after."        → Complete ✓
```

---

## Reproducing Original Issues

### Original Session That Had Problems
- Dataset: 4,009 stories
- Provider: nahcrof
- Incomplete: 840 (21%)
- Duration: 24+ hours
- Settings: 32k tokens, 100 workers, streaming

### This Stress Test
- Dataset: 100 stories
- Provider: nahcrof (test 1) or chutes (test 2)
- Expected: ~27% success (nahcrof), ~96% success (chutes)
- Duration: 5-15 minutes per provider
- Settings: 32k tokens, 100 workers, streaming

**The 73% incomplete rate in stress test → ~21-27% would slip through old validation → Matches original 21%!**

---

## Files in This Package

```
tests/api-reliability-test/
├── README.md                    # This file
├── run_test.sh                  # Test runner script
├── analyze_results.py           # Results analysis script
├── config.nahcrof.toml          # nahcrof test config
├── config.chutes.toml           # chutes test config
├── .env.nahcrof.example         # API key template (nahcrof)
├── .env.chutes.example          # API key template (chutes)
└── (generated after running tests)
    ├── test_nahcrof.log         # Full test output
    ├── test_chutes.log          # Full test output
    └── results.json             # Analysis results
```

---

## Validation Code Reference

The validation that catches these issues is in:
- `internal/orchestrator/validation.go` - isIncompleteOutput(), validateFinishReason()
- `internal/orchestrator/worker.go` - Integration into processing pipeline

**Key functions:**
```go
// Detects mid-sentence cutoffs
func isIncompleteOutput(text string, finishReason string) (bool, string)

// Validates API completion status
func validateFinishReason(finishReason string, hasReasoning bool, contentLength int) (bool, string)
```

---

## Expected Behavior

### With nahcrof (ai.nahcrof.com)
- Many "Incomplete output detected" warnings
- Many "Invalid finish_reason" warnings  
- Low success rate (27-40%)
- High retry rate
- Dataset quality: 100% (validation prevents incomplete from saving)

### With chutes (llm.chutes.ai)
- Few warnings
- High success rate (95-99%)
- Low retry rate
- Dataset quality: 100%

---

## Troubleshooting

### Test Fails Immediately
- Check API keys in `.env.nahcrof` or `.env.chutes`
- Verify API endpoint URLs are correct
- Check network connectivity

### Test Times Out
- Normal for nahcrof (many retries)
- Increase timeout in run_test.sh: `timeout 3600` (1 hour)

### Different Results Than Expected
- API reliability may vary by time of day/load
- Some variation is normal (±10%)
- Consistent patterns across multiple runs indicate real issues

### No Incomplete Outputs Detected
- Might be a good run (APIs can be intermittent)
- Try running multiple times
- Increase concurrency: `concurrency = 200`
- Increase story count: `num_subtopics = 20`

---

## Complete Reproduction Steps for Others

### Step-by-Step Instructions

1. **Clone the Repository**
   ```bash
   git clone https://github.com/yourusername/VellumForge2.git
   cd VellumForge2
   ```

2. **Checkout the Correct Branch**
   ```bash
   git checkout CoT
   git log --oneline -1
   # Verify: Should show commit 67bb2a7 or later
   ```

3. **Install Dependencies and Build**
   ```bash
   # Install Go dependencies
   make install
   
   # Build the binary
   make build
   
   # Verify build
   ls -lh bin/vellumforge2
   ```

4. **Navigate to Test Directory**
   ```bash
   cd tests/api-reliability-test
   pwd
   # Should show: /path/to/VellumForge2/tests/api-reliability-test
   ```

5. **Configure API Keys**
   ```bash
   # Copy example files
   cp .env.nahcrof.example .env.nahcrof
   cp .env.chutes.example .env.chutes
   
   # Edit with your API keys
   nano .env.nahcrof  # Add: API_KEY=your_nahcrof_key
   nano .env.chutes   # Add: API_KEY=your_chutes_key
   ```

6. **Run the Test**
   ```bash
   # Return to repository root
   cd ../..
   
   # Run test script
   ./tests/api-reliability-test/run_test.sh
   
   # Wait 30-45 minutes for completion
   ```

7. **Review Results**
   ```bash
   # Results display automatically
   # Or manually run analysis:
   python3 tests/api-reliability-test/analyze_results.py
   ```

8. **Check Generated Files**
   ```bash
   # Test session directories
   ls -lt output/session_*/
   
   # Test logs
   cat tests/api-reliability-test/test_nahcrof.log
   cat tests/api-reliability-test/test_chutes.log
   
   # Analysis results
   cat tests/api-reliability-test/results.json
   ```

### Expected Results

Results should match within ±10%:
- nahcrof: 27% success, ~71-73 incomplete caught
- chutes: 96% success, ~4 incomplete caught

### Troubleshooting for Recipients

**Binary not found:**
```bash
# Make sure you built from repository root
cd /path/to/VellumForge2
make build
```

**Permission denied on scripts:**
```bash
chmod +x tests/api-reliability-test/run_test.sh
chmod +x tests/api-reliability-test/analyze_results.py
```

**API key not working:**
```bash
# Verify format (no quotes, no spaces)
cat tests/api-reliability-test/.env.nahcrof
# Should show: API_KEY=abc123xyz (no quotes)
```

**Python script errors:**
```bash
# Check Python version
python3 --version
# Requires Python 3.x
```

---

## Analysis Commands

### Manual Analysis

```bash
# Count incomplete outputs caught
grep "Incomplete output detected" output/session_*/session.log | wc -l

# Count missing finish_reason
grep "Invalid finish_reason" output/session_*/session.log | wc -l

# Check dataset quality
python3 << 'EOF'
import json
with open('output/latest/dataset.jsonl') as f:
    records = [json.loads(line) for line in f]
    complete = sum(1 for r in records if r['output'].strip()[-1] in '.!?"')
    print(f"Quality: {complete}/{len(records)} complete ({complete/len(records)*100:.1f}%)")
EOF
```

### Automated Analysis

```bash
# Run the provided analysis script
python3 tests/api-reliability-test/analyze_results.py
```

---

## References

- Original issue: 840 incomplete outputs (21%) in production run
- Validation implementation: Commits 67bb2a7, 65f6b99

---

## License

Same as VellumForge2 main project.

---

**Status:** Ready to use  
**Last Updated:** 2025-11-13  
**Version:** 1.0
