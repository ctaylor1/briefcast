package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"gorm.io/gorm"
)

func TestCharacterize_WhisperXRunnerForwardsConfigAndLegacyResume(t *testing.T) {
	python := requireWorkingPython(t)
	for _, resume := range []string{"", "not-json", `{"completed_seconds":120,"language":"en","segments":[{"text":"Earlier."}]}`} {
		t.Run(fmt.Sprintf("resume-%d", len(resume)), func(t *testing.T) {
			dir := t.TempDir()
			script := writeTempScript(t, dir, "capture.py", `import json, os
resume_path = os.environ.get("WHISPERX_RESUME_FILE", "")
print(json.dumps({
    "config": json.loads(os.environ["WHISPERX_CONFIG_JSON"]),
    "resume": open(resume_path, encoding="utf-8").read() if resume_path else "",
    "segments_path": os.environ["WHISPERX_SEGMENTS_FILE"],
    "log_output": os.environ["LOG_OUTPUT"],
    "hf_home": os.environ["HF_HOME"],
    "has_token": os.environ.get("WHISPERX_HF_TOKEN") == "synthetic-test-token",
    "telemetry": [os.environ.get(k) for k in ["HF_HUB_DISABLE_TELEMETRY", "DO_NOT_TRACK", "PYANNOTE_METRICS_ENABLED"]]
}))
`)
			cfg := WhisperXConfig{
				Python: python, Script: script, HFHome: filepath.Join(dir, "cache"),
				HFToken: "synthetic-test-token", DisableTelemetry: true, Model: "tiny.en", Language: "en",
				Device: "cpu", ComputeType: "int8", BatchSize: 3, ChunkSeconds: 90,
				BeamSize: 7, Patience: 2, ConditionOnPrev: false, InitialPrompt: "Test prompt.",
				VADChunkSize: 30, VADOnset: 0.6, VADOffset: 0.4, VADMethod: "silero",
				Align: false, Diarization: true, DiarizationModel: "test-diarization", MinSpeakers: 1, MaxSpeakers: 3,
			}
			t.Setenv("LOG_OUTPUT", "stdout")
			audio := filepath.Join(dir, "audio.mp3")
			output, err := RunWhisperXWithProgress(audio, cfg, resume, nil)
			if err != nil {
				t.Fatalf("capture runner configuration: %v", err)
			}
			var got struct {
				Config       whisperxScriptConfig `json:"config"`
				Resume       string               `json:"resume"`
				SegmentsPath string               `json:"segments_path"`
				LogOutput    string               `json:"log_output"`
				HFHome       string               `json:"hf_home"`
				HasToken     bool                 `json:"has_token"`
				Telemetry    []string             `json:"telemetry"`
			}
			if err := json.Unmarshal(output, &got); err != nil {
				t.Fatal(err)
			}
			want := whisperxScriptConfig{
				Model: "tiny.en", Language: "en", Device: "cpu", ComputeType: "int8", BatchSize: 3, ChunkSeconds: 90,
				ASROptions: map[string]interface{}{"beam_size": float64(7), "patience": float64(2), "condition_on_previous_text": false, "initial_prompt": "Test prompt."},
				VADOptions: map[string]interface{}{"chunk_size": float64(30), "vad_onset": 0.6, "vad_offset": 0.4},
				VADMethod:  "silero", Align: false, Diarization: true, DiarizeModel: "test-diarization", MinSpeakers: 1, MaxSpeakers: 3,
			}
			if !reflect.DeepEqual(got.Config, want) {
				t.Fatalf("script config changed: got %+v want %+v", got.Config, want)
			}
			wantResume := resume
			if !json.Valid([]byte(resume)) {
				wantResume = ""
			}
			if got.Resume != wantResume || got.SegmentsPath != audio+".briefcast.whisperx.resume.json" {
				t.Fatalf("unexpected resume forwarding: resume=%q segments=%q", got.Resume, got.SegmentsPath)
			}
			if got.LogOutput != "stderr" || got.HFHome != cfg.HFHome || !got.HasToken || !reflect.DeepEqual(got.Telemetry, []string{"1", "1", "0"}) {
				t.Fatalf("unexpected child environment: %+v", got)
			}
		})
	}
}

func TestCharacterize_WhisperXRunnerRetainsCheckpointOnErrors(t *testing.T) {
	python := requireWorkingPython(t)
	for _, tc := range []struct {
		name, body, errorText string
	}{
		{"exit", "print('synthetic failure', file=sys.stderr)\nsys.exit(2)", "synthetic failure"},
		{"invalid-output", "print('not JSON')", "not valid JSON"},
		{"timeout", "time.sleep(5)", "timed out"},
		{"success", "print(json.dumps({'segments': []}))", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			audio := filepath.Join(dir, "episode.mp3")
			sidecar := audio + ".briefcast.whisperx.resume.json"
			if tc.name == "timeout" {
				t.Setenv(whisperxTimeoutEnv, "1")
			}
			script := writeTempScript(t, dir, "outcome.py", `import json, os, sys, time
with open(os.environ["WHISPERX_SEGMENTS_FILE"], "w", encoding="utf-8") as f:
    json.dump({"segments":[{"text":"Earlier."}], "completed_seconds":120}, f)
with open(os.environ["WHISPERX_PROGRESS_FILE"], "w", encoding="utf-8") as f:
    json.dump({"stage":"transcribing", "percent":60, "checkpoint":{"completed_seconds":120}}, f)
`+tc.body)
			var updates []whisperxProgressUpdate
			_, err := RunWhisperXWithProgress(audio, WhisperXConfig{Python: python, Script: script}, "", func(update whisperxProgressUpdate) {
				updates = append(updates, update)
			})
			if tc.errorText == "" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
					t.Fatalf("successful runner did not remove sidecar: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.errorText) {
					t.Fatalf("expected %q, got %v", tc.errorText, err)
				}
				if raw, err := os.ReadFile(sidecar); err != nil || !strings.Contains(string(raw), "Earlier.") {
					t.Fatalf("checkpoint lost on failure: %q %v", raw, err)
				}
			}
			if len(updates) == 0 || updates[len(updates)-1].Percent != 60 {
				t.Fatalf("final progress was not delivered: %+v", updates)
			}
		})
	}
	// Launch failure must preserve an already existing checkpoint as well.
	dir := t.TempDir()
	audio := filepath.Join(dir, "episode.mp3")
	sidecar := audio + ".briefcast.whisperx.resume.json"
	if err := os.WriteFile(sidecar, []byte("checkpoint"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := writeTempScript(t, dir, "unused.py", "print('{}')")
	_, err := RunWhisperX(audio, WhisperXConfig{Python: filepath.Join(dir, "missing-python"), Script: script})
	if err == nil || !strings.Contains(err.Error(), "failed to start") {
		t.Fatalf("expected child launch failure, got %v", err)
	}
	if raw, err := os.ReadFile(sidecar); err != nil || string(raw) != "checkpoint" {
		t.Fatalf("launch failure lost checkpoint: %q %v", raw, err)
	}
}

func TestCharacterize_TranscriptResumePersistsExportsThenSummarizes(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, dir string) {
		item.DownloadPath = filepath.Join(dir, "resume.mp3")
		if err := os.WriteFile(item.DownloadPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
			t.Fatal(err)
		}
		item.TranscriptJSON, item.CanonicalTranscript, item.TranscriptCheckpointJSON = "", "", ""
		item.TranscriptStatus, item.TranscriptProgressStage = "processing", "transcribing"
		item.TranscriptProgressPct, item.TranscriptRetryCount = 0, 0
		item.TranscriptNextAttempt = nil
		item.LLMSummary, item.LLMSummaryStatus = "", "pending"
	})
	configureCharacterizationWhisperX(t, `import json, os, sys
audio = sys.argv[1]
counter = audio + ".runs"
runs = int(open(counter).read()) + 1 if os.path.exists(counter) else 1
with open(counter, "w") as f:
    f.write(str(runs))
sidecar = os.environ["WHISPERX_SEGMENTS_FILE"]
if runs == 1:
    with open(sidecar, "w", encoding="utf-8") as f:
        json.dump({"segments":[{"start":0,"end":1,"text":"Earlier sentence."}], "completed_seconds":120}, f)
    with open(os.environ["WHISPERX_PROGRESS_FILE"], "w", encoding="utf-8") as f:
        json.dump({"stage":"transcribing", "percent":65, "checkpoint":{"completed_seconds":120,"segments_file":sidecar}}, f)
    print("synthetic interrupted inference", file=sys.stderr)
    sys.exit(2)
with open(os.environ["WHISPERX_RESUME_FILE"], encoding="utf-8") as f:
    resume = json.load(f)
assert resume["completed_seconds"] == 120
with open(sidecar, encoding="utf-8") as f:
    segments = json.load(f)["segments"]
segments.append({"start":120,"end":121,"text":"New sentence."})
print(json.dumps({"provider":"whisperx", "segments_pre_align":segments,
    "segments":[{"text":"Post alignment text.","words":[{"word":"untimed"}]}],
    "alignment":{"requested":True,"used":False,"error":"align_failed:ValueError"}}))
`)
	started := time.Now().UTC()
	if err := TranscribePendingEpisodes(); err == nil {
		t.Fatal("expected first attempt to fail")
	}
	failed := loadCharacterizationPodcastItem(t, item.ID)
	if failed.TranscriptStatus != "failed" || failed.TranscriptProgressPct != 65 || failed.TranscriptRetryCount != 1 || failed.TranscriptCheckpointJSON == "" {
		t.Fatalf("failure progress/resume state was lost: %+v", failed)
	}
	if failed.TranscriptNextAttempt == nil || failed.TranscriptNextAttempt.Before(started.Add(59*time.Second)) {
		t.Fatalf("retry was not delayed: %v", failed.TranscriptNextAttempt)
	}
	if !FileExists(item.DownloadPath) || !FileExists(item.DownloadPath+".briefcast.whisperx.resume.json") {
		t.Fatal("failed inference lost audio or checkpoint")
	}
	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatalf("future retry should not execute: %v", err)
	}
	if raw, err := os.ReadFile(item.DownloadPath + ".runs"); err != nil || string(raw) != "1" {
		t.Fatalf("retry ran before it was due: %q %v", raw, err)
	}

	const canonical = "Earlier sentence.\n\nNew sentence."
	observed := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var current db.PodcastItem
		err := db.GetPodcastItemByID(item.ID, &current)
		if err == nil && (current.TranscriptStatus != "available" || current.CanonicalTranscript != canonical || current.TranscriptCheckpointJSON != "") {
			err = fmt.Errorf("summary ran before transcript commit: status=%s canonical=%q", current.TranscriptStatus, current.CanonicalTranscript)
		}
		for _, variant := range transcriptExportVariants {
			path := filepath.Join(resolveExportDir(), variant.dir, sanitizeAssetName(podcastTitle(&current)), pubDateSubdir(current.PubDate), exportEpisodeFileName(&current)+variant.extension)
			if raw, readErr := os.ReadFile(path); err == nil && (readErr != nil || string(raw) != canonical) {
				err = fmt.Errorf("summary ran before %s export: content=%q error=%v", variant.extension, raw, readErr)
			}
		}
		var request struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&request); err == nil && decodeErr != nil {
			err = decodeErr
		}
		containsCanonical := false
		for _, message := range request.Messages {
			containsCanonical = containsCanonical || strings.Contains(message.Content, canonical)
		}
		if err == nil && !containsCanonical {
			err = fmt.Errorf("summary did not consume the canonical transcript")
		}
		observed <- err
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"test-summary-model","choices":[{"message":{"content":"Summary after resume."}}]}`))
	}))
	defer server.Close()
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "synthetic-test-key")
	t.Setenv("LLM_BASE_URL", server.URL)
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	failed.TranscriptNextAttempt = &past
	if err := db.UpdatePodcastItem(&failed); err != nil {
		t.Fatal(err)
	}
	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatalf("resume attempt failed: %v", err)
	}
	select {
	case err := <-observed:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("summary was not invoked")
	}
	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.CanonicalTranscript != canonical || got.TranscriptRetryCount != 0 || got.TranscriptNextAttempt != nil || got.TranscriptLastError != "" || got.TranscriptModel != "characterization-whisper-model" {
		t.Fatalf("resume success did not clear retry state or preserve canonical text: %+v", got)
	}
	if got.LLMSummary != "Summary after resume." || got.LLMSummaryModel != "test-summary-model" {
		t.Fatalf("summary output/lineage missing: %q %q", got.LLMSummary, got.LLMSummaryModel)
	}
	if !strings.Contains(got.TranscriptJSON, "align_failed:ValueError") || !strings.Contains(got.TranscriptJSON, "untimed") {
		t.Fatal("raw fallback metadata or optional word payload was lost")
	}
	if FileExists(item.DownloadPath + ".briefcast.whisperx.resume.json") {
		t.Fatal("successful local attempt left its sidecar")
	}
	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(item.DownloadPath + ".runs"); err != nil || string(raw) != "2" {
		t.Fatalf("completed transcript was transcribed again: %q %v", raw, err)
	}
}

func TestCharacterize_TranscriptLocalErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		redownload    bool
	}{
		{"inference", "synthetic inference failure", false},
		{"audio", "Failed to load audio", true},
		// Known defect: local string matching also treats a missing model path
		// as corrupt audio. HTTP mode must use typed errors and preserve media.
		{"missing-model-path", "No such file or directory: model.cache", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, dir string) {
				item.DownloadPath = filepath.Join(dir, "episode.mp3")
				if err := os.WriteFile(item.DownloadPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
					t.Fatal(err)
				}
				item.TranscriptJSON, item.TranscriptCheckpointJSON = "", ""
				item.TranscriptStatus = "pending_whisperx"
				item.TranscriptNextAttempt = nil
				item.TranscriptRetryCount = 0
			})
			configureCharacterizationWhisperX(t, "import sys\nprint("+fmt.Sprintf("%q", tc.message)+", file=sys.stderr)\nsys.exit(2)\n")
			if err := TranscribePendingEpisodes(); err == nil {
				t.Fatal("expected subprocess failure")
			}
			got := loadCharacterizationPodcastItem(t, item.ID)
			if tc.redownload {
				if FileExists(item.DownloadPath) || got.DownloadPath != "" || got.DownloadStatus != db.NotDownloaded || got.TranscriptStatus != "pending_whisperx" || got.TranscriptProgressStage != "waiting_for_download" {
					t.Fatalf("local audio-error redownload behavior changed: %+v", got)
				}
			} else if !FileExists(item.DownloadPath) || got.DownloadStatus != db.Downloaded || got.TranscriptStatus != "failed" || got.TranscriptRetryCount != 1 {
				t.Fatalf("generic error must retain media and schedule a retry: %+v", got)
			}
		})
	}
}

func TestCharacterize_TranscriptDownstreamFailuresDoNotRerunInference(t *testing.T) {
	dir, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, dir string) {
		item.DownloadPath = filepath.Join(dir, "episode.mp3")
		if err := os.WriteFile(item.DownloadPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
			t.Fatal(err)
		}
		item.TranscriptJSON, item.CanonicalTranscript, item.TranscriptCheckpointJSON = "", "", ""
		item.TranscriptStatus = "pending_whisperx"
		item.TranscriptNextAttempt = nil
		item.LLMSummary, item.LLMSummaryStatus = "", "pending"
	})
	configureCharacterizationWhisperX(t, `import json, sys
with open(sys.argv[1] + ".runs", "a") as f:
    f.write("run\n")
print(json.dumps({"segments":[{"text":"Committed transcript."}]}))
`)
	// A regular file at DATA makes export directory creation fail on all hosts.
	blockedAssets := filepath.Join(dir, "blocked-assets")
	if err := os.WriteFile(blockedAssets, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATA", blockedAssets)
	requests := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "synthetic-test-key")
	t.Setenv("LLM_BASE_URL", server.URL)
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatal(err)
	}
	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatalf("local orchestration surfaces inference errors only: %v", err)
	}
	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.TranscriptStatus != "available" || got.CanonicalTranscript != "Committed transcript." || got.LLMSummaryStatus != "failed" || got.LLMSummaryError == "" {
		t.Fatalf("downstream failure changed transcript success or lost summary error: %+v", got)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one summary invocation, got %d", len(requests))
	}
	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(item.DownloadPath + ".runs"); err != nil || strings.TrimSpace(string(raw)) != "run" {
		t.Fatalf("downstream failure repeated inference: %q %v", raw, err)
	}
	if len(requests) != 1 {
		t.Fatal("transcription scheduler repeated failed summarization")
	}
}

func TestCharacterize_TranscriptSaveFailureStillRunsDownstream(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, dir string) {
		item.DownloadPath = filepath.Join(dir, "episode.mp3")
		if err := os.WriteFile(item.DownloadPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
			t.Fatal(err)
		}
		item.TranscriptJSON, item.CanonicalTranscript, item.TranscriptCheckpointJSON = "", "", ""
		item.TranscriptStatus, item.LLMSummaryStatus = "pending_whisperx", "pending"
		item.TranscriptNextAttempt = nil
		item.LLMSummary = ""
	})
	configureCharacterizationWhisperX(t, `import json, os
with open(os.environ["WHISPERX_SEGMENTS_FILE"], "w") as f:
    f.write("checkpoint")
print(json.dumps({"segments":[{"text":"Uncommitted transcript."}]}))
`)
	const callback = "characterize_reject_available_transcript"
	if err := db.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "podcast_items" {
			return
		}
		field := tx.Statement.Schema.LookUpField("TranscriptStatus")
		if field != nil {
			value, _ := field.ValueOf(tx.Statement.Context, tx.Statement.ReflectValue)
			if value == "available" {
				tx.AddError(errors.New("injected transcript write failure"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DB.Callback().Update().Remove(callback) })
	observed := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var current db.PodcastItem
		err := db.GetPodcastItemByID(item.ID, &current)
		if err == nil && (current.TranscriptJSON != "" || current.TranscriptStatus != "processing") {
			err = fmt.Errorf("injected save failure did not leave transcript uncommitted")
		}
		observed <- err
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Uncommitted summary."}}]}`))
	}))
	defer server.Close()
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "synthetic-test-key")
	t.Setenv("LLM_BASE_URL", server.URL)
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatal(err)
	}
	if err := TranscribePendingEpisodes(); err == nil || !strings.Contains(err.Error(), "injected transcript write failure") {
		t.Fatalf("expected injected DB error, got %v", err)
	}
	// Known defects: cleanup and exports precede a durable transcript commit,
	// and a failed commit does not prevent an external summary request.
	select {
	case err := <-observed:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("local save-failure downstream behavior changed")
	}
	for _, variant := range transcriptExportVariants {
		path := filepath.Join(resolveExportDir(), variant.dir, sanitizeAssetName(podcastTitle(&item)), pubDateSubdir(item.PubDate), exportEpisodeFileName(&item)+variant.extension)
		if raw, err := os.ReadFile(path); err != nil || string(raw) != "Uncommitted transcript." {
			t.Fatalf("local save-failure export behavior changed: %q %v", raw, err)
		}
	}
	if FileExists(item.DownloadPath + ".briefcast.whisperx.resume.json") {
		t.Fatal("local successful child cleanup behavior changed")
	}
	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.TranscriptJSON != "" || got.LLMSummary != "" {
		t.Fatal("injected DB failure unexpectedly persisted output")
	}
}
