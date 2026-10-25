package treemap

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
	header = 28
	side   = 6
	line   = 1.25
)

// section is config.treemap when it is an object. Any other value reads as one without keys.
func section(config *jsobj.Object) *jsobj.Object {
	v, _ := config.Get("treemap")
	o, _ := v.(*jsobj.Object)
	return o
}

func option(config *jsobj.Object, key string, fallback, least, most float64) float64 {
	value, _ := section(config).Get(key)
	if v, ok := value.(float64); ok && v >= least {
		return min(v, most)
	}
	return fallback
}

func renderTreemap(m *treemapModel, config *jsobj.Object, rc *core.Context) core.Rendered {
	opts := rc.Options
	size := 16.0
	if opts.FontSize != nil {
		size = *opts.FontSize
	}
	small := jsmath.Round(float64(size * 0.875))
	tiny := jsmath.Round(float64(size * 0.75))
	least := jsmath.Round(float64(size * 0.5625))
	measurer := rc.Measurer()
	treemap := section(config)
	var pad float64
	if opts.Padding != nil {
		pad = *opts.Padding
	} else {
		pad = option(config, "diagramPadding", 8, 0, 2000)
	}

	// Mermaid sizes the diagram as ten times `nodeWidth` by ten times `nodeHeight`.
	width := jsmath.Round(common.CapWidth(opts, float64(option(config, "nodeWidth", 64, 1, 400)*10), 200, float64(2*pad)))
	height := jsmath.Round(float64(option(config, "nodeHeight", 40, 1, 400) * 10))
	showValues := true
	if v, _ := treemap.Get("showValues"); v == false {
		showValues = false
	}
	valueFormat := ","
	if v, _ := treemap.Get("valueFormat"); v != nil {
		if s, ok := v.(string); ok {
			valueFormat = s
		}
	}
	format := formatter(valueFormat)
	boxes := layoutTreemap(m.roots, width, height, insets{gap: option(config, "padding", 4, 0, 200), header: header, side: side})

	title := text.Layout(m.title, false, measurer, size, common.TitleRoom(opts), text.Bold, rc.Icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 8
	}
	left := pad + max(0, float64((title.Width-width)/2))
	top := pad + titleHeight

	var sections, leaves strings.Builder
	for _, b := range boxes {
		node := b.node
		style := svg.ResolveStyle(node.cssCompiledStyles)
		color := svg.SeriesColor(b.series)
		x := left + b.x
		y := top + b.y
		name := jsstr.Trim(text.DecodeEntities(node.name))
		value := ""
		if showValues {
			value = format(b.value)
		}
		classSelector := ""
		if node.classSelector != nil {
			classSelector = *node.classSelector
		}
		group := `data-id="` + svg.EscText(node.name) + `">`
		rect := `<rect x="` + svg.Num(x) + `" y="` + svg.Num(y) + `" width="` + svg.Num(b.w) + `" height="` + svg.Num(b.h) + `" fill="` + color + `"`

		if !node.leaf {
			label := ""
			if b.h >= header {
				room := b.w - 2*side - 4
				valueWidth := 0.0
				if value != "" {
					valueWidth = measurer.Width(value, tiny, 0)
				}
				withValue := valueWidth > 0 && room-valueWidth-12 >= float64(3*small)
				labelRoom := room
				if withValue {
					labelRoom = room - valueWidth - 12
				}
				fitted, ok := common.FitText(name, labelRoom, small, small, measurer, text.Bold)
				baseline := y + header/2 + 1
				if ok {
					label += `<text x="` + svg.Num(x+side+2) + `" y="` + svg.Num(baseline+float64(small*0.35)) + `" font-weight="var(--_hw)"` + style.Text + `>` + svg.Esc(fitted.Text) + `</text>`
				}
				if withValue {
					label += `<text class="pele-value" x="` + svg.Num(x+b.w-side-2) + `" y="` + svg.Num(baseline+float64(tiny*0.35)) + `" font-size="` + jsnum.Format(tiny) +
						`" text-anchor="end" fill="var(--_m)"` + style.Text + `>` + svg.Esc(value) + `</text>`
				}
			}
			// A section is a tint of its color, unless a class gives it a fill of its own.
			tint := ` fill-opacity="0.12"`
			if strings.Contains(style.Shape, `"fill:`) || strings.Contains(style.Shape, `;fill:`) {
				tint = ""
			}
			sections.WriteString(`<g class="pele-node pele-section` + svg.ClassNames(classSelector) + `" ` + group +
				rect + svg.Rounded(tint+` stroke="`+color+`"`+style.Shape) + `/>` + label + `</g>`)
			continue
		}

		label := ""
		room := b.w - 8
		tallest := min(size, math.Floor((b.h-4)/line))
		if tallest >= least {
			if fitted, ok := common.FitText(name, room, tallest, least, measurer, 0); ok {
				cx := svg.Num(x + float64(b.w/2))
				cy := y + float64(b.h/2)
				valueSize := max(least, jsmath.Round(float64(fitted.Size*0.8)))
				lines := (fitted.Size + valueSize) * line
				if value != "" && lines <= b.h-4 && measurer.Width(value, valueSize, 0) <= room {
					first := cy - float64(lines/2) + float64(float64(fitted.Size*line)/2)
					second := cy + float64(lines/2) - float64(float64(valueSize*line)/2)
					label = `<text x="` + cx + `" y="` + svg.Num(first+float64(fitted.Size*0.35)) + `" font-size="` + svg.Num(fitted.Size) + `"` + style.Text + `>` + svg.Esc(fitted.Text) + `</text>` +
						`<text class="pele-value" x="` + cx + `" y="` + svg.Num(second+float64(valueSize*0.35)) + `" font-size="` + svg.Num(valueSize) + `"` + style.Text + `>` + svg.Esc(value) + `</text>`
				} else {
					label = `<text x="` + cx + `" y="` + svg.Num(cy+float64(fitted.Size*0.35)) + `" font-size="` + svg.Num(fitted.Size) + `"` + style.Text + `>` + svg.Esc(fitted.Text) + `</text>`
				}
			}
		}
		leaves.WriteString(`<g class="pele-node pele-leaf` + svg.ClassNames(classSelector) + `" ` + group + rect + svg.Rounded(style.Shape) + `/>` + label + `</g>`)
	}

	totalWidth := math.Ceil(max(width, title.Width) + float64(2*pad))
	totalHeight := math.Ceil(height + titleHeight + float64(2*pad))
	content := svg.LabelSVG(title, float64(totalWidth/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, rc.Icons)
	if sections.Len() > 0 {
		content += `<g class="pele-sections" font-size="` + jsnum.Format(small) + `">` + sections.String() + `</g>`
	}
	if leaves.Len() > 0 {
		content += `<g class="pele-leaves" text-anchor="middle" fill="var(--_bg)">` + leaves.String() + `</g>`
	}

	doc := svg.Document("treemap", totalWidth, totalHeight, size, svg.DocumentOptions{
		IDPrefix:       opts.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     opts.FontFamily,
		FontFamilyMono: opts.FontFamilyMono,
	}, svg.Accessible{AccTitle: m.accTitle, AccDescr: m.accDescr}, content)
	return core.Rendered{SVG: doc, Width: totalWidth, Height: totalHeight, Links: []core.Link{}}
}
