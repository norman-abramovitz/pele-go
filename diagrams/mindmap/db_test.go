package mindmap

import (
	"strings"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func parse(src string) (db *mindmapDb, err *perr.Error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*perr.Error)
			if !ok {
				panic(r)
			}
			err = e
		}
	}()
	db = &mindmapDb{}
	parseMindmap(src, db, nil)
	return db, nil
}

// Ported from tests/compat/mindmap/mindmap.spec.ts.
func TestTree(t *testing.T) {
	db, err := parse("mindmap\n    root\n      child1\n          leaf1\n      child2\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(db.nodes) != 4 {
		t.Fatalf("got %d nodes", len(db.nodes))
	}
	root := db.nodes[0]
	if !root.isRoot || len(root.children) != 2 || root.children[0].descr != "child1" || root.children[1].descr != "child2" {
		t.Errorf("root %+v", root)
	}
	if leaf := root.children[0].children; len(leaf) != 1 || leaf[0].descr != "leaf1" || leaf[0].section != 0 {
		t.Errorf("leaf %+v", leaf)
	}
	if root.children[1].section != 1 {
		t.Errorf("child2 section %d, want 1", root.children[1].section)
	}
}

func TestOneRoot(t *testing.T) {
	tests := map[string]string{
		"two roots":        "mindmap\n    root\n    fakeRoot",
		"root out of line": "mindmap\n          root\n        fakeRoot\n    realRootWrongPlace",
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parse(src)
			want := `There can be only one root. No parent could be found for ("fakeRoot")`
			if err == nil || err.Message != want || err.Code != perr.Semantic {
				t.Fatalf("got %v, want %q", err, want)
			}
		})
	}
}

func TestDecorations(t *testing.T) {
	for name, src := range map[string]string{
		"class first": "mindmap\n    root[The root]\n    :::m-4 p-8\n    ::icon(bomb)\n",
		"icon first":  "mindmap\n    root[The root]\n    ::icon(bomb)\n    :::m-4 p-8\n",
	} {
		t.Run(name, func(t *testing.T) {
			db, err := parse(src)
			if err != nil {
				t.Fatal(err)
			}
			n := db.nodes[0]
			if n.nodeID != "root" || n.descr != "The root" || n.typ != 2 || n.class != "m-4 p-8" || n.icon == nil || *n.icon != "bomb" {
				t.Errorf("node %+v", n)
			}
		})
	}
}

// Sections wrap after eleven first-level branches, as in Mermaid.
func TestSectionsWrap(t *testing.T) {
	db, err := parse("mindmap\nroot\n" + strings.Repeat("  b\n", 12))
	if err != nil {
		t.Fatal(err)
	}
	if got := db.nodes[0].children[11].section; got != 0 {
		t.Errorf("twelfth branch section %d, want 0", got)
	}
}
