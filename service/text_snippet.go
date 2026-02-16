package service

import "strings"

func containsTerm(value, term string) bool {
	if value == "" || term == "" {
		return false
	}
	return strings.Contains(strings.ToLower(value), term)
}

func pickSnippet(primary, fallback, term string) string {
	text := strings.TrimSpace(primary)
	if text == "" {
		text = strings.TrimSpace(fallback)
	}
	if text == "" {
		return ""
	}
	return makeSnippet(text, term, 140)
}

func makeSnippet(text, term string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 140
	}
	lower := strings.ToLower(text)
	idx := strings.Index(lower, term)
	if idx < 0 {
		if len(text) <= maxLen {
			return text
		}
		return strings.TrimSpace(text[:maxLen]) + "…"
	}
	start := idx - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
	}
	snippet := strings.TrimSpace(text[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(text) {
		snippet = snippet + "…"
	}
	return snippet
}
