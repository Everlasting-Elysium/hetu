// Package rclone implements kernel.StorageProvider over an rclone RC daemon.
// The daemon must run with --rc-serve so that file content is served via HTTP
// GET on the same address. List and Stat use the RC JSON API; Open uses the
// built-in HTTP file server with Range support.
package rclone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// ProviderName is the registry key for the rclone storage provider.
const ProviderName = "rclone"

// Provider talks to an rclone rcd instance over HTTP.
type Provider struct {
	baseURL      string // e.g. "http://localhost:5572"
	remote       string // e.g. "remote:"
	rcClient     *http.Client // short-lived RC API calls (List, Stat)
	streamClient *http.Client // long-lived streaming reads (Open)
	user         string
	pass         string
}

var _ kernel.StorageProvider = (*Provider)(nil)

// New returns a provider that talks to the rclone RC daemon at addr.
// remote is the rclone remote spec (e.g. "remote:"). user/pass are optional
// basic-auth credentials.
func New(addr, remote, user, pass string) *Provider {
	sharedTransport := &http.Transport{
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Provider{
		baseURL: "http://" + addr,
		remote:  remote,
		rcClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: sharedTransport,
		},
		// streamClient has no total timeout so large file reads are not
		// aborted mid-stream. Stalls are caught by ResponseHeaderTimeout
		// and the request context passed through Open.
		streamClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
		user: user,
		pass: pass,
	}
}

// Name returns the provider registry key.
func (p *Provider) Name() string { return ProviderName }

// List returns entries directly under path by calling POST /operations/list.
func (p *Provider) List(ctx context.Context, dir string) ([]domain.Entry, error) {
	body, err := json.Marshal(rcListReq{FS: p.remote, Remote: dir})
	if err != nil {
		return nil, fmt.Errorf("rclone list marshal: %w", err)
	}
	resp, err := p.post(ctx, "/operations/list", body)
	if err != nil {
		return nil, fmt.Errorf("rclone list %q: %w", dir, err)
	}
	defer resp.Body.Close()
	if err := checkRC(resp); err != nil {
		return nil, fmt.Errorf("rclone list %q: %w", dir, err)
	}
	var result rcListResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("rclone list decode: %w", err)
	}
	out := make([]domain.Entry, 0, len(result.List))
	for _, item := range result.List {
		modTime, _ := time.Parse(time.RFC3339Nano, item.ModTime)
		out = append(out, domain.Entry{
			Name:    item.Name,
			Path:    path.Join(dir, item.Name),
			IsDir:   item.IsDir,
			Size:    item.Size,
			ModTime: modTime,
		})
	}
	return out, nil
}

// Stat returns metadata for the object at path via POST /operations/stat.
func (p *Provider) Stat(ctx context.Context, filePath string) (domain.FileInfo, error) {
	body, err := json.Marshal(rcStatReq{FS: p.remote, Remote: filePath})
	if err != nil {
		return domain.FileInfo{}, fmt.Errorf("rclone stat marshal: %w", err)
	}
	resp, err := p.post(ctx, "/operations/stat", body)
	if err != nil {
		return domain.FileInfo{}, fmt.Errorf("rclone stat %q: %w", filePath, err)
	}
	defer resp.Body.Close()
	if err := checkRC(resp); err != nil {
		return domain.FileInfo{}, fmt.Errorf("rclone stat %q: %w", filePath, err)
	}
	var result rcStatResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return domain.FileInfo{}, fmt.Errorf("rclone stat decode: %w", err)
	}
	if result.Item == nil {
		return domain.FileInfo{}, fmt.Errorf("rclone stat %q: %w", filePath, domain.ErrNotFound)
	}
	modTime, _ := time.Parse(time.RFC3339Nano, result.Item.ModTime)
	return domain.FileInfo{
		Size:    result.Item.Size,
		IsDir:   result.Item.IsDir,
		ModTime: modTime,
	}, nil
}

// Open opens a file for reading via the rclone HTTP file server (--rc-serve).
// The returned io.ReadSeekCloser uses HTTP Range requests for seeking; every
// Read/Seek issues requests bound to ctx, so ctx must stay valid for as long
// as the returned reader is used (do not cancel it before Close).
func (p *Provider) Open(ctx context.Context, filePath string) (io.ReadSeekCloser, error) {
	// First stat to get file size (needed for Seek from end).
	info, err := p.Stat(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("rclone open %q: %w", filePath, err)
	}
	if info.IsDir {
		return nil, fmt.Errorf("rclone open %q: is a directory", filePath)
	}
	serveURL := p.serveURL(filePath)
	return newHTTPReadSeekCloser(ctx, p.streamClient, serveURL, info.Size, p.user, p.pass), nil
}

// serveURL builds the HTTP file-server URL for the given path, escaping each
// segment so that special characters (?, #, %, spaces) are percent-encoded.
// The [remote:] prefix is kept literal because rclone expects unescaped brackets.
func (p *Provider) serveURL(filePath string) string {
	segments := strings.Split(filePath, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return p.baseURL + "/[" + p.remote + "]/" + strings.Join(segments, "/")
}

// post sends a JSON POST to the RC API.
func (p *Provider) post(ctx context.Context, endpoint string, jsonBody []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.user != "" {
		req.SetBasicAuth(p.user, p.pass)
	}
	return p.rcClient.Do(req)
}

// checkRC returns an error if the HTTP status indicates failure. A 404 (or an
// error message mentioning "not found") is mapped to domain.ErrNotFound so
// callers can use errors.Is consistently across providers.
func checkRC(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	var rcErr struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&rcErr)
	if resp.StatusCode == http.StatusNotFound ||
		(rcErr.Error != "" && strings.Contains(strings.ToLower(rcErr.Error), "not found")) {
		return fmt.Errorf("rc %d: %s: %w", resp.StatusCode, rcErr.Error, domain.ErrNotFound)
	}
	if rcErr.Error != "" {
		return fmt.Errorf("rc %d: %s", resp.StatusCode, rcErr.Error)
	}
	return fmt.Errorf("rc status %d", resp.StatusCode)
}

// --- RC API request/response types ---

type rcListReq struct {
	FS     string `json:"fs"`
	Remote string `json:"remote"`
}

type rcListResp struct {
	List []rcItem `json:"list"`
}

type rcStatReq struct {
	FS     string `json:"fs"`
	Remote string `json:"remote"`
}

type rcStatResp struct {
	Item *rcItem `json:"item"`
}

type rcItem struct {
	Path    string `json:"Path"`
	Name    string `json:"Name"`
	Size    int64  `json:"Size"`
	ModTime string `json:"ModTime"`
	IsDir   bool   `json:"IsDir"`
}
