package scraper

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"
)

const (
	ubuntuCDMirrorsURL = "https://launchpad.net/ubuntu/+cdmirrors"
	httpTimeout        = 30 * time.Second
)

// Mirror represents a Ubuntu CD mirror
type Mirror struct {
	URL     string
	Country string
	Name    string
}

// ListMirrors fetches and parses the list of Ubuntu CD mirrors from Launchpad
func ListMirrors(ctx context.Context) ([]Mirror, error) {
	slog.Info("Fetching mirror list from Launchpad")

	client := &http.Client{
		Timeout: httpTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ubuntuCDMirrorsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch mirrors page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read the entire response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	html := string(body)

	// Extract mirror HTTP URLs from the HTML file
	// Pattern: <a href="http://example.com/path/">http</a>
	mirrorRegex := regexp.MustCompile(`<a href="(http[^"]+)">https?</a>`)
	matches := mirrorRegex.FindAllStringSubmatch(html, -1)

	var mirrors []Mirror
	for _, match := range matches {
		if len(match) > 1 {
			mirror := Mirror{
				URL: match[1],
			}
			mirrors = append(mirrors, mirror)
		}
	}

	slog.Info("Successfully fetched mirrors", "count", len(mirrors))
	return mirrors, nil
}
