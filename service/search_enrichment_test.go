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

func TestEnrichSearchResultsFillsDescriptionsAndChartRanks(t *testing.T) {
	t.Setenv(outboundAllowPrivateEnv, "true")
	resetAppleChartCacheForTests()
	t.Cleanup(resetAppleChartCacheForTests)

	var chartHits, describedFeedHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chart":
			atomic.AddInt32(&chartHits, 1)
			_, _ = w.Write([]byte(`{"feed":{"results":[{"id":"7"},{"id":"42"}]}}`))
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
	origChart := appleTopChartURL
	appleTopChartURL = server.URL + "/chart"
	t.Cleanup(func() { appleTopChartURL = origChart })

	results := []*model.CommonSearchResultModel{
		{URL: server.URL + "/feed", Title: "Ranked", AppleID: "42"},
		{URL: server.URL + "/described", Title: "Has text", Description: "Directory text", AppleID: "99"},
		{URL: server.URL + "/missing", Title: "Broken feed", AppleID: "7"},
	}
	EnrichSearchResults(results)

	if results[0].Description != "From the feed" || results[0].AppleChartRank != 2 {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if results[1].Description != "Directory text" || results[1].AppleChartRank != 0 || atomic.LoadInt32(&describedFeedHits) != 0 {
		t.Fatalf("existing description must be kept without fetching: %+v hits=%d", results[1], describedFeedHits)
	}
	if results[2].Description != "" || results[2].AppleChartRank != 1 {
		t.Fatalf("broken feed should leave description empty but keep rank: %+v", results[2])
	}

	EnrichSearchResults([]*model.CommonSearchResultModel{{Title: "again", Description: "x", AppleID: "7"}})
	if atomic.LoadInt32(&chartHits) != 1 {
		t.Fatalf("expected chart to be cached, fetched %d times", chartHits)
	}
}

func TestEnrichSearchResultsFailsSoft(t *testing.T) {
	t.Setenv(outboundAllowPrivateEnv, "true")
	resetAppleChartCacheForTests()
	t.Cleanup(resetAppleChartCacheForTests)

	var chartHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chart" {
			atomic.AddInt32(&chartHits, 1)
		}
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer server.Close()
	origChart := appleTopChartURL
	appleTopChartURL = server.URL + "/chart"
	t.Cleanup(func() { appleTopChartURL = origChart })

	results := []*model.CommonSearchResultModel{{URL: server.URL + "/feed", Title: "A", AppleID: "1"}}
	start := time.Now()
	EnrichSearchResults(results)
	if elapsed := time.Since(start); elapsed > searchEnrichmentBudget {
		t.Fatalf("enrichment exceeded its budget: %s", elapsed)
	}
	if results[0].Description != "" || results[0].AppleChartRank != 0 || results[0].Title != "A" {
		t.Fatalf("failed sources must leave the result unchanged: %+v", results[0])
	}
	if got := atomic.LoadInt32(&chartHits); got != int32(appleChartAttempts) {
		t.Fatalf("expected %d chart attempts, got %d", appleChartAttempts, got)
	}
	EnrichSearchResults([]*model.CommonSearchResultModel{{Description: "x", AppleID: "1"}})
	if got := atomic.LoadInt32(&chartHits); got != int32(appleChartAttempts) {
		t.Fatalf("a failed chart fetch should not be retried immediately, fetched %d times", got)
	}

	// Results without Apple IDs never touch the chart.
	resetAppleChartCacheForTests()
	EnrichSearchResults([]*model.CommonSearchResultModel{{Description: "x"}})
	if got := atomic.LoadInt32(&chartHits); got != int32(appleChartAttempts) {
		t.Fatalf("chart fetched for results without Apple IDs")
	}
}

func TestAppleChartRetriesThenServesLastGoodChart(t *testing.T) {
	t.Setenv(outboundAllowPrivateEnv, "true")
	resetAppleChartCacheForTests()
	t.Cleanup(resetAppleChartCacheForTests)

	var hits int32
	var down atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if down.Load() || n == 1 {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"feed":{"results":[{"id":"5"}]}}`))
	}))
	defer server.Close()
	origChart := appleTopChartURL
	appleTopChartURL = server.URL
	t.Cleanup(func() { appleTopChartURL = origChart })

	first := []*model.CommonSearchResultModel{{Description: "x", AppleID: "5"}}
	EnrichSearchResults(first)
	if first[0].AppleChartRank != 1 || atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("expected success on the second attempt, rank=%d hits=%d", first[0].AppleChartRank, hits)
	}

	// Expire the cache and make the endpoint fail: the last good chart is still used.
	down.Store(true)
	appleChartCacheMu.Lock()
	appleChartCachedAt = time.Now().Add(-appleChartCacheTTL - time.Minute)
	appleChartCacheMu.Unlock()
	second := []*model.CommonSearchResultModel{{Description: "x", AppleID: "5"}}
	EnrichSearchResults(second)
	if second[0].AppleChartRank != 1 {
		t.Fatalf("expected stale chart to be served after a failed refresh, got rank %d", second[0].AppleChartRank)
	}

	// Beyond the stale limit the old chart is dropped.
	appleChartCacheMu.Lock()
	appleChartCachedAt = time.Now().Add(-appleChartCacheTTL - time.Minute)
	appleChartGoodAt = time.Now().Add(-appleChartStaleLimit - time.Minute)
	appleChartCacheMu.Unlock()
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
"releaseDate":"2026-10-20T22:30:00Z","trackCount":891,"genres":["Entrepreneurship","Podcasts","Business"]}]}`
	if err := json.Unmarshal([]byte(payload), &itunes); err != nil {
		t.Fatalf("decode itunes payload: %v", err)
	}
	got := GetSearchFromItunes(itunes.Results[0])
	if got.URL != "https://rss.example/tfs" || got.Author != "Tim Ferriss" || got.EpisodeCount != 891 ||
		got.LatestEpisodeDate != "2026-10-20T22:30:00Z" || got.AppleID != "863897795" {
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
	if strings.Contains(string(encoded), "863897795") {
		t.Fatalf("internal Apple ID must not be serialized: %s", encoded)
	}
}
