package platform

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode"
)

var (
	ErrExternalOpenerUnavailable = errors.New("external opener unavailable")
	ErrExternalOpenFailed        = errors.New("external open failed")
)

// externalOpenerEnvironmentKeys is the minimum process environment needed by
// xdg-open and the desktop launchers it delegates to. In particular, keep this
// as an allow-list so Telegram credentials and unrelated application secrets
// are never inherited by an external media viewer.
var externalOpenerEnvironmentKeys = []string{
	"PATH",
	"HOME",
	"USER",
	"LOGNAME",
	"LANG",
	"LANGUAGE",
	"LC_ALL",
	"LC_CTYPE",
	"LC_MESSAGES",
	"DISPLAY",
	"WAYLAND_DISPLAY",
	"XAUTHORITY",
	"XDG_RUNTIME_DIR",
	"DBUS_SESSION_BUS_ADDRESS",
	"XDG_CURRENT_DESKTOP",
	"XDG_SESSION_DESKTOP",
	"XDG_SESSION_TYPE",
	"DESKTOP_SESSION",
	"DE",
	"DESKTOP",
	"KDE_FULL_SESSION",
	"GNOME_DESKTOP_SESSION_ID",
	"XDG_CONFIG_HOME",
	"XDG_CONFIG_DIRS",
	"XDG_DATA_HOME",
	"XDG_DATA_DIRS",
	"BROWSER",
	"GDK_BACKEND",
	"MOZ_ENABLE_WAYLAND",
	"QT_QPA_PLATFORM",
}

// OpenFile opens a local file with the system-default external application.
func OpenFile(ctx context.Context, localPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", localPath)
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/c", "start", "", localPath)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", localPath)
	}
	// If the opener binary is not found, report unavailable rather than failing.
	if _, err := exec.LookPath(cmd.Path); err != nil {
		return ErrExternalOpenerUnavailable
	}
	cmd.Env = externalOpenerEnvironment()
	if err := cmd.Run(); err != nil {
		return ErrExternalOpenFailed
	}
	return nil
}

// OpenURL launches a web link without invoking a shell or inheriting secrets.
func OpenURL(ctx context.Context, target string) error {
	// TDLib marks scheme-less domains too (e.g. example.com). Keep the
	// original text for copying; only the desktop opener needs an absolute URL.
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "" || strings.Contains(parsed.Scheme, ".") {
		// url.Parse mistakes the hostname in example.com:8080 for a scheme.
		target = "https://" + target
		parsed, err = url.Parse(target)
	}
	if err != nil || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) || parsed.Hostname() == "" || strings.IndexFunc(target, unicode.IsControl) >= 0 {
		return ErrExternalOpenFailed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", target)
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", target)
	default:
		// On generic desktops xdg-open runs the browser in the foreground;
		// waiting for it can keep the effect command pending until the browser exits.
		// gio uses the registered default URL handler and returns after launch.
		if path, err := exec.LookPath("gio"); err == nil {
			cmd = exec.CommandContext(ctx, path, "open", target)
		} else {
			cmd = exec.CommandContext(ctx, "xdg-open", target)
			if _, err := exec.LookPath(cmd.Path); err != nil {
				return ErrExternalOpenerUnavailable
			}
			cmd.Env = externalOpenerEnvironment()
			if err := cmd.Start(); err != nil {
				return ErrExternalOpenFailed
			}
			go func() { _ = cmd.Wait() }() // reap without waiting for the browser to close
			return nil
		}
	}
	if _, err := exec.LookPath(cmd.Path); err != nil {
		return ErrExternalOpenerUnavailable
	}
	cmd.Env = externalOpenerEnvironment()
	if err := cmd.Run(); err != nil {
		return ErrExternalOpenFailed
	}
	return nil
}

func externalOpenerEnvironment() []string {
	environment := make([]string, 0, len(externalOpenerEnvironmentKeys))
	for _, key := range externalOpenerEnvironmentKeys {
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}
