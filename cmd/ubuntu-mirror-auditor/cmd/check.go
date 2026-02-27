package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gauthier/ubuntu-mirror-auditor/pkg/checker"
	"github.com/spf13/cobra"
)

var (
	checkFile string
)

var checkCmd = &cobra.Command{
	Use:   "check [mirror_url]",
	Short: "Check the integrity of a specific mirror",
	Long: `Check the integrity of a specific mirror by comparing its contents with the official Ubuntu archive.
The tool uses the latest server cdimage as a reference for comparison.`,
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runCheck,
}

func init() {
	checkCmd.Flags().StringVarP(&checkFile, "file", "f", "", "Path to the artifact to use for comparison")
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	mirrorURL := args[0]
	ctx := context.Background()

	slog.Info("Checking mirror", "url", mirrorURL, "artifact", checkFile)

	c := checker.NewChecker("downloads", "corrupted")
	result, err := c.CheckMirror(ctx, mirrorURL, checkFile)
	if err != nil {
		return fmt.Errorf("failed to check mirror: %w", err)
	}

	if result.Success {
		fmt.Printf("✓ Mirror check passed for %s\n", mirrorURL)
		return nil
	}

	if result.ErrorMessage != "" {
		fmt.Printf("✗ Mirror check failed with error: %s\n", result.ErrorMessage)
		return fmt.Errorf("check failed")
	}

	fmt.Printf("✗ Mirror check failed: checksum mismatch\n")
	if len(result.CorruptedFiles) > 0 {
		fmt.Printf("Corrupted files saved: %v\n", result.CorruptedFiles)
	}

	return fmt.Errorf("check failed")
}
