// Package timeline is the timeline diagram type.
package timeline

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// Diagram is the timeline diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "timeline",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := newTimelineDb()
		if title != nil {
			db.title = *title
		}
		parseTimeline(jsstr.UTF16(source), db, limits.Origin)
		return db
	},
	Render: func(model any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderTimeline(model.(*timelineDb), config, rc)
	},
}
