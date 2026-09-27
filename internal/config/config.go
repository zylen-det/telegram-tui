package config

import (
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pelletier/go-toml/v2"
)

const defaultImageProtocol = "kitty"

type Preferences struct {
	APIID         int32             `toml:"api_id"`
	APIHash       string            `toml:"api_hash,omitempty"`
	DatabaseKey   string            `toml:"database_key,omitempty"`
	ImageProtocol string            `toml:"image_protocol"`
	SenderColors  map[string]string `toml:"sender_colors,omitempty"`
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
	if _, err := preferences.SenderPalette(); err != nil {
		return Preferences{}, fmt.Errorf("decode preferences: %w", err)
	}
	return preferences, nil
}

func SavePreferences(path string, preferences Preferences) error {
	if _, err := preferences.SenderPalette(); err != nil {
		return fmt.Errorf("encode preferences: %w", err)
	}
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

// SenderPalette returns the seven built-in Telegram sender colors with optional
// per-ID config overrides. TDLib assigns IDs; the client chooses their RGBs.
func (p Preferences) SenderPalette() ([7]color.RGBA, error) {
	palette := [7]color.RGBA{
		{R: 0xCC, G: 0x50, B: 0x49, A: 255}, // red
		{R: 0xD6, G: 0x77, B: 0x22, A: 255}, // orange
		{R: 0x95, G: 0x5C, B: 0xDB, A: 255}, // violet
		{R: 0x40, G: 0xA9, B: 0x20, A: 255}, // green
		{R: 0x30, G: 0x9E, B: 0xBA, A: 255}, // cyan
		{R: 0x36, G: 0x8A, B: 0xD1, A: 255}, // blue
		{R: 0xC7, G: 0x50, B: 0x8B, A: 255}, // pink
	}
	for key, value := range p.SenderColors {
		if len(key) != 1 || key[0] < '0' || key[0] > '6' {
			return palette, fmt.Errorf("sender_colors: invalid ID %q (expected 0-6)", key)
		}
		if len(value) != 7 || value[0] != '#' {
			return palette, fmt.Errorf("sender_colors.%s: expected #RRGGBB", key)
		}
		rgb, err := strconv.ParseUint(value[1:], 16, 24)
		if err != nil {
			return palette, fmt.Errorf("sender_colors.%s: expected #RRGGBB", key)
		}
		palette[key[0]-'0'] = color.RGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}
	}
	return palette, nil
}

func defaultPreferences() Preferences {
	palette, _ := (Preferences{}).SenderPalette()
	colors := make(map[string]string, len(palette))
	for id, value := range palette {
		colors[strconv.Itoa(id)] = fmt.Sprintf("#%02X%02X%02X", value.R, value.G, value.B)
	}
	return Preferences{ImageProtocol: defaultImageProtocol, SenderColors: colors}
}
