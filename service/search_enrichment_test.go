package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ctaylor1/briefcast/model"
)

func TestExtractFeedChannelDescription(t *testing.T) {
	long := strings.Repeat("word ", 400)
	cases := []struct {
		name string
		xml  string
		want string
	}{
		{
			name: "rss description with CDATA html, ignores items",
			xml: `<?xml version="1.0"?><rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"><channel>
<title>Show</title><description><![CDATA[<p>Deep   conversations &amp; <b>ideas</b>.</p>]]></description>
<item><description>Episode text</description></item></channel></rss>`,
			want: "Deep conversations & ideas.",
		},
		{
			// Regression: <link> was treated as an HTML void tag and ended the channel early.
			name: "rss link and atom link before description",
			xml: `<rss xmlns:atom="http://www.w3.org/2005/Atom"><channel><title>Show</title>
<link>https://show.example/</link><atom:link href="https://show.example/feed" rel="self"/>
<description><![CDATA[Summaries<br /><br />More <a href="https://x.example">here</a>]]></description></channel></rss>`,
			want: "Summaries More here",
		},
		{
			name: "falls back to itunes summary",
			xml: `<rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"><channel><title>Show</title>
<itunes:summary>Summary text</itunes:summary><item><description>Episode</description></item></channel></rss>`,
			want: "Summary text",
		},
		{
			name: "itunes subtitle when nothing else",
			xml:  `<rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"><channel><itunes:subtitle>Short tagline</itunes:subtitle></channel></rss>`,
			want: "Short tagline",
		},
		{
			name: "never uses episode-level description",
			xml:  `<rss><channel><title>Show</title><item><description>Episode only</description></item></channel></rss>`,
			want: "",
		},
		{
			name: "nested description inside image is not the channel description",
			xml:  `<rss><channel><image><description>Logo</description></image><description>Real</description></channel></rss>`,
			want: "Real",
		},
		{
			name: "atom subtitle",
			xml:  `<feed xmlns="http://www.w3.org/2005/Atom"><title>Show</title><subtitle>Atom tagline</subtitle><entry><summary>Entry</summary></entry></feed>`,
			want: "Atom tagline",
		},
		{
			name: "truncated document still yields the description",
			xml:  `<rss><channel><description>Cut short</description><item><title>Ep`,
			want: "Cut short",
		},
		{
			name: "not xml",
			xml:  `{"json": true}`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractFeedChannelDescription(strings.NewReader(tc.xml)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}

	got := extractFeedChannelDescription(strings.NewReader(`<rss><channel><description>` + long + `</description></channel></rss>`))
	if n := utf8.RuneCountInString(got); n > searchDescriptionMaxRunes+1 || n < searchDescriptionMaxRunes-10 || !strings.HasSuffix(got, "…") {
		t.Fatalf("expected truncation to about %d runes plus ellipsis, got %d runes", searchDescriptionMaxRunes, n)
	}
}

// legacyChart renders itunes.apple.com toppodcasts JSON for the given Apple IDs.
func legacyChart(ids ...string) []byte {
	type attrs struct {
		ID string `json:"im:id"`
	}
	type entry struct {
		ID struct {
			Attributes attrs `json:"attributes"`
		} `json:"id"`
	}
	entries := make([]entry, len(ids))
	for i, id := range ids {
		entries[i].ID.Attributes.ID = id
	}
	body, _ := json.Marshal(map[string]any{"feed": map[string]any{"entry": entries}})
	return body
}

func useChartServer(t *testing.T, server *httptest.Server) {
	t.Helper()
	t.Setenv(outboundAllowPrivateEnv, "true")
	resetAppleChartCacheForTests()
	origLegacy, origGenre, origMarketing := appleLegacyChartURL, appleLegacyGenreChartURL, appleTopChartURL
	appleLegacyChartURL = server.URL + "/legacy"
	appleLegacyGenreChartURL = server.URL + "/genre/%s"
	appleTopChartURL = server.URL + "/marketing"
	t.Cleanup(func() {
		appleLegacyChartURL, appleLegacyGenreChartURL, appleTopChartURL = origLegacy, origGenre, origMarketing
		resetAppleChartCacheForTests()
	})
}

func TestEnrichSearchResultsFillsDescriptionsAndChartRanks(t *testing.T) {
	var chartHits, describedFeedHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/legacy":
			atomic.AddInt32(&chartHits, 1)
			_, _ = w.Write(legacyChart("7", "42"))
		case "/genre/1321":
			atomic.AddInt32(&chartHits, 1)
			_, _ = w.Write(legacyChart("99", "42"))
		case "/feed":
			_, _ = w.Write([]byte(`<rss><channel><description>From the feed</description></channel></rss>`))
		case "/described":
			atomic.AddInt32(&describedFeedHits, 1)
			_, _ = w.Write([]byte(`<rss><channel><description>Should not be fetched</description></channel></rss>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	useChartServer(t, server)

	results := []*model.CommonSearchResultModel{
		{URL: server.URL + "/feed", Title: "Ranked", AppleID: "42", AppleGenreID: "1321", AppleGenre: "Business"},
		{URL: server.URL + "/described", Title: "Has text", Description: "Directory text", AppleID: "99", AppleGenreID: "1321", AppleGenre: "Business"},
		{URL: server.URL + "/missing", Title: "Broken feed", AppleID: "7"},
	}
	EnrichSearchResults(results)

	if results[0].Description != "From the feed" || results[0].AppleChartRank != 2 ||
		results[0].AppleGenreRank != 2 || results[0].AppleGenreName != "Business" {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if results[1].Description != "Directory text" || results[1].AppleChartRank != 0 || results[1].AppleGenreRank != 1 ||
		atomic.LoadInt32(&describedFeedHits) != 0 {
		t.Fatalf("existing description must be kept without fetching: %+v hits=%d", results[1], describedFeedHits)
	}
	if results[2].Description != "" || results[2].AppleChartRank != 1 || results[2].AppleGenreName != "" {
		t.Fatalf("broken feed should leave description empty but keep rank: %+v", results[2])
	}
	if atomic.LoadInt32(&chartHits) != 2 {
		t.Fatalf("expected one overall and one genre chart fetch, got %d", chartHits)
	}

	EnrichSearchResults([]*model.CommonSearchResultModel{{Title: "again", Description: "x", AppleID: "7", AppleGenreID: "1321"}})
	if atomic.LoadInt32(&chartHits) != 2 {
		t.Fatalf("expected charts to be cached, fetched %d times", chartHits)
	}
}

func TestAppleChartFallsBackToMarketingToolsAndIgnoresBadGenreIDs(t *testing.T) {
	var genreHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/legacy":
			http.Error(w, "gone", http.StatusNotFound)
		case r.URL.Path == "/marketing":
			_, _ = w.Write([]byte(`{"feed":{"results":[{"id":"3"},{"id":"8"}]}}`))
		case strings.HasPrefix(r.URL.Path, "/genre/"):
			atomic.AddInt32(&genreHits, 1)
			_, _ = w.Write(legacyChart("8"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	useChartServer(t, server)

	results := []*model.CommonSearchResultModel{{Description: "x", AppleID: "8", AppleGenreID: "../../x", AppleGenre: "Bad"}}
	EnrichSearchResults(results)
	if results[0].AppleChartRank != 2 || results[0].AppleGenreRank != 0 || atomic.LoadInt32(&genreHits) != 0 {
		t.Fatalf("expected marketing fallback rank and no genre lookup for an invalid id: %+v hits=%d", results[0], genreHits)
	}
}

func TestEnrichSearchResultsFailsSoft(t *testing.T) {
	var chartHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/legacy" || r.URL.Path == "/marketing" {
			atomic.AddInt32(&chartHits, 1)
		}
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer server.Close()
	useChartServer(t, server)

	// Each attempt tries the legacy chart, then the marketing-tools fallback.
	perFetch := int32(appleChartAttempts * 2)
	results := []*model.CommonSearchResultModel{{URL: server.URL + "/feed", Title: "A", AppleID: "1"}}
	start := time.Now()
	EnrichSearchResults(results)
	if elapsed := time.Since(start); elapsed > searchEnrichmentBudget {
		t.Fatalf("enrichment exceeded its budget: %s", elapsed)
	}
	if results[0].Description != "" || results[0].AppleChartRank != 0 || results[0].Title != "A" {
		t.Fatalf("failed sources must leave the result unchanged: %+v", results[0])
	}
	if got := atomic.LoadInt32(&chartHits); got != perFetch {
		t.Fatalf("expected %d chart requests, got %d", perFetch, got)
	}
	EnrichSearchResults([]*model.CommonSearchResultModel{{Description: "x", AppleID: "1"}})
	if got := atomic.LoadInt32(&chartHits); got != perFetch {
		t.Fatalf("a failed chart fetch should not be retried immediately, fetched %d times", got)
	}

	// Results without Apple IDs never touch the chart.
	resetAppleChartCacheForTests()
	EnrichSearchResults([]*model.CommonSearchResultModel{{Description: "x"}})
	if got := atomic.LoadInt32(&chartHits); got != perFetch {
		t.Fatalf("chart fetched for results without Apple IDs")
	}
}

func TestAppleChartRetriesThenServesLastGoodChart(t *testing.T) {
	var legacyHits int32
	var down atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/legacy" {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		n := atomic.AddInt32(&legacyHits, 1)
		if down.Load() || n == 1 {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		_, _ = w.Write(legacyChart("5"))
	}))
	defer server.Close()
	useChartServer(t, server)

	first := []*model.CommonSearchResultModel{{Description: "x", AppleID: "5"}}
	EnrichSearchResults(first)
	if first[0].AppleChartRank != 1 || atomic.LoadInt32(&legacyHits) != 2 {
		t.Fatalf("expected success on the second attempt, rank=%d hits=%d", first[0].AppleChartRank, legacyHits)
	}

	// Expire the cache and make the endpoint fail: the last good chart is still used.
	down.Store(true)
	expireAppleChartForTests(appleOverallChartKey, 0)
	second := []*model.CommonSearchResultModel{{Description: "x", AppleID: "5"}}
	EnrichSearchResults(second)
	if second[0].AppleChartRank != 1 {
		t.Fatalf("expected stale chart to be served after a failed refresh, got rank %d", second[0].AppleChartRank)
	}

	// Beyond the stale limit the old chart is dropped.
	expireAppleChartForTests(appleOverallChartKey, appleChartStaleLimit+time.Minute)
	third := []*model.CommonSearchResultModel{{Description: "x", AppleID: "5"}}
	EnrichSearchResults(third)
	if third[0].AppleChartRank != 0 {
		t.Fatalf("expected no rank once the chart is too old, got %d", third[0].AppleChartRank)
	}
}

func TestSearchResultMappingIncludesPopularityMetadata(t *testing.T) {
	var itunes model.ItunesResponse
	payload := `{"resultCount":1,"results":[{"collectionId":863897795,"trackId":863897795,"artistName":"Tim Ferriss",
"trackName":"The Tim Ferriss Show","feedUrl":"https://rss.example/tfs","artworkUrl600":"https://img.example/a.jpg",
"releaseDate":"2026-10-20T22:30:00Z","trackCount":891,"genres":["Entrepreneurship","Podcasts","Business"],
"genreIds":["1493","26","1321"],"primaryGenreName":"Entrepreneurship"}]}`
	if err := json.Unmarshal([]byte(payload), &itunes); err != nil {
		t.Fatalf("decode itunes payload: %v", err)
	}
	got := GetSearchFromItunes(itunes.Results[0])
	if got.URL != "https://rss.example/tfs" || got.Author != "Tim Ferriss" || got.EpisodeCount != 891 ||
		got.LatestEpisodeDate != "2026-10-20T22:30:00Z" || got.AppleID != "863897795" ||
		got.AppleGenreID != "1493" || got.AppleGenre != "Entrepreneurship" || got.AppleGenreName != "" {
		t.Fatalf("unexpected itunes mapping: %+v", got)
	}
	if strings.Join(got.Categories, ",") != "Entrepreneurship,Business" {
		t.Fatalf("expected genres without the generic Podcasts entry, got %v", got.Categories)
	}

	gp := GetSearchFromGpodder(model.GPodcast{URL: "u", Title: "t", Description: "d", Author: "a", Subscribers: 32, SubscribersLastWeek: 30})
	if gp.Subscribers != 32 || gp.SubscribersLastWeek != 30 || gp.Author != "a" {
		t.Fatalf("unexpected gpodder mapping: %+v", gp)
	}

	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "863897795") || strings.Contains(string(encoded), "1493") {
		t.Fatalf("internal Apple IDs must not be serialized: %s", encoded)
	}
}
