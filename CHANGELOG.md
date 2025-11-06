# Changelog

All notable changes to VellumForge2 from v1.0.0 to present.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.5.3] - 2025-11-06

### Added
- **Standalone Upload Test Tool**: New `cmd/upload_test` utility for testing HuggingFace uploads
  - Standalone binary for debugging upload issues
  - Direct upload testing without full generation pipeline
  - Usage: `upload_test <repo-id> <session-dir>`

### Changed
- **Complete LFS Upload Refactor**: Switched to proper Git LFS Batch API specification
  - Now uses correct LFS endpoint: `{repo}.git/info/lfs/objects/batch`
  - Implements Git LFS Batch API v1 with proper request/response structures
  - Multipart upload support for large files (>5MB)
    - Automatic chunking based on server-specified chunk size
    - Part-by-part upload with ETag collection
    - Completion request with all part ETags
  - Proper handling of files that already exist on server (nil actions response)
  - Correct HTTP headers: `Content-Type: application/vnd.git-lfs+json`
  - Upload retry logic preserved with new API structure
- **Removed Dataset Metadata Header**: Eliminated metadata header in dataset.jsonl
  - Previous workaround for LFS cache collisions no longer needed
  - Cleaner dataset files without leading metadata record
  - HuggingFace LFS now handles file uniqueness correctly with proper API

### Fixed
- LFS uploads now work reliably with HuggingFace's actual LFS implementation
- Large file uploads (>5MB) now supported via multipart protocol
- Files that exist on server no longer cause spurious warnings
- Upload failures due to incorrect LFS API usage eliminated

---

## [1.5.2] - 2025-11-06

### Changed
- **Benchmark Configuration**: Refined rate limiting and benchmark parameters
  - Adjusted default concurrency from 64 to 48 workers in examples
  - Updated NVIDIA NIM rate limit examples to more conservative values
  - Added cooldown period between benchmark runs (10 seconds)
  - Benchmark script now saves configuration snapshot for each run
  - Improved summary formatting in benchmark results

### Fixed
- Configuration examples now use realistic rate limits for NVIDIA NIM endpoints
- Benchmark runs no longer interfere with each other due to cooldown period
- Better tracking of configuration used for each benchmark session

---

## [1.5.1] - 2025-11-06

### Changed
- Version bump to track additional improvements added to 1.5.0 release
- Documentation date updated to reflect recent changes

---

## [1.5.0] - 2025-11-05

### Added
- **Multiple Dataset Modes**: Support for four different training formats
  - `sft` - Simple instruction-output pairs for supervised fine-tuning (1 model required)
  - `dpo` - Standard preference pairs: prompt, chosen, rejected (2 models, HuggingFace TRL compatible)
  - `kto` - Unpaired preferences with binary labels (2 models, HuggingFace TRL KTOTrainer compatible)
  - `mo-dpo` - Full multi-objective DPO with detailed judge scoring (3 models)
  - Configure via `dataset_mode` in `[generation]` section
  - Separate write methods for each format in dataset writer
- **Judge Filtering**: Optional quality control for SFT, DPO, KTO modes
  - Filter responses before dataset generation based on quality scores
  - `judge_filtering.enabled` configuration option
  - `use_explanations = false` for scores only (40-60% token savings vs full evaluation)
  - `min_chosen_score` and `max_rejected_score` thresholds
  - Async parallel evaluation during generation (no additional latency)
  - MO-DPO mode always includes full judge evaluation
- **SFT Mode Topic Columns**: Configurable `include_topic_columns` option
  - Controls whether main_topic/sub_topic columns are included in SFT output
  - Default: true (includes columns)
  - Set to false for minimal instruction-output format
- **Prompt Generation Robustness**: Enhanced reliability for prompt generation phase
  - Configurable retry logic: `subtopic_retry_attempts` and `prompt_retry_attempts`
  - Over-generation with deduplication: `subtopic_over_generation_multiplier` and `prompt_over_generation_multiplier`
  - Smart handling of under/over-generation cases
  - Detailed logging of retry attempts and exclusion list usage
  - Configurable `prompt_parallel_requests` for throughput optimization
- **Optional System Prompts**: Reduce model refusals and improve response quality
  - `system_prompt` configuration option for each model
  - Helps models understand their role and reduces safety over-filtering
  - Particularly useful for creative writing tasks
- **Security Hardening**: Multiple critical security and stability fixes
  - Fixed WaitGroup race conditions causing premature goroutine exits
  - Proper context cancellation handling throughout pipeline
  - Enhanced input validation for configuration parameters
  - Defensive programming patterns for nil pointer checks
  - Comprehensive linting fixes (P0 and P1 priority)

### Changed
- **Configuration Structure**: Enhanced config validation and organization
  - Added `JudgeFilteringConfig` struct for filtering settings
  - Added `DatasetMode` type with validation
  - Improved error messages for invalid mode configurations
  - Model requirements validated based on selected mode
  - New robustness configuration fields for retry and over-generation behavior
  - Increased default `max_output_tokens` and `context_size` for better model performance
- **Worker Logic**: Refactored preference pair generation for mode support
  - Separate handling for SFT (no rejected), DPO/KTO (rejected required), MO-DPO (judge required)
  - Judge filtering integrated into worker pipeline
  - Records filtered before writing based on score thresholds
  - Improved context cancellation detection for graceful shutdown
- **Judge Evaluation**: Enhanced judge functionality
  - Support for score-only evaluation (no explanations)
  - Compatible with filtering mode
  - Maintains full evaluation for MO-DPO mode
  - Better logging for retry attempts and temperature configuration
- **Dataset Writer**: Multi-format output support
  - `writeRecord()` dispatches to mode-specific methods
  - `writeSFTRecord()` for SFT format
  - `writeDPORecord()` for DPO format
  - `writeKTORecord()` for KTO format (2 rows per pair)
  - `writeMODPORecord()` for MO-DPO format (unchanged)
- **Rate Limiting**: Improved effective rate limit calculations
  - Enhanced burst capacity calculations in `GetEffectiveRateLimit()`
  - Better handling of prompt generation worker count relative to rate limits
  - More accurate rate limiting to prevent 429 errors

### Documentation
- **Complete Documentation Consolidation**: Restructured all documentation
  - Consolidated README.md with comprehensive feature overview
  - Rewrote GETTING_STARTED.md with step-by-step tutorial and troubleshooting
  - Rewrote DATASET_MODES.md with detailed specifications for all four modes
  - Combined all config examples into single comprehensive configs/config.example.toml
  - Removed split documentation structure (docs/ directory)
  - Removed AI-style writing and emojis throughout
  - All information verified against codebase
  - Professional, technical writing style
- **Configuration Reference**: Single config.example.toml with complete inline documentation
  - All options documented with defaults and recommendations
  - Mode-specific configuration sections
  - Real-world values and examples
- **Archived Documentation**: Moved README_OLD.md to REPORTS/README_OLD_v1.4.10.md
- **Created Reports**:
  - REPORTS/DOCUMENTATION_UPDATE_2025-11-05.md - Complete consolidation summary
  - REPORTS/DOCUMENTATION_CONSOLIDATION_2025-11-05.md - Initial analysis (superseded)

### Fixed
- **Critical Stability Issues**:
  - Fixed WaitGroup race conditions that could cause premature worker exits
  - Proper handling of context cancellation during generation
  - Eliminated goroutine leaks through proper cleanup patterns
- **Security Vulnerabilities**:
  - Enhanced input validation prevents potential injection attacks
  - Defensive nil pointer checks throughout codebase
  - Proper error handling to prevent information leakage
- **Configuration and Validation**:
  - Configuration validation now checks model requirements based on dataset mode
  - Judge model validation only required when judge filtering enabled or mo-dpo mode
  - Better error messages for misconfigured retry and over-generation parameters
- **Performance and Reliability**:
  - Context size adjustments improve generation stability
  - Temperature configuration properly applied in ChatCompletion methods
  - Benchmark scripts use Python for duration calculation (more precise than bc)

---

## [1.4.11] - 2025-10-31

### Fixed
- **Critical Performance Regression**: Eliminated 80% performance regression in judge evaluation
  - **Root Cause**: API retry loop was making up to 3 API calls per parse failure instead of trying different parse strategies locally
  - **Solution**: Refactored to separate API retries from JSON parse retries
    - Single API call per judge evaluation (no retry loop)
    - Multi-strategy progressive JSON parsing on same response (4 strategies, <4ms total)
  - **Performance Improvement**:
    - Average job time: 9.1s → 5.2s (**-42.6%** faster)
    - Total session time: 292s → 167s (**-42.7%** faster)  
    - API cost reduction: **-60%** for judge evaluations
  - **Reliability**: Parse success rate maintained at 100% with Strategy 1 (standard extraction + sanitization)
  - **Implementation Details**:
    - Added `parseJudgeResponseWithRetries()`: Progressive 4-strategy parser
      - Strategy 1 (standard): ExtractJSON → SanitizeJSON → Unmarshal
      - Strategy 2 (aggressive): ExtractJSON → RepairJSON → Unmarshal
      - Strategy 3 (multi-pass): Extract → Repair → Sanitize → Repair → Unmarshal
      - Strategy 4 (partial recovery): Lenient decoder for incomplete JSON
    - Removed obsolete `maxParseRetries`, `parseRetryDelay`, and `isJSONParseError()` function
    - Fixed linter warnings (ineffectual assignments)
- **JSON Parse Resilience**: Backup strategies available for edge cases without API overhead

### Changed
- Judge evaluation now makes exactly 1 API call per response (previously up to 3 on parse failure)
- Parse errors now trigger local repair strategies instead of API retries
- Removed artificial delays between parse attempts (was 1 second per retry)

### Performance
- **124.5 seconds saved** per 32-job session (from 291.9s to 167.4s)
- Zero API retry overhead for parse failures
- All parse attempts succeed with Strategy 1 (100% fast-path success)
- Time saved scales linearly with dataset size

### Documentation
- Created `REPORTS/PERFORMANCE_REGRESSION_ANALYSIS_2025-10-31.md`: Root cause analysis
- Created `REPORTS/PERFORMANCE_FIX_IMPLEMENTATION_2025-10-31.md`: Implementation details
- Created `REPORTS/POST_FIX_SESSION_ANALYSIS_2025-10-31.md`: Real-world validation results
- Created `REPORTS/SESSION_COMPARISON_CLARIFICATION_2025-10-31.md`: Performance comparison methodology

---

## [1.4.4] - 2025-10-30

### Added
- **Provider-Based Global Rate Limiting**: Major feature to prevent rate limit errors across multiple models sharing the same provider
  - `provider_rate_limits` configuration: Set global RPM limits per provider (nvidia, openai, anthropic, together, etc.)
  - Automatic provider detection from base URLs
  - Provider-level rate limiters override individual model limits
  - Prevents burst overages when multiple models share the same API endpoint
- **Configurable Burst Capacity**: Fine-tune rate limiter burst behavior
  - `provider_burst_percent` configuration (1-50%, default: 15%)
  - Higher burst (20-25%) improves throughput with 64+ workers
  - Lower burst (10-12%) reduces rate limit errors
  - Model-level limiters still use 20% burst (unchanged)
- **Performance Logging**: Added detailed API request and job processing metrics
  - `rate_limit_wait_ms`: Time spent waiting for rate limiter token
  - `api_duration_ms`: Actual LLM API call duration  
  - `total_ms`: End-to-end request time
  - Per-job breakdown: `chosen_ms`, `rejected_ms`, `judge_ms`, `total_ms`
- **Asynchronous Judge Evaluation**: Judge runs in background, non-blocking
  - Jobs complete immediately after chosen/rejected generation
  - Judge evaluation happens asynchronously
  - Reduces perceived job duration by ~30-50%
- **Benchmarking Scripts**: Comprehensive tools for performance testing
  - `scripts/benchmark_workers.sh`: Test multiple worker counts (4, 8, 16, 32, 64, 96, 128, 256)
  - `scripts/quick_benchmark.sh`: Quick 3-point benchmark
  - Automatic results aggregation with CSV export
  - Performance metrics: throughput, avg job time, rate wait, blocking %
- **BENCHMARK_README.md**: Complete guide for running performance tests

### Changed
- **Concurrency Recommendations Updated**: Based on actual benchmark data
  - Old recommendation: 4-16 workers
  - New recommendation: **64-256 workers** for maximum throughput
  - Benchmark results (32 jobs, NVIDIA NIM + local Phi-4):
    - 16 workers: 7.06/min | 32 workers: 7.57/min | 64 workers: 11.91/min
    - 96 workers: 14.79/min | **256 workers: 17.60/min** (2.5x faster than old recommendation)
- Worker pool scales much better with provider rate limiting
- Updated all configuration examples with realistic worker counts
- Enhanced logging to show provider rate limit configuration at startup

### Fixed
- Multiple models using same provider no longer create duplicate rate limiters
- Rate limit errors reduced when using shared provider endpoints (main + judge on same API)
- Unnecessary nil checks removed from map length checks (Go best practices)

### Performance
- **2.5x throughput improvement** with high worker counts (256 workers vs 16 workers)
- Provider rate limiting prevents 429 errors while maintaining high throughput
- Async judge evaluation reduces job completion time perception
- Better utilization of available API quota with provider-level coordination

### Documentation
- Updated concurrency recommendations in README.md, GETTING_STARTED.md, config.toml, config.example.toml
- Added provider rate limiting configuration examples
- Added burst capacity tuning guidelines
- Added benchmark data to support higher worker count recommendations

---

## [1.3.7] - 2025-10-29

### Added
- **Retry Logic for HuggingFace Uploads**: Implemented exponential backoff retry mechanism for LFS preupload and file upload operations
  - `PreuploadLFSWithRetry()`: 3 attempts with 2s, 4s, 8s delays
  - `UploadLFSFileWithRetry()`: 3 attempts with exponential backoff
  - Configurable maximum retries via `MaxRetries` constant
- **Enhanced HTTP Timeout Configuration**: Separated timeout concerns for different operations
  - DefaultTimeout: 300s (general operations)
  - PreuploadTimeout: 300s (LFS preupload requests)
  - LFSUploadTimeout: 600s (LFS file uploads)
  - CommitTimeout: 300s (commit operations)
- **Improved JSON Sanitization**: Enhanced `SanitizeJSON()` to handle single-quoted property values
  - Automatically converts `"key": 'value'` to `"key": "value"`
  - Properly escapes double quotes within single-quoted strings
  - Fixes issues with judge responses from kimi-k2-instruct-0905
- **Other**
  - Automatic `.gitattributes` generation in HuggingFace uploads to prevent JSONL files from being stored in Git LFS with `-text` flag
  - Comprehensive documentation on newline encoding for training datasets
  - Analysis tools for dataset schema validation

### Changed
- Refactored `Uploader` struct to use four separate HTTP clients for different operation types
- Increased default HTTP timeout from 120s to 300-600s depending on operation type
- `Upload()` method now uses retry-enabled functions for LFS operations
- HuggingFace upload module now automatically creates proper `.gitattributes` file during uploads
- `.gitattributes` explicitly excludes `*.jsonl` files from LFS to ensure proper text rendering in dataset viewer

### Fixed
- **Critical Dataset Schema Bug**: Fixed type inconsistency in `calculateAverageScore()` function
  - Changed `return 0` to `return 0.0` to ensure float64 type consistency
  - Prevents PyArrow schema validation errors in HuggingFace dataset viewer
  - Affected 16% of dataset records (166/1038) in previous version
- HuggingFace upload failures due to HTTP timeouts for large files (18MB+)
- Network resilience: transient failures no longer cause entire upload to fail
- JSON parsing errors from judge model responses with single-quoted strings
- HuggingFace dataset viewer now properly renders newlines in JSONL files instead of showing literal `\n\n`

### Performance
- Expected upload success rate improvement: 0% → 95%+
- Expected generation success rate improvement: 99.71% → 99.9%+
- Reduced upload failures from timeout issues

---

## [1.3.4] - 2025-10-28

### Added
- **Configurable Retry Logic in API Client**: Added `maxRetries` configuration option
  - Configurable per model in `config.toml`
  - Exponential backoff for API request retries
  - Improved resilience for transient network failures

### Changed
- API client now supports model-specific retry configurations
- Enhanced error handling for API requests with configurable retry attempts

---

## [1.3.2] - 2025-10-28

### Fixed
- Updated VERSION to 1.3.2 in Makefile
- Improved JSON configuration comments for clarity
- Minor code formatting improvements

---

## [1.3.1] - 2025-10-28

### Fixed
- Updated VERSION to 1.3.1 in Makefile
- Code formatting improvements across various files
- Minor consistency fixes

---

## [1.3.0] - 2025-10-28

### Added
- **Checkpoint and Resume Functionality**: Major feature for long-running generation sessions
  - Automatic checkpoint saving at configurable intervals
  - Resume incomplete sessions from last checkpoint
  - CLI commands: `checkpoint list`, `checkpoint inspect`, `checkpoint resume`
  - Async I/O support for non-blocking checkpoint writes
  - Checkpoint validation to ensure compatibility with current configuration
- **Structured JSON Output Support**: Enhanced JSON handling for better LLM response parsing
  - Support for structured JSON in API responses
  - Improved extraction of JSON from markdown code blocks
  - Better handling of nested JSON structures
- **JSON Repair Function**: `RepairJSON()` utility to fix common JSON issues
  - Handles malformed JSON with unescaped characters
  - Fixes double closing brackets
  - Repairs incomplete JSON structures
  - Comprehensive test coverage
- **Subtopic Chunking**: Improved subtopic generation with better chunking strategies
- **Template Retry Support**: Added `IsRetry` flag to template data for retry-aware prompts

### Changed
- Session manager now supports resuming from existing directories
- Dataset writer can append to existing files when resuming
- Orchestrator enhanced with phase completion checks and job filtering
- Configuration extended with checkpoint-related options
- Rate limiter enhanced with logging for rate changes
- Backoff cap implemented for chat completion retries

### Fixed
- Improved handling of malformed JSON responses from LLMs
- Better error recovery in generation pipeline
- Enhanced validation of configuration parameters

### Documentation
- Created `CHECKPOINT_IMPLEMENTATION.md`: Complete guide on checkpoint functionality

---

## [1.2.0] - 2025-10-28

### Added
- **Smart Over-Generation Strategy**: Intelligent subtopic generation with configurable buffer
  - Generates extra subtopics to account for deduplication
  - Configurable over-generation multiplier
  - Automatic deduplication with smart filtering
  - Maximum exclusion list size limiting
- **Enhanced JSON Validation**: Improved validation and repair capabilities
  - Better handling of edge cases in JSON parsing
  - Stricter validation rules
  - Automatic correction of common LLM JSON mistakes
- **Template Caching**: Implemented template caching with concurrency support
  - Thread-safe cache implementation
  - Significant performance improvement for repeated template usage
  - Comprehensive test coverage for caching behavior
- **Configuration Validation**: Added extensive configuration validation tests
  - Ensures limits are enforced correctly
  - Validates over-generation buffer settings
  - Tests for exclusion list truncation logic

### Changed
- Refactored subtopic generation to use constant for over-generation multiplier
- Extracted magic numbers to named constants throughout codebase
- Simplified rate limiter locking mechanism
- Replaced manual string contains checks with stdlib functions
- Centralized JSON utilities for better maintainability

### Fixed
- Template injection vulnerabilities through validation
- Enhanced error logging for deferred close calls
- Improved string handling in validation utilities
- Better error recovery in subtopic generation

### Documentation
- Created `REPORTS/AUDIT_IMPROVEMENTS.md`: Code audit findings and improvements
- Created `REPORTS/CODE_REVIEW_FIXES.md`: Fixes from code review
- Updated configuration documentation with new fields
- Added available template variables to all prompt templates

---

## [1.1.0] - 2025-10-28

### Added
- **Iterative Regeneration**: Ensures reliable subtopic count generation
  - Retries subtopic generation until exact count is reached
  - Configurable maximum retry attempts
  - Smart handling of over/under generation
  - Detailed logging of retry attempts
- **Comprehensive JSON Sanitization**: Handles common LLM JSON output issues
  - Removes markdown code blocks (```json, ```javascript)
  - Handles double closing brackets
  - Fixes unescaped newlines in JSON strings
  - Repairs malformed quote escaping
  - Extensive test coverage

### Changed
- Updated configuration for subtopic and prompt generation
- Refined rate limits for better API usage
- Enhanced story generation templates
- Improved documentation structure

### Fixed
- Malformed JSON responses from LLM APIs no longer crash generation
- Subtopic generation now reliably produces exact requested count
- Better handling of edge cases in JSON parsing

### Documentation
- Created `REPORTS/ITERATIVE_REGENERATION_SOLUTION.md`
- Created `REPORTS/JSON_SANITIZATION_FIX.md`
- Updated README with iterative regeneration feature details
- Added complete template examples to documentation

---

## [1.0.2] - 2025-10-27

### Changed
- Refactored codebase to remove redundant and duplicate code
- Improved code organization and maintainability
- Cleaned up unused functions and variables

### Documentation
- Created `REPORTS/CLEANUP_SUMMARY-2.md`

---

## [1.0.1] - 2025-10-27

### Changed
- Updated section titles for clarity in prompt and response generation templates
- Improved template readability
- Enhanced template documentation

### Documentation
- Updated README with complete template examples
- Improved Getting Started guide

---

## [1.0.0] - 2025-10-27

Initial stable release of VellumForge2.

### Added
- **Core Generation Pipeline**:
  - Multi-phase generation: Topics → Subtopics → Prompts → Stories
  - Concurrent generation with configurable worker pools
  - LLM-as-a-Judge evaluation with customizable rubrics
  - DPO dataset output in JSONL format
- **Configuration System**:
  - TOML-based configuration
  - Multiple model support with independent configurations
  - Customizable prompt templates
  - Rate limiting per model
- **API Integration**:
  - OpenAI-compatible API support
  - Custom API endpoint configuration
  - Retry logic with exponential backoff
  - Rate limiting with burst support
- **Output Management**:
  - Session-based output directories
  - Comprehensive logging
  - Checkpoint files for resume capability (basic)
  - Configuration backup in session directories
- **HuggingFace Hub Integration**:
  - Direct upload to HuggingFace datasets
  - LFS support for large files
  - Automatic repository creation
- **CLI Interface**:
  - `run` command for generation
  - `--upload-to-hf` flag for automatic upload
  - Verbose logging option
- **Testing & CI/CD**:
  - Comprehensive test suite
  - GitHub Actions CI workflow
  - Automated releases
  - golangci-lint integration

### Documentation
- Complete README with usage examples
- GETTING_STARTED.md for new users
- Template documentation
- Configuration examples
- API integration guides

---

## Version History Summary

| Version | Date | Key Features |
|---------|------|--------------|
| **1.5.3** | 2025-11-06 | **Complete LFS refactor**: Git LFS Batch API, multipart uploads, upload test tool |
| **1.5.2** | 2025-11-06 | Refined benchmark configuration, rate limit tuning, cooldown periods |
| **1.5.1** | 2025-11-06 | Version tracking for 1.5.0 improvements |
| **1.5.0** | 2025-11-05 | **Major release**: Multiple dataset modes (SFT/DPO/KTO/MO-DPO), judge filtering, robustness, security |
| **1.4.11** | 2025-10-31 | **Critical performance fix**: Eliminated 80% regression, 42% faster judge evaluation |
| **1.4.4** | 2025-10-30 | Provider rate limiting, configurable burst, async judge, benchmarking |
| **1.3.7** | 2025-10-29 | Retry logic, timeout optimization, JSON sanitization, schema fix |
| **1.3.4** | 2025-10-28 | Configurable retries |
| **1.3.2** | 2025-10-28 | Version bump, minor fixes |
| **1.3.1** | 2025-10-28 | Code formatting |
| **1.3.0** | 2025-10-28 | Checkpoint/resume, JSON repair, structured output |
| **1.2.0** | 2025-10-28 | Smart over-generation, template caching, validation |
| **1.1.0** | 2025-10-28 | Iterative regeneration, JSON sanitization |
| **1.0.2** | 2025-10-27 | Code cleanup |
| **1.0.1** | 2025-10-27 | Template improvements |
| **1.0.0** | 2025-10-27 | Initial stable release |

---

## Migration Guide

### From 1.5.2 to 1.5.3
- No configuration changes required
- LFS uploads now use proper Git LFS Batch API (transparent to users)
- Dataset files no longer include metadata header (cleaner format)
- New `upload_test` utility available for debugging uploads

### From 1.5.1 to 1.5.2
- No breaking changes
- Consider adjusting worker counts based on new benchmark guidance (48-64 recommended)
- Review rate limit configurations if using NVIDIA NIM endpoints

### From 1.5.0 to 1.5.1
- No changes required, version bump only

### From 1.4.x to 1.5.0
- **REQUIRED**: Add `dataset_mode` to `[generation]` section in config:
  ```toml
  [generation]
  dataset_mode = "mo-dpo"  # or "sft", "dpo", "kto"
  ```
- **Optional**: Add judge filtering configuration:
  ```toml
  [judge_filtering]
  enabled = true
  use_explanations = false
  min_chosen_score = 7.0
  max_rejected_score = 5.0
  ```
- **Optional**: Add robustness configuration for better reliability:
  ```toml
  [generation]
  subtopic_retry_attempts = 3
  prompt_retry_attempts = 3
  subtopic_over_generation_multiplier = 1.15
  prompt_over_generation_multiplier = 1.15
  prompt_parallel_requests = 5
  ```
- **Optional**: Add system prompts to model configurations:
  ```toml
  [[models]]
  role = "main"
  system_prompt = "You are a creative writing assistant..."
  ```
- Review model requirements based on selected dataset mode:
  - SFT: 1 model (main)
  - DPO/KTO: 2 models (main + rejected)
  - MO-DPO: 3 models (main + rejected + judge)

### From 1.3.x to 1.4.0
- No breaking changes
- HuggingFace uploads now automatically include proper `.gitattributes`
- Existing repositories may need manual fix (use `fix_dataset_lfs.py`)

### From 1.2.x to 1.3.0
- Add checkpoint configuration to `config.toml`:
  ```toml
  enable_checkpoints = true
  checkpoint_interval = 60  # seconds
  ```
- New CLI commands available: `checkpoint list`, `checkpoint inspect`, `checkpoint resume`

### From 1.1.x to 1.2.0
- Add over-generation configuration to `config.toml`:
  ```toml
  [generation]
  over_generation_buffer = 1.2
  max_exclusion_list_size = 10000
  ```

### From 1.0.x to 1.1.0
- No breaking changes
- JSON handling is now more robust automatically
- Subtopic generation is more reliable

---

## Development Statistics

### Code Metrics (v1.0.0 to v1.5.3)
- **Total Commits**: 50+ since v1.0.0
- **Files Changed**: 80+ files
- **Lines Added**: ~8000+
- **Lines Removed**: ~2000+
- **Test Coverage**: Significantly improved with comprehensive validation
- **Major Releases**: 6 feature releases (1.0.0, 1.1.0, 1.2.0, 1.3.0, 1.4.4, 1.5.0)

### Key Improvements
- **Reliability**: 99.9%+ upload success with proper LFS API implementation
- **Data Quality**: 99.9%+ generation success with robustness features
- **Security**: Critical P0/P1 fixes for race conditions and input validation
- **Flexibility**: 4 dataset formats supporting different training approaches
- **Performance**: 2.5x throughput with optimized rate limiting and async judge
- **Maintainability**: Major code cleanup, refactoring, and documentation consolidation

---

## Contributors

- **lemon07r** - Main developer and maintainer

---

## Links

- **Repository**: https://github.com/lemon07r/vellumforge2
- **Issues**: https://github.com/lemon07r/vellumforge2/issues
- **Releases**: https://github.com/lemon07r/vellumforge2/releases
- **Example Dataset**: https://huggingface.co/datasets/lemon07r/VellumK2-Fantasy-DPO-Small-01

---

## Support

For questions, issues, or contributions:
1. Check the [Getting Started Guide](GETTING_STARTED.md)
2. Review [Documentation](README.md)
3. Search [Existing Issues](https://github.com/lemon07r/vellumforge2/issues)
4. Open a [New Issue](https://github.com/lemon07r/vellumforge2/issues/new)

---

