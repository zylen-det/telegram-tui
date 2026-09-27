//go:build !tdlib

package telegram

import (
	"github.com/zylen-det/tuilegram/internal/auth"
	"github.com/zylen-det/tuilegram/internal/config"
	"github.com/zylen-det/tuilegram/internal/domain"
)

func New(runtime config.Runtime, prompts auth.Prompter) (Client, error) {
	return nil, domain.AppError{
		Kind:    domain.ErrorVersion,
		Op:      "initialize Telegram",
		Message: "this binary was built without TDLib support; rebuild with make build",
	}
}
