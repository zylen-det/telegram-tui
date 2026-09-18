package pixel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
)

type Key struct {
	UniqueID        string
	PixelWidth      int
	PixelHeight     int
	Theme           string
	RendererVersion int
}

type Cache struct {
	directory string
}

func NewCache(directory string) *Cache {
	return &Cache{directory: directory}
}

func (c *Cache) Get(key Key) (Avatar, bool, error) {
	path, err := c.path(key)
	if err != nil {
		return Avatar{}, false, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Avatar{}, false, nil
	}
	if err != nil {
		return Avatar{}, false, fmt.Errorf("read cached avatar: %w", err)
	}

	var avatar Avatar
	if err := json.Unmarshal(data, &avatar); err != nil {
		return Avatar{}, false, fmt.Errorf("decode cached avatar: %w", err)
	}
	if err := validateAvatar(avatar); err != nil {
		return Avatar{}, false, fmt.Errorf("validate cached avatar: %w", err)
	}
	return avatar, true, nil
}

func (c *Cache) Put(key Key, avatar Avatar) error {
	if err := validateAvatar(avatar); err != nil {
		return fmt.Errorf("validate avatar for cache: %w", err)
	}
	data, err := json.Marshal(avatar)
	if err != nil {
		return fmt.Errorf("encode cached avatar: %w", err)
	}
	path, err := c.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.directory, 0o700); err != nil {
		return fmt.Errorf("create avatar cache directory: %w", err)
	}
	if err := os.Chmod(c.directory, 0o700); err != nil {
		return fmt.Errorf("set avatar cache directory permissions: %w", err)
	}

	temporary, err := os.CreateTemp(c.directory, ".avatar-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary avatar cache file: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	committed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("set temporary avatar cache permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary avatar cache file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary avatar cache file: %w", err)
	}
	closeErr := temporary.Close()
	closed = true
	if closeErr != nil {
		return fmt.Errorf("close temporary avatar cache file: %w", closeErr)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace cached avatar: %w", err)
	}
	committed = true
	return nil
}

func (c *Cache) path(key Key) (string, error) {
	encoded, err := json.Marshal(key)
	if err != nil {
		return "", fmt.Errorf("encode avatar cache key: %w", err)
	}
	digest := sha256.Sum256(encoded)
	filename := hex.EncodeToString(digest[:]) + ".json"
	return filepath.Join(c.directory, filename), nil
}

func validateAvatar(avatar Avatar) error {
	if avatar.Width <= 0 {
		return fmt.Errorf("avatar width must be positive: %d", avatar.Width)
	}
	if avatar.Height <= 0 {
		return fmt.Errorf("avatar height must be positive: %d", avatar.Height)
	}
	if avatar.Width > math.MaxInt/avatar.Height {
		return fmt.Errorf("avatar cell count overflows dimensions: %dx%d", avatar.Width, avatar.Height)
	}
	cellCount := avatar.Width * avatar.Height
	if cellCount != len(avatar.Cells) {
		return fmt.Errorf("avatar cell count is %d, want %d", len(avatar.Cells), cellCount)
	}
	return nil
}
