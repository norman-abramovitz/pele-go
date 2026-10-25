package state

import (
	"strings"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// Ported from upstream's tests/compat/state: the parts of the model a render does not show.

func parseGraph(t *testing.T, src string) *stateDb {
	t.Helper()
	db := newStateDb()
	parseState(src, db, nil)
	return db
}

// slots maps each node to its colour slot, -1 for none.
func slots(t *testing.T, src string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, node := range parseGraph(t, src).graph.nodes {
		out[node.id] = -1
		if node.colorIndex != nil {
			out[node.id] = *node.colorIndex
		}
	}
	return out
}

func TestColorSlots(t *testing.T) {
	tests := map[string]struct {
		src  string
		want map[string]int
	}{
		"each composite in containment order": {
			src:  "stateDiagram-v2\n[*] --> Boot\nstate Boot {\n[*] --> Firmware\nstate Kernel {\n[*] --> Scheduler\nstate Drivers {\n[*] --> Probe\n}\n}\n}\nBoot --> Running\nstate Running {\n[*] --> Serving\n}\n",
			want: map[string]int{"Boot": 0, "Kernel": 1, "Drivers": 2, "Running": 3, "Firmware": -1},
		},
		"styled composite spends its slot": {
			src:  "stateDiagram-v2\nclassDef pinned fill:#111827,stroke:#F59E0B\nstate First {\n[*] --> A\n}\nstate Second {\n[*] --> B\n}\nstate Third {\n[*] --> C\n}\nclass Second pinned\n",
			want: map[string]int{"First": 0, "Second": -1, "Third": 2},
		},
		"composite named twice takes one slot": {
			src:  "stateDiagram-v2\n[*] --> Loop\nstate Loop {\n[*] --> Spin\n}\nLoop --> Loop\nLoop --> Exit\nstate Exit {\n[*] --> Bye\n}\n",
			want: map[string]int{"Loop": 0, "Exit": 1},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := slots(t, tc.src)
			for id, want := range tc.want {
				if got[id] != want {
					t.Errorf("%s: slot %d, want %d", id, got[id], want)
				}
			}
		})
	}
}

func TestRegionsShareTheirCompositesSlot(t *testing.T) {
	db := parseGraph(t, "stateDiagram-v2\nstate First {\n[*] --> A\n--\n[*] --> B\n}\nstate Second {\n[*] --> C\n--\n[*] --> D\n}\n")
	slot := map[string]int{}
	for _, node := range db.graph.nodes {
		if node.colorIndex != nil {
			slot[node.id] = *node.colorIndex
		}
	}
	regions := 0
	for _, node := range db.graph.nodes {
		if node.shape != "divider" {
			continue
		}
		regions++
		if node.parentId == nil || *node.colorIndex != slot[*node.parentId] {
			t.Errorf("region %s does not share its composite's slot", node.id)
		}
	}
	if regions != 4 {
		t.Errorf("%d regions, want 4", regions)
	}
}

func TestNoteEdgeIsDashed(t *testing.T) {
	db := parseGraph(t, "stateDiagram-v2\n[*] --> Active\nActive --> Idle\nnote right of Active\n  A note\nend note\n")
	notes := 0
	for _, edge := range db.graph.edges {
		isNote := strings.Contains(edge.classes, "note-edge")
		if isNote {
			notes++
		}
		if edge.dashed != isNote {
			t.Errorf("edge %s: dashed %v, classes %q", edge.id, edge.dashed, edge.classes)
		}
	}
	if notes != 1 {
		t.Errorf("%d note edges, want 1", notes)
	}
}

func TestGroupWithDescriptionFails(t *testing.T) {
	defer func() {
		e, ok := recover().(*perr.Error)
		if !ok || e.Code != perr.Semantic || e.Message != "Group nodes can only have label. Remove the additional description for node [A]" {
			t.Errorf("got %v", e)
		}
	}()
	parseGraph(t, "stateDiagram-v2\nstate A {\n[*] --> B\n}\nA : one\nA : two\n")
}
