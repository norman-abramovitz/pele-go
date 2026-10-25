// Package radar is the radar diagram type: curves of values over axes around a center.
package radar

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the radar diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "radar",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		return buildRadar(parseRadar(source, limits.Origin), title)
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderRadar(m.(*radarModel), config, rc)
	},
}
