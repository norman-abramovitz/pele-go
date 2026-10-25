package usecase

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/layout"
	"github.com/Broderick-Westrope/pele-go/internal/omap"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const (
	nodeSep         = 32
	edgeSep         = 16
	rankSep         = 72
	groupPad        = 20
	loop            = 26
	figureW         = 40
	figureH         = 60
	figureGap       = 6
	tabPad          = 6
	cellPad         = 10
	rowPad          = 8
	fold            = 9
	extensionLength = 12
	portGap         = 14
	shapeAttrs      = ` fill="var(--_s)" stroke="var(--_b)"`
)

const (
	kindActor = iota
	kindEllipse
	kindBox
	kindJSON
	kindBoundary
)

type view struct {
	node       *graphNode
	kind       int
	style      svg.ResolvedStyle
	label      text.Label
	stereotype text.Label
	// The size of what is drawn; the layout box is larger when the node carries a
	// self-relationship.
	w     float64
	h     float64
	rows  [][2]text.Label
	keyW  float64
	headH float64
}

func numberOption(config *jsobj.Object, key string, fallback float64) float64 {
	section, _ := config.Get("usecase")
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

// literal: a plain label is shown as written, with no tags, no markdown and no `\n` breaks.
// Its markup characters are handed to the label parser as character references, which it
// turns back into characters.
func literal(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '&', '<', '>', '\\':
			b.WriteString("&#" + jsnum.Format(float64(c)) + ";")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func layoutText(raw string, markdown bool, m text.Measurer, size, wrap float64, base text.Style, icons *text.Icons) text.Label {
	if raw != "" && !markdown {
		raw = literal(raw)
	}
	return text.Layout(raw, markdown, m, size, wrap, base, icons)
}

// anchor is the point of a node's outline on one of its four sides (0 top, 1 right, 2 bottom,
// 3 left), along away from the middle of that side. Lines meet an actor at its figure, not at
// its label.
func anchor(v *view, c *layout.CNode, side int, along float64) (float64, float64) {
	sign := -1.0
	if side == 1 || side == 2 {
		sign = 1
	}
	upright := side == 1 || side == 3
	reach := v.h / 2
	if upright {
		reach = v.w / 2
	}
	middle := 0.0
	switch v.kind {
	case kindActor:
		if upright {
			reach = figureW/2 + 3
			middle = (figureH - v.h) / 2
		} else {
			reach += 2
		}
	case kindEllipse:
		span := v.w / 2
		if upright {
			span = v.h / 2
		}
		q := along / span
		reach *= math.Sqrt(max(1-float64(q*q), 0))
	}
	if upright {
		return c.X + float64(sign*reach), c.Y + middle + along
	}
	return c.X + along, c.Y + float64(sign*reach)
}

// sideLength is how much of a side edges may spread over.
func sideLength(v *view, side int) float64 {
	upright := side == 1 || side == 3
	if v.kind == kindActor {
		if upright {
			return float64(figureH) * 0.5
		}
		return float64(figureW) * 0.6
	}
	length := v.w
	if upright {
		length = v.h
	}
	if v.kind == kindEllipse {
		return length * 0.6
	}
	return length - 16
}

type end struct {
	route  []float64
	at     int
	toward float64
}

// attachEnds: the layout brings every edge that ends on one side of a node to the middle of
// that side. This spaces them along the side in the order they fan out, and puts each on the
// node's outline.
func attachEnds(cedges []layout.CEdge, cnodes []layout.CNode, views []*view) {
	var sides omap.Map[int, []*end]
	add := func(index int, route []float64, at, next int) {
		c := &cnodes[index]
		if c.IsGroup {
			return
		}
		dx := route[at] - c.X
		dy := route[at+1] - c.Y
		var side int
		switch {
		case math.Abs(math.Abs(dy)-float64(c.H/2)) < 0.5:
			side = 2
			if dy < 0 {
				side = 0
			}
		case math.Abs(math.Abs(dx)-float64(c.W/2)) < 0.5:
			side = 1
			if dx < 0 {
				side = 3
			}
		default:
			return
		}
		key := index*4 + side
		e := &end{route: route, at: at, toward: route[next+side&1]}
		list, _ := sides.Get(key)
		sides.Set(key, append(list, e))
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
		index := key >> 2
		side := key & 3
		v := views[index]
		step := 0.0
		if len(ends) > 1 {
			step = min(portGap, sideLength(v, side)/float64(len(ends)-1))
		}
		slices.SortStableFunc(ends, func(a, b *end) int { return cmp.Compare(a.toward, b.toward) })
		for i, e := range ends {
			x, y := anchor(v, &cnodes[index], side, (float64(i)-float64(float64(len(ends)-1)/2))*step)
			e.route[e.at] = x
			e.route[e.at+1] = y
		}
	}
}

// extension is the hollow triangle of a generalization, with its tip at (x, y).
func extension(x, y, dx, dy float64, color string) string {
	bx := x - float64(dx*extensionLength)
	by := y - float64(dy*extensionLength)
	const half = extensionLength * 0.5
	return `<path class="pele-marker" d="M` + svg.Num(x) + `,` + svg.Num(y) +
		`L` + svg.Num(bx-float64(dy*half)) + `,` + svg.Num(by+float64(dx*half)) +
		`L` + svg.Num(bx+float64(dy*half)) + `,` + svg.Num(by-float64(dx*half)) +
		`Z" fill="var(--_bg)" stroke="` + color + `" stroke-linejoin="round"/>`
}

func trim(typ string) float64 {
	if typ == "extension" {
		return extensionLength
	}
	return svg.MarkerTrim(typ)
}

func mark(typ string, x, y, dx, dy float64, color string) string {
	if typ == "extension" {
		return extension(x, y, dx, dy, color)
	}
	return svg.Marker(typ, x, y, dx, dy, color)
}

func figure(v *view, icons *text.Icons) string {
	node := v.node
	shape := v.style.Shape
	line := ` fill="none" stroke="var(--_b)"` + v.style.Line
	head := -21.0
	radius := 9.0
	var out string
	switch node.actorType {
	case "hollow":
		radius = 8
		out = `<circle cy="-21" r="8" fill="var(--_bg)" stroke="var(--_b)"` + shape + `/>` +
			`<path d="M-20,-9H20V0H5.5L20,15.5L12,25.5L0,12L-12,25.5L-20,15.5L-5.5,0H-20Z" fill="var(--_bg)" stroke="var(--_b)" stroke-linejoin="round"` + shape + `/>`
	case "awesome":
		out = `<path d="M0,-30A11,11 0 1 1 0,-8A11,11 0 1 1 0,-30ZM-20,25C-20,9 -12,-3 0,-3C12,-3 20,9 20,25C20,28 18,30 15,30H-15C-18,30 -20,28 -20,25Z" fill="var(--_b)"` + shape + `/>`
	case "icon":
		head = -14
		radius = 12
		out = `<rect x="-22" y="-26" width="44" height="44"` + svg.Rounded(shapeAttrs+shape) + `/>` +
			`<svg class="pele-icon" data-icon="` + svg.EscText(node.icon) + `" x="-14" y="-18" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
			icons.Resolve(node.icon) + `</svg>`
	default:
		out = `<circle cy="-21" r="9"` + shapeAttrs + shape + `/><path d="M0,-12V10M-14,-3H14M0,10L-12,30M0,10L12,30"` + line + `/>`
	}
	if node.business {
		// The slash of a business actor: a chord across the lower right of the head.
		out += `<path class="pele-business" d="M` + svg.Num(0.12*radius) + `,` + svg.Num(head+float64(0.99*radius)) +
			`L` + svg.Num(0.92*radius) + `,` + svg.Num(head-float64(0.39*radius)) + `"` + line + `/>`
	}
	return out
}

func renderUsecase(model *usecaseModel, config *jsobj.Object, rc *core.Context) core.Rendered {
	if !common.RunsAcross(model.direction) {
		return draw(model, config, rc, false)
	}
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered { return draw(model, config, rc, down) })
}

// trimRelationship is s.replace(/^default relationship relationship-\S+\s?/, ”).
func trimRelationship(s string) string {
	rest, ok := strings.CutPrefix(s, "default relationship relationship-")
	if !ok {
		return s
	}
	i := 0
	for i < len(rest) {
		r, n := jsstr.Decode(rest, i)
		if jsstr.IsSpace(r) {
			break
		}
		i += n
	}
	if i == 0 {
		return s
	}
	if i < len(rest) {
		_, n := jsstr.Decode(rest, i)
		i += n
	}
	return rest[i:]
}

// draw: turned draws a diagram that runs across as one that runs down.
func draw(model *usecaseModel, config *jsobj.Object, rc *core.Context, turned bool) core.Rendered {
	options := rc.Options
	graph := buildUsecaseGraph(model)
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	small := jsmath.Round(size * 0.875)
	tiny := jsmath.Round(size * 0.8125)
	measurer := rc.Measurer()
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	icons := rc.Icons
	rootDir := layout.Direction(model.direction)
	if turned {
		rootDir = layout.TB
	}
	sideways := rootDir == layout.LR || rootDir == layout.RL

	index := map[string]int{}
	var views []*view
	var cnodes []layout.CNode
	for i, node := range graph.nodes {
		index[node.id] = i
	}

	for _, node := range graph.nodes {
		declarations := node.cssCompiledStyles
		if len(node.cssStyles) > 0 {
			declarations = append(slices.Clone(node.cssCompiledStyles), node.cssStyles...)
		}
		style := svg.ResolveStyle(declarations)
		var base text.Style
		if style.Bold {
			base |= text.Bold
		}
		if style.Italic {
			base |= text.Italic
		}
		fontSize := size
		if style.FontSize != 0 {
			fontSize = style.FontSize
		}
		markdown := node.labelType == "markdown"
		stereotypeText := ""
		if node.stereotype != "" {
			stereotypeText = "«" + node.stereotype + "»"
		}
		stereotype := layoutText(stereotypeText, false, measurer, tiny, 200, base, icons)
		v := &view{node: node, kind: kindBox, style: style, label: stereotype, stereotype: stereotype}
		var c layout.CNode
		if node.isGroup {
			v.kind = kindBoundary
			v.label = layoutText(node.label, markdown, measurer, fontSize, 4000, base, icons)
			titleH := v.label.Height
			c = layout.NewCNode(0, 0, -1)
			c.IsGroup = true
			c.PadX = groupPad
			c.PadBottom = groupPad
			switch {
			case node.boundaryType == "package":
				c.PadTop = titleH + 2*tabPad + 16
			case titleH > 0:
				c.PadTop = titleH + 16
			default:
				c.PadTop = groupPad
			}
			c.MinW = v.label.Width + 2*groupPad
		} else {
			switch {
			case node.isJSON:
				v.kind = kindJSON
				v.label = layoutText(node.label, false, measurer, fontSize, 4000, base|text.Bold, icons)
				valueW := 0.0
				for _, row := range node.jsonRows {
					key := layoutText(row.key, false, measurer, small, 4000, base, icons)
					value := layoutText(row.value, false, measurer, small, 260, base, icons)
					v.rows = append(v.rows, [2]text.Label{key, value})
					v.keyW = max(v.keyW, key.Width)
					valueW = max(valueW, value.Width)
					v.h += max(key.Height, value.Height, jsmath.Round(small*1.5)) + rowPad
				}
				// The root of an empty object has a value and no key, and then there is no key column.
				if v.keyW > 0 {
					v.keyW += 2 * cellPad
				}
				v.headH = max(v.label.Height, jsmath.Round(fontSize*1.5)) + rowPad
				columns := 0.0
				if len(v.rows) > 0 {
					columns = v.keyW + valueW + 2*cellPad
				}
				v.w = max(v.label.Width+2*cellPad, columns, 72)
				v.h += v.headH
			case node.actorType != "":
				v.kind = kindActor
				v.label = layoutText(node.label, markdown, measurer, fontSize, 140, base, icons)
				v.w = max(figureW, v.label.Width, stereotype.Width)
				v.h = figureH + figureGap + stereotype.Height + v.label.Height
			default:
				labelSize := fontSize
				if node.shape == "note" {
					labelSize = small
				}
				v.label = layoutText(node.label, markdown, measurer, labelSize, 180, base, icons)
				tw := max(v.label.Width, stereotype.Width)
				th := v.label.Height + stereotype.Height
				padding2 := float64(2 * node.padding)
				switch node.shape {
				case "usecaseEllipse", "usecaseBusiness":
					v.kind = kindEllipse
					// An oval needs more room around its text than a box, and a business oval
					// room for its slash.
					extra := 4.0
					if node.business {
						extra = 16
					}
					v.w = max(tw+padding2+extra, 88)
					v.h = max(th+28, 48)
				case "note":
					v.w = tw + padding2 + 6
					v.h = th + 18
				default:
					v.w = max(tw+padding2+12, 80)
					v.h = max(th+24, 44)
				}
			}
			c = layout.NewCNode(v.w, v.h, -1)
		}
		c.Seq = float64(len(views))
		views = append(views, v)
		cnodes = append(cnodes, c)
	}
	for i, node := range graph.nodes {
		cnodes[i].Parent = -1
		if node.parentID != "" {
			if up, ok := index[node.parentID]; ok {
				cnodes[i].Parent = up
			}
		}
	}

	var cedges []layout.CEdge
	var edgeLabels []text.Label
	var drawn []*graphEdge
	for _, edge := range graph.edges {
		src, okSrc := index[edge.start]
		dst, okDst := index[edge.end]
		if !okSrc || !okDst {
			continue
		}
		semantic := edge.relationshipType == "include" || edge.relationshipType == "extend"
		raw := edge.label
		if semantic {
			raw = "«" + edge.label + "»"
		}
		label := layoutText(raw, edge.labelType == "markdown", measurer, small, 200, 0, icons)
		if src == dst {
			// A self-relationship loops out across the flow, and the node's box grows to hold it.
			if sideways {
				room := 0.0
				if label.Height > 0 {
					room = label.Height + 8
				}
				cnodes[src].H += 2 * (loop + room)
			} else {
				room := 0.0
				if label.Width > 0 {
					room = label.Width + 12
				}
				cnodes[src].W += 2 * (loop + room)
			}
		}
		drawn = append(drawn, edge)
		edgeLabels = append(edgeLabels, label)
		ce := layout.CEdge{Src: src, Dst: dst, Minlen: float64(edge.minlen)}
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

	title := svg.TitleLabel(model.title, measurer, size, 4000, icons)
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

	// Where edges cross each boundary's top border, so that a title can keep out of their way.
	crossings := svg.Crossings{}
	for i := range cedges {
		svg.MarkCrossings(crossings, cedges[i].Route)
	}
	attachEnds(cedges, cnodes, views)

	var clusters, nodesOut strings.Builder
	for i, v := range views {
		c := &cnodes[i]
		node := v.node
		classes := svg.ClassNames(svg.StripDefault(node.cssClasses))
		id := svg.EscText(node.id)
		textAttrs := v.style.Text
		shape := v.style.Shape
		w := v.w
		h := v.h

		if v.kind == kindBoundary {
			x := c.X - float64(c.W/2)
			y := c.Y - float64(c.H/2)
			box := ` fill="var(--_a)" fill-opacity="0.5" stroke="var(--_b)"` + shape
			titleAttrs := ` class="pele-cluster-label" fill="var(--_m)"` + textAttrs
			var body string
			if node.boundaryType == "package" {
				tabW := min(v.label.Width+2*tabPad+8, c.W)
				tabH := v.label.Height + 2*tabPad
				body = `<rect class="pele-boundary-tab" x="` + svg.Num(x) + `" y="` + svg.Num(y) + `" width="` + svg.Num(tabW) + `" height="` + svg.Num(tabH) + `"` + box + `/>` +
					`<rect x="` + svg.Num(x) + `" y="` + svg.Num(y+tabH) + `" width="` + svg.Num(c.W) + `" height="` + svg.Num(c.H-tabH) + `"` + box + `/>` +
					svg.LabelSVG(v.label, x+float64(tabW/2), y+float64(tabH/2), titleAttrs, icons)
			} else {
				body = `<rect x="` + svg.Num(x) + `" y="` + svg.Num(y) + `" width="` + svg.Num(c.W) + `" height="` + svg.Num(c.H) + `"` + svg.Rounded(box) + `/>` +
					svg.LabelSVG(v.label, titleX(c, v.label.Width, crossings), y+8+float64(v.label.Height/2), titleAttrs, icons)
			}
			clusters.WriteString(`<g class="pele-cluster pele-boundary` + classes + `" data-id="` + id + `">` + body + `</g>`)
			continue
		}

		var body, kind string
		switch v.kind {
		case kindActor:
			kind = "pele-actor"
			top := float64(-h / 2)
			stereotypeY := top + figureH + figureGap + float64(v.stereotype.Height/2)
			body = `<g class="pele-actor-figure" transform="translate(0,` + svg.Num(top+figureH/2) + `)">` + figure(v, icons) + `</g>` +
				svg.LabelSVG(v.stereotype, 0, stereotypeY, ` class="pele-stereotype" font-size="`+jsnum.Format(tiny)+`" fill="var(--_m)"`, icons) +
				svg.LabelSVG(v.label, 0, float64(h/2)-float64(v.label.Height/2), ` class="pele-label"`+textAttrs, icons)
		case kindJSON:
			kind = "pele-json"
			x0 := float64(-w / 2)
			y := float64(-h/2) + v.headH
			var cells strings.Builder
			lines := "M" + svg.Num(x0) + "," + svg.Num(y) + "H" + svg.Num(x0+w)
			if v.keyW > 0 {
				lines += "M" + svg.Num(x0+v.keyW) + "," + svg.Num(y) + "V" + svg.Num(float64(h/2))
			}
			for _, row := range v.rows {
				key, value := row[0], row[1]
				rowH := max(key.Height, value.Height, jsmath.Round(small*1.5)) + rowPad
				cells.WriteString(svg.LabelSVG(key, x0+cellPad+float64(key.Width/2), y+float64(rowH/2), ` class="pele-json-key" fill="var(--_m)"`, icons) +
					leftLabel(value, x0+v.keyW+cellPad, y+float64(rowH/2), ` class="pele-json-value"`+textAttrs, icons))
				y += rowH
			}
			body = `<rect x="` + svg.Num(x0) + `" y="` + svg.Num(-h/2) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) + `"` + svg.Rounded(shapeAttrs+shape) + `/>` +
				`<path class="pele-json-lines" d="` + lines + `" fill="none" stroke="var(--_b)"` + v.style.Line + `/>` +
				svg.LabelSVG(v.label, 0, float64(-h/2)+float64(v.headH/2), ` class="pele-label" font-weight="var(--_hw)"`+textAttrs, icons) +
				`<g font-size="` + jsnum.Format(small) + `">` + cells.String() + `</g>`
		default:
			labelY := v.stereotype.Height / 2
			var outline string
			switch {
			case v.kind == kindEllipse:
				kind = "pele-usecase"
				outline = `<ellipse rx="` + svg.Num(w/2) + `" ry="` + svg.Num(h/2) + `"` + shapeAttrs + shape + `/>`
				if node.business {
					// The slash of a business use case, cutting off the right end of the oval
					// beside the text.
					from := float64(w/2) - 15
					to := float64(w/2) - 5
					arc := func(at float64) float64 {
						q := at / (w / 2)
						return (h / 2) * math.Sqrt(1-float64(q*q))
					}
					outline += `<path class="pele-business" d="M` + svg.Num(from) + `,` + svg.Num(arc(from)) + `L` + svg.Num(to) + `,` + svg.Num(-arc(to)) +
						`" fill="none" stroke="var(--_b)"` + v.style.Line + `/>`
				}
			case node.shape == "note":
				kind = "pele-note"
				r := float64(w / 2)
				b := float64(h / 2)
				outline = `<path d="M` + svg.Num(-r) + `,` + svg.Num(-b) + `H` + svg.Num(r-fold) + `L` + svg.Num(r) + `,` + svg.Num(fold-b) + `V` + svg.Num(b) + `H` + svg.Num(-r) +
					`Z" fill="var(--_a)" stroke="var(--_b)" stroke-linejoin="round"` + shape + `/>` +
					`<path d="M` + svg.Num(r-fold) + `,` + svg.Num(-b) + `V` + svg.Num(fold-b) + `H` + svg.Num(r) + `" fill="none" stroke="var(--_b)" stroke-linejoin="round"` + v.style.Line + `/>`
			default:
				kind = "pele-usecase"
				outline = `<rect x="` + svg.Num(-w/2) + `" y="` + svg.Num(-h/2) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) + `"` + svg.Rounded(shapeAttrs+shape) + `/>`
			}
			noteSize := ""
			if node.shape == "note" {
				noteSize = ` font-size="` + jsnum.Format(small) + `"`
			}
			body = outline +
				svg.LabelSVG(v.stereotype, 0, -v.label.Height/2, ` class="pele-stereotype" font-size="`+jsnum.Format(tiny)+`" fill="var(--_m)"`, icons) +
				svg.LabelSVG(v.label, 0, labelY, ` class="pele-label"`+noteSize+textAttrs, icons)
		}
		nodesOut.WriteString(`<g class="pele-node ` + kind + classes + `" data-id="` + id + `" transform="translate(` + svg.Num(c.X) + `,` + svg.Num(c.Y) + `)">` + body + `</g>`)
	}

	var edgesOut, labelsOut strings.Builder
	loopSeen := map[int]int{}
	for i := range cedges {
		ce := &cedges[i]
		edge := drawn[i]
		label := edgeLabels[i]
		var style *svg.ResolvedStyle
		if len(edge.style) > 0 || len(edge.cssCompiledStyles) > 0 {
			resolved := svg.ResolveStyle(append(slices.Clone(edge.cssCompiledStyles), edge.style...))
			style = &resolved
		}
		startType := edge.arrowTypeStart
		endType := edge.arrowTypeEnd
		var path svg.EdgePath
		if ce.Src == ce.Dst {
			c := &cnodes[ce.Src]
			v := views[ce.Src]
			k := float64(loopSeen[ce.Src])
			loopSeen[ce.Src]++
			side := 1
			if sideways {
				side = 2
			}
			spread := min(sideLength(v, side)/2, 8+float64(k*6))
			reach := loop + float64(k*8)
			ax, ay := anchor(v, c, side, -spread)
			bx, by := anchor(v, c, side, spread)
			length := jsmath.Hypot(reach, spread)
			if length == 0 || math.IsNaN(length) {
				length = 1
			}
			ux, uy := reach/length, spread/length
			if sideways {
				ux, uy = spread/length, reach/length
			}
			st := trim(startType)
			et := trim(endType)
			// Both ends leave the node leaning away from each other, and the curve joins them
			// outside it.
			out := func(x, y, sign, far float64) string {
				if sideways {
					return svg.Num(x+float64(sign*spread)) + "," + svg.Num(y+far)
				}
				return svg.Num(x+far) + "," + svg.Num(y+float64(sign*spread))
			}
			sx, sy := ax+float64(ux*st), ay-float64(uy*st)
			if sideways {
				sx, sy = ax-float64(ux*st), ay+float64(uy*st)
			}
			ex := bx + float64(ux*et)
			ey := by + float64(uy*et)
			path = svg.EdgePath{
				D:   "M" + svg.Num(sx) + "," + svg.Num(sy) + "C" + out(ax, ay, -1, reach) + " " + out(bx, by, 1, reach) + " " + svg.Num(ex) + "," + svg.Num(ey),
				SX:  ax,
				SY:  ay,
				SDX: -ux,
				SDY: uy,
				EX:  bx,
				EY:  by,
				EDX: -ux,
				EDY: -uy,
			}
			if sideways {
				path.SDX, path.SDY = ux, -uy
				ce.LabelX = c.X
				ce.LabelY = max(ay, by) + float64(reach*0.75) + 6 + float64(label.Height/2)
			} else {
				ce.LabelX = max(ax, bx) + float64(reach*0.75) + 8 + float64(label.Width/2)
				ce.LabelY = c.Y
			}
		} else {
			if len(ce.Route) < 6 {
				continue
			}
			st, et := trim(startType), trim(endType)
			path = svg.RoutePath(ce.Route, "", st, et, svg.DefaultReach(st), svg.DefaultReach(et))
		}
		color := "var(--_l)"
		lineAttrs, textStyle := "", ""
		if style != nil {
			if style.Stroke != "" {
				color = svg.Esc(style.Stroke)
			}
			lineAttrs, textStyle = style.Line, style.Text
		}
		// Mermaid animates these dashes. A still picture keeps the dashes.
		dashes := ""
		switch {
		case edge.internal:
			dashes = "2 3"
		case edge.pattern == "dotted":
			dashes = "5 4"
		case edge.animate:
			dashes = "9 5"
		}
		dashAttr := ""
		if dashes != "" {
			dashAttr = ` stroke-dasharray="` + dashes + `"`
		}
		edgesOut.WriteString(`<g class="pele-edge pele-` + edge.relationshipType + svg.ClassNames(trimRelationship(edge.classes)) + `" data-id="` + svg.EscText(edge.id) + `"` + lineAttrs + `>` +
			`<path d="` + path.D + `"` + dashAttr + `/>` +
			mark(startType, path.SX, path.SY, path.SDX, path.SDY, color) +
			mark(endType, path.EX, path.EY, path.EDX, path.EDY, color) +
			`</g>`)
		if label.Width > 0 {
			semantic := edge.relationshipType == "include" || edge.relationshipType == "extend"
			attrs := ""
			if semantic {
				attrs = ` fill="var(--_m)"`
			}
			labelsOut.WriteString(svg.EdgeLabelSVG(edge.id, label, ce.LabelX, ce.LabelY, attrs+textStyle, icons))
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
		content += `<g class="pele-edge-labels" font-size="` + jsnum.Format(small) + `">` + labelsOut.String() + `</g>`
	}
	content += `<g class="pele-nodes">` + nodesOut.String() + `</g>`

	doc := svg.Document("usecase", width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccTitle: model.accTitle, AccDescr: model.accDescr}, content)
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: []core.Link{}}
}

// titleX: a boundary's title sits in the middle of its top edge. Where edges enter through
// that edge it moves into the stretch between them that is nearest the middle, or the widest
// if none has room.
func titleX(c *layout.CNode, width float64, crossings svg.Crossings) float64 {
	xs, ok := crossings[jsmath.Round(c.Y-float64(c.H/2))]
	if width == 0 || !ok {
		return c.X
	}
	left := c.X - float64(c.W/2) + 6
	right := c.X + float64(c.W/2) - 6
	var cuts []float64
	for _, x := range xs {
		if x > left && x < right {
			cuts = append(cuts, x)
		}
	}
	slices.SortStableFunc(cuts, cmp.Compare[float64])
	cuts = append(cuts, right)
	need := float64(width/2) + 6
	best := c.X
	bestOff := math.Inf(1)
	widest := 0.0
	from := left
	for _, cut := range cuts {
		if cut-from >= float64(2*need) {
			x := min(max(c.X, from+need), cut-need)
			if math.Abs(x-c.X) < bestOff {
				bestOff = math.Abs(x - c.X)
				best = x
			}
		} else if math.IsInf(bestOff, 1) && cut-from > widest {
			widest = cut - from
			best = (from + cut) / 2
		}
		from = cut
	}
	return best
}

func leftLabel(label text.Label, x, cy float64, attrs string, icons *text.Icons) string {
	if len(label.Lines) < 2 {
		return svg.LabelSVG(label, x+float64(label.Width/2), cy, attrs, icons)
	}
	var out strings.Builder
	for i := range label.Lines {
		out.WriteString(svg.LabelSVG(text.LabelLine(label, i), x+float64(label.Widths[i]/2), cy-float64(label.Height/2)+text.LineMiddle(label, i), attrs, icons))
	}
	return out.String()
}
