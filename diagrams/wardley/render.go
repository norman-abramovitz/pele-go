package wardley

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
	"github.com/Broderick-Westrope/pele-go/internal/omap"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

var defaultStages = []string{"Genesis", "Custom Built", "Product", "Commodity"}

const (
	// canvasMargin is the room Mermaid's canvas has around the plot on every side.
	canvasMargin = 48
	radius       = 6
	square       = 10
	labelOffset  = 8
	pipelinePad  = 15
	forceWidth   = 60
	forceHeight  = 30
	forceHead    = 20
	accent       = "var(--_c)"
	line         = "var(--_l)"
	// tri and dot are 1.2 * R and 0.7 * R as JS rounds them.
	tri = 7.199999999999999
	dot = 4.199999999999999
)

// Variables, not constants, so that the divisions round as JS's do.
var (
	sqrt2 = math.Sqrt2
	pi    = math.Pi
)

type placed struct {
	x, y float64
	node *wardleyNode
}

func section(config *jsobj.Object, key string) (any, bool) {
	v, _ := config.Get("wardley-beta")
	obj, ok := v.(*jsobj.Object)
	if !ok {
		return nil, false
	}
	return obj.Get(key)
}

func setting(config *jsobj.Object, key string, fallback, lo, hi float64) float64 {
	v, _ := section(config, key)
	if n, ok := v.(float64); ok && n >= lo && n <= hi {
		return n
	}
	return fallback
}

func clamp(value, lo, hi float64) float64 {
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}
	return value
}

// startLabel writes a label with its lines starting at x and its first baseline at y.
func startLabel(label text.Label, x, y float64, attrs string) string {
	shared := svg.SharedStyle(label)
	var body strings.Builder
	baseline := y
	for _, ln := range label.Lines {
		first := true
		for _, span := range ln {
			if span.Icon != "" {
				continue
			}
			pos := ""
			if first {
				pos = ` x="` + svg.Num(x) + `" y="` + svg.Num(baseline) + `"`
			}
			body.WriteString(`<tspan` + pos + svg.SpanStyle(span.Style&^shared) + `>` + svg.Esc(span.Text) + `</tspan>`)
			first = false
		}
		baseline += label.LineHeight
	}
	if body.Len() == 0 {
		return ""
	}
	return `<text` + svg.TextStyle(attrs, shared) + ` xml:space="preserve">` + body.String() + `</text>`
}

func dimension(config *jsobj.Object, given *float64, key string, fallback float64) float64 {
	var value float64
	if given != nil && !math.IsInf(*given, 0) && !math.IsNaN(*given) {
		value = *given
	} else {
		value = setting(config, key, fallback, 1, 1e6)
	}
	return clamp(value, 200, 10000) - 2*canvasMargin
}

func renderWardley(m wardleyModel, config *jsobj.Object, rc *core.Context) core.Rendered {
	var width, height *float64
	if m.size != nil {
		width, height = &m.size.width, &m.size.height
	}
	natural := dimension(config, width, "width", 900)
	return common.FitWidth(rc.Options, natural, 160, func(plotW float64, tight bool) core.Rendered {
		return draw(m, config, rc, plotW, natural, tight, height)
	})
}

func draw(m wardleyModel, config *jsobj.Object, rc *core.Context, plotW, naturalW float64, tight bool, givenHeight *float64) core.Rendered {
	opts := rc.Options
	size := 16.0
	if opts.FontSize != nil {
		size = *opts.FontSize
	}
	small := jsmath.Round(size * 0.8125)
	tiny := jsmath.Round(size * 0.6875)
	measurer := rc.Measurer()
	pad := 8.0
	if opts.Padding != nil {
		pad = *opts.Padding
	}

	// A narrower map is not as much shorter, so that components keep some room between them.
	plotH := float64(dimension(config, givenHeight, "height", 600) * max(0.75, plotW/naturalW))
	showGrid, _ := section(config, "showGrid")

	minX, minY, maxX, maxY := 0.0, 0.0, plotW, plotH
	grow := func(x0, y0, x1, y1 float64) {
		if x0 < minX {
			minX = x0
		}
		if y0 < minY {
			minY = y0
		}
		if x1 > maxX {
			maxX = x1
		}
		if y1 > maxY {
			maxY = y1
		}
	}
	centered := func(label text.Label, cx, cy float64, attrs string) string {
		halfW, halfH := float64(label.Width/2), float64(label.Height/2)
		grow(cx-halfW, cy-halfH, cx+halfW, cy+halfH)
		return svg.LabelSVG(label, cx, cy, attrs, rc.Icons)
	}
	starting := func(label text.Label, x, y float64, attrs string) string {
		grow(x, y-label.Size, x+label.Width, y-label.Size+label.Height+4)
		return startLabel(label, x, y, attrs)
	}
	defaultWidth := 4000.0
	if tight {
		defaultWidth = max(64, min(120, plotW/2))
	}
	layout := func(raw string, fontSize, maxWidth float64, style text.Style) text.Label {
		return text.Layout(raw, false, measurer, fontSize, maxWidth, style, rc.Icons)
	}
	projectX := func(value float64) float64 { return float64(value / 100 * plotW) }
	projectY := func(value float64) float64 { return plotH - float64(value/100*plotH) }
	smallAttr := jsnum.Format(small)
	tinyAttr := jsnum.Format(tiny)
	tiny1Attr := jsnum.Format(tiny + 1)

	stages := m.axes.stages
	if len(stages) == 0 {
		stages = defaultStages
	}
	boundaries := m.axes.stageBoundaries
	custom := len(boundaries) == len(stages)
	var frame, stageLabels strings.Builder
	stageBottom := plotH
	previous := 0.0
	count := float64(len(stages))
	for index, stage := range stages {
		start := float64(index) / count
		end := float64(index+1) / count
		if custom {
			start = previous
			end = clamp(boundaries[index], 0, 1)
		}
		previous = end
		if index > 0 {
			frame.WriteString(`<path class="pele-wardley-stage" d="M` + svg.Num(start*plotW) + `,0V` + svg.Num(plotH) + `" stroke="var(--_b)" stroke-dasharray="4 4"/>`)
		}
		label := layout(stage, small, max(48, float64(math.Abs(end-start)*plotW)-12), 0)
		stageLabels.WriteString(centered(label, float64((start+end)/2*plotW), plotH+8+float64(label.Height/2), ` class="pele-wardley-stage-label" font-size="`+smallAttr+`" fill="var(--_m)"`))
		stageBottom = max(stageBottom, plotH+8+label.Height)
	}
	if showGrid == true {
		for i := 1.0; i < 4; i++ {
			frame.WriteString(`<path class="pele-wardley-grid" d="M` + svg.Num(plotW*i/4) + `,0V` + svg.Num(plotH) + `M0,` + svg.Num(plotH*i/4) + `H` + svg.Num(plotW) + `" stroke="var(--_b)" stroke-dasharray="2 6"/>`)
		}
	}
	frame.WriteString(`<path class="pele-wardley-axis" d="M0,0V` + svg.Num(plotH) + `H` + svg.Num(plotW) + `" stroke="` + line + `"/>`)
	xTitle := layout("Evolution", small, defaultWidth, 0)
	yTitle := layout("Visibility", small, defaultWidth, 0)
	axisAttrs := ` class="pele-wardley-axis-label" font-size="` + smallAttr + `" font-weight="var(--_hw)"`
	axisLabels := centered(xTitle, float64(plotW/2), stageBottom+6+float64(xTitle.Height/2), axisAttrs) +
		svg.LabelSVG(yTitle, -16, plotH/2, axisAttrs+` transform="rotate(-90 -16 `+svg.Num(plotH/2)+`)"`, nil)
	grow(-16-float64(yTitle.Height/2), 0, 0, 0)
	legendTop := stageBottom + 6 + xTitle.Height + 10

	var positions omap.Map[string, *placed]
	for _, node := range m.nodes {
		positions.Set(node.id, &placed{x: projectX(node.x), y: projectY(node.y), node: node})
	}

	// Pipelines: a box around the components, with the parent as a square on its top edge.
	var pipelines strings.Builder
	members := map[string]map[string]bool{}
	for _, pipeline := range m.pipelines {
		set := map[string]bool{}
		for _, id := range pipeline.componentIDs {
			set[id] = true
		}
		members[pipeline.nodeID] = set
		var parts []*placed
		for _, id := range pipeline.componentIDs {
			if part, ok := positions.Get(id); ok {
				parts = append(parts, part)
			}
		}
		if len(parts) == 0 {
			continue
		}
		slices.SortStableFunc(parts, func(a, b *placed) int { return cmp.Compare(a.x, b.x) })
		left := parts[0].x
		right := parts[len(parts)-1].x
		y := parts[0].y
		top := y - 2*radius
		if parent, ok := positions.Get(pipeline.nodeID); ok {
			parent.x = (left + right) / 2
			parent.y = top - float64(square)/6
		}
		grow(left-pipelinePad, top, right+pipelinePad, top+4*radius)
		pipelines.WriteString(`<g class="pele-wardley-pipeline" data-id="` + svg.EscText(pipeline.nodeID) + `">` +
			`<rect x="` + svg.Num(left-pipelinePad) + `" y="` + svg.Num(top) + `" width="` + svg.Num(right-left+2*pipelinePad) + `" height="` + svg.Num(4*radius) +
			`" fill="var(--_s)" stroke="var(--_b)"` + svg.RoundedStyle + `/>`)
		if len(parts) > 1 {
			pipelines.WriteString(`<path d="M` + svg.Num(left) + `,` + svg.Num(y) + `H` + svg.Num(right) + `" stroke="` + line + `" stroke-dasharray="4 4"/>`)
		}
		pipelines.WriteString(`</g>`)
	}

	anchorWidth := 4000.0
	if tight {
		anchorWidth = max(64, plotW/2)
	}
	// reach is how far from a node's center a line to it stops.
	anchorLabels := map[*wardleyNode]text.Label{}
	reach := func(at *placed, ux, uy float64) float64 {
		node := at.node
		if node.className == "anchor" {
			label, ok := anchorLabels[node]
			if !ok {
				label = layout(node.label, small, anchorWidth, text.Bold)
				anchorLabels[node] = label
			}
			return 1 / jsmath.Hypot(ux/(float64(label.Width/2)+5), uy/float64(small*0.8))
		}
		if node.isPipelineParent {
			return square / sqrt2
		}
		if node.sourceStrategy != "" {
			return 2 * radius
		}
		return radius
	}

	// Quadrants around a node that a link runs through: 1 up and right, 2 down and right, 4
	// up and left, 8 down and left.
	taken := map[*placed]int{}
	occupy := func(at *placed, ux, uy float64) {
		if math.Abs(ux) < 0.25 || math.Abs(uy) < 0.25 {
			return
		}
		quadrant := 8
		switch {
		case ux > 0 && uy < 0:
			quadrant = 1
		case ux > 0:
			quadrant = 2
		case uy < 0:
			quadrant = 4
		}
		taken[at] |= quadrant
	}

	var links strings.Builder
	for _, link := range m.links {
		from, okFrom := positions.Get(link.source)
		to, okTo := positions.Get(link.target)
		if !okFrom || !okTo || from == to {
			continue
		}
		// A pipeline component belongs to its parent already.
		if members[link.target][link.source] {
			continue
		}
		dx := to.x - from.x
		dy := to.y - from.y
		distance := jsmath.Hypot(dx, dy)
		if distance < 0.01 {
			continue
		}
		ux := dx / distance
		uy := dy / distance
		start := reach(from, ux, uy)
		end := reach(to, ux, uy)
		if start+end >= distance {
			continue
		}
		occupy(from, ux, uy)
		occupy(to, -ux, -uy)
		x1 := from.x + float64(ux*start)
		y1 := from.y + float64(uy*start)
		x2 := to.x - float64(ux*end)
		y2 := to.y - float64(uy*end)
		heads := ""
		trim := svg.MarkerTrim("arrow_point")
		if link.flow == flowForward || link.flow == flowBidirectional {
			heads += svg.Marker("arrow_point", x2, y2, ux, uy, line)
			x2 -= float64(ux * trim)
			y2 -= float64(uy * trim)
		}
		if link.flow == flowBackward || link.flow == flowBidirectional {
			heads += svg.Marker("arrow_point", x1, y1, -ux, -uy, line)
			x1 += float64(ux * trim)
			y1 += float64(uy * trim)
		}
		caption := ""
		if link.label != "" {
			label := layout(link.label, tiny+1, 200, 0)
			// Beside the middle of the line, on its upper side.
			side := -1.0
			if ux >= 0 {
				side = 1
			}
			px := uy * side
			py := -ux * side
			away := 5 + float64(float64(math.Abs(px)*label.Width)/2) + float64(float64(math.Abs(py)*label.Height)/2)
			caption = centered(label, float64((from.x+to.x)/2)+float64(px*away), float64((from.y+to.y)/2)+float64(py*away), ` class="pele-edge-label" font-size="`+tiny1Attr+`" fill="var(--_m)"`)
		}
		dashedClass, dashedAttr := "", ""
		if link.dashed {
			dashedClass, dashedAttr = " pele-wardley-dashed", ` stroke-dasharray="6 6"`
		}
		links.WriteString(`<g class="pele-edge` + dashedClass + `" data-id="` + svg.EscText(link.source) + `-` + svg.EscText(link.target) + `">` +
			`<path d="M` + svg.Num(x1) + `,` + svg.Num(y1) + `L` + svg.Num(x2) + `,` + svg.Num(y2) + `" stroke="` + line + `"` + dashedAttr + `/>` +
			heads + caption + `</g>`)
	}

	// Evolution: a dashed arrow to where the component is heading, drawn as an open circle.
	var trends strings.Builder
	for _, trend := range m.trends {
		origin, ok := positions.Get(trend.nodeID)
		if !ok {
			continue
		}
		tx := projectX(trend.targetX)
		ty := projectY(trend.targetY)
		dx := tx - origin.x
		dy := ty - origin.y
		distance := jsmath.Hypot(dx, dy)
		arrow := ""
		start := 0.0
		if distance > 0 {
			start = reach(origin, dx/distance, dy/distance)
		}
		if distance > start+radius+4 {
			ux := dx / distance
			uy := dy / distance
			ex := tx - float64(ux*(radius+2))
			ey := ty - float64(uy*(radius+2))
			trim := svg.MarkerTrim("arrow_point")
			arrow = `<path d="M` + svg.Num(origin.x+float64(ux*start)) + `,` + svg.Num(origin.y+float64(uy*start)) + `L` + svg.Num(ex-float64(ux*trim)) + `,` +
				svg.Num(ey-float64(uy*trim)) + `" stroke="` + accent + `" stroke-dasharray="4 4"/>` + svg.Marker("arrow_point", ex, ey, ux, uy, accent)
		}
		grow(tx-radius, ty-radius, tx+radius, ty+radius)
		trends.WriteString(`<g class="pele-wardley-trend" data-id="` + svg.EscText(trend.nodeID) + `">` + arrow +
			`<circle cx="` + svg.Num(tx) + `" cy="` + svg.Num(ty) + `" r="6" fill="var(--_bg)" stroke="` + accent + `"/></g>`)
	}

	var nodes strings.Builder
	for _, at := range positions.All() {
		node := at.node
		x, y := at.x, at.y
		anchor := node.className == "anchor"
		strategy := node.sourceStrategy
		var shape strings.Builder
		if strategy != "" {
			fill := "var(--_bg)"
			switch strategy {
			case "outsource":
				fill = "var(--_m)"
			case "buy":
				fill = "var(--_a)"
			case "build":
				fill = "var(--_s)"
			}
			shape.WriteString(`<circle class="pele-wardley-` + strategy + `" cx="` + svg.Num(x) + `" cy="` + svg.Num(y) + `" r="12" fill="` + fill + `" stroke="` + line + `"/>`)
		}
		switch {
		case strategy == "market":
			bx := float64(tri * jsmath.Cos(pi/6))
			by := y + float64(tri*jsmath.Sin(pi/6))
			shape.WriteString(`<path d="M` + svg.Num(x) + `,` + svg.Num(y-tri) + `L` + svg.Num(x-bx) + `,` + svg.Num(by) + `H` + svg.Num(x+bx) + `Z" fill="none" stroke="` + line + `"/>`)
			for _, c := range [...][2]float64{{x, y - tri}, {x - bx, by}, {x + bx, by}} {
				shape.WriteString(`<circle cx="` + svg.Num(c[0]) + `" cy="` + svg.Num(c[1]) + `" r="` + svg.Num(dot) + `" fill="var(--_bg)" stroke="` + line + `"/>`)
			}
		case node.isPipelineParent:
			shape.WriteString(`<rect x="` + svg.Num(x-square/2) + `" y="` + svg.Num(y-square/2) + `" width="10" height="10" fill="var(--_bg)" stroke="` + line + `"/>`)
		case !anchor:
			shape.WriteString(`<circle cx="` + svg.Num(x) + `" cy="` + svg.Num(y) + `" r="6" fill="var(--_bg)" stroke="` + line + `"/>`)
		}
		if node.inertia {
			offset := float64(radius)
			if node.isPipelineParent {
				offset = square / 2
			}
			offset += 10
			if strategy != "" {
				offset += radius + 4
			}
			shape.WriteString(`<rect class="pele-wardley-inertia" x="` + svg.Num(x+offset) + `" y="` + svg.Num(y-radius) + `" width="4" height="12" fill="` + line + `"/>`)
		}
		var ox, oy *float64
		if node.labelOffsetX != nil {
			ox = new(clamp(*node.labelOffsetX, -5000, 5000))
		}
		if node.labelOffsetY != nil {
			oy = new(clamp(*node.labelOffsetY, -5000, 5000))
		}
		or := func(v *float64, fallback float64) float64 {
			if v != nil {
				return *v
			}
			return fallback
		}
		var caption string
		if anchor {
			label, ok := anchorLabels[node]
			if !ok {
				label = layout(node.label, small, anchorWidth, text.Bold)
			}
			caption = centered(label, x+or(ox, 0), y+or(oy, -3), ` class="pele-label" font-size="`+smallAttr+`" font-weight="var(--_hw)"`)
		} else {
			label := layout(node.label, small, defaultWidth, 0)
			attrs := ` class="pele-label" font-size="` + smallAttr + `"`
			lift := float64(labelOffset)
			if strategy != "" {
				lift += 10
			}
			switch {
			case ox != nil || oy != nil:
				caption = starting(label, x+or(ox, lift), y+or(oy, -lift), attrs)
			case node.inPipeline:
				caption = centered(label, x, y+2*radius+4+float64(label.Height/2), attrs)
			default:
				// The label goes up and to the right unless a link leaves that way; then the
				// next free corner.
				busy := taken[at]
				corner := 1
				switch {
				case busy&1 == 0:
					corner = 1
				case busy&2 == 0:
					corner = 2
				case busy&4 == 0:
					corner = 4
				case busy&8 == 0:
					corner = 8
				}
				lx := x - lift - label.Width
				if corner == 1 || corner == 2 {
					lx = x + lift
				}
				ly := y + lift + float64(small*0.75)
				if corner == 1 || corner == 4 {
					ly = y - lift
				}
				caption = starting(label, lx, ly, attrs)
			}
		}
		grow(x-2*radius, y-2*radius, x+2*radius+20, y+2*radius)
		class := "component"
		switch {
		case anchor:
			class = "anchor"
		case node.isPipelineParent:
			class = "pipeline-parent"
		case node.inPipeline:
			class = "pipeline-component"
		}
		nodes.WriteString(`<g class="pele-node pele-wardley-` + class + `" data-id="` + svg.EscText(node.id) + `">` + shape.String() + caption + `</g>`)
	}

	var annotations strings.Builder
	for _, annotation := range m.annotations {
		number := jsnum.Format(annotation.number)
		label := layout(number, tiny, 4000, text.Bold)
		var marks strings.Builder
		for _, coordinate := range annotation.coordinates {
			x := projectX(coordinate[0])
			y := projectY(coordinate[1])
			r := max(10, float64(label.Width/2)+4)
			grow(x-r, y-r, x+r, y+r)
			marks.WriteString(`<circle cx="` + svg.Num(x) + `" cy="` + svg.Num(y) + `" r="` + svg.Num(r) + `" fill="var(--_bg)" stroke="` + line + `"/>` +
				svg.LabelSVG(label, x, y, ` font-size="`+tinyAttr+`" font-weight="var(--_hw)"`, nil))
		}
		annotations.WriteString(`<g class="pele-wardley-annotation" data-id="` + svg.EscText(number) + `">` + marks.String() + `</g>`)
	}
	var listed []wardleyAnnotation
	for _, a := range m.annotations {
		if a.text != "" {
			listed = append(listed, a)
		}
	}
	slices.SortStableFunc(listed, func(a, b wardleyAnnotation) int { return cmp.Compare(a.number, b.number) })
	if len(listed) > 0 {
		listWidth := 320.0
		if tight {
			listWidth = max(80, plotW-20)
		}
		lines := make([]text.Label, len(listed))
		for i, a := range listed {
			lines[i] = layout(jsnum.Format(a.number)+". "+a.text, small, listWidth, 0)
		}
		width, height := 0.0, 0.0
		for _, l := range lines {
			width = max(width, l.Width)
			height += l.Height
		}
		width += 20
		height += 14
		// Without a position the list goes under the map, where it covers nothing.
		bx, by := 0.0, legendTop
		if box := m.annotationsBox; box != nil {
			bx = clamp(projectX(box[0]), 0, max(0, plotW-width))
			by = clamp(projectY(box[1]), 0, max(0, plotH-height))
		}
		grow(bx, by, bx+width, by+height)
		var rows strings.Builder
		y := by + 7
		for _, l := range lines {
			rows.WriteString(startLabel(l, bx+10, y+float64(l.LineHeight/2)+float64(small*0.35), ` font-size="`+smallAttr+`"`))
			y += l.Height
		}
		annotations.WriteString(`<g class="pele-wardley-annotations"><rect x="` + svg.Num(bx) + `" y="` + svg.Num(by) + `" width="` + svg.Num(width) + `" height="` + svg.Num(height) +
			`" fill="var(--_bg)" stroke="var(--_b)"` + svg.RoundedStyle + `/>` + rows.String() + `</g>`)
	}

	var notes strings.Builder
	for _, note := range m.notes {
		notes.WriteString(starting(layout(note.text, small, min(360, plotW*0.6), text.Bold), projectX(note.x), projectY(note.y),
			` class="pele-wardley-note" font-size="`+smallAttr+`" font-weight="var(--_hw)"`))
	}

	// Accelerators point right, deaccelerators left.
	var forces strings.Builder
	force := func(name string, px, py float64, forward bool) {
		x := projectX(px)
		y := projectY(py)
		const half = forceHeight / 2
		tip, tail, neck, class := x, x+forceWidth, x+forceHead, "deaccelerator"
		if forward {
			tip, tail, neck, class = x+forceWidth, x, x+forceWidth-forceHead, "accelerator"
		}
		d := `M` + svg.Num(tail) + `,` + svg.Num(y-half) + `H` + svg.Num(neck) + `V` + svg.Num(y-half-8) + `L` + svg.Num(tip) + `,` + svg.Num(y) +
			`L` + svg.Num(neck) + `,` + svg.Num(y+half+8) + `V` + svg.Num(y+half) + `H` + svg.Num(tail) + `Z`
		grow(x, y-half-8, x+forceWidth, y+half+8)
		forces.WriteString(`<g class="pele-wardley-` + class + `" data-id="` + svg.EscText(name) + `">` +
			`<path d="` + d + `" fill="var(--_s)" stroke="` + line + `"/>` +
			centered(layout(name, tiny+1, 160, text.Bold), x+forceWidth/2, y+half+8+12, ` font-size="`+tiny1Attr+`" font-weight="var(--_hw)"`) +
			`</g>`)
	}
	for _, a := range m.accelerators {
		force(a.name, a.x, a.y, true)
	}
	for _, d := range m.deaccelerators {
		force(d.name, d.x, d.y, false)
	}

	titleWidth := 4000.0
	if tight {
		titleWidth = max(80, plotW)
	}
	title := text.Layout(m.title, false, measurer, size, titleWidth, text.Bold, rc.Icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}
	left := min(minX, float64(plotW/2)-float64(title.Width/2))
	right := max(maxX, float64(plotW/2)+float64(title.Width/2))
	tx := pad - left
	ty := pad + titleHeight - minY
	totalWidth := math.Ceil(right - left + float64(2*pad))
	totalHeight := math.Ceil(maxY - minY + titleHeight + float64(2*pad))
	var content strings.Builder
	content.WriteString(svg.LabelSVG(title, tx+float64(plotW/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil))
	content.WriteString(`<g transform="translate(` + svg.Num(tx) + `,` + svg.Num(ty) + `)">`)
	content.WriteString(`<g class="pele-wardley-frame" fill="none">` + frame.String() + `</g>`)
	content.WriteString(stageLabels.String())
	content.WriteString(axisLabels)
	content.WriteString(pipelines.String())
	if links.Len() > 0 {
		content.WriteString(`<g class="pele-edges" fill="none">` + links.String() + `</g>`)
	}
	if trends.Len() > 0 {
		content.WriteString(`<g class="pele-wardley-trends" fill="none">` + trends.String() + `</g>`)
	}
	content.WriteString(nodes.String())
	content.WriteString(annotations.String())
	content.WriteString(notes.String())
	content.WriteString(forces.String())
	content.WriteString(`</g>`)

	doc := svg.Document("wardley", totalWidth, totalHeight, size, svg.DocumentOptions{
		IDPrefix:       opts.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     opts.FontFamily,
		FontFamilyMono: opts.FontFamilyMono,
	}, svg.Accessible{AccTitle: m.accTitle, AccDescr: m.accDescr}, content.String())
	return core.Rendered{SVG: doc, Width: totalWidth, Height: totalHeight, Links: []core.Link{}}
}
