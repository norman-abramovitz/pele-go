package c4

import (
	"strings"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func parsed(t *testing.T, src string) *c4Db {
	t.Helper()
	db := newC4Db()
	parseC4(src+"\n", db, nil)
	return db
}

func field(el *element, name string) string {
	if p := el.field(name); p != nil {
		return *p
	}
	return el.label
}

// From upstream's tests/compat/c4/c4NamedAttributes.spec.ts: a named argument fills the field
// it names, whichever slot it is written in.
func TestNamedAttributeFillsItsField(t *testing.T) {
	tests := map[string]struct {
		header, template string
		fields           []string
		boundary         bool
	}{
		"system":             {"C4Context", `System(x, "L", $F="V")`, []string{"descr", "tags", "sprite", "link"}, false},
		"person":             {"C4Context", `Person(x, "L", $F="V")`, []string{"descr", "tags", "sprite", "link"}, false},
		"container":          {"C4Container", `Container(x, "L", $F="V")`, []string{"descr", "techn", "tags", "sprite", "link"}, false},
		"component":          {"C4Component", `Component(x, "L", $F="V")`, []string{"descr", "techn", "tags", "sprite", "link"}, false},
		"system boundary":    {"C4Context", `System_Boundary(x, "L", $F="V")`, []string{"type", "tags", "link"}, true},
		"container boundary": {"C4Container", `Container_Boundary(x, "L", $F="V")`, []string{"type", "tags", "link"}, true},
		"node":               {"C4Deployment", `Node(x, "L", $F="V")`, []string{"type", "descr", "tags", "sprite", "link"}, true},
	}
	for name, tc := range tests {
		for _, f := range tc.fields {
			t.Run(name+" "+f, func(t *testing.T) {
				src := tc.header + "\n" + strings.Replace(tc.template, "$F", "$"+f, 1)
				if tc.boundary {
					src += " {\nContainer(i, \"I\")\n}"
				}
				db := parsed(t, src)
				el := db.shapes[0]
				if tc.boundary {
					el = db.boundaries[1]
				}
				if got := field(el, f); got != "V" {
					t.Fatalf("%s = %q, want V", f, got)
				}
			})
		}
	}
}

func TestRelNamedAttribute(t *testing.T) {
	for _, f := range []string{"techn", "descr", "tags", "sprite", "link"} {
		t.Run(f, func(t *testing.T) {
			db := parsed(t, "C4Context\nPerson(a, \"A\")\nPerson(b, \"B\")\nRel(a, b, \"uses\", $"+f+"=\"V\")")
			if got := *db.rels[0].field(f); got != "V" {
				t.Fatalf("%s = %q, want V", f, got)
			}
		})
	}
}

func TestBoundary(t *testing.T) {
	db := parsed(t, "C4Context\ntitle System Context\nBoundary(b1, \"BankBoundary\") {\nSystem(SystemAA, \"Internet Banking System\")\n}")
	if len(db.boundaries) != 2 {
		t.Fatalf("got %d boundaries, want 2", len(db.boundaries))
	}
	b := db.boundaries[1]
	if b.alias != "b1" || b.label != "BankBoundary" || b.typ != "system" || b.parent != db.boundaries[0] {
		t.Fatalf("boundary = %+v", b)
	}
	if db.shapes[0].parent != b {
		t.Fatal("system is not inside the boundary")
	}
	if db.title != "System Context" || db.c4Type != "C4Context" {
		t.Fatalf("title %q, type %q", db.title, db.c4Type)
	}
}

// From c4Diagram.spec.ts: keywords can be arguments.
func TestKeywordsAsArguments(t *testing.T) {
	db := parsed(t, "C4Context\ntitle title\nPerson(Person, \"Person\", \"Person\")")
	s := db.shapes[0]
	if db.title != "title" || s.alias != "Person" || s.label != "Person" || s.descr != "Person" {
		t.Fatalf("title %q, shape %+v", db.title, s)
	}
}

func TestSpliceKindIntoShortBoundary(t *testing.T) {
	db := parsed(t, "C4Context\nEnterprise_Boundary(\"b\") {\nPerson(\"p\")\n}")
	if got := db.boundaries[1].label; got != "ENTERPRISE" {
		t.Fatalf("label = %q, want ENTERPRISE", got)
	}
}

func TestUpdateStyles(t *testing.T) {
	db := parsed(t, "C4Context\nPerson(a, \"A\")\nPerson(b, \"B\")\nRel(a, b, \"uses\")\n"+
		"UpdateElementStyle(a, $bgColor=\"red\")\nUpdateRelStyle(a, b, \"blue\", \"green\", \"-12px\", $offsetY=\"0x10\")\n"+
		"UpdateLayoutConfig($c4ShapeInRow=\"3\", \"0\")")
	if db.shapes[0].bgColor != "red" {
		t.Fatalf("bgColor = %q", db.shapes[0].bgColor)
	}
	r := db.rels[0]
	if r.textColor != "blue" || r.lineColor != "green" || r.offsetX != -12 || r.offsetY != 16 {
		t.Fatalf("rel = %+v", r)
	}
	if db.shapeInRow != 3 || db.boundaryInRow != 2 {
		t.Fatalf("in row %v, %v", db.shapeInRow, db.boundaryInRow)
	}
}

func TestParseErrors(t *testing.T) {
	tests := map[string]struct {
		src, want string
	}{
		"no header":    {"Person(a)", "Expecting 'direction_tb', 'direction_bt', 'direction_rl', 'direction_lr', 'C4_CONTEXT', 'C4_CONTAINER', 'C4_COMPONENT', 'C4_DYNAMIC', 'C4_DEPLOYMENT', got 'PERSON'"},
		"unknown word": {"C4Context\nFoo(a)", "Lexical error on line 2. Unrecognized text."},
		"unclosed":     {"C4Context\nBoundary(\"b\") {\nPerson(\"a\")", "Expecting 'NEWLINE', 'RBRACE', got 'EOF'"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				e, ok := recover().(*perr.Error)
				if !ok || !strings.Contains(e.Message, tc.want) {
					t.Fatalf("error = %v, want one containing %q", e, tc.want)
				}
			}()
			parsed(t, tc.src)
		})
	}
}
