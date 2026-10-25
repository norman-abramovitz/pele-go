package quadrant

import (
	"slices"
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
	gap     = 4
	careful = 300
)

type dot struct {
	cx, cy, reach float64
	text          text.Label
}

func setting(config *jsobj.Object, key string) any {
	section, _ := config.Get("quadrantChart")
	obj, _ := section.(*jsobj.Object)
	v, _ := obj.Get(key)
	return v
}

func length(config *jsobj.Object, key string, fallback, most float64) float64 {
	if v, ok := setting(config, key).(float64); ok && v >= 0 {
		return min(v, most)
	}
	return fallback
}

// pick is cond ? a : b.
func pick(cond bool, a, b float64) float64 {
	if cond {
		return a
	}
	return b
}

func hex(color string) string {
	if strings.HasPrefix(color, "#") {
		return color
	}
	return "#" + color
}

func renderQuadrant(model *quadrantDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	options := rc.Options
	icons := rc.Icons
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	axisSize := jsmath.Round(size * 0.875)
	pointSize := jsmath.Round(size * 0.75)
	measurer := rc.Measurer()
	natural := jsmath.Round(max(100, length(config, "chartWidth", 500, 4000)))
	width := jsmath.Round(common.CapWidth(options, natural, 280, 0))
	height := jsmath.Round(float64(max(100, length(config, "chartHeight", 500, 4000))*width) / natural)
	padding := 8.0
	if options.Padding != nil {
		padding = *options.Padding
	}
	outer := length(config, "quadrantPadding", padding, 200)
	xPad := length(config, "xAxisLabelPadding", 5, 200)
	yPad := length(config, "yAxisLabelPadding", 5, 200)
	hasPoints := len(model.points) > 0

	label := func(t *quadrantText, fontSize, most float64) text.Label {
		if t == nil {
			return text.Layout("", false, measurer, fontSize, max(most, 40), 0, icons)
		}
		return text.Layout(t.text, t.markdown, measurer, fontSize, max(most, 40), 0, icons)
	}

	titleText := ""
	if model.title != nil {
		titleText = *model.title
	}
	title := text.Layout(titleText, false, measurer, size, max(width-float64(2*outer), 40), text.Bold, icons)
	titlePad := length(config, "titlePadding", 10, 200)
	titleSpace := 0.0
	if title.Height > 0 {
		titleSpace = title.Height + titlePad
	}

	nonEmpty := func(t *quadrantText) bool { return t != nil && t.text != "" }
	hasX := nonEmpty(model.xAxisLeft) || nonEmpty(model.xAxisRight)
	hasY := nonEmpty(model.yAxisBottom) || nonEmpty(model.yAxisTop)
	// Mermaid moves the x axis labels below the chart as soon as there are points.
	xBelow := hasPoints || setting(config, "xAxisPosition") == "bottom"
	yRight := setting(config, "yAxisPosition") == "right"

	side := width - float64(2*outer)
	xLeftRoom := side
	if model.xAxisRight != nil {
		xLeftRoom = float64(side/2) - 16
	}
	xLeft := label(model.xAxisLeft, axisSize, xLeftRoom)
	xRight := label(model.xAxisRight, axisSize, float64(side/2)-16)
	xHeight := max(xLeft.Height, xRight.Height)
	xSpace := 0.0
	if hasX {
		xSpace = xHeight + xPad
	}
	tall := height - float64(2*outer) - titleSpace - xSpace
	yBottomRoom := tall
	if model.yAxisTop != nil {
		yBottomRoom = float64(tall/2) - 16
	}
	yBottom := label(model.yAxisBottom, axisSize, yBottomRoom)
	yTop := label(model.yAxisTop, axisSize, float64(tall/2)-16)
	yHeight := max(yBottom.Height, yTop.Height)
	ySpace := 0.0
	if hasY {
		ySpace = yHeight + yPad
	}

	left := outer + pick(yRight, 0, ySpace)
	top := outer + titleSpace + pick(xBelow, 0, xSpace)
	w := max(0, side-ySpace)
	h := max(0, tall)
	tileW := max(0, float64((w-gap)/2))
	tileH := max(0, float64((h-gap)/2))

	// Boxes a point label keeps clear of: the gaps between quadrants, quadrant labels,
	// points, and the labels placed before it.
	taken := []float64{left + tileW, top, left + w - tileW, top + h, left, top + tileH, left + w, top + h - tileH}
	isClear := func(from int, x0, y0, x1, y1 float64) bool {
		for k := from; k < len(taken); k += 4 {
			if x0 < taken[k+2] && x1 > taken[k] && y0 < taken[k+3] && y1 > taken[k+1] {
				return false
			}
		}
		return true
	}

	textTop := length(config, "quadrantTextTopPadding", 5, 200)
	var quadrants strings.Builder
	for k := range 4 {
		x := left
		if k == 0 || k == 3 {
			x = left + w - tileW
		}
		y := top
		if k >= 2 {
			y = top + h - tileH
		}
		t := label(model.quadrants[k], size, tileW-16)
		cx := x + float64(tileW/2)
		cy := y + float64(tileH/2)
		if hasPoints {
			cy = y + textTop + float64(t.Height/2)
		}
		halfW, halfH := float64(t.Width/2), float64(t.Height/2)
		taken = append(taken, cx-halfW, cy-halfH, cx+halfW, cy+halfH)
		quadrants.WriteString(`<g class="pele-quadrant" data-id="` + jsnum.Format(float64(k+1)) + `">` +
			`<rect x="` + svg.Num(x) + `" y="` + svg.Num(y) + `" width="` + svg.Num(tileW) + `" height="` + svg.Num(tileH) + `"` + svg.RoundedStyle + `/>` +
			svg.LabelSVG(t, cx, cy, ` class="pele-label" fill="var(--_fg)" stroke="none"`, icons) +
			`</g>`)
	}

	axes := ""
	if hasX {
		y := top - xPad - float64(xHeight/2)
		if xBelow {
			y = top + h + xPad + float64(xHeight/2)
		}
		leftX := left + float64(xLeft.Width/2)
		if xRight.Height > 0 {
			leftX = left + float64(w/4)
		}
		axes += svg.LabelSVG(xLeft, leftX, y, "", icons) + svg.LabelSVG(xRight, left+float64(float64(w*3)/4), y, "", icons)
	}
	if hasY {
		x := left - yPad - float64(yHeight/2)
		if yRight {
			x = left + w + yPad + float64(yHeight/2)
		}
		turned := func(t text.Label, y float64) string {
			if t.Height <= 0 {
				return ""
			}
			return `<g transform="translate(` + svg.Num(x) + `,` + svg.Num(y) + `) rotate(-90)">` + svg.LabelSVG(t, 0, 0, "", icons) + `</g>`
		}
		bottomY := top + h - float64(yBottom.Width/2)
		if yTop.Height > 0 {
			bottomY = top + float64(float64(h*3)/4)
		}
		axes += turned(yBottom, bottomY) + turned(yTop, top+float64(h/4))
	}

	radius := length(config, "pointRadius", 5, 200)
	textPad := length(config, "pointTextPadding", 5, 200)
	fill := svg.SeriesColor(0)
	var dots []dot
	var circles []string
	for _, point := range model.points {
		if !(point.x >= 0 && point.x <= 1 && point.y >= 0 && point.y <= 1) {
			continue
		}
		shared, hasShared := model.classes[point.className]
		r := radius
		switch {
		case point.radius != nil:
			r = *point.radius
		case hasShared && shared.radius != nil:
			r = *shared.radius
		}
		r = min(r, 2000)
		color := point.color
		if color == nil && hasShared {
			color = shared.color
		}
		strokeColor := point.strokeColor
		if strokeColor == nil && hasShared {
			strokeColor = shared.strokeColor
		}
		strokeWidth := point.strokeWidth
		if strokeWidth == nil && hasShared {
			strokeWidth = shared.strokeWidth
		}
		var styles []string
		if color != nil {
			styles = append(styles, "fill:"+hex(*color))
		}
		if strokeWidth != nil {
			if strokeColor != nil {
				styles = append(styles, "stroke:"+hex(*strokeColor))
			}
			styles = append(styles, "stroke-width:"+*strokeWidth)
		}
		cx := left + float64(point.x*w)
		cy := top + float64((1-point.y)*h)
		widthReach := 0.0
		if strokeWidth != nil {
			widthReach = float64(min(jsnum.ParseInt(*strokeWidth), 400) / 2)
		}
		reach := r + widthReach
		dots = append(dots, dot{cx: cx, cy: cy, reach: reach, text: label(&point.text, pointSize, 200)})
		stroke := ""
		if strokeWidth != nil && strokeColor == nil {
			stroke = ` stroke="` + fill + `"`
		}
		circles = append(circles, `<circle class="pele-point`+svg.ClassNames(point.className)+`" data-id="`+svg.EscText(point.text.text)+
			`" cx="`+svg.Num(cx)+`" cy="`+svg.Num(cy)+`" r="`+svg.Num(r)+`"`+stroke+svg.ResolveStyle(styles).Shape+`/>`)
	}

	// A label goes below its point, or above, right or left of it when that is the first
	// place that is inside the chart and free. A second round lets it cross the gaps between
	// quadrants. Large charts skip the search for a free place.
	isCareful := len(dots) <= careful
	if isCareful {
		for _, d := range dots {
			taken = append(taken, d.cx-d.reach, d.cy-d.reach, d.cx+d.reach, d.cy+d.reach)
		}
	}
	var labels strings.Builder
	for _, d := range dots {
		cx, cy, t := d.cx, d.cy, d.text
		halfW := float64(t.Width / 2)
		halfH := float64(t.Size*0.6) + float64((t.Height-t.LineHeight)/2)
		away := d.reach + textPad
		spots := [8]float64{cx, cy + away + halfH, cx, cy - away - halfH, cx + away + halfW, cy, cx - away - halfW, cy}
		tx := cx
		ty := spots[1]
		found := false
		for k := 0; k < 16; k += 2 {
			spot := k % 8
			x := spots[spot]
			if spot < 4 && t.Width < w {
				x = min(max(spots[spot], left+halfW), left+w-halfW)
			}
			y := spots[spot+1]
			if x-halfW < left || x+halfW > left+w || y-halfH < top || y+halfH > top+h {
				continue
			}
			from := 8
			if k < 8 {
				from = 0
			}
			free := !isCareful || isClear(from, x-halfW, y-halfH, x+halfW, y+halfH)
			if !found || free {
				tx = x
				ty = y
				found = true
			}
			if free {
				break
			}
		}
		if isCareful {
			taken = append(taken, tx-halfW, ty-halfH, tx+halfW, ty+halfH)
		}
		labels.WriteString(svg.LabelSVG(t, tx, ty, ` class="pele-label"`, icons))
	}

	// Mermaid puts each new point under the ones before it.
	slices.Reverse(circles)
	content := svg.LabelSVG(title, float64(width/2), float64(titlePad/2)+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, icons) +
		`<g class="pele-quadrants" fill="var(--_s)" stroke="var(--_b)">` + quadrants.String() + `</g>`
	if axes != "" {
		content += `<g class="pele-axis-labels" font-size="` + jsnum.Format(axisSize) + `" fill="var(--_m)">` + axes + `</g>`
	}
	if len(dots) > 0 {
		content += `<g class="pele-points" fill="` + fill + `">` + strings.Join(circles, "") + `</g>` +
			`<g class="pele-point-labels" font-size="` + jsnum.Format(pointSize) + `">` + labels.String() + `</g>`
	}

	acc := svg.Accessible{}
	if model.accTitle != nil {
		acc.AccTitle = *model.accTitle
	}
	if model.accDescr != nil {
		acc.AccDescr = *model.accDescr
	}
	doc := svg.Document("quadrantChart", width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, acc, content)
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: []core.Link{}}
}
