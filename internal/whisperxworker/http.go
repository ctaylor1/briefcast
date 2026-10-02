package whisperxworker

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
)

func readToken(path string) ([32]byte, error) {
	var sum [32]byte
	raw, err := readBounded(path, 4096)
	if err != nil {
		return sum, errors.New("service token file is unavailable")
	}
	token := strings.TrimSpace(string(raw))
	if len(token) < 32 || strings.ContainsAny(token, "\r\n\t ") {
		return sum, errors.New("service token must contain at least 32 characters without whitespace")
	}
	return sha256.Sum256([]byte(token)), nil
}
func sendJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func sendError(w http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	problem := transcription.Error("STATE_UNAVAILABLE", "Worker state is unavailable", true)
	var typed *transcription.WorkerError
	if errors.Is(err, sql.ErrNoRows) {
		status = http.StatusNotFound
		problem = transcription.Error("INVALID_CONFIG", "Unknown job UUID", false)
	} else if errors.Is(err, errConflict) {
		status = http.StatusConflict
		problem = errConflict
	} else if errors.Is(err, errGone) {
		status = http.StatusGone
		problem = errGone
	} else if errors.As(err, &typed) {
		problem = typed
		switch typed.Code {
		case "INVALID_CONFIG", "INPUT_CHANGED", "INPUT_MISSING", "INPUT_UNREADABLE":
			status = http.StatusBadRequest
		case "CHECKPOINT_INCOMPATIBLE":
			status = http.StatusConflict
		case "BUSY":
			status = http.StatusTooManyRequests
			w.Header().Set("Retry-After", "2")
		}
	} else {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
			problem = transcription.Error("INVALID_CONFIG", "Control body exceeds limit", false)
		}
	}
	sendJSON(w, status, struct {
		Error *transcription.WorkerError `json:"error"`
	}{problem})
}
func (w *Worker) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(rw http.ResponseWriter, r *http.Request) { sendJSON(rw, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /health/ready", func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		err := w.healthLocked()
		counts, e := w.store.counts()
		w.mu.Unlock()
		if err != nil || e != nil || !counts.Accepting {
			sendJSON(rw, 503, map[string]string{"status": "unavailable"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if w.runner.Ready(ctx) != nil {
			sendJSON(rw, 503, map[string]string{"status": "unavailable"})
			return
		}
		sendJSON(rw, 200, map[string]string{"status": "ok"})
	})
	private := http.NewServeMux()
	private.HandleFunc("GET /v1/capabilities", func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		counts, err := w.store.counts()
		ready := w.fault == nil && !w.stopping
		w.mu.Unlock()
		if err != nil {
			sendError(rw, err)
			return
		}
		sendJSON(rw, 200, transcription.Capabilities{ContractVersions: []int{1}, WorkerInstanceID: w.store.instance, WorkerVersion: w.cfg.WorkerVersion, PipelineVersion: transcription.PipelineVersion, EngineProfile: w.cfg.EngineProfile, PackageVersions: w.cfg.PackageVersions, SupportedModels: w.cfg.Models, SupportedDiarizationModels: w.cfg.DiarizationModels, SupportedVADMethods: []string{"pyannote", "silero"}, CheckpointFormats: []string{transcription.CheckpointFormat}, Device: w.cfg.Device, ComputeType: w.cfg.ComputeType, BatchSize: w.cfg.BatchSize, ModelReady: ready, Capacity: transcription.Capacity{MaxRunning: w.cfg.MaxRunning, MaxWaiting: w.cfg.MaxWaiting, Running: counts.Running, Waiting: counts.Queued, Accepting: counts.Accepting && ready}})
	})
	private.HandleFunc("PUT /v1/jobs/{id}", func(rw http.ResponseWriter, r *http.Request) {
		body, err := readLimit(http.MaxBytesReader(rw, r.Body, transcription.ControlLimit), transcription.ControlLimit)
		if err != nil {
			sendError(rw, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		status, created, err := w.Submit(ctx, r.PathValue("id"), body)
		if err != nil {
			sendError(rw, err)
			return
		}
		code := 200
		if created {
			code = 201
		}
		sendJSON(rw, code, status)
	})
	private.HandleFunc("GET /v1/jobs/{id}", func(rw http.ResponseWriter, r *http.Request) {
		status, err := w.Status(r.PathValue("id"))
		if err != nil {
			sendError(rw, err)
			return
		}
		sendJSON(rw, 200, status)
	})
	private.HandleFunc("GET /v1/jobs/{id}/result", func(rw http.ResponseWriter, r *http.Request) {
		raw, err := w.Result(r.PathValue("id"))
		if err != nil {
			sendError(rw, err)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("Cache-Control", "no-store")
		_, _ = rw.Write(raw)
	})
	private.HandleFunc("POST /v1/jobs/{id}/cancel", func(rw http.ResponseWriter, r *http.Request) {
		status, err := w.Cancel(r.PathValue("id"))
		if err != nil {
			sendError(rw, err)
			return
		}
		sendJSON(rw, 200, status)
	})
	private.HandleFunc("POST /v1/jobs/{id}/ack", func(rw http.ResponseWriter, r *http.Request) {
		raw, err := readLimit(http.MaxBytesReader(rw, r.Body, transcription.ControlLimit), transcription.ControlLimit)
		if err != nil {
			sendError(rw, err)
			return
		}
		var body transcription.AckRequest
		if _, err = transcription.DecodeStrict(raw, &body); err != nil || !transcription.ValidSHA256(body.ResultSHA256) {
			sendError(rw, transcription.Error("INVALID_CONFIG", "Invalid result receipt", false))
			return
		}
		ack, err := w.Ack(r.PathValue("id"), body.ResultSHA256)
		if err != nil {
			sendError(rw, err)
			return
		}
		sendJSON(rw, 200, ack)
	})
	for _, op := range []string{"drain", "resume"} {
		private.HandleFunc("POST /v1/admin/"+op, func(rw http.ResponseWriter, r *http.Request) {
			accept := op == "resume"
			counts, err := w.Maintenance(&accept)
			if err != nil {
				sendError(rw, err)
				return
			}
			sendJSON(rw, 200, counts)
		})
	}
	mux.Handle("/v1/", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token := ""
		if strings.HasPrefix(header, "Bearer ") {
			token = strings.TrimPrefix(header, "Bearer ")
		}
		sum := sha256.Sum256([]byte(token))
		if subtle.ConstantTimeCompare(sum[:], w.tokenDigest[:]) != 1 {
			rw.Header().Set("WWW-Authenticate", "Bearer")
			sendJSON(rw, 401, map[string]any{"error": transcription.Error("INVALID_CONFIG", "Unauthorized", false)})
			return
		}
		private.ServeHTTP(rw, r)
	}))
	return mux
}
