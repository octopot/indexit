package telegram

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	gotdsession "github.com/gotd/td/session"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// CodeResender is an optional interface a UserAuthenticator may implement to
// receive the live auth client. With it, the authenticator can resend the login
// code (e.g. to switch the delivery channel from the Telegram app to SMS).
type CodeResender interface {
	AttachClient(*auth.Client)
}

func Login(ctx context.Context, runner Runner, authenticator auth.UserAuthenticator) error {
	return runner.Run(ctx, func(ctx context.Context, api API, client *auth.Client) error {
		if resender, ok := authenticator.(CodeResender); ok {
			resender.AttachClient(client)
		}
		if err := client.IfNecessary(ctx, auth.NewFlow(authenticator, auth.SendCodeOptions{})); err != nil {
			return err
		}
		if status, err := client.Status(ctx); err == nil && status.Authorized && status.User != nil {
			slog.Default().Info("authorized",
				"user_id", status.User.ID,
				"username", status.User.Username,
				"name", displayUser(status.User))
		}
		return nil
	})
}

func Logout(ctx context.Context, runner Runner, sessionPath string) error {
	err := runner.Run(ctx, func(ctx context.Context, api API, client *auth.Client) error {
		_, err := api.AuthLogOut(ctx)
		return err
	})
	if removeErr := os.Remove(sessionPath); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
		err = removeErr
	}
	return err
}

type Status struct {
	SessionPath string
	Authorized  bool
	DC          int
	User        *tg.User
}

func GetStatus(ctx context.Context, runner Runner, sessionPath string) (Status, error) {
	status := Status{SessionPath: sessionPath}
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		return status, nil
	} else if err != nil {
		return status, err
	}
	if data, err := (&gotdsession.Loader{
		Storage: &gotdsession.FileStorage{Path: sessionPath},
	}).Load(ctx); err == nil {
		status.DC = data.DC
	}
	err := runner.Run(ctx, func(ctx context.Context, api API, client *auth.Client) error {
		authStatus, err := client.Status(ctx)
		if err != nil {
			return err
		}
		status.Authorized = authStatus.Authorized
		status.User = authStatus.User
		return nil
	})
	return status, err
}

func PrintStatus(w io.Writer, status Status) error {
	if _, err := fmt.Fprintf(w, "session: %s\n", status.SessionPath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "authorized: %t\n", status.Authorized); err != nil {
		return err
	}
	if status.DC != 0 {
		if _, err := fmt.Fprintf(w, "dc: %d\n", status.DC); err != nil {
			return err
		}
	}
	if status.User != nil {
		_, err := fmt.Fprintf(w, "user: %d %s\n", status.User.ID, displayUser(status.User))
		return err
	}
	return nil
}

func displayUser(user *tg.User) string {
	if user == nil {
		return ""
	}
	name := user.FirstName
	if user.LastName != "" {
		name += " " + user.LastName
	}
	if user.Username != "" {
		name += " @" + user.Username
	}
	return name
}
