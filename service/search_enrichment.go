package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ctaylor1/briefcast/model"
	strip "github.com/grokify/html-strip-tags-go"
)

// Directory search enrichment: fills in missing podcast descriptions from the
// feed's channel header and adds Apple Top Podcasts chart positions (overall and
// per genre). Every step is best effort and bounded; a slow or failing source
// never fails the search.

// Apple chart sources. The legacy iTunes RSS (same host as search) is fast,
// returns 200 entries and supports per-genre charts; Apple does not document it,
// so the marketing-tools API (top 100, intermittently failing) is the fallback
// for the overall chart.
var (
	appleLegacyChartURL      = "https://itunes.apple.com/us/rss/toppodcasts/limit=200/json"
	appleLegacyGenreChartURL = "https://itunes.apple.com/us/rss/toppodcasts/limit=200/genre=%s/json"
	appleTopChartURL         = "https://rss.marketingtools.apple.com/api/v2/us/podcasts/top/100/podcasts.json"
)

const appleOverallChartKey = ""

var (
	searchEnrichmentBudget     = 8 * time.Second
	appleChartAttemptTimeout   = 2500 * time.Millisecond
	appleChartAttempts         = 3
	appleChartCacheTTL         = 6 * time.Hour
	appleChartFailureRetry     = 10 * time.Minute
	appleChartStaleLimit       = 48 * time.Hour // keep serving the last good chart this long
	maxGenreChartsPerSearch    = 4
	feedDescriptionReadLimit   = int64(512 * 1024)
	feedDescriptionConcurrency = 6
	searchDescriptionMaxRunes  = 600
	maxSearchResultsToDescribe = 50
	appleGenreIDPattern        = regexp.MustCompile(`^[0-9]{1,6}$`)

	appleChartsMu sync.Mutex
	appleCharts   = map[string]*appleChartEntry{}
)

type appleChartEntry struct {
	mu          sync.Mutex
	ranks       map[string]int // last successful chart: Apple ID -> 1-based position
	goodAt      time.Time      // when ranks was fetched
	attemptedAt time.Time      // last fetch attempt
	lastOK      bool
}

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
	genres := make([]string, 0, maxGenreChartsPerSearch)
	seenGenre := map[string]bool{}
	for _, r := range results {
		if r == nil || r.AppleID == "" {
			continue
		}
		hasAppleID = true
		g := r.AppleGenreID
		if appleGenreIDPattern.MatchString(g) && !seenGenre[g] && len(genres) < maxGenreChartsPerSearch {
			seenGenre[g] = true
			genres = append(genres, g)
		}
	}
	if !hasAppleID {
		return
	}

	keys := append([]string{appleOverallChartKey}, genres...)
	charts := make([]map[string]int, len(keys))
	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			charts[i] = appleChartRanks(ctx, key)
		}(i, key)
	}
	wg.Wait()
	byKey := make(map[string]map[string]int, len(keys))
	for i, key := range keys {
		byKey[key] = charts[i]
	}

	for _, r := range results {
		if r == nil || r.AppleID == "" {
			continue
		}
		if rank, ok := byKey[appleOverallChartKey][r.AppleID]; ok {
			r.AppleChartRank = rank
		}
		if r.AppleGenreID == "" {
			continue
		}
		if rank, ok := byKey[r.AppleGenreID][r.AppleID]; ok {
			r.AppleGenreRank = rank
			r.AppleGenreName = r.AppleGenre
		}
	}
}

func appleChartEntryFor(key string) *appleChartEntry {
	appleChartsMu.Lock()
	defer appleChartsMu.Unlock()
	entry, ok := appleCharts[key]
	if !ok {
		entry = &appleChartEntry{}
		appleCharts[key] = entry
	}
	return entry
}

// appleChartRanks returns Apple ID -> 1-based position for the overall chart
// (appleOverallChartKey) or a genre chart, cached with retries and a stale fallback.
func appleChartRanks(ctx context.Context, key string) map[string]int {
	entry := appleChartEntryFor(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	ttl := appleChartCacheTTL
	if !entry.lastOK {
		ttl = appleChartFailureRetry
	}
	if !entry.attemptedAt.IsZero() && time.Since(entry.attemptedAt) < ttl {
		return entry.usable()
	}

	var ranks map[string]int
	var err error
	for attempt := 0; attempt < appleChartAttempts && ctx.Err() == nil; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, appleChartAttemptTimeout)
		ranks, err = fetchAppleChart(attemptCtx, key)
		cancel()
		if err == nil {
			break
		}
	}
	entry.attemptedAt = time.Now()
	if err != nil || len(ranks) == 0 {
		Logger.Warnw("apple top podcasts chart unavailable", "genre", key, "error", err, "serving_stale", entry.usable() != nil)
		entry.lastOK = false
		return entry.usable()
	}
	entry.lastOK = true
	entry.ranks = ranks
	entry.goodAt = entry.attemptedAt
	return ranks
}

// usable returns the last good chart unless it is too old. Caller holds entry.mu.
func (e *appleChartEntry) usable() map[string]int {
	if e.ranks == nil || time.Since(e.goodAt) > appleChartStaleLimit {
		return nil
	}
	return e.ranks
}

func resetAppleChartCacheForTests() {
	appleChartsMu.Lock()
	defer appleChartsMu.Unlock()
	appleCharts = map[string]*appleChartEntry{}
}

// expireAppleChartForTests makes the next lookup refetch; goodAge ages the last good copy.
func expireAppleChartForTests(key string, goodAge time.Duration) {
	entry := appleChartEntryFor(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.attemptedAt = time.Now().Add(-appleChartCacheTTL - time.Minute)
	if goodAge > 0 {
		entry.goodAt = time.Now().Add(-goodAge)
	}
}

// fetchAppleChart tries the legacy iTunes RSS, then (overall chart only) the marketing-tools API.
func fetchAppleChart(ctx context.Context, key string) (map[string]int, error) {
	chartURL := appleLegacyChartURL
	if key != appleOverallChartKey {
		if !appleGenreIDPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid apple genre id %q", key)
		}
		chartURL = fmt.Sprintf(appleLegacyGenreChartURL, key)
	}
	ranks, err := fetchChartJSON(ctx, chartURL, parseLegacyAppleChart)
	if err == nil && len(ranks) > 0 {
		return ranks, nil
	}
	if key == appleOverallChartKey && ctx.Err() == nil {
		alt, altErr := fetchChartJSON(ctx, appleTopChartURL, parseMarketingAppleChart)
		if altErr == nil && len(alt) > 0 {
			return alt, nil
		}
		if altErr != nil {
			err = altErr
		}
	}
	if err == nil {
		err = fmt.Errorf("apple chart was empty")
	}
	return nil, err
}

func fetchChartJSON(ctx context.Context, chartURL string, parse func([]byte) ([]string, error)) (map[string]int, error) {
	req, err := getRequest(chartURL)
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
	ids, err := parse(body)
	if err != nil {
		return nil, err
	}
	ranks := make(map[string]int, len(ids))
	for i, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			if _, seen := ranks[id]; !seen {
				ranks[id] = i + 1
			}
		}
	}
	return ranks, nil
}

type legacyAppleChartEntry struct {
	ID struct {
		Attributes struct {
			ID string `json:"im:id"`
		} `json:"attributes"`
	} `json:"id"`
}

// parseLegacyAppleChart reads itunes.apple.com/.../rss/toppodcasts JSON, where
// "entry" is an array (or a single object when only one podcast is listed).
func parseLegacyAppleChart(body []byte) ([]string, error) {
	var parsed struct {
		Feed struct {
			Entry json.RawMessage `json:"entry"`
		} `json:"feed"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	raw := parsed.Feed.Entry
	if len(raw) == 0 {
		return nil, nil
	}
	var entries []legacyAppleChartEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		var single legacyAppleChartEntry
		if err := json.Unmarshal(raw, &single); err != nil {
			return nil, err
		}
		entries = []legacyAppleChartEntry{single}
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID.Attributes.ID)
	}
	return ids, nil
}

// parseMarketingAppleChart reads rss.marketingtools.apple.com top-podcasts JSON.
func parseMarketingAppleChart(body []byte) ([]string, error) {
	var parsed struct {
		Feed struct {
			Results []struct {
				ID string `json:"id"`
			} `json:"results"`
		} `json:"feed"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(parsed.Feed.Results))
	for _, entry := range parsed.Feed.Results {
		ids = append(ids, entry.ID)
	}
	return ids, nil
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
