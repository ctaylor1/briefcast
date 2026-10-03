package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

func TestMonolithRollbackDryRunBackupAndPreservation(t *testing.T) {
	f := newRemoteTestFixture(t)
	b, err := newHTTPTranscriptionBackend(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	envelope, err := b.prepare(context.Background(), f.item)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	if err := db.DB.Model(&db.PodcastItem{}).Where("id = ?", f.item.ID).Updates(map[string]any{
		"transcript_checkpoint_json": string(raw), "transcript_status": "processing", "transcript_progress_pct": 45,
		"bookmark_date": time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC), "is_played": true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "rollback.json")
	counts, err := PrepareMonolithRollback(db.DB, backup, false)
	if err != nil || counts.Selected != 1 || counts.Reset != 0 {
		t.Fatalf("dry run: %+v %v", counts, err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("dry run wrote a backup")
	}
	counts, err = PrepareMonolithRollback(db.DB, backup, true)
	if err != nil || counts.Reset != 1 {
		t.Fatalf("apply: %+v %v", counts, err)
	}
	var item db.PodcastItem
	if err := db.GetPodcastItemByID(f.item.ID, &item); err != nil {
		t.Fatal(err)
	}
	if item.TranscriptCheckpointJSON != "" || item.TranscriptProgressPct != 0 || item.TranscriptStatus != "pending_whisperx" || item.BookmarkDate.IsZero() || !item.IsPlayed || item.DownloadPath != f.item.DownloadPath || item.LLMSummaryStatus != f.item.LLMSummaryStatus {
		t.Fatal("rollback changed unrelated fields or retained remote progress")
	}
	if _, err := os.Stat(item.DownloadPath); err != nil {
		t.Fatal("audio changed")
	}
	stored, err := os.ReadFile(backup)
	if err != nil || !json.Valid(stored) {
		t.Fatal("backup missing")
	}
	// Existing backups are never replaced, even when the queue is now empty.
	if _, err := PrepareMonolithRollback(db.DB, backup, true); err == nil {
		t.Fatal("overwrote backup")
	}
}

func TestMonolithRollbackUnknownEnvelopeAndCompletedOutput(t *testing.T) {
	f := newRemoteTestFixture(t)
	if err := db.DB.Model(&db.PodcastItem{}).Where("id = ?", f.item.ID).Updates(map[string]any{
		"transcript_checkpoint_json": `{"version":2,"backend":"http"}`, "transcript_status": "failed",
	}).Error; err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "rollback.json")
	if _, err := PrepareMonolithRollback(db.DB, backup, true); err == nil {
		t.Fatal("unknown envelope reset")
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("invalid plan created backup")
	}
	if err := db.DB.Model(&db.PodcastItem{}).Where("id = ?", f.item.ID).Updates(map[string]any{"transcript_json": `{"segments":[]}`, "canonical_transcript": "preserved output"}).Error; err != nil {
		t.Fatal(err)
	}
	counts, err := PrepareMonolithRollback(db.DB, "", false)
	if err != nil || counts.Selected != 0 {
		t.Fatal("completed transcript selected")
	}
}

func TestMonolithRollbackPersistenceFailureKeepsOriginalEnvelope(t *testing.T) {
	f := newRemoteTestFixture(t)
	b, err := newHTTPTranscriptionBackend(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	envelope, err := b.prepare(context.Background(), f.item)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	if err := db.DB.Model(&db.PodcastItem{}).Where("id = ?", f.item.ID).Updates(map[string]any{"transcript_checkpoint_json": string(raw), "transcript_status": "processing"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec("CREATE TRIGGER reject_rollback BEFORE UPDATE ON podcast_items WHEN NEW.transcript_checkpoint_json='' BEGIN SELECT RAISE(ABORT,'injected failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "rollback.json")
	counts, err := PrepareMonolithRollback(db.DB, backup, true)
	if err == nil || counts.Reset != 0 {
		t.Fatal("failed transaction claimed a reset")
	}
	item := loadCharacterizationPodcastItem(t, f.item.ID)
	if item.TranscriptCheckpointJSON != string(raw) || item.TranscriptStatus != "processing" {
		t.Fatal("failed transaction discarded checkpoint")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatal("pre-change backup lost")
	}
}
