//go:build tdlib

package telegram

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/buildinfo"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

const (
	testAPIHash     = "api-hash-private-value"
	testDatabaseKey = "database-key-private-value"
	testPhone       = "+15551234567"
	testCode        = "verification-code-private-value"
	testPassword    = "password-private-value"
)

type recordingPrompter struct {
	value   string
	err     error
	prompts []auth.Prompt
}

func (p *recordingPrompter) Ask(_ context.Context, prompt auth.Prompt) (string, error) {
	p.prompts = append(p.prompts, prompt)
	return p.value, p.err
}

type recordingAuthorizationClient struct {
	parameters *td.SetTdlibParametersRequest
	phone      *td.SetAuthenticationPhoneNumberRequest
	code       *td.CheckAuthenticationCodeRequest
	password   *td.CheckAuthenticationPasswordRequest
	err        error
}

func (c *recordingAuthorizationClient) SetTdlibParameters(_ context.Context, request *td.SetTdlibParametersRequest) (*td.Ok, error) {
	c.parameters = request
	return &td.Ok{}, c.err
}

func (c *recordingAuthorizationClient) SetAuthenticationPhoneNumber(_ context.Context, request *td.SetAuthenticationPhoneNumberRequest) (*td.Ok, error) {
	c.phone = request
	return &td.Ok{}, c.err
}

func (c *recordingAuthorizationClient) CheckAuthenticationCode(_ context.Context, request *td.CheckAuthenticationCodeRequest) (*td.Ok, error) {
	c.code = request
	return &td.Ok{}, c.err
}

func (c *recordingAuthorizationClient) CheckAuthenticationPassword(_ context.Context, request *td.CheckAuthenticationPasswordRequest) (*td.Ok, error) {
	c.password = request
	return &td.Ok{}, c.err
}

func TestInstalledTDLibMatchesWrapper(t *testing.T) {
	if err := preflightVersion(); err != nil {
		t.Fatal("installed TDLib does not match the pinned wrapper")
	}
}

func TestPreflightVersionAcceptsChecksummedPrebuiltVersion(t *testing.T) {
	var requests []string
	err := preflightVersionWith(func(request *td.GetOptionRequest) (td.OptionValue, error) {
		requests = append(requests, request.Name)
		switch request.Name {
		case "commit_hash":
			return &td.OptionValueString{Value: prebuiltTDLibCommit}, nil
		case "version":
			return &td.OptionValueString{Value: pinnedTDLibVersion}, nil
		default:
			t.Fatalf("unexpected option request %q", request.Name)
			return nil, nil
		}
	})
	if err != nil {
		t.Fatalf("preflight rejected checksummed prebuilt TDLib: %v", err)
	}
	if len(requests) != 2 || requests[0] != "commit_hash" || requests[1] != "version" {
		t.Fatalf("preflight requests = %v, want commit_hash then version", requests)
	}
}

func TestPreflightVersionRejectsMismatchedAndInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		value td.OptionValue
		kind  domain.ErrorKind
	}{
		{name: "mismatch", value: &td.OptionValueString{Value: "different-commit"}, kind: domain.ErrorVersion},
		{name: "unknown prebuilt", value: &td.OptionValueString{Value: prebuiltTDLibCommit}, kind: domain.ErrorVersion},
		{name: "wrong type", value: &td.OptionValueEmpty{}, kind: domain.ErrorVersion},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := preflightVersionWith(func(request *td.GetOptionRequest) (td.OptionValue, error) {
				if request.Name == "version" {
					return &td.OptionValueString{Value: "different-version"}, nil
				}
				if request.Name != "commit_hash" {
					t.Fatal("preflight requested the wrong TDLib option")
				}
				return test.value, nil
			})
			var appError domain.AppError
			if !errors.As(err, &appError) || appError.Kind != test.kind {
				t.Fatal("preflight error does not report a version mismatch")
			}
		})
	}
}

func TestPreflightVersionPreservesReadCause(t *testing.T) {
	cause := errors.New("TDLib option read failed")
	err := preflightVersionWith(func(*td.GetOptionRequest) (td.OptionValue, error) {
		return nil, cause
	})
	if !errors.Is(err, cause) {
		t.Fatal("preflight error does not preserve its cause")
	}
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Kind != domain.ErrorVersion || appError.Op != "read TDLib version" {
		t.Fatal("preflight read error does not match its domain contract")
	}
}

func TestAuthorizationHandlerBuildsPersistentParameters(t *testing.T) {
	runtimeConfig := testRuntime()
	handler := newAuthorizationHandler(context.Background(), runtimeConfig, &recordingPrompter{})
	client := &recordingAuthorizationClient{}

	if err := handler.handle(client, &td.AuthorizationStateWaitTdlibParameters{}); err != nil {
		t.Fatal("handle parameters returned an error")
	}
	request := client.parameters
	if request == nil {
		t.Fatal("SetTdlibParameters was not called")
	}
	if request.UseTestDc || !request.UseFileDatabase || !request.UseChatInfoDatabase || !request.UseMessageDatabase || request.UseSecretChats {
		t.Fatal("TDLib database flags do not match the persistent non-secret-chat configuration")
	}
	if request.DatabaseDirectory != runtimeConfig.Paths.TDLibDatabase || request.FilesDirectory != runtimeConfig.Paths.TDLibFiles {
		t.Fatal("TDLib directories do not match runtime paths")
	}
	if request.ApiId != runtimeConfig.APIID || request.ApiHash != runtimeConfig.APIHash {
		t.Fatal("TDLib API credentials do not match runtime credentials")
	}
	if string(request.DatabaseEncryptionKey) != string(runtimeConfig.DatabaseKey) {
		t.Fatal("TDLib database key does not match runtime key")
	}
	if request.SystemLanguageCode == "" || request.DeviceModel == "" || request.SystemVersion != runtime.GOOS {
		t.Fatal("TDLib system identity fields are incomplete")
	}
	if request.ApplicationVersion != buildinfo.Current().Version {
		t.Fatal("TDLib application version does not match build metadata")
	}

	runtimeConfig.DatabaseKey[0] = 'X'
	if string(request.DatabaseEncryptionKey) == string(runtimeConfig.DatabaseKey) {
		t.Fatal("TDLib parameters retain mutable runtime database-key storage")
	}
}

func TestAuthorizationHandlerSubmitsPhoneNumber(t *testing.T) {
	prompts := &recordingPrompter{value: testPhone}
	handler := newAuthorizationHandler(context.Background(), testRuntime(), prompts)
	client := &recordingAuthorizationClient{}

	if err := handler.handle(client, &td.AuthorizationStateWaitPhoneNumber{}); err != nil {
		t.Fatal("handle phone returned an error")
	}
	assertSinglePrompt(t, prompts, auth.Prompt{Kind: auth.PromptPhone, Label: "Phone number"})
	if client.phone == nil || client.phone.PhoneNumber != testPhone {
		t.Fatal("phone request does not contain the prompted phone number")
	}
}

func TestAuthorizationHandlerSubmitsVerificationCode(t *testing.T) {
	prompts := &recordingPrompter{value: testCode}
	handler := newAuthorizationHandler(context.Background(), testRuntime(), prompts)
	client := &recordingAuthorizationClient{}

	if err := handler.handle(client, &td.AuthorizationStateWaitCode{}); err != nil {
		t.Fatal("handle code returned an error")
	}
	assertSinglePrompt(t, prompts, auth.Prompt{Kind: auth.PromptCode, Label: "Verification code", Secret: true})
	if client.code == nil || client.code.Code != testCode {
		t.Fatal("code request does not contain the prompted verification code")
	}
}

func TestAuthorizationHandlerSubmitsPassword(t *testing.T) {
	prompts := &recordingPrompter{value: testPassword}
	handler := newAuthorizationHandler(context.Background(), testRuntime(), prompts)
	client := &recordingAuthorizationClient{}

	if err := handler.handle(client, &td.AuthorizationStateWaitPassword{}); err != nil {
		t.Fatal("handle password returned an error")
	}
	assertSinglePrompt(t, prompts, auth.Prompt{Kind: auth.PromptPassword, Label: "Telegram 2FA password", Secret: true})
	if client.password == nil || client.password.Password != testPassword {
		t.Fatal("password request does not contain the prompted password")
	}
}

func TestAuthorizationHandlerAcceptsTerminalStates(t *testing.T) {
	states := []td.AuthorizationState{
		&td.AuthorizationStateReady{},
		&td.AuthorizationStateClosing{},
		&td.AuthorizationStateClosed{},
	}
	for _, state := range states {
		prompts := &recordingPrompter{}
		handler := newAuthorizationHandler(context.Background(), testRuntime(), prompts)
		client := &recordingAuthorizationClient{}
		if err := handler.handle(client, state); err != nil {
			t.Fatal("terminal authorization state returned an error")
		}
		if len(prompts.prompts) != 0 || client.parameters != nil || client.phone != nil || client.code != nil || client.password != nil {
			t.Fatal("terminal authorization state caused an authorization side effect")
		}
	}
}

func TestAuthorizationHandlerRejectsUnsupportedStatesWithoutPayloads(t *testing.T) {
	const privatePayload = "unsupported-state-private-payload"
	states := []td.AuthorizationState{
		&td.AuthorizationStateWaitEmailAddress{},
		&td.AuthorizationStateWaitRegistration{TermsOfService: &td.TermsOfService{Text: &td.FormattedText{Text: privatePayload}}},
		&td.AuthorizationStateWaitPremiumPurchase{SupportEmailAddress: privatePayload},
		&td.AuthorizationStateWaitOtherDeviceConfirmation{Link: privatePayload},
	}
	for _, state := range states {
		handler := newAuthorizationHandler(context.Background(), testRuntime(), &recordingPrompter{})
		err := handler.handle(&recordingAuthorizationClient{}, state)
		assertAuthorizationError(t, err, "authorize")
		if strings.Contains(err.Error(), privatePayload) {
			t.Fatal("unsupported-state error exposes state payload")
		}
	}
}

func TestAuthorizationHandlerHidesPromptAndRequestErrors(t *testing.T) {
	privateCause := errors.New("private authorization payload in cause")
	tests := []struct {
		name    string
		handler *authorizationHandler
		client  *recordingAuthorizationClient
		state   td.AuthorizationState
	}{
		{
			name:    "prompt",
			handler: newAuthorizationHandler(context.Background(), testRuntime(), &recordingPrompter{err: privateCause}),
			client:  &recordingAuthorizationClient{},
			state:   &td.AuthorizationStateWaitPassword{},
		},
		{
			name:    "request",
			handler: newAuthorizationHandler(context.Background(), testRuntime(), &recordingPrompter{value: testCode}),
			client:  &recordingAuthorizationClient{err: privateCause},
			state:   &td.AuthorizationStateWaitCode{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.handler.handle(test.client, test.state)
			assertAuthorizationError(t, err, "authorize")
			if !errors.Is(err, privateCause) {
				t.Fatal("authorization error does not preserve its cause")
			}
			if strings.Contains(err.Error(), privateCause.Error()) {
				t.Fatal("authorization error exposes private cause text")
			}
		})
	}
}

func TestAuthorizationHandlerUsesItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	prompts := &contextPrompter{}
	handler := newAuthorizationHandler(ctx, testRuntime(), prompts)

	err := handler.handle(&recordingAuthorizationClient{}, &td.AuthorizationStateWaitPhoneNumber{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("authorization error does not preserve context cancellation")
	}
}

type contextPrompter struct{}

func (*contextPrompter) Ask(ctx context.Context, _ auth.Prompt) (string, error) {
	return "", ctx.Err()
}

func assertSinglePrompt(t *testing.T, prompts *recordingPrompter, want auth.Prompt) {
	t.Helper()
	if len(prompts.prompts) != 1 {
		t.Fatalf("prompt count = %d, want 1", len(prompts.prompts))
	}
	got := prompts.prompts[0]
	if got.Kind != want.Kind || got.Label != want.Label || got.Secret != want.Secret {
		t.Fatal("authorization prompt does not match its contract")
	}
}

func assertAuthorizationError(t *testing.T, err error, operation string) {
	t.Helper()
	if err == nil {
		t.Fatal("authorization error = nil")
	}
	var appError domain.AppError
	if !errors.As(err, &appError) {
		t.Fatal("authorization error is not a domain.AppError")
	}
	if appError.Kind != domain.ErrorAuthorization || appError.Op != operation {
		t.Fatal("authorization error kind or operation does not match")
	}
}

func testRuntime() config.Runtime {
	return config.Runtime{
		APIID:       12345,
		APIHash:     testAPIHash,
		DatabaseKey: []byte(testDatabaseKey),
		Paths: config.Paths{
			TDLibDatabase: "/private/tdlib-database",
			TDLibFiles:    "/private/tdlib-files",
		},
	}
}
