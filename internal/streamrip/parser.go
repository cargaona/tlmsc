package streamrip

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// searchResult represents a single search result from rip search -o JSON output
type searchResult struct {
	Source    string `json:"source"`
	MediaType string `json:"media_type"`
	ID        string `json:"id"`
	Desc      string `json:"desc"`
}

// ParseSearchJSON parses JSON output from rip search -o into Album objects
func ParseSearchJSON(data []byte) ([]Album, error) {
	var results []searchResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	var albums []Album
	for _, result := range results {
		album := parseSearchResult(result)
		if album != nil {
			albums = append(albums, *album)
		}
	}

	return albums, nil
}

// parseSearchResult converts a searchResult to an Album
func parseSearchResult(result searchResult) *Album {
	// Parse desc field: format is "<Title> by <Artist>"
	parts := strings.Split(result.Desc, " by ")
	if len(parts) < 2 {
		return nil
	}

	title := strings.TrimSpace(parts[0])
	artist := strings.TrimSpace(strings.Join(parts[1:], " by ")) // Handle artists with " by " in name

	return &Album{
		ID:     result.ID,
		Title:  title,
		Artist: artist,
		Year:   0, // Year not provided in JSON output
		Source: result.Source,
	}
}

// ParseSearchOutput parses streamrip search output into Album objects
func ParseSearchOutput(output string, source string) ([]Album, error) {
	var albums []Album

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse album line from streamrip output
		// Format: <ID> | <Title> | <Artist> | <Year>
		album := parseAlbumLine(line, source)
		if album != nil {
			albums = append(albums, *album)
		}
	}

	return albums, nil
}

// parseAlbumLine parses a single album line from streamrip output
func parseAlbumLine(line string, source string) *Album {
	// Split by pipe character
	parts := strings.Split(line, "|")
	if len(parts) < 4 {
		return nil
	}

	albumID := strings.TrimSpace(parts[0])
	title := strings.TrimSpace(parts[1])
	artist := strings.TrimSpace(parts[2])
	yearStr := strings.TrimSpace(parts[3])

	// Try to parse year
	year := 0
	if y, err := strconv.Atoi(yearStr); err == nil {
		year = y
	}

	return &Album{
		ID:     albumID,
		Title:  title,
		Artist: artist,
		Year:   year,
		Source: source,
	}
}

// ParseDownloadProgress parses download progress from streamrip output.
// Note: streamrip v2 uses rich/textual TUI progress bars that don't emit
// text when piped (non-TTY). In practice, no intermediate progress events
// are parsed — only the explicit "starting" and "completed" events from
// Client.Download are used. This function is kept for forward compatibility.
func ParseDownloadProgress(line string) (*Progress, error) {
	// Look for progress indicators in streamrip output
	// Common patterns: "Downloading: 50%", "[████████░░░░░░░░░░] 50%", "Track: Song Title"

	progress := &Progress{
		Status: "downloading",
	}

	// Pattern: percentage followed by % sign
	percentPattern := regexp.MustCompile(`(\d+)\s*%`)
	if matches := percentPattern.FindStringSubmatch(line); matches != nil {
		if p, err := strconv.Atoi(matches[1]); err == nil {
			progress.Percent = p
			return progress, nil
		}
	}

	// Pattern: "Track:" or "Downloading:" with track name
	trackPattern := regexp.MustCompile(`(?:Track|Downloading):\s*(.+)`)
	if matches := trackPattern.FindStringSubmatch(line); matches != nil {
		progress.CurrentTrack = strings.TrimSpace(matches[1])
		return progress, nil
	}

	// Pattern: "Downloaded" or "Completed"
	if strings.Contains(strings.ToLower(line), "completed") || strings.Contains(strings.ToLower(line), "done") {
		progress.Status = "completed"
		progress.Percent = 100
		return progress, nil
	}

	return nil, nil
}
