package orchestrator

import (
	"testing"

	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/pkg/models"
)

type stubWriter struct {
	lastSFTRecord models.SFTRecord
	lastReasoning string
}

func (s *stubWriter) WriteSFTRecord(record models.SFTRecord, reasoning string) error {
	s.lastSFTRecord = record
	s.lastReasoning = reasoning
	return nil
}

func (s *stubWriter) WriteDPORecord(models.DPORecord, string, string) error { panic("unexpected call") }
func (s *stubWriter) WriteKTORecord(models.KTORecord, string) error         { panic("unexpected call") }
func (s *stubWriter) WriteRecord(models.DatasetRecord) (int, error)         { panic("unexpected call") }
func (s *stubWriter) UpdateRecord(int, *models.JudgeResult) error           { panic("unexpected call") }
func (s *stubWriter) Flush() error                                          { return nil }
func (s *stubWriter) Close() error                                          { return nil }

func TestWriteSFTRecordAlpaca(t *testing.T) {
	writer := &stubWriter{}
	orch := &Orchestrator{
		cfg: &config.Config{
			Generation: config.GenerationConfig{
				DatasetMode:         models.DatasetModeSFT,
				IncludeTopicColumns: true,
				SFTFormat:           models.SFTFormatAlpaca,
			},
		},
		dataWriter: writer,
	}

	result := models.GenerationResult{
		Job: models.GenerationJob{
			MainTopic: "main",
			SubTopic:  "sub",
			Prompt:    "instruction",
		},
		Chosen: "answer",
	}

	if err := orch.writeSFTRecord(result); err != nil {
		t.Fatalf("writeSFTRecord returned error: %v", err)
	}

	if writer.lastSFTRecord.Instruction != "instruction" || writer.lastSFTRecord.Output != "answer" {
		t.Fatalf("alpaca record mismatch: %+v", writer.lastSFTRecord)
	}
	if writer.lastSFTRecord.MainTopic != "main" || writer.lastSFTRecord.SubTopic != "sub" {
		t.Fatalf("expected topic columns to be included: %+v", writer.lastSFTRecord)
	}
	if len(writer.lastSFTRecord.Conversations) != 0 {
		t.Fatalf("alpaca record should not include conversations")
	}
}

func TestWriteSFTRecordShareGPT(t *testing.T) {
	writer := &stubWriter{}
	orch := &Orchestrator{
		cfg: &config.Config{
			Generation: config.GenerationConfig{
				DatasetMode:         models.DatasetModeSFT,
				IncludeTopicColumns: false,
				SFTFormat:           models.SFTFormatShareGPT,
			},
		},
		dataWriter: writer,
	}

	result := models.GenerationResult{
		Job: models.GenerationJob{
			Prompt: "say hi",
		},
		Chosen: "hello",
	}

	if err := orch.writeSFTRecord(result); err != nil {
		t.Fatalf("writeSFTRecord returned error: %v", err)
	}

	if len(writer.lastSFTRecord.Conversations) != 2 {
		t.Fatalf("expected 2 conversations, got %d", len(writer.lastSFTRecord.Conversations))
	}
	if writer.lastSFTRecord.Conversations[0].From != "human" || writer.lastSFTRecord.Conversations[0].Value != "say hi" {
		t.Fatalf("unexpected human turn: %+v", writer.lastSFTRecord.Conversations[0])
	}
	if writer.lastSFTRecord.Conversations[1].From != "gpt" || writer.lastSFTRecord.Conversations[1].Value != "hello" {
		t.Fatalf("unexpected gpt turn: %+v", writer.lastSFTRecord.Conversations[1])
	}
	if writer.lastSFTRecord.Instruction != "" || writer.lastSFTRecord.Output != "" {
		t.Fatalf("sharegpt record should not set instruction/output: %+v", writer.lastSFTRecord)
	}
}
