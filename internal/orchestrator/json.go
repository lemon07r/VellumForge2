package orchestrator

import (
	"github.com/lamim/vellumforge2/internal/util"
)

// extractJSON is a convenience wrapper around util.ExtractJSON
// Kept for backward compatibility within the orchestrator package
func extractJSON(s string) string {
	return util.ExtractJSON(s)
}
