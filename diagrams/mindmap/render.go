package mindmap

import (
	"math"
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

// shapeNames is indexed by Mermaid's node type. A node without delimiters is text on a line,
// except the root.
var shapeNames = [...]string{"", "rounded", "rect", "circle", "cloud", "bang", "hex"}

const rootShape = "stadium"

const (
	gapX = 36
	gapY = 8
	// groupGap is between two subtrees, of which at least one has children of its own.
	groupGap   = 18
	branchGapX = 52
	branchGapY = 28
	textPad    = 6
	pillPad    = 12
	tint       = 0.22
	iconGap    = 6
	// Running down, each level is set in by indent, and links leave a node stem from its left edge.
	indent  = 28
	stem    = 12
	bendMax = 8
	rowGap  = 10
)

// leftLabel is a label with each line starting at left, for an outline, where centered lines
// would look ragged.
func leftLabel(label text.Label, left, cy float64, icons *text.Icons) string {
	if len(label.Lines) < 2 {
		return svg.LabelSVG(label, left+float64(label.Width/2), cy, ` class="pele-label"`, icons)
	}
	out := ""
	top := cy - float64(label.Height/2)
	for k := range label.Lines {
		out += svg.LabelSVG(text.LabelLine(label, k), left+float64(label.Widths[k]/2), top+text.LineMiddle(label, k), ` class="pele-label"`, icons)
	}
	return out
}

func renderMindmap(model *mindmapDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered { return draw(model, config, rc, down) })
}

// draw's down draws the map as an outline: the root at the top, and each level set in under
// its parent. It is far narrower than branches spreading to both sides, and as tall as it
// needs to be.
func draw(model *mindmapDb, config *jsobj.Object, rc *core.Context, down bool) core.Rendered {
	options := rc.Options
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	measurer := rc.Measurer()
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	icons := rc.Icons
	wrap := 200.0
	if section, ok := config.Get("mindmap"); ok {
		if obj, isObj := section.(*jsobj.Object); isObj {
			if v, has := obj.Get("maxNodeWidth"); has {
				if maxNodeWidth, isNum := v.(float64); isNum && maxNodeWidth > 0 {
					wrap = min(maxNodeWidth, 2000)
				}
			}
		}
	}
	glyph := float64(size * 1.25)

	nodes := model.nodes
	n := len(nodes)
	parent := make([]int, n)
	depth := make([]int, n)
	for i := range n {
		for _, child := range nodes[i].children {
			parent[child.id] = i
			depth[child.id] = depth[i] + 1
		}
	}
	// An outline has the width the host gives it, less what its level is set in.
	room := math.Inf(1)
	if down && options.MaxWidth != nil && *options.MaxWidth > 0 {
		room = *options.MaxWidth - float64(2*pad)
	}
	labels := make([]text.Label, n)
	shapes := make([]string, n)
	w := make([]float64, n)
	h := make([]float64, n)
	dy := make([]float64, n)
	// anchor is where links meet the node, measured down from its top: its middle.
	anchor := make([]float64, n)
	// drawn is whether the host has the node's icon. One it does not have takes no room.
	drawn := make([]bool, n)

	for i := range n {
		node := nodes[i]
		drawn[i] = node.icon != nil && icons.Markup(*node.icon) != ""
		iconRoom := 0.0
		if drawn[i] {
			iconRoom = glyph + iconGap
		}
		fit := min(wrap, max(72, room-float64(depth[i]*indent)-2*textPad-iconRoom))
		label := text.Layout(node.descr, true, measurer, size, fit, 0, icons)
		labels[i] = label
		tw := label.Width
		if drawn[i] {
			extra := glyph
			if label.Width > 0 {
				extra += iconGap
			}
			tw += extra
		}
		th := max(label.Height, jsmath.Round(size*1.5))
		shape := ""
		if node.typ >= 0 && node.typ < len(shapeNames) {
			shape = shapeNames[node.typ]
		}
		if shape == "" && i == 0 {
			shape = rootShape
		}
		// In an outline the root heads a column, and the trunk drops from under its left end. A circle
		// or a cloud would stand wide of that, so the root is a pill there unless it is a box already.
		if down && i == 0 && shape != "rect" && shape != "rounded" {
			shape = rootShape
		}
		shapes[i] = shape
		if shape == "" {
			// A node with no shape of its own is plain text when it ends a branch, and text in a
			// tinted pill when the branch goes on from it, which sets the levels apart.
			pill := len(node.children) > 0
			if pill {
				w[i] = tw + 2*pillPad
				h[i] = th + 6
			} else {
				w[i] = tw + 2*textPad
				h[i] = th
			}
			anchor[i] = h[i] / 2
		} else {
			s := svg.ShapeSizeOf(shape, tw, th)
			w[i] = s.W
			h[i] = s.H
			dy[i] = s.DY
			anchor[i] = s.H / 2
		}
	}

	// Each subtree takes a band of height extent. rel is a subtree's offset among its siblings,
	// own the node's offset inside its band, and kids the offset of its children's bands.
	extent := make([]float64, n)
	rel := make([]float64, n)
	own := make([]float64, n)
	kids := make([]float64, n)
	stacked := 0.0

	// stack stacks subtrees top to bottom and returns the height at which their parent's anchor
	// belongs.
	stack := func(list []*mindmapNode, from, to int, gap float64) float64 {
		y := 0.0
		last := 0.0
		for k := from; k < to; k++ {
			c := list[k].id
			rel[c] = y
			grouped := len(list[k].children) > 0 || (k+1 < to && len(list[k+1].children) > 0)
			last = gap
			if grouped {
				last = max(gap, groupGap)
			}
			y += extent[c] + last
		}
		stacked = y - last
		a := list[from].id
		b := list[to-1].id
		return float64((rel[a] + own[a] + anchor[a] + rel[b] + own[b] + anchor[b]) / 2)
	}

	// Nodes are in depth-first order, so going backwards visits children before their parent.
	for i := n - 1; i > 0; i-- {
		children := nodes[i].children
		if len(children) == 0 {
			extent[i] = h[i]
			continue
		}
		top := stack(children, 0, len(children), gapY) - anchor[i]
		low := min(0, top)
		extent[i] = max(stacked, top+h[i]) - low
		own[i] = top - low
		kids[i] = -low
	}

	x := make([]float64, n)
	y := make([]float64, n)
	band := make([]float64, n)
	side := make([]float64, n)
	// trunk is, running down, how far the trunk under each node has been drawn.
	trunk := make([]float64, n)

	if down {
		// Nodes are in depth-first order, which is the order of an outline's rows.
		row := 0.0
		for i := range n {
			if i > 0 && parent[i] == 0 {
				row += rowGap
			}
			x[i] = float64(depth[i]*indent) + float64(w[i]/2)
			y[i] = row
			row += h[i] + rowGap
		}
	} else if n > 0 {
		// The first branches go right and the rest left, split where the two sides are closest in height.
		branches := nodes[0].children
		count := len(branches)
		total := 0.0
		for _, branch := range branches {
			total += extent[branch.id]
		}
		split := count
		best := math.Inf(1)
		right := 0.0
		for k := 1; k < count; k++ {
			right += extent[branches[k-1].id]
			diff := math.Abs(float64(2*right) - total + float64((2*k-count)*branchGapY))
			if diff < best {
				best = diff
				split = k
			}
		}
		for from, to, dir := 0, split, 1.0; from < count; from, to, dir = to, count, -1 {
			middle := stack(branches, from, to, branchGapY)
			for k := from; k < to; k++ {
				c := branches[k].id
				band[c] = rel[c] - middle
				side[c] = dir
			}
		}
		y[0] = -h[0] / 2
		for i := range n {
			if i > 0 {
				y[i] = band[i] + own[i]
			}
			for _, child := range nodes[i].children {
				c := child.id
				gap := float64(branchGapX)
				if i > 0 {
					side[c] = side[i]
					band[c] = band[i] + kids[i] + rel[c]
					gap = gapX
				}
				x[c] = x[i] + float64(side[c]*(float64(w[i]/2)+gap+float64(w[c]/2)))
			}
		}
	}

	minX, maxX, minY, maxY := 0.0, 0.0, 0.0, 0.0
	var out, edges, body strings.Builder
	color := ""
	flush := func(section int) {
		if body.Len() == 0 {
			return
		}
		out.WriteString(`<g class="pele-branch pele-section-` + strconv.Itoa(section) + `">` +
			`<g class="pele-edges" fill="none" stroke="` + color + `" stroke-width="1.5">` + edges.String() + `</g>` + body.String() + `</g>`)
		edges.Reset()
		body.Reset()
	}

	// The root is drawn last, on top of the links that leave it.
	for k := 1; k <= n; k++ {
		i := k % n
		node := nodes[i]
		shape := shapes[i]
		label := labels[i]
		cx := x[i]
		top := y[i]
		minX = min(minX, cx-float64(w[i]/2))
		maxX = max(maxX, cx+float64(w[i]/2))
		minY = min(minY, top)
		maxY = max(maxY, top+h[i])

		if i == 0 || parent[i] == 0 {
			flush(nodes[k-1].section)
		}
		if i > 0 && down {
			if parent[i] == 0 {
				color = svg.SeriesColor(node.section)
			}
			p := parent[i]
			// The link drops from under its parent and turns in to the node's left. It starts where
			// the link to the sibling above turned in, so each stretch of a shared trunk has one color.
			// Under the root's rounded end, the trunk starts where that end has come down to the bottom.
			st := float64(stem)
			if p == 0 {
				st = min(max(stem, h[0]/2), indent-bendMax)
			}
			x1 := x[p] - float64(w[p]/2) + min(st, w[p]/2)
			y1 := trunk[p]
			if !(y1 != 0 && y1 == y1) {
				y1 = y[p] + h[p]
			}
			left := cx - float64(w[i]/2)
			x2 := left
			if shape != "" {
				x2 += svg.ShapeInset(shape, w[i], h[i], 3, 0)
			}
			y2 := top + anchor[i]
			bend := max(0, min(bendMax, x2-x1, y2-y1))
			// The next link carries on from where this one leaves the trunk, so the trunk has no gap.
			trunk[p] = y2 - bend
			edges.WriteString(`<path class="pele-edge" d="M` + svg.Num(x1) + "," + svg.Num(y1) + "V" + svg.Num(y2-bend) + "Q" + svg.Num(x1) + "," + svg.Num(y2) + " " +
				svg.Num(x1+bend) + "," + svg.Num(y2) + "H" + svg.Num(x2) + `"/>`)
		} else if i > 0 {
			if parent[i] == 0 {
				color = svg.SeriesColor(node.section)
			}
			p := parent[i]
			dir := side[i]
			from := 3
			if dir > 0 {
				from = 1
			}
			inset := 0.0
			if shapes[p] != "" {
				inset = svg.ShapeInset(shapes[p], w[p], h[p], from, 0)
			}
			x1 := x[p] + float64(dir*(float64(w[p]/2)-inset))
			y1 := y[p] + anchor[p]
			near := cx - float64(float64(dir*w[i])/2)
			x2 := near
			if shape != "" {
				x2 += float64(dir * svg.ShapeInset(shape, w[i], h[i], 4-from, 0))
			}
			y2 := top + anchor[i]
			mid := svg.Num((x1 + x2) / 2)
			edges.WriteString(`<path class="pele-edge" d="M` + svg.Num(x1) + "," + svg.Num(y1) + "C" + mid + "," + svg.Num(y1) + " " + mid + "," + svg.Num(y2) + " " +
				svg.Num(x2) + "," + svg.Num(y2) + `"/>`)
		}

		pill := shape == "" && len(node.children) > 0
		// Every node off the root is filled with a tint of its branch's color and outlined in it,
		// whether its shape was given in the source or is the pill of a node that branches.
		// The root is neutral.
		tinted := ` fill="` + color + `" fill-opacity="` + jsnum.Format(tint) + `" stroke="` + color + `"`
		inner := ""
		if shape != "" {
			if i > 0 {
				inner = svg.DrawShape(shape, w[i], h[i], tinted, ` stroke="`+color+`"`)
			} else {
				inner = svg.DrawShape(shape, w[i], h[i], ` fill="var(--_s)" stroke="var(--_b)"`, ` stroke="var(--_b)"`)
			}
		} else if pill {
			inner = `<rect x="` + svg.Num(-w[i]/2) + `" y="` + svg.Num(-h[i]/2) + `" width="` + svg.Num(w[i]) + `" height="` + svg.Num(h[i]) +
				`" rx="` + svg.Num(h[i]/2) + `"` + tinted + `/>`
		}
		shift := 0.0
		if node.icon != nil && drawn[i] {
			g := glyph
			if label.Width > 0 {
				g += iconGap
			}
			shift = float64(g / 2)
			inner += common.IconSVG(*node.icon, -shift-float64(label.Width/2), dy[i]-float64(glyph/2), glyph, icons)
		}
		kind := shape
		if kind == "" {
			kind = "text"
			if pill {
				kind = "branch"
			}
		}
		root := ""
		if i == 0 {
			root = " pele-root"
		}
		group := `<g class="pele-node pele-shape-` + kind + root + svg.ClassNames(node.class) + `" data-id="` + svg.EscText(node.nodeID) +
			`" transform="translate(` + svg.Num(cx) + "," + svg.Num(top+float64(h[i]/2)) + `)">` + inner
		if down && shape == "" {
			group += leftLabel(label, shift-float64(label.Width/2), dy[i], icons)
		} else {
			group += svg.LabelSVG(label, shift, dy[i], ` class="pele-label"`, icons)
		}
		group += "</g>"
		if i > 0 {
			body.WriteString(group)
		} else {
			out.WriteString(group)
		}
	}

	titleText := ""
	if model.title != nil {
		titleText = *model.title
	}
	title := svg.TitleLabel(titleText, measurer, size, common.TitleRoom(options), icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 16
	}
	width := max(maxX-minX, title.Width)
	totalWidth := math.Ceil(width + float64(2*pad))
	totalHeight := math.Ceil(maxY - minY + titleHeight + float64(2*pad))
	content := svg.LabelSVG(title, totalWidth/2, pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil) +
		`<g transform="translate(` + svg.Num(pad-minX+float64((width-maxX+minX)/2)) + "," + svg.Num(pad+titleHeight-minY) + `)">` + out.String() + `</g>`

	doc := svg.Document("mindmap", totalWidth, totalHeight, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{}, content)
	return core.Rendered{SVG: doc, Width: totalWidth, Height: totalHeight, Links: []core.Link{}}
}
