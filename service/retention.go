package service

import (
	"sort"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
)

var retentionNow = func() time.Time {
	return time.Now().UTC()
}

// ApplyRetentionPolicies enforces global retention settings with this precedence:
//  1. per-podcast keep-all override
//  2. keep-latest rule (if configured)
//  3. age-based deletion rule (if configured)
//
// This ordering keeps behavior deterministic across runs and avoids conflicting
// deletes when multiple retention knobs are enabled.
func ApplyRetentionPolicies() error {
	const jobName = "RetentionCleanup"
	jobLogger, _ := logging.NewJobSugar(jobName)
	start := retentionNow()
	jobLogger.Infow("job_started")
	defer func() {
		jobLogger.Infow("job_finished", "duration_ms", retentionNow().Sub(start).Milliseconds())
	}()

	lock := db.GetLock(jobName)
	if lock.IsLocked() {
		jobLogger.Infow("job_skipped_lock_exists")
		return nil
	}
	db.Lock(jobName, 120)
	defer db.Unlock(jobName)

	setting := db.GetOrCreateSetting()
	if setting.RetentionKeepAll {
		jobLogger.Infow("retention_skipped", "reason", "keep_all_enabled")
		return nil
	}
	if setting.RetentionKeepLatest <= 0 && setting.RetentionDeleteAfterDays <= 0 {
		jobLogger.Infow("retention_skipped", "reason", "no_rules_configured")
		return nil
	}

	var podcasts []db.Podcast
	if err := db.FindAllPodcastsPlain(&podcasts); err != nil {
		jobLogger.Errorw("failed_to_fetch_podcasts", "error", err)
		return err
	}

	var items []db.PodcastItem
	if err := db.FindDownloadedPodcastItems(&items); err != nil {
		jobLogger.Errorw("failed_to_fetch_downloaded_items", "error", err)
		return err
	}
	if len(items) == 0 {
		jobLogger.Infow("no_downloaded_items")
		return nil
	}

	itemsByPodcast := make(map[string][]db.PodcastItem, len(podcasts))
	for _, item := range items {
		itemsByPodcast[item.PodcastID] = append(itemsByPodcast[item.PodcastID], item)
	}

	now := retentionNow()
	keptCount := 0
	deletedCount := 0
	skippedCount := 0

	for _, podcast := range podcasts {
		if podcast.RetentionKeepAll {
			skippedCount += len(itemsByPodcast[podcast.ID])
			continue
		}

		podcastItems := itemsByPodcast[podcast.ID]
		if len(podcastItems) == 0 {
			continue
		}

		sort.Slice(podcastItems, func(i, j int) bool {
			return retentionReferenceTime(podcastItems[i]).After(retentionReferenceTime(podcastItems[j]))
		})

		protected := make(map[string]struct{})
		if setting.RetentionKeepLatest > 0 {
			limit := setting.RetentionKeepLatest
			if limit > len(podcastItems) {
				limit = len(podcastItems)
			}
			for i := 0; i < limit; i++ {
				protected[podcastItems[i].ID] = struct{}{}
				keptCount++
			}
		}

		for i := 0; i < len(podcastItems); i++ {
			item := podcastItems[i]
			if _, ok := protected[item.ID]; ok {
				continue
			}

			shouldDelete := false
			if setting.RetentionKeepLatest > 0 {
				shouldDelete = true
			} else if setting.RetentionDeleteAfterDays > 0 {
				ref := retentionReferenceTime(item)
				if !ref.IsZero() {
					cutoff := now.Add(-time.Duration(setting.RetentionDeleteAfterDays) * 24 * time.Hour)
					if ref.Before(cutoff) {
						if !setting.RetentionDeleteOnlyPlayed || item.IsPlayed {
							shouldDelete = true
						}
					}
				}
			}

			if !shouldDelete {
				keptCount++
				continue
			}

			if err := DeleteEpisodeFile(item.ID); err != nil {
				jobLogger.Warnw("retention_delete_failed", "podcast_item_id", item.ID, "error", err)
				skippedCount++
				continue
			}
			deletedCount++
		}
	}

	jobLogger.Infow("retention_completed", "deleted", deletedCount, "kept", keptCount, "skipped", skippedCount)
	return nil
}

func retentionReferenceTime(item db.PodcastItem) time.Time {
	if !item.PubDate.IsZero() {
		return item.PubDate.UTC()
	}
	if !item.DownloadDate.IsZero() {
		return item.DownloadDate.UTC()
	}
	return item.CreatedAt.UTC()
}
