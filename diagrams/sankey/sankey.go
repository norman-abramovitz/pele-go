// Package sankey is the sankey diagram type: flows between nodes drawn as bands whose
// thickness follows their value.
package sankey

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the sankey diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "sankey",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := newSankeyDb()
		if title != nil && *title != "" {
			db.title = *title
		}
		parseSankey(prepareTextForParsing(source, nil), db, source, limits.Origin)
		return db
	},
	Render: func(model any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderSankey(model.(*sankeyDb), config, rc)
	},
}
