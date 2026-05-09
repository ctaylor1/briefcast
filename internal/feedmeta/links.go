package feedmeta

import (
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// ShowNoteLinkData holds a link extracted from show notes.
type ShowNoteLinkData struct {
	URL      string
	Title    string
	Domain   string
	Position int
}

var bareURLRe = regexp.MustCompile(`https?://[^\s<>"` + "`" + `\)\]\}]+`)

var noiseExtensions = map[string]bool{
	".mp3": true, ".m4a": true, ".ogg": true, ".wav": true, ".flac": true,
	".mp4": true, ".m4v": true, ".mov": true, ".avi": true,
}

var noiseDomainPrefixes = []string{
	"itunes.apple.com",
	"podcasts.apple.com",
	"open.spotify.com",
	"music.amazon.com",
	"castro.fm",
	"overcast.fm",
	"pocketcasts.com",
	"podcastaddict.com",
	"player.fm",
}

func isNoiseURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return true
	}
	lower := strings.ToLower(parsed.Path)
	for ext := range noiseExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	host := strings.ToLower(parsed.Hostname())
	for _, prefix := range noiseDomainPrefixes {
		if host == prefix || strings.HasSuffix(host, "."+prefix) {
			return true
		}
	}
	return false
}

func extractDomain(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimPrefix(host, "www.")
	return host
}

// ExtractLinksFromHTML parses HTML and returns all <a href> links.
func ExtractLinksFromHTML(htmlContent string) []ShowNoteLinkData {
	if strings.TrimSpace(htmlContent) == "" {
		return nil
	}
	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))
	var links []ShowNoteLinkData
	seen := make(map[string]bool)
	pos := 0

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		tn, hasAttr := tokenizer.TagName()
		if string(tn) != "a" || !hasAttr {
			continue
		}

		var href string
		for {
			key, val, more := tokenizer.TagAttr()
			if string(key) == "href" {
				href = strings.TrimSpace(string(val))
			}
			if !more {
				break
			}
		}
		if href == "" || isNoiseURL(href) {
			continue
		}
		if seen[href] {
			continue
		}
		seen[href] = true

		title := extractAnchorText(tokenizer)
		links = append(links, ShowNoteLinkData{
			URL:      href,
			Title:    title,
			Domain:   extractDomain(href),
			Position: pos,
		})
		pos++
	}
	return links
}

func extractAnchorText(tokenizer *html.Tokenizer) string {
	var buf strings.Builder
	depth := 1
	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return strings.TrimSpace(buf.String())
		case html.TextToken:
			buf.Write(tokenizer.Text())
		case html.StartTagToken:
			depth++
		case html.EndTagToken:
			depth--
			if depth <= 0 {
				return strings.TrimSpace(buf.String())
			}
		}
	}
}

// ExtractBareURLs finds URLs in plain text that aren't inside HTML tags.
func ExtractBareURLs(text string) []ShowNoteLinkData {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	matches := bareURLRe.FindAllString(text, -1)
	var links []ShowNoteLinkData
	seen := make(map[string]bool)
	pos := 0
	for _, rawURL := range matches {
		rawURL = strings.TrimRight(rawURL, ".,;:!?")
		if seen[rawURL] || isNoiseURL(rawURL) {
			continue
		}
		seen[rawURL] = true
		links = append(links, ShowNoteLinkData{
			URL:      rawURL,
			Domain:   extractDomain(rawURL),
			Position: pos,
		})
		pos++
	}
	return links
}

// ExtractShowNoteLinks extracts links from HTML show notes, falling back to
// bare URL extraction from plain text. Results are deduplicated by URL.
func ExtractShowNoteLinks(htmlContent, plainText string) []ShowNoteLinkData {
	links := ExtractLinksFromHTML(htmlContent)
	seen := make(map[string]bool, len(links))
	for _, l := range links {
		seen[l.URL] = true
	}
	for _, l := range ExtractBareURLs(plainText) {
		if !seen[l.URL] {
			seen[l.URL] = true
			l.Position = len(links)
			links = append(links, l)
		}
	}
	return links
}
