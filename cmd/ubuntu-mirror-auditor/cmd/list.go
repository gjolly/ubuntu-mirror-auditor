package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gauthier/ubuntu-mirror-auditor/pkg/scraper"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:          "list",
	Short:        "List all available Ubuntu CD mirrors",
	Long:         `List all available Ubuntu CD mirrors by parsing the content from https://launchpad.net/ubuntu/+cdmirrors`,
	SilenceUsage: true,
	RunE:         runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	mirrors, err := scraper.ListMirrors(ctx)
	if err != nil {
		return fmt.Errorf("failed to list mirrors: %w", err)
	}

	slog.Info("Retrieved mirrors", "count", len(mirrors))

	for _, mirror := range mirrors {
		fmt.Println(mirror.URL)
	}

	return nil
}
