package config

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"

	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

const databaseKeySize = 32

type Runtime struct {
	Preferences Preferences
	APIID       int32
	APIHash     string
	DatabaseKey []byte
	Paths       Paths
}

type Resolver struct {
	Paths   Paths
	Prompts auth.Prompter
	Getenv  func(string) string
}

func (r Resolver) Resolve(ctx context.Context) (Runtime, error) {
	preferences, err := LoadPreferences(r.Paths.ConfigFile)
	if err != nil {
		return Runtime{}, fmt.Errorf("load preferences: %w", err)
	}
	getenv := r.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	changed := false
	apiID, prompted, err := r.resolveAPIID(ctx, getenv("TELEGRAM_API_ID"), preferences.APIID)
	if err != nil {
		return Runtime{}, err
	}
	if prompted {
		preferences.APIID = apiID
		changed = true
	}

	apiHash, prompted, err := r.resolveAPIHash(ctx, getenv("TELEGRAM_API_HASH"), preferences.APIHash)
	if err != nil {
		return Runtime{}, err
	}
	if prompted {
		preferences.APIHash = apiHash
		changed = true
	}

	databaseKey, generated, err := resolveDatabaseKey(preferences.DatabaseKey)
	if err != nil {
		return Runtime{}, err
	}
	if generated {
		preferences.DatabaseKey = base64.RawStdEncoding.EncodeToString(databaseKey)
		changed = true
	}
	if changed {
		if err := SavePreferences(r.Paths.ConfigFile, preferences); err != nil {
			return Runtime{}, fmt.Errorf("save preferences: %w", err)
		}
	}

	return Runtime{Preferences: preferences, APIID: apiID, APIHash: apiHash, DatabaseKey: databaseKey, Paths: r.Paths}, nil
}

func (r Resolver) resolveAPIID(ctx context.Context, environmentValue string, configuredValue int32) (int32, bool, error) {
	if environmentValue != "" {
		apiID, err := parseAPIID(environmentValue)
		return apiID, false, err
	}
	if configuredValue != 0 {
		if configuredValue <= 0 {
			return 0, false, invalidAPIIDError()
		}
		return configuredValue, false, nil
	}
	value, err := r.Prompts.Ask(ctx, auth.Prompt{Kind: auth.PromptAPIID, Label: "Telegram API ID"})
	if err != nil {
		return 0, false, err
	}
	apiID, err := parseAPIID(value)
	return apiID, true, err
}

func parseAPIID(value string) (int32, error) {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed <= 0 {
		return 0, invalidAPIIDError()
	}
	return int32(parsed), nil
}

func invalidAPIIDError() error {
	return domain.AppError{Kind: domain.ErrorConfiguration, Op: "resolve API ID", Message: "Telegram API ID must be a positive integer"}
}

func (r Resolver) resolveAPIHash(ctx context.Context, environmentValue, configuredValue string) (string, bool, error) {
	if environmentValue != "" {
		value, err := validateAPIHash(environmentValue)
		return value, false, err
	}
	if configuredValue != "" {
		value, err := validateAPIHash(configuredValue)
		return value, false, err
	}
	apiHash, err := r.Prompts.Ask(ctx, auth.Prompt{Kind: auth.PromptAPIHash, Label: "Telegram API hash", Secret: true})
	if err != nil {
		return "", false, err
	}
	value, err := validateAPIHash(apiHash)
	return value, true, err
}

func validateAPIHash(apiHash string) (string, error) {
	if apiHash == "" {
		return "", domain.AppError{Kind: domain.ErrorConfiguration, Op: "resolve API hash", Message: "Telegram API hash cannot be empty"}
	}
	return apiHash, nil
}

func resolveDatabaseKey(encoded string) ([]byte, bool, error) {
	if encoded != "" {
		key, err := decodeDatabaseKey(encoded)
		return key, false, err
	}
	databaseKey := make([]byte, databaseKeySize)
	if _, err := rand.Read(databaseKey); err != nil {
		return nil, false, domain.AppError{Kind: domain.ErrorStorage, Op: "generate database key", Message: "could not generate database key", Cause: err}
	}
	return databaseKey, true, nil
}

func decodeDatabaseKey(encoded string) ([]byte, error) {
	databaseKey, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(databaseKey) != databaseKeySize {
		return nil, domain.AppError{Kind: domain.ErrorStorage, Op: "decode database key", Message: "stored database key is invalid", Cause: err}
	}
	return databaseKey, nil
}
