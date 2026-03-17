package cover

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

// deezerAlbum represents the relevant fields from Deezer's album API
type deezerAlbum struct {
	CoverBig string `json:"cover_big"`
}

// deezerSearchResponse represents the Deezer search API response
type deezerSearchResponse struct {
	Data []deezerAlbum `json:"data"`
}

// FetchCoverURL returns a cover art URL for the given album.
// For deezer source, it uses the album ID directly.
// For other sources, it falls back to searching Deezer by artist + title.
// Returns empty string if no cover is found.
func FetchCoverURL(albumID string, source string, artist string, title string) string {
	if source == "deezer" {
		if url := fetchDeezerCover(albumID); url != "" {
			return url
		}
	}

	// Fallback: search Deezer by artist + title
	return searchDeezerCover(artist, title)
}

// fetchDeezerCover fetches cover art directly from Deezer album API
func fetchDeezerCover(albumID string) string {
	resp, err := httpClient.Get(fmt.Sprintf("https://api.deezer.com/album/%s", albumID))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var album deezerAlbum
	if err := json.NewDecoder(resp.Body).Decode(&album); err != nil {
		return ""
	}

	return album.CoverBig
}

// searchDeezerCover searches Deezer for an album and returns the first cover
func searchDeezerCover(artist string, title string) string {
	query := url.QueryEscape(fmt.Sprintf("%s %s", artist, title))
	resp, err := httpClient.Get(fmt.Sprintf("https://api.deezer.com/search/album?q=%s&limit=1", query))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var result deezerSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	if len(result.Data) > 0 && result.Data[0].CoverBig != "" {
		return result.Data[0].CoverBig
	}

	return ""
}
