package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePythonExplicitPath(t *testing.T) {
	t.Setenv(feedparserPythonEnv, "not-a-real-python")
	path, err := resolvePython()
	if err != nil {
		t.Fatalf("expected explicit python path to be accepted, got %v", err)
	}
	if path != "not-a-real-python" {
		t.Fatalf("expected explicit path, got %q", path)
	}
}

func TestParseFeedWithFeedparserStubScript(t *testing.T) {
	pythonPath := requireWorkingPython(t)

	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "feedparser_stub.py")
	body := "#!/usr/bin/env python3\nimport json\nprint(json.dumps({'feed': {'title':'Test Feed'}, 'entries':[{'title':'Episode'}]}))\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatalf("failed to write stub script: %v", err)
	}
	t.Setenv(feedparserPythonEnv, pythonPath)
	t.Setenv(feedparserScriptEnv, scriptPath)

	parsed, err := ParseFeedWithFeedparser([]byte("<rss/>"))
	if err != nil {
		t.Fatalf("ParseFeedWithFeedparser failed: %v", err)
	}
	if parsed.Feed["title"] != "Test Feed" {
		t.Fatalf("expected feed title, got %+v", parsed.Feed)
	}
	if len(parsed.Entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(parsed.Entries))
	}
}

func TestParseFeedWithFeedparserInvalidOutput(t *testing.T) {
	pythonPath := requireWorkingPython(t)

	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "feedparser_bad.py")
	body := "#!/usr/bin/env python3\nprint('not-json')\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatalf("failed to write bad script: %v", err)
	}
	t.Setenv(feedparserPythonEnv, pythonPath)
	t.Setenv(feedparserScriptEnv, scriptPath)

	if _, err := ParseFeedWithFeedparser([]byte("<rss/>")); err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestFetchFeedWithFeedparser(t *testing.T) {
	setupRetentionTestDB(t)
	pythonPath := requireWorkingPython(t)

	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "feedparser_fetch.py")
	body := "#!/usr/bin/env python3\nimport json\nprint(json.dumps({'feed': {'title':'Fetched Feed'}, 'entries': []}))\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatalf("failed to write fetch script: %v", err)
	}
	t.Setenv(feedparserPythonEnv, pythonPath)
	t.Setenv(feedparserScriptEnv, scriptPath)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<rss>feed-body</rss>"))
	}))
	defer server.Close()

	parsed, raw, err := FetchFeedWithFeedparser(server.URL)
	if err != nil {
		t.Fatalf("FetchFeedWithFeedparser failed: %v", err)
	}
	if parsed.Feed["title"] != "Fetched Feed" {
		t.Fatalf("expected parsed feed title, got %+v", parsed.Feed)
	}
	if string(raw) != "<rss>feed-body</rss>" {
		t.Fatalf("expected raw feed body returned, got %q", string(raw))
	}
}
