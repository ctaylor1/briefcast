package service

import (
	"encoding/json"
	"strings"
)

func searchTranscriptMatches(raw string, term string, limit int) []LocalSearchResult {
	results := make([]LocalSearchResult, 0, limit)
	var payload interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return results
	}
	switch typed := payload.(type) {
	case map[string]interface{}:
		if segments, ok := typed["segments"].([]interface{}); ok {
			for _, segment := range segments {
				segmentMap, ok := segment.(map[string]interface{})
				if !ok {
					continue
				}
				text := strings.TrimSpace(stringValue(segmentMap["text"]))
				if text == "" || !containsTerm(text, term) {
					continue
				}
				start := parseFloat(segmentMap["start"])
				if start < 0 {
					start = parseFloat(segmentMap["start_time"])
				}
				match := LocalSearchResult{
					TranscriptSnippet: makeSnippet(text, term, 160),
				}
				if start >= 0 {
					match.StartSeconds = &start
				}
				results = append(results, match)
				if len(results) >= limit {
					return results
				}
			}
		}
	case []interface{}:
		for _, asset := range typed {
			assetMap, ok := asset.(map[string]interface{})
			if !ok {
				continue
			}
			content := strings.TrimSpace(stringValue(assetMap["content"]))
			if content == "" || !containsTerm(content, term) {
				continue
			}
			results = append(results, LocalSearchResult{
				TranscriptSnippet: makeSnippet(content, term, 160),
			})
			if len(results) >= limit {
				return results
			}
		}
	}
	return results
}
