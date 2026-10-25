package agentflow

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Ported from upstream's tests/compat/agentflow specs: diagnostics.spec.ts,
// agentflow-containment-cycle.spec.ts, containerOrder.spec.ts, colorSlots.spec.ts,
// agentflow-yaml-error-position.spec.ts and agentflow-metadata-prototype-keys.spec.ts.

// diagnosticsFor parses text as the generated parser does and builds its graph, which runs
// the validators.
func diagnosticsFor(t *testing.T, text string) []*diagnostic {
	t.Helper()
	db, err := parseRaw(text, true)
	if err != nil {
		t.Fatalf("parse: %s", err.Message)
	}
	buildAgentGraph(db, 0, nil)
	return db.diagnostics
}

var reproductions = map[string]string{
	"SHAPE_UNSUPPORTED": "agentflow-beta TB\n  a[\"Alpha\"]\n  a@{ shape: \"triangle\" }\n  b[\"Beta\"]\n  a --> b",
	"SHAPE_REMOVED":     "agentflow-beta TB\n  a[\"Alpha\"]\n  a@{ shape: \"cylinder\" }\n  b[\"Beta\"]\n  a --> b",
	"CONTAINMENT_VIOLATION": "agentflow-beta TB\n  flow A[\"Flow A\"]\n    a1[\"Task A1\"]\n    a1 --> B\n  end\n" +
		"  flow B[\"Flow B\"]\n    b1[\"Task B1\"]\n    b1 --> A\n  end",
}

func TestDiagnosticVocabularyHasAReproductionForEveryEmittedID(t *testing.T) {
	for id := range emitted {
		if !slices.Contains(diagnosticIDs, id) {
			t.Errorf("%s is emitted but not in the vocabulary", id)
		}
		if _, ok := reproductions[id]; !ok {
			t.Errorf("no reproduction for %s", id)
		}
	}
	if len(reproductions) != len(emitted) {
		t.Errorf("%d reproductions for %d emitted ids", len(reproductions), len(emitted))
	}
}

func TestDiagnosticsAreEmitted(t *testing.T) {
	for id, source := range reproductions {
		t.Run(id, func(t *testing.T) {
			found := slices.ContainsFunc(diagnosticsFor(t, source), func(d *diagnostic) bool { return d.id == id })
			if !found {
				t.Errorf("no %s", id)
			}
		})
	}
}

func TestNoDiagnosticsForACleanDiagram(t *testing.T) {
	if got := diagnosticsFor(t, "agentflow-beta TB\n  a[\"Alpha\"]\n  b[\"Beta\"]\n  a --> b"); len(got) != 0 {
		t.Errorf("got %d diagnostics", len(got))
	}
}

func mutual(viewA, viewB string) string {
	return "agentflow-beta TB\n  flow A[\"Flow A\"]" + viewA + "\n    a1[\"Task A1\"]\n    a1 --> B\n  end\n" +
		"  flow B[\"Flow B\"]" + viewB + "\n    b1[\"Task B1\"]\n    b1 --> A\n  end"
}

func graphOf(t *testing.T, text string, palette int) (*agentflowDb, *agentGraph) {
	t.Helper()
	db := mustParse(t, text)
	return db, buildAgentGraph(db, palette, nil)
}

func nodeByID(graph *agentGraph, id string) *graphNode {
	for _, n := range graph.nodes {
		if n.id == id {
			return n
		}
	}
	return nil
}

func TestMutualContainmentKeepsTheFirstNestingAndReportsTheOneClosingTheLoop(t *testing.T) {
	db, graph := graphOf(t, mutual("", ""), 0)
	parent := func(id string) string {
		n := nodeByID(graph, id)
		if n == nil || !n.hasParent {
			return "(none)"
		}
		return n.parentID
	}
	for _, n := range graph.nodes {
		seen := map[string]bool{}
		for current := parent(n.id); current != "(none)"; current = parent(current) {
			if seen[current] {
				t.Fatalf("parent chain from %s loops", n.id)
			}
			seen[current] = true
		}
	}
	want := map[string]string{"A": "B", "B": "(none)", "a1": "A", "b1": "B"}
	for id, p := range want {
		if got := parent(id); got != p {
			t.Errorf("parent of %s = %s, want %s", id, got, p)
		}
	}
	i := slices.IndexFunc(db.diagnostics, func(d *diagnostic) bool { return d.id == "CONTAINMENT_VIOLATION" && d.nodeID == "B" })
	if i < 0 || db.diagnostics[i].severity != "warning" {
		t.Errorf("no containment warning for B")
	}
}

func TestMutualContainmentWithCollapsedContainers(t *testing.T) {
	_, graph := graphOf(t, mutual("@{ view: collapsed }", ""), 0)
	a := nodeByID(graph, "A")
	if a == nil || a.shape != "collapsedGroup" || a.hasParent {
		t.Errorf("collapsed A = %+v", a)
	}
	_, graph = graphOf(t, mutual("@{ view: collapsed }", "@{ view: collapsed }"), 0)
	if len(graph.nodes) == 0 {
		t.Error("both collapsed draws nothing")
	}
	graphOf(t, "agentflow-beta TB\n  flow A[\"A\"]@{ view: collapsed }\n    a1[\"a1\"]\n    a1 --> B\n  end\n"+
		"  flow B[\"B\"]\n    b1[\"b1\"]\n    b1 --> C\n  end\n  flow C[\"C\"]\n    c1[\"c1\"]\n    c1 --> A\n  end", 0)
}

func slotsByID(t *testing.T, text string) map[string]int {
	t.Helper()
	db, err := parseSource(text)
	if err != nil {
		t.Fatalf("parse: %s", err.Message)
	}
	slots := map[string]int{}
	for _, n := range buildAgentGraph(db, 12, nil).nodes {
		if n.colorIndex >= 0 {
			slots[n.id] = n.colorIndex
		}
	}
	return slots
}

func TestContainersAreNumberedInTheOrderWritten(t *testing.T) {
	tests := map[string]struct {
		src  string
		want map[string]int
	}{
		"first declared first": {
			src:  "agentflow-beta TB\n flow first[\"First\"]\n a[\"a\"]\n end\n flow second[\"Second\"]\n b[\"b\"]\n end\n flow third[\"Third\"]\n c[\"c\"]\n end\n",
			want: map[string]int{"first": kindCount, "second": kindCount + 1, "third": kindCount + 2},
		},
		"parent before nested": {
			src:  "agentflow-beta TB\n flow outer[\"Outer\"]\n flow inner[\"Inner\"]\n a[\"a\"]\n end\n end\n flow sibling[\"Sibling\"]\n b[\"b\"]\n end\n",
			want: map[string]int{"outer": kindCount, "inner": kindCount + 1, "sibling": kindCount + 2},
		},
		"collapsed keeps its place": {
			src: "agentflow-beta TB\n flow one[\"One\"]\n a[\"a\"]\n end\n flow two[\"Two\"]\n b[\"b\"]\n end\n two@{ view: \"collapsed\" }\n" +
				" flow three[\"Three\"]\n c[\"c\"]\n end\n",
			want: map[string]int{"one": kindCount, "two": kindCount + 1, "three": kindCount + 2},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := slotsByID(t, tc.src)
			for id, slot := range tc.want {
				if got[id] != slot {
					t.Errorf("slot of %s = %d, want %d", id, got[id], slot)
				}
			}
		})
	}
}

func TestColorSlotsFollowKindsAndContainerOrder(t *testing.T) {
	kinds := map[string]string{"a": "tool", "b": "decision", "c": "tool"}
	kindOf := func(id string) string { return kinds[id] }
	nodes := []*graphNode{{id: "a"}, {id: "b"}, {id: "c"}}
	assignColorSlots(nodes, kindOf, map[string]int{}, 12)
	if nodes[0].kind != "tool" || nodes[1].kind != "decision" || nodes[0].kind != nodes[2].kind {
		t.Errorf("kinds %q %q %q", nodes[0].kind, nodes[1].kind, nodes[2].kind)
	}

	nodes = []*graphNode{{id: "one", isGroup: true, colorIndex: -1}, {id: "b", colorIndex: -1}, {id: "two", isGroup: true, colorIndex: -1},
		{id: "three", isGroup: true, colorIndex: -1}}
	assignColorSlots(nodes, kindOf, map[string]int{"one": 0, "two": 1, "three": 2}, 12)
	if nodes[0].colorIndex != kindCount || nodes[2].colorIndex != kindCount+1 || nodes[3].colorIndex != kindCount+2 || nodes[1].colorIndex != -1 {
		t.Errorf("slots %d %d %d %d", nodes[0].colorIndex, nodes[1].colorIndex, nodes[2].colorIndex, nodes[3].colorIndex)
	}

	// A container never folds back onto a kind slot, however many there are.
	for i := range 24 {
		if slot := containerSlot(i, 12); slot < kindCount || slot%12 < kindCount {
			t.Errorf("container %d takes slot %d", i, slot)
		}
	}
	// On a palette shorter than the kinds, the slot is one the palette has.
	if containerSlotCount(3) != 1 || containerSlot(0, 3) != containerSlot(0, 3)%3 {
		t.Errorf("palette of 3: count %d, slot %d", containerSlotCount(3), containerSlot(0, 3))
	}
	for _, length := range []int{1, 3, 6, 7, 8, 12, 24} {
		for i := range containerSlotCount(length) {
			slot := containerSlot(i, length)
			if slot >= max(1, length) || slot != slot%max(1, length) {
				t.Errorf("palette %d, container %d: slot %d", length, i, slot)
			}
		}
	}
}

func TestShapeErrorsAreLocatedWithoutAPositionInTheMessage(t *testing.T) {
	tests := map[string]struct {
		offset   int
		wantLine int
	}{
		"no front matter":             {offset: 0, wantLine: 2},
		"three lines of front matter": {offset: 3, wantLine: 5},
	}
	position := regexp.MustCompile(`\(\d+:\d+\)`)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			db := newAgentflowDb(agentflowDbOptions{maxEdges: 500, lineOffset: tc.offset})
			err := catch(func() { parseAgentflow("agentflow-beta TB\n  a[\"A\"]@{ shape: Rounded }", db, false, nil) })
			if err == nil || !strings.Contains(err.Message, "No such shape") || position.MatchString(err.Message) {
				t.Fatalf("error %v", err)
			}
			if err.Line != tc.wantLine {
				t.Errorf("line %d, want %d", err.Line, tc.wantLine)
			}
		})
	}
	if _, err := parseRaw("agentflow-beta TB\n  a[\"A\"]@{ shape: rounded }\n\n  b[\"B\"]", false); err != nil {
		t.Errorf("well-formed block: %s", err.Message)
	}
}

func metadataOf(t *testing.T, text, id string) *jsobj.Object {
	t.Helper()
	db := mustParse(t, text)
	buildAgentGraph(db, 0, nil)
	v, _ := db.nodes.Get(id)
	return v.metadata
}

func TestPrototypeKeysAreStrippedFromMetadata(t *testing.T) {
	m := metadataOf(t, "agentflow-beta TB\n  a[\"A\"]\n  a@{\n    __proto__:\n      polluted: true\n  }", "a")
	if slices.Contains(m.Keys(), "__proto__") {
		t.Error("__proto__ kept")
	}
	m = metadataOf(t, "agentflow-beta TB\n  a[\"A\"]\n  a@{ constructor: \"x\", prototype: \"y\", description: \"kept\" }", "a")
	if !slices.Equal(m.Keys(), []string{"description"}) {
		t.Errorf("keys %v", m.Keys())
	}
	m = metadataOf(t, "agentflow-beta TB\n  a[\"A\"]\n  a@{\n    params:\n      - name: first\n        __proto__:\n          polluted: true\n    description: \"kept\"\n  }", "a")
	params, _ := m.Get("params")
	list, _ := params.([]any)
	if len(list) != 1 || stringify(list[0]) != `{"name":"first"}` {
		t.Errorf("params %s", stringify(params))
	}
	m = metadataOf(t, "agentflow-beta TB\n  a[\"A\"]\n  a@{ shape: rounded, description: \"hello\", myExtension: 42 }", "a")
	if d, _ := m.Get("description"); d != "hello" {
		t.Errorf("description %v", d)
	}
	if n, _ := m.Get("myExtension"); n != 42.0 {
		t.Errorf("myExtension %v", n)
	}
}
