package platform

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Clipboard interface {
	WriteText(context.Context, string) error
}

var (
	ErrClipboardUnavailable = errors.New("clipboard unavailable")
	ErrClipboardWriteFailed = errors.New("clipboard write failed")
)

const clipboardTimeout = 2 * time.Second

type clipboardCommandRunner func(context.Context, string, []string, io.Reader, []string) error

type ProductionClipboard struct {
	getenv  func(string) string
	lookup  func(string) (string, error)
	run     clipboardCommandRunner
	timeout time.Duration
}

func NewProductionClipboard() *ProductionClipboard {
	return &ProductionClipboard{
		getenv:  os.Getenv,
		lookup:  exec.LookPath,
		run:     runClipboardCommand,
		timeout: clipboardTimeout,
	}
}

func (c *ProductionClipboard) WriteText(ctx context.Context, text string) error {
	name, args, ok := c.backend()
	if !ok {
		return ErrClipboardUnavailable
	}
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := c.run(commandCtx, name, args, strings.NewReader(text), c.environment()); err != nil {
		return ErrClipboardWriteFailed
	}
	return nil
}

func (c *ProductionClipboard) backend() (string, []string, bool) {
	// WAYLAND_SOCKET refers to a pre-opened file descriptor. This adapter does
	// not inherit arbitrary descriptors, so only select wl-copy when a named
	// WAYLAND_DISPLAY connection is available; otherwise try X11 backends.
	waylandSupported := c.getenv("WAYLAND_DISPLAY") != ""
	candidates := []struct {
		enabled bool
		name    string
		args    []string
	}{
		{enabled: waylandSupported, name: "wl-copy"},
		{enabled: true, name: "xclip", args: []string{"-selection", "clipboard"}},
		{enabled: true, name: "xsel", args: []string{"--clipboard", "--input"}},
	}
	for _, candidate := range candidates {
		if !candidate.enabled {
			continue
		}
		path, err := c.lookup(candidate.name)
		if err == nil {
			return path, candidate.args, true
		}
	}
	return "", nil, false
}

func (c *ProductionClipboard) environment() []string {
	keys := []string{"PATH", "HOME", "LANG", "LC_ALL", "DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR"}
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		if value := c.getenv(key); value != "" {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}

func runClipboardCommand(ctx context.Context, name string, args []string, stdin io.Reader, environment []string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = stdin
	command.Env = environment
	return command.Run()
}

func newProductionClipboardForTest(environment, paths map[string]string, runner clipboardCommandRunner, timeout time.Duration) *ProductionClipboard {
	return &ProductionClipboard{
		getenv: func(key string) string { return environment[key] },
		lookup: func(name string) (string, error) {
			if path := paths[name]; path != "" {
				return path, nil
			}
			return "", exec.ErrNotFound
		},
		run:     runner,
		timeout: timeout,
	}
}

// FakeClipboard records only invocation count so test diagnostics stay content-opaque.
type FakeClipboard struct {
	Writes  int
	Err     error
	Matches func(string) bool
	Matched bool
}

func (f *FakeClipboard) WriteText(_ context.Context, text string) error {
	f.Writes++
	if f.Matches != nil {
		f.Matched = f.Matches(text)
	}
	return f.Err
}
