//go:build tdlib

package telegram

import (
	"context"
	"runtime"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/buildinfo"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

type authorizationClient interface {
	SetTdlibParameters(context.Context, *td.SetTdlibParametersRequest) (*td.Ok, error)
	SetAuthenticationPhoneNumber(context.Context, *td.SetAuthenticationPhoneNumberRequest) (*td.Ok, error)
	CheckAuthenticationCode(context.Context, *td.CheckAuthenticationCodeRequest) (*td.Ok, error)
	CheckAuthenticationPassword(context.Context, *td.CheckAuthenticationPasswordRequest) (*td.Ok, error)
}

type authorizationHandler struct {
	ctx        context.Context
	parameters *td.SetTdlibParametersRequest
	prompts    auth.Prompter
}

func newAuthorizationHandler(ctx context.Context, runtimeConfig config.Runtime, prompts auth.Prompter) *authorizationHandler {
	info := buildinfo.Current()
	return &authorizationHandler{
		ctx: ctx,
		parameters: &td.SetTdlibParametersRequest{
			DatabaseDirectory:     runtimeConfig.Paths.TDLibDatabase,
			FilesDirectory:        runtimeConfig.Paths.TDLibFiles,
			DatabaseEncryptionKey: append([]byte(nil), runtimeConfig.DatabaseKey...),
			UseFileDatabase:       true,
			UseChatInfoDatabase:   true,
			UseMessageDatabase:    true,
			UseSecretChats:        false,
			ApiId:                 runtimeConfig.APIID,
			ApiHash:               runtimeConfig.APIHash,
			SystemLanguageCode:    "en",
			DeviceModel:           "Kitty terminal",
			SystemVersion:         runtime.GOOS,
			ApplicationVersion:    info.Version,
		},
		prompts: prompts,
	}
}

func (handler *authorizationHandler) Handle(client *td.Client, state td.AuthorizationState) error {
	return handler.handle(client, state)
}

func (handler *authorizationHandler) handle(client authorizationClient, state td.AuthorizationState) error {
	switch state.AuthorizationStateConstructor() {
	case td.ConstructorAuthorizationStateWaitTdlibParameters:
		_, err := client.SetTdlibParameters(handler.ctx, handler.parameters)
		return safeAuthorizationError("set TDLib parameters", err)
	case td.ConstructorAuthorizationStateWaitPhoneNumber:
		value, err := handler.ask(auth.Prompt{Kind: auth.PromptPhone, Label: "Phone number"})
		if err != nil {
			return err
		}
		_, err = client.SetAuthenticationPhoneNumber(handler.ctx, &td.SetAuthenticationPhoneNumberRequest{PhoneNumber: value})
		return safeAuthorizationError("submit phone number", err)
	case td.ConstructorAuthorizationStateWaitCode:
		value, err := handler.ask(auth.Prompt{Kind: auth.PromptCode, Label: "Verification code", Secret: true})
		if err != nil {
			return err
		}
		_, err = client.CheckAuthenticationCode(handler.ctx, &td.CheckAuthenticationCodeRequest{Code: value})
		return safeAuthorizationError("submit verification code", err)
	case td.ConstructorAuthorizationStateWaitPassword:
		value, err := handler.ask(auth.Prompt{Kind: auth.PromptPassword, Label: "Telegram 2FA password", Secret: true})
		if err != nil {
			return err
		}
		_, err = client.CheckAuthenticationPassword(handler.ctx, &td.CheckAuthenticationPasswordRequest{Password: value})
		return safeAuthorizationError("submit 2FA password", err)
	case td.ConstructorAuthorizationStateReady,
		td.ConstructorAuthorizationStateClosing,
		td.ConstructorAuthorizationStateClosed:
		return nil
	default:
		return domain.AppError{
			Kind:    domain.ErrorAuthorization,
			Op:      "authorize",
			Message: "unsupported TDLib authorization state: " + state.AuthorizationStateConstructor(),
		}
	}
}

func (handler *authorizationHandler) ask(prompt auth.Prompt) (string, error) {
	if handler.prompts == nil {
		return "", domain.AppError{
			Kind:    domain.ErrorAuthorization,
			Op:      "authorize",
			Message: "authorization prompt service is unavailable",
		}
	}
	value, err := handler.prompts.Ask(handler.ctx, prompt)
	if err != nil {
		return "", domain.AppError{
			Kind:    domain.ErrorAuthorization,
			Op:      "authorize",
			Message: "authorization prompt was canceled or failed",
			Cause:   err,
		}
	}
	return value, nil
}

func safeAuthorizationError(operation string, cause error) error {
	if cause == nil {
		return nil
	}
	return domain.AppError{
		Kind:    domain.ErrorAuthorization,
		Op:      "authorize",
		Message: "could not " + operation,
		Cause:   cause,
	}
}

func (*authorizationHandler) Close() {}

var _ td.AuthorizationStateHandler = (*authorizationHandler)(nil)
