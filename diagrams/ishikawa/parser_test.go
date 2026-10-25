package ishikawa

import (
	"fmt"
	"strings"
	"testing"
)

func outline(n *ishikawaNode) string {
	if n == nil {
		return "<nil>"
	}
	var parts []string
	for _, c := range n.children {
		parts = append(parts, outline(c))
	}
	if len(parts) == 0 {
		return n.text
	}
	return fmt.Sprintf("%s(%s)", n.text, strings.Join(parts, ","))
}

// Ported from tests/compat/ishikawa/ishikawa.spec.ts.
func TestParseIshikawa(t *testing.T) {
	tests := map[string]struct {
		source, want string
	}{
		"basic hierarchy": {
			source: "ishikawa-beta\n    Blurry Photo\n        Process\n            Out of focus\n        User\n            Shaky hands\n",
			want:   "Blurry Photo(Process(Out of focus),User(Shaky hands))",
		},
		"unindented root": {
			source: "ishikawa-beta\nProblem\nCause A\n  Subcause A1\nCause B\n",
			want:   "Problem(Cause A(Subcause A1),Cause B)",
		},
		"effect indented more than causes": {
			source: "ishikawa-beta\n    Problem\nCause A\n  Subcause A1\nCause B\n",
			want:   "Problem(Cause A(Subcause A1),Cause B)",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			db := &ishikawaDb{}
			parseIshikawa(tc.source, db, nil)
			if got := outline(db.root); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}
