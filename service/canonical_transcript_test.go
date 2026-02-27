package service

import "testing"

// TestBuildCanonicalTranscriptWithoutSpeaker handles the corresponding operation.
func TestBuildCanonicalTranscriptWithoutSpeaker(t *testing.T) {
	segments := []canonicalTranscriptSegment{
		{Text: "  Hello   world.  "},
		{Text: "Second    paragraph   here"},
	}

	got := buildCanonicalTranscript(segments)
	want := "Hello world.\n\nSecond paragraph here"
	if got != want {
		t.Fatalf("expected canonical transcript %q, got %q", want, got)
	}
}

// TestBuildCanonicalTranscriptWithSpeakerMerging handles the corresponding operation.
func TestBuildCanonicalTranscriptWithSpeakerMerging(t *testing.T) {
	segments := []canonicalTranscriptSegment{
		{Speaker: "speaker_00", Text: "Hello", Start: 0, End: 1, HasStart: true, HasEnd: true},
		{Speaker: "SPEAKER_00", Text: "world", Start: 1.5, End: 2.0, HasStart: true, HasEnd: true},
		{Speaker: "SPEAKER_00", Text: "after pause", Start: 4.2, End: 5.1, HasStart: true, HasEnd: true},
		{Speaker: "speaker_01", Text: "reply", Start: 5.3, End: 6.1, HasStart: true, HasEnd: true},
	}

	got := buildCanonicalTranscript(segments)
	want := "SPEAKER_00: Hello world\nSPEAKER_00: after pause\nSPEAKER_01: reply"
	if got != want {
		t.Fatalf("expected canonical transcript %q, got %q", want, got)
	}
}

// TestBuildCanonicalTranscriptNormalizesSpacingAndPunctuation handles the corresponding operation.
func TestBuildCanonicalTranscriptNormalizesSpacingAndPunctuation(t *testing.T) {
	segments := []canonicalTranscriptSegment{
		{Text: "  Hi   ,   there   ! "},
		{Text: "\tThis  is   a   test ;  done ."},
	}

	got := buildCanonicalTranscript(segments)
	want := "Hi, there!\n\nThis is a test; done."
	if got != want {
		t.Fatalf("expected canonical transcript %q, got %q", want, got)
	}
}

// TestBuildCanonicalTranscriptPrefersPreAlignSegments handles the corresponding operation.
func TestBuildCanonicalTranscriptPrefersPreAlignSegments(t *testing.T) {
	raw := `{
		"segments_pre_align":[{"start":0,"end":1,"text":" pre align   text "}],
		"segments":[{"start":0,"end":1,"text":"post align text","words":[{"word":"post"}]}]
	}`

	got := buildCanonicalTranscriptFromTranscriptJSON(raw)
	want := "pre align text"
	if got != want {
		t.Fatalf("expected canonical transcript %q, got %q", want, got)
	}
}
