// Package treemap is the treemap diagram type: nested rectangles sized by value.
package treemap

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the treemap diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "treemap",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		return buildTreemap(parseTreemap(source, limits.Origin), title)
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderTreemap(m.(*treemapModel), config, rc)
	},
}
