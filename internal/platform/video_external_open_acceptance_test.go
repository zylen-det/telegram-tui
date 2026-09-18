package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVideoExternalOpenAcceptance_SystemDefaultOpenerBoundary(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "opened")
	environment := filepath.Join(dir, "environment")
	writeAcceptanceOpener(t, dir, "printf '%s' \"$1\" > "+strconv.Quote(marker)+"\n/usr/bin/env > "+strconv.Quote(environment)+"\nexit 0\n")
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	t.Setenv("VIDEO_OPEN_PRIVATE_SENTINEL", "must-not-reach-opener")

	localPath := "/tmp/private received clip.mp4"
	if err := OpenFile(context.Background(), localPath); err != nil {
		t.Fatalf("OpenFile() error = %v, want nil", err)
	}
	opened, err := os.ReadFile(marker)
	if err != nil || string(opened) != localPath {
		t.Fatalf("system opener argument = %q, err=%v, want exact local path", string(opened), err)
	}
	envBytes, err := os.ReadFile(environment)
	if err != nil {
		t.Fatalf("read opener environment: %v", err)
	}
	envText := string(envBytes)
	if !strings.Contains(envText, "WAYLAND_DISPLAY=wayland-test") {
		t.Fatal("desktop session environment did not reach the system opener")
	}
	if strings.Contains(envText, "VIDEO_OPEN_PRIVATE_SENTINEL") || strings.Contains(envText, "must-not-reach-opener") {
		t.Fatal("arbitrary private environment reached the system opener")
	}
}

func TestVideoExternalOpenAcceptance_SafeFailureAndCancellation(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PATH", dir)
		privatePath := "/private/video/unavailable.mp4"
		err := OpenFile(context.Background(), privatePath)
		if !errors.Is(err, ErrExternalOpenerUnavailable) || err.Error() != ErrExternalOpenerUnavailable.Error() {
			t.Fatalf("unavailable error = %v, want constant safe error", err)
		}
		if strings.Contains(err.Error(), privatePath) {
			t.Fatal("unavailable error exposed the private local path")
		}
	})

	t.Run("launcher failure", func(t *testing.T) {
		dir := t.TempDir()
		writeAcceptanceOpener(t, dir, "printf 'private-launcher-detail' >&2\nexit 9\n")
		t.Setenv("PATH", dir)
		privatePath := "/private/video/failure.mp4"
		err := OpenFile(context.Background(), privatePath)
		if !errors.Is(err, ErrExternalOpenFailed) || err.Error() != ErrExternalOpenFailed.Error() {
			t.Fatalf("launcher error = %v, want constant safe error", err)
		}
		for _, private := range []string{privatePath, "private-launcher-detail"} {
			if strings.Contains(err.Error(), private) {
				t.Fatalf("launcher error exposed %q", private)
			}
		}
	})

	t.Run("pre-canceled context never launches", func(t *testing.T) {
		dir := t.TempDir()
		marker := filepath.Join(dir, "must-not-exist")
		writeAcceptanceOpener(t, dir, "printf launched > "+strconv.Quote(marker)+"\nexit 0\n")
		t.Setenv("PATH", dir)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := OpenFile(ctx, "/tmp/canceled.mp4")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled error = %v, want context.Canceled", err)
		}
		if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("pre-canceled open launched the opener: %v", statErr)
		}
	})
}

func writeAcceptanceOpener(t *testing.T, dir, body string) {
	t.Helper()
	path := filepath.Join(dir, "xdg-open")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatalf("write xdg-open fixture: %v", err)
	}
}
