// Package transcription contains pipeline contracts shared by the app and worker.
// It has no application database or orchestration dependencies.
package transcription

import "encoding/json"

// ScriptConfig is the existing WHISPERX_CONFIG_JSON payload. Deployment-owned
// fields are intentionally retained here; this is not the HTTP job options shape.
type ScriptConfig struct {
	Model        string                 `json:"model"`
	Language     string                 `json:"language"`
	Device       string                 `json:"device"`
	ComputeType  string                 `json:"compute_type"`
	BatchSize    int                    `json:"batch_size"`
	ChunkSeconds int                    `json:"chunk_seconds"`
	ASROptions   map[string]interface{} `json:"asr_options"`
	VADOptions   map[string]interface{} `json:"vad_options"`
	VADMethod    string                 `json:"vad_method"`
	Align        bool                   `json:"align"`
	Diarization  bool                   `json:"diarization"`
	DiarizeModel string                 `json:"diarization_model"`
	MinSpeakers  int                    `json:"min_speakers"`
	MaxSpeakers  int                    `json:"max_speakers"`
}

// ProgressUpdate is emitted by the Python pipeline; persistence belongs to its caller.
type ProgressUpdate struct {
	Stage            string          `json:"stage"`
	Percent          int             `json:"percent"`
	CompletedSeconds float64         `json:"completed_seconds,omitempty"`
	TotalSeconds     float64         `json:"total_seconds,omitempty"`
	Checkpoint       json.RawMessage `json:"checkpoint,omitempty"`
	Error            string          `json:"error,omitempty"`
}

// LegacyCheckpoint describes the local checkpoint format. SegmentsFile is a local
// path and must never be used as a worker API media reference.
type LegacyCheckpoint struct {
	CompletedSeconds float64 `json:"completed_seconds,omitempty"`
	TotalSeconds     float64 `json:"total_seconds,omitempty"`
	Language         string  `json:"language,omitempty"`
	SegmentsFile     string  `json:"segments_file,omitempty"`
}
