//go:build linux

package client

import (
	"errors"
	"sync"
	"testing"
	"time"
)

var errStopAuthorization = errors.New("stop authorization before setting TDLib parameters")

type earlyCloseAuthorizer struct {
	waitCalled chan *Client
	release    chan struct{}
	once       sync.Once
}

func (a *earlyCloseAuthorizer) Handle(client *Client, state AuthorizationState) error {
	if state.AuthorizationStateConstructor() == ConstructorAuthorizationStateWaitTdlibParameters {
		a.once.Do(func() {
			a.waitCalled <- client
			<-a.release
		})
		return errStopAuthorization
	}
	return nil
}

func (*earlyCloseAuthorizer) Close() {}

type newClientResult struct {
	client *Client
	err    error
}

func TestNewClientInstallsOptionsBeforeAuthorizationAndClosesWithoutRace(t *testing.T) {
	authorizer := &earlyCloseAuthorizer{
		waitCalled: make(chan *Client, 1),
		release:    make(chan struct{}),
	}
	optionStarted := make(chan struct{})
	releaseOption := make(chan struct{})
	secondOptionStarted := make(chan struct{})
	closedSeen := make(chan struct{})
	var closedOnce sync.Once
	result := make(chan newClientResult, 1)

	go func() {
		client, err := NewClient(
			authorizer,
			func(*Client) {
				close(optionStarted)
				<-releaseOption
			},
			func(*Client) { close(secondOptionStarted) },
			WithResultHandler(NewCallbackResultHandler(func(result Type) {
				update, ok := result.(*UpdateAuthorizationState)
				if ok && update.AuthorizationState.AuthorizationStateConstructor() == ConstructorAuthorizationStateClosed {
					closedOnce.Do(func() { close(closedSeen) })
				}
			})),
			WithFallbackTimeout(100*time.Millisecond),
		)
		result <- newClientResult{client: client, err: err}
	}()

	receiveForkSignal(t, optionStarted, "option start")
	optionsRanConcurrently := false
	select {
	case <-secondOptionStarted:
		optionsRanConcurrently = true
	case <-time.After(250 * time.Millisecond):
	}
	close(releaseOption)
	client := receiveForkClient(t, authorizer.waitCalled)
	readerStarted := make(chan struct{})
	stopReader := make(chan struct{})
	readerDone := make(chan struct{})
	go observeClosedState(client, readerStarted, stopReader, readerDone)
	receiveForkSignal(t, readerStarted, "closed-state reader")
	close(authorizer.release)

	creation := receiveNewClientResult(t, result)
	receiveForkSignal(t, closedSeen, "authorizationStateClosed callback")
	close(stopReader)
	receiveForkSignal(t, readerDone, "closed-state reader shutdown")
	if optionsRanConcurrently {
		t.Fatal("client options ran concurrently instead of completing in declaration order")
	}
	if creation.client != nil {
		t.Fatal("NewClient returned a client after the authorizer rejected TDLib parameters")
	}
	if creation.err == nil {
		t.Fatal("NewClient returned no error after the authorizer rejected TDLib parameters")
	}
	if !errors.Is(creation.err, errStopAuthorization) {
		t.Fatal("NewClient did not preserve the authorizer sentinel")
	}
	if creation.err != errStopAuthorization {
		t.Fatal("NewClient wrapped the authorizer sentinel despite a successful close request")
	}
}

func observeClosedState(client *Client, started chan<- struct{}, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	close(started)
	for {
		select {
		case <-stop:
			return
		default:
			_ = client.isClosed.Load()
		}
	}
}

func receiveForkSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func receiveNewClientResult(t *testing.T, result <-chan newClientResult) newClientResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for NewClient")
		return newClientResult{}
	}
}

func receiveForkClient(t *testing.T, clients <-chan *Client) *Client {
	t.Helper()
	select {
	case client := <-clients:
		return client
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the authorization handler")
		return nil
	}
}
