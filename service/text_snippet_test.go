package service

import (
	"strings"
	"testing"
)

func TestContainsTerm(t *testing.T) {
	if !containsTerm("Hello World", "world") {
		t.Fatalf("expected case-insensitive match")
	}
	if containsTerm("", "world") {
		t.Fatalf("expected empty value to return false")
	}
	if containsTerm("hello", "") {
		t.Fatalf("expected empty term to return false")
	}
}

func TestPickSnippet(t *testing.T) {
	if got := pickSnippet("", "Fallback body", "body"); got == "" {
		t.Fatalf("expected fallback snippet")
	}
	if got := pickSnippet("Primary body", "Fallback", "primary"); got == "" {
		t.Fatalf("expected primary snippet")
	}
	if got := pickSnippet("", "   ", "x"); got != "" {
		t.Fatalf("expected empty snippet, got %q", got)
	}
}

func TestMakeSnippet(t *testing.T) {
	text := "0123456789abcdefghijklmnopqrstuvwxyz"

	noMatch := makeSnippet(text, "zzz", 10)
	if noMatch != "0123456789…" {
		t.Fatalf("unexpected no-match snippet: %q", noMatch)
	}

	match := makeSnippet(text, "mnop", 12)
	if match == "" {
		t.Fatalf("expected non-empty snippet")
	}
	if !strings.HasPrefix(match, "…") {
		t.Fatalf("expected snippet to be prefixed with ellipsis, got %q", match)
	}
	if !strings.HasSuffix(match, "…") {
		t.Fatalf("expected snippet to be suffixed with ellipsis, got %q", match)
	}
}
