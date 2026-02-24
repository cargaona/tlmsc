package streamrip

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Client struct {
	Debug bool
}

func NewClient(debug bool) *Client {
	return &Client{Debug: debug}
}

// Search searches for albums on the given source
func (c *Client) Search(query string, source string) ([]Album, error) {
	// Create a temporary JSON file for output
	tmpFile, err := os.CreateTemp("", "rip-search-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Run: rip search <source> album <query> -o <tmpfile>
	cmd := exec.Command("rip", "search", source, "album", query, "-o", tmpPath)

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Read the JSON output file
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read search results: %w", err)
	}

	// Parse JSON output
	return ParseSearchJSON(data)
}

// Download downloads an album by ID from the given source
// Progress is sent to the progressCh channel
func (c *Client) Download(albumID, source string, progressCh chan<- Progress) error {
	// Use: rip id <source> album <albumID>
	cmd := exec.Command("rip", "id", source, "album", albumID)

	// Pipe stderr (streamrip writes progress to stderr, not stdout)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("download failed to start: %w", err)
	}

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if c.Debug {
			fmt.Printf("[streamrip] %s\n", line)
		}

		if progress, err := ParseDownloadProgress(line); err == nil && progress != nil {
			progressCh <- *progress
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("download command failed: %w", err)
	}

	// Send explicit completion status
	progressCh <- Progress{
		Status:  "completed",
		Percent: 100,
	}

	return nil
}

// CheckStreamripInstalled checks if streamrip is installed
func (c *Client) CheckStreamripInstalled() bool {
	cmd := exec.Command("rip", "--version")
	return cmd.Run() == nil
}

// GetStagingFolder returns the folder path for a downloaded album
func GetStagingFolder(stagingPath string, albumTitle string) string {
	// Sanitize album title for folder name
	sanitized := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, albumTitle)

	return filepath.Join(stagingPath, strings.TrimSpace(sanitized))
}
