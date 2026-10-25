package git

import (
	"slices"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Mermaid's commit ids end in random digits; upstream derives them from the sequence number.
func TestCommitIDsAreDeterministic(t *testing.T) {
	m := buildGit(parseGit("gitGraph\ncommit\ncommit\ncommit id: \"x\"\ncommit\n", nil), jsobj.New(), nil)
	var got []string
	for id := range m.commits.Keys() {
		got = append(got, id)
	}
	if want := []string{"0-9e3779b", "1-3c6ef36", "x", "3-78dde6c"}; !slices.Equal(got, want) {
		t.Errorf("ids %q, want %q", got, want)
	}
}

// Ported from tests/compat/git/gitGraph.spec.ts: branches sort by order, then by creation.
func TestLaneOrder(t *testing.T) {
	tests := map[string]struct {
		source string
		want   []string
	}{
		"creation order": {
			source: "gitGraph\ncommit\nbranch b1\nbranch b2\n",
			want:   []string{"main", "b1", "b2"},
		},
		"explicit order": {
			source: "gitGraph\ncommit\nbranch b1 order: 3\nbranch b2 order: 1\nbranch b3\n",
			want:   []string{"main", "b3", "b2", "b1"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := buildGit(parseGit(tc.source, nil), jsobj.New(), nil)
			if !slices.Equal(m.lanes, tc.want) {
				t.Errorf("lanes %q, want %q", m.lanes, tc.want)
			}
		})
	}
}
