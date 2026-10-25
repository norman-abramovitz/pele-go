package requirement

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
)

const (
	nodeSep        = 40
	edgeSep        = 16
	rankSep        = 56
	padX           = 14
	padY           = 10
	rowGap         = 2
	columnGap      = 12
	minWidth       = 120
	nameWrap       = 280
	valueWrap      = 240
	loopReach      = 26
	containsRadius = 6
	arrowLength    = 9
	arrowHalf      = 4.5
	endStep        = 28
	endMargin      = 14
)

type row struct {
	key    text.Label
	value  text.Label
	height float64
}

type nodeView struct {
	node       *graphNode
	style      svg.ResolvedStyle
	kind       text.Label
	name       text.Label
	rows       []row
	keyWidth   float64
	headHeight float64
	w          float64
	h          float64
}

// restyle adds bold or italic to a whole label after its markdown is read, since emphasis
// markers only toggle styles the label does not start with.
func restyle(label text.Label, bits text.Style, measurer text.Measurer) text.Label {
	if bits == 0 || len(label.Lines) == 0 {
		return label
	}
	width := 0.0
	widths := make([]float64, 0, len(label.Lines))
	lines := make([][]text.Span, len(label.Lines))
	for i, line := range label.Lines {
		total := 0.0
		spans := make([]text.Span, len(line))
		for j, span := range line {
			style := span.Style | bits
			w := span.Width
			if span.Icon == "" && span.Math == nil {
				w = measurer.Width(span.Text, label.Size, style)
			}
			total += w
			spans[j] = span
			spans[j].Style, spans[j].Width = style, w
		}
		widths = append(widths, total)
		if total > width {
			width = total
		}
		lines[i] = spans
	}
	label.Lines = lines
	label.Widths = widths
	label.Width = width
	return label
}

// startText writes a measured label with its lines starting at x, the first line's box
// starting at top.
func startText(label text.Label, x, top float64, attrs string) string {
	shared := svg.SharedStyle(label)
	var body, math strings.Builder
	for i, line := range label.Lines {
		middle := top + text.LineMiddle(label, i)
		if text.HasMath(line) {
			math.WriteString(svg.LabelSVG(text.LabelLine(label, i), x+float64(label.Widths[i]/2), middle, attrs, nil))
			continue
		}
		y := middle + float64(label.Size*0.35)
		first := true
		for _, span := range line {
			if span.Icon != "" || span.Text == "" {
				continue
			}
			pos := ""
			if first {
				pos = ` x="` + svg.Num(x) + `" y="` + svg.Num(y) + `"`
			}
			body.WriteString(`<tspan` + pos + svg.SpanStyle(span.Style&^shared) + `>` + svg.Esc(span.Text) + `</tspan>`)
			first = false
		}
	}
	if body.Len() == 0 {
		return math.String()
	}
	return `<text` + svg.TextStyle(attrs, shared) + ` text-anchor="start" xml:space="preserve">` + body.String() + `</text>` + math.String()
}

// containsMarker is the containment mark: a circled plus whose rim touches (x, y), where
// (dx, dy) points at the node.
func containsMarker(x, y, dx, dy float64) string {
	const r = containsRadius
	cx := x - float64(dx*r)
	cy := y - float64(dy*r)
	return `<circle class="pele-marker" cx="` + svg.Num(cx) + `" cy="` + svg.Num(cy) + `" r="` + jsnum.Format(r) + `"/>` +
		`<path class="pele-marker" d="M` + svg.Num(cx-float64(dx*r)) + `,` + svg.Num(cy-float64(dy*r)) +
		`L` + svg.Num(cx+float64(dx*r)) + `,` + svg.Num(cy+float64(dy*r)) +
		`M` + svg.Num(cx+float64(dy*r)) + `,` + svg.Num(cy-float64(dx*r)) +
		`L` + svg.Num(cx-float64(dy*r)) + `,` + svg.Num(cy+float64(dx*r)) + `"/>`
}

// arrowMarker is an open arrowhead with its tip at (x, y), pointing along (dx, dy).
func arrowMarker(x, y, dx, dy float64) string {
	bx := x - float64(dx*arrowLength)
	by := y - float64(dy*arrowLength)
	return `<path class="pele-marker" d="M` + svg.Num(bx-float64(dy*arrowHalf)) + `,` + svg.Num(by+float64(dx*arrowHalf)) +
		`L` + svg.Num(x) + `,` + svg.Num(y) +
		`L` + svg.Num(bx+float64(dy*arrowHalf)) + `,` + svg.Num(by-float64(dx*arrowHalf)) + `" stroke-linejoin="round"/>`
}

type routeEnd struct {
	route  []float64
	at     int
	toward float64
}

// spreadEnds: the layout attaches every edge to the middle of a node's side. Ends that share a
// side are spread along it, in the order of where they head, so their markers do not sit on
// each other.
func spreadEnds(cnodes []layout.CNode, cedges []layout.CEdge, views []*nodeView) {
	var sides omap.Map[int, []*routeEnd]
	for i := range cedges {
		edge := &cedges[i]
		route := edge.Route
		if edge.Src == edge.Dst || len(route) < 6 {
			continue
		}
		for _, at := range [2]int{0, len(route) - 3} {
			node := edge.Dst
			next := at - 3
			if at == 0 {
				node = edge.Src
				next = 3
			}
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
				continue
			}
			key := node*4 + side
			toward := next
			if side%2 != 0 {
				toward = next + 1
			}
			list, _ := sides.Get(key)
			sides.Set(key, append(list, &routeEnd{route: route, at: at, toward: route[toward]}))
		}
	}
	for key, list := range sides.All() {
		if len(list) < 2 {
			continue
		}
		node := key >> 2
		vertical := key%2 == 1
		length := views[node].w
		if vertical {
			length = views[node].h
		}
		span := length - 2*endMargin
		step := max(0, min(endStep, span/float64(len(list)-1)))
		slices.SortStableFunc(list, func(a, b *routeEnd) int {
			switch {
			case a.toward < b.toward:
				return -1
			case a.toward > b.toward:
				return 1
			}
			return 0
		})
		for j, end := range list {
			offset := float64((float64(j) - float64(float64(len(list)-1)/2)) * step)
			if vertical {
				end.route[end.at+1] = cnodes[node].Y + offset
			} else {
				end.route[end.at] = cnodes[node].X + offset
			}
		}
	}
}

func renderRequirement(db *requirementDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	if !common.RunsAcross(db.direction) {
		return draw(db, config, rc, false)
	}
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered { return draw(db, config, rc, down) })
}

// draw: turned draws a diagram that runs across as one that runs down.
func draw(db *requirementDb, _ *jsobj.Object, rc *core.Context, turned bool) core.Rendered {
	options := rc.Options
	graph := buildRequirementGraph(db)
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	small := jsmath.Round(size * 0.875)
	measurer := rc.Measurer()
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	icons := rc.Icons

	index := map[string]int{}
	var views []*nodeView
	var cnodes []layout.CNode
	for k := range graph.nodes {
		node := &graph.nodes[k]
		style := svg.ResolveStyle(node.cssStyles)
		var base text.Style
		if style.Bold {
			base |= text.Bold
		}
		if style.Italic {
			base |= text.Italic
		}
		nameSize, bodySize := size, small
		if style.FontSize != 0 {
			nameSize, bodySize = style.FontSize, style.FontSize
		}
		req := node.requirement
		kindName := "Element"
		if req != nil {
			kindName = req.typ
		}
		kind := restyle(text.Layout("«"+kindName+"»", false, measurer, bodySize, 4000, 0, icons), base, measurer)
		name := restyle(text.Layout(node.id, true, measurer, nameSize, nameWrap, 0, icons), base|text.Bold, measurer)
		var fields [][2]string
		if req != nil {
			fields = [][2]string{
				{"ID", req.requirementID},
				{"Text", req.text},
				{"Risk", req.risk},
				{"Verification", req.verifyMethod},
			}
		} else {
			fields = [][2]string{
				{"Type", node.element.typ},
				{"Doc Ref", node.element.docRef},
			}
		}
		var rows []row
		keyWidth := 0.0
		valueWidth := 0.0
		bodyHeight := 0.0
		for _, field := range fields {
			if field[1] == "" {
				continue
			}
			key := restyle(text.Layout(field[0], false, measurer, bodySize, 4000, 0, icons), base, measurer)
			value := restyle(text.Layout(field[1], true, measurer, bodySize, valueWrap, 0, icons), base, measurer)
			height := max(key.Height, value.Height)
			rows = append(rows, row{key: key, value: value, height: height})
			keyWidth = max(keyWidth, key.Width)
			valueWidth = max(valueWidth, value.Width)
			bodyHeight += height + rowGap
		}
		if len(rows) > 0 {
			bodyHeight += 2*padY - rowGap
		}
		headHeight := 2*padY + kind.Height + name.Height
		tableWidth := 0.0
		if len(rows) > 0 {
			tableWidth = keyWidth + columnGap + valueWidth + 2*padX
		}
		w := max(minWidth, max(kind.Width, name.Width)+2*padX, tableWidth)
		// A requirement and an element may share a name; relationships then go to the
		// requirement.
		if _, ok := index[node.id]; !ok {
			index[node.id] = len(views)
		}
		c := layout.NewCNode(w, headHeight+bodyHeight, -1)
		c.Seq = float64(len(views))
		views = append(views, &nodeView{node: node, style: style, kind: kind, name: name, rows: rows, keyWidth: keyWidth,
			headHeight: headHeight, w: w, h: headHeight + bodyHeight})
		cnodes = append(cnodes, c)
	}

	rootDir := layout.Direction(graph.direction)
	if turned {
		rootDir = layout.TB
	}
	sideways := rootDir == layout.LR || rootDir == layout.RL
	var cedges []layout.CEdge
	var edgeLabels []text.Label
	var drawn []*graphEdge
	for k := range graph.edges {
		edge := &graph.edges[k]
		if edge.start == "" || edge.end == "" {
			continue
		}
		src, okSrc := index[edge.start]
		dst, okDst := index[edge.end]
		if !okSrc || !okDst {
			continue
		}
		label := text.Layout("«"+edge.typ+"»", false, measurer, small, 4000, 0, icons)
		if src == dst {
			// A self-loop sits beside the node, across the flow, and the node's box grows to
			// hold it.
			if sideways {
				cnodes[src].H += float64(2 * (loopReach + label.Height + 8))
			} else {
				cnodes[src].W += float64(2 * (loopReach + label.Width + 12))
			}
		}
		drawn = append(drawn, edge)
		edgeLabels = append(edgeLabels, label)
		cedges = append(cedges, layout.CEdge{
			Src:    src,
			Dst:    dst,
			Minlen: 1,
			LabelW: label.Width + 12,
			LabelH: label.Height + 4,
			Route:  []float64{},
		})
	}

	result := layout.CompoundLayout(rc.Ctx, cnodes, cedges, rootDir, layout.LayeredOptions{
		NodeSep: nodeSep,
		EdgeSep: edgeSep,
		RankSep: rankSep,
		PortSep: 20,
	})

	spreadEnds(cnodes, cedges, views)

	title := svg.TitleLabel(db.title, measurer, size, 4000, icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}
	inner := max(result.Width, title.Width)
	ox := pad + float64((inner-result.Width)/2)
	oy := pad + titleHeight
	width := math.Ceil(inner + float64(2*pad))
	height := math.Ceil(result.Height + titleHeight + float64(2*pad))

	var nodesOut strings.Builder
	for i, view := range views {
		w, h, style := view.w, view.h, view.style
		left := float64(-w / 2)
		top := float64(-h / 2)
		body := `<rect x="` + svg.Num(left) + `" y="` + svg.Num(top) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) +
			`" fill="var(--_s)" stroke="var(--_b)"` + svg.Rounded(style.Shape) + `/>` +
			svg.LabelSVG(view.kind, 0, top+padY+float64(view.kind.Height/2),
				` class="pele-stereotype" font-size="`+jsnum.Format(view.kind.Size)+`" fill="var(--_m)"`+style.Text, nil) +
			svg.LabelSVG(view.name, 0, top+padY+view.kind.Height+float64(view.name.Height/2),
				` class="pele-label" font-size="`+jsnum.Format(view.name.Size)+`" font-weight="var(--_hw)"`+style.Text, nil)
		if len(view.rows) > 0 {
			divider := top + view.headHeight
			body += `<path class="pele-divider" d="M` + svg.Num(left) + `,` + svg.Num(divider) + `H` + svg.Num(-left) +
				`" stroke="var(--_b)"` + style.Line + `/>`
			y := divider + padY
			var rows strings.Builder
			for _, r := range view.rows {
				rows.WriteString(startText(r.key, left+padX, y, ` class="pele-field" fill="var(--_m)"`+style.Text) +
					startText(r.value, left+padX+view.keyWidth+columnGap, y, ` class="pele-value"`+style.Text))
				y += r.height + rowGap
			}
			body += `<g class="pele-fields" font-size="` + jsnum.Format(view.rows[0].key.Size) + `">` + rows.String() + `</g>`
		}
		kind := "pele-element"
		if view.node.requirement != nil {
			kind = "pele-requirement"
		}
		nodesOut.WriteString(`<g class="pele-node ` + kind + svg.ClassNames(svg.StripDefault(view.node.cssClasses)) +
			`" data-id="` + svg.EscText(view.node.id) + `" transform="translate(` + svg.Num(cnodes[i].X+ox) + `,` +
			svg.Num(cnodes[i].Y+oy) + `)">` + body + `</g>`)
	}

	var edgesOut, labelsOut strings.Builder
	loopSeen := map[int]int{}
	for i := range cedges {
		ce := &cedges[i]
		edge := drawn[i]
		label := edgeLabels[i]
		startTrim := 0.0
		if edge.contains {
			startTrim = 2 * containsRadius
		}
		var path svg.EdgePath
		labelX := ce.LabelX + ox
		labelY := ce.LabelY + oy
		if ce.Src == ce.Dst {
			view := views[ce.Src]
			cx := cnodes[ce.Src].X + ox
			cy := cnodes[ce.Src].Y + oy
			k := float64(loopSeen[ce.Src])
			loopSeen[ce.Src]++
			half := view.h / 2
			if sideways {
				half = view.w / 2
			}
			spread := min(half-4, 12+float64(k*6))
			reach := loopReach + float64(k*8)
			// Local frame: out points away from the node, along runs along its side.
			at := func(out, along float64) string {
				if sideways {
					return svg.Num(cx+along) + "," + svg.Num(cy+float64(view.h/2)+out)
				}
				return svg.Num(cx+float64(view.w/2)+out) + "," + svg.Num(cy+along)
			}
			// A cubic whose control points are this far out peaks at reach.
			pull := reach / 0.75
			path.D = "M" + at(startTrim, -spread) + "C" + at(pull, -spread) + " " + at(pull, spread) + " " + at(0, spread)
			if sideways {
				path.SX, path.SY, path.SDX, path.SDY = cx-spread, cy+float64(view.h/2), 0, -1
				path.EX, path.EY, path.EDX, path.EDY = cx+spread, cy+float64(view.h/2), 0, -1
				labelX = cx
				labelY = cy + float64(view.h/2) + reach + 4 + float64(ce.LabelH/2)
			} else {
				path.SX, path.SY, path.SDX, path.SDY = cx+float64(view.w/2), cy-spread, -1, 0
				path.EX, path.EY, path.EDX, path.EDY = cx+float64(view.w/2), cy+spread, -1, 0
				labelX = cx + float64(view.w/2) + reach + 4 + float64(ce.LabelW/2)
				labelY = cy
			}
		} else {
			route := slices.Clone(ce.Route)
			for k := 0; k < len(route); k += 3 {
				route[k] += ox
				route[k+1] += oy
			}
			path = svg.RoutePath(route, "", startTrim, 0, svg.DefaultReach(startTrim), svg.DefaultReach(0))
		}
		kind, dash, marker := "arrow", ` stroke-dasharray="5 4"`, ""
		if edge.contains {
			kind, dash = "contains", ""
			marker = containsMarker(path.SX, path.SY, path.SDX, path.SDY)
		} else {
			marker = arrowMarker(path.EX, path.EY, path.EDX, path.EDY)
		}
		edgesOut.WriteString(`<g class="pele-edge pele-relationship-` + kind + `" data-id="` + svg.EscText(edge.id) + `">` +
			`<path d="` + path.D + `"` + dash + `/>` + marker + `</g>`)
		labelsOut.WriteString(svg.EdgeLabelSVG(edge.id, label, labelX, labelY, "", nil))
	}

	content := svg.LabelSVG(title, width/2, pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil)
	if edgesOut.Len() > 0 {
		content += `<g class="pele-edges" fill="none" stroke="var(--_l)" stroke-linecap="round">` + edgesOut.String() + `</g>`
	}
	if labelsOut.Len() > 0 {
		content += `<g class="pele-edge-labels" font-size="` + jsnum.Format(small) + `" fill="var(--_m)">` + labelsOut.String() + `</g>`
	}
	content += `<g class="pele-nodes">` + nodesOut.String() + `</g>`

	doc := svg.Document("requirement", width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccTitle: db.accTitle, AccDescr: db.accDescr}, content)
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: []core.Link{}}
}
