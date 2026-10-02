package service

import (
	"context"

	"github.com/ctaylor1/briefcast/internal/transcription"
)

// transcriptionBackend owns execution and its prerequisites. The scheduler owns
// the application queue, progress persistence, retries and downstream work.
// Methods may be called concurrently for distinct episodes. Progress callbacks
// must finish before Transcribe returns and must be serialized per invocation.
type transcriptionBackend interface {
	Preflight(context.Context) error
	Transcribe(ctx context.Context, audioPath, resumeCheckpoint string, onProgress func(transcription.ProgressUpdate)) ([]byte, error)
}

// These aliases preserve existing local callers while the pipeline contract is
// shared with the worker without importing application service or DB packages.
type whisperxScriptConfig = transcription.ScriptConfig
type whisperxProgressUpdate = transcription.ProgressUpdate
type whisperxResumeCheckpoint = transcription.LegacyCheckpoint
