package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gauthier/ubuntu-mirror-auditor/pkg/checker"
	"github.com/gauthier/ubuntu-mirror-auditor/pkg/database"
	"github.com/spf13/cobra"
)

var (
	daemonMirrorsFile  string
	daemonFile         string
	daemonDownloadDir  string
	daemonCorruptedDir string
	daemonDB           string
	daemonConcurrent   int
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the auditor in daemon mode",
	Long: `Run the auditor in daemon mode, continuously checking the integrity of mirrors listed in a file.
The file should contain one mirror URL per line.`,
	SilenceUsage: true,
	RunE:         runDaemon,
}

func init() {
	daemonCmd.Flags().StringVarP(&daemonMirrorsFile, "mirrors", "m", "", "File containing mirror URLs (one per line)")
	daemonCmd.Flags().StringVarP(&daemonFile, "file", "f", "", "Path to the artifact to use for comparison")
	daemonCmd.Flags().StringVarP(&daemonDownloadDir, "download-dir", "d", "downloads", "Directory to store downloaded files")
	daemonCmd.Flags().StringVar(&daemonCorruptedDir, "corrupted-dir", "corrupted", "Directory to store corrupted files")
	daemonCmd.Flags().StringVar(&daemonDB, "db", "mirrors.db", "Path to the SQLite database")
	daemonCmd.Flags().IntVarP(&daemonConcurrent, "concurrent", "c", 4, "Number of concurrent mirror checks")
	daemonCmd.MarkFlagRequired("mirrors")
	rootCmd.AddCommand(daemonCmd)
}

func runDaemon(cmd *cobra.Command, args []string) error {
	slog.Info("Starting daemon mode",
		"mirrors_file", daemonMirrorsFile,
		"database", daemonDB,
		"download_dir", daemonDownloadDir,
		"corrupted_dir", daemonCorruptedDir,
		"concurrent", daemonConcurrent)

	if daemonConcurrent < 1 {
		return fmt.Errorf("concurrent workers must be at least 1")
	}

	// Read mirrors from file
	mirrors, err := readMirrorsFromFile(daemonMirrorsFile)
	if err != nil {
		return fmt.Errorf("failed to read mirrors file: %w", err)
	}

	if len(mirrors) == 0 {
		return fmt.Errorf("no mirrors found in file")
	}

	slog.Info("Loaded mirrors", "count", len(mirrors))

	// Open database
	db, err := database.NewDB(daemonDB)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Initialize mirrors in database (insert placeholder records for new mirrors)
	if err := db.InitializeMirrors(mirrors); err != nil {
		return fmt.Errorf("failed to initialize mirrors: %w", err)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create context that will be cancelled on signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals in a goroutine
	go func() {
		sig := <-sigChan
		slog.Info("Received signal, shutting down gracefully...", "signal", sig)
		cancel()

		// Clean up temporary download directory (corrupted directory is separate)
		slog.Info("Cleaning up download directory", "dir", daemonDownloadDir)
		if err := os.RemoveAll(daemonDownloadDir); err != nil {
			slog.Error("Failed to clean up download directory", "error", err)
		}
	}()

	// Create work queue
	workQueue := make(chan string, daemonConcurrent*2)

	// Use a mutex to synchronize database writes
	var dbMutex sync.Mutex

	// Start worker pool
	var wg sync.WaitGroup

	for i := 0; i < daemonConcurrent; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Create a checker for this worker
			c := checker.NewChecker(daemonDownloadDir, daemonCorruptedDir)

			for mirrorURL := range workQueue {
				slog.Info("Checking mirror", "worker", workerID, "url", mirrorURL)

				// Check the mirror with retry logic
				var result *checker.CheckResult
				maxRetries := 3
				for attempt := 1; attempt <= maxRetries; attempt++ {
					result, err = c.CheckMirror(ctx, mirrorURL, daemonFile)
					if err != nil {
						slog.Error("Failed to check mirror", "worker", workerID, "error", err, "attempt", attempt)
						if attempt < maxRetries {
							slog.Info("Retrying...", "worker", workerID, "attempt", attempt+1)
							time.Sleep(5 * time.Second)
							continue
						}
						break
					}
					break
				}

				// Record the result in the database
				record := database.ProbeRecord{
					MirrorURL: mirrorURL,
					Time:      time.Now(),
					TestFile:  result.TestFile,
					Result:    result.Success,
				}

				if result.ErrorMessage != "" {
					record.TestError = &result.ErrorMessage
				}

				if len(result.CorruptedFiles) > 0 {
					corrupted := strings.Join(result.CorruptedFiles, ",")
					record.CorruptedFiles = &corrupted
				}

				// Synchronize database writes
				dbMutex.Lock()
				if err := db.InsertProbe(record); err != nil {
					slog.Error("Failed to insert probe record", "worker", workerID, "error", err)
				}
				dbMutex.Unlock()

				if result.Success {
					slog.Info("Mirror check passed", "worker", workerID, "mirror", mirrorURL)
				} else {
					slog.Warn("Mirror check failed", "worker", workerID, "mirror", mirrorURL, "error", result.ErrorMessage)
				}
			}
		}(i)
	}

	// Coordinator goroutine - continuously feeds work to the queue
	go func() {
		defer close(workQueue)
		for {
			// Check if context is cancelled
			select {
			case <-ctx.Done():
				slog.Info("Coordinator stopping due to context cancellation")
				return
			default:
			}

			// Get the oldest checked mirrors to fill the queue
			// We'll try to get multiple mirrors at once to keep workers busy
			dbMutex.Lock()
			mirrorURL, err := db.GetOldestCheckedMirror(mirrors)
			dbMutex.Unlock()

			if err != nil {
				slog.Error("Failed to get oldest checked mirror", "error", err)
				time.Sleep(10 * time.Second)
				continue
			}

			// Send mirror to work queue (this will block if queue is full)
			// Use select to also check for context cancellation while sending
			select {
			case workQueue <- mirrorURL:
				// Successfully queued
			case <-ctx.Done():
				slog.Info("Coordinator stopping due to context cancellation")
				return
			}

			// Small delay to prevent tight loop
			time.Sleep(1 * time.Second)
		}
	}()

	// Wait for workers to finish
	wg.Wait()
	slog.Info("All workers stopped, daemon shutdown complete")

	return nil
}

func readMirrorsFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var mirrors []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			mirrors = append(mirrors, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return mirrors, nil
}
