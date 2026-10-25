// Package mindmap is the mindmap diagram type: an indented outline drawn as branches
// spreading from a root.
package mindmap

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the mindmap diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "mindmap",
	Math: true,
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, _ core.Limits) any {
		db := &mindmapDb{title: title}
		parseMindmap(source, db)
		return db
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderMindmap(m.(*mindmapDb), config, rc)
	},
}
