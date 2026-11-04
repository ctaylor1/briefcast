package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ctaylor1/briefcast/model"
	strip "github.com/grokify/html-strip-tags-go"
)

// Directory search enrichment: fills in missing podcast descriptions from the
// feed's channel header and adds Apple Top Podcasts chart positions. Every step
// is best effort and bounded; a slow or failing source never fails the search.

var appleTopChartURL = "https://rss.marketingtools.apple.com/api/v2/us/podcasts/top/100/podcasts.json"

var (
	searchEnrichmentBudget     = 8 * time.Second
	appleChartFetchTimeout     = 4 * time.Second
	appleChartCacheTTL         = 6 * time.Hour
	appleChartFailureRetry     = 10 * time.Minute
	feedDescriptionReadLimit   = int64(512 * 1024)
	feedDescriptionConcurrency = 6
	searchDescriptionMaxRunes  = 600
	maxSearchResultsToDescribe = 50
	appleChartCacheMu          sync.Mutex
	appleChartCachedRanks      map[string]int
	appleChartCachedAt         time.Time
	appleChartLastFetchOK      bool
)

// EnrichSearchResults fills empty descriptions and Apple chart ranks in place.
func EnrichSearchResults(results []*model.CommonSearchResultModel) {
	if len(results) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchEnrichmentBudget)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		applyAppleChartRanks(ctx, results)
	}()

	// Describe results in display order so the top results are filled first
	// if the budget runs out.
	pending := make(chan *model.CommonSearchResultModel, len(results))
	for i, result := range results {
		if i >= maxSearchResultsToDescribe {
			break
		}
		if result == nil || strings.TrimSpace(result.Description) != "" || strings.TrimSpace(result.URL) == "" {
			continue
		}
		pending <- result
	}
	close(pending)
	for w := 0; w < feedDescriptionConcurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range pending {
				if ctx.Err() != nil {
					return
				}
				r.Description = fetchFeedDescription(ctx, r.URL)
			}
		}()
	}
	wg.Wait()
}

func applyAppleChartRanks(ctx context.Context, results []*model.CommonSearchResultModel) {
	hasAppleID := false
	for _, r := range results {
		if r != nil && r.AppleID != "" {
			hasAppleID = true
			break
		}
	}
	if !hasAppleID {
		return
	}
	ranks := appleTopChartRanks(ctx)
	if len(ranks) == 0 {
		return
	}
	for _, r := range results {
		if r == nil || r.AppleID == "" {
			continue
		}
		if rank, ok := ranks[r.AppleID]; ok {
			r.AppleChartRank = rank
		}
	}
}

// appleTopChartRanks returns Apple ID -> 1-based chart position, cached.
func appleTopChartRanks(ctx context.Context) map[string]int {
	appleChartCacheMu.Lock()
	defer appleChartCacheMu.Unlock()

	ttl := appleChartCacheTTL
	if !appleChartLastFetchOK {
		ttl = appleChartFailureRetry
	}
	if !appleChartCachedAt.IsZero() && time.Since(appleChartCachedAt) < ttl {
		return appleChartCachedRanks
	}

	fetchCtx, cancel := context.WithTimeout(ctx, appleChartFetchTimeout)
	defer cancel()
	ranks, err := fetchAppleTopChartRanks(fetchCtx)
	appleChartCachedAt = time.Now()
	if err != nil {
		Logger.Warnw("apple top podcasts chart unavailable", "error", err)
		appleChartLastFetchOK = false
		appleChartCachedRanks = nil
		return nil
	}
	appleChartLastFetchOK = true
	appleChartCachedRanks = ranks
	return ranks
}

func resetAppleChartCacheForTests() {
	appleChartCacheMu.Lock()
	defer appleChartCacheMu.Unlock()
	appleChartCachedRanks = nil
	appleChartCachedAt = time.Time{}
	appleChartLastFetchOK = false
}

type appleChartResponse struct {
	Feed struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	} `json:"feed"`
}

func fetchAppleTopChartRanks(ctx context.Context) (map[string]int, error) {
	req, err := getRequest(appleTopChartURL)
	if err != nil {
		return nil, err
	}
	resp, err := doRequestWithHostLimit(httpClient(), req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("apple chart returned HTTP %d", resp.StatusCode)
	}
	body, err := readBoundedOutboundBody(resp.Body, resp.ContentLength)
	if err != nil {
		return nil, err
	}
	var parsed appleChartResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	ranks := make(map[string]int, len(parsed.Feed.Results))
	for i, entry := range parsed.Feed.Results {
		if id := strings.TrimSpace(entry.ID); id != "" {
			if _, seen := ranks[id]; !seen {
				ranks[id] = i + 1
			}
		}
	}
	return ranks, nil
}

// fetchFeedDescription reads only the start of a feed and returns its channel description.
func fetchFeedDescription(ctx context.Context, feedURL string) string {
	req, err := getRequest(feedURL)
	if err != nil {
		return ""
	}
	resp, err := doRequestWithHostLimit(httpClient(), req.WithContext(ctx))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ""
	}
	return extractFeedChannelDescription(io.LimitReader(resp.Body, feedDescriptionReadLimit))
}

// extractFeedChannelDescription returns the podcast-level description of an RSS
// or Atom document, preferring <description>, then itunes:summary, then
// itunes:subtitle (Atom: <subtitle>). Episode-level text is never used.
// Truncated or malformed input yields whatever was found before the error.
func extractFeedChannelDescription(r io.Reader) string {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	// No HTML auto-close: RSS <link>…</link> is a content element, and treating
	// it as a void tag would desynchronize element depth.
	dec.Entity = xml.HTMLEntity
	dec.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }

	found := map[string]string{}
	depth := 0
	channelDepth := -1
	captureKey := ""
	captureDepth := 0
	var text strings.Builder

	best := func() string {
		for _, key := range []string{"description", "summary", "subtitle"} {
			if v := cleanSearchDescription(found[key]); v != "" {
				return v
			}
		}
		return ""
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			return best()
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			name := strings.ToLower(t.Name.Local)
			isItunes := strings.Contains(strings.ToLower(t.Name.Space), "itunes")
			if channelDepth < 0 {
				if (name == "channel" || name == "feed") && !isItunes {
					channelDepth = depth
				}
				continue
			}
			if depth != channelDepth+1 || captureKey != "" {
				continue
			}
			switch {
			case (name == "item" || name == "entry") && !isItunes:
				return best()
			case name == "description" && !isItunes:
				captureKey = "description"
			case name == "summary" && isItunes:
				captureKey = "summary"
			case name == "subtitle":
				captureKey = "subtitle"
			}
			if captureKey != "" {
				captureDepth = depth
				text.Reset()
			}
		case xml.CharData:
			if captureKey != "" {
				text.Write(t)
			}
		case xml.EndElement:
			if captureKey != "" && depth == captureDepth {
				if _, exists := found[captureKey]; !exists {
					found[captureKey] = text.String()
				}
				if captureKey == "description" && cleanSearchDescription(found[captureKey]) != "" {
					return best()
				}
				captureKey = ""
			}
			if depth == channelDepth {
				return best()
			}
			depth--
		}
	}
}

var htmlBreakSpacer = strings.NewReplacer(
	"<br", " <br", "<BR", " <BR", "<p", " <p", "<P", " <P", "</p>", "</p> ", "</P>", "</P> ",
	"<li", " <li", "<div", " <div", "</div>", "</div> ",
)

func cleanSearchDescription(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = html.UnescapeString(s)
	s = htmlBreakSpacer.Replace(s) // keep words apart when tags are removed
	s = strip.StripTags(s)
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > searchDescriptionMaxRunes {
		runes := []rune(s)
		s = strings.TrimSpace(string(runes[:searchDescriptionMaxRunes])) + "…"
	}
	return s
}
