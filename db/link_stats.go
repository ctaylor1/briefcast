package db

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// Boilerplate defaults. A target is boilerplate for a podcast when the
// podcast has at least BoilerplateMinEpisodes episodes with stored links and
// the target appears in at least BoilerplatePercent percent of them (host
// profiles, "follow us" accounts, sponsor pages).
const (
	DefaultBoilerplateMinEpisodes = 5
	DefaultBoilerplatePercent     = 50
	defaultTopLinkTargetsLimit    = 50
)

// BoilerplateRule decides which (podcast, target) pairs are boilerplate.
type BoilerplateRule struct {
	MinEpisodes int
	Percent     int
}

func (r BoilerplateRule) withDefaults() BoilerplateRule {
	if r.MinEpisodes <= 0 {
		r.MinEpisodes = DefaultBoilerplateMinEpisodes
	}
	if r.Percent <= 0 || r.Percent > 100 {
		r.Percent = DefaultBoilerplatePercent
	}
	return r
}

// IsBoilerplate applies the rule to a target that appears in targetEpisodes
// of a podcast's podcastEpisodes. It mirrors the SQL in TopLinkTargets.
func (r BoilerplateRule) IsBoilerplate(targetEpisodes, podcastEpisodes int64) bool {
	r = r.withDefaults()
	return podcastEpisodes >= int64(r.MinEpisodes) && targetEpisodes*100 >= podcastEpisodes*int64(r.Percent)
}

// TopLinkTargetsQuery filters and limits a link target ranking. Empty fields
// do not filter.
type TopLinkTargetsQuery struct {
	Kind      string
	Platform  string
	PodcastID string
	// Since (inclusive) and Until (exclusive) filter by episode pub date.
	Since *time.Time
	Until *time.Time
	// IncludeBoilerplate keeps occurrences in podcasts where the target is
	// boilerplate. Boilerplate is judged over each podcast's full history,
	// not only the Since/Until window.
	IncludeBoilerplate bool
	Boilerplate        BoilerplateRule
	Limit              int
}

// LinkTargetRank is one ranked link target.
type LinkTargetRank struct {
	TargetID       string  `json:"targetId"`
	NormalizedKey  string  `json:"normalizedKey"`
	CanonicalURL   string  `json:"canonicalUrl"`
	Kind           string  `json:"kind"`
	Platform       string  `json:"platform"`
	Handle         string  `json:"handle,omitempty"`
	Domain         string  `json:"domain"`
	ParentTargetID *string `json:"parentTargetId,omitempty"`
	// EpisodeCount is the number of distinct episodes that link to the
	// target, the ranking key.
	EpisodeCount int64 `json:"episodeCount"`
	PodcastCount int64 `json:"podcastCount"`
	// BoilerplatePodcastCount is the number of counted podcasts where the
	// target is boilerplate. It is always 0 unless boilerplate is included.
	BoilerplatePodcastCount int64      `json:"boilerplatePodcastCount"`
	FirstSeen               *time.Time `json:"firstSeen,omitempty"`
	LastSeen                *time.Time `json:"lastSeen,omitempty"`
}

type linkTargetRankRow struct {
	TargetID                string
	NormalizedKey           string
	CanonicalURL            string
	Kind                    string
	Platform                string
	Handle                  string
	Domain                  string
	ParentTargetID          *string
	EpisodeCount            int64
	PodcastCount            int64
	BoilerplatePodcastCount int64
	FirstSeen               flexibleTime
	LastSeen                flexibleTime
}

// TopLinkTargets ranks link targets by the number of distinct episodes that
// link to them. Links without a target (not yet backfilled, or not
// normalizable) are not ranked.
func TopLinkTargets(q TopLinkTargetsQuery) ([]LinkTargetRank, error) {
	rule := q.Boilerplate.withDefaults()
	limit := q.Limit
	if limit <= 0 {
		limit = defaultTopLinkTargetsLimit
	}

	// Arguments are appended in placeholder order: the two CTEs, the main
	// WHERE clause, then LIMIT.
	var args []any
	episodesScope := ""
	boilerplateScope := ""
	if q.PodcastID != "" {
		episodesScope = " WHERE podcast_id = ?"
		boilerplateScope = " AND l.podcast_id = ?"
		args = append(args, q.PodcastID)
	}
	args = append(args, rule.MinEpisodes)
	if q.PodcastID != "" {
		args = append(args, q.PodcastID)
	}
	args = append(args, rule.Percent)

	conditions := []string{"1 = 1"}
	if q.PodcastID != "" {
		conditions = append(conditions, "l.podcast_id = ?")
		args = append(args, q.PodcastID)
	}
	if q.Kind != "" {
		conditions = append(conditions, "t.kind = ?")
		args = append(args, q.Kind)
	}
	if q.Platform != "" {
		conditions = append(conditions, "t.platform = ?")
		args = append(args, q.Platform)
	}
	if q.Since != nil {
		conditions = append(conditions, "i.pub_date >= ?")
		args = append(args, *q.Since)
	}
	if q.Until != nil {
		conditions = append(conditions, "i.pub_date < ?")
		args = append(args, *q.Until)
	}
	if !q.IncludeBoilerplate {
		conditions = append(conditions, "b.podcast_id IS NULL")
	}
	args = append(args, limit)

	query := `WITH podcast_episodes AS (
	SELECT podcast_id, COUNT(DISTINCT podcast_item_id) AS episodes
	FROM show_note_links` + episodesScope + `
	GROUP BY podcast_id
),
boilerplate AS (
	SELECT l.podcast_id, l.link_target_id
	FROM show_note_links l
	JOIN podcast_episodes p ON p.podcast_id = l.podcast_id
	WHERE l.link_target_id IS NOT NULL AND p.episodes >= ?` + boilerplateScope + `
	GROUP BY l.podcast_id, l.link_target_id, p.episodes
	HAVING COUNT(DISTINCT l.podcast_item_id) * 100 >= p.episodes * ?
)
SELECT t.id AS target_id, t.normalized_key, t.canonical_url, t.kind, t.platform,
	t.handle, t.domain, t.parent_target_id,
	COUNT(DISTINCT l.podcast_item_id) AS episode_count,
	COUNT(DISTINCT l.podcast_id) AS podcast_count,
	COUNT(DISTINCT b.podcast_id) AS boilerplate_podcast_count,
	MIN(i.pub_date) AS first_seen,
	MAX(i.pub_date) AS last_seen
FROM show_note_links l
JOIN link_targets t ON t.id = l.link_target_id
JOIN podcast_items i ON i.id = l.podcast_item_id
LEFT JOIN boilerplate b ON b.podcast_id = l.podcast_id AND b.link_target_id = l.link_target_id
WHERE ` + strings.Join(conditions, " AND ") + `
GROUP BY t.id, t.normalized_key, t.canonical_url, t.kind, t.platform,
	t.handle, t.domain, t.parent_target_id
ORDER BY episode_count DESC, last_seen DESC, t.normalized_key ASC
LIMIT ?`

	var rows []linkTargetRankRow
	if err := DB.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	ranks := make([]LinkTargetRank, len(rows))
	for i, row := range rows {
		ranks[i] = LinkTargetRank{
			TargetID:                row.TargetID,
			NormalizedKey:           row.NormalizedKey,
			CanonicalURL:            row.CanonicalURL,
			Kind:                    row.Kind,
			Platform:                row.Platform,
			Handle:                  row.Handle,
			Domain:                  row.Domain,
			ParentTargetID:          row.ParentTargetID,
			EpisodeCount:            row.EpisodeCount,
			PodcastCount:            row.PodcastCount,
			BoilerplatePodcastCount: row.BoilerplatePodcastCount,
			FirstSeen:               row.FirstSeen.ptr(),
			LastSeen:                row.LastSeen.ptr(),
		}
	}
	return ranks, nil
}

// GetLinkTargetByID loads one link target.
func GetLinkTargetByID(id string) (LinkTarget, error) {
	var target LinkTarget
	err := DB.Where("id = ?", id).First(&target).Error
	return target, err
}

// LinkTargetPodcastCount is how often one podcast links to a target.
type LinkTargetPodcastCount struct {
	PodcastID    string `json:"podcastId"`
	PodcastTitle string `json:"podcastTitle"`
	EpisodeCount int64  `json:"episodeCount"`
	// PodcastEpisodeCount is the number of the podcast's episodes with any
	// stored link, the denominator for boilerplate detection.
	PodcastEpisodeCount int64 `json:"podcastEpisodeCount"`
	Boilerplate         bool  `json:"boilerplate"`
}

// LinkTargetPodcastCounts returns, per podcast, the number of episodes that
// link to a target, most first.
func LinkTargetPodcastCounts(targetID string, rule BoilerplateRule) ([]LinkTargetPodcastCount, error) {
	var counts []LinkTargetPodcastCount
	err := DB.Raw(`SELECT l.podcast_id, p.title AS podcast_title,
	COUNT(DISTINCT l.podcast_item_id) AS episode_count,
	(SELECT COUNT(DISTINCT s.podcast_item_id) FROM show_note_links s WHERE s.podcast_id = l.podcast_id) AS podcast_episode_count
FROM show_note_links l
JOIN podcasts p ON p.id = l.podcast_id
WHERE l.link_target_id = ?
GROUP BY l.podcast_id, p.title
ORDER BY episode_count DESC, p.title ASC`, targetID).Scan(&counts).Error
	if err != nil {
		return nil, err
	}
	for i := range counts {
		counts[i].Boilerplate = rule.IsBoilerplate(counts[i].EpisodeCount, counts[i].PodcastEpisodeCount)
	}
	return counts, nil
}

// LinkTargetOccurrence is one episode's link to a target.
type LinkTargetOccurrence struct {
	EpisodeID    string     `json:"episodeId"`
	EpisodeTitle string     `json:"episodeTitle"`
	PubDate      *time.Time `json:"pubDate,omitempty"`
	PodcastID    string     `json:"podcastId"`
	PodcastTitle string     `json:"podcastTitle"`
	URL          string     `json:"url"`
	Title        string     `json:"title"`
	ContextLabel string     `json:"contextLabel"`
}

type linkTargetOccurrenceRow struct {
	EpisodeID    string
	EpisodeTitle string
	PubDate      flexibleTime
	PodcastID    string
	PodcastTitle string
	URL          string
	Title        string
	ContextLabel string
}

// LinkTargetOccurrences returns the newest episode links to a target.
func LinkTargetOccurrences(targetID string, limit int) ([]LinkTargetOccurrence, error) {
	var rows []linkTargetOccurrenceRow
	err := DB.Table("show_note_links AS l").
		Select("l.podcast_item_id AS episode_id, i.title AS episode_title, i.pub_date AS pub_date, "+
			"l.podcast_id AS podcast_id, p.title AS podcast_title, l.url AS url, l.title AS title, l.context_label AS context_label").
		Joins("JOIN podcast_items i ON i.id = l.podcast_item_id").
		Joins("JOIN podcasts p ON p.id = l.podcast_id").
		Where("l.link_target_id = ?", targetID).
		Order("i.pub_date DESC, l.position ASC, l.url ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	occurrences := make([]LinkTargetOccurrence, len(rows))
	for i, row := range rows {
		occurrences[i] = LinkTargetOccurrence{
			EpisodeID:    row.EpisodeID,
			EpisodeTitle: row.EpisodeTitle,
			PubDate:      row.PubDate.ptr(),
			PodcastID:    row.PodcastID,
			PodcastTitle: row.PodcastTitle,
			URL:          row.URL,
			Title:        row.Title,
			ContextLabel: row.ContextLabel,
		}
	}
	return occurrences, nil
}

// flexibleTime scans a timestamp from either driver. Postgres returns
// time.Time; SQLite returns aggregate results such as MIN(pub_date) as the
// stored text, whose layout depends on the SQLite driver.
type flexibleTime struct {
	Time  time.Time
	Valid bool
}

var sqliteTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999Z07:00",
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02",
}

// Scan implements sql.Scanner.
func (t *flexibleTime) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*t = flexibleTime{}
		return nil
	case time.Time:
		*t = flexibleTime{Time: v, Valid: true}
		return nil
	case []byte:
		return t.parse(string(v))
	case string:
		return t.parse(v)
	default:
		return fmt.Errorf("cannot scan %T into a time", src)
	}
}

func (t *flexibleTime) parse(raw string) error {
	raw = strings.TrimSpace(raw)
	// time.Time.String() appends a monotonic clock reading.
	if i := strings.Index(raw, " m="); i >= 0 {
		raw = raw[:i]
	}
	for _, layout := range sqliteTimeLayouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			*t = flexibleTime{Time: parsed, Valid: true}
			return nil
		}
	}
	return fmt.Errorf("unrecognized timestamp %q", raw)
}

// Value implements driver.Valuer; GORM requires it on struct-typed fields.
func (t flexibleTime) Value() (driver.Value, error) {
	if !t.Valid {
		return nil, nil
	}
	return t.Time, nil
}

func (t flexibleTime) ptr() *time.Time {
	if !t.Valid || t.Time.IsZero() {
		return nil
	}
	value := t.Time
	return &value
}
