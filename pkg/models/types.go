package models

import "time"

// DatasetMode represents the type of dataset to generate
type DatasetMode string

const (
	// DatasetModeSFT generates simple instruction-output pairs for supervised fine-tuning
	DatasetModeSFT DatasetMode = "sft"
	// DatasetModeDPO generates standard DPO format (prompt, chosen, rejected)
	DatasetModeDPO DatasetMode = "dpo"
	// DatasetModeKTO generates unpaired preferences with binary labels (2 rows per pair)
	DatasetModeKTO DatasetMode = "kto"
	// DatasetModeMODPO generates full multi-objective DPO with judge scoring (current implementation)
	DatasetModeMODPO DatasetMode = "mo-dpo"
)

// SFTFormat represents the JSON serialization style for SFT mode outputs
type SFTFormat string

const (
	SFTFormatAlpaca   SFTFormat = "alpaca"
	SFTFormatShareGPT SFTFormat = "sharegpt"
)

// DatasetRecord represents a single record in the MO-DPO dataset (full feature set)
type DatasetRecord struct {
	MainTopic          string                   `json:"main_topic"`
	SubTopic           string                   `json:"sub_topic"`
	Prompt             string                   `json:"prompt"`
	Chosen             string                   `json:"chosen"`
	Rejected           string                   `json:"rejected"`
	ChosenScores       map[string]CriteriaScore `json:"chosen_scores,omitempty"`
	RejectedScores     map[string]CriteriaScore `json:"rejected_scores,omitempty"`
	ChosenScoreTotal   float64                  `json:"chosen_score_total,omitempty"`
	RejectedScoreTotal float64                  `json:"rejected_score_total,omitempty"`
	PreferenceMargin   float64                  `json:"preference_margin,omitempty"`
}

// ShareGPTMessage represents a single conversational turn in ShareGPT format
type ShareGPTMessage struct {
	From  string `json:"from"`
	Value string `json:"value"`
}

// SFTRecord can represent either Alpaca-style or ShareGPT-style outputs
type SFTRecord struct {
	MainTopic string `json:"main_topic,omitempty"`
	SubTopic  string `json:"sub_topic,omitempty"`

	// Alpaca fields
	Instruction string     `json:"instruction,omitempty"`
	Input       string     `json:"input,omitempty"`
	Output      string     `json:"output,omitempty"`
	System      string     `json:"system,omitempty"`
	History     [][]string `json:"history,omitempty"`

	// ShareGPT fields
	Conversations []ShareGPTMessage `json:"conversations,omitempty"`
}

// DPORecord represents a standard DPO preference pair
type DPORecord struct {
	Prompt   string `json:"prompt"`
	Chosen   string `json:"chosen"`
	Rejected string `json:"rejected"`
}

// KTORecord represents an unpaired preference record with binary label
type KTORecord struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
	Label      bool   `json:"label"`
}

// CriteriaScore represents the score and reasoning for a single rubric criterion
type CriteriaScore struct {
	Score     int    `json:"score"`
	Reasoning string `json:"reasoning"`
}

// GenerationJob represents a task to generate a preference pair
type GenerationJob struct {
	ID        int
	MainTopic string
	SubTopic  string
	Prompt    string
}

// GenerationResult represents the result of generating a preference pair
type GenerationResult struct {
	Job               GenerationJob
	Chosen            string
	ChosenReasoning   string // Chain-of-Thought reasoning for chosen response (if captured)
	Rejected          string
	RejectedReasoning string // Chain-of-Thought reasoning for rejected response (if captured)
	JudgeResult       *JudgeResult
	Error             error
	Duration          time.Duration
}

// JudgeResult represents the output from the LLM-as-a-Judge evaluation
type JudgeResult struct {
	ChosenScores       map[string]CriteriaScore `json:"chosen_scores"`
	RejectedScores     map[string]CriteriaScore `json:"rejected_scores"`
	ChosenScoreTotal   float64                  `json:"chosen_score_total"`
	RejectedScoreTotal float64                  `json:"rejected_score_total"`
	PreferenceMargin   float64                  `json:"preference_margin"`
}

// SessionStats tracks statistics for a generation session
type SessionStats struct {
	StartTime       time.Time
	EndTime         time.Time
	TotalPrompts    int
	SuccessCount    int
	FailureCount    int
	FilteredCount   int // Number of records filtered by judge
	TotalDuration   time.Duration
	AverageDuration time.Duration
}
