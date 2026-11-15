package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

const (
	llmHealthTimeout  = 5 * time.Second
	llmHealthCacheTTL = 60 * time.Second
)

// LLM server check error classes.
const (
	LLMHealthErrorInvalidURL      = "invalid_url"
	LLMHealthErrorTimeout         = "timeout"
	LLMHealthErrorRefused         = "refused"
	LLMHealthErrorUnreachable     = "unreachable"
	LLMHealthErrorUnauthorized    = "unauthorized"
	LLMHealthErrorNotFound        = "not_found"
	LLMHealthErrorHTTP            = "http_error"
	LLMHealthErrorInvalidResponse = "invalid_response"
)

// LLMServerHealth is the result of one check of the configured LLM server.
// It never includes the key or the full base URL.
type LLMServerHealth struct {
	CheckedAt time.Time `json:"checked_at"`
	Host      string    `json:"host,omitempty"`
	Reachable bool      `json:"reachable"`
	LatencyMS int64     `json:"latency_ms"`
	// ModelListed reports whether GET /models lists the summary model; nil
	// when the check does not apply (for example OpenRouter's routed models).
	ModelListed *bool  `json:"model_listed,omitempty"`
	ErrorClass  string `json:"error_class,omitempty"`
	Error       string `json:"error,omitempty"`
	// Ollama only, from GET /api/ps.
	Ollama *OllamaModelState `json:"ollama,omitempty"`
}

// OllamaModelState describes the summary model as Ollama has it loaded.
type OllamaModelState struct {
	Loaded        bool    `json:"loaded"`
	ContextLength int     `json:"context_length,omitempty"`
	SizeBytes     int64   `json:"size_bytes,omitempty"`
	VRAMBytes     int64   `json:"vram_bytes,omitempty"`
	GPUShare      float64 `json:"gpu_share,omitempty"` // VRAMBytes / SizeBytes, 1 = fully on the GPU
	Error         string  `json:"error,omitempty"`
}

var llmHealthCache struct {
	mu     sync.Mutex
	key    string
	result *LLMServerHealth
}

// CheckLLMServer checks the configured LLM server, reusing a result up to a
// minute old for the same server and model. It makes no request when
// summaries are not configured.
func CheckLLMServer(ctx context.Context) (LLMServerHealth, bool) {
	cfg := LoadLLMConfig()
	if !cfg.Enabled || strings.TrimSpace(cfg.APIKey) == "" {
		return LLMServerHealth{}, false
	}
	model := cfg.Model
	if setting := currentSetting(); setting != nil {
		model = ResolveSummarizationModel(setting, cfg)
	}
	key := cfg.Provider + "|" + cfg.BaseURL + "|" + model
	llmHealthCache.mu.Lock()
	defer llmHealthCache.mu.Unlock()
	if cached := llmHealthCache.result; cached != nil && llmHealthCache.key == key && time.Since(cached.CheckedAt) < llmHealthCacheTTL {
		return *cached, true
	}
	result := checkLLMServer(ctx, cfg, model)
	llmHealthCache.key, llmHealthCache.result = key, &result
	return result, true
}

func currentSetting() *db.Setting {
	if db.DB == nil {
		return nil
	}
	return db.GetOrCreateSetting()
}

func resetLLMHealthCacheForTests() {
	llmHealthCache.mu.Lock()
	defer llmHealthCache.mu.Unlock()
	llmHealthCache.key, llmHealthCache.result = "", nil
}

func checkLLMServer(ctx context.Context, cfg LLMConfig, model string) LLMServerHealth {
	out := LLMServerHealth{CheckedAt: time.Now().UTC()}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	parsed, err := validateOutboundURL(base+"/models", outboundPurposeLLM)
	if err != nil {
		out.ErrorClass, out.Error = LLMHealthErrorInvalidURL, "LLM base URL is not allowed or invalid"
		return out
	}
	out.Host = parsed.Host

	start := time.Now()
	var listed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	status, err := llmHealthGetJSON(ctx, cfg, parsed.String(), &listed)
	out.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		out.ErrorClass, out.Error = classifyLLMHealthError(status, err)
		return out
	}
	out.Reachable = true
	if !strings.EqualFold(cfg.Provider, "openrouter") {
		found := false
		for _, m := range listed.Data {
			if m.ID == model {
				found = true
				break
			}
		}
		out.ModelListed = &found
	}
	if strings.EqualFold(cfg.Provider, "ollama") {
		out.Ollama = checkOllamaModel(ctx, cfg, base, model)
	}
	return out
}

// checkOllamaModel reads Ollama's loaded models. A model partly in system
// memory runs part of each step on the CPU, which makes summaries slow.
func checkOllamaModel(ctx context.Context, cfg LLMConfig, base, model string) *OllamaModelState {
	state := &OllamaModelState{}
	root := strings.TrimSuffix(base, "/v1")
	parsed, err := validateOutboundURL(root+"/api/ps", outboundPurposeLLM)
	if err != nil {
		state.Error = "Ollama URL is not allowed or invalid"
		return state
	}
	var ps struct {
		Models []struct {
			Name          string `json:"name"`
			Model         string `json:"model"`
			Size          int64  `json:"size"`
			SizeVRAM      int64  `json:"size_vram"`
			ContextLength int    `json:"context_length"`
		} `json:"models"`
	}
	status, err := llmHealthGetJSON(ctx, cfg, parsed.String(), &ps)
	if err != nil {
		_, state.Error = classifyLLMHealthError(status, err)
		return state
	}
	for _, m := range ps.Models {
		if m.Name != model && m.Model != model {
			continue
		}
		state.Loaded = true
		state.ContextLength = m.ContextLength
		state.SizeBytes, state.VRAMBytes = m.Size, m.SizeVRAM
		if m.Size > 0 {
			state.GPUShare = float64(m.SizeVRAM) / float64(m.Size)
		}
		break
	}
	return state
}

func llmHealthGetJSON(ctx context.Context, cfg LLMConfig, target string, into any) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, llmHealthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	client := &http.Client{
		Transport:     outboundHTTPTransport(outboundPurposeLLM),
		CheckRedirect: checkOutboundRedirect(outboundPurposeLLM),
		Timeout:       llmHealthTimeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := readBoundedOutboundBody(resp.Body, resp.ContentLength)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, into); err != nil {
		return resp.StatusCode, errInvalidLLMHealthResponse
	}
	return resp.StatusCode, nil
}

var errInvalidLLMHealthResponse = errors.New("response is not the expected JSON")

// classifyLLMHealthError returns an error class and a short message that
// names no URL or key.
func classifyLLMHealthError(status int, err error) (string, string) {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return LLMHealthErrorUnauthorized, fmt.Sprintf("the server refused the key (HTTP %d)", status)
	case status == http.StatusNotFound:
		return LLMHealthErrorNotFound, "the server has no /models endpoint at this base URL (HTTP 404)"
	case status != 0 && errors.Is(err, errInvalidLLMHealthResponse):
		return LLMHealthErrorInvalidResponse, "the server's answer was not the expected JSON"
	case status != 0:
		return LLMHealthErrorHTTP, fmt.Sprintf("the server answered HTTP %d", status)
	case isTimeoutError(err):
		return LLMHealthErrorTimeout, fmt.Sprintf("no answer within %d s", int(llmHealthTimeout.Seconds()))
	case errors.Is(err, syscall.ECONNREFUSED):
		return LLMHealthErrorRefused, "connection refused: is the server running and listening on that port?"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return LLMHealthErrorUnreachable, "the host name did not resolve"
	}
	if strings.Contains(err.Error(), "outbound URL rejected") {
		return LLMHealthErrorInvalidURL, "the LLM address is not allowed by the outbound URL policy"
	}
	return LLMHealthErrorUnreachable, "the server could not be reached"
}
