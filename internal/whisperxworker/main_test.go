package whisperxworker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/google/uuid"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__whisperx_child" {
		if RunGuardian(os.Args[2:]) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) > 2 && os.Args[1] == "__crash_test" {
		if runCrashHelper(os.Args[2]) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type crashConfig struct {
	Config         Config
	Request        transcription.JobRequest
	Python, Script string
}

func runCrashHelper(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg crashConfig
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	runtime := filepath.Join(filepath.Dir(cfg.Config.StateDir), "runtime")
	if err = os.MkdirAll(filepath.Join(runtime, "tmp"), 0700); err != nil {
		return err
	}
	runner := &PythonRunner{cfg: PythonConfig{Python: cfg.Python, Script: cfg.Script, CacheRoot: runtime}, versions: cfg.Config.PackageVersions, environment: []string{"PATH=" + os.Getenv("PATH"), "HOME=" + runtime, "TMPDIR=" + filepath.Join(runtime, "tmp")}}
	w, err := Open(context.Background(), cfg.Config, runner)
	if err != nil {
		return err
	}
	raw, err = json.Marshal(cfg.Request)
	if err != nil {
		return err
	}
	id := uuid.NewString()
	if _, _, err = w.Submit(context.Background(), id, raw); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(cfg.Config.StateDir, "helper-id"), []byte(id), 0600); err != nil {
		return err
	}
	select {}
}
