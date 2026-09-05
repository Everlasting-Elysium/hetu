package ai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// Kind classifies an AI sidecar failure so callers can react by category rather
// than by HTTP status. The taxonomy follows Serpent's vendor-adapter model (see
// docs/ai-and-3d.md): retryable transport/limit failures are separated from
// terminal client/credential failures.
type Kind string

const (
	// KindNetwork is a transport failure or a transient upstream 5xx. Retryable.
	KindNetwork Kind = "network"
	// KindTimeout is a request that exceeded its deadline (client or gateway). Retryable.
	KindTimeout Kind = "timeout"
	// KindRateLimit is HTTP 429; back off and retry.
	KindRateLimit Kind = "rate_limit"
	// KindAuth is HTTP 401/403: bad or missing credentials. Terminal.
	KindAuth Kind = "auth"
	// KindQuota is HTTP 402: the account is out of quota/credit. Terminal.
	KindQuota Kind = "quota"
	// KindInvalid is a bad request or an unimplemented capability (400/422/501). Terminal.
	KindInvalid Kind = "invalid"
)

// ErrNotImplemented is the cause wrapped in an [*Error] when the sidecar returns
// 501 Not Implemented (the Phase 1 stub does this for /embed and /tag). Callers
// detect it with errors.Is to skip gracefully instead of failing a job.
var ErrNotImplemented = errors.New("ai: sidecar capability not implemented")

// Error is the typed failure returned by every [Client] method. Op names the
// operation ("tag", "health", ...); Status is the HTTP status (0 for transport
// failures); Err is the underlying cause (a sentinel, a server message, or a
// wrapped transport error).
type Error struct {
	Kind   Kind
	Op     string
	Status int
	Err    error
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("ai %s: %s (status %d): %v", e.Op, e.Kind, e.Status, e.Err)
	}
	return fmt.Sprintf("ai %s: %s: %v", e.Op, e.Kind, e.Err)
}

// Unwrap exposes the underlying cause for errors.Is/As.
func (e *Error) Unwrap() error { return e.Err }

// Retryable reports whether re-issuing the request could succeed.
func (e *Error) Retryable() bool {
	switch e.Kind {
	case KindNetwork, KindTimeout, KindRateLimit:
		return true
	case KindAuth, KindQuota, KindInvalid:
		return false
	default:
		return false
	}
}

// classifyTransport maps a client.Do error (no HTTP response) to a [*Error].
func classifyTransport(op string, err error) *Error {
	kind := KindNetwork
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		kind = KindTimeout
	}
	return &Error{Kind: kind, Op: op, Err: err}
}

// classifyStatus maps a non-2xx HTTP status to a [*Error]. msg is the trimmed
// server body used to enrich the error string.
func classifyStatus(op string, status int, msg string) *Error {
	if status == http.StatusNotImplemented {
		return &Error{Kind: KindInvalid, Op: op, Status: status, Err: ErrNotImplemented}
	}
	e := &Error{Op: op, Status: status, Err: statusCause(status, msg)}
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		e.Kind = KindAuth
	case status == http.StatusPaymentRequired:
		e.Kind = KindQuota
	case status == http.StatusTooManyRequests:
		e.Kind = KindRateLimit
	case status == http.StatusRequestTimeout, status == http.StatusGatewayTimeout:
		e.Kind = KindTimeout
	case status >= 500:
		e.Kind = KindNetwork
	default:
		// 400/422 and any other 4xx: a terminal client error.
		e.Kind = KindInvalid
	}
	return e
}

func statusCause(status int, msg string) error {
	if msg != "" {
		return errors.New(msg)
	}
	return errors.New(http.StatusText(status))
}
