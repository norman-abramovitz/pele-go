package kanban

import (
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func parse(source string) *kanbanDb {
	db := &kanbanDb{}
	parseKanban(jsstr.UTF16(source), db, nil)
	return db
}

func failure(source string) (e *perr.Error) {
	defer func() { e, _ = recover().(*perr.Error) }()
	parse(source)
	return nil
}

// Ported from tests/compat/kanban/kanban.spec.ts.
func TestParseKanban(t *testing.T) {
	tests := map[string]struct {
		source string
		check  func(t *testing.T, s []*kanbanNode)
	}{
		"KNBN-2 hierarchy": {
			source: "kanban\n    root\n      child1\n      child2\n",
			check: func(t *testing.T, s []*kanbanNode) {
				if len(s) != 1 || s[0].label != "root" || len(s[0].items) != 2 || s[0].items[0].label != "child1" || s[0].items[1].label != "child2" {
					t.Errorf("got %+v", s)
				}
			},
		},
		"KNBN-4 deeper levels are cards": {
			source: "kanban\n    root\n      child1\n        leaf1\n      child2",
			check: func(t *testing.T, s []*kanbanNode) {
				if len(s) != 1 || len(s[0].items) != 3 {
					t.Errorf("got %+v", s)
				}
			},
		},
		"KNBN-7 id and label": {
			source: "kanban\n    root[The root]\n      ",
			check: func(t *testing.T, s []*kanbanNode) {
				if s[0].id != "root" || s[0].label != "The root" {
					t.Errorf("got %+v", s[0])
				}
			},
		},
		"KNBN-15 classes and icon": {
			source: "kanban\n    root[The root]\n    :::m-4 p-8\n    ::icon(bomb)\n    ",
			check: func(t *testing.T, s []*kanbanNode) {
				if s[0].cssClasses != "m-4 p-8" || s[0].icon != "bomb" {
					t.Errorf("got %+v", s[0])
				}
			},
		},
		"KNBN-34 multiline metadata": {
			source: "kanban\n        root@{\n          icon: star\n          assigned: knsv\n        }\n    ",
			check: func(t *testing.T, s []*kanbanNode) {
				if s[0].id != "root" || s[0].icon != "star" || s[0].assigned != "knsv" {
					t.Errorf("got %+v", s[0])
				}
			},
		},
		"KNBN-36 label from metadata": {
			source: "kanban\n        root@{ icon: star, label: 'fix things' }\n    ",
			check: func(t *testing.T, s []*kanbanNode) {
				if s[0].label != "fix things" {
					t.Errorf("got %+v", s[0])
				}
			},
		},
		"KNBN-37 ticket and priority": {
			source: "kanban\n        root@{ ticket: MC-1234, priority: high }\n    ",
			check: func(t *testing.T, s []*kanbanNode) {
				if s[0].ticket != "MC-1234" || s[0].priority != "high" {
					t.Errorf("got %+v", s[0])
				}
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.check(t, parse(tc.source).sections)
		})
	}
}

func TestKanbanErrors(t *testing.T) {
	tests := map[string]struct {
		source string
		code   perr.Code
		want   string
	}{
		"KNBN-6 root in the wrong place": {
			source: "kanban\n          root\n        fakeRoot\n    realRootWrongPlace",
			code:   perr.Semantic,
			want:   `Items without section detected, found section ("fakeRoot")`,
		},
		"upper case shape": {
			source: "kanban\n  a@{ shape: Rect }\n",
			code:   perr.Semantic,
			want:   "No such shape: Rect. Shape names should be lowercase.",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := failure(tc.source)
			if e == nil || e.Code != tc.code || e.Message != tc.want || e.Type != "kanban" {
				t.Errorf("got %#v", e)
			}
		})
	}
}
