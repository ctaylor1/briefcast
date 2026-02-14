package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type FeedParserResult struct {
	Feed          map[string]interface{}   `json:"feed"`
	Entries       []map[string]interface{} `json:"entries"`
	Bozo          bool                     `json:"bozo"`
	Version       string                   `json:"version"`
	BozoException string                   `json:"bozo_exception"`
}

const (
	defaultFeedparserScript = "scripts/feedparser_parse.py"
	feedparserPythonEnv     = "FEEDPARSER_PYTHON"
	feedparserScriptEnv     = "FEEDPARSER_SCRIPT"
)

func FetchFeedWithFeedparser(url string) (FeedParserResult, []byte, error) {
	body, err := makeQuery(url)
	if err != nil {
		return FeedParserResult{}, nil, err
	}
	parsed, err := ParseFeedWithFeedparser(body)
	if err != nil {
		return FeedParserResult{}, body, err
	}
	return parsed, body, nil
}

func ParseFeedWithFeedparser(body []byte) (FeedParserResult, error) {
	pythonPath, err := resolvePython()
	if err != nil {
		return FeedParserResult{}, err
	}

	scriptPath := strings.TrimSpace(os.Getenv(feedparserScriptEnv))
	if scriptPath == "" {
		scriptPath = defaultFeedparserScript
	}
	if abs, absErr := filepath.Abs(scriptPath); absErr == nil {
		scriptPath = abs
	}

	cmd := exec.Command(pythonPath, scriptPath)
	cmd.Stdin = bytes.NewReader(body)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return FeedParserResult{}, fmt.Errorf("feedparser failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var result FeedParserResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return FeedParserResult{}, fmt.Errorf("feedparser output decode failed: %w", err)
	}
	return result, nil
}

func resolvePython() (string, error) {
	explicit := strings.TrimSpace(os.Getenv(feedparserPythonEnv))
	if explicit != "" {
		if path, err := exec.LookPath(explicit); err == nil {
			return path, nil
		}
		return explicit, nil
	}

	candidates := []string{"python3", "python"}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("python interpreter not found; set %s or install python", feedparserPythonEnv)
}
