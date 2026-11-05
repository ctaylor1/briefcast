package db

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ShowNoteLinkWrite is one link occurrence in an episode's show notes, with
// the canonical target its URL normalizes to.
type ShowNoteLinkWrite struct {
	Link ShowNoteLink
	// Target is nil when the URL could not be normalized; the occurrence is
	// still stored, without a target.
	Target *LinkTarget
	// Parent is the author profile of a post whose author is known.
	Parent *LinkTarget
}

var linkTargetUpsertColumns = []string{
	"canonical_url", "kind", "platform", "handle", "parent_target_id",
	"domain", "extraction_version", "updated_at",
}

var showNoteLinkUpsertColumns = []string{
	"title", "domain", "position", "link_target_id", "extraction_version", "updated_at",
}

// SaveShowNoteLinks upserts link targets and link occurrences in one
// transaction. It is idempotent: an occurrence is identified by
// (podcast_item_id, url) and a target by its normalized key, so calling it
// again for the same episode updates rows instead of duplicating them.
// Re-running it after a normalizer change re-points occurrences at their new
// targets.
func SaveShowNoteLinks(writes []ShowNoteLinkWrite) error {
	if len(writes) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		parents := make(map[string]LinkTarget)
		for _, write := range writes {
			if write.Target != nil && write.Parent != nil {
				parents[write.Parent.NormalizedKey] = *write.Parent
			}
		}
		parentIDs, err := upsertLinkTargets(tx, parents)
		if err != nil {
			return err
		}

		targets := make(map[string]LinkTarget)
		for _, write := range writes {
			if write.Target == nil {
				continue
			}
			target := *write.Target
			target.ParentTargetID = nil
			if write.Parent != nil {
				if id, ok := parentIDs[write.Parent.NormalizedKey]; ok {
					target.ParentTargetID = &id
				}
			}
			targets[target.NormalizedKey] = target
		}
		targetIDs, err := upsertLinkTargets(tx, targets)
		if err != nil {
			return err
		}

		links := make([]ShowNoteLink, 0, len(writes))
		seen := make(map[string]bool, len(writes))
		for _, write := range writes {
			link := write.Link
			occurrence := link.PodcastItemID + "\x00" + link.URL
			if seen[occurrence] {
				continue
			}
			seen[occurrence] = true
			link.ID = ""
			link.LinkTargetID = nil
			if write.Target != nil {
				if id, ok := targetIDs[write.Target.NormalizedKey]; ok {
					link.LinkTargetID = &id
				}
			}
			links = append(links, link)
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "podcast_item_id"}, {Name: "url"}},
			DoUpdates: clause.AssignmentColumns(showNoteLinkUpsertColumns),
		}).Create(&links).Error
	})
}

// upsertLinkTargets inserts or updates targets by normalized key and returns
// the stored ID for each key. IDs are read back after the upsert because a
// conflicting row keeps its existing ID.
func upsertLinkTargets(tx *gorm.DB, targets map[string]LinkTarget) (map[string]string, error) {
	ids := make(map[string]string, len(targets))
	if len(targets) == 0 {
		return ids, nil
	}
	rows := make([]LinkTarget, 0, len(targets))
	keys := make([]string, 0, len(targets))
	for key, target := range targets {
		target.ID = ""
		rows = append(rows, target)
		keys = append(keys, key)
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "normalized_key"}},
		DoUpdates: clause.AssignmentColumns(linkTargetUpsertColumns),
	}).Create(&rows).Error; err != nil {
		return nil, err
	}
	var stored []LinkTarget
	if err := tx.Select("id", "normalized_key").Where("normalized_key IN ?", keys).Find(&stored).Error; err != nil {
		return nil, err
	}
	for _, target := range stored {
		ids[target.NormalizedKey] = target.ID
	}
	return ids, nil
}

// GetLinkTargetByKey loads a link target by its normalized key.
func GetLinkTargetByKey(key string) (LinkTarget, error) {
	var target LinkTarget
	err := DB.Where("normalized_key = ?", key).First(&target).Error
	return target, err
}

// PodcastItemIDsWithShowNoteLinks returns the IDs of episodes that have at
// least one stored show note link.
func PodcastItemIDsWithShowNoteLinks() (map[string]bool, error) {
	var ids []string
	if err := DB.Model(&ShowNoteLink{}).Distinct().Pluck("podcast_item_id", &ids).Error; err != nil {
		return nil, err
	}
	return toIDSet(ids), nil
}

// PodcastItemIDsNeedingLinkExtraction returns the IDs of episodes with a
// stored link written by an extraction version older than currentVersion.
// Staleness is checked per link row because targets are shared: another
// episode re-saving a target does not fix this episode's links.
func PodcastItemIDsNeedingLinkExtraction(currentVersion int) (map[string]bool, error) {
	var ids []string
	err := DB.Model(&ShowNoteLink{}).
		Where("extraction_version < ?", currentVersion).
		Distinct().
		Pluck("podcast_item_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return toIDSet(ids), nil
}

func toIDSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}
