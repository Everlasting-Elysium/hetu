package model3d

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeSidecar stands in for the Blender /convert endpoint with the same
// contract: read the multipart "model" field, echo back deterministic GLB bytes.
// It records the ?ext= query and uploaded body so tests can assert the request.
type fakeSidecar struct {
	glb     []byte
	gotExt  string
	gotBody []byte
	status  int
}

func (f *fakeSidecar) handler(w http.ResponseWriter, r *http.Request) {
	f.gotExt = r.URL.Query().Get("ext")
	file, _, err := r.FormFile("model")
	if err != nil {
		http.Error(w, "no model", http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()
	f.gotBody, _ = io.ReadAll(file)
	if f.status != 0 && f.status != http.StatusOK {
		http.Error(w, "boom", f.status)
		return
	}
	_, _ = w.Write(f.glb)
}

func addrOf(ts *httptest.Server) string {
	return strings.TrimPrefix(ts.URL, "http://")
}

func TestConvertToGLB_Success(t *testing.T) {
	fake := &fakeSidecar{glb: []byte("glTF-binary-bytes")}
	ts := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer ts.Close()

	src := strings.NewReader("solid-stl-source")
	var out bytes.Buffer
	if err := ConvertToGLB(context.Background(), addrOf(ts), "stl", src, &out); err != nil {
		t.Fatalf("ConvertToGLB() error = %v", err)
	}
	if out.String() != "glTF-binary-bytes" {
		t.Errorf("output = %q, want the sidecar GLB bytes", out.String())
	}
	if fake.gotExt != "stl" {
		t.Errorf("sidecar ext = %q, want %q", fake.gotExt, "stl")
	}
	if string(fake.gotBody) != "solid-stl-source" {
		t.Errorf("sidecar body = %q, want the source bytes", fake.gotBody)
	}
}

func TestConvertToGLB_NoSidecar(t *testing.T) {
	err := ConvertToGLB(context.Background(), "", "stl", strings.NewReader("x"), io.Discard)
	if err == nil {
		t.Fatal("ConvertToGLB() with empty addr = nil, want error")
	}
}

func TestConvertToGLB_SidecarError(t *testing.T) {
	fake := &fakeSidecar{status: http.StatusInternalServerError}
	ts := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer ts.Close()

	err := ConvertToGLB(context.Background(), addrOf(ts), "obj", strings.NewReader("x"), io.Discard)
	if err == nil {
		t.Fatal("ConvertToGLB() on sidecar 500 = nil, want error")
	}
}
