# Getting Started with VellumForge2

Complete guide to installing, configuring, and running VellumForge2 for the first time.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Configuration](#configuration)
- [First Run](#first-run)
- [Common Use Cases](#common-use-cases)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)
- [Advanced Configuration](#advanced-configuration)

## Prerequisites

- **Go 1.21+** (if building from source)
- **API Key** for OpenAI-compatible provider (OpenAI, NVIDIA, Anthropic, Together AI, etc.)
- **512MB RAM minimum** (2GB+ recommended for large datasets)
- **Stable internet** for API calls

## Installation

### Option 1: Download Prebuilt Binary (Recommended)

1. Visit the [releases page](https://github.com/lemon07r/vellumforge2/releases)
2. Download the binary for your platform:
   - Linux: `vellumforge2-linux-amd64`
   - macOS Intel/AMD: `vellumforge2-darwin-amd64`
   - macOS Apple Silicon: `vellumforge2-darwin-arm64`
   - Windows: `vellumforge2-windows-amd64.exe`
3. Make it executable (Linux/macOS):
   ```bash
   chmod +x vellumforge2 # may require sudo
   ```

### Option 2: Build from Source

```bash
# Clone repository
git clone https://github.com/lemon07r/vellumforge2.git
cd vellumforge2

# Install dependencies
make install

# Build binary
make build

# Binary location: ./bin/vellumforge2
```

#### Platform-Specific Builds

```bash
# Build for all platforms
make build-all

# Or build for specific platform
make build-linux
make build-darwin
make build-windows
```

### Verify Installation

```bash
./bin/vellumforge2 --version
# Output: vellumforge2 version 1.5.0 (commit: ..., built: ...)
```

## Configuration

### Step 1: Create Configuration File

```bash
# Copy example configuration
cp configs/config.example.toml config.toml
```

Edit `config.toml` with your settings. Minimal configuration:

```toml
[generation]
main_topic = "Fantasy Fiction"
num_subtopics = 10
num_prompts_per_subtopic = 2
concurrency = 32
dataset_mode = "dpo"  # Options: sft, dpo, kto, mo-dpo

[models.main]
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "moonshotai/kimi-k2-instruct-0905"
temperature = 0.6
max_output_tokens = 8192
rate_limit_per_minute = 40

[models.rejected]  # Required for DPO/KTO/MO-DPO
base_url = "http://localhost:8080/v1"
model_name = "phi-4-mini-instruct"
temperature = 0.0
max_output_tokens = 4096

[prompt_templates]
chosen_generation = "Write a compelling story (400-600 words): {{.Prompt}}"
rejected_generation = "Write a simple story (200-300 words): {{.Prompt}}"
```

**Note:** Using a local model for rejected responses significantly reduces API costs and can help avoid rate limits to help generate datsets faster. See [Local Server Setup](#local-server-setup) below.

### Step 2: Set Up API Keys

```bash
# Copy environment template
cp configs/.env.example .env
```

Edit `.env` with your API keys:

```bash
# Choose the provider you're using
NVIDIA_API_KEY=nvapi-your-key-here
# or
OPENAI_API_KEY=sk-your-key-here
# or
ANTHROPIC_API_KEY=sk-ant-your-key-here

# Optional: For Hugging Face uploads
HUGGINGFACE_TOKEN=hf_your-token-here
```

**Important:** Keep `.env` secure. Never commit it to version control.

## First Run

### Test with Small Dataset

Start with a small test run to verify everything works:

```bash
# Edit config.toml to use small values:
# num_subtopics = 4
# num_prompts_per_subtopic = 2
# This will generate 8 preference pairs (takes ~2-5 minutes)

./bin/vellumforge2 run --config config.toml --verbose
```

Expected output:

```
Loaded configuration from config.toml
Session: session_2025-11-05T12-34-56
Mode: DPO (standard preference pairs)

[Stage 1/3] Generating subtopics...
Generated 4/4 subtopics

[Stage 2/3] Generating prompts...
Generated 8/8 prompts

[Stage 3/3] Generating preference pairs...
[========] 8/8 jobs (100.0%) | 2.1 jobs/min

Generation complete
Success: 8/8 | Failures: 0 | Time: 3m 47s
Output: output/session_2025-11-05T12-34-56/dataset.jsonl
```

### Check the Results

```bash
# View first record
head -n 1 output/session_*/dataset.jsonl | jq .

# Output (DPO format):
{
  "prompt": "Write a fantasy story about a dragon discovering ancient magic",
  "chosen": "In the mountains of Eldoria, where mist cloaked the highest peaks...",
  "rejected": "There was a dragon named Bob. He found some magic..."
}

# Count records
wc -l output/session_*/dataset.jsonl
```

## Common Use Cases

### 1. Generate Story Dataset

For creative fiction with strong vs weak responses:

```toml
[generation]
main_topic = "Fantasy Fiction"
num_subtopics = 100
num_prompts_per_subtopic = 10
concurrency = 64
dataset_mode = "dpo"

[models.main]
model_name = "moonshotai/kimi-k2-instruct-0905"
temperature = 0.6
max_output_tokens = 8192

[models.rejected]
model_name = "meta/llama-3.1-8b-instruct"  # Smaller model for weaker responses
temperature = 0.0 # Adjust temperature for desired effect on responses
max_output_tokens = 4096

[prompt_templates]
chosen_generation = '''You are a master storyteller. Write a compelling short story (400-600 words) based on this prompt:

{{.Prompt}}

Include vivid descriptions, engaging characters, and strong narrative voice.'''

rejected_generation = '''Write a simple story based on this prompt:

{{.Prompt}}

Write 200-300 words.'''
```

### 2. Technical Writing Dataset

For instructional or technical content:

```toml
[generation]
main_topic = "Software Development Tutorials"
num_subtopics = 50
num_prompts_per_subtopic = 5
concurrency = 64
dataset_mode = "sft"  # Simple instruction-output pairs

[models.main]
model_name = "minimaxai/minimax-m2"
temperature = 0.4  # Lower temperature for technical accuracy
max_output_tokens = 3072

[prompt_templates]
chosen_generation = '''Write a clear, accurate technical explanation for:

{{.Prompt}}

Include code examples where appropriate and explain key concepts step-by-step.'''
```

### 3. With Judge Filtering

For quality-controlled datasets:

```toml
[generation]
dataset_mode = "dpo"

[judge_filtering]
enabled = true
use_explanations = false  # Scores only for efficiency
min_chosen_score = 4.5    # Keep only high-quality responses
max_rejected_score = 2.5

[models.judge]
enabled = true
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "meta/llama-3.1-70b-instruct"
temperature = 0.4
max_output_tokens = 2048

[prompt_templates]
judge_rubric = '''Evaluate this story on a 1-5 scale for:

STORY:
{{.StoryText}}

Criteria:
1. plot_quality
2. writing_quality
3. creativity

Return ONLY valid JSON:
{
  "plot_quality": {"score": 1-5, "reasoning": "..."},
  "writing_quality": {"score": 1-5, "reasoning": "..."},
  "creativity": {"score": 1-5, "reasoning": "..."}
}'''
```

### 4. Upload to Hugging Face

```bash
# Set token in .env
echo "HUGGINGFACE_TOKEN=hf_your_token" >> .env

# Generate and upload
./bin/vellumforge2 run --config config.toml \
  --upload-to-hf \
  --hf-repo-id username/my-fantasy-dataset
```

## Troubleshooting

### Rate Limit Exceeded (429 Errors)

**Symptom:** Frequent rate limit errors in logs

**Solution 1:** Configure provider-level rate limiting

```toml
[provider_rate_limits]
nvidia = 30  # Reduce from default 40
provider_burst_percent = 10  # Reduce burst capacity

[generation]
concurrency = 32  # Reduce workers
```

**Solution 2:** Reduce per-model limits

```toml
[models.main]
rate_limit_per_minute = 30  # Lower than actual limit for safety
```

**Solution 3:** Use local model for rejected responses

```toml
[models.rejected]
base_url = "http://localhost:8080/v1"  # No API rate limits
model_name = "phi-4-mini-instruct"
```

### JSON Parsing Errors

**Symptom:** "Failed to parse JSON response" in logs

**Solution:** VellumForge2 has 4 fallback parsing strategies achieving 99%+ success rate. If issues persist:

1. Lower temperature for JSON generation:
   ```toml
   [models.main]
   temperature = 0.7
   structure_temperature = 0.2  # For JSON outputs
   ```

2. Use concrete examples in prompts:
   ```toml
   [prompt_templates]
   subtopic_generation = '''Generate subtopics.

   Return ONLY a JSON array:
   ["subtopic 1", "subtopic 2", "subtopic 3"]

   Generate now:'''
   ```

3. Avoid `use_json_mode` if model wraps arrays:
   ```toml
   [models.main]
   use_json_mode = false  # Some models wrap arrays in objects
   ```

### Getting Fewer Subtopics/Prompts Than Requested

**Symptom:** Generated 8 subtopics instead of 10

**Solution:** The over-generation strategy handles this automatically. If consistently getting fewer items:

```toml
[generation]
over_generation_buffer = 0.25  # Increase from default 0.15 (15%) to 25%
```

This requests 25% extra, then deduplicates and trims to exact count.

For large counts, use chunking:

```toml
[generation]
num_subtopics = 200
subtopic_chunk_size = 30  # Request in chunks of 30
```

### Out of Memory

**Symptom:** Process killed or memory exhaustion errors

**Solution:** Reduce concurrency

```toml
[generation]
concurrency = 32  # Or lower depending on available RAM
```

Memory usage scales with concurrency and max_output_tokens. Approximate formula:
```
RAM ≈ concurrency × max_output_tokens × 4 bytes × 3 (main + rejected + judge)
```

### Connection Timeout

**Symptom:** "connection refused" or "dial tcp" errors

**Solution:** Increase backoff duration

```toml
[models.main]
max_backoff_seconds = 180  # Increase from default 120
max_retries = 10  # Increase from default 3 for unstable connections
```

For local servers, set higher retries:

```toml
[models.rejected]
max_retries = 20
```

### Cannot Stop Generation

**Problem:** Need to interrupt long-running generation

**Solution:** Press Ctrl+C for graceful shutdown

```
^C
Interrupt signal received. Shutting down gracefully...
Saving checkpoint...
Checkpoint saved: output/session_2025-11-05T12-34-56/checkpoint.json
Shutdown complete. Progress saved.

Resume with:
  vellumforge2 checkpoint resume session_2025-11-05T12-34-56
```

If graceful shutdown fails, use Ctrl+C twice for immediate exit (checkpoint may not save).

### Generation Interrupted, Want to Resume

**Solution:** Use checkpoint resume feature

```bash
# Enable checkpointing in config.toml
[generation]
enable_checkpointing = true
checkpoint_interval = 10  # Save every 10 jobs

# If interrupted, list available sessions
./bin/vellumforge2 checkpoint list

# Inspect checkpoint
./bin/vellumforge2 checkpoint inspect session_2025-11-05T12-34-56

# Resume generation
./bin/vellumforge2 checkpoint resume session_2025-11-05T12-34-56
```

The resume command validates checkpoint compatibility and appends to existing dataset.

### Config Validation Errors

**Symptom:** "validation error: concurrency exceeds maximum"

**Solution:** If you need values exceeding safety limits:

```toml
[generation]
disable_validation_limits = true  # USE WITH CAUTION
concurrency = 512  # Now allowed
num_subtopics = 50000  # Now allowed
```

**Warning:** Very high values may cause memory exhaustion or API rate limits. Only enable if you know what you're doing.

### Exclusion List Truncated Warning

**Symptom:** "Exclusion list truncated from 75 to 50 items"

**Explanation:** This is normal behavior when retrying after duplicates. The exclusion list is limited to prevent context overflow.

**Solution:** If you need larger exclusion lists (large context models):

```toml
[generation]
max_exclusion_list_size = 100  # Increase from default 50
```

## Best Practices

### 1. Start Small

Always test with small values first:

```toml
[generation]
num_subtopics = 4
num_prompts_per_subtopic = 2
# Total: 8 records, takes ~3-5 minutes
```

Verify output quality before scaling to production values.

### 2. Monitor API Costs

Track spending using provider dashboards:

- OpenAI: https://platform.openai.com/usage
- Anthropic: https://console.anthropic.com/settings/usage

You can use small local models for rejected responses to help reduce costs.

### 3. Use Version Control

Track configuration changes:

```bash
git add config.toml
git commit -m "Configure fantasy fiction DPO dataset"
```

Never commit `.env` file with API keys.

### 4. Backup Sessions

Session directories contain everything needed to reproduce results:

```bash
# Backup completed session
tar -czf session_2025-11-05.tar.gz output/session_2025-11-05T12-34-56/

# Archive to separate location
mv session_2025-11-05.tar.gz ~/backups/
```

### 5. Test Model Combinations

Experiment with different main/rejected model pairs:

```toml
# Option 1: Same model, different temperatures
[models.main]
model_name = "meta/llama-3.1-70b-instruct"
temperature = 0.6

[models.rejected]
model_name = "meta/llama-3.1-70b-instruct"
temperature = 1.2

# Option 2: Different models
[models.main]
model_name = "meta/llama-3.1-70b-instruct"

[models.rejected]
model_name = "meta/llama-3.1-8b-instruct"

# Option 3: Instruct vs base model
[models.main]
model_name = "meta/llama-3.1-70b-instruct"

[models.rejected]
model_name = "meta/llama-3.1-70b"  # Base model (no instruct)
```

### 6. Large Subtopic Counts

For num_subtopics > 100, always use chunking:

```toml
[generation]
num_subtopics = 500
subtopic_chunk_size = 30  # Request in chunks
over_generation_buffer = 0.20  # Slightly higher buffer
max_exclusion_list_size = 75  # Larger exclusion list
```

This significantly reduces JSON parsing errors.

## Advanced Configuration

### Local Server Setup

#### llama.cpp

```bash
# Download model
wget https://huggingface.co/unsloth/Phi-4-mini-instruct-GGUF/resolve/main/Phi-4-mini-instruct-Q4_K_M.gguf # OR any other small model, try to keep it similar to the model you want to train e.g. use Qwen3 4b if you plan to train Qwen3 32b, etc.

# Start server (found in /build/bin/ after building, see https://github.com/ggml-org/llama.cpp for more information)
./llama-server -m ~/ggufs/Phi-4-mini-instruct-Q4_K_M.gguf -c 8192 --no-webui --parallel 8 --n-gpu-layers 99 --flash-attn on --cont-batching # Adjust based on your system's resources, these are good settings for performance. Context size can be lowered for better performance, and the parallelism can be decreased for more tokens/sec at the cost of less requests per minute.

# Configure in config.toml
[models.rejected]
base_url = "http://localhost:8080/v1"
model_name = "phi-4-mini-instruct-Q6_K.gguf"
rate_limit_per_minute = 60  # Local models can handle higher rates
max_retries = 20  # Higher for local startup time
```

#### Ollama (not recommended over other options)

```bash
# Install Ollama
curl -fsSL https://ollama.com/install.sh | sh

# Pull model
ollama pull phi3

# Starts automatically at localhost:11434

# Configure in config.toml
[models.rejected]
base_url = "http://localhost:11434/v1"
model_name = "phi3"
rate_limit_per_minute = 60
max_retries = 20
```

vLLM is recommended for more advanced users for better performance and scalability.

### Custom Prompt Templates

Full customization with template variables:

```toml
[prompt_templates]

subtopic_generation = '''You are an expert in {{.MainTopic}}. Generate {{.NumSubtopics}} unique subtopics.

{{if .IsRetry}}Avoid these already generated: {{.ExcludeSubtopics}}
{{end}}

Requirements:
- Specific and detailed
- Unique from each other
- Rich with potential

Return ONLY a JSON array: ["subtopic 1", "subtopic 2", ...]'''

prompt_generation = '''Main topic: {{.MainTopic}}
Subtopic: {{.SubTopic}}

Generate {{.NumPrompts}} unique and compelling prompts for creative writing.

Return ONLY a JSON array: ["prompt 1", "prompt 2", ...]'''

chosen_generation = '''Main Topic: {{.MainTopic}}
Subtopic: {{.SubTopic}}
Prompt: {{.Prompt}}

Write a masterful response showcasing expert-level quality.'''

rejected_generation = '''Prompt: {{.Prompt}}

Write a basic response.'''
```

Available variables:
- `subtopic_generation`: MainTopic, NumSubtopics, IsRetry, ExcludeSubtopics
- `prompt_generation`: SubTopic, NumPrompts, MainTopic
- `chosen_generation`: Prompt, MainTopic, SubTopic
- `rejected_generation`: Prompt, MainTopic, SubTopic
- `judge_rubric`: Prompt, StoryText

### Performance Tuning

#### High-Throughput Setup

For maximum speed with provider rate limiting:

```toml
[provider_rate_limits]
nvidia = 40
provider_burst_percent = 25  # Higher burst for better throughput

[generation]
concurrency = 256  # Maximum workers
over_generation_buffer = 0.15

[models.main]
rate_limit_per_minute = 40

[models.rejected]
base_url = "http://localhost:8080/v1"  # Local model = no rate limits
rate_limit_per_minute = 120
```

#### Conservative Setup

For avoiding rate limits entirely:

```toml
[provider_rate_limits]
nvidia = 30  # Below actual limit
provider_burst_percent = 10  # Low burst

[generation]
concurrency = 32  # Conservative worker count

[models.main]
rate_limit_per_minute = 30
```

#### Balanced Setup

For good throughput with reliability:

```toml
[provider_rate_limits]
nvidia = 40
provider_burst_percent = 15  # Default

[generation]
concurrency = 64  # Moderate worker count

[models.main]
rate_limit_per_minute = 40
```

## Next Steps

- Explore dataset modes: [DATASET_MODES.md](DATASET_MODES.md)
- Benchmark your config: [BENCHMARK_README.md](BENCHMARK_README.md)
- Check changelog: [CHANGELOG.md](CHANGELOG.md)
- View complete config reference: [configs/config.example.toml](configs/config.example.toml)

## Getting Help

- Check [Troubleshooting](#troubleshooting) section above
- Search [GitHub Issues](https://github.com/lemon07r/vellumforge2/issues)
- Ask in [Discussions](https://github.com/lemon07r/vellumforge2/discussions)
- Review example datasets: [HuggingFace Collection](https://huggingface.co/collections/lemon07r/vellumforge2-datasets)
