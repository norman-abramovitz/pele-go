package cynefin

import (
	"math"
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

type quadrant struct {
	name     string
	col, row float64
	color    int
}

var quadrants = [...]quadrant{
	{"complex", 0, 0, 0},
	{"complicated", 1, 0, 1},
	{"chaotic", 0, 1, 3},
	{"clear", 1, 1, 2},
}

const confusionColor = 4

// meta is each domain's name, decision model and kind of practice.
var meta = map[string][3]string{
	"complex":     {"Complex", "Probe → Sense → Respond", "Emergent Practices"},
	"complicated": {"Complicated", "Sense → Analyse → Respond", "Good Practices"},
	"clear":       {"Clear", "Sense → Categorise → Respond", "Best Practices"},
	"chaotic":     {"Chaotic", "Act → Sense → Respond", "Novel Practices"},
	"confusion":   {"Confusion", "", "Disorder"},
}

const (
	maxConfusionItems = 3
	itemMaxWidth      = 200
	itemPadX          = 10
	itemPadY          = 4
	itemGap           = 6
	headGap           = 10
	margin            = 20
	clearance         = 10
	steps             = 48
)

type badge struct {
	text     string
	label    text.Label
	w, h     float64
	overflow bool
}

type block struct {
	name      string
	title     text.Label
	subtitles []text.Label
	badges    []badge
	w, h      float64
	cx, cy    float64
}

func section(config *jsobj.Object, key string) any {
	v, _ := config.Get("cynefin")
	obj, ok := v.(*jsobj.Object)
	if !ok {
		return nil
	}
	value, _ := obj.Get(key)
	return value
}

func setting(config *jsobj.Object, key string, fallback, lo, hi float64) float64 {
	if n, ok := section(config, key).(float64); ok && n >= lo && n <= hi {
		return n
	}
	return fallback
}

func renderCynefin(m cynefinModel, config *jsobj.Object, rc *core.Context) core.Rendered {
	opts := rc.Options
	size := 16.0
	if opts.FontSize != nil {
		size = *opts.FontSize
	}
	small := jsmath.Round(size * 0.8125)
	itemSize := jsmath.Round(size * 0.875)
	measurer := rc.Measurer()
	pad := 8.0
	if opts.Padding != nil {
		pad = *opts.Padding
	}
	showDescriptions := section(config, "showDomainDescriptions") != false
	amplitude := setting(config, "boundaryAmplitude", 8, 0, 50)
	id := "pele"
	if opts.IDPrefix != nil {
		id = *opts.IDPrefix
	}
	seed := resolveSeed(setting(config, "seed", 0, -1e15, 1e15), id)
	itemLine := jsmath.Round(itemSize * 1.5)
	smallAttr := jsnum.Format(small)
	layout := func(raw string, fontSize, maxWidth float64, style text.Style) text.Label {
		return text.Layout(raw, false, measurer, fontSize, maxWidth, style, rc.Icons)
	}

	newBadge := func(t string, overflow bool) badge {
		label := layout(t, itemSize, itemMaxWidth, 0)
		return badge{text: t, label: label, w: label.Width + 2*itemPadX, h: max(label.Height, itemLine) + 2*itemPadY, overflow: overflow}
	}

	newBlock := func(name string) *block {
		meta := meta[name]
		b := &block{name: name, title: layout(meta[0], size, 4000, text.Bold)}
		if showDescriptions {
			for _, t := range meta[1:] {
				if t != "" {
					b.subtitles = append(b.subtitles, layout(t, small, 4000, text.Italic))
				}
			}
		}
		items := m.domains[name]
		shown := len(items)
		if name == "confusion" && len(items) > maxConfusionItems {
			shown = maxConfusionItems
		}
		for i := range shown {
			b.badges = append(b.badges, newBadge(items[i], false))
		}
		if shown < len(items) {
			b.badges = append(b.badges, newBadge("+"+strconv.Itoa(len(items)-shown)+" more", true))
		}
		b.w = b.title.Width
		b.h = b.title.Height
		for _, s := range b.subtitles {
			b.w = max(b.w, s.Width)
			b.h += s.Height
		}
		if len(b.badges) > 0 {
			b.h += headGap - itemGap
		}
		for _, item := range b.badges {
			b.w = max(b.w, item.w)
			b.h += itemGap + item.h
		}
		return b
	}

	var blocksInOrder [len(quadrants)]*block
	for i, q := range quadrants {
		blocksInOrder[i] = newBlock(q.name)
	}
	confusion := newBlock("confusion")
	rx := max(88, float64(confusion.w*0.71)+8)
	ry := max(64, float64(confusion.h*0.71)+6)

	quadW := setting(config, "width", 640, 100, 10000) / 2
	quadH := setting(config, "height", 440, 100, 10000) / 2
	for _, b := range blocksInOrder {
		quadW = max(quadW, b.w+2*margin)
		quadH = max(quadH, b.h+2*margin)
	}
	// A block sits in the middle of its quadrant, so its inner corner must stay clear of the
	// ellipse.
	for _, b := range blocksInOrder {
		dx := (quadW - b.w) / 2
		dy := (quadH - b.h) / 2
		ex := dx / (rx + clearance)
		ey := dy / (ry + clearance)
		if float64(ex*ex)+float64(ey*ey) >= 1 {
			continue
		}
		wider := b.w + float64(2*(rx+clearance))
		taller := b.h + float64(2*(ry+clearance))
		if float64(wider*quadH) <= float64(quadW*taller) {
			quadW = max(quadW, wider)
		} else {
			quadH = max(quadH, taller)
		}
	}

	title := layout(m.title, size, 4000, text.Bold)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}
	width := 2 * quadW
	height := 2 * quadH
	x0 := pad
	y0 := pad + titleHeight
	cx := x0 + quadW
	cy := y0 + quadH
	right := x0 + width
	bottom := y0 + height

	foldOpts := fold
	foldOpts.segments, foldOpts.pinEnd = 4, true
	westOpts := horizon
	westOpts.segments, westOpts.pinEnd = 4, true
	eastOpts := horizon
	eastOpts.segments, eastOpts.pinStart = 4, true
	foldCurve := wave(y0, cy, cx, seed, amplitude, foldOpts)
	west := wave(x0, cx, cy, seed+100, amplitude, westOpts)
	east := wave(cx, right, cy, seed+200, amplitude, eastOpts)
	drop := cliff(cx, cy, bottom, float64(width*0.03))
	at := func(curve []float64, end bool) string {
		if end {
			return svg.Num(curve[len(curve)-2]) + "," + svg.Num(curve[len(curve)-1])
		}
		return svg.Num(curve[0]) + "," + svg.Num(curve[1])
	}

	regions := map[string]string{
		"complex":     "M" + svg.Num(x0) + "," + svg.Num(y0) + "L" + at(foldCurve, false) + forward(foldCurve) + backward(west) + "Z",
		"complicated": "M" + at(foldCurve, false) + "L" + svg.Num(right) + "," + svg.Num(y0) + "L" + at(east, true) + backward(east) + backward(foldCurve) + "Z",
		"chaotic":     "M" + at(west, false) + forward(west) + forward(drop) + "L" + svg.Num(x0) + "," + svg.Num(bottom) + "Z",
		"clear":       "M" + at(east, false) + forward(east) + "L" + svg.Num(right) + "," + svg.Num(bottom) + "L" + at(drop, true) + backward(drop) + "Z",
	}

	var fills strings.Builder
	for _, q := range quadrants {
		fills.WriteString(`<path class="pele-cynefin-region" data-id="` + q.name + `" d="` + regions[q.name] + `" fill="` + svg.SeriesColor(q.color) + `" fill-opacity="0.14"/>`)
	}
	boundaries := `<g class="pele-cynefin-boundaries" fill="none" stroke="var(--_b)" stroke-dasharray="6 3">` +
		`<path d="` + curvePath(foldCurve) + `"/><path d="` + curvePath(west) + `"/><path d="` + curvePath(east) + `"/>` +
		`<path class="pele-cynefin-cliff" d="` + curvePath(drop) + `" stroke="var(--_l)" stroke-width="2" stroke-dasharray="none" stroke-linecap="round"/></g>`
	oval := ellipsePath(cx, cy, rx, ry)
	ellipse := `<path d="` + oval + `" fill="var(--_bg)"/>` +
		`<path class="pele-cynefin-region" data-id="confusion" d="` + oval + `" fill="` + svg.SeriesColor(confusionColor) + `" fill-opacity="0.18" stroke="var(--_b)" stroke-dasharray="4 2"/>`

	itemAttr := jsnum.Format(itemSize)
	drawBlock := func(b *block) string {
		y := b.cy - float64(b.h/2)
		var out strings.Builder
		out.WriteString(svg.LabelSVG(b.title, b.cx, y+float64(b.title.Height/2), ` class="pele-label" font-weight="var(--_hw)"`, nil))
		y += b.title.Height
		for _, s := range b.subtitles {
			out.WriteString(svg.LabelSVG(s, b.cx, y+float64(s.Height/2), ` class="pele-cynefin-subtitle" font-size="`+smallAttr+`" font-style="italic" fill="var(--_m)"`, nil))
			y += s.Height
		}
		y += headGap - itemGap
		for _, item := range b.badges {
			y += itemGap
			class, id, dash, fill := "", ` data-id="`+svg.EscText(item.text)+`"`, "", ""
			if item.overflow {
				class, id, dash, fill = " pele-cynefin-overflow", "", ` stroke-dasharray="3 2"`, ` fill="var(--_m)"`
			}
			out.WriteString(`<g class="pele-node` + class + `"` + id + `>` +
				`<rect x="` + svg.Num(b.cx-float64(item.w/2)) + `" y="` + svg.Num(y) + `" width="` + svg.Num(item.w) + `" height="` + svg.Num(item.h) +
				`" fill="var(--_bg)" stroke="var(--_b)"` + dash + svg.RoundedStyle + `/>` +
				svg.LabelSVG(item.label, b.cx, y+float64(item.h/2), ` class="pele-label" font-size="`+itemAttr+`"`+fill, rc.Icons) +
				`</g>`)
			y += item.h
		}
		return `<g class="pele-cynefin-domain" data-id="` + b.name + `">` + out.String() + `</g>`
	}

	blocks := map[string]*block{}
	var domains strings.Builder
	for index, q := range quadrants {
		b := blocksInOrder[index]
		b.cx = x0 + float64(quadW*(q.col+0.5))
		b.cy = y0 + float64(quadH*(q.row+0.5))
		blocks[q.name] = b
		domains.WriteString(drawBlock(b))
	}
	confusion.cx = cx
	confusion.cy = cy
	blocks["confusion"] = confusion
	domains.WriteString(drawBlock(confusion))

	inside := func(b *block, x, y float64) bool {
		if b == confusion {
			ex := (x - cx) / (rx + 4)
			ey := (y - cy) / (ry + 4)
			return float64(ex*ex)+float64(ey*ey) <= 1
		}
		return math.Abs(x-b.cx) <= float64(b.w/2)+8 && math.Abs(y-b.cy) <= float64(b.h/2)+8
	}

	var edges strings.Builder
	seen := map[string]int{}
	for _, transition := range m.transitions {
		from, okFrom := blocks[transition.from]
		to, okTo := blocks[transition.to]
		if !okFrom || !okTo || from == to {
			continue
		}
		key := transition.to + " " + transition.from
		if transition.from < transition.to {
			key = transition.from + " " + transition.to
		}
		repeat := seen[key]
		seen[key] = repeat + 1

		dx := to.cx - from.cx
		dy := to.cy - from.cy
		length := jsmath.Hypot(dx, dy)
		if length == 0 || math.IsNaN(length) {
			length = 1
		}
		mx := float64((from.cx + to.cx) / 2)
		my := float64((from.cy + to.cy) / 2)
		// Curves bow away from the middle of the diagram, which keeps them off the ellipse.
		outward := -1.0
		if float64(-dy*(mx-cx))+float64(dx*(my-cy)) >= 0 {
			outward = 1
		}
		nx := -dy / length * outward
		ny := dx / length * outward
		// A diagonal runs through the middle, so it bows far enough to pass the ellipse.
		diagonal := from != confusion && to != confusion && math.Abs(dx) > 1 && math.Abs(dy) > 1
		var bow float64
		if diagonal {
			bow = 2 * (jsmath.Hypot(float64(rx*nx), float64(ry*ny)) + 16)
		} else {
			bow = float64(length * 0.15)
		}
		bow += float64(float64(repeat) * 56)
		px := mx + float64(nx*bow)
		py := my + float64(ny*bow)
		point := func(t float64) (float64, float64) {
			u := 1 - t
			return float64(u*u*from.cx) + float64(2*u*t*px) + float64(t*t*to.cx),
				float64(u*u*from.cy) + float64(2*u*t*py) + float64(t*t*to.cy)
		}
		insideAt := func(b *block, t float64) bool {
			x, y := point(t)
			return inside(b, x, y)
		}
		crossing := func(b *block, lo, hi float64, leaving bool) float64 {
			for range 16 {
				mid := float64((lo + hi) / 2)
				if insideAt(b, mid) == leaving {
					lo = mid
				} else {
					hi = mid
				}
			}
			return float64((lo + hi) / 2)
		}
		i := 1
		for i <= steps && insideAt(from, float64(i)/steps) {
			i++
		}
		if i > steps {
			continue
		}
		t0 := crossing(from, float64(i-1)/steps, float64(i)/steps, true)
		for i <= steps && !insideAt(to, float64(i)/steps) {
			i++
		}
		t1 := 1.0
		if i <= steps {
			t1 = crossing(to, float64(i-1)/steps, float64(i)/steps, false)
		}

		sx, sy := point(t0)
		ex, ey := point(t1)
		blossom := func(a, b, c float64) float64 {
			return float64((1-t0)*(1-t1)*a) + float64((float64((1-t0)*t1)+float64(t0*(1-t1)))*b) + float64(t0*t1*c)
		}
		qx := blossom(from.cx, px, to.cx)
		qy := blossom(from.cy, py, to.cy)
		el := jsmath.Hypot(ex-qx, ey-qy)
		if el == 0 || math.IsNaN(el) {
			el = 1
		}
		ux := (ex - qx) / el
		uy := (ey - qy) / el
		head := svg.Marker("arrow_point", ex, ey, ux, uy, "var(--_l)")
		trim := svg.MarkerTrim("arrow_point")
		ex -= float64(ux * trim)
		ey -= float64(uy * trim)

		label := ""
		if transition.label != "" {
			l := layout(transition.label, small, itemMaxWidth, 0)
			lx, ly := point(float64((t0 + t1) / 2))
			away := float64(float64(math.Abs(nx)*l.Width)/2) + float64(float64(math.Abs(ny)*l.Height)/2) + 6
			half := float64(l.Width/2) + 4
			tx := min(max(lx+float64(nx*away), x0+half), max(right-half, x0+half))
			label = svg.LabelSVG(l, tx, ly+float64(ny*away), ` class="pele-edge-label" font-size="`+smallAttr+`"`, rc.Icons)
		}
		edges.WriteString(`<g class="pele-edge" data-id="` + transition.from + `-` + transition.to + `">` +
			`<path d="M` + svg.Num(sx) + `,` + svg.Num(sy) + `Q` + svg.Num(qx) + `,` + svg.Num(qy) + ` ` + svg.Num(ex) + `,` + svg.Num(ey) + `" fill="none" stroke="var(--_l)"/>` +
			head + label + `</g>`)
	}

	totalWidth := math.Ceil(max(width, title.Width) + float64(2*pad))
	totalHeight := math.Ceil(height + titleHeight + float64(2*pad))
	var content strings.Builder
	content.WriteString(svg.LabelSVG(title, x0+float64(width/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil))
	content.WriteString(`<g class="pele-cynefin-regions">` + fills.String() + `</g>`)
	content.WriteString(boundaries)
	content.WriteString(ellipse)
	content.WriteString(domains.String())
	if edges.Len() > 0 {
		content.WriteString(`<g class="pele-edges">` + edges.String() + `</g>`)
	}

	doc := svg.Document("cynefin", totalWidth, totalHeight, size, svg.DocumentOptions{
		IDPrefix:       opts.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     opts.FontFamily,
		FontFamilyMono: opts.FontFamilyMono,
	}, svg.Accessible{AccTitle: m.accTitle, AccDescr: m.accDescr}, content.String())
	return core.Rendered{SVG: doc, Width: totalWidth, Height: totalHeight, Links: []core.Link{}}
}
