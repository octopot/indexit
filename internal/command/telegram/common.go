package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"go.octolab.org/toolset/indexit/internal/exitcode"
	indexlog "go.octolab.org/toolset/indexit/internal/log"
	tgsvc "go.octolab.org/toolset/indexit/internal/telegram"
	tgproxy "go.octolab.org/toolset/indexit/internal/telegram/proxy"
	tgsession "go.octolab.org/toolset/indexit/internal/telegram/session"
)

func pathsFromFlags(opt *options) (tgsession.Paths, error) {
	paths, err := tgsession.DefaultPaths()
	if err != nil {
		return tgsession.Paths{}, err
	}
	if opt.sessionPath != "" {
		paths.Session = opt.sessionPath
	}
	if opt.peersPath != "" {
		paths.Peers = opt.peersPath
	}
	return paths, nil
}

// openPeerCache loads the peer cache for a command that reads or fills it.
// Its entries reach the fetchers only through Bind inside the running client,
// which checks that the cache belongs to the account of the session.
func openPeerCache(log *slog.Logger, paths tgsession.Paths, client *tgsvc.Client) (*tgsvc.PeerCache, error) {
	cache, err := tgsvc.OpenPeerCache(paths.Peers, client.Session(), log)
	if err != nil {
		return nil, err
	}
	log.Info("cache: loaded", "path", paths.Peers)
	return cache, nil
}

// savePeerCache saves the peer cache when the run verified its owner and
// folds a save error into the error of the run.
func savePeerCache(log *slog.Logger, cache *tgsvc.PeerCache, err error) error {
	saved, saveErr := cache.Save()
	if err != nil {
		return err
	}
	if saveErr != nil {
		return saveErr
	}
	if saved {
		log.Info("cache: persisted", "peers", cache.Len(), "path", cache.Path())
	} else {
		log.Info("cache: not persisted, the account is not verified", "path", cache.Path())
	}
	return nil
}

func contextFromFlags(cmd *cobra.Command, opt *options) (context.Context, context.CancelFunc, error) {
	timeout, err := time.ParseDuration(opt.timeout)
	if err != nil {
		return nil, nil, fmt.Errorf("parse --timeout: %w", err)
	}
	if timeout <= 0 {
		return cmd.Context(), func() {}, nil
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	return ctx, cancel, nil
}

func newClient(cmd *cobra.Command, paths tgsession.Paths) (*tgsvc.Client, error) {
	return buildClient(cmd, paths, false)
}

// newQRClient builds a client with the update stream enabled, as required by the
// QR login flow (it awaits the updateLoginToken confirmation).
func newQRClient(cmd *cobra.Command, paths tgsession.Paths) (*tgsvc.Client, error) {
	return buildClient(cmd, paths, true)
}

func buildClient(cmd *cobra.Command, paths tgsession.Paths, withUpdates bool) (*tgsvc.Client, error) {
	if err := tgsession.EnsureSessionPath(paths.Session); err != nil {
		return nil, err
	}
	apiID, apiHash, err := tgsession.CredentialsFromEnv()
	if err != nil {
		return nil, err
	}
	descriptor, err := proxyFromEnv()
	if err != nil {
		return nil, err
	}
	settings := indexlog.FromContext(cmd.Context())
	opts := tgsvc.ClientOptions{
		Logger:      settings.Logger,
		GotdLog:     settings.GotdLog,
		Heartbeat:   settings.Heartbeat,
		WithUpdates: withUpdates,
	}
	if descriptor != nil {
		resolver, err := descriptor.Resolver()
		if err != nil {
			return nil, fmt.Errorf("build proxy resolver: %w", err)
		}
		opts.Resolver = resolver
	}
	logProxy(cmd, descriptor)
	return tgsvc.NewClient(apiID, apiHash, paths.Session, opts), nil
}

// proxyFromEnv reads the proxy settings; malformed ones are a usage error.
func proxyFromEnv() (*tgproxy.Descriptor, error) {
	descriptor, err := tgproxy.FromEnv()
	if err != nil {
		return nil, usageErr(err)
	}
	return descriptor, nil
}

// logProxy logs the proxy in use, if any. Every command logs it once: when
// it builds a client, or itself when it does not.
func logProxy(cmd *cobra.Command, descriptor *tgproxy.Descriptor) {
	if descriptor == nil {
		return
	}
	indexlog.FromContext(cmd.Context()).Logger.Info("proxy",
		"type", string(descriptor.Type),
		"host", descriptor.Host,
		"port", descriptor.Port)
}

func validateFormat(format string) error {
	if format != "jsonl" {
		return fmt.Errorf("unsupported --format %q: only jsonl is supported", format)
	}
	return nil
}

func parseRFC3339(value, name string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s as RFC3339: %w", name, err)
	}
	return t, nil
}

func usageErr(err error) error {
	if err == nil {
		return nil
	}
	return exitcode.New(exitcode.Usage, err)
}
