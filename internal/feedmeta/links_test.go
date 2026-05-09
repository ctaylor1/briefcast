package feedmeta

import (
	"testing"
)

func TestExtractLinksFromHTML_BasicAnchors(t *testing.T) {
	html := `<p>Check out <a href="https://example.com/article">this article</a> and
		<a href="https://blog.example.org/post/1">another post</a>.</p>`
	links := ExtractLinksFromHTML(html)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0].URL != "https://example.com/article" {
		t.Errorf("link 0 URL = %q", links[0].URL)
	}
	if links[0].Title != "this article" {
		t.Errorf("link 0 Title = %q", links[0].Title)
	}
	if links[0].Domain != "example.com" {
		t.Errorf("link 0 Domain = %q", links[0].Domain)
	}
	if links[0].Position != 0 {
		t.Errorf("link 0 Position = %d", links[0].Position)
	}
	if links[1].Position != 1 {
		t.Errorf("link 1 Position = %d", links[1].Position)
	}
}

func TestExtractLinksFromHTML_DeduplicatesURLs(t *testing.T) {
	html := `<a href="https://example.com">first</a> <a href="https://example.com">second</a>`
	links := ExtractLinksFromHTML(html)
	if len(links) != 1 {
		t.Fatalf("expected 1 link after dedup, got %d", len(links))
	}
	if links[0].Title != "first" {
		t.Errorf("expected first occurrence title, got %q", links[0].Title)
	}
}

func TestExtractLinksFromHTML_FiltersNoise(t *testing.T) {
	html := `<a href="https://example.com/article">good</a>
		<a href="https://podcasts.apple.com/us/podcast/123">apple</a>
		<a href="https://open.spotify.com/show/abc">spotify</a>
		<a href="https://example.com/audio.mp3">audio</a>
		<a href="mailto:test@example.com">email</a>
		<a href="javascript:void(0)">js</a>`
	links := ExtractLinksFromHTML(html)
	if len(links) != 1 {
		t.Fatalf("expected 1 link after noise filtering, got %d", len(links))
	}
	if links[0].URL != "https://example.com/article" {
		t.Errorf("expected article link, got %q", links[0].URL)
	}
}

func TestExtractLinksFromHTML_StripWWW(t *testing.T) {
	html := `<a href="https://www.nytimes.com/article">NYT</a>`
	links := ExtractLinksFromHTML(html)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].Domain != "nytimes.com" {
		t.Errorf("expected nytimes.com, got %q", links[0].Domain)
	}
}

func TestExtractLinksFromHTML_Empty(t *testing.T) {
	if links := ExtractLinksFromHTML(""); links != nil {
		t.Fatalf("expected nil, got %d links", len(links))
	}
	if links := ExtractLinksFromHTML("   "); links != nil {
		t.Fatalf("expected nil, got %d links", len(links))
	}
}

func TestExtractBareURLs_FindsURLs(t *testing.T) {
	text := "Check out https://example.com/article and also https://blog.test.org/post."
	links := ExtractBareURLs(text)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0].URL != "https://example.com/article" {
		t.Errorf("link 0 URL = %q", links[0].URL)
	}
	if links[1].URL != "https://blog.test.org/post" {
		t.Errorf("link 1 URL = %q", links[1].URL)
	}
}

func TestExtractBareURLs_TrimsPunctuation(t *testing.T) {
	text := "Visit https://example.com/page, or https://example.com/other."
	links := ExtractBareURLs(text)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0].URL != "https://example.com/page" {
		t.Errorf("expected trimmed URL, got %q", links[0].URL)
	}
}

func TestExtractBareURLs_Empty(t *testing.T) {
	if links := ExtractBareURLs(""); links != nil {
		t.Fatalf("expected nil, got %d links", len(links))
	}
}

func TestExtractShowNoteLinks_CombinesHTMLAndText(t *testing.T) {
	html := `<a href="https://example.com/a">link A</a>`
	text := "Also see https://example.com/a and https://other.com/b"
	links := ExtractShowNoteLinks(html, text)
	if len(links) != 2 {
		t.Fatalf("expected 2 links (1 HTML + 1 bare), got %d", len(links))
	}
	if links[0].URL != "https://example.com/a" {
		t.Errorf("first link should be from HTML, got %q", links[0].URL)
	}
	if links[0].Title != "link A" {
		t.Errorf("first link should have HTML title, got %q", links[0].Title)
	}
	if links[1].URL != "https://other.com/b" {
		t.Errorf("second link should be bare URL, got %q", links[1].URL)
	}
}

func TestExtractShowNoteLinks_DeduplicatesAcrossSources(t *testing.T) {
	html := `<a href="https://example.com">Example</a>`
	text := "Visit https://example.com for more info."
	links := ExtractShowNoteLinks(html, text)
	if len(links) != 1 {
		t.Fatalf("expected 1 link after cross-source dedup, got %d", len(links))
	}
}

func TestExtractDomain(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://www.example.com/path", "example.com"},
		{"https://sub.domain.org/page", "sub.domain.org"},
		{"http://EXAMPLE.COM", "example.com"},
		{"not-a-url", ""},
	}
	for _, tc := range cases {
		got := extractDomain(tc.input)
		if got != tc.want {
			t.Errorf("extractDomain(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestIsNoiseURL(t *testing.T) {
	cases := []struct {
		url   string
		noise bool
	}{
		{"https://example.com/article", false},
		{"https://podcasts.apple.com/us/podcast/123", true},
		{"https://open.spotify.com/show/abc", true},
		{"https://example.com/file.mp3", true},
		{"mailto:test@example.com", true},
		{"javascript:void(0)", true},
		{"ftp://files.example.com/data", true},
	}
	for _, tc := range cases {
		got := isNoiseURL(tc.url)
		if got != tc.noise {
			t.Errorf("isNoiseURL(%q) = %v, want %v", tc.url, got, tc.noise)
		}
	}
}
