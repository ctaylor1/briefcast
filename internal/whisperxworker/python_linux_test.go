package whisperxworker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/google/uuid"
)

func stubPython(t *testing.T, cfg Config, scriptBody string) *PythonRunner {
	t.Helper()
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skip("Python unavailable")
	}
	for _, command := range []string{"ffmpeg", "ffprobe"} {
		if _, err = exec.LookPath(command); err != nil {
			t.Skip("audio tools unavailable")
		}
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "child.py")
	if err = os.WriteFile(script, []byte(scriptBody), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(dir, "tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	return &PythonRunner{cfg: PythonConfig{Python: python, Script: script, CacheRoot: dir}, versions: cfg.PackageVersions, environment: []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "TMPDIR=" + filepath.Join(dir, "tmp"), "LOG_OUTPUT=stderr"}}
}

func TestPythonSupervisorSuccessAndMalformedOutput(t *testing.T) {
	for _, valid := range []bool{true, false} {
		name := "valid"
		if !valid {
			name = "invalid"
		}
		t.Run(name, func(t *testing.T) {
			cfg, request, output := workerFixture(t)
			if !valid {
				output = []byte(`{"error":"not a transcript"}`)
			}
			script := "import sys,base64\nsys.stdout.buffer.write(base64.b64decode('" + base64.StdEncoding.EncodeToString(output) + "'))\n"
			runner := stubPython(t, cfg, script)
			w := openTestWorker(t, cfg, runner)
			id := uuid.NewString()
			submitTest(t, w, id, request)
			state := "succeeded"
			if !valid {
				state = "failed"
			}
			status := waitState(t, w, id, state)
			if !valid && status.Error.Code != "INTERNAL" {
				t.Fatal(status.Error)
			}
		})
	}
}

func TestPythonSupervisorCancellationReapsDescendantsAndRetainsLease(t *testing.T) {
	cfg, request, _ := workerFixture(t)
	runner := stubPython(t, cfg, `import os,sys,time,subprocess
child=subprocess.Popen([sys.executable,'-c','import time;time.sleep(60)'])
open(sys.argv[1]+'.child','w').write(str(child.pid))
time.sleep(60)
`)
	w := openTestWorker(t, cfg, runner)
	id := uuid.NewString()
	submitTest(t, w, id, request)
	pidFile := filepath.Join(cfg.MediaRoot, filepath.FromSlash(request.Input.RelativePath)) + ".child"
	deadline := time.Now().Add(5 * time.Second)
	var child int
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(pidFile); err == nil {
			child, _ = strconv.Atoi(string(b))
			if child > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if child <= 0 {
		t.Fatal("child did not start")
	}
	if _, err := w.Cancel(id); err != nil {
		t.Fatal(err)
	}
	waitState(t, w, id, "cancelled")
	if err := syscall.Kill(child, 0); err != syscall.ESRCH {
		t.Fatalf("descendant still exists: %v", err)
	}
}

func TestPythonSupervisorBoundsOutputAndDiagnostics(t *testing.T) {
	cfg, request, _ := workerFixture(t)
	cfg.ResultLimit = 1024
	runner := stubPython(t, cfg, "import sys,time\nsys.stderr.write('private-diagnostic'*10000)\nsys.stdout.write('x'*1000000)\nsys.stdout.flush()\ntime.sleep(60)\n")
	w := openTestWorker(t, cfg, runner)
	id := uuid.NewString()
	submitTest(t, w, id, request)
	status := waitState(t, w, id, "failed")
	if strings.Contains(status.Error.Message, "private-diagnostic") {
		t.Fatal("private stderr escaped")
	}
	for name, limit := range map[string]int64{"stdout.json": 1024, "stderr.log": 64 * 1024} {
		info, err := os.Stat(filepath.Join(w.store.jobDir(id), name))
		if err != nil || info.Size() > limit {
			t.Fatalf("unbounded %s: %v %v", name, info, err)
		}
	}
}

func TestPythonSupervisorProgressFailureStopsChild(t *testing.T) {
	cfg, request, _ := workerFixture(t)
	runner := stubPython(t, cfg, `import os,time,json
open(os.environ['WHISPERX_PROGRESS_FILE'],'w').write(json.dumps({'stage':'transcribing','percent':40}))
time.sleep(60)
`)
	s, err := openStore(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	id := uuid.NewString()
	dir := s.jobDir(id)
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = runner.Run(ctx, RunSpec{JobID: id, Directory: dir, AudioPath: filepath.Join(cfg.MediaRoot, filepath.FromSlash(request.Input.RelativePath)), Request: request, Effective: transcription.EffectiveConfig{Device: cfg.Device, ComputeType: cfg.ComputeType, BatchSize: cfg.BatchSize, EngineProfile: cfg.EngineProfile}, ResultLimit: cfg.ResultLimit, OwnerLock: s.lock}, func(transcription.ProgressUpdate) error {
		return transcription.Error("STATE_UNAVAILABLE", "Injected progress failure", true)
	})
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("progress failure did not stop process: %v", err)
	}
}

func TestPythonGuardianWorkerCrashStopsDescendantsBeforeNewOwnership(t *testing.T) {
	cfg, request, output := workerFixture(t)
	runner := stubPython(t, cfg, `import os,sys,time,subprocess
child=subprocess.Popen([sys.executable,'-c','import time;time.sleep(60)'])
open(sys.argv[1]+'.child','w').write(str(child.pid))
time.sleep(60)
`)
	configuration := crashConfig{Config: cfg, Request: request, Python: runner.cfg.Python, Script: runner.cfg.Script}
	raw, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "configuration.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "__crash_test", path)
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	pidFile := filepath.Join(cfg.MediaRoot, filepath.FromSlash(request.Input.RelativePath)) + ".child"
	var child int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if raw, e := os.ReadFile(pidFile); e == nil {
			child, _ = strconv.Atoi(string(raw))
			if child > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if child <= 0 {
		t.Fatal("crash-test child did not start")
	}
	if s, e := openStore(cfg.StateDir); e == nil {
		s.close()
		t.Fatal("lease missing before crash")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	deadline = time.Now().Add(5 * time.Second)
	var reopened *store
	for time.Now().Before(deadline) {
		reopened, err = openStore(cfg.StateDir)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("guardian did not release state after cleanup")
	}
	reopened.close()
	if err = syscall.Kill(child, 0); err != syscall.ESRCH {
		t.Fatalf("new owner acquired before descendant reaping: %v", err)
	}
	idRaw, err := os.ReadFile(filepath.Join(cfg.StateDir, "helper-id"))
	if err != nil {
		t.Fatal(err)
	}
	w := openTestWorker(t, cfg, successRunner(output))
	status := waitState(t, w, string(idRaw), "succeeded")
	if status.Revision < 3 {
		t.Fatal("crashed running job was not reconciled")
	}
}

func TestMediaFIFORejectedWithoutBlocking(t *testing.T) {
	cfg, request, output := workerFixture(t)
	w := openTestWorker(t, cfg, successRunner(output))
	path := filepath.Join(cfg.MediaRoot, "fifo")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	request.Input.RelativePath = "fifo"
	raw, _ := json.Marshal(request)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := w.Submit(ctx, uuid.NewString(), raw); err == nil {
		t.Fatal("accepted FIFO")
	}
}
