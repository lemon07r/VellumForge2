package util

import (
	"strings"
	"testing"
)

func TestCleanMetaFromLLMResponse_NoMeta(t *testing.T) {
	original := "Once upon a time, there was a simple story."
	clean := CleanMetaFromLLMResponse(original)
	if clean != original {
		t.Fatalf("expected no change, got %q", clean)
	}
}

func TestCleanMetaFromLLMResponse_TruncatesMeta(t *testing.T) {
	original := "Story text goes here.\n\nWe don't want too many lines but a simple long story."
	clean := CleanMetaFromLLMResponse(original)
	if clean == original {
		t.Fatalf("expected meta to be removed, but content was unchanged")
	}
	if clean == "" {
		t.Fatalf("expected non-empty content after cleaning meta")
	}
	if !strings.Contains(clean, "Story text goes here.") {
		t.Fatalf("expected story content to be preserved, got %q", clean)
	}
}