// Package log builds indexit's structured logger from CLI flags and provides
// helpers for stashing it on a context so subcommands can pull it back out.
//
// The output handler is intentionally minimal — for a one-shot CLI the user
// values readable stderr lines more than machine-parseable structured logs.
// Level prefixes are added only for non-info levels; info lines look like
// plain text so existing user expectations ("loaded .env from /path", "proxy:
// socks5 host:port") are preserved.
package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Settings carries the live verbosity choices made on the command line.
type Settings struct {
	Logger    *slog.Logger
	GotdLog   *zap.Logger
	Heartbeat time.Duration
	Verbose   int
	Quiet     bool
}

// Options is what callers pass to Setup.
type Options struct {
	Out       io.Writer
	Verbose   int
	Quiet     bool
	Heartbeat time.Duration
}

// Setup builds the loggers from the given options and returns the resulting
// Settings. The returned slog.Logger is also installed as slog.Default so any
// library that uses package-level slog inherits the same destination.
func Setup(opts Options) *Settings {
	level := slog.LevelInfo
	switch {
	case opts.Quiet:
		level = slog.LevelError
	case opts.Verbose >= 1:
		level = slog.LevelDebug
	}

	logger := slog.New(&handler{w: opts.Out, level: level, mu: new(sync.Mutex)})
	slog.SetDefault(logger)

	gotd := zap.NewNop()
	if opts.Verbose >= 3 {
		encoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
		ws := zapcore.AddSync(opts.Out)
		gotd = zap.New(zapcore.NewCore(encoder, ws, zapcore.DebugLevel))
	}

	return &Settings{
		Logger:    logger,
		GotdLog:   gotd,
		Heartbeat: opts.Heartbeat,
		Verbose:   opts.Verbose,
		Quiet:     opts.Quiet,
	}
}

type ctxKey struct{}

// WithSettings stashes settings on the context.
func WithSettings(ctx context.Context, s *Settings) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// FromContext returns settings stored on the context, or a Nop-ish default.
func FromContext(ctx context.Context) *Settings {
	if s, ok := ctx.Value(ctxKey{}).(*Settings); ok && s != nil {
		return s
	}
	return defaultSettings()
}

var (
	defaultOnce sync.Once
	defaultVal  *Settings
)

func defaultSettings() *Settings {
	defaultOnce.Do(func() {
		defaultVal = &Settings{
			Logger:    slog.New(&handler{w: io.Discard, level: slog.LevelError, mu: new(sync.Mutex)}),
			GotdLog:   zap.NewNop(),
			Heartbeat: 0,
		}
	})
	return defaultVal
}

// handler renders slog records as compact human-friendly lines on the writer.
type handler struct {
	w     io.Writer
	level slog.Leveler
	// mu is shared with every handler cloned by WithAttrs/WithGroup: they all
	// write to the same w, so they must serialize against one another.
	mu     *sync.Mutex
	attrs  []slog.Attr
	groups []string
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	switch {
	case r.Level >= slog.LevelError:
		b.WriteString("error: ")
	case r.Level >= slog.LevelWarn:
		b.WriteString("warn: ")
	case r.Level < slog.LevelInfo:
		b.WriteString("debug: ")
	}
	b.WriteString(r.Message)
	for _, a := range h.attrs {
		writeAttr(&b, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&b, a)
		return true
	})
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func writeAttr(b *strings.Builder, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	b.WriteByte(' ')
	b.WriteString(a.Key)
	b.WriteByte('=')
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if strings.ContainsAny(s, " \t\"\n") {
			fmt.Fprintf(b, "%q", s)
		} else {
			b.WriteString(s)
		}
	default:
		b.WriteString(v.String())
	}
}
