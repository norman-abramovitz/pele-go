// Package xychart is the xychart diagram type: bar and line series over a category or
// numeric x axis.
package xychart

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the xychart diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type:    "xychart",
	Section: "xyChart",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := newXyChartDb()
		db.title = title
		parseXyChart(source, db, limits.Origin)
		return db
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderXyChart(m.(*xyChartDb), config, rc)
	},
}
