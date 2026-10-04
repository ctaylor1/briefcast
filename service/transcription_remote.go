package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/ctaylor1/briefcast/internal/transcription"
)

func transcribeRemotePending(parent context.Context, cfg WhisperXConfig, b *httpTranscriptionBackend, lock *db.JobLock) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	l := &remoteLease{lock: *lock, ctx: ctx, cancel: cancel}
	defer l.release()
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Duration(max(1, cfg.LockRefreshSecs)) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := l.renew(); err != nil {
					logging.Sugar().Warnw("remote_transcription_lease_lost")
					return
				}
			}
		}
	}()
	defer func() { close(stop); <-done }()
	defer b.client.CloseIdleConnections()
	if err := b.Preflight(ctx); err != nil {
		return err
	}
	firstErr := reconcileRemoteReceipts(ctx, b, l)
	items, err := remoteQueueItems(cfg, time.Now().UTC())
	if err != nil {
		return errors.Join(firstErr, err)
	}
	var mu sync.Mutex
	runWorkerPool(items, boundedWorkerCount(cfg.MaxConcurrency, 1, len(items)), func(item db.PodcastItem) {
		if err := ctx.Err(); err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
			return
		}
		err := transcribeRemoteItem(ctx, b, l, item)
		if err != nil {
			logging.Sugar().Warnw("remote_transcription_item_failed", "podcast_item_id", item.ID, "error", err)
			mu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
		}
	})
	return firstErr
}

// Filter permanent failures before applying the run limit. Keyset pagination
// prevents a blocked first page from starving later due episodes.
func remoteQueueItems(cfg WhisperXConfig, now time.Time) ([]db.PodcastItem, error) {
	statuses := []string{"pending_whisperx", "processing"}
	if cfg.RetryFailed {
		statuses = append(statuses, "failed")
	}
	var selected []db.PodcastItem
	after := ""
	priority := -1
	var due, downloaded time.Time
	const prioritySQL = "CASE transcript_status WHEN 'processing' THEN 0 WHEN 'pending_whisperx' THEN 1 WHEN 'failed' THEN 2 ELSE 3 END"
	const dueSQL = "COALESCE(transcript_next_attempt, download_date)"
	for {
		var page []db.PodcastItem
		q := db.DB.Where("download_status = ? AND download_path <> '' AND COALESCE(transcript_json, '') = '' AND deleted_at IS NULL", db.Downloaded).Where("transcript_status IN ?", statuses).Where("transcript_next_attempt IS NULL OR transcript_next_attempt <= ?", now)
		if after != "" {
			q = q.Where("("+prioritySQL+") > ? OR (("+prioritySQL+") = ? AND ("+dueSQL+" > ? OR ("+dueSQL+" = ? AND (download_date > ? OR (download_date = ? AND id > ?)))))", priority, priority, due, due, downloaded, downloaded, after)
		}
		err := q.Order(prioritySQL + " ASC").Order(dueSQL + " ASC").Order("download_date ASC").Order("id ASC").Limit(100).Find(&page).Error
		if err != nil {
			return nil, err
		}
		for _, item := range page {
			after = item.ID
			priority = 2
			if item.TranscriptStatus == "processing" {
				priority = 0
			} else if item.TranscriptStatus == "pending_whisperx" {
				priority = 1
			}
			downloaded, due = item.DownloadDate, item.DownloadDate
			if item.TranscriptNextAttempt != nil {
				due = *item.TranscriptNextAttempt
			}
			manual := item.TranscriptStatus == "pending_whisperx" && item.TranscriptProgressStage == "queued_for_retry"
			if item.TranscriptStatus == "failed" && item.TranscriptProgressStage == "blocked" && !manual {
				continue
			}
			if e, err := decodeRemoteEnvelope(item.TranscriptCheckpointJSON); err == nil && e.Permanent && !manual {
				continue
			}
			selected = append(selected, item)
			if cfg.MaxItemsPerRun > 0 && len(selected) >= cfg.MaxItemsPerRun {
				return selected, nil
			}
		}
		if len(page) < 100 {
			return selected, nil
		}
	}
}

func transcribeRemoteItem(ctx context.Context, b *httpTranscriptionBackend, l *remoteLease, item db.PodcastItem) error {
	if err := l.check(); err != nil {
		return err
	}
	e, err := b.prepare(ctx, item)
	if err != nil {
		return failRemoteItem(b, l, &item, nil, err)
	}
	r, _, _ := transcription.DecodeRequest([]byte(e.RequestJSON))
	input, err := fingerprintRemoteMedia(ctx, getEnvString("DATA", "/assets"), item.DownloadPath)
	if err != nil {
		return failRemoteItem(b, l, &item, e, err)
	}
	if input != r.Input {
		return failRemoteItem(b, l, &item, e, remoteProblem("INPUT_CHANGED", "Local audio differs from persisted submission", false))
	}
	e.Permanent = false
	raw, _ := json.Marshal(e)
	stage := "starting"
	pct := max(1, item.TranscriptProgressPct)
	if e.LastRevision > 0 {
		stage = "reconnecting"
	} else {
		pct = 1
	}
	if e.RestartReason != "" {
		stage = "restarting_from_zero"
		pct = 1
	}
	updates := map[string]any{"transcript_checkpoint_json": string(raw), "transcript_status": "processing", "transcript_progress_stage": stage, "transcript_progress_pct": pct, "transcript_next_attempt": nil, "transcript_last_error": ""}
	// This commit is mandatory before any GET/PUT for this job identity.
	if err = l.update(&item, updates, false); err != nil {
		return err
	}
	item.TranscriptCheckpointJSON = string(raw)
	item.TranscriptStatus = "processing"
	item.TranscriptProgressStage = stage
	item.TranscriptProgressPct = pct
	check := func() error {
		if err = l.check(); err != nil {
			return err
		}
		var count int64
		err := db.DB.Model(&db.PodcastItem{}).Where("id = ? AND deleted_at IS NULL AND download_path = ? AND file_url = ? AND guid = ? AND download_status = ? AND transcript_status = ? AND COALESCE(transcript_json, '') = '' AND transcript_checkpoint_json = ?", item.ID, item.DownloadPath, item.FileURL, item.GUID, db.Downloaded, item.TranscriptStatus, item.TranscriptCheckpointJSON).Count(&count).Error
		if err != nil {
			return err
		}
		if count != 1 {
			return errRemoteOwnership
		}
		return ctx.Err()
	}
	result, err := b.execute(ctx, e, func(e *remoteEnvelope, s transcription.JobStatus) error {
		raw, _ := json.Marshal(e)
		pct := max(item.TranscriptProgressPct, min(s.Percent, 99))
		stage := s.Stage
		if s.State == "succeeded" {
			stage = "saving_result"
		}
		updates := map[string]any{"transcript_checkpoint_json": string(raw), "transcript_progress_stage": stage, "transcript_progress_pct": pct}
		if err := l.update(&item, updates, false); err != nil {
			return err
		}
		item.TranscriptCheckpointJSON = string(raw)
		item.TranscriptProgressPct = pct
		item.TranscriptProgressStage = stage
		return nil
	}, check)
	if err != nil {
		return failRemoteItem(b, l, &item, e, err)
	}
	if err = check(); err != nil {
		return err
	}
	input, err = fingerprintRemoteMedia(ctx, getEnvString("DATA", "/assets"), item.DownloadPath)
	if err != nil {
		return failRemoteItem(b, l, &item, e, err)
	}
	if input != r.Input {
		return failRemoteItem(b, l, &item, e, remoteProblem("INPUT_CHANGED", "Local audio changed before result commit", false))
	}
	e.ReceiptResultSHA256 = &result.ResultSHA256
	raw, _ = json.Marshal(e)
	now := time.Now().UTC()
	text := string(result.Transcript)
	updates = map[string]any{"transcript_json": text, "canonical_transcript": buildCanonicalTranscriptFromTranscriptJSON(text), "canonical_transcript_version": canonicalTranscriptVersionCurrent, "canonical_updated_at": &now, "transcript_status": "available", "transcript_retry_count": 0, "transcript_next_attempt": nil, "transcript_last_error": "", "transcript_progress_pct": 100, "transcript_progress_stage": "complete", "transcript_checkpoint_json": string(raw), "transcript_model": r.Options.Model}
	if err = l.update(&item, updates, false); err != nil {
		return err
	}
	// Reload after the column-only commit before downstream work. Avoid carrying
	// an old snapshot's unrelated library fields into exports or summaries.
	if err = db.GetPodcastItemByID(item.ID, &item); err != nil {
		return err
	}
	if item.TranscriptCheckpointJSON != string(raw) || item.TranscriptJSON != text || item.TranscriptStatus != "available" {
		return errRemoteOwnership
	}
	ackErr := acknowledgeRemoteItem(ctx, b, l, &item, e)
	if err = l.check(); err != nil {
		return errors.Join(ackErr, err)
	}
	ExportTranscript(&item)
	if err = summarizeRemoteCommittedItem(l, &item); err != nil {
		logging.Sugar().Warnw("remote_transcription_summary_failed", "podcast_item_id", item.ID, "error", err)
	}
	return ackErr
}

func failRemoteItem(b *httpTranscriptionBackend, l *remoteLease, item *db.PodcastItem, e *remoteEnvelope, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, errRemoteOwnership) || l.ctx.Err() != nil {
		return err
	}
	var problem *remoteError
	if !errors.As(err, &problem) {
		return err
	} // DB errors preserve identity/state.
	status, stage := "processing", "reconnecting"
	reconnectDelay := b.poll
	if e != nil {
		e.ReconnectFailures++
		reconnectDelay = whisperxRetryDelay(e.ReconnectFailures, max(1, int(b.poll/time.Second)), 15)
	}
	next := time.Now().UTC().Add(min(max(reconnectDelay, problem.retryAfter), 15*time.Second))
	var nextPtr *time.Time = &next
	retries := item.TranscriptRetryCount
	if problem.terminal {
		status, stage = "failed", "failed"
		retries++
		if !b.cfg.RetryFailed {
			nextPtr = nil
		} else {
			next = time.Now().UTC().Add(whisperxRetryDelay(retries, b.cfg.RetryDelaySeconds, b.cfg.RetryMaxDelay))
		}
	}
	if !problem.problem.Retryable {
		status, stage = "failed", "blocked"
		nextPtr = nil
	}
	if e != nil {
		e.Permanent = !problem.problem.Retryable || (problem.terminal && !b.cfg.RetryFailed)
	}
	updates := map[string]any{"transcript_status": status, "transcript_progress_stage": stage, "transcript_retry_count": retries, "transcript_next_attempt": nextPtr, "transcript_last_error": trimToLength(problem.Error(), b.cfg.RetryMaxErrorChars)}
	if e != nil {
		raw, _ := json.Marshal(e)
		updates["transcript_checkpoint_json"] = string(raw)
	}
	// Only locally verified absence rearms downloading. A remote INPUT_MISSING,
	// mount problem or model error never authorizes removal/reset of valid audio.
	if problem.problem.Code == "INPUT_MISSING" && problem.status == 0 && !problem.terminal {
		if _, statErr := fingerprintRemoteMedia(l.ctx, getEnvString("DATA", "/assets"), item.DownloadPath); statErr != nil {
			var local *remoteError
			if errors.As(statErr, &local) && local.problem.Code == "INPUT_MISSING" {
				updates["download_status"] = db.NotDownloaded
				updates["download_path"] = ""
				updates["downloaded_bytes"] = 0
				updates["download_total_bytes"] = 0
				updates["transcript_status"] = "pending_whisperx"
				updates["transcript_progress_stage"] = "waiting_for_download"
				updates["transcript_checkpoint_json"] = ""
				updates["transcript_progress_pct"] = 0
			}
		}
	}
	if updateErr := l.update(item, updates, false); updateErr != nil {
		return errors.Join(err, updateErr)
	}
	return err
}

func validateRemoteReceipt(l *remoteLease, item *db.PodcastItem, e *remoteEnvelope) error {
	if err := l.check(); err != nil {
		return err
	}
	canonical, err := transcription.CanonicalJSON([]byte(item.TranscriptJSON))
	if err != nil || e.ReceiptResultSHA256 == nil || transcription.Digest(canonical) != *e.ReceiptResultSHA256 || transcription.ValidateTranscript(canonical) != nil {
		return remoteProblem("INVALID_CONFIG", "Committed transcript differs from receipt", false)
	}
	// Fence each use of the receipt against generation or committed-data changes.
	var count int64
	if err = db.DB.Model(&db.PodcastItem{}).Where("id = ? AND deleted_at IS NULL AND transcript_status = 'available' AND transcript_json = ? AND transcript_checkpoint_json = ?", item.ID, item.TranscriptJSON, item.TranscriptCheckpointJSON).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return errRemoteOwnership
	}
	return nil
}

func acknowledgeRemoteItem(ctx context.Context, b *httpTranscriptionBackend, l *remoteLease, item *db.PodcastItem, e *remoteEnvelope) error {
	if err := validateRemoteReceipt(l, item, e); err != nil {
		return err
	}
	if err := b.acknowledge(ctx, e); err != nil {
		next := time.Now().UTC().Add(whisperxRetryDelay(1, b.cfg.RetryDelaySeconds, b.cfg.RetryMaxDelay))
		if saveErr := l.update(item, map[string]any{"transcript_next_attempt": &next}, true); saveErr != nil {
			return errors.Join(err, saveErr)
		}
		return err
	}
	if err := l.update(item, map[string]any{"transcript_checkpoint_json": "", "transcript_next_attempt": nil}, true); err != nil {
		return err
	}
	item.TranscriptCheckpointJSON = ""
	item.TranscriptNextAttempt = nil
	return nil
}

func reconcileRemoteReceipts(ctx context.Context, b *httpTranscriptionBackend, l *remoteLease) error {
	var items []db.PodcastItem
	// Completed HTTP envelopes are written by json.Marshal(remoteEnvelope).
	// Filter their fixed receipt marker before the batch limit so completed
	// legacy checkpoints cannot consume every reconciliation slot. The decoded
	// identity/digest are still validated before any ACK or checkpoint clearing.
	if err := db.DB.Where("transcript_status = 'available' AND transcript_json <> '' AND deleted_at IS NULL").Where("transcript_checkpoint_json LIKE ? AND transcript_checkpoint_json LIKE ?", `%"backend":"http"%`, `%"receipt_result_sha256":"%`).Where("transcript_next_attempt IS NULL OR transcript_next_attempt <= ?", time.Now().UTC()).Order("COALESCE(transcript_next_attempt, updated_at) ASC").Order("id ASC").Limit(100).Find(&items).Error; err != nil {
		return err
	}
	var first error
	for _, item := range items {
		if err := l.check(); err != nil {
			return errors.Join(first, err)
		}
		e, err := decodeRemoteEnvelope(item.TranscriptCheckpointJSON)
		if err != nil {
			// Keep the envelope for inspection; defer this row below without ACK.
		} else if e.DeploymentID != b.deployment || e.EpisodeID != item.ID {
			err = remoteProblem("INVALID_CONFIG", "Receipt identity mismatch", false)
		} else if e.ReceiptResultSHA256 != nil {
			// A crash after the transcript commit may precede all downstream
			// work. Validate the durable receipt before exporting or summarizing,
			// and retain it until that work has been attempted on recovery.
			err = validateRemoteReceipt(l, &item, e)
			if err == nil {
				ExportTranscript(&item)
				// Preserve a completed or manually edited summary across receipts.
				if strings.TrimSpace(item.LLMSummary) == "" {
					if summaryErr := summarizeRemoteCommittedItem(l, &item); summaryErr != nil {
						logging.Sugar().Warnw("remote_transcription_summary_failed", "podcast_item_id", item.ID, "error", summaryErr)
					}
				}
				err = acknowledgeRemoteItem(ctx, b, l, &item, e)
			}
		} else {
			continue
		}
		if err != nil && first == nil {
			first = err
		}
		if err != nil && !errors.Is(err, errRemoteOwnership) && ctx.Err() == nil {
			// Defer corrupt/identity-mismatched receipts too; preserve their data
			// for inspection while permitting later due receipts to advance.
			next := time.Now().UTC().Add(whisperxRetryDelay(1, b.cfg.RetryDelaySeconds, b.cfg.RetryMaxDelay))
			if saveErr := l.update(&item, map[string]any{"transcript_next_attempt": &next}, true); saveErr != nil {
				first = errors.Join(first, saveErr)
			}
		}
	}
	return first
}

// Existing summary transport/content/export behavior, with column-only writes
// fenced against generation changes during the external request. Remote jobs
// must never route through SummarizeEpisode's stale whole-record saves.
func summarizeRemoteCommittedItem(l *remoteLease, item *db.PodcastItem) error {
	cfg := LoadLLMConfig()
	setting := db.GetOrCreateSetting()
	if !cfg.Enabled || !setting.SummarizationEnabled || strings.TrimSpace(cfg.APIKey) == "" {
		return nil
	}
	cfg.Model = ResolveSummarizationModel(setting, cfg)
	prompt := ResolveSummarizationPrompt(setting, cfg)
	userPrompt := ResolveSummarizationUserPrompt(setting, cfg)
	if strings.TrimSpace(item.CanonicalTranscript) == "" {
		return nil
	}
	status := "processing"
	if strings.TrimSpace(item.LLMSummary) != "" {
		status = "available"
	}
	if err := l.update(item, map[string]any{"llm_summary_status": status}, true); err != nil {
		return err
	}
	summary, model, err := summarizeTranscriptWithModel(item.CanonicalTranscript, prompt, userPrompt, cfg)
	if err != nil {
		if status == "processing" {
			status = "failed"
		}
		saveErr := l.update(item, map[string]any{"llm_summary_status": status, "llm_summary_error": trimToLength(err.Error(), 1000)}, true)
		return errors.Join(err, saveErr)
	}
	now := time.Now().UTC()
	if err = l.update(item, map[string]any{"llm_summary": summary, "llm_summary_status": "available", "llm_summary_error": "", "llm_summary_date": &now, "llm_summary_model": model, "llm_summary_prompt": prompt}, true); err != nil {
		return err
	}
	if err = db.GetPodcastItemByID(item.ID, item); err != nil {
		return err
	}
	ExportSummary(item)
	return nil
}
