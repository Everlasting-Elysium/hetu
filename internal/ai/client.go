package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	defaultTimeout  = 30 * time.Second
	maxErrBodyBytes = 4 << 10 // cap when reading an error body for its message
)

// Config configures a [Client]. Only BaseURL is required; zero fields take
// sensible defaults.
type Config struct {
	BaseURL string        // sidecar base URL, e.g. "http://localhost:8091"
	Timeout time.Duration // per-request total timeout (default 30s)
	Retry   RetryPolicy   // retry policy for retryable failures
	Logger  *slog.Logger  // structured logger (default slog.Default())
}

// Client is a Go client for the hetu AI sidecar. It is safe for concurrent use.
type Client struct {
	baseURL string
	http    *http.Client
	retry   RetryPolicy
	log     *slog.Logger
}

// request describes one contract call: the operation label used in logs and
// errors, the HTTP method, the path under BaseURL, and an optional body to
// JSON-encode.
type request struct {
	op     string
	method string
	path   string
	body   any
}

// New returns a Client for the sidecar described by cfg.
func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	// A fully-zero policy means "use defaults"; a partial one is filled in.
	retry := DefaultRetryPolicy()
	if cfg.Retry != (RetryPolicy{}) {
		retry = cfg.Retry.normalized()
	}
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		retry: retry,
		log:   log,
	}
}

// Health reports whether the sidecar is ready (GET /health).
func (c *Client) Health(ctx context.Context) (Health, error) {
	var out Health
	err := c.do(ctx, request{op: "health", method: http.MethodGet, path: "/health"}, &out)
	return out, err
}

// Embed returns a CLIP embedding for ref (POST /embed).
func (c *Client) Embed(ctx context.Context, ref AssetRef) (EmbedResult, error) {
	var out EmbedResult
	err := c.do(ctx, request{op: "embed", method: http.MethodPost, path: "/embed", body: ref}, &out)
	return out, err
}

// Tag returns auto-tags and a caption for ref (POST /tag).
func (c *Client) Tag(ctx context.Context, ref AssetRef) (TagResult, error) {
	var out TagResult
	err := c.do(ctx, request{op: "tag", method: http.MethodPost, path: "/tag", body: ref}, &out)
	return out, err
}

// Caption returns a natural-language description for ref (POST /caption).
func (c *Client) Caption(ctx context.Context, ref AssetRef) (CaptionResult, error) {
	var out CaptionResult
	err := c.do(ctx, request{op: "caption", method: http.MethodPost, path: "/caption", body: ref}, &out)
	return out, err
}

// OCR extracts text from ref (POST /ocr).
func (c *Client) OCR(ctx context.Context, ref AssetRef) (OCRResult, error) {
	var out OCRResult
	err := c.do(ctx, request{op: "ocr", method: http.MethodPost, path: "/ocr", body: ref}, &out)
	return out, err
}

// do executes r, retrying retryable failures per the client's RetryPolicy, and
// decodes a 2xx body into out. The returned error is always a [*Error] except
// when ctx is cancelled, in which case it is the context error.
func (c *Client) do(ctx context.Context, r request, out any) error {
	var payload []byte
	if r.body != nil {
		b, err := json.Marshal(r.body)
		if err != nil {
			return &Error{Kind: KindInvalid, Op: r.op, Err: fmt.Errorf("marshal request: %w", err)}
		}
		payload = b
	}
	var last error
	for attempt := 1; attempt <= c.retry.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		retryAfter, err := c.attempt(ctx, r, payload, out)
		if err == nil {
			return nil
		}
		last = err
		var aerr *Error
		if !errors.As(err, &aerr) || !aerr.Retryable() || attempt == c.retry.MaxAttempts {
			return err
		}
		c.log.DebugContext(ctx, "ai retrying",
			slog.String("op", r.op), slog.String("kind", string(aerr.Kind)),
			slog.Int("attempt", attempt), slog.Int("max", c.retry.MaxAttempts))
		if serr := sleep(ctx, c.retry.backoff(attempt, retryAfter)); serr != nil {
			return serr
		}
	}
	return last
}

// attempt issues a single request and returns any Retry-After hint plus a typed
// error (nil on a decoded 2xx response).
func (c *Client) attempt(ctx context.Context, r request, payload []byte, out any) (time.Duration, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	httpReq, err := http.NewRequestWithContext(ctx, r.method, c.baseURL+r.path, body)
	if err != nil {
		return 0, &Error{Kind: KindInvalid, Op: r.op, Err: fmt.Errorf("build request: %w", err)}
	}
	httpReq.Header.Set(HeaderContractVersion, ContractVersion)
	httpReq.Header.Set("Accept", "application/json")
	if payload != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, ctxErr // a cancelled/expired parent context is terminal
		}
		return 0, classifyTransport(r.op, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out != nil {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				return 0, &Error{Kind: KindInvalid, Op: r.op, Status: resp.StatusCode, Err: fmt.Errorf("decode response: %w", err)}
			}
		}
		return 0, nil
	}
	return parseRetryAfter(resp.Header), classifyStatus(r.op, resp.StatusCode, readErrBody(resp.Body))
}

// readErrBody reads a bounded error body and extracts a message from the common
// JSON shapes ({"error"|"detail"|"status":...}) or the trimmed raw text.
func readErrBody(r io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(r, maxErrBodyBytes))
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return ""
	}
	var msg struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
		Status string `json:"status"`
	}
	if json.Unmarshal(data, &msg) == nil {
		switch {
		case msg.Error != "":
			return msg.Error
		case msg.Detail != "":
			return msg.Detail
		case msg.Status != "":
			return msg.Status
		}
	}
	return trimmed
}
