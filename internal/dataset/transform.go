package dataset

import
(
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/lamim/vellumforge2/internal/api"
	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/internal/util"
	"github.com/lamim/vellumforge2/pkg/models"
)

// TransformMode specifies what kind of dataset operation to perform.
type TransformMode string

const (
	// TransformSFTToDPO converts an SFT dataset into a DPO dataset by generating rejected responses.
	TransformSFTToDPO TransformMode = "sft-to-dpo"
	// TransformRegenRejected regenerates the rejected responses for an existing DPO dataset.
	TransformRegenRejected TransformMode = "regen-rejected"
)

// Options controls dataset transformation behaviour.
type Options struct {
	InputPath  string
	OutputPath string
	// Concurrency controls how many rejected generations run in parallel.
	Concurrency int
}

// Run performs a dataset transformation using the provided config and API client.
//
// It supports two modes:
//   - TransformSFTToDPO: read SFT records and write DPO records with newly generated rejected responses
//   - TransformRegenRejected: read DPO records and regenerate the rejected field using the rejected model
func Run(
	ctx context.Context,
	logger *slog.Logger,
	mode TransformMode,
	cfg *config.Config,
	secrets *config.Secrets,
	client *api.Client,
	opts Options,
) error {
	if opts.InputPath == "" {
		return fmt.Errorf("input path is required")
	}
	if opts.OutputPath == "" {
		return fmt.Errorf("output path is required")
	}

	// Ensure rejected model is configured; both modes rely on it.
	rejectedModel, ok := cfg.Models["rejected"]
	if !ok {
		return fmt.Errorf("config is missing 'rejected' model; it is required for dataset transforms")
	}

	if cfg.PromptTemplates.RejectedGeneration == "" {
		return fmt.Errorf("config.prompt_templates.rejected_generation is required for dataset transforms")
	}

	if opts.Concurrency <= 0 {
		// Reuse pipeline default when not explicitly set.
		opts.Concurrency = cfg.Generation.Concurrency
		if opts.Concurrency <= 0 {
			opts.Concurrency = 4
		}
	}

	switch mode {
	case TransformSFTToDPO:
		return runSFTToDPO(ctx, logger, cfg, secrets, client, rejectedModel, opts)
	case TransformRegenRejected:
		return runRegenRejected(ctx, logger, cfg, secrets, client, rejectedModel, opts)
	default:
		return fmt.Errorf("unsupported transform mode: %s", mode)
	}
}

// runSFTToDPO converts an SFT dataset into a DPO dataset by generating new rejected responses.
func runSFTToDPO(
	ctx context.Context,
	logger *slog.Logger,
	cfg *config.Config,
	secrets *config.Secrets,
	client *api.Client,
	rejectedModel config.ModelConfig,
	opts Options,
) error {
	inputFile, err := os.Open(opts.InputPath)
	if err != nil {
		return fmt.Errorf("failed to open input dataset: %w", err)
	}
	defer func() { _ = inputFile.Close() }()

	outputFile, err := os.Create(opts.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create output dataset: %w", err)
	}
	defer func() { _ = outputFile.Close() }()

	scanner := bufio.NewScanner(inputFile)
	// Allow large JSONL lines (long stories).
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	apiKey := secrets.GetAPIKey(rejectedModel.BaseURL)
	if apiKey == "" && !isLocalEndpoint(rejectedModel.BaseURL) {
		logger.Warn("No API key found for rejected model base URL", "base_url", rejectedModel.BaseURL)
	}

	lineNum := 0
	encoder := json.NewEncoder(outputFile)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		lineNum++
		if line == "" {
			continue
		}

		var sft models.SFTRecord
		if err := json.Unmarshal([]byte(line), &sft); err != nil {
			return fmt.Errorf("line %d: failed to parse SFT record: %w", lineNum, err)
		}

		prompt, chosen, err := extractPromptAndChosen(&sft, cfg.Generation.SFTFormat)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNum, err)
		}

		rejected, err := generateRejected(ctx, logger, cfg, client, rejectedModel, apiKey, prompt)
		if err != nil {
			return fmt.Errorf("line %d: failed to generate rejected response: %w", lineNum, err)
		}

		record := models.DPORecord{
			Prompt:   prompt,
			Chosen:   chosen,
			Rejected: rejected,
		}

		if err := encoder.Encode(&record); err != nil {
			return fmt.Errorf("line %d: failed to write DPO record: %w", lineNum, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed while reading input dataset: %w", err)
	}

	logger.Info("SFT→DPO transform completed",
		"input", opts.InputPath,
		"output", opts.OutputPath,
		"lines", lineNum)

	return nil
}

// runRegenRejected regenerates the rejected responses of an existing DPO dataset.
func runRegenRejected(
	ctx context.Context,
	logger *slog.Logger,
	cfg *config.Config,
	secrets *config.Secrets,
	client *api.Client,
	rejectedModel config.ModelConfig,
	opts Options,
) error {
	inputFile, err := os.Open(opts.InputPath)
	if err != nil {
		return fmt.Errorf("failed to open input dataset: %w", err)
	}
	defer func() { _ = inputFile.Close() }()

	outputFile, err := os.Create(opts.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create output dataset: %w", err)
	}
	defer func() { _ = outputFile.Close() }()

	scanner := bufio.NewScanner(inputFile)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	apiKey := secrets.GetAPIKey(rejectedModel.BaseURL)
	if apiKey == "" && !isLocalEndpoint(rejectedModel.BaseURL) {
		logger.Warn("No API key found for rejected model base URL", "base_url", rejectedModel.BaseURL)
	}

	encoder := json.NewEncoder(outputFile)
	lineNum := 0

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		lineNum++
		if line == "" {
			continue
		}

		var dpo models.DPORecord
		if err := json.Unmarshal([]byte(line), &dpo); err != nil {
			return fmt.Errorf("line %d: failed to parse DPO record: %w", lineNum, err)
		}

		if strings.TrimSpace(dpo.Prompt) == "" {
			return fmt.Errorf("line %d: DPO record is missing prompt field", lineNum)
		}
		if strings.TrimSpace(dpo.Chosen) == "" {
			return fmt.Errorf("line %d: DPO record is missing chosen field", lineNum)
		}

		rejected, err := generateRejected(ctx, logger, cfg, client, rejectedModel, apiKey, dpo.Prompt)
		if err != nil {
			return fmt.Errorf("line %d: failed to regenerate rejected response: %w", lineNum, err)
		}

		dpo.Rejected = rejected
		if err := encoder.Encode(&dpo); err != nil {
			return fmt.Errorf("line %d: failed to write updated DPO record: %w", lineNum, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed while reading input dataset: %w", err)
	}

	logger.Info("DPO rejected regeneration completed",
		"input", opts.InputPath,
		"output", opts.OutputPath,
		"lines", lineNum)

	return nil
}

// extractPromptAndChosen derives the prompt and chosen text from an SFT record
// using the configured SFT format.
func extractPromptAndChosen(record *models.SFTRecord, format models.SFTFormat) (string, string, error) {
	switch format {
	case models.SFTFormatAlpaca:
		prompt := strings.TrimSpace(record.Instruction)
		if record.Input != "" {
			if prompt != "" {
				prompt += "\n\n" + strings.TrimSpace(record.Input)
			} else {
				prompt = strings.TrimSpace(record.Input)
			}
		}
		if prompt == "" {
			return "", "", fmt.Errorf("alpaca SFT record missing instruction/input")
		}
		if strings.TrimSpace(record.Output) == "" {
			return "", "", fmt.Errorf("alpaca SFT record missing output")
		}
		return prompt, record.Output, nil

	case models.SFTFormatShareGPT:
		var prompt string
		var chosen string

		for _, msg := range record.Conversations {
			from := strings.ToLower(strings.TrimSpace(msg.From))
			if prompt == "" && (from == "human" || from == "user") {
				prompt = strings.TrimSpace(msg.Value)
			}
		}

		for i := len(record.Conversations) - 1; i >= 0; i-- {
			from := strings.ToLower(strings.TrimSpace(record.Conversations[i].From))
			if from == "gpt" || from == "assistant" {
				chosen = record.Conversations[i].Value
				break
			}
		}

		if prompt == "" {
			return "", "", fmt.Errorf("sharegpt SFT record missing human prompt")
		}
		if strings.TrimSpace(chosen) == "" {
			return "", "", fmt.Errorf("sharegpt SFT record missing assistant completion")
		}
		return prompt, chosen, nil

	default:
		return "", "", fmt.Errorf("unsupported SFT format for transform: %s", format)
	}
}

// generateRejected calls the rejected model using the configured rejected_generation template.
func generateRejected(
	ctx context.Context,
	logger *slog.Logger,
	cfg *config.Config,
	client *api.Client,
	rejectedModel config.ModelConfig,
	apiKey string,
	prompt string,
) (string, error) {
	renderedPrompt, err := util.RenderTemplate(cfg.PromptTemplates.RejectedGeneration, map[string]interface{}{
		"Prompt": prompt,
	})
	if err != nil {
		return "", fmt.Errorf("failed to render rejected template: %w", err)
	}

	messages := []api.Message{}
	if cfg.PromptTemplates.RejectedSystemPrompt != "" {
		messages = append(messages, api.Message{
			Role:    "system",
			Content: cfg.PromptTemplates.RejectedSystemPrompt,
		})
	}
	messages = append(messages, api.Message{
		Role:    "user",
		Content: renderedPrompt,
	})

	var resp *api.ChatCompletionResponse
	if rejectedModel.UseStreaming {
		resp, err = client.ChatCompletionStreaming(ctx, rejectedModel, apiKey, messages)
	} else {
		resp, err = client.ChatCompletion(ctx, rejectedModel, apiKey, messages)
	}
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from rejected model")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("empty content returned from rejected model")
	}

	return content, nil
}

// isLocalEndpoint mirrors the internal api helper for determining if a base URL is local.
func isLocalEndpoint(endpoint string) bool {
	return strings.Contains(endpoint, "://127.0.0.1") ||
		strings.Contains(endpoint, "://localhost") ||
		strings.Contains(endpoint, "://[::1]") ||
		strings.Contains(endpoint, "://0.0.0.0")
}
