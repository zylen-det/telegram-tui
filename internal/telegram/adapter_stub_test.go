//go:build !tdlib

package telegram

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestNewWithoutTDLibReturnsSafeVersionError(t *testing.T) {
	runtime := config.Runtime{
		APIID:       123456789,
		APIHash:     "api-hash-sensitive-sentinel",
		DatabaseKey: []byte("database-key-sensitive-sentinel"),
	}
	client, err := New(runtime, nil)
	if client != nil {
		t.Fatal("New() client is non-nil without TDLib support")
	}
	if err == nil {
		t.Fatal("New() error = nil, want version error")
	}
	var appError domain.AppError
	if !errors.As(err, &appError) {
		t.Fatalf("New() error type = %T, want domain.AppError", err)
	}
	if appError.Kind != domain.ErrorVersion || appError.Op != "initialize Telegram" || appError.Message != "this binary was built without TDLib support; rebuild with make build" {
		t.Fatal("New() did not return the documented version error")
	}
	wantText := "initialize Telegram: this binary was built without TDLib support; rebuild with make build"
	if err.Error() != wantText {
		t.Fatalf("New() error text = %q, want documented safe text", err.Error())
	}
	prohibited := []string{
		strconv.FormatInt(int64(runtime.APIID), 10),
		runtime.APIHash,
		string(runtime.DatabaseKey),
		"APIID",
		"APIHash",
		"DatabaseKey",
	}
	for _, value := range prohibited {
		if strings.Contains(err.Error(), value) {
			t.Fatal("New() error text exposed a runtime field or secret")
		}
	}
}
