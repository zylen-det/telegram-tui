package platform

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// Notifier performs one desktop notification without exposing its content
// through a child process command line.
type Notifier interface {
	Notify(context.Context, Notification) error
}

// Notification carries presentation-only content for one desktop notification.
type Notification struct {
	Title string
	Body  string
}

var (
	ErrNotifierUnavailable = errors.New("desktop notifications unavailable")
	ErrNotifierCallFailed  = errors.New("desktop notification failed")
)

const (
	notificationAppName       = "telegram-tui"
	notificationDisplayMillis = int32(5000)
	notificationCallTimeout   = 2 * time.Second
)

type notificationCall func(context.Context, Notification, int32) error

// ProductionNotifier calls the freedesktop Notifications service directly on
// the session bus.
type ProductionNotifier struct {
	call    notificationCall
	timeout time.Duration
}

func NewProductionNotifier() *ProductionNotifier {
	return &ProductionNotifier{call: notifyFreedesktop, timeout: notificationCallTimeout}
}

func newProductionNotifierForTest(call notificationCall, timeout time.Duration) *ProductionNotifier {
	return &ProductionNotifier{call: call, timeout: timeout}
}

func (n *ProductionNotifier) Notify(ctx context.Context, notification Notification) error {
	if n == nil || n.call == nil {
		return ErrNotifierUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	notification.Title = escapeNotificationMarkup(notification.Title)
	notification.Body = escapeNotificationMarkup(notification.Body)
	err := n.call(callCtx, notification, notificationDisplayMillis)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotifierUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ErrNotifierUnavailable
	default:
		return ErrNotifierCallFailed
	}
}

func notifyFreedesktop(ctx context.Context, notification Notification, displayMillis int32) error {
	conn, err := dbus.SessionBusPrivateNoAutoStartup(dbus.WithContext(ctx))
	if err != nil {
		return ErrNotifierUnavailable
	}
	defer conn.Close()
	if err := conn.Auth(nil); err != nil {
		return ErrNotifierUnavailable
	}
	if err := conn.Hello(); err != nil {
		return ErrNotifierUnavailable
	}

	call := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications").CallWithContext(
		ctx,
		"org.freedesktop.Notifications.Notify",
		0,
		notificationAppName,
		uint32(0),
		"",
		notification.Title,
		notification.Body,
		[]string{},
		map[string]dbus.Variant{
			"category": dbus.MakeVariant("im.received"),
			"urgency":  dbus.MakeVariant(byte(1)),
		},
		displayMillis,
	)
	return call.Err
}

func escapeNotificationMarkup(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	return strings.ReplaceAll(text, ">", "&gt;")
}

// FakeNotifier records only invocation count and an optional opaque match.
type FakeNotifier struct {
	Notifies int
	Err      error
	Matches  func(Notification) bool
	Matched  bool
}

func (f *FakeNotifier) Notify(_ context.Context, notification Notification) error {
	f.Notifies++
	if f.Matches != nil {
		f.Matched = f.Matches(notification)
	}
	return f.Err
}
