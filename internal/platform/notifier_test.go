package platform

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFakeNotifierRecordsOpaqueMatch(t *testing.T) {
	fake := &FakeNotifier{Matches: func(notification Notification) bool {
		return notification.Title == "Chat" && notification.Body == "Sender: private"
	}}
	if err := fake.Notify(context.Background(), Notification{Title: "Chat", Body: "Sender: private"}); err != nil {
		t.Fatal(err)
	}
	if fake.Notifies != 1 || !fake.Matched {
		t.Fatalf("fake notification = count:%d matched:%t", fake.Notifies, fake.Matched)
	}
	var notifier Notifier = fake
	if notifier == nil {
		t.Fatal("fake does not satisfy Notifier")
	}
}

func TestProductionNotifierSanitizesAndBoundsCall(t *testing.T) {
	var got Notification
	var gotDisplayMillis int32
	notifier := newProductionNotifierForTest(func(ctx context.Context, notification Notification, displayMillis int32) error {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > time.Second {
			t.Fatal("notification call did not receive the bounded context")
		}
		got = notification
		gotDisplayMillis = displayMillis
		return nil
	}, time.Second)

	if err := notifier.Notify(context.Background(), Notification{Title: "<Chat> & friends", Body: "A: <private> & safe"}); err != nil {
		t.Fatal(err)
	}
	if got.Title != "&lt;Chat&gt; &amp; friends" || got.Body != "A: &lt;private&gt; &amp; safe" {
		t.Fatalf("sanitized notification did not match")
	}
	if gotDisplayMillis != notificationDisplayMillis {
		t.Fatalf("display timeout = %d, want %d", gotDisplayMillis, notificationDisplayMillis)
	}
}

func TestProductionNotifierHonorsCancellationAndReturnsSafeErrors(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		notifier := newProductionNotifierForTest(func(ctx context.Context, _ Notification, _ int32) error {
			<-ctx.Done()
			return ctx.Err()
		}, 20*time.Millisecond)
		if err := notifier.Notify(context.Background(), Notification{}); !errors.Is(err, ErrNotifierUnavailable) {
			t.Fatalf("timeout error = %v", err)
		}
	})

	t.Run("raw failure", func(t *testing.T) {
		notifier := newProductionNotifierForTest(func(context.Context, Notification, int32) error {
			return errors.New("private D-Bus detail")
		}, time.Second)
		err := notifier.Notify(context.Background(), Notification{})
		if !errors.Is(err, ErrNotifierCallFailed) || err.Error() != ErrNotifierCallFailed.Error() {
			t.Fatalf("call error was not replaced by safe sentinel: %v", err)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		notifier := newProductionNotifierForTest(func(context.Context, Notification, int32) error {
			return ErrNotifierUnavailable
		}, time.Second)
		if err := notifier.Notify(context.Background(), Notification{}); err != ErrNotifierUnavailable {
			t.Fatalf("unavailable error = %v", err)
		}
	})
}

func TestNilProductionNotifierIsUnavailable(t *testing.T) {
	var notifier *ProductionNotifier
	if err := notifier.Notify(context.Background(), Notification{}); err != ErrNotifierUnavailable {
		t.Fatalf("nil notifier error = %v", err)
	}
}
