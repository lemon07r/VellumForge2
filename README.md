# VellumForge2

High-performance synthetic dataset generator for LLM training. Generates DPO, SFT, KTO, and MO-DPO datasets using any OpenAI-compatible API with hierarchical generation pipeline, checkpoint/resume support, and optional LLM-as-Judge evaluation.

```bash
./bin/vellumforge2 run --config config.toml
```

## Features

### Multiple Dataset Formats
- **SFT** - Simple instruction-output pairs for supervised fine-tuning
- **DPO** - Standard preference pairs (prompt, chosen, rejected) compatible with HuggingFace TRL
- **KTO** - Unpaired preferences with binary labels compatible with HuggingFace TRL KTOTrainer
- **MO-DPO** - Full multi-objective DPO with detailed judge scoring for reward modeling

### High Performance
- Concurrent worker pool supporting up to 1024 parallel requests or more
- Provider-level and per-model rate limiting with configurable burst capacity
- Checkpoint/resume for interrupted sessions
- Asynchronous judge evaluation (non-blocking)
- Smart over-generation strategy achieving 95%+ count accuracy
- Robust 4-strategy JSON parsing with 99%+ success rate

### Provider Agnostic
Works with any OpenAI-compatible API: OpenAI, NVIDIA NIM, Anthropic, Together AI, llama.cpp, Ollama, LM Studio, kobold.cpp, vLLM, and more.

### Configurable Pipeline
- Hierarchical generation: Main topic → Subtopics → Prompts → Preference pairs
- Custom prompt templates at every stage
- Optional LLM-as-Judge quality filtering (40-60% token savings vs full evaluation)
- Flexible rate limiting strategies

### Hugging Face Integration
One-command dataset uploads with automatic repository creation using native NDJSON commit API (no external dependancies like HF CLI required).

## Installation

### Prebuilt Binaries

Download from [releases page](https://github.com/lemon07r/vellumforge2/releases) for Linux, macOS (x86_64/ARM64), and Windows.

### From Source

```bash
git clone https://github.com/lemon07r/vellumforge2.git
cd vellumforge2
make build
# Binary at ./bin/vellumforge2
```

## Quick Start

```bash
# 1. Copy configuration template
cp configs/config.example.toml config.toml
cp configs/.env.example .env

# 2. Edit .env with your API keys
# NVIDIA_API_KEY=nvapi-your-key
# OPENAI_API_KEY=sk-your-key

# 3. Edit config.toml with your settings
# Choose dataset_mode, configure models, customize prompts

# 4. Generate dataset
./bin/vellumforge2 run --config config.toml

# 5. Results in output/session_YYYY-MM-DDTHH-MM-SS/dataset.jsonl
```

See [GETTING_STARTED.md](GETTING_STARTED.md) for step-by-step tutorial.

## Configuration

Minimal configuration:

```toml
[generation]
main_topic = "Fantasy Fiction"
num_subtopics = 64
num_prompts_per_subtopic = 2
concurrency = 64
dataset_mode = "dpo"  # Options: sft, dpo, kto, mo-dpo

[models.main]
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "moonshotai/kimi-k2-instruct-0905"
temperature = 0.6
max_output_tokens = 8192
rate_limit_per_minute = 40

[models.rejected]  # Required for DPO/KTO/MO-DPO
base_url = "http://localhost:8080/v1" # Default URL for llama.cpp local server, but you can use any api of choice
model_name = "phi-4-mini-instruct"
temperature = 0.0
max_output_tokens = 4096

[prompt_templates]
chosen_generation = "Write a compelling story (400-600 words): {{.Prompt}}"
rejected_generation = "Write a simple story (200-300 words): {{.Prompt}}"
```

Complete configuration reference in [configs/config.example.toml](configs/config.example.toml).

## Dataset Modes

### Mode Selection

| Mode | Output Format | Models Required | HuggingFace TRL | Use Case |
|------|--------------|-----------------|-----------------|----------|
| sft | instruction → output | Main only | SFTTrainer | Basic fine-tuning |
| dpo | prompt, chosen, rejected | Main + Rejected | DPOTrainer | Preference optimization |
| kto | prompt, completion, label | Main + Rejected | KTOTrainer | Unpaired preferences |
| mo-dpo | Full schema + judge scores | Main + Rejected + Judge | Custom | Multi-objective training |

### Example Outputs

**SFT Format:**
```json
{
  "instruction": "Write a fantasy story about dragons",
  "output": "In the mountains of Eldoria..."
}
```

**DPO Format:**
```json
{
  "prompt": "Write a fantasy story about dragons",
  "chosen": "In the ancient mountains of Eldoria, where mist...",
  "rejected": "There was a dragon. It was big..."
}
```

**KTO Format (2 rows per pair):**
```json
{"prompt": "Write about dragons", "completion": "Good story...", "label": true}
{"prompt": "Write about dragons", "completion": "Bad story...", "label": false}
```

**MO-DPO Format:**
```json
{
  "prompt": "Write a fantasy story about dragons",
  "chosen": "In the mountains...",
  "rejected": "There was a dragon...",
  "chosen_scores": {
    "plot_quality": {"score": 5, "reasoning": "Excellent narrative..."},
    "creativity": {"score": 4, "reasoning": "Fresh perspective..."}
  },
  "rejected_scores": {
    "plot_quality": {"score": 2, "reasoning": "Minimal development..."},
    "creativity": {"score": 2, "reasoning": "Generic treatment..."}
  },
  "chosen_score_total": 4.5,
  "rejected_score_total": 2.0,
  "preference_margin": 2.5
}
```

See [DATASET_MODES.md](DATASET_MODES.md) for detailed format specifications and configuration examples.

## Optional Judge Filtering

Available for SFT, DPO, KTO modes. MO-DPO always includes full judge evaluation.

```toml
[judge_filtering]
enabled = true
use_explanations = false  # Scores only = 40-60% token savings
min_chosen_score = 4.0    # Keep chosen responses >= 4.0
max_rejected_score = 3.0  # Keep rejected responses <= 3.0

[models.judge]
enabled = true
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "meta/llama-3.1-70b-instruct"
temperature = 0.4
max_output_tokens = 2048
```

Filters responses before writing to dataset based on quality scores. Use when API budget is limited or training time is expensive.

## Rate Limiting

### Provider-Level Limits

Global rate limits shared across all models from same provider:

```toml
[provider_rate_limits]
nvidia = 40  # All NVIDIA models share this 40 RPM limit
provider_burst_percent = 15  # 15% burst capacity (default)
```

Takes precedence over per-model limits. Prevents 429 errors when multiple models share one API endpoint.

### Per-Model Limits

Individual model rate limiting:

```toml
[models.main]
rate_limit_per_minute = 40  # Overridden by provider_rate_limits if set
```

### Optimization

Recommended configuration for high throughput:

```toml
[generation]
concurrency = 128  # Or 256 for high throughput, recommended to test with the bencmark scripts

[provider_rate_limits]
nvidia = 40
provider_burst_percent = 20  # Higher burst for better throughput
```

Conservative configuration for avoiding rate limits:

```toml
[generation]
concurrency = 32

[provider_rate_limits]
nvidia = 30
provider_burst_percent = 10  # Lower burst for fewer 429 errors
```

See [BENCHMARK_README.md](BENCHMARK_README.md) for benchmarking guide using our easy to use benchmark scripts.

## Checkpoint & Resume

Enable automatic checkpointing:

```toml
[generation]
enable_checkpointing = true
checkpoint_interval = 10  # Save every 10 completed jobs
```

Resume interrupted session:

```bash
# List available checkpoints
./bin/vellumforge2 checkpoint list

# Inspect checkpoint
./bin/vellumforge2 checkpoint inspect session_2025-11-05T12-34-56

# Resume generation
./bin/vellumforge2 checkpoint resume session_2025-11-05T12-34-56

# Resume with specific config and env file
./bin/vellumforge2 checkpoint resume session_2025-11-05T12-34-56 \
  --config config.sft.toml \
  --env-file .env
```

Graceful shutdown with Ctrl+C automatically saves checkpoint.

## CLI Commands

### Generate Dataset

```bash
# Basic generation
./bin/vellumforge2 run --config config.toml

# With verbose logging
./bin/vellumforge2 run --config config.toml --verbose

# Upload to Hugging Face
./bin/vellumforge2 run --config config.toml \
  --upload-to-hf --hf-repo-id username/my-dataset #--hf-repo-id not required if set in config file
```

### Checkpoint Management

```bash
# List checkpoints
./bin/vellumforge2 checkpoint list

# Inspect checkpoint
./bin/vellumforge2 checkpoint inspect <session-dir>

# Resume from checkpoint
./bin/vellumforge2 checkpoint resume <session-dir>

# Resume with specific config (important if checkpoint used different config file)
./bin/vellumforge2 checkpoint resume <session-dir> --config config.sft.toml --env-file .env
```

### Dataset Transform (SFT→DPO & Rejected Regeneration)

```bash
# Convert an existing SFT dataset into DPO (generates rejected responses)
./bin/vellumforge2 transform \
  --config config.dpo.toml \
  --mode sft-to-dpo \
  --input path/to/sft_dataset.jsonl \
  --output path/to/dpo_from_sft.jsonl

# Regenerate rejected responses for an existing DPO dataset
./bin/vellumforge2 transform \
  --config config.dpo.toml \
  --mode regen-rejected \
  --input path/to/dpo_dataset.jsonl \
  --output path/to/dpo_dataset.regen.jsonl

# Optional: checkpoint/resume for long transforms
./bin/vellumforge2 transform \
  --config config.dpo.toml \
  --mode regen-rejected \
  --input path/to/dpo_dataset.jsonl \
  --output path/to/dpo_dataset.regen.jsonl \
  --checkpoint path/to/transform.checkpoint.json \
  --resume
```

### Other

```bash
# Show version
./bin/vellumforge2 --version

# Show help
./bin/vellumforge2 --help
```

## Output Structure

```
output/
└── session_2025-11-05T12-34-56/
    ├── dataset.jsonl       # Training dataset
    ├── config.toml.bak     # Configuration snapshot
    ├── checkpoint.json     # Resume state (if checkpointing enabled)
    └── session.log         # Structured JSON logs
```

## Example Datasets

Generated with VellumForge2 using Kimi K2 0905 + Phi-4 Instruct:

- **VellumK2-Fantasy-DPO-Tiny-01**: 126 rows - Testing and validation
- **VellumK2-Fantasy-DPO-Small-01**: 1,038 rows - Light training and experiments
- **VellumK2-Fantasy-DPO-Medium-01**: 3,069 rows - Combination training component
- **VellumK2-Fantasy-DPO-Large-01**: 10,222 rows - Large-scale training 
- **VellumK2-Unfettered-DPO-01**: 2,576 rows - Decensoring dataset to reduce refusal on sensitive content

[View all example datasets](https://huggingface.co/collections/lemon07r/vellumforge2-datasets)

## Troubleshooting

### Rate Limit Errors (429)

Reduce concurrency and global limits:

```toml
[provider_rate_limits]
nvidia = 30

[generation]
concurrency = 32
```

### Getting Fewer Records Than Expected

Increase over-generation buffer:

```toml
[generation]
over_generation_buffer = 0.25  # Increase from default 0.15
```

### JSON Parsing Errors

VellumForge2 has 4 fallback parsing strategies achieving 99%+ success rate. If issues persist:
- Use lower temperature (< 0.9)
- Ensure templates explicitly request JSON format
- Set `structure_temperature` lower than `temperature` for JSON generation

### Out of Memory

Reduce concurrency:

```toml
[generation]
concurrency = 32  # Or lower
```

### Connection Refused for Rejected Model

Start local server or use same API endpoint:

```toml
[models.rejected]
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "meta/llama-3.1-8b-instruct"  # Smaller model
```

### Request Timeout Errors for Long-Form Generation

For long responses (>4000 words or >16k tokens), increase HTTP timeout:

```toml
[models.main]
http_timeout_seconds = 900  # 15 minutes (default: 120)
# For very long-form (32k+ tokens):
# http_timeout_seconds = 1800  # 30 minutes
```

Typical generation times:
- 4k tokens: ~1-2 minutes
- 16k tokens: ~3-5 minutes
- 32k tokens: ~5-10+ minutes

Also increase retry settings for stability:

```toml
[models.main]
max_retries = 8              # More retries for long requests (default: 3)
max_backoff_seconds = 300    # Longer backoff cap (default: 120)
```

See [GETTING_STARTED.md](GETTING_STARTED.md) for more troubleshooting.

## Documentation

- [GETTING_STARTED.md](GETTING_STARTED.md) - Step-by-step tutorial
- [DATASET_MODES.md](DATASET_MODES.md) - Detailed format specifications
- [BENCHMARK_README.md](BENCHMARK_README.md) - Performance benchmarking guide
- [CHANGELOG.md](CHANGELOG.md) - Version history
- [configs/config.example.toml](configs/config.example.toml) - Complete configuration reference

## System Requirements

- Go 1.21+ (for building from source)
- Memory: 512MB minimum, 2GB+ recommended
- Network: Stable internet for API calls
- Disk: ~10MB per 1000 records

## Contributing

Contributions welcome:

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Run `make lint && make test`
5. Submit a pull request

## License

MIT License - see [LICENSE](LICENSE) file.

## Citation

```bibtex
@software{vellumforge2,
  title = {VellumForge2: Synthetic Dataset Generator for LLM Training},
  author = {Lamim},
  year = {2025},
  url = {https://github.com/lemon07r/vellumforge2},
  version = {1.6.0}
}
```

## Acknowledgments

- [Direct Preference Optimization](https://arxiv.org/abs/2305.18290) by Rafailov et al.
- [Kahneman-Tversky Optimization](https://arxiv.org/abs/2402.01306) for KTO
- [HuggingFace TRL](https://github.com/huggingface/trl) for training framework inspiration

## Support

- [GitHub Issues](https://github.com/lemon07r/vellumforge2/issues) - Bug reports and feature requests
- [Discussions](https://github.com/lemon07r/vellumforge2/discussions) - Questions and community help

---

Current Version: v1.6.0
Last Updated: 2025-11-06
