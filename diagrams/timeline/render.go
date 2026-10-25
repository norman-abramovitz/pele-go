package timeline

import (
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const (
	padX        = 12
	padY        = 8
	minText     = 152
	maxText     = 216
	eventTextTD = 264
	gap         = 20
	axis        = 20
	spine       = 14
	eventGap    = 8
	sectionGap  = 8
	tail        = 12
	onColor     = ` fill="var(--_bg)"`
	line        = ` fill="none" stroke="var(--_b)"`
)

type group struct {
	section int
	periods []int
}

func box(x, y, w, h float64, attrs string) string {
	return `<rect x="` + svg.Num(x) + `" y="` + svg.Num(y) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) + `"` + svg.Rounded(attrs) + `/>`
}

func renderTimeline(model *timelineDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	if model.direction == "TD" {
		return draw(model, config, rc, true)
	}
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered { return draw(model, config, rc, down) })
}

// disablesMulticolor is config.timeline?.disableMulticolor === true.
func disablesMulticolor(config *jsobj.Object) bool {
	section, _ := config.Get("timeline")
	conf, ok := section.(*jsobj.Object)
	if !ok {
		return false
	}
	v, _ := conf.Get("disableMulticolor")
	return v == true
}

func draw(model *timelineDb, config *jsobj.Object, rc *core.Context, vertical bool) core.Rendered {
	opts := rc.Options
	size := 16.0
	if opts.FontSize != nil {
		size = *opts.FontSize
	}
	small := jsmath.Round(size * 0.875)
	measurer := rc.Measurer()
	pad := 8.0
	if opts.Padding != nil {
		pad = *opts.Padding
	}
	multicolor := !disablesMulticolor(config)
	periods := model.periods
	hasSections := len(model.sections) > 0

	// Mermaid draws nothing for periods that come before the first section; here they lead, in a neutral color.
	groups := make([]*group, len(model.sections))
	for section := range model.sections {
		groups[section] = &group{section: section}
	}
	leading := &group{section: -1}
	for i, period := range periods {
		if period.sectionIndex >= 0 && period.sectionIndex < len(groups) {
			groups[period.sectionIndex].periods = append(groups[period.sectionIndex].periods, i)
		} else {
			leading.periods = append(leading.periods, i)
		}
	}
	if len(leading.periods) > 0 {
		groups = append([]*group{leading}, groups...)
	}

	// Columns widen, up to a point, for a word that would not fit; past that the word is cut.
	// Running down, the two columns share the width the host has.
	room := math.Inf(1)
	if vertical && opts.MaxWidth != nil && *opts.MaxWidth > 0 {
		room = *opts.MaxWidth - float64(2*pad)
	}
	const taken = axis + spine*2 + 4*padX
	periodText := min(minText, max(80, jsmath.Round((room-taken)*0.38)))
	eventText := min(eventTextTD, max(110, room-taken-periodText))
	textW := periodText
	eventTextW := func() float64 {
		if vertical {
			return eventText
		}
		return textW
	}
	periodLabels := make([]text.Label, len(periods))
	eventLabels := make([][]text.Label, len(periods))
	layout := func(fit func(raw string, measurer text.Measurer, size, max float64, base text.Style) text.Label) {
		for i, p := range periods {
			periodLabels[i] = fit(p.text, measurer, size, textW, 0)
		}
		for i, p := range periods {
			labels := make([]text.Label, len(p.events))
			for k, e := range p.events {
				labels[k] = fit(e, measurer, small, eventTextW(), 0)
			}
			eventLabels[i] = labels
		}
	}
	layout(common.PlainLabel)
	widest := 0.0
	for _, label := range periodLabels {
		widest = max(widest, label.Width)
	}
	if !vertical {
		for _, labels := range eventLabels {
			for _, label := range labels {
				widest = max(widest, label.Width)
			}
		}
	}
	overflow := widest > textW
	if vertical {
		for _, labels := range eventLabels {
			for _, label := range labels {
				overflow = overflow || label.Width > eventText
			}
		}
	}
	if overflow {
		limit := periodText
		if math.IsInf(room, 1) {
			limit = maxText
		}
		textW = min(max(math.Ceil(widest), periodText), limit)
		layout(common.FitLabel)
	}

	lineHeight := jsmath.Round(size * 1.5)
	colW := textW + 2*padX
	eventW := eventTextW() + 2*padX
	chipHeight := func(label text.Label) float64 { return max(label.Height, lineHeight) + 2*padY }
	eventHeight := func(label text.Label) float64 { return max(label.Height, jsmath.Round(small*1.5)) + 2*padY }
	colorOf := func(g *group, period int) string {
		if g.section >= 0 {
			return svg.SeriesColor(g.section)
		}
		if hasSections {
			return "var(--_m)"
		}
		if multicolor {
			return svg.SeriesColor(period)
		}
		return svg.SeriesColor(0)
	}

	hasEvents := false
	for _, labels := range eventLabels {
		if len(labels) > 0 {
			hasEvents = true
		}
	}
	slots := 0
	for _, g := range groups {
		slots += max(len(g.periods), 1)
	}

	pitch := colW + gap
	drawn := len(periods) > 0
	bodyW := 0.0
	switch {
	case vertical:
		extra := 0.0
		if hasEvents {
			extra = axis + spine*2 + eventW
		} else if drawn {
			extra = axis + tail
		}
		bodyW = colW + extra
	case slots > 0:
		extra := 0.0
		if drawn {
			extra = tail + svg.Arrow
		}
		bodyW = float64(float64(slots)*pitch) - gap + extra
	}
	title := common.FitLabel(model.title, measurer, size, max(bodyW, 320), text.Bold)
	titleH := 0.0
	if title.Height > 0 {
		titleH = title.Height + 16
	}
	width := max(bodyW, title.Width)
	left := pad + float64((width-bodyW)/2)
	top := pad + titleH

	sectionLabel := func(g *group, w float64) text.Label {
		return common.FitLabel(model.sections[g.section], measurer, size, w-2*padX, text.Bold)
	}
	header := func(g *group, label text.Label, x, y, w, h float64) string {
		return box(x, y, w, h, ` fill="`+svg.SeriesColor(g.section)+`"`) +
			svg.LabelSVG(label, x+float64(w/2), y+float64(h/2), ` font-weight="var(--_hw)"`+onColor, nil)
	}
	period := func(i int, color string, x, y, h float64) string {
		return box(x, y, colW, h, ` fill="`+color+`"`) + svg.LabelSVG(periodLabels[i], x+float64(colW/2), y+float64(h/2), onColor, nil)
	}
	event := func(label text.Label, color string, x, y, h float64) string {
		return `<g class="pele-node pele-event">` +
			box(x, y, eventW, h, ` fill="`+color+`" fill-opacity="0.16"`) +
			svg.LabelSVG(label, x+float64(eventW/2), y+float64(h/2), ` font-size="`+jsnum.Format(small)+`"`, nil) +
			`</g>`
	}
	dot := func(x, y float64, color string) string {
		return `<circle class="pele-dot" cx="` + svg.Num(x) + `" cy="` + svg.Num(y) + `" r="4" fill="` + color + `" stroke="var(--_bg)" stroke-width="2"/>`
	}
	open := func(g *group) string {
		if g.section < 0 {
			return ""
		}
		return `<g class="pele-cluster pele-section" data-id="` + svg.EscText(jsstr.Trim(model.sections[g.section])) + `">`
	}
	openPeriod := func(i int) string {
		return `<g class="pele-node pele-period" data-id="` + svg.EscText(jsstr.Trim(periods[i].text)) + `">`
	}

	var body strings.Builder
	// axisPath goes before body, which is written first.
	axisPath := ""
	var height float64

	if vertical {
		axisX := left + colW + axis
		spineX := axisX + spine
		eventsX := spineX + spine
		y := top
		for _, g := range groups {
			body.WriteString(open(g))
			if g.section >= 0 {
				label := sectionLabel(g, bodyW)
				h := chipHeight(label)
				body.WriteString(header(g, label, left, y, bodyW, h))
				y += h + sectionGap + 4
			}
			for _, i := range g.periods {
				color := colorOf(g, i)
				h := chipHeight(periodLabels[i])
				// A period and its first event share one center line, so the line between them runs straight.
				first := h
				if len(eventLabels[i]) > 0 {
					first = eventHeight(eventLabels[i][0])
				}
				cy := y + float64(max(h, first)/2)
				var d strings.Builder
				d.WriteString("M" + svg.Num(left+colW) + "," + svg.Num(cy) + "H" + svg.Num(axisX))
				var cards strings.Builder
				ey := cy - float64(first/2)
				last := cy
				for _, label := range eventLabels[i] {
					eh := eventHeight(label)
					last = ey + float64(eh/2)
					d.WriteString("M" + svg.Num(spineX) + "," + svg.Num(last) + "H" + svg.Num(eventsX))
					cards.WriteString(event(label, color, eventsX, ey, eh))
					ey += eh + eventGap
				}
				if cards.Len() > 0 {
					d.WriteString("M" + svg.Num(axisX) + "," + svg.Num(cy) + "H" + svg.Num(spineX) + "V" + svg.Num(last))
				}
				body.WriteString(openPeriod(i) +
					period(i, color, left, cy-float64(h/2), h) +
					`<path class="pele-edge" d="` + d.String() + `"` + line + `/>` +
					dot(axisX, cy, color) +
					cards.String() +
					`</g>`)
				y = max(cy+float64(h/2), ey-eventGap) + gap
			}
			if len(g.periods) == 0 {
				y += gap - sectionGap - 4
			}
			if g.section >= 0 {
				body.WriteString(`</g>`)
			}
		}
		if len(groups) > 0 {
			y -= gap
		}
		if drawn {
			tip := y + tail + svg.Arrow
			axisPath = `<path class="pele-axis" d="M` + svg.Num(axisX) + `,` + svg.Num(top) + `V` + svg.Num(tip-svg.Arrow+1) + `" fill="none" stroke="var(--_l)"/>` +
				svg.Marker("arrow_point", axisX, tip, 0, 1, "var(--_l)")
			y = tip
		}
		height = y + pad
	} else {
		headerH := 0.0
		sectionLabels := make([]*text.Label, len(groups))
		for k, g := range groups {
			if g.section < 0 {
				continue
			}
			label := sectionLabel(g, float64(float64(max(len(g.periods), 1))*pitch)-gap)
			headerH = max(headerH, chipHeight(label))
			sectionLabels[k] = &label
		}
		chipH := 0.0
		for _, label := range periodLabels {
			chipH = max(chipH, chipHeight(label))
		}
		periodsY := top
		if hasSections {
			periodsY = top + (headerH + sectionGap)
		}
		axisY := periodsY + chipH
		if chipH > 0 {
			axisY = periodsY + chipH + axis
		}
		eventsY := axisY + axis
		bottom := top + headerH
		if drawn {
			bottom = axisY + tail
		}
		slot := 0
		for k, g := range groups {
			span := max(len(g.periods), 1)
			gx := left + float64(float64(slot)*pitch)
			body.WriteString(open(g))
			if label := sectionLabels[k]; label != nil {
				body.WriteString(header(g, *label, gx, top, float64(float64(span)*pitch)-gap, headerH))
			}
			for j, i := range g.periods {
				color := colorOf(g, i)
				x := gx + float64(float64(j)*pitch)
				cx := x + float64(colW/2)
				to := axisY
				if len(eventLabels[i]) > 0 {
					to = eventsY
				}
				var d strings.Builder
				d.WriteString("M" + svg.Num(cx) + "," + svg.Num(periodsY+chipH) + "V" + svg.Num(to))
				var cards strings.Builder
				ey := eventsY
				for _, eventLabel := range eventLabels[i] {
					eh := eventHeight(eventLabel)
					if ey > eventsY {
						d.WriteString("M" + svg.Num(cx) + "," + svg.Num(ey-eventGap) + "V" + svg.Num(ey))
					}
					cards.WriteString(event(eventLabel, color, x, ey, eh))
					ey += eh + eventGap
				}
				bottom = max(bottom, ey-eventGap)
				body.WriteString(openPeriod(i) +
					period(i, color, x, periodsY, chipH) +
					`<path class="pele-edge" d="` + d.String() + `"` + line + `/>` +
					dot(cx, axisY, color) +
					cards.String() +
					`</g>`)
			}
			if g.section >= 0 {
				body.WriteString(`</g>`)
			}
			slot += span
		}
		if drawn {
			tip := left + bodyW
			axisPath = `<path class="pele-axis" d="M` + svg.Num(left) + `,` + svg.Num(axisY) + `H` + svg.Num(tip-svg.Arrow+1) + `" fill="none" stroke="var(--_l)"/>` +
				svg.Marker("arrow_point", tip, axisY, 1, 0, "var(--_l)")
		}
		height = bottom + pad
	}

	totalWidth := math.Ceil(width + float64(2*pad))
	totalHeight := math.Ceil(max(height, float64(pad*2)))
	content := svg.LabelSVG(title, float64(totalWidth/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil) + axisPath + body.String()
	return core.Rendered{
		SVG: svg.Document("timeline", totalWidth, totalHeight, size, svg.DocumentOptions{
			IDPrefix:       opts.IDPrefix,
			Responsive:     rc.Responsive,
			FontFamily:     opts.FontFamily,
			FontFamilyMono: opts.FontFamilyMono,
		}, svg.Accessible{AccTitle: model.accTitle, AccDescr: model.accDescr}, content),
		Width:  totalWidth,
		Height: totalHeight,
		Links:  []core.Link{},
	}
}
