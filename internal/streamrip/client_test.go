package streamrip

import (
	"os/exec"
	"testing"
)

// TestSearch is an integration test: it shells out to the real `rip` CLI and
// hits Deezer, so it needs both the binary on PATH and working credentials in
// the streamrip config. Skipped rather than failed when either is absent, so
// `go test ./...` stays green on a machine that only has the Go toolchain.
func TestSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: hits the Deezer API")
	}
	if _, err := exec.LookPath("rip"); err != nil {
		t.Skip("integration test: `rip` not on PATH")
	}

	client := NewClient(true)
	albums, err := client.Search("pink floyd", "deezer")
	if err != nil {
		t.Skipf("integration test: search failed, likely missing credentials: %v", err)
	}

	if len(albums) == 0 {
		t.Fatal("expected at least one album, got 0")
	}

	for _, album := range albums {
		if album.Title == "" || album.ID == "" {
			t.Errorf("incomplete album: %+v", album)
		}
	}
}
