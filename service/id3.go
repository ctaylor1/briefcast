package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultMutagenScript = "scripts/mutagen_id3_extract.py"
	defaultMutagenTimeoutSeconds = 20
	mutagenPythonEnv     = "MUTAGEN_PYTHON"
	mutagenScriptEnv     = "MUTAGEN_SCRIPT"
	mutagenTimeoutEnv    = "MUTAGEN_TIMEOUT_SECONDS"
)

func ExtractID3Metadata(path string) ([]byte, error) {
	pythonPath, err := resolveMutagenPython()
	if err != nil {
		return nil, err
	}

	scriptPath := strings.TrimSpace(os.Getenv(mutagenScriptEnv))
	if scriptPath == "" {
		scriptPath = defaultMutagenScript
	}
	if abs, absErr := filepath.Abs(scriptPath); absErr == nil {
		scriptPath = abs
	}

	timeoutSeconds := getEnvInt(mutagenTimeoutEnv, defaultMutagenTimeoutSeconds)
	cmdCtx := context.Background()
	cancel := func() {}
	if timeoutSeconds > 0 {
		cmdCtx, cancel = context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	}
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, pythonPath, scriptPath, path)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrText := strings.TrimSpace(stderr.String())
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf(
				"mutagen extraction timed out after %d seconds: %s",
				timeoutSeconds,
				stderrText,
			)
		}
		return nil, fmt.Errorf("mutagen extraction failed: %w: %s", err, stderrText)
	}

	return stdout.Bytes(), nil
}

func resolveMutagenPython() (string, error) {
	explicit := strings.TrimSpace(os.Getenv(mutagenPythonEnv))
	if explicit != "" {
		if path, err := exec.LookPath(explicit); err == nil {
			return path, nil
		}
		return explicit, nil
	}

	if path, err := resolvePython(); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("python interpreter not found; set %s (or %s) or install python", mutagenPythonEnv, feedparserPythonEnv)
}
