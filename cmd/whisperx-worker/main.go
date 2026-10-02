package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/ctaylor1/briefcast/internal/whisperxworker"
)

var workerVersion = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "__whisperx_child" {
		if err := whisperxworker.RunGuardian(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		return
	}
	if err := run(os.Args[1:]); err != nil {
		slog.Error("worker_command_failed", "error", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	op := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		op = args[0]
		args = args[1:]
	}
	if op == "version" {
		fmt.Println(workerVersion)
		return nil
	}
	if op == "drain" || op == "resume" || op == "status" {
		return maintenance(op, args)
	}
	if op == "gc" {
		flags := flag.NewFlagSet(op, flag.ContinueOnError)
		state := flags.String("state", "", "Worker state directory (worker must be stopped)")
		if err := flags.Parse(args); err != nil {
			return err
		}
		if *state == "" || flags.NArg() != 0 {
			return errors.New("gc requires --state and no positional arguments")
		}
		return whisperxworker.CollectGarbage(*state, time.Now().UTC())
	}
	if op != "serve" {
		return errors.New("supported commands: serve, drain, resume, status, gc, version")
	}
	flags := flag.NewFlagSet(op, flag.ContinueOnError)
	listen := flags.String("listen", ":8090", "Private HTTP listen address")
	state := flags.String("state", "/state", "Worker journal directory")
	media := flags.String("media", "/media", "Read-only media root")
	cache := flags.String("cache", "/cache", "Persistent runtime caches")
	token := flags.String("token-file", "", "Bearer token file (required)")
	hfToken := flags.String("hf-token-file", "", "Optional Hugging Face token file")
	python := flags.String("python", "python", "Deployment-owned interpreter")
	script := flags.String("script", "scripts/whisperx_transcribe.py", "Deployment-owned pipeline")
	profile := flags.String("profile", "", "Immutable engine profile (required)")
	models := flags.String("models", "medium.en", "Comma-separated allowed model identifiers")
	diarModels := flags.String("diarization-models", "pyannote/speaker-diarization-3.1", "Comma-separated allowed diarization model identifiers")
	device := flags.String("device", "cpu", "cpu or cuda")
	compute := flags.String("compute-type", "int8", "Deployment-owned compute type")
	batch := flags.Int("batch-size", 16, "Deployment-owned batch size")
	running := flags.Int("max-running", 1, "Maximum running jobs")
	waiting := flags.Int("max-waiting", 4, "Maximum waiting jobs")
	budget := flags.Int("execution-timeout", 21600, "Cumulative execution budget in seconds")
	resultLimit := flags.Int64("result-limit", transcription.DefaultResultLimit, "Maximum result/checkpoint bytes per artifact")
	minFree := flags.Uint64("min-free-bytes", 512<<20, "State free-space acceptance floor")
	grace := flags.Duration("stop-grace", 180*time.Second, "Chunk stop grace before forced group cleanup")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *grace <= 0 {
		return errors.New("invalid worker arguments")
	}
	if *token == "" || *profile == "" {
		return errors.New("serve requires --token-file and --profile")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	probe, stopProbe := context.WithTimeout(ctx, 30*time.Second)
	runner, err := whisperxworker.NewPythonRunner(probe, whisperxworker.PythonConfig{Python: *python, Script: *script, CacheRoot: *cache, HFTokenFile: *hfToken, LogLevel: "warning", Device: *device})
	stopProbe()
	if err != nil {
		return err
	}
	pipelineHash, err := runner.PipelineSHA256()
	if err != nil {
		return err
	}
	cfg := whisperxworker.Config{
		StateDir: *state, MediaRoot: *media, TokenFile: *token,
		EngineProfile: *profile, WorkerVersion: workerVersion,
		Device: *device, ComputeType: *compute, BatchSize: *batch,
		ExecutionTimeoutSeconds: *budget, MaxRunning: *running, MaxWaiting: *waiting,
		ResultLimit: *resultLimit, MinFreeBytes: *minFree,
		Models: splitModels(*models), DiarizationModels: splitModels(*diarModels),
		PackageVersions: runner.PackageVersions(), PipelineSHA256: pipelineHash,
	}
	worker, err := whisperxworker.Open(ctx, cfg, runner)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: *listen, Handler: worker.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServe() }()
	gcTicker := time.NewTicker(time.Hour)
	defer gcTicker.Stop()
	var serveErr error
serveLoop:
	for {
		select {
		case <-ctx.Done():
			break serveLoop
		case serveErr = <-stopped:
			break serveLoop
		case <-gcTicker.C:
			if err = worker.PruneAcknowledged(time.Now().UTC()); err != nil {
				slog.Error("worker_gc_failed")
			}
		}
	}
	shutdown, stopShutdown := context.WithTimeout(context.Background(), *grace)
	defer stopShutdown()
	shutdownErr := server.Shutdown(shutdown)
	if shutdownErr != nil {
		_ = server.Close()
	}
	closeErr := worker.Close(shutdown)
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(serveErr, shutdownErr, closeErr)
}
func splitModels(s string) []string {
	values := []string{}
	for _, value := range strings.Split(s, ",") {
		if v := strings.TrimSpace(value); v != "" {
			values = append(values, v)
		}
	}
	return values
}

func maintenance(op string, args []string) error {
	flags := flag.NewFlagSet(op, flag.ContinueOnError)
	endpoint := flags.String("url", "http://whisperx:8090", "Operator-configured private endpoint")
	tokenFile := flags.String("token-file", "", "Bearer token file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional argument")
	}
	base, err := url.Parse(*endpoint)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return errors.New("invalid worker endpoint")
	}
	file, err := os.Open(*tokenFile)
	if err != nil {
		return errors.New("token file unavailable")
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(raw) > 4096 {
		return errors.New("token file unreadable")
	}
	token := strings.TrimSpace(string(raw))
	if len(token) < 32 || strings.ContainsAny(token, "\r\n\t ") {
		return errors.New("invalid service token")
	}
	method, path := "POST", "/v1/admin/"+op
	if op == "status" {
		method, path = "GET", "/v1/capabilities"
	}
	base.Path = path
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, base.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("worker redirects are forbidden") }}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("worker request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("worker returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, transcription.ControlLimit+1))
	if err != nil || len(body) > transcription.ControlLimit {
		return errors.New("invalid worker response")
	}
	if !json.Valid(body) {
		return errors.New("invalid worker JSON response")
	}
	// Response bodies are authenticated protocol data; never print arbitrary
	// transport errors or token-bearing request objects.
	fmt.Println(string(body))
	return nil
}
