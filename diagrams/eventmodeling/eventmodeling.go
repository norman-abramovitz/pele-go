// Package eventmodeling is the eventmodeling diagram type.
package eventmodeling

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the eventmodeling diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type:         "eventmodeling",
	KeepComments: true,
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		return buildEventModel(parseEventModel(source, limits.Origin), title, limits.Origin)
	},
	Render: func(model any, _ *jsobj.Object, rc *core.Context) core.Rendered {
		return renderEventModel(model.(*eventModelingModel), rc)
	},
}
