package db

import (
	"fmt"
	"testing"
	"time"
)

var linkStatsBase = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func linkStatsDay(n int) time.Time {
	return linkStatsBase.AddDate(0, 0, n-1)
}

func webTarget(key string) *LinkTarget {
	return &LinkTarget{
		NormalizedKey:     key,
		CanonicalURL:      "https://example.test/" + key,
		Kind:              "web",
		Platform:          "web",
		Domain:            "example.test",
		ExtractionVersion: 2,
	}
}

func saveLink(t *testing.T, podcastID, itemID, rawURL, label string, target *LinkTarget) {
	t.Helper()
	write := testLinkWrite(podcastID, itemID, rawURL, 0, target, nil)
	write.Link.ContextLabel = label
	if err := SaveShowNoteLinks([]ShowNoteLinkWrite{write}); err != nil {
		t.Fatalf("save %s: %v", rawURL, err)
	}
}

type linkStatsFixture struct {
	podcastA, podcastB Podcast
}

// setupLinkStatsFixture stores:
//   - podcast A, 6 episodes (days 1-6): x:host in all 6 (boilerplate),
//     web:article in days 1-2, and one link without a target in day 1.
//   - podcast B, 2 episodes (days 3 and 6): web:article in both, x:host in
//     day 6. B has too few episodes for anything to be boilerplate.
func setupLinkStatsFixture(t *testing.T) linkStatsFixture {
	t.Helper()
	setupDBForTest(t)
	fixture := linkStatsFixture{
		podcastA: newPodcast(t, "Alpha", "https://example.com/a.xml"),
		podcastB: newPodcast(t, "Bravo", "https://example.com/b.xml"),
	}
	a, b := fixture.podcastA, fixture.podcastB

	for day := 1; day <= 6; day++ {
		item := newPodcastItem(t, a.ID, fmt.Sprintf("a-%d", day), fmt.Sprintf("A%d", day), NotDownloaded, linkStatsDay(day))
		saveLink(t, a.ID, item.ID, "https://x.com/host", "Hosts", testTarget("x:host", "profile", 2))
		if day <= 2 {
			saveLink(t, a.ID, item.ID, "https://example.test/article", "Referenced", webTarget("web:article"))
		}
		if day == 1 {
			saveLink(t, a.ID, item.ID, "mailto:someone@example.com", "", nil)
		}
	}
	for _, day := range []int{3, 6} {
		item := newPodcastItem(t, b.ID, fmt.Sprintf("b-%d", day), fmt.Sprintf("B%d", day), NotDownloaded, linkStatsDay(day))
		saveLink(t, b.ID, item.ID, "https://example.test/article?utm_source=b", "Links", webTarget("web:article"))
		if day == 6 {
			saveLink(t, b.ID, item.ID, "https://twitter.com/host", "Guest", testTarget("x:host", "profile", 2))
		}
	}
	return fixture
}

type rankSummary struct {
	key                              string
	episodes, podcasts, boilerplates int64
}

func summarizeRanks(ranks []LinkTargetRank) []rankSummary {
	out := make([]rankSummary, len(ranks))
	for i, rank := range ranks {
		out[i] = rankSummary{rank.NormalizedKey, rank.EpisodeCount, rank.PodcastCount, rank.BoilerplatePodcastCount}
	}
	return out
}

func assertRanks(t *testing.T, q TopLinkTargetsQuery, want ...rankSummary) []LinkTargetRank {
	t.Helper()
	ranks, err := TopLinkTargets(q)
	if err != nil {
		t.Fatalf("TopLinkTargets(%+v): %v", q, err)
	}
	got := summarizeRanks(ranks)
	if len(got) != len(want) {
		t.Fatalf("TopLinkTargets(%+v) = %+v, want %+v", q, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TopLinkTargets(%+v) = %+v, want %+v", q, got, want)
		}
	}
	return ranks
}

func timePtr(value time.Time) *time.Time { return &value }

func TestTopLinkTargetsExcludesBoilerplateByDefault(t *testing.T) {
	setupLinkStatsFixture(t)
	// x:host is boilerplate in A, so only B's episode counts.
	assertRanks(t, TopLinkTargetsQuery{},
		rankSummary{"web:article", 4, 2, 0},
		rankSummary{"x:host", 1, 1, 0},
	)
}

func TestTopLinkTargetsIncludeBoilerplate(t *testing.T) {
	setupLinkStatsFixture(t)
	ranks := assertRanks(t, TopLinkTargetsQuery{IncludeBoilerplate: true},
		rankSummary{"x:host", 7, 2, 1},
		rankSummary{"web:article", 4, 2, 0},
	)
	article := ranks[1]
	if article.FirstSeen == nil || !article.FirstSeen.Equal(linkStatsDay(1)) {
		t.Errorf("article first seen = %v, want %v", article.FirstSeen, linkStatsDay(1))
	}
	if article.LastSeen == nil || !article.LastSeen.Equal(linkStatsDay(6)) {
		t.Errorf("article last seen = %v, want %v", article.LastSeen, linkStatsDay(6))
	}
	if article.Kind != "web" || article.Platform != "web" || article.CanonicalURL == "" || article.TargetID == "" {
		t.Errorf("article metadata incomplete: %+v", article)
	}
}

func TestTopLinkTargetsFilters(t *testing.T) {
	fixture := setupLinkStatsFixture(t)

	assertRanks(t, TopLinkTargetsQuery{PodcastID: fixture.podcastA.ID},
		rankSummary{"web:article", 2, 1, 0},
	)
	assertRanks(t, TopLinkTargetsQuery{PodcastID: fixture.podcastA.ID, IncludeBoilerplate: true},
		rankSummary{"x:host", 6, 1, 1},
		rankSummary{"web:article", 2, 1, 0},
	)
	assertRanks(t, TopLinkTargetsQuery{Kind: "profile", IncludeBoilerplate: true},
		rankSummary{"x:host", 7, 2, 1},
	)
	assertRanks(t, TopLinkTargetsQuery{Platform: "web", IncludeBoilerplate: true},
		rankSummary{"web:article", 4, 2, 0},
	)
	assertRanks(t, TopLinkTargetsQuery{Kind: "post"})
	assertRanks(t, TopLinkTargetsQuery{IncludeBoilerplate: true, Limit: 1},
		rankSummary{"x:host", 7, 2, 1},
	)
}

func TestTopLinkTargetsDateWindow(t *testing.T) {
	setupLinkStatsFixture(t)
	// Boilerplate is judged over all history, so x:host stays boilerplate
	// in A even though the window holds only A's days 3-6.
	assertRanks(t, TopLinkTargetsQuery{Since: timePtr(linkStatsDay(3))},
		rankSummary{"web:article", 2, 1, 0},
		rankSummary{"x:host", 1, 1, 0},
	)
	assertRanks(t, TopLinkTargetsQuery{Until: timePtr(linkStatsDay(3))},
		rankSummary{"web:article", 2, 1, 0},
	)
	// Equal counts and last-seen dates fall back to key order.
	assertRanks(t, TopLinkTargetsQuery{Since: timePtr(linkStatsDay(2)), Until: timePtr(linkStatsDay(4)), IncludeBoilerplate: true},
		rankSummary{"web:article", 2, 2, 0},
		rankSummary{"x:host", 2, 1, 1},
	)
}

func TestTopLinkTargetsCountsEpisodesNotURLs(t *testing.T) {
	fixture := setupLinkStatsFixture(t)
	// A second URL for the same target in the same episode counts once.
	var item PodcastItem
	if err := DB.Where("guid = ?", "b-3").First(&item).Error; err != nil {
		t.Fatal(err)
	}
	saveLink(t, fixture.podcastB.ID, item.ID, "https://example.test/article#comments", "", webTarget("web:article"))
	assertRanks(t, TopLinkTargetsQuery{Platform: "web"},
		rankSummary{"web:article", 4, 2, 0},
	)
}

func TestLinkTargetPodcastCountsFlagsBoilerplate(t *testing.T) {
	fixture := setupLinkStatsFixture(t)
	host, err := GetLinkTargetByKey("x:host")
	if err != nil {
		t.Fatal(err)
	}
	counts, err := LinkTargetPodcastCounts(host.ID, BoilerplateRule{})
	if err != nil {
		t.Fatal(err)
	}
	want := []LinkTargetPodcastCount{
		{PodcastID: fixture.podcastA.ID, PodcastTitle: "Alpha", EpisodeCount: 6, PodcastEpisodeCount: 6, Boilerplate: true},
		{PodcastID: fixture.podcastB.ID, PodcastTitle: "Bravo", EpisodeCount: 1, PodcastEpisodeCount: 2, Boilerplate: false},
	}
	if len(counts) != len(want) {
		t.Fatalf("counts = %+v, want %+v", counts, want)
	}
	for i := range want {
		if counts[i] != want[i] {
			t.Errorf("counts[%d] = %+v, want %+v", i, counts[i], want[i])
		}
	}
}

func TestLinkTargetOccurrencesNewestFirst(t *testing.T) {
	fixture := setupLinkStatsFixture(t)
	article, err := GetLinkTargetByKey("web:article")
	if err != nil {
		t.Fatal(err)
	}
	occurrences, err := LinkTargetOccurrences(article.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	wantTitles := []string{"B6", "B3", "A2"}
	if len(occurrences) != len(wantTitles) {
		t.Fatalf("got %d occurrences, want %d: %+v", len(occurrences), len(wantTitles), occurrences)
	}
	for i, title := range wantTitles {
		if occurrences[i].EpisodeTitle != title {
			t.Errorf("occurrence %d = %q, want %q", i, occurrences[i].EpisodeTitle, title)
		}
	}
	first := occurrences[0]
	if first.PodcastID != fixture.podcastB.ID || first.PodcastTitle != "Bravo" ||
		first.URL != "https://example.test/article?utm_source=b" || first.ContextLabel != "Links" {
		t.Errorf("unexpected occurrence %+v", first)
	}
	if first.PubDate == nil || !first.PubDate.Equal(linkStatsDay(6)) {
		t.Errorf("pub date = %v, want %v", first.PubDate, linkStatsDay(6))
	}
}

func TestBoilerplateRule(t *testing.T) {
	rule := BoilerplateRule{}
	cases := []struct {
		target, podcast int64
		want            bool
	}{
		{5, 5, true},
		{3, 6, true},  // exactly 50%
		{2, 6, false}, // below 50%
		{4, 4, false}, // too few episodes to judge
		{0, 0, false},
	}
	for _, tc := range cases {
		if got := rule.IsBoilerplate(tc.target, tc.podcast); got != tc.want {
			t.Errorf("IsBoilerplate(%d, %d) = %v, want %v", tc.target, tc.podcast, got, tc.want)
		}
	}
	strict := BoilerplateRule{MinEpisodes: 2, Percent: 90}
	if strict.IsBoilerplate(8, 10) || !strict.IsBoilerplate(9, 10) {
		t.Error("custom rule thresholds not applied")
	}
}

func TestFlexibleTimeScan(t *testing.T) {
	want := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	inputs := []any{
		want,
		"2026-10-01 12:00:00+00:00",
		"2026-10-01 12:00:00Z",
		"2026-10-01T12:00:00Z",
		"2026-10-01 14:00:00+02:00",
		[]byte("2026-10-01 12:00:00.000 +0000 UTC"),
		"2026-10-01 12:00:00 +0000 UTC m=+0.000000001",
		"2026-10-01 12:00:00",
	}
	for _, input := range inputs {
		var got flexibleTime
		if err := got.Scan(input); err != nil {
			t.Errorf("Scan(%v): %v", input, err)
			continue
		}
		if !got.Valid || !got.Time.Equal(want) {
			t.Errorf("Scan(%v) = %v, want %v", input, got.Time, want)
		}
	}

	var empty flexibleTime
	if err := empty.Scan(nil); err != nil || empty.ptr() != nil {
		t.Errorf("Scan(nil) = %+v, %v; want invalid", empty, err)
	}
	var bad flexibleTime
	if err := bad.Scan("not a time"); err == nil {
		t.Error("Scan of an unparseable string succeeded")
	}
}
