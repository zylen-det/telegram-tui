package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/blacktop/go-termimg"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/buildinfo"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/frontend"
	"github.com/zylen-det/telegram-tui/internal/logging"
	"github.com/zylen-det/telegram-tui/internal/media/avatar"
	"github.com/zylen-det/telegram-tui/internal/media/kitty"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
	"github.com/zylen-det/telegram-tui/internal/platform"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

type appOptions struct {
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	clipboard platform.Clipboard
}

var startApplication = productionApplication

const helpText = `Usage: telegram-tui [options]

Options:
  -h, --help       Show help
  -v, --version    Show version`

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 1 {
		switch args[0] {
		case "-h", "--help":
			_, _ = fmt.Fprintln(stdout, helpText)
			return 0
		case "-v", "--version":
			_, _ = fmt.Fprintln(stdout, buildinfo.Current().Version)
			return 0
		}
	}
	if len(args) != 0 {
		_, _ = fmt.Fprintln(stderr, "telegram-tui supports only -h, --help, -v, and --version; credentials belong in the TUI, environment, or local config")
		return 1
	}
	if err := startApplication(ctx, appOptions{stdin: stdin, stdout: stdout, stderr: stderr}); err != nil {
		var alreadyRunning *platform.AlreadyRunningError
		if errors.As(err, &alreadyRunning) {
			_, _ = fmt.Fprintln(stderr, "another telegram-tui instance is already running; close it normally before retrying")
			return 1
		}
		_, _ = fmt.Fprintln(stderr, "telegram-tui could not start or close safely; verify the local config, Kitty, and TDLib setup")
		return 1
	}
	return 0
}

func productionApplication(parent context.Context, options appOptions) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	paths := config.ResolvePaths(os.Getenv, home)
	if err := ensureRuntimeDirectories(paths); err != nil {
		return err
	}
	return withInstanceOwnership(paths.StateDir, func() error {
		return runOwnedProductionApplication(parent, options, paths)
	})
}

func withInstanceOwnership(stateDir string, start func() error) (resultErr error) {
	lock, err := platform.AcquireInstanceLock(stateDir)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, lock.Close()) }()
	return start()
}

func runOwnedProductionApplication(parent context.Context, options appOptions, paths config.Paths) error {
	logger, logCloser, err := logging.New(filepath.Join(paths.StateDir, "telegram-tui.log"), slog.LevelInfo)
	if err != nil {
		return err
	}
	defer logCloser.Close()
	slog.SetDefault(logger)
	logger.Info("startup", logging.Attrs(logging.Fields{Operation: "startup"})...)

	broker := auth.NewBroker()
	resolver := config.Resolver{Paths: paths, Prompts: broker, Getenv: os.Getenv}
	cache := pixel.NewCache(paths.AvatarCacheDir)
	avatarRenderer := avatar.Renderer{Cache: cache, Theme: "dark", Background: color.NRGBA{R: 15, G: 17, B: 20, A: 255}}
	ctx, cancel := context.WithCancel(parent)
	protocol := resolveThumbnailProtocol(paths)
	os.Setenv("TERMIMG_BYPASS_DETECTION", thumbnailProtocolBypass(protocol))
	handler := newProductionHandler(ctx, resolver, telegram.New, broker, avatarRenderer, options.clipboard, logger, protocol)
	model, err := frontend.NewAppModel(frontend.InitialState(), handler)
	if err != nil {
		cancel()
		return err
	}

	images := kitty.NewManager(options.stdout)
	cellPixels := func(int, int) image.Point { return image.Pt(1, 2) }
	if file, ok := options.stdout.(*os.File); ok {
		cellPixels = frontend.LinuxCellPixelsForFD(file.Fd())
	}
	output := frontend.NewOutputOverlay(options.stdout, images, cellPixels)
	model.SetOutputOverlay(output)
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(options.stdin),
		tea.WithOutput(output),
		tea.WithoutSignalHandler(),
	)
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	return runProductionBubbleTea(parent, cancel, output, program, model, signals)
}

func newProductionHandler(ctx context.Context, resolver frontend.RuntimeResolver, factory frontend.ClientFactory, broker *auth.Broker, avatars frontend.AvatarRenderer, clipboard platform.Clipboard, logger *slog.Logger, protocol termimg.Protocol) *frontend.Handler {
	if clipboard == nil {
		clipboard = platform.NewProductionClipboard()
	}
	handler := frontend.NewHandler(ctx, resolver, factory, broker, avatars, clipboard)
	handler.SetThumbnailRenderer(thumbnail.NewRenderer(protocol))
	handler.SetLogger(logger)
	handler.SetNotifier(platform.NewProductionNotifier())
	return handler
}

// resolveThumbnailProtocol picks the thumbnail display protocol from the
// image_protocol preference. Kitty is honored only when the terminal actually
// reports Kitty graphics support from its environment so transmit sequences
// never reach an incompatible terminal; everything else falls back to
// half-blocks.
func resolveThumbnailProtocol(paths config.Paths) termimg.Protocol {
	preferences, err := config.LoadPreferences(paths.ConfigFile)
	if err == nil && preferences.ImageProtocol == "kitty" && termimg.DetectKittyFromEnvironment() {
		return termimg.Kitty
	}
	return termimg.Halfblocks
}

// thumbnailProtocolBypass maps a protocol onto the go-termimg detection bypass
// value. The bypass keeps go-termimg from querying the terminal while Bubble
// Tea owns stdin.
func thumbnailProtocolBypass(protocol termimg.Protocol) string {
	if protocol == termimg.Kitty {
		return "kitty"
	}
	return "halfblocks"
}

type productionOverlay interface {
	Clear() error
}

type productionProgram interface {
	Run() (tea.Model, error)
	Send(tea.Msg)
}

type productionStatus interface {
	ShutdownError() error
}

func runProductionBubbleTea(parent context.Context, cancel context.CancelFunc, overlay productionOverlay, program productionProgram, status productionStatus, signals <-chan os.Signal) (resultErr error) {
	stopSignals := make(chan struct{})
	signalsDone := make(chan struct{})
	go func() {
		defer close(signalsDone)
		for {
			select {
			case <-stopSignals:
				return
			case received, ok := <-signals:
				if !ok {
					return
				}
				switch received {
				case os.Interrupt, syscall.SIGTERM:
					program.Send(frontend.ProcessQuitMsg{})
				}
			}
		}
	}()
	defer func() {
		close(stopSignals)
		<-signalsDone
		cancel()
		resultErr = errors.Join(resultErr, overlay.Clear())
	}()

	_, resultErr = program.Run()
	if errors.Is(resultErr, tea.ErrProgramKilled) && parent.Err() != nil {
		resultErr = nil
	}
	if status != nil {
		resultErr = errors.Join(resultErr, status.ShutdownError())
	}
	return resultErr
}

func ensureRuntimeDirectories(paths config.Paths) error {
	for _, path := range []string{paths.StateDir, paths.DataDir, paths.TDLibDatabase, paths.TDLibFiles, paths.AvatarCacheDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}
