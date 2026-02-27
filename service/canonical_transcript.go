package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	canonicalTranscriptVersionCurrent = 1
	canonicalSpeakerMergeGapSeconds   = 1.0
	canonicalUnknownSpeakerLabel      = "SPEAKER_UNKNOWN"
)

var (
	canonicalWhitespaceRe             = regexp.MustCompile(`\s+`)
	canonicalSpaceBeforePunctuationRe = regexp.MustCompile(`\s+([,.;:!?])`)
)

type canonicalTranscriptSegment struct {
	Speaker  string
	Text     string
	Start    float64
	End      float64
	HasStart bool
	HasEnd   bool
}

func buildCanonicalTranscriptFromTranscriptJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return normalizeCanonicalText(trimmed)
	}

	segments := extractCanonicalSegments(payload)
	if len(segments) > 0 {
		return buildCanonicalTranscript(segments)
	}

	assets := extractTranscriptAssetContents(payload)
	if len(assets) > 0 {
		return strings.Join(assets, "\n\n")
	}
	return ""
}

func buildCanonicalTranscript(segments []canonicalTranscriptSegment) string {
	if len(segments) == 0 {
		return ""
	}
	if hasCanonicalSpeakerSegments(segments) {
		return buildCanonicalTranscriptWithSpeakers(segments)
	}
	return buildCanonicalTranscriptWithoutSpeakers(segments)
}

func hasCanonicalSpeakerSegments(segments []canonicalTranscriptSegment) bool {
	for _, segment := range segments {
		if strings.TrimSpace(segment.Speaker) != "" {
			return true
		}
	}
	return false
}

func buildCanonicalTranscriptWithoutSpeakers(segments []canonicalTranscriptSegment) string {
	lines := make([]string, 0, len(segments))
	for _, segment := range segments {
		text := normalizeCanonicalText(segment.Text)
		if text == "" {
			continue
		}
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n\n")
}

func buildCanonicalTranscriptWithSpeakers(segments []canonicalTranscriptSegment) string {
	lines := make([]string, 0, len(segments))
	currentSpeaker := ""
	currentText := ""
	currentEnd := 0.0
	currentHasEnd := false

	flushCurrent := func() {
		if currentText == "" {
			return
		}
		lines = append(lines, fmt.Sprintf("%s: %s", currentSpeaker, normalizeCanonicalText(currentText)))
		currentSpeaker = ""
		currentText = ""
		currentEnd = 0
		currentHasEnd = false
	}

	for _, segment := range segments {
		text := normalizeCanonicalText(segment.Text)
		if text == "" {
			continue
		}

		speaker := normalizeCanonicalSpeaker(segment.Speaker)
		if speaker == "" {
			speaker = canonicalUnknownSpeakerLabel
		}

		shouldMerge := currentText != "" && currentSpeaker == speaker && currentHasEnd && segment.HasStart
		if shouldMerge {
			gapSeconds := segment.Start - currentEnd
			shouldMerge = gapSeconds >= 0 && gapSeconds <= canonicalSpeakerMergeGapSeconds
		}

		if shouldMerge {
			currentText = currentText + " " + text
			if segment.HasEnd {
				currentEnd = segment.End
				currentHasEnd = true
			}
			continue
		}

		flushCurrent()
		currentSpeaker = speaker
		currentText = text
		currentEnd = segment.End
		currentHasEnd = segment.HasEnd
	}

	flushCurrent()
	return strings.Join(lines, "\n")
}

func extractCanonicalSegments(payload interface{}) []canonicalTranscriptSegment {
	switch typed := payload.(type) {
	case map[string]interface{}:
		if segments := decodeCanonicalSegments(typed["segments_pre_align"]); len(segments) > 0 {
			return segments
		}
		if segments := decodeCanonicalSegments(typed["pre_alignment_segments"]); len(segments) > 0 {
			return segments
		}
		return decodeCanonicalSegments(typed["segments"])
	case []interface{}:
		return decodeCanonicalSegments(typed)
	default:
		return nil
	}
}

func decodeCanonicalSegments(value interface{}) []canonicalTranscriptSegment {
	rows, ok := value.([]interface{})
	if !ok {
		return nil
	}

	segments := make([]canonicalTranscriptSegment, 0, len(rows))
	for _, row := range rows {
		segment, ok := decodeCanonicalSegment(row)
		if !ok {
			continue
		}
		segments = append(segments, segment)
	}
	return segments
}

func decodeCanonicalSegment(value interface{}) (canonicalTranscriptSegment, bool) {
	segmentMap, ok := value.(map[string]interface{})
	if !ok {
		return canonicalTranscriptSegment{}, false
	}

	text := normalizeCanonicalText(stringValue(segmentMap["text"]))
	if text == "" {
		return canonicalTranscriptSegment{}, false
	}

	speaker := normalizeCanonicalSpeaker(stringValue(segmentMap["speaker"]))
	if speaker == "" {
		speaker = normalizeCanonicalSpeaker(stringValue(segmentMap["speaker_id"]))
	}

	start, hasStart := parseCanonicalTime(segmentMap, "start", "start_time")
	end, hasEnd := parseCanonicalTime(segmentMap, "end", "end_time")

	return canonicalTranscriptSegment{
		Speaker:  speaker,
		Text:     text,
		Start:    start,
		End:      end,
		HasStart: hasStart,
		HasEnd:   hasEnd,
	}, true
}

func parseCanonicalTime(segment map[string]interface{}, key string, fallback string) (float64, bool) {
	if parsed := parseFloat(segment[key]); parsed >= 0 {
		return parsed, true
	}
	if fallback != "" {
		if parsed := parseFloat(segment[fallback]); parsed >= 0 {
			return parsed, true
		}
	}
	return 0, false
}

func extractTranscriptAssetContents(payload interface{}) []string {
	switch typed := payload.(type) {
	case map[string]interface{}:
		content := normalizeCanonicalText(stringValue(typed["content"]))
		if content == "" {
			return nil
		}
		return []string{content}
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, row := range typed {
			rowMap, ok := row.(map[string]interface{})
			if !ok {
				continue
			}
			content := normalizeCanonicalText(stringValue(rowMap["content"]))
			if content == "" {
				continue
			}
			values = append(values, content)
		}
		return values
	default:
		return nil
	}
}

func normalizeCanonicalSpeaker(value string) string {
	normalized := normalizeCanonicalText(value)
	if normalized == "" {
		return ""
	}
	normalized = strings.ReplaceAll(normalized, " ", "_")
	return strings.ToUpper(normalized)
}

func normalizeCanonicalText(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	normalized := canonicalWhitespaceRe.ReplaceAllString(trimmed, " ")
	normalized = canonicalSpaceBeforePunctuationRe.ReplaceAllString(normalized, "$1")
	return strings.TrimSpace(normalized)
}
