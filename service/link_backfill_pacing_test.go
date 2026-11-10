package service

import (
	"fmt"
	"testing"
	"time"
)

// The backfill pauses between batches of writes so other SQLite writers can
// take the lock; it used to write back to back for the whole run.
func TestLinkBackfillPausesBetweenBatchesOfWrites(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "pacing", true)
	notes := `<p><a href="https://example.com/a">A</a></p>`
	const episodes = 2*linkBackfillYieldEvery + 5
	for i := 0; i < episodes; i++ {
		createShowNotesItem(t, podcast, fmt.Sprintf("episode-%02d", i), notes)
	}

	previous := linkBackfillPause
	t.Cleanup(func() { linkBackfillPause = previous })
	var pauses []time.Duration
	linkBackfillPause = func(d time.Duration) { pauses = append(pauses, d) }

	succeeded, _, err := BackfillShowNoteLinks()
	if err != nil {
		t.Fatal(err)
	}
	if succeeded != episodes {
		t.Fatalf("succeeded=%d, want %d", succeeded, episodes)
	}
	if len(pauses) != 2 {
		t.Fatalf("expected 2 pauses for %d writes, got %d", episodes, len(pauses))
	}
	for _, d := range pauses {
		if d <= 100*time.Millisecond {
			t.Fatalf("a pause of %v is shorter than SQLite's busy-handler sleep", d)
		}
	}
}
