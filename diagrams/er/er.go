// Package er is the entity relationship diagram type.
package er

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the er diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "er",
	Math: true,
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := newErDb()
		if title != nil && *title != "" {
			db.title = *title
		}
		parseEr(source, db, limits.Origin)
		return db
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderEr(m.(*erDb), config, rc)
	},
}
