package flowchart

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the flowchart diagram type.
var Diagram = &core.Diagram{
	Type:  "flowchart",
	Math:  true,
	Parse: Parse,
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return RenderFlowchart(m.(*FlowDb), config, rc, nil)
	},
}

// Parse is the flowchart parser, which swimlane shares.
func Parse(source string, config *jsobj.Object, title *string, _ int, limits core.Limits) any {
	inheritDir := false
	if flow, ok := section(config, "flowchart"); ok {
		v, _ := flow.Get("inheritDir")
		inheritDir = v == true
	}
	db := newFlowDb(flowDbOptions{InheritDir: inheritDir, MaxEdges: limits.MaxEdges})
	if title != nil && *title != "" {
		db.Title = *title
	}
	parseFlowchart(source, db)
	return db
}
