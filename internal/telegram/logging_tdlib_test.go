//go:build tdlib

package telegram

import (
	"errors"
	"testing"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestConfigureTDLibLoggingUsesPrivateStateFileWithoutCapturingApplicationStderr(t *testing.T) {
	var stream *td.SetLogStreamRequest
	var verbosity *td.SetLogVerbosityLevelRequest
	err := configureTDLibLoggingWith("/private/state/telegram-tui/tdlib.log",
		func(request *td.SetLogStreamRequest) (*td.Ok, error) { stream = request; return &td.Ok{}, nil },
		func(request *td.SetLogVerbosityLevelRequest) (*td.Ok, error) {
			verbosity = request
			return &td.Ok{}, nil
		},
	)
	if err != nil {
		t.Fatal("configureTDLibLoggingWith returned an unexpected error")
	}
	file, ok := stream.LogStream.(*td.LogStreamFile)
	if !ok || file.Path != "/private/state/telegram-tui/tdlib.log" || file.MaxFileSize != tdlibLogMaxFileSize || file.RedirectStderr {
		t.Fatalf("TDLib log stream = %#v", stream.LogStream)
	}
	if verbosity == nil || verbosity.NewVerbosityLevel != 2 {
		t.Fatalf("TDLib verbosity = %#v, want warnings", verbosity)
	}
}

func TestConfigureTDLibLoggingReturnsSanitizedErrors(t *testing.T) {
	privateCause := errors.New("private TDLib logging detail")
	for _, test := range []struct {
		name      string
		path      string
		stream    setTDLibLogStreamFunc
		verbosity setTDLibLogVerbosityFunc
	}{
		{name: "missing path", stream: func(*td.SetLogStreamRequest) (*td.Ok, error) { return &td.Ok{}, nil }, verbosity: func(*td.SetLogVerbosityLevelRequest) (*td.Ok, error) { return &td.Ok{}, nil }},
		{name: "stream", path: "/state/log", stream: func(*td.SetLogStreamRequest) (*td.Ok, error) { return nil, privateCause }, verbosity: func(*td.SetLogVerbosityLevelRequest) (*td.Ok, error) { return &td.Ok{}, nil }},
		{name: "verbosity", path: "/state/log", stream: func(*td.SetLogStreamRequest) (*td.Ok, error) { return &td.Ok{}, nil }, verbosity: func(*td.SetLogVerbosityLevelRequest) (*td.Ok, error) { return nil, privateCause }},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := configureTDLibLoggingWith(test.path, test.stream, test.verbosity)
			var appError domain.AppError
			if !errors.As(err, &appError) || appError.Kind != domain.ErrorStorage || appError.Op != "configure TDLib log" {
				t.Fatalf("error = %#v", err)
			}
			if err != nil && errors.Is(err, privateCause) && err.Error() == privateCause.Error() {
				t.Fatal("TDLib logging error exposed raw cause")
			}
		})
	}
}
