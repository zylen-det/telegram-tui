package platform

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClipboardFakeRecordsWriteWithoutExposingContent(t *testing.T) {
	fake := &FakeClipboard{}
	if err := fake.WriteText(context.Background(), "private"); err != nil {
		t.Fatal("clipboard fake returned an error")
	}
	if fake.Writes != 1 {
		t.Fatalf("clipboard writes = %d", fake.Writes)
	}
	var clipboard Clipboard = fake
	if clipboard == nil {
		t.Fatal("fake does not satisfy clipboard boundary")
	}
}

func TestProductionClipboardBackendPreferenceAndStdinOnly(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		paths    map[string]string
		wantName string
		wantArgs []string
	}{
		{name: "wayland", env: map[string]string{"WAYLAND_DISPLAY": "wayland-1"}, paths: map[string]string{"wl-copy": "/usr/bin/wl-copy", "xclip": "/usr/bin/xclip"}, wantName: "/usr/bin/wl-copy"},
		{name: "inherited wayland socket falls back", env: map[string]string{"WAYLAND_SOCKET": "9"}, paths: map[string]string{"wl-copy": "/usr/bin/wl-copy", "xclip": "/usr/bin/xclip"}, wantName: "/usr/bin/xclip", wantArgs: []string{"-selection", "clipboard"}},
		{name: "xclip fallback", env: map[string]string{}, paths: map[string]string{"xclip": "/usr/bin/xclip", "xsel": "/usr/bin/xsel"}, wantName: "/usr/bin/xclip", wantArgs: []string{"-selection", "clipboard"}},
		{name: "xsel fallback", env: map[string]string{}, paths: map[string]string{"xsel": "/usr/bin/xsel"}, wantName: "/usr/bin/xsel", wantArgs: []string{"--clipboard", "--input"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matched := false
			runner := func(_ context.Context, name string, args []string, stdin io.Reader, env []string) error {
				if name != test.wantName || !reflect.DeepEqual(args, test.wantArgs) {
					t.Fatalf("backend = %q %#v, want %q %#v", name, args, test.wantName, test.wantArgs)
				}
				payload, err := io.ReadAll(stdin)
				if err != nil {
					t.Fatal("could not inspect stdin")
				}
				matched = string(payload) == "private message"
				for _, arg := range args {
					if arg == "private message" {
						t.Fatal("clipboard content entered argv")
					}
				}
				return nil
			}
			clipboard := newProductionClipboardForTest(test.env, test.paths, runner, time.Second)
			if err := clipboard.WriteText(context.Background(), "private message"); err != nil {
				t.Fatalf("WriteText returned safe failure: %v", err)
			}
			if !matched {
				t.Fatal("clipboard content did not arrive through stdin")
			}
		})
	}
}

func TestProductionClipboardDiscoversBackendAtExecutionTime(t *testing.T) {
	paths := map[string]string{}
	runs := 0
	clipboard := newProductionClipboardForTest(map[string]string{"WAYLAND_DISPLAY": "wayland-1"}, paths, func(context.Context, string, []string, io.Reader, []string) error {
		runs++
		return nil
	}, time.Second)
	if err := clipboard.WriteText(context.Background(), "opaque"); !errors.Is(err, ErrClipboardUnavailable) {
		t.Fatal("missing backend did not return the constant unavailable error")
	}
	paths["wl-copy"] = "/usr/bin/wl-copy"
	if err := clipboard.WriteText(context.Background(), "opaque"); err != nil || runs != 1 {
		t.Fatal("backend added after construction was not discovered at execution time")
	}
}

func TestProductionClipboardBoundsExecutionAndSanitizesFailure(t *testing.T) {
	private := "private clipboard content"
	clipboard := newProductionClipboardForTest(map[string]string{"DISPLAY": ":1", "SECRET_TOKEN": private}, map[string]string{"xclip": "/usr/bin/xclip"}, func(ctx context.Context, _ string, _ []string, _ io.Reader, env []string) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("clipboard command context has no deadline")
		}
		for _, value := range env {
			if value == "SECRET_TOKEN="+private {
				t.Fatal("unrelated environment secret was inherited")
			}
		}
		return errors.New(private)
	}, 20*time.Millisecond)
	err := clipboard.WriteText(context.Background(), private)
	if !errors.Is(err, ErrClipboardWriteFailed) || err.Error() != ErrClipboardWriteFailed.Error() {
		t.Fatal("runner failure was not replaced by the constant safe error")
	}
}

func TestProductionClipboardCommandRunnerExecutesWithoutShellAndHonorsCancellation(t *testing.T) {
	wc, err := exec.LookPath("wc")
	if err != nil {
		t.Skip("wc is unavailable")
	}
	if err := runClipboardCommand(context.Background(), wc, []string{"-c"}, strings.NewReader("opaque"), []string{"LANG=C"}); err != nil {
		t.Fatal("fixed executable did not consume stdin")
	}

	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := runClipboardCommand(ctx, sleep, []string{"10"}, strings.NewReader(""), []string{"LANG=C"}); err == nil || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("fixed executable ignored bounded context cancellation")
	}
}
