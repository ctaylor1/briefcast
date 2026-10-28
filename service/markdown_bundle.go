package service

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ctaylor1/briefcast/db"
	pm "github.com/ctaylor1/briefcast/internal/podcastmarkdown"
)

// EpisodeMarkdownBundle preserves the relative layout of Markdown, provenance,
// and exact inputs when exporting away from the application's assets directory.
func EpisodeMarkdownBundle(item *db.PodcastItem, kind string) ([]byte, string, error) {
	content, err := EpisodeMarkdown(item, kind)
	if err != nil {
		return nil, "", err
	}
	if content == "" {
		return nil, "", fmt.Errorf("no Markdown available")
	}
	meta, _, err := pm.Parse(content)
	if err != nil {
		return nil, "", err
	}
	dir := markdownDirectory(item, kind)
	full, err := readMarkdownProvenance(meta, dir)
	if err != nil {
		return nil, "", err
	}
	ref, ok := meta["provenance"].(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("Markdown has no provenance companion")
	}
	files := map[string][]byte{}
	add := func(path string, data []byte) error {
		if !IsPathWithinAssetsDir(path) {
			return fmt.Errorf("archive path is outside assets")
		}
		relative, err := filepath.Rel(resolveExportDir(), path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = data
		return nil
	}
	name := exportFileComponent(exportEpisodeFileName(item), fmt.Sprintf(".r%v.md", meta["document_revision"]))
	if err := add(filepath.Join(dir, name), []byte(content)); err != nil {
		return nil, "", err
	}
	provenancePath := filepath.Join(dir, filepath.FromSlash(ref["uri"].(string)))
	data, err := os.ReadFile(provenancePath)
	if err != nil {
		return nil, "", err
	}
	if pm.Hash(data) != ref["sha256"] {
		return nil, "", fmt.Errorf("provenance file hash mismatch")
	}
	if err := add(provenancePath, data); err != nil {
		return nil, "", err
	}
	for hash := range full["input_snapshots"].(map[string]any) {
		path := filepath.Join(markdownProvenanceDir(), "inputs", hash+".txt")
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", err
		}
		if pm.Hash(data) != hash {
			return nil, "", fmt.Errorf("input snapshot hash mismatch")
		}
		if err := add(path, data); err != nil {
			return nil, "", err
		}
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			return nil, "", err
		}
		if _, err := entry.Write(content); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	downloadName := fmt.Sprintf("Briefcast-%s-%s-r%v.zip", strings.TrimPrefix(meta["document_id"].(string), "urn:uuid:"), kind, meta["document_revision"])
	return buffer.Bytes(), downloadName, nil
}
