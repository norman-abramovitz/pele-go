package requirement

import (
	"slices"
	"strings"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func parse(t *testing.T, source string) *requirementDb {
	t.Helper()
	db := newRequirementDb()
	parseRequirement(source, db, nil)
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
	parseRequirement(source, newRequirementDb(), nil)
	return nil
}

func TestRequirementFields(t *testing.T) {
	db := parse(t, "requirementDiagram\n\nrequirement test_req {\nid: test_id\ntext: the test text.\nrisk: High\nverifymethod: Analysis\n}\n")
	req, ok := db.requirements.Get("test_req")
	if !ok {
		t.Fatal("requirement test_req missing")
	}
	want := requirement{name: "test_req", typ: "Requirement", requirementID: "test_id", text: "the test text.", risk: "High",
		verifyMethod: "Analysis", styling: styling{classes: []string{"default"}}}
	if req.name != want.name || req.typ != want.typ || req.requirementID != want.requirementID || req.text != want.text ||
		req.risk != want.risk || req.verifyMethod != want.verifyMethod || !slices.Equal(req.classes, want.classes) {
		t.Errorf("got %+v, want %+v", *req, want)
	}
}

// The grammar is right recursive, so of two repeated fields the first one written wins.
func TestRepeatedFieldFirstWins(t *testing.T) {
	db := parse(t, "requirementDiagram\nelement e {\ntype: one\ntype: two\ndocref: d\n}\n")
	el, _ := db.elements.Get("e")
	if el.typ != "one" || el.docRef != "d" {
		t.Errorf("got type %q docRef %q, want one, d", el.typ, el.docRef)
	}
}

func TestRelationshipDirections(t *testing.T) {
	db := parse(t, "requirementDiagram\na - satisfies -> b\nc <- traces - d\n")
	want := []relation{{typ: "satisfies", src: "a", dst: "b"}, {typ: "traces", src: "d", dst: "c"}}
	if !slices.Equal(db.relations, want) {
		t.Errorf("got %+v, want %+v", db.relations, want)
	}
}

func TestStylesAndClasses(t *testing.T) {
	db := parse(t, "requirementDiagram\nrequirement r {\n}\nelement e {\n}\nclassDef c1 color:red\nclassDef c2 stroke-width:4px,stroke: yellow\nr:::c1\nclass e c2\nstyle r fill:blue\n")
	r, _ := db.requirements.Get("r")
	e, _ := db.elements.Get("e")
	if !slices.Equal(r.cssStyles, []string{"color:red", "fill:blue"}) {
		t.Errorf("requirement styles %q", r.cssStyles)
	}
	if !slices.Equal(e.cssStyles, []string{"stroke-width:4px", "stroke:yellow"}) {
		t.Errorf("element styles %q", e.cssStyles)
	}
	if !slices.Equal(r.classes, []string{"default", "c1"}) {
		t.Errorf("requirement classes %q", r.classes)
	}
	c1, _ := db.classes.Get("c1")
	if !slices.Equal(c1.textStyles, []string{"color:red"}) {
		t.Errorf("c1 text styles %q", c1.textStyles)
	}
}

func TestDirectionAndAccessibility(t *testing.T) {
	db := parse(t, "accTitle: hi\nrequirementDiagram\ndirection LR\naccDescr {\n  line one\n   line two\n}\n")
	if db.direction != "LR" {
		t.Errorf("direction %q, want LR", db.direction)
	}
	if db.accTitle != "hi" {
		t.Errorf("accTitle %q", db.accTitle)
	}
	if db.accDescr != "line one\nline two" {
		t.Errorf("accDescr %q", db.accDescr)
	}
}

func TestSyntaxError(t *testing.T) {
	err := parseErr(t, "requirementDiagram\nrequirement r {\nrisk: often\n}\n")
	want := "Expecting 'LOW_RISK', 'MED_RISK', 'HIGH_RISK', got 'unqString'"
	if err.Code != perr.Syntax || !strings.Contains(err.Message, want) {
		t.Errorf("got %q, want it to contain %q", err.Message, want)
	}
}
