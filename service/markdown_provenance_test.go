package service

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	pm "github.com/ctaylor1/briefcast/internal/podcastmarkdown"
)

func TestMarkdownProvenanceIntegrityStopsRevision(t *testing.T) {
	for _, alteration := range []string{"missing", "tampered", "outside"} {
		t.Run(alteration, func(t *testing.T) {
			item := markdownTestItem(t)
			content, err := EpisodeMarkdown(&item, "summary")
			if err != nil {
				t.Fatal(err)
			}
			meta, _, _ := pm.Parse(content)
			ref := meta["provenance"].(map[string]any)
			filename := filepath.Join(markdownDirectory(&item, "summary"), filepath.FromSlash(ref["uri"].(string)))
			switch alteration {
			case "missing":
				err = os.Remove(filename)
			case "tampered":
				err = os.WriteFile(filename, []byte("{}"), 0644)
			case "outside":
				ref["uri"] = "../../../../../../outside.json"
				content, err = pm.Encode(meta, "Original summary")
				if err == nil {
					err = db.DB.Model(&db.MarkdownRevision{}).Where("episode_id = ?", item.ID).Update("content", content).Error
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			item.Title = "A replacement title"
			if _, err := EpisodeMarkdown(&item, "summary"); err == nil {
				t.Fatal("ignored unavailable or invalid provenance")
			}
			var count int64
			if err := db.DB.Model(&db.MarkdownRevision{}).Where("episode_id = ?", item.ID).Count(&count).Error; err != nil || count != 1 {
				t.Fatal("committed a revision after failed integrity checks", err)
			}
		})
	}
}

func TestMarkdownBundleRetainsSeparatedPortableCompanions(t *testing.T) {
	item := markdownTestItem(t)
	for _, kind := range []string{"transcript", "summary"} {
		data, filename, err := EpisodeMarkdownBundle(&item, kind)
		if err != nil || !strings.HasSuffix(filename, ".zip") {
			t.Fatal("failed to create portable package", err)
		}
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		files := map[string][]byte{}
		var notePath string
		for _, entry := range reader.File {
			if strings.HasPrefix(entry.Name, "/") || strings.Contains(entry.Name, "..") {
				t.Fatal("nonportable archive member")
			}
			stream, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(stream)
			stream.Close()
			if err != nil {
				t.Fatal(err)
			}
			files[entry.Name] = body
			if strings.HasSuffix(entry.Name, ".md") {
				notePath = entry.Name
			}
		}
		meta, body, err := pm.Parse(string(files[notePath]))
		if err != nil {
			t.Fatal(err)
		}
		if kind == "summary" && body != item.LLMSummary {
			t.Fatal("changed summary Markdown")
		}
		if kind == "transcript" && body != item.CanonicalTranscript {
			t.Fatal("changed plain transcript")
		}
		ref := meta["provenance"].(map[string]any)
		provenancePath := path.Clean(path.Join(path.Dir(notePath), ref["uri"].(string)))
		if !strings.HasPrefix(provenancePath, "provenance/podcast-markdown/") || pm.Hash(files[provenancePath]) != ref["sha256"] {
			t.Fatal("package lost the separate provenance reference")
		}
		full := markdownTestProvenance(t, &item, kind, meta)
		for hash := range full["input_snapshots"].(map[string]any) {
			if pm.Hash(files[path.Join(path.Dir(provenancePath), "inputs", hash+".txt")]) != hash {
				t.Fatal("package lost exact source bytes")
			}
		}
	}
}
