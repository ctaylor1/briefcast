package feedmeta

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestNormalizeLink_AllInShowNotesFixture characterizes normalization of a
// real show note block: hosts, show accounts, credits, and referenced posts
// and articles.
func TestNormalizeLink_AllInShowNotesFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "allin_show_notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	links := ExtractShowNoteLinks("", string(raw))

	want := []struct {
		key       string
		kind      LinkKind
		platform  string
		parentKey string
	}{
		{"x:chamath", LinkKindProfile, "x", ""},
		{"x:jason", LinkKindProfile, "x", ""},
		{"x:davidsacks", LinkKindProfile, "x", ""},
		{"x:friedberg", LinkKindProfile, "x", ""},
		{"x:theallinpod", LinkKindProfile, "x", ""},
		{"instagram:theallinpod", LinkKindProfile, "instagram", ""},
		{"tiktok:allin", LinkKindProfile, "tiktok", ""},
		{"linkedin:company/allinpod", LinkKindProfile, "linkedin", ""},
		{"short:rb.gy/tppkzl", LinkKindShortener, "web", ""},
		{"x:yung_spielburg", LinkKindProfile, "x", ""},
		{"x:thezacheffect", LinkKindProfile, "x", ""},
		{"web:semafor.com/article/10/28/2026/all-in-podcast-cools-on-super-intelligence", LinkKindWeb, "web", ""},
		{"x:status:2115458726396310008", LinkKindPost, "x", "x:andrewbenson"},
		{"web:nbcnews.com/business/markets/anthropic-releases-ipo-filing-ahead-highly-anticipated-stock-debut-rcna605676", LinkKindWeb, "web", ""},
		{"web:reuters.com/business/anthropic-recorded-net-loss-over-80-billion-first-half-this-year-wsj-reports-2026-10-20", LinkKindWeb, "web", ""},
		{"x:status:2114785991462428811", LinkKindPost, "x", "x:rdominguezibar"},
		{"x:status:2115121906630238287", LinkKindPost, "x", "x:jaminball"},
		{"web:vox.com/future-perfect/507875/anthropic-ipo-stop-ai-race-safety", LinkKindWeb, "web", ""},
		{"web:anthropic.com/institute/measuring-pace-of-ai-development", LinkKindWeb, "web", ""},
		{"web:nbcnews.com/business/markets/alphabet-google-stock-ai-advertising-rcna606322", LinkKindWeb, "web", ""},
		{"x:status:2115539079999971384", LinkKindPost, "x", "x:economyapp"},
		{"web:fortune.com/2026/10/28/alphabet-google-stock-drops-cloud-revenue-doubles-free-cash-flow-negative", LinkKindWeb, "web", ""},
		{"x:status:2115541399898571180", LinkKindPost, "x", "x:couch_investor"},
		{"x:status:2112702741995037179", LinkKindPost, "x", "x:geiger_capital"},
		{"x:status:2089729804941594660", LinkKindPost, "x", "x:patrickruffini"},
		{"x:status:2080229375773941871", LinkKindPost, "x", "x:maxanderson"},
		{"web:natesilver.net/p/trump-approval-ratings-nate-silver-bulletin", LinkKindWeb, "web", ""},
	}
	if len(links) != len(want) {
		t.Fatalf("extracted %d links, want %d", len(links), len(want))
	}
	for i, link := range links {
		got, err := NormalizeLink(link.URL)
		if err != nil {
			t.Fatalf("NormalizeLink(%q) error: %v", link.URL, err)
		}
		w := want[i]
		if got.Key != w.key || got.Kind != w.kind || got.Platform != w.platform || got.ParentKey != w.parentKey {
			t.Errorf("NormalizeLink(%q) = {Key:%q Kind:%q Platform:%q Parent:%q}, want {%q %q %q %q}",
				link.URL, got.Key, got.Kind, got.Platform, got.ParentKey, w.key, w.kind, w.platform, w.parentKey)
		}
	}
}

func TestNormalizeLink_EquivalentURLsShareKey(t *testing.T) {
	groups := map[string][]string{
		"x:jason": {
			"https://x.com/Jason",
			"https://x.com/jason/",
			"http://www.x.com/jason#bio",
			"https://twitter.com/jason?s=20",
			"https://mobile.twitter.com/Jason/with_replies",
		},
		"x:status:2115458726396310008": {
			"https://x.com/AndrewBenson/status/2115458726396310008",
			"https://twitter.com/andrewbenson/status/2115458726396310008?s=46&t=abc",
			"https://x.com/AndrewBenson/status/2115458726396310008/photo/1",
			"https://x.com/i/web/status/2115458726396310008",
		},
		"youtube:video:dQw4w9WgXcQ": {
			"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			"https://youtu.be/dQw4w9WgXcQ?si=share123",
			"https://m.youtube.com/watch?v=dQw4w9WgXcQ&t=42",
			"https://youtube.com/shorts/dQw4w9WgXcQ",
		},
		"web:example.com/article?a=1&b=2": {
			"https://example.com/article?b=2&a=1",
			"http://www.example.com/article/?a=1&b=2&utm_source=podcast&utm_medium=shownotes",
			"https://EXAMPLE.com:443/article?a=1&fbclid=xyz&b=2#section",
		},
		"instagram:theallinpod": {
			"https://www.instagram.com/theallinpod",
			"https://instagram.com/TheAllInPod/?igsh=abc",
		},
		"linkedin:company/allinpod": {
			"https://www.linkedin.com/company/allinpod",
			"https://uk.linkedin.com/company/AllInPod/about/",
		},
	}
	for wantKey, urls := range groups {
		for _, raw := range urls {
			got, err := NormalizeLink(raw)
			if err != nil {
				t.Errorf("NormalizeLink(%q) error: %v", raw, err)
				continue
			}
			if got.Key != wantKey {
				t.Errorf("NormalizeLink(%q).Key = %q, want %q", raw, got.Key, wantKey)
			}
		}
	}
}

func TestNormalizeLink_DistinctTargetsKeepDistinctKeys(t *testing.T) {
	pairs := [][2]string{
		// Web paths are case-sensitive.
		{"https://example.com/Page", "https://example.com/page"},
		// Meaningful query params are kept.
		{"https://example.com/watch?id=1", "https://example.com/watch?id=2"},
		// Shortener codes are case-sensitive.
		{"https://bit.ly/AbC", "https://bit.ly/abc"},
		// A post is not its author's profile.
		{"https://x.com/jason/status/1", "https://x.com/jason"},
		// Non-default ports are part of the target.
		{"https://example.com:8443/a", "https://example.com/a"},
	}
	for _, pair := range pairs {
		a, errA := NormalizeLink(pair[0])
		b, errB := NormalizeLink(pair[1])
		if errA != nil || errB != nil {
			t.Fatalf("unexpected errors: %v, %v", errA, errB)
		}
		if a.Key == b.Key {
			t.Errorf("%q and %q both normalized to %q", pair[0], pair[1], a.Key)
		}
	}
}

func TestNormalizeLink_Classification(t *testing.T) {
	cases := []struct {
		raw          string
		key          string
		canonicalURL string
		kind         LinkKind
		platform     string
		handle       string
		parentKey    string
		domain       string
	}{
		{"https://x.com/AndrewBenson/status/2115458726396310008", "x:status:2115458726396310008", "https://x.com/andrewbenson/status/2115458726396310008", LinkKindPost, "x", "andrewbenson", "x:andrewbenson", "x.com"},
		{"https://x.com/i/status/123", "x:status:123", "https://x.com/i/status/123", LinkKindPost, "x", "", "", "x.com"},
		{"https://twitter.com/home", "web:x.com/home", "https://x.com/home", LinkKindWeb, "x", "", "", "x.com"},
		{"https://x.com/search?q=allin", "web:x.com/search?q=allin", "https://x.com/search?q=allin", LinkKindWeb, "x", "", "", "x.com"},
		{"https://www.instagram.com/p/CxYz_12-ab/", "instagram:post:CxYz_12-ab", "https://instagram.com/p/CxYz_12-ab", LinkKindPost, "instagram", "", "", "instagram.com"},
		{"https://www.instagram.com/theallinpod/reel/CxYz/", "instagram:post:CxYz", "https://instagram.com/p/CxYz", LinkKindPost, "instagram", "theallinpod", "instagram:theallinpod", "instagram.com"},
		{"https://www.tiktok.com/@allin/video/7300000000000000000?lang=en", "tiktok:video:7300000000000000000", "https://tiktok.com/@allin/video/7300000000000000000", LinkKindPost, "tiktok", "allin", "tiktok:allin", "tiktok.com"},
		{"https://vm.tiktok.com/ZMabc123/", "short:vm.tiktok.com/ZMabc123", "https://vm.tiktok.com/ZMabc123", LinkKindShortener, "web", "", "", "vm.tiktok.com"},
		{"https://www.linkedin.com/in/Jane-Doe-123/", "linkedin:in/jane-doe-123", "https://linkedin.com/in/jane-doe-123", LinkKindProfile, "linkedin", "jane-doe-123", "", "linkedin.com"},
		{"https://www.linkedin.com/feed/update/urn:li:activity:7000/", "linkedin:post:urn:li:activity:7000", "https://linkedin.com/feed/update/urn:li:activity:7000", LinkKindPost, "linkedin", "", "", "linkedin.com"},
		{"https://www.youtube.com/@AllInPodcast/videos", "youtube:@allinpodcast", "https://youtube.com/@allinpodcast", LinkKindProfile, "youtube", "allinpodcast", "", "youtube.com"},
		{"https://www.youtube.com/channel/UCESLZhusAkFfsNsApnjF_Cg", "youtube:channel:UCESLZhusAkFfsNsApnjF_Cg", "https://youtube.com/channel/UCESLZhusAkFfsNsApnjF_Cg", LinkKindProfile, "youtube", "", "", "youtube.com"},
		{"https://www.threads.net/@TheAllInPod/post/C1abc", "threads:post:C1abc", "https://threads.net/@theallinpod/post/C1abc", LinkKindPost, "threads", "theallinpod", "threads:theallinpod", "threads.net"},
		{"https://bsky.app/profile/Jay.bsky.team/post/3abc", "bluesky:post:jay.bsky.team/3abc", "https://bsky.app/profile/jay.bsky.team/post/3abc", LinkKindPost, "bluesky", "jay.bsky.team", "bluesky:jay.bsky.team", "bsky.app"},
		{"https://rb.gy/tppkzl", "short:rb.gy/tppkzl", "https://rb.gy/tppkzl", LinkKindShortener, "web", "", "", "rb.gy"},
		{"http://www.natesilver.net/p/post/", "web:natesilver.net/p/post", "https://natesilver.net/p/post", LinkKindWeb, "web", "", "", "natesilver.net"},
		{"https://example.com", "web:example.com", "https://example.com", LinkKindWeb, "web", "", "", "example.com"},
		{"https://example.com:8443/a", "web:example.com:8443/a", "https://example.com:8443/a", LinkKindWeb, "web", "", "", "example.com"},
	}
	for _, tc := range cases {
		got, err := NormalizeLink(tc.raw)
		if err != nil {
			t.Errorf("NormalizeLink(%q) error: %v", tc.raw, err)
			continue
		}
		want := NormalizedLink{
			Key: tc.key, CanonicalURL: tc.canonicalURL, Kind: tc.kind, Platform: tc.platform,
			Handle: tc.handle, ParentKey: tc.parentKey, Domain: tc.domain,
		}
		if got != want {
			t.Errorf("NormalizeLink(%q)\n got  %+v\n want %+v", tc.raw, got, want)
		}
	}
}

func TestParentProfile(t *testing.T) {
	posts := []string{
		"https://x.com/AndrewBenson/status/2115458726396310008",
		"https://www.instagram.com/theallinpod/reel/CxYz/",
		"https://www.tiktok.com/@allin/video/7300000000000000000",
		"https://www.threads.net/@TheAllInPod/post/C1abc",
		"https://bsky.app/profile/Jay.bsky.team/post/3abc",
	}
	for _, raw := range posts {
		post, err := NormalizeLink(raw)
		if err != nil {
			t.Fatalf("NormalizeLink(%q) error: %v", raw, err)
		}
		parent, ok := ParentProfile(post)
		if !ok {
			t.Errorf("ParentProfile(%q) reported no parent", raw)
			continue
		}
		if parent.Key != post.ParentKey || parent.Kind != LinkKindProfile || parent.Platform != post.Platform {
			t.Errorf("ParentProfile(%q) = %+v, want profile with key %q", raw, parent, post.ParentKey)
		}
	}

	for _, raw := range []string{
		"https://x.com/i/status/123", // author unknown
		"https://x.com/jason",        // already a profile
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://example.com/article",
	} {
		link, err := NormalizeLink(raw)
		if err != nil {
			t.Fatalf("NormalizeLink(%q) error: %v", raw, err)
		}
		if parent, ok := ParentProfile(link); ok {
			t.Errorf("ParentProfile(%q) = %+v, want none", raw, parent)
		}
	}
}

func TestNormalizeLink_RejectsUnsupported(t *testing.T) {
	for _, raw := range []string{
		"",
		"not a url",
		"mailto:hosts@example.com",
		"javascript:alert(1)",
		"ftp://files.example.com/a",
		"https:///no-host",
		"://bad",
	} {
		if _, err := NormalizeLink(raw); !errors.Is(err, ErrUnsupportedLink) {
			t.Errorf("NormalizeLink(%q) error = %v, want ErrUnsupportedLink", raw, err)
		}
	}
}
