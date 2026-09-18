//go:build tdlib

package telegram

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

type lifecycleTransport struct {
	closeCalls  atomic.Int32
	logoutCalls atomic.Int32
	closeCalled chan struct{}
	calledOnce  sync.Once
	err         error
}

type controlledCloseTransport struct {
	closeCalls      atomic.Int32
	logoutCalls     atomic.Int32
	closeCalled     chan struct{}
	requestContexts chan context.Context
	release         chan struct{}
	calledOnce      sync.Once
}

func newControlledCloseTransport() *controlledCloseTransport {
	return &controlledCloseTransport{
		closeCalled:     make(chan struct{}),
		requestContexts: make(chan context.Context, 1),
		release:         make(chan struct{}),
	}
}

func (t *controlledCloseTransport) Close(ctx context.Context, _ *td.Client) (*td.Ok, error) {
	t.closeCalls.Add(1)
	t.calledOnce.Do(func() { close(t.closeCalled) })
	t.requestContexts <- ctx
	select {
	case <-t.release:
		return &td.Ok{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *controlledCloseTransport) LogOut(context.Context, *td.Client) (*td.Ok, error) {
	t.logoutCalls.Add(1)
	return &td.Ok{}, nil
}

func newLifecycleTransport() *lifecycleTransport {
	return &lifecycleTransport{closeCalled: make(chan struct{})}
}

func (t *lifecycleTransport) Close(context.Context, *td.Client) (*td.Ok, error) {
	t.closeCalls.Add(1)
	t.calledOnce.Do(func() { close(t.closeCalled) })
	return &td.Ok{}, t.err
}

func (t *lifecycleTransport) LogOut(context.Context, *td.Client) (*td.Ok, error) {
	t.logoutCalls.Add(1)
	return &td.Ok{}, nil
}

type adapterHarness struct {
	adapter       *Adapter
	resultHandler td.ResultHandler
	factoryCalls  int
	transport     *lifecycleTransport
}

func newAdapterHarness() *adapterHarness {
	harness := &adapterHarness{transport: newLifecycleTransport()}
	harness.adapter = newAdapter(
		config.Runtime{},
		&recordingPrompter{},
		func(_ td.AuthorizationStateHandler, resultHandler td.ResultHandler) (*td.Client, error) {
			harness.factoryCalls++
			harness.resultHandler = resultHandler
			return &td.Client{}, nil
		},
		harness.transport.Close,
	)
	return harness
}

func TestAdapterStartEmitsReadyThenClosed(t *testing.T) {
	harness := newAdapterHarness()
	updates := make(chan Update, 2)
	result := make(chan error, 1)
	go func() { result <- harness.adapter.Start(context.Background(), updates) }()

	if _, ok := receiveAdapterUpdate(t, updates).(Ready); !ok {
		t.Fatal("first adapter update is not Ready")
	}
	harness.resultHandler.OnResult(&td.UpdateAuthorizationState{AuthorizationState: &td.AuthorizationStateClosed{}})
	if _, ok := receiveAdapterUpdate(t, updates).(Closed); !ok {
		t.Fatal("second adapter update is not Closed")
	}
	if err := receiveAdapterError(t, result); err != nil {
		t.Fatal("Start returned an error after closed authorization state")
	}
	if harness.factoryCalls != 1 {
		t.Fatalf("client factory calls = %d, want 1", harness.factoryCalls)
	}
}

func TestAdapterStartDoesNotBlockOnClosedStateDuringClientCreation(t *testing.T) {
	callbackReturned := make(chan struct{})
	releaseFactory := make(chan struct{})
	adapter := newAdapter(
		config.Runtime{},
		&recordingPrompter{},
		func(_ td.AuthorizationStateHandler, resultHandler td.ResultHandler) (*td.Client, error) {
			resultHandler.OnResult(&td.UpdateAuthorizationState{AuthorizationState: &td.AuthorizationStateClosed{}})
			close(callbackReturned)
			<-releaseFactory
			return &td.Client{}, nil
		},
		newLifecycleTransport().Close,
	)
	updates := make(chan Update)
	result := make(chan error, 1)
	go func() { result <- adapter.Start(context.Background(), updates) }()

	receiveSignal(t, callbackReturned, "closed-state callback return")
	close(releaseFactory)
	if _, ok := receiveAdapterUpdate(t, updates).(Ready); !ok {
		t.Fatal("first adapter update is not Ready")
	}
	assertAdapterCallBlocked(t, result)
	if _, ok := receiveAdapterUpdate(t, updates).(Closed); !ok {
		t.Fatal("second adapter update is not Closed")
	}
	if err := receiveAdapterError(t, result); err != nil {
		t.Fatal("Start returned an error after early closed authorization state")
	}
}

func TestAdapterStartReturnsOnContextCancellation(t *testing.T) {
	harness := newAdapterHarness()
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan Update, 1)
	result := make(chan error, 1)
	go func() { result <- harness.adapter.Start(ctx, updates) }()
	receiveAdapterUpdate(t, updates)

	cancel()
	if err := receiveAdapterError(t, result); !errors.Is(err, context.Canceled) {
		t.Fatal("Start does not preserve context cancellation")
	}
	if harness.transport.closeCalls.Load() != 0 {
		t.Fatal("Start cancellation issued an implicit close request")
	}
}

func TestAdapterStartHasSingleOwner(t *testing.T) {
	harness := newAdapterHarness()
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan Update, 1)
	first := make(chan error, 1)
	go func() { first <- harness.adapter.Start(ctx, updates) }()
	receiveAdapterUpdate(t, updates)

	err := harness.adapter.Start(context.Background(), make(chan Update, 1))
	assertAdapterError(t, err, "start Telegram")
	if harness.factoryCalls != 1 {
		t.Fatalf("client factory calls = %d, want 1", harness.factoryCalls)
	}
	cancel()
	receiveAdapterError(t, first)
}

func TestAdapterStartRejectsNilUpdateChannelBeforeCreatingClient(t *testing.T) {
	harness := newAdapterHarness()
	err := harness.adapter.Start(context.Background(), nil)
	assertAdapterError(t, err, "start Telegram")
	if harness.factoryCalls != 0 {
		t.Fatalf("client factory calls = %d, want 0", harness.factoryCalls)
	}
}

func TestAdapterStartHidesClientFactoryError(t *testing.T) {
	privateCause := errors.New("factory included api-hash-private-value and database-key-private-value")
	adapter := newAdapter(
		testRuntime(),
		&recordingPrompter{},
		func(td.AuthorizationStateHandler, td.ResultHandler) (*td.Client, error) { return nil, privateCause },
		newLifecycleTransport().Close,
	)
	err := adapter.Start(context.Background(), make(chan Update, 1))
	assertAdapterError(t, err, "start Telegram")
	if !errors.Is(err, privateCause) {
		t.Fatal("Start error does not preserve its cause")
	}
	if strings.Contains(err.Error(), testAPIHash) || strings.Contains(err.Error(), testDatabaseKey) {
		t.Fatal("Start error exposes runtime secrets")
	}
}

func TestAdapterCloseWaitsForClosedAuthorizationState(t *testing.T) {
	harness := newAdapterHarness()
	updates := make(chan Update, 2)
	startResult := make(chan error, 1)
	go func() { startResult <- harness.adapter.Start(context.Background(), updates) }()
	receiveAdapterUpdate(t, updates)

	closeResult := make(chan error, 1)
	go func() { closeResult <- harness.adapter.Close(context.Background()) }()
	receiveSignal(t, harness.transport.closeCalled, "close request")
	assertAdapterCallPending(t, closeResult)

	harness.resultHandler.OnResult(&td.UpdateAuthorizationState{AuthorizationState: &td.AuthorizationStateClosed{}})
	if err := receiveAdapterError(t, closeResult); err != nil {
		t.Fatal("Close returned an error after closed authorization state")
	}
	receiveAdapterUpdate(t, updates)
	if err := receiveAdapterError(t, startResult); err != nil {
		t.Fatal("Start returned an error after close completion")
	}
	if harness.transport.closeCalls.Load() != 1 || harness.transport.logoutCalls.Load() != 0 {
		t.Fatal("adapter did not preserve the session during close")
	}
}

func TestAdapterCloseDoesNotCompleteForClosingState(t *testing.T) {
	harness := newAdapterHarness()
	updates := make(chan Update, 2)
	startResult := make(chan error, 1)
	go func() { startResult <- harness.adapter.Start(context.Background(), updates) }()
	receiveAdapterUpdate(t, updates)

	ctx, cancel := context.WithCancel(context.Background())
	closeResult := make(chan error, 1)
	go func() { closeResult <- harness.adapter.Close(ctx) }()
	receiveSignal(t, harness.transport.closeCalled, "close request")
	harness.resultHandler.OnResult(&td.UpdateAuthorizationState{AuthorizationState: &td.AuthorizationStateClosing{}})
	assertAdapterCallPending(t, closeResult)

	cancel()
	if err := receiveAdapterError(t, closeResult); !errors.Is(err, context.Canceled) {
		t.Fatal("Close does not preserve context cancellation while waiting for closed state")
	}
	harness.resultHandler.OnResult(&td.UpdateAuthorizationState{AuthorizationState: &td.AuthorizationStateClosed{}})
	receiveAdapterUpdate(t, updates)
	receiveAdapterError(t, startResult)
}

func TestAdapterCloseTimesOutWithoutClosedAuthorizationState(t *testing.T) {
	harness := newAdapterHarness()
	updates := make(chan Update, 1)
	startCtx, cancelStart := context.WithCancel(context.Background())
	startResult := make(chan error, 1)
	go func() { startResult <- harness.adapter.Start(startCtx, updates) }()
	receiveAdapterUpdate(t, updates)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := harness.adapter.Close(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Close does not preserve its context deadline")
	}
	cancelStart()
	receiveAdapterError(t, startResult)
}

func TestAdapterCloseHidesTransportError(t *testing.T) {
	harness := newAdapterHarness()
	privateCause := errors.New("close error included api-hash-private-value and database-key-private-value")
	harness.transport.err = privateCause
	updates := make(chan Update, 1)
	startCtx, cancelStart := context.WithCancel(context.Background())
	startResult := make(chan error, 1)
	go func() { startResult <- harness.adapter.Start(startCtx, updates) }()
	receiveAdapterUpdate(t, updates)

	err := harness.adapter.Close(context.Background())
	assertAdapterError(t, err, "close Telegram")
	if !errors.Is(err, privateCause) {
		t.Fatal("Close error does not preserve its cause")
	}
	if strings.Contains(err.Error(), testAPIHash) || strings.Contains(err.Error(), testDatabaseKey) {
		t.Fatal("Close error exposes runtime secrets")
	}
	cancelStart()
	receiveAdapterError(t, startResult)
}

func TestAdapterConcurrentCloseIssuesOneCloseAndNoLogout(t *testing.T) {
	harness := newAdapterHarness()
	updates := make(chan Update, 2)
	startResult := make(chan error, 1)
	go func() { startResult <- harness.adapter.Start(context.Background(), updates) }()
	receiveAdapterUpdate(t, updates)

	const callers = 16
	results := make(chan error, callers)
	for range callers {
		go func() { results <- harness.adapter.Close(context.Background()) }()
	}
	receiveSignal(t, harness.transport.closeCalled, "close request")
	harness.resultHandler.OnResult(&td.UpdateAuthorizationState{AuthorizationState: &td.AuthorizationStateClosed{}})
	for range callers {
		if err := receiveAdapterError(t, results); err != nil {
			t.Fatal("concurrent Close returned an error")
		}
	}
	if harness.transport.closeCalls.Load() != 1 {
		t.Fatalf("close request calls = %d, want 1", harness.transport.closeCalls.Load())
	}
	if harness.transport.logoutCalls.Load() != 0 {
		t.Fatalf("logout request calls = %d, want 0", harness.transport.logoutCalls.Load())
	}
	receiveAdapterUpdate(t, updates)
	receiveAdapterError(t, startResult)
}

func TestAdapterCloseRequestOutlivesFirstCaller(t *testing.T) {
	transport := newControlledCloseTransport()
	harness := &adapterHarness{}
	harness.adapter = newAdapter(
		config.Runtime{},
		&recordingPrompter{},
		func(_ td.AuthorizationStateHandler, resultHandler td.ResultHandler) (*td.Client, error) {
			harness.resultHandler = resultHandler
			return &td.Client{}, nil
		},
		transport.Close,
	)
	updates := make(chan Update, 2)
	startResult := make(chan error, 1)
	go func() { startResult <- harness.adapter.Start(context.Background(), updates) }()
	receiveAdapterUpdate(t, updates)

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() { firstResult <- harness.adapter.Close(firstCtx) }()
	receiveSignal(t, transport.closeCalled, "controlled close request")
	requestCtx := receiveRequestContext(t, transport.requestContexts)
	cancelFirst()
	if err := receiveAdapterError(t, firstResult); !errors.Is(err, context.Canceled) {
		t.Fatal("first Close does not preserve its caller cancellation")
	}
	select {
	case <-requestCtx.Done():
		t.Fatal("first caller cancellation canceled the shared close request")
	default:
	}

	secondResult := make(chan error, 1)
	go func() { secondResult <- harness.adapter.Close(context.Background()) }()
	assertAdapterCallPending(t, secondResult)
	close(transport.release)
	harness.resultHandler.OnResult(&td.UpdateAuthorizationState{AuthorizationState: &td.AuthorizationStateClosed{}})
	if err := receiveAdapterError(t, secondResult); err != nil {
		t.Fatal("second Close did not complete the shared close request")
	}
	if transport.closeCalls.Load() != 1 {
		t.Fatalf("close request calls = %d, want 1", transport.closeCalls.Load())
	}
	if transport.logoutCalls.Load() != 0 {
		t.Fatalf("logout request calls = %d, want 0", transport.logoutCalls.Load())
	}
	receiveAdapterUpdate(t, updates)
	if err := receiveAdapterError(t, startResult); err != nil {
		t.Fatal("Start returned an error after controlled close completion")
	}
}

func TestAdapterCloseWithCanceledContextDoesNotIssueRequest(t *testing.T) {
	harness := newAdapterHarness()
	updates := make(chan Update, 1)
	startCtx, cancelStart := context.WithCancel(context.Background())
	startResult := make(chan error, 1)
	go func() { startResult <- harness.adapter.Start(startCtx, updates) }()
	receiveAdapterUpdate(t, updates)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := harness.adapter.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("Close does not preserve preexisting context cancellation")
	}
	if harness.transport.closeCalls.Load() != 0 {
		t.Fatal("Close issued a request for an already canceled context")
	}
	cancelStart()
	receiveAdapterError(t, startResult)
}

func receiveAdapterUpdate(t *testing.T, updates <-chan Update) Update {
	t.Helper()
	select {
	case update := <-updates:
		return update
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for adapter update")
		return nil
	}
}

func receiveAdapterError(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for adapter call")
		return nil
	}
}

func receiveSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func receiveRequestContext(t *testing.T, contexts <-chan context.Context) context.Context {
	t.Helper()
	select {
	case ctx := <-contexts:
		return ctx
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for close request context")
		return nil
	}
}

func assertAdapterCallPending(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case <-result:
		t.Fatal("adapter call completed before closed authorization state")
	default:
	}
}

func assertAdapterCallBlocked(t *testing.T, result <-chan error) {
	t.Helper()
	timer := time.NewTimer(20 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-result:
		t.Fatal("adapter call completed before the blocked update was consumed")
	case <-timer.C:
	}
}

func assertAdapterError(t *testing.T, err error, operation string) {
	t.Helper()
	if err == nil {
		t.Fatal("adapter error = nil")
	}
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Kind != domain.ErrorInternal || appError.Op != operation {
		t.Fatal("adapter error does not match its domain contract")
	}
}
