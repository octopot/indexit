package command

import (
	"errors"
	"time"

	"github.com/spf13/cobra"
	cli "go.octolab.org/toolkit/cli/cobra"

	"go.octolab.org/toolset/indexit/internal/buildinfo"
	"go.octolab.org/toolset/indexit/internal/command/telegram"
	"go.octolab.org/toolset/indexit/internal/config"
	"go.octolab.org/toolset/indexit/internal/exitcode"
	indexlog "go.octolab.org/toolset/indexit/internal/log"
	"go.octolab.org/toolset/indexit/skills"
)

// New returns the new root command with the whole command tree. Call
// buildinfo.Set first: the version command reports what it holds.
func New() *cobra.Command {
	var (
		envFile   string
		verbose   int
		quiet     bool
		heartbeat time.Duration
	)

	command := cobra.Command{
		Use:   "indexit",
		Short: "indexit",
		Long:  "indexit",

		Args: cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if quiet && verbose > 0 {
				return exitcode.New(exitcode.Usage,
					errors.New("--quiet and --verbose are mutually exclusive"))
			}
			settings := indexlog.Setup(indexlog.Options{
				Out:       cmd.ErrOrStderr(),
				Verbose:   verbose,
				Quiet:     quiet,
				Heartbeat: heartbeat,
			})
			cmd.SetContext(indexlog.WithSettings(cmd.Context(), settings))

			result, err := config.Load(config.Options{
				EnvFile:  envFile,
				Explicit: cmd.Flags().Changed("env-file"),
			})
			if err != nil {
				return exitcode.New(exitcode.Usage, err)
			}
			if result.Path != "" {
				settings.Logger.Info("loaded .env", "path", result.Path)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},

		SilenceErrors: false,
		SilenceUsage:  true,
	}

	command.PersistentFlags().StringVar(&envFile, "env-file", "",
		"path to .env file (default: auto-discover; pass \"\" to disable)")
	command.PersistentFlags().CountVarP(&verbose, "verbose", "v",
		"increase log verbosity (-v debug, -vvv enables gotd internals)")
	command.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false,
		"suppress everything below error level (stdout payload unaffected)")
	command.PersistentFlags().DurationVar(&heartbeat, "heartbeat", 10*time.Second,
		"emit a 'still waiting...' line if a network call exceeds this duration (0 disables)")

	/* configure instance */
	command.AddCommand(
		telegram.New(),
		skillCommand(skills.Indexit()),
		cli.NewVersionCommand(buildinfo.Version, buildinfo.Date, buildinfo.Commit),
	)

	return &command
}
