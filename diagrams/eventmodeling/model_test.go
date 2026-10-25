package eventmodeling

import (
	"testing"
)

// Ported from tests/compat/eventmodeling: namespaces get their own lanes after their group's,
// and a frame with no sources is fed by the nearest earlier frame in another lane.
func TestLanesAndRelations(t *testing.T) {
	m := buildEventModel(parseEventModel("eventmodeling\ntf 01 ui Shop.Cart\ntf 02 cmd AddItem\ntf 03 evt ItemAdded\ntf 04 evt Other.Thing\n", nil), nil, nil)
	var labels []string
	for _, l := range m.lanes {
		labels = append(labels, l.label)
	}
	want := []string{"UI/A: Shop", "Command/Read Model", "Events", "Stream: Other"}
	if len(labels) != len(want) {
		t.Fatalf("lanes %q, want %q", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Errorf("lanes %q, want %q", labels, want)
		}
	}
	if len(m.relations) != 3 || m.relations[2].source.id != "03" {
		t.Errorf("relations %v", m.relations)
	}
}

func TestBlockText(t *testing.T) {
	tests := map[string]struct{ raw, want string }{
		"dedent": {raw: "{\n    a\n      b\n}", want: "a\n  b"},
		"inline": {raw: `"x y"`, want: "x y"},
		"braces": {raw: "{ k: 1 }", want: "k: 1"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := blockText(tc.raw); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
