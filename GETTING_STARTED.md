# Getting Started with VellumForge2

This guide will help you get VellumForge2 up and running in minutes.

## Prerequisites

- Go 1.25 or higher
- API key for an OpenAI-compatible LLM provider (e.g., NVIDIA, OpenAI, Together AI), or local server for serving an OpenAI-compatible API endpoint (llama.cpp or kobold.cpp server, etc)
- (Optional) Hugging Face account with write token for dataset uploads

## Installation

### Step 1: Install Go

If you don't have Go installed:

**Linux/macOS:**
```bash
wget https://go.dev/dl/go1.21.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

**Or use your package manager:**
```bash
# Ubuntu/Debian
sudo apt install golang-go

# macOS
brew install go
```

### Step 2: Clone and Build

```bash
# Clone the repository
cd ~/Development
git clone https://github.com/lamim/vellumforge2.git  # Or use your fork
cd vellumforge2

# Download dependencies
go mod download

# Build the binary
make build

# Verify the build
./bin/vellumforge2 --version
```

## Configuration

### Step 1: Set Up Configuration File

```bash
# Copy the example configuration
cp configs/config.example.toml config.toml

# Edit with your preferred editor (Even Better TOML VSCode extension is recommended)
nano config.toml  # or vim, code, etc.
```

**Minimal configuration changes:**

```toml
[generation]
main_topic = "Your Topic Here"  # Change this to your desired theme
num_subtopics = 2               # Start small for testing
num_prompts_per_subtopic = 2    # Start small for testing
concurrency = 4                 # Adjust based on API limits

[models.main]
base_url = "https://integrate.api.nvidia.com/v1"  # Your API endpoint
model_name = "your-model-name"                     # Your model
rate_limit_per_minute = 20                         # Your API rate limit

[models.rejected]
# Same structure as main, typically using a weaker model
```

### Step 2: Set Up API Keys

```bash
# Copy the example env file
cp configs/.env.example .env

# Edit with your API keys
nano .env
```

**Example .env:**
```bash
# For NVIDIA API
NVIDIA_API_KEY=nvapi-your-actual-key-here

# OR for OpenAI
OPENAI_API_KEY=sk-your-actual-key-here

# Optional: For Hugging Face uploads
HUGGING_FACE_TOKEN=hf_your-actual-token-here
```

## First Run

### Test with Small Dataset

```bash
# Run with verbose logging to see what's happening
./bin/vellumforge2 run --config config.toml --verbose
```

**What happens:**
1. Creates a timestamped session directory (e.g., `session_2025-10-27T15-30-00/`)
2. Generates subtopics from your main topic with **automatic retry** if count falls short
3. Generates prompts for each subtopic
4. Creates preference pairs (chosen/rejected responses)
5. Saves everything to `output/session_*/dataset.jsonl`

**New Feature**: If the LLM returns fewer subtopics than requested (e.g., 271 instead of 344), VellumForge2 automatically:
- Detects the gap
- Makes up to 5 retry attempts to generate missing items
- Filters duplicates
- Shows detailed progress in logs

### Check the Results

```bash
# List session directories
ls -la session_*

# View the dataset
cat output/session_*/dataset.jsonl | jq .  # Pretty print JSON

# Check the logs
less session_*/session.log
```

## Common Use Cases

### 1. Generate Story Dataset

```toml
[generation]
main_topic = "Science Fiction Space Opera"
num_subtopics = 5
num_prompts_per_subtopic = 10

[models.main]
model_name = "meta/llama-3.1-70b-instruct"
temperature = 0.7

[models.rejected]
model_name = "meta/llama-3.1-8b-instruct"
temperature = 0.9  # Higher temp for weaker responses
```

### 2. Technical Writing Dataset

```toml
[generation]
main_topic = "Software Development Best Practices"
num_subtopics = 8
num_prompts_per_subtopic = 15

[models.main]
model_name = "gpt-4o"
temperature = 0.5  # Lower for technical accuracy

[models.rejected]
model_name = "gpt-3.5-turbo"
temperature = 0.7
```

### 3. With Story Generation and LLM-as-a-Judge

```toml
[models.judge]
enabled = true
model_name = "gpt-4o"  # Use a strong model for evaluation
temperature = 0.2      # Low temp for consistent scoring

[prompt_templates]
# Story generation templates (REQUIRED)
chosen_generation = '''You are a talented fantasy writer.
Write a compelling story (400-600 words) for this prompt:

{{.Prompt}}

Include vivid descriptions, engaging characters, and strong narrative voice.'''

rejected_generation = '''Write a fantasy story for this prompt:

{{.Prompt}}

Write 300-400 words.'''

# Judge evaluation template
judge_rubric = '''
Evaluate the story based on:
1. Plot quality
2. Character development  
3. Writing style
4. Creativity

Return JSON with scores 1-5 for each criterion.
'''
```

### 4. Upload to Hugging Face

```bash
./bin/vellumforge2 run \
  --config config.toml \
  --upload-to-hf \
  --hf-repo-id your-username/your-dataset-name
```

## Troubleshooting

### Problem: "API rate limit exceeded"

**Solution:** Reduce concurrency and rate limits
```toml
[generation]
concurrency = 2  # Lower parallelism

[models.main]
rate_limit_per_minute = 10  # Lower limit
```

### Problem: "Failed to parse JSON response"

**Causes:**
- Model doesn't follow instructions to return JSON
- Temperature too high causing erratic output

**Solutions:**
1. Lower temperature: `temperature = 0.3`
2. Update prompt template to be more explicit about JSON format
3. Try a different model

**Note**: VellumForge2 has built-in JSON extraction and auto-retry, so most parse errors are handled automatically.

### Problem: "Getting fewer subtopics than requested"

**Example**: Requested 344 subtopics, only got 271

**Solution**: VellumForge2 automatically handles this with **smart over-generation**:
- Requests 115% of target count initially
- Makes ONE retry attempt if still short
- Filters duplicates automatically (case-insensitive)
- Achieves 95%+ accuracy vs 79% with single-shot
- Passes exclusion list to avoid repeats
- Logs detailed progress

**What you'll see in logs**:
```
INFO  Initial subtopic generation attempt=1 target=344 remaining=344
INFO  Subtopic generation attempt complete received=271 unique_added=271
WARN  Regenerating missing subtopics attempt=2 remaining=73
INFO  Subtopic generation attempt complete received=62 unique_added=60 total_unique=331
```

**Expected Success Rate**: 95-100% for counts up to 500

### Problem: "Out of memory"

**Solution:** Reduce concurrency
```toml
[generation]
concurrency = 2  # Use fewer workers
```

### Problem: "Connection timeout"

**Solutions:**
1. Check your internet connection
2. Verify API endpoint is accessible
3. Increase timeout in code if needed

## Best Practices

### 1. Start Small
Begin with 2 subtopics and 2 prompts per subtopic to verify everything works before scaling up.

### 2. Monitor API Costs
Track API usage in your provider's dashboard, especially when generating large datasets.

### 3. Use Version Control
Keep your `config.toml` in git (but not `.env`!) to track dataset generation parameters.

### 4. Backup Sessions
Important sessions should be archived:
```bash
tar -czf important-session.tar.gz session_2025-10-27T15-30-00/
```

### 5. Test Model Combinations
Experiment with different model pairs for main/rejected to find optimal preference signals:
- Strong vs Weak model
- Same model with different temperatures
- Instruct vs Base model

### 6. Large Subtopic Counts
When generating large numbers of subtopics (100+):
- Iterative regeneration will automatically retry until target is reached
- Watch logs for duplicate filtering counts
- Typical success rate is 95-100% for counts up to 500
- If model exhausts creativity, it will stop with 97%+ of target

## Advanced Configuration

### Smart Over-Generation Strategy

VellumForge2 uses an intelligent over-generation strategy to reliably achieve target counts:

**How it works:**
1. **Over-generate**: Requests 115% of target count (e.g., 395 for 344 target)
2. **Deduplicate**: Removes duplicate subtopics (case-insensitive)
3. **Trim or Retry**: 
   - If count ≥ target → trim to exact count
   - If count < target → make ONE retry for the difference

**Benefits:**
- ✅ 95%+ count accuracy (vs 79% with single-shot)
- ✅ Fewer API calls (1-2 requests vs 3-5 with iterative)
- ✅ Robust JSON validation prevents parse failures
- ✅ Graceful degradation (returns partial results if retry fails)

**Logging output:**
```
INFO  Generating subtopics with over-generation strategy target=344 requesting=395 buffer_percent=15
INFO  Initial subtopic generation complete requested=395 received=390 unique=385 duplicates_filtered=5
INFO  Target count achieved final_count=344 excess_trimmed=41
```

### Custom Prompt Templates

```toml
[prompt_templates]
# Subtopic generation with smart over-generation and retry support
subtopic_generation = '''
You are an expert in {{.MainTopic}}.
Generate {{.NumSubtopics}} specific subtopics.

{{if .IsRetry}}NOTE: Avoid these already generated: {{.ExcludeSubtopics}}
{{end}}

Return ONLY a JSON array: ["topic1", "topic2", ...]
'''

prompt_generation = '''
Create {{.NumPrompts}} diverse prompts about: {{.SubTopic}}
Each prompt should be 2-3 sentences.
Return ONLY a JSON array: ["prompt1", "prompt2", ...]
'''
```

**Template Variables**:

- **Subtopic Generation:**
  - `{{.MainTopic}}` - Your main theme
  - `{{.NumSubtopics}}` - Count to generate (auto-adjusted for over-generation and retry)
  - `{{.IsRetry}}` - Boolean, true on retry attempts (optional)
  - `{{.ExcludeSubtopics}}` - Comma-separated list of already generated subtopics (optional, only on retry)

- **Prompt Generation:**
  - `{{.SubTopic}}` - Current subtopic
  - `{{.NumPrompts}}` - Prompts to generate
  - `{{.MainTopic}}` - Main topic (also available)

- **Story Generation (chosen_generation, rejected_generation):**
  - `{{.Prompt}}` - The writing prompt
  - `{{.MainTopic}}` - Main topic
  - `{{.SubTopic}}` - Current subtopic

- **Judge Evaluation:**
  - `{{.Prompt}}` - Original writing prompt
  - `{{.StoryText}}` - Story to evaluate

### Multiple API Providers

Different models can use different providers:

```toml
[models.main]
base_url = "https://api.openai.com/v1"
model_name = "gpt-4o"

[models.rejected]
base_url = "https://integrate.api.nvidia.com/v1"
model_name = "meta/llama-3.1-8b-instruct"

[models.judge]
base_url = "https://api.anthropic.com/v1"
model_name = "claude-3-opus"
```

### Performance Tuning

For maximum throughput (with sufficient API quota):

```toml
[generation]
concurrency = 16  # High parallelism

[models.main]
rate_limit_per_minute = 100  # If your plan supports it

[models.rejected]
rate_limit_per_minute = 100
```

For minimal API usage:

```toml
[generation]
concurrency = 1  # Sequential processing

[models.main]
rate_limit_per_minute = 5  # Minimal rate
```

## Next Steps

1. **Scale Up**: Once you've verified the basic setup works, increase `num_subtopics` and `num_prompts_per_subtopic`

2. **Enable Judge**: Add LLM-as-a-Judge evaluation to get quality scores

3. **Share**: Upload your dataset to Hugging Face to contribute to the community

4. **Train**: Use your generated dataset with a DPO training framework like:
   - [TRL](https://github.com/huggingface/trl)
   - [Alignment Handbook](https://github.com/huggingface/alignment-handbook)
   - [Axolotl](https://github.com/OpenAccess-AI-Collective/axolotl)

## Getting Help

- Read the [full README](README.md)
- Report bugs in [GitHub Issues](https://github.com/lamim/vellumforge2/issues)
- Ask questions in [Discussions](https://github.com/lamim/vellumforge2/discussions)
- Check [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md) for technical details

## Example: Complete First Run

```bash
# 1. Setup
cd vellumforge2
cp configs/config.example.toml config.toml
cp configs/.env.example .env

# 2. Edit config.toml
# - Set main_topic to "Fantasy Adventures"
# - Set num_subtopics to 2
# - Set num_prompts_per_subtopic to 2
# - Configure your API endpoint and model

# 3. Edit .env with your API key
echo "NVIDIA_API_KEY=nvapi-your-key" > .env

# 4. Build
make build

# 5. Run
./bin/vellumforge2 run --config config.toml --verbose

# 6. Check results
ls session_*/
cat output/session_*/dataset.jsonl | jq .

# Success! You've generated your first DPO dataset!
```

Congratulations! You're now ready to generate synthetic DPO datasets at scale. 🎉
