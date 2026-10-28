// Package podcastmarkdown implements Briefcast's portable podcast-markdown 1.2 contract.
package podcastmarkdown

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/google/uuid"
	"golang.org/x/text/language"
)

const Generator = "Briefcast podcast-markdown"
const GeneratorVersion = "1.2.3"

type Metadata = map[string]any

func Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ResourceID is independent of filenames, titles, and publisher identifiers.
func ResourceID(kind, identity string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("briefcast/podcast-markdown/"+kind+"/"+identity)).URN()
}

func WebURL(value string) string {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return ""
	}
	return u.String()
}

// Encode uses JSON values within a YAML mapping. This quotes dates, IDs and
// versions without allowing source text to inject YAML syntax.
func Encode(meta Metadata, body string) (string, error) {
	if err := Validate(meta); err != nil {
		return "", err
	}
	if !utf8.ValidString(body) {
		return "", fmt.Errorf("Markdown body is not UTF-8")
	}
	keys := make([]string, 0, len(meta))
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("---\n")
	for _, key := range keys {
		k, _ := json.Marshal(key)
		v, err := json.MarshalIndent(meta[key], "  ", "  ")
		if err != nil {
			return "", fmt.Errorf("metadata %s: %w", key, err)
		}
		fmt.Fprintf(&b, "%s: %s\n", k, v)
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n")
	result := b.String()
	// Parse our emitted YAML as well as validating the contract before publishing.
	if _, _, err := Parse(result); err != nil {
		return "", err
	}
	return result, nil
}

func Parse(document string) (Metadata, string, error) {
	document = strings.ReplaceAll(document, "\r\n", "\n")
	if !utf8.ValidString(document) || !strings.HasPrefix(document, "---\n") {
		return nil, "", fmt.Errorf("expected UTF-8 YAML frontmatter at the beginning")
	}
	end := strings.Index(document[4:], "\n---\n")
	if end < 0 {
		return nil, "", fmt.Errorf("unterminated frontmatter")
	}
	header := document[4 : 4+end]
	// The generated format has only JSON-compatible values. Refuse unfamiliar
	// executable tags, anchors or aliases rather than silently resolving them.
	tree, err := parser.ParseBytes([]byte(header), 0)
	if err != nil {
		return nil, "", fmt.Errorf("invalid YAML frontmatter")
	}
	if len(tree.Docs) != 1 {
		return nil, "", fmt.Errorf("frontmatter must be one mapping")
	}
	for _, kind := range []ast.NodeType{ast.TagType, ast.AnchorType, ast.AliasType, ast.DirectiveType} {
		if len(ast.FilterFile(kind, tree)) > 0 {
			return nil, "", fmt.Errorf("YAML tags, anchors, aliases and directives are unsupported")
		}
	}
	var meta Metadata
	if err := yaml.Unmarshal([]byte(header), &meta); err != nil {
		return nil, "", fmt.Errorf("invalid YAML frontmatter")
	}
	// Round-trip to normalize maps/lists/numbers and reject non-JSON values.
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, "", err
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, "", err
	}
	return meta, strings.TrimSpace(document[4+end+5:]), nil
}

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func NormalizeLanguage(value string) string {
	tag, err := language.Parse(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	if err != nil || tag == language.Und {
		return ""
	}
	return tag.String()
}

// Missing describes historical gaps. It never fills them with current settings.
func Missing(meta Metadata) []string {
	fields := []string{}
	switch meta["content_type"] {
	case "transcript":
		fields = append(fields, "transcription_method", "transcribed_at", "transcription_task", "timestamp_granularity")
		if meta["transcription_method"] == "unknown" {
			fields = append(fields, "transcription_method_origin")
		}
		if meta["transcription_method"] == "asr" || meta["transcription_method"] == "hybrid" {
			fields = append(fields, "transcription_engine", "transcription_engine_version", "transcription_model")
			if meta["asr_backend"] != nil {
				fields = append(fields, "asr_backend_version")
			}
			if meta["transcription_engine"] == "WhisperX" {
				fields = append(fields, "asr_backend", "asr_backend_version")
			}
		}
	case "summary":
		fields = append(fields, "summarized_at", "model_provider", "model_requested", "prompt_id", "prompt_version", "prompt_sha256", "generation_settings")
	case "source_note":
		fields = append(fields, "compiled_at", "compilation_method")
	}
	missing := []string{}
	if inherited, ok := meta["inherited_provenance_missing"].([]any); ok {
		for _, raw := range inherited {
			if value, ok := raw.(string); ok {
				missing = append(missing, value)
			}
		}
	}
	for _, field := range fields {
		if field == "generation_settings" && meta["provenance"] != nil {
			continue // Validated in the full companion record by the exporter.
		}
		if value, ok := meta[field]; !ok || value == "" {
			missing = append(missing, field)
		}
	}
	inputs, _ := meta["inputs"].([]any)
	if len(inputs) == 0 {
		missing = append(missing, "inputs")
	}
	for i, raw := range inputs {
		input, _ := raw.(map[string]any)
		for _, field := range []string{"kind", "id", "revision", "uri", "sha256", "coverage"} {
			if value, ok := input[field]; !ok || value == "" {
				missing = append(missing, fmt.Sprintf("inputs[%d].%s", i, field))
			}
		}
		if source, ok := input["source_provenance"].(map[string]any); ok {
			if gaps, ok := source["provenance_missing"].([]any); ok {
				for _, gap := range gaps {
					missing = append(missing, fmt.Sprintf("inputs[%d].source_provenance.%v", i, gap))
				}
			}
		}
	}
	if events, ok := meta["processing_events"].([]any); ok {
		for i, raw := range events {
			event, _ := raw.(map[string]any)
			if inputs, ok := event["inputs"].([]any); ok {
				for j, raw := range inputs {
					input, _ := raw.(map[string]any)
					for _, field := range []string{"kind", "id", "revision", "uri", "sha256", "coverage"} {
						if value, ok := input[field]; !ok || value == "" {
							missing = append(missing, fmt.Sprintf("processing_events[%d].inputs[%d].%s", i, j, field))
						}
					}
				}
			}
		}
	}
	sort.Strings(missing)
	unique := []string{}
	for _, field := range missing {
		if len(unique) == 0 || unique[len(unique)-1] != field {
			unique = append(unique, field)
		}
	}
	return unique
}

func SetProvenanceStatus(meta Metadata) {
	missing := Missing(meta)
	meta["provenance_missing"] = missing
	meta["provenance_status"] = "complete"
	if len(missing) > 0 {
		meta["provenance_status"] = "partial"
	}
}

func Validate(meta Metadata) error {
	if raw, exists := meta["provenance"]; exists {
		ref, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid provenance reference")
		}
		uri, ok := ref["uri"].(string)
		if !ok || uri == "" || strings.HasPrefix(uri, "/") || strings.ContainsAny(uri, "\\:\x00") || !sha256Pattern.MatchString(fmt.Sprint(ref["sha256"])) {
			return fmt.Errorf("invalid provenance reference")
		}
	}
	for _, field := range []string{"metadata_schema", "schema_version", "document_id", "episode_id", "source_type", "content_type", "title", "generation_id", "generator", "generator_version", "created", "updated", "provenance_status"} {
		if value, ok := meta[field].(string); !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("required string %s is missing", field)
		}
	}
	if meta["metadata_schema"] != "podcast-markdown" || meta["schema_version"] != "1.2" || meta["source_type"] != "podcast_episode" {
		return fmt.Errorf("unsupported metadata contract")
	}
	for _, key := range []string{"document_id", "episode_id", "generation_id"} {
		value := meta[key].(string)
		if !strings.HasPrefix(value, "urn:uuid:") {
			return fmt.Errorf("%s must be a UUID URN", key)
		}
		if _, err := uuid.Parse(value); err != nil {
			return fmt.Errorf("invalid %s", key)
		}
	}
	if !positiveInteger(meta["document_revision"]) {
		return fmt.Errorf("document_revision must be a positive integer")
	}
	for _, key := range []string{"created", "updated", "transcribed_at", "summarized_at", "compiled_at", "verified_at"} {
		if value, ok := meta[key]; ok {
			s, ok := value.(string)
			if !ok {
				return fmt.Errorf("%s must be a quoted datetime", key)
			}
			if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
				return fmt.Errorf("invalid %s", key)
			}
		}
	}
	if v, ok := meta["date_published"]; ok {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("date_published must be a string")
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return fmt.Errorf("invalid date_published")
		}
	}
	if v, ok := meta["language"]; ok {
		s, ok := v.(string)
		if !ok || strings.Contains(s, "_") {
			return fmt.Errorf("invalid language tag")
		}
		if _, err := language.Parse(s); err != nil {
			return fmt.Errorf("invalid language tag")
		}
	}
	for _, key := range []string{"source_url", "feed_url", "audio_url"} {
		if v, ok := meta[key]; ok && WebURL(fmt.Sprint(v)) == "" {
			return fmt.Errorf("invalid %s", key)
		}
	}
	if v, ok := meta["episode_number"]; ok {
		if _, ok := v.(string); !ok {
			return fmt.Errorf("episode_number must be a string")
		}
	}
	if v, ok := meta["duration_seconds"]; ok && !nonnegativeInteger(v) {
		return fmt.Errorf("invalid duration_seconds")
	}
	for _, key := range []string{"hosts", "guests"} {
		if v, ok := meta[key]; ok {
			raw, _ := json.Marshal(v)
			var names []string
			if len(raw) == 0 || raw[0] != '[' || json.Unmarshal(raw, &names) != nil {
				return fmt.Errorf("%s must be a list of names", key)
			}
		}
	}
	if meta["content_type"] != "transcript" && meta["content_type"] != "summary" && meta["content_type"] != "source_note" {
		return fmt.Errorf("unsupported content_type")
	}
	if meta["content_type"] == "transcript" {
		if !oneOf(meta["transcription_method"], "asr", "human", "hybrid", "unknown") {
			return fmt.Errorf("invalid transcription_method")
		}
		if !oneOf(meta["timestamp_granularity"], "word", "segment", "none") {
			return fmt.Errorf("invalid timestamp_granularity")
		}
		if v, ok := meta["transcription_task"]; ok && !oneOf(v, "transcribe", "translate") {
			return fmt.Errorf("invalid transcription_task")
		}
	}
	if !oneOf(meta["provenance_status"], "complete", "partial", "unknown") {
		return fmt.Errorf("invalid provenance_status")
	}
	missingJSON, _ := json.Marshal(meta["provenance_missing"])
	var missing []string
	if len(missingJSON) == 0 || missingJSON[0] != '[' || json.Unmarshal(missingJSON, &missing) != nil {
		return fmt.Errorf("provenance_missing must be a list")
	}
	for _, field := range Missing(meta) {
		found := false
		for _, recorded := range missing {
			if field == recorded {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("undeclared provenance gap: %s", field)
		}
	}
	if len(missing) > 0 && meta["provenance_status"] == "complete" {
		return fmt.Errorf("incomplete provenance marked complete")
	}
	if meta["production_origin"] == "captured" {
		for _, field := range Missing(meta) {
			if strings.Contains(field, ".source_provenance.") || strings.HasSuffix(field, ".uri") || field == "resumed_asr_history" {
				continue
			}
			return fmt.Errorf("new production did not capture %s", field)
		}
	}
	if prompt, ok := meta["prompt"].(map[string]any); ok {
		bytes, err := json.Marshal(prompt)
		if err != nil || Hash(bytes) != meta["prompt_sha256"] {
			return fmt.Errorf("retained prompt hash mismatch")
		}
		for _, key := range []string{"system", "user_template"} {
			if _, ok := prompt[key].(string); !ok {
				return fmt.Errorf("missing retained prompt %s", key)
			}
		}
	} else if meta["production_origin"] == "captured" && meta["content_type"] == "summary" && meta["provenance"] == nil {
		return fmt.Errorf("new summary has no retained prompt")
	}
	inputs, ok := meta["inputs"].([]any)
	if !ok {
		return fmt.Errorf("inputs must be a list")
	}
	for _, raw := range inputs {
		input, ok := raw.(map[string]any)
		if !ok || !oneOf(input["kind"], "media", "transcript", "summary", "source_note", "other") || !oneOf(input["coverage"], "full", "partial", "unknown") {
			return fmt.Errorf("invalid input record")
		}
		if hash, ok := input["sha256"]; ok && !sha256Pattern.MatchString(fmt.Sprint(hash)) {
			return fmt.Errorf("invalid input hash")
		}
		if text, ok := input["text"].(string); ok && Hash([]byte(text)) != input["sha256"] {
			return fmt.Errorf("input snapshot hash mismatch")
		}
		if input["coverage"] == "partial" && input["locators"] == nil {
			return fmt.Errorf("partial input requires locators")
		}
		if uri, ok := input["uri"].(string); ok && (strings.HasPrefix(uri, "/") || strings.Contains(uri, "\\") || (strings.Contains(uri, ":") && WebURL(uri) == "")) {
			return fmt.Errorf("nonportable input URI")
		}
	}
	if err := validateProcessingEvents(meta["processing_events"]); err != nil {
		return err
	}
	if snapshots, ok := meta["input_snapshots"].(map[string]any); ok {
		for hash, raw := range snapshots {
			if !sha256Pattern.MatchString(hash) {
				return fmt.Errorf("retained input snapshot hash mismatch")
			}
			switch value := raw.(type) {
			case string: // Read earlier exports without discarding their history.
				if Hash([]byte(value)) != hash {
					return fmt.Errorf("retained input snapshot hash mismatch")
				}
			case map[string]any:
				if value["sha256"] != hash || value["uri"] != "inputs/"+hash+".txt" {
					return fmt.Errorf("invalid input snapshot reference")
				}
			default:
				return fmt.Errorf("invalid input snapshot")
			}
		}
	}
	if status, ok := meta["verification_status"]; ok {
		checked := map[string]string{"transcript": "checked_against_media", "summary": "checked_against_transcript", "source_note": "checked_against_inputs"}[fmt.Sprint(meta["content_type"])]
		if status != "unreviewed" && status != checked {
			return fmt.Errorf("invalid verification_status")
		}
		if status == checked {
			for _, key := range []string{"verified_at", "verified_by", "verified_revision", "verification_inputs"} {
				if meta[key] == nil {
					return fmt.Errorf("verification missing %s", key)
				}
			}
			if fmt.Sprint(meta["verified_revision"]) != fmt.Sprint(meta["document_revision"]) {
				return fmt.Errorf("verification is for a different revision")
			}
		}
	}
	return nil
}

func validateProcessingEvents(raw any) error {
	if raw == nil {
		return nil
	}
	events, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("processing_events must be a list")
	}
	for _, raw := range events {
		event, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid processing event")
		}
		for _, key := range []string{"operation", "completed_at"} {
			if value, ok := event[key].(string); !ok || value == "" {
				return fmt.Errorf("processing event missing %s", key)
			}
		}
		if _, err := time.Parse(time.RFC3339Nano, event["completed_at"].(string)); err != nil {
			return fmt.Errorf("invalid event completion datetime")
		}
		if tool, ok := event["tool"].(string); ok && tool != "" && event["tool_version"] == nil {
			return fmt.Errorf("processing event missing tool_version")
		}
		inputs, ok := event["inputs"].([]any)
		if !ok {
			return fmt.Errorf("event inputs must be a list")
		}
		for _, raw := range inputs {
			input, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid event input")
			}
			if text, ok := input["text"].(string); ok && Hash([]byte(text)) != input["sha256"] {
				return fmt.Errorf("event input hash mismatch")
			}
		}
		output, ok := event["output"].(map[string]any)
		if !ok || output["id"] == nil || !positiveInteger(output["revision"]) {
			return fmt.Errorf("event missing output identity/revision")
		}
	}
	return nil
}

func oneOf(value any, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func nonnegativeInteger(value any) bool {
	switch v := value.(type) {
	case int:
		return v >= 0
	case float64:
		return v >= 0 && v == float64(int64(v))
	case int64:
		return v >= 0
	}
	return false
}
func positiveInteger(value any) bool { return nonnegativeInteger(value) && fmt.Sprint(value) != "0" }
