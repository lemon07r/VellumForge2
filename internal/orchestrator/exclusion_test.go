package orchestrator

import "testing"

func TestTruncateExclusionList(t *testing.T) {
	tests := []struct {
		name         string
		items        []string
		maxSize      int
		wantLen      int
		wantTrunc    bool
		wantLastItem string
	}{
		{
			name:         "no truncation needed - empty list",
			items:        []string{},
			maxSize:      10,
			wantLen:      0,
			wantTrunc:    false,
			wantLastItem: "",
		},
		{
			name:         "no truncation needed - below max",
			items:        []string{"a", "b", "c"},
			maxSize:      10,
			wantLen:      3,
			wantTrunc:    false,
			wantLastItem: "c",
		},
		{
			name:         "truncate to last N items",
			items:        []string{"a", "b", "c", "d", "e"},
			maxSize:      3,
			wantLen:      3,
			wantTrunc:    true,
			wantLastItem: "e", // Should keep last 3: c, d, e
		},
		{
			name:         "exact size - no truncation",
			items:        []string{"a", "b", "c"},
			maxSize:      3,
			wantLen:      3,
			wantTrunc:    false,
			wantLastItem: "c",
		},
		{
			name:         "truncate large list",
			items:        []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"},
			maxSize:      5,
			wantLen:      5,
			wantTrunc:    true,
			wantLastItem: "10", // Should keep last 5
		},
		{
			name:         "maxSize of 1",
			items:        []string{"a", "b", "c"},
			maxSize:      1,
			wantLen:      1,
			wantTrunc:    true,
			wantLastItem: "c", // Should keep only last item
		},
		{
			name:         "single item, maxSize 1",
			items:        []string{"a"},
			maxSize:      1,
			wantLen:      1,
			wantTrunc:    false,
			wantLastItem: "a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, truncated := truncateExclusionList(tt.items, tt.maxSize)

			if len(result) != tt.wantLen {
				t.Errorf("len(result) = %d, want %d", len(result), tt.wantLen)
			}
			if truncated != tt.wantTrunc {
				t.Errorf("truncated = %v, want %v", truncated, tt.wantTrunc)
			}
			if tt.wantLen > 0 && result[len(result)-1] != tt.wantLastItem {
				t.Errorf("last item = %s, want %s", result[len(result)-1], tt.wantLastItem)
			}

			// Verify we kept the LAST N items
			if truncated && tt.wantLen > 0 {
				expectedStartIdx := len(tt.items) - tt.maxSize
				expectedFirstItem := tt.items[expectedStartIdx]
				if result[0] != expectedFirstItem {
					t.Errorf("first item after truncation = %s, want %s", result[0], expectedFirstItem)
				}
			}
		})
	}
}

func TestTruncateExclusionListRealScenario(t *testing.T) {
	// Simulate a real scenario with many failed subtopics
	var items []string
	for i := 1; i <= 300; i++ {
		items = append(items, "Subtopic"+string(rune(i)))
	}

	maxSize := 50
	result, truncated := truncateExclusionList(items, maxSize)

	if !truncated {
		t.Error("Expected truncation for 300 items with maxSize 50")
	}
	if len(result) != maxSize {
		t.Errorf("Expected result length = %d, got %d", maxSize, len(result))
	}

	// Verify we kept the last 50 items (most recent failures)
	expectedFirst := items[250] // 300 - 50 = 250
	if result[0] != expectedFirst {
		t.Errorf("First item in truncated list = %s, want %s", result[0], expectedFirst)
	}

	expectedLast := items[299]
	if result[49] != expectedLast {
		t.Errorf("Last item in truncated list = %s, want %s", result[49], expectedLast)
	}
}
