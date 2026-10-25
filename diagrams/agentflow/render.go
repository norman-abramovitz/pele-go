package agentflow

import (
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/flowchart"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/layout"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// palette is the number of series tokens, which is the palette Mermaid's slot rules are
// applied to.
const palette = 8

const (
	nodeSep   = 40
	edgeSep   = 16
	rankSep   = 48
	groupPad  = 20
	loopReach = 26
	dots      = 14
	collapsed = "collapsed"
)

// flowEnd is the side a flow runs toward, with sides numbered clockwise from the top.
var flowEnd = map[layout.Dir]int{layout.TB: 2, layout.RL: 3, layout.BT: 0, layout.LR: 1}

// section is config[key] when it is a plain object.
func section(config *jsobj.Object, key string) *jsobj.Object {
	v, _ := config.Get(key)
	obj, _ := v.(*jsobj.Object)
	return obj
}

// buildGraph builds the graph a model draws. Building it also records the model's shape
// and containment diagnostics, so the parser does it once.
func buildGraph(db *agentflowDb, config *jsobj.Object) *agentGraph {
	if db.graph == nil {
		var curve any
		if v, _ := section(config, "flowchart").Get("curve"); v != nil {
			if s, ok := v.(string); ok {
				curve = s
			}
		}
		db.graph = buildAgentGraph(db, palette, curve)
	}
	return db.graph
}

func numberOption(config *jsobj.Object, key string, fallback float64) float64 {
	value, _ := section(config, "agentflow").Get(key)
	if v, ok := value.(float64); ok && v > 0 {
		return min(v, 2000)
	}
	return fallback
}

// tint is a palette color as an outline with a light wash of the same color inside. A fill
// the author set replaces the wash and stays opaque.
func tint(slot int, wash string, style svg.ResolvedStyle) string {
	color := svg.SeriesColor(slot)
	opacity := ` fill-opacity="` + wash + `"`
	// /[";]fill:/
	if strings.Contains(style.Shape, `"fill:`) || strings.Contains(style.Shape, ";fill:") {
		opacity = ""
	}
	return ` fill="` + color + `"` + opacity + ` stroke="` + color + `"`
}

type nodeView struct {
	node  *graphNode
	style svg.ResolvedStyle
	label text.Label
	shape string
	w, h  float64
	dy    float64
	loops int
}

var flowchartClasses = strings.NewReplacer("edge-thickness-normal", "", "edge-pattern-solid", "", "flowchart-link", "")

func renderAgentflow(db *agentflowDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	if !common.RunsAcross(db.direction) {
		return draw(db, config, rc, false)
	}
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered { return draw(db, config, rc, down) })
}

// curveName is the curve an edge names: only a string names one.
func curveName(curve any) string {
	s, _ := curve.(string)
	return s
}

// draw draws the diagram; turned draws one that runs across as one that runs down.
func draw(db *agentflowDb, config *jsobj.Object, rc *core.Context, turned bool) core.Rendered {
	options := rc.Options
	graph := buildGraph(db, config)
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	edgeSize := jsmath.Round(size * 0.875)
	measurer := rc.Measurer()
	wrapWidth := numberOption(config, "wrappingWidth", 120)
	minWidth := numberOption(config, "minNodeWidth", 120)
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	icons := rc.Icons

	index := map[string]int{}
	views := make([]nodeView, 0, len(graph.nodes))
	cnodes := make([]layout.CNode, 0, len(graph.nodes))
	for i, node := range graph.nodes {
		index[node.id] = i
	}

	for _, node := range graph.nodes {
		declarations := node.cssCompiledStyles
		if len(node.cssStyles) > 0 {
			declarations = append(append([]string{}, node.cssCompiledStyles...), node.cssStyles...)
		}
		style := svg.ResolveStyle(declarations)
		var textStyle text.Style
		if style.Bold {
			textStyle |= text.Bold
		}
		if style.Italic {
			textStyle |= text.Italic
		}
		markdown := node.labelType == "markdown"
		fontSize := size
		if style.FontSize != 0 {
			fontSize = style.FontSize
		}
		var view nodeView
		var c layout.CNode
		switch {
		case node.isGroup:
			label := text.Layout(node.label, markdown, measurer, fontSize, 4000, textStyle, icons)
			view = nodeView{node: node, style: style, label: label, shape: "group"}
			c = layout.NewCNode(0, 0, -1)
			c.IsGroup = true
			if node.dir != "" {
				c.Dir = layout.Direction(node.dir)
			}
			c.PadX = groupPad
			c.PadTop = groupPad
			if label.Height > 0 {
				c.PadTop = label.Height + 16
			}
			c.PadBottom = groupPad
			c.MinW = label.Width + 2*groupPad - 12
		case node.shape == "collapsedGroup":
			label := text.Layout(node.label, markdown, measurer, fontSize, wrapWidth, textStyle, icons)
			w := max(label.Width, minWidth) + 2*svg.PadX
			h := label.Height + 2*svg.PadY + dots
			view = nodeView{node: node, style: style, label: label, shape: collapsed, w: w, h: h, dy: -dots / 2}
			c = layout.NewCNode(w, h, -1)
		default:
			shape := flowchart.CanonicalShape(node.shape)
			label := text.Layout(node.label, markdown, measurer, fontSize, wrapWidth, textStyle, icons)
			// A diamond grows twice as fast as its label, so it keeps the label's own width.
			tw := max(label.Width, minWidth)
			if shape == "diam" {
				tw = label.Width
			}
			s := svg.ShapeSizeOf(shape, tw, label.Height)
			view = nodeView{node: node, style: style, label: label, shape: shape, w: s.W, h: s.H, dy: s.DY}
			c = layout.NewCNode(s.W, s.H, -1)
		}
		c.Seq = float64(len(views))
		views = append(views, view)
		cnodes = append(cnodes, c)
	}
	for i, node := range graph.nodes {
		cnodes[i].Parent = -1
		if node.hasParent {
			if p, ok := index[node.parentID]; ok {
				cnodes[i].Parent = p
			}
		}
	}

	rootDir := layout.TB
	if !turned {
		rootDir = layout.Direction(db.direction)
	}
	// flowOf is the direction of the level a node sits in.
	flowOf := func(i int) layout.Dir {
		var dir layout.Dir
		for p, hops := cnodes[i].Parent, 0; p >= 0 && dir == "" && hops < 10000; p, hops = cnodes[p].Parent, hops+1 {
			dir = cnodes[p].Dir
		}
		if dir == "" {
			return rootDir
		}
		return dir
	}
	flowsSideways := func(i int) bool {
		dir := flowOf(i)
		return dir == layout.LR || dir == layout.RL
	}

	depth := make([]int, len(cnodes))
	for i := range depth {
		depth[i] = -1
	}
	for i := range cnodes {
		var chain []int
		j := i
		for j >= 0 && depth[j] < 0 && len(chain) <= len(cnodes) {
			chain = append(chain, j)
			j = cnodes[j].Parent
		}
		d := 0
		if j >= 0 {
			d = depth[j] + 1
		}
		for k := len(chain) - 1; k >= 0; k-- {
			depth[chain[k]] = d
			d++
		}
	}
	contains := func(group, i int) bool {
		j := i
		for d := depth[i]; d > depth[group]; d-- {
			j = cnodes[j].Parent
		}
		return j == group && i != group
	}

	var cedges []layout.CEdge
	var edgeLabels []text.Label
	var drawn []*graphEdge
	// held are the edges between a node and a container around it. The layout has no place
	// for them, so they are drawn afterwards, between the node and the container's border.
	type heldEdge struct {
		edge     *graphEdge
		label    text.Label
		src, dst int
	}
	var held []heldEdge
	for _, edge := range graph.edges {
		src, okSrc := index[edge.start]
		dst, okDst := index[edge.end]
		if !okSrc || !okDst {
			continue
		}
		label := text.Layout(edge.label, edge.labelType == "markdown", measurer, edgeSize, wrapWidth, 0, icons)
		if contains(src, dst) || contains(dst, src) {
			held = append(held, heldEdge{edge, label, src, dst})
			continue
		}
		if src == dst {
			// A self-loop sits beside the node, across the flow, and the node's box grows to hold it.
			views[src].loops++
			if flowsSideways(src) {
				extra := 0.0
				if label.Height > 0 {
					extra = label.Height + 8
				}
				cnodes[src].H += float64(2 * (loopReach + extra))
			} else {
				extra := 0.0
				if label.Width > 0 {
					extra = label.Width + 12
				}
				cnodes[src].W += float64(2 * (loopReach + extra))
			}
		}
		drawn = append(drawn, edge)
		edgeLabels = append(edgeLabels, label)
		minlen := edge.minlen
		if minlen == 0 {
			minlen = 1
		}
		ce := layout.CEdge{Src: src, Dst: dst, Minlen: minlen, Route: []float64{}}
		if label.Width > 0 {
			ce.LabelW = label.Width + 12
		}
		if label.Height > 0 {
			ce.LabelH = label.Height + 4
		}
		cedges = append(cedges, ce)
	}

	result := layout.CompoundLayout(rc.Ctx, cnodes, cedges, rootDir, layout.LayeredOptions{
		NodeSep: numberOption(config, "nodeSpacing", nodeSep),
		EdgeSep: edgeSep,
		RankSep: numberOption(config, "rankSpacing", rankSep),
		PortSep: 20,
	})

	title := text.Layout(db.title, false, measurer, size, 4000, text.Bold, icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}
	inner := max(result.Width, title.Width)
	ox := pad + float64((inner-result.Width)/2)
	oy := pad + titleHeight
	layout.ShiftLayout(cnodes, cedges, ox, oy)
	width := math.Ceil(inner + float64(2*pad))
	height := math.Ceil(result.Height + titleHeight + float64(2*pad))
	links := []core.Link{}

	// An edge between a node and a container around it runs straight between the node and
	// the container's border: out along the flow, or in against it. If another node is in the
	// way it takes a side instead, and failing that the far end.
	var heldPaths []svg.EdgePath
	var heldLabels []float64
	var heldRoutes [][]float64
	careful := len(held)*len(cnodes) < 2e6
	for _, h := range held {
		out := contains(h.dst, h.src)
		outer, i := h.src, h.dst
		if out {
			outer, i = h.dst, h.src
		}
		g := &cnodes[outer]
		n := &cnodes[i]
		dir := g.Dir
		if dir == "" {
			dir = flowOf(outer)
		}
		end := flowEnd[dir]
		first := (end + 2) & 3
		if out {
			first = end
		}
		toward := func(side int) []float64 {
			upright := side&1 == 0
			sign := 1.0
			if side == 0 || side == 3 {
				sign = -1
			}
			var nx, ny, bx, by, axis float64
			if upright {
				nx, ny = n.X, n.Y+float64(float64(sign*n.H)/2)
				bx, by = n.X, g.Y+float64(float64(sign*g.H)/2)
			} else {
				nx, ny = n.X+float64(float64(sign*n.W)/2), n.Y
				bx, by = g.X+float64(float64(sign*g.W)/2), n.Y
				axis = 1
			}
			if out {
				return []float64{nx, ny, axis, bx, by, axis}
			}
			return []float64{bx, by, axis, nx, ny, axis}
		}
		blocked := func(r []float64) bool {
			for k := range cnodes {
				c := &cnodes[k]
				if k != i && !c.IsGroup &&
					c.X+float64(c.W/2) > min(r[0], r[3])-1 &&
					c.X-float64(c.W/2) < max(r[0], r[3])+1 &&
					c.Y+float64(c.H/2) > min(r[1], r[4])-1 &&
					c.Y-float64(c.H/2) < max(r[1], r[4])+1 {
					return true
				}
			}
			return false
		}
		route := toward(first)
		if careful && blocked(route) {
			for _, side := range [...]int{(first + 1) & 3, (first + 3) & 3, (first + 2) & 3} {
				if other := toward(side); !blocked(other) {
					route = other
					break
				}
			}
		}
		heldRoutes = append(heldRoutes, route)
		heldLabels = append(heldLabels, (route[0]+route[3])/2, (route[1]+route[4])/2)
		drawnRoute := append([]float64{}, route...)
		at := 3
		if out {
			at = 0
		}
		svg.InsetRoute(drawnRoute, at, views[i].shape, views[i].w, views[i].h, n)
		trim := svg.MarkerTrim(h.edge.arrowTypeEnd)
		heldPaths = append(heldPaths, svg.RoutePath(drawnRoute, "linear", 0, trim, svg.DefaultReach(0), svg.DefaultReach(trim)))
	}

	crossings := svg.Crossings{}
	anyGroup := false
	for i := range cnodes {
		if cnodes[i].IsGroup {
			anyGroup = true
		}
	}
	if anyGroup {
		for i := range cedges {
			svg.MarkCrossings(crossings, cedges[i].Route)
		}
		for _, route := range heldRoutes {
			svg.MarkCrossings(crossings, route)
		}
	}

	var clusters, nodesOut strings.Builder
	for i := range views {
		view := &views[i]
		c := &cnodes[i]
		node := view.node
		x, y := c.X, c.Y
		classes := svg.ClassNames(svg.StripDefault(node.cssClasses))
		id := svg.EscText(node.id)
		colorIndex := node.colorIndex
		if colorIndex < 0 {
			colorIndex = kindCount
		}
		if c.IsGroup {
			titleX := svg.ClusterTitleX(c, view.label.Width, crossings, groupPad)
			clusters.WriteString(`<g class="pele-cluster pele-flow` + classes + `" data-id="` + id + `">` +
				`<rect x="` + svg.Num(x-float64(c.W/2)) + `" y="` + svg.Num(y-float64(c.H/2)) + `" width="` + svg.Num(c.W) + `" height="` + svg.Num(c.H) +
				`"` + svg.Rounded(tint(colorIndex, "0.06", view.style)+view.style.Shape) + `/>` +
				svg.LabelSVG(view.label, titleX, y-float64(c.H/2)+8+float64(view.label.Height/2), ` class="pele-cluster-label" fill="var(--_m)"`+view.style.Text, icons) +
				`</g>`)
			continue
		}

		w, h := view.w, view.h
		kind := ""
		var shape string
		if view.shape == collapsed {
			color := svg.SeriesColor(colorIndex)
			line := float64(h/2) - dots - 2
			mid := line + (dots+2)/2
			shape = `<rect x="` + svg.Num(-w/2) + `" y="` + svg.Num(-h/2) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) +
				`"` + svg.Rounded(tint(colorIndex, "0.06", view.style)+view.style.Shape) + `/>` +
				`<path d="M` + svg.Num(float64(-w/2)+10) + `,` + svg.Num(line) + `H` + svg.Num(float64(w/2)-10) + `" fill="none" stroke="` + color +
				`" stroke-dasharray="3 3"` + view.style.Line + `/>` +
				`<path class="pele-collapsed-mark" d="` + dot(-9, mid) + dot(0, mid) + dot(9, mid) + `" fill="` + color + `" stroke="none"/>`
		} else {
			nodeKind := node.kind
			if nodeKind == "" {
				nodeKind = "task"
			}
			slot, ok := kindSlot[nodeKind]
			if !ok {
				slot = 1
			}
			kind = " pele-kind-" + nodeKind
			shape = svg.DrawShape(view.shape, w, h, tint(slot, "0.14", view.style)+view.style.Shape,
				` stroke="`+svg.SeriesColor(slot)+`"`+view.style.Line)
		}

		label := svg.LabelSVG(view.label, 0, view.dy, ` class="pele-label"`+view.style.Text, icons)
		internal := strings.Contains(classes+" ", " internal-link ")
		if internal && node.label != "" && (options.Links == nil || *options.Links) {
			// The label is a note name, but it still ends up in an href, so it gets the same check as a URL.
			name := text.DecodeEntities(node.label)
			safe := util.InternalLink(name, options)
			target := svg.Esc(name)
			href := ""
			if safe {
				href = ` href="` + target + `"` + util.RelAttr(options)
			}
			label = `<a class="internal-link"` + href + ` data-href="` + target + `">` + label + `</a>`
			links = append(links, core.Link{ID: node.id, Href: name, Internal: true})
		}
		body := shape + label
		if node.tooltip != nil && *node.tooltip != "" {
			body = `<title>` + svg.EscText(*node.tooltip) + `</title>` + body
		}
		if node.link != nil && *node.link != "" {
			attrs := ""
			if node.linkTarget != nil && *node.linkTarget != "" {
				attrs = ` target="` + svg.Esc(*node.linkTarget) + `"`
			}
			body = util.Linked(body, *node.link, node.id, &links, options, attrs)
		}
		nodesOut.WriteString(`<g class="pele-node pele-shape-` + view.shape + kind + classes + `" data-id="` + id + `" transform="translate(` +
			svg.Num(x) + `,` + svg.Num(y) + `)">` + body + `</g>`)
	}

	var edgesOut, labelsOut strings.Builder
	put := func(edge *graphEdge, label text.Label, path svg.EdgePath, x, y float64) {
		var style svg.ResolvedStyle
		if len(edge.style) > 0 || len(edge.cssCompiledStyles) > 0 {
			style = svg.ResolveStyle(append(append([]string{}, edge.cssCompiledStyles...), edge.style...))
		}
		endType := edge.arrowTypeEnd
		color := "var(--_l)"
		if style.Stroke != "" {
			color = svg.Esc(style.Stroke)
		}
		kind := "sequence"
		if edge.thickness == "dotted" {
			kind = "reference"
		} else if endType == "arrow_cross" {
			kind = "failure"
		}
		dash := ""
		if edge.thickness == "dotted" {
			dash = ` stroke-dasharray="3 4"`
		}
		classes := svg.ClassNames(flowchartClasses.Replace(edge.classes))
		edgesOut.WriteString(`<g class="pele-edge pele-edge-` + kind + classes + `" data-id="` + svg.EscText(edge.id) + `"` + style.Line + `>` +
			`<path d="` + path.D + `"` + dash + `/>` +
			svg.Marker(endType, path.EX, path.EY, path.EDX, path.EDY, color) +
			`</g>`)
		if label.Width > 0 {
			labelsOut.WriteString(svg.EdgeLabelSVG(edge.id, label, x, y, style.Text, icons))
		}
	}

	loopSeen := map[int]int{}
	for i := range cedges {
		ce := &cedges[i]
		edge := drawn[i]
		endType := edge.arrowTypeEnd
		var path svg.EdgePath
		if ce.Src == ce.Dst {
			view := views[ce.Src]
			k := loopSeen[ce.Src]
			loopSeen[ce.Src] = k + 1
			c := &cnodes[ce.Src]
			path, ce.LabelX, ce.LabelY = svg.LoopPath(c.X, c.Y, view.w, view.h, float64(k), flowsSideways(ce.Src), loopReach,
				svg.MarkerTrim(endType), ce.LabelW, ce.LabelH)
		} else {
			route := append([]float64{}, ce.Route...)
			src, dst := views[ce.Src], views[ce.Dst]
			svg.InsetRoute(route, 0, src.shape, src.w, src.h, &cnodes[ce.Src])
			svg.InsetRoute(route, len(route)-3, dst.shape, dst.w, dst.h, &cnodes[ce.Dst])
			trim := svg.MarkerTrim(endType)
			path = svg.RoutePath(route, curveName(edge.curve), 0, trim, svg.DefaultReach(0), svg.DefaultReach(trim))
		}
		put(edge, edgeLabels[i], path, ce.LabelX, ce.LabelY)
	}
	for i, h := range held {
		put(h.edge, h.label, heldPaths[i], heldLabels[2*i], heldLabels[2*i+1])
	}

	var content strings.Builder
	content.WriteString(svg.LabelSVG(title, width/2, pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil))
	if clusters.Len() > 0 {
		content.WriteString(`<g class="pele-clusters">` + clusters.String() + `</g>`)
	}
	if edgesOut.Len() > 0 {
		content.WriteString(`<g class="pele-edges" fill="none" stroke="var(--_l)" stroke-linecap="round">` + edgesOut.String() + `</g>`)
	}
	if labelsOut.Len() > 0 {
		content.WriteString(`<g class="pele-edge-labels" font-size="` + jsnum.Format(edgeSize) + `">` + labelsOut.String() + `</g>`)
	}
	content.WriteString(`<g class="pele-nodes">` + nodesOut.String() + `</g>`)

	acc := svg.Accessible{}
	if db.accTitle != nil {
		acc.AccTitle = *db.accTitle
	}
	if db.accDescr != nil {
		acc.AccDescr = *db.accDescr
	}
	doc := svg.Document("agentflow", width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, acc, content.String())
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: links}
}

func dot(x, y float64) string {
	return `M` + svg.Num(x-1.75) + `,` + svg.Num(y) + `a1.75,1.75 0 1 0 3.5,0a1.75,1.75 0 1 0 -3.5,0Z`
}
