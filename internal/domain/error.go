package domain

import (
	"fmt"
	"time"
)

type ErrorKind string

const (
	ErrorConfiguration ErrorKind = "configuration"
	ErrorAuthorization ErrorKind = "authorization"
	ErrorNetwork       ErrorKind = "network"
	ErrorRateLimit     ErrorKind = "rate_limit"
	ErrorPermission    ErrorKind = "permission"
	ErrorNotFound      ErrorKind = "not_found"
	ErrorMedia         ErrorKind = "media"
	ErrorStorage       ErrorKind = "storage"
	ErrorVersion       ErrorKind = "version_mismatch"
	ErrorInternal      ErrorKind = "internal"
)

type AppError struct {
	Kind       ErrorKind
	Op         string
	Message    string
	RetryAfter time.Duration
	Cause      error
}

func (e AppError) Error() string {
	if e.Op == "" {
		return e.Message
	}

	return fmt.Sprintf("%s: %s", e.Op, e.Message)
}

func (e AppError) Unwrap() error {
	return e.Cause
}

func (e AppError) Retryable() bool {
	switch e.Kind {
	case ErrorNetwork, ErrorRateLimit, ErrorMedia:
		return true
	default:
		return false
	}
}
