package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/db"
	pm "github.com/ctaylor1/briefcast/internal/podcastmarkdown"
)

const markdownProvenanceSchema = "briefcast-podcast-provenance"

func markdownProvenanceDir() string {
	return filepath.Join(resolveExportDir(), "provenance", "podcast-markdown")
}

// The sidecar contains the complete metadata before presentation is shortened.
// It has no output-byte hash of itself and never embeds source payload text.
type markdownProvenance struct {
	Schema   string      `json:"provenance_schema"`
	Version  string      `json:"schema_version"`
	Metadata pm.Metadata `json:"metadata"`
}

func markdownRevisionDirectory(previous db.MarkdownRevision) (string, error) {
	if previous.ExportPath != "" {
		path := filepath.Join(resolveExportDir(), filepath.FromSlash(previous.ExportPath))
		if !IsPathWithinAssetsDir(path) {
			return "", fmt.Errorf("Markdown export path is outside assets")
		}
		return filepath.Dir(path), nil
	}
	meta, _, err := pm.Parse(previous.Content)
	if err != nil {
		return "", err
	}
	published, _ := time.Parse("2006-01-02", fmt.Sprint(meta["date_published"]))
	item := db.PodcastItem{Podcast: db.Podcast{Title: fmt.Sprint(meta["podcast"])}, PubDate: published}
	return markdownDirectory(&item, previous.ContentType), nil
}

func markdownRevisionMetadata(previous db.MarkdownRevision) (pm.Metadata, error) {
	meta, _, err := pm.Parse(previous.Content)
	if err != nil {
		return nil, err
	}
	dir, err := markdownRevisionDirectory(previous)
	if err != nil {
		return nil, err
	}
	return readMarkdownProvenance(meta, dir)
}

func readMarkdownProvenance(meta pm.Metadata, dir string) (pm.Metadata, error) {
	if meta["provenance"] == nil {
		return meta, nil
	} // Earlier inline revisions.
	if err := pm.Validate(meta); err != nil {
		return nil, err
	}
	ref := meta["provenance"].(map[string]any)
	path := filepath.Join(dir, filepath.FromSlash(ref["uri"].(string)))
	if !isPathWithinDir(path, markdownProvenanceDir()) {
		return nil, fmt.Errorf("provenance reference is outside the provenance directory")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Markdown provenance: %w", err)
	}
	if pm.Hash(data) != ref["sha256"] {
		return nil, fmt.Errorf("provenance file hash mismatch")
	}
	var sidecar markdownProvenance
	if json.Unmarshal(data, &sidecar) != nil || sidecar.Schema != markdownProvenanceSchema || sidecar.Version != "1" {
		return nil, fmt.Errorf("invalid Markdown provenance file")
	}
	full := sidecar.Metadata
	if full["provenance"] != nil || full["document_id"] != meta["document_id"] || fmt.Sprint(full["document_revision"]) != fmt.Sprint(meta["document_revision"]) {
		return nil, fmt.Errorf("provenance document identity mismatch")
	}
	if err := pm.Validate(full); err != nil {
		return nil, err
	}
	if err := verifyProvenanceSnapshots(full); err != nil {
		return nil, err
	}
	return full, nil
}

func verifyProvenanceSnapshots(meta pm.Metadata) error {
	snapshots, _ := meta["input_snapshots"].(map[string]any)
	for hash, raw := range snapshots {
		ref, ok := raw.(map[string]any)
		if !ok || ref["uri"] != "inputs/"+hash+".txt" || ref["sha256"] != hash {
			return fmt.Errorf("invalid provenance input reference")
		}
		path := filepath.Join(markdownProvenanceDir(), "inputs", hash+".txt")
		if !isPathWithinDir(path, markdownProvenanceDir()) {
			return fmt.Errorf("input reference is outside provenance directory")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read provenance input companion: %w", err)
		}
		if pm.Hash(data) != hash {
			return fmt.Errorf("existing input snapshot hash mismatch")
		}
	}
	return nil
}

func publishMarkdownArtifact(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := publishMarkdownFile(path, data); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if pm.Hash(existing) != pm.Hash(data) {
			return fmt.Errorf("existing provenance artifact hash mismatch")
		}
	}
	return nil
}

func separateMarkdownProvenance(meta pm.Metadata, item *db.PodcastItem, kind string, previous db.MarkdownRevision) error {
	root := markdownProvenanceDir()
	if err := externalizeMarkdownSnapshots(meta, root, previous); err != nil {
		return err
	}
	if err := verifyProvenanceSnapshots(meta); err != nil {
		return err
	}
	data, err := json.MarshalIndent(markdownProvenance{Schema: markdownProvenanceSchema, Version: "1", Metadata: meta}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	filename := fmt.Sprintf("%s.r%v.%s.json", strings.TrimPrefix(meta["document_id"].(string), "urn:uuid:"), meta["document_revision"], pm.Hash(data)[:12])
	path := filepath.Join(root, filename)
	if err := publishMarkdownArtifact(path, data); err != nil {
		return err
	}
	// Keep exact source IDs, hashes and coverage in the note. The archive retains
	// all additional input attributes and prompt/settings/processing history.
	inputs := []any{}
	for _, raw := range meta["inputs"].([]any) {
		input := raw.(map[string]any)
		lean := map[string]any{}
		for _, key := range []string{"kind", "id", "revision", "uri", "sha256", "coverage", "locators", "source_document", "episode_coverage"} {
			if value, ok := input[key]; ok {
				lean[key] = value
			}
		}
		if uri, ok := lean["uri"].(string); ok && strings.HasPrefix(uri, "inputs/") {
			// Only our own content-addressed companions are relocated. Publisher
			// web URLs and other source locations retain their original meaning.
			hash, _ := input["sha256"].(string)
			if uri == "inputs/"+hash+".txt" {
				relative, err := filepath.Rel(markdownDirectory(item, kind), filepath.Join(root, filepath.FromSlash(uri)))
				if err != nil {
					return err
				}
				lean["uri"] = filepath.ToSlash(relative)
			}
		}
		inputs = append(inputs, lean)
	}
	meta["inputs"] = inputs
	for _, key := range []string{"input_snapshots", "processing_events", "prompt", "generation_settings", "legacy_system_prompt", "media_source", "legacy_frontmatter_source", "external_file_observation", "export_input_sha256"} {
		delete(meta, key)
	}
	relative, err := filepath.Rel(markdownDirectory(item, kind), path)
	if err != nil {
		return err
	}
	meta["provenance"] = map[string]any{"uri": filepath.ToSlash(relative), "sha256": pm.Hash(data)}
	return nil
}
