package service

import (
	"net"
	"os/exec"
	"strings"
	"testing"
)

func resetServiceTestState() {
	resetDownloadManagerState()
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
