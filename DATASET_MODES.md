# Dataset Modes

VellumForge2 supports four dataset formats for different dataset structures. Select the mode that matches your training framework and objectives.

## Quick Reference

| Mode | Output Format | Models Required | HuggingFace TRL | Training Method |
|------|--------------|-----------------|-----------------|-----------------|
| sft | instruction, output | Main only | SFTTrainer | Supervised fine-tuning |
| dpo | prompt, chosen, rejected | Main + Rejected | DPOTrainer | Direct preference optimization |
| kto | prompt, completion, label | Main + Rejected | KTOTrainer | Kahneman-Tversky optimization |
| mo-dpo | Full schema + judge scores | Main + Rejected + Judge | Custom | Multi-objective DPO |

## Configuration

Set the mode in config.toml:

```toml
[generation]
dataset_mode = "dpo"  # Options: sft, dpo, kto, mo-dpo
```

---

## SFT Mode

### Purpose

Simple instruction-output pairs for supervised fine-tuning.

### Output Format

```json
{
  "main_topic": "Fantasy Fiction",
  "sub_topic": "Dragon Lore",
  "instruction": "Write a story about dragons discovering ancient magic",
  "output": "In the mountains of Eldoria, where mist cloaked the highest peaks..."
}
```

**Note:** `main_topic` and `sub_topic` fields can be excluded by setting `include_topic_columns = false` in config.

### Configuration Example

```toml
[generation]
dataset_mode = "sft"
include_topic_columns = true  # Set false to exclude topic columns

[models.main]
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "meta/llama-3.1-70b-instruct"
temperature = 0.7
max_output_tokens = 2048
rate_limit_per_minute = 40

# models.rejected is OPTIONAL for SFT mode
# Only main model generates responses

[prompt_templates]
chosen_generation = "You are an expert writer. Write a story: {{.Prompt}}"
```

### With Optional Judge Filtering

```toml
[judge_filtering]
enabled = true
use_explanations = false  # Scores only = 40-60% token savings
min_chosen_score = 4.0    # Keep responses with avg score >= 4.0

[models.judge]
enabled = true
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "meta/llama-3.1-70b-instruct"
temperature = 0.4
max_output_tokens = 2048
```

### HuggingFace TRL Usage

```python
from datasets import load_dataset
from trl import SFTTrainer

dataset = load_dataset("json", data_files="dataset.jsonl")

trainer = SFTTrainer(
    model="your-model",
    train_dataset=dataset,
    # SFT expects instruction/output columns
)
trainer.train()
```

### Use Cases

- Basic instruction-following fine-tuning
- Domain adaptation to specific topics or styles
- Rapid prototyping (fastest mode, lowest API cost)

---

## DPO Mode

### Purpose

Standard preference pairs for Direct Preference Optimization and variants (WPO, SimPO, ORPO).

### Output Format

```json
{
  "prompt": "Write a fantasy story about dragons discovering ancient magic",
  "chosen": "In the mountains of Eldoria, where mist cloaked the highest peaks, an ancient dragon named Azura uncovered a crystalline artifact...",
  "rejected": "There was a dragon named Bob. He found some magic. It was old magic from a long time ago..."
}
```

### Configuration Example

```toml
[generation]
dataset_mode = "dpo"

[models.main]
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "meta/llama-3.1-70b-instruct"
temperature = 0.7
max_output_tokens = 2048

[models.rejected]
base_url = "http://localhost:8080/v1"  # Use local model to save costs
model_name = "phi-4-mini-instruct"
temperature = 1.0  # Higher temp for worse responses
max_output_tokens = 1024

[prompt_templates]
chosen_generation = "You are a master storyteller. Write a compelling story (400-600 words): {{.Prompt}}"
rejected_generation = "Write a simple story (200-300 words): {{.Prompt}}"
```

### Strategy for Quality Pairs

**Chosen Model Configuration:**
- Use stronger model
- Lower temperature for deterministic responses, higher for creativity (adjust based on model)
- Detailed instructions
- Higher max_output_tokens

**Rejected Model Configuration:**
- Use weaker model OR same model with different prompt
- Lower temperature for deterministic responses, higher for creativity (adjust based on model)
- Simpler instructions
- Lower max_output_tokens

### HuggingFace TRL Usage

```python
from datasets import load_dataset
from trl import DPOTrainer

dataset = load_dataset("json", data_files="dataset.jsonl")

trainer = DPOTrainer(
    model="your-model",
    train_dataset=dataset,
    # DPO expects: prompt, chosen, rejected columns
)
trainer.train()
```

### Use Cases

- Preference optimization training
- HuggingFace TRL workflows
- Relative quality comparisons between responses

---

## KTO Mode

### Purpose

Unpaired preferences with binary labels for Kahneman-Tversky Optimization.

### Output Format

Generates 2 rows per preference pair:

```json
// Row 1: Chosen response (positive example)
{
  "prompt": "Write a fantasy story about dragons discovering ancient magic",
  "completion": "In the mountains of Eldoria, where mist cloaked the highest peaks...",
  "label": true
}

// Row 2: Rejected response (negative example)
{
  "prompt": "Write a fantasy story about dragons discovering ancient magic",
  "completion": "There was a dragon named Bob. He found some magic...",
  "label": false
}
```

### Configuration Example

```toml
[generation]
dataset_mode = "kto"

# Model configuration same as DPO mode
[models.main]
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "meta/llama-3.1-70b-instruct"
temperature = 0.7

[models.rejected]
base_url = "http://localhost:8080/v1"
model_name = "phi-4-mini-instruct"
temperature = 1.0
```

### Key Differences from DPO

- **Unpaired format:** Each response is a separate row
- **Binary labels:** `true` for desirable, `false` for undesirable
- **2× rows:** Generates twice as many rows as input pairs
- **Training:** Uses Kahneman-Tversky prospect theory instead of direct preference comparison

### HuggingFace TRL Usage

```python
from datasets import load_dataset
from trl import KTOTrainer

dataset = load_dataset("json", data_files="dataset.jsonl")

trainer = KTOTrainer(
    model="your-model",
    train_dataset=dataset,
    # KTO expects: prompt, completion, label columns
)
trainer.train()
```

### Use Cases

- Training on absolute quality rather than relative preferences
- Working with binary feedback data
- Unpaired feedback scenarios

---

## MO-DPO Mode

### Purpose

Multi-objective preference optimization with detailed rubric-based judge scoring.

### Output Format

```json
{
  "main_topic": "Fantasy Fiction",
  "sub_topic": "Dragon Lore",
  "prompt": "Write a fantasy story about dragons discovering ancient magic",
  "chosen": "In the mountains of Eldoria...",
  "rejected": "There was a dragon named Bob...",

  "chosen_scores": {
    "plot_quality": {
      "score": 5,
      "reasoning": "Excellent narrative arc with clear causality and rising tension..."
    },
    "creativity": {
      "score": 4,
      "reasoning": "Fresh perspective on dragon mythology with unique magic system..."
    },
    "writing_quality": {
      "score": 5,
      "reasoning": "Polished prose with vivid imagery and strong voice..."
    }
  },

  "rejected_scores": {
    "plot_quality": {
      "score": 2,
      "reasoning": "Minimal plot development, lacks structure..."
    },
    "creativity": {
      "score": 2,
      "reasoning": "Generic treatment of common tropes..."
    },
    "writing_quality": {
      "score": 2,
      "reasoning": "Simple prose, lacks descriptive depth..."
    }
  },

  "chosen_score_total": 4.67,
  "rejected_score_total": 2.0,
  "preference_margin": 2.67
}
```

### Configuration Example

```toml
[generation]
dataset_mode = "mo-dpo"

[models.main]
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "meta/llama-3.1-70b-instruct"
temperature = 0.7

[models.rejected]
base_url = "http://localhost:8080/v1"
model_name = "phi-4-mini-instruct"
temperature = 1.0

[models.judge]
enabled = true  # REQUIRED for mo-dpo mode
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "meta/llama-3.1-70b-instruct"
temperature = 0.4  # Lower temp for consistent evaluation
max_output_tokens = 4096

[prompt_templates]
judge_rubric = '''Evaluate this story on multiple criteria (1-5 scale):

PROMPT: {{.Prompt}}
STORY: {{.StoryText}}

Rate each criterion with score and reasoning:

Return ONLY valid JSON:
{
  "plot_quality": {"score": 1-5, "reasoning": "..."},
  "creativity": {"score": 1-5, "reasoning": "..."},
  "writing_quality": {"score": 1-5, "reasoning": "..."}
}'''
```

### Training Approaches

**1. Standard DPO:**
Use `prompt`, `chosen`, `rejected` columns (ignore scores).

**2. Reward Model Training:**
Use `chosen_score_total` and `rejected_score_total` as labels.

**3. Multi-Objective RL:**
```python
for criterion in ["plot_quality", "creativity", "writing_quality"]:
    chosen_score = data["chosen_scores"][criterion]["score"]
    rejected_score = data["rejected_scores"][criterion]["score"]
    # Apply criterion-specific weights
```

**4. Preference Margin Training:**
Use `preference_margin` for weighted DPO loss.

### Use Cases

- Reward model training with per-criterion scores
- Multi-objective optimization across different quality dimensions
- Detailed quality analysis and understanding preference factors
- Curriculum learning progressing from simple to complex objectives

---

## Optional Judge Filtering

Available for SFT, DPO, KTO modes. MO-DPO always includes full judge evaluation.

### Purpose

Filter low-quality responses before writing to dataset.

### Configuration

```toml
[judge_filtering]
enabled = true
use_explanations = false     # false = scores only (40-60% token savings)
min_chosen_score = 4.0       # Keep chosen responses >= 4.0 (1.0-5.0 scale)
max_rejected_score = 3.0     # Keep rejected responses <= 3.0 (1.0-5.0 scale)

[models.judge]
enabled = true
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "meta/llama-3.1-70b-instruct"
temperature = 0.4 # Use lower temperature for deterministic responses
max_output_tokens = 2048
```

### How It Works

1. Generate chosen + rejected responses
2. Judge evaluates both (score only, no reasoning if `use_explanations = false`)
3. Filter decision:
   - Skip pair if `chosen_score < min_chosen_score`
   - Skip pair if `rejected_score > max_rejected_score`
4. Write to dataset (only pairs passing filter)

### Performance Impact

- Without explanations: 40-60% fewer tokens than full judge evaluation
- Parallel evaluation: No additional latency (runs during generation)
- Quality improvement: Removes obviously bad pairs
- Slower when sharing rate limits with main or rejected model

### When to Use Filtering

**Use filtering when:**
- API budget is limited (focus on quality over quantity)
- Training time is expensive (better data = less training)
- You need consistent baseline quality

**Skip filtering when:**
- You want maximum dataset size
- You're training on diversity over quality
- When speed matters

---

## Performance Comparison

| Mode | API Calls per Pair | Typical Time | Output Rows | Cost |
|------|-------------------|--------------|-------------|------|
| SFT | 1-2 | ~5s | 1 | $ |
| DPO | 2 | ~8s | 1 | $$ |
| KTO | 2 | ~8s | 2 | $$ |
| MO-DPO | 4 | ~20s | 1 | $$$$ |
| +Filtering | +2 | +4s | Variable | +$ |

Times based on NVIDIA NIM API with 64 workers.

---

## Mode Selection Guide

### Choose SFT if:
- You're new to LLM training
- You just need basic instruction-following
- You want maximum speed and minimum cost

### Choose DPO if:
- You want standard preference optimization
- You're using HuggingFace TRL
- You are creating clear better/worse examples
- Want to train using a DPO variant (WPO, Simpo, ORPO, etc.)

### Choose KTO if:
- You are creating binary feedback (like/dislike)
- You're training on absolute quality
- Good for base models (not instruct models)

### Choose MO-DPO if:
- You need multi-objective optimization
- You're training reward models
- You want detailed quality analysis
- Budget allows for extra API calls

---

## Switching Between Modes

Change the mode by editing config.toml:

```toml
[generation]
dataset_mode = "dpo"  # Change to: sft, dpo, kto, or mo-dpo
```

Your prompt templates and model configurations remain the same (except judge requirement for MO-DPO).

### Example Workflow

```bash
# Quick initial dataset (DPO)
# Edit config.toml: dataset_mode = "dpo", num_subtopics = 100
./bin/vellumforge2 run --config config.toml

# High-quality final dataset (MO-DPO)
# Edit config.toml: dataset_mode = "mo-dpo", num_subtopics = 1000
./bin/vellumforge2 run --config config.toml
```

---

## Configuration Best Practices

BEST PRACTICE is to use the benchmarking script against your config.toml to evaluate the optimal number of workers. These are just guidelines to give you a starting point. See [BENCHMARK_README.md](BENCHMARK_README.md).

### For SFT

```toml
[generation]
dataset_mode = "sft"
include_topic_columns = false
concurrency = 128  # Adjust based on rate limits, system resources, and API speed. Use the benchmark scripts to help

[models.main]
temperature = 0.7
max_output_tokens = 2048
```

### For DPO/KTO

```toml
[generation]
dataset_mode = "dpo"  # or "kto"
concurrency = 64

[models.main]
temperature = 0.6  # Lower for more deterministic, or higher for more creativity

[models.rejected]
temperature = 0.0  # Higher for more variance, or lower for less creativity
max_output_tokens = 1024  # Lower than main for less output
```

### For MO-DPO

```toml
[generation]
dataset_mode = "mo-dpo"
concurrency = 64

[models.judge]
enabled = true  # REQUIRED
temperature = 0.4  # Lower for consistent scoring
max_output_tokens = 16384  # Higher for detailed reasoning
```

---

## Next Steps

- Complete configuration reference: [configs/config.example.toml](configs/config.example.toml)
- Getting started guide: [GETTING_STARTED.md](GETTING_STARTED.md)
- Performance optimization: [BENCHMARK_README.md](BENCHMARK_README.md)
