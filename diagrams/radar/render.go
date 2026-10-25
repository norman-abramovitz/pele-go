package radar

import (
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const (
	swatch = 12
	row    = 26
	gap    = 24
)

type point struct{ x, y float64 }

func option(config *jsobj.Object, key string, fallback, least, most float64) float64 {
	section, _ := config.Get("radar")
	obj, _ := section.(*jsobj.Object)
	if v, ok := obj.Get(key); ok {
		if n, isNum := v.(float64); isNum && n >= least {
			return min(n, most)
		}
	}
	return fallback
}

func relativeRadius(value, lo, hi, radius float64) float64 {
	clipped := min(max(value, lo), hi)
	scaled := float64(radius * (clipped - lo))
	// Keep the original rounding unless multiplication overflows.
	if !math.IsInf(scaled, 0) && !math.IsNaN(scaled) {
		return scaled / (hi - lo)
	}
	return radius * float64((clipped-lo)/(hi-lo))
}

// closedRoundCurve is a closed Catmull-Rom spline through the points, as cubic Bézier
// segments.
func closedRoundCurve(points []point, tension float64) string {
	n := len(points)
	var d strings.Builder
	d.WriteString("M" + svg.Num(points[0].x) + "," + svg.Num(points[0].y))
	for i := range n {
		p0 := points[(i-1+n)%n]
		p1 := points[i]
		p2 := points[(i+1)%n]
		p3 := points[(i+2)%n]
		d.WriteString(" C" + svg.Num(p1.x+float64((p2.x-p0.x)*tension)) + "," + svg.Num(p1.y+float64((p2.y-p0.y)*tension)) +
			" " + svg.Num(p2.x-float64((p3.x-p1.x)*tension)) + "," + svg.Num(p2.y-float64((p3.y-p1.y)*tension)) +
			" " + svg.Num(p2.x) + "," + svg.Num(p2.y))
	}
	return d.String() + " Z"
}

func polygon(points []point) string {
	var out strings.Builder
	for _, p := range points {
		if out.Len() > 0 {
			out.WriteByte(' ')
		}
		out.WriteString(svg.Num(p.x) + "," + svg.Num(p.y))
	}
	return out.String()
}

func renderRadar(model *radarModel, config *jsobj.Object, rc *core.Context) core.Rendered {
	natural := option(config, "width", 320, 1, 4000)
	return common.FitWidth(rc.Options, natural, 160, func(width float64, tight bool) core.Rendered {
		return draw(model, config, rc, width, float64(option(config, "height", 320, 1, 4000)*width)/natural, tight)
	})
}

// draw draws the chart; tight is set when it does not fit the width it has with its legend
// beside it.
func draw(model *radarModel, config *jsobj.Object, rc *core.Context, width, height float64, tight bool) core.Rendered {
	options := rc.Options
	icons := rc.Icons
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

	axisScale := option(config, "axisScaleFactor", 1, 0, 10)
	labelFactor := option(config, "axisLabelFactor", 1.05, 0, 10)
	tension := option(config, "curveTension", 0.17, 0, 1)
	radius := float64(min(width, height) / 2)

	axes, curves := model.axes, model.curves
	ticks, graticule, lo := model.options.ticks, model.options.graticule, model.options.min
	count := len(axes)
	unit := make([]point, count)
	for i := range axes {
		angle := float64(2*math.Pi*float64(i))/float64(count) - math.Pi/2
		unit[i] = point{jsmath.Cos(angle), jsmath.Sin(angle)}
	}

	hi := math.Inf(-1)
	if model.options.max != nil {
		hi = *model.options.max
	} else {
		for _, c := range curves {
			for _, value := range c.entries {
				if value > hi {
					hi = value
				}
			}
		}
	}

	// The chart is drawn around the origin. Margins reserve space; labels and the legend
	// widen it.
	x0 := float64(-width/2) - option(config, "marginLeft", 16, 0, 2000)
	x1 := float64(width/2) + option(config, "marginRight", 16, 0, 2000)
	y0 := float64(-height/2) - option(config, "marginTop", 16, 0, 2000)
	y1 := float64(height/2) + option(config, "marginBottom", 16, 0, 2000)

	var grid strings.Builder
	for i := 0.0; i < ticks; i++ {
		r := min(radius, float64(radius*(i+1))/ticks)
		if graticule == "circle" {
			grid.WriteString(`<circle r="` + svg.Num(r) + `"/>`)
		} else {
			ring := make([]point, count)
			for k, u := range unit {
				ring[k] = point{u.x * r, u.y * r}
			}
			grid.WriteString(`<polygon points="` + polygon(ring) + `"/>`)
		}
	}

	var spokes, labels strings.Builder
	for i, u := range unit {
		reach := radius * axisScale
		spokes.WriteString("M0,0L" + svg.Num(u.x*reach) + "," + svg.Num(u.y*reach))
		label := text.Layout(axes[i].label, false, measurer, small, 4000, 0, icons)
		at := float64(radius*labelFactor) + 6
		halfW, halfH := float64(label.Width/2), float64(label.Height/2)
		// Each label hangs off its axis on the side away from the center.
		dx, dy := 0.0, 0.0
		switch {
		case u.x > 0.01:
			dx = halfW
		case u.x < -0.01:
			dx = -halfW
		}
		switch {
		case u.y > 0.01:
			dy = halfH
		case u.y < -0.01:
			dy = -halfH
		}
		cx := float64(u.x*at) + dx
		cy := float64(u.y*at) + dy
		labels.WriteString(`<g class="pele-axis-label" data-id="` + svg.EscText(axes[i].name) + `">` + svg.LabelSVG(label, cx, cy, "", icons) + `</g>`)
		x0 = min(x0, cx-halfW)
		x1 = max(x1, cx+halfW)
		y0 = min(y0, cy-halfH)
		y1 = max(y1, cy+halfH)
	}

	var shapes strings.Builder
	for index, c := range curves {
		// Mermaid leaves out a curve that does not have a value for every axis.
		if len(c.entries) != count {
			continue
		}
		points := make([]point, count)
		for i, value := range c.entries {
			r := relativeRadius(value, lo, hi, radius)
			if math.IsInf(r, 0) || math.IsNaN(r) {
				r = 0
			}
			points[i] = point{unit[i].x * r, unit[i].y * r}
		}
		color := svg.SeriesColor(index)
		attrs := ` class="pele-curve" data-id="` + svg.EscText(c.name) + `" fill="` + color + `" stroke="` + color + `"`
		if graticule == "circle" {
			shapes.WriteString(`<path` + attrs + ` d="` + closedRoundCurve(points, tension) + `"/>`)
		} else {
			shapes.WriteString(`<polygon` + attrs + ` points="` + polygon(points) + `"/>`)
		}
	}

	var legend []text.Label
	if model.options.showLegend {
		for _, c := range curves {
			legend = append(legend, text.Layout(c.label, false, measurer, small, 4000, 0, icons))
		}
	}
	legendWidth := 0.0
	for _, l := range legend {
		legendWidth = max(legendWidth, l.Width)
	}
	legendHeight := float64(len(legend) * row)
	legendX := x1 + gap
	legendTop := float64(-legendHeight / 2)
	if tight {
		legendX = float64(-(swatch + 8 + legendWidth) / 2)
		legendTop = y1 + gap/2
	}
	if len(legend) > 0 && tight {
		x0 = min(x0, legendX)
		x1 = max(x1, -legendX)
		y1 = legendTop + legendHeight
	} else if len(legend) > 0 {
		x1 = legendX + swatch + 8 + legendWidth
		y0 = min(y0, float64(-legendHeight/2))
		y1 = max(y1, float64(legendHeight/2))
	}
	var legendOut strings.Builder
	for index, l := range legend {
		y := legendTop + float64(float64(index)*row) + row/2
		color := svg.SeriesColor(index)
		legendOut.WriteString(`<g class="pele-legend-item" data-id="` + svg.EscText(curves[index].name) + `">` +
			`<rect x="` + svg.Num(legendX) + `" y="` + svg.Num(y-swatch/2) + `" width="` + jsnum.Format(swatch) + `" height="` + jsnum.Format(swatch) +
			`" rx="2" fill="` + color + `" fill-opacity="0.4" stroke="` + color + `"/>` +
			svg.LabelSVG(l, legendX+swatch+8+float64(l.Width/2), y, "", icons) +
			`</g>`)
	}

	title := text.Layout(model.title, false, measurer, size, common.TitleRoom(options), text.Bold, icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 8
	}
	bodyWidth := max(x1-x0, title.Width)
	totalWidth := math.Ceil(bodyWidth + float64(2*pad))
	totalHeight := math.Ceil(y1 - y0 + titleHeight + float64(2*pad))
	origin := "translate(" + svg.Num(pad+float64((bodyWidth-(x1-x0))/2)-x0) + "," + svg.Num(pad+titleHeight-y0) + ")"

	var content strings.Builder
	content.WriteString(svg.LabelSVG(title, float64(totalWidth/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, icons))
	content.WriteString(`<g transform="` + origin + `">`)
	if grid.Len() > 0 {
		content.WriteString(`<g class="pele-graticule" fill="none" stroke="var(--_a)">` + grid.String() + `</g>`)
	}
	if spokes.Len() > 0 {
		content.WriteString(`<path class="pele-axes" d="` + spokes.String() + `" fill="none" stroke="var(--_b)"/>`)
	}
	if shapes.Len() > 0 {
		content.WriteString(`<g class="pele-curves" fill-opacity="0.2" stroke-width="1.5" stroke-linejoin="round">` + shapes.String() + `</g>`)
	}
	if labels.Len() > 0 {
		content.WriteString(`<g class="pele-axis-labels" font-size="` + jsnum.Format(small) + `">` + labels.String() + `</g>`)
	}
	if legendOut.Len() > 0 {
		content.WriteString(`<g class="pele-legend" font-size="` + jsnum.Format(small) + `">` + legendOut.String() + `</g>`)
	}
	content.WriteString("</g>")

	doc := svg.Document("radar", totalWidth, totalHeight, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccTitle: model.accTitle, AccDescr: model.accDescr}, content.String())
	return core.Rendered{SVG: doc, Width: totalWidth, Height: totalHeight, Links: []core.Link{}}
}
