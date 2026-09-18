//go:build tdlib

package telegram

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func normalizeError(op string, err error) domain.AppError {
	kind := domain.ErrorInternal
	message := "Telegram request failed"
	var retryAfter time.Duration

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		kind = domain.ErrorNetwork
		message = "Telegram request was interrupted; retry when the connection is available"
	} else {
		if response, ok := tdResponseError(err); ok {
			code := response.Code
			retryAfter = floodWait(response.Message)
			switch {
			case code == 401:
				kind = domain.ErrorAuthorization
				message = "Telegram authorization is required"
			case code == 403:
				kind = domain.ErrorPermission
				message = "Telegram denied this action"
			case code == 404:
				kind = domain.ErrorNotFound
				message = "Telegram data was not found"
			case code == 420 || code == 429 || strings.HasPrefix(response.Message, "FLOOD_WAIT_"):
				kind = domain.ErrorRateLimit
				message = "Telegram rate limit reached; retry later"
			case code >= 500 && code <= 599:
				kind = domain.ErrorNetwork
				message = "Telegram is temporarily unavailable"
			}
		}
	}

	return domain.AppError{
		Kind:       kind,
		Op:         op,
		Message:    message,
		RetryAfter: retryAfter,
		Cause:      err,
	}
}

func tdResponseError(err error) (*td.Error, bool) {
	var response td.ResponseError
	if errors.As(err, &response) && response.Err != nil {
		return response.Err, true
	}
	var responsePointer *td.ResponseError
	if errors.As(err, &responsePointer) && responsePointer != nil && responsePointer.Err != nil {
		return responsePointer.Err, true
	}
	return nil, false
}

func floodWait(message string) time.Duration {
	const prefix = "FLOOD_WAIT_"
	if !strings.HasPrefix(message, prefix) {
		return 0
	}
	value := strings.TrimPrefix(message, prefix)
	end := 0
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	seconds, err := strconv.ParseInt(value[:end], 10, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
