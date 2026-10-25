// Package venn is the venn diagram type: sets drawn as circles whose areas and overlaps
// follow the sizes given, with labels and texts placed inside their regions.
package venn

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the venn diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "venn",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := newVennDb()
		if title != nil && *title != "" {
			db.title = *title
		}
		parseVenn(source, db, limits.Origin)
		return db
	},
	Render: func(model any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderVenn(model.(*vennDb), config, rc)
	},
}
