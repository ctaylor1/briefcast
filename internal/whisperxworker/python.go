package whisperxworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
)

type PythonConfig struct{ Python, Script, CacheRoot, HFTokenFile, LogLevel, Device string }
type PythonRunner struct {
	cfg         PythonConfig
	versions    map[string]string
	environment []string
}

func NewPythonRunner(ctx context.Context, cfg PythonConfig) (*PythonRunner, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("worker subprocess supervision requires Linux")
	}
	var err error
	cfg.Python, err = exec.LookPath(cfg.Python)
	if err != nil {
		return nil, errors.New("worker Python is unavailable")
	}
	cfg.Python, err = filepath.Abs(cfg.Python)
	if err != nil {
		return nil, err
	}
	cfg.Script, err = filepath.Abs(cfg.Script)
	if err != nil {
		return nil, err
	}
	cfg.CacheRoot, err = filepath.Abs(cfg.CacheRoot)
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{"huggingface", "torch", "nltk", "tmp", "home"} {
		if err = os.MkdirAll(filepath.Join(cfg.CacheRoot, dir), 0700); err != nil {
			return nil, err
		}
	}
	env := []string{}
	for _, key := range []string{"PATH", "LANG", "LC_ALL", "TZ", "LD_LIBRARY_PATH", "CUDA_VISIBLE_DEVICES", "NVIDIA_VISIBLE_DEVICES", "NVIDIA_DRIVER_CAPABILITIES", "HF_HUB_OFFLINE", "TRANSFORMERS_OFFLINE", "HF_DATASETS_OFFLINE"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	env = append(env, "HOME="+filepath.Join(cfg.CacheRoot, "home"), "HF_HOME="+filepath.Join(cfg.CacheRoot, "huggingface"), "HUGGINGFACE_HUB_CACHE="+filepath.Join(cfg.CacheRoot, "huggingface", "hub"), "TORCH_HOME="+filepath.Join(cfg.CacheRoot, "torch"), "NLTK_DATA="+filepath.Join(cfg.CacheRoot, "nltk"), "TMPDIR="+filepath.Join(cfg.CacheRoot, "tmp"), "PYTHONDONTWRITEBYTECODE=1", "HF_HUB_DISABLE_TELEMETRY=1", "DO_NOT_TRACK=1", "PYANNOTE_METRICS_ENABLED=0", "LOG_OUTPUT=stderr", "WHISPERX_THIRD_PARTY_LOG_LEVEL="+cfg.LogLevel)
	if cfg.HFTokenFile != "" {
		raw, err := readBounded(cfg.HFTokenFile, 4096)
		if err != nil {
			return nil, errors.New("HF token file unavailable")
		}
		env = append(env, "WHISPERX_HF_TOKEN="+strings.TrimSpace(string(raw)))
	}
	probe := `import importlib.metadata as m, json, pathlib, shutil, sys
compile(pathlib.Path(sys.argv[1]).read_text(), sys.argv[1], 'exec')
assert shutil.which('ffmpeg') and shutil.which('ffprobe')
if sys.argv[2] == 'cuda':
 import torch
 assert torch.cuda.is_available()
print(json.dumps({name:m.version(name) for name in ['whisperx','torch','torchaudio','ctranslate2','faster-whisper','pyannote.audio']}))`
	cmd := exec.CommandContext(ctx, cfg.Python, "-c", probe, cfg.Script, cfg.Device)
	cmd.Env = env
	var stdout bytes.Buffer
	cmd.Stdout = &limitBuffer{buffer: &stdout, limit: 64 * 1024}
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		return nil, errors.New("worker runtime prerequisite probe failed")
	}
	var versions map[string]string
	if err = json.Unmarshal(stdout.Bytes(), &versions); err != nil {
		return nil, err
	}
	r := &PythonRunner{cfg: cfg, versions: versions, environment: env}
	if err = r.Ready(ctx); err != nil {
		return nil, err
	}
	return r, nil
}
func (r *PythonRunner) PackageVersions() map[string]string {
	result := map[string]string{}
	for k, v := range r.versions {
		result[k] = v
	}
	return result
}
func (r *PythonRunner) Ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, path := range []string{r.cfg.Python, r.cfg.Script} {
		stat, err := os.Stat(path)
		if err != nil || !stat.Mode().IsRegular() {
			return errors.New("execution file is unavailable")
		}
	}
	for _, command := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(command); err != nil {
			return errors.New("audio tools are unavailable")
		}
	}
	file, err := os.CreateTemp(filepath.Join(r.cfg.CacheRoot, "tmp"), "readiness-*")
	if err != nil {
		return errors.New("runtime cache is unavailable")
	}
	name := file.Name()
	err = errors.Join(file.Close(), os.Remove(name))
	return err
}

func (r *PythonRunner) PipelineSHA256() (string, error) {
	raw, err := readBounded(r.cfg.Script, transcription.ControlLimit)
	if err != nil {
		return "", err
	}
	return transcription.Digest(raw), nil
}

type limitBuffer struct {
	buffer *bytes.Buffer
	limit  int
}

func (b *limitBuffer) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > b.limit {
		return 0, errors.New("probe output exceeds limit")
	}
	return b.buffer.Write(p)
}

type boundedFile struct {
	file      *os.File
	remaining int64
	truncate  bool
	abort     func()
}

func (b *boundedFile) Write(p []byte) (int, error) {
	n := len(p)
	allowed := min(int64(n), b.remaining)
	if allowed > 0 {
		written, err := b.file.Write(p[:allowed])
		b.remaining -= int64(written)
		if err != nil {
			return written, err
		}
	}
	if int64(n) > allowed && !b.truncate {
		b.abort()
		return int(allowed), errors.New("child output exceeds limit")
	}
	return n, nil
}

func (r *PythonRunner) Run(ctx context.Context, s RunSpec, onProgress func(transcription.ProgressUpdate) error) error {
	config := transcription.ScriptConfig{Model: s.Request.Options.Model, Language: s.Request.Options.Language, Device: s.Effective.Device, ComputeType: s.Effective.ComputeType, BatchSize: s.Effective.BatchSize, ChunkSeconds: s.Request.Options.ChunkSeconds, ASROptions: map[string]interface{}{"beam_size": s.Request.Options.ASROptions.BeamSize, "patience": s.Request.Options.ASROptions.Patience, "condition_on_previous_text": s.Request.Options.ASROptions.ConditionOnPreviousText, "initial_prompt": s.Request.Options.ASROptions.InitialPrompt}, VADOptions: map[string]interface{}{"chunk_size": s.Request.Options.VADOptions.ChunkSize, "vad_onset": s.Request.Options.VADOptions.VADOnset, "vad_offset": s.Request.Options.VADOptions.VADOffset}, VADMethod: s.Request.Options.VADMethod, Align: s.Request.Options.Align, Diarization: s.Request.Options.Diarization, DiarizeModel: s.Request.Options.DiarizationModel, MinSpeakers: s.Request.Options.MinSpeakers, MaxSpeakers: s.Request.Options.MaxSpeakers}
	payload, err := json.Marshal(config)
	if err != nil {
		return err
	}
	rec := &record{Request: s.Request, Status: transcription.JobStatus{EffectiveConfig: s.Effective}, PipelineVersion: transcription.PipelineVersion}
	bindingJSON, err := json.Marshal(binding(rec))
	if err != nil {
		return err
	}
	env := append([]string{}, r.environment...)
	env = append(env, "WHISPERX_CONFIG_JSON="+string(payload), "WHISPERX_WORKER_MODE=1", "WHISPERX_WORKER_CONTEXT="+string(bindingJSON), "WHISPERX_PROGRESS_FILE="+filepath.Join(s.Directory, "progress.json"), "WHISPERX_SEGMENTS_FILE="+filepath.Join(s.Directory, "segments.json"), "WHISPERX_STOP_FILE="+filepath.Join(s.Directory, "stop"))
	if len(s.Resume) > 0 {
		resumePath := filepath.Join(s.Directory, "resume.json")
		if err = durableWrite(resumePath, s.Resume); err != nil {
			return err
		}
		env = append(env, "WHISPERX_RESUME_FILE="+resumePath)
	} else {
		if err = os.Remove(filepath.Join(s.Directory, "segments.json")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err = os.Remove(filepath.Join(s.Directory, "progress.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	stdout, err := os.OpenFile(filepath.Join(s.Directory, "stdout.json"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(filepath.Join(s.Directory, "stderr.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer stderr.Close()
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "__whisperx_child", r.cfg.Python, r.cfg.Script, s.AudioPath)
	cmd.Env = env
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var stopOnce sync.Once
	abort := func() { stopOnce.Do(func() { _ = input.Close() }) }
	defer abort()
	cmd.Stdout = &boundedFile{file: stdout, remaining: s.ResultLimit, abort: abort}
	cmd.Stderr = &boundedFile{file: stderr, remaining: 64 * 1024, truncate: true, abort: abort}
	configureGuardian(cmd, s.OwnerLock)
	if err = cmd.Start(); err != nil {
		return err
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	last := ""
	var progressErr error
	readProgress := func() {
		raw, err := readBounded(filepath.Join(s.Directory, "progress.json"), transcription.ControlLimit)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			progressErr = err
			abort()
			return
		}
		if string(raw) == last {
			return
		}
		last = string(raw)
		var update transcription.ProgressUpdate
		if _, err = transcription.CanonicalJSON(raw); err != nil {
			progressErr = err
			abort()
			return
		}
		if err = json.Unmarshal(raw, &update); err != nil {
			progressErr = err
			abort()
			return
		}
		if onProgress != nil {
			if err = onProgress(update); err != nil {
				progressErr = err
				abort()
			}
		}
	}
	done := ctx.Done()
	for {
		select {
		case err = <-wait:
			readProgress()
			if syncErr := stdout.Sync(); syncErr != nil {
				return syncErr
			}
			if progressErr != nil {
				return progressErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return transcription.Error("INTERNAL", "Python execution failed; inspect private diagnostics", true)
			}
			return nil
		case <-ticker.C:
			readProgress()
		case <-done:
			abort()
			done = nil
		}
	}
}
