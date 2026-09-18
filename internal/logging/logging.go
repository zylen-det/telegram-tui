package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

var allowedKeys = map[string]struct{}{
	"operation":   {},
	"kind":        {},
	"chat_id":     {},
	"duration_ms": {},
	"count":       {},
}

type Fields struct {
	Operation string
	Kind      domain.ErrorKind
	ChatID    domain.ChatID
	Duration  time.Duration
	Count     int
}

func Attrs(fields Fields) []any {
	values := make([]any, 0, 10)
	if fields.Operation != "" {
		values = append(values, "operation", fields.Operation)
	}
	if fields.Kind != "" {
		values = append(values, "kind", string(fields.Kind))
	}
	if fields.ChatID != 0 {
		values = append(values, "chat_id", int64(fields.ChatID))
	}
	if fields.Duration != 0 {
		values = append(values, "duration_ms", fields.Duration.Milliseconds())
	}
	if fields.Count != 0 {
		values = append(values, "count", fields.Count)
	}
	return values
}

func New(path string, level slog.Level) (*slog.Logger, io.Closer, error) {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, nil, err
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return nil, nil, err
	}
	writer := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    5,
		MaxBackups: 3,
		MaxAge:     14,
		Compress:   true,
	}
	base := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: level})
	return slog.New(&allowListHandler{next: base}), &lockedCloser{closer: writer}, nil
}

type allowListHandler struct {
	next slog.Handler
}

func (h *allowListHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *allowListHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, "telegram-tui", record.PC)
	record.Attrs(func(attribute slog.Attr) bool {
		attribute.Value = attribute.Value.Resolve()
		if _, ok := allowedKeys[attribute.Key]; !ok {
			return true
		}
		clean.AddAttrs(attribute)
		return true
	})
	return h.next.Handle(ctx, clean)
}

func (h *allowListHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	filtered := make([]slog.Attr, 0, len(attributes))
	for _, attribute := range attributes {
		if _, ok := allowedKeys[attribute.Key]; ok {
			attribute.Value = attribute.Value.Resolve()
			filtered = append(filtered, attribute)
		}
	}
	return &allowListHandler{next: h.next.WithAttrs(filtered)}
}

func (h *allowListHandler) WithGroup(string) slog.Handler {
	// Group names create an unbounded key namespace, so reject grouping.
	return h
}

type lockedCloser struct {
	mu     sync.Mutex
	closer io.Closer
	closed bool
}

func (c *lockedCloser) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.closer.Close()
}
