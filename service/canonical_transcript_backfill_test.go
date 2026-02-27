package service

import (
	"testing"

	"github.com/ctaylor1/briefcast/db"
)

// TestBackfillCanonicalTranscripts handles the corresponding operation.
func TestBackfillCanonicalTranscripts(t *testing.T) {
	setupRetentionTestDB(t)

	podcast := createPodcast(t, "canonical-backfill", false)

	needsBackfill := createServicePodcastItem(t, podcast, "needs-backfill", db.Downloaded)
	needsBackfill.TranscriptStatus = "available"
	needsBackfill.TranscriptJSON = `{
		"segments":[
			{"start":0.0,"end":1.0,"speaker":"speaker_00","text":"  Hello  ","words":[{"start":0.1,"word":"Hello"}]},
			{"start":1.4,"end":2.0,"speaker":"speaker_00","text":"world !","words":[{"start":1.5,"word":"world"}]}
		]
	}`
	needsBackfill.CanonicalTranscript = ""
	needsBackfill.CanonicalTranscriptVersion = 0
	needsBackfill.CanonicalUpdatedAt = nil
	if err := db.UpdatePodcastItem(&needsBackfill); err != nil {
		t.Fatalf("failed to seed backfill target item: %v", err)
	}

	alreadyCanonical := createServicePodcastItem(t, podcast, "already-canonical", db.Downloaded)
	alreadyCanonical.TranscriptStatus = "available"
	alreadyCanonical.TranscriptJSON = `{"segments":[{"start":0.0,"end":1.0,"text":"preset"}]}`
	alreadyCanonical.CanonicalTranscript = "SPEAKER_00: preset"
	alreadyCanonical.CanonicalTranscriptVersion = canonicalTranscriptVersionCurrent
	if err := db.UpdatePodcastItem(&alreadyCanonical); err != nil {
		t.Fatalf("failed to seed already canonical item: %v", err)
	}

	updated, err := BackfillCanonicalTranscripts(1, canonicalTranscriptVersionCurrent)
	if err != nil {
		t.Fatalf("backfill failed: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected one row updated by backfill, got %d", updated)
	}

	var refreshed db.PodcastItem
	if err := db.GetPodcastItemByID(needsBackfill.ID, &refreshed); err != nil {
		t.Fatalf("failed to reload backfilled item: %v", err)
	}
	if refreshed.CanonicalTranscript != "SPEAKER_00: Hello world!" {
		t.Fatalf("unexpected canonical transcript %q", refreshed.CanonicalTranscript)
	}
	if refreshed.CanonicalTranscriptVersion != canonicalTranscriptVersionCurrent {
		t.Fatalf(
			"expected canonical transcript version %d, got %d",
			canonicalTranscriptVersionCurrent,
			refreshed.CanonicalTranscriptVersion,
		)
	}
	if refreshed.CanonicalUpdatedAt == nil {
		t.Fatalf("expected canonical updated timestamp to be set")
	}

	updated, err = BackfillCanonicalTranscripts(1, canonicalTranscriptVersionCurrent)
	if err != nil {
		t.Fatalf("second backfill failed: %v", err)
	}
	if updated != 0 {
		t.Fatalf("expected second backfill run to be idempotent, got %d updates", updated)
	}
}
