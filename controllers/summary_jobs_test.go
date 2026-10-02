package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

func summaryJobTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/settings/backfill-summaries", BackfillSummaries)
	router.GET("/settings/backfill-summaries", GetBackfillSummariesStatus)
	router.POST("/settings/resummarize", ResummarizeSummaries)
	return router
}

func summaryJobRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func TestSummaryJobsRejectMissingLLMConfiguration(t *testing.T) {
	setupControllersTestDB(t)
	router := summaryJobTestRouter()

	for _, tc := range []struct {
		name       string
		enabled    string
		key        string
		wantErrMsg string
	}{
		{name: "disabled", enabled: "false", key: "test-key", wantErrMsg: "LLM summarization is not enabled"},
		{name: "missing key", enabled: "true", key: "", wantErrMsg: "LLM_API_KEY is not configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LLM_ENABLED", tc.enabled)
			t.Setenv("LLM_API_KEY", tc.key)
			for _, endpoint := range []struct {
				path string
				body string
			}{
				{path: "/settings/backfill-summaries"},
				{path: "/settings/resummarize", body: `{}`},
			} {
				resp := summaryJobRequest(router, http.MethodPost, endpoint.path, endpoint.body)
				if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), tc.wantErrMsg) {
					t.Fatalf("POST %s: expected 400 with %q, got %d %s", endpoint.path, tc.wantErrMsg, resp.Code, resp.Body.String())
				}
			}
		})
	}
}

func TestSummaryJobStatusReportsCompletedBackfill(t *testing.T) {
	setupControllersTestDB(t)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	router := summaryJobTestRouter()

	resp := summaryJobRequest(router, http.MethodPost, "/settings/backfill-summaries", "")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d %s", resp.Code, resp.Body.String())
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp = summaryJobRequest(router, http.MethodGet, "/settings/backfill-summaries", "")
		var status service.SummaryBackfillStatus
		if err := json.Unmarshal(resp.Body.Bytes(), &status); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		if resp.Code == http.StatusOK && !status.Running {
			if status.Total != 0 || status.Completed != 0 || status.Failed != 0 || status.Error != "" {
				t.Fatalf("unexpected completed empty backfill status: %+v", status)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for backfill completion")
}

func TestSummaryJobRejectsConcurrentStartAndReportsFailures(t *testing.T) {
	setupControllersTestDB(t)
	_, item := createControllerPodcastAndItem(t)
	item.CanonicalTranscript = "some podcast content"
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("save transcript: %v", err)
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		started <- struct{}{}
		<-release
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer llm.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", llm.URL)
	router := summaryJobTestRouter()

	resp := summaryJobRequest(router, http.MethodPost, "/settings/backfill-summaries", "")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d %s", resp.Code, resp.Body.String())
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for LLM request")
	}
	resp = summaryJobRequest(router, http.MethodPost, "/settings/resummarize", `{}`)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409 for concurrent summary job, got %d %s", resp.Code, resp.Body.String())
	}
	close(release)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp = summaryJobRequest(router, http.MethodGet, "/settings/backfill-summaries", "")
		var status service.SummaryBackfillStatus
		if err := json.Unmarshal(resp.Body.Bytes(), &status); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		if !status.Running {
			if status.Total != 1 || status.Completed != 1 || status.Failed != 1 {
				t.Fatalf("expected one failed summary in final status, got %+v", status)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for failed backfill completion")
}
