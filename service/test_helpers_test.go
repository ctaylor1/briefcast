package service

import (
	"os/exec"
	"strings"
	"testing"
)

func requireWorkingPython(t *testing.T) string {
	t.Helper()
	pythonPath, err := resolvePython()
	if err != nil {
		t.Skipf("python not available: %v", err)
	}

	cmd := exec.Command(pythonPath, "-c", "print('ok')")
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Skipf("python is not runnable (%s): %v (%s)", pythonPath, runErr, strings.TrimSpace(string(output)))
	}

	return pythonPath
}
