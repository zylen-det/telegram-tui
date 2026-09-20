package frontend

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/avatar"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/media/thumbnail"
	"github.com/zylen-det/telegram-tui/internal/platform"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func TestHandlerCopyMessageUsesInjectedClipboardAndSafeEvents(t *testing.T) {
	clipboard := &platform.FakeClipboard{Matches: func(value string) bool { return value == "private" }}
	handler := NewHandler(context.Background(), nil, nil, nil, nil, clipboard)
	events := collectHandlerEvents(handler, WriteClipboard{Text: "private"})
	if len(events) != 1 {
		t.Fatalf("copy success event count = %d", len(events))
	}
	if _, ok := events[0].(ClipboardWritten); !ok {
		t.Fatalf("copy success event type = %T", events[0])
	}
	if clipboard.Writes != 1 || !clipboard.Matched {
		t.Fatalf("clipboard write count = %d", clipboard.Writes)
	}
	clipboard.Err = errors.New("raw clipboard detail")
	events = collectHandlerEvents(handler, WriteClipboard{Text: "private"})
	failed, ok := events[0].(ClipboardWriteFailed)
	if !ok || failed.Error.Message != "Could not copy message" || failed.Error.Cause == nil {
		t.Fatalf("copy failure event = type:%T safe:%t cause:%t", events[0], ok && failed.Error.Message == "Could not copy message", ok && failed.Error.Cause != nil)
	}
}

func TestHandlerDesktopNotificationIsBestEffort(t *testing.T) {
	private := "private notification body"
	notifier := &platform.FakeNotifier{Matches: func(notification platform.Notification) bool {
		return notification.Title == "Chat title" && notification.Body == private
	}}
	handler := NewHandler(context.Background(), nil, nil, nil, nil)
	handler.SetNotifier(notifier)

	events := collectHandlerEvents(handler, ShowDesktopNotification{Title: "Chat title", Body: private})
	if len(events) != 0 || notifier.Notifies != 1 || !notifier.Matched {
		t.Fatalf("notification result = events:%d calls:%d matched:%t", len(events), notifier.Notifies, notifier.Matched)
	}

	notifier.Err = errors.New("private transport failure")
	events = collectHandlerEvents(handler, ShowDesktopNotification{Title: "Chat title", Body: private})
	if len(events) != 0 || notifier.Notifies != 2 {
		t.Fatalf("failed notification was not silent: events:%d calls:%d", len(events), notifier.Notifies)
	}

	withoutNotifier := NewHandler(context.Background(), nil, nil, nil, nil)
	if events = collectHandlerEvents(withoutNotifier, ShowDesktopNotification{Title: "Chat title", Body: private}); len(events) != 0 {
		t.Fatalf("missing notifier emitted events: %#v", events)
	}
}

func TestHandlerReactToMessageMapsSuccessAndConstantFailure(t *testing.T) {
	client := &handlerClient{}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})

	events := collectHandlerEvents(handler, ReactToMessage{RequestID: 9, ChatID: 71, MessageID: 93, Emoji: "👍", Remove: false})
	if len(events) != 1 {
		t.Fatalf("add events = %#v, want one", events)
	}
	changed, ok := events[0].(ReactionChanged)
	if !ok || changed.RequestID != 9 || changed.ChatID != 71 || changed.MessageID != 93 || changed.Emoji != "👍" || changed.Removed {
		t.Fatalf("add event = %#v", events[0])
	}

	events = collectHandlerEvents(handler, ReactToMessage{RequestID: 10, ChatID: 72, MessageID: 94, Emoji: "❤️", Remove: true})
	removed, ok := events[0].(ReactionChanged)
	if !ok || removed.RequestID != 10 || removed.ChatID != 72 || removed.MessageID != 94 || removed.Emoji != "❤️" || !removed.Removed {
		t.Fatalf("remove event = %#v", events[0])
	}

	client.mu.Lock()
	if client.reactRequest != (telegram.ReactToMessageRequest{ChatID: 72, MessageID: 94, Emoji: "❤️", Remove: true}) {
		t.Fatalf("react call = %#v", client.reactRequest)
	}
	client.mu.Unlock()

	client.err = errors.New("opaque reaction transport secret 71 93")
	events = collectHandlerEvents(handler, ReactToMessage{RequestID: 11, ChatID: 71, MessageID: 93, Emoji: "🔥"})
	failed, ok := events[0].(ReactionFailed)
	if !ok || failed.RequestID != 11 || failed.ChatID != 71 || failed.MessageID != 93 {
		t.Fatalf("failure event = %#v", events[0])
	}
	if failed.Error.Message != "Reaction failed" || failed.Error.Cause != nil || strings.Contains(failed.Error.Message, "opaque") || strings.Contains(failed.Error.Message, "71") || strings.Contains(failed.Error.Message, "93") {
		t.Fatalf("failure error = %#v", failed.Error)
	}

	client.mu.Lock()
	client.err = nil
	client.mu.Unlock()
	unavailable := NewHandler(context.Background(), nil, nil, nil, nil)
	events = collectHandlerEvents(unavailable, ReactToMessage{RequestID: 12, ChatID: 71, MessageID: 93, Emoji: "👍"})
	failed, ok = events[0].(ReactionFailed)
	if !ok || failed.Error.Message != "Reaction failed" || failed.Error.Cause != nil {
		t.Fatalf("unavailable-client path event = %#v", events[0])
	}
}

func TestHandlerPreservesCommandCorrelationAndUsesExpectedOperations(t *testing.T) {
	now := time.Unix(500, 0).UTC()
	client := &handlerClient{
		chatPage:    telegram.ChatPage{Chats: []domain.Chat{{ID: 9}}, Done: true},
		messagePage: telegram.MessagePage{Messages: []domain.Message{{ID: 11, ChatID: 9}}, Done: true},
		sent:        domain.Message{ID: -30, ChatID: 9, Text: "hello"},
		avatarFile:  telegram.LocalFile{Path: "/tmp/avatar.png"},
	}
	rendered := pixel.Avatar{Width: 1, Height: 1, Cells: []pixel.Cell{{Rune: 'A'}}}
	renderer := &handlerAvatarRenderer{result: rendered}
	handler := newTestHandler(t, client, renderer)
	handler.now = func() time.Time { return now }

	tests := []struct {
		name    string
		command Effect
		want    Event
	}{
		{name: "chats", command: LoadChats{RequestID: 4, Cursor: telegram.ChatCursor{Limit: 17}}, want: ChatsLoaded{RequestID: 4, Page: client.chatPage}},
		{name: "messages", command: LoadMessages{RequestID: 5, ChatID: 9, Cursor: telegram.MessageCursor{FromMessageID: 88, Limit: 20}}, want: MessagesLoaded{RequestID: 5, ChatID: 9, Page: client.messagePage}},
		{name: "send", command: SendText{RequestID: 6, LocalID: -1, ChatID: 9, Text: "hello"}, want: TextQueued{RequestID: 6, LocalID: -1, Message: client.sent}},
		{name: "render", command: RenderAvatar{Key: "small:chat-list", Ref: domain.AvatarRef{UniqueID: "small"}, Role: avatar.RoleChatList}, want: AvatarRendered{Key: "small:chat-list", Cells: rendered}},
		{name: "open", command: OpenAvatar{RequestID: 7, Title: "Mina", Ref: domain.AvatarRef{UniqueID: "small", OriginalUniqueID: "big"}}, want: AvatarOpened{RequestID: 7, Title: "Mina", Path: "/tmp/avatar.png"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := collectHandlerEvents(handler, test.command)
			if !reflect.DeepEqual(events, []Event{test.want}) {
				t.Fatalf("events = %#v, want %#v", events, []Event{test.want})
			}
		})
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.chatCursor != (telegram.ChatCursor{Limit: 17}) || client.messageChat != 9 || client.messageCursor.FromMessageID != 88 {
		t.Fatalf("load calls = (%#v, %d, %#v)", client.chatCursor, client.messageChat, client.messageCursor)
	}
	if client.sendChat != 9 || client.sendText != "hello" {
		t.Fatalf("send call = (%d, %q)", client.sendChat, client.sendText)
	}
	if len(client.avatarSizes) != 2 || client.avatarSizes[0] != telegram.AvatarSmall || client.avatarSizes[1] != telegram.AvatarOriginal {
		t.Fatalf("avatar sizes = %#v", client.avatarSizes)
	}
	if renderer.path != "/tmp/avatar.png" || renderer.role != avatar.RoleChatList {
		t.Fatalf("renderer call = (%q, %q)", renderer.path, renderer.role)
	}
}

func TestHandlerFailuresAreTypedAndSafe(t *testing.T) {
	rawDetail := "raw transport detail"
	client := &handlerClient{err: errors.New(rawDetail)}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{err: errors.New(rawDetail)})
	now := time.Unix(900, 0).UTC()
	handler.now = func() time.Time { return now }

	tests := []struct {
		name    string
		command Effect
		check   func(Event) (domain.AppError, bool)
	}{
		{name: "chats", command: LoadChats{RequestID: 4}, check: func(value Event) (domain.AppError, bool) {
			event, ok := value.(ChatsLoadFailed)
			return event.Error, ok && event.RequestID == 4
		}},
		{name: "messages", command: LoadMessages{RequestID: 5, ChatID: 9}, check: func(value Event) (domain.AppError, bool) {
			event, ok := value.(MessagesLoadFailed)
			return event.Error, ok && event.RequestID == 5 && event.ChatID == 9
		}},
		{name: "send", command: SendText{RequestID: 6, LocalID: -1, ChatID: 9, Text: "private body"}, check: func(value Event) (domain.AppError, bool) {
			event, ok := value.(TextQueueFailed)
			return event.Error, ok && event.RequestID == 6 && event.LocalID == -1 && event.FailedAt == now
		}},
		{name: "render", command: RenderAvatar{Key: "key", Ref: domain.AvatarRef{UniqueID: "small"}}, check: func(value Event) (domain.AppError, bool) {
			event, ok := value.(AvatarRenderFailed)
			return event.Error, ok && event.Key == "key"
		}},
		{name: "open", command: OpenAvatar{RequestID: 8, Title: "Mina", Ref: domain.AvatarRef{UniqueID: "small"}}, check: func(value Event) (domain.AppError, bool) {
			event, ok := value.(AvatarOpenFailed)
			return event.Error, ok && event.RequestID == 8
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := collectHandlerEvents(handler, test.command)
			if len(events) != 1 {
				t.Fatalf("events = %#v, want one", events)
			}
			appError, ok := test.check(events[0])
			if !ok {
				t.Fatalf("event = %#v, wrong type or correlation", events[0])
			}
			if appError.Kind != domain.ErrorInternal || strings.Contains(appError.Message, rawDetail) || appError.Cause == nil {
				t.Fatalf("safe error = %#v", appError)
			}
		})
	}
}

func TestHandlerBootstrapCreatesOnceAndStreamsOrderedUpdates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &handlerClient{startUpdates: []telegram.Update{telegram.Ready{}, telegram.ConnectionChanged{State: domain.ConnectionOnline}}}
	resolver := &handlerResolver{runtime: config.Runtime{APIID: 17}}
	factoryCalls := 0
	handler := NewHandler(ctx, resolver, func(runtime config.Runtime, _ auth.Prompter) (telegram.Client, error) {
		factoryCalls++
		if runtime.APIID != 17 {
			t.Fatalf("runtime APIID = %d", runtime.APIID)
		}
		return client, nil
	}, auth.NewBroker(), &handlerAvatarRenderer{})
	handler.now = func() time.Time { return time.Unix(100, int64(factoryCalls)).UTC() }

	if msg := handler.Cmd(LoadBootstrap{})(); msg != nil {
		t.Fatalf("bootstrap returned %#v, want nil", msg)
	}
	first := receiveWithTimeout(t, handler.Updates(), "ready update")
	second := receiveWithTimeout(t, handler.Updates(), "connection update")
	if _, ok := first.(TelegramEvent); !ok {
		t.Fatalf("first event = %#v", first)
	}
	if got, ok := second.(TelegramEvent); !ok || reflect.TypeOf(got.Value) != reflect.TypeOf(telegram.ConnectionChanged{}) {
		t.Fatalf("second event = %#v", second)
	}

	if msg := handler.Cmd(LoadBootstrap{})(); msg != nil {
		t.Fatalf("second bootstrap returned %#v, want nil", msg)
	}
	if factoryCalls != 1 {
		t.Fatalf("factory calls = %d, want 1", factoryCalls)
	}
}

func TestHandlerSubmitPromptPassesExactResponse(t *testing.T) {
	broker := auth.NewBroker()
	handler := NewHandler(context.Background(), &handlerResolver{}, func(config.Runtime, auth.Prompter) (telegram.Client, error) { return &handlerClient{}, nil }, broker, &handlerAvatarRenderer{})
	answer := make(chan string, 1)
	go func() {
		value, _ := broker.Ask(context.Background(), auth.Prompt{Kind: auth.PromptCode})
		answer <- value
	}()
	prompt := receiveWithTimeout(t, broker.Prompts(), "prompt")
	collectHandlerEvents(handler, SubmitPrompt{Response: auth.Response{PromptID: prompt.ID, Value: "exact response"}})
	if got := receiveWithTimeout(t, answer, "prompt response"); got != "exact response" {
		t.Fatalf("response = %q", got)
	}
}

func TestHandlerBootstrapForwardsBrokerPromptAndExactSubmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broker := auth.NewBroker()
	client := &promptingHandlerClient{answer: make(chan string, 1)}
	handler := NewHandler(ctx, &handlerResolver{}, func(_ config.Runtime, prompter auth.Prompter) (telegram.Client, error) {
		client.prompter = prompter
		return client, nil
	}, broker, &handlerAvatarRenderer{})
	handler.Cmd(LoadBootstrap{})()

	requested := receiveWithTimeout(t, handler.PromptStream(), "forwarded prompt")
	if requested.Kind != auth.PromptCode || requested.ID == 0 {
		t.Fatalf("prompt = %#v", requested)
	}
	if events := collectHandlerEvents(handler, SubmitPrompt{Response: auth.Response{PromptID: requested.ID, Value: "02719"}}); len(events) != 0 {
		t.Fatalf("successful prompt submit emitted %#v", events)
	}
	if got := receiveWithTimeout(t, client.answer, "client prompt answer"); got != "02719" {
		t.Fatalf("client answer = %q", got)
	}
	if event, ok := receiveWithTimeout(t, handler.Updates(), "ready after prompt").(TelegramEvent); !ok {
		t.Fatalf("ready event = %#v", event)
	}
}

func TestHandlerBootstrapForwardsResolverPromptBeforeCreatingClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broker := auth.NewBroker()
	resolver := &promptingHandlerResolver{prompter: broker, answer: make(chan string, 1)}
	client := &handlerClient{}
	factoryCalled := make(chan struct{}, 1)
	handler := NewHandler(ctx, resolver, func(_ config.Runtime, _ auth.Prompter) (telegram.Client, error) {
		factoryCalled <- struct{}{}
		return client, nil
	}, broker, &handlerAvatarRenderer{})
	bootstrapDone := make(chan struct{})
	go func() {
		handler.Cmd(LoadBootstrap{})()
		close(bootstrapDone)
	}()

	requested := receiveWithTimeout(t, handler.PromptStream(), "resolver prompt")
	if requested.Kind != auth.PromptAPIID {
		t.Fatalf("resolver prompt = %#v", requested)
	}
	if events := collectHandlerEvents(handler, SubmitPrompt{Response: auth.Response{PromptID: requested.ID, Value: "17001"}}); len(events) != 0 {
		t.Fatalf("successful resolver prompt submit emitted %#v", events)
	}
	if got := receiveWithTimeout(t, resolver.answer, "resolver answer"); got != "17001" {
		t.Fatalf("resolver answer = %q", got)
	}
	receiveWithTimeout(t, factoryCalled, "factory after resolver")
	receiveWithTimeout(t, bootstrapDone, "bootstrap return")
}

func TestHandlerBootstrapClassifiesStartFailureAroundReady(t *testing.T) {
	for _, test := range []struct {
		name        string
		updates     []telegram.Update
		wantStartup bool
	}{
		{name: "before ready", wantStartup: true},
		{name: "after ready", updates: []telegram.Update{telegram.Ready{}}, wantStartup: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &handlerClient{startUpdates: test.updates, startErr: errors.New("unsafe start detail")}
			handler := NewHandler(ctx, &handlerResolver{}, func(config.Runtime, auth.Prompter) (telegram.Client, error) { return client, nil }, auth.NewBroker(), &handlerAvatarRenderer{})
			handler.Cmd(LoadBootstrap{})()
			if len(test.updates) > 0 {
				if _, ok := receiveWithTimeout(t, handler.Updates(), "ready").(TelegramEvent); !ok {
					t.Fatal("first event was not TelegramEvent")
				}
			}
			failure := receiveWithTimeout(t, handler.Updates(), "start failure")
			if test.wantStartup {
				if event, ok := failure.(StartupFailed); !ok || event.Error.Kind != domain.ErrorInternal || strings.Contains(event.Error.Message, "unsafe") {
					t.Fatalf("startup failure = %#v", failure)
				}
			} else if event, ok := failure.(OperationFailed); !ok || event.Error.Kind != domain.ErrorInternal || strings.Contains(event.Error.Message, "unsafe") {
				t.Fatalf("operation failure = %#v", failure)
			}
		})
	}
}

func TestHandlerShutdownNeverClosesBeforeStartBegins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &startBarrierClient{started: make(chan struct{}), release: make(chan struct{})}
	handler := NewHandler(ctx, &handlerResolver{}, func(config.Runtime, auth.Prompter) (telegram.Client, error) {
		return client, nil
	}, auth.NewBroker(), &handlerAvatarRenderer{})
	bootstrapDone := make(chan struct{})
	go func() {
		handler.Cmd(LoadBootstrap{})()
		close(bootstrapDone)
	}()

	receiveWithTimeout(t, client.started, "client Start invocation")
	collectHandlerEvents(handler, BeginShutdown{})
	client.mu.Lock()
	closedBeforeStart := client.closedBeforeStart
	client.mu.Unlock()
	if closedBeforeStart {
		t.Fatal("Close ran before Start began")
	}
	close(client.release)
	receiveWithTimeout(t, bootstrapDone, "bootstrap return")
}

func TestSafeErrorPreservesAppErrorAndSanitizesContextAndArbitraryErrors(t *testing.T) {
	typed := domain.AppError{Kind: domain.ErrorRateLimit, Message: "wait safely", RetryAfter: 2 * time.Second}
	got := safeError("send", typed)
	if got.Kind != typed.Kind || got.Message != typed.Message || got.Op != "send" || got.Cause == nil {
		t.Fatalf("typed safeError = %#v", got)
	}
	canceled := safeError("load", context.DeadlineExceeded)
	if canceled.Kind != domain.ErrorNetwork || canceled.Message != "operation canceled or timed out" {
		t.Fatalf("context safeError = %#v", canceled)
	}
	arbitrary := safeError("load", errors.New("raw internal transport detail"))
	if arbitrary.Kind != domain.ErrorInternal || arbitrary.Message != "operation failed; retry or inspect the sanitized log" || strings.Contains(arbitrary.Message, "transport") {
		t.Fatalf("arbitrary safeError = %#v", arbitrary)
	}
}

func TestHandlerShutdownCancelsAndClosesExactlyOnce(t *testing.T) {
	client := &handlerClient{}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := append(collectHandlerEvents(handler, BeginShutdown{}), collectHandlerEvents(handler, BeginShutdown{})...)
	if client.closeCalls != 1 || len(events) != 1 {
		t.Fatalf("close calls = %d, events = %#v", client.closeCalls, events)
	}
	if _, ok := events[0].(ShutdownComplete); !ok {
		t.Fatalf("shutdown event = %#v", events[0])
	}
	if handler.workCtx.Err() != context.Canceled {
		t.Fatalf("work context error = %v", handler.workCtx.Err())
	}
}

func TestHandlerForwardMessageSuccessEmitsCorrelatedMessageForwarded(t *testing.T) {
	client := &handlerClient{}
	handler := &Handler{client: client, workCtx: context.Background(), now: time.Now}
	events := collectHandlerEvents(handler, ForwardMessageCommand{RequestID: 7, SourceChatID: 9, SourceMessageID: 2, DestinationChatID: 10})
	if len(events) != 1 {
		t.Fatalf("forward success event count = %d", len(events))
	}
	forwarded, ok := events[0].(MessageForwarded)
	if !ok || forwarded.RequestID != 7 || forwarded.DestinationChatID != 10 {
		t.Fatalf("forward success event = %#v", events[0])
	}
	if client.forwardRequest != (telegram.ForwardMessageRequest{SourceChatID: 9, SourceMessageID: 2, DestinationChatID: 10}) {
		t.Fatalf("forward request = %#v", client.forwardRequest)
	}
}

func TestHandlerForwardMessageFailureDoesNotEmitRawCause(t *testing.T) {
	raw := errors.New("opaque-transport-secret")
	client := &handlerClient{err: raw}
	handler := &Handler{client: client, workCtx: context.Background(), now: time.Now}
	events := collectHandlerEvents(handler, ForwardMessageCommand{RequestID: 7, SourceChatID: 9, SourceMessageID: 2, DestinationChatID: 10})
	var got Event
	if len(events) > 0 {
		got = events[0]
	}
	failure, ok := got.(MessageForwardFailed)
	if !ok || failure.Error.Cause != nil || failure.Error.Message != "Forward failed" || failure.RequestID != 7 || failure.DestinationChatID != 10 {
		t.Fatalf("forward failure = type:%T event:%#v", got, got)
	}
	if client.forwardRequest != (telegram.ForwardMessageRequest{SourceChatID: 9, SourceMessageID: 2, DestinationChatID: 10}) {
		t.Fatalf("forward request = %#v", client.forwardRequest)
	}
}

func TestHandlerDeleteMessageFailureDoesNotEmitRawCause(t *testing.T) {
	raw := errors.New("opaque-transport-secret")
	client := &handlerClient{err: raw}
	handler := &Handler{client: client, workCtx: context.Background(), now: time.Now}
	events := collectHandlerEvents(handler, DeleteMessageCommand{RequestID: 7, ChatID: 9, MessageID: 2, Revoke: true})
	var got Event
	if len(events) > 0 {
		got = events[0]
	}
	failure, ok := got.(MessageDeleteFailed)
	if !ok || failure.Error.Cause != nil || failure.Error.Message != "Delete failed" || failure.RequestID != 7 || failure.ChatID != 9 || failure.MessageID != 2 {
		t.Fatalf("delete failure = type:%T event:%#v", got, got)
	}
	if client.deleteRequest != (telegram.DeleteMessageRequest{ChatID: 9, MessageID: 2, Revoke: true}) {
		t.Fatalf("delete request = %#v", client.deleteRequest)
	}
}

func TestHandlerDeleteMessageSuccessEmitsCorrelatedDeleted(t *testing.T) {
	client := &handlerClient{}
	handler := &Handler{client: client, workCtx: context.Background(), now: time.Now}
	events := collectHandlerEvents(handler, DeleteMessageCommand{RequestID: 7, ChatID: 9, MessageID: 2, Revoke: false})
	if len(events) != 1 {
		t.Fatalf("delete success event count = %d", len(events))
	}
	deleted, ok := events[0].(MessageDeleted)
	if !ok || deleted.RequestID != 7 || deleted.ChatID != 9 || deleted.MessageID != 2 {
		t.Fatalf("delete success event = %#v", events[0])
	}
	if client.deleteRequest != (telegram.DeleteMessageRequest{ChatID: 9, MessageID: 2, Revoke: false}) {
		t.Fatalf("delete request = %#v", client.deleteRequest)
	}
}

func TestHandlerPinMessageSuccessEmitsCorrelatedPinChanged(t *testing.T) {
	client := &handlerClient{}
	handler := &Handler{client: client, workCtx: context.Background(), now: time.Now}
	events := collectHandlerEvents(handler, PinMessageCommand{RequestID: 7, ChatID: 9, MessageID: 2, Unpin: false})
	if len(events) != 1 {
		t.Fatalf("pin success event count = %d", len(events))
	}
	changed, ok := events[0].(MessagePinChanged)
	if !ok || changed.RequestID != 7 || changed.ChatID != 9 || changed.MessageID != 2 || !changed.Pinned {
		t.Fatalf("pin success event = %#v", events[0])
	}
	if client.pinRequest != (telegram.PinMessageRequest{ChatID: 9, MessageID: 2, Unpin: false}) {
		t.Fatalf("pin request = %#v", client.pinRequest)
	}
}

func TestHandlerPinMessageUnpinSuccessEmitsPinnedFalse(t *testing.T) {
	client := &handlerClient{}
	handler := &Handler{client: client, workCtx: context.Background(), now: time.Now}
	events := collectHandlerEvents(handler, PinMessageCommand{RequestID: 7, ChatID: 9, MessageID: 2, Unpin: true})
	changed, ok := events[0].(MessagePinChanged)
	if !ok || changed.Pinned {
		t.Fatalf("unpin success event = %#v", events[0])
	}
	if client.pinRequest != (telegram.PinMessageRequest{ChatID: 9, MessageID: 2, Unpin: true}) {
		t.Fatalf("unpin request = %#v", client.pinRequest)
	}
}

func TestHandlerPinMessageFailureDoesNotEmitRawCause(t *testing.T) {
	raw := errors.New("opaque-transport-secret")
	client := &handlerClient{err: raw}
	handler := &Handler{client: client, workCtx: context.Background(), now: time.Now}
	events := collectHandlerEvents(handler, PinMessageCommand{RequestID: 7, ChatID: 9, MessageID: 2, Unpin: false})
	var got Event
	if len(events) > 0 {
		got = events[0]
	}
	failure, ok := got.(MessagePinFailed)
	if !ok || failure.Error.Cause != nil || failure.Error.Message != "Pin failed" || failure.RequestID != 7 || failure.ChatID != 9 || failure.MessageID != 2 {
		t.Fatalf("pin failure = type:%T event:%#v", got, got)
	}
	if client.pinRequest != (telegram.PinMessageRequest{ChatID: 9, MessageID: 2, Unpin: false}) {
		t.Fatalf("pin request = %#v", client.pinRequest)
	}
}

func TestHandlerEditFailureDoesNotEmitRawCause(t *testing.T) {
	raw := errors.New("opaque-transport-secret")
	client := &handlerClient{err: raw}
	handler := &Handler{client: client, workCtx: context.Background(), now: time.Now}
	events := collectHandlerEvents(handler, EditText{RequestID: 7, ChatID: 9, MessageID: 2, Text: "opaque-edit"})
	var got Event
	if len(events) > 0 {
		got = events[0]
	}
	failure, ok := got.(TextEditFailed)
	if !ok || failure.Error.Cause != nil || failure.Error.Message != "Edit failed" || failure.RequestID != 7 || failure.ChatID != 9 || failure.MessageID != 2 {
		t.Fatal("edit failure was not sanitized or correlated")
	}
}

func collectHandlerEvents(handler *Handler, effect Effect) []Event {
	cmd := handler.Cmd(effect)
	if cmd == nil {
		return nil
	}
	return flattenMessages(cmd())
}

// flattenMessages expands one command result, including a tea.BatchMsg, into
// the ordered application events it produced.
func flattenMessages(msg tea.Msg) []Event {
	switch value := msg.(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		events := make([]Event, 0, len(value))
		for _, cmd := range value {
			events = append(events, flattenMessages(cmd())...)
		}
		return events
	default:
		return []Event{value}
	}
}

func newTestHandler(t *testing.T, client telegram.Client, renderer AvatarRenderer) *Handler {
	t.Helper()
	handler := NewHandler(context.Background(), &handlerResolver{}, func(config.Runtime, auth.Prompter) (telegram.Client, error) { return client, nil }, auth.NewBroker(), renderer)
	handler.client = client
	handler.startOnce.Do(func() { close(handler.startBegan) })
	return handler
}

type handlerResolver struct {
	runtime config.Runtime
	err     error
}

func (r *handlerResolver) Resolve(context.Context) (config.Runtime, error) { return r.runtime, r.err }

type promptingHandlerResolver struct {
	prompter auth.Prompter
	answer   chan string
}

func (r *promptingHandlerResolver) Resolve(ctx context.Context) (config.Runtime, error) {
	answer, err := r.prompter.Ask(ctx, auth.Prompt{Kind: auth.PromptAPIID, Label: "API ID"})
	if err != nil {
		return config.Runtime{}, err
	}
	r.answer <- answer
	return config.Runtime{APIID: 17001}, nil
}

type handlerAvatarRenderer struct {
	result pixel.Avatar
	err    error
	path   string
	role   avatar.Role
}

func (r *handlerAvatarRenderer) Render(_ context.Context, path string, _ domain.AvatarRef, role avatar.Role) (pixel.Avatar, error) {
	r.path, r.role = path, role
	return r.result, r.err
}

type startBarrierClient struct {
	mu                sync.Mutex
	started           chan struct{}
	release           chan struct{}
	startBegan        bool
	closedBeforeStart bool
}

func (c *startBarrierClient) Start(ctx context.Context, _ chan<- telegram.Update) error {
	c.mu.Lock()
	c.startBegan = true
	close(c.started)
	c.mu.Unlock()
	select {
	case <-c.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *startBarrierClient) LoadChats(context.Context, telegram.ChatCursor) (telegram.ChatPage, error) {
	return telegram.ChatPage{}, nil
}
func (c *startBarrierClient) LoadMessages(context.Context, domain.ChatID, telegram.MessageCursor) (telegram.MessagePage, error) {
	return telegram.MessagePage{}, nil
}
func (c *startBarrierClient) LoadTopics(context.Context, domain.ChatID, telegram.TopicCursor) (telegram.TopicPage, error) {
	return telegram.TopicPage{}, nil
}
func (c *startBarrierClient) LoadMembers(context.Context, domain.ChatID, telegram.MemberCursor) (telegram.MemberPage, error) {
	return telegram.MemberPage{}, nil
}
func (c *startBarrierClient) LoadUser(context.Context, domain.UserID) (domain.User, error) {
	return domain.User{}, nil
}
func (c *startBarrierClient) AddContact(context.Context, telegram.AddContactRequest) error {
	return nil
}
func (c *startBarrierClient) RemoveContact(context.Context, telegram.RemoveContactRequest) error {
	return nil
}
func (c *startBarrierClient) SetUserBlocked(context.Context, telegram.SetUserBlockedRequest) error {
	return nil
}
func (c *startBarrierClient) ApplyChatAction(context.Context, telegram.ChatActionRequest) error {
	return nil
}
func (c *startBarrierClient) SearchChatMessages(context.Context, domain.ChatID, string, telegram.MessageSearchCursor) (telegram.MessageSearchPage, error) {
	return telegram.MessageSearchPage{}, nil
}
func (c *startBarrierClient) SearchPinnedMessages(context.Context, domain.ChatID, telegram.MessageSearchCursor) (telegram.MessageSearchPage, error) {
	return telegram.MessageSearchPage{}, nil
}
func (c *startBarrierClient) LoadMessageContext(context.Context, domain.ChatID, domain.MessageID) (telegram.MessagePage, error) {
	return telegram.MessagePage{}, nil
}
func (c *startBarrierClient) LoadTopicMessageContext(context.Context, domain.ChatID, domain.TopicID, domain.MessageID) (telegram.MessagePage, error) {
	return telegram.MessagePage{}, nil
}
func (c *startBarrierClient) GetMessageProperties(context.Context, domain.ChatID, domain.MessageID) (domain.MessageCapabilities, error) {
	return domain.MessageCapabilities{}, nil
}
func (c *startBarrierClient) SetDraft(context.Context, telegram.SetDraftRequest) error { return nil }
func (c *startBarrierClient) SendText(context.Context, telegram.SendTextRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *startBarrierClient) EditText(context.Context, telegram.EditTextRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *startBarrierClient) DeleteMessage(context.Context, telegram.DeleteMessageRequest) error {
	return nil
}
func (c *startBarrierClient) PinMessage(context.Context, telegram.PinMessageRequest) error {
	return nil
}
func (c *startBarrierClient) ReactToMessage(context.Context, telegram.ReactToMessageRequest) error {
	return nil
}
func (c *startBarrierClient) ForwardMessage(context.Context, telegram.ForwardMessageRequest) error {
	return nil
}
func (c *startBarrierClient) SendPhoto(context.Context, telegram.SendPhotoRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *startBarrierClient) SendVideo(context.Context, telegram.SendVideoRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *startBarrierClient) SendAudio(context.Context, telegram.SendAudioRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *startBarrierClient) SendDocument(context.Context, telegram.SendDocumentRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *startBarrierClient) LoadBotCommands(context.Context, domain.ChatID) ([]domain.BotCommand, error) {
	return nil, nil
}
func (c *startBarrierClient) LoadStickers(context.Context) ([]domain.StickerRef, error) {
	return nil, nil
}
func (c *startBarrierClient) SendSticker(context.Context, telegram.SendStickerRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *startBarrierClient) DownloadAvatar(context.Context, domain.AvatarRef, telegram.AvatarSize) (telegram.LocalFile, error) {
	return telegram.LocalFile{}, nil
}
func (c *startBarrierClient) DownloadMedia(context.Context, domain.MediaFileRef) (telegram.LocalFile, error) {
	return telegram.LocalFile{}, nil
}
func (c *startBarrierClient) SearchPublicChat(context.Context, string) (domain.Chat, error) {
	return domain.Chat{}, nil
}
func (c *startBarrierClient) SearchPublicChats(context.Context, string) ([]domain.Chat, error) {
	return nil, nil
}
func (c *startBarrierClient) SearchAllMessages(context.Context, string, int) (telegram.MessageSearchPage, error) {
	return telegram.MessageSearchPage{}, nil
}
func (c *startBarrierClient) OpenChat(context.Context, domain.ChatID) error  { return nil }
func (c *startBarrierClient) CloseChat(context.Context, domain.ChatID) error { return nil }
func (c *startBarrierClient) Close(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closedBeforeStart = !c.startBegan
	return nil
}

type handlerClient struct {
	mu                   sync.Mutex
	chatPage             telegram.ChatPage
	messagePage          telegram.MessagePage
	memberPage           telegram.MemberPage
	searchPage           telegram.MessageSearchPage
	sent                 domain.Message
	avatarFile           telegram.LocalFile
	err                  error
	chatCursor           telegram.ChatCursor
	messageChat          domain.ChatID
	messageCursor        telegram.MessageCursor
	memberChat           domain.ChatID
	memberCursor         telegram.MemberCursor
	loadUser             domain.User
	loadUserID           domain.UserID
	botCommands          []domain.BotCommand
	botCommandsChat      domain.ChatID
	searchChat           domain.ChatID
	searchQuery          string
	searchCursor         telegram.MessageSearchCursor
	contextChat          domain.ChatID
	contextMessage       domain.MessageID
	topicPage            telegram.TopicPage
	topicChat            domain.ChatID
	topicCursor          telegram.TopicCursor
	topicContextChat     domain.ChatID
	topicContextID       domain.TopicID
	topicContextMessage  domain.MessageID
	sendChat             domain.ChatID
	sendText             string
	sendRequest          telegram.SendTextRequest
	draftRequest         telegram.SetDraftRequest
	avatarSizes          []telegram.AvatarSize
	mediaCalls           []domain.MediaFileRef
	startUpdates         []telegram.Update
	startErr             error
	closeCalls           int
	deleteRequest        telegram.DeleteMessageRequest
	contactRequest       telegram.AddContactRequest
	removeContactRequest telegram.RemoveContactRequest
	blockRequest         telegram.SetUserBlockedRequest
	forwardRequest       telegram.ForwardMessageRequest
	pinRequest           telegram.PinMessageRequest
	reactRequest         telegram.ReactToMessageRequest
	publicChat           domain.Chat
	publicUsername       string
	photoSendRequest     telegram.SendPhotoRequest
	videoSendRequest     telegram.SendVideoRequest
	documentSendRequest  telegram.SendDocumentRequest
	audioSendRequest     telegram.SendAudioRequest
}

type promptingHandlerClient struct {
	prompter auth.Prompter
	answer   chan string
}

func (c *promptingHandlerClient) Start(ctx context.Context, updates chan<- telegram.Update) error {
	answer, err := c.prompter.Ask(ctx, auth.Prompt{Kind: auth.PromptCode, Label: "Code"})
	if err != nil {
		return err
	}
	c.answer <- answer
	select {
	case updates <- telegram.Ready{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	<-ctx.Done()
	return ctx.Err()
}
func (c *promptingHandlerClient) LoadChats(context.Context, telegram.ChatCursor) (telegram.ChatPage, error) {
	return telegram.ChatPage{}, nil
}
func (c *promptingHandlerClient) LoadMessages(context.Context, domain.ChatID, telegram.MessageCursor) (telegram.MessagePage, error) {
	return telegram.MessagePage{}, nil
}
func (c *promptingHandlerClient) LoadTopics(context.Context, domain.ChatID, telegram.TopicCursor) (telegram.TopicPage, error) {
	return telegram.TopicPage{}, nil
}
func (c *promptingHandlerClient) LoadMembers(context.Context, domain.ChatID, telegram.MemberCursor) (telegram.MemberPage, error) {
	return telegram.MemberPage{}, nil
}
func (c *promptingHandlerClient) LoadUser(context.Context, domain.UserID) (domain.User, error) {
	return domain.User{}, nil
}
func (c *promptingHandlerClient) AddContact(context.Context, telegram.AddContactRequest) error {
	return nil
}
func (c *promptingHandlerClient) RemoveContact(context.Context, telegram.RemoveContactRequest) error {
	return nil
}
func (c *promptingHandlerClient) SetUserBlocked(context.Context, telegram.SetUserBlockedRequest) error {
	return nil
}
func (c *promptingHandlerClient) ApplyChatAction(context.Context, telegram.ChatActionRequest) error {
	return nil
}
func (c *promptingHandlerClient) SearchChatMessages(context.Context, domain.ChatID, string, telegram.MessageSearchCursor) (telegram.MessageSearchPage, error) {
	return telegram.MessageSearchPage{}, nil
}
func (c *promptingHandlerClient) SearchPinnedMessages(context.Context, domain.ChatID, telegram.MessageSearchCursor) (telegram.MessageSearchPage, error) {
	return telegram.MessageSearchPage{}, nil
}
func (c *promptingHandlerClient) LoadMessageContext(context.Context, domain.ChatID, domain.MessageID) (telegram.MessagePage, error) {
	return telegram.MessagePage{}, nil
}
func (c *promptingHandlerClient) LoadTopicMessageContext(context.Context, domain.ChatID, domain.TopicID, domain.MessageID) (telegram.MessagePage, error) {
	return telegram.MessagePage{}, nil
}
func (c *promptingHandlerClient) GetMessageProperties(context.Context, domain.ChatID, domain.MessageID) (domain.MessageCapabilities, error) {
	return domain.MessageCapabilities{}, nil
}
func (c *promptingHandlerClient) SetDraft(context.Context, telegram.SetDraftRequest) error {
	return nil
}
func (c *promptingHandlerClient) SendText(context.Context, telegram.SendTextRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *promptingHandlerClient) EditText(context.Context, telegram.EditTextRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *promptingHandlerClient) DeleteMessage(context.Context, telegram.DeleteMessageRequest) error {
	return nil
}
func (c *promptingHandlerClient) PinMessage(context.Context, telegram.PinMessageRequest) error {
	return nil
}
func (c *promptingHandlerClient) ReactToMessage(context.Context, telegram.ReactToMessageRequest) error {
	return nil
}
func (c *promptingHandlerClient) ForwardMessage(context.Context, telegram.ForwardMessageRequest) error {
	return nil
}
func (c *promptingHandlerClient) SendPhoto(context.Context, telegram.SendPhotoRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *promptingHandlerClient) SendVideo(context.Context, telegram.SendVideoRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *promptingHandlerClient) SendAudio(context.Context, telegram.SendAudioRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *promptingHandlerClient) SendDocument(context.Context, telegram.SendDocumentRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *promptingHandlerClient) LoadBotCommands(context.Context, domain.ChatID) ([]domain.BotCommand, error) {
	return nil, nil
}
func (c *promptingHandlerClient) LoadStickers(context.Context) ([]domain.StickerRef, error) {
	return nil, nil
}
func (c *promptingHandlerClient) SendSticker(context.Context, telegram.SendStickerRequest) (domain.Message, error) {
	return domain.Message{}, nil
}
func (c *promptingHandlerClient) DownloadAvatar(context.Context, domain.AvatarRef, telegram.AvatarSize) (telegram.LocalFile, error) {
	return telegram.LocalFile{}, nil
}
func (c *promptingHandlerClient) DownloadMedia(context.Context, domain.MediaFileRef) (telegram.LocalFile, error) {
	return telegram.LocalFile{}, nil
}
func (c *promptingHandlerClient) SearchPublicChat(context.Context, string) (domain.Chat, error) {
	return domain.Chat{}, nil
}
func (c *promptingHandlerClient) SearchPublicChats(context.Context, string) ([]domain.Chat, error) {
	return nil, nil
}
func (c *promptingHandlerClient) SearchAllMessages(context.Context, string, int) (telegram.MessageSearchPage, error) {
	return telegram.MessageSearchPage{}, nil
}
func (c *promptingHandlerClient) OpenChat(context.Context, domain.ChatID) error  { return nil }
func (c *promptingHandlerClient) CloseChat(context.Context, domain.ChatID) error { return nil }
func (c *promptingHandlerClient) Close(context.Context) error                    { return nil }

func (c *handlerClient) Start(ctx context.Context, updates chan<- telegram.Update) error {
	for _, update := range c.startUpdates {
		select {
		case updates <- update:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if c.startErr != nil {
		return c.startErr
	}
	<-ctx.Done()
	return ctx.Err()
}
func (c *handlerClient) LoadChats(_ context.Context, cursor telegram.ChatCursor) (telegram.ChatPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.chatCursor = cursor
	return c.chatPage, c.err
}
func (c *handlerClient) LoadMessages(_ context.Context, chatID domain.ChatID, cursor telegram.MessageCursor) (telegram.MessagePage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messageChat, c.messageCursor = chatID, cursor
	return c.messagePage, c.err
}
func (c *handlerClient) LoadTopics(_ context.Context, chatID domain.ChatID, cursor telegram.TopicCursor) (telegram.TopicPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.topicChat, c.topicCursor = chatID, cursor
	return c.topicPage, c.err
}
func (c *handlerClient) LoadUser(_ context.Context, userID domain.UserID) (domain.User, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadUserID = userID
	return c.loadUser, c.err
}
func (c *handlerClient) LoadMembers(_ context.Context, chatID domain.ChatID, cursor telegram.MemberCursor) (telegram.MemberPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memberChat, c.memberCursor = chatID, cursor
	return c.memberPage, c.err
}
func (c *handlerClient) AddContact(_ context.Context, request telegram.AddContactRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contactRequest = request
	return c.err
}
func (c *handlerClient) RemoveContact(_ context.Context, request telegram.RemoveContactRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeContactRequest = request
	return c.err
}
func (c *handlerClient) SetUserBlocked(_ context.Context, request telegram.SetUserBlockedRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blockRequest = request
	return c.err
}
func (c *handlerClient) ApplyChatAction(context.Context, telegram.ChatActionRequest) error {
	return c.err
}
func (c *handlerClient) SearchChatMessages(_ context.Context, chatID domain.ChatID, query string, cursor telegram.MessageSearchCursor) (telegram.MessageSearchPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.searchChat, c.searchQuery, c.searchCursor = chatID, query, cursor
	return c.searchPage, c.err
}
func (c *handlerClient) SearchPinnedMessages(_ context.Context, chatID domain.ChatID, cursor telegram.MessageSearchCursor) (telegram.MessageSearchPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.searchChat, c.searchQuery, c.searchCursor = chatID, "", cursor
	return c.searchPage, c.err
}
func (c *handlerClient) LoadMessageContext(_ context.Context, chatID domain.ChatID, messageID domain.MessageID) (telegram.MessagePage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contextChat, c.contextMessage = chatID, messageID
	return c.messagePage, c.err
}
func (c *handlerClient) LoadTopicMessageContext(_ context.Context, chatID domain.ChatID, topicID domain.TopicID, messageID domain.MessageID) (telegram.MessagePage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.topicContextChat, c.topicContextID, c.topicContextMessage = chatID, topicID, messageID
	return c.messagePage, c.err
}
func (c *handlerClient) GetMessageProperties(context.Context, domain.ChatID, domain.MessageID) (domain.MessageCapabilities, error) {
	return domain.MessageCapabilities{Copy: true}, c.err
}
func (c *handlerClient) SetDraft(_ context.Context, request telegram.SetDraftRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.draftRequest = request
	return c.err
}
func (c *handlerClient) SendText(_ context.Context, request telegram.SendTextRequest) (domain.Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sendChat, c.sendText, c.sendRequest = request.ChatID, request.Text, request
	return c.sent, c.err
}
func (c *handlerClient) EditText(_ context.Context, request telegram.EditTextRequest) (domain.Message, error) {
	return domain.Message{ID: request.MessageID, ChatID: request.ChatID, Kind: domain.MessageText, Text: request.Text}, c.err
}
func (c *handlerClient) DeleteMessage(ctx context.Context, request telegram.DeleteMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleteRequest = request
	return c.err
}
func (c *handlerClient) PinMessage(ctx context.Context, request telegram.PinMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pinRequest = request
	return c.err
}
func (c *handlerClient) ReactToMessage(ctx context.Context, request telegram.ReactToMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reactRequest = request
	return c.err
}
func (c *handlerClient) ForwardMessage(ctx context.Context, request telegram.ForwardMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forwardRequest = request
	return c.err
}
func (c *handlerClient) DownloadAvatar(_ context.Context, _ domain.AvatarRef, size telegram.AvatarSize) (telegram.LocalFile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.avatarSizes = append(c.avatarSizes, size)
	return c.avatarFile, c.err
}
func (c *handlerClient) DownloadMedia(_ context.Context, ref domain.MediaFileRef) (telegram.LocalFile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mediaCalls = append(c.mediaCalls, ref)
	return c.avatarFile, c.err
}
func (c *handlerClient) SearchPublicChat(_ context.Context, username string) (domain.Chat, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.publicUsername = username
	return c.publicChat, c.err
}
func (c *handlerClient) SearchPublicChats(_ context.Context, query string) ([]domain.Chat, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	return nil, nil
}
func (c *handlerClient) SearchAllMessages(_ context.Context, query string, limit int) (telegram.MessageSearchPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return telegram.MessageSearchPage{}, c.err
	}
	return telegram.MessageSearchPage{}, nil
}
func (c *handlerClient) SendPhoto(ctx context.Context, request telegram.SendPhotoRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.photoSendRequest = request
	return c.sent, c.err
}
func (c *handlerClient) SendVideo(ctx context.Context, request telegram.SendVideoRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.videoSendRequest = request
	return c.sent, c.err
}
func (c *handlerClient) SendAudio(ctx context.Context, request telegram.SendAudioRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.audioSendRequest = request
	return c.sent, c.err
}
func (c *handlerClient) SendDocument(ctx context.Context, request telegram.SendDocumentRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.documentSendRequest = request
	return c.sent, c.err
}
func (c *handlerClient) LoadBotCommands(ctx context.Context, chatID domain.ChatID) ([]domain.BotCommand, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.botCommandsChat = chatID
	return append([]domain.BotCommand(nil), c.botCommands...), c.err
}
func (c *handlerClient) LoadStickers(ctx context.Context) ([]domain.StickerRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, c.err
}
func (c *handlerClient) SendSticker(ctx context.Context, _ telegram.SendStickerRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	return c.sent, c.err
}
func (c *handlerClient) Close(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeCalls++
	return nil
}
func (c *handlerClient) OpenChat(context.Context, domain.ChatID) error  { return c.err }
func (c *handlerClient) CloseChat(context.Context, domain.ChatID) error { return c.err }

func TestHandlerLoadBotCommandsPreservesRequestAndChatIdentity(t *testing.T) {
	catalog := []domain.BotCommand{{Name: "start", Description: "Start bot"}}
	client := &handlerClient{botCommands: catalog}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, LoadBotCommands{RequestID: 41, ChatID: 9})
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	loaded, ok := events[0].(BotCommandsLoaded)
	if !ok || loaded.RequestID != 41 || loaded.ChatID != 9 || !reflect.DeepEqual(loaded.Commands, catalog) {
		t.Fatalf("loaded event = %#v", events[0])
	}
	if client.botCommandsChat != 9 {
		t.Fatalf("client chat = %d", client.botCommandsChat)
	}
}

func TestHandlerSendPhotoSuccess(t *testing.T) {
	client := &handlerClient{}
	client.sent = domain.Message{
		ID: 100, ChatID: 9, Kind: domain.MessagePhoto,
		Outgoing: true, SendState: domain.SendSucceeded,
	}
	renderer := &handlerAvatarRenderer{}
	handler := newTestHandler(t, client, renderer)
	events := collectHandlerEvents(handler, SendPhoto{
		RequestID: 1, LocalID: domain.MessageID(-5),
		ChatID: 9, LocalPath: "/path/photo.jpg",
		Caption: "caption", ReplyToMessageID: 42,
	})
	if len(events) != 1 {
		t.Fatalf("emitted %d events, want 1", len(events))
	}
	ev, ok := events[0].(PhotoQueued)
	if !ok || ev.RequestID != 1 || ev.LocalID != -5 || ev.ChatID != 9 {
		t.Fatalf("event = %#v", events[0])
	}
	if client.photoSendRequest.LocalPath != "/path/photo.jpg" {
		t.Fatalf("localPath = %q", client.photoSendRequest.LocalPath)
	}
	if client.photoSendRequest.Caption != "caption" {
		t.Fatalf("caption = %q", client.photoSendRequest.Caption)
	}
	if client.photoSendRequest.ReplyToMessageID != 42 {
		t.Fatalf("replyTo = %d", client.photoSendRequest.ReplyToMessageID)
	}
}

func TestHandlerSendPhotoTransportFailureSanitized(t *testing.T) {
	rawDetail := "/private/path SECRET_CAPTION RAW_DETAIL"
	client := &handlerClient{err: errors.New(rawDetail)}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	now := time.Unix(999, 0).UTC()
	handler.now = func() time.Time { return now }
	events := collectHandlerEvents(handler, SendPhoto{
		RequestID: 7, LocalID: domain.MessageID(-3),
		ChatID: 9, LocalPath: "/private/path",
		Caption:          "SECRET_CAPTION",
		ReplyToMessageID: 14,
	})
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	failed, ok := events[0].(PhotoQueueFailed)
	if !ok || failed.RequestID != 7 || failed.LocalID != -3 || failed.ChatID != 9 || failed.FailedAt != now {
		t.Fatalf("correlation = %#v", failed)
	}
	if failed.Error.Kind != domain.ErrorInternal {
		t.Fatalf("kind = %v", failed.Error.Kind)
	}
	if failed.Error.Op != "send photo" {
		t.Fatalf("op = %q", failed.Error.Op)
	}
	// Effect fields are public; they must contain path/caption.
	// Error.Message must contain none of the sentinels.
	if strings.Contains(failed.Error.Message, "/private/path") || strings.Contains(failed.Error.Message, "SECRET_CAPTION") || strings.Contains(failed.Error.Message, "RAW_DETAIL") {
		t.Fatalf("raw leaked into message: %q", failed.Error.Message)
	}
	if failed.Error.Cause == nil {
		t.Fatal("error should have cause")
	}
	// Error() must also be clean.
	if strings.Contains(failed.Error.Error(), "/private/path") || strings.Contains(failed.Error.Error(), "SECRET") {
		t.Fatalf("Error() leaked: %q", failed.Error.Error())
	}
	// Follow safeError cause conventions: raw errors become the Cause.
	if failed.Error.Cause == nil {
		t.Fatal("cause is nil")
	}
	var causeErr error = failed.Error.Cause
	if causeErr.Error() == "" {
		t.Fatalf("cause error message is empty")
	}
}

func TestHandlerSendPhotoUnavailableClient(t *testing.T) {
	handler := &Handler{workCtx: context.Background(), now: time.Now}
	events := collectHandlerEvents(handler, SendPhoto{
		RequestID: 8, LocalID: domain.MessageID(-4),
		ChatID: 9, LocalPath: "/path",
	})
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	failed, ok := events[0].(PhotoQueueFailed)
	if !ok {
		t.Fatalf("event = %#v", events[0])
	}
	if failed.Error.Kind != domain.ErrorInternal || failed.Error.Op != "send photo" {
		t.Fatalf("error = %#v", failed.Error)
	}
	if strings.Contains(failed.Error.Message, "Telegram client unavailable") || strings.Contains(failed.Error.Error(), "Telegram client unavailable") {
		t.Fatal("raw client message leaked")
	}
}

func TestHandlerOpenMessageMediaSuccessEmitAndCallCorrelation(t *testing.T) {
	client := &handlerClient{
		err:        nil,
		avatarFile: telegram.LocalFile{Path: "/tmp/photo.jpg"},
	}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, OpenMessageMediaFile{RequestID: 42, ChatID: 9, MessageID: 2, Title: "Photo", File: domain.MediaFileRef{ID: 101}})
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	opened, ok := events[0].(MessageMediaOpened)
	if !ok {
		t.Fatalf("event type = %T, want MessageMediaOpened", events[0])
	}
	if opened.RequestID != 42 || opened.ChatID != 9 || opened.MessageID != 2 || opened.Title != "Photo" {
		t.Fatalf("opened = %#v", opened)
	}
	if !opened.File.Downloaded || opened.File.LocalPath != "/tmp/photo.jpg" {
		t.Fatalf("file = %#v", opened.File)
	}
	client.mu.Lock()
	if len(client.mediaCalls) != 1 || client.mediaCalls[0].ID != 101 {
		t.Fatalf("media calls = %#v", client.mediaCalls)
	}
	client.mu.Unlock()
}

func TestHandlerOpenMessageMediaFailureEmitsConstantError(t *testing.T) {
	client := &handlerClient{err: errors.New("raw transport secret 101")}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, OpenMessageMediaFile{RequestID: 43, ChatID: 10, MessageID: 5, Title: "Photo", File: domain.MediaFileRef{ID: 101}})
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	failed, ok := events[0].(MessageMediaOpenFailed)
	if !ok {
		t.Fatalf("event type = %T, want MessageMediaOpenFailed", events[0])
	}
	if failed.RequestID != 43 || failed.ChatID != 10 || failed.MessageID != 5 {
		t.Fatalf("failed = %#v", failed)
	}
	if failed.Error.Kind != domain.ErrorMedia || failed.Error.Op != "open message media" || failed.Error.Message != "Could not open image" {
		t.Fatalf("error = %#v", failed.Error)
	}
	if failed.Error.Cause == nil {
		t.Fatal("error should have cause")
	}
}

func TestHandlerOpenMessageMediaUnavailableClient(t *testing.T) {
	handler := NewHandler(context.Background(), nil, nil, nil, nil)
	events := collectHandlerEvents(handler, OpenMessageMediaFile{RequestID: 44, ChatID: 10, MessageID: 5, Title: "Photo", File: domain.MediaFileRef{ID: 101}})
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	failed, ok := events[0].(MessageMediaOpenFailed)
	if !ok {
		t.Fatalf("event type = %T, want MessageMediaOpenFailed", events[0])
	}
	if failed.Error.Cause == nil {
		t.Fatal("error should have cause")
	}
}

func TestHandlerOpenMessageMediaTitleAwareDownloadFailure(t *testing.T) {
	private := "/private/media-path transport cause"
	for _, test := range []struct {
		title       string
		wantMessage string
		wantCause   bool
	}{
		{title: "Photo", wantMessage: "Could not open image", wantCause: true},
		{title: "Video", wantMessage: "Could not open video"},
		{title: "Audio", wantMessage: "Could not open audio"},
		{title: "File", wantMessage: "Could not open file"},
		{title: "Animation", wantMessage: "Could not open animation"},
		{title: "Voice note", wantMessage: "Could not open voice note"},
		{title: "Video note", wantMessage: "Could not open video note"},
	} {
		t.Run(test.title, func(t *testing.T) {
			client := &handlerClient{err: errors.New(private)}
			handler := newTestHandler(t, client, &handlerAvatarRenderer{})
			events := collectHandlerEvents(handler, OpenMessageMediaFile{RequestID: 45, ChatID: 10, MessageID: 5, Title: test.title, File: domain.MediaFileRef{ID: 101}})
			if len(events) != 1 {
				t.Fatalf("events = %#v, want one", events)
			}
			failed, ok := events[0].(MessageMediaOpenFailed)
			if !ok || failed.RequestID != 45 || failed.ChatID != 10 || failed.MessageID != 5 {
				t.Fatalf("failure = %#v", events[0])
			}
			if failed.Error.Kind != domain.ErrorMedia || failed.Error.Op != "open message media" || failed.Error.Message != test.wantMessage || strings.Contains(failed.Error.Error(), private) {
				t.Fatalf("safe error = %#v", failed.Error)
			}
			if test.wantCause != (failed.Error.Cause != nil) {
				t.Fatalf("Cause presence = %t, want %t", failed.Error.Cause != nil, test.wantCause)
			}
			if !test.wantCause && failed.Error.Cause != nil {
				t.Fatalf("external-opener error leaked cause: %v", failed.Error.Cause)
			}
		})
	}
}

func TestHandlerOpenAudioUnavailableClientUsesSafeAudioFailure(t *testing.T) {
	handler := NewHandler(context.Background(), nil, nil, nil, nil)
	events := collectHandlerEvents(handler, OpenMessageMediaFile{RequestID: 46, ChatID: 10, MessageID: 6, Title: "Audio", File: domain.MediaFileRef{ID: 102}})
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one", events)
	}
	failed, ok := events[0].(MessageMediaOpenFailed)
	if !ok || failed.RequestID != 46 || failed.ChatID != 10 || failed.MessageID != 6 {
		t.Fatalf("failure = %#v", events[0])
	}
	if failed.Error.Kind != domain.ErrorMedia || failed.Error.Op != "open message media" || failed.Error.Message != "Could not open audio" || failed.Error.Cause != nil {
		t.Fatalf("safe Audio client failure = %#v", failed.Error)
	}
	for _, private := range []string{"Telegram client unavailable", "operation failed", "/private/"} {
		if strings.Contains(failed.Error.Error(), private) {
			t.Fatalf("Audio client failure leaked %q: %#v", private, failed.Error)
		}
	}
}

func TestHandlerOpenMessageMediaFailurePreservesCorrelation(t *testing.T) {
	client := &handlerClient{err: errors.New("raw detail")}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, OpenMessageMediaFile{
		RequestID: 50, ChatID: 99, MessageID: 100, Title: "Custom",
		File: domain.MediaFileRef{ID: 200, CanDownload: true},
	})
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	failed, ok := events[0].(MessageMediaOpenFailed)
	if !ok {
		t.Fatalf("event type = %T, want MessageMediaOpenFailed", events[0])
	}
	if failed.RequestID != 50 {
		t.Fatalf("request ID = %d, want 50", failed.RequestID)
	}
	if failed.ChatID != 99 {
		t.Fatalf("chat ID = %d, want 99", failed.ChatID)
	}
	if failed.MessageID != 100 {
		t.Fatalf("message ID = %d, want 100", failed.MessageID)
	}
	if failed.Error.Op != "open message media" {
		t.Fatalf("error op = %q, want open message media", failed.Error.Op)
	}
}

func TestHandlerOpenMessageMediaSuccessPreservesCorrelation(t *testing.T) {
	client := &handlerClient{
		err:        nil,
		avatarFile: telegram.LocalFile{Path: "/tmp/correlated.jpg"},
	}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, OpenMessageMediaFile{
		RequestID: 55, ChatID: 88, MessageID: 77, Title: "Correlated",
		File: domain.MediaFileRef{ID: 300, CanDownload: true},
	})
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	opened, ok := events[0].(MessageMediaOpened)
	if !ok {
		t.Fatalf("event type = %T, want MessageMediaOpened", events[0])
	}
	if opened.RequestID != 55 || opened.ChatID != 88 || opened.MessageID != 77 || opened.Title != "Correlated" {
		t.Fatalf("correlation mismatch: request=%d chat=%d message=%d title=%s",
			opened.RequestID, opened.ChatID, opened.MessageID, opened.Title)
	}
	client.mu.Lock()
	if len(client.mediaCalls) != 1 || client.mediaCalls[0].ID != 300 {
		t.Fatalf("media calls = %#v", client.mediaCalls)
	}
	client.mu.Unlock()
}

func TestHandlerOpenMessageMediaMapsExactSuccessAndSafeFailure(t *testing.T) {
	// Success path: exact full MediaFileRef input/output, one transport call.
	successClient := &handlerClient{
		err:        nil,
		avatarFile: telegram.LocalFile{Path: "/tmp/exact.jpg"},
	}
	handler := newTestHandler(t, successClient, &handlerAvatarRenderer{})
	inputFile := domain.MediaFileRef{ID: 500, UniqueID: "exact-500", Size: 12345, ExpectedSize: 12345, CanDownload: true}
	successEvents := collectHandlerEvents(handler, OpenMessageMediaFile{
		RequestID: 60, ChatID: 7, MessageID: 15, Title: "ExactTitle",
		File: inputFile,
	})
	if len(successEvents) != 1 {
		t.Fatalf("success events count = %d, want 1", len(successEvents))
	}
	opened, ok := successEvents[0].(MessageMediaOpened)
	if !ok {
		t.Fatalf("success event type = %T, want MessageMediaOpened", successEvents[0])
	}
	// Full correlation check.
	if opened.RequestID != 60 || opened.ChatID != 7 || opened.MessageID != 15 || opened.Title != "ExactTitle" {
		t.Fatalf("success correlation: request=%d chat=%d message=%d title=%s",
			opened.RequestID, opened.ChatID, opened.MessageID, opened.Title)
	}
	// Exact full MediaFileRef: original fields preserved, Downloaded=true, LocalPath=returned.
	wantFile := domain.MediaFileRef{
		ID: 500, UniqueID: "exact-500", Size: 12345, ExpectedSize: 12345,
		CanDownload: true, Downloaded: true, LocalPath: "/tmp/exact.jpg",
	}
	if !reflect.DeepEqual(opened.File, wantFile) {
		t.Fatalf("success file = %#v, want %#v", opened.File, wantFile)
	}
	successClient.mu.Lock()
	if len(successClient.mediaCalls) != 1 {
		t.Fatalf("media calls count = %d, want 1", len(successClient.mediaCalls))
	}
	if !reflect.DeepEqual(successClient.mediaCalls[0], inputFile) {
		t.Fatalf("media call ref = %#v, want %#v", successClient.mediaCalls[0], inputFile)
	}
	successClient.mu.Unlock()

	// Failure path: exact correlation with constant safe error, raw private absent.
	failClient := &handlerClient{err: errors.New("raw sentinel 501 private")}
	failHandler := newTestHandler(t, failClient, &handlerAvatarRenderer{})
	failEvents := collectHandlerEvents(failHandler, OpenMessageMediaFile{
		RequestID: 61, ChatID: 8, MessageID: 16, Title: "FailTitle",
		File: domain.MediaFileRef{ID: 501, CanDownload: true},
	})
	if len(failEvents) != 1 {
		t.Fatalf("failure events count = %d, want 1", len(failEvents))
	}
	failed, ok := failEvents[0].(MessageMediaOpenFailed)
	if !ok {
		t.Fatalf("failure event type = %T, want MessageMediaOpenFailed", failEvents[0])
	}
	if failed.RequestID != 61 || failed.ChatID != 8 || failed.MessageID != 16 {
		t.Fatalf("failure correlation: request=%d chat=%d message=%d",
			failed.RequestID, failed.ChatID, failed.MessageID)
	}
	if failed.Error.Kind != domain.ErrorMedia {
		t.Fatalf("error kind = %v, want ErrorMedia", failed.Error.Kind)
	}
	if failed.Error.Op != "open message media" {
		t.Fatalf("error op = %q, want open message media", failed.Error.Op)
	}
	if failed.Error.Message != "Could not open image" {
		t.Fatalf("error message = %q, want Could not open image", failed.Error.Message)
	}
	// Raw private sentinel absent from user-facing Message.
	if strings.Contains(failed.Error.Message, "raw sentinel") {
		t.Fatal("raw sentinel leaked into user-facing error message")
	}
	if strings.Contains(failed.Error.Message, "private") {
		t.Fatal("private leaked into user-facing error message")
	}
	if failed.Error.Cause == nil {
		t.Fatal("error should have cause")
	}
	if failed.Error.Cause.Error() != "raw sentinel 501 private" {
		t.Fatalf("cause error = %q, want raw sentinel 501 private", failed.Error.Cause.Error())
	}

	// Unavailable client (nil): same safe Kind/Op/Message, no raw Telegram message.
	nilHandler := NewHandler(context.Background(), nil, nil, nil, nil)
	nilEvents := collectHandlerEvents(nilHandler, OpenMessageMediaFile{
		RequestID: 62, ChatID: 8, MessageID: 17, Title: "NilClientTitle",
		File: domain.MediaFileRef{ID: 502, CanDownload: true},
	})
	if len(nilEvents) != 1 {
		t.Fatalf("nil client events count = %d, want 1", len(nilEvents))
	}
	nilFailed, ok := nilEvents[0].(MessageMediaOpenFailed)
	if !ok {
		t.Fatalf("nil client event type = %T, want MessageMediaOpenFailed", nilEvents[0])
	}
	if nilFailed.RequestID != 62 || nilFailed.ChatID != 8 || nilFailed.MessageID != 17 {
		t.Fatalf("nil client correlation mismatch: request=%d chat=%d message=%d", nilFailed.RequestID, nilFailed.ChatID, nilFailed.MessageID)
	}
	if nilFailed.Error.Kind != domain.ErrorMedia {
		t.Fatalf("nil client error kind = %v, want ErrorMedia", nilFailed.Error.Kind)
	}
	if nilFailed.Error.Op != "open message media" {
		t.Fatalf("nil client error op = %q, want open message media", nilFailed.Error.Op)
	}
	if nilFailed.Error.Message != "Could not open image" {
		t.Fatalf("nil client error message = %q, want Could not open image", nilFailed.Error.Message)
	}
	if nilFailed.Error.Cause == nil {
		t.Fatal("nil client error should have cause")
	}
	var unavailable domain.AppError
	if !errors.As(nilFailed.Error.Cause, &unavailable) {
		t.Fatalf("nil client cause type = %T, want domain.AppError", nilFailed.Error.Cause)
	}
	if unavailable.Kind != domain.ErrorInternal ||
		unavailable.Op != "open message media" ||
		unavailable.Message != "operation failed; retry or inspect the sanitized log" ||
		unavailable.RetryAfter != 0 {
		t.Fatalf("nil client sanitized cause = %#v", unavailable)
	}
	if unavailable.Cause == nil || unavailable.Cause.Error() != "Telegram client unavailable" {
		t.Fatalf("nil client root cause = %v, want Telegram client unavailable", unavailable.Cause)
	}
}

func TestHandlerThumbnailDownloadSuccess(t *testing.T) {
	client := &handlerClient{
		err:        nil,
		avatarFile: telegram.LocalFile{Path: "/tmp/thumb-dl.jpg"},
	}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, DownloadThumbnail{RequestID: 71, ChatID: 9, MessageID: 2, File: domain.MediaFileRef{ID: 7, CanDownload: true}})
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	downloaded, ok := events[0].(ThumbnailDownloaded)
	if !ok {
		t.Fatalf("event type = %T, want ThumbnailDownloaded", events[0])
	}
	if downloaded.RequestID != 71 || downloaded.ChatID != 9 || downloaded.MessageID != 2 {
		t.Fatalf("correlation mismatch: request=%d chat=%d message=%d", downloaded.RequestID, downloaded.ChatID, downloaded.MessageID)
	}
	if !downloaded.File.Downloaded || downloaded.File.LocalPath != "/tmp/thumb-dl.jpg" {
		t.Fatalf("file = %#v", downloaded.File)
	}
	client.mu.Lock()
	if len(client.mediaCalls) != 1 || client.mediaCalls[0].ID != 7 {
		t.Fatalf("media calls = %#v", client.mediaCalls)
	}
	client.mu.Unlock()
}

func TestHandlerThumbnailDownloadFailure(t *testing.T) {
	client := &handlerClient{err: errors.New("raw transport secret 707")}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	events := collectHandlerEvents(handler, DownloadThumbnail{RequestID: 72, ChatID: 10, MessageID: 5, File: domain.MediaFileRef{ID: 7}})
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	failed, ok := events[0].(ThumbnailDownloadFailed)
	if !ok {
		t.Fatalf("event type = %T, want ThumbnailDownloadFailed", events[0])
	}
	if failed.RequestID != 72 || failed.ChatID != 10 || failed.MessageID != 5 {
		t.Fatalf("correlation mismatch: request=%d chat=%d message=%d", failed.RequestID, failed.ChatID, failed.MessageID)
	}
	if failed.Error.Op != "download thumbnail" {
		t.Fatalf("error op = %q, want download thumbnail", failed.Error.Op)
	}
	if strings.Contains(failed.Error.Message, "raw transport secret 707") {
		t.Fatalf("raw transport leak in sanitized message: %q", failed.Error.Message)
	}
	if failed.Error.Cause == nil {
		t.Fatal("error should have a cause")
	}
}

func TestHandlerThumbnailDownloadUnavailableClient(t *testing.T) {
	handler := NewHandler(context.Background(), nil, nil, nil, nil)
	events := collectHandlerEvents(handler, DownloadThumbnail{RequestID: 73, ChatID: 10, MessageID: 5, File: domain.MediaFileRef{ID: 7}})
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	failed, ok := events[0].(ThumbnailDownloadFailed)
	if !ok {
		t.Fatalf("event type = %T, want ThumbnailDownloadFailed", events[0])
	}
	if failed.Error.Kind != domain.ErrorInternal {
		t.Fatalf("error kind = %v, want ErrorInternal", failed.Error.Kind)
	}
	if failed.Error.Op != "download thumbnail" {
		t.Fatalf("error op = %q, want download thumbnail", failed.Error.Op)
	}
	if failed.Error.Message != "operation failed; retry or inspect the sanitized log" {
		t.Fatalf("error message = %q", failed.Error.Message)
	}
	if failed.Error.Cause == nil || failed.Error.Cause.Error() != "Telegram client unavailable" {
		t.Fatalf("cause = %v, want Telegram client unavailable", failed.Error.Cause)
	}
}

// fakeThumbnailRenderer implements thumbnail.Renderer for testing.
type fakeThumbnailRenderer struct {
	result thumbnail.Block
	err    error
	path   string
}

func (f *fakeThumbnailRenderer) Render(path string, width, height int) (thumbnail.Block, error) {
	f.path = path
	return f.result, f.err
}

func TestHandlerThumbnailRenderedEmittedOnSuccess(t *testing.T) {
	client := &handlerClient{
		err:        nil,
		avatarFile: telegram.LocalFile{Path: "/tmp/thumb-render.jpg"},
	}
	renderer := &fakeThumbnailRenderer{
		result: thumbnail.Block{Text: "\u2584\u2580", Width: 20, Height: 8},
	}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	handler.SetThumbnailRenderer(renderer)

	events := collectHandlerEvents(handler, DownloadThumbnail{RequestID: 80, ChatID: 9, MessageID: 2, File: domain.MediaFileRef{ID: 7, CanDownload: true}})
	if len(events) != 2 {
		t.Fatalf("events count = %d, want 2", len(events))
	}
	// First event: ThumbnailDownloaded
	downloaded, ok := events[0].(ThumbnailDownloaded)
	if !ok {
		t.Fatalf("first event type = %T, want ThumbnailDownloaded", events[0])
	}
	if downloaded.RequestID != 80 || downloaded.ChatID != 9 || downloaded.MessageID != 2 {
		t.Fatalf("downloaded correlation mismatch: request=%d chat=%d message=%d", downloaded.RequestID, downloaded.ChatID, downloaded.MessageID)
	}
	// Second event: ThumbnailRendered
	rendered, ok := events[1].(ThumbnailRendered)
	if !ok {
		t.Fatalf("second event type = %T, want ThumbnailRendered", events[1])
	}
	if rendered.RequestID != 80 || rendered.ChatID != 9 || rendered.MessageID != 2 {
		t.Fatalf("rendered correlation mismatch: request=%d chat=%d message=%d", rendered.RequestID, rendered.ChatID, rendered.MessageID)
	}
	if rendered.Block.Width != 20 || rendered.Block.Height != 8 {
		t.Fatalf("rendered block = width=%d height=%d, want 20x8", rendered.Block.Width, rendered.Block.Height)
	}
}

func TestHandlerThumbnailNoRenderWhenRendererNil(t *testing.T) {
	client := &handlerClient{
		err:        nil,
		avatarFile: telegram.LocalFile{Path: "/tmp/thumb-norender.jpg"},
	}
	handler := newTestHandler(t, client, &handlerAvatarRenderer{})
	// Deliberately do NOT call SetThumbnailRenderer.

	events := collectHandlerEvents(handler, DownloadThumbnail{RequestID: 90, ChatID: 9, MessageID: 3, File: domain.MediaFileRef{ID: 8}})
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	if _, ok := events[0].(ThumbnailRendered); ok {
		t.Fatalf("unexpected ThumbnailRendered event when renderer is nil")
	}
	if _, ok := events[0].(ThumbnailDownloaded); !ok {
		t.Fatalf("expected ThumbnailDownloaded, got %T", events[0])
	}
}
