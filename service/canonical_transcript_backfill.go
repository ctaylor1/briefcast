package service

import (
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

const defaultCanonicalBackfillBatchSize = 500

type canonicalBackfillRow struct {
	ID             string
	TranscriptJSON string
}

type canonicalBackfillUpdate struct {
	ID                  string
	CanonicalTranscript string
}

// BackfillCanonicalTranscripts handles the corresponding operation.
func BackfillCanonicalTranscripts(batchSize int, targetVersion int) (int64, error) {
	if batchSize <= 0 {
		batchSize = defaultCanonicalBackfillBatchSize
	}
	if targetVersion <= 0 {
		targetVersion = canonicalTranscriptVersionCurrent
	}

	var (
		lastID       string
		totalUpdated int64
	)

	for {
		rows, err := loadCanonicalBackfillBatch(lastID, batchSize, targetVersion)
		if err != nil {
			return totalUpdated, err
		}
		if len(rows) == 0 {
			return totalUpdated, nil
		}

		updates := make([]canonicalBackfillUpdate, 0, len(rows))
		for _, row := range rows {
			canonical := buildCanonicalTranscriptFromTranscriptJSON(row.TranscriptJSON)
			if strings.TrimSpace(canonical) == "" {
				continue
			}
			updates = append(updates, canonicalBackfillUpdate{
				ID:                  row.ID,
				CanonicalTranscript: canonical,
			})
		}

		if len(updates) > 0 {
			updatedCount, err := applyCanonicalBackfillBatch(updates, targetVersion, time.Now().UTC())
			if err != nil {
				return totalUpdated, err
			}
			totalUpdated += updatedCount
		}

		lastID = rows[len(rows)-1].ID
	}
}

func loadCanonicalBackfillBatch(lastID string, batchSize int, targetVersion int) ([]canonicalBackfillRow, error) {
	rows := make([]canonicalBackfillRow, 0, batchSize)
	query := db.DB.Model(&db.PodcastItem{}).
		Select("id", "transcript_json").
		Where("transcript_json IS NOT NULL AND transcript_json <> ''").
		Where("(canonical_transcript IS NULL OR canonical_transcript_version IS NULL OR canonical_transcript_version < ?)", targetVersion).
		Order("id asc").
		Limit(batchSize)

	if strings.TrimSpace(lastID) != "" {
		query = query.Where("id > ?", lastID)
	}

	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func applyCanonicalBackfillBatch(updates []canonicalBackfillUpdate, targetVersion int, updatedAt time.Time) (int64, error) {
	var sqlBuilder strings.Builder
	args := make([]interface{}, 0, len(updates)*3+3)

	sqlBuilder.WriteString("UPDATE podcast_items SET canonical_transcript = CASE id ")
	for _, update := range updates {
		sqlBuilder.WriteString("WHEN ? THEN ? ")
		args = append(args, update.ID, update.CanonicalTranscript)
	}

	sqlBuilder.WriteString("ELSE canonical_transcript END, canonical_transcript_version = ?, canonical_updated_at = ? WHERE id IN (")
	args = append(args, targetVersion, updatedAt)
	for index, update := range updates {
		if index > 0 {
			sqlBuilder.WriteString(",")
		}
		sqlBuilder.WriteString("?")
		args = append(args, update.ID)
	}
	sqlBuilder.WriteString(") AND (canonical_transcript IS NULL OR canonical_transcript_version IS NULL OR canonical_transcript_version < ?)")
	args = append(args, targetVersion)

	result := db.DB.Exec(sqlBuilder.String(), args...)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
