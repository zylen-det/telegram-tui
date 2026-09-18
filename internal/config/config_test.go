package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	want := Preferences{
		APIID:         12345,
		APIHash:       "local-api-hash",
		DatabaseKey:   "local-database-key",
		ImageProtocol: "kitty",
	}

	if err := SavePreferences(path, want); err != nil {
		t.Fatalf("SavePreferences() error = %v", err)
	}

	got, err := LoadPreferences(path)
	if err != nil {
		t.Fatalf("LoadPreferences() error = %v", err)
	}
	if got != want {
		t.Fatalf("LoadPreferences() = %#v, want %#v", got, want)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	serialized := strings.ToLower(string(contents))
	if !strings.Contains(serialized, "api_id") {
		t.Fatalf("serialized preferences = %q, want api_id", contents)
	}
	if !strings.Contains(serialized, "api_hash") || !strings.Contains(serialized, "database_key") {
		t.Fatalf("serialized local config omits credentials: %q", contents)
	}
}

func TestLoadPreferencesReturnsDefaultsForMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")

	got, err := LoadPreferences(path)
	if err != nil {
		t.Fatalf("LoadPreferences() error = %v", err)
	}
	want := Preferences{ImageProtocol: "kitty"}
	if got != want {
		t.Fatalf("LoadPreferences() = %#v, want %#v", got, want)
	}
}

func TestLoadPreferencesDefaultsEmptyImageProtocol(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     Preferences
	}{
		{
			name:     "missing",
			contents: "api_id = 99\n",
			want:     Preferences{APIID: 99, ImageProtocol: "kitty"},
		},
		{
			name:     "empty",
			contents: "image_protocol = \"\"\n",
			want:     Preferences{ImageProtocol: "kitty"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(test.contents), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			got, err := LoadPreferences(path)
			if err != nil {
				t.Fatalf("LoadPreferences() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("LoadPreferences() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestLoadPreferencesRejectsInvalidTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("api_id = [\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := LoadPreferences(path); err == nil {
		t.Fatal("LoadPreferences() error = nil, want invalid TOML error")
	}
}

func TestLoadPreferencesReturnsReadErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	if _, err := LoadPreferences(path); err == nil {
		t.Fatal("LoadPreferences() error = nil, want read error")
	}
}

func TestSavePreferencesCreatesPrivateFilesWithoutTemporaryResidue(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "new-parent")
	path := filepath.Join(parent, "config.toml")

	if err := SavePreferences(path, Preferences{APIID: 12345, ImageProtocol: "kitty"}); err != nil {
		t.Fatalf("SavePreferences() error = %v", err)
	}

	parentInfo, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("Stat(parent) error = %v", err)
	}
	if got, want := parentInfo.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Fatalf("parent mode = %o, want %o", got, want)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(config) error = %v", err)
	}
	if got, want := fileInfo.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("config mode = %o, want %o", got, want)
	}

	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.toml" {
		t.Fatalf("directory entries = %v, want only config.toml", entryNames(entries))
	}
}

func TestSavePreferencesReplacesExistingFileWithPrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("api_id = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	want := Preferences{APIID: 54321, ImageProtocol: "kitty"}
	if err := SavePreferences(path, want); err != nil {
		t.Fatalf("SavePreferences() error = %v", err)
	}

	got, err := LoadPreferences(path)
	if err != nil {
		t.Fatalf("LoadPreferences() error = %v", err)
	}
	if got != want {
		t.Fatalf("LoadPreferences() = %#v, want %#v", got, want)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(config) error = %v", err)
	}
	if got, want := fileInfo.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("config mode = %o, want %o", got, want)
	}
}

func TestSavePreferencesRemovesTemporaryFileWhenRenameFails(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "config.toml")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	err := SavePreferences(destination, Preferences{APIID: 12345, ImageProtocol: "kitty"})
	if err == nil {
		t.Fatal("SavePreferences() error = nil, want rename error")
	}

	destinationInfo, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("Stat(destination) error = %v", err)
	}
	if !destinationInfo.IsDir() {
		t.Fatalf("destination mode = %v, want directory", destinationInfo.Mode())
	}

	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".config.toml-") {
			t.Fatalf("temporary file remains after rename error: %q", entry.Name())
		}
	}
	if len(entries) != 1 || entries[0].Name() != "config.toml" {
		t.Fatalf("directory entries = %v, want only config.toml directory", entryNames(entries))
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names
}
