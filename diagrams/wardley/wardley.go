// Package wardley is the Wardley map diagram type: components placed by visibility and
// evolution, with links, pipelines, trends, annotations and forces.
package wardley

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the Wardley map diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type:    "wardley",
	Section: "wardley-beta",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		return buildWardley(parseWardley(source, limits.Origin), title)
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderWardley(m.(wardleyModel), config, rc)
	},
}
