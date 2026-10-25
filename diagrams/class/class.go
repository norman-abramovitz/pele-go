// Package class is the class diagram type.
package class

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the class diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "class",
	Math: true,
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, _ core.Limits) any {
		db := newClassDb()
		if title != nil {
			db.title = *title
		}
		parseClassDiagram(source, db)
		return db
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderClass(m.(*classDb), config, rc)
	},
}
