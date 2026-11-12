package service

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"gorm.io/gorm"
)

// The work-queue snapshot was rewritten so that no query reads a stored
// transcript (48 s on a production copy before, about 0.1 s after). These tests
// keep the old queries as the reference and check that the new ones agree on
// every combination of the states they test.

const legacyNoTranscript = "(transcript_json IS NULL OR transcript_json = '')"

func legacyCount(t *testing.T, where string, args ...any) int {
	t.Helper()
	var n int64
	if err := db.DB.Model(&db.PodcastItem{}).Where(where, args...).Count(&n).Error; err != nil {
		t.Fatalf("legacy count %q: %v", where, err)
	}
	return int(n)
}

func legacyTranscriptCounts(t *testing.T, now time.Time) WorkQueueTranscriptCounts {
	work := []string{"pending_whisperx", "processing", "failed"}
	return WorkQueueTranscriptCounts{
		Complete:       legacyCount(t, "transcript_status = ? OR (transcript_json IS NOT NULL AND transcript_json <> '')", "available"),
		Queued:         legacyCount(t, legacyNoTranscript+" AND transcript_status = ?", "pending_whisperx"),
		Processing:     legacyCount(t, legacyNoTranscript+" AND transcript_status = ?", "processing"),
		Failed:         legacyCount(t, legacyNoTranscript+" AND transcript_status = ?", "failed"),
		RetryDue:       legacyCount(t, "download_status = ? AND download_path <> '' AND "+legacyNoTranscript+" AND transcript_status = ? AND (transcript_next_attempt IS NULL OR transcript_next_attempt <= ?)", db.Downloaded, "failed", now.UTC()),
		RetryScheduled: legacyCount(t, legacyNoTranscript+" AND transcript_status = ? AND transcript_next_attempt > ?", "failed", now.UTC()),
		Blocked:        legacyCount(t, legacyNoTranscript+" AND transcript_status IN ? AND (download_status <> ? OR download_path IS NULL OR download_path = '')", work, db.Downloaded),
	}
}

func legacySummaryCounts(t *testing.T) WorkQueueSummaryCounts {
	candidates := "canonical_transcript IS NOT NULL AND canonical_transcript <> '' AND (llm_summary IS NULL OR llm_summary = '')"
	return WorkQueueSummaryCounts{
		Complete:            legacyCount(t, "llm_summary_status = ? OR (llm_summary IS NOT NULL AND llm_summary <> '')", "available"),
		Processing:          legacyCount(t, "llm_summary_status = ?", "processing"),
		Failed:              legacyCount(t, "llm_summary_status = ?", "failed"),
		Missing:             legacyCount(t, candidates+" AND (llm_summary_status IS NULL OR llm_summary_status = '')"),
		EligibleForBackfill: legacyCount(t, candidates+" AND (llm_summary_status IS NULL OR llm_summary_status = '' OR llm_summary_status = 'failed' OR (llm_summary_status = 'processing' AND updated_at < ?))", summaryProcessingStaleBefore(LoadLLMConfig(), time.Now())),
		BlockedNoTranscript: legacyCount(t, "(llm_summary IS NULL OR llm_summary = '') AND (canonical_transcript IS NULL OR canonical_transcript = '')"),
	}
}

// legacyBuildWorkQueueItems is the list query before the rewrite, unchanged.
func legacyBuildWorkQueueItems(limit int) ([]WorkQueueItem, error) {
	queryLimit := max(limit*4, 100)
	var items []db.PodcastItem
	result := db.DB.Preload("Podcast").
		Where(
			db.DB.Where("transcript_status IN ?", []string{"pending_whisperx", "processing", "failed"}).
				Where("(transcript_json IS NULL OR transcript_json = '')"),
		).
		Or(
			db.DB.Where("canonical_transcript IS NOT NULL AND canonical_transcript <> ''").
				Where("(llm_summary IS NULL OR llm_summary = '' OR llm_summary_status IN ?)", []string{"processing", "failed"}).
				Where("(llm_summary_status IS NULL OR llm_summary_status = '' OR llm_summary_status IN ?)", []string{"processing", "failed"}),
		).
		Order("updated_at DESC").
		Limit(queryLimit).
		Find(&items)
	if result.Error != nil {
		return nil, result.Error
	}
	legacySummaryNeedsWork := func(item db.PodcastItem) bool {
		if strings.TrimSpace(item.CanonicalTranscript) == "" {
			return false
		}
		status := normalizedStatus(item.LLMSummaryStatus, "")
		if status == "processing" || status == "failed" {
			return true
		}
		return strings.TrimSpace(item.LLMSummary) == "" && status == ""
	}
	queued := make([]queuedWorkItem, 0, len(items)*2)
	now := time.Now().UTC()
	for _, item := range items {
		if transcriptNeedsWork(item) {
			queued = append(queued, queuedWorkItem{item: buildTranscriptWorkQueueItem(item, now), priority: transcriptWorkPriority(item, now)})
		}
		if legacySummaryNeedsWork(item) {
			queued = append(queued, queuedWorkItem{item: buildSummaryWorkQueueItem(item), priority: summaryWorkPriority(item)})
		}
	}
	sort.SliceStable(queued, func(i, j int) bool {
		if queued[i].priority != queued[j].priority {
			return queued[i].priority < queued[j].priority
		}
		if !queued[i].item.UpdatedAt.Equal(queued[j].item.UpdatedAt) {
			return queued[i].item.UpdatedAt.After(queued[j].item.UpdatedAt)
		}
		return queued[i].item.Title < queued[j].item.Title
	})
	out := make([]WorkQueueItem, 0, min(len(queued), limit))
	for i, entry := range queued {
		if i >= limit {
			break
		}
		out = append(out, entry.item)
	}
	return out, nil
}

// seedWorkQueueMatrix stores one episode for every combination of the states the
// snapshot tests, with distinct update times so that list order is defined.
// Whitespace-only text is left out: the old list treated it as empty while the
// old counts did not, and the new list follows the counts.
func seedWorkQueueMatrix(t *testing.T, now time.Time) {
	t.Helper()
	podcast := createPodcast(t, "work-queue-matrix", false)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	texts := []string{"null", "", "present"}
	downloads := []struct {
		status db.DownloadStatus
		path   string
	}{{db.Downloaded, "/a.mp3"}, {db.Downloaded, ""}, {db.Downloaded, "null"}, {db.NotDownloaded, "/a.mp3"}}
	attempts := []*time.Time{nil, &past, &future}

	var items []db.PodcastItem
	nulls := map[string][]int{}
	value := func(column string, choice, present string, index int) string {
		switch choice {
		case "null":
			nulls[column] = append(nulls[column], index)
			return ""
		case "present":
			return present
		}
		return choice
	}
	for _, transcriptJSON := range texts {
		for _, transcriptStatus := range []string{"available", "pending_whisperx", "processing", "failed"} {
			for _, download := range downloads {
				for _, attempt := range attempts {
					for _, canonical := range texts {
						for _, summary := range texts {
							for _, summaryStatus := range []string{"null", "", "available", "processing", "failed"} {
								i := len(items)
								updated := now.Add(time.Duration(i) * time.Millisecond)
								if i%2 == 0 {
									updated = now.Add(-30*24*time.Hour + time.Duration(i)*time.Millisecond)
								}
								item := db.PodcastItem{
									PodcastID:               podcast.ID,
									Title:                   fmt.Sprintf("episode %05d", i),
									PubDate:                 now.Add(-time.Duration(i) * time.Minute),
									DownloadStatus:          download.status,
									DownloadPath:            value("download_path", download.path, "", i),
									TranscriptJSON:          value("transcript_json", transcriptJSON, `{"segments":[]}`, i),
									TranscriptStatus:        transcriptStatus,
									TranscriptNextAttempt:   attempt,
									TranscriptProgressPct:   i % 100,
									TranscriptProgressStage: fmt.Sprintf("stage %d", i%7),
									TranscriptRetryCount:    i % 4,
									TranscriptLastError:     fmt.Sprintf("transcript error %d", i%5),
									TranscriptModel:         "medium.en",
									CanonicalTranscript:     value("canonical_transcript", canonical, "words", i),
									LLMSummary:              value("llm_summary", summary, "a summary", i),
									LLMSummaryStatus:        value("llm_summary_status", summaryStatus, "", i),
									LLMSummaryError:         fmt.Sprintf("summary error %d", i%3),
									LLMSummaryModel:         fmt.Sprintf("model %d", i%2),
								}
								item.UpdatedAt = updated
								item.CreatedAt = updated
								items = append(items, item)
							}
						}
					}
				}
			}
		}
	}
	if err := db.DB.CreateInBatches(&items, 500).Error; err != nil {
		t.Fatalf("seed work queue matrix: %v", err)
	}
	for column, indexes := range nulls {
		ids := make([]string, 0, len(indexes))
		for _, i := range indexes {
			ids = append(ids, items[i].ID)
		}
		for start := 0; start < len(ids); start += 500 {
			end := min(start+500, len(ids))
			if err := db.DB.Exec("UPDATE podcast_items SET "+column+" = NULL WHERE id IN ?", ids[start:end]).Error; err != nil {
				t.Fatalf("set %s to NULL: %v", column, err)
			}
		}
	}
}

func renderWorkQueueItems(items []WorkQueueItem) string {
	var out strings.Builder
	for _, it := range items {
		next := "-"
		if it.NextAttempt != nil {
			next = it.NextAttempt.UTC().Format(time.RFC3339Nano)
		}
		fmt.Fprintf(&out, "%s|%s|%s|%s|%s|%s|%s|%s|%s|%d|%s|%d|%s|%s|%s\n", it.Kind, it.ID, it.Status, it.StatusLabel,
			it.Category, it.Title, it.PodcastTitle, it.PubDate.UTC().Format(time.RFC3339Nano), it.UpdatedAt.UTC().Format(time.RFC3339Nano),
			it.ProgressPct, it.ProgressStage, it.RetryCount, next, it.LastError, it.Model)
	}
	return out.String()
}

func TestWorkQueueSnapshotMatchesTheOldQueries(t *testing.T) {
	setupRetentionTestDB(t)
	now := time.Now().UTC()
	seedWorkQueueMatrix(t, now)

	transcripts, err := buildTranscriptQueueCounts(now)
	if err != nil {
		t.Fatal(err)
	}
	if want := legacyTranscriptCounts(t, now); transcripts != want {
		t.Fatalf("transcript counts differ:\n got %+v\nwant %+v", transcripts, want)
	}
	summaries, err := buildSummaryQueueCounts()
	if err != nil {
		t.Fatal(err)
	}
	if want := legacySummaryCounts(t); summaries != want {
		t.Fatalf("summary counts differ:\n got %+v\nwant %+v", summaries, want)
	}
	for _, limit := range []int{5, 50, 200} {
		got, err := buildWorkQueueItems(limit)
		if err != nil {
			t.Fatal(err)
		}
		want, err := legacyBuildWorkQueueItems(limit)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != limit {
			t.Fatalf("limit %d: expected a full list from the matrix, got %d items", limit, len(got))
		}
		if g, w := renderWorkQueueItems(got), renderWorkQueueItems(want); g != w {
			t.Fatalf("limit %d: items differ.\n got:\n%s\nwant:\n%s", limit, g, w)
		}
	}
}

// Every statement the snapshot runs against podcast_items must use an index, and
// each one that tests a stored transcript or summary must use the index built
// for that test. A plan that tests those columns some other way reads every
// stored transcript; that is how the snapshot came to take 48 s.
func TestWorkQueueSnapshotQueriesUseTheirIndexes(t *testing.T) {
	setupRetentionTestDB(t)
	if db.DB.Dialector.Name() != "sqlite" {
		t.Skip("query plans are SQLite-specific")
	}
	podcast := createPodcast(t, "plans", false)
	for i, item := range []db.PodcastItem{
		{TranscriptStatus: "pending_whisperx", DownloadStatus: db.Downloaded, DownloadPath: "/a.mp3"},
		{TranscriptStatus: "available", TranscriptJSON: "{}", CanonicalTranscript: "words"},
		{TranscriptStatus: "available", TranscriptJSON: "{}", CanonicalTranscript: "words", LLMSummaryStatus: "failed"},
	} {
		item.PodcastID = podcast.ID
		item.Title = fmt.Sprintf("plan %d", i)
		if err := db.DB.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}

	type statement struct {
		sql  string
		vars []any
	}
	var statements []statement
	const callback = "test:capture_work_queue_sql"
	if err := db.DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		statements = append(statements, statement{tx.Statement.SQL.String(), append([]any(nil), tx.Statement.Vars...)})
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.DB.Callback().Query().Remove(callback) })
	if _, err := BuildWorkQueueSnapshot(50); err != nil {
		t.Fatal(err)
	}

	// First match wins. The summary-retry query finds its few rows by status and
	// may read their transcript; the retry-due count has an exact download
	// status, which narrows the existing queue index to a handful of rows.
	rules := []struct {
		contains []string
		indexes  string
	}{
		{[]string{"canonical_transcript", "llm_summary IS NULL"}, "idx_podcast_items_summary_missing"},
		{[]string{"canonical_transcript", "llm_summary_status IN"}, "idx_podcast_items_summary_list"},
		{[]string{"llm_summary IS NOT NULL"}, "idx_podcast_items_with_summary"},
		{[]string{"transcript_json", "download_status = "}, "idx_podcast_items_no_transcript_work idx_podcast_items_transcript_queue"},
		{[]string{"transcript_json"}, "idx_podcast_items_no_transcript_work"},
		{[]string{"canonical_transcript"}, "(no other canonical_transcript test is expected)"},
	}
	checked := 0
	for _, s := range statements {
		if !strings.Contains(s.sql, "podcast_items") {
			continue
		}
		checked++
		rows, err := db.DB.Raw("EXPLAIN QUERY PLAN "+s.sql, s.vars...).Rows()
		if err != nil {
			t.Fatalf("explain %s: %v", s.sql, err)
		}
		var plan strings.Builder
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(detail + "\n")
			if strings.Contains(detail, "podcast_items") && strings.HasPrefix(detail, "SCAN") && !strings.Contains(detail, "USING") {
				t.Errorf("table scan:\n%s\nplan: %s", s.sql, detail)
			}
		}
		rows.Close()
		for _, rule := range rules {
			matches := true
			for _, fragment := range rule.contains {
				matches = matches && strings.Contains(s.sql, fragment)
			}
			if !matches {
				continue
			}
			used := false
			for _, index := range strings.Fields(rule.indexes) {
				used = used || strings.Contains(plan.String(), index)
			}
			if !used {
				t.Errorf("statement matching %q should use %s:\n%s\nplan:\n%s", rule.contains, rule.indexes, s.sql, plan.String())
			}
			break
		}
	}
	if checked < 15 {
		t.Fatalf("expected the snapshot's podcast_items statements to be captured, got %d", checked)
	}
}
