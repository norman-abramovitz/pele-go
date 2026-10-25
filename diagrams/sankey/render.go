package sankey

import (
	"math"
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/omap"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const (
	labelGap  = 6
	valueGap  = 15
	labelRoom = 1.8
	careful   = 300
)

// contentId hashes text twice to 32 bits, so different gradients practically never share an
// id.
func contentId(text string) string {
	a := uint32(0x811c9dc5)
	b := uint32(5381)
	for _, c := range jsstr.UTF16(text) {
		a = (a ^ uint32(c)) * 0x01000193
		b = b*33 + uint32(c)
	}
	return strconv.FormatUint(uint64(a), 36) + strconv.FormatUint(uint64(b), 36)
}

// fillOf is /^ style="fill:(.*);"$/.exec(style)?.[1].
func fillOf(style string) (string, bool) {
	const head, tail = ` style="fill:`, `;"`
	if len(style) < len(head)+len(tail) || !strings.HasPrefix(style, head) || !strings.HasSuffix(style, tail) {
		return "", false
	}
	inner := style[len(head) : len(style)-len(tail)]
	for i := 0; i < len(inner); {
		r, n := jsstr.Decode(inner, i)
		if jsstr.IsLineEnd(r) {
			return "", false
		}
		i += n
	}
	return inner, true
}

// isIDPrefix is /^[\w-]+$/.
func isIDPrefix(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// bandKey is a band number as a Map key, where NaN is one key as SameValueZero has it.
func bandKey(band float64) uint64 {
	if band != band {
		return math.Float64bits(math.NaN())
	}
	return math.Float64bits(band + 0)
}

type nodeLabel struct {
	value   string
	width   float64
	after   bool
	left, y float64
}

func numberOption(conf *jsobj.Object, key string, fallback, least, most float64) float64 {
	v, _ := conf.Get(key)
	if value, ok := v.(float64); ok && value >= least {
		return min(value, most)
	}
	return fallback
}

func textOption(conf *jsobj.Object, key string) string {
	switch v, _ := conf.Get(key); value := v.(type) {
	case string:
		return value
	case float64:
		if isFinite(value) {
			return jsnum.Format(value)
		}
	}
	return ""
}

// rounded keeps two decimals, as Mermaid shows values; numbers too large for that are left
// as they are.
func rounded(value float64) string {
	if math.Abs(value) < 1e15 {
		value = jsmath.Round(value*100) / 100
	}
	return jsnum.Format(value)
}

// fillStyle is a colour from the configuration, as an inline style that overrides the series
// colour.
func fillStyle(color any) string {
	if s, ok := color.(string); ok {
		return svg.ResolveStyle([]string{"fill:" + s}).Shape
	}
	return ""
}

func renderSankey(model *sankeyDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	section, _ := config.Get("sankey")
	conf, _ := section.(*jsobj.Object)
	natural := numberOption(conf, "width", 600, 1, 10000)
	return common.FitWidth(rc.Options, natural, 160, func(chartWidth float64, _ bool) core.Rendered {
		return draw(model, conf, rc, chartWidth, natural)
	})
}

func draw(model *sankeyDb, conf *jsobj.Object, rc *core.Context, chartWidth, natural float64) core.Rendered {
	options := rc.Options
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	least := jsmath.Round(size * 0.625)
	// Labels shrink with a chart narrowed to fit, as they would if the whole picture were scaled down.
	small := max(least, jsmath.Round(size*0.875*min(1, chartWidth/natural)))
	measurer := rc.Measurer()
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}

	chartHeight := numberOption(conf, "height", 400, 1, 10000)
	nodeWidth := min(numberOption(conf, "nodeWidth", 10, 1, 200), chartWidth)
	showValues, _ := conf.Get("showValues")
	showsValues := showValues != false
	prefix := text.DecodeEntities(textOption(conf, "prefix"))
	suffix := text.DecodeEntities(textOption(conf, "suffix"))
	labelStyle, _ := conf.Get("labelStyle")
	outlined := labelStyle == "outlined"
	align := alignment("justify")
	if a, _ := conf.Get("nodeAlignment"); a == "left" || a == "right" || a == "center" {
		align = alignment(a.(string))
	}
	linkColor := "gradient"
	if c, ok := conf.Get("linkColor"); ok {
		if s, isString := c.(string); isString {
			linkColor = s
		}
	}

	padding := numberOption(conf, "nodePadding", 12, 0, 200)
	if showsValues {
		padding += valueGap
	}
	lay := func(labelSize float64) ([]placedNode, []placedLink) {
		return layoutSankey(model, layoutOptions{
			width:       chartWidth,
			height:      chartHeight,
			nodeWidth:   nodeWidth,
			nodePadding: padding,
			labelHeight: labelSize * 1.15,
			alignment:   align,
		})
	}
	nodes, links := lay(small)
	// Labels also shrink with the most crowded column, so each keeps about labelRoom lines.
	rows := map[int]int{}
	fullest := 1
	for _, node := range nodes {
		rows[node.layer]++
		fullest = max(fullest, rows[node.layer])
	}
	if crowded := max(least, jsmath.Round(chartHeight/float64(fullest)/labelRoom)); crowded < small {
		small = crowded
		nodes, links = lay(small)
	}
	lineHeight := small * 1.5

	custom := map[string]string{}
	if colors, ok := conf.Get("nodeColors"); ok {
		if obj, isObj := colors.(*jsobj.Object); isObj {
			for _, id := range obj.Keys() {
				color, _ := obj.Get(id)
				if style := fillStyle(color); style != "" {
					custom[id] = style
				}
			}
		}
	}
	nodeStyle := func(i int) string { return custom[model.nodes[i].id] }
	// Match the whole fill to preserve semicolons in escaped entities.
	nodeColor := func(i int) string {
		if fill, ok := fillOf(nodeStyle(i)); ok {
			return fill
		}
		return svg.SeriesColor(i)
	}

	// Labels sit beside the bars, on the side that faces the middle of the chart unless that
	// side is crowded.
	central := 0
	most := 0.0
	for _, node := range nodes {
		if node.value > most {
			most = node.value
			central = node.layer
		}
	}
	minX, maxX := 0.0, chartWidth
	minY, maxY := 0.0, chartHeight
	labels := make([]*nodeLabel, len(nodes))
	for i, node := range nodes {
		name := text.DecodeEntities(model.nodes[i].id)
		value := ""
		if showsValues {
			value = prefix + rounded(node.value) + suffix
		}
		measured := name
		if value != "" {
			measured = name + " " + value
		}
		width := measurer.Width(measured, small, 0)
		var after bool
		if outlined {
			after = node.layer >= central
		} else {
			after = node.x < chartWidth/2
		}
		x := node.x - labelGap
		if after {
			x = node.x + nodeWidth + labelGap
		}
		y := node.y + float64(node.height/2)
		left := x - width
		if after {
			minX = min(minX, x)
			maxX = max(maxX, x+width)
			left = x
		} else {
			minX = min(minX, x-width)
			maxX = max(maxX, x)
		}
		minY = min(minY, y-float64(small*0.75))
		maxY = max(maxY, y+float64(small*0.75))
		labels[i] = &nodeLabel{value: value, width: width, after: after, left: left, y: y}
	}
	// A label moves at most one line; retain two bands on each side.
	bands := map[uint64][]*nodeLabel{}
	if len(nodes) <= careful {
		for _, label := range labels {
			band := math.Floor(label.y / lineHeight)
			for _, key := range [...]float64{band - 2, band - 1, band, band + 1, band + 2} {
				k := bandKey(key)
				bands[k] = append(bands[k], label)
			}
		}
	}
	var labelsOut, nodesOut strings.Builder
	for i, node := range nodes {
		id := model.nodes[i].id
		name := text.DecodeEntities(id)
		height := max(node.height, 1)
		nodesOut.WriteString(`<rect class="pele-sankey-node" data-id="` + svg.EscText(id) + `" x="` + svg.Num(node.x) + `" y="` + svg.Num(node.y) + `" width="` +
			svg.Num(nodeWidth) + `" height="` + svg.Num(height) + `" rx="` + svg.Num(min(2, height/2)) + `" fill="` + svg.SeriesColor(i) + `"` + nodeStyle(i) + `/>`)

		label := labels[i]
		overlapsLabel := func(left, y float64) bool {
			for _, other := range bands[bandKey(math.Floor(y/lineHeight))] {
				if other != label && math.Abs(other.y-y) < lineHeight && left < other.left+other.width+labelGap && left+label.width > other.left-labelGap {
					return true
				}
			}
			return false
		}
		// Try the other side, then an adjacent line when both sides are crowded.
		if len(nodes) <= careful && overlapsLabel(label.left, label.y) {
			half := float64(small * 0.75)
		place:
			for _, dy := range [...]float64{0, -lineHeight, lineHeight} {
				for _, after := range [...]bool{label.after, !label.after} {
					left := node.x - labelGap - label.width
					if after {
						left = node.x + nodeWidth + labelGap
					}
					y := label.y + dy
					if overlapsLabel(left, y) || left < minX || left+label.width > maxX || y-half < minY || y+half > maxY {
						continue
					}
					if someNode(nodes, func(j int, bar placedNode) bool {
						return j != i && left < bar.x+nodeWidth+labelGap && left+label.width > bar.x-labelGap && y-half < bar.y+bar.height && y+half > bar.y
					}) {
						continue
					}
					// A label stays nearer its own bar than any other in the column, so it can't name that one.
					own := max(0, node.y-y, y-node.y-node.height)
					if someNode(nodes, func(j int, bar placedNode) bool {
						return j != i && bar.x == node.x && max(0, bar.y-y, y-bar.y-bar.height) <= own
					}) {
						continue
					}
					label.left = left
					label.y = y
					label.after = after
					break place
				}
			}
		}
		value, after, y := label.value, label.after, label.y
		x := node.x - labelGap
		if after {
			x = node.x + nodeWidth + labelGap
		}
		anchor := "end"
		if after {
			anchor = "start"
		}
		position := ` x="` + svg.Num(x) + `" y="` + svg.Num(y+float64(small*0.35)) + `" text-anchor="` + anchor + `"`
		body := svg.Esc(name)
		if value != "" {
			body += ` <tspan class="pele-sankey-value" fill="var(--_m)">` + svg.Esc(value) + `</tspan>`
		}
		labelsOut.WriteString(`<g class="pele-label" data-id="` + svg.EscText(id) + `">`)
		if outlined {
			plain := svg.Esc(name)
			if value != "" {
				plain += " " + svg.Esc(value)
			}
			labelsOut.WriteString(`<text` + position + ` fill="none" stroke="var(--_bg)" stroke-width="3" stroke-linejoin="round">` + plain + `</text>`)
		}
		labelsOut.WriteString(`<text` + position + `>` + body + `</text></g>`)
	}

	fixed := ""
	if linkColor != "gradient" && linkColor != "source" && linkColor != "target" {
		fixed = fillStyle(linkColor)
	}
	// Without an idPrefix diagrams share gradient ids, and Chrome then uses a hidden or
	// differently themed copy's gradient. Other characters would end the url() and void the
	// whole fill.
	idPrefix := ""
	if options.IDPrefix != nil {
		idPrefix = *options.IDPrefix
	}
	gradient := linkColor == "gradient" && isIDPrefix(idPrefix)
	var gradients omap.Map[string, string]
	var linksOut strings.Builder
	for _, link := range links {
		end := link.source
		if linkColor == "target" {
			end = link.target
		}
		half := float64(max(link.thickness, 1) / 2)
		x0 := nodes[link.source].x + nodeWidth
		x1 := nodes[link.target].x
		style := fixed
		if style == "" {
			style = nodeStyle(end)
		}
		paint := `fill="` + svg.SeriesColor(end) + `"` + style
		if gradient {
			// Ids come from the gradient itself, so diagrams sharing a prefix agree on them. The
			// colour after the url() is the fallback.
			stops := `x1="` + svg.Num(x0) + `" x2="` + svg.Num(x1) + `"><stop offset="0" stop-color="` + nodeColor(link.source) + `"/>` +
				`<stop offset="1" stop-color="` + nodeColor(link.target) + `"/>`
			id := idPrefix + "-sankey-" + contentId(stops)
			gradients.Set(id, `<linearGradient id="`+id+`" gradientUnits="userSpaceOnUse" `+stops+`</linearGradient>`)
			paint = `fill="url(#` + id + `) ` + nodeColor(link.source) + `"`
		}
		mid := svg.Num((x0 + x1) / 2)
		from := svg.EscText(model.nodes[link.source].id)
		to := svg.EscText(model.nodes[link.target].id)
		linksOut.WriteString(`<path class="pele-sankey-link" data-id="` + from + `-&gt;` + to + `" d="M` + svg.Num(x0) + `,` + svg.Num(link.sourceY-half) +
			`C` + mid + `,` + svg.Num(link.sourceY-half) + ` ` + mid + `,` + svg.Num(link.targetY-half) + ` ` + svg.Num(x1) + `,` + svg.Num(link.targetY-half) +
			`L` + svg.Num(x1) + `,` + svg.Num(link.targetY+half) +
			`C` + mid + `,` + svg.Num(link.targetY+half) + ` ` + mid + `,` + svg.Num(link.sourceY+half) + ` ` + svg.Num(x0) + `,` + svg.Num(link.sourceY+half) +
			`Z" ` + paint + `>` +
			`<title>` + from + ` → ` + to + `: ` + svg.Esc(prefix) + rounded(link.value) + svg.Esc(suffix) + `</title></path>`)
	}

	title := text.Layout(model.title, false, measurer, size, common.TitleRoom(options), text.Bold, rc.Icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}
	inner := max(maxX-minX, title.Width)
	ox := pad - minX + float64((inner-(maxX-minX))/2)
	oy := pad + titleHeight - minY
	width := math.Ceil(inner + float64(2*pad))
	height := math.Ceil(maxY - minY + titleHeight + float64(2*pad))

	content := svg.LabelSVG(title, width/2, pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, rc.Icons) +
		`<g transform="translate(` + svg.Num(ox) + `,` + svg.Num(oy) + `)">` +
		defs(&gradients) +
		`<g class="pele-sankey-links" fill-opacity="0.4">` + linksOut.String() + `</g>` +
		`<g class="pele-sankey-nodes">` + nodesOut.String() + `</g>` +
		`<g class="pele-labels" font-size="` + jsnum.Format(small) + `">` + labelsOut.String() + `</g>` +
		`</g>`
	doc := svg.Document("sankey", width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{}, content)
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: []core.Link{}}
}

func defs(gradients *omap.Map[string, string]) string {
	if gradients.Len() == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<defs>")
	for _, g := range gradients.All() {
		b.WriteString(g)
	}
	return b.String() + "</defs>"
}

func someNode(nodes []placedNode, test func(j int, bar placedNode) bool) bool {
	for j, bar := range nodes {
		if test(j, bar) {
			return true
		}
	}
	return false
}
