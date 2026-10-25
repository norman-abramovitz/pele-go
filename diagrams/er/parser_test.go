package er

import (
	"slices"
	"strings"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func parse(t *testing.T, source string) *erDb {
	t.Helper()
	db := newErDb()
	parseEr(source, db, nil)
	return db
}

func parseErr(t *testing.T, source string) (err *perr.Error) {
	t.Helper()
	defer func() {
		r := recover()
		e, ok := r.(*perr.Error)
		if !ok {
			t.Fatalf("parse %q: got %v, want a *perr.Error", source, r)
		}
		err = e
	}()
	parseEr(source, newErDb(), nil)
	return nil
}

func TestRelationshipCardinalities(t *testing.T) {
	tests := map[string]struct {
		source  string
		want    relSpec
		wantIDs [2]string
	}{
		"crow's feet": {
			source:  "erDiagram\nCUSTOMER ||--o{ ORDER : places\n",
			want:    relSpec{cardA: "ZERO_OR_MORE", relType: "IDENTIFYING", cardB: "ONLY_ONE"},
			wantIDs: [2]string{"entity-CUSTOMER-0", "entity-ORDER-1"},
		},
		// The second "1" of "1 1:" is the entity, not a cardinality (Mermaid issue #7472).
		"numeric entity after a 1": {
			source:  "erDiagram\na many to 1 1: label\n",
			want:    relSpec{cardA: "ONLY_ONE", relType: "IDENTIFYING", cardB: "ZERO_OR_MORE"},
			wantIDs: [2]string{"entity-a-0", "entity-1-1"},
		},
		"1 straight before a dotted line": {
			source:  "erDiagram\nA 1..0+ B : r\n",
			want:    relSpec{cardA: "ZERO_OR_MORE", relType: "NON_IDENTIFYING", cardB: "ONLY_ONE"},
			wantIDs: [2]string{"entity-A-0", "entity-B-1"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			db := parse(t, tc.source)
			if len(db.relationships) != 1 {
				t.Fatalf("got %d relationships, want 1", len(db.relationships))
			}
			rel := db.relationships[0]
			if rel.relSpec != tc.want {
				t.Errorf("relSpec = %+v, want %+v", rel.relSpec, tc.want)
			}
			if got := [2]string{rel.entityA, rel.entityB}; got != tc.wantIDs {
				t.Errorf("ends = %v, want %v", got, tc.wantIDs)
			}
		})
	}
}

func TestAttributesKeepWrittenOrderAcrossBlocks(t *testing.T) {
	db := parse(t, "erDiagram\nBOOK {\nstring title PK \"the name\"\n}\nBOOK {\nstring author FK, UK\nfloat price\n}\n")
	book, _ := db.entities.Get("BOOK")
	var got []string
	for _, a := range book.attributes {
		got = append(got, a.typ+" "+a.name+" ["+strings.Join(a.keys, ",")+"] "+a.comment)
	}
	want := []string{"string title [PK] the name", "string author [FK,UK] ", "float price [] "}
	if !slices.Equal(got, want) {
		t.Errorf("attributes = %q, want %q", got, want)
	}
}

// A node belongs to the first subgraph that lists it; inner subgraphs are closed first.
func TestSubgraphsClaimNodesOnce(t *testing.T) {
	db := parse(t, "erDiagram\nsubgraph outer [Outer Box]\nsubgraph inner\ndirection LR\nA\nend\nA\nB\nend\n")
	if len(db.subgraphs) != 2 {
		t.Fatalf("got %d subgraphs, want 2", len(db.subgraphs))
	}
	inner, outer := db.subgraphs[0], db.subgraphs[1]
	if inner.id != "inner" || inner.dir != "LR" || !slices.Equal(inner.nodes, []string{"A"}) {
		t.Errorf("inner = %+v", inner)
	}
	if outer.title != "Outer Box" || !slices.Equal(outer.nodes, []string{"inner", "B"}) {
		t.Errorf("outer = %+v", outer)
	}
}

func TestParseErrors(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"unclosed subgraph": {
			source: "erDiagram\nsubgraph G\nA\n",
			want:   "Expecting 'END', 'EOF'",
		},
		"bad style text": {
			source: "erDiagram\nstyle A fill:#f00!\n",
			want:   "Lexical error on line 2",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := parseErr(t, tc.source)
			if !strings.Contains(err.Message, tc.want) {
				t.Errorf("message = %q, want it to contain %q", err.Message, tc.want)
			}
		})
	}
}
