package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/gin-gonic/gin"
)

func makeLinksRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/links/top", GetTopLinks)
	router.GET("/links/targets/:id", GetLinkTarget)
	return router
}

func getJSON(t *testing.T, router *gin.Engine, path string, wantStatus int, out any) {
	t.Helper()
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
	if resp.Code != wantStatus {
		t.Fatalf("GET %s = %d %s, want %d", path, resp.Code, resp.Body.String(), wantStatus)
	}
	if out != nil {
		if err := json.Unmarshal(resp.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
}

// seedLinkRanking stores one podcast whose six episodes all link to a host
// profile (boilerplate) and two of which link to an article post.
func seedLinkRanking(t *testing.T) (podcast db.Podcast, hostID, postID string) {
	t.Helper()
	podcast = db.Podcast{Title: "Ranked", URL: "https://example.com/ranked.xml"}
	if err := db.CreatePodcast(&podcast); err != nil {
		t.Fatal(err)
	}
	host := &db.LinkTarget{NormalizedKey: "x:host", CanonicalURL: "https://x.com/host", Kind: "profile", Platform: "x", Handle: "host", Domain: "x.com", ExtractionVersion: 2}
	post := &db.LinkTarget{NormalizedKey: "x:status:1", CanonicalURL: "https://x.com/host/status/1", Kind: "post", Platform: "x", Handle: "host", Domain: "x.com", ExtractionVersion: 2}
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for day := 0; day < 6; day++ {
		item := db.PodcastItem{PodcastID: podcast.ID, GUID: "ranked-" + string(rune('a'+day)), Title: "Episode " + string(rune('A'+day)), PubDate: base.AddDate(0, 0, day)}
		if err := db.CreatePodcastItem(&item); err != nil {
			t.Fatal(err)
		}
		writes := []db.ShowNoteLinkWrite{{
			Link:   db.ShowNoteLink{PodcastID: podcast.ID, PodcastItemID: item.ID, URL: "https://x.com/host", Domain: "x.com", ExtractionVersion: 2},
			Target: host,
		}}
		if day >= 4 {
			writes = append(writes, db.ShowNoteLinkWrite{
				Link:   db.ShowNoteLink{PodcastID: podcast.ID, PodcastItemID: item.ID, URL: "https://x.com/host/status/1", Domain: "x.com", Position: 1, ContextLabel: "Referenced in the show", ExtractionVersion: 2},
				Target: post,
				Parent: host,
			})
		}
		if err := db.SaveShowNoteLinks(writes); err != nil {
			t.Fatal(err)
		}
	}
	hostTarget, err := db.GetLinkTargetByKey("x:host")
	if err != nil {
		t.Fatal(err)
	}
	postTarget, err := db.GetLinkTargetByKey("x:status:1")
	if err != nil {
		t.Fatal(err)
	}
	return podcast, hostTarget.ID, postTarget.ID
}

type topLinksResponse struct {
	Count              int                 `json:"count"`
	IncludeBoilerplate bool                `json:"includeBoilerplate"`
	Targets            []db.LinkTargetRank `json:"targets"`
}

func TestGetTopLinksRanksAndHidesBoilerplate(t *testing.T) {
	setupControllersTestDB(t)
	podcast, hostID, postID := seedLinkRanking(t)
	router := makeLinksRouter()

	var hidden topLinksResponse
	getJSON(t, router, "/links/top", http.StatusOK, &hidden)
	if hidden.Count != 1 || hidden.Targets[0].TargetID != postID || hidden.Targets[0].EpisodeCount != 2 {
		t.Fatalf("default ranking = %+v, want only the post with 2 episodes", hidden)
	}
	if hidden.Targets[0].ParentTargetID == nil || *hidden.Targets[0].ParentTargetID != hostID {
		t.Fatalf("post parent = %v, want %s", hidden.Targets[0].ParentTargetID, hostID)
	}

	var all topLinksResponse
	getJSON(t, router, "/links/top?includeBoilerplate=true&podcastId="+podcast.ID+"&platform=X", http.StatusOK, &all)
	if !all.IncludeBoilerplate || all.Count != 2 || all.Targets[0].TargetID != hostID ||
		all.Targets[0].EpisodeCount != 6 || all.Targets[0].BoilerplatePodcastCount != 1 {
		t.Fatalf("ranking with boilerplate = %+v", all)
	}

	var windowed topLinksResponse
	getJSON(t, router, "/links/top?includeBoilerplate=true&kind=profile&since=2026-10-02&until=2026-10-03", http.StatusOK, &windowed)
	if windowed.Count != 1 || windowed.Targets[0].EpisodeCount != 2 {
		t.Fatalf("windowed ranking = %+v, want host in 2 episodes (date-only until is inclusive)", windowed)
	}
}

func TestGetTopLinksRejectsInvalidParams(t *testing.T) {
	setupControllersTestDB(t)
	router := makeLinksRouter()
	for _, query := range []string{
		"kind=person",
		"platform=x.com",
		"since=yesterday",
		"until=2026-13-01",
		"since=2026-10-05&until=2026-10-01",
		"includeBoilerplate=maybe",
		"limit=0",
		"limit=201",
		"limit=ten",
	} {
		getJSON(t, router, "/links/top?"+query, http.StatusBadRequest, nil)
	}
}

func TestGetLinkTargetReturnsCountsAndOccurrences(t *testing.T) {
	setupControllersTestDB(t)
	podcast, hostID, postID := seedLinkRanking(t)
	router := makeLinksRouter()

	var post struct {
		Target       map[string]any              `json:"target"`
		Parent       map[string]any              `json:"parent"`
		EpisodeCount int64                       `json:"episodeCount"`
		PodcastCount int                         `json:"podcastCount"`
		Podcasts     []db.LinkTargetPodcastCount `json:"podcasts"`
		Occurrences  []db.LinkTargetOccurrence   `json:"occurrences"`
	}
	getJSON(t, router, "/links/targets/"+postID+"?limit=1", http.StatusOK, &post)
	if post.Target["targetId"] != postID || post.Target["kind"] != "post" || post.Parent["targetId"] != hostID {
		t.Fatalf("target/parent = %v / %v", post.Target, post.Parent)
	}
	if post.EpisodeCount != 2 || post.PodcastCount != 1 || len(post.Podcasts) != 1 ||
		post.Podcasts[0].PodcastID != podcast.ID || post.Podcasts[0].Boilerplate {
		t.Fatalf("counts = %d episodes, %d podcasts, %+v", post.EpisodeCount, post.PodcastCount, post.Podcasts)
	}
	if len(post.Occurrences) != 1 || post.Occurrences[0].EpisodeTitle != "Episode F" ||
		post.Occurrences[0].ContextLabel != "Referenced in the show" {
		t.Fatalf("occurrences = %+v, want newest episode only", post.Occurrences)
	}

	var host struct {
		Podcasts []db.LinkTargetPodcastCount `json:"podcasts"`
	}
	getJSON(t, router, "/links/targets/"+hostID, http.StatusOK, &host)
	if len(host.Podcasts) != 1 || !host.Podcasts[0].Boilerplate || host.Podcasts[0].EpisodeCount != 6 {
		t.Fatalf("host podcasts = %+v, want boilerplate in 6 episodes", host.Podcasts)
	}

	getJSON(t, router, "/links/targets/does-not-exist", http.StatusNotFound, nil)
	getJSON(t, router, "/links/targets/"+postID+"?limit=501", http.StatusBadRequest, nil)
}
