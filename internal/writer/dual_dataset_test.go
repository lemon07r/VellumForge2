package writer

import (
	"testing"

	"github.com/lamim/vellumforge2/internal/util"
	"github.com/lamim/vellumforge2/pkg/models"
)

func TestApplyReasoningToSFTRecordAlpaca(t *testing.T) {
	record := models.SFTRecord{
		Instruction: "prompt",
		Output:      "final answer",
	}
	reasoning := "chain of thought"

	updated := applyReasoningToSFTRecord(record, reasoning)
	expected := util.CombineReasoningAndContent(reasoning, record.Output)

	if updated.Output != expected {
		t.Fatalf("expected output with reasoning, got %q", updated.Output)
	}
	if len(updated.Conversations) != 0 {
		t.Fatalf("alpaca record should not gain conversations, got %d", len(updated.Conversations))
	}
}

func TestApplyReasoningToSFTRecordShareGPT(t *testing.T) {
	record := models.SFTRecord{
		Conversations: []models.ShareGPTMessage{
			{From: "human", Value: "hello"},
			{From: "gpt", Value: "response"},
		},
	}
	reasoning := "reasoning block"

	updated := applyReasoningToSFTRecord(record, reasoning)
	if len(updated.Conversations) != 2 {
		t.Fatalf("expected conversations length 2, got %d", len(updated.Conversations))
	}

	expected := util.CombineReasoningAndContent(reasoning, "response")
	if updated.Conversations[1].Value != expected {
		t.Fatalf("expected assistant message to be wrapped, got %q", updated.Conversations[1].Value)
	}
}
