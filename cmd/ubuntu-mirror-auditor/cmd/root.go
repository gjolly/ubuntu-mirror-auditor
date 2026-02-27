package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose bool
	rootCmd = &cobra.Command{
		Use:   "ubuntu-mirror-auditor",
		Short: "A tool to check the integrity of Ubuntu CD mirrors",
		Long: `Ubuntu Mirror Auditor is a tool that lets you check the integrity of Ubuntu CD mirrors.
It compares the contents of a mirror with the official Ubuntu archive and reports any discrepancies.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Configure logging
			logLevel := slog.LevelInfo
			if verbose {
				logLevel = slog.LevelDebug
			}

			handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: logLevel,
			})
			slog.SetDefault(slog.New(handler))
		},
	}
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("Command failed", "error", err)
		os.Exit(1)
	}
}
