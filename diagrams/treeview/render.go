package treeview

import (
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const (
	iconGap           = 6
	descGap           = 16
	guideInset        = 7
	elbow             = 4
	highlightOverhang = 4
)

func setting(config *jsobj.Object, key string, fallback, least, most float64) float64 {
	section, _ := config.Get("treeView")
	o, _ := section.(*jsobj.Object)
	value, _ := o.Get(key)
	if v, ok := value.(float64); ok && v >= least && v <= most {
		return v
	}
	return fallback
}

type treeSettings struct {
	paddingX, paddingY, rowIndent, lineThickness float64
}

// settingsOf is Mermaid's layout options, with Pele's defaults.
func settingsOf(config *jsobj.Object) treeSettings {
	return treeSettings{
		paddingX:      setting(config, "paddingX", 5, 0, 100),
		paddingY:      setting(config, "paddingY", 2, 0, 100),
		rowIndent:     setting(config, "rowIndent", 15, 0, 400),
		lineThickness: setting(config, "lineThickness", 1, 0.25, 12),
	}
}

type row struct {
	node        *treeNode
	x, cy       float64
	icon        string
	text        string
	labelX      float64
	highlighted bool
	// label and description are set only when drawn tight.
	label, description *text.Label
	top, height        float64
}

func renderTreeView(m treeViewModel, config *jsobj.Object, rc *core.Context) core.Rendered {
	return common.Tighten(rc.Options, func(tight bool) core.Rendered { return draw(m, config, rc, tight) })
}

func draw(m treeViewModel, config *jsobj.Object, rc *core.Context, tight bool) core.Rendered {
	opts := rc.Options
	size := 16.0
	if opts.FontSize != nil {
		size = *opts.FontSize
	}
	measurer := rc.Measurer()
	pad := 8.0
	if opts.Padding != nil {
		pad = *opts.Padding
	}
	maxWidth := 320.0
	if opts.MaxWidth != nil {
		maxWidth = *opts.MaxWidth
	}
	icons := iconConfig(config)

	s := settingsOf(config)
	paddingX, paddingY := s.paddingX, s.paddingY
	maxDepth := 0
	type entry struct {
		node  *treeNode
		depth int
	}
	var depths []entry
	for _, node := range m.root.children {
		depths = append(depths, entry{node, 0})
	}
	for i := 0; tight && i < len(depths); i++ {
		e := depths[i]
		maxDepth = max(maxDepth, e.depth)
		for _, child := range e.node.children {
			depths = append(depths, entry{child, e.depth + 1})
		}
	}
	step := s.rowIndent + paddingX
	if tight {
		step = min(step, max(2, (maxWidth-120)/float64(max(1, maxDepth))))
	}
	rowHeight := jsmath.Round(float64(size*1.5)) + float64(2*paddingY)
	iconSize := size

	titleRoom := 4000.0
	if tight {
		titleRoom = common.TitleRoom(opts)
	}
	title := text.Layout(m.title, false, measurer, size, titleRoom, text.Bold, rc.Icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}

	// Siblings line up: when one entry of a folder has an icon, the others leave room for one.
	hasIcon := func(list []*treeNode) bool {
		for _, child := range list {
			if getNodeIcon(child, icons) != "" {
				return true
			}
		}
		return false
	}
	var rows []*row
	rowOf := map[*treeNode]*row{}
	type pendingEntry struct {
		node  *treeNode
		depth int
		slot  bool
	}
	var pending []pendingEntry
	push := func(list []*treeNode, depth int) {
		slot := hasIcon(list)
		for i := len(list) - 1; i >= 0; i-- {
			pending = append(pending, pendingEntry{list[i], depth, slot})
		}
	}
	push(m.root.children, 0)
	labelRight := 0.0
	anyHighlight := false
	rowsHeight := 0.0
	for len(pending) > 0 {
		p := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		node := p.node
		x := float64(float64(p.depth) * step)
		label := text.DecodeEntities(node.name)
		slotWidth := 0.0
		if p.slot {
			slotWidth = iconSize + iconGap
		}
		labelX := x + slotWidth
		var style text.Style
		if node.nodeType == nodeDirectory {
			style = text.Bold
		}
		room := max(40, maxWidth-float64(2*pad)-2*highlightOverhang-labelX)
		var fitted, description *text.Label
		if tight {
			l := common.LiteralLabel(label, measurer, size, room, style)
			fitted = &l
			if node.description != "" {
				d := common.LiteralLabel(text.DecodeEntities(node.description), measurer, size, room, text.Italic)
				description = &d
			}
		}
		var width float64
		if fitted != nil {
			width = fitted.Width
		} else {
			width = measurer.Width(label, size, style)
		}
		if labelX+width > labelRight {
			labelRight = labelX + width
		}
		// A class annotation is a single name, so it holds "highlight" only by being it.
		highlighted := node.cssClass == "highlight"
		if highlighted {
			anyHighlight = true
		}
		height := rowHeight
		cy := rowsHeight + float64(height/2)
		if tight {
			labelHeight, descHeight := 0.0, 0.0
			if fitted != nil {
				labelHeight = fitted.Height
			}
			if description != nil {
				descHeight = description.Height
			}
			height = max(rowHeight, labelHeight+descHeight+float64(2*paddingY))
			cy = rowsHeight + (paddingY + float64(max(iconSize, labelHeight)/2))
		}
		r := &row{node: node, x: x, cy: cy, icon: getNodeIcon(node, icons), text: label, labelX: labelX, highlighted: highlighted,
			label: fitted, description: description, top: rowsHeight, height: height}
		rowsHeight += height
		rows = append(rows, r)
		rowOf[node] = r
		push(node.children, p.depth+1)
	}

	descX := labelRight + descGap
	width := labelRight
	for _, r := range rows {
		description := r.node.description
		if description == "" {
			continue
		}
		var w float64
		if r.description != nil {
			w = r.description.Width
		} else {
			w = measurer.Width(text.DecodeEntities(description), size, text.Italic)
		}
		at := descX
		if tight {
			at = r.labelX
		}
		width = max(width, at+w)
	}
	width = max(width, title.Width)
	overhang := 0.0
	if anyHighlight {
		overhang = highlightOverhang
	}
	inset := pad + overhang
	top := pad + titleHeight
	totalWidth := math.Ceil(width + float64(2*inset))
	totalHeight := math.Ceil(top + rowsHeight + pad)

	var guides strings.Builder
	for _, r := range rows {
		children := r.node.children
		if len(children) == 0 {
			continue
		}
		gx := inset + r.x + guideInset
		tick := max(gx, inset+r.x+step-paddingX)
		last := top + rowOf[children[len(children)-1]].cy
		radius := min(elbow, tick-gx)
		d := "M" + svg.Num(gx) + "," + svg.Num(top+r.top+r.height-2) + "V" + svg.Num(last-radius)
		if radius > 0 {
			d += "Q" + svg.Num(gx) + "," + svg.Num(last) + " " + svg.Num(gx+radius) + "," + svg.Num(last) + "H" + svg.Num(tick)
		}
		for i := 0; i < len(children)-1 && tick > gx; i++ {
			d += "M" + svg.Num(gx) + "," + svg.Num(top+rowOf[children[i]].cy) + "H" + svg.Num(tick)
		}
		guides.WriteString(`<path d="` + d + `"/>`)
	}

	baseline := float64(size * 0.35)
	var body strings.Builder
	for _, r := range rows {
		node := r.node
		cy := top + r.cy
		directory := node.nodeType == nodeDirectory
		weight := ""
		if directory {
			weight = ` font-weight="var(--_hw)"`
		}
		var inner strings.Builder
		if r.highlighted {
			x := inset + r.x - highlightOverhang
			inner.WriteString(`<rect class="pele-tree-highlight" x="` + svg.Num(x) + `" y="` + svg.Num(top+r.top+1) + `" width="` +
				svg.Num(totalWidth-inset+highlightOverhang-x) + `" height="` + svg.Num(r.height-2) +
				`" fill="var(--_c)" fill-opacity="0.14"` + svg.RoundedStyle + `/>`)
		}
		if r.icon != "" {
			name := ""
			if strings.HasPrefix(r.icon, builtinPack+":") {
				name = r.icon[len(builtinPack)+1:]
			}
			glyph, ok := glyphs[name]
			if !ok {
				if directory {
					glyph = glyphs["folder"]
				} else {
					glyph = glyphs["file"]
				}
			}
			drawn := rc.Icons.Resolve(r.icon)
			if drawn == "" {
				drawn = `<path d="` + glyph + `" stroke="var(--_m)" stroke-width="1.5"/>`
			}
			inner.WriteString(`<svg class="pele-icon" data-icon="` + svg.EscText(r.icon) + `" x="` + svg.Num(inset+r.x) + `" y="` +
				svg.Num(cy-float64(iconSize/2)) + `" width="` + svg.Num(iconSize) + `" height="` + svg.Num(iconSize) +
				`" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
				drawn + `</svg>`)
		}
		if r.label != nil {
			inner.WriteString(common.LeftLabelSVG(*r.label, inset+r.labelX, top+r.top+paddingY, ` class="pele-label"`+weight))
		} else if r.text != "" {
			inner.WriteString(`<text class="pele-label" x="` + svg.Num(inset+r.labelX) + `" y="` + svg.Num(cy+baseline) + `"` + weight +
				` xml:space="preserve">` + svg.Esc(r.text) + `</text>`)
		}
		if r.description != nil {
			labelHeight := 0.0
			if r.label != nil {
				labelHeight = r.label.Height
			}
			inner.WriteString(common.LeftLabelSVG(*r.description, inset+r.labelX, top+r.top+paddingY+labelHeight,
				` class="pele-tree-description" font-style="italic" fill="var(--_m)"`))
		} else if node.description != "" {
			inner.WriteString(`<text class="pele-tree-description" x="` + svg.Num(inset+descX) + `" y="` + svg.Num(cy+baseline) +
				`" font-style="italic" fill="var(--_m)" xml:space="preserve">` + svg.Esc(text.DecodeEntities(node.description)) + `</text>`)
		}
		kind := "file"
		if directory {
			kind = "dir"
		}
		body.WriteString(`<g class="pele-node pele-tree-` + kind + svg.ClassNames(node.cssClass) + `" data-id="` + svg.EscText(node.name) + `">` +
			inner.String() + `</g>`)
	}

	content := svg.LabelSVG(title, float64(totalWidth/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, rc.Icons)
	if guides.Len() > 0 {
		content += `<g class="pele-tree-guides" fill="none" stroke="var(--_b)" stroke-width="` + svg.Num(s.lineThickness) + `">` + guides.String() + `</g>`
	}
	content += body.String()

	doc := svg.Document("treeView", totalWidth, totalHeight, size, svg.DocumentOptions{
		IDPrefix:       opts.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     opts.FontFamily,
		FontFamilyMono: opts.FontFamilyMono,
	}, svg.Accessible{AccTitle: m.accTitle, AccDescr: m.accDescr}, content)
	return core.Rendered{SVG: doc, Width: totalWidth, Height: totalHeight, Links: []core.Link{}}
}
