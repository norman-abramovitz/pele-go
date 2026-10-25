// Package c4 is the C4 diagram type: Mermaid's C4Context, C4Container, C4Component,
// C4Dynamic and C4Deployment diagrams.
package c4

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the c4 diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "c4",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := newC4Db()
		if title != nil && *title != "" {
			db.title = *title
		}
		parseC4(source, db, limits.Origin)
		return db
	},
	Render: func(m any, _ *jsobj.Object, rc *core.Context) core.Rendered {
		return renderC4(m.(*c4Db), rc)
	},
}
