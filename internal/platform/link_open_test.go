package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOpenURLSystemOpenerAndSafeFailures(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("xdg-open test")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "opened")
	environment := filepath.Join(dir, "environment")
	writeLinkLauncher(t, dir, "gio", "test \"$1\" = open || exit 9\nprintf '%s' \"$2\" > "+strconv.Quote(marker)+"\n/usr/bin/env > "+strconv.Quote(environment)+"\n")
	t.Setenv("PATH", dir)
	t.Setenv("TELEGRAM_API_HASH", "private-link-secret")
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	t.Setenv("MOZ_ENABLE_WAYLAND", "1")
	target := "https://example.org/path?q=one&x=two"
	if err := OpenURL(context.Background(), target); err != nil {
		t.Fatalf("open URL: %v", err)
	}
	opened, err := os.ReadFile(marker)
	if err != nil || string(opened) != target {
		t.Fatalf("opener argument = %q, err=%v", opened, err)
	}
	env, err := os.ReadFile(environment)
	if err != nil || !strings.Contains(string(env), "WAYLAND_DISPLAY=wayland-test") || !strings.Contains(string(env), "MOZ_ENABLE_WAYLAND=1") || strings.Contains(string(env), "private-link-secret") {
		t.Fatalf("opener environment did not respect allow-list: %v", err)
	}
	for _, invalid := range []string{"javascript:alert(1)", "file:///etc/passwd", "https://example.org\nX", "https://"} {
		if err := OpenURL(context.Background(), invalid); !errors.Is(err, ErrExternalOpenFailed) {
			t.Errorf("unsafe target %q: %v", invalid, err)
		}
	}
	for _, bare := range []string{"www.one.example", "example.com", "example.com:8080/path"} {
		if err := OpenURL(context.Background(), bare); err != nil {
			t.Fatalf("open TDLib URL %q: %v", bare, err)
		}
		if opened, err := os.ReadFile(marker); err != nil || string(opened) != "https://"+bare {
			t.Fatalf("URL %q opener argument = %q, err=%v", bare, opened, err)
		}
	}
	writeLinkLauncher(t, dir, "gio", "exit 9\n")
	if err := OpenURL(context.Background(), target); err == nil || !errors.Is(err, ErrExternalOpenFailed) || strings.Contains(err.Error(), target) {
		t.Fatalf("launcher failure = %v", err)
	}
}

func TestOpenURLFallbackDoesNotWaitForBrowserExit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("xdg-open fallback test")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "started")
	writeLinkLauncher(t, dir, "xdg-open", "printf '%s' \"$1\" > "+strconv.Quote(marker)+"\nexec /bin/sleep 10\n")
	t.Setenv("PATH", dir) // no gio
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	if err := OpenURL(ctx, "https://example.org"); err != nil {
		t.Fatalf("xdg-open fallback: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("opening URL waited for browser exit")
	}
	deadline := time.After(time.Second)
	for {
		if opened, err := os.ReadFile(marker); err == nil && string(opened) == "https://example.org" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("fallback did not launch the selected URL")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func writeLinkLauncher(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatalf("write launcher: %v", err)
	}
}
