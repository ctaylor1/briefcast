package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
	defaultFeedparserTimeoutSeconds = 30
	feedparserPythonEnv     = "FEEDPARSER_PYTHON"
	feedparserScriptEnv     = "FEEDPARSER_SCRIPT"
	feedparserTimeoutEnv    = "FEEDPARSER_TIMEOUT_SECONDS"
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

	timeoutSeconds := getEnvInt(feedparserTimeoutEnv, defaultFeedparserTimeoutSeconds)
	cmdCtx := context.Background()
	cancel := func() {}
	if timeoutSeconds > 0 {
		cmdCtx, cancel = context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	}
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, pythonPath, scriptPath)
	cmd.Stdin = bytes.NewReader(body)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrText := strings.TrimSpace(stderr.String())
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return FeedParserResult{}, fmt.Errorf(
				"feedparser timed out after %d seconds: %s",
				timeoutSeconds,
				stderrText,
			)
		}
		return FeedParserResult{}, fmt.Errorf("feedparser failed: %w: %s", err, stderrText)
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
