package architecture

import (
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func build(t *testing.T, source string) (db *architectureDb, err *perr.Error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*perr.Error)
			if !ok {
				panic(r)
			}
			err = e
		}
	}()
	db = newArchitectureDb()
	populate(parseArchitecture(source, nil), db)
	return db, nil
}

// Ported from tests/compat/architecture/architecture.spec.ts.
func TestPopulate(t *testing.T) {
	db, err := build(t, "architecture-beta\n"+
		"group api(cloud)[API]\n"+
		"service db(database)[\"Data base\"] in api\n"+
		"service fe\"Front\"['end']\n"+
		"junction j in api\n"+
		"db:R <-[call]-> L:fe\n"+
		"j{group}:B -- T:fe\n"+
		"align row db j\n")
	if err != nil {
		t.Fatal(err)
	}
	db1, _ := db.nodes.Get("db")
	if db1.icon != "database" || db1.title != "Data base" || db1.in != "api" || db1.junction {
		t.Errorf("db: %+v", db1)
	}
	fe, _ := db.nodes.Get("fe")
	if fe.iconText != "Front" || fe.title != "end" {
		t.Errorf("fe: %+v", fe)
	}
	if len(db.edges) != 2 {
		t.Fatalf("edges: %+v", db.edges)
	}
	e := db.edges[0]
	if e.lhsDir != 'R' || e.rhsDir != 'L' || !e.lhsInto || !e.rhsInto || e.title != "call" {
		t.Errorf("edge 0: %+v", e)
	}
	if e := db.edges[1]; !e.lhsGroup || e.rhsGroup || e.lhsDir != 'B' || e.rhsDir != 'T' {
		t.Errorf("edge 1: %+v", e)
	}
	if len(db.layoutHints) != 1 || db.layoutHints[0].direction != "row" {
		t.Errorf("hints: %+v", db.layoutHints)
	}
}

func TestPopulateErrors(t *testing.T) {
	tests := map[string]struct {
		source string
		code   perr.Code
		want   string
	}{
		"duplicate id": {
			source: "architecture-beta\nservice a\njunction a\n",
			code:   perr.Semantic,
			want:   "The junction id [a] is already in use by another node",
		},
		"missing parent": {
			source: "architecture-beta\nservice a in g\n",
			code:   perr.Semantic,
			want:   "The service [a]'s parent does not exist. Please make sure the parent is created before this service",
		},
		"parent is a node": {
			source: "architecture-beta\nservice b\nservice a in b\n",
			code:   perr.Semantic,
			want:   "The service [a]'s parent is not a group",
		},
		"within itself": {
			source: "architecture-beta\ngroup g in g\n",
			code:   perr.Semantic,
			want:   "The group [g] cannot be placed within itself",
		},
		"edge to group": {
			source: "architecture-beta\ngroup g\nservice a\na:L -- R:g\n",
			code:   perr.Semantic,
			want:   "The right-hand id [g] is a group. Add {group} to a service inside it instead.",
		},
		"group boundary": {
			source: "architecture-beta\ngroup g\nservice a in g\nservice b in g\na{group}:L -- R:b\n",
			code:   perr.Semantic,
			want:   "The left-hand id [a] is modified to traverse the group boundary, but the edge does not pass through two groups.",
		},
		"align repeat": {
			source: "architecture-beta\nservice a\nalign row a a\n",
			code:   perr.Semantic,
			want:   "align row lists [a] more than once",
		},
		"direction splits id": {
			source: "architecture-beta\nservice Lb\n",
			code:   perr.Syntax,
			want:   "Parse error on line 2:\nservice Lb\n--------^\nExpecting token of type 'ID' but found `L`.",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := build(t, tc.source)
			if err == nil {
				t.Fatal("no error")
			}
			if err.Code != tc.code || err.Message != tc.want {
				t.Errorf("got %s %q, want %s %q", err.Code, err.Message, tc.code, tc.want)
			}
		})
	}
}
