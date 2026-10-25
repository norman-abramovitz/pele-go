// Package git is the gitGraph diagram type.
package git

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the gitGraph diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "gitGraph",
	Parse: func(source string, config *jsobj.Object, title *string, _ int, limits core.Limits) any {
		return buildGit(parseGit(source, limits.Origin), config, title)
	},
	Render: func(model any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderGit(model.(*gitModel), config, rc)
	},
}
