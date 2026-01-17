package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fatih/color"
	"go.octolab.org/errors"
	"go.octolab.org/safe"
	"go.octolab.org/unsafe"

	"go.octolab.org/toolset/indexit/internal/buildinfo"
	"go.octolab.org/toolset/indexit/internal/command"
	"go.octolab.org/toolset/indexit/internal/exitcode"
)

const unknown = "unknown"

var (
	commit  = unknown
	date    = unknown
	version = "dev"
	exit    = os.Exit
	stderr  = color.Error
	stdout  = color.Output
)

func main() {
	buildinfo.Set(version, commit, date)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			unsafe.DoSilent(fmt.Fprintln(stderr, "interrupt received, finishing current page..."))
			cancel()
		case <-ctx.Done():
			return
		}
		<-sigCh
		unsafe.DoSilent(fmt.Fprintln(stderr, "second interrupt, exiting now"))
		os.Exit(130)
	}()

	root := command.New()
	root.SetErr(stderr)
	root.SetOut(stdout)

	safe.Do(func() error { return root.ExecuteContext(ctx) }, shutdown)
}

func shutdown(err error) {
	code := exitcode.FromError(err)
	if isUsageError(err) {
		code = exitcode.Usage
	}
	var recovered errors.Recovered
	if errors.As(err, &recovered) {
		unsafe.DoSilent(fmt.Fprintf(stderr, "recovered: %+v\n", recovered.Cause()))
		unsafe.DoSilent(fmt.Fprintln(stderr, "---"))
		unsafe.DoSilent(fmt.Fprintf(stderr, "%+v\n", err))
		code = exitcode.Fail
	}
	exit(code)
}

func isUsageError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "unknown command") ||
		strings.Contains(message, "unknown flag") ||
		strings.Contains(message, "required flag") ||
		strings.Contains(message, "requires at least") ||
		strings.Contains(message, "accepts ") ||
		strings.Contains(message, "invalid argument")
}
