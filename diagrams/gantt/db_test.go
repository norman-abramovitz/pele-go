package gantt

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Broderick-Westrope/pele-go/internal/jsdate"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// Ported from upstream's tests/compat/gantt/ganttDb.spec.ts, which checks Mermaid's model
// of scheduled tasks.

func scheduleOf(t *testing.T, loc *time.Location, lines ...string) []*task {
	t.Helper()
	db := newGanttDb()
	parseGantt("gantt\n"+strings.Join(lines, "\n")+"\n", db, nil)
	return db.getTasks(at(loc, 2031, 4, 6, 9, 30).Time(), loc).tasks
}

func isoDay(loc *time.Location, iso string) jsdate.Date {
	d, ok := parseStrict(iso, "YYYY-MM-DD", jsdate.New(0, loc))
	if !ok {
		panic(iso)
	}
	return d
}

func sameTime(a *jsdate.Date, b jsdate.Date) bool { return a != nil && a.Time() == b.Time() }

func TestScheduleSkipsExcludedDays(t *testing.T) {
	inZones(t, func(t *testing.T, loc *time.Location) {
		tasks := scheduleOf(t, loc,
			"dateFormat YYYY-MM-DD",
			"excludes weekends 2019-02-06,friday",
			"section weekends skip test",
			"test1 :id1,2019-02-01,1d",
			"test2 :id2,after id1,2d",
			"test3 :id3,after id2,7d",
			"test4 :id4,2019-02-01,2019-02-20",
			"test5 :id5,after id4,1d",
			"section full ending task on last day",
			"test6 :id6,2019-02-13,2d",
			"test7 :id7,after id6,1d",
		)
		want := []struct{ start, end, renderEnd string }{
			{"2019-02-01", "2019-02-04", "2019-02-02"},
			{"2019-02-04", "2019-02-07", "2019-02-06"},
			{"2019-02-07", "2019-02-20", "2019-02-20"},
			// A fixed end date is never moved.
			{"2019-02-01", "2019-02-20", ""},
			{"2019-02-20", "2019-02-21", "2019-02-21"},
			{"2019-02-13", "2019-02-18", "2019-02-15"},
			{"2019-02-18", "2019-02-19", ""},
		}
		for i, w := range want {
			got := tasks[i]
			if !sameTime(got.startTime, isoDay(loc, w.start)) || !sameTime(got.endTime, isoDay(loc, w.end)) {
				t.Errorf("task %s: %v to %v, want %s to %s", got.id, valueOf(got.startTime), valueOf(got.endTime), w.start, w.end)
			}
			if i != 6 && (w.renderEnd == "") != (got.renderEndTime == nil) ||
				w.renderEnd != "" && !sameTime(got.renderEndTime, isoDay(loc, w.renderEnd)) {
				t.Errorf("task %s: drawn to %v, want %q", got.id, valueOf(got.renderEndTime), w.renderEnd)
			}
		}
	})
}

func TestScheduleWeekendFromFriday(t *testing.T) {
	inZones(t, func(t *testing.T, loc *time.Location) {
		tasks := scheduleOf(t, loc, "dateFormat YYYY-MM-DD", "excludes weekends", "weekend friday", "test1 :id1,2024-02-28, 3d")
		if !sameTime(tasks[0].endTime, isoDay(loc, "2024-03-04")) {
			t.Errorf("end = %v, want 2024-03-04", valueOf(tasks[0].endTime))
		}
	})
}

func TestScheduleFailsWhenEveryDayIsExcluded(t *testing.T) {
	loc := time.UTC
	func() {
		defer func() {
			var e *perr.Error
			if r, _ := recover().(error); !errors.As(r, &e) || !strings.HasPrefix(e.Message, "Failed to find a valid date") {
				t.Errorf("recovered %v, want the error for a task with no valid end", r)
			}
		}()
		scheduleOf(t, loc, "dateFormat YYYY-MM-DD", "excludes weekends,monday,tuesday,wednesday,thursday,friday",
			"test1 :id1,2019-02-01,7d")
	}()
	scheduleOf(t, loc, "dateFormat YYYY-MM-DD", "excludes weekends,monday,tuesday,wednesday,thursday", "test1 :id1,2019-02-01,7d")
}

func TestExcludesMergeAcrossStatements(t *testing.T) {
	tests := map[string]struct {
		lines []string
		want  []string
	}{
		"merged":   {[]string{"excludes weekends", "excludes 2019-02-06", "excludes friday, monday"}, []string{"weekends", "2019-02-06", "friday", "monday"}},
		"deduped":  {[]string{"excludes weekends,2019-02-06", "excludes weekends 2019-02-07"}, []string{"weekends", "2019-02-06", "2019-02-07"}},
		"includes": {[]string{"includes 2019-02-06", "includes 2019-02-07,2019-02-08"}, []string{"2019-02-06", "2019-02-07", "2019-02-08"}},
	}
	for name, tc := range tests {
		db := newGanttDb()
		parseGantt("gantt\n"+strings.Join(tc.lines, "\n")+"\n", db, nil)
		got := db.excludes
		if name == "includes" {
			got = db.includes
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}

func TestScheduleRelativeToSeveralTasks(t *testing.T) {
	inZones(t, func(t *testing.T, loc *time.Location) {
		after := scheduleOf(t, loc, "dateFormat YYYY-MM-DD", "section sec1", "task1 :id1,after id2 id3 id4,1d",
			"task2 :id2,2013-01-01,1d", "task3 :id3,2013-02-01,3d", "task4 :id4,2013-02-01,2d")
		if !sameTime(after[0].endTime, at(loc, 2013, 1, 5)) {
			t.Errorf("after: end = %v, want 2013-02-05", valueOf(after[0].endTime))
		}
		until := scheduleOf(t, loc, "dateFormat YYYY-MM-DD", "section sec1", "task1 :id1,2013-01-01,until id2 id3 id4",
			"task2 :id2,2013-01-11,1d", "task3 :id3,2013-02-10,1d", "task4 :id4,2013-02-12,1d")
		if !sameTime(until[0].endTime, at(loc, 2013, 0, 11)) {
			t.Errorf("until: end = %v, want 2013-01-11", valueOf(until[0].endTime))
		}
	})
}

func TestScheduleInclusiveEndDates(t *testing.T) {
	inZones(t, func(t *testing.T, loc *time.Location) {
		tasks := scheduleOf(t, loc, "dateFormat YYYY-MM-DD", "inclusiveEndDates", "test1 :id1,2019-02-01,1d", "test2 :id2,2019-02-01,2019-02-03")
		if !sameTime(tasks[0].endTime, isoDay(loc, "2019-02-02")) {
			t.Errorf("duration: end = %v, want 2019-02-02", valueOf(tasks[0].endTime))
		}
		if !sameTime(tasks[1].endTime, isoDay(loc, "2019-02-04")) || !tasks[1].manualEndTime || tasks[1].renderEndTime != nil {
			t.Errorf("end date: end = %v, manual %v, drawn to %v; want 2019-02-04, manual, not moved",
				valueOf(tasks[1].endTime), tasks[1].manualEndTime, valueOf(tasks[1].renderEndTime))
		}
	})
}

func TestScheduleRejectsRidiculousYears(t *testing.T) {
	defer func() {
		var e *perr.Error
		if r, _ := recover().(error); !errors.As(r, &e) || e.Message != "Invalid date:202304" {
			t.Errorf("recovered %v, want Invalid date:202304", r)
		}
	}()
	scheduleOf(t, time.UTC, "dateFormat YYYYMMDD", "test1 :id1,202304,1d")
}

func TestScheduleFallsBackToTheJSDateParser(t *testing.T) {
	tasks := scheduleOf(t, time.UTC, "dateFormat YYYY-MM-DD", "section Vacation", "London Trip 1 :2024-12-01, 7d",
		"London Trip 2 :202-12-01, 7d")
	if got := []float64{tasks[0].startTime.FullYear(), tasks[1].startTime.FullYear()}; !slices.Equal(got, []float64{2024, 202}) {
		t.Errorf("years = %v, want [2024 202]", got)
	}
}
