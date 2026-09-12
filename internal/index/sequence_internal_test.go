package index

import (
	"sort"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// isImg is a test predicate matching the raster extensions the real image
// handler matches, so groupSequences behaves as it does in a live scan.
func isImg(ext string) bool {
	switch ext {
	case "png", "jpg", "jpeg", "gif", "bmp", "tiff":
		return true
	default:
		return false
	}
}

func seqEntries(names ...string) []domain.Entry {
	out := make([]domain.Entry, len(names))
	for i, n := range names {
		out[i] = domain.Entry{Name: n, Path: n}
	}
	return out
}

func TestGroupSequences(t *testing.T) {
	tests := []struct {
		name        string
		files       []string
		wantGroups  [][]string // each ordered by frame number; group[0] is the anchor
		wantSingles []string   // order-insensitive
	}{
		{
			name:       "zero-padded consecutive run is one ordered sequence",
			files:      []string{"explosion_0001.png", "explosion_0002.png", "explosion_0003.png"},
			wantGroups: [][]string{{"explosion_0001.png", "explosion_0002.png", "explosion_0003.png"}},
		},
		{
			name:       "unpadded numbers sort numerically, not lexically",
			files:      []string{"f1.png", "f10.png", "f2.png", "f3.png", "f4.png", "f5.png", "f6.png", "f7.png", "f8.png", "f9.png"},
			wantGroups: [][]string{{"f1.png", "f2.png", "f3.png", "f4.png", "f5.png", "f6.png", "f7.png", "f8.png", "f9.png", "f10.png"}},
		},
		{
			name:       "a gap splits into two runs",
			files:      []string{"a_1.png", "a_2.png", "a_4.png", "a_5.png"},
			wantGroups: [][]string{{"a_1.png", "a_2.png"}, {"a_4.png", "a_5.png"}},
		},
		{
			name:        "an isolated number beside a run is a single",
			files:       []string{"a_1.png", "a_3.png", "a_4.png"},
			wantGroups:  [][]string{{"a_3.png", "a_4.png"}},
			wantSingles: []string{"a_1.png"},
		},
		{
			name:        "a lone numbered file is not a sequence",
			files:       []string{"solo_1.png"},
			wantSingles: []string{"solo_1.png"},
		},
		{
			name:        "a name without trailing digits is a single",
			files:       []string{"photo.png"},
			wantSingles: []string{"photo.png"},
		},
		{
			name:       "different stems in one dir are separate sequences",
			files:      []string{"a_1.png", "a_2.png", "b_1.png", "b_2.png"},
			wantGroups: [][]string{{"a_1.png", "a_2.png"}, {"b_1.png", "b_2.png"}},
		},
		{
			name:        "non-image files never join a sequence",
			files:       []string{"a_1.png", "a_2.png", "notes.txt"},
			wantGroups:  [][]string{{"a_1.png", "a_2.png"}},
			wantSingles: []string{"notes.txt"},
		},
		{
			name:        "numbered non-images stay individual assets",
			files:       []string{"doc_1.pdf", "doc_2.pdf"},
			wantSingles: []string{"doc_1.pdf", "doc_2.pdf"},
		},
		{
			name:        "same stem different extension does not group",
			files:       []string{"a_1.png", "a_2.jpg"},
			wantSingles: []string{"a_1.png", "a_2.jpg"},
		},
		{
			name:       "extension case is ignored when grouping",
			files:      []string{"frame_1.PNG", "frame_2.png"},
			wantGroups: [][]string{{"frame_1.PNG", "frame_2.png"}},
		},
		{
			name:       "directory order is irrelevant; anchor is the lowest number",
			files:      []string{"c_3.png", "c_1.png", "c_2.png"},
			wantGroups: [][]string{{"c_1.png", "c_2.png", "c_3.png"}},
		},
		{
			name:        "mixed: a run, a standalone image, and a non-image",
			files:       []string{"seq_08.png", "seq_09.png", "seq_10.png", "hero.png", "readme.md"},
			wantGroups:  [][]string{{"seq_08.png", "seq_09.png", "seq_10.png"}},
			wantSingles: []string{"hero.png", "readme.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups, singles := groupSequences(seqEntries(tt.files...), isImg)

			if got := sortGroups(groupNames(groups)); !equalGroups(got, sortGroups(tt.wantGroups)) {
				t.Errorf("groups = %v, want %v", got, tt.wantGroups)
			}
			wantSingles := append([]string(nil), tt.wantSingles...)
			sort.Strings(wantSingles)
			if got := singleNames(singles); !equalStrings(got, wantSingles) {
				t.Errorf("singles = %v, want %v", got, wantSingles)
			}
			// Partition invariant: every input file is grouped or single exactly once.
			if total := countFrames(groups) + len(singles); total != len(tt.files) {
				t.Errorf("accounted %d files, want %d (groups+singles must partition input)", total, len(tt.files))
			}
		})
	}
}

// TestGroupSequences_DuplicateNumbers locks the degenerate case where two files
// parse to the same number (e.g. "1.png" and "01.png"): a repeat breaks the run,
// so they never silently fold into a 3-frame sequence, and the partition
// invariant still holds.
func TestGroupSequences_DuplicateNumbers(t *testing.T) {
	groups, singles := groupSequences(seqEntries("1.png", "01.png", "2.png"), isImg)
	if total := countFrames(groups) + len(singles); total != 3 {
		t.Fatalf("accounted %d files, want 3", total)
	}
	for _, g := range groups {
		if len(g) > 2 {
			t.Errorf("group %v has >2 frames from a duplicate number", g)
		}
	}
}

func groupNames(groups [][]domain.Entry) [][]string {
	out := make([][]string, len(groups))
	for i, g := range groups {
		names := make([]string, len(g))
		for j, e := range g {
			names[j] = e.Name
		}
		out[i] = names
	}
	return out
}

func singleNames(singles []domain.Entry) []string {
	out := make([]string, len(singles))
	for i, e := range singles {
		out[i] = e.Name
	}
	sort.Strings(out)
	return out
}

// sortGroups orders the outer group list by each group's anchor name so a
// comparison ignores bucket order while preserving in-group frame order.
func sortGroups(g [][]string) [][]string {
	out := make([][]string, len(g))
	copy(out, g)
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) == 0 || len(out[j]) == 0 {
			return len(out[i]) < len(out[j])
		}
		return out[i][0] < out[j][0]
	})
	return out
}

func equalGroups(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalStrings(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func countFrames(groups [][]domain.Entry) int {
	n := 0
	for _, g := range groups {
		n += len(g)
	}
	return n
}
