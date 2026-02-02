package telegram

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"go.octolab.org/toolset/indexit/internal/buildinfo"
	"go.octolab.org/toolset/indexit/internal/config"
	"go.octolab.org/toolset/indexit/internal/dashboard"
	indexlog "go.octolab.org/toolset/indexit/internal/log"
	"go.octolab.org/toolset/indexit/internal/preview"
	"go.octolab.org/toolset/indexit/internal/stats"
	tgsvc "go.octolab.org/toolset/indexit/internal/telegram"
)

// Port is the port of the image service; AGENTS.md records the ports of the
// local services.
const Port = 17840

// Listen holds the default addresses: indexit.localhost resolves to both.
var Listen = []string{
	net.JoinHostPort("127.0.0.1", strconv.Itoa(Port)),
	net.JoinHostPort("::1", strconv.Itoa(Port)),
}

func NewService() *cobra.Command {
	var opt options
	var listen []string
	root := &cobra.Command{Use: "service", Short: "Manage the local photograph service", Args: cobra.NoArgs}
	run := &cobra.Command{Use: "run", Short: "Serve saved and on-demand Telegram photographs on loopback", Long: "Serves saved and on-demand Telegram photographs over HTTP on loopback.\n" +
		"It runs as your user and listens on 127.0.0.1 and ::1 at port " + strconv.Itoa(Port) + "; a default\n" +
		"address the system lacks, as ::1 with IPv6 turned off, is skipped.\n" +
		"--listen replaces the defaults and may repeat. Photograph URLs use origin\n" +
		"under [service] in config.toml, " + preview.DefaultOrigin + " by default.\n" +
		"Without a config.toml, it writes one with every key commented out.\n\n" +
		"A photograph is saved the first time its URL is requested and served from\n" +
		"disk from then on. images under [service] moves the photographs apart from\n" +
		"the rest of the data, as to a folder another computer shares.\n\n" +
		"On this machine, / is a dashboard of the saved photographs. dashboard = false\n" +
		"under [service] turns it off. Request counts go to stats/ in the data directory.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		for _, address := range listen {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return usageErr(fmt.Errorf("--listen: %w", err))
			}
			if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
				return usageErr(fmt.Errorf("--listen must use a literal loopback IP address, not %q", host))
			}
			// 0 asks the system for a free port, as tests do.
			if _, err := strconv.ParseUint(port, 10, 16); err != nil {
				return usageErr(fmt.Errorf("--listen must end with a port from 0 to 65535, not %q", port))
			}
		}
		log := indexlog.FromContext(cmd.Context()).Logger
		configPath, err := config.FilePath()
		if err != nil {
			return usageErr(err)
		}
		// The service writes the default config, so there is a file to edit;
		// without one it still starts on the defaults.
		if created, err := config.EnsureFile(configPath); err != nil {
			log.Warn("config: cannot create the default file", "path", configPath, "error", err)
		} else if created {
			log.Info("config: created the default file", "path", configPath)
		}
		file, err := config.ReadFile(configPath)
		if err != nil {
			return usageErr(err)
		}
		store, err := preview.FromConfig(file)
		if err != nil {
			return usageErr(err)
		}
		paths, err := pathsFromFlags(&opt)
		if err != nil {
			return err
		}
		statsDir := filepath.Join(store.Root, "stats")
		recorder := stats.NewRecorder(statsDir, log)
		var board *dashboard.Dashboard
		if file.Service.DashboardEnabled() {
			board = dashboard.New(store, paths.Peers, stats.NewReader(statsDir))
			board.Dropped = recorder.Dropped
			board.Version = buildinfo.Display()
			board.ConfigPath = configPath
			board.Logger = log
		}
		previews := &tgsvc.Previews{Peers: paths.Peers, Log: log}
		handler := store.Handler(func(ctx context.Context, src preview.Source, path string) error {
			client, err := newClient(cmd, paths)
			if err != nil {
				log.Error("preview: client unavailable", "error", err)
				return err
			}
			err = client.RunMedia(ctx, func(ctx context.Context, api tgsvc.MediaAPI) error {
				return previews.Download(ctx, api, client.Session(), src, path, tgsvc.RateGuard{})
			})
			if err != nil {
				log.Error("preview: download failed", "channel", src.Channel, "message", src.Message, "error", err)
			}
			return err
		}, func(outcome preview.Outcome) {
			recorder.Observe(outcome)
			if board != nil && outcome.Kind == preview.Download {
				board.Refresh()
			}
		})
		listeners, err := listenAll(cmd.Context(), listen, !cmd.Flags().Changed("listen"), func(address string, err error) {
			log.Warn("service: address unavailable, skipped", "address", address, "error", err)
		})
		if err != nil {
			return err
		}
		addresses := make([]string, 0, len(listeners))
		for _, listener := range listeners {
			addresses = append(addresses, listener.Addr().String())
		}
		// The statistics outlive the requests in flight: they stop after serve.
		background, stop := context.WithCancel(context.WithoutCancel(cmd.Context()))
		var workers sync.WaitGroup
		workers.Go(func() { recorder.Run(background) })
		served := handler
		if board != nil {
			served = board.Handler(handler)
			workers.Go(func() { board.Run(background) })
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "image service: %s (URLs: %s, data: %s, photographs: %s)\n", strings.Join(addresses, ", "), store.Origin, store.Root, store.ImageDir())
		err = serve(cmd.Context(), listeners, served, 5*time.Second)
		stop()
		workers.Wait()
		return err
	}}
	run.Flags().StringArrayVar(&listen, "listen", Listen, "loopback address to listen on, repeatable; replaces the defaults")
	run.Flags().StringVar(&opt.sessionPath, "session", "", "path to Telegram session file")
	root.AddCommand(run)
	return root
}

// listenAll binds every address. With optional set, an address that the
// system does not have, such as [::1] with IPv6 turned off, is passed to skip
// and left out; any other failure, or no address bound at all, is an error.
func listenAll(ctx context.Context, addresses []string, optional bool, skip func(address string, err error)) ([]net.Listener, error) {
	var listeners []net.Listener
	closeAll := func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}
	for _, address := range addresses {
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
		if err != nil {
			if optional && unavailable(err) {
				skip(address, err)
				continue
			}
			closeAll()
			return nil, err
		}
		listeners = append(listeners, listener)
	}
	if len(listeners) == 0 {
		return nil, fmt.Errorf("no address to listen on: %v", addresses)
	}
	return listeners, nil
}

// unavailable reports whether err means the system has no such address or
// address family, rather than, say, a port already in use.
func unavailable(err error) bool {
	return errors.Is(err, syscall.EADDRNOTAVAIL) || errors.Is(err, syscall.EAFNOSUPPORT) ||
		errors.Is(err, syscall.EPROTONOSUPPORT)
}

// serve answers on every listener with handler until ctx ends, then shuts
// down gracefully: it stops accepting connections and waits up to grace for
// the responses in flight. A failure of a listener ends it with that error.
func serve(ctx context.Context, listeners []net.Listener, handler http.Handler, grace time.Duration) error {
	server := &http.Server{BaseContext: func(net.Listener) context.Context { return ctx }, Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, WriteTimeout: 3 * time.Minute}
	served := make(chan error, len(listeners))
	for _, listener := range listeners {
		go func() { served <- server.Serve(listener) }()
	}
	var err error
	pending := len(listeners)
	select {
	case err = <-served:
		pending--
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if e := server.Shutdown(shutdown); e != nil {
		_ = server.Close()
		err = errors.Join(err, fmt.Errorf("shutdown: %w", e))
	}
	for ; pending > 0; pending-- {
		if e := <-served; !errors.Is(e, http.ErrServerClosed) {
			err = errors.Join(err, e)
		}
	}
	return err
}
