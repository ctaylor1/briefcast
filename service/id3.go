package service

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	defaultMutagenScript = "scripts/mutagen_id3_extract.py"
	mutagenPythonEnv     = "MUTAGEN_PYTHON"
	mutagenScriptEnv     = "MUTAGEN_SCRIPT"
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

	cmd := exec.Command(pythonPath, scriptPath, path)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("mutagen extraction failed: %w: %s", err, strings.TrimSpace(stderr.String()))
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
