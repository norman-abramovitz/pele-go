package flowchart

import (
	"slices"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// Model-level assertions ported from upstream's tests/compat/flowchart, where they explain
// behaviour a parity failure would not.

func parse(t *testing.T, src string) *FlowDb {
	t.Helper()
	db := newFlowDb(flowDbOptions{MaxEdges: 500})
	parseFlowchart(src, db, nil)
	return db
}

func parseErr(t *testing.T, src string) *perr.Error {
	t.Helper()
	var got *perr.Error
	func() {
		defer func() {
			if r := recover(); r != nil {
				e, ok := r.(*perr.Error)
				if !ok {
					panic(r)
				}
				got = e
			}
		}()
		parse(t, src)
	}()
	if got == nil {
		t.Fatalf("parse %q: no error", src)
	}
	return got
}

func TestEdges(t *testing.T) {
	tests := map[string]struct {
		src         string
		dir         string
		typ, stroke string
		length      float64
		edgeText    string
	}{
		"arrow":           {src: "graph TD;\nA-->B;", dir: "TB", typ: "arrow_point", stroke: "normal", length: 1},
		"angle as LR":     {src: "graph >;A-->B;", dir: "LR", typ: "arrow_point", stroke: "normal", length: 1},
		"angle as RL":     {src: "graph <;A-->B;", dir: "RL", typ: "arrow_point", stroke: "normal", length: 1},
		"caret as BT":     {src: "graph ^;A-->B;", dir: "BT", typ: "arrow_point", stroke: "normal", length: 1},
		"v as TB":         {src: "graph v;A-->B;", dir: "TB", typ: "arrow_point", stroke: "normal", length: 1},
		"open":            {src: "graph TD;A---B;", dir: "TB", typ: "arrow_open", stroke: "normal", length: 1},
		"cross":           {src: "graph TD;A--xB;", dir: "TB", typ: "arrow_cross", stroke: "normal", length: 1},
		"circle":          {src: "graph TD;A--oB;", dir: "TB", typ: "arrow_circle", stroke: "normal", length: 1},
		"double point":    {src: "graph TD;A<-->B;", dir: "TB", typ: "double_arrow_point", stroke: "normal", length: 1},
		"double cross":    {src: "graph TD;A x--x B;", dir: "TB", typ: "double_arrow_cross", stroke: "normal", length: 1},
		"thick":           {src: "graph TD;A==>B;", dir: "TB", typ: "arrow_point", stroke: "thick", length: 1},
		"dotted":          {src: "graph TD;A-.->B;", dir: "TB", typ: "arrow_point", stroke: "dotted", length: 1},
		"long dotted":     {src: "graph TD;A-...->B;", dir: "TB", typ: "arrow_point", stroke: "dotted", length: 3},
		"invisible":       {src: "graph TD;A~~~B;", dir: "TB", typ: "arrow_open", stroke: "invisible", length: 1},
		"long":            {src: "graph TD;A----->B;", dir: "TB", typ: "arrow_point", stroke: "normal", length: 4},
		"capped length":   {src: "graph TD;A---------------->B;", dir: "TB", typ: "arrow_point", stroke: "normal", length: 10},
		"text":            {src: "graph TD;A-- text -->B;", dir: "TB", typ: "arrow_point", stroke: "normal", length: 1, edgeText: "text"},
		"pipe text":       {src: "graph TD;A-->|text|B;", dir: "TB", typ: "arrow_point", stroke: "normal", length: 1, edgeText: "text"},
		"thick text":      {src: "graph TD;A== text ==>B;", dir: "TB", typ: "arrow_point", stroke: "thick", length: 1, edgeText: "text"},
		"dotted text":     {src: "graph TD;A-. text .->B;", dir: "TB", typ: "arrow_point", stroke: "dotted", length: 1, edgeText: "text"},
		"bracketed start": {src: "graph TD;A<-- text -->B;", dir: "TB", typ: "double_arrow_point", stroke: "normal", length: 1, edgeText: "text"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			db := parse(t, tc.src)
			if db.Direction != tc.dir {
				t.Errorf("direction = %q, want %q", db.Direction, tc.dir)
			}
			if db.Nodes.Len() != 2 || len(db.Edges) != 1 {
				t.Fatalf("got %d nodes and %d edges", db.Nodes.Len(), len(db.Edges))
			}
			e := db.Edges[0]
			if e.Start != "A" || e.End != "B" || e.Type != tc.typ || e.Stroke != tc.stroke || e.Length != tc.length || e.Text != tc.edgeText {
				t.Errorf("edge = %+v", e)
			}
		})
	}
}

func TestEdgeIDs(t *testing.T) {
	db := parse(t, "flowchart TD\nA e1@--> B\nA --> B\nA --> B\ne1@{ curve: linear, animate: true }")
	var ids []string
	for _, e := range db.Edges {
		ids = append(ids, e.ID)
	}
	if want := []string{"e1", "L_A_B_2", "L_A_B_3"}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	if db.Edges[0].Interpolate != "linear" || !db.Edges[0].IsUserDefinedID {
		t.Errorf("edge = %+v", db.Edges[0])
	}
	if db.Nodes.Has("e1") {
		t.Error("metadata for an edge made a node")
	}
}

func TestChainsAndAmpersands(t *testing.T) {
	db := parse(t, "graph TD\nA & B --> C & D --> E")
	var pairs []string
	for _, e := range db.Edges {
		pairs = append(pairs, e.Start+e.End)
	}
	if want := []string{"AC", "AD", "BC", "BD", "CE", "DE"}; !slices.Equal(pairs, want) {
		t.Errorf("edges = %v, want %v", pairs, want)
	}
}

func TestSubgraphs(t *testing.T) {
	tests := map[string]struct {
		src       string
		id, title string
		nodes     []string
	}{
		"tab indentation":   {src: "graph TB\nsubgraph One\n\ta1-->a2\nend", id: "One", title: "One", nodes: []string{"a2", "a1"}},
		"chain":             {src: "graph TB\nsubgraph One\n\ta1-->a2-->a3\nend", id: "One", title: "One", nodes: []string{"a3", "a2", "a1"}},
		"quoted title":      {src: "graph TB\nsubgraph \"Some Title\"\n\ta1-->a2\nend", id: "subGraph0", title: "Some Title", nodes: []string{"a2", "a1"}},
		"id and title":      {src: "graph TB\nsubgraph some-id[Some Title]\n\ta1-->a2\nend", id: "some-id", title: "Some Title", nodes: []string{"a2", "a1"}},
		"words in title":    {src: "graph TD;A-->B;subgraph old style that is broken;c-->d;end;", id: "subGraph0", title: "old style that is broken", nodes: []string{"d", "c"}},
		"number first":      {src: "graph TD\nA\nsubgraph 1test\nA\nend", id: "1test", title: "1test", nodes: []string{"A"}},
		"quoted in bracket": {src: "graph TD;A-->B;subgraph uid2[\"text of doom\"];c-->d;end;", id: "uid2", title: "text of doom", nodes: []string{"d", "c"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			db := parse(t, tc.src)
			if len(db.Subgraphs) != 1 {
				t.Fatalf("got %d subgraphs", len(db.Subgraphs))
			}
			sg := db.Subgraphs[0]
			if sg.ID != tc.id || sg.Title != tc.title || !slices.Equal(sg.Nodes, tc.nodes) {
				t.Errorf("subgraph = %+v", sg)
			}
		})
	}
}

func TestNestedSubgraphsBelongToTheirParent(t *testing.T) {
	db := parse(t, "flowchart TB\nsubgraph outer\nsubgraph inner\na\nend\nb\nend")
	if len(db.Subgraphs) != 2 || db.Subgraphs[0].ID != "inner" || !slices.Equal(db.Subgraphs[1].Nodes, []string{"inner", "b"}) {
		t.Errorf("subgraphs = %+v", db.Subgraphs)
	}
}

func TestStyles(t *testing.T) {
	db := parse(t, "graph TD\nA-->B\nclassDef exClass fill:#f9f,color:red\nclass A exClass\nstyle B stroke:#333\nlinkStyle 0 stroke:#ff3\nlinkStyle default interpolate basis")
	def := db.Classes["exClass"]
	if !slices.Equal(def.Styles, []string{"fill:#f9f", "color:red"}) || !slices.Equal(def.TextStyles, []string{"color:red"}) {
		t.Errorf("class = %+v", def)
	}
	a, _ := db.Nodes.Get("A")
	b, _ := db.Nodes.Get("B")
	if !slices.Equal(a.Classes, []string{"exClass"}) || !slices.Equal(b.Styles, []string{"stroke:#333"}) {
		t.Errorf("A = %+v, B = %+v", a, b)
	}
	if !slices.Equal(db.Edges[0].Style, []string{"stroke:#ff3", "fill:none"}) || db.DefaultInterpolate != "basis" {
		t.Errorf("edge = %+v, default interpolate %q", db.Edges[0], db.DefaultInterpolate)
	}
}

func TestLinkStyleOutOfBounds(t *testing.T) {
	err := parseErr(t, "graph TD\nA-->B\nlinkStyle 1 stroke:red")
	want := "The index 1 for linkStyle is out of bounds. Valid indices for linkStyle are between 0 and 0. (Help: Ensure that the index is within the range of existing edges.)"
	if err.Message != want || err.Code != perr.Semantic {
		t.Errorf("error = %q (%s)", err.Message, err.Code)
	}
}

func TestNodeData(t *testing.T) {
	db := parse(t, "flowchart TB\nA@{ shape: diamond, label: \"Decide\" }\nB@{ icon: \"fa:user\", form: circle, pos: t, h: 60 }")
	a, _ := db.Nodes.Get("A")
	if a.Type != "diamond" || a.Text != "Decide" || a.LabelType != "markdown" {
		t.Errorf("A = %+v", a)
	}
	b, _ := db.Nodes.Get("B")
	if b.Icon != "fa:user" || b.Form != "circle" || b.Pos != "t" || b.AssetHeight != 60 || b.Text != "" {
		t.Errorf("B = %+v", b)
	}
}

func TestBadShapes(t *testing.T) {
	tests := map[string]string{
		"upper case":            "No such shape: Rect. Shape names should be lowercase.",
		"unicode 16 upper case": "No such shape: \u1c89. Shape names should be lowercase.",
		"unknown":               "No such shape: blob.",
	}
	srcs := map[string]string{
		"upper case":            "flowchart TB\nA@{ shape: Rect }",
		"unicode 16 upper case": "flowchart TB\nA@{ shape: \u1c89 }",
		"unknown":               "flowchart TB\nA@{ shape: blob }",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			if err := parseErr(t, srcs[name]); err.Message != want {
				t.Errorf("error = %q, want %q", err.Message, want)
			}
		})
	}
}

func TestClickAndTooltip(t *testing.T) {
	db := parse(t, "graph TD\nA-->B\nclick A \"https://example.com\" \"Go there\" _blank\nclick B callback")
	a, _ := db.Nodes.Get("A")
	b, _ := db.Nodes.Get("B")
	if a.Link != "https://example.com/" || a.LinkTarget != "_blank" || db.Tooltips["A"] != "Go there" || !slices.Equal(b.Classes, []string{"clickable"}) {
		t.Errorf("A = %+v, B = %+v, tooltips %v", a, b, db.Tooltips)
	}
}

func TestSyntaxErrors(t *testing.T) {
	tests := map[string]struct {
		src, want string
	}{
		"unclosed subgraph": {src: "graph TD\nsubgraph A\nB", want: "Parse error on line 3:\nB\n-^\nExpecting 'end', got '$end'"},
		"lexical":           {src: "graph TD\nA[\"`a` b\"]", want: "Lexical error on line 2. Unrecognized text.\nA[\"`a` b\"]\n-----^"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if err := parseErr(t, tc.src); err.Message != tc.want {
				t.Errorf("error = %q, want %q", err.Message, tc.want)
			}
		})
	}
}

func TestEdgeLimit(t *testing.T) {
	db := newFlowDb(flowDbOptions{MaxEdges: 2})
	defer func() {
		e, ok := recover().(*perr.Error)
		if !ok || e.Message != "Edge limit exceeded. The limit is 2." || e.Code != perr.Limit {
			t.Errorf("error = %v", e)
		}
	}()
	parseFlowchart("graph TD\nA & B --> C & D\n", db, nil)
}
