// Package architecture is the architecture diagram type: services, junctions and groups
// placed on a grid, joined by edges that leave and enter named sides.
package architecture

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the architecture diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "architecture",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := newArchitectureDb()
		if title != nil {
			db.title = *title
		}
		populate(parseArchitecture(source, limits.Origin), db)
		return db
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderArchitecture(m.(*architectureDb), config, rc)
	},
}
