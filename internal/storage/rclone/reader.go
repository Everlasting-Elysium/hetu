package rclone

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// errClosed is returned when Read is called after Close.
var errClosed = errors.New("rclone: read on closed reader")

// httpReadSeekCloser implements io.ReadSeekCloser over HTTP Range requests.
// Each Seek discards the current response body and the next Read opens a new
// request starting at the seeked offset. It is not safe for concurrent use.
//
// The context passed to Open is reused for every Range request the reader
// issues, so that context must remain valid for the lifetime of the reader.
type httpReadSeekCloser struct {
	ctx    context.Context
	client *http.Client
	url    string
	size   int64
	user   string
	pass   string

	offset int64         // current logical position
	body   io.ReadCloser // current open response body (nil until first Read)
	closed bool          // set by Close; guards read-after-close
}

func newHTTPReadSeekCloser(ctx context.Context, client *http.Client, url string, size int64, user, pass string) *httpReadSeekCloser {
	return &httpReadSeekCloser{
		ctx:    ctx,
		client: client,
		url:    url,
		size:   size,
		user:   user,
		pass:   pass,
	}
}

// Read reads from the current offset, opening an HTTP request if needed.
func (r *httpReadSeekCloser) Read(p []byte) (int, error) {
	if r.closed {
		return 0, errClosed
	}
	if r.offset >= r.size {
		return 0, io.EOF
	}
	if r.body == nil {
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	n, err := r.body.Read(p)
	r.offset += int64(n)
	return n, err
}

// Seek sets the offset for the next Read. It closes any in-flight body so
// the next Read will issue a fresh Range request.
func (r *httpReadSeekCloser) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.offset + offset
	case io.SeekEnd:
		abs = r.size + offset
	default:
		return 0, fmt.Errorf("rclone seek: invalid whence %d", whence)
	}
	if abs < 0 {
		return 0, fmt.Errorf("rclone seek: negative offset %d", abs)
	}
	if abs != r.offset {
		r.closeBody()
	}
	r.offset = abs
	return abs, nil
}

// Close releases the underlying HTTP response body. Subsequent Reads error.
func (r *httpReadSeekCloser) Close() error {
	r.closed = true
	return r.closeBody()
}

// open issues a GET with Range header starting at r.offset.
func (r *httpReadSeekCloser) open() error {
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return fmt.Errorf("rclone open request: %w", err)
	}
	if r.user != "" {
		req.SetBasicAuth(r.user, r.pass)
	}
	if r.offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", r.offset))
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("rclone open fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return fmt.Errorf("rclone open: http %d", resp.StatusCode)
	}
	// When we requested a range (offset > 0) the server MUST answer 206.
	// A 200 means it ignored Range and returned the full body from byte 0,
	// which would silently desync our offset accounting and corrupt reads.
	if r.offset > 0 && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return fmt.Errorf("rclone open: server ignored Range (http %d)", resp.StatusCode)
	}
	r.body = resp.Body
	return nil
}

// closeBody drains and closes the current body if any.
func (r *httpReadSeekCloser) closeBody() error {
	if r.body == nil {
		return nil
	}
	err := r.body.Close()
	r.body = nil
	return err
}
