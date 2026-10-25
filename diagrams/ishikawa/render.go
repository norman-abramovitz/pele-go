package ishikawa

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

// slant is how far a bone moves toward the tail for each unit it rises from the spine.
const slant = 0.5

var length = jsmath.Hypot(slant, 1)

const (
	firstRow = 14
	rowGap   = 6
	boneEnd  = 8
	twigStep = 22
	stub     = 18
	textGap  = 5
	pairGap  = 26
	headGap  = 34
	tail     = 28
	boxPadX  = 10
	boxPadY  = 5
)

// bone is a cause below a category. Bones at even depths run level, from the slanted bone
// above them toward the tail; bones at odd depths slant away from the spine, from the level
// bone above them.
type bone struct {
	label  text.Label
	depth  int
	parent int
	// slot is its place among its siblings, counted from the spine outward.
	slot int
	kids int
	// rise is how far its row is from the spine.
	rise float64
	// at is where it meets its parent, measured level from the category's bone: 0 on it,
	// negative toward the tail.
	at float64
	// textEnd is where its text ends.
	textEnd float64
}

type category struct {
	node       *ishikawaNode
	label      text.Label
	boxW, boxH float64
	bones      []*bone
	// rise is the length of its bone, as distance from the spine.
	rise float64
	// reach is how far its causes reach toward the tail, measured level from its bone.
	reach float64
}

type arrangeFrame struct {
	node   *ishikawaNode
	depth  int
	parent int
	slot   int
	// bone is set when the bone was made and only its row is still to come.
	bone int
}

// arrange gives every cause of a category a row of its own, so that no two texts can meet,
// and measures how far the whole of it reaches. A level bone takes the row nearest the spine
// and its twigs the rows beyond; a slanted twig takes the row beyond everything that hangs
// off it. The outline may be nested as deep as its indentation goes, so the walk keeps a
// stack of its own.
func arrange(node *ishikawaNode, up bool, measurer text.Measurer, size, small float64, icons *text.Icons) *category {
	label := text.Layout(node.text, false, measurer, size, 180, 0, icons)
	lineHeight := jsmath.Round(float64(small * 1.5))
	var bones []*bone
	var rows []int
	var stack []arrangeFrame
	// Above the spine the first cause goes farthest out, so that the causes read from the top
	// down.
	push := func(children []*ishikawaNode, depth, parent int) {
		n := len(children)
		for slot := n - 1; slot >= 0; slot-- {
			child := children[slot]
			if up {
				child = children[n-1-slot]
			}
			stack = append(stack, arrangeFrame{node: child, depth: depth, parent: parent, slot: slot, bone: -1})
		}
	}
	push(node.children, 2, -1)
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if f.bone != -1 {
			rows = append(rows, f.bone)
			continue
		}
		index := len(bones)
		bones = append(bones, &bone{
			label:  text.Layout(f.node.text, false, measurer, small, 200, 0, icons),
			depth:  f.depth,
			parent: f.parent,
			slot:   f.slot,
			kids:   len(f.node.children),
		})
		if f.depth%2 == 0 {
			rows = append(rows, index)
		} else {
			f.bone = index
			stack = append(stack, f)
		}
		push(f.node.children, f.depth+1, index)
	}

	rise := float64(firstRow)
	for _, index := range rows {
		height := max(bones[index].label.Height, lineHeight) + rowGap
		bones[index].rise = rise + float64(height/2)
		rise += height
	}
	boxW := label.Width + 2*boxPadX
	reach := float64(boxW / 2)
	for _, b := range bones {
		if b.depth%2 == 0 {
			if b.parent != -1 {
				b.at = bones[b.parent].at
			}
			steps := float64(stub)
			if b.kids > 0 {
				steps = float64((b.kids + 1) * twigStep)
			}
			b.textEnd = b.at - steps - textGap
		} else {
			// Twigs stand along their level bone, the one nearest the spine nearest the tail.
			parent := bones[b.parent]
			b.at = parent.at - float64((parent.kids-b.slot)*twigStep)
			b.textEnd = b.at - textGap
		}
		reach = max(reach, b.label.Width-b.textEnd)
	}
	return &category{node: node, label: label, boxW: boxW, boxH: label.Height + 2*boxPadY, bones: bones, rise: rise + boneEnd, reach: reach}
}

func renderIshikawa(m *ishikawaDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered { return draw(m, config, rc, down) })
}

type settings struct {
	size, small, pad float64
	measurer         text.Measurer
}

func settingsOf(opts core.Options, rc *core.Context) settings {
	size := 16.0
	if opts.FontSize != nil {
		size = *opts.FontSize
	}
	pad := 8.0
	if opts.Padding != nil {
		pad = *opts.Padding
	}
	return settings{size: size, small: jsmath.Round(float64(size * 0.875)), pad: pad, measurer: rc.Measurer()}
}

func document(width, height float64, s settings, rc *core.Context, content string) string {
	opts := rc.Options
	return svg.Document("ishikawa", width, height, s.size, svg.DocumentOptions{
		IDPrefix:       opts.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     opts.FontFamily,
		FontFamilyMono: opts.FontFamilyMono,
	}, svg.Accessible{}, content)
}

func titleText(m *ishikawaDb) string {
	if m.title == nil {
		return ""
	}
	return *m.title
}

func draw(m *ishikawaDb, _ *jsobj.Object, rc *core.Context, down bool) core.Rendered {
	s := settingsOf(rc.Options, rc)
	size, small, pad, measurer := s.size, s.small, s.pad, s.measurer
	if down {
		return verticalCauses(m, s, rc)
	}
	root := m.root
	title := text.Layout(titleText(m), false, measurer, size, 4000, text.Bold, rc.Icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}

	body := ""
	minX, maxX, minY, maxY := 0.0, 0.0, 0.0, 0.0
	if root != nil {
		head := text.Layout(root.text, false, measurer, size, 200, text.Bold, rc.Icons)
		headH := max(head.Height+24, 48)
		headW := head.Width + 24 + float64(headH/2)
		categories := make([]*category, len(root.children))
		for i, node := range root.children {
			categories[i] = arrange(node, i%2 == 0, measurer, size, small, rc.Icons)
		}
		// Even a category without causes has a bone long enough to carry its name clear of the
		// head.
		for _, c := range categories {
			c.rise = max(c.rise, float64(headH/2)+14)
		}

		// Categories meet the spine in pairs, one above and one below, each pair as far toward
		// the tail from the one before as the wider of the two needs.
		var xs []float64
		x := float64(-headGap)
		for i := 0; i < len(categories); i += 2 {
			if i > 0 {
				room := 0.0
				for side := range 2 {
					before := categories[i-2+side]
					nextW := 0.0
					if i+side < len(categories) {
						nextW = float64(categories[i+side].boxW / 2)
					}
					room = max(room, before.reach+nextW)
				}
				x -= room + pairGap
			}
			xs = append(xs, x, x)
		}
		tailX := float64(-headGap)
		if len(categories) > 0 {
			tailX = x - tail
		}

		maxX = headW
		minX = tailX
		minY = -headH / 2
		maxY = headH / 2
		var groups strings.Builder
		for i, c := range categories {
			sign := 1.0
			if i%2 == 0 {
				sign = -1
			}
			ax := xs[i]
			px := func(at, rise float64) float64 { return ax + at - float64(rise*slant) }
			endX := px(0, c.rise)
			endY := float64(sign * c.rise)
			// The bone stops short of the spine to leave its arrowhead a clean point.
			const trim = svg.Arrow - 1
			d := "M" + svg.Num(ax-float64((slant/length)*trim)) + "," + svg.Num((sign*trim)/length) + "L" + svg.Num(endX) + "," + svg.Num(endY)
			var texts strings.Builder
			for _, b := range c.bones {
				y := float64(sign * b.rise)
				if b.depth%2 == 0 {
					d += "M" + svg.Num(px(b.at, b.rise)) + "," + svg.Num(y) + "H" + svg.Num(px(b.textEnd+textGap, b.rise))
				} else {
					from := c.bones[b.parent].rise
					d += "M" + svg.Num(px(b.at, from)) + "," + svg.Num(float64(sign*from)) + "L" + svg.Num(px(b.at, b.rise)) + "," + svg.Num(y)
				}
				right := px(b.textEnd, b.rise)
				attrs := ""
				if b.depth > 2 {
					attrs = ` fill="var(--_m)"`
				}
				texts.WriteString(svg.LabelSVG(b.label, right-float64(b.label.Width/2), y, attrs, rc.Icons))
				minX = min(minX, right-b.label.Width)
			}
			boxY := endY + float64(sign*float64(c.boxH/2))
			minX = min(minX, endX-float64(c.boxW/2))
			maxX = max(maxX, endX+float64(c.boxW/2))
			minY = min(minY, boxY-float64(c.boxH/2))
			maxY = max(maxY, boxY+float64(c.boxH/2))
			groups.WriteString(`<g class="pele-ishikawa-category" data-id="` + svg.EscText(c.node.text) + `">` +
				`<path class="pele-ishikawa-bones" d="` + d + `" fill="none" stroke="var(--_l)"/>` +
				svg.Marker("arrow_point", ax, 0, slant/length, -sign/length, "var(--_l)") +
				`<rect x="` + svg.Num(endX-float64(c.boxW/2)) + `" y="` + svg.Num(boxY-float64(c.boxH/2)) + `" width="` + svg.Num(c.boxW) + `" height="` +
				svg.Num(c.boxH) + `" fill="var(--_s)" stroke="var(--_b)"` + svg.RoundedStyle + `/>` +
				svg.LabelSVG(c.label, endX, boxY, ` class="pele-label"`, rc.Icons))
			if texts.Len() > 0 {
				groups.WriteString(`<g class="pele-ishikawa-causes" font-size="` + jsnum.Format(small) + `">` + texts.String() + `</g>`)
			}
			groups.WriteString(`</g>`)
		}

		r := float64(headH / 2)
		body = `<path class="pele-ishikawa-spine" d="M` + svg.Num(tailX) + `,0H` + svg.Num(-(svg.Arrow - 1)) + `" fill="none" stroke="var(--_l)"/>` +
			svg.Marker("arrow_point", 0, 0, 1, 0, "var(--_l)") +
			groups.String() +
			`<g class="pele-ishikawa-head" data-id="` + svg.EscText(root.text) + `">` +
			`<path d="M0,` + svg.Num(-r) + `H` + svg.Num(headW-r) + `A` + svg.Num(r) + `,` + svg.Num(r) + ` 0 0 1 ` + svg.Num(headW-r) + `,` + svg.Num(r) +
			`H0Z" fill="var(--_s)" stroke="var(--_b)" stroke-linejoin="round"/>` +
			svg.LabelSVG(head, 12+float64(head.Width/2), 0, ` class="pele-label" font-weight="var(--_hw)"`, rc.Icons) +
			`</g>`
	}

	inner := max(maxX-minX, title.Width)
	width := math.Ceil(inner + float64(2*pad))
	height := math.Ceil(maxY - minY + titleHeight + float64(2*pad))
	ox := pad + float64((inner-(maxX-minX))/2) - minX
	oy := pad + titleHeight - minY
	content := svg.LabelSVG(title, float64(width/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, rc.Icons)
	if body != "" {
		content += `<g transform="translate(` + svg.Num(ox) + `,` + svg.Num(oy) + `)">` + body + `</g>`
	}
	return core.Rendered{SVG: document(width, height, s, rc, content), Width: width, Height: height, Links: []core.Link{}}
}

type entry struct {
	node          *ishikawaNode
	depth, parent int
	x, y, w, h    float64
	label         text.Label
}

func verticalCauses(m *ishikawaDb, s settings, rc *core.Context) core.Rendered {
	size, small, pad, measurer := s.size, s.small, s.pad, s.measurer
	maxWidth := 320.0
	if rc.Options.MaxWidth != nil {
		maxWidth = *rc.Options.MaxWidth
	}
	room := max(140, maxWidth-float64(2*pad))
	title := text.Layout(titleText(m), false, measurer, size, common.TitleRoom(rc.Options), text.Bold, rc.Icons)
	var order []*entry
	type pendingEntry struct {
		node          *ishikawaNode
		depth, parent int
	}
	var pending []pendingEntry
	if m.root != nil {
		pending = append(pending, pendingEntry{node: m.root, parent: -1})
	}
	depth := 0
	for len(pending) > 0 {
		e := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		index := len(order)
		order = append(order, &entry{node: e.node, depth: e.depth, parent: e.parent})
		depth = max(depth, e.depth)
		for i := len(e.node.children) - 1; i >= 0; i-- {
			pending = append(pending, pendingEntry{node: e.node.children[i], depth: e.depth + 1, parent: index})
		}
	}
	indent := min(20, max(2, (room-120)/float64(max(1, depth))))
	titleRoom := 0.0
	if jsobj.Truthy(title.Height) {
		titleRoom = title.Height + 12
	}
	y := pad + titleRoom
	width := title.Width
	var nodes, edges strings.Builder
	for _, e := range order {
		e.x = pad + float64(float64(e.depth)*indent)
		labelRoom := max(60, room-float64(float64(e.depth)*indent)-20)
		labelSize, style := small, text.Style(0)
		if e.depth < 2 {
			labelSize, style = size, text.Bold
		}
		e.label = common.BreakLabel(text.Layout(e.node.text, false, measurer, labelSize, labelRoom, style, rc.Icons), measurer, labelRoom)
		e.w = e.label.Width + 20
		e.h = e.label.Height + 12
		e.y = y
		y += e.h + 12
		width = max(width, e.x-pad+e.w)
		class := "pele-ishikawa-cause"
		switch e.depth {
		case 0:
			class = "pele-ishikawa-head"
		case 1:
			class = "pele-ishikawa-category"
		}
		nodes.WriteString(`<g class="pele-node ` + class + `" data-id="` + svg.EscText(e.node.text) + `">`)
		if e.depth < 2 {
			rx := "4"
			if e.depth == 0 {
				rx = "12"
			}
			nodes.WriteString(`<rect x="` + svg.Num(e.x) + `" y="` + svg.Num(e.y) + `" width="` + svg.Num(e.w) + `" height="` + svg.Num(e.h) +
				`" rx="` + rx + `" fill="var(--_s)" stroke="var(--_b)"/>`)
		}
		attrs := ` class="pele-label"`
		if e.depth < 2 {
			attrs += ` font-weight="var(--_hw)"`
		}
		if e.depth > 1 {
			attrs += ` font-size="` + jsnum.Format(small) + `"`
		}
		nodes.WriteString(svg.LabelSVG(e.label, e.x+float64(e.w/2), e.y+float64(e.h/2), attrs, rc.Icons) + `</g>`)
		if e.parent >= 0 {
			parent := order[e.parent]
			gx := parent.x + float64(indent/2)
			edges.WriteString(`<path class="pele-edge" d="M` + svg.Num(gx) + `,` + svg.Num(parent.y+parent.h) + `V` + svg.Num(e.y+float64(e.h/2)) + `H` +
				svg.Num(e.x) + `" fill="none" stroke="var(--_b)"/>`)
		}
	}
	totalWidth := math.Ceil(width + float64(2*pad))
	height := math.Ceil(max(pad+title.Height, y-12) + pad)
	content := svg.LabelSVG(title, float64(totalWidth/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, rc.Icons) +
		edges.String() + nodes.String()
	return core.Rendered{SVG: document(totalWidth, height, s, rc, content), Width: totalWidth, Height: height, Links: []core.Link{}}
}
