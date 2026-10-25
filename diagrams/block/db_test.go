package block

import "testing"

// Ported from tests/compat/block/blockDB.spec.ts.
func TestEdgeStrings(t *testing.T) {
	tests := map[string]struct {
		link, thickness, pattern, start, end string
	}{
		"plain":       {link: "-->", thickness: "normal", pattern: "solid", start: "arrow_open", end: "arrow_point"},
		"dotted":      {link: "-.->", thickness: "normal", pattern: "dotted", start: "arrow_open", end: "arrow_point"},
		"thick":       {link: "==>", thickness: "thick", pattern: "solid", start: "arrow_open", end: "arrow_point"},
		"both ends":   {link: "<-->", thickness: "normal", pattern: "solid", start: "arrow_point", end: "arrow_point"},
		"thick both":  {link: "<==>", thickness: "thick", pattern: "solid", start: "arrow_point", end: "arrow_point"},
		"cross":       {link: "--x", thickness: "normal", pattern: "solid", start: "arrow_open", end: "arrow_cross"},
		"circle":      {link: "--o", thickness: "normal", pattern: "solid", start: "arrow_open", end: "arrow_circle"},
		"cross start": {link: "x--x", thickness: "normal", pattern: "solid", start: "arrow_cross", end: "arrow_cross"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := [4]string{edgeStrToThickness(tc.link), edgeStrToPattern(tc.link), edgeStrToEdgeStartData(tc.link), edgeStrToEdgeData(tc.link)}
			if want := [4]string{tc.thickness, tc.pattern, tc.start, tc.end}; got != want {
				t.Errorf("%s: got %q, want %q", tc.link, got, want)
			}
		})
	}
}

// Ported from tests/compat/block/blockColorIndex.spec.ts: composites take palette slots in
// source order, and simple blocks take none.
func TestColorIndex(t *testing.T) {
	tests := map[string]struct {
		source string
		want   map[string]int
	}{
		"declaration order": {
			source: "block-beta\nblock:first\na\nend\nblock:second\nb\nend\nblock:third\nc\nend\n",
			want:   map[string]int{"first": 0, "second": 1, "third": 2, "a": -1},
		},
		"simple blocks": {
			source: "block-beta\na\nb\nblock:group\nc\nend\n",
			want:   map[string]int{"a": -1, "b": -1, "c": -1, "group": 0},
		},
		"outer before inner": {
			source: "block-beta\nblock:outer\nblock:inner\na\nend\nend\nblock:sibling\nb\nend\n",
			want:   map[string]int{"outer": 0, "inner": 1, "sibling": 2},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			db := newBlockDb()
			parseBlock(tc.source, db, nil)
			for id, want := range tc.want {
				b, ok := db.byId.Get(id)
				if !ok {
					t.Fatalf("no block %s", id)
				}
				if b.colorIndex != want {
					t.Errorf("%s: colorIndex %d, want %d", id, b.colorIndex, want)
				}
			}
		})
	}
}

// Ported from tests/compat/block/layout.spec.ts.
func TestCalculateBlockPosition(t *testing.T) {
	tests := map[string]struct {
		columns, position, px, py float64
	}{
		"first":      {columns: 2, position: 0, px: 0, py: 0},
		"second":     {columns: 2, position: 1, px: 1, py: 0},
		"wraps":      {columns: 2, position: 2, px: 0, py: 1},
		"second row": {columns: 2, position: 3, px: 1, py: 1},
		"third row":  {columns: 2, position: 4, px: 0, py: 2},
		"one column": {columns: 1, position: 3, px: 0, py: 3},
		"single row": {columns: -1, position: 7, px: 7, py: 0},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			px, py := calculateBlockPosition(tc.columns, tc.position)
			if px != tc.px || py != tc.py {
				t.Errorf("got (%v, %v), want (%v, %v)", px, py, tc.px, tc.py)
			}
		})
	}
}

// Ported from tests/compat/block/layout.spec.ts: a wider gap makes a wider grid.
func TestLayoutGap(t *testing.T) {
	width := func(gap float64) float64 {
		root := &cell{columns: 2, children: []*cell{newCell(100, 50, 1), newCell(100, 50, 1)}}
		w, _ := layout(root, gap)
		return w
	}
	if got := width(4); got != 204 {
		t.Errorf("gap 4: width %v, want 204", got)
	}
	if got := width(20); got != 220 {
		t.Errorf("gap 20: width %v, want 220", got)
	}
}
