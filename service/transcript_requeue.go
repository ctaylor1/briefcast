package service

import (
	"errors"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
)

// ErrTranscriptNotRequeueable means the episode has no finished, acknowledged
// transcript that can be redone (not downloaded, still transcribing, or a
// worker receipt is still being reconciled).
var ErrTranscriptNotRequeueable = errors.New("episode has no finished transcript that can be redone")

// RequeueTranscription discards an episode's finished transcript so the
// WhisperX queue transcribes it again with the current settings.
//
// Only acknowledged results qualify: once the worker receipt is acknowledged
// the checkpoint is cleared, so an empty checkpoint means no worker job or
// receipt is in flight and the next run creates a fresh job identity. The
// single conditional UPDATE keeps this atomic with respect to the queue.
// The Markdown export is rewritten when the new transcript is committed.
func RequeueTranscription(id string) error {
	result := db.DB.Model(&db.PodcastItem{}).
		Where("id = ? AND deleted_at IS NULL AND download_status = ? AND download_path <> ''", id, db.Downloaded).
		Where("transcript_status = ? AND COALESCE(transcript_checkpoint_json, '') = ''", "available").
		Updates(map[string]any{
			"transcript_json":              "",
			"canonical_transcript":         "",
			"canonical_transcript_version": 0,
			"canonical_updated_at":         nil,
			"transcript_status":            "pending_whisperx",
			"transcript_progress_pct":      0,
			"transcript_progress_stage":    "",
			"transcript_retry_count":       0,
			"transcript_next_attempt":      nil,
			"transcript_last_error":        "",
			"transcript_model":             "",
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTranscriptNotRequeueable
	}
	logging.Sugar().Infow("transcription_requeued", "podcast_item_id", id)
	return nil
}
