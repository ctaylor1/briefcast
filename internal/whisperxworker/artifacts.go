package whisperxworker

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
)

type checkpointBinding struct {
	Format          string `json:"format"`
	InputSHA256     string `json:"input_sha256"`
	OptionsSHA256   string `json:"options_sha256"`
	PipelineVersion string `json:"pipeline_version"`
	EngineProfile   string `json:"engine_profile"`
	DeploymentID    string `json:"deployment_id"`
	EpisodeID       string `json:"episode_id"`
	GenerationID    string `json:"generation_id"`
}
type checkpointPayload struct {
	Segments         json.RawMessage   `json:"segments"`
	CompletedSeconds float64           `json:"completed_seconds"`
	TotalSeconds     float64           `json:"total_seconds"`
	Language         string            `json:"language"`
	Worker           checkpointBinding `json:"worker"`
}

func binding(r *record) checkpointBinding {
	return checkpointBinding{transcription.CheckpointFormat, r.Request.Input.SHA256, r.Status.EffectiveConfig.OptionsSHA256, r.PipelineVersion, r.Request.EngineProfile, r.Request.DeploymentID, r.Request.EpisodeID, r.Request.GenerationID}
}
func validateCheckpoint(raw []byte, r *record) (checkpointPayload, []byte, error) {
	var cp checkpointPayload
	b, err := transcription.CanonicalJSON(raw)
	if err != nil {
		return cp, nil, err
	}
	if err = json.Unmarshal(b, &cp); err != nil {
		return cp, nil, err
	}
	if cp.Worker != binding(r) || cp.CompletedSeconds <= 0 || cp.TotalSeconds <= 0 || cp.CompletedSeconds > cp.TotalSeconds || cp.Language == "" || (cp.CompletedSeconds != cp.TotalSeconds && cp.CompletedSeconds/float64(r.Request.Options.ChunkSeconds) != float64(int64(cp.CompletedSeconds/float64(r.Request.Options.ChunkSeconds)))) {
		return cp, nil, errors.New("invalid checkpoint fingerprints or offset")
	}
	if err = transcription.ValidateSegments(cp.Segments); err != nil {
		return cp, nil, err
	}
	return cp, b, nil
}
func (w *Worker) readCheckpoint(r *record) ([]byte, error) {
	if r.Checkpoint == nil {
		return nil, os.ErrNotExist
	}
	ref := r.Checkpoint
	if !transcription.ValidMediaPath(ref.Path) || !transcription.ValidSHA256(ref.SHA256) {
		return nil, errors.New("invalid checkpoint reference")
	}
	root, err := os.OpenRoot(w.store.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(filepath.FromSlash(ref.Path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := readLimit(f, w.cfg.ResultLimit)
	if err != nil {
		return nil, err
	}
	cp, b, err := validateCheckpoint(raw, r)
	if err != nil {
		return nil, err
	}
	if transcription.Digest(b) != ref.SHA256 || cp.CompletedSeconds != ref.CompletedSeconds {
		return nil, errors.New("checkpoint integrity failed")
	}
	// The script consumes inline segments; no worker-private path is sent on wire.
	return b, nil
}
func (w *Worker) captureCheckpoint(r *record) error {
	raw, err := readBounded(filepath.Join(w.store.jobDir(r.Status.JobID), "segments.json"), w.cfg.ResultLimit)
	if os.IsNotExist(err) && r.Checkpoint != nil {
		_, err = w.readCheckpoint(r)
		return err
	}
	if err != nil {
		return err
	}
	cp, b, err := validateCheckpoint(raw, r)
	if err != nil {
		return err
	}
	digest := transcription.Digest(b)
	if r.Checkpoint != nil && r.Checkpoint.SHA256 == digest {
		return nil
	}
	if r.Checkpoint != nil && cp.CompletedSeconds < r.Checkpoint.CompletedSeconds {
		return nil
	}
	if w.cfg.MinFreeBytes > 0 {
		free, spaceErr := freeBytes(w.store.dir)
		if spaceErr != nil || free < w.cfg.MinFreeBytes+uint64(len(b)) {
			return transcription.Error("STATE_UNAVAILABLE", "Insufficient checkpoint storage space", true)
		}
	}
	var segments []json.RawMessage
	if err = json.Unmarshal(cp.Segments, &segments); err != nil {
		return err
	}
	name := "checkpoint-" + digest + ".json"
	relative := filepath.ToSlash(filepath.Join("jobs", r.Status.JobID, name))
	if err = durableWrite(filepath.Join(w.store.jobDir(r.Status.JobID), name), b); err != nil {
		return transcription.Error("STATE_UNAVAILABLE", "Checkpoint persistence failed", true)
	}
	r.Checkpoint = &checkpointRef{Path: relative, SHA256: digest, CompletedSeconds: cp.CompletedSeconds, SegmentCount: len(segments)}
	r.Status.Checkpoint.CompletedSeconds = cp.CompletedSeconds
	r.Status.Checkpoint.SegmentCount = len(segments)
	r.Status.Checkpoint.Compatible = true
	r.Status.CompletedSeconds = max(r.Status.CompletedSeconds, cp.CompletedSeconds)
	r.Status.TotalSeconds = max(r.Status.TotalSeconds, cp.TotalSeconds)
	return nil
}

func (w *Worker) normalizeResult(r *record) ([]byte, string, error) {
	raw, err := readBounded(filepath.Join(w.store.jobDir(r.Status.JobID), "stdout.json"), w.cfg.ResultLimit)
	if err != nil {
		return nil, "", err
	}
	if err = transcription.ValidatePipelineTranscript(raw); err != nil {
		return nil, "", transcription.Error("INTERNAL", "Child produced an invalid transcript", true)
	}
	var body map[string]json.RawMessage
	if err = json.Unmarshal(raw, &body); err != nil {
		return nil, "", err
	}
	for key, want := range map[string]any{"model": r.Request.Options.Model, "device": r.Status.EffectiveConfig.Device, "compute_type": r.Status.EffectiveConfig.ComputeType, "batch_size": r.Status.EffectiveConfig.BatchSize, "asr_options": r.Request.Options.ASROptions, "vad_options": r.Request.Options.VADOptions} {
		expected, e := transcription.MarshalCanonical(want)
		if e != nil {
			return nil, "", e
		}
		actual, e := transcription.CanonicalJSON(body[key])
		if e != nil || string(expected) != string(actual) {
			return nil, "", transcription.Error("INVALID_CONFIG", "Child configuration differs from accepted configuration", false)
		}
	}
	var alignment struct {
		Requested bool `json:"requested"`
	}
	var diarization struct {
		Enabled     bool   `json:"enabled"`
		Model       string `json:"model"`
		MinSpeakers int    `json:"min_speakers"`
		MaxSpeakers int    `json:"max_speakers"`
	}
	if err = json.Unmarshal(body["alignment"], &alignment); err != nil {
		return nil, "", err
	}
	if err = json.Unmarshal(body["diarization"], &diarization); err != nil {
		return nil, "", err
	}
	o := r.Request.Options
	if alignment.Requested != o.Align || diarization.Enabled != o.Diarization || diarization.Model != o.DiarizationModel || diarization.MinSpeakers != o.MinSpeakers || diarization.MaxSpeakers != o.MaxSpeakers {
		return nil, "", transcription.Error("INVALID_CONFIG", "Child stage options differ from accepted configuration", false)
	}
	var metadata map[string]json.RawMessage
	if err = json.Unmarshal(body["metadata"], &metadata); err != nil {
		return nil, "", err
	}
	var generated string
	if json.Unmarshal(metadata["generated_at"], &generated) != nil {
		return nil, "", errors.New("invalid result generation time")
	}
	if _, err = time.Parse(time.RFC3339Nano, generated); err != nil {
		return nil, "", err
	}
	fields := map[string]any{"worker_version": w.cfg.WorkerVersion, "pipeline_version": r.PipelineVersion, "api_version": "1.0.0", "engine_profile": r.Request.EngineProfile, "input_sha256": r.Request.Input.SHA256, "options_sha256": r.Status.EffectiveConfig.OptionsSHA256, "job_id": r.Status.JobID, "package_versions": w.cfg.PackageVersions, "whisperx_version": w.cfg.PackageVersions["whisperx"], "torch_version": w.cfg.PackageVersions["torch"]}
	for key, value := range fields {
		metadata[key], err = json.Marshal(value)
		if err != nil {
			return nil, "", err
		}
	}
	body["metadata"], err = json.Marshal(metadata)
	if err != nil {
		return nil, "", err
	}
	transcript, err := transcription.MarshalCanonical(body)
	if err != nil {
		return nil, "", err
	}
	if err = transcription.ValidateTranscript(transcript); err != nil {
		return nil, "", err
	}
	digest := transcription.Digest(transcript)
	result, err := transcription.MarshalCanonical(transcription.JobResult{ContractVersion: 1, JobID: r.Status.JobID, RequestDigest: r.Status.RequestDigest, ResultSHA256: digest, Transcript: transcript})
	if err != nil {
		return nil, "", err
	}
	if int64(len(result)) > w.cfg.ResultLimit {
		return nil, "", transcription.Error("INTERNAL", "Result exceeds configured size limit", true)
	}
	return result, digest, nil
}
func (w *Worker) resultLocked(r *record) ([]byte, error) {
	if r.Status.State != "succeeded" {
		return nil, errConflict
	}
	if r.Pruned {
		return nil, errGone
	}
	raw, err := readBounded(w.store.resultPath(r.Status.JobID), w.cfg.ResultLimit)
	if err != nil {
		return nil, err
	}
	if _, err = transcription.CanonicalJSON(raw); err != nil {
		return nil, err
	}
	var result transcription.JobResult
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	b, err := transcription.CanonicalJSON(result.Transcript)
	if err != nil {
		return nil, err
	}
	if result.ContractVersion != 1 || result.JobID != r.Status.JobID || result.RequestDigest != r.Status.RequestDigest || result.ResultSHA256 != r.ResultDigest || transcription.Digest(b) != r.ResultDigest {
		return nil, errors.New("result integrity failure")
	}
	if err = transcription.ValidateTranscript(b); err != nil {
		return nil, err
	}
	return raw, nil
}
func (w *Worker) Result(id string) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	r, err := w.store.get(id)
	if err != nil {
		return nil, err
	}
	raw, err := w.resultLocked(r)
	if err != nil && r.Status.State == "succeeded" && !r.Pruned {
		w.unavailable(err)
	}
	return raw, err
}
func (w *Worker) Ack(id, digest string) (transcription.AckResponse, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	r, err := w.store.get(id)
	if err != nil {
		return transcription.AckResponse{}, err
	}
	if r.Status.State != "succeeded" || r.ResultDigest != digest {
		return transcription.AckResponse{}, errConflict
	}
	if r.AcknowledgedAt == nil {
		if _, err = w.resultLocked(r); err != nil {
			return transcription.AckResponse{}, err
		}
		now := time.Now().UTC()
		r.AcknowledgedAt = &now
		if err = w.store.save(r, false); err != nil {
			w.unavailable(err)
			return transcription.AckResponse{}, err
		}
	}
	return transcription.AckResponse{JobID: id, ResultSHA256: r.ResultDigest, AcknowledgedAt: *r.AcknowledgedAt}, nil
}

var errGone = transcription.Error("INTERNAL", "Acknowledged result has been pruned", false)

// PruneAcknowledged retains identities and checkpoints referenced by another
// attempt. Failed/cancelled source checkpoints remain available for resume.
// Persisting the tombstone first makes a crash during deletion safe to repeat.
func (w *Worker) PruneAcknowledged(now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	records, err := w.store.list("WHERE state='succeeded' AND acknowledged=1")
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.AcknowledgedAt == nil || now.Sub(*r.AcknowledgedAt) < 7*24*time.Hour {
			continue
		}
		if !r.Pruned {
			checkpoint := r.Checkpoint
			r.Pruned = true
			r.Checkpoint = nil
			if err = w.store.save(r, false); err != nil {
				w.unavailable(err)
				return err
			}
			if checkpoint != nil {
				if !transcription.ValidMediaPath(checkpoint.Path) {
					return errors.New("invalid checkpoint retention reference")
				}
				var references int
				if err = w.store.db.QueryRow("SELECT COUNT(*) FROM jobs WHERE json_extract(data,'$.checkpoint.path')=?", checkpoint.Path).Scan(&references); err != nil {
					return err
				}
				if references == 0 {
					root, e := os.OpenRoot(w.store.dir)
					if e != nil {
						return e
					}
					e = root.Remove(filepath.FromSlash(checkpoint.Path))
					root.Close()
					if e != nil && !os.IsNotExist(e) {
						return e
					}
				}
			}
		}
		for _, name := range []string{"result.json", "stdout.json", "stderr.log", "progress.json", "resume.json", "segments.json"} {
			if err = os.Remove(filepath.Join(w.store.jobDir(r.Status.JobID), name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if err = syncDirectory(w.store.jobDir(r.Status.JobID)); err != nil {
			return err
		}
	}
	return nil
}

// CollectGarbage is an offline maintenance command and acquires the same
// exclusive state lease as serve. Hot journal copying/maintenance is forbidden.
func CollectGarbage(stateDir string, now time.Time) error {
	s, err := openStore(stateDir)
	if err != nil {
		return err
	}
	defer s.close()
	w := &Worker{store: s}
	return w.PruneAcknowledged(now)
}

// Drop only unreferenced snapshot files and abandoned Go temporary siblings.
// Serialized journal updates keep references from another accepted attempt safe.
func (w *Worker) cleanupArtifacts(r *record) {
	id := r.Status.JobID
	entries, err := os.ReadDir(w.store.jobDir(id))
	if err != nil {
		slog.Warn("worker_artifact_cleanup_failed", "job_id", id)
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		remove := strings.HasPrefix(name, ".write-") && !entry.IsDir()
		if strings.HasPrefix(name, "checkpoint-") && strings.HasSuffix(name, ".json") && transcription.ValidSHA256(strings.TrimSuffix(strings.TrimPrefix(name, "checkpoint-"), ".json")) {
			relative := filepath.ToSlash(filepath.Join("jobs", id, name))
			var references int
			if err = w.store.db.QueryRow("SELECT COUNT(*) FROM jobs WHERE json_extract(data,'$.checkpoint.path')=?", relative).Scan(&references); err != nil {
				slog.Warn("worker_artifact_cleanup_failed", "job_id", id)
				return
			}
			remove = references == 0
		}
		if r.Status.State == "succeeded" && (name == "stdout.json" || name == "resume.json" || name == "segments.json") {
			remove = true
		}
		if remove {
			if err = os.Remove(filepath.Join(w.store.jobDir(id), name)); err != nil && !os.IsNotExist(err) {
				slog.Warn("worker_artifact_cleanup_failed", "job_id", id)
			}
		}
	}
	if err = syncDirectory(w.store.jobDir(id)); err != nil {
		slog.Warn("worker_artifact_cleanup_failed", "job_id", id)
	}
}
