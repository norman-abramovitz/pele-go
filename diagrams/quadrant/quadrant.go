// Package quadrant is the quadrantChart diagram type: points placed on a chart of four
// labelled quadrants.
package quadrant

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the quadrantChart diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "quadrantChart",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := newQuadrantDb()
		db.title = title
		parseQuadrant(source, db, limits.Origin)
		return db
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderQuadrant(m.(*quadrantDb), config, rc)
	},
}
