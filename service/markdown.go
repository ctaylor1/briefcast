package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/feedmeta"
	pm "github.com/ctaylor1/briefcast/internal/podcastmarkdown"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// EpisodeMarkdown is shared by disk and UI exports. No browser-side metadata
// reconstruction is needed, so renaming or changing vaults preserves identity.
func EpisodeMarkdown(item *db.PodcastItem, kind string) (string, error) {
	if item.ID == "" {
		return "", fmt.Errorf("episode has no stable ID")
	}
	// Resolve associations on a copy so source fingerprints and event snapshots
	// are identical for batch exports and preloaded UI requests.
	resolved := *item
	if resolved.Podcast.ID == "" {
		if err := db.GetPodcastByID(resolved.PodcastID, &resolved.Podcast); err != nil {
			return "", err
		}
	}
	item = &resolved
	meta, body, err := markdownSource(item, kind)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(body) == "" {
		return "", nil
	}
	docID := pm.ResourceID(kind, item.ID)
	// Optimistic allocation with a database unique constraint, without process-
	// global locks. Concurrent identical exports converge on the same revision.
	for attempt := 0; attempt < 5; attempt++ {
		var previous db.MarkdownRevision
		query := db.DB.Where("document_id = ?", docID).Order("revision desc").Limit(1).Find(&previous)
		if query.Error != nil {
			return "", query.Error
		}
		if previous.ID != "" {
			if err := retainUnfamiliarMarkdownMetadata(meta, previous); err != nil {
				return "", err
			}
		} else if err := retainLegacyMarkdownMetadata(meta, item, kind); err != nil {
			return "", err
		}
		fingerprintBytes, err := json.Marshal([]any{pm.GeneratorVersion, meta, body})
		if err != nil {
			return "", err
		}
		fingerprint := pm.Hash(fingerprintBytes)
		if previous.ID != "" && previous.Fingerprint == fingerprint {
			return previous.Content, nil
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		meta["metadata_schema"], meta["schema_version"] = "podcast-markdown", "1.2"
		meta["document_id"], meta["document_revision"] = docID, previous.Revision+1
		meta["episode_id"] = pm.ResourceID("episode", item.ID)
		meta["source_type"], meta["content_type"] = "podcast_episode", kind
		meta["created"], meta["updated"] = now, now
		meta["generation_id"] = uuid.New().URN()
		meta["generator"], meta["generator_version"] = pm.Generator, pm.GeneratorVersion
		meta["verification_status"] = "unreviewed"
		if previous.ID != "" {
			old, parseErr := markdownRevisionMetadata(previous)
			if parseErr != nil {
				return "", parseErr
			}
			meta["created"] = old["created"]
			if snapshots, ok := old["input_snapshots"].(map[string]any); ok {
				meta["input_snapshots"] = snapshots
			}
			// Carry unfamiliar metadata forward; known production fields are built
			// afresh so a replacement cannot inherit old model or review claims.
			for key, value := range old {
				if !ownedMarkdownField(key) {
					if _, exists := meta[key]; !exists {
						meta[key] = value
					}
				}
			}
			prior := old["processing_events"]
			if events, ok := prior.([]any); ok {
				current, _ := meta["processing_events"].([]any)
				meta["processing_events"] = mergeMarkdownEvents(events, current)
			}
			meta["previous_revision"] = map[string]any{"id": docID, "revision": previous.Revision, "sha256": pm.Hash([]byte(previous.Content))}
		}
		events, _ := meta["processing_events"].([]any)
		exportInputs := markdownExportInputs(item, kind)
		if legacy, ok := meta["legacy_frontmatter_source"].(map[string]any); ok {
			exportInputs = append(exportInputs, legacy)
		}
		meta["processing_events"] = append(events, map[string]any{
			"event_id": meta["generation_id"], "operation": "markdown_export", "completed_at": now,
			"tool": pm.Generator, "tool_version": pm.GeneratorVersion,
			"inputs": exportInputs, "output": map[string]any{"id": docID, "revision": previous.Revision + 1},
			"settings": map[string]any{"transcript_rendering": "plain canonical text; no added Markdown, segment labels, or timestamps"},
		})
		for _, raw := range meta["processing_events"].([]any) {
			if event, ok := raw.(map[string]any); ok && event["output"] == nil {
				event["output"] = map[string]any{"id": docID, "revision": previous.Revision + 1}
			}
		}
		pm.SetProvenanceStatus(meta)
		compactMarkdownSnapshots(meta)
		// Keep payload bytes out of YAML properties. Publish exact companion inputs
		// before committing a revision that references them, including UI exports.
		if err := separateMarkdownProvenance(meta, item, kind, previous); err != nil {
			return "", err
		}
		content, encodeErr := pm.Encode(meta, body)
		if encodeErr != nil {
			return "", encodeErr
		}
		record := db.MarkdownRevision{DocumentID: docID, Revision: previous.Revision + 1, EpisodeID: item.ID, ContentType: kind, Fingerprint: fingerprint, Content: content}
		// Do not allow database error logging to dump full private documents/prompts.
		result := db.DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if result.Error != nil {
			return "", result.Error
		}
		if result.RowsAffected == 1 {
			return content, nil
		}
		// Rebuild the source map after a concurrent allocation.
		meta, body, err = markdownSource(item, kind)
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("Markdown revision changed concurrently; retry export")
}

func retainLegacyMarkdownMetadata(meta pm.Metadata, item *db.PodcastItem, kind string) error {
	if resolveExportDir() == "" {
		return nil
	}
	dir := assetMarkdownTranscriptDir
	if kind == "summary" {
		dir = assetMarkdownSummariesDir
	}
	path := filepath.Join(resolveExportDir(), dir, sanitizeAssetName(podcastTitle(item)), pubDateSubdir(item.PubDate), exportFileComponent(exportEpisodeFileName(item), ".md"))
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ReplaceAll(string(content), "\r\n", "\n"), "---\n") {
		return nil
	}
	legacy, _, err := pm.Parse(string(content))
	if err != nil {
		return fmt.Errorf("legacy Markdown frontmatter is invalid; original preserved: %w", err)
	}
	for key, value := range legacy {
		if !ownedMarkdownField(key) {
			if _, exists := meta[key]; !exists {
				meta[key] = value
			}
		}
	}
	hash := pm.Hash(content)
	meta["legacy_frontmatter_source"] = map[string]any{"kind": "other", "id": pm.ResourceID("legacy-markdown", hash), "revision": 1, "uri": filepath.Base(path), "sha256": hash, "coverage": "partial", "locators": []any{map[string]any{"section": "yaml_frontmatter"}}}
	return nil
}

func retainUnfamiliarMarkdownMetadata(meta pm.Metadata, previous db.MarkdownRevision) error {
	old, err := markdownRevisionMetadata(previous)
	if err != nil {
		return err
	}
	copyUnknown := func(from pm.Metadata) {
		for key, value := range from {
			if !ownedMarkdownField(key) {
				if _, exists := meta[key]; !exists {
					meta[key] = value
				}
			}
		}
	}
	copyUnknown(old)
	if previous.ExportPath == "" || resolveExportDir() == "" {
		return nil
	}
	path := filepath.Join(resolveExportDir(), filepath.FromSlash(previous.ExportPath))
	if !IsPathWithinAssetsDir(path) {
		return fmt.Errorf("Markdown export path is outside assets")
	}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if string(content) == previous.Content {
		return nil
	}
	// A changed exported file is retained untouched. Read its unfamiliar metadata
	// into the next revision, without inventing an edit time, author or review.
	external, _, err := pm.Parse(string(content))
	if err != nil {
		return fmt.Errorf("edited Markdown has invalid frontmatter; preserve and repair it before re-export: %w", err)
	}
	for key, value := range external {
		if !ownedMarkdownField(key) {
			meta[key] = value
		}
	}
	meta["external_file_observation"] = map[string]any{"sha256": pm.Hash(content), "document_revision": previous.Revision}
	return nil
}

func mergeMarkdownEvents(prior, current []any) []any {
	result := append([]any{}, prior...)
	seen := map[string]bool{}
	for _, event := range prior {
		seen[markdownEventKey(event)] = true
	}
	for _, event := range current {
		if !seen[markdownEventKey(event)] {
			result = append(result, event)
			seen[markdownEventKey(event)] = true
		}
	}
	return result
}

func markdownEventKey(event any) string {
	if m, ok := event.(map[string]any); ok && m["event_id"] != nil {
		return fmt.Sprint(m["event_id"])
	}
	b, _ := json.Marshal(event)
	return string(b)
}

func ownedMarkdownField(key string) bool {
	if key == "input_snapshots" || key == "provenance" {
		return true
	}
	if strings.Contains("|production_origin|inherited_provenance_missing|source_language|output_language|media_source|legacy_model_identifier|legacy_system_prompt|legacy_pipeline_completed_at|", "|"+key+"|") {
		return true
	}
	return strings.Contains("|metadata_schema|schema_version|document_id|document_revision|episode_id|source_type|content_type|title|language|created|updated|generation_id|generator|generator_version|inputs|processing_events|provenance_status|provenance_missing|episode_title|podcast|date_published|source_url|feed_url|rss_guid|episode_number|hosts|guests|duration_seconds|audio_url|transcription_method|transcribed_at|transcription_engine|transcription_engine_version|asr_backend|asr_backend_version|transcription_model|transcription_model_revision|transcription_task|timestamp_granularity|transcribed_by|summarized_at|model_provider|model_requested|model_resolved|model_revision|prompt_id|prompt_version|prompt_sha256|prompt|generation_settings|verification_status|verified_at|verified_by|verified_revision|verification_inputs|previous_revision|", "|"+key+"|")
}

func markdownSource(item *db.PodcastItem, kind string) (pm.Metadata, string, error) {
	podcast := item.Podcast
	if podcast.ID == "" {
		if err := db.GetPodcastByID(item.PodcastID, &podcast); err != nil {
			return nil, "", err
		}
	}
	entry, feed := map[string]any{}, map[string]any{}
	if err := decodeOptionalMetadata(item.ItemMetadata, &entry); err != nil {
		return nil, "", err
	}
	if err := decodeOptionalMetadata(podcast.FeedMetadata, &feed); err != nil {
		return nil, "", err
	}
	meta := pm.Metadata{"episode_title": item.Title, "podcast": podcast.Title, "inputs": []any{}}
	// A change to exact source bytes is material even if the rendered text is equal.
	sourceInputs, _ := json.Marshal(markdownExportInputs(item, kind))
	meta["export_input_sha256"] = pm.Hash(sourceInputs)
	if !item.PubDate.IsZero() {
		meta["date_published"] = item.PubDate.Format("2006-01-02")
	}
	for key, value := range map[string]string{"source_url": episodeWebURL(entry), "feed_url": podcast.URL, "audio_url": item.FileURL} {
		if key == "source_url" && (value == item.FileURL || value == podcast.URL) {
			continue
		}
		if value = pm.WebURL(value); value != "" {
			meta[key] = value
		}
	}
	if item.GUID != "" {
		meta["rss_guid"] = item.GUID
	}
	if item.Duration > 0 {
		meta["duration_seconds"] = item.Duration
	}
	// Only publisher episode numbers qualify. Library ordering is not one.
	if n := feedmeta.PickFirstNonEmpty(feedmeta.GetString(entry, "itunes_episode"), feedmeta.GetString(entry, "podcast_episode"), feedmeta.GetString(entry, "episode_number")); n != "" {
		meta["episode_number"] = n
	}
	for _, key := range []string{"hosts", "guests"} {
		if names, ok := entry[key].([]any); ok {
			meta[key] = names
		}
	}
	addKnownPodcastPeople(meta, entry, feed)
	switch kind {
	case "transcript":
		meta["title"] = item.Title + " — Transcript"
		body, granularity := markdownTranscriptBody(item)
		meta["timestamp_granularity"] = granularity
		meta["transcription_method"] = "unknown"
		var raw map[string]any
		if json.Unmarshal([]byte(item.TranscriptJSON), &raw) == nil && raw != nil {
			if raw["provider"] == "whisperx" {
				meta["transcription_method"], meta["transcription_engine"] = "asr", "WhisperX"
				copyMarkdownString(meta, "transcription_model", raw, "model")
				copyMarkdownString(meta, "transcription_model_revision", raw, "model_revision")
				copyMarkdownString(meta, "language", raw, "language")
				if media, ok := raw["source_media"].(map[string]any); ok {
					meta["media_source"] = media
				}
				if runtime, ok := raw["metadata"].(map[string]any); ok {
					copyMarkdownString(meta, "transcription_engine_version", runtime, "whisperx_version")
					copyMarkdownString(meta, "asr_backend_version", runtime, "faster_whisper_version")
					if meta["asr_backend_version"] != nil {
						meta["asr_backend"] = "faster-whisper"
					}
					copyMarkdownString(meta, "transcribed_at", runtime, "transcribed_at")
					if runtime["transcribed_at"] == nil && runtime["asr_performed"] == nil {
						copyMarkdownString(meta, "legacy_pipeline_completed_at", runtime, "generated_at")
					}
				}
				if provenance, ok := raw["provenance"].(map[string]any); ok {
					for key, value := range provenance {
						meta[key] = value
					}
					if inputs, ok := meta["inputs"].([]any); ok {
						for _, raw := range inputs {
							if input, ok := raw.(map[string]any); ok {
								if uri := pm.WebURL(item.FileURL); uri != "" && input["kind"] == "media" {
									input["uri"] = uri
								}
							}
						}
					}
				}
			} else if assets, ok := raw["inputs"].([]any); ok {
				meta["inputs"] = assets
			}
		}
		if meta["transcription_model"] == nil && item.TranscriptModel != "" {
			meta["transcription_model"] = item.TranscriptModel
		}
		if runtime, ok := raw["metadata"].(map[string]any); ok && runtime["asr_performed"] == false {
			// Finishing alignment from a complete checkpoint performs no new ASR.
			// Its runtime/model/time must not replace unknown historical ASR facts.
			for _, key := range []string{"transcribed_at", "transcription_model", "transcription_model_revision", "transcription_engine_version", "asr_backend", "asr_backend_version", "transcription_task"} {
				delete(meta, key)
			}
		}
		// Imported transcripts are inputs, not evidence of human transcription.
		if assets := transcriptAssetInputs(item.TranscriptJSON); len(assets) > 0 {
			meta["inputs"] = assets
			var imported []feedmeta.TranscriptAsset
			if json.Unmarshal([]byte(item.TranscriptJSON), &imported) == nil {
				for _, asset := range imported {
					if asset.Content != "" && asset.Language != "" {
						meta["language"] = asset.Language
						break
					}
				}
			}
		}
		addMediaInputLocations(meta, item)
		if value, ok := meta["language"].(string); ok {
			if normalized := pm.NormalizeLanguage(value); normalized != "" {
				meta["language"] = normalized
			} else {
				delete(meta, "language")
			}
		}
		return meta, body, nil
	case "summary":
		meta["title"] = item.Title + " — Summary"
		if item.LLMSummaryDate != nil {
			meta["summarized_at"] = item.LLMSummaryDate.UTC().Format(time.RFC3339Nano)
		}
		if strings.TrimSpace(item.SummaryProvenanceJSON) != "" {
			var provenance map[string]any
			if err := json.Unmarshal([]byte(item.SummaryProvenanceJSON), &provenance); err != nil {
				return nil, "", fmt.Errorf("invalid summary provenance: %w", err)
			}
			for key, value := range provenance {
				meta[key] = value
			}
		} else {
			// Old model fields may have fallen back to the requested identifier;
			// preserve this ambiguity instead of claiming a provider resolution.
			delete(meta, "model_resolved")
			if item.LLMSummaryModel != "" {
				meta["legacy_model_identifier"] = item.LLMSummaryModel
			}
			if item.LLMSummaryPrompt != "" {
				meta["legacy_system_prompt"] = item.LLMSummaryPrompt
			}
		}
		return meta, strings.TrimSpace(item.LLMSummary), nil
	default:
		return nil, "", fmt.Errorf("unsupported Markdown file type %q", kind)
	}
}

func addKnownPodcastPeople(meta pm.Metadata, sources ...map[string]any) {
	for _, source := range sources {
		for _, key := range []string{"podcast_person", "podcast_persons", "persons"} {
			raw := source[key]
			people, ok := raw.([]any)
			if person, ok := raw.(map[string]any); ok {
				people = []any{person}
			}
			if !ok && len(people) == 0 {
				continue
			}
			for _, raw := range people {
				person, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				role := strings.ToLower(feedmeta.GetString(person, "role"))
				field := ""
				if role == "host" {
					field = "hosts"
				}
				if role == "guest" {
					field = "guests"
				}
				name := feedmeta.PickFirstNonEmpty(feedmeta.GetString(person, "name"), feedmeta.GetString(person, "value"))
				if field == "" || name == "" {
					continue
				}
				names, _ := meta[field].([]any)
				found := false
				for _, current := range names {
					if current == name {
						found = true
					}
				}
				if !found {
					meta[field] = append(names, name)
				}
			}
		}
	}
}

func addMediaInputLocations(meta pm.Metadata, item *db.PodcastItem) {
	uri := pm.WebURL(item.FileURL)
	if uri == "" && item.DownloadPath != "" && IsPathWithinAssetsDir(item.DownloadPath) {
		dir := filepath.Join(resolveExportDir(), assetMarkdownTranscriptDir, sanitizeAssetName(podcastTitle(item)), pubDateSubdir(item.PubDate))
		if relative, err := filepath.Rel(dir, item.DownloadPath); err == nil {
			uri = filepath.ToSlash(relative)
		}
	}
	if uri == "" {
		return
	}
	inputs, _ := meta["inputs"].([]any)
	sourceHashes := map[string]bool{}
	if media, ok := meta["media_source"].(map[string]any); ok {
		sourceHashes[fmt.Sprint(media["sha256"])] = true
	}
	for _, raw := range inputs {
		if input, ok := raw.(map[string]any); ok && input["kind"] == "media" {
			sourceHashes[fmt.Sprint(input["sha256"])] = true
		}
	}
	var visit func(any)
	visit = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if v["kind"] == "media" && sourceHashes[fmt.Sprint(v["sha256"])] {
				v["uri"] = uri
			}
			for _, child := range v {
				visit(child)
			}
		case []any:
			for _, child := range v {
				visit(child)
			}
		}
	}
	visit(meta)
}

func decodeOptionalMetadata(raw string, target *map[string]any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("invalid episode/feed metadata: %w", err)
	}
	return nil
}

func episodeWebURL(entry map[string]any) string {
	if s := pm.WebURL(feedmeta.GetString(entry, "link")); s != "" {
		return s
	}
	links, _ := entry["links"].([]any)
	for _, raw := range links {
		link, _ := raw.(map[string]any)
		if link["rel"] == "alternate" {
			if s := pm.WebURL(feedmeta.GetString(link, "href")); s != "" {
				return s
			}
		}
	}
	return ""
}

func copyMarkdownString(to map[string]any, key string, from map[string]any, source string) {
	if s, ok := from[source].(string); ok && s != "" && s != "unknown" {
		to[key] = s
	}
}

func transcriptAssetInputs(raw string) []any {
	var assets []feedmeta.TranscriptAsset
	if json.Unmarshal([]byte(raw), &assets) != nil {
		return nil
	}
	inputs := []any{}
	for _, asset := range assets {
		if asset.Content == "" {
			continue
		}
		hash := pm.Hash([]byte(asset.Content))
		input := map[string]any{"kind": "transcript", "id": pm.ResourceID("import", hash), "revision": 1, "sha256": hash, "coverage": "unknown", "text": asset.Content}
		if uri := pm.WebURL(asset.URL); uri != "" {
			input["uri"] = uri
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func markdownTranscriptBody(item *db.PodcastItem) (string, string) {
	// Match the existing plain-text export, including stored paragraphs, speaker
	// labels and manual corrections. Original ASR timing remains in input companions.
	return exportableCanonicalTranscript(item), "none"
}

// writeEpisodeMarkdown never overwrites an existing file, including manual
// edits and legacy exports. Conflicts get a distinct export filename. Identity
// and revision live in the document, not in its filename.
func writeEpisodeMarkdown(item *db.PodcastItem, kind string) error {
	content, err := EpisodeMarkdown(item, kind)
	if err != nil || content == "" {
		return err
	}
	meta, _, err := pm.Parse(content)
	if err != nil {
		return err
	}
	base := filepath.Join(markdownDirectory(item, kind), exportEpisodeFileName(item))
	if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
		return err
	}
	if _, err := readMarkdownProvenance(meta, filepath.Dir(base)); err != nil {
		return err
	}
	for i := 0; i < 1000; i++ {
		path := filepath.Join(filepath.Dir(base), exportFileComponent(filepath.Base(base), ".md"))
		if i > 0 {
			suffix := fmt.Sprintf(".r%v.%s.%d.md", meta["document_revision"], pm.Hash([]byte(content))[:12], i)
			path = filepath.Join(filepath.Dir(base), exportFileComponent(filepath.Base(base), suffix))
		}
		old, readErr := os.ReadFile(path)
		if readErr == nil {
			if string(old) == content {
				return recordMarkdownExportPath(meta, path)
			}
			continue
		}
		if !os.IsNotExist(readErr) {
			return readErr
		}
		openErr := publishMarkdownFile(path, []byte(content))
		if errors.Is(openErr, os.ErrExist) {
			continue
		}
		if openErr != nil {
			return openErr
		}
		return recordMarkdownExportPath(meta, path)
	}
	return fmt.Errorf("too many conflicting Markdown export files")
}

func markdownDirectory(item *db.PodcastItem, kind string) string {
	dir := assetMarkdownTranscriptDir
	if kind == "summary" {
		dir = assetMarkdownSummariesDir
	}
	return filepath.Join(resolveExportDir(), dir, sanitizeAssetName(podcastTitle(item)), pubDateSubdir(item.PubDate))
}

func markdownExportInputs(item *db.PodcastItem, kind string) []any {
	inputs := []any{}
	add := func(inputKind, format, text string) {
		if text == "" {
			return
		}
		hash := pm.Hash([]byte(text))
		inputs = append(inputs, map[string]any{"kind": inputKind, "id": pm.ResourceID("stored-"+inputKind, hash), "revision": 1, "uri": "inputs/" + hash + ".txt", "sha256": hash, "coverage": "full", "format": format, "text": text})
	}
	if kind == "transcript" {
		add("transcript", "briefcast-transcript-json", item.TranscriptJSON)
		add("transcript", "canonical-plain-text", item.CanonicalTranscript)
	} else {
		add("summary", "markdown", item.LLMSummary)
	}
	add("other", "episode-feed-metadata-json", item.ItemMetadata)
	add("other", "podcast-feed-metadata-json", item.Podcast.FeedMetadata)
	return inputs
}

func compactMarkdownSnapshots(meta pm.Metadata) {
	snapshots, _ := meta["input_snapshots"].(map[string]any)
	if snapshots == nil {
		snapshots = map[string]any{}
	}
	var visit func(any)
	visit = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if text, ok := v["text"].(string); ok && v["sha256"] == pm.Hash([]byte(text)) {
				snapshots[pm.Hash([]byte(text))] = text
				delete(v, "text")
			}
			for key, child := range v {
				if key != "input_snapshots" {
					visit(child)
				}
			}
		case []any:
			for _, child := range v {
				visit(child)
			}
		}
	}
	visit(meta)
	meta["input_snapshots"] = snapshots
}

func externalizeMarkdownSnapshots(meta pm.Metadata, dir string, previous db.MarkdownRevision) error {
	if err := pm.Validate(meta); err != nil {
		return err
	}
	if resolveExportDir() == "" {
		return fmt.Errorf("assets directory is required for Markdown input companions")
	}
	var priorDirs []string
	if previous.ID != "" {
		if previous.ExportPath != "" {
			path := filepath.Join(resolveExportDir(), filepath.FromSlash(previous.ExportPath))
			if !IsPathWithinAssetsDir(path) {
				return fmt.Errorf("Markdown export path is outside assets")
			}
			priorDirs = append(priorDirs, filepath.Dir(path))
		} else {
			// UI requests may have prepared a document and companions before its
			// .md was exported. Recover that directory from its retained metadata.
			old, _, err := pm.Parse(previous.Content)
			if err != nil {
				return err
			}
			published, _ := time.Parse("2006-01-02", fmt.Sprint(old["date_published"]))
			priorItem := db.PodcastItem{Podcast: db.Podcast{Title: fmt.Sprint(old["podcast"])}, PubDate: published}
			priorDirs = append(priorDirs, markdownDirectory(&priorItem, previous.ContentType))
		}
	}
	if err := writeMarkdownInputSnapshots(meta, dir, priorDirs...); err != nil {
		return err
	}
	snapshots, _ := meta["input_snapshots"].(map[string]any)
	for hash := range snapshots {
		snapshots[hash] = map[string]any{"uri": "inputs/" + hash + ".txt", "sha256": hash}
	}
	return nil
}

func recordMarkdownExportPath(meta pm.Metadata, path string) error {
	relative, err := filepath.Rel(resolveExportDir(), path)
	if err != nil {
		return err
	}
	return db.DB.Model(&db.MarkdownRevision{}).Where("document_id = ? AND revision = ?", meta["document_id"], meta["document_revision"]).Update("export_path", filepath.ToSlash(relative)).Error
}

func publishMarkdownFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".markdown-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	// Linking a complete temporary file is atomic and refuses to replace any
	// existing destination. A failed write cannot leave a partial .md document.
	return os.Link(temp, path)
}

func writeMarkdownInputSnapshots(value any, dir string, priorDirs ...string) error {
	switch v := value.(type) {
	case map[string]any:
		if snapshots, ok := v["input_snapshots"].(map[string]any); ok {
			for hash, raw := range snapshots {
				text, inline := raw.(string)
				if !inline {
					reference, ok := raw.(map[string]any)
					if !ok || reference["sha256"] != hash || reference["uri"] != "inputs/"+hash+".txt" {
						return fmt.Errorf("invalid input snapshot reference")
					}
					var found bool
					for _, sourceDir := range append([]string{dir}, priorDirs...) {
						payload, err := os.ReadFile(filepath.Join(sourceDir, "inputs", hash+".txt"))
						if os.IsNotExist(err) {
							continue
						}
						if err != nil {
							return err
						}
						if pm.Hash(payload) != hash {
							return fmt.Errorf("existing input snapshot hash mismatch")
						}
						text, found = string(payload), true
						break
					}
					if !found {
						return fmt.Errorf("input companion %s is missing; restore it before exporting", hash)
					}
				}
				if pm.Hash([]byte(text)) != hash {
					return fmt.Errorf("input snapshot hash mismatch")
				}
				if err := writeMarkdownInputSnapshots(map[string]any{"sha256": hash, "uri": "inputs/" + hash + ".txt", "text": text}, dir); err != nil {
					return err
				}
			}
		}
		if text, ok := v["text"].(string); ok {
			hash := pm.Hash([]byte(text))
			if v["uri"] == "inputs/"+hash+".txt" {
				path := filepath.Join(dir, "inputs", hash+".txt")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return err
				}
				if err := publishMarkdownFile(path, []byte(text)); err != nil {
					if !errors.Is(err, os.ErrExist) {
						return err
					}
					existing, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					if pm.Hash(existing) != hash {
						return fmt.Errorf("existing input snapshot hash mismatch")
					}
				}
			}
		}
		for _, child := range v {
			if err := writeMarkdownInputSnapshots(child, dir); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := writeMarkdownInputSnapshots(child, dir); err != nil {
				return err
			}
		}
	}
	return nil
}
