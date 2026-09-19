package platform

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
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

func externalOpenerEnvironment() []string {
	environment := make([]string, 0, len(externalOpenerEnvironmentKeys))
	for _, key := range externalOpenerEnvironmentKeys {
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}
