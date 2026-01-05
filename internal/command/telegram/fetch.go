package telegram

import (
	"context"
	"errors"
	"time"

	"github.com/gotd/td/telegram/auth"
	"github.com/spf13/cobra"

	indexlog "go.octolab.org/toolset/indexit/internal/log"
	tgsvc "go.octolab.org/toolset/indexit/internal/telegram"
	"go.octolab.org/toolset/indexit/internal/telegram/output"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

type countingWriter struct {
	inner tgsvc.Writer
	count int
}

func (w *countingWriter) Write(v any) error {
	if err := w.inner.Write(v); err != nil {
		return err
	}
	w.count++
	return nil
}

type fetchOptions struct {
	format   string
	output   string
	limit    int
	pageSize int
}

type topicsOptions struct {
	dialog string
}

type mediaOptions struct {
	messagesOptions

	dir       string
	kinds     []string
	overwrite bool
}

type messagesOptions struct {
	dialog string
	minID  int
	maxID  int
	from   string
	to     string
}

func fetchCommand(opt *options) *cobra.Command {
	var fetchOpt fetchOptions
	command := cobra.Command{
		Use:   "fetch",
		Short: "Fetch Telegram data as JSONL",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	command.PersistentFlags().StringVar(&fetchOpt.format, "format", "jsonl", "output format")
	command.PersistentFlags().StringVarP(&fetchOpt.output, "output", "o", "-", "output path, or - for stdout")
	command.PersistentFlags().IntVar(&fetchOpt.limit, "limit", 0, "maximum number of emitted records; 0 means all")
	command.PersistentFlags().IntVar(&fetchOpt.pageSize, "page-size", 100, "Telegram page size")
	command.AddCommand(
		fetchDialogsCommand(opt, &fetchOpt),
		fetchMessagesCommand(opt, &fetchOpt),
		fetchTopicsCommand(opt, &fetchOpt),
		fetchMediaCommand(opt, &fetchOpt),
	)
	return &command
}

func fetchDialogsCommand(opt *options, fetchOpt *fetchOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "dialogs",
		Short: "Fetch Telegram dialogs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(fetchOpt.format); err != nil {
				return usageErr(err)
			}
			paths, err := pathsFromFlags(opt)
			if err != nil {
				return err
			}
			ctx, cancel, err := contextFromFlags(cmd, opt)
			if err != nil {
				return err
			}
			defer cancel()
			client, err := newClient(cmd, paths)
			if err != nil {
				return err
			}
			log := indexlog.FromContext(cmd.Context()).Logger
			cache, err := peers.Load(paths.Peers)
			if err != nil {
				return err
			}
			log.Info("cache: loaded", "peers", cache.Len(), "path", paths.Peers)
			writer, err := output.New(fetchOpt.output, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			defer writer.Close()
			counter := &countingWriter{inner: writer}
			start := time.Now()
			err = client.Run(ctx, func(ctx context.Context, api tgsvc.API, _ *auth.Client) error {
				return tgsvc.FetchDialogs(ctx, api, cache, counter, tgsvc.DialogsOptions{
					Limit:    fetchOpt.limit,
					PageSize: fetchOpt.pageSize,
				}, tgsvc.RateGuard{})
			})
			if saveErr := cache.Save(paths.Peers); err == nil {
				err = saveErr
				log.Info("cache: persisted", "peers", cache.Len(), "path", paths.Peers)
			}
			log.Info("done", "dialogs", counter.count, "elapsed", time.Since(start).Round(time.Millisecond))
			return err
		},
	}
}

func fetchMessagesCommand(opt *options, fetchOpt *fetchOptions) *cobra.Command {
	var msgOpt messagesOptions
	command := cobra.Command{
		Use:   "messages",
		Short: "Fetch Telegram messages from one dialog",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(fetchOpt.format); err != nil {
				return usageErr(err)
			}
			ref, err := uid.Parse(msgOpt.dialog)
			if err != nil {
				return usageErr(err)
			}
			from, err := parseRFC3339(msgOpt.from, "--from")
			if err != nil {
				return usageErr(err)
			}
			to, err := parseRFC3339(msgOpt.to, "--to")
			if err != nil {
				return usageErr(err)
			}
			paths, err := pathsFromFlags(opt)
			if err != nil {
				return err
			}
			ctx, cancel, err := contextFromFlags(cmd, opt)
			if err != nil {
				return err
			}
			defer cancel()
			client, err := newClient(cmd, paths)
			if err != nil {
				return err
			}
			log := indexlog.FromContext(cmd.Context()).Logger
			cache, err := peers.Load(paths.Peers)
			if err != nil {
				return err
			}
			log.Info("cache: loaded", "peers", cache.Len(), "path", paths.Peers)
			writer, err := output.New(fetchOpt.output, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			defer writer.Close()
			counter := &countingWriter{inner: writer}
			start := time.Now()
			err = client.Run(ctx, func(ctx context.Context, api tgsvc.API, _ *auth.Client) error {
				err := tgsvc.FetchMessages(ctx, api, cache, counter, tgsvc.MessagesOptions{
					Peer:     ref,
					Limit:    fetchOpt.limit,
					PageSize: fetchOpt.pageSize,
					MinID:    msgOpt.minID,
					MaxID:    msgOpt.maxID,
					From:     from,
					To:       to,
				}, tgsvc.RateGuard{})
				if errors.Is(err, tgsvc.ErrColdPeer) {
					return usageErr(tgsvc.ColdPeerHint(err, ref.String()))
				}
				return err
			})
			if saveErr := cache.Save(paths.Peers); err == nil {
				err = saveErr
				log.Info("cache: persisted", "peers", cache.Len(), "path", paths.Peers)
			}
			log.Info("done", "messages", counter.count, "elapsed", time.Since(start).Round(time.Millisecond))
			return err
		},
	}
	command.Flags().StringVar(&msgOpt.dialog, "dialog", "", "dialog UID")
	command.Flags().IntVar(&msgOpt.minID, "min-id", 0, "minimum MTProto message ID")
	command.Flags().IntVar(&msgOpt.maxID, "max-id", 0, "maximum MTProto message ID")
	command.Flags().StringVar(&msgOpt.from, "from", "", "start timestamp in RFC3339")
	command.Flags().StringVar(&msgOpt.to, "to", "", "end timestamp in RFC3339")
	_ = command.MarkFlagRequired("dialog")
	return &command
}

func fetchTopicsCommand(opt *options, fetchOpt *fetchOptions) *cobra.Command {
	var topicOpt topicsOptions
	command := cobra.Command{
		Use:   "topics",
		Short: "Fetch forum topics of one dialog",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(fetchOpt.format); err != nil {
				return usageErr(err)
			}
			ref, err := uid.Parse(topicOpt.dialog)
			if err != nil {
				return usageErr(err)
			}
			paths, err := pathsFromFlags(opt)
			if err != nil {
				return err
			}
			ctx, cancel, err := contextFromFlags(cmd, opt)
			if err != nil {
				return err
			}
			defer cancel()
			client, err := newClient(cmd, paths)
			if err != nil {
				return err
			}
			log := indexlog.FromContext(cmd.Context()).Logger
			cache, err := peers.Load(paths.Peers)
			if err != nil {
				return err
			}
			log.Info("cache: loaded", "peers", cache.Len(), "path", paths.Peers)
			writer, err := output.New(fetchOpt.output, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			defer writer.Close()
			counter := &countingWriter{inner: writer}
			start := time.Now()
			err = client.Run(ctx, func(ctx context.Context, api tgsvc.API, _ *auth.Client) error {
				err := tgsvc.FetchTopics(ctx, api, cache, counter, tgsvc.TopicsOptions{
					Peer:     ref,
					Limit:    fetchOpt.limit,
					PageSize: fetchOpt.pageSize,
				}, tgsvc.RateGuard{})
				if errors.Is(err, tgsvc.ErrColdPeer) {
					return usageErr(tgsvc.ColdPeerHint(err, ref.String()))
				}
				return err
			})
			if saveErr := cache.Save(paths.Peers); err == nil {
				err = saveErr
				log.Info("cache: persisted", "peers", cache.Len(), "path", paths.Peers)
			}
			log.Info("done", "topics", counter.count, "elapsed", time.Since(start).Round(time.Millisecond))
			return err
		},
	}
	command.Flags().StringVar(&topicOpt.dialog, "dialog", "", "dialog UID")
	_ = command.MarkFlagRequired("dialog")
	return &command
}

func fetchMediaCommand(opt *options, fetchOpt *fetchOptions) *cobra.Command {
	var mediaOpt mediaOptions
	command := cobra.Command{
		Use:   "media",
		Short: "Download media of one dialog into a directory",
		Long: "Download media of one dialog into a directory.\n\n" +
			"Files go to --dir; --output keeps its usual meaning and receives the\n" +
			"JSONL manifest, one record per file.\n\n" +
			"To narrow the run to one forum topic, address it as channel:<id>:<topic>.\n" +
			"A t.me/c/<peer>/<n> link means message <n>, not topic <n> — pasted alone\n" +
			"it walks the whole dialog.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(fetchOpt.format); err != nil {
				return usageErr(err)
			}
			ref, err := uid.Parse(mediaOpt.dialog)
			if err != nil {
				return usageErr(err)
			}
			from, err := parseRFC3339(mediaOpt.from, "--from")
			if err != nil {
				return usageErr(err)
			}
			to, err := parseRFC3339(mediaOpt.to, "--to")
			if err != nil {
				return usageErr(err)
			}
			paths, err := pathsFromFlags(opt)
			if err != nil {
				return err
			}
			ctx, cancel, err := contextFromFlags(cmd, opt)
			if err != nil {
				return err
			}
			defer cancel()
			client, err := newClient(cmd, paths)
			if err != nil {
				return err
			}
			log := indexlog.FromContext(cmd.Context()).Logger
			cache, err := peers.Load(paths.Peers)
			if err != nil {
				return err
			}
			log.Info("cache: loaded", "peers", cache.Len(), "path", paths.Peers)
			writer, err := output.New(fetchOpt.output, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			defer writer.Close()
			counter := &countingWriter{inner: writer}
			start := time.Now()
			err = client.RunMedia(ctx, func(ctx context.Context, api tgsvc.MediaAPI) error {
				err := tgsvc.FetchMedia(ctx, api, cache, counter, tgsvc.MediaOptions{
					Peer:      ref,
					Dir:       mediaOpt.dir,
					Kinds:     mediaOpt.kinds,
					Limit:     fetchOpt.limit,
					PageSize:  fetchOpt.pageSize,
					MinID:     mediaOpt.minID,
					MaxID:     mediaOpt.maxID,
					From:      from,
					To:        to,
					Overwrite: mediaOpt.overwrite,
				}, tgsvc.RateGuard{})
				if errors.Is(err, tgsvc.ErrColdPeer) {
					return usageErr(tgsvc.ColdPeerHint(err, ref.String()))
				}
				return err
			})
			if saveErr := cache.Save(paths.Peers); err == nil {
				err = saveErr
				log.Info("cache: persisted", "peers", cache.Len(), "path", paths.Peers)
			}
			log.Info("done", "files", counter.count, "elapsed", time.Since(start).Round(time.Millisecond))
			return err
		},
	}
	command.Flags().StringVar(&mediaOpt.dialog, "dialog", "", "dialog UID")
	command.Flags().StringVar(&mediaOpt.dir, "dir", "", "destination directory for downloaded files")
	command.Flags().StringSliceVar(&mediaOpt.kinds, "media", nil,
		"media types to download, e.g. photo,video; empty means every downloadable type")
	command.Flags().BoolVar(&mediaOpt.overwrite, "overwrite", false, "re-download files that already exist")
	command.Flags().IntVar(&mediaOpt.minID, "min-id", 0, "minimum MTProto message ID")
	command.Flags().IntVar(&mediaOpt.maxID, "max-id", 0, "maximum MTProto message ID")
	command.Flags().StringVar(&mediaOpt.from, "from", "", "start timestamp in RFC3339")
	command.Flags().StringVar(&mediaOpt.to, "to", "", "end timestamp in RFC3339")
	_ = command.MarkFlagRequired("dialog")
	_ = command.MarkFlagRequired("dir")
	return &command
}
