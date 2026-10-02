package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/google/uuid"
)

// This transport is exclusively for the operator-configured worker endpoint.
// Feed/media URL policy is unchanged. No episode URL or path selects a host.
type httpTranscriptionBackend struct {
	cfg               WhisperXConfig
	base              *url.URL
	client            *http.Client
	token, deployment string
	caps              transcription.Capabilities
	poll              time.Duration
}

type remoteError struct {
	problem    *transcription.WorkerError
	status     int
	terminal   bool
	retryAfter time.Duration
}

func (e *remoteError) Error() string { return e.problem.Error() }
func (e *remoteError) Unwrap() error { return e.problem }
func remoteProblem(code, message string, retry bool) *remoteError {
	return &remoteError{problem: transcription.Error(code, message, retry)}
}

func configuredTranscriptionBackend(cfg WhisperXConfig) (transcriptionBackend, error) {
	switch cfg.Backend {
	case "", "local":
		return localTranscriptionBackend{cfg: cfg}, nil
	case "http":
		return newHTTPTranscriptionBackend(cfg)
	default:
		return nil, remoteProblem("INVALID_CONFIG", "Unknown transcription backend", false)
	}
}

func newHTTPTranscriptionBackend(cfg WhisperXConfig) (*httpTranscriptionBackend, error) {
	u, err := url.Parse(cfg.ServiceURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return nil, remoteProblem("INVALID_CONFIG", "Invalid worker endpoint", false)
	}
	raw, err := readRemoteBoundedFile(cfg.ServiceTokenFile, 4096)
	if err != nil {
		return nil, remoteProblem("INVALID_CONFIG", "Worker token file unavailable", false)
	}
	token := strings.TrimSpace(string(raw))
	if len(token) < 32 || strings.ContainsAny(token, "\r\n\t ") {
		return nil, remoteProblem("INVALID_CONFIG", "Invalid worker token", false)
	}
	if cfg.ServiceResultLimit < 0 {
		return nil, remoteProblem("INVALID_CONFIG", "Invalid worker result limit", false)
	}
	if cfg.ServiceResultLimit == 0 {
		cfg.ServiceResultLimit = transcription.DefaultResultLimit
	}
	if cfg.ServiceResultLimit > 1<<30 {
		return nil, remoteProblem("INVALID_CONFIG", "Worker result limit exceeds 1 GiB", false)
	}
	id, err := transcriptionDeploymentID(getEnvString("CONFIG", "/config"))
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 60 * time.Second, MaxIdleConns: 16, MaxIdleConnsPerHost: 4, MaxResponseHeaderBytes: 16 << 10, DisableCompression: true}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("worker redirects forbidden") }}
	poll := time.Duration(getEnvInt(whisperxProgressPollEnv, 2000)) * time.Millisecond
	if poll <= 0 {
		poll = 2 * time.Second
	}
	poll = min(poll, 15*time.Second)
	return &httpTranscriptionBackend{cfg: cfg, base: u, client: client, token: token, deployment: id, poll: poll}, nil
}

func transcriptionDeploymentID(dir string) (string, error) {
	failure := remoteProblem("INVALID_CONFIG", "Deployment identity unavailable or invalid", false)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", failure
	}
	path := filepath.Join(dir, "whisperx-deployment-id")
	read := func() (string, error) {
		raw, err := readRemoteBoundedFile(path, 128)
		if err != nil {
			return "", failure
		}
		id := strings.TrimSpace(string(raw))
		if !transcription.ValidUUID(id) {
			return "", failure
		}
		return id, nil
	}
	if _, err := os.Stat(path); err == nil {
		return read()
	} else if !os.IsNotExist(err) {
		return "", failure
	}
	f, err := os.CreateTemp(dir, ".whisperx-identity-")
	if err != nil {
		return "", failure
	}
	defer os.Remove(f.Name())
	id := uuid.NewString()
	_, err = f.WriteString(id + "\n")
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return "", failure
	}
	// Publish fully flushed content without overwriting a concurrent creator.
	if err = os.Link(f.Name(), path); err != nil && !os.IsExist(err) {
		return "", failure
	}
	if runtime.GOOS != "windows" {
		d, e := os.Open(dir)
		if e != nil {
			return "", failure
		}
		e = d.Sync()
		d.Close()
		if e != nil {
			return "", failure
		}
	}
	return read()
}

func readRemoteBoundedFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	return readRemoteBounded(f, limit)
}

var errRemoteResponseLimit = errors.New("worker response exceeds limit")

func readRemoteBounded(r io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errRemoteResponseLimit
	}
	return raw, nil
}

func (b *httpTranscriptionBackend) request(ctx context.Context, method, path string, body []byte, result bool) ([]byte, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	u := *b.base
	u.Path = path
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(deadline, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, remoteProblem("INVALID_CONFIG", "Invalid worker request", false)
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := b.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, remoteProblem("WORKER_UNAVAILABLE", "Worker transport unavailable", true)
	}
	defer response.Body.Close()
	code := response.StatusCode
	if code != 200 && !(method == "PUT" && code == 201) {
		// Never include remote messages/response bodies in logs or stored errors.
		e := remoteProblem("WORKER_UNAVAILABLE", "Worker request failed", code == 429 || code >= 500)
		e.status = code
		switch code {
		case 401, 403:
			e = remoteProblem("INVALID_CONFIG", "Worker authorization failed", false)
			e.status = code
		case 404:
			e = remoteProblem("STATE_UNAVAILABLE", "Worker job is unknown", false)
			e.status = code
		case 409, 410:
			e = remoteProblem("INVALID_CONFIG", "Worker identity or result conflict", false)
			e.status = code
		case 429:
			e.problem = transcription.Error("BUSY", "Worker is at capacity or draining", true)
		case 400, 413:
			e.problem = transcription.Error("INVALID_CONFIG", "Worker rejected submission", false)
		}
		raw, readErr := readRemoteBounded(response.Body, transcription.ControlLimit)
		if readErr == nil && code != 401 && code != 403 {
			var envelope struct {
				Error *transcription.WorkerError `json:"error"`
			}
			if _, err = transcription.CanonicalJSON(raw); err == nil && json.Unmarshal(raw, &envelope) == nil && envelope.Error != nil && knownWorkerCode(envelope.Error.Code) {
				e.problem.Code = envelope.Error.Code
				if code == 400 || code >= 500 {
					e.problem.Retryable = envelope.Error.Retryable
				}
			}
		}
		if when, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
			e.retryAfter = min(max(time.Until(when), time.Second), 15*time.Second)
		} else if seconds, err := time.ParseDuration(response.Header.Get("Retry-After") + "s"); err == nil {
			e.retryAfter = min(max(seconds, time.Second), 15*time.Second)
		}
		return nil, code, e
	}
	limit := int64(transcription.ControlLimit)
	if result {
		limit = b.cfg.ServiceResultLimit
	}
	var raw []byte
	if result {
		// Bound the response on disk before materializing JSON for validation/DB.
		f, e := os.CreateTemp("", "briefcast-worker-result-")
		if e != nil {
			return nil, code, remoteProblem("STATE_UNAVAILABLE", "Result temporary storage unavailable", true)
		}
		defer os.Remove(f.Name())
		defer f.Close()
		n, e := io.Copy(f, io.LimitReader(response.Body, limit+1))
		if e != nil {
			var diskError *os.PathError
			if errors.As(e, &diskError) {
				return nil, code, remoteProblem("STATE_UNAVAILABLE", "Result temporary storage failed", true)
			}
			return nil, code, remoteProblem("WORKER_UNAVAILABLE", "Worker result transfer interrupted", true)
		}
		if n > limit {
			return nil, code, remoteProblem("INVALID_CONFIG", "Invalid worker result size", false)
		}
		if _, e = f.Seek(0, io.SeekStart); e != nil {
			return nil, code, remoteProblem("STATE_UNAVAILABLE", "Result temporary storage unreadable", true)
		}
		raw, err = readRemoteBounded(f, limit)
	} else {
		raw, err = readRemoteBounded(response.Body, limit)
	}
	if err != nil {
		if errors.Is(err, errRemoteResponseLimit) {
			return nil, code, remoteProblem("INVALID_CONFIG", "Invalid worker response size", false)
		}
		if result {
			return nil, code, remoteProblem("STATE_UNAVAILABLE", "Result temporary storage unreadable", true)
		}
		return nil, code, remoteProblem("WORKER_UNAVAILABLE", "Worker response transfer interrupted", true)
	}
	canonical, err := transcription.CanonicalJSON(raw)
	if err != nil {
		return nil, code, remoteProblem("INVALID_CONFIG", "Invalid worker JSON response", false)
	}
	return canonical, code, nil
}

func knownWorkerCode(code string) bool {
	return slices.Contains([]string{"WORKER_UNAVAILABLE", "BUSY", "MODEL_UNAVAILABLE", "MODEL_ACCESS_DENIED", "INVALID_CONFIG", "INPUT_MISSING", "INPUT_CHANGED", "INPUT_UNREADABLE", "STATE_UNAVAILABLE", "CHECKPOINT_INCOMPATIBLE", "TIMEOUT", "CANCELLED", "INTERNAL"}, code)
}
func (b *httpTranscriptionBackend) Preflight(ctx context.Context) error {
	raw, _, err := b.request(ctx, "GET", "/v1/capabilities", nil, false)
	if err != nil {
		return err
	}
	var c transcription.Capabilities
	if json.Unmarshal(raw, &c) != nil || !slices.Contains(c.ContractVersions, 1) || !transcription.ValidUUID(c.WorkerInstanceID) || c.EngineProfile == "" || c.PipelineVersion != transcription.PipelineVersion || c.Device == "" || c.ComputeType == "" || c.BatchSize < 1 || len(c.PackageVersions) == 0 {
		return remoteProblem("INVALID_CONFIG", "Incompatible worker capabilities", false)
	}
	if !c.ModelReady {
		return remoteProblem("MODEL_UNAVAILABLE", "Worker runtime is not ready", true)
	}
	b.caps = c
	return nil
}

func (b *httpTranscriptionBackend) checkOptions(o transcription.Options, profile string, expected transcription.EffectiveConfig) error {
	c := b.caps
	if profile != c.EngineProfile || !slices.Contains(c.SupportedModels, o.Model) || !slices.Contains(c.SupportedVADMethods, o.VADMethod) || (o.Diarization && !slices.Contains(c.SupportedDiarizationModels, o.DiarizationModel)) || !slices.Contains(c.CheckpointFormats, transcription.CheckpointFormat) || (expected.Device != "" && expected.Device != "auto" && expected.Device != c.Device) || (expected.ComputeType != "" && expected.ComputeType != "auto" && expected.ComputeType != c.ComputeType) || (expected.BatchSize > 0 && expected.BatchSize != c.BatchSize) {
		return remoteProblem("INVALID_CONFIG", "Worker profile or options differ from application expectations", false)
	}
	return nil
}

// Transcribe is the execution seam; orchestration persists its envelope first.
func (b *httpTranscriptionBackend) Transcribe(ctx context.Context, audioPath, checkpoint string, onProgress func(transcription.ProgressUpdate)) ([]byte, error) {
	e, err := decodeRemoteEnvelope(checkpoint)
	if err != nil {
		return nil, err
	}
	result, err := b.execute(ctx, e, func(e *remoteEnvelope, s transcription.JobStatus) error {
		if onProgress != nil {
			raw, _ := json.Marshal(e)
			onProgress(transcription.ProgressUpdate{Stage: s.Stage, Percent: min(s.Percent, 99), Checkpoint: raw})
		}
		return ctx.Err()
	}, func() error { return ctx.Err() })
	if err != nil {
		return nil, err
	}
	return result.Transcript, nil
}

func (b *httpTranscriptionBackend) execute(ctx context.Context, e *remoteEnvelope, persist func(*remoteEnvelope, transcription.JobStatus) error, check func() error) (*transcription.JobResult, error) {
	if e.DeploymentID != b.deployment {
		return nil, remoteProblem("INVALID_CONFIG", "Deployment identity differs from persisted job", false)
	}
	r, _, err := transcription.DecodeRequest([]byte(e.RequestJSON))
	if err != nil {
		return nil, remoteProblem("INVALID_CONFIG", "Invalid persisted submission", false)
	}
	for {
		if err = check(); err != nil {
			return nil, err
		}
		raw, code, requestErr := b.request(ctx, "GET", "/v1/jobs/"+e.JobID, nil, false)
		if code == 404 {
			if e.WorkerInstanceID != nil || (e.PreparedWorkerInstanceID != "" && e.PreparedWorkerInstanceID != b.caps.WorkerInstanceID) {
				return nil, remoteProblem("STATE_UNAVAILABLE", "Accepted job is missing; operator reconciliation required", false)
			}
			if err = b.checkOptions(r.Options, e.EngineProfile, e.Expected); err != nil {
				return nil, err
			}
			if err = check(); err != nil {
				return nil, err
			}
			raw, _, requestErr = b.request(ctx, "PUT", "/v1/jobs/"+e.JobID, []byte(e.RequestJSON), false)
		}
		if requestErr != nil {
			return nil, requestErr
		}
		var s transcription.JobStatus
		if json.Unmarshal(raw, &s) != nil || !validRemoteStatus(s, e, r) {
			return nil, remoteProblem("INVALID_CONFIG", "Worker status does not match persisted job", false)
		}
		if s.Revision >= e.LastRevision {
			e.ReconnectFailures = 0
			e.WorkerInstanceID = &s.WorkerInstanceID
			e.LastRevision = s.Revision
			if s.State == "failed" || s.State == "cancelled" {
				e.TerminalState = s.State
				e.TerminalRetryable = s.Error != nil && s.Error.Retryable
				e.Checkpoint = s.Checkpoint
			}
			if err = persist(e, s); err != nil {
				return nil, err
			}
			if err = check(); err != nil {
				return nil, err
			}
			switch s.State {
			case "succeeded":
				return b.result(ctx, e, r, s)
			case "failed", "cancelled":
				code := "INTERNAL"
				if s.Error != nil && knownWorkerCode(s.Error.Code) {
					code = s.Error.Code
				}
				problem := remoteProblem(code, "Worker attempt ended unsuccessfully", e.TerminalRetryable)
				problem.terminal = true
				return nil, problem
			}
		}
		wait := min(b.poll+time.Duration(rand.Int64N(int64(b.poll/5)+1)), 15*time.Second)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func validRemoteStatus(s transcription.JobStatus, e *remoteEnvelope, r transcription.JobRequest) bool {
	if e.PreparedWorkerInstanceID != "" && e.PreparedWorkerInstanceID != s.WorkerInstanceID {
		return false
	}
	if s.ContractVersion != 1 || s.JobID != e.JobID || s.RequestDigest != e.RequestDigest || !transcription.ValidUUID(s.WorkerInstanceID) || (e.WorkerInstanceID != nil && *e.WorkerInstanceID != s.WorkerInstanceID) || s.Attempt != e.Attempt || s.Revision < 1 || s.Percent < 0 || s.Percent > 100 || math.IsNaN(s.CompletedSeconds) || s.CompletedSeconds < 0 || s.TotalSeconds < s.CompletedSeconds || s.CreatedAt.IsZero() || s.UpdatedAt.IsZero() || s.EffectiveConfig.EngineProfile != r.EngineProfile || s.EffectiveConfig.OptionsSHA256 != e.OptionsSHA256 || s.EffectiveConfig.BatchSize < 1 || s.EffectiveConfig.ExecutionTimeoutSeconds < 1 {
		return false
	}
	if (e.Expected.Device != "" && e.Expected.Device != "auto" && s.EffectiveConfig.Device != e.Expected.Device) || (e.Expected.ComputeType != "" && e.Expected.ComputeType != "auto" && s.EffectiveConfig.ComputeType != e.Expected.ComputeType) || (e.Expected.BatchSize > 0 && s.EffectiveConfig.BatchSize != e.Expected.BatchSize) || (e.Expected.ExecutionTimeoutSeconds > 0 && s.EffectiveConfig.ExecutionTimeoutSeconds != e.Expected.ExecutionTimeoutSeconds) {
		return false
	}
	return slices.Contains([]string{"queued", "running", "cancelling", "succeeded", "failed", "cancelled"}, s.State)
}

func (b *httpTranscriptionBackend) result(ctx context.Context, e *remoteEnvelope, r transcription.JobRequest, s transcription.JobStatus) (*transcription.JobResult, error) {
	raw, _, err := b.request(ctx, "GET", "/v1/jobs/"+e.JobID+"/result", nil, true)
	if err != nil {
		return nil, err
	}
	var result transcription.JobResult
	if json.Unmarshal(raw, &result) != nil || result.ContractVersion != 1 || result.JobID != e.JobID || result.RequestDigest != e.RequestDigest || !transcription.ValidSHA256(result.ResultSHA256) {
		return nil, remoteProblem("INVALID_CONFIG", "Worker result identity mismatch", false)
	}
	transcript, err := transcription.CanonicalJSON(result.Transcript)
	if err != nil || transcription.Digest(transcript) != result.ResultSHA256 || transcription.ValidateTranscript(transcript) != nil {
		return nil, remoteProblem("INVALID_CONFIG", "Worker result validation failed", false)
	}
	var body map[string]json.RawMessage
	_ = json.Unmarshal(transcript, &body)
	var metadata map[string]json.RawMessage
	_ = json.Unmarshal(body["metadata"], &metadata)
	var apiVersion string
	if json.Unmarshal(metadata["api_version"], &apiVersion) != nil || strings.Split(apiVersion, ".")[0] != "1" {
		return nil, remoteProblem("INVALID_CONFIG", "Incompatible worker result API", false)
	}
	if e.EngineProfile == b.caps.EngineProfile {
		expected, _ := transcription.MarshalCanonical(b.caps.PackageVersions)
		actual, parseErr := transcription.CanonicalJSON(metadata["package_versions"])
		if parseErr != nil || !bytes.Equal(expected, actual) {
			return nil, remoteProblem("INVALID_CONFIG", "Worker result package profile mismatch", false)
		}
	}
	for key, want := range map[string]string{"engine_profile": e.EngineProfile, "input_sha256": e.InputSHA256, "options_sha256": e.OptionsSHA256, "job_id": e.JobID, "pipeline_version": transcription.PipelineVersion} {
		var got string
		if json.Unmarshal(metadata[key], &got) != nil || got != want {
			return nil, remoteProblem("INVALID_CONFIG", "Worker result provenance mismatch", false)
		}
	}
	for key, want := range map[string]any{"model": r.Options.Model, "language": r.Options.Language, "device": s.EffectiveConfig.Device, "compute_type": s.EffectiveConfig.ComputeType, "batch_size": s.EffectiveConfig.BatchSize, "asr_options": r.Options.ASROptions, "vad_options": r.Options.VADOptions} {
		if key == "language" && r.Options.Language == "auto" {
			continue
		}
		expected, _ := transcription.MarshalCanonical(want)
		actual, err := transcription.CanonicalJSON(body[key])
		if err != nil || !bytes.Equal(expected, actual) {
			return nil, remoteProblem("INVALID_CONFIG", "Worker result options mismatch", false)
		}
	}
	var alignment struct {
		Requested bool `json:"requested"`
	}
	var diar struct {
		Enabled bool   `json:"enabled"`
		Model   string `json:"model"`
		Min     int    `json:"min_speakers"`
		Max     int    `json:"max_speakers"`
	}
	_ = json.Unmarshal(body["alignment"], &alignment)
	_ = json.Unmarshal(body["diarization"], &diar)
	if alignment.Requested != r.Options.Align || diar.Enabled != r.Options.Diarization || diar.Model != r.Options.DiarizationModel || diar.Min != r.Options.MinSpeakers || diar.Max != r.Options.MaxSpeakers {
		return nil, remoteProblem("INVALID_CONFIG", "Worker result stage options mismatch", false)
	}
	result.Transcript = transcript
	return &result, nil
}

func (b *httpTranscriptionBackend) acknowledge(ctx context.Context, e *remoteEnvelope) error {
	body, _ := json.Marshal(transcription.AckRequest{ResultSHA256: *e.ReceiptResultSHA256})
	raw, _, err := b.request(ctx, "POST", "/v1/jobs/"+e.JobID+"/ack", body, false)
	if err != nil {
		return err
	}
	var ack transcription.AckResponse
	if json.Unmarshal(raw, &ack) != nil || ack.JobID != e.JobID || ack.ResultSHA256 != *e.ReceiptResultSHA256 || ack.AcknowledgedAt.IsZero() {
		return remoteProblem("INVALID_CONFIG", "Invalid worker receipt response", false)
	}
	return nil
}
