// Package pie is the pie chart diagram type.
package pie

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the pie chart diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "pie",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		return buildPie(parsePie(source, limits.Origin), title)
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderPie(m.(model), config, rc)
	},
}
