// Package kanban is the kanban diagram type.
package kanban

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// Diagram is the kanban diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "kanban",
	Math: true,
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := &kanbanDb{}
		if title != nil {
			db.title = *title
		}
		parseKanban(jsstr.UTF16(source), db, limits.Origin)
		return db
	},
	Render: func(model any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderKanban(model.(*kanbanDb), config, rc)
	},
}
