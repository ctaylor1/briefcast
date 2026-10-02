package transcription

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const ContractVersion = 1
const CheckpointFormat = "briefcast-whisperx-chunks-v1"
const PipelineVersion = "briefcast-script-v1"
const ControlLimit = 1 << 20
const DefaultResultLimit = 64 << 20

type WorkerError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (e *WorkerError) Error() string { return e.Code + ": " + e.Message }
func Error(code, message string, retryable bool) *WorkerError {
	return &WorkerError{code, message, retryable}
}

type JobRequest struct {
	ContractVersion int        `json:"contract_version"`
	DeploymentID    string     `json:"deployment_id"`
	EpisodeID       string     `json:"episode_id"`
	GenerationID    string     `json:"generation_id"`
	Attempt         int        `json:"attempt"`
	Input           MediaInput `json:"input"`
	EngineProfile   string     `json:"engine_profile"`
	Options         Options    `json:"options"`
	ResumeFromJobID *string    `json:"resume_from_job_id"`
}
type MediaInput struct {
	RelativePath string `json:"relative_path"`
	SizeBytes    int64  `json:"size_bytes"`
	SHA256       string `json:"sha256"`
}
type Options struct {
	Model            string     `json:"model"`
	Language         string     `json:"language"`
	ChunkSeconds     int        `json:"chunk_seconds"`
	Align            bool       `json:"align"`
	Diarization      bool       `json:"diarization"`
	DiarizationModel string     `json:"diarization_model"`
	MinSpeakers      int        `json:"min_speakers"`
	MaxSpeakers      int        `json:"max_speakers"`
	VADMethod        string     `json:"vad_method"`
	ASROptions       ASROptions `json:"asr_options"`
	VADOptions       VADOptions `json:"vad_options"`
}
type ASROptions struct {
	BeamSize                int     `json:"beam_size"`
	Patience                float64 `json:"patience"`
	ConditionOnPreviousText bool    `json:"condition_on_previous_text"`
	InitialPrompt           string  `json:"initial_prompt"`
}
type VADOptions struct {
	ChunkSize int     `json:"chunk_size"`
	VADOnset  float64 `json:"vad_onset"`
	VADOffset float64 `json:"vad_offset"`
}
type EffectiveConfig struct {
	EngineProfile           string `json:"engine_profile"`
	OptionsSHA256           string `json:"options_sha256"`
	Device                  string `json:"device"`
	ComputeType             string `json:"compute_type"`
	BatchSize               int    `json:"batch_size"`
	ExecutionTimeoutSeconds int    `json:"execution_timeout_seconds"`
}
type CheckpointSummary struct {
	Format           string  `json:"format"`
	Compatible       bool    `json:"compatible"`
	CompletedSeconds float64 `json:"completed_seconds"`
	SegmentCount     int     `json:"segment_count"`
	ResumeFromJobID  *string `json:"resume_from_job_id"`
	Reason           string  `json:"reason,omitempty"`
}
type JobStatus struct {
	ContractVersion  int               `json:"contract_version"`
	JobID            string            `json:"job_id"`
	RequestDigest    string            `json:"request_digest"`
	WorkerInstanceID string            `json:"worker_instance_id"`
	State            string            `json:"state"`
	Revision         int64             `json:"revision"`
	Stage            string            `json:"stage"`
	Percent          int               `json:"percent"`
	CompletedSeconds float64           `json:"completed_seconds"`
	TotalSeconds     float64           `json:"total_seconds"`
	Attempt          int               `json:"attempt"`
	Checkpoint       CheckpointSummary `json:"checkpoint"`
	EffectiveConfig  EffectiveConfig   `json:"effective_config"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	StartedAt        *time.Time        `json:"started_at"`
	FinishedAt       *time.Time        `json:"finished_at"`
	Error            *WorkerError      `json:"error"`
}
type JobResult struct {
	ContractVersion int             `json:"contract_version"`
	JobID           string          `json:"job_id"`
	RequestDigest   string          `json:"request_digest"`
	ResultSHA256    string          `json:"result_sha256"`
	Transcript      json.RawMessage `json:"transcript"`
}
type AckRequest struct {
	ResultSHA256 string `json:"result_sha256"`
}
type AckResponse struct {
	JobID          string    `json:"job_id"`
	ResultSHA256   string    `json:"result_sha256"`
	AcknowledgedAt time.Time `json:"acknowledged_at"`
}
type MaintenanceStatus struct {
	Accepting             bool `json:"accepting"`
	Queued                int  `json:"queued"`
	Running               int  `json:"running"`
	UnacknowledgedResults int  `json:"unacknowledged_results"`
}
type Capacity struct {
	MaxRunning int  `json:"max_running"`
	MaxWaiting int  `json:"max_waiting"`
	Running    int  `json:"running"`
	Waiting    int  `json:"waiting"`
	Accepting  bool `json:"accepting"`
}
type Capabilities struct {
	ContractVersions           []int             `json:"contract_versions"`
	WorkerInstanceID           string            `json:"worker_instance_id"`
	WorkerVersion              string            `json:"worker_version"`
	PipelineVersion            string            `json:"pipeline_version"`
	EngineProfile              string            `json:"engine_profile"`
	PackageVersions            map[string]string `json:"package_versions"`
	SupportedModels            []string          `json:"supported_models"`
	SupportedDiarizationModels []string          `json:"supported_diarization_models"`
	SupportedVADMethods        []string          `json:"supported_vad_methods"`
	CheckpointFormats          []string          `json:"checkpoint_formats"`
	Device                     string            `json:"device"`
	ComputeType                string            `json:"compute_type"`
	BatchSize                  int               `json:"batch_size"`
	ModelReady                 bool              `json:"model_ready"`
	Capacity                   Capacity          `json:"capacity"`
}

var shaPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidSHA256(s string) bool { return shaPattern.MatchString(s) }
func ValidUUID(s string) bool {
	u, err := uuid.Parse(s)
	return err == nil && u != uuid.Nil && u.String() == s
}
func ValidMediaPath(s string) bool {
	if s == "" || utf8.RuneCountInString(s) > 4096 || strings.ContainsAny(s, "\\:\x00") || strings.HasPrefix(s, "/") || path.Clean(s) != s || s == "." {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if part == ".." || part == "." || part == "" {
			return false
		}
	}
	return true
}

// DecodeStrict rejects missing, null, duplicate and unknown request fields.
// Canonicalization happens before typed decoding so 1 and 1.0 are identical.
func DecodeStrict(raw []byte, dst any) ([]byte, error) {
	canonical, err := CanonicalJSON(raw)
	if err != nil {
		return nil, err
	}
	var tree any
	if err = json.Unmarshal(canonical, &tree); err != nil {
		return nil, err
	}
	if err = requiredFields(tree, reflect.TypeOf(dst).Elem()); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(canonical)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(dst); err != nil {
		return nil, err
	}
	return canonical, nil
}
func requiredFields(value any, typ reflect.Type) error {
	if typ.Kind() == reflect.Pointer {
		if value == nil {
			return nil
		}
		return requiredFields(value, typ.Elem())
	}
	if value == nil {
		return errors.New("null required field")
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return errors.New("expected object")
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		v, ok := obj[key]
		if !ok {
			return fmt.Errorf("missing %s", key)
		}
		if err := requiredFields(v, field.Type); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}
func DecodeRequest(raw []byte) (JobRequest, []byte, error) {
	var r JobRequest
	b, err := DecodeStrict(raw, &r)
	if err != nil {
		return r, nil, err
	}
	o := r.Options
	if r.ContractVersion != 1 || !ValidUUID(r.DeploymentID) || !ValidUUID(r.EpisodeID) || !ValidUUID(r.GenerationID) || r.Attempt < 1 || r.Attempt > 1000 || !ValidMediaPath(r.Input.RelativePath) || r.Input.SizeBytes < 1 || r.Input.SizeBytes > 9007199254740991 || !ValidSHA256(r.Input.SHA256) || !bounded(r.EngineProfile, 1, 128) || (r.ResumeFromJobID != nil && !ValidUUID(*r.ResumeFromJobID)) {
		return r, nil, errors.New("invalid request identity or input")
	}
	if !bounded(o.Model, 1, 128) || !bounded(o.Language, 1, 32) || !bounded(o.DiarizationModel, 1, 256) || o.ChunkSeconds < 1 || o.ChunkSeconds > 3600 || o.MinSpeakers < 1 || o.MaxSpeakers > 32 || o.MaxSpeakers < o.MinSpeakers || (o.VADMethod != "pyannote" && o.VADMethod != "silero") || o.ASROptions.BeamSize < 1 || o.ASROptions.BeamSize > 64 || !inRange(o.ASROptions.Patience, 0, 10) || !bounded(o.ASROptions.InitialPrompt, 0, 4096) || o.VADOptions.ChunkSize < 1 || o.VADOptions.ChunkSize > 3600 || !inRange(o.VADOptions.VADOnset, 0, 1) || !inRange(o.VADOptions.VADOffset, 0, 1) {
		return r, nil, errors.New("invalid transcription options")
	}
	return r, b, nil
}
func bounded(s string, min, max int) bool {
	n := utf8.RuneCountInString(s)
	return n >= min && n <= max
}
func inRange(n, lo, hi float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= lo && n <= hi
}

// ValidateTranscript accepts optional/extension fields but requires the provider
// payload and validates every supplied segment/word without inventing timing.
func ValidatePipelineTranscript(raw []byte) error {
	b, err := CanonicalJSON(raw)
	if err != nil {
		return err
	}
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(b, &obj); err != nil {
		return err
	}
	required := []string{"provider", "model", "language", "device", "compute_type", "batch_size", "asr_options", "vad_options", "vad_method", "aligned", "alignment", "diarization", "segments_pre_align", "segments", "metadata"}
	for _, key := range required {
		if v, ok := obj[key]; !ok || string(v) == "null" {
			return fmt.Errorf("missing transcript %s", key)
		}
	}
	var head struct {
		Provider    string `json:"provider"`
		Model       string `json:"model"`
		Language    string `json:"language"`
		Device      string `json:"device"`
		ComputeType string `json:"compute_type"`
		BatchSize   int    `json:"batch_size"`
		VADMethod   string `json:"vad_method"`
		Aligned     bool   `json:"aligned"`
		Alignment   struct {
			Requested bool   `json:"requested"`
			Used      bool   `json:"used"`
			Error     string `json:"error"`
		} `json:"alignment"`
		Diarization struct {
			Enabled     bool   `json:"enabled"`
			Used        bool   `json:"used"`
			Model       string `json:"model"`
			MinSpeakers int    `json:"min_speakers"`
			MaxSpeakers int    `json:"max_speakers"`
			Error       string `json:"error"`
		} `json:"diarization"`
	}
	if err = json.Unmarshal(b, &head); err != nil {
		return err
	}
	if head.Provider != "whisperx" || !bounded(head.Model, 1, 128) || !bounded(head.Language, 1, 32) || (head.Device != "cpu" && head.Device != "cuda") || head.ComputeType == "" || head.BatchSize < 1 || (head.VADMethod != "pyannote" && head.VADMethod != "silero") || head.Aligned != head.Alignment.Used || (!head.Alignment.Requested && head.Alignment.Used) || (!head.Diarization.Enabled && head.Diarization.Used) || !bounded(head.Alignment.Error, 0, 2048) || !bounded(head.Diarization.Error, 0, 2048) {
		return errors.New("invalid transcript provenance or outcomes")
	}
	for key, fields := range map[string][]string{"alignment": {"requested", "used", "error"}, "diarization": {"enabled", "used", "model", "min_speakers", "max_speakers", "error"}, "metadata": {"generated_at", "whisperx_version", "torch_version", "resumed_from_checkpoint", "checkpoint_segment_count"}} {
		var object map[string]json.RawMessage
		if err = json.Unmarshal(obj[key], &object); err != nil {
			return err
		}
		for _, field := range fields {
			if v, ok := object[field]; !ok || string(v) == "null" {
				return fmt.Errorf("missing %s.%s", key, field)
			}
		}
	}
	var asr ASROptions
	var vad VADOptions
	if _, err = DecodeStrict(obj["asr_options"], &asr); err != nil {
		return err
	}
	if _, err = DecodeStrict(obj["vad_options"], &vad); err != nil {
		return err
	}
	if asr.BeamSize < 1 || asr.BeamSize > 64 || !inRange(asr.Patience, 0, 10) || !bounded(asr.InitialPrompt, 0, 4096) || vad.ChunkSize < 1 || vad.ChunkSize > 3600 || !inRange(vad.VADOnset, 0, 1) || !inRange(vad.VADOffset, 0, 1) {
		return errors.New("invalid transcript options")
	}
	for _, key := range []string{"segments", "segments_pre_align"} {
		if err = ValidateSegments(obj[key]); err != nil {
			return err
		}
	}
	return nil
}

// ValidateTranscript additionally requires the worker's normalized provenance.
// The Python pipeline payload is validated separately before these fields exist.
func ValidateTranscript(raw []byte) error {
	if err := ValidatePipelineTranscript(raw); err != nil {
		return err
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(body["metadata"], &metadata); err != nil {
		return err
	}
	for _, key := range []string{"generated_at", "whisperx_version", "torch_version", "worker_version", "pipeline_version", "api_version", "engine_profile", "input_sha256", "options_sha256", "job_id", "package_versions", "resumed_from_checkpoint", "checkpoint_segment_count"} {
		if value, ok := metadata[key]; !ok || string(value) == "null" {
			return fmt.Errorf("missing metadata %s", key)
		}
	}
	for _, key := range []string{"generated_at", "whisperx_version", "torch_version", "worker_version", "pipeline_version", "api_version", "engine_profile", "input_sha256", "options_sha256", "job_id"} {
		var value string
		if err := json.Unmarshal(metadata[key], &value); err != nil || !bounded(value, 1, 256) {
			return fmt.Errorf("invalid metadata %s", key)
		}
		switch key {
		case "generated_at":
			if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
				return err
			}
		case "input_sha256", "options_sha256":
			if !ValidSHA256(value) {
				return errors.New("invalid metadata fingerprint")
			}
		case "job_id":
			if !ValidUUID(value) {
				return errors.New("invalid metadata job identity")
			}
		}
	}
	var resumed bool
	var count int
	var packages map[string]string
	if json.Unmarshal(metadata["resumed_from_checkpoint"], &resumed) != nil || json.Unmarshal(metadata["checkpoint_segment_count"], &count) != nil || count < 0 || json.Unmarshal(metadata["package_versions"], &packages) != nil || len(packages) == 0 {
		return errors.New("invalid metadata types")
	}
	for name, version := range packages {
		if !bounded(name, 1, 128) || !bounded(version, 1, 128) {
			return errors.New("invalid package provenance")
		}
	}
	return nil
}
func ValidateSegments(raw []byte) error {
	var segments []map[string]json.RawMessage
	if len(raw) == 0 || raw[0] != '[' {
		return errors.New("segments must be an array")
	}
	if err := json.Unmarshal(raw, &segments); err != nil {
		return err
	}
	if len(segments) > 100000 {
		return errors.New("too many segments")
	}
	for _, s := range segments {
		if err := validateText(s, "text", 100000, true); err != nil {
			return err
		}
		if err := validateTimes(s); err != nil {
			return err
		}
		if err := validateText(s, "speaker", 128, false); err != nil {
			return err
		}
		if rawWords, ok := s["words"]; ok {
			var words []map[string]json.RawMessage
			if len(rawWords) == 0 || rawWords[0] != '[' {
				return errors.New("words must be an array")
			}
			if err := json.Unmarshal(rawWords, &words); err != nil {
				return err
			}
			if len(words) > 100000 {
				return errors.New("too many words")
			}
			for _, w := range words {
				if err := validateText(w, "word", 10000, true); err != nil {
					return err
				}
				if err := validateTimes(w); err != nil {
					return err
				}
				if err := validateText(w, "speaker", 128, false); err != nil {
					return err
				}
				if score, ok := w["score"]; ok {
					var n float64
					if string(score) == "null" || json.Unmarshal(score, &n) != nil || !inRange(n, 0, 1) {
						return errors.New("invalid word score")
					}
				}
			}
		}
	}
	return nil
}
func validateText(obj map[string]json.RawMessage, key string, max int, required bool) error {
	b, ok := obj[key]
	if !ok && !required {
		return nil
	}
	var s string
	if !ok || string(b) == "null" || json.Unmarshal(b, &s) != nil || !bounded(s, 0, max) {
		return fmt.Errorf("invalid %s", key)
	}
	return nil
}
func validateTimes(obj map[string]json.RawMessage) error {
	var times [2]*float64
	for i, key := range []string{"start", "end"} {
		if b, ok := obj[key]; ok {
			var n float64
			if string(b) == "null" || json.Unmarshal(b, &n) != nil || !inRange(n, 0, math.MaxFloat64) {
				return errors.New("invalid timing")
			}
			times[i] = &n
		}
	}
	if times[0] != nil && times[1] != nil && *times[1] < *times[0] {
		return errors.New("end precedes start")
	}
	return nil
}
