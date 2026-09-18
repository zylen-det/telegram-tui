package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAppErrorError(t *testing.T) {
	tests := []struct {
		name string
		err  AppError
		want string
	}{
		{
			name: "without operation",
			err:  AppError{Message: "request failed"},
			want: "request failed",
		},
		{
			name: "with operation",
			err:  AppError{Op: "load chats", Message: "request failed"},
			want: "load chats: request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppErrorUnwrap(t *testing.T) {
	cause := errors.New("connection reset")
	err := AppError{Message: "request failed", Cause: cause}

	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(%v, cause) = false, want true", err)
	}
}

func TestAppErrorDoesNotExposeCause(t *testing.T) {
	cause := errors.New("telegram_token=123456:super-secret-message-content")
	err := AppError{
		Op:      "send message",
		Message: "request failed",
		Cause:   cause,
	}

	got := err.Error()
	if strings.Contains(got, cause.Error()) {
		t.Fatalf("Error() = %q, must not expose sensitive cause", got)
	}
	const want = "send message: request failed"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(%v, cause) = false, want true", err)
	}
}

func TestAppErrorRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  AppError
		want bool
	}{
		{
			name: "network",
			err:  AppError{Kind: ErrorNetwork},
			want: true,
		},
		{
			name: "rate limit with retry delay",
			err:  AppError{Kind: ErrorRateLimit, RetryAfter: 30 * time.Second},
			want: true,
		},
		{
			name: "media",
			err:  AppError{Kind: ErrorMedia},
			want: true,
		},
		{
			name: "configuration",
			err:  AppError{Kind: ErrorConfiguration},
			want: false,
		},
		{
			name: "authorization",
			err:  AppError{Kind: ErrorAuthorization},
			want: false,
		},
		{
			name: "permission",
			err:  AppError{Kind: ErrorPermission},
			want: false,
		},
		{
			name: "not found",
			err:  AppError{Kind: ErrorNotFound},
			want: false,
		},
		{
			name: "storage",
			err:  AppError{Kind: ErrorStorage},
			want: false,
		},
		{
			name: "version mismatch",
			err:  AppError{Kind: ErrorVersion},
			want: false,
		},
		{
			name: "internal",
			err:  AppError{Kind: ErrorInternal},
			want: false,
		},
		{
			name: "unknown",
			err:  AppError{Kind: ErrorKind("unknown")},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Retryable(); got != tt.want {
				t.Fatalf("Retryable() = %t, want %t", got, tt.want)
			}
		})
	}
}
