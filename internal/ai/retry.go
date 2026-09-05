package ai

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// RetryPolicy bounds how many times a retryable request is re-issued and how
// long the client backs off between attempts.
type RetryPolicy struct {
	MaxAttempts int           // total attempts including the first (>= 1)
	BaseDelay   time.Duration // backoff before the first retry; doubles each attempt
	MaxDelay    time.Duration // cap on a single backoff
}

// DefaultRetryPolicy is the client default: 3 attempts with 100ms→400ms backoff.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    2 * time.Second,
	}
}

// normalized fills zero fields with defaults so a partial policy is usable.
func (p RetryPolicy) normalized() RetryPolicy {
	if p.MaxAttempts < 1 {
		p.MaxAttempts = 1
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = 100 * time.Millisecond
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = 2 * time.Second
	}
	return p
}

// backoff returns the delay before retrying the given 1-based attempt. A server
// Retry-After (retryAfter > 0) wins; otherwise it is exponential base*2^(n-1)
// capped at MaxDelay, with full jitter over [d/2, d] to desynchronize retries.
func (p RetryPolicy) backoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return min(retryAfter, p.MaxDelay)
	}
	d := p.BaseDelay << (attempt - 1)
	if d <= 0 || d > p.MaxDelay {
		d = p.MaxDelay
	}
	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

// sleep waits for d or until ctx is done, returning ctx.Err() if cancelled.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// parseRetryAfter reads a Retry-After header in delta-seconds form (the
// HTTP-date form is ignored) into a non-negative duration.
func parseRetryAfter(h http.Header) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
