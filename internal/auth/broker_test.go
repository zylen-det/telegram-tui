package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewBrokerUsesUnbufferedChannels(t *testing.T) {
	broker := NewBroker()

	if capacity := cap(broker.Prompts()); capacity != 0 {
		t.Fatalf("prompt channel capacity = %d, want 0", capacity)
	}
}

func TestBrokerAskAndSubmitRendezvous(t *testing.T) {
	broker := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan askResult, 1)
	go func() {
		value, err := broker.Ask(ctx, Prompt{
			Kind:   PromptPhone,
			Label:  "Phone number",
			Secret: false,
		})
		result <- askResult{value: value, err: err}
	}()

	prompt := receivePrompt(t, broker.Prompts())
	if prompt.ID == 0 {
		t.Fatal("prompt ID = 0, want nonzero")
	}
	if prompt.Kind != PromptPhone || prompt.Label != "Phone number" || prompt.Secret {
		t.Fatalf("prompt = %#v, want phone prompt", prompt)
	}

	if err := broker.Submit(ctx, Response{PromptID: prompt.ID, Value: "+15551234567"}); err != nil {
		t.Fatalf("Submit() error = %v, want nil", err)
	}

	got := receiveAskResult(t, result)
	if got.err != nil {
		t.Fatalf("Ask() error = %v, want nil", got.err)
	}
	if got.value != "+15551234567" {
		t.Fatalf("Ask() value = %q, want %q", got.value, "+15551234567")
	}
}

func TestBrokerAskAssignsIncreasingIDs(t *testing.T) {
	broker := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	first, firstResult := askAndReceivePrompt(t, ctx, broker)
	if err := broker.Submit(ctx, Response{PromptID: first.ID, Value: "first"}); err != nil {
		t.Fatalf("first Submit() error = %v, want nil", err)
	}
	if got := receiveAskResult(t, firstResult); got.err != nil || got.value != "first" {
		t.Fatalf("first Ask() = (%q, %v), want (%q, nil)", got.value, got.err, "first")
	}

	second, secondResult := askAndReceivePrompt(t, ctx, broker)
	if second.ID <= first.ID {
		t.Fatalf("second prompt ID = %d, want greater than %d", second.ID, first.ID)
	}
	if err := broker.Submit(ctx, Response{PromptID: second.ID, Value: "second"}); err != nil {
		t.Fatalf("second Submit() error = %v, want nil", err)
	}
	if got := receiveAskResult(t, secondResult); got.err != nil || got.value != "second" {
		t.Fatalf("second Ask() = (%q, %v), want (%q, nil)", got.value, got.err, "second")
	}
}

func TestBrokerSubmitDoesNotReturnBeforeNextAskCanStart(t *testing.T) {
	broker := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, firstResult := askAndReceivePrompt(t, ctx, broker)
	submitResult := make(chan error, 1)
	go func() { submitResult <- broker.Submit(ctx, Response{PromptID: first.ID, Value: "first"}) }()
	if got := receiveAskResult(t, firstResult); got.err != nil || got.value != "first" {
		t.Fatalf("first Ask() = (%q, %v)", got.value, got.err)
	}
	if err := receiveError(t, submitResult); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	secondResult := make(chan askResult, 1)
	go func() {
		value, err := broker.Ask(ctx, Prompt{Kind: PromptPhone, Label: "Phone"})
		secondResult <- askResult{value: value, err: err}
	}()
	second := receivePrompt(t, broker.Prompts())
	if second.Kind != PromptPhone {
		t.Fatalf("second prompt = %#v", second)
	}
	if err := broker.Submit(ctx, Response{PromptID: second.ID, Value: "phone"}); err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}
	if got := receiveAskResult(t, secondResult); got.err != nil || got.value != "phone" {
		t.Fatalf("second Ask() = (%q, %v)", got.value, got.err)
	}
}

func TestBrokerAskRejectsMismatchedResponseID(t *testing.T) {
	broker := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan askResult, 1)
	go func() {
		value, err := broker.Ask(ctx, Prompt{Kind: PromptCode, Label: "Code"})
		result <- askResult{value: value, err: err}
	}()

	prompt := receivePrompt(t, broker.Prompts())
	if err := broker.Submit(ctx, Response{PromptID: prompt.ID + 1, Value: "wrong"}); err == nil || err.Error() != "authorization response ID mismatch" {
		t.Fatalf("mismatched Submit() error = %v, want authorization response ID mismatch", err)
	}
	assertAskPending(t, result)

	if err := broker.Submit(ctx, Response{PromptID: prompt.ID, Value: "12345"}); err != nil {
		t.Fatalf("matching Submit() error = %v, want nil", err)
	}
	got := receiveAskResult(t, result)
	if got.err != nil || got.value != "12345" {
		t.Fatalf("Ask() = (%q, %v), want (%q, nil)", got.value, got.err, "12345")
	}
}

func TestBrokerRejectsStaleResponseWithoutAffectingNextAsk(t *testing.T) {
	broker := NewBroker()
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstResult := make(chan askResult, 1)

	go func() {
		value, err := broker.Ask(firstCtx, Prompt{Kind: PromptPhone, Label: "Phone"})
		firstResult <- askResult{value: value, err: err}
	}()
	firstPrompt := receivePrompt(t, broker.Prompts())
	cancelFirst()
	first := receiveAskResult(t, firstResult)
	if !errors.Is(first.err, context.Canceled) {
		t.Fatalf("first Ask() error = %v, want %v", first.err, context.Canceled)
	}

	staleResult := make(chan error, 1)
	go func() {
		staleResult <- broker.Submit(context.Background(), Response{
			PromptID: firstPrompt.ID,
			Value:    "stale",
		})
	}()

	secondCtx, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()
	secondResult := make(chan askResult, 1)
	go func() {
		value, err := broker.Ask(secondCtx, Prompt{Kind: PromptCode, Label: "Code"})
		secondResult <- askResult{value: value, err: err}
	}()
	secondPrompt := receivePrompt(t, broker.Prompts())

	if err := receiveError(t, staleResult); err == nil {
		t.Fatal("stale Submit() error = nil, want an explicit rejection")
	}
	assertAskPending(t, secondResult)

	if err := broker.Submit(secondCtx, Response{PromptID: secondPrompt.ID, Value: "67890"}); err != nil {
		t.Fatalf("matching Submit() error = %v, want nil", err)
	}
	second := receiveAskResult(t, secondResult)
	if second.err != nil || second.value != "67890" {
		t.Fatalf("second Ask() = (%q, %v), want (%q, nil)", second.value, second.err, "67890")
	}
}

func TestBrokerAskReturnsCanceledWhenPromptIsUnobserved(t *testing.T) {
	broker := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	go func() {
		_, err := broker.Ask(ctx, Prompt{Kind: PromptAPIID, Label: "API ID"})
		result <- err
	}()
	cancel()

	if err := receiveError(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ask() error = %v, want %v", err, context.Canceled)
	}
}

func TestBrokerAskCancellationAfterPromptIsReceived(t *testing.T) {
	broker := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	go func() {
		_, err := broker.Ask(ctx, Prompt{Kind: PromptPassword, Label: "Password", Secret: true})
		result <- err
	}()

	receivePrompt(t, broker.Prompts())
	cancel()

	if err := receiveError(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ask() error = %v, want %v", err, context.Canceled)
	}
}

func TestBrokerSubmitRejectsResponseWithoutActivePrompt(t *testing.T) {
	broker := NewBroker()

	if err := broker.Submit(context.Background(), Response{PromptID: 1, Value: "unused"}); err == nil || err.Error() != "no active authorization prompt" {
		t.Fatalf("Submit() error = %v, want no active authorization prompt", err)
	}
}

func TestBrokerRejectsConcurrentAsk(t *testing.T) {
	broker := NewBroker()
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, err := broker.Ask(firstCtx, Prompt{Kind: PromptPhone})
		firstResult <- err
	}()
	receivePrompt(t, broker.Prompts())

	secondCtx, cancelSecond := context.WithCancel(context.Background())
	cancelSecond()
	if _, err := broker.Ask(secondCtx, Prompt{Kind: PromptCode}); err == nil || err.Error() != "authorization prompt already active" {
		t.Fatalf("concurrent Ask() error = %v, want authorization prompt already active", err)
	}

	cancelFirst()
	if err := receiveError(t, firstResult); !errors.Is(err, context.Canceled) {
		t.Fatalf("first Ask() error = %v, want %v", err, context.Canceled)
	}
}

type askResult struct {
	value string
	err   error
}

func askAndReceivePrompt(t *testing.T, ctx context.Context, broker *Broker) (Prompt, <-chan askResult) {
	t.Helper()
	result := make(chan askResult, 1)
	go func() {
		value, err := broker.Ask(ctx, Prompt{Kind: PromptPhone})
		result <- askResult{value: value, err: err}
	}()
	return receivePrompt(t, broker.Prompts()), result
}

func receivePrompt(t *testing.T, prompts <-chan Prompt) Prompt {
	t.Helper()
	select {
	case prompt := <-prompts:
		return prompt
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prompt")
		return Prompt{}
	}
}

func receiveAskResult(t *testing.T, results <-chan askResult) askResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Ask result")
		return askResult{}
	}
}

func receiveError(t *testing.T, results <-chan error) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error")
		return nil
	}
}

func assertAskPending(t *testing.T, results <-chan askResult) {
	t.Helper()
	select {
	case result := <-results:
		t.Fatalf("Ask() returned early with value %q and error %v", result.value, result.err)
	default:
	}
}
