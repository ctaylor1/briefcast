package feedmeta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func labelsOf(links []ShowNoteLinkData) []string {
	labels := make([]string, len(links))
	for i, link := range links {
		labels[i] = link.ContextLabel
	}
	return labels
}

func assertLabels(t *testing.T, links []ShowNoteLinkData, want []string) {
	t.Helper()
	got := labelsOf(links)
	if len(got) != len(want) {
		t.Fatalf("got %d links %q, want %d labels %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("link %d (%s) label = %q, want %q", i, links[i].URL, got[i], want[i])
		}
	}
}

func TestContextLabels_AllInShowNotesFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "allin_show_notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	links := ExtractShowNoteLinks("", string(raw))

	want := []string{"", "", "", "", // hosts, before any label
		"Follow on X", "Follow on Instagram", "Follow on TikTok", "Follow on LinkedIn",
		"Intro Music Credit", "Intro Music Credit", // rb.gy and @yung_spielburg
		"Intro Video Credit",
	}
	for len(want) < 27 {
		want = append(want, "Referenced in the show")
	}
	assertLabels(t, links, want)
}

func TestContextLabels_HTMLHeadingsAndColonLabels(t *testing.T) {
	notes := `<p>Thanks for listening.</p>
		<h3>Referenced in the show</h3>
		<ul>
			<li><a href="https://example.com/a">Article A</a> — a great piece</li>
			<li><a href="https://example.com/b">Article B</a></li>
		</ul>
		<p><strong>Sponsors:</strong> <a href="https://sponsor.example">Sponsor</a></p>`
	assertLabels(t, ExtractLinksFromHTML(notes), []string{
		"Referenced in the show", "Referenced in the show", "Sponsors",
	})
}

func TestContextLabels_LinkBeforeAnyLabelHasNone(t *testing.T) {
	notes := `<p>Hosts <a href="https://x.com/a">A</a></p><p>Links:</p><p><a href="https://example.com">B</a></p>`
	assertLabels(t, ExtractLinksFromHTML(notes), []string{"", "Links"})
}

func TestContextLabels_UsesLastSentenceBeforeColon(t *testing.T) {
	text := "Great episode today. We covered a lot! Links mentioned: https://example.com/a"
	assertLabels(t, ExtractBareURLs(text), []string{"Links mentioned"})
}

func TestContextLabels_IgnoresOverlongProseEndingInColon(t *testing.T) {
	prose := strings.Repeat("word ", 40) + "and then he said:"
	text := "Sources: https://example.com/a " + prose + " https://example.com/b"
	assertLabels(t, ExtractBareURLs(text), []string{"Sources", "Sources"})
}

func TestContextLabels_UnescapesEntitiesAndCollapsesWhitespace(t *testing.T) {
	text := "Q&amp;A   links:\n https://example.com/a"
	assertLabels(t, ExtractBareURLs(text), []string{"Q&A links"})
}

func TestContextLabels_SkippedAnchorTextIsNotALabel(t *testing.T) {
	notes := `<p>Listen:</p>
		<a href="https://podcasts.apple.com/us/podcast/123">Apple Podcasts:</a>
		<a href="https://example.com/site">Site</a>`
	links := ExtractLinksFromHTML(notes)
	assertLabels(t, links, []string{"Listen"})
	if links[0].URL != "https://example.com/site" {
		t.Fatalf("unexpected link %q", links[0].URL)
	}
}

func TestContextLabels_PlainTextLines(t *testing.T) {
	text := "Hosts\nhttps://x.com/a\nReferenced:\nhttps://example.com/b\nhttps://example.com/c"
	assertLabels(t, ExtractBareURLs(text), []string{"", "Referenced", "Referenced"})
}

func TestContextLabels_DuplicateURLKeepsFirstLabel(t *testing.T) {
	text := "Hosts: https://x.com/a Referenced: https://x.com/a https://example.com/b"
	links := ExtractBareURLs(text)
	assertLabels(t, links, []string{"Hosts", "Referenced"})
}
