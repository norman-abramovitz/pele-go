// Package gantt is the gantt diagram type: tasks scheduled on a time axis, read in the
// render's time zone.
package gantt

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsdate"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the gantt diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "gantt",
	Parse: func(source string, config *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := newGanttDb()
		if title != nil && *title != "" {
			db.title = *title
		}
		if gantt, ok := config.Get("gantt"); ok {
			if section, isObj := gantt.(*jsobj.Object); isObj {
				if mode, has := section.Get("displayMode"); has {
					if s, isStr := mode.(string); isStr {
						db.displayMode = s
					}
				}
			}
		}
		parseGantt(source, db, limits.Origin)
		return db
	},
	Render: func(model any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderGantt(model.(*ganttDb), config, rc)
	},
}

// task is upstream's GanttTask. Upstream's startTime and endTime are undefined, and
// renderEndTime null, where these are nil.
type task struct {
	id string
	// task is the task's label, under the name Mermaid gives it.
	task          string
	section       string
	startTime     *jsdate.Date
	endTime       *jsdate.Date
	renderEndTime *jsdate.Date
	manualEndTime bool
	processed     bool
	active        bool
	done          bool
	crit          bool
	milestone     bool
	vert          bool
	// order is the row index in source order; -1 for vertical markers, which take no row.
	order   int
	classes []string
	// prevTaskID is the id of the task before, or nil for the first.
	prevTaskID *string
	raw        rawTask
}

type rawTask struct {
	data string
	// start is nil when the task gives no start.
	start *string
	end   string
}
