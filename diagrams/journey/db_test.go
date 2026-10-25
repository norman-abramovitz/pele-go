package journey

import (
	"math"
	"reflect"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

func parse(t *testing.T, source string) *journeyDb {
	t.Helper()
	db := &journeyDb{}
	parseJourney(jsstr.UTF16(source), db, nil)
	return db
}

// Ported from tests/compat/journey/journeyDb.spec.js.
func TestTasksAndActors(t *testing.T) {
	db := &journeyDb{}
	db.setAccTitle("Shopping")
	db.setAccDescription("A user journey for family shopping")
	db.addSection("Journey to the shops")
	db.addTask("Get car keys", ":5:Dad")
	db.addTask("Go to car", ":3:Dad, Mum, Child#1, Child#2")
	db.addTask("Drive to supermarket", ":4:Dad")
	db.addSection("Do shopping")
	db.addTask("Go shopping", ":5:Mum")

	if db.accTitle != "Shopping" || db.accDescr != "A user journey for family shopping" {
		t.Errorf("accessibility: %q, %q", db.accTitle, db.accDescr)
	}
	want := []journeyTask{
		{task: "Get car keys", score: 5, people: []string{"Dad"}, section: "Journey to the shops"},
		{task: "Go to car", score: 3, people: []string{"Dad", "Mum", "Child#1", "Child#2"}, section: "Journey to the shops"},
		{task: "Drive to supermarket", score: 4, people: []string{"Dad"}, section: "Journey to the shops"},
		{task: "Go shopping", score: 5, people: []string{"Mum"}, section: "Do shopping"},
	}
	if !reflect.DeepEqual(db.tasks, want) {
		t.Errorf("tasks: %+v", db.tasks)
	}
	if got := journeyActors(db.tasks); !reflect.DeepEqual(got, []string{"Child#1", "Child#2", "Dad", "Mum"}) {
		t.Errorf("actors: %q", got)
	}
	if !reflect.DeepEqual(db.sections, []string{"Journey to the shops", "Do shopping"}) {
		t.Errorf("sections: %q", db.sections)
	}
}

// Actors sort by UTF-16 code unit, so a character outside the BMP comes before U+FFFD.
func TestActorsSortByCodeUnit(t *testing.T) {
	got := journeyActors([]journeyTask{{people: []string{"\ufffd", "\U0001f600", "b", "a", "b"}}})
	if want := []string{"a", "b", "\U0001f600", "\ufffd"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Ported from tests/compat/journey/journey.spec.js.
func TestParseJourney(t *testing.T) {
	t.Run("multiline accDescr", func(t *testing.T) {
		db := parse(t, "journey\naccDescr {\n        A user journey for\n        family shopping\n      }"+
			"title Adding journey diagram functionality to mermaid\n"+
			"accTitle: Adding acc journey diagram functionality to mermaid\n"+
			"section Order from website")
		if db.accDescr != "A user journey for\nfamily shopping" {
			t.Errorf("accDescr %q", db.accDescr)
		}
		if db.title != "Adding journey diagram functionality to mermaid" {
			t.Errorf("title %q", db.title)
		}
		if db.accTitle != "Adding acc journey diagram functionality to mermaid" {
			t.Errorf("accTitle %q", db.accTitle)
		}
	})
	t.Run("tasks", func(t *testing.T) {
		db := parse(t, "journey\ntitle Adding journey diagram functionality to mermaid\nsection Documentation\n"+
			"A task: 5: Alice, Bob, Charlie\nB task: 3:Bob, Charlie\nC task: 5\nD task: 5: Charlie, Alice\nE task: 5:\n"+
			"section Another section\nP task: 5:\nQ task: 5:\nR task: 5:")
		want := []journeyTask{
			{task: "A task", score: 5, people: []string{"Alice", "Bob", "Charlie"}, section: "Documentation"},
			{task: "B task", score: 3, people: []string{"Bob", "Charlie"}, section: "Documentation"},
			{task: "C task", score: 5, people: []string{}, section: "Documentation"},
			{task: "D task", score: 5, people: []string{"Charlie", "Alice"}, section: "Documentation"},
			{task: "E task", score: 5, people: []string{""}, section: "Documentation"},
			{task: "P task", score: 5, people: []string{""}, section: "Another section"},
			{task: "Q task", score: 5, people: []string{""}, section: "Another section"},
			{task: "R task", score: 5, people: []string{""}, section: "Another section"},
		}
		if !reflect.DeepEqual(db.tasks, want) {
			t.Errorf("tasks: %+v", db.tasks)
		}
	})
	t.Run("score that is not a number", func(t *testing.T) {
		db := parse(t, "journey\nA: x\n")
		if len(db.tasks) != 1 || !math.IsNaN(db.tasks[0].score) {
			t.Errorf("tasks: %+v", db.tasks)
		}
	})
}
