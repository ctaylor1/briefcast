package service

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func setupLLMHealthTest(t *testing.T, provider string, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	setupRetentionTestDB(t)
	resetLLMHealthCacheForTests()
	t.Cleanup(resetLLMHealthCacheForTests)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "health-secret-key")
	t.Setenv("LLM_PROVIDER", provider)
	t.Setenv("LLM_BASE_URL", server.URL+"/v1")
	t.Setenv("LLM_MODEL", "qwen3.6:27b")
	return server
}

func TestCheckLLMServerReportsOllamaModelAndGPUShare(t *testing.T) {
	var authorized atomic.Bool
	setupLLMHealthTest(t, "ollama", func(w http.ResponseWriter, r *http.Request) {
		authorized.Store(r.Header.Get("Authorization") == "Bearer health-secret-key")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"llama3"},{"id":"qwen3.6:27b"}]}`))
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3.6:27b","model":"qwen3.6:27b","size":19625169383,"size_vram":15771159755,"context_length":65536}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	got, checked := CheckLLMServer(context.Background())
	if !checked || !got.Reachable || got.ModelListed == nil || !*got.ModelListed || got.ErrorClass != "" || !authorized.Load() {
		t.Fatalf("unexpected health: %+v", got)
	}
	if got.Ollama == nil || !got.Ollama.Loaded || got.Ollama.ContextLength != 65536 || got.Ollama.GPUShare < 0.80 || got.Ollama.GPUShare > 0.81 {
		t.Fatalf("unexpected Ollama state: %+v", got.Ollama)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "health-secret-key") || strings.Contains(string(raw), "/v1") || strings.Contains(string(raw), "http://") {
		t.Fatalf("health exposed the key or the URL: %s", raw)
	}
}

func TestCheckLLMServerClassifiesProblems(t *testing.T) {
	for _, tc := range []struct {
		name       string
		handler    http.HandlerFunc
		wantClass  string
		wantListed *bool
	}{
		{name: "model missing", handler: func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"data":[{"id":"llama3"}]}`))
		}, wantListed: new(bool)},
		{name: "unauthorized", handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}, wantClass: LLMHealthErrorUnauthorized},
		{name: "not found", handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}, wantClass: LLMHealthErrorNotFound},
		{name: "not JSON", handler: func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`<html>`))
		}, wantClass: LLMHealthErrorInvalidResponse},
		{name: "timeout", handler: func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
		}, wantClass: LLMHealthErrorTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupLLMHealthTest(t, "openai", tc.handler)
			got, checked := CheckLLMServer(context.Background())
			if !checked || got.ErrorClass != tc.wantClass {
				t.Fatalf("class %q, want %q: %+v", got.ErrorClass, tc.wantClass, got)
			}
			if tc.wantListed != nil && (got.ModelListed == nil || *got.ModelListed != *tc.wantListed) {
				t.Fatalf("model listed %v, want %v", got.ModelListed, *tc.wantListed)
			}
			if got.Ollama != nil {
				t.Fatal("a non-Ollama provider must not call /api/ps")
			}
			if strings.Contains(got.Error, "http://") || strings.Contains(got.Error, "health-secret-key") {
				t.Fatalf("error exposed the URL or key: %q", got.Error)
			}
		})
	}
}

func TestCheckLLMServerRefusedConnection(t *testing.T) {
	setupRetentionTestDB(t)
	resetLLMHealthCacheForTests()
	t.Cleanup(resetLLMHealthCacheForTests)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "k")
	t.Setenv("LLM_BASE_URL", "http://"+addr+"/v1")
	got, _ := CheckLLMServer(context.Background())
	if got.Reachable || got.ErrorClass != LLMHealthErrorRefused {
		t.Fatalf("expected refused, got %+v", got)
	}
}

func TestCheckLLMServerCachesForAMinuteAndSkipsWhenUnconfigured(t *testing.T) {
	var calls atomic.Int32
	setupLLMHealthTest(t, "openai", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen3.6:27b"}]}`))
	})
	for i := 0; i < 3; i++ {
		if _, checked := CheckLLMServer(context.Background()); !checked {
			t.Fatal("expected a check")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one request in the cache window, got %d", calls.Load())
	}
	t.Setenv("LLM_ENABLED", "false")
	if _, checked := CheckLLMServer(context.Background()); checked || calls.Load() != 1 {
		t.Fatalf("an unconfigured LLM must not be checked (calls %d)", calls.Load())
	}
}

func TestCheckLLMServerHonoursThePrivateHostPolicy(t *testing.T) {
	var calls atomic.Int32
	setupLLMHealthTest(t, "openai", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	t.Setenv(outboundAllowPrivateLLMEnv, "false")
	got, _ := CheckLLMServer(context.Background())
	if got.ErrorClass != LLMHealthErrorInvalidURL || calls.Load() != 0 {
		t.Fatalf("a private LLM host must be refused by policy: %+v (calls %d)", got, calls.Load())
	}
}
