// Package block is the block diagram type: blocks laid out on a grid, with composites,
// block arrows and links between them.
package block

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the block diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "block",
	Math: true,
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, _ core.Limits) any {
		db := newBlockDb()
		db.title = title
		parseBlock(source, db)
		return db
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderBlock(m.(*blockDb), config, rc)
	},
}
