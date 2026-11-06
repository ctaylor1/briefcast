package service

import (
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"gorm.io/gorm"
)

// sqliteQueryPlan returns how SQLite will run the query, without running it.
func sqliteQueryPlan(t *testing.T, query *gorm.DB) string {
	t.Helper()
	var items []db.PodcastItem
	statement := query.Session(&gorm.Session{DryRun: true}).Find(&items).Statement
	rows, err := db.DB.Raw("EXPLAIN QUERY PLAN "+statement.SQL.String(), statement.Vars...).Rows()
	if err != nil {
		t.Fatalf("explain failed: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("read plan: %v", err)
		}
		plan.WriteString(detail + "\n")
	}
	return plan.String()
}

// These two queries run about once a minute. Without their indexes they read through
// every stored transcript, which held SQLite's lock for seconds and failed other jobs.
func TestPerMinuteTranscriptQueriesUseTheirIndexes(t *testing.T) {
	setupRetentionTestDB(t)
	now := time.Now().UTC()
	cases := map[string]struct {
		query *gorm.DB
		index string
	}{
		"worker queue": {
			query: remoteQueueQuery([]string{"pending_whisperx", "processing"}, now),
			index: "idx_podcast_items_transcript_queue",
		},
		"completed receipts": {
			query: remoteReceiptQuery(now),
			index: "idx_podcast_items_open_checkpoint",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			plan := sqliteQueryPlan(t, tc.query)
			if !strings.Contains(plan, tc.index) {
				t.Fatalf("expected the plan to use %s, got:\n%s", tc.index, plan)
			}
		})
	}
}

func TestReceiptQueryFindsReceiptsAndSkipsEmptyCheckpoints(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := db.Podcast{Title: "Receipts", URL: "https://example.com/feed"}
	if err := db.DB.Create(&podcast).Error; err != nil {
		t.Fatal(err)
	}
	seed := func(guid, checkpoint string) {
		t.Helper()
		item := db.PodcastItem{
			PodcastID:                podcast.ID,
			Title:                    guid,
			GUID:                     guid,
			TranscriptStatus:         "available",
			TranscriptJSON:           "{}",
			TranscriptCheckpointJSON: checkpoint,
		}
		if err := db.DB.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}
	seed("receipt", `{"backend":"http","receipt_result_sha256":"abc"}`)
	seed("no-receipt", `{"backend":"http"}`)
	seed("no-checkpoint", "")

	var items []db.PodcastItem
	if err := remoteReceiptQuery(time.Now().UTC()).Find(&items).Error; err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].GUID != "receipt" {
		t.Fatalf("expected only the receipt row, got %d rows", len(items))
	}
}
