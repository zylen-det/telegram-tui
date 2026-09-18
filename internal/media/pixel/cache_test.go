package pixel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestCacheRoundTrip(t *testing.T) {
	cache := NewCache(filepath.Join(t.TempDir(), "avatars"))
	key := Key{UniqueID: "telegram-file-17", PixelWidth: 4, PixelHeight: 6, Theme: "dark", RendererVersion: 3}
	want := Avatar{
		Width:  2,
		Height: 1,
		Cells: []Cell{
			{Rune: '▀', Foreground: color.NRGBA{R: 1, G: 2, B: 3, A: 255}, Background: color.NRGBA{R: 4, G: 5, B: 6, A: 255}},
			{Rune: '▀', Foreground: color.NRGBA{R: 7, G: 8, B: 9, A: 255}, Background: color.NRGBA{R: 10, G: 11, B: 12, A: 255}},
		},
	}

	if err := cache.Put(key, want); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, found, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !found {
		t.Fatal("Get() found = false, want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Get() avatar = %#v, want %#v", got, want)
	}
}

func TestCacheGetMissing(t *testing.T) {
	cache := NewCache(filepath.Join(t.TempDir(), "does-not-exist"))

	got, found, err := cache.Get(Key{UniqueID: "missing"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Fatal("Get() found = true, want false")
	}
	if !reflect.DeepEqual(got, Avatar{}) {
		t.Fatalf("Get() avatar = %#v, want zero Avatar", got)
	}
}

func TestCacheGetInvalidJSON(t *testing.T) {
	directory := t.TempDir()
	key := Key{UniqueID: "corrupt", PixelWidth: 2, PixelHeight: 2, Theme: "dark", RendererVersion: 1}
	path := filepath.Join(directory, expectedCacheFilename(t, key))
	if err := os.WriteFile(path, []byte(`{"Width":2`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, found, err := NewCache(directory).Get(key)
	if err == nil {
		t.Fatal("Get() error = nil, want JSON error")
	}
	if found {
		t.Fatal("Get() found = true, want false for corrupt JSON")
	}
	if !reflect.DeepEqual(got, Avatar{}) {
		t.Fatalf("Get() avatar = %#v, want zero Avatar", got)
	}
}

func TestCacheGetRejectsSemanticallyInvalidAvatar(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{name: "null", json: `null`},
		{name: "zero width", json: `{"Width":0,"Height":1,"Cells":[]}`},
		{name: "negative width", json: `{"Width":-1,"Height":1,"Cells":[]}`},
		{name: "zero height", json: `{"Width":1,"Height":0,"Cells":[]}`},
		{name: "negative height", json: `{"Width":1,"Height":-1,"Cells":[]}`},
		{name: "cell count mismatch", json: `{"Width":2,"Height":1,"Cells":[{}]}`},
		{name: "dimension overflow", json: fmt.Sprintf(`{"Width":%d,"Height":2,"Cells":[]}`, math.MaxInt)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directory := t.TempDir()
			cache := NewCache(directory)
			key := Key{UniqueID: "secret-uid", Theme: "secret-theme"}
			path, err := cache.path(key)
			if err != nil {
				t.Fatalf("path() error = %v", err)
			}
			if err := os.WriteFile(path, []byte(tt.json), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			got, found, err := cache.Get(key)
			if err == nil {
				t.Fatal("Get() error = nil, want semantic validation error")
			}
			if found {
				t.Fatal("Get() found = true, want false")
			}
			if !reflect.DeepEqual(got, Avatar{}) {
				t.Fatalf("Get() avatar = %#v, want zero Avatar", got)
			}
			if strings.Contains(err.Error(), key.UniqueID) || strings.Contains(err.Error(), key.Theme) || strings.Contains(err.Error(), tt.json) {
				t.Fatalf("Get() error exposes cache input: %q", err)
			}
		})
	}
}

func TestCachePutRejectsSemanticallyInvalidAvatar(t *testing.T) {
	tests := []struct {
		name   string
		avatar Avatar
	}{
		{name: "zero width", avatar: Avatar{Height: 1}},
		{name: "negative width", avatar: Avatar{Width: -1, Height: 1}},
		{name: "zero height", avatar: Avatar{Width: 1}},
		{name: "negative height", avatar: Avatar{Width: 1, Height: -1}},
		{name: "cell count mismatch", avatar: Avatar{Width: 2, Height: 1, Cells: []Cell{{}}}},
		{name: "dimension overflow", avatar: Avatar{Width: math.MaxInt, Height: 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "cache")
			cache := NewCache(directory)
			key := Key{UniqueID: "secret-uid", Theme: "secret-theme"}
			path, err := cache.path(key)
			if err != nil {
				t.Fatalf("path() error = %v", err)
			}

			err = cache.Put(key, tt.avatar)
			if err == nil {
				t.Fatal("Put() error = nil, want semantic validation error")
			}
			if strings.Contains(err.Error(), key.UniqueID) || strings.Contains(err.Error(), key.Theme) {
				t.Fatalf("Put() error exposes cache key: %q", err)
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("Stat(cache path) error = %v, want not exist", statErr)
			}
		})
	}
}

func TestCacheKeyFieldsAreIsolated(t *testing.T) {
	cache := NewCache(t.TempDir())
	keys := []Key{
		{UniqueID: "uid", PixelWidth: 2, PixelHeight: 4, Theme: "dark", RendererVersion: 1},
		{UniqueID: "other", PixelWidth: 2, PixelHeight: 4, Theme: "dark", RendererVersion: 1},
		{UniqueID: "uid", PixelWidth: 3, PixelHeight: 4, Theme: "dark", RendererVersion: 1},
		{UniqueID: "uid", PixelWidth: 2, PixelHeight: 6, Theme: "dark", RendererVersion: 1},
		{UniqueID: "uid", PixelWidth: 2, PixelHeight: 4, Theme: "light", RendererVersion: 1},
		{UniqueID: "uid", PixelWidth: 2, PixelHeight: 4, Theme: "dark", RendererVersion: 2},
	}

	for i, key := range keys {
		avatar := Avatar{Width: 1, Height: 1, Cells: []Cell{{Rune: rune('a' + i)}}}
		if err := cache.Put(key, avatar); err != nil {
			t.Fatalf("Put(key %d) error = %v", i, err)
		}
	}
	for i, key := range keys {
		got, found, err := cache.Get(key)
		if err != nil {
			t.Fatalf("Get(key %d) error = %v", i, err)
		}
		want := Avatar{Width: 1, Height: 1, Cells: []Cell{{Rune: rune('a' + i)}}}
		if !found || !reflect.DeepEqual(got, want) {
			t.Errorf("Get(key %d) = (%#v, %v), want (%#v, true)", i, got, found, want)
		}
	}
}

func TestCacheSeparatesLegacyDelimiterCollision(t *testing.T) {
	keyA := Key{UniqueID: "a:1", PixelWidth: 2, PixelHeight: 4, Theme: "x", RendererVersion: 5}
	keyB := Key{UniqueID: "a", PixelWidth: 1, PixelHeight: 2, Theme: "4:x", RendererVersion: 5}
	legacyA := fmt.Sprintf("%s:%d:%d:%s:%d", keyA.UniqueID, keyA.PixelWidth, keyA.PixelHeight, keyA.Theme, keyA.RendererVersion)
	legacyB := fmt.Sprintf("%s:%d:%d:%s:%d", keyB.UniqueID, keyB.PixelWidth, keyB.PixelHeight, keyB.Theme, keyB.RendererVersion)
	if legacyA != legacyB {
		t.Fatalf("test setup does not collide: %q != %q", legacyA, legacyB)
	}
	if expectedCacheFilename(t, keyA) == expectedCacheFilename(t, keyB) {
		t.Fatal("structured key hashes collided")
	}

	cache := NewCache(t.TempDir())
	wantA := Avatar{Width: 1, Height: 1, Cells: []Cell{{Rune: 'A'}}}
	wantB := Avatar{Width: 1, Height: 1, Cells: []Cell{{Rune: 'B'}}}
	if err := cache.Put(keyA, wantA); err != nil {
		t.Fatalf("Put(keyA) error = %v", err)
	}
	if err := cache.Put(keyB, wantB); err != nil {
		t.Fatalf("Put(keyB) error = %v", err)
	}
	assertCachedAvatar(t, cache, keyA, wantA)
	assertCachedAvatar(t, cache, keyB, wantB)
}

func TestCacheFilenameIsHashWithoutRawKeyData(t *testing.T) {
	directory := t.TempDir()
	key := Key{UniqueID: "raw-user-id", PixelWidth: 2, PixelHeight: 4, Theme: "raw-theme", RendererVersion: 1}
	if err := NewCache(directory).Put(key, validAvatar()); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("cache entries = %d, want 1", len(entries))
	}
	name := entries[0].Name()
	if !regexp.MustCompile(`^[0-9a-f]{64}\.json$`).MatchString(name) {
		t.Fatalf("cache filename = %q, want SHA-256 hex plus .json", name)
	}
	if strings.Contains(name, key.UniqueID) || strings.Contains(name, key.Theme) {
		t.Fatalf("cache filename %q contains raw key data", name)
	}
	if name != expectedCacheFilename(t, key) {
		t.Fatalf("cache filename = %q, want %q", name, expectedCacheFilename(t, key))
	}
}

func TestCacheCreatesPrivateDirectoryAndFile(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "nested", "cache")
	key := Key{UniqueID: "permissions"}
	if err := NewCache(directory).Put(key, validAvatar()); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	dirInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatalf("Stat(directory) error = %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("directory mode = %04o, want 0700", got)
	}
	fileInfo, err := os.Stat(filepath.Join(directory, expectedCacheFilename(t, key)))
	if err != nil {
		t.Fatalf("Stat(cache file) error = %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("cache file mode = %04o, want 0600", got)
	}
}

func TestCacheTightensExistingDirectoryPermissions(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "avatars")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	key := Key{UniqueID: "existing-permissions"}

	if err := NewCache(directory).Put(key, validAvatar()); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	dirInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatalf("Stat(directory) error = %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("directory mode = %04o, want 0700", got)
	}
	fileInfo, err := os.Stat(filepath.Join(directory, expectedCacheFilename(t, key)))
	if err != nil {
		t.Fatalf("Stat(cache file) error = %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("cache file mode = %04o, want 0600", got)
	}
}

func TestCacheOverwriteReplacesContentAndMode(t *testing.T) {
	directory := t.TempDir()
	key := Key{UniqueID: "overwrite"}
	path := filepath.Join(directory, expectedCacheFilename(t, key))
	if err := os.WriteFile(path, []byte(`{"Width":99}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	want := Avatar{Width: 1, Height: 1, Cells: []Cell{{Rune: '▀'}}}

	cache := NewCache(directory)
	if err := cache.Put(key, want); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	assertCachedAvatar(t, cache, key, want)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("overwritten cache file mode = %04o, want 0600", got)
	}
}

func TestCacheRenameFailureRemovesTemporaryFile(t *testing.T) {
	directory := t.TempDir()
	key := Key{UniqueID: "rename-failure"}
	destination := filepath.Join(directory, expectedCacheFilename(t, key))
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatalf("Mkdir(destination) error = %v", err)
	}

	if err := NewCache(directory).Put(key, validAvatar()); err == nil {
		t.Fatal("Put() error = nil, want rename error")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(destination) || !entries[0].IsDir() {
		t.Fatalf("entries after failed rename = %#v, want only destination directory", entries)
	}
}

func TestCacheSuccessfulPutLeavesNoTemporaryFile(t *testing.T) {
	directory := t.TempDir()
	key := Key{UniqueID: "no-temp"}
	if err := NewCache(directory).Put(key, validAvatar()); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != expectedCacheFilename(t, key) {
		t.Fatalf("entries after Put() = %#v, want only final cache file", entries)
	}
}

func expectedCacheFilename(t *testing.T, key Key) string {
	t.Helper()
	encoded, err := json.Marshal(key)
	if err != nil {
		t.Fatalf("json.Marshal(Key) error = %v", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]) + ".json"
}

func assertCachedAvatar(t *testing.T, cache *Cache, key Key, want Avatar) {
	t.Helper()
	got, found, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !found || !reflect.DeepEqual(got, want) {
		t.Fatalf("Get() = (%#v, %v), want (%#v, true)", got, found, want)
	}
}

func validAvatar() Avatar {
	return Avatar{Width: 1, Height: 1, Cells: []Cell{{Rune: '▀'}}}
}
