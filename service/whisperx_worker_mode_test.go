package service

import (
	"encoding/json"
	"testing"
)

func TestLocalWhisperXDoesNotInheritWorkerProtocol(t *testing.T) {
	t.Setenv("WHISPERX_WORKER_MODE", "1")
	t.Setenv("WHISPERX_WORKER_CONTEXT", "invalid worker context")
	python := requireWorkingPython(t)
	script := writeTempScript(t, t.TempDir(), "local_mode.py", `import json,os,sys
json.dump({'segments': [], 'worker_mode':os.environ.get('WHISPERX_WORKER_MODE')},sys.stdout)
`)
	raw, err := RunWhisperX("", WhisperXConfig{Python: python, Script: script})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		WorkerMode string `json:"worker_mode"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.WorkerMode != "0" {
		t.Fatal("local adapter inherited worker protocol")
	}
}
