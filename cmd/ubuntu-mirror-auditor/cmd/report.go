package cmd

import (
	"fmt"
	"log/slog"

	"github.com/gauthier/ubuntu-mirror-auditor/pkg/database"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

var (
	reportDB string
)

var reportCmd = &cobra.Command{
	Use:          "report",
	Short:        "Generate a report of mirror checks",
	Long:         `Generate a report of the mirror checks stored in the database.`,
	SilenceUsage: true,
	RunE:         runReport,
}

func init() {
	reportCmd.Flags().StringVar(&reportDB, "db", "mirrors.db", "Path to the SQLite database")
	rootCmd.AddCommand(reportCmd)
}

type ReportData struct {
	MirrorsTested int            `yaml:"mirrors_tested"`
	TotalMirrors  int            `yaml:"total_mirrors"`
	FailedChecks  int            `yaml:"failed_checks"`
	ErrorChecks   int            `yaml:"error_checks"`
	FailedMirrors []FailedMirror `yaml:"failed_mirrors,omitempty"`
	ErrorMirrors  []ErrorMirror  `yaml:"errors,omitempty"`
}

type FailedMirror struct {
	MirrorURL       string   `yaml:"mirror_url"`
	LastFailedCheck string   `yaml:"last_failed_check"`
	CorruptedFiles  []string `yaml:"corrupted_files,omitempty"`
}

type ErrorMirror struct {
	MirrorURL       string `yaml:"mirror_url"`
	LastFailedCheck string `yaml:"last_failed_check"`
	ErrorMessage    string `yaml:"error_message"`
}

func runReport(cmd *cobra.Command, args []string) error {
	slog.Info("Generating report", "database", reportDB)

	db, err := database.NewDB(reportDB)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Get latest probe for each mirror
	probes, err := db.GetLatestProbesByMirror()
	if err != nil {
		return fmt.Errorf("failed to get probes: %w", err)
	}

	// Get distinct mirror count
	distinctCount, err := db.GetDistinctMirrorCount()
	if err != nil {
		return fmt.Errorf("failed to get distinct mirror count: %w", err)
	}

	report := ReportData{
		MirrorsTested: distinctCount,
		TotalMirrors:  len(probes), // This is actually the same as distinctCount
	}

	// Process probes
	for _, probe := range probes {
		if !probe.Result {
			if probe.TestError != nil && *probe.TestError != "" {
				// Error case
				report.ErrorChecks++
				report.ErrorMirrors = append(report.ErrorMirrors, ErrorMirror{
					MirrorURL:       probe.MirrorURL,
					LastFailedCheck: probe.Time.Format("2006-01-02T15:04:05Z"),
					ErrorMessage:    *probe.TestError,
				})
			} else {
				// Failed checksum case
				report.FailedChecks++
				failed := FailedMirror{
					MirrorURL:       probe.MirrorURL,
					LastFailedCheck: probe.Time.Format("2006-01-02T15:04:05Z"),
				}
				if probe.CorruptedFiles != nil && *probe.CorruptedFiles != "" {
					// Split the comma-separated corrupted files
					files := splitCorruptedFiles(*probe.CorruptedFiles)
					failed.CorruptedFiles = files
				}
				report.FailedMirrors = append(report.FailedMirrors, failed)
			}
		}
	}

	// Output report as YAML
	yamlData, err := yaml.Marshal(&report)
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	fmt.Println(string(yamlData))
	return nil
}

func splitCorruptedFiles(s string) []string {
	if s == "" {
		return nil
	}

	var result []string
	for _, file := range splitString(s, ",") {
		trimmed := trimSpace(file)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitString(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep[0] {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}
