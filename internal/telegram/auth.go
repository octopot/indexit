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

	"go.octolab.org/toolset/indexit/internal/telegram/peers"
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
	remove := func(err error) error {
		if removeErr := os.Remove(sessionPath); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
			return removeErr
		}
		return err
	}
	if client, ok := runner.(*Client); ok {
		return client.runAfter(ctx, func(ctx context.Context) error { _, err := client.client.API().AuthLogOut(ctx); return err }, remove)
	}
	err := runner.Run(ctx, func(ctx context.Context, api API, client *auth.Client) error {
		_, err := api.AuthLogOut(ctx)
		return err
	})
	return remove(err)
}

type Status struct {
	SessionPath string
	Authorized  bool
	DC          int
	User        *tg.User
	// PeerCachePath is the peer cache of the session; PeerCacheOwner, the
	// account recorded in it, or nil when it records none; PeerCacheError,
	// why the cache could not be read.
	PeerCachePath  string
	PeerCacheOwner *peers.Account
	PeerCacheError error
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
		if _, err := fmt.Fprintf(w, "user: %d %s\n", status.User.ID, displayUser(status.User)); err != nil {
			return err
		}
	}
	if status.PeerCachePath == "" {
		return nil
	}
	if _, err := fmt.Fprintf(w, "peer cache: %s\n", status.PeerCachePath); err != nil {
		return err
	}
	owner := "none"
	switch {
	case status.PeerCacheError != nil:
		owner = fmt.Sprintf("unreadable (%v)", status.PeerCacheError)
	case status.PeerCacheOwner != nil:
		owner = fmt.Sprintf("%d", status.PeerCacheOwner.ID)
		if status.User != nil && status.User.ID != status.PeerCacheOwner.ID {
			owner += " (another account: the next fetch starts it empty)"
		}
	}
	_, err := fmt.Fprintf(w, "peer cache account: %s\n", owner)
	return err
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
