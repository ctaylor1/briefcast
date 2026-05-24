package service

import (
	"encoding/json"
	"net/url"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/feedmeta"
)

func ParseAlternateURLs(raw string) []string {
	if raw == "" {
		return nil
	}
	var urls []string
	if err := json.Unmarshal([]byte(raw), &urls); err != nil {
		return nil
	}
	return urls
}

func MarshalAlternateURLs(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	b, err := json.Marshal(urls)
	if err != nil {
		return ""
	}
	return string(b)
}

func ResolveAlternateFileURLs(podcast *db.Podcast) error {
	altFeeds := ParseAlternateURLs(podcast.AlternateFeedURLs)
	if len(altFeeds) == 0 {
		return nil
	}

	var podcastItems []db.PodcastItem
	if err := db.GetAllPodcastItemsByPodcastID(podcast.ID, &podcastItems); err != nil {
		return err
	}
	if len(podcastItems) == 0 {
		return nil
	}

	guidToItem := make(map[string]*db.PodcastItem, len(podcastItems))
	for i := range podcastItems {
		if podcastItems[i].GUID != "" {
			guidToItem[podcastItems[i].GUID] = &podcastItems[i]
		}
	}

	for _, feedURL := range altFeeds {
		parsed, _, err := FetchFeedWithFeedparser(feedURL)
		if err != nil {
			Logger.Warnw("failed to fetch alternate feed", "url", feedURL, "podcast_id", podcast.ID, "error", err)
			continue
		}

		for _, entry := range parsed.Entries {
			guid := feedmeta.ExtractEntryGUID(entry)
			if guid == "" {
				continue
			}
			item, ok := guidToItem[guid]
			if !ok {
				continue
			}
			enclosureURL := feedmeta.ExtractEnclosureURL(entry)
			if enclosureURL == "" || enclosureURL == item.FileURL {
				continue
			}

			existing := ParseAlternateURLs(item.AlternateFileURLs)
			if containsURL(existing, enclosureURL) {
				continue
			}
			existing = append(existing, enclosureURL)
			item.AlternateFileURLs = MarshalAlternateURLs(existing)
			if err := db.UpdatePodcastItem(item); err != nil {
				Logger.Warnw("failed to save alternate file URL", "podcast_item_id", item.ID, "error", err)
			}
		}
	}

	return nil
}

func GetNextAlternateURL(item *db.PodcastItem, failedURL string) (string, bool) {
	alternates := ParseAlternateURLs(item.AlternateFileURLs)
	if len(alternates) == 0 {
		return "", false
	}

	allURLs := append([]string{item.FileURL}, alternates...)
	for i, u := range allURLs {
		if u == failedURL && i+1 < len(allURLs) {
			return allURLs[i+1], true
		}
	}
	return "", false
}

func ValidateFeedURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func containsURL(urls []string, target string) bool {
	for _, u := range urls {
		if u == target {
			return true
		}
	}
	return false
}
