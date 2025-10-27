package writer

import (
	"context"
	"log/slog"
	"os"
)

// multiHandler wraps multiple handlers to write to multiple destinations
type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if err := handler.Handle(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

// SetupLogger creates a multi-handler logger that writes to both stdout and the session log file
func SetupLogger(sessionMgr *SessionManager, logLevel slog.Level) (*slog.Logger, *os.File, error) {
	// Open log file with buffering
	logFile, err := os.OpenFile(sessionMgr.GetLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, err
	}

	// Create handlers for both stdout (text) and file (JSON)
	textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})

	jsonHandler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: logLevel,
	})

	// Use multi-handler to write to both
	logger := slog.New(&multiHandler{
		handlers: []slog.Handler{textHandler, jsonHandler},
	})

	return logger, logFile, nil
}
