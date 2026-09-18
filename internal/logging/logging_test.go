package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestLoggerEmitsOnlyAllowListedSanitizedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "telegram-tui.log")
	logger, closer, err := New(path, slog.LevelDebug)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	secrets := []string{
		"api-hash-secret", "+15550001111", "02719", "two-factor-secret",
		"database-key-secret", "private message body", "draft-secret", "raw update payload",
	}
	logger.Info("operation", Attrs(Fields{
		Operation: "send text",
		Kind:      domain.ErrorRateLimit,
		ChatID:    99,
		Duration:  1500 * time.Millisecond,
		Count:     3,
	})...)
	// The handler must drop arbitrary attributes even if a caller attempts to log them.
	logger.Error(secrets[6],
		"text", secrets[5],
		"password", secrets[3],
		"error", domain.AppError{Kind: domain.ErrorInternal, Message: secrets[0]},
		"payload", secrets[7],
	)
	if err := closer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, secret := range secrets {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("log contains prohibited value %q", secret)
		}
	}
	text := string(data)
	for _, want := range []string{"send text", "rate_limit", `"chat_id":99`, `"duration_ms":1500`, `"count":3`} {
		if !strings.Contains(text, want) {
			t.Fatalf("log missing %q: %s", want, text)
		}
	}
}

func TestAttrsUsesOnlyPublicAllowList(t *testing.T) {
	attrs := Attrs(Fields{Operation: "load", Kind: domain.ErrorNetwork, ChatID: 7, Duration: time.Second, Count: 2})
	if len(attrs)%2 != 0 {
		t.Fatalf("Attrs() returned odd key/value count: %#v", attrs)
	}
	allowed := map[string]bool{"operation": true, "kind": true, "chat_id": true, "duration_ms": true, "count": true}
	for index := 0; index < len(attrs); index += 2 {
		key, ok := attrs[index].(string)
		if !ok || !allowed[key] {
			t.Fatalf("Attrs() key = %#v, not allow-listed", attrs[index])
		}
	}
}

func TestNewCreatesPrivateParentDirectory(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "private")
	logger, closer, err := New(filepath.Join(parent, "app.log"), slog.LevelInfo)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger.Info("ready")
	if err := closer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	info, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory mode = %o, want 700", got)
	}
}
