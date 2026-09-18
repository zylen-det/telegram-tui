//go:build tdlib

package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestNormalizeErrorCategorizesTDLibFailures(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		kind       domain.ErrorKind
		retryAfter time.Duration
	}{
		{name: "authorization", err: responseError(401, "AUTH_KEY_UNREGISTERED"), kind: domain.ErrorAuthorization},
		{name: "permission", err: responseError(403, "CHAT_WRITE_FORBIDDEN"), kind: domain.ErrorPermission},
		{name: "wrapped pointer", err: errors.Join(errors.New("transport"), &td.ResponseError{Err: &td.Error{Code: 403, Message: "CHAT_WRITE_FORBIDDEN"}}), kind: domain.ErrorPermission},
		{name: "not found", err: responseError(404, "CHAT_NOT_FOUND"), kind: domain.ErrorNotFound},
		{name: "420", err: responseError(420, "SLOWMODE_WAIT_4"), kind: domain.ErrorRateLimit},
		{name: "flood wait", err: responseError(400, "FLOOD_WAIT_37"), kind: domain.ErrorRateLimit, retryAfter: 37 * time.Second},
		{name: "too many requests", err: responseError(429, "TOO_MANY_REQUESTS"), kind: domain.ErrorRateLimit},
		{name: "server", err: responseError(503, "UPSTREAM_UNAVAILABLE"), kind: domain.ErrorNetwork},
		{name: "canceled", err: context.Canceled, kind: domain.ErrorNetwork},
		{name: "deadline", err: context.DeadlineExceeded, kind: domain.ErrorNetwork},
		{name: "default", err: responseError(400, "MESSAGE_EMPTY"), kind: domain.ErrorInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeError("send text", test.err)
			if got.Kind != test.kind || got.Op != "send text" || got.RetryAfter != test.retryAfter {
				t.Fatalf("normalized fields = (%q, %q, %s), want (%q, %q, %s)", got.Kind, got.Op, got.RetryAfter, test.kind, "send text", test.retryAfter)
			}
			if !errors.Is(got, test.err) {
				t.Fatal("normalized error does not preserve its cause")
			}
		})
	}
}

func TestNormalizeErrorUsesSafePublicMessage(t *testing.T) {
	private := "FLOOD_WAIT_9 api-hash-secret +15551234567 login-code password database-key outbound-message"
	cause := responseError(420, private)
	got := normalizeError("send text", cause)

	for _, secret := range strings.Fields(private) {
		if strings.Contains(got.Error(), secret) {
			t.Fatalf("public error contains private token %q", secret)
		}
	}
	if !errors.Is(got, cause) {
		t.Fatal("safe error discarded the diagnostic cause")
	}
}

func responseError(code int32, message string) error {
	return td.ResponseError{Err: &td.Error{Code: code, Message: message}}
}
