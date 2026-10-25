package timeline

import (
	"reflect"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func parse(source string) *timelineDb {
	db := newTimelineDb()
	parseTimeline(jsstr.UTF16(source), db, nil)
	return db
}

func events(db *timelineDb) map[string][]string {
	out := map[string][]string{}
	for _, p := range db.periods {
		out[p.text] = p.events
	}
	return out
}

// Ported from tests/compat/timeline/timeline.spec.js.
func TestParseTimeline(t *testing.T) {
	tests := map[string]struct {
		source    string
		direction string
		title     string
		sections  []string
		events    map[string][]string
	}{
		"default direction": {source: "timeline\n    section abc-123", direction: "LR", sections: []string{"abc-123"}, events: map[string][]string{}},
		"TD":                {source: "timeline TD\n    section abc-123", direction: "TD", sections: []string{"abc-123"}, events: map[string][]string{}},
		"LR":                {source: "timeline LR\n    section abc-123", direction: "LR", sections: []string{"abc-123"}, events: map[string][]string{}},
		"two sections": {
			source:    "timeline\n    section abc-123\n    task1\n    task2\n    section abc-456\n    task3\n    task4",
			direction: "LR",
			sections:  []string{"abc-123", "abc-456"},
			events:    map[string][]string{"task1": {}, "task2": {}, "task3": {}, "task4": {}},
		},
		"multiline events": {
			source:    "timeline\n    section abc-123\n      task1: event1\n      task2: event2: event3\n           : event4: event5\n   ",
			direction: "LR",
			sections:  []string{"abc-123"},
			events:    map[string][]string{"task1": {"event1"}, "task2": {"event2", "event3", "event4", "event5"}},
		},
		"semicolons": {
			source:    "timeline\n      title ;my;title;\n      section ;a;bc-123;\n      ;ta;sk1;: ;ev;ent1; : ;ev;ent2; : ;ev;ent3;\n   ",
			direction: "LR",
			title:     ";my;title;",
			sections:  []string{";a;bc-123;"},
			events:    map[string][]string{";ta;sk1;": {";ev;ent1; ", ";ev;ent2; ", ";ev;ent3;"}},
		},
		"hashtags": {
			source:    "timeline\n      title #my#title#\n      section #a#bc-123#\n      task1: #ev#ent1# : #ev#ent2# : #ev#ent3#\n   ",
			direction: "LR",
			title:     "#my#title#",
			sections:  []string{"#a#bc-123#"},
			events:    map[string][]string{"task1": {"#ev#ent1# ", "#ev#ent2# ", "#ev#ent3#"}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			db := parse(tc.source)
			if db.direction != tc.direction || db.title != tc.title {
				t.Errorf("direction %q, title %q", db.direction, db.title)
			}
			if !reflect.DeepEqual(db.sections, tc.sections) {
				t.Errorf("sections %q", db.sections)
			}
			if got := events(db); !reflect.DeepEqual(got, tc.events) {
				t.Errorf("events %q", got)
			}
		})
	}
}

// An event before any period is an error, where Mermaid throws a TypeError.
func TestEventWithoutPeriod(t *testing.T) {
	defer func() {
		e, ok := recover().(*perr.Error)
		if !ok || e.Code != perr.Semantic || e.Message != "An event needs a time period before it." || e.Type != "timeline" {
			t.Errorf("got %#v", e)
		}
	}()
	parse("timeline\n: event\n")
}
