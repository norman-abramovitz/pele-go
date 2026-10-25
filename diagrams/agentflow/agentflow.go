// Package agentflow is the agentflow diagram type: a flowchart of agents, tools and the flows
// that hold them, with diagnostics about what the diagram means.
package agentflow

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the agentflow diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type:         "agentflow",
	KeepComments: true,
	Parse: func(source string, config *jsobj.Object, title *string, lineOffset int, limits core.Limits) any {
		// Mermaid reads this setting from the flowchart section for agentflow too.
		inheritDir, _ := section(config, "flowchart").Get("inheritDir")
		direction, _ := config.Get("direction")
		dir, _ := direction.(string)
		db := newAgentflowDb(agentflowDbOptions{
			inheritDir: inheritDir == true,
			direction:  dir,
			lineOffset: lineOffset,
			source:     &source,
			origin:     limits.Origin,
			maxEdges:   limits.MaxEdges,
		})
		if title != nil && *title != "" {
			db.title = *title
		}
		parseAgentflow(source, db, false, limits.Origin)
		buildGraph(db, config)
		return db
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderAgentflow(m.(*agentflowDb), config, rc)
	},
}
