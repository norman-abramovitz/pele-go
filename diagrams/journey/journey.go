// Package journey is the user journey diagram type.
package journey

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// Diagram is the journey diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "journey",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := &journeyDb{}
		if title != nil {
			db.title = *title
		}
		parseJourney(jsstr.UTF16(source), db, limits.Origin)
		return db
	},
	Render: func(model any, _ *jsobj.Object, rc *core.Context) core.Rendered {
		return renderJourney(model.(*journeyDb), rc)
	},
}
