// Package requirement is the requirement diagram type.
package requirement

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the requirement diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "requirement",
	Math: true,
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := newRequirementDb()
		if title != nil && *title != "" {
			db.title = *title
		}
		parseRequirement(source, db, limits.Origin)
		return db
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderRequirement(m.(*requirementDb), config, rc)
	},
}
