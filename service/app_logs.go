package service

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/internal/logging"
)

const (
	defaultAppLogLimit      = 200
	maxAppLogLimit          = 500
	maxAppLogFiles          = 12
	maxAppLogTailBytes      = 2 * 1024 * 1024
	maxAppLogRawLength      = 4000
	maxAppLogStringLength   = 1000
	maxAppLogImpactEntries  = 25
	appLogFallbackDirectory = "logs"
)

var sensitiveLogPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)((?:"?(?:api[_-]?key|apikey|hf_token|password|passwd|refresh_token|token|secret|authorization|cookie)"?\s*[:=]\s*"?))[^"\s,;&}]+`),
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+)[^\s,;"]+`),
	regexp.MustCompile(`(?i)(api[_-]?key\s*[:=]\s*)[^\s,;&"}]+`),
	regexp.MustCompile(`(?i)(token\s*[:=]\s*)[^\s,;&"}]+`),
	regexp.MustCompile(`(?i)(password\s*[:=]\s*)[^\s,;&"}]+`),
	regexp.MustCompile(`(?i)(secret\s*[:=]\s*)[^\s,;&"}]+`),
}

type AppLogResponse struct {
	Entries         []AppLogEntry  `json:"entries"`
	ImpactEntries   []AppLogEntry  `json:"impactEntries"`
	Sources         []AppLogSource `json:"sources"`
	ReadErrors      []string       `json:"readErrors,omitempty"`
	Limit           int            `json:"limit"`
	TotalDiscovered int            `json:"totalDiscovered"`
}

type AppLogSource struct {
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
	SizeBytes int64     `json:"sizeBytes"`
}

type AppLogEntry struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	Level        string                 `json:"level"`
	Source       string                 `json:"source"`
	Service      string                 `json:"service,omitempty"`
	Caller       string                 `json:"caller,omitempty"`
	Message      string                 `json:"message"`
	HumanMessage string                 `json:"humanMessage"`
	Category     string                 `json:"category"`
	UserImpact   bool                   `json:"userImpact"`
	Fields       map[string]interface{} `json:"fields,omitempty"`
	Raw          string                 `json:"raw,omitempty"`
}

type discoveredLogFile struct {
	path string
	info os.FileInfo
}

func NormalizeAppLogLimit(limit int) int {
	if limit <= 0 {
		return defaultAppLogLimit
	}
	if limit > maxAppLogLimit {
		return maxAppLogLimit
	}
	return limit
}

func GetRecentAppLogs(limit int) (AppLogResponse, error) {
	limit = NormalizeAppLogLimit(limit)
	files := discoverAppLogFiles()

	response := AppLogResponse{
		Limit:   limit,
		Sources: make([]AppLogSource, 0, len(files)),
	}

	entries := make([]AppLogEntry, 0, limit)
	for _, file := range files {
		response.Sources = append(response.Sources, AppLogSource{
			Name:      filepath.Base(file.path),
			UpdatedAt: file.info.ModTime().UTC(),
			SizeBytes: file.info.Size(),
		})

		fileEntries, err := parseAppLogFile(file.path)
		if err != nil {
			response.ReadErrors = append(response.ReadErrors, fmt.Sprintf("%s: %s", filepath.Base(file.path), err.Error()))
			continue
		}
		entries = append(entries, fileEntries...)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Timestamp.Equal(entries[j].Timestamp) {
			return entries[i].Source > entries[j].Source
		}
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	response.TotalDiscovered = len(entries)
	response.ImpactEntries = latestImpactEntries(entries)
	if len(entries) > limit {
		entries = entries[:limit]
	}
	response.Entries = entries
	return response, nil
}

func latestImpactEntries(entries []AppLogEntry) []AppLogEntry {
	impacts := make([]AppLogEntry, 0, min(len(entries), maxAppLogImpactEntries))
	for _, entry := range entries {
		if !entry.UserImpact {
			continue
		}
		impacts = append(impacts, entry)
		if len(impacts) >= maxAppLogImpactEntries {
			break
		}
	}
	return impacts
}

func discoverAppLogFiles() []discoveredLogFile {
	candidates := make(map[string]struct{})
	for _, path := range logging.ConfiguredLogFilePaths() {
		candidates[path] = struct{}{}
	}
	for _, pattern := range logging.ConfiguredLogFileGlobs() {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, match := range matches {
			candidates[match] = struct{}{}
		}
	}
	for _, path := range fallbackAppLogFiles() {
		candidates[path] = struct{}{}
	}

	files := make([]discoveredLogFile, 0, len(candidates))
	for candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || !info.Mode().IsRegular() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(candidate), ".log") {
			continue
		}
		files = append(files, discoveredLogFile{path: candidate, info: info})
	}

	sort.SliceStable(files, func(i, j int) bool {
		return files[i].info.ModTime().After(files[j].info.ModTime())
	})
	if len(files) > maxAppLogFiles {
		files = files[:maxAppLogFiles]
	}
	return files
}

func fallbackAppLogFiles() []string {
	roots := []string{appLogFallbackDirectory, "/logs"}
	seenRoots := make(map[string]struct{}, len(roots))
	files := make([]string, 0)

	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		cleanRoot := filepath.Clean(root)
		if _, exists := seenRoots[cleanRoot]; exists {
			continue
		}
		seenRoots[cleanRoot] = struct{}{}

		info, err := os.Stat(cleanRoot)
		if err != nil || !info.IsDir() {
			continue
		}
		_ = filepath.WalkDir(cleanRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(path), ".log") {
				files = append(files, path)
			}
			return nil
		})
	}
	return files
}

func parseAppLogFile(path string) ([]AppLogEntry, error) {
	raw, err := readFileTail(path, maxAppLogTailBytes)
	if err != nil {
		return nil, err
	}

	source := filepath.Base(path)
	entries := make([]AppLogEntry, 0)
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), maxAppLogTailBytes)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry, ok := parseAppLogLine(line, source)
		if ok {
			entries = append(entries, entry)
			continue
		}
		if len(entries) > 0 {
			last := &entries[len(entries)-1]
			last.Raw = trimLogString(last.Raw+"\n"+redactSensitiveString(line), maxAppLogRawLength)
		}
	}
	if err := scanner.Err(); err != nil {
		return entries, err
	}
	return entries, nil
}

func readFileTail(path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size <= maxBytes {
		return io.ReadAll(file)
	}

	if _, err := file.Seek(-maxBytes, io.SeekEnd); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if idx := bytes.IndexByte(raw, '\n'); idx >= 0 && idx+1 < len(raw) {
		raw = raw[idx+1:]
	}
	return raw, nil
}

func parseAppLogLine(line string, source string) (AppLogEntry, bool) {
	if entry, ok := parseJSONAppLogLine(line, source); ok {
		return entry, true
	}
	if entry, ok := parseZapConsoleLogLine(line, source); ok {
		return entry, true
	}
	return parsePythonTextLogLine(line, source)
}

func parseJSONAppLogLine(line string, source string) (AppLogEntry, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "{") {
		return AppLogEntry{}, false
	}

	var payload map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return AppLogEntry{}, false
	}

	timestamp, ok := parseLogTimestamp(stringField(payload, "ts"))
	if !ok {
		timestamp, ok = parseLogTimestamp(stringField(payload, "time"))
	}
	if !ok {
		return AppLogEntry{}, false
	}

	message := firstNonEmpty(stringField(payload, "msg"), stringField(payload, "message"))
	level := normalizeLogLevel(stringField(payload, "level"))
	fields := sanitizedLogFields(payload, "ts", "time", "level", "msg", "message", "caller", "logger", "service")
	entry := AppLogEntry{
		Timestamp: timestamp,
		Level:     level,
		Source:    source,
		Service:   trimLogString(stringField(payload, "service"), maxAppLogStringLength),
		Caller:    trimLogString(firstNonEmpty(stringField(payload, "caller"), stringField(payload, "logger")), maxAppLogStringLength),
		Message:   trimLogString(redactSensitiveString(message), maxAppLogStringLength),
		Fields:    fields,
		Raw:       trimLogString(redactSensitiveString(line), maxAppLogRawLength),
	}
	finalizeAppLogEntry(&entry)
	return entry, true
}

func parseZapConsoleLogLine(line string, source string) (AppLogEntry, bool) {
	parts := strings.SplitN(line, "\t", 5)
	if len(parts) < 4 {
		return AppLogEntry{}, false
	}

	timestamp, ok := parseLogTimestamp(parts[0])
	if !ok {
		return AppLogEntry{}, false
	}

	fields := map[string]interface{}{}
	if len(parts) == 5 {
		fields = parseFieldsJSON(parts[4])
	}
	entry := AppLogEntry{
		Timestamp: timestamp,
		Level:     normalizeLogLevel(parts[1]),
		Source:    source,
		Caller:    trimLogString(parts[2], maxAppLogStringLength),
		Message:   trimLogString(redactSensitiveString(parts[3]), maxAppLogStringLength),
		Fields:    fields,
		Raw:       trimLogString(redactSensitiveString(line), maxAppLogRawLength),
	}
	if service, ok := fields["service"].(string); ok {
		entry.Service = trimLogString(service, maxAppLogStringLength)
	}
	finalizeAppLogEntry(&entry)
	return entry, true
}

func parsePythonTextLogLine(line string, source string) (AppLogEntry, bool) {
	parts := strings.SplitN(line, " ", 5)
	if len(parts) < 5 {
		return AppLogEntry{}, false
	}
	timestamp, ok := parseLogTimestamp(parts[0])
	if !ok {
		return AppLogEntry{}, false
	}

	message, fields := parsePythonTextMessage(parts[4])
	entry := AppLogEntry{
		Timestamp: timestamp,
		Level:     normalizeLogLevel(parts[1]),
		Source:    source,
		Service:   trimLogString(parts[2], maxAppLogStringLength),
		Caller:    trimLogString(parts[3], maxAppLogStringLength),
		Message:   trimLogString(redactSensitiveString(message), maxAppLogStringLength),
		Fields:    fields,
		Raw:       trimLogString(redactSensitiveString(line), maxAppLogRawLength),
	}
	finalizeAppLogEntry(&entry)
	return entry, true
}

func parsePythonTextMessage(raw string) (string, map[string]interface{}) {
	message := raw
	fields := map[string]interface{}{}

	if idx := strings.Index(message, " exception="); idx >= 0 {
		fields["exception"] = redactLogValue(message[idx+len(" exception="):])
		message = strings.TrimSpace(message[:idx])
	}
	if idx := strings.Index(message, " context="); idx >= 0 {
		contextRaw := strings.TrimSpace(message[idx+len(" context="):])
		message = strings.TrimSpace(message[:idx])
		if parsed := parseFieldsJSON(contextRaw); len(parsed) > 0 {
			fields["context"] = parsed
		} else if contextRaw != "" {
			fields["context"] = redactLogValue(contextRaw)
		}
	}

	return message, fields
}

func parseFieldsJSON(raw string) map[string]interface{} {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return map[string]interface{}{}
	}
	var payload map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return map[string]interface{}{}
	}
	return sanitizedLogFields(payload)
}

func sanitizedLogFields(payload map[string]interface{}, omit ...string) map[string]interface{} {
	omitSet := make(map[string]struct{}, len(omit))
	for _, key := range omit {
		omitSet[key] = struct{}{}
	}
	fields := make(map[string]interface{}, len(payload))
	for key, value := range payload {
		if _, skip := omitSet[key]; skip {
			continue
		}
		fields[key] = redactLogValueForKey(key, value)
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func redactLogValueForKey(key string, value interface{}) interface{} {
	if isSensitiveLogKey(key) {
		return "***REDACTED***"
	}
	return redactLogValue(value)
}

func redactLogValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return sanitizedLogFields(typed)
	case []interface{}:
		values := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			values = append(values, redactLogValue(item))
		}
		return values
	case json.Number:
		if i, err := typed.Int64(); err == nil {
			return i
		}
		if f, err := typed.Float64(); err == nil {
			return f
		}
		return typed.String()
	case string:
		return trimLogString(redactSensitiveString(typed), maxAppLogStringLength)
	default:
		return typed
	}
}

func isSensitiveLogKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)
	for _, marker := range []string{"api_key", "apikey", "auth", "authorization", "cookie", "hf_token", "password", "passwd", "refresh_token", "secret", "set_cookie", "token"} {
		if normalized == marker || strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func redactSensitiveString(value string) string {
	redacted := value
	for _, pattern := range sensitiveLogPatterns {
		redacted = pattern.ReplaceAllString(redacted, `${1}***REDACTED***`)
	}
	return redacted
}

func parseLogTimestamp(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z0700",
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z0700",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func normalizeLogLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return "debug"
	case "warn", "warning":
		return "warn"
	case "error":
		return "error"
	case "fatal", "panic", "dpanic":
		return "fatal"
	default:
		return "info"
	}
}

func finalizeAppLogEntry(entry *AppLogEntry) {
	entry.Category = classifyAppLogCategory(*entry)
	entry.UserImpact = isUserImpactingAppLog(*entry)
	entry.HumanMessage = humanizeAppLogMessage(*entry)
	entry.ID = appLogEntryID(*entry)
}

func classifyAppLogCategory(entry AppLogEntry) string {
	combined := strings.ToLower(entry.Message + " " + entry.Caller + " " + entry.Service + " " + fieldsText(entry.Fields))
	switch {
	case strings.Contains(combined, "summary") || strings.Contains(combined, "summariz") || strings.Contains(combined, "llm"):
		return "Summary"
	case strings.Contains(combined, "transcript") || strings.Contains(combined, "whisperx"):
		return "Transcript"
	case strings.Contains(combined, "download") || strings.Contains(combined, "audio file"):
		return "Download"
	case strings.Contains(combined, "feed") || strings.Contains(combined, "podcast"):
		return "Feed"
	case strings.Contains(combined, "backup") || strings.Contains(combined, "export"):
		return "Export"
	case strings.Contains(combined, "http_request"):
		return "HTTP"
	case strings.Contains(combined, "database") || strings.Contains(combined, "migration"):
		return "System"
	default:
		return "App"
	}
}

func isUserImpactingAppLog(entry AppLogEntry) bool {
	if entry.Level != "warn" && entry.Level != "error" && entry.Level != "fatal" {
		return false
	}
	combined := strings.ToLower(entry.Message + " " + fieldsText(entry.Fields))
	if strings.Contains(combined, "failed") ||
		strings.Contains(combined, "failure") ||
		strings.Contains(combined, "error") ||
		strings.Contains(combined, "corrupt") ||
		strings.Contains(combined, "unreadable") ||
		strings.Contains(combined, "not found") ||
		strings.Contains(combined, "incomplete") ||
		strings.Contains(combined, "timed out") {
		switch entry.Category {
		case "Summary", "Transcript", "Download", "Export":
			return true
		}
	}
	return false
}

func humanizeAppLogMessage(entry AppLogEntry) string {
	errorText := stringField(entry.Fields, "error")
	if errorText == "" {
		errorText = stringField(entry.Fields, "exception")
	}

	switch entry.Category {
	case "Download":
		episode := firstNonEmpty(stringField(entry.Fields, "episode"), stringField(entry.Fields, "title"))
		podcast := stringField(entry.Fields, "podcast")
		if episode != "" && podcast != "" {
			return appendErrorText(fmt.Sprintf("Download failed for %q from %q", episode, podcast), errorText)
		}
		if episode != "" {
			return appendErrorText(fmt.Sprintf("Download failed for %q", episode), errorText)
		}
	case "Summary":
		title := firstNonEmpty(stringField(entry.Fields, "title"), stringField(entry.Fields, "episode"))
		if title != "" {
			return appendErrorText(fmt.Sprintf("Summary failed for %q", title), errorText)
		}
	case "Transcript":
		title := firstNonEmpty(stringField(entry.Fields, "episode"), stringField(entry.Fields, "title"))
		if title != "" {
			return appendErrorText(fmt.Sprintf("Transcript failed for %q", title), errorText)
		}
	}

	base := strings.TrimSpace(strings.ReplaceAll(entry.Message, "_", " "))
	if base == "" {
		base = "Log entry"
	}
	base = strings.ToUpper(base[:1]) + base[1:]
	return appendErrorText(base, errorText)
}

func appendErrorText(base string, errorText string) string {
	errorText = strings.TrimSpace(redactSensitiveString(errorText))
	if errorText == "" {
		return base
	}
	return trimLogString(base+": "+errorText, maxAppLogStringLength)
}

func appLogEntryID(entry AppLogEntry) string {
	hash := sha1.Sum([]byte(entry.Timestamp.Format(time.RFC3339Nano) + entry.Source + entry.Level + entry.Message + entry.Raw))
	return hex.EncodeToString(hash[:10])
}

func fieldsText(fields map[string]interface{}) string {
	if len(fields) == 0 {
		return ""
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return ""
	}
	return string(raw)
}

func stringField(fields map[string]interface{}, key string) string {
	if len(fields) == 0 {
		return ""
	}
	value, ok := fields[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case json.Number:
		return strings.TrimSpace(typed.String())
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		return strings.TrimSpace(fmt.Sprint(typed))
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(raw))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func trimLogString(value string, maxLength int) string {
	value = strings.TrimSpace(value)
	if maxLength <= 0 || len(value) <= maxLength {
		return value
	}
	if maxLength <= len("...(truncated)") {
		return value[:maxLength]
	}
	return value[:maxLength-len("...(truncated)")] + "...(truncated)"
}

func ParseAppLogLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultAppLogLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if limit <= 0 || limit > maxAppLogLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", maxAppLogLimit)
	}
	return limit, nil
}
