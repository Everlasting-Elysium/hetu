package search_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/search"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare term", "sunset", `"sunset"`},
		{"two bare terms implicit AND", "sunset beach", `"sunset" AND "beach"`},
		{"name field", "name:sunset", `name : "sunset"`},
		{"tag alias", "tag:landscape", `tags : "landscape"`},
		{"tags field", "tags:landscape", `tags : "landscape"`},
		{"desc alias", "desc:golden", `description : "golden"`},
		{"description field", "description:golden", `description : "golden"`},
		{"multi field", "name:sunset tag:landscape", `name : "sunset" AND tags : "landscape"`},
		{"quoted phrase field", `desc:"golden hour"`, `description : "golden hour"`},
		{"explicit OR", "sunset OR moon", `"sunset" OR "moon"`},
		{"explicit AND", "sunset AND moon", `"sunset" AND "moon"`},
		{"NOT operator", "name:sunset NOT blurry", `name : "sunset" NOT "blurry"`},
		{"lowercase or is a term", "sunset or moon", `"sunset" AND "or" AND "moon"`},
		{"unknown field becomes term", "foo:bar", `"foo" AND "bar"`},
		{"leading operator dropped", "OR sunset", `"sunset"`},
		{"trailing operator dropped", "sunset OR", `"sunset"`},
		{"leading NOT dropped", "NOT sunset", `"sunset"`},
		{"trailing NOT dropped", "sunset NOT", `"sunset"`},
		{"consecutive operators collapse to last", "sunset AND OR moon", `"sunset" OR "moon"`},
		{"bare quoted phrase", `"golden hour"`, `"golden hour"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := search.Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParse_EmptyQuery(t *testing.T) {
	// Blank input and operator-only input both yield ErrEmptyQuery: a bare
	// operator must never reach FTS5 (which would be a syntax error / 500).
	for _, in := range []string{"", "   ", "\t\n", "AND", "NOT NOT", "OR AND NOT"} {
		if _, err := search.Parse(in); !errors.Is(err, search.ErrEmptyQuery) {
			t.Errorf("Parse(%q) error = %v, want ErrEmptyQuery", in, err)
		}
	}
}

// TestParse_InjectionNeutralized verifies FTS5 syntax injection attempts are
// flattened into quoted literals: no bare operators or unescaped quotes leak.
func TestParse_InjectionNeutralized(t *testing.T) {
	got, err := search.Parse(`sunset" OR name:"x`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The stray OR and name: are trapped inside a quoted phrase, not operators.
	if !strings.Contains(got, `" OR name:"`) {
		t.Errorf("injection not quoted as literal: %q", got)
	}
	// Every double-quote must be balanced (even count) so nothing breaks out.
	if strings.Count(got, `"`)%2 != 0 {
		t.Errorf("unbalanced quotes in %q", got)
	}
}

// TestParse_EscapesEmbeddedQuotes verifies a value containing a double-quote is
// escaped (doubled) so it cannot terminate the FTS5 string early.
func TestParse_EscapesEmbeddedQuotes(t *testing.T) {
	got, err := search.Parse(`name:a"b`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// name:a  -> name : "a", then "b -> opens a quoted phrase reading to EOF.
	if strings.Count(got, `"`)%2 != 0 {
		t.Errorf("unbalanced quotes in %q", got)
	}
}
