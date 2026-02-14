package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ctaylor1/briefcast/db"
)

type Chapter struct {
	Title        string  `json:"title"`
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds,omitempty"`
}

type ChapterResponse struct {
	Source   string    `json:"source"`
	Chapters []Chapter `json:"chapters"`
}

func BuildChapterResponse(item db.PodcastItem) ChapterResponse {
	raw := strings.TrimSpace(item.ChaptersJSON)
	source := strings.TrimSpace(item.ChaptersType)
	if raw == "" && strings.TrimSpace(item.ID3ChaptersJSON) != "" {
		raw = item.ID3ChaptersJSON
		if source == "" {
			source = "id3"
		}
	}
	if raw == "" {
		return ChapterResponse{Source: "none", Chapters: []Chapter{}}
	}
	chapters := parseChapters(raw)
	if source == "" {
		source = "feed"
	}
	return ChapterResponse{Source: source, Chapters: chapters}
}

func parseChapters(raw string) []Chapter {
	var payload interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	switch typed := payload.(type) {
	case map[string]interface{}:
		if chapters, ok := typed["chapters"]; ok {
			return parseChapterList(chapters)
		}
		return parseChapterList(typed)
	case []interface{}:
		return parseChapterList(typed)
	default:
		return nil
	}
}

func parseChapterList(value interface{}) []Chapter {
	list, ok := value.([]interface{})
	if !ok {
		return nil
	}
	chapters := make([]Chapter, 0, len(list))
	for i, item := range list {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		title := pickString(entry, "title", "name", "text", "label", "summary", "description", "caption")
		start := readTime(entry, false, "startTime", "start_time", "start")
		if start < 0 {
			start = readTime(entry, true, "start_time_ms", "startTimeMs", "startTimeMS", "start_ms")
		}
		if start < 0 {
			continue
		}
		end := readTime(entry, false, "endTime", "end_time", "end")
		if end < 0 {
			end = readTime(entry, true, "end_time_ms", "endTimeMs", "endTimeMS", "end_ms")
		}
		if title == "" {
			title = "Chapter " + strconv.Itoa(i+1)
		}
		chapter := Chapter{
			Title:        title,
			StartSeconds: start,
		}
		if end > 0 {
			chapter.EndSeconds = end
		}
		chapters = append(chapters, chapter)
	}
	sort.SliceStable(chapters, func(i, j int) bool {
		return chapters[i].StartSeconds < chapters[j].StartSeconds
	})
	return chapters
}

func pickString(entry map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := entry[key]
		if !ok {
			continue
		}
		if s := stringValue(value); s != "" {
			return s
		}
		if nested, ok := value.(map[string]interface{}); ok {
			if s := stringValue(nested["value"]); s != "" {
				return s
			}
			if s := stringValue(nested["text"]); s != "" {
				return s
			}
		}
		if list, ok := value.([]interface{}); ok && len(list) > 0 {
			if s := stringValue(list[0]); s != "" {
				return s
			}
		}
	}
	return ""
}

func readTime(entry map[string]interface{}, milliseconds bool, keys ...string) float64 {
	for _, key := range keys {
		value, ok := entry[key]
		if !ok {
			continue
		}
		if seconds := parseTimeValue(value, milliseconds); seconds >= 0 {
			return seconds
		}
	}
	return -1
}

func parseTimeValue(value interface{}, milliseconds bool) float64 {
	switch typed := value.(type) {
	case float64:
		if milliseconds {
			return typed / 1000
		}
		return typed
	case string:
		return parseTimeString(typed, milliseconds)
	case map[string]interface{}:
		if s := stringValue(typed["value"]); s != "" {
			return parseTimeString(s, milliseconds)
		}
		if s := stringValue(typed["text"]); s != "" {
			return parseTimeString(s, milliseconds)
		}
		return -1
	case []interface{}:
		if len(typed) == 0 {
			return -1
		}
		return parseTimeValue(typed[0], milliseconds)
	default:
		return -1
	}
}

func parseTimeString(raw string, milliseconds bool) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return -1
	}
	if !strings.Contains(raw, ":") {
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			if milliseconds {
				return f / 1000
			}
			return f
		}
		return -1
	}
	parts := strings.Split(raw, ":")
	if len(parts) > 3 {
		return -1
	}
	values := make([]float64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return -1
		}
		f, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return -1
		}
		values = append(values, f)
	}
	if len(values) == 2 {
		return values[0]*60 + values[1]
	}
	if len(values) == 3 {
		return values[0]*3600 + values[1]*60 + values[2]
	}
	return -1
}

func stringValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case map[string]interface{}:
		if s := stringValue(typed["value"]); s != "" {
			return s
		}
		if s := stringValue(typed["text"]); s != "" {
			return s
		}
		return ""
	default:
		return ""
	}
}
