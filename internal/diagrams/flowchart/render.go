package flowchart

import (
	"context"
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/layout"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

const (
	nodeSep    = 40
	edgeSep    = 16
	rankSep    = 48
	groupPad   = 20
	loopReach  = 26
	shapeAttrs = ` fill="var(--_s)" stroke="var(--_b)"`
	lineAttrs  = ` stroke="var(--_b)"`
)

func numberOption(section *jsobj.Object, key string, fallback float64) float64 {
	value, _ := section.Get(key)
	if v, ok := value.(float64); ok && v > 0 {
		return min(v, 2000)
	}
	return fallback
}

// FlowVariant is a diagram type that is a flowchart with a different layout, which may draw
// its top-level groups itself. DrawGroup is nil when it does not.
type FlowVariant struct {
	Type      string
	Prepare   func(graph *FlowGraph)
	Layout    func(ctx context.Context, nodes []layout.CNode, edges []layout.CEdge, dir layout.Dir, opt layout.LayeredOptions, tight bool) layout.CompoundResult
	DrawGroup func(c *layout.CNode, label text.Label, style svg.ResolvedStyle, classes, id string, dir layout.Dir, icons *text.Icons) string
}

func assetSize(value, fallback float64) float64 {
	if value > 0 && value <= 4000 {
		return value
	}
	return fallback
}

type nodeView struct {
	node  *GraphNode
	style svg.ResolvedStyle
	label text.Label
	shape string
	w, h  float64
	dy    float64
	loops int
}

// RenderFlowchart draws a flowchart, or with a variant a diagram type built on one.
func RenderFlowchart(db *FlowDb, config *jsobj.Object, rc *core.Context, variant *FlowVariant) core.Rendered {
	fitted := func(down bool) core.Rendered {
		return common.Tighten(rc.Options, func(tight bool) core.Rendered { return draw(db, config, rc, variant, down, tight) })
	}
	if common.RunsAcross(db.Direction) {
		return common.TurnToFit(rc.Options, fitted)
	}
	return fitted(false)
}

// section is config[key] when it is a plain object.
func section(config *jsobj.Object, key string) (*jsobj.Object, bool) {
	v, _ := config.Get(key)
	obj, ok := v.(*jsobj.Object)
	return obj, ok
}

var flowchartClasses = strings.NewReplacer("edge-thickness-normal", "", "edge-pattern-solid", "", "flowchart-link", "")

// draw draws a flowchart; turned draws one that runs across as one that runs down, and tight
// sets nodes closer together.
func draw(db *FlowDb, config *jsobj.Object, rc *core.Context, variant *FlowVariant, turned, tight bool) core.Rendered {
	options := rc.Options
	pad := float64(groupPad)
	if tight {
		pad = 12
	}
	groupPadding := pad
	var flow *jsobj.Object
	if variant != nil {
		flow, _ = section(config, variant.Type)
	}
	if flow == nil {
		var ok bool
		if flow, ok = section(config, "flowchart"); !ok {
			flow = jsobj.New()
		}
	}
	curveValue, _ := flow.Get("curve")
	curve, curveIsString := curveValue.(string)
	graph := buildFlowGraph(db, curve)
	if variant != nil {
		variant.Prepare(graph)
	}
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	edgeSize := jsmath.Round(size * 0.875)
	measurer := rc.Measurer()
	wrapWidth := numberOption(flow, "wrappingWidth", 200)
	if variant != nil && tight {
		wrapWidth = min(wrapWidth, 100)
	}
	padding := 8.0
	if options.Padding != nil {
		padding = *options.Padding
	}
	icons := rc.Icons

	index := map[string]int{}
	views := make([]nodeView, 0, len(graph.Nodes))
	cnodes := make([]layout.CNode, 0, len(graph.Nodes))
	for i, node := range graph.Nodes {
		index[node.ID] = i
	}

	for _, node := range graph.Nodes {
		declarations := node.CSSCompiledStyles
		if len(node.CSSStyles) > 0 {
			declarations = append(append([]string{}, node.CSSCompiledStyles...), node.CSSStyles...)
		}
		style := svg.ResolveStyle(declarations)
		var textStyle text.Style
		if style.Bold {
			textStyle |= text.Bold
		}
		if style.Italic {
			textStyle |= text.Italic
		}
		markdown := node.LabelType == "markdown"
		fontSize := size
		if style.FontSize != 0 {
			fontSize = style.FontSize
		}
		var view nodeView
		var c layout.CNode
		if node.IsGroup {
			maxWidth := 4000.0
			if variant != nil && tight {
				maxWidth = max(80, common.TitleRoom(options)/2)
			}
			label := text.Layout(node.Label, markdown, measurer, fontSize, maxWidth, textStyle, icons)
			view = nodeView{node: node, style: style, label: label, shape: "group"}
			c = layout.NewCNode(0, 0, -1)
			c.IsGroup = true
			if node.Dir != "" {
				c.Dir = layout.Direction(node.Dir)
			}
			c.PadX = groupPadding
			c.PadTop = groupPad
			if label.Height > 0 {
				c.PadTop = label.Height + 16
			}
			c.PadBottom = groupPadding
			c.MinW = label.Width + float64(2*groupPadding) - 12
		} else {
			shape := CanonicalShape(node.Shape)
			labelText := ""
			if node.Img != "" || node.Icon != "" || svg.ShapeHasLabel(shape) {
				labelText = node.Label
			}
			label := text.Layout(labelText, markdown, measurer, fontSize, wrapWidth, textStyle, icons)
			var w, h, dy float64
			if node.Img != "" || node.Icon != "" {
				side := 48.0
				if node.Img != "" {
					side = 80
				}
				aw := assetSize(node.AssetWidth, side)
				ah := assetSize(node.AssetHeight, side)
				w = max(aw, label.Width+8)
				h = ah
				if label.Height > 0 {
					h += label.Height + 6
				}
				dy = ah / 2
				if node.Pos == "t" {
					dy = -ah / 2
				}
				if label.Height == 0 {
					dy = 0
				}
			} else {
				s := svg.ShapeSizeOf(shape, label.Width, label.Height)
				w, h, dy = s.W, s.H, s.DY
			}
			view = nodeView{node: node, style: style, label: label, shape: shape, w: w, h: h, dy: dy}
			c = layout.NewCNode(w, h, -1)
		}
		c.Seq = float64(len(views))
		views = append(views, view)
		cnodes = append(cnodes, c)
	}
	for i, node := range graph.Nodes {
		cnodes[i].Parent = -1
		if node.HasParent {
			if p, ok := index[node.ParentID]; ok {
				cnodes[i].Parent = p
			}
		}
	}

	rootDir := layout.TB
	if !turned {
		rootDir = layout.Direction(db.Direction)
	}
	flowsSideways := func(i int) bool {
		var dir layout.Dir
		for p, hops := cnodes[i].Parent, 0; p >= 0 && dir == "" && hops < 10000; p, hops = cnodes[p].Parent, hops+1 {
			dir = cnodes[p].Dir
		}
		if dir == "" {
			dir = rootDir
		}
		return dir == layout.LR || dir == layout.RL
	}
	// The edges on one side of a node spread along it; a picture or an icon keeps them at its middle.
	for i, view := range views {
		if view.node.IsGroup {
			cnodes[i].Span = math.Inf(1)
		} else if view.node.Img == "" && view.node.Icon == "" {
			cnodes[i].Span = svg.ShapeSpan(view.shape, view.w, view.h, flowsSideways(i))
		}
	}

	var cedges []layout.CEdge
	var edgeLabels []text.Label
	var drawn []*GraphEdge
	for _, edge := range graph.Edges {
		src, okSrc := index[edge.Start]
		dst, okDst := index[edge.End]
		if !okSrc || !okDst {
			continue
		}
		label := text.Layout(edge.Label, edge.LabelType == "markdown", measurer, edgeSize, wrapWidth, 0, icons)
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
		minlen := edge.Minlen
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

	nodeSpacing := numberOption(flow, "nodeSpacing", nodeSep)
	rankSpacing := numberOption(flow, "rankSpacing", rankSep)
	if tight {
		nodeSpacing *= 0.5
		rankSpacing *= 0.75
	}
	layoutOptions := layout.LayeredOptions{
		NodeSep:  nodeSpacing,
		EdgeSep:  edgeSep,
		RankSep:  rankSpacing,
		PortSep:  20,
		Tracks:   curveIsString && strings.HasPrefix(curve, "step"),
		Straight: curveIsString && curve == "linear",
		HeadRoom: svg.MarkerTrim("arrow_point"),
	}
	var result layout.CompoundResult
	if variant != nil {
		result = variant.Layout(rc.Ctx, cnodes, cedges, rootDir, layoutOptions, tight)
	} else {
		result = layout.CompoundLayout(rc.Ctx, cnodes, cedges, rootDir, layoutOptions)
	}

	titleWidth := 4000.0
	if tight {
		titleWidth = common.TitleRoom(options)
	}
	title := svg.TitleLabel(db.Title, measurer, size, titleWidth, icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}
	inner := max(result.Width, title.Width)
	ox := padding + float64((inner-result.Width)/2)
	oy := padding + titleHeight
	layout.ShiftLayout(cnodes, cedges, ox, oy)
	width := math.Ceil(inner + float64(2*padding))
	height := math.Ceil(result.Height + titleHeight + float64(2*padding))
	links := []core.Link{}

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
	}
	parentOf := func(i int) int { return cnodes[i].Parent }

	var clusters, struckTitles, nodesOut strings.Builder
	for i := range views {
		view := &views[i]
		c := &cnodes[i]
		node := view.node
		x, y := c.X, c.Y
		classes := svg.ClassNames(svg.StripDefault(node.CSSClasses))
		id := svg.EscText(node.ID)
		if c.IsGroup && variant != nil && variant.DrawGroup != nil && c.Parent < 0 {
			clusters.WriteString(variant.DrawGroup(c, view.label, view.style, classes, id, rootDir, icons))
			continue
		}
		if c.IsGroup {
			titleX := svg.ClusterTitleX(c, view.label.Width, crossings, groupPadding)
			titleY := y - float64(c.H/2) + 8 + float64(view.label.Height/2)
			titleAttrs := ` class="pele-cluster-label" fill="var(--_m)"` + view.style.Text
			struck := svg.TitleCrossed(c, view.label.Width, titleX, crossings)
			if struck {
				struckTitles.WriteString(svg.StruckTitle(view.label, titleX, titleY, titleAttrs, id, parentOf, i, icons))
			}
			clusters.WriteString(`<g class="pele-cluster` + classes + `" data-id="` + id + `">` +
				`<rect x="` + svg.Num(x-float64(c.W/2)) + `" y="` + svg.Num(y-float64(c.H/2)) + `" width="` + svg.Num(c.W) + `" height="` + svg.Num(c.H) +
				`" fill="var(--_a)" fill-opacity="0.5" stroke="var(--_b)"` + svg.Rounded(view.style.Shape) + `/>`)
			if !struck {
				clusters.WriteString(svg.LabelSVG(view.label, titleX, titleY, titleAttrs, icons))
			}
			clusters.WriteString(`</g>`)
			continue
		}

		w, h := view.w, view.h
		var body string
		switch {
		case node.Img != "":
			aw := assetSize(node.AssetWidth, 80)
			ah := assetSize(node.AssetHeight, 80)
			top := -ah / 2
			if view.label.Height > 0 {
				top = -h / 2
				if node.Pos == "t" {
					top = float64(h/2) - ah
				}
			}
			if src, ok := util.ImageURL(util.SafeURL(node.Img), options); ok {
				aspect := "none"
				if node.Constraint == "on" {
					aspect = "xMidYMid meet"
				}
				body = `<image href="` + svg.Esc(src) + `" x="` + svg.Num(-aw/2) + `" y="` + svg.Num(top) + `" width="` + svg.Num(aw) +
					`" height="` + svg.Num(ah) + `" preserveAspectRatio="` + aspect + `"/>`
			}
		case node.Icon != "":
			side := assetSize(node.AssetHeight, 48)
			top := -side / 2
			if view.label.Height > 0 {
				top = -h / 2
				if node.Pos == "t" {
					top = float64(h/2) - side
				}
			}
			frame := ""
			switch node.Shape {
			case "icon":
			case "iconCircle":
				frame = `<circle cy="` + svg.Num(top+float64(side/2)) + `" r="` + svg.Num(side/2) + `"` + shapeAttrs + view.style.Shape + `/>`
			default:
				rx := "0"
				if node.Shape == "iconRounded" {
					rx = "8"
				}
				frame = `<rect x="` + svg.Num(-side/2) + `" y="` + svg.Num(top) + `" width="` + svg.Num(side) + `" height="` + svg.Num(side) +
					`" rx="` + rx + `"` + shapeAttrs + view.style.Shape + `/>`
			}
			glyph := float64(side * 0.6)
			body = frame + `<svg class="pele-icon" data-icon="` + svg.EscText(node.Icon) + `" x="` + svg.Num(-glyph/2) + `" y="` +
				svg.Num(top+float64((side-glyph)/2)) + `" width="` + svg.Num(glyph) + `" height="` + svg.Num(glyph) +
				`" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
				icons.Resolve(node.Icon) + `</svg>`
		default:
			body = svg.DrawShape(view.shape, w, h, shapeAttrs+view.style.Shape, lineAttrs+view.style.Line)
		}

		label := svg.LabelSVG(view.label, 0, view.dy, ` class="pele-label"`+view.style.Text, icons)
		internal := strings.Contains(classes+" ", " internal-link ")
		if internal && node.Label != "" && (options.Links == nil || *options.Links) {
			// The label is a note name, but it still ends up in an href, so it gets the same check as a URL.
			name := text.DecodeEntities(node.Label)
			_, safe := util.LinkURL(util.SanitizeURL(name), options)
			target := svg.Esc(name)
			href := ""
			if safe {
				href = ` href="` + target + `"` + util.RelAttr(options)
			}
			label = `<a class="internal-link"` + href + ` data-href="` + target + `">` + label + `</a>`
			links = append(links, core.Link{ID: node.ID, Href: name, Internal: true})
		}
		body += label
		if node.Tooltip != "" {
			body = `<title>` + svg.EscText(node.Tooltip) + `</title>` + body
		}
		if node.Link != "" {
			attrs := ""
			if node.LinkTarget != "" {
				attrs = ` target="` + svg.Esc(node.LinkTarget) + `"`
			}
			body = util.Linked(body, node.Link, node.ID, &links, options, attrs)
		}
		nodesOut.WriteString(`<g class="pele-node pele-shape-` + view.shape + classes + `" data-id="` + id + `" transform="translate(` +
			svg.Num(x) + `,` + svg.Num(y) + `)">` + body + `</g>`)
	}

	var reach svg.SideReach
	for i := range cedges {
		ce := &cedges[i]
		if ce.Src == ce.Dst || drawn[i].Thickness == "invisible" || len(ce.Route) < 6 {
			continue
		}
		reach.Add(ce.Src, svg.EndSide(ce.Route, 0, &cnodes[ce.Src]), drawn[i].ArrowTypeStart)
		reach.Add(ce.Dst, svg.EndSide(ce.Route, len(ce.Route)-3, &cnodes[ce.Dst]), drawn[i].ArrowTypeEnd)
	}

	var edgesOut, labelsOut strings.Builder
	loopSeen := map[int]int{}
	for i := range cedges {
		ce := &cedges[i]
		edge := drawn[i]
		label := edgeLabels[i]
		var style svg.ResolvedStyle
		if len(edge.Style) > 0 || len(edge.CSSCompiledStyles) > 0 {
			style = svg.ResolveStyle(append(append([]string{}, edge.CSSCompiledStyles...), edge.Style...))
		}
		if edge.Thickness != "invisible" {
			startType := edge.ArrowTypeStart
			endType := edge.ArrowTypeEnd
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
				srcBox, dstBox := &cnodes[ce.Src], &cnodes[ce.Dst]
				leaves := reach.Of(ce.Src, svg.EndSide(route, 0, srcBox))
				arrives := reach.Of(ce.Dst, svg.EndSide(route, len(route)-3, dstBox))
				src, dst := views[ce.Src], views[ce.Dst]
				svg.InsetRoute(route, 0, src.shape, src.w, src.h, srcBox)
				svg.InsetRoute(route, len(route)-3, dst.shape, dst.w, dst.h, dstBox)
				path = svg.RoutePath(route, edge.Curve, svg.MarkerTrim(startType), svg.MarkerTrim(endType), leaves, arrives)
			}
			color := "var(--_l)"
			if style.Stroke != "" {
				color = svg.Esc(style.Stroke)
			}
			attrs := ""
			if edge.Thickness == "thick" {
				attrs = ` stroke-width="2.5"`
			} else if edge.Thickness == "dotted" {
				attrs = ` stroke-dasharray="3 4"`
			}
			classes := svg.ClassNames(flowchartClasses.Replace(edge.Classes))
			edgesOut.WriteString(`<g class="pele-edge` + classes + `" data-id="` + svg.EscText(edge.ID) + `"` + style.Line + `>` +
				`<path d="` + path.D + `"` + attrs + `/>` +
				svg.Marker(startType, path.SX, path.SY, path.SDX, path.SDY, color) +
				svg.Marker(endType, path.EX, path.EY, path.EDX, path.EDY, color) +
				`</g>`)
		}
		if label.Width > 0 {
			labelsOut.WriteString(svg.EdgeLabelSVG(edge.ID, label, ce.LabelX, ce.LabelY, style.Text, icons))
		}
	}

	typ := "flowchart"
	if variant != nil {
		typ = variant.Type
	}
	var content strings.Builder
	content.WriteString(svg.LabelSVG(title, width/2, padding+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil))
	if clusters.Len() > 0 {
		content.WriteString(`<g class="pele-clusters">` + clusters.String() + `</g>`)
	}
	if edgesOut.Len() > 0 {
		content.WriteString(`<g class="pele-edges" fill="none" stroke="var(--_l)" stroke-linecap="round">` + edgesOut.String() + `</g>`)
	}
	if struckTitles.Len() > 0 {
		content.WriteString(`<g class="pele-cluster-titles">` + struckTitles.String() + `</g>`)
	}
	if labelsOut.Len() > 0 {
		content.WriteString(`<g class="pele-edge-labels" font-size="` + jsnum.Format(edgeSize) + `">` + labelsOut.String() + `</g>`)
	}
	content.WriteString(`<g class="pele-nodes">` + nodesOut.String() + `</g>`)

	doc := svg.Document(typ, width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccTitle: db.AccTitle, AccDescr: db.AccDescr}, content.String())
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: links}
}
