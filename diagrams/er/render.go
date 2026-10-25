package er

import (
	"math"
	"slices"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/layout"
	"github.com/Broderick-Westrope/pele-go/internal/omap"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

const (
	nodeSep     = 48
	edgeSep     = 20
	rankSep     = 64
	groupPad    = 20
	loopReach   = 28
	loopBand    = 38
	minWidth    = 96
	boxPadX     = 18
	boxPadY     = 12
	headPadY    = 9
	cellPadX    = 12
	cellGap     = 16
	rowPad      = 9
	commentWrap = 260
	noWrap      = 1e6
	portGap     = 18
	portMargin  = 14
	boxAttrs    = ` fill="var(--_s)" stroke="var(--_b)"`
)

func numberOption(config *jsobj.Object, key string, fallback float64) float64 {
	section, _ := config.Get("er")
	obj, ok := section.(*jsobj.Object)
	if !ok {
		return fallback
	}
	value, _ := obj.Get(key)
	if v, isNum := value.(float64); isNum && v > 0 {
		return min(v, 2000)
	}
	return fallback
}

// cells are a row's type, name, keys and comment.
type cells [4]text.Label

type row struct {
	cells cells
	h     float64
}

type nodeView struct {
	node  *erGraphNode
	style svg.ResolvedStyle
	label text.Label
	w     float64
	h     float64
	headH float64
	rows  []row
	// cols are the left edge of each column, measured from the left edge of the box.
	cols    []float64
	rowSize float64
	keySize float64
	// below is whether self-relationships loop out of the bottom side rather than the right.
	below bool
}

// loop is a relationship from an entity to itself, with the stretch of the side it loops out of.
type loop struct {
	label text.Label
	band  float64
	at    float64
}

// textLeft writes a measured label starting at x, with the baseline of its first line at y.
func textLeft(label text.Label, x, y float64, attrs string, icons *text.Icons) string {
	lines := label.Lines
	if len(lines) == 0 {
		return ""
	}
	if len(lines) == 1 && len(lines[0]) == 1 {
		if only := lines[0][0]; only.Icon == "" && only.Math == nil && only.Style == 0 {
			return `<text` + attrs + ` x="` + svg.Num(x) + `" y="` + svg.Num(y) + `">` + svg.Esc(only.Text) + `</text>`
		}
	}
	shared := svg.SharedStyle(label)
	shift := float64(label.Size * 0.35)
	top := y - shift - float64(label.LineHeight/2)
	var body, extra strings.Builder
	for i, line := range lines {
		middle := top + text.LineMiddle(label, i)
		if text.HasMath(line) {
			extra.WriteString(svg.LabelSVG(text.LabelLine(label, i), x+float64(label.Widths[i]/2), middle, attrs, icons))
			continue
		}
		y = middle + shift
		hasIcon := false
		for _, span := range line {
			if span.Icon != "" {
				hasIcon = true
			}
		}
		sx := x
		placed := false
		for _, span := range line {
			if span.Icon != "" {
				side := float64(label.Size * 1.1)
				extra.WriteString(`<svg class="pele-icon" data-icon="` + svg.EscText(span.Icon) + `" x="` + svg.Num(sx) + `" y="` +
					svg.Num(y-float64(label.Size*0.35)-float64(side/2)) + `" width="` + svg.Num(side) + `" height="` + svg.Num(side) +
					`" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
					icons.Resolve(span.Icon) + `</svg>`)
			} else if span.Text != "" {
				pos := ""
				if hasIcon || !placed {
					pos = ` x="` + svg.Num(sx) + `" y="` + svg.Num(y) + `"`
				}
				body.WriteString(`<tspan` + pos + svg.SpanStyle(span.Style&^shared) + `>` + svg.Esc(span.Text) + `</tspan>`)
				placed = true
			}
			sx += span.Width
		}
	}
	return `<text` + svg.TextStyle(attrs, shared) + ` xml:space="preserve">` + body.String() + `</text>` + extra.String()
}

// cellText: Mermaid reads `~` pairs as angle brackets. They are written as entities so the
// label parser does not take `<int>` for a tag.
func cellText(raw string) string {
	converted := common.ParseGenericTypes(raw)
	if converted == raw {
		return raw
	}
	return strings.ReplaceAll(strings.ReplaceAll(converted, "<", "&lt;"), ">", "&gt;")
}

func renderEr(db *erDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	if !common.RunsAcross(db.direction) {
		return draw(db, config, rc, false)
	}
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered { return draw(db, config, rc, down) })
}

// draw: turned draws a diagram that runs across as one that runs down.
func draw(db *erDb, config *jsobj.Object, rc *core.Context, turned bool) core.Rendered {
	options := rc.Options
	graph := buildErGraph(db)
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	smallSize := jsmath.Round(size * 0.875)
	measurer := rc.Measurer()
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	icons := rc.Icons

	index := map[string]int{}
	var views []*nodeView
	var cnodes []layout.CNode
	for i, node := range graph.nodes {
		index[node.id] = i
	}

	var cedges []layout.CEdge
	var edgeLabels []text.Label
	var drawn []*erGraphEdge
	var loops omap.Map[int, []*loop]
	var loopOf []*loop
	for k := range graph.edges {
		edge := &graph.edges[k]
		src, okSrc := index[edge.start]
		dst, okDst := index[edge.end]
		if !okSrc || !okDst {
			continue
		}
		label := text.Layout(edge.label, true, measurer, smallSize, 200, 0, icons)
		var lp *loop
		if src == dst {
			lp = &loop{label: label}
			list, _ := loops.Get(src)
			loops.Set(src, append(list, lp))
		}
		loopOf = append(loopOf, lp)
		drawn = append(drawn, edge)
		edgeLabels = append(edgeLabels, label)
		labelW, labelH := 0.0, 0.0
		if label.Width > 0 {
			labelW = label.Width + 12
		}
		if label.Height > 0 {
			labelH = label.Height + 4
		}
		cedges = append(cedges, layout.CEdge{Src: src, Dst: dst, Minlen: 1, LabelW: labelW, LabelH: labelH, Route: []float64{}})
	}

	// horizontal is the direction of the level a node is laid out in: its nearest subgraph
	// that sets one, or the diagram's.
	horizontal := func(i int) bool {
		at := i
		// Subgraphs can name each other as members, so the walk up is bounded.
		for steps := 0; steps < len(graph.nodes); steps++ {
			node := &graph.nodes[at]
			if !node.hasParent {
				break
			}
			up, ok := index[node.parentID]
			if !ok {
				break
			}
			if dir := graph.nodes[up].dir; dir != "" {
				return dir == "LR" || dir == "RL"
			}
			at = up
		}
		return db.direction == "LR" || db.direction == "RL"
	}

	for i := range graph.nodes {
		node := &graph.nodes[i]
		declarations := node.cssCompiledStyles
		if len(node.cssStyles) > 0 {
			declarations = slices.Concat(node.cssCompiledStyles, node.cssStyles)
		}
		style := svg.ResolveStyle(declarations)
		var base text.Style
		if style.Bold {
			base |= text.Bold
		}
		if style.Italic {
			base |= text.Italic
		}
		nameSize, rowSize, keySize := size, smallSize, jsmath.Round(size*0.75)
		if style.FontSize != 0 {
			nameSize, rowSize, keySize = style.FontSize, style.FontSize, style.FontSize
		}
		var label text.Label
		if node.isGroup {
			label = text.Layout(node.label, true, measurer, nameSize, 4000, base, icons)
		} else {
			name := node.alias
			if name == "" {
				name = node.label
			}
			label = text.Layout(name, true, measurer, nameSize, 240, base, icons)
		}
		own, _ := loops.Get(i)
		// Loops leave by a side the layout keeps free of other edges: the right when ranks run
		// down, the bottom when they run across.
		below := len(own) > 0 && !node.isGroup && horizontal(i)
		view := &nodeView{node: node, style: style, label: label, rowSize: rowSize, keySize: keySize, below: below}
		reach := 0.0
		bands := 0.0
		for _, lp := range own {
			if below {
				lp.band = max(loopBand, lp.label.Width+12)
				reach = max(reach, lp.label.Height)
			} else {
				lp.band = max(loopBand, lp.label.Height+6)
				reach = max(reach, lp.label.Width)
			}
			bands += lp.band
		}
		minW := float64(minWidth)
		if below {
			minW = max(minWidth, bands+8)
		}
		var c layout.CNode
		if node.isGroup {
			c = layout.NewCNode(0, 0, -1)
			c.IsGroup = true
			if node.dir != "" {
				c.Dir = layout.Direction(node.dir)
			}
			c.PadX = groupPad
			c.PadTop = groupPad
			if view.label.Height > 0 {
				c.PadTop = view.label.Height + 16
			}
			c.PadBottom = groupPad
			c.MinW = max(view.label.Width+2*groupPad-12, minWidth)
		} else {
			nameH := max(label.Height, jsmath.Round(nameSize*1.5))
			if len(node.attributes) == 0 {
				view.w = max(label.Width+2*boxPadX, minW)
				if below {
					view.h = max(nameH+2*boxPadY, 0)
				} else {
					view.h = max(nameH+2*boxPadY, bands+8)
				}
			} else {
				rowLine := jsmath.Round(rowSize * 1.5)
				var widths [4]float64
				for _, attribute := range node.attributes {
					cs := cells{
						text.Layout(cellText(attribute.typ), true, measurer, rowSize, noWrap, base, icons),
						text.Layout(cellText(attribute.name), true, measurer, rowSize, noWrap, base, icons),
						text.Layout(strings.Join(attribute.keys, ", "), false, measurer, keySize, noWrap, base, icons),
						text.Layout(cellText(attribute.comment), true, measurer, rowSize, commentWrap, base, icons),
					}
					h := rowLine
					for k := range 4 {
						if cs[k].Width > widths[k] {
							widths[k] = cs[k].Width
						}
						if cs[k].Height > h {
							h = cs[k].Height
						}
					}
					view.rows = append(view.rows, row{cells: cs, h: h + rowPad})
					view.h += h + rowPad
				}
				columns := 0
				inner := 0.0
				for _, w := range widths {
					if w == 0 {
						continue
					}
					if columns > 0 {
						inner += w + cellGap
					} else {
						inner += w
					}
					columns++
				}
				view.w = max(inner+2*cellPadX, label.Width+2*boxPadX, minW)
				// A box wider than its table shares the spare width between the columns.
				spare := 0.0
				if columns > 0 {
					spare = (view.w - inner - 2*cellPadX) / float64(columns)
				}
				x := float64(cellPadX)
				for _, w := range widths {
					view.cols = append(view.cols, x)
					if w > 0 {
						x += w + spare + cellGap
					}
				}
				view.headH = nameH + 2*headPadY
				view.h += view.headH
			}
			// The layout keeps a node centered in its box, so room for loops is added on both
			// sides.
			room := 0.0
			if len(own) > 0 {
				room = float64(2 * (markerLength + loopReach*0.75 + 10 + reach))
			}
			if below {
				c = layout.NewCNode(view.w, view.h+room, -1)
			} else {
				c = layout.NewCNode(view.w+room, view.h, -1)
			}
		}
		c.Seq = float64(len(views))
		views = append(views, view)
		cnodes = append(cnodes, c)
	}
	for i := range graph.nodes {
		node := &graph.nodes[i]
		cnodes[i].Parent = -1
		if node.hasParent {
			if up, ok := index[node.parentID]; ok {
				cnodes[i].Parent = up
			}
		}
	}

	rootDir := layout.Direction(db.direction)
	if turned {
		rootDir = layout.TB
	}
	result := layout.CompoundLayout(rc.Ctx, cnodes, cedges, rootDir, layout.LayeredOptions{
		NodeSep: numberOption(config, "nodeSpacing", nodeSep),
		EdgeSep: edgeSep,
		RankSep: numberOption(config, "rankSpacing", rankSep),
		PortSep: 24,
	})

	spreadEnds(cedges, cnodes, views)

	// A subgraph gets its size from the layout, so its loops hang outside it and may widen the
	// diagram.
	layoutWidth := result.Width
	for i, own := range loops.All() {
		c := &cnodes[i]
		if !c.IsGroup {
			continue
		}
		reach := 0.0
		for _, lp := range own {
			reach = max(reach, lp.label.Width)
		}
		layoutWidth = max(layoutWidth, c.X+float64(c.W/2)+markerLength+loopReach*0.75+10+reach)
	}

	title := svg.TitleLabel(db.title, measurer, size, 4000, icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}
	inner := max(layoutWidth, title.Width)
	ox := pad + float64((inner-layoutWidth)/2)
	oy := pad + titleHeight
	layout.ShiftLayout(cnodes, cedges, ox, oy)
	width := math.Ceil(inner + float64(2*pad))
	height := math.Ceil(result.Height + titleHeight + float64(2*pad))
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

	var clusters, nodesOut strings.Builder
	for i, view := range views {
		c := &cnodes[i]
		node := view.node
		classes := svg.ClassNames(svg.StripDefault(node.cssClasses))
		if c.IsGroup {
			view.w = c.W
			view.h = c.H
			x := c.X - float64(c.W/2)
			y := c.Y - float64(c.H/2)
			titleX := svg.ClusterTitleX(c, view.label.Width, crossings, groupPad)
			clusters.WriteString(`<g class="pele-cluster` + classes + `" data-id="` + svg.EscText(node.id) + `">` +
				`<rect x="` + svg.Num(x) + `" y="` + svg.Num(y) + `" width="` + svg.Num(c.W) + `" height="` + svg.Num(c.H) +
				`" fill="var(--_a)" fill-opacity="0.5" stroke="var(--_b)"` + svg.Rounded(view.style.Shape) + `/>` +
				svg.LabelSVG(view.label, titleX, y+8+float64(view.label.Height/2), ` class="pele-cluster-label" fill="var(--_m)"`+view.style.Text, icons) +
				`</g>`)
			continue
		}

		w := view.w
		h := view.h
		x0 := float64(-w / 2)
		y0 := float64(-h / 2)
		txt := view.style.Text
		shape := `x="` + svg.Num(x0) + `" y="` + svg.Num(y0) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) + `"`
		nameY := 0.0
		if len(view.rows) > 0 {
			nameY = y0 + float64(view.headH/2)
		}
		name := svg.LabelSVG(view.label, 0, nameY, ` class="pele-label"`+txt, icons)
		display := node.alias
		if display == "" {
			display = node.label
		}
		display = text.DecodeEntities(display)
		if strings.Contains(classes+" ", " internal-link ") && name != "" && (options.Links == nil || *options.Links) {
			// The name ends up in an href, so it gets the same check as a URL.
			_, safe := util.LinkURL(util.SanitizeURL(display), options)
			target := svg.Esc(display)
			href := ""
			if safe {
				href = ` href="` + target + `"` + util.RelAttr(options)
			}
			name = `<a class="internal-link"` + href + ` data-href="` + target + `">` + name + `</a>`
			links = append(links, core.Link{ID: node.label, Href: display, Internal: true})
		}

		var body string
		if len(view.rows) == 0 {
			body = `<rect ` + shape + svg.Rounded(boxAttrs+view.style.Shape) + `/>` + name
		} else {
			rowLine := jsmath.Round(view.rowSize * 1.5)
			lineColor := "var(--_b)"
			if view.style.Stroke != "" {
				lineColor = svg.Esc(view.style.Stroke)
			}
			var stripes, cellsOut strings.Builder
			y := y0 + view.headH
			for r, rw := range view.rows {
				if r%2 == 0 {
					span := `x="` + svg.Num(x0) + `" width="` + svg.Num(w) + `"`
					if r == len(view.rows)-1 {
						// The last row follows the box's rounded bottom corners: a rounded rect,
						// squared off on top.
						stripes.WriteString(`<rect ` + span + ` y="` + svg.Num(y) + `" height="` + svg.Num(rw.h) + `"` + svg.RoundedStyle + `/>` +
							`<rect ` + span + ` y="` + svg.Num(y) + `" height="` + svg.Num(rw.h/2) + `"/>`)
					} else {
						stripes.WriteString(`<rect ` + span + ` y="` + svg.Num(y) + `" height="` + svg.Num(rw.h) + `"/>`)
					}
				}
				baseline := y + rowPad/2.0 + float64(rowLine/2) + float64(view.rowSize*0.35)
				cellsOut.WriteString(
					textLeft(rw.cells[0], x0+view.cols[0], baseline, ` class="pele-er-type" fill="var(--_m)"`+txt, icons) +
						textLeft(rw.cells[1], x0+view.cols[1], baseline, ` class="pele-er-name"`+txt, icons) +
						textLeft(rw.cells[2], x0+view.cols[2], baseline, ` class="pele-er-keys" font-size="`+jsnum.Format(view.keySize)+`" fill="var(--_m)"`+txt, icons) +
						textLeft(rw.cells[3], x0+view.cols[3], baseline, ` class="pele-er-comment" fill="var(--_m)"`+txt, icons))
				y += rw.h
			}
			dividerY := svg.Num(y0 + view.headH)
			opacity := ""
			if strings.Contains(view.style.Shape, "fill:") {
				opacity = ` opacity="0.6"`
			}
			body = `<rect ` + shape + svg.Rounded(boxAttrs+view.style.Shape) + `/>` +
				// Over a fill the diagram chose, the shading is a tint of that fill. The opacity
				// is on the group because the two parts of the last row overlap.
				`<g class="pele-er-stripes" fill="var(--_bg)"` + opacity + `>` + stripes.String() + `</g>` +
				`<path class="pele-er-divider" d="M` + svg.Num(x0) + `,` + dividerY + `H` + svg.Num(x0+w) + `" stroke="` + lineColor + `"/>` +
				`<rect ` + shape + ` fill="none" stroke="var(--_b)"` + svg.Rounded(view.style.Line) + `/>` +
				name +
				`<g class="pele-er-attributes" font-size="` + jsnum.Format(view.rowSize) + `">` + cellsOut.String() + `</g>`
		}
		nodesOut.WriteString(`<g class="pele-node pele-entity` + classes + `" data-id="` + svg.EscText(node.label) +
			`" transform="translate(` + svg.Num(c.X) + `,` + svg.Num(c.Y) + `)">` + body + `</g>`)
	}

	// Self-relationships share the side they loop out of, squeezed together if it is too short
	// for them.
	for i, own := range loops.All() {
		view := views[i]
		total := 0.0
		for _, lp := range own {
			total += lp.band
		}
		side := view.h
		if view.below {
			side = view.w
		}
		scale := min(1, max(side-8, 0)/total)
		at := float64(float64(-total*scale) / 2)
		for _, lp := range own {
			lp.band = float64(lp.band * scale)
			lp.at = at + float64(lp.band/2)
			at += lp.band
		}
	}

	var edgesOut, labelsOut strings.Builder
	for i := range cedges {
		ce := &cedges[i]
		edge := drawn[i]
		label := edgeLabels[i]
		var d, start, end string
		if lp := loopOf[i]; lp != nil {
			c := &cnodes[ce.Src]
			view := views[ce.Src]
			spread := max(min(float64(lp.band/2)-6, 13), 1)
			const far = markerLength + loopReach*0.75 + 6
			if view.below {
				x := c.X + lp.at
				y := c.Y + float64(view.h/2)
				turn := svg.Num(y + markerLength)
				out := svg.Num(y + markerLength + loopReach)
				d = "M" + svg.Num(x-spread) + "," + svg.Num(y) + "V" + turn + "C" + svg.Num(x-spread) + "," + out + " " +
					svg.Num(x+spread) + "," + out + " " + svg.Num(x+spread) + "," + turn + "V" + svg.Num(y)
				start = erMarker(edge.arrowTypeStart, x-spread, y, 0, -1)
				end = erMarker(edge.arrowTypeEnd, x+spread, y, 0, -1)
				ce.LabelX = x
				ce.LabelY = y + far + float64(label.Height/2)
			} else {
				x := c.X + float64(view.w/2)
				y := c.Y + lp.at
				turn := svg.Num(x + markerLength)
				out := svg.Num(x + markerLength + loopReach)
				d = "M" + svg.Num(x) + "," + svg.Num(y-spread) + "H" + turn + "C" + out + "," + svg.Num(y-spread) + " " +
					out + "," + svg.Num(y+spread) + " " + turn + "," + svg.Num(y+spread) + "H" + svg.Num(x)
				start = erMarker(edge.arrowTypeStart, x, y-spread, -1, 0)
				end = erMarker(edge.arrowTypeEnd, x, y+spread, -1, 0)
				ce.LabelX = x + far + float64(label.Width/2)
				ce.LabelY = y
			}
		} else {
			route := straighten(ce.Route)
			if len(route) < 6 {
				continue
			}
			inset(route, 0, views[ce.Src], &cnodes[ce.Src])
			inset(route, len(route)-3, views[ce.Dst], &cnodes[ce.Dst])
			// A curve stops short of each box so that the marker sits on a straight run of line.
			dx := math.Abs(route[len(route)-3] - route[0])
			dy := math.Abs(route[len(route)-2] - route[1])
			direct := len(route) == 6 && (dx < 0.01 || dy < 0.01 || jsmath.Hypot(dx, dy) <= 3*markerLength)
			stub := 0.0
			if !direct {
				stub = markerLength
			}
			path := svg.RoutePath(route, "", stub, stub, svg.DefaultReach(stub), svg.DefaultReach(stub))
			d = path.D
			if stub > 0 {
				d = "M" + svg.Num(path.SX) + "," + svg.Num(path.SY) + "L" + path.D[1:] + "L" + svg.Num(path.EX) + "," + svg.Num(path.EY)
			}
			start = erMarker(edge.arrowTypeStart, path.SX, path.SY, path.SDX, path.SDY)
			end = erMarker(edge.arrowTypeEnd, path.EX, path.EY, path.EDX, path.EDY)
		}
		dashed := edge.pattern == "dashed"
		kind, dash := "identifying", ""
		if dashed {
			kind, dash = "non-identifying", ` stroke-dasharray="4 3"`
		}
		edgesOut.WriteString(`<g class="pele-edge pele-er-` + kind + `" data-id="` + svg.EscText(edge.id) + `">` +
			`<path d="` + d + `"` + dash + `/>` + start + end + `</g>`)
		if label.Width > 0 {
			labelsOut.WriteString(svg.EdgeLabelSVG(edge.id, label, ce.LabelX, ce.LabelY, "", icons))
		}
	}

	content := svg.LabelSVG(title, width/2, pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil)
	if clusters.Len() > 0 {
		content += `<g class="pele-clusters">` + clusters.String() + `</g>`
	}
	if edgesOut.Len() > 0 {
		content += `<g class="pele-edges" fill="none" stroke="var(--_l)" stroke-linecap="round">` + edgesOut.String() + `</g>`
	}
	if labelsOut.Len() > 0 {
		content += `<g class="pele-edge-labels" font-size="` + jsnum.Format(smallSize) + `">` + labelsOut.String() + `</g>`
	}
	content += `<g class="pele-nodes">` + nodesOut.String() + `</g>`

	doc := svg.Document("er", width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccTitle: db.accTitle, AccDescr: db.accDescr}, content)
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: links}
}

func straighten(route []float64) []float64 {
	out := slices.Clone(route[:min(3, len(route))])
	for k := 3; k < len(route); k += 3 {
		prev := len(out) - 3
		flat := func(axis int) bool {
			return math.Abs(out[prev+axis]-route[k+axis]) < 0.01 && math.Abs(route[k+axis]-route[k+3+axis]) < 0.01
		}
		if k+3 < len(route) && (flat(0) || flat(1)) {
			continue
		}
		out = append(out, route[k], route[k+1], route[k+2])
	}
	return out
}

type routeEnd struct {
	route  []float64
	at     int
	toward float64
}

// spreadEnds: the layout brings every edge that ends on one side of a box to the middle of
// that side, where their markers would cover each other. This spaces them along the side, in
// the order they fan out.
func spreadEnds(cedges []layout.CEdge, cnodes []layout.CNode, views []*nodeView) {
	var sides omap.Map[int, []*routeEnd]
	add := func(node int, route []float64, at, next int) {
		c := &cnodes[node]
		dx := route[at] - c.X
		dy := route[at+1] - c.Y
		var side int
		if math.Abs(math.Abs(dy)-float64(c.H/2)) < 0.5 {
			side = 2
			if dy < 0 {
				side = 0
			}
		} else if math.Abs(math.Abs(dx)-float64(c.W/2)) < 0.5 {
			side = 1
			if dx < 0 {
				side = 3
			}
		} else {
			return
		}
		key := node*4 + side
		end := &routeEnd{route: route, at: at, toward: route[next+side&1]}
		list, _ := sides.Get(key)
		sides.Set(key, append(list, end))
	}
	for i := range cedges {
		e := &cedges[i]
		last := len(e.Route) - 3
		if e.Src == e.Dst || last < 3 {
			continue
		}
		add(e.Src, e.Route, 0, 3)
		add(e.Dst, e.Route, last, last-3)
	}
	for key, ends := range sides.All() {
		if len(ends) < 2 {
			continue
		}
		node := key >> 2
		vertical := key&1 == 1
		w, h := views[node].w, views[node].h
		if cnodes[node].IsGroup {
			w, h = cnodes[node].W, cnodes[node].H
		}
		length := w
		if vertical {
			length = h
		}
		step := max(min(portGap, (length-2*portMargin)/float64(len(ends)-1)), 0)
		center := cnodes[node].X
		if vertical {
			center = cnodes[node].Y
		}
		slices.SortStableFunc(ends, func(a, b *routeEnd) int {
			switch {
			case a.toward < b.toward:
				return -1
			case a.toward > b.toward:
				return 1
			}
			return 0
		})
		axis := 0
		if vertical {
			axis = 1
		}
		for i, end := range ends {
			end.route[end.at+axis] = center + float64((float64(i)-float64(float64(len(ends)-1)/2))*step)
		}
	}
}

// inset: an entity with a self-relationship is laid out larger than it is drawn; this moves a
// route end from the side of that larger box onto the entity's own side.
func inset(route []float64, at int, view *nodeView, c *layout.CNode) {
	if c.IsGroup {
		return
	}
	dx := route[at] - c.X
	dy := route[at+1] - c.Y
	if c.W != view.w && math.Abs(math.Abs(dx)-float64(c.W/2)) < 0.5 {
		route[at] -= float64(float64(jsmath.Sign(dx)*(c.W-view.w)) / 2)
	}
	if c.H != view.h && math.Abs(math.Abs(dy)-float64(c.H/2)) < 0.5 {
		route[at+1] -= float64(float64(jsmath.Sign(dy)*(c.H-view.h)) / 2)
	}
}
