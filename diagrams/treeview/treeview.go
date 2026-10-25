// Package treeview is the treeView diagram type: a file tree, written with indentation or with
// box-drawing characters.
package treeview

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the treeView diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "treeView",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		return buildTreeView(readTreeView(source, limits.Origin), title)
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderTreeView(m.(treeViewModel), config, rc)
	},
}
