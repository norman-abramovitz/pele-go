// Package state is the state diagram type.
package state

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the state diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "state",
	Math: true,
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := newStateDb()
		if title != nil && *title != "" {
			db.title = *title
		}
		parseState(source, db, limits.Origin)
		return db
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderState(m.(*stateDb), config, rc)
	},
}
