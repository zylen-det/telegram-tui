package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const defaultImageProtocol = "kitty"

type Preferences struct {
	APIID         int32  `toml:"api_id"`
	APIHash       string `toml:"api_hash,omitempty"`
	DatabaseKey   string `toml:"database_key,omitempty"`
	ImageProtocol string `toml:"image_protocol"`
}

func LoadPreferences(path string) (Preferences, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return defaultPreferences(), nil
	}
	if err != nil {
		return Preferences{}, fmt.Errorf("read preferences: %w", err)
	}

	var preferences Preferences
	if err := toml.Unmarshal(contents, &preferences); err != nil {
		return Preferences{}, fmt.Errorf("decode preferences: %w", err)
	}
	if preferences.ImageProtocol == "" {
		preferences.ImageProtocol = defaultImageProtocol
	}
	return preferences, nil
}

func SavePreferences(path string, preferences Preferences) error {
	contents, err := toml.Marshal(preferences)
	if err != nil {
		return fmt.Errorf("encode preferences: %w", err)
	}

	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create preferences directory: %w", err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return fmt.Errorf("secure preferences directory: %w", err)
	}

	temporary, err := os.CreateTemp(parent, ".config.toml-*")
	if err != nil {
		return fmt.Errorf("create temporary preferences file: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary preferences file: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		return fmt.Errorf("write temporary preferences file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary preferences file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary preferences file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace preferences file: %w", err)
	}
	removeTemporary = false
	return nil
}

func defaultPreferences() Preferences {
	return Preferences{ImageProtocol: defaultImageProtocol}
}
