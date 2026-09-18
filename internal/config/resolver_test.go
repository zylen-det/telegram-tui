package config

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

type promptResult struct {
	value string
	err   error
}

type scriptedPrompter struct {
	results []promptResult
	prompts []auth.Prompt
}

func (p *scriptedPrompter) Ask(_ context.Context, prompt auth.Prompt) (string, error) {
	p.prompts = append(p.prompts, prompt)
	if len(p.results) == 0 {
		return "", errors.New("unexpected prompt")
	}
	result := p.results[0]
	p.results = p.results[1:]
	return result.value, result.err
}

func TestResolverFirstRunPromptsAndPersistsLocalConfig(t *testing.T) {
	paths := testPaths(t)
	prompts := &scriptedPrompter{results: []promptResult{{value: "12345"}, {value: "first-run-api-hash"}}}
	runtime, err := (Resolver{Paths: paths, Prompts: prompts, Getenv: emptyGetenv}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if runtime.APIID != 12345 || runtime.APIHash != "first-run-api-hash" || len(runtime.DatabaseKey) != databaseKeySize {
		t.Fatal("first-run runtime did not contain prompted credentials and generated database key")
	}
	assertPrompts(t, prompts.prompts, []auth.Prompt{
		{Kind: auth.PromptAPIID, Label: "Telegram API ID"},
		{Kind: auth.PromptAPIHash, Label: "Telegram API hash", Secret: true},
	})
	stored, err := LoadPreferences(paths.ConfigFile)
	if err != nil {
		t.Fatal("could not reload persisted config")
	}
	if stored.APIID != 12345 || stored.APIHash != "first-run-api-hash" || stored.DatabaseKey == "" {
		t.Fatal("local config did not persist all first-run values")
	}
	decoded, err := base64.RawStdEncoding.DecodeString(stored.DatabaseKey)
	if err != nil || !reflect.DeepEqual(decoded, runtime.DatabaseKey) {
		t.Fatal("stored database key does not match runtime database key")
	}
	info, err := os.Stat(paths.ConfigFile)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestResolverRestartUsesStoredConfigWithoutPromptingOrRewritingKey(t *testing.T) {
	paths := testPaths(t)
	wantKey := encodedKey(9, databaseKeySize)
	want := Preferences{APIID: 31415, APIHash: "stored-api-hash", DatabaseKey: wantKey, ImageProtocol: "kitty"}
	if err := SavePreferences(paths.ConfigFile, want); err != nil {
		t.Fatal("could not seed config")
	}
	prompts := &scriptedPrompter{}
	runtime, err := (Resolver{Paths: paths, Prompts: prompts, Getenv: emptyGetenv}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(prompts.prompts) != 0 || runtime.APIID != want.APIID || runtime.APIHash != want.APIHash || runtime.Preferences != want {
		t.Fatal("restart did not reuse stored config")
	}
	for _, value := range runtime.DatabaseKey {
		if value != 9 {
			t.Fatal("restart database key changed")
		}
	}
}

func TestResolverEnvironmentOverridesStoredCredentialsWithoutPersistingOverrides(t *testing.T) {
	paths := testPaths(t)
	stored := Preferences{APIID: 111, APIHash: "stored-api-hash", DatabaseKey: encodedKey(7, databaseKeySize), ImageProtocol: "kitty"}
	if err := SavePreferences(paths.ConfigFile, stored); err != nil {
		t.Fatal("could not seed config")
	}
	environment := map[string]string{"TELEGRAM_API_ID": "222", "TELEGRAM_API_HASH": "environment-api-hash"}
	runtime, err := (Resolver{Paths: paths, Prompts: &scriptedPrompter{}, Getenv: func(key string) string { return environment[key] }}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if runtime.APIID != 222 || runtime.APIHash != "environment-api-hash" {
		t.Fatal("environment credentials did not override stored credentials")
	}
	after, _ := LoadPreferences(paths.ConfigFile)
	if after != stored {
		t.Fatal("one-launch environment override was copied to config")
	}
}

func TestResolverMigratesExistingAPIIDOnlyConfig(t *testing.T) {
	paths := pathsWithAPIID(t, 36708500)
	prompts := &scriptedPrompter{results: []promptResult{{value: "new-api-hash"}}}
	runtime, err := (Resolver{Paths: paths, Prompts: prompts, Getenv: emptyGetenv}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if runtime.APIID != 36708500 || runtime.APIHash != "new-api-hash" || len(runtime.DatabaseKey) != databaseKeySize {
		t.Fatal("API-ID-only migration did not complete")
	}
	assertPrompts(t, prompts.prompts, []auth.Prompt{{Kind: auth.PromptAPIHash, Label: "Telegram API hash", Secret: true}})
	stored, _ := LoadPreferences(paths.ConfigFile)
	if stored.APIHash == "" || stored.DatabaseKey == "" {
		t.Fatal("migration did not persist API hash and database key")
	}
}

func TestResolverRejectsInvalidStoredDatabaseKey(t *testing.T) {
	for _, value := range []string{"%%%", encodedKey(1, databaseKeySize-1)} {
		paths := testPaths(t)
		if err := SavePreferences(paths.ConfigFile, Preferences{APIID: 1, APIHash: "hash", DatabaseKey: value}); err != nil {
			t.Fatal("could not seed invalid key")
		}
		_, err := (Resolver{Paths: paths, Prompts: &scriptedPrompter{}, Getenv: emptyGetenv}).Resolve(context.Background())
		assertAppError(t, err, domain.ErrorStorage, "decode database key", "stored database key is invalid")
		if strings.Contains(err.Error(), value) {
			t.Fatal("database key error exposed stored input")
		}
	}
}

func TestResolverRejectsInvalidAPIIDWithoutEchoingInput(t *testing.T) {
	for _, value := range []string{"private-invalid-id", "2147483648", "0"} {
		paths := testPaths(t)
		_, err := (Resolver{Paths: paths, Prompts: &scriptedPrompter{}, Getenv: func(key string) string {
			if key == "TELEGRAM_API_ID" {
				return value
			}
			return "hash"
		}}).Resolve(context.Background())
		assertAppError(t, err, domain.ErrorConfiguration, "resolve API ID", "Telegram API ID must be a positive integer")
		if strings.Contains(err.Error(), value) {
			t.Fatal("API ID error echoed private input")
		}
	}
}

func TestResolverRejectsEmptyPromptedAPIHash(t *testing.T) {
	paths := pathsWithAPIID(t, 1)
	_, err := (Resolver{Paths: paths, Prompts: &scriptedPrompter{results: []promptResult{{value: ""}}}, Getenv: emptyGetenv}).Resolve(context.Background())
	assertAppError(t, err, domain.ErrorConfiguration, "resolve API hash", "Telegram API hash cannot be empty")
}

func testPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	return Paths{ConfigFile: filepath.Join(root, "config", "config.toml"), StateDir: filepath.Join(root, "state"), DataDir: filepath.Join(root, "data"), TDLibDatabase: filepath.Join(root, "data", "database"), TDLibFiles: filepath.Join(root, "data", "files"), AvatarCacheDir: filepath.Join(root, "cache")}
}

func pathsWithAPIID(t *testing.T, apiID int32) Paths {
	t.Helper()
	paths := testPaths(t)
	if err := SavePreferences(paths.ConfigFile, Preferences{APIID: apiID}); err != nil {
		t.Fatal(err)
	}
	return paths
}

func emptyGetenv(string) string { return "" }

func encodedKey(value byte, length int) string {
	return base64.RawStdEncoding.EncodeToString([]byte(strings.Repeat(string(value), length)))
}

func assertPrompts(t *testing.T, got, want []auth.Prompt) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prompts = %#v, want %#v", got, want)
	}
}

func assertAppError(t *testing.T, err error, kind domain.ErrorKind, operation, message string) {
	t.Helper()
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Kind != kind || appError.Op != operation || appError.Message != message {
		t.Fatalf("error = %#v, want %s/%s/%s", err, kind, operation, message)
	}
}
