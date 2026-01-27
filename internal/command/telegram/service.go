package telegram

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	indexlog "go.octolab.org/toolset/indexit/internal/log"
	"go.octolab.org/toolset/indexit/internal/preview"
	tgsvc "go.octolab.org/toolset/indexit/internal/telegram"
)

func NewService() *cobra.Command {
	var opt options
	var listen string
	root := &cobra.Command{Use: "service", Short: "Manage the local photograph service", Args: cobra.NoArgs}
	run := &cobra.Command{Use: "run", Short: "Serve saved and on-demand Telegram photographs on loopback", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		host, _, err := net.SplitHostPort(listen)
		if err != nil {
			return usageErr(err)
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return usageErr(errors.New("--listen must use a literal loopback IP address"))
		}
		store, err := preview.Default()
		if err != nil {
			return err
		}
		paths, err := pathsFromFlags(&opt)
		if err != nil {
			return err
		}
		log := indexlog.FromContext(cmd.Context()).Logger
		handler := store.Handler(func(ctx context.Context, src preview.Source, path string) error {
			client, err := newClient(cmd, paths)
			if err != nil {
				log.Error("preview: client unavailable", "error", err)
				return err
			}
			err = client.RunMedia(ctx, func(ctx context.Context, api tgsvc.MediaAPI) error {
				return tgsvc.DownloadPreview(ctx, api, src, path, tgsvc.RateGuard{})
			})
			if err != nil {
				log.Error("preview: download failed", "message", src.Message, "error", err)
			}
			return err
		})
		listener, err := (&net.ListenConfig{}).Listen(cmd.Context(), "tcp", listen)
		if err != nil {
			return err
		}
		server := &http.Server{BaseContext: func(net.Listener) context.Context { return cmd.Context() }, Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, WriteTimeout: 3 * time.Minute}
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-cmd.Context().Done():
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = server.Shutdown(ctx)
			case <-done:
			}
		}()
		fmt.Fprintf(cmd.ErrOrStderr(), "image service: %s (URLs: %s, data: %s)\n", listener.Addr(), preview.Origin, store.Root)
		err = server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}}
	run.Flags().StringVar(&listen, "listen", "127.0.0.1:17840", "loopback address to listen on")
	run.Flags().StringVar(&opt.sessionPath, "session", "", "path to Telegram session file")
	root.AddCommand(run)
	return root
}
