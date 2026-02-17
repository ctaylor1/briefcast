package sanitize

import (
	"strings"
	"testing"

	parser "golang.org/x/net/html"
)

func TestHTMLAllowingCleansDangerousAttributes(t *testing.T) {
	input := `<div id="root" onclick="evil()"><a href="javascript:alert(1)" rel="noopener">link</a><img src="https://img.example/pic.jpg" onerror="evil()"/><script>alert(1)</script></div>`
	output, err := HTMLAllowing(input)
	if err != nil {
		t.Fatalf("HTMLAllowing returned error: %v", err)
	}

	if strings.Contains(output, "onclick") {
		t.Fatalf("expected onclick to be removed, got %q", output)
	}
	if strings.Contains(output, "javascript:") || strings.Contains(output, "href=") {
		t.Fatalf("expected unsafe href to be removed, got %q", output)
	}
	if strings.Contains(output, "<script") || strings.Contains(output, "alert(1)") {
		t.Fatalf("expected script tag and contents to be removed, got %q", output)
	}
	if !strings.Contains(output, `src="https://img.example/pic.jpg"`) {
		t.Fatalf("expected img src to be preserved, got %q", output)
	}
	if !strings.Contains(output, "<a") || !strings.Contains(output, "</a>") {
		t.Fatalf("expected anchor tag to remain, got %q", output)
	}
}

func TestHTMLAllowingCustomAllowList(t *testing.T) {
	input := `<p>paragraph</p><span>inline</span>`
	output, err := HTMLAllowing(input, []string{"span"}, []string{})
	if err != nil {
		t.Fatalf("HTMLAllowing returned error: %v", err)
	}
	if strings.Contains(output, "<p>") || strings.Contains(output, "</p>") {
		t.Fatalf("expected p tags removed, got %q", output)
	}
	if !strings.Contains(output, "<span>inline</span>") {
		t.Fatalf("expected span to remain, got %q", output)
	}
}

func TestHTMLStripsTagsAndUnescapesCommonEntities(t *testing.T) {
	input := `<p>Tom &amp; Jerry</p><br />x&#8217;y`
	output := HTML(input)
	expected := "Tom & Jerry\n\nx'y"
	if output != expected {
		t.Fatalf("expected %q, got %q", expected, output)
	}
}

func TestPathNameAndBaseName(t *testing.T) {
	if got := Path("Cafe&Tea"); got != "cafe-tea" {
		t.Fatalf("expected %q, got %q", "cafe-tea", got)
	}

	if got := Name("My.Show/Épisode_1.mp3"); got != "My-Show-Episode-1-mp3" {
		t.Fatalf("unexpected Name output: %q", got)
	}

	if got := BaseName("A/B.C_1"); got != "A-B-C-1" {
		t.Fatalf("unexpected BaseName output: %q", got)
	}
}

func TestAccents(t *testing.T) {
	if got := Accents("Łódź ß"); got != "Lodź ss" {
		t.Fatalf("expected %q, got %q", "Lodź ss", got)
	}
}

func TestCleanAttributes(t *testing.T) {
	attrs := []parser.Attribute{
		{Key: "class", Val: "episode"},
		{Key: "href", Val: "javascript:alert(1)"},
		{Key: "src", Val: "data:text/plain,boom"},
		{Key: "href", Val: "https://example.com"},
	}

	cleaned := cleanAttributes(attrs, []string{"class", "href", "src"})

	if len(cleaned) != 2 {
		t.Fatalf("expected 2 attributes after cleaning, got %d", len(cleaned))
	}
	if cleaned[0].Key != "class" || cleaned[0].Val != "episode" {
		t.Fatalf("unexpected first attribute: %+v", cleaned[0])
	}
	if cleaned[1].Key != "href" || cleaned[1].Val != "https://example.com" {
		t.Fatalf("unexpected second attribute: %+v", cleaned[1])
	}
}

func TestIncludes(t *testing.T) {
	values := []string{"one", "two", "three"}
	if !includes(values, "two") {
		t.Fatalf("expected includes to return true")
	}
	if includes(values, "four") {
		t.Fatalf("expected includes to return false")
	}
}
