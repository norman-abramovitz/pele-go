// Package railroad is the railroad diagram type: syntax diagrams drawn from a grammar written
// in Mermaid's own notation, EBNF, ABNF or PEG.
package railroad

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the railroad diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "railroad",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		return parseRailroad(source, title, limits.Origin)
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderRailroad(m.(*railroadDb), config, rc)
	},
}
