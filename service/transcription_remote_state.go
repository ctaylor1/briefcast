package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type remoteEnvelope struct {
	Version                  int                             `json:"version"`
	Backend                  string                          `json:"backend"`
	DeploymentID             string                          `json:"deployment_id"`
	EpisodeID                string                          `json:"episode_id"`
	GenerationID             string                          `json:"generation_id"`
	JobID                    string                          `json:"job_id"`
	Attempt                  int                             `json:"attempt"`
	WorkerInstanceID         *string                         `json:"worker_instance_id"`
	PreparedWorkerInstanceID string                          `json:"prepared_worker_instance_id,omitempty"`
	EngineProfile            string                          `json:"engine_profile"`
	InputSHA256              string                          `json:"input_sha256"`
	OptionsSHA256            string                          `json:"options_sha256"`
	RequestDigest            string                          `json:"request_digest"`
	RequestJSON              string                          `json:"request_json"`
	LastRevision             int64                           `json:"last_revision"`
	ReceiptResultSHA256      *string                         `json:"receipt_result_sha256"`
	LegacyImport             json.RawMessage                 `json:"legacy_import"`
	Expected                 transcription.EffectiveConfig   `json:"expected_effective_config"`
	TerminalState            string                          `json:"terminal_state,omitempty"`
	TerminalRetryable        bool                            `json:"terminal_retryable,omitempty"`
	Checkpoint               transcription.CheckpointSummary `json:"checkpoint"`
	Permanent                bool                            `json:"permanent,omitempty"`
	ReconnectFailures        int                             `json:"reconnect_failures,omitempty"`
	RestartReason            string                          `json:"restart_reason,omitempty"`
}

func decodeRemoteEnvelope(raw string) (*remoteEnvelope, error) {
	invalid := remoteProblem("INVALID_CONFIG", "Unsupported or corrupt remote checkpoint", false)
	if len(raw) > 2*transcription.ControlLimit {
		return nil, invalid
	}
	canonical, err := transcription.CanonicalJSON([]byte(raw))
	if err != nil {
		return nil, invalid
	}
	var e remoteEnvelope
	if json.Unmarshal(canonical, &e) != nil || e.Version != 1 || e.Backend != "http" || !transcription.ValidUUID(e.JobID) || !transcription.ValidUUID(e.GenerationID) || !transcription.ValidUUID(e.DeploymentID) || !transcription.ValidUUID(e.EpisodeID) || e.Attempt < 1 || e.LastRevision < 0 || (e.WorkerInstanceID != nil && !transcription.ValidUUID(*e.WorkerInstanceID)) || (e.ReceiptResultSHA256 != nil && !transcription.ValidSHA256(*e.ReceiptResultSHA256)) {
		return nil, invalid
	}
	if e.PreparedWorkerInstanceID != "" && !transcription.ValidUUID(e.PreparedWorkerInstanceID) {
		return nil, invalid
	}
	r, body, err := transcription.DecodeRequest([]byte(e.RequestJSON))
	if err != nil {
		return nil, invalid
	}
	options, _ := transcription.MarshalCanonical(r.Options)
	if string(body) != e.RequestJSON || transcription.Digest(body) != e.RequestDigest || transcription.Digest(options) != e.OptionsSHA256 || r.Input.SHA256 != e.InputSHA256 || r.DeploymentID != e.DeploymentID || r.EpisodeID != e.EpisodeID || r.GenerationID != e.GenerationID || r.Attempt != e.Attempt || r.EngineProfile != e.EngineProfile {
		return nil, invalid
	}
	return &e, nil
}

func remoteOptions(cfg WhisperXConfig) transcription.Options {
	return transcription.Options{Model: cfg.Model, Language: cfg.Language, ChunkSeconds: cfg.ChunkSeconds, Align: cfg.Align, Diarization: cfg.Diarization, DiarizationModel: cfg.DiarizationModel, MinSpeakers: cfg.MinSpeakers, MaxSpeakers: cfg.MaxSpeakers, VADMethod: cfg.VADMethod, ASROptions: transcription.ASROptions{BeamSize: cfg.BeamSize, Patience: cfg.Patience, ConditionOnPreviousText: cfg.ConditionOnPrev, InitialPrompt: cfg.InitialPrompt}, VADOptions: transcription.VADOptions{ChunkSize: cfg.VADChunkSize, VADOnset: cfg.VADOnset, VADOffset: cfg.VADOffset}}
}

type remoteContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r remoteContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func fingerprintRemoteMedia(ctx context.Context, rootPath, audioPath string) (transcription.MediaInput, error) {
	if err := ctx.Err(); err != nil {
		return transcription.MediaInput{}, err
	}
	invalid := remoteProblem("INPUT_UNREADABLE", "Local audio is outside the media root or is not readable", false)
	rootAbs, err := filepath.Abs(rootPath)
	if err != nil {
		return transcription.MediaInput{}, invalid
	}
	audioAbs, err := filepath.Abs(audioPath)
	if err != nil {
		return transcription.MediaInput{}, invalid
	}
	rel, err := filepath.Rel(rootAbs, audioAbs)
	slash := filepath.ToSlash(rel)
	if err != nil || !transcription.ValidMediaPath(slash) {
		return transcription.MediaInput{}, invalid
	}
	root, err := os.OpenRoot(rootAbs)
	if err != nil {
		return transcription.MediaInput{}, invalid
	}
	defer root.Close()
	// Stat rejects special files before opening (a FIFO open could block).
	info, err := root.Stat(rel)
	if err != nil {
		if os.IsNotExist(err) {
			return transcription.MediaInput{}, remoteProblem("INPUT_MISSING", "Local audio does not exist", false)
		}
		return transcription.MediaInput{}, invalid
	}
	if !info.Mode().IsRegular() {
		return transcription.MediaInput{}, invalid
	}
	f, err := root.Open(rel)
	if err != nil {
		return transcription.MediaInput{}, invalid
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return transcription.MediaInput{}, invalid
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(remoteContextReader{ctx: ctx, reader: f}, before.Size()+1))
	if ctx.Err() != nil {
		return transcription.MediaInput{}, ctx.Err()
	}
	if err != nil {
		return transcription.MediaInput{}, invalid
	}
	after, err := f.Stat()
	if err != nil || n != before.Size() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return transcription.MediaInput{}, remoteProblem("INPUT_CHANGED", "Local audio changed while hashing", false)
	}
	return transcription.MediaInput{RelativePath: slash, SizeBytes: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func (b *httpTranscriptionBackend) prepare(ctx context.Context, item db.PodcastItem) (*remoteEnvelope, error) {
	if strings.TrimSpace(item.TranscriptCheckpointJSON) != "" {
		var marker map[string]json.RawMessage
		if json.Unmarshal([]byte(item.TranscriptCheckpointJSON), &marker) != nil {
			return nil, remoteProblem("INVALID_CONFIG", "Corrupt stored checkpoint", false)
		}
		if _, versioned := marker["version"]; versioned || marker["backend"] != nil {
			e, err := decodeRemoteEnvelope(item.TranscriptCheckpointJSON)
			if err != nil {
				return nil, err
			}
			if e.EpisodeID != item.ID || e.DeploymentID != b.deployment {
				return nil, remoteProblem("INVALID_CONFIG", "Checkpoint identity differs from episode or deployment", false)
			}
			if e.TerminalState == "" {
				return e, nil
			}
			manual := item.TranscriptProgressStage == "queued_for_retry" && item.TranscriptStatus == "pending_whisperx"
			if (!e.TerminalRetryable || !b.cfg.RetryFailed) && !manual {
				return nil, remoteProblem("INVALID_CONFIG", "Terminal attempt requires explicit repair", false)
			}
			r, _, _ := transcription.DecodeRequest([]byte(e.RequestJSON))
			input, err := fingerprintRemoteMedia(ctx, getEnvString("DATA", "/assets"), item.DownloadPath)
			if err != nil {
				return nil, err
			}
			if input != r.Input {
				return nil, remoteProblem("INPUT_CHANGED", "Local audio differs from persisted submission", false)
			}
			r.Attempt++
			r.ResumeFromJobID = nil
			if e.Checkpoint.Compatible && e.Checkpoint.CompletedSeconds > 0 && e.Checkpoint.Format == transcription.CheckpointFormat && e.EngineProfile == b.caps.EngineProfile {
				id := e.JobID
				r.ResumeFromJobID = &id
			} else {
				e.RestartReason = "No verified compatible worker checkpoint; retry starts from zero"
			}
			// Preserve the accepted options even if environment settings changed.
			prepared, err := makeRemoteEnvelope(r, e.Expected, e.LegacyImport, e.RestartReason)
			if prepared != nil {
				prepared.PreparedWorkerInstanceID = b.caps.WorkerInstanceID
			}
			return prepared, err
		}
	}
	input, err := fingerprintRemoteMedia(ctx, getEnvString("DATA", "/assets"), item.DownloadPath)
	if err != nil {
		return nil, err
	}
	profile := b.cfg.EngineProfile
	if profile == "" {
		profile = b.caps.EngineProfile
	}
	expected := transcription.EffectiveConfig{EngineProfile: profile, Device: b.cfg.Device, ComputeType: b.cfg.ComputeType, BatchSize: b.cfg.BatchSize, ExecutionTimeoutSeconds: getEnvInt(whisperxTimeoutEnv, defaultWhisperXTimeoutSeconds)}
	r := transcription.JobRequest{ContractVersion: 1, DeploymentID: b.deployment, EpisodeID: item.ID, GenerationID: uuid.NewString(), Attempt: 1, Input: input, EngineProfile: profile, Options: remoteOptions(b.cfg)}
	if err = b.checkOptions(r.Options, profile, expected); err != nil {
		return nil, err
	}
	var legacy json.RawMessage
	reason := ""
	if strings.TrimSpace(item.TranscriptCheckpointJSON) != "" {
		// Legacy sidecars lack option/input provenance. Preserve the original;
		// trusted rollout import requires evidence supplied by Stage G tooling.
		var old transcription.LegacyCheckpoint
		if _, err = transcription.CanonicalJSON([]byte(item.TranscriptCheckpointJSON)); err != nil || json.Unmarshal([]byte(item.TranscriptCheckpointJSON), &old) != nil {
			return nil, remoteProblem("INVALID_CONFIG", "Corrupt legacy checkpoint", false)
		}
		reason = "Legacy checkpoint has no verified input/options provenance; restarted from zero"
		legacy, _ = json.Marshal(struct {
			Checkpoint json.RawMessage `json:"checkpoint"`
			Reason     string          `json:"reason"`
		}{json.RawMessage(item.TranscriptCheckpointJSON), reason})
	}
	prepared, err := makeRemoteEnvelope(r, expected, legacy, reason)
	if prepared != nil {
		prepared.PreparedWorkerInstanceID = b.caps.WorkerInstanceID
	}
	return prepared, err
}

func makeRemoteEnvelope(r transcription.JobRequest, expected transcription.EffectiveConfig, legacy json.RawMessage, reason string) (*remoteEnvelope, error) {
	raw, err := transcription.MarshalCanonical(r)
	if err != nil {
		return nil, err
	}
	if _, _, err = transcription.DecodeRequest(raw); err != nil {
		return nil, remoteProblem("INVALID_CONFIG", "Invalid transcription options", false)
	}
	options, _ := transcription.MarshalCanonical(r.Options)
	return &remoteEnvelope{Version: 1, Backend: "http", DeploymentID: r.DeploymentID, EpisodeID: r.EpisodeID, GenerationID: r.GenerationID, JobID: uuid.NewString(), Attempt: r.Attempt, EngineProfile: r.EngineProfile, InputSHA256: r.Input.SHA256, OptionsSHA256: transcription.Digest(options), RequestDigest: transcription.Digest(raw), RequestJSON: string(raw), Expected: expected, LegacyImport: legacy, RestartReason: reason}, nil
}

var errRemoteOwnership = errors.New("transcription ownership or generation changed")

// Fence the existing job-lock row by its exact acquired/renewed date, not its
// reusable row ID. Renewals and writes share this mutex; successors cannot be
// renewed/unlocked by the old invocation. No schema change is required.
type remoteLease struct {
	mu     sync.Mutex
	lock   db.JobLock
	ctx    context.Context
	cancel context.CancelFunc
}

func (l *remoteLease) predicate(q *gorm.DB) *gorm.DB {
	return q.Where("EXISTS (SELECT 1 FROM job_locks WHERE id = ? AND date = ? AND duration = ? AND deleted_at IS NULL)", l.lock.ID, l.lock.Date, l.lock.Duration)
}
func (l *remoteLease) check() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.checkLocked()
}
func (l *remoteLease) checkLocked() error {
	if err := l.ctx.Err(); err != nil {
		return err
	}
	if !l.lock.IsLockedAt(time.Now().UTC()) {
		l.cancel()
		return errRemoteOwnership
	}
	var count int64
	err := db.DB.Model(&db.JobLock{}).Where("id = ? AND date = ? AND duration = ? AND deleted_at IS NULL", l.lock.ID, l.lock.Date, l.lock.Duration).Count(&count).Error
	if err != nil || count != 1 {
		l.cancel()
		if err != nil {
			return err
		}
		return errRemoteOwnership
	}
	return nil
}
func (l *remoteLease) renew() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.checkLocked(); err != nil {
		return err
	}
	now := time.Now().UTC()
	result := db.DB.Model(&db.JobLock{}).Where("id = ? AND date = ? AND duration = ?", l.lock.ID, l.lock.Date, l.lock.Duration).Updates(map[string]any{"date": now})
	if result.Error != nil || result.RowsAffected != 1 {
		l.cancel()
		if result.Error != nil {
			return result.Error
		}
		return errRemoteOwnership
	}
	l.lock.Date = now
	return nil
}
func (l *remoteLease) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	db.DB.Model(&db.JobLock{}).Where("id = ? AND date = ? AND duration = ?", l.lock.ID, l.lock.Date, l.lock.Duration).Updates(map[string]any{"date": time.Time{}, "duration": 0})
}

func (l *remoteLease) update(item *db.PodcastItem, updates map[string]any, receipt bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.checkLocked(); err != nil {
		return err
	}
	q := db.DB.Model(&db.PodcastItem{}).Where("id = ? AND deleted_at IS NULL AND download_path = ? AND download_status = ? AND file_url = ? AND guid = ? AND transcript_status = ? AND COALESCE(transcript_checkpoint_json, '') = ?", item.ID, item.DownloadPath, db.Downloaded, item.FileURL, item.GUID, item.TranscriptStatus, item.TranscriptCheckpointJSON)
	if receipt {
		q = q.Where("transcript_json = ?", item.TranscriptJSON)
	} else {
		q = q.Where("COALESCE(transcript_json, '') = ''")
	}
	result := l.predicate(q).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errRemoteOwnership
	}
	return nil
}
