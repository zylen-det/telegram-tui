package platform

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var (
	ErrExternalOpenerUnavailable = errors.New("external opener unavailable")
	ErrExternalOpenFailed        = errors.New("external open failed")
)

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
	// Filter environment: pass desktop session vars but exclude private prefixes.
	filteredEnv := make([]string, 0, len(os.Environ()))
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "VIDEO_OPEN_") {
			continue
		}
		filteredEnv = append(filteredEnv, env)
	}
	cmd.Env = filteredEnv
	if err := cmd.Run(); err != nil {
		return ErrExternalOpenFailed
	}
	return nil
}
