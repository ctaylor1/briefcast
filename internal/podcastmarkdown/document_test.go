package podcastmarkdown

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepresentativeExamplesValidate(t *testing.T) {
	paths, err := filepath.Glob("../../docs/podcast-markdown/examples/*.md")
	if err != nil || len(paths) != 3 {
		t.Fatalf("missing representative examples: %v", err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			meta, _, err := Parse(string(content))
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(meta); err != nil {
				t.Fatal(err)
			}
			if ref, ok := meta["provenance"].(map[string]any); ok {
				data, err := os.ReadFile(filepath.Join(filepath.Dir(path), filepath.FromSlash(ref["uri"].(string))))
				if err != nil || Hash(data) != ref["sha256"] {
					t.Fatal("invalid example provenance reference", err)
				}
				var archive struct {
					Metadata Metadata `json:"metadata"`
				}
				if err := json.Unmarshal(data, &archive); err != nil {
					t.Fatal(err)
				}
				if err := Validate(archive.Metadata); err != nil {
					t.Fatal(err)
				}
				if archive.Metadata["document_id"] != meta["document_id"] {
					t.Fatal("example provenance identity mismatch")
				}
			}
		})
	}
}

func testMetadata() Metadata {
	m := Metadata{
		"metadata_schema": "podcast-markdown", "schema_version": "1.2", "document_id": ResourceID("summary", "one"),
		"document_revision": 1, "episode_id": ResourceID("episode", "one"), "generation_id": ResourceID("event", "one"),
		"source_type": "podcast_episode", "content_type": "summary", "title": "A title: \"quoted\"\nnot_a_key: true",
		"generator": Generator, "generator_version": GeneratorVersion, "created": "2026-10-27T12:00:00Z", "updated": "2026-10-27T12:00:00Z",
		"inputs": []any{},
	}
	SetProvenanceStatus(m)
	return m
}

func TestEncodeQuotesAndParsesWithoutMetadataInjection(t *testing.T) {
	meta := testMetadata()
	meta["date_published"] = "2026-10-01"
	meta["rss_guid"] = "001"
	encoded, err := Encode(meta, "## Summary\n\nA qualification.")
	if err != nil {
		t.Fatal(err)
	}
	parsed, body, err := Parse(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if parsed["title"] != meta["title"] || parsed["not_a_key"] != nil || parsed["date_published"] != "2026-10-01" || !strings.HasPrefix(body, "## Summary") {
		t.Fatal("metadata/body changed", parsed)
	}
}

func TestParseRejectsUnsafeOrAmbiguousYAML(t *testing.T) {
	for _, header := range []string{"key: one\nkey: two", "key: &anchor one\nother: *anchor", "key: !!str test"} {
		if _, _, err := Parse("---\n" + header + "\n---\n\ntext"); err == nil {
			t.Fatalf("accepted unsafe header %q", header)
		}
	}
}

func TestParsePreservesUnfamiliarMultilineText(t *testing.T) {
	meta, _, err := Parse("---\ncustom_description: |\n  *ordinary emphasis* is text, not an alias.\n---\n\nBody")
	if err != nil || !strings.Contains(meta["custom_description"].(string), "ordinary emphasis") {
		t.Fatal("lost unfamiliar multiline metadata", err)
	}
}

func TestValidationRejectsFalseCompletenessAndBadHashes(t *testing.T) {
	meta := testMetadata()
	meta["provenance_status"] = "complete"
	if err := Validate(meta); err == nil {
		t.Fatal("accepted missing production history as complete")
	}
	meta = testMetadata()
	meta["inputs"] = []any{map[string]any{"kind": "transcript", "coverage": "full", "id": "id", "revision": 1, "uri": "inputs/test.txt", "sha256": Hash([]byte("one")), "text": "two"}}
	SetProvenanceStatus(meta)
	if err := Validate(meta); err == nil {
		t.Fatal("accepted incorrect input hash")
	}
}

func TestSnapshotReferencesArePortableAndValidated(t *testing.T) {
	meta := testMetadata()
	hash := Hash([]byte("exact source bytes"))
	ref := map[string]any{"sha256": hash, "uri": "inputs/" + hash + ".txt"}
	meta["input_snapshots"] = map[string]any{hash: ref}
	content, err := Encode(meta, "## Summary\n\nReadable Markdown.")
	if err != nil || strings.Contains(content, "exact source bytes") {
		t.Fatal("failed to encode metadata-only snapshot references", err)
	}
	ref["uri"] = "../outside.txt"
	if err := Validate(meta); err == nil {
		t.Fatal("accepted an invalid companion path")
	}
}
