package service

import (
	"errors"
	"strings"
	"testing"
)

func TestAddOpmlRejectsOversizedPayload(t *testing.T) {
	err := AddOpml(strings.Repeat("x", MaxOPMLContentBytes+1))
	if !errors.Is(err, ErrOPMLTooLarge) {
		t.Fatalf("expected oversized OPML error, got %v", err)
	}
}
