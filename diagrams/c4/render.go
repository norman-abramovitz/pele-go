package c4

import (
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

const (
	pairShift = 7
	narrowGap = 36
	widen     = 24
	maxOffset = 2000
	// A label may cover this much of a shape's edge, where there is no text.
	edge = 8
	// Above this many box comparisons, labels stay at the middle and lines are not steered
	// around shapes.
	careBudget = 2000000
)

// Places along a relation to try for its label, nearest the middle first.
var slide = [...]float64{0.5, 0.4, 0.6, 0.3, 0.7, 0.2, 0.8}

func userStyle(fill, stroke, color string) svg.ResolvedStyle {
	var declarations []string
	if fill != "" {
		declarations = append(declarations, "fill:"+fill)
	}
	if stroke != "" {
		declarations = append(declarations, "stroke:"+stroke)
	}
	if color != "" {
		declarations = append(declarations, "color:"+color)
	}
	return svg.ResolveStyle(declarations)
}

// unhideEntities is url.replace(/\ufb02\u00b0\u00b0?/g, '#').replace(/\u00b6\u00df/g, ';').
func unhideEntities(url string) string {
	url = strings.ReplaceAll(url, "\ufb02\u00b0\u00b0", "#")
	url = strings.ReplaceAll(url, "\ufb02\u00b0", "#")
	return strings.ReplaceAll(url, "\u00b6\u00df", ";")
}

func link(body, url, id string, links *[]core.Link, options core.Options) string {
	if url == "" {
		return body
	}
	// Entity encoding has hidden any `#name;` in the address. Put it back, so that the check
	// sees what a browser would.
	if strings.Contains(url, "\u00b6\u00df") {
		url = unhideEntities(url)
	}
	return util.Linked(body, util.SanitizeURL(url), id, links, options, "")
}

func renderC4(db *c4Db, rc *core.Context) core.Rendered {
	return common.Tighten(rc.Options, func(tight bool) core.Rendered {
		if !tight {
			r, _ := draw(db, rc, false, narrowGap)
			return r
		}
		best, squeezed := draw(db, rc, true, narrowGap)
		for gap := float64(narrowGap + widen); squeezed > 0 && gap <= narrowGap+3*widen; gap += widen {
			wider, left := draw(db, rc, true, gap)
			if left < squeezed {
				best, squeezed = wider, left
			}
		}
		return best
	})
}

func draw(db *c4Db, rc *core.Context, tight bool, gap float64) (core.Rendered, int) {
	options := rc.Options
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	small := jsmath.Round(float64(size * 0.8125))
	measurer := rc.Measurer()
	icons := rc.Icons
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	links := []core.Link{}
	room := 0.0
	if tight {
		maxWidth := 320.0
		if options.MaxWidth != nil {
			maxWidth = *options.MaxWidth
		}
		room = max(80, maxWidth-float64(2*pad))
	}
	lay := layoutC4(db, measurer, icons, size, small, room, tight, gap)
	order, boxes := lay.order, lay.boxes
	root := order[0]

	minX, minY := 0.0, 0.0
	maxX, maxY := root.w, root.h
	grow := func(x0, y0, x1, y1 float64) {
		minX = min(minX, x0)
		minY = min(minY, y0)
		maxX = max(maxX, x1)
		maxY = max(maxY, y1)
	}

	ends := make([]*box, 0, 2*len(db.rels))
	pairs := map[*box]map[*box]bool{}
	for _, r := range db.rels {
		var a, b *box
		if r.fromAttr == nil {
			if from := db.getC4Shape(r.from); from != nil {
				a = boxes[from]
			}
		}
		if r.toAttr == nil {
			if to := db.getC4Shape(r.to); to != nil {
				b = boxes[to]
			}
		}
		if a == nil || b == nil {
			e := perr.New(`C4 rel "`+r.from+`" -> "`+r.to+`" references an unknown shape or boundary`, perr.Semantic)
			e.Type = "c4"
			perr.Throw(e)
		}
		ends = append(ends, a, b)
		targets := pairs[a]
		if targets == nil {
			targets = map[*box]bool{}
			pairs[a] = targets
		}
		targets[b] = true
	}

	// What a relation's label should not cover: shapes, arrowheads, boundary headings, and
	// the labels placed before it.
	var nodes []*box
	var taken []float64
	for _, g := range order {
		for _, n := range g.nodes {
			nodes = append(nodes, &n.box)
			taken = append(taken, n.x+edge, n.y+edge, n.x+n.w-edge, n.y+n.h-edge)
		}
	}
	careful := len(db.rels)*(len(nodes)+len(order)+3*len(db.rels)) <= careBudget
	covered := func(x0, y0, x1, y1 float64) float64 {
		area := 0.0
		for i := 0; i < len(taken); i += 4 {
			w := min(x1, taken[i+2]) - max(x0, taken[i])
			if w <= 0 {
				continue
			}
			h := min(y1, taken[i+3]) - max(y0, taken[i+1])
			if h > 0 {
				area += float64(w * h)
			}
		}
		return area
	}
	offset := func(value float64) float64 {
		if !truthy(value) {
			return 0
		}
		return max(-maxOffset, min(maxOffset, value))
	}

	// Every line is drawn before any label is placed, so that labels can keep off the
	// arrowheads.
	var edgesOut strings.Builder
	trim := svg.MarkerTrim("arrow_point")
	ls := newLanes()
	for _, g := range order {
		if g != root {
			ls.frame(&g.box)
		}
	}
	routes := make([]route, len(db.rels))
	for index, r := range db.rels {
		a := ends[index*2]
		b := ends[index*2+1]
		style := userStyle("", r.lineColor, "")
		color := "var(--_l)"
		if style.Stroke != "" {
			color = svg.Esc(style.Stroke)
		}
		startArrow := r.typ == "birel" || r.typ == "rel_b"
		endArrow := r.typ != "rel_b"
		shift := 0.0
		if a != b && pairs[b][a] {
			shift = pairShift
		}
		startTrim, endTrim := 0.0, 0.0
		if startArrow {
			startTrim = trim
		}
		if endArrow {
			endTrim = trim
		}
		rt := routeRel(a, b, shift, nodes, careful, ls, startTrim, endTrim)
		grow(min(rt.x0, rt.x1), min(rt.y0, rt.y1), max(rt.x0, rt.x1), max(rt.y0, rt.y1))
		if startArrow && careful {
			taken = append(taken, rt.sx-svg.Arrow, rt.sy-svg.Arrow, rt.sx+svg.Arrow, rt.sy+svg.Arrow)
		}
		if endArrow && careful {
			taken = append(taken, rt.ex-svg.Arrow, rt.ey-svg.Arrow, rt.ex+svg.Arrow, rt.ey+svg.Arrow)
		}
		edgesOut.WriteString(`<g class="pele-edge" data-id="` + svg.EscText(r.from+"-"+r.to) + `"` + style.Line + `><path d="` + rt.d + `"/>`)
		if startArrow {
			edgesOut.WriteString(svg.Marker("arrow_point", rt.sx, rt.sy, rt.sdx, rt.sdy, color))
		}
		if endArrow {
			edgesOut.WriteString(svg.Marker("arrow_point", rt.ex, rt.ey, rt.edx, rt.edy, color))
		}
		edgesOut.WriteString("</g>")
		routes[index] = rt
	}

	// A boundary's heading goes at the left, the middle or the right of its top edge: the
	// first of them no line crosses.
	for _, g := range order {
		if !careful || g.headW == 0 {
			continue
		}
		top := g.y + headTop
		bottom := g.y + g.head - lane
		fewest := math.MaxInt
		for _, x := range [...]float64{headLeft, (g.w - g.headW) / 2, g.w - headLeft - g.headW} {
			count := 0
			for _, rt := range routes {
				if crosses(rt.points, g.x+x-4, top, g.x+x+g.headW+4, bottom) {
					count++
				}
			}
			if count < fewest {
				fewest = count
				g.headX = x
			}
			if count == 0 {
				break
			}
		}
		taken = append(taken, g.x+g.headX-4, top, g.x+g.headX+g.headW+4, bottom)
	}

	var labelsOut strings.Builder
	for index, r := range db.rels {
		rt := routes[index]
		id := r.from + "-" + r.to
		style := userStyle("", "", r.textColor)
		txt := r.label
		if db.c4Type == "C4Dynamic" {
			txt = jsnum.Format(float64(index+1)) + ": " + txt
		}
		label := text.Layout(txt, false, measurer, small, rt.room, 0, icons)
		technText := ""
		if r.techn != "" {
			technText = "[" + r.techn + "]"
		}
		techn := text.Layout(technText, false, measurer, small, rt.room, text.Italic, icons)
		lw := max(label.Width, techn.Width) + labelPad
		lh := label.Height + techn.Height
		if lh == 0 {
			continue
		}

		// The label sits on the line, or just beside it when another relation runs the
		// opposite way.
		beside := float64(math.Abs(rt.nx)*(lw/2)) + float64(math.Abs(rt.ny)*(lh/2)) + 2
		aside := beside
		if truthy(rt.lean) {
			aside = max(0, min(rt.lean, beside-14))
		}
		// Manual offsets are tuned to the original spacing. Reposition compact labels beside
		// their routed arrows so an offset cannot leave them floating outside a boundary.
		offsetX, offsetY := 0.0, 0.0
		if !tight {
			offsetX, offsetY = offset(r.offsetX), offset(r.offsetY)
		}
		dx := float64(rt.nx*aside) + offsetX
		dy := float64(rt.ny*aside) + offsetY
		at := 0.5
		if careful {
			// Slide along the line to the spot that covers least. Offsets from the diagram are
			// kept unless a spot on the line itself covers less than any they lead to: they are
			// often tuned for Mermaid's spacing.
			least := math.Inf(1)
			moved := dx != float64(rt.nx*aside) || dy != float64(rt.ny*aside)
			passes := 1
			if moved {
				passes = 2
			}
			for pass := 0; pass < passes && least > 0; pass++ {
				px, py := dx, dy
				if pass != 0 {
					px, py = float64(rt.nx*aside), float64(rt.ny*aside)
				}
				for _, t := range slide {
					cx := rt.x0 + float64((rt.x1-rt.x0)*t) + px
					cy := rt.y0 + float64((rt.y1-rt.y0)*t) + py
					area := covered(cx-float64(lw/2), cy-float64(lh/2), cx+float64(lw/2), cy+float64(lh/2))
					if area < least {
						least = area
						at = t
						dx = px
						dy = py
					}
					if area == 0 {
						break
					}
				}
			}
		}
		cx := rt.x0 + float64((rt.x1-rt.x0)*at) + dx
		top := rt.y0 + float64((rt.y1-rt.y0)*at) + dy - float64(lh/2)
		if careful {
			taken = append(taken, cx-float64(lw/2), top, cx+float64(lw/2), top+lh)
		}
		grow(cx-float64(lw/2), top, cx+float64(lw/2), top+lh)
		labelsOut.WriteString(`<g class="pele-edge-label" data-id="` + svg.EscText(id) + `">`)
		if r.descr != "" {
			labelsOut.WriteString(`<title>` + svg.EscText(r.descr) + `</title>`)
		}
		labelsOut.WriteString(`<rect x="` + svg.Num(cx-float64(lw/2)) + `" y="` + svg.Num(top) + `" width="` + svg.Num(lw) + `" height="` + svg.Num(lh) + `" rx="3" fill="var(--_bg)"/>`)
		labelsOut.WriteString(link(
			svg.LabelSVG(label, cx, top+float64(label.Height/2), style.Text, nil)+
				svg.LabelSVG(techn, cx, top+label.Height+float64(techn.Height/2), ` class="pele-c4-techn" fill="var(--_m)"`+style.Text, nil),
			r.link, id, &links, options))
		labelsOut.WriteString("</g>")
	}

	smallText := jsnum.Format(small)
	var clusters, nodesOut strings.Builder
	for _, g := range order {
		if g != root {
			boundary := g.boundary
			style := userStyle(boundary.bgColor, boundary.borderColor, boundary.fontColor)
			x := g.x + g.headX
			y := g.y + headTop
			txt := common.LeftLabelSVG(g.labels[0], x, y, ` class="pele-cluster-label" font-weight="var(--_hw)"`+style.Text)
			y += g.labels[0].Height
			txt += common.LeftLabelSVG(g.labels[1], x, y, ` class="pele-c4-type" font-size="`+smallText+`" fill="var(--_m)"`+style.Text)
			y += g.labels[1].Height
			txt += common.LeftLabelSVG(g.labels[2], x, y, ` class="pele-c4-descr" font-size="`+smallText+`" fill="var(--_m)"`+style.Text)
			kind, dash := "boundary", ` stroke-dasharray="6 4"`
			if boundary.nodeType != "" {
				kind, dash = "node", ""
			}
			clusters.WriteString(`<g class="pele-cluster pele-c4-` + kind + `" data-id="` + svg.EscText(boundary.alias) + `">` +
				`<rect x="` + svg.Num(g.x) + `" y="` + svg.Num(g.y) + `" width="` + svg.Num(g.w) + `" height="` + svg.Num(g.h) + `" fill="none" stroke="var(--_b)"` +
				dash + svg.Rounded(style.Shape) + `/>` +
				link(txt, boundary.link, boundary.alias, &links, options) +
				"</g>")
		}
		for _, n := range g.nodes {
			shape := n.shape
			external := strings.HasPrefix(shape.kind, "external")
			style := userStyle(shape.bgColor, shape.borderColor, shape.fontColor)
			stroke := ` stroke="var(--_b)"`
			if external {
				stroke += ` stroke-dasharray="4 3"`
			}
			w, h, form, labels := n.w, n.h, n.form, n.labels
			dx := 0.0
			if form == "h-cyl" {
				dx = -queueRx(h) / 2
			}
			dy := 0.0
			if form == "person" {
				dy = head / 2
			} else if form == "cyl" {
				dy = cylRy(w) / 2
			}
			muted := ` fill="var(--_m)"` + style.Text
			y := dy - float64(n.textH/2)
			txt := svg.LabelSVG(labels[0], dx, y+float64(labels[0].Height/2), ` class="pele-c4-type" font-size="`+jsnum.Format(small-1)+`"`+muted, nil)
			y += labels[0].Height
			txt += svg.LabelSVG(labels[1], dx, y+float64(labels[1].Height/2), ` class="pele-label" font-weight="var(--_hw)"`+style.Text, nil)
			y += labels[1].Height
			txt += svg.LabelSVG(labels[2], dx, y+float64(labels[2].Height/2), ` class="pele-c4-techn" font-size="`+smallText+`"`+muted, nil)
			y += labels[2].Height + 4
			txt += svg.LabelSVG(labels[3], dx, y+float64(labels[3].Height/2), ` class="pele-c4-descr" font-size="`+smallText+`"`+style.Text, nil)
			fill, extClass := "--_s", ""
			if external {
				fill, extClass = "--_bg", " pele-c4-external"
			}
			nodesOut.WriteString(`<g class="pele-node pele-shape-` + form + ` pele-c4-` + shape.kind + extClass + `" data-id="` + svg.EscText(shape.alias) +
				`" transform="translate(` + svg.Num(n.x+float64(w/2)) + `,` + svg.Num(n.y+float64(h/2)) + `)">` +
				link(svg.DrawShape(form, w, h, ` fill="var(`+fill+`)"`+stroke+style.Shape, stroke+style.Line)+txt, shape.link, shape.alias, &links, options) +
				"</g>")
		}
	}

	titleRoom := 4000.0
	if tight {
		titleRoom = common.TitleRoom(options)
	}
	title := text.Layout(db.title, false, measurer, size, titleRoom, text.Bold, icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}
	inner := max(maxX-minX, title.Width)
	width := math.Ceil(inner + float64(2*pad))
	height := math.Ceil(maxY - minY + titleHeight + float64(2*pad))
	ox := pad - minX + float64((inner-(maxX-minX))/2)
	oy := pad + titleHeight - minY

	var content strings.Builder
	content.WriteString(svg.LabelSVG(title, width/2, pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil))
	content.WriteString(`<g transform="translate(` + svg.Num(ox) + `,` + svg.Num(oy) + `)">`)
	if clusters.Len() > 0 {
		content.WriteString(`<g class="pele-clusters">` + clusters.String() + `</g>`)
	}
	if edgesOut.Len() > 0 {
		content.WriteString(`<g class="pele-edges" fill="none" stroke="var(--_l)" stroke-linecap="round">` + edgesOut.String() + `</g>`)
	}
	if nodesOut.Len() > 0 {
		content.WriteString(`<g class="pele-nodes">` + nodesOut.String() + `</g>`)
	}
	if labelsOut.Len() > 0 {
		content.WriteString(`<g class="pele-edge-labels" font-size="` + smallText + `">` + labelsOut.String() + `</g>`)
	}
	content.WriteString("</g>")

	doc := svg.Document("c4", width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccDescr: db.accDescr}, content.String())
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: links}, ls.squeezed
}
