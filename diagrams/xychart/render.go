package xychart

import (
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const (
	tick      = 4
	swatch    = 12
	row       = 26
	legendGap = 24
	limit     = 1e300
)

type mark struct {
	at    float64
	label text.Label
}

// record returns value when it is a plain object; nil stands for upstream's {}.
func record(value any) *jsobj.Object {
	o, _ := value.(*jsobj.Object)
	return o
}

func get(config *jsobj.Object, key string) any {
	v, _ := config.Get(key)
	return v
}

func on(config *jsobj.Object, key string, fallback bool) bool {
	if v, ok := get(config, key).(bool); ok {
		return v
	}
	return fallback
}

func dimension(value any, fallback float64) float64 {
	if v, ok := value.(float64); ok && v > 0 {
		return jsmath.Round(min(max(v, 100), 4000))
	}
	return fallback
}

// finite clamps a value to the drawable range; ok is false for upstream's undefined.
func finite(value float64, has bool) (float64, bool) {
	if !has || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, false
	}
	return min(max(value, -limit), limit), true
}

// pick is cond ? a : b.
func pick(cond bool, a, b float64) float64 {
	if cond {
		return a
	}
	return b
}

func widest(marks []mark) float64 {
	width := 0.0
	for _, m := range marks {
		width = max(width, m.label.Width)
	}
	return width
}

// aligned writes a label with its left edge, center or right edge at x. A plain one-line
// label keeps its place when the text is wider or narrower than measured.
func aligned(label text.Label, x, y float64, anchor string, icons *text.Icons) string {
	if len(label.Lines) == 1 {
		line := label.Lines[0]
		if len(line) == 1 && line[0].Icon == "" && line[0].Style == 0 {
			return `<text x="` + svg.Num(x) + `" y="` + svg.Num(y+float64(label.Size*0.35)) + `" text-anchor="` + anchor + `">` + svg.Esc(line[0].Text) + `</text>`
		}
	}
	shift := 0.0
	switch anchor {
	case "start":
		shift = float64(label.Width / 2)
	case "end":
		shift = float64(-label.Width / 2)
	}
	return svg.LabelSVG(label, x+shift, y, "", icons)
}

func renderXyChart(model *xyChartDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	if len(model.plots) == 0 {
		e := perr.New("No Plot to render, please provide a plot with some data", perr.Semantic)
		e.Type = "xychart"
		perr.Throw(e)
	}
	options := rc.Options
	icons := rc.Icons
	chart := record(get(config, "xyChart"))
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	small := jsmath.Round(size * 0.875)
	tiny := jsmath.Round(size * 0.75)
	measurer := rc.Measurer()
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	natural := dimension(get(chart, "width"), 700)
	width := common.CapWidth(options, natural, 240, 0)
	// A narrower chart is not as much shorter, so that the plot keeps room for its marks.
	height := jsmath.Round(dimension(get(chart, "height"), 500) * max(0.6, width/natural))
	orientation := model.orientation
	if orientation == "" {
		orientation, _ = get(chart, "chartOrientation").(string)
	}
	horizontal := orientation == "horizontal"
	// The value of a bar is always written past its end, which is what
	// showDataLabelOutsideBar asks for in Mermaid. Inside, whether it fits changes from bar
	// to bar and with the width of the chart.
	showValues := on(chart, "showDataLabel", false)
	x := model.xAxis
	plots := model.plots
	label := func(raw string, fontSize, maxWidth float64) text.Label {
		return text.Layout(raw, false, measurer, fontSize, maxWidth, 0, icons)
	}
	lineHeight := label("0", small, 4000).Height
	tinyHeight := label("0", tiny, 4000).Height

	titleText := ""
	if on(chart, "showTitle", true) && model.title != nil {
		titleText = *model.title
	}
	title := text.Layout(titleText, false, measurer, size, width-float64(2*pad), text.Bold, icons)
	titleSpace := 0.0
	if title.Height > 0 {
		titleSpace = title.Height + 12
	}

	type legendItem struct {
		index int
		label text.Label
	}
	var legend []legendItem
	legendWidth := 0.0
	if on(chart, "showLegend", true) {
		for index, p := range plots {
			if p.title == "" {
				continue
			}
			l := label(p.title, small, width*0.3)
			legend = append(legend, legendItem{index, l})
			legendWidth = max(legendWidth, l.Width+swatch+8)
		}
	}
	legendSpace := 0.0
	if len(legend) > 0 {
		legendSpace = legendWidth + legendGap
	}

	// The value range: as written, or taken from the data, where bars stand on zero and the
	// range grows to whole steps.
	bars := 0
	for _, p := range plots {
		if p.bar {
			bars++
		}
	}
	lo, ok := finite(model.yAxis.min, true)
	if !ok {
		lo = math.Inf(1)
	}
	hi, ok := finite(model.yAxis.max, true)
	if !ok {
		hi = math.Inf(-1)
	}
	isFinite := func(v float64) bool { return !math.IsInf(v, 0) && !math.IsNaN(v) }
	exact := model.hasSetYAxis && isFinite(lo) && isFinite(hi) && lo != hi
	if !exact {
		lo = math.Inf(1)
		hi = math.Inf(-1)
		for _, p := range plots {
			for _, d := range p.data {
				value, ok := finite(d.value, d.has)
				if !ok {
					continue
				}
				if value < lo {
					lo = value
				}
				if value > hi {
					hi = value
				}
			}
		}
		if lo > hi {
			lo, hi = 0, 0
		}
		if bars > 0 {
			lo = min(lo, 0)
			hi = max(hi, 0)
		}
		if lo == hi {
			if lo == 0 {
				hi = 1
			} else {
				lo -= float64(math.Abs(lo) / 2)
				hi += float64(math.Abs(hi) / 2)
			}
		}
	}

	valueMarks := func(length, spacing float64) []mark {
		step := tickStep(math.Abs(hi-lo), max(2, min(10, math.Floor(length/spacing))))
		if !exact {
			lo = tickValue(step, math.Floor(tickIndex(step, lo)+1e-9))
			hi = tickValue(step, math.Ceil(tickIndex(step, hi)-1e-9))
		}
		var marks []mark
		for _, value := range ticksBetween(min(lo, hi), max(lo, hi), step) {
			marks = append(marks, mark{at: (value - lo) / (hi - lo), label: label(jsnum.Format(value), small, 4000)})
		}
		return marks
	}

	// Places along the category axis are fractions of its length. Categories and bars sit
	// in the middle of equal slots; the points of a line on a numeric axis run from end to
	// end.
	count := 1
	if x.band {
		count = max(len(x.categories), 1)
	} else {
		for _, p := range plots {
			count = max(count, len(p.data))
		}
	}
	slot := 1 / float64(count)
	inset := 0.0
	if x.band || bars > 0 {
		inset = float64(slot / 2)
	}
	x0, hasX0 := finite(x.min, !x.band)
	x1, hasX1 := finite(x.max, !x.band)
	ranged := hasX0 && hasX1 && x0 != x1
	place := func(i, n int) float64 {
		if x.band {
			return (float64(i) + 0.5) * slot
		}
		f := 0.5
		switch {
		case n > 1:
			f = float64(i) / float64(n-1)
		case ranged:
			f = 0
		}
		return inset + float64(f*(1-float64(2*inset)))
	}

	categoryConf := record(get(chart, "xAxis"))
	valueConf := record(get(chart, "yAxis"))
	leftConf, bottomConf := valueConf, categoryConf
	if horizontal {
		leftConf, bottomConf = categoryConf, valueConf
	}
	leftTitleText, bottomTitleText := "", ""
	if on(leftConf, "showTitle", true) {
		if horizontal {
			leftTitleText = x.title
		} else {
			leftTitleText = model.yAxis.title
		}
	}
	if on(bottomConf, "showTitle", true) {
		if horizontal {
			bottomTitleText = model.yAxis.title
		} else {
			bottomTitleText = x.title
		}
	}
	leftTitle := label(leftTitleText, small, height*0.8)
	bottomTitle := label(bottomTitleText, small, width*0.8)
	leftTick := 0.0
	if on(leftConf, "showTick", true) {
		leftTick = tick
	}
	bottomTick := 0.0
	if on(bottomConf, "showTick", true) {
		bottomTick = tick
	}
	leftLabels := on(leftConf, "showLabel", true)
	bottomLabels := on(bottomConf, "showLabel", true)

	wrapAt := width * 0.3
	if !horizontal {
		wrapAt = max(48, float64((width-float64(2*pad)-64-legendSpace)*slot)-8)
	}
	var categories []text.Label
	if x.band && on(categoryConf, "showLabel", true) {
		for _, category := range x.categories {
			categories = append(categories, label(category, small, wrapAt))
		}
	}
	categoryWidth := 0.0
	categoryHeight := 0.0
	for _, l := range categories {
		categoryWidth = max(categoryWidth, l.Width)
		categoryHeight = max(categoryHeight, l.Height)
	}

	// The labelled places of the category axis. Labels that would run into each other are
	// thinned out.
	categoryMarks := func(length float64) []mark {
		var marks []mark
		switch {
		case x.band:
			need := 8.0
			if len(categories) > 0 {
				if horizontal {
					need = categoryHeight
				} else {
					need = categoryWidth + 12
				}
			}
			every := max(1, math.Ceil(need/float64(length*slot)))
			for i := 0.0; i < float64(len(x.categories)); i += every {
				var l text.Label
				if int(i) < len(categories) {
					l = categories[int(i)]
				}
				marks = append(marks, mark{at: (i + 0.5) * slot, label: l})
			}
		case ranged:
			spacing := 80.0
			if horizontal {
				spacing = 32
			}
			step := tickStep(math.Abs(x1-x0), max(2, min(10, math.Floor(length/spacing))))
			for _, value := range ticksBetween(min(x0, x1), max(x0, x1), step) {
				marks = append(marks, mark{
					at:    inset + float64(((value-x0)/(x1-x0))*(1-float64(2*inset))),
					label: label(jsnum.Format(value), small, 4000),
				})
			}
		case hasX0:
			marks = append(marks, mark{at: 0.5, label: label(jsnum.Format(x0), small, 4000)})
		}
		return marks
	}

	// Room past the end of the value axis for what is written beside a point or a bar.
	pointLabelWidth := 0.0
	valueWidth := 0.0
	for _, p := range plots {
		for _, l := range p.pointLabels {
			pointLabelWidth = max(pointLabelWidth, measurer.Width(l, tiny, 0))
		}
		if showValues && horizontal && p.bar {
			for _, d := range p.data {
				s := ""
				if v, ok := finite(d.value, d.has); ok {
					s = jsnum.Format(v)
				}
				valueWidth = max(valueWidth, measurer.Width(s, tiny, 0))
			}
		}
	}

	bottomLabelHeight := 0.0
	if bottomLabels {
		if horizontal || !x.band {
			bottomLabelHeight = lineHeight
		} else {
			bottomLabelHeight = categoryHeight
		}
	}
	bottomSpace := bottomTick + pick(bottomLabelHeight > 0, bottomLabelHeight+4, 0) + pick(bottomTitle.Height > 0, bottomTitle.Height+4, 0)
	headroom := 0.0
	if !horizontal {
		switch {
		case pointLabelWidth > 0 || showValues:
			headroom = tinyHeight + 4
		case leftLabels:
			headroom = float64(lineHeight / 2)
		}
	}
	top := pad + titleSpace + headroom
	plotHeight := max(1, height-pad-bottomSpace-top)
	bottom := top + plotHeight

	var leftMarks []mark
	if horizontal {
		leftMarks = categoryMarks(plotHeight)
	} else {
		leftMarks = valueMarks(plotHeight, 56)
	}
	leftLabelWidth := 0.0
	if leftLabels {
		leftLabelWidth = widest(leftMarks)
	}
	left := pad + pick(leftTitle.Height > 0, leftTitle.Height+6, 0) + pick(leftLabelWidth > 0, leftLabelWidth+6, 0) + leftTick
	beside := max(pick(pointLabelWidth > 0, pointLabelWidth+12, 0), pick(valueWidth > 0, valueWidth+8, 0))
	reach := 0.0
	if horizontal {
		reach = min(width*0.25, beside)
	}
	plotWidth := max(1, width-pad-left-legendSpace-reach)
	var bottomMarks []mark
	if horizontal {
		bottomMarks = valueMarks(plotWidth, 80)
	} else {
		bottomMarks = categoryMarks(plotWidth)
	}
	if bottomLabels && legendSpace+reach == 0 {
		over := 0.0
		for _, m := range bottomMarks {
			over = max(over, float64(m.at*plotWidth)+float64(m.label.Width/2)-plotWidth)
		}
		plotWidth = max(1, plotWidth-over)
	}
	right := left + plotWidth

	px := func(c, v float64) float64 {
		if horizontal {
			return left + float64(v*plotWidth)
		}
		return left + float64(c*plotWidth)
	}
	py := func(c, v float64) float64 {
		if horizontal {
			return top + float64(c*plotHeight)
		}
		return bottom - float64(v*plotHeight)
	}
	scale := func(value float64) float64 { return (value - lo) / (hi - lo) }
	clamp := func(v float64) float64 { return min(max(v, 0), 1) }

	// A line across the plot at a place on the value axis.
	rule := func(v float64) string {
		if horizontal {
			return "M" + svg.Num(px(0, v)) + "," + svg.Num(top) + "V" + svg.Num(bottom)
		}
		return "M" + svg.Num(left) + "," + svg.Num(py(0, v)) + "H" + svg.Num(right)
	}

	var grid strings.Builder
	gridMarks := leftMarks
	if horizontal {
		gridMarks = bottomMarks
	}
	for _, m := range gridMarks {
		grid.WriteString(rule(m.at))
	}

	var axis, labels strings.Builder
	if on(leftConf, "showAxisLine", true) {
		axis.WriteString("M" + svg.Num(left) + "," + svg.Num(top) + "V" + svg.Num(bottom))
	}
	if on(bottomConf, "showAxisLine", true) {
		axis.WriteString("M" + svg.Num(left) + "," + svg.Num(bottom) + "H" + svg.Num(right))
	}
	for _, m := range leftMarks {
		y := bottom - float64(m.at*plotHeight)
		if horizontal {
			y = top + float64(m.at*plotHeight)
		}
		if leftTick != 0 {
			axis.WriteString("M" + svg.Num(left) + "," + svg.Num(y) + "h" + jsnum.Format(-leftTick))
		}
		if leftLabels {
			labels.WriteString(aligned(m.label, left-leftTick-6, y, "end", icons))
		}
	}
	for _, m := range bottomMarks {
		at := left + float64(m.at*plotWidth)
		if bottomTick != 0 {
			axis.WriteString("M" + svg.Num(at) + "," + svg.Num(bottom) + "v" + jsnum.Format(bottomTick))
		}
		if bottomLabels {
			labels.WriteString(aligned(m.label, at, bottom+bottomTick+4+float64(m.label.Height/2), "middle", icons))
		}
	}

	span := plotWidth * slot
	if horizontal {
		span = plotHeight * slot
	}
	group := span * 0.7
	thickness := group / float64(max(bars, 1))
	gap := 0.0
	if bars > 1 {
		gap = min(2, thickness*0.2)
	}
	base := clamp(scale(0))
	// Bars that go both ways get a line to stand on.
	if bars > 0 && base > 0 && base < 1 {
		axis.WriteString(rule(base))
	}
	// Lines are drawn over the bars wherever they were declared, as a bar would hide part
	// of one under it. A series keeps the color of its place in the source.
	var series, lines, outside, pointLabels strings.Builder
	bar := 0
	for index, p := range plots {
		color := svg.SeriesColor(index)
		id := ""
		if p.title != "" {
			id = ` data-id="` + svg.EscText(p.title) + `"`
		}
		// A series declared before the categories can hold more values than there are
		// categories.
		n := len(p.data)
		if x.band {
			n = min(len(p.data), count)
		}
		if p.bar {
			offset := float64((float64(bar)-float64(float64(bars-1)/2))*thickness) - float64((thickness-gap)/2)
			bar++
			var rects strings.Builder
			for i := range n {
				value, ok := finite(p.data[i].value, p.data[i].has)
				if !ok {
					continue
				}
				c := place(i, n)
				v := clamp(scale(value))
				from := min(base, v)
				to := max(base, v)
				var bx, by, bw, bh float64
				if horizontal {
					bx = px(c, from)
					by = py(c, 0) + offset
					bw = float64((to - from) * plotWidth)
					bh = thickness - gap
				} else {
					bx = px(c, 0) + offset
					by = py(c, to)
					bw = thickness - gap
					bh = float64((to - from) * plotHeight)
				}
				rects.WriteString(`<rect x="` + svg.Num(bx) + `" y="` + svg.Num(by) + `" width="` + svg.Num(bw) + `" height="` + svg.Num(bh) + `"/>`)
				if !showValues {
					continue
				}
				valueText := jsnum.Format(value)
				w := measurer.Width(valueText, tiny, 0)
				// Towards the end of the bar: 1 when it runs up or right from the baseline,
				// -1 the other way.
				out := -1.0
				if v >= base {
					out = 1
				}
				tx := bx + float64(bw/2)
				ty := by + float64(bh/2)
				anchor := "middle"
				if horizontal {
					if bh < tiny {
						continue
					}
					if out > 0 {
						tx = bx + bw + float64(out*6)
						anchor = "start"
					} else {
						tx = bx + float64(out*6)
						anchor = "end"
					}
				} else {
					if w+2 > span/float64(bars) {
						continue
					}
					edge := by + bh
					if out > 0 {
						edge = by
					}
					ty = edge - float64(out*(float64(tinyHeight/2)+2))
				}
				outside.WriteString(`<text x="` + svg.Num(tx) + `" y="` + svg.Num(ty+float64(tiny*0.35)) + `" text-anchor="` + anchor + `">` + valueText + `</text>`)
			}
			series.WriteString(`<g class="pele-series pele-bars"` + id + ` fill="` + color + `">` + rects.String() + `</g>`)
			continue
		}

		// A line is cut where it leaves the value range, so it stays inside the plot.
		var d, dots strings.Builder
		pen := false
		pc := 0.0
		pv := math.NaN()
		for i := range n {
			value, ok := finite(p.data[i].value, p.data[i].has)
			if !ok {
				pv = math.NaN()
				continue
			}
			c := place(i, n)
			v := scale(value)
			visible := v >= 0 && v <= 1
			if pv != pv {
				if visible {
					d.WriteString("M" + svg.Num(px(c, v)) + "," + svg.Num(py(c, v)))
				}
				pen = visible
			} else {
				dv := v - pv
				t0 := 0.0
				t1 := 1.0
				if dv != 0 {
					a := -pv / dv
					b := (1 - pv) / dv
					t0 = max(t0, min(a, b))
					t1 = min(t1, max(a, b))
				} else if !visible {
					t0 = 2
				}
				if t0 <= t1 {
					c0 := pc + float64((c-pc)*t0)
					v0 := pv + float64(dv*t0)
					c1 := pc + float64((c-pc)*t1)
					v1 := pv + float64(dv*t1)
					if !pen || t0 > 0 {
						d.WriteString("M" + svg.Num(px(c0, v0)) + "," + svg.Num(py(c0, v0)))
					}
					d.WriteString("L" + svg.Num(px(c1, v1)) + "," + svg.Num(py(c1, v1)))
				}
				pen = visible
			}
			pc = c
			pv = v
			if !visible {
				continue
			}
			cx := px(c, v)
			cy := py(c, v)
			if span >= 12 {
				dots.WriteString(`<circle cx="` + svg.Num(cx) + `" cy="` + svg.Num(cy) + `" r="3"/>`)
			}
			raw := ""
			if i < len(p.pointLabels) {
				raw = p.pointLabels[i]
			}
			l := label(raw, tiny, 4000)
			if l.Height > 0 {
				half := float64(l.Width / 2)
				if horizontal {
					pointLabels.WriteString(aligned(l, cx+10, cy, "start", icons))
				} else {
					pointLabels.WriteString(aligned(l, max(min(cx, right-half), left+half), cy-6-float64(l.Height/2), "middle", icons))
				}
			}
		}
		lines.WriteString(`<g class="pele-series pele-line"` + id + ` fill="` + color + `">`)
		if d.Len() > 0 {
			lines.WriteString(`<path d="` + d.String() + `" fill="none" stroke="` + color + `" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>`)
		}
		lines.WriteString(dots.String() + "</g>")
	}

	titles := ""
	if leftTitle.Height > 0 {
		titles += `<g transform="translate(` + svg.Num(pad+float64(leftTitle.Height/2)) + `,` + svg.Num(top+float64(plotHeight/2)) + `) rotate(-90)">` + svg.LabelSVG(leftTitle, 0, 0, "", icons) + `</g>`
	}
	titles += svg.LabelSVG(bottomTitle, left+float64(plotWidth/2), height-pad-float64(bottomTitle.Height/2), "", icons)

	var legendOut strings.Builder
	legendHeight := 0.0
	for _, item := range legend {
		legendHeight += max(row, item.label.Height+6)
	}
	ly := top + max(0, float64((plotHeight-legendHeight)/2))
	lx := right + reach + legendGap
	for _, item := range legend {
		r := max(row, item.label.Height+6)
		if ly+r > height {
			break
		}
		y := ly + float64(r/2)
		color := svg.SeriesColor(item.index)
		legendOut.WriteString(`<g class="pele-legend-item" data-id="` + svg.EscText(plots[item.index].title) + `">`)
		if plots[item.index].bar {
			legendOut.WriteString(`<rect x="` + svg.Num(lx) + `" y="` + svg.Num(y-swatch/2) + `" width="` + jsnum.Format(swatch) + `" height="` + jsnum.Format(swatch) + `" rx="2" fill="` + color + `"/>`)
		} else {
			legendOut.WriteString(`<path d="M` + svg.Num(lx) + `,` + svg.Num(y) + `h` + jsnum.Format(swatch) + `" stroke="` + color + `" stroke-width="2" stroke-linecap="round"/>`)
		}
		legendOut.WriteString(svg.LabelSVG(item.label, lx+swatch+8+float64(item.label.Width/2), y, "", icons) + "</g>")
		ly += r
	}

	var content strings.Builder
	content.WriteString(svg.LabelSVG(title, width/2, pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, icons))
	if grid.Len() > 0 {
		content.WriteString(`<path class="pele-grid" d="` + grid.String() + `" stroke="var(--_a)"/>`)
	}
	content.WriteString(`<g class="pele-plot">` + series.String() + lines.String() + `</g>`)
	if axis.Len() > 0 {
		content.WriteString(`<path class="pele-axis" d="` + axis.String() + `" fill="none" stroke="var(--_b)"/>`)
	}
	if labels.Len() > 0 {
		content.WriteString(`<g class="pele-axis-labels" font-size="` + jsnum.Format(small) + `" fill="var(--_m)">` + labels.String() + `</g>`)
	}
	if titles != "" {
		content.WriteString(`<g class="pele-axis-titles" font-size="` + jsnum.Format(small) + `">` + titles + `</g>`)
	}
	if outside.Len() > 0 {
		content.WriteString(`<g class="pele-data-labels" font-size="` + jsnum.Format(tiny) + `">` + outside.String() + `</g>`)
	}
	if pointLabels.Len() > 0 {
		content.WriteString(`<g class="pele-point-labels" font-size="` + jsnum.Format(tiny) + `">` + pointLabels.String() + `</g>`)
	}
	if legendOut.Len() > 0 {
		content.WriteString(`<g class="pele-legend" font-size="` + jsnum.Format(small) + `">` + legendOut.String() + `</g>`)
	}

	acc := svg.Accessible{}
	if model.accTitle != nil {
		acc.AccTitle = *model.accTitle
	}
	if model.accDescr != nil {
		acc.AccDescr = *model.accDescr
	}
	doc := svg.Document("xychart", width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, acc, content.String())
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: []core.Link{}}
}
