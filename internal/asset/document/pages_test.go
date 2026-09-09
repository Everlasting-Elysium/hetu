package document

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func TestParsePagesLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
		ok   bool
	}{
		{"pdfinfo", "Title:   x\nPages:          12\nEncrypted: no\n", 12, true},
		{"mutool", "PDF-1.5\nPages: 3\nRetrieving info...\n", 3, true},
		{"tight", "Pages:7\n", 7, true},
		{"none", "Title: x\nEncrypted: no\n", 0, false},
		{"garbage", "Pages: many\n", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := parsePagesLine([]byte(tc.in))
			if tc.ok {
				if err != nil {
					t.Fatalf("parsePagesLine(%q) error = %v", tc.in, err)
				}
				if n != tc.want {
					t.Fatalf("parsePagesLine(%q) = %d, want %d", tc.in, n, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("parsePagesLine(%q) = %d, want error", tc.in, n)
			}
		})
	}
}

// TestSniffPDF checks PDF detection (a PDF-compatible .ai counts as PDF) and
// that the reader is always rewound to the start for the caller.
func TestSniffPDF(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"pdf", []byte("%PDF-1.4\nrest"), true},
		{"ai as pdf", []byte("%PDF-1.5 ai"), true},
		{"pptx zip", []byte("PK\x03\x04rest"), false},
		{"short", []byte("%PD"), false},
		{"empty", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := bytes.NewReader(tc.data)
			got, err := sniffPDF(r)
			if err != nil {
				t.Fatalf("sniffPDF error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("sniffPDF(%q) = %v, want %v", tc.data, got, tc.want)
			}
			if pos, _ := r.Seek(0, io.SeekCurrent); pos != 0 {
				t.Fatalf("sniffPDF left reader at %d, want 0", pos)
			}
		})
	}
}

func TestPrepareDegradesWithoutTool(t *testing.T) {
	h := discardHandler(rendererNone, "")
	doc, err := h.Prepare(context.Background(), bytes.NewReader(minimalPDF()))
	if !errors.Is(err, domain.ErrNoThumbnail) {
		t.Fatalf("Prepare err = %v, want ErrNoThumbnail", err)
	}
	if doc != nil {
		t.Fatalf("Prepare doc = %v, want nil", doc)
	}
}

func TestRenderPageDegradesWithoutTool(t *testing.T) {
	h := discardHandler(rendererNone, "")
	err := h.RenderPage(context.Background(), bytes.NewReader(minimalPDF()), 1, io.Discard)
	if !errors.Is(err, domain.ErrNoThumbnail) {
		t.Fatalf("RenderPage err = %v, want ErrNoThumbnail", err)
	}
}
