package feedmeta

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// ShowNoteLinkData holds a link extracted from show notes.
type ShowNoteLinkData struct {
	URL      string
	Title    string
	Domain   string
	Position int
	// ContextLabel is the section label that introduces the link, such as
	// "Referenced in the show" or "Follow on X", without its trailing colon.
	// Empty when no label precedes the link.
	ContextLabel string
}

// maxContextLabelRunes caps labels; longer text ending in a colon is prose,
// not a section label.
const maxContextLabelRunes = 120

var sentenceEndRe = regexp.MustCompile(`[.!?]\s+`)

// labelFromText returns the label that the text before a link introduces:
// the last line (or the last sentence on it) when it ends in a colon, e.g.
// "Great show. Referenced in the show:" gives "Referenced in the show".
func labelFromText(segment string) (string, bool) {
	// Only text after the last URL in the segment can introduce the next link.
	if locs := bareURLRe.FindAllStringIndex(segment, -1); len(locs) > 0 {
		segment = segment[locs[len(locs)-1][1]:]
	}
	segment = strings.TrimRightFunc(segment, unicode.IsSpace)
	if !strings.HasSuffix(segment, ":") {
		return "", false
	}
	if i := strings.LastIndexAny(segment, "\r\n"); i >= 0 {
		segment = segment[i+1:]
	}
	if locs := sentenceEndRe.FindAllStringIndex(segment, -1); len(locs) > 0 {
		segment = segment[locs[len(locs)-1][1]:]
	}
	return cleanLabel(segment)
}

// cleanLabel unescapes entities, collapses whitespace, drops a trailing
// colon, and rejects empty or overlong labels.
func cleanLabel(text string) (string, bool) {
	label := strings.Join(strings.Fields(html.UnescapeString(text)), " ")
	label = strings.TrimSpace(strings.TrimSuffix(label, ":"))
	if label == "" || utf8.RuneCountInString(label) > maxContextLabelRunes {
		return "", false
	}
	return label, true
}

var headingTags = map[string]bool{"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true}

// lineBreakTags end a line of text, so a label on one line is not merged with
// text on the next.
var lineBreakTags = map[string]bool{
	"br": true, "p": true, "div": true, "li": true, "ul": true, "ol": true,
	"tr": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
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

// ExtractLinksFromHTML parses HTML and returns all <a href> links, each with
// the section label (heading, or text ending in a colon) that precedes it.
// A label applies to following links until another label appears.
func ExtractLinksFromHTML(htmlContent string) []ShowNoteLinkData {
	if strings.TrimSpace(htmlContent) == "" {
		return nil
	}
	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))
	var links []ShowNoteLinkData
	seen := make(map[string]bool)
	pos := 0

	var segment strings.Builder // text since the previous link or heading
	var heading strings.Builder
	headingDepth := 0
	label := ""

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.TextToken:
			text := tokenizer.Text()
			segment.Write(text)
			if headingDepth > 0 {
				heading.Write(text)
			}
			continue
		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			name := string(tn)
			if lineBreakTags[name] {
				segment.WriteByte('\n')
			}
			if headingTags[name] && headingDepth > 0 {
				headingDepth--
				if headingDepth == 0 {
					if headingLabel, ok := cleanLabel(heading.String()); ok {
						label = headingLabel
					}
					heading.Reset()
					segment.Reset()
				}
			}
			continue
		case html.StartTagToken, html.SelfClosingTagToken:
		default:
			continue
		}

		tn, hasAttr := tokenizer.TagName()
		name := string(tn)
		if lineBreakTags[name] {
			segment.WriteByte('\n')
		}
		if headingTags[name] && tt == html.StartTagToken {
			headingDepth++
			continue
		}
		if name != "a" {
			continue
		}

		if textLabel, ok := labelFromText(segment.String()); ok {
			label = textLabel
		}
		segment.Reset()

		var href string
		for hasAttr {
			key, val, more := tokenizer.TagAttr()
			if string(key) == "href" {
				href = strings.TrimSpace(string(val))
			}
			hasAttr = more
		}
		// Consume the anchor's text even for skipped links so it cannot be
		// mistaken for a label introducing the next link.
		title := ""
		if tt == html.StartTagToken {
			title = extractAnchorText(tokenizer)
		}
		if href == "" || isNoiseURL(href) || seen[href] {
			continue
		}
		seen[href] = true

		links = append(links, ShowNoteLinkData{
			URL:          href,
			Title:        title,
			Domain:       extractDomain(href),
			Position:     pos,
			ContextLabel: label,
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

// ExtractBareURLs finds URLs in plain text that aren't inside HTML tags, each
// with the label (text ending in a colon) that precedes it. A label applies to
// following URLs until another label appears.
func ExtractBareURLs(text string) []ShowNoteLinkData {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var links []ShowNoteLinkData
	seen := make(map[string]bool)
	pos := 0
	label := ""
	prevEnd := 0
	for _, loc := range bareURLRe.FindAllStringIndex(text, -1) {
		if textLabel, ok := labelFromText(text[prevEnd:loc[0]]); ok {
			label = textLabel
		}
		rawURL := strings.TrimRight(text[loc[0]:loc[1]], ".,;:!?")
		// Trimmed punctuation belongs to the text before the next URL.
		prevEnd = loc[0] + len(rawURL)
		if seen[rawURL] || isNoiseURL(rawURL) {
			continue
		}
		seen[rawURL] = true
		links = append(links, ShowNoteLinkData{
			URL:          rawURL,
			Domain:       extractDomain(rawURL),
			Position:     pos,
			ContextLabel: label,
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
