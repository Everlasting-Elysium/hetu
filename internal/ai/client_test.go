package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// newFakeSidecar serves the full AI contract with canned successful bodies. It
// mirrors ai/server.py's surface so the Go client is exercised end-to-end.
func newFakeSidecar(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, Health{OK: true})
	})
	mux.HandleFunc("POST /tag", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, TagResult{
			Tags:    []Tag{{Name: "cat", Confidence: 0.98}},
			Caption: "a cat on a mat",
			Model:   "stub",
		})
	})
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, EmbedResult{Vector: []float32{0.1, 0.2, 0.3}, Dim: 3, Model: "stub"})
	})
	mux.HandleFunc("POST /caption", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, CaptionResult{Caption: "a photo", Model: "stub"})
	})
	mux.HandleFunc("POST /ocr", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, OCRResult{Text: "hello", Model: "stub"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testClient(baseURL string) *Client {
	return New(Config{
		BaseURL: baseURL,
		Timeout: 2 * time.Second,
		Retry:   RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond},
		Logger:  discardLogger(),
	})
}

func TestClient_Health(t *testing.T) {
	c := testClient(newFakeSidecar(t).URL)
	h, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !h.OK {
		t.Error("expected OK=true")
	}
}

func TestClient_Tag(t *testing.T) {
	c := testClient(newFakeSidecar(t).URL)
	res, err := c.Tag(context.Background(), AssetRef{Ref: "photo.jpg"})
	if err != nil {
		t.Fatalf("Tag: %v", err)
	}
	if len(res.Tags) != 1 || res.Tags[0].Name != "cat" || res.Tags[0].Confidence != 0.98 {
		t.Errorf("tags = %+v", res.Tags)
	}
	if res.Caption != "a cat on a mat" {
		t.Errorf("caption = %q", res.Caption)
	}
}

func TestClient_Embed(t *testing.T) {
	c := testClient(newFakeSidecar(t).URL)
	res, err := c.Embed(context.Background(), AssetRef{Ref: "photo.jpg"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if res.Dim != 3 || len(res.Vector) != 3 {
		t.Errorf("embed = %+v", res)
	}
}

func TestClient_Caption(t *testing.T) {
	c := testClient(newFakeSidecar(t).URL)
	res, err := c.Caption(context.Background(), AssetRef{Ref: "photo.jpg"})
	if err != nil {
		t.Fatalf("Caption: %v", err)
	}
	if res.Caption != "a photo" {
		t.Errorf("caption = %q", res.Caption)
	}
}

func TestClient_OCR(t *testing.T) {
	c := testClient(newFakeSidecar(t).URL)
	res, err := c.OCR(context.Background(), AssetRef{Ref: "scan.png"})
	if err != nil {
		t.Fatalf("OCR: %v", err)
	}
	if res.Text != "hello" {
		t.Errorf("text = %q", res.Text)
	}
}

func TestClient_SendsContractVersion(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(HeaderContractVersion)
		writeJSON(w, http.StatusOK, Health{OK: true})
	}))
	t.Cleanup(srv.Close)
	if _, err := testClient(srv.URL).Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if got != ContractVersion {
		t.Errorf("contract header = %q, want %q", got, ContractVersion)
	}
}

// TestClient_NotImplemented reproduces the Phase 1 stub: /tag returns 501. The
// client must surface ErrNotImplemented as a terminal KindInvalid error so the
// orchestrator can skip gracefully.
func TestClient_NotImplemented(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"status": "not_implemented"})
	}))
	t.Cleanup(srv.Close)

	_, err := testClient(srv.URL).Tag(context.Background(), AssetRef{Ref: "photo.jpg"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
	var aerr *Error
	if !errors.As(err, &aerr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if aerr.Kind != KindInvalid || aerr.Status != http.StatusNotImplemented {
		t.Errorf("error = %+v, want KindInvalid status 501", aerr)
	}
	if aerr.Retryable() {
		t.Error("501 must not be retryable")
	}
}

func TestClient_ErrorClassification(t *testing.T) {
	cases := []struct {
		status    int
		kind      Kind
		retryable bool
	}{
		{http.StatusUnauthorized, KindAuth, false},
		{http.StatusForbidden, KindAuth, false},
		{http.StatusPaymentRequired, KindQuota, false},
		{http.StatusTooManyRequests, KindRateLimit, true},
		{http.StatusBadRequest, KindInvalid, false},
		{http.StatusUnprocessableEntity, KindInvalid, false},
		{http.StatusRequestTimeout, KindTimeout, true},
		{http.StatusGatewayTimeout, KindTimeout, true},
		{http.StatusInternalServerError, KindNetwork, true},
		{http.StatusBadGateway, KindNetwork, true},
		{http.StatusServiceUnavailable, KindNetwork, true},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(srv.Close)
			// One attempt only: classification does not depend on retrying.
			c := New(Config{
				BaseURL: srv.URL,
				Retry:   RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
				Logger:  discardLogger(),
			})
			_, err := c.Tag(context.Background(), AssetRef{Ref: "photo.jpg"})
			var aerr *Error
			if !errors.As(err, &aerr) {
				t.Fatalf("expected *Error, got %T: %v", err, err)
			}
			if aerr.Kind != tc.kind {
				t.Errorf("kind = %q, want %q", aerr.Kind, tc.kind)
			}
			if aerr.Retryable() != tc.retryable {
				t.Errorf("retryable = %v, want %v", aerr.Retryable(), tc.retryable)
			}
			if aerr.Status != tc.status {
				t.Errorf("status = %d, want %d", aerr.Status, tc.status)
			}
		})
	}
}

func TestClient_RetriesThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, TagResult{Caption: "ok"})
	}))
	t.Cleanup(srv.Close)

	res, err := testClient(srv.URL).Tag(context.Background(), AssetRef{Ref: "photo.jpg"})
	if err != nil {
		t.Fatalf("Tag: %v", err)
	}
	if res.Caption != "ok" {
		t.Errorf("caption = %q", res.Caption)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestClient_RetriesExhausted(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	_, err := testClient(srv.URL).Tag(context.Background(), AssetRef{Ref: "photo.jpg"})
	var aerr *Error
	if !errors.As(err, &aerr) || aerr.Kind != KindNetwork {
		t.Fatalf("expected network *Error, got %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("calls = %d, want 3 (MaxAttempts)", got)
	}
}

func TestClient_TerminalNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	_, err := testClient(srv.URL).Tag(context.Background(), AssetRef{Ref: "photo.jpg"})
	var aerr *Error
	if !errors.As(err, &aerr) || aerr.Kind != KindInvalid {
		t.Fatalf("expected invalid *Error, got %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1 (terminal must not retry)", got)
	}
}

func TestClient_TransportErrorIsNetwork(t *testing.T) {
	// Reserved port 127.0.0.1:1 refuses connections quickly.
	c := New(Config{
		BaseURL: "http://127.0.0.1:1",
		Retry:   RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		Logger:  discardLogger(),
	})
	_, err := c.Health(context.Background())
	var aerr *Error
	if !errors.As(err, &aerr) || aerr.Kind != KindNetwork {
		t.Fatalf("expected network *Error, got %v", err)
	}
	if aerr.Status != 0 {
		t.Errorf("transport error status = %d, want 0", aerr.Status)
	}
}

func TestClient_ContextCanceledIsTerminal(t *testing.T) {
	c := testClient(newFakeSidecar(t).URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Health(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := map[string]time.Duration{
		"":   0,
		"2":  2 * time.Second,
		"-1": 0,
		"x":  0,
		"0":  0,
		"30": 30 * time.Second,
	}
	for in, want := range cases {
		h := http.Header{}
		if in != "" {
			h.Set("Retry-After", in)
		}
		if got := parseRetryAfter(h); got != want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestBackoff(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 5, BaseDelay: 100 * time.Millisecond, MaxDelay: time.Second}
	// A server Retry-After wins and is capped at MaxDelay.
	if d := p.backoff(1, 5*time.Second); d != time.Second {
		t.Errorf("retry-after cap = %v, want %v", d, time.Second)
	}
	if d := p.backoff(1, 200*time.Millisecond); d != 200*time.Millisecond {
		t.Errorf("retry-after = %v, want 200ms", d)
	}
	// Exponential backoff with full jitter stays within [full/2, full].
	for attempt := 1; attempt <= 4; attempt++ {
		full := p.BaseDelay << (attempt - 1)
		if full > p.MaxDelay {
			full = p.MaxDelay
		}
		d := p.backoff(attempt, 0)
		if d < full/2 || d > full {
			t.Errorf("backoff(%d) = %v, want within [%v, %v]", attempt, d, full/2, full)
		}
	}
}

func TestRetryPolicyNormalized(t *testing.T) {
	p := RetryPolicy{}.normalized()
	if p.MaxAttempts != 1 || p.BaseDelay <= 0 || p.MaxDelay <= 0 {
		t.Errorf("normalized zero policy = %+v", p)
	}
}

func TestNewUsesDefaultRetryWhenZero(t *testing.T) {
	c := New(Config{BaseURL: "http://localhost:8091"})
	if c.retry != DefaultRetryPolicy() {
		t.Errorf("retry = %+v, want default %+v", c.retry, DefaultRetryPolicy())
	}
}
