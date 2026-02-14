package feedmeta

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type TranscriptAsset struct {
	URL      string `json:"url"`
	Type     string `json:"type,omitempty"`
	Language string `json:"language,omitempty"`
	Rel      string `json:"rel,omitempty"`
	Content  string `json:"content,omitempty"`
}

func MarshalMetadata(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func GetString(mapData map[string]interface{}, key string) string {
	if mapData == nil {
		return ""
	}
	value, ok := mapData[key]
	if !ok {
		return ""
	}
	return stringValue(value)
}

func GetNestedString(mapData map[string]interface{}, keys ...string) string {
	current := mapData
	for i := 0; i < len(keys); i++ {
		key := keys[i]
		if current == nil {
			return ""
		}
		value, ok := current[key]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			return stringValue(value)
		}
		next, ok := value.(map[string]interface{})
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

func PickFirstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func PickLongest(values ...string) string {
	best := ""
	for _, value := range values {
		value = strings.TrimSpace(value)
		if len(value) > len(best) {
			best = value
		}
	}
	return best
}

func CollectContentValues(entry map[string]interface{}) []string {
	raw, ok := entry["content"]
	if !ok {
		return nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	values := make([]string, 0, len(list))
	for _, item := range list {
		if itemMap, ok := item.(map[string]interface{}); ok {
			if value := stringValue(itemMap["value"]); value != "" {
				values = append(values, value)
			}
		}
	}
	return values
}

func CollectDetailValue(entry map[string]interface{}, key string) string {
	value, ok := entry[key]
	if !ok {
		return ""
	}
	if valueMap, ok := value.(map[string]interface{}); ok {
		return stringValue(valueMap["value"])
	}
	return stringValue(value)
}

func ExtractEntryShowNotesHTML(entry map[string]interface{}) string {
	candidates := []string{
		CollectDetailValue(entry, "summary_detail"),
		CollectDetailValue(entry, "description_detail"),
		GetString(entry, "summary"),
		GetString(entry, "description"),
		GetString(entry, "itunes_summary"),
		GetString(entry, "subtitle"),
	}
	candidates = append(candidates, CollectContentValues(entry)...)
	return PickLongest(candidates...)
}

func ExtractFeedShowNotesHTML(feed map[string]interface{}) string {
	candidates := []string{
		CollectDetailValue(feed, "summary_detail"),
		CollectDetailValue(feed, "description_detail"),
		GetString(feed, "summary"),
		GetString(feed, "description"),
		GetString(feed, "itunes_summary"),
		GetString(feed, "subtitle"),
	}
	return PickLongest(candidates...)
}

func ExtractImageURL(mapData map[string]interface{}) string {
	return PickFirstNonEmpty(
		GetNestedString(mapData, "image", "href"),
		GetNestedString(mapData, "image", "url"),
		GetNestedString(mapData, "itunes_image", "href"),
		GetString(mapData, "itunes_image"),
	)
}

func ExtractEntryImage(entry map[string]interface{}, fallback string) string {
	image := ExtractImageURL(entry)
	if image != "" {
		return image
	}
	return fallback
}

func ExtractEnclosureURL(entry map[string]interface{}) string {
	if raw, ok := entry["enclosures"]; ok {
		if list, ok := raw.([]interface{}); ok {
			for _, item := range list {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if href := stringValue(itemMap["href"]); href != "" {
						return href
					}
					if href := stringValue(itemMap["url"]); href != "" {
						return href
					}
				}
			}
		}
	}
	if raw, ok := entry["links"]; ok {
		if list, ok := raw.([]interface{}); ok {
			for _, item := range list {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if strings.EqualFold(stringValue(itemMap["rel"]), "enclosure") {
					if href := stringValue(itemMap["href"]); href != "" {
						return href
					}
				}
			}
		}
	}
	return GetString(entry, "link")
}

func ExtractEntryGUID(entry map[string]interface{}) string {
	return PickFirstNonEmpty(
		GetString(entry, "id"),
		GetString(entry, "guid"),
		GetString(entry, "link"),
	)
}

func ParseEntryDate(entry map[string]interface{}) time.Time {
	candidates := []string{
		GetString(entry, "published"),
		GetString(entry, "updated"),
		GetString(entry, "created"),
		GetString(entry, "published_parsed"),
		GetString(entry, "updated_parsed"),
	}
	return ParseFeedTime(candidates...)
}

func ParseFeedTime(values ...string) time.Time {
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, value); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func ParseDurationSeconds(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if !strings.Contains(raw, ":") {
		if seconds, err := strconv.Atoi(raw); err == nil {
			return seconds
		}
		return 0
	}

	parts := strings.Split(raw, ":")
	if len(parts) > 3 {
		return 0
	}
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return 0
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return 0
		}
		values = append(values, value)
	}

	if len(values) == 2 {
		return values[0]*60 + values[1]
	}
	if len(values) == 3 {
		return values[0]*3600 + values[1]*60 + values[2]
	}
	return 0
}

func ExtractPodcastChapters(entry map[string]interface{}) (string, string) {
	keys := []string{"podcast_chapters", "chapters", "psc_chapters"}
	for _, key := range keys {
		url, typ := extractURLAndType(entry[key])
		if url != "" {
			return url, typ
		}
	}
	return "", ""
}

func ExtractTranscripts(entry map[string]interface{}) []TranscriptAsset {
	keys := []string{"podcast_transcript", "transcript", "transcripts"}
	assets := make([]TranscriptAsset, 0)
	seen := make(map[string]struct{})
	for _, key := range keys {
		for _, asset := range extractTranscriptAssets(entry[key]) {
			if asset.URL == "" {
				continue
			}
			if _, exists := seen[asset.URL]; exists {
				continue
			}
			seen[asset.URL] = struct{}{}
			assets = append(assets, asset)
		}
	}
	return assets
}

func extractTranscriptAssets(value interface{}) []TranscriptAsset {
	assets := make([]TranscriptAsset, 0)
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			assets = append(assets, extractTranscriptAssets(item)...)
		}
	case map[string]interface{}:
		url, _ := extractURLAndType(typed)
		if url == "" {
			url = PickFirstNonEmpty(
				GetString(typed, "url"),
				GetString(typed, "href"),
				GetString(typed, "src"),
			)
		}
		asset := TranscriptAsset{
			URL:      url,
			Type:     PickFirstNonEmpty(GetString(typed, "type"), GetString(typed, "mime_type")),
			Language: PickFirstNonEmpty(GetString(typed, "language"), GetString(typed, "lang")),
			Rel:      GetString(typed, "rel"),
		}
		if asset.URL != "" {
			assets = append(assets, asset)
		}
	case string:
		if strings.TrimSpace(typed) != "" {
			assets = append(assets, TranscriptAsset{URL: strings.TrimSpace(typed)})
		}
	}
	return assets
}

func extractURLAndType(value interface{}) (string, string) {
	switch typed := value.(type) {
	case map[string]interface{}:
		url := PickFirstNonEmpty(
			GetString(typed, "url"),
			GetString(typed, "href"),
			GetString(typed, "src"),
		)
		typ := PickFirstNonEmpty(GetString(typed, "type"), GetString(typed, "mime_type"))
		return url, typ
	case []interface{}:
		for _, item := range typed {
			url, typ := extractURLAndType(item)
			if url != "" {
				return url, typ
			}
		}
	case string:
		if strings.TrimSpace(typed) != "" {
			return strings.TrimSpace(typed), ""
		}
	}
	return "", ""
}

func stringValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case float64:
		return fmt.Sprintf("%v", typed)
	default:
		return ""
	}
}
