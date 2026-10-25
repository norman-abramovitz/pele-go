// Package cynefin is the Cynefin diagram type: the five Cynefin domains with their items, and
// transitions between them.
package cynefin

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the Cynefin diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "cynefin",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		return buildCynefin(parseCynefin(source, limits.Origin), title)
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderCynefin(m.(cynefinModel), config, rc)
	},
}
