package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	glebarezsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestIntegrationFeedDownloadWhisperX(t *testing.T) {
	pythonPath := requireWorkingPython(t)

	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "assets")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	t.Setenv("CONFIG", tempDir)
	t.Setenv("DATA", dataDir)
	dbURL := strings.TrimSpace(os.Getenv("BRIEFCAST_INTEGRATION_DB_URL"))
	if dbURL == "" {
		dbURL = filepath.Join(tempDir, "briefcast.db")
	}
	t.Setenv("DATABASE_URL", dbURL)

	scriptDir := filepath.Join(tempDir, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("failed to create script dir: %v", err)
	}

	feedScript := writeTempScript(t, scriptDir, "feedparser_stub.py", feedparserStub)
	whisperScript := writeTempScript(t, scriptDir, "whisperx_stub.py", whisperxStub)

	t.Setenv("FEEDPARSER_PYTHON", pythonPath)
	t.Setenv("FEEDPARSER_SCRIPT", feedScript)
	t.Setenv("WHISPERX_ENABLED", "true")
	t.Setenv("WHISPERX_PYTHON", pythonPath)
	t.Setenv("WHISPERX_SCRIPT", whisperScript)
	t.Setenv("WHISPERX_DIARIZATION", "false")

	server := httptest.NewServer(testFeedServer())
	t.Setenv("TEST_BASE_URL", server.URL)
	t.Cleanup(server.Close)

	var err error
	db.DB, err = db.Init()
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") || strings.Contains(err.Error(), "requires cgo") {
			db.DB, err = gorm.Open(glebarezsqlite.Open(dbURL), &gorm.Config{})
			if err != nil {
				t.Fatalf("db init fallback failed: %v", err)
			}
		} else {
			t.Fatalf("db init failed: %v", err)
		}
	}
	sqlDB, sqlErr := db.DB.DB()
	if sqlErr == nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
	db.Migrate()

	setting := db.GetOrCreateSetting()
	setting.AutoDownload = true
	setting.DownloadOnAdd = true
	setting.InitialDownloadCount = 1
	setting.MaxDownloadConcurrency = 1
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("update settings failed: %v", err)
	}

	podcast, err := AddPodcast(server.URL + "/feed")
	if err != nil {
		t.Fatalf("add podcast failed: %v", err)
	}
	if err := AddPodcastItems(&podcast, true); err != nil {
		t.Fatalf("add podcast items failed: %v", err)
	}
	if err := DownloadMissingEpisodes(); err != nil {
		t.Fatalf("download missing episodes failed: %v", err)
	}
	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatalf("transcribe pending episodes failed: %v", err)
	}

	var items []db.PodcastItem
	if err := db.GetAllPodcastItemsByPodcastId(podcast.ID, &items); err != nil {
		t.Fatalf("fetch podcast items failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 podcast item, got %d", len(items))
	}

	item := items[0]
	if item.DownloadStatus != db.Downloaded {
		t.Fatalf("expected downloaded status, got %v", item.DownloadStatus)
	}
	if item.TranscriptStatus != "available" {
		t.Fatalf("expected transcript available, got %q", item.TranscriptStatus)
	}
	if !strings.Contains(item.TranscriptJSON, "\"segments\"") {
		t.Fatalf("expected transcript segments, got %s", item.TranscriptJSON)
	}
	if item.SummaryHTML == "" {
		t.Fatalf("expected summary html to be populated")
	}
}

func testFeedServer() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/feed", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, feedXMLTemplate, r.Host, r.Host)
	})
	mux.HandleFunc("/audio.mp3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("fake audio data"))
	})
	mux.HandleFunc("/cover.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake image data"))
	})
	return mux
}

func writeTempScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("failed to write script %s: %v", name, err)
	}
	return path
}

const feedXMLTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <title>Integration Podcast</title>
    <description>Feed summary</description>
    <itunes:image href="http://%s/cover.jpg" />
    <item>
      <title>Episode 1</title>
      <guid>episode-1</guid>
      <pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
      <enclosure url="http://%s/audio.mp3" type="audio/mpeg" length="123" />
      <description><![CDATA[<p>Episode summary</p>]]></description>
    </item>
  </channel>
</rss>
`

const feedparserStub = `#!/usr/bin/env python3
import json
import os
import sys

base = os.environ.get("TEST_BASE_URL", "")
payload = {
    "feed": {
        "title": "Integration Podcast",
        "itunes_author": "Tester",
        "image": {"href": base + "/cover.jpg"},
        "summary": "<p>Feed summary</p>",
    },
    "entries": [
        {
            "title": "Episode 1",
            "guid": "episode-1",
            "published": "Mon, 01 Jan 2024 00:00:00 GMT",
            "itunes_duration": "123",
            "summary": "<p>Episode summary</p>",
            "enclosures": [{"href": base + "/audio.mp3"}],
        }
    ],
}
json.dump(payload, sys.stdout)
`

const whisperxStub = `#!/usr/bin/env python3
import json
import sys

payload = {
    "provider": "whisperx",
    "segments": [
        {"start": 0.0, "end": 1.0, "text": "hello world"}
    ],
    "metadata": {"generated_at": "2026-02-13T00:00:00Z"},
}
json.dump(payload, sys.stdout)
`
