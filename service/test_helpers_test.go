package service

import (
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func resetServiceTestState() {
	resetDownloadManagerState()
	resetRepairWorkState()
	exportAllRunning.Store(false)
	briefpointSyncRunning.Store(false)
	linkBackfillRunning.Store(false)
	summaryBackfillRunning.Store(false)
	lookupOutboundIPAddrs = net.DefaultResolver.LookupIPAddr
	resetOutboundRequestLimiterForTests()
}

func runPostOpmlRefreshSynchronously(t *testing.T) {
	t.Helper()
	original := startPostOpmlRefresh
	startPostOpmlRefresh = refreshEpisodesAfterOPMLImport
	t.Cleanup(func() {
		startPostOpmlRefresh = original
	})
}

func requireWorkingPython(t *testing.T) string {
	t.Helper()
	candidates := []string{"python3", "python", "py"}
	if explicit := strings.TrimSpace(os.Getenv(feedparserPythonEnv)); explicit != "" {
		candidates = []string{explicit}
	}

	failures := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		pythonPath, err := exec.LookPath(candidate)
		if err != nil {
			failures = append(failures, candidate+": not found")
			continue
		}
		cmd := exec.Command(pythonPath, "-c", "print('ok')")
		output, runErr := cmd.CombinedOutput()
		if runErr == nil {
			return pythonPath
		}
		failures = append(failures, pythonPath+": "+runErr.Error()+" ("+strings.TrimSpace(string(output))+")")
	}

	t.Skipf("python is not runnable: %s", strings.Join(failures, "; "))
	return ""
}
