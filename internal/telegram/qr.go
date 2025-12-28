package telegram

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// ShowQR renders a QR login token so the user can confirm it from an
// already-authorized official Telegram client. It may be called more than once
// because the token is re-exported when it expires.
type ShowQR func(ctx context.Context, token qrlogin.Token) error

// PasswordPrompt asks the user for the cloud (2FA) password. It is only invoked
// when the account has two-step verification enabled.
type PasswordPrompt func(ctx context.Context) (string, error)

// LoginQR authorizes the session via Telegram's QR login flow, bypassing the
// phone-code delivery entirely: indexit displays a login token (via show) that
// the user accepts from a logged-in device. If the account has a cloud password,
// QR acceptance yields SESSION_PASSWORD_NEEDED and password completes the 2FA
// step. The client must be built with ClientOptions.WithUpdates, since the
// confirmation arrives as updateLoginToken.
func LoginQR(ctx context.Context, client *Client, show ShowQR, password PasswordPrompt) error {
	if client.dispatcher == nil {
		return fmt.Errorf("qr login requires a client built with updates enabled")
	}
	return client.run(ctx, func(ctx context.Context) error {
		authClient := client.client.Auth()
		if status, err := authClient.Status(ctx); err == nil && status.Authorized {
			logAuthorized(status.User)
			return nil
		}

		authorization, err := client.client.QR().Auth(
			ctx,
			qrlogin.OnLoginToken(*client.dispatcher),
			func(ctx context.Context, token qrlogin.Token) error {
				return show(ctx, token)
			},
		)
		if err != nil {
			if !tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
				return fmt.Errorf("qr login: %w", err)
			}
			authorization, err = completeWithPassword(ctx, authClient, password)
			if err != nil {
				return err
			}
		}
		if user, ok := authorization.User.(*tg.User); ok {
			logAuthorized(user)
		}
		return nil
	})
}

// completeWithPassword finishes a QR login that stopped at the 2FA step.
func completeWithPassword(
	ctx context.Context,
	authClient *auth.Client,
	password PasswordPrompt,
) (*tg.AuthAuthorization, error) {
	pwd, err := password(ctx)
	if err != nil {
		return nil, fmt.Errorf("qr login: read 2fa password: %w", err)
	}
	authorization, err := authClient.Password(ctx, pwd)
	if err != nil {
		return nil, fmt.Errorf("qr login: 2fa: %w", err)
	}
	return authorization, nil
}

func logAuthorized(user *tg.User) {
	if user == nil {
		return
	}
	slog.Default().Info("authorized",
		"user_id", user.ID,
		"username", user.Username,
		"name", displayUser(user))
}
