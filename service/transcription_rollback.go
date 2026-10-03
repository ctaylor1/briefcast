package service

// Offline monolith rollback tooling deliberately shares the HTTP envelope
// decoder. It must never start the scheduler, unlock leases or contact a worker.
import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"gorm.io/gorm"
)

type RemoteRollbackCounts struct {
	Selected int `json:"selected"`
	Reset    int `json:"reset"`
}

type rollbackRow struct {
	ID                       string
	TranscriptStatus         string
	TranscriptCheckpointJSON string
	TranscriptProgressPct    int
	TranscriptProgressStage  string
	TranscriptRetryCount     int
	TranscriptNextAttempt    *time.Time
	TranscriptLastError      string
}

// PrepareMonolithRollback is an operator-only operation. Apply requires both
// split services stopped by the caller, and a new private backup filename.
// Completed outputs and legacy checkpoints are excluded. Unknown remote
// formats abort the entire transaction instead of broad queue resets.
func PrepareMonolithRollback(handle *gorm.DB, backupPath string, apply bool) (RemoteRollbackCounts, error) {
	var counts RemoteRollbackCounts
	err := handle.Transaction(func(tx *gorm.DB) error {
		var rows []rollbackRow
		if err := tx.Model(&db.PodcastItem{}).Select("id, transcript_status, transcript_checkpoint_json, transcript_progress_pct, transcript_progress_stage, transcript_retry_count, transcript_next_attempt, transcript_last_error").Where("deleted_at IS NULL AND transcript_status IN ? AND COALESCE(transcript_json, '') = '' AND COALESCE(canonical_transcript, '') = '' AND COALESCE(transcript_checkpoint_json, '') <> ''", []string{"pending_whisperx", "processing", "failed"}).Order("id").Scan(&rows).Error; err != nil {
			return errors.New("rollback inventory query failed")
		}
		selected := make([]rollbackRow, 0)
		for _, row := range rows {
			var marker map[string]json.RawMessage
			if json.Unmarshal([]byte(row.TranscriptCheckpointJSON), &marker) != nil || marker == nil {
				return errors.New("rollback inventory contains corrupt checkpoint")
			}
			if marker["backend"] == nil && marker["version"] == nil {
				continue
			}
			envelope, err := decodeRemoteEnvelope(row.TranscriptCheckpointJSON)
			if err != nil || envelope.EpisodeID != row.ID || envelope.ReceiptResultSHA256 != nil {
				return errors.New("rollback inventory contains unknown or committed remote checkpoint")
			}
			selected = append(selected, row)
		}
		counts.Selected = len(selected)
		if !apply {
			return nil
		}
		if strings.TrimSpace(backupPath) == "" {
			return errors.New("private rollback backup filename required")
		}
		backup, err := json.MarshalIndent(struct {
			Version int
			Rows    []rollbackRow
		}{1, selected}, "", "  ")
		if err != nil {
			return err
		}
		file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return errors.New("cannot create new private rollback backup")
		}
		_, writeErr := file.Write(backup)
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			return errors.New("rollback backup persistence failed")
		}
		for _, row := range selected {
			result := tx.Model(&db.PodcastItem{}).Where("id = ? AND deleted_at IS NULL AND transcript_status = ? AND transcript_checkpoint_json = ? AND COALESCE(transcript_json, '') = '' AND COALESCE(canonical_transcript, '') = ''", row.ID, row.TranscriptStatus, row.TranscriptCheckpointJSON).Updates(map[string]any{
				"transcript_status": "pending_whisperx", "transcript_checkpoint_json": "", "transcript_progress_pct": 0,
				"transcript_progress_stage": "", "transcript_retry_count": 0, "transcript_next_attempt": nil, "transcript_last_error": "",
			})
			if result.Error != nil || result.RowsAffected != 1 {
				return errors.New("rollback row changed; no reset committed")
			}
		}
		return nil
	})
	if err == nil && apply {
		counts.Reset = counts.Selected
	}
	return counts, err
}
