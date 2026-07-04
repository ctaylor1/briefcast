package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAppLogFileClassifiesAndRedacts(t *testing.T) {
	resetServiceTestState()
	tempDir := t.TempDir()

	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	logPath := filepath.Join(logDir, "briefcast-test.log")
	logBody := strings.Join([]string{
		`{"level":"warn","ts":"2026-07-01T12:00:00Z","caller":"service/summary_backfill.go:59","msg":"summary failed","episode_id":"episode-1","title":"Episode One","error":"api_key=super-secret"}`,
		"2026-07-01T12:01:00Z\tERROR\tservice/podcastService.go:874\tfailed to download episode\t{\"service\":\"briefcast\",\"podcast\":\"Podcast One\",\"episode\":\"Episode Two\",\"url\":\"https://example.com/audio.mp3\",\"error\":\"download failed with status 500\"}",
		`2026-07-01T12:02:00+00:00 WARNING briefcast-whisperx __main__ audio file not found context={"audio_file":"/tmp/audio.mp3","hf_token":"real-token"}`,
	}, "\n")
	if err := os.WriteFile(logPath, []byte(logBody), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	entries, err := parseAppLogFile(logPath)
	if err != nil {
		t.Fatalf("parseAppLogFile failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 parsed entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Category != "Summary" || !entries[0].UserImpact {
		t.Fatalf("expected summary entry to be user-impacting, got %+v", entries[0])
	}
	if entries[1].HumanMessage != `Download failed for "Episode Two" from "Podcast One": download failed with status 500` {
		t.Fatalf("unexpected download human message: %q", entries[1].HumanMessage)
	}
	if entries[2].Category != "Transcript" || !entries[2].UserImpact {
		t.Fatalf("expected transcript entry to be user-impacting, got %+v", entries[2])
	}
	joined, err := marshalAppLogsForTest(AppLogResponse{Entries: entries, ImpactEntries: latestImpactEntries(entries)})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(joined, "super-secret") || strings.Contains(joined, "real-token") {
		t.Fatalf("expected sensitive values to be redacted: %s", joined)
	}
	if !strings.Contains(joined, "***REDACTED***") {
		t.Fatalf("expected redaction marker in response: %s", joined)
	}
	if impacts := latestImpactEntries(entries); len(impacts) != 3 {
		t.Fatalf("expected 3 impact entries, got %d", len(impacts))
	}
}

func TestParseAppLogLimit(t *testing.T) {
	if got, err := ParseAppLogLimit(""); err != nil || got != defaultAppLogLimit {
		t.Fatalf("expected default limit, got %d, %v", got, err)
	}
	if got, err := ParseAppLogLimit("25"); err != nil || got != 25 {
		t.Fatalf("expected explicit limit, got %d, %v", got, err)
	}
	if _, err := ParseAppLogLimit("bad"); err == nil || !strings.Contains(err.Error(), "integer") {
		t.Fatalf("expected integer validation error, got %v", err)
	}
	if _, err := ParseAppLogLimit("999"); err == nil || !strings.Contains(err.Error(), "between") {
		t.Fatalf("expected range validation error, got %v", err)
	}
}

func marshalAppLogsForTest(response AppLogResponse) (string, error) {
	raw, err := json.Marshal(response)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
