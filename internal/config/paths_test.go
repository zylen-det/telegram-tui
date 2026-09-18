package config

import (
	"path/filepath"
	"testing"
)

func TestResolvePathsUsesAbsoluteXDGOverrides(t *testing.T) {
	environment := map[string]string{
		"XDG_CONFIG_HOME": "/custom/config",
		"XDG_STATE_HOME":  "/custom/state",
		"XDG_DATA_HOME":   "/custom/data",
		"XDG_CACHE_HOME":  "/custom/cache",
	}
	getenv := func(key string) string {
		return environment[key]
	}

	got := ResolvePaths(getenv, "/home/alice")
	want := Paths{
		ConfigFile:     filepath.Join("/custom/config", "telegram-tui", "config.toml"),
		StateDir:       filepath.Join("/custom/state", "telegram-tui"),
		TDLibLog:       filepath.Join("/custom/state", "telegram-tui", "tdlib.log"),
		DataDir:        filepath.Join("/custom/data", "telegram-tui"),
		TDLibDatabase:  filepath.Join("/custom/data", "telegram-tui", "tdlib", "database"),
		TDLibFiles:     filepath.Join("/custom/cache", "telegram-tui", "avatars", "files"),
		AvatarCacheDir: filepath.Join("/custom/cache", "telegram-tui", "avatars", "pixels"),
	}

	if got != want {
		t.Fatalf("ResolvePaths() = %#v, want %#v", got, want)
	}
}

func TestResolvePathsIgnoresUnsetAndRelativeXDGValues(t *testing.T) {
	environment := map[string]string{
		"XDG_CONFIG_HOME": "relative/config",
		"XDG_STATE_HOME":  "relative/state",
		"XDG_DATA_HOME":   "relative/data",
		"XDG_CACHE_HOME":  "relative/cache",
	}
	getenv := func(key string) string {
		return environment[key]
	}

	got := ResolvePaths(getenv, "/home/bob")
	want := Paths{
		ConfigFile:     filepath.Join("/home/bob", ".config", "telegram-tui", "config.toml"),
		StateDir:       filepath.Join("/home/bob", ".local", "state", "telegram-tui"),
		TDLibLog:       filepath.Join("/home/bob", ".local", "state", "telegram-tui", "tdlib.log"),
		DataDir:        filepath.Join("/home/bob", ".local", "share", "telegram-tui"),
		TDLibDatabase:  filepath.Join("/home/bob", ".local", "share", "telegram-tui", "tdlib", "database"),
		TDLibFiles:     filepath.Join("/home/bob", ".cache", "telegram-tui", "avatars", "files"),
		AvatarCacheDir: filepath.Join("/home/bob", ".cache", "telegram-tui", "avatars", "pixels"),
	}

	if got != want {
		t.Fatalf("ResolvePaths() = %#v, want %#v", got, want)
	}
}
