// Package swimlane is the swimlane diagram type: a flowchart whose top-level subgraphs are
// lanes, with the same syntax and model.
package swimlane

import (
	"context"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/flowchart"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/layout"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const defaultLane = "__swimlane_default__"

// prepare puts the nodes outside every lane in one unnamed lane, as Mermaid does. Lanes all
// run the same way, so a direction inside a subgraph has no effect.
func prepare(graph *flowchart.FlowGraph) {
	loose := false
	for _, node := range graph.Nodes {
		if node.IsGroup {
			node.Dir = ""
		} else if !node.HasParent {
			loose = true
		}
	}
	if !loose {
		return
	}
	for _, node := range graph.Nodes {
		if !node.IsGroup && !node.HasParent {
			node.ParentID = defaultLane
			node.HasParent = true
		}
	}
	graph.Nodes = append(graph.Nodes, &flowchart.GraphNode{
		ID:                defaultLane,
		LabelType:         "text",
		Shape:             "rect",
		IsGroup:           true,
		CSSStyles:         []string{},
		CSSCompiledStyles: []string{},
		ColorIndex:        -1,
	})
}

// drawLane draws a lane: an outline the length of the diagram, with its title in a band at
// the start.
func drawLane(c *layout.CNode, label text.Label, style svg.ResolvedStyle, classes, id string, dir layout.Dir, icons *text.Icons) string {
	left := c.X - float64(c.W/2)
	top := c.Y - float64(c.H/2)
	sideways := dir == layout.LR || dir == layout.RL
	cx, cy := c.X, top+float64(c.PadTop/2)
	bandW, bandH := c.W, c.PadTop
	if sideways {
		cx, cy = left+float64(c.PadTop/2), c.Y
		bandW, bandH = c.PadTop, c.H
	}
	labelSVG := svg.LabelSVG(label, cx, cy, ` class="pele-cluster-label" fill="var(--_m)"`+style.Text, icons)
	if sideways && labelSVG != "" {
		labelSVG = `<g transform="rotate(-90 ` + svg.Num(cx) + ` ` + svg.Num(cy) + `)">` + labelSVG + `</g>`
	}
	return `<g class="pele-cluster pele-lane` + classes + `" data-id="` + id + `">` +
		`<rect x="` + svg.Num(left) + `" y="` + svg.Num(top) + `" width="` + svg.Num(c.W) + `" height="` + svg.Num(c.H) + `" fill="none" stroke="var(--_b)"/>` +
		`<rect x="` + svg.Num(left) + `" y="` + svg.Num(top) + `" width="` + svg.Num(bandW) + `" height="` + svg.Num(bandH) +
		`" fill="var(--_a)" stroke="var(--_b)"` + style.Shape + `/>` +
		labelSVG +
		`</g>`
}

var variant = &flowchart.FlowVariant{
	Type:    "swimlane",
	Prepare: prepare,
	Layout: func(ctx context.Context, nodes []layout.CNode, edges []layout.CEdge, dir layout.Dir, opt layout.LayeredOptions, tight bool) layout.CompoundResult {
		lanePad := float64(layout.LanePad)
		if tight {
			lanePad = 12
		}
		return layout.LaneLayout(ctx, nodes, edges, dir, opt, lanePad)
	},
	DrawGroup: drawLane,
}

// Diagram is the swimlane diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type:  "swimlane",
	Math:  true,
	Parse: flowchart.Parse,
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return flowchart.RenderFlowchart(m.(*flowchart.FlowDb), config, rc, variant)
	},
}
