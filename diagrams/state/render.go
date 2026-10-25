package state

import (
	"math"
	"slices"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/layout"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

const (
	nodeSep   = 40
	edgeSep   = 16
	rankSep   = 44
	groupPad  = 16
	regionPad = 10
	padX      = 16
	padY      = 10
	loopReach = 26
	fold      = 8
	noteWrap  = 320
	corner    = 10
	surface   = ` fill="var(--_s)" stroke="var(--_b)"`
	rule      = ` fill="none" stroke="var(--_b)"`
)

type kind int

const (
	kState kind = iota
	kDescribed
	kStart
	kEnd
	kChoice
	kFork
	kJoin
	kNote
	kComposite
	kRegion
	kHidden
)

var kinds = map[string]kind{
	"rectWithTitle": kDescribed,
	"stateStart":    kStart,
	"stateEnd":      kEnd,
	"choice":        kChoice,
	"fork":          kFork,
	"join":          kJoin,
	"note":          kNote,
	"divider":       kHidden,
}

var kindClasses = [...]string{
	"pele-node pele-state",
	"pele-node pele-state",
	"pele-node pele-state-start",
	"pele-node pele-state-end",
	"pele-node pele-state-choice",
	"pele-node pele-state-fork",
	"pele-node pele-state-join",
	"pele-note",
	"pele-cluster pele-state-composite",
}

type view struct {
	// nil for the unseen helpers the layout needs.
	node   *stateNode
	kind   kind
	style  *svg.ResolvedStyle
	label  text.Label
	lines  []text.Label
	w, h   float64
	titleH float64
}

func vertical(dir layout.Dir) bool {
	return dir == layout.TB || dir == layout.BT
}

func numberOption(config *jsobj.Object, key string, fallback float64) float64 {
	section, _ := config.Get("state")
	obj, ok := section.(*jsobj.Object)
	if !ok {
		return fallback
	}
	value, _ := obj.Get(key)
	if v, ok := value.(float64); ok && v > 0 {
		return min(v, 2000)
	}
	return fallback
}

// unquote strips the quotes Mermaid keeps on a click statement's strings, as it does when it draws.
func unquote(s string) string {
	from, to := 0, len(s)
	for from < to && s[from] == '"' {
		from++
	}
	for to > from && s[to-1] == '"' {
		to--
	}
	return s[from:to]
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

// jsSign is Math.sign, which keeps -0 and NaN.
func jsSign(v float64) float64 {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return v
}

func rect(x, y, w, h float64, rest string) string {
	return `<rect x="` + svg.Num(x) + `" y="` + svg.Num(y) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) + `"` + rest + `/>`
}

func group(dir layout.Dir, padX, padTop, padBottom float64) layout.CNode {
	c := layout.NewCNode(0, 0, -1)
	c.IsGroup = true
	c.Dir = dir
	c.PadX = padX
	c.PadTop = padTop
	c.PadBottom = padBottom
	return c
}

// stripMermaidClasses is s.replace(/(?:^| )statediagram-(?:state|cluster-alt|cluster|note)(?= |$)/g, ”).
func stripMermaidClasses(s string) string {
	const prefix = "statediagram-"
	match := func(i int) int {
		starts := make([]int, 0, 2)
		if i == 0 {
			starts = append(starts, 0)
		}
		if i < len(s) && s[i] == ' ' {
			starts = append(starts, i+1)
		}
		for _, j := range starts {
			if !strings.HasPrefix(s[j:], prefix) {
				continue
			}
			j += len(prefix)
			for _, alt := range [...]string{"state", "cluster-alt", "cluster", "note"} {
				e := j + len(alt)
				if strings.HasPrefix(s[j:], alt) && (e == len(s) || s[e] == ' ') {
					return e
				}
			}
		}
		return -1
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if e := match(i); e != -1 {
			i = e
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func renderState(db *stateDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	if !common.RunsAcross(db.direction()) {
		return draw(db, config, rc, false)
	}
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered { return draw(db, config, rc, down) })
}

// draw lays the diagram out and draws it; turned draws a diagram that runs across as one that
// runs down.
func draw(db *stateDb, config *jsobj.Object, rc *core.Context, turned bool) core.Rendered {
	graph := db.graph
	// State diagrams follow the flowchart curve.
	var curve string
	if flow, ok := config.Get("flowchart"); ok {
		if obj, ok := flow.(*jsobj.Object); ok {
			if v, ok := obj.Get("curve"); ok {
				curve, _ = v.(string)
			}
		}
	}
	options := rc.Options
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	small := jsmath.Round(size * 0.875)
	measurer := rc.Measurer()
	wrapWidth := numberOption(config, "wrappingWidth", 200)
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	icons := rc.Icons
	rootDir := layout.TB
	if !turned {
		rootDir = layout.Direction(db.direction())
	}
	layoutText := func(raw string, fontSize, wrap float64, style text.Style) text.Label {
		return text.Layout(raw, true, measurer, fontSize, wrap, style, icons)
	}

	index := map[string]int{}
	var views []*view
	var cnodes []layout.CNode
	add := func(node *stateNode, k kind, c *layout.CNode) int {
		views = append(views, &view{node: node, kind: k})
		if c != nil {
			cnodes = append(cnodes, *c)
		}
		return len(views) - 1
	}
	for _, node := range graph.nodes {
		if node.shape == "noteGroup" {
			continue
		}
		var k kind
		switch {
		case node.isGroup && node.shape == "divider":
			k = kRegion
		case node.isGroup:
			k = kComposite
		default:
			var ok bool
			if k, ok = kinds[node.shape]; !ok {
				k = kState
			}
		}
		index[node.id] = add(node, k, nil)
	}
	states := len(views)

	logical := make([]int, states)
	for i := 0; i < states; i++ {
		logical[i] = -1
		if parentID := views[i].node.parentId; parentID != nil {
			if parent, ok := index[*parentID]; ok && parent != i && views[parent].node.isGroup {
				logical[i] = parent
			}
		}
	}
	// Composites that are redeclared inside each other form a ring with no way in from the root.
	// One member of each ring moves to the root, which leaves a tree.
	{
		kids := make([][]int, states)
		var queue []int
		placed := make([]bool, states)
		for i := 0; i < states; i++ {
			if logical[i] == -1 {
				placed[i] = true
				queue = append(queue, i)
			} else {
				kids[logical[i]] = append(kids[logical[i]], i)
			}
		}
		done := 0
		settle := func() {
			for ; done < len(queue); done++ {
				for _, kid := range kids[queue[done]] {
					if placed[kid] {
						continue
					}
					placed[kid] = true
					queue = append(queue, kid)
				}
			}
		}
		settle()
		seen := make([]int, states)
		for i := range seen {
			seen[i] = -1
		}
		for i := 0; i < states; i++ {
			if placed[i] {
				continue
			}
			ring := i
			for seen[ring] != i {
				seen[ring] = i
				ring = logical[ring]
			}
			logical[ring] = -1
			placed[ring] = true
			queue = append(queue, ring)
			settle()
		}
	}
	owners := make([]int, states)
	for i := 0; i < states; i++ {
		owners[i] = -1
		noteFor := views[i].node.noteFor
		if noteFor == nil {
			continue
		}
		owner, ok := index[*noteFor]
		if !ok {
			continue
		}
		owners[i] = owner
		logical[i] = logical[owner]
	}

	// As in Mermaid, a composite that sets no direction runs top to bottom whatever surrounds it.
	innerDir := func(composite int) layout.Dir { return layout.Direction(views[composite].node.dir) }
	// Whether the flow around a state runs top to bottom or bottom to top.
	across := func(i int) bool {
		if logical[i] == -1 {
			return vertical(rootDir)
		}
		return vertical(innerDir(logical[i]))
	}

	degree := make([]int, states)
	noted := make([]bool, states)
	// When each state first takes part in a transition. Layout follows this order rather than
	// the order of declaration, so a state declared up front does not become the top of the flow.
	appear := make([]float64, states)
	for i := range appear {
		appear[i] = math.Inf(1)
	}
	appeared := 0.0
	counts := make([]int, 2*states)
	for _, edge := range graph.edges {
		src, ok1 := index[edge.start]
		dst, ok2 := index[edge.end]
		if !ok1 || !ok2 {
			continue
		}
		if edge.dashed {
			if views[src].kind == kNote {
				noted[dst] = true
			} else {
				noted[src] = true
			}
			continue
		}
		counts[2*src]++
		degree[src] = max(degree[src], counts[2*src])
		counts[2*dst+1]++
		degree[dst] = max(degree[dst], counts[2*dst+1])
		if math.IsInf(appear[src], 1) {
			appear[src] = appeared
			appeared++
		}
		if math.IsInf(appear[dst], 1) {
			appear[dst] = appeared
			appeared++
		}
	}

	for i := 0; i < states; i++ {
		v := views[i]
		node := v.node
		declarations := node.cssCompiledStyles
		if len(node.cssStyles) > 0 {
			declarations = append(slices.Clip(node.cssCompiledStyles), node.cssStyles...)
		}
		style := svg.ResolveStyle(declarations)
		var textStyle text.Style
		if style.Bold {
			textStyle |= text.Bold
		}
		if style.Italic {
			textStyle |= text.Italic
		}
		fontSize := size
		if style.FontSize != 0 {
			fontSize = style.FontSize
		}
		v.style = &style
		var c *layout.CNode
		switch v.kind {
		case kComposite:
			v.label = layoutText(node.label, fontSize, 4000, textStyle)
			if v.label.Height > 0 {
				v.titleH = v.label.Height + 12
			}
			g := group(innerDir(i), groupPad, v.titleH+groupPad, groupPad)
			g.MinW = v.label.Width + 2*groupPad
			c = &g
		case kRegion:
			g := group(innerDir(i), regionPad, regionPad, regionPad)
			c = &g
		case kStart:
			v.w, v.h = 14, 14
		case kEnd:
			v.w, v.h = 18, 18
		case kChoice:
			v.w, v.h = 30, 30
		case kFork, kJoin:
			length := clamp(float64(40*degree[i]+16), 72, 320)
			if across(i) {
				v.w, v.h = length, 8
			} else {
				v.w, v.h = 8, length
			}
		case kHidden:
		case kNote:
			v.label = layoutText(node.label, small, max(wrapWidth, noteWrap), 0)
			v.w = v.label.Width + 24
			v.h = v.label.Height + 16
		default:
			v.label = layoutText(node.label, fontSize, wrapWidth, textStyle)
			width := v.label.Width
			height := v.label.Height + 2*padY
			if v.kind == kDescribed {
				height += 14
				for _, line := range node.description {
					if line == "" {
						line = " "
					}
					label := layoutText(line, small, wrapWidth, textStyle)
					v.lines = append(v.lines, label)
					if label.Width > width {
						width = label.Width
					}
					height += label.Height
				}
			}
			v.w = max(width+2*padX, 56)
			v.h = height
		}
		if c == nil {
			n := layout.NewCNode(v.w, v.h, -1)
			c = &n
		}
		// The transitions on one side of a state's box spread along it, clear of its corners.
		if v.kind == kState || v.kind == kDescribed {
			if across(i) {
				c.Span = v.w - 24
			} else {
				c.Span = v.h - 24
			}
		}
		cnodes = append(cnodes, *c)
	}

	var cedges []layout.CEdge
	var edgeLabels []text.Label
	var drawn []stateEdge
	// The transitions each state makes to itself: how many, their widest label, and their labels' height.
	loopCount := make([]int, states)
	loopWidth := make([]float64, states)
	loopHeight := make([]float64, states)
	// A transition between a composite and a state inside it runs between that state and a point
	// just inside the composite, and is then drawn on to the composite's border.
	anchors := map[int]int{}
	encloses := func(composite, item int) bool {
		if !cnodes[composite].IsGroup {
			return false
		}
		for p := logical[item]; p != -1; p = logical[p] {
			if p == composite {
				return true
			}
		}
		return false
	}
	anchor := func(composite int, first bool) int {
		anchors[len(cedges)] = composite
		if first {
			appear = append(appear, -1)
		} else {
			appear = append(appear, math.Inf(1))
		}
		c := layout.NewCNode(0, 0, composite)
		return add(nil, kHidden, &c)
	}
	for _, edge := range graph.edges {
		src, ok1 := index[edge.start]
		dst, ok2 := index[edge.end]
		if !ok1 || !ok2 {
			continue
		}
		label := layoutText(edge.label, small, wrapWidth, 0)
		labelW := 0.0
		if label.Width > 0 {
			labelW = label.Width + 12
		}
		switch {
		case src == dst:
			loopCount[src]++
			loopWidth[src] = max(loopWidth[src], labelW)
			loopHeight[src] += label.Height
		case encloses(src, dst):
			src = anchor(src, true)
		case encloses(dst, src):
			dst = anchor(dst, false)
		}
		drawn = append(drawn, edge)
		edgeLabels = append(edgeLabels, label)
		labelH := 0.0
		if label.Height > 0 {
			labelH = label.Height + 4
		}
		cedges = append(cedges, layout.CEdge{Src: src, Dst: dst, Minlen: 1, LabelW: labelW, LabelH: labelH, Route: []float64{}})
	}

	// A state with notes, or a composite with self-transitions, sits in an unseen group of its
	// own: the notes go beside the state, across the flow, without disturbing its transitions.
	for i := 0; i < states; i++ {
		if owners[i] != -1 {
			continue
		}
		// Loops hang off the right, or below when the flow is horizontal, with their labels beyond them.
		room := 0.0
		if loopCount[i] > 0 {
			reach := float64(loopReach + (loopCount[i]-1)*8)
			if across(i) {
				room = reach + loopWidth[i]
			} else {
				room = float64(reach*0.75) + 4 + loopHeight[i]
			}
			if item := &cnodes[i]; !item.IsGroup {
				if across(i) {
					item.W += float64(2 * room)
					item.H = max(item.H, loopHeight[i])
				} else {
					item.H += float64(2 * room)
					item.W = max(item.W, loopWidth[i])
				}
				room = 0
			}
		}
		if !noted[i] && room == 0 {
			cnodes[i].Parent = logical[i]
			continue
		}
		var c layout.CNode
		if across(i) {
			c = group(layout.LR, room, 0, 0)
		} else {
			c = group(layout.TB, 0, room, room)
		}
		c.Parent = logical[i]
		appear = append(appear, appear[i])
		parent := add(nil, kHidden, &c)
		cnodes[i].Parent = parent
	}
	for i := 0; i < states; i++ {
		if owners[i] != -1 {
			cnodes[i].Parent = cnodes[owners[i]].Parent
		}
	}

	{
		// Numbers the tree so that, at every level, initial states come first and the rest follow
		// in the order the transitions introduce them.
		members := make([][]int, len(cnodes))
		var top []int
		for i := range cnodes {
			if p := cnodes[i].Parent; p >= 0 {
				members[p] = append(members[p], i)
			} else {
				top = append(top, i)
			}
		}
		initial := func(i int) int {
			if views[i].kind == kStart {
				return 0
			}
			return 1
		}
		byFlow := func(a, b int) int {
			if d := initial(a) - initial(b); d != 0 {
				return d
			}
			if appear[a] != appear[b] {
				if appear[a] < appear[b] {
					return -1
				}
				return 1
			}
			return a - b
		}
		seq := 0.0
		slices.SortStableFunc(top, byFlow)
		slices.Reverse(top)
		pending := top
		for len(pending) > 0 {
			i := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			cnodes[i].Seq = seq
			seq++
			inside := members[i]
			slices.SortStableFunc(inside, byFlow)
			for k := len(inside) - 1; k >= 0; k-- {
				pending = append(pending, inside[k])
			}
		}
	}

	result := layout.CompoundLayout(rc.Ctx, cnodes, cedges, rootDir, layout.LayeredOptions{
		NodeSep:  numberOption(config, "nodeSpacing", nodeSep),
		EdgeSep:  edgeSep,
		RankSep:  numberOption(config, "rankSpacing", rankSep),
		PortSep:  20,
		Tracks:   curve == "step" || curve == "stepBefore" || curve == "stepAfter",
		Straight: curve == "linear",
		HeadRoom: svg.MarkerTrim("arrow_point"),
	})

	width := math.Ceil(result.Width + float64(2*pad))
	height := math.Ceil(result.Height + float64(2*pad))
	var links []core.Link

	// Route points by rounded y, to find the edges that cross a composite's title bar.
	crossings := map[float64][]float64{}
	for _, e := range cedges {
		route := e.Route
		for k := 3; k < len(route)-3; k += 3 {
			key := jsmath.Round(route[k+1])
			crossings[key] = append(crossings[key], route[k])
		}
	}

	children := make([][]int, len(cnodes))
	var order []int
	for i := range cnodes {
		if p := cnodes[i].Parent; p >= 0 {
			children[p] = append(children[p], i)
		} else {
			order = append(order, i)
		}
	}
	roots := slices.Clone(order)
	for q := 0; q < len(order); q++ {
		order = append(order, children[order[q]]...)
	}

	var clusters, dividers, nodesOut, notesOut strings.Builder
	// Draws the dashed lines between the regions among a composite's members.
	divide := func(items []int, left, top, right, bottom float64) {
		var regions []int
		for _, i := range items {
			if views[i].kind == kRegion {
				regions = append(regions, i)
			}
		}
		if len(regions) < 2 {
			return
		}
		lo := func(i int, x bool) float64 {
			if x {
				return cnodes[i].X - float64(cnodes[i].W/2)
			}
			return cnodes[i].Y - float64(cnodes[i].H/2)
		}
		hi := func(i int, x bool) float64 {
			if x {
				return cnodes[i].X + float64(cnodes[i].W/2)
			}
			return cnodes[i].Y + float64(cnodes[i].H/2)
		}
		for _, x := range [...]bool{true, false} {
			sorted := slices.Clone(regions)
			slices.SortStableFunc(sorted, func(a, b int) int {
				d := lo(a, x) - lo(b, x)
				if d < 0 {
					return -1
				}
				if d > 0 {
					return 1
				}
				return a - b
			})
			apart := true
			for k := 1; k < len(sorted); k++ {
				if hi(sorted[k-1], x) > lo(sorted[k], x)+0.5 {
					apart = false
				}
			}
			if !apart {
				continue
			}
			for k := 1; k < len(sorted); k++ {
				at := svg.Num(float64((hi(sorted[k-1], x)+lo(sorted[k], x))/2) + pad)
				if x {
					dividers.WriteString(`<path class="pele-divider" d="M` + at + "," + svg.Num(top) + "V" + svg.Num(bottom) + `"/>`)
				} else {
					dividers.WriteString(`<path class="pele-divider" d="M` + svg.Num(left) + "," + at + "H" + svg.Num(right) + `"/>`)
				}
			}
			return
		}
		// Regions that sit neither side by side nor in a stack each get an outline.
		for _, i := range regions {
			c := &cnodes[i]
			dividers.WriteString(rect(c.X-float64(c.W/2)+pad, c.Y-float64(c.H/2)+pad, c.W, c.H, ` class="pele-divider" rx="`+jsnum.Format(corner)+`"`))
		}
	}
	divide(roots, float64(pad/2), float64(pad/2), width-float64(pad/2), height-float64(pad/2))

	smallText := jsnum.Format(small)
	for _, i := range order {
		v := views[i]
		node := v.node
		style := v.style
		if node == nil || style == nil || v.kind >= kRegion {
			continue
		}
		c := &cnodes[i]
		x := c.X + pad
		y := c.Y + pad
		w := v.w
		h := v.h
		labelAttrs := ` class="pele-label"` + style.Text
		var body string

		switch v.kind {
		case kComposite:
			left := x - float64(c.W/2)
			top := y - float64(c.H/2)
			body = rect(left, top, c.W, c.H, ` rx="`+jsnum.Format(corner)+`" fill="var(--_a)" fill-opacity="0.5" stroke="var(--_b)"`+style.Shape)
			if v.titleH > 0 {
				body += `<path d="M` + svg.Num(left) + "," + svg.Num(top+v.titleH) + "H" + svg.Num(left+c.W) + `"` + rule + style.Line + `/>` +
					svg.LabelSVG(v.label, titleX(c, v.label.Width, crossings)+pad, top+float64(v.titleH/2), ` class="pele-cluster-label"`+style.Text, icons)
			}
			divide(children[i], left, top+v.titleH, left+c.W, top+c.H)
		case kStart:
			body = `<circle r="7" fill="var(--_l)"` + style.Shape + `/>`
		case kEnd:
			body = `<circle r="8.5" fill="var(--_bg)" stroke="var(--_l)"` + style.Line + `/><circle r="5" fill="var(--_l)"` + style.Shape + `/>`
		case kChoice:
			body = `<polygon points="0,` + svg.Num(-h/2) + " " + svg.Num(w/2) + ",0 0," + svg.Num(h/2) + " " + svg.Num(-w/2) + `,0"` + surface + style.Shape + `/>`
		case kFork, kJoin:
			body = rect(-w/2, -h/2, w, h, ` rx="2" fill="var(--_l)"`+style.Shape)
		case kNote:
			r := svg.Num(w / 2)
			b := svg.Num(h / 2)
			foldX := svg.Num(float64(w/2) - fold)
			cornerY := svg.Num(fold - float64(h/2))
			body = `<path d="M-` + r + ",-" + b + "H" + foldX + "L" + r + "," + cornerY + "V" + b + "H-" + r + `Z" fill="var(--_bg)" stroke="var(--_b)"/>` +
				`<path d="M` + foldX + ",-" + b + "V" + cornerY + "H" + r + `"` + rule + `/>` +
				svg.LabelSVG(v.label, 0, 0, ` class="pele-label" font-size="`+smallText+`" fill="var(--_m)"`, icons)
		default:
			body = rect(-w/2, -h/2, w, h, ` rx="`+svg.Num(min(corner, h/2))+`"`+surface+style.Shape)
			if v.kind == kState {
				body += svg.LabelSVG(v.label, 0, 0, labelAttrs, icons)
				break
			}
			at := padY - float64(h/2) + v.label.Height
			body += `<path d="M` + svg.Num(-w/2) + "," + svg.Num(at+6) + "H" + svg.Num(w/2) + `"` + rule + style.Line + `/>` +
				svg.LabelSVG(v.label, 0, at-float64(v.label.Height/2), labelAttrs, icons)
			at += 14
			for _, line := range v.lines {
				body += svg.LabelSVG(line, 0, at+float64(line.Height/2), ` class="pele-state-description" font-size="`+smallText+`"`+style.Text, icons)
				at += line.Height
			}
		}

		if link, ok := db.links[node.id]; ok {
			// Entity codes are resolved first, so the URL is checked in the form a browser would follow.
			if tooltip := text.DecodeEntities(unquote(link.tooltip)); tooltip != "" {
				body = `<title>` + svg.Esc(tooltip) + `</title>` + body
			}
			body = util.Linked(body, util.SafeURL(unquote(link.url)), node.id, &links, options, ` target="_blank"`)
		}
		open := `<g class="` + kindClasses[v.kind] + svg.ClassNames(stripMermaidClasses(node.cssClasses)) + `" data-id="` + svg.EscText(node.id) + `"`
		switch v.kind {
		case kComposite:
			clusters.WriteString(open + ">" + body + "</g>")
		case kNote:
			notesOut.WriteString(open + ` transform="translate(` + svg.Num(x) + "," + svg.Num(y) + `)">` + body + "</g>")
		default:
			nodesOut.WriteString(open + ` transform="translate(` + svg.Num(x) + "," + svg.Num(y) + `)">` + body + "</g>")
		}
	}

	// Moves a route end from the layout box onto the drawn shape, and spreads the ends along a bar.
	attach := func(route []float64, at, i int) {
		c := &cnodes[i]
		if c.IsGroup {
			return
		}
		v := views[i]
		bar := v.kind == kFork || v.kind == kJoin
		other := at - 3
		if at == 0 {
			other = 3
		}
		// 0 is the x axis and 1 the y axis; the end sits on a side that faces along `axis`.
		for _, axis := range [...]int{1, 0} {
			extent, drawnExtent, breadth, center, cross := c.W, v.w, v.h, c.X+pad, c.Y+pad
			if axis == 1 {
				extent, drawnExtent, breadth, center, cross = c.H, v.h, v.w, c.Y+pad, c.X+pad
			}
			delta := route[at+axis] - center
			if math.Abs(math.Abs(delta)-float64(extent/2)) >= 0.5 {
				continue
			}
			route[at+axis] -= float64(jsSign(delta) * (extent - drawnExtent) / 2)
			if bar && breadth > drawnExtent {
				route[at+1-axis] = clamp(route[other+1-axis], cross-float64(breadth/2)+6, cross+float64(breadth/2)-6)
			}
			return
		}
	}

	var reach svg.SideReach
	for i := range cedges {
		ce := &cedges[i]
		if ce.Src == ce.Dst || len(ce.Route) < 6 || drawn[i].dashed {
			continue
		}
		reach.Add(ce.Dst, svg.EndSide(ce.Route, len(ce.Route)-3, &cnodes[ce.Dst]), "arrow_point")
	}

	var edgesOut, labelsOut strings.Builder
	loopSeen := make([]int, states)
	loopText := make([]float64, states)
	for i := range cedges {
		ce := &cedges[i]
		edge := drawn[i]
		label := edgeLabels[i]
		note := edge.dashed
		endType := "arrow_point"
		if note {
			endType = "none"
		}
		var path svg.EdgePath
		if ce.Src == ce.Dst {
			a := &cnodes[ce.Src]
			v := views[ce.Src]
			k := loopSeen[ce.Src]
			loopSeen[ce.Src]++
			// The loop hangs off a side the flow does not use: the right, or below when the flow is horizontal.
			side := across(ce.Src)
			w, h := v.w, v.h
			if a.IsGroup {
				w, h = a.W, a.H
			}
			out := float64(h / 2)
			spreadFrom := w
			if side {
				out = float64(w / 2)
				spreadFrom = h
			}
			spread := max(min(float64(spreadFrom/2)-4, float64(8+k*6)), 2)
			loop := float64(loopReach + k*8)
			hyp := jsmath.Hypot(loop, spread)
			if hyp == 0 || math.IsNaN(hyp) {
				hyp = 1
			}
			trim := svg.MarkerTrim(endType) / hyp
			// Offsets are given for a loop on the right and turned a quarter for one below.
			px := func(dx, dy float64) float64 {
				if side {
					return a.X + pad + (out + dx)
				}
				return a.X + pad + -dy
			}
			py := func(dx, dy float64) float64 {
				if side {
					return a.Y + pad + dy
				}
				return a.Y + pad + (out + dx)
			}
			pt := func(dx, dy float64) string { return svg.Num(px(dx, dy)) + "," + svg.Num(py(dx, dy)) }
			path = svg.EdgePath{
				D: "M" + pt(0, -spread) + "C" + pt(loop, float64(-2*spread)) + " " + pt(loop, float64(2*spread)) + " " +
					pt(float64(loop*trim), spread+float64(spread*trim)),
				EX:  px(0, spread),
				EY:  py(0, spread),
				EDX: px(-loop, -spread) - px(0, 0),
				EDY: py(-loop, -spread) - py(0, 0),
			}
			length := jsmath.Hypot(path.EDX, path.EDY)
			if length == 0 || math.IsNaN(length) {
				length = 1
			}
			path.EDX /= length
			path.EDY /= length
			// Labels stack beyond the outermost loop, in the order of their transitions.
			furthest := float64(loopReach + (loopCount[ce.Src]-1)*8)
			below := loopText[ce.Src] + float64(label.Height/2)
			loopText[ce.Src] += label.Height
			if side {
				ce.LabelX = a.X + out + furthest + 4 + float64(ce.LabelW/2)
				ce.LabelY = a.Y - float64(loopHeight[ce.Src]/2) + below
			} else {
				ce.LabelX = a.X
				ce.LabelY = a.Y + out + float64(furthest*0.75) + 4 + below
			}
		} else {
			route := slices.Clone(ce.Route)
			leaves := reach.Of(ce.Src, svg.EndSide(route, 0, &cnodes[ce.Src]))
			arrives := reach.Of(ce.Dst, svg.EndSide(route, len(route)-3, &cnodes[ce.Dst]))
			for k := 0; k < len(route); k += 3 {
				route[k] += pad
				route[k+1] += pad
			}
			attach(route, 0, ce.Src)
			attach(route, len(route)-3, ce.Dst)
			if composite, ok := anchors[i]; ok && len(route) >= 6 {
				g := &cnodes[composite]
				inward := views[ce.Src].kind == kHidden
				at := len(route) - 3
				if inward {
					at = 0
				}
				near := (g.Dir == layout.TB || g.Dir == layout.LR) == inward
				var point []float64
				if vertical(g.Dir) {
					py := g.Y + pad + float64(g.H/2)
					if near {
						py = g.Y + pad + (views[composite].titleH - float64(g.H/2))
					}
					point = []float64{route[at], py, 0}
				} else {
					px := g.X + pad + float64(g.W/2)
					if near {
						px = g.X + pad + -float64(g.W/2)
					}
					point = []float64{px, route[at+1], 1}
				}
				if inward {
					route = append(point, route...)
				} else {
					route = append(route, point...)
				}
			}
			path = svg.RoutePath(route, curve, 0, svg.MarkerTrim(endType), leaves, arrives)
		}
		class := "pele-transition"
		if note {
			class = `pele-note-edge" stroke="var(--_b)" stroke-dasharray="3 4`
		}
		edgesOut.WriteString(`<g class="pele-edge ` + class + `" data-id="` + svg.EscText(edge.id) + `">` +
			`<path d="` + path.D + `"/>` +
			svg.Marker(endType, path.EX, path.EY, path.EDX, path.EDY, "var(--_l)") +
			"</g>")
		if label.Width > 0 {
			labelsOut.WriteString(svg.EdgeLabelSVG(edge.id, label, ce.LabelX+pad, ce.LabelY+pad, "", icons))
		}
	}

	var content strings.Builder
	if clusters.Len() > 0 {
		content.WriteString(`<g class="pele-clusters">` + clusters.String() + "</g>")
	}
	if dividers.Len() > 0 {
		content.WriteString(`<g class="pele-dividers" stroke-dasharray="4 4"` + rule + ">" + dividers.String() + "</g>")
	}
	if edgesOut.Len() > 0 {
		content.WriteString(`<g class="pele-edges" fill="none" stroke="var(--_l)" stroke-linecap="round">` + edgesOut.String() + "</g>")
	}
	if labelsOut.Len() > 0 {
		content.WriteString(`<g class="pele-edge-labels" font-size="` + smallText + `">` + labelsOut.String() + "</g>")
	}
	content.WriteString(`<g class="pele-nodes">` + nodesOut.String() + notesOut.String() + "</g>")
	titled := svg.WithTitle(db.title, measurer, size, pad, width, height, content.String(), icons)
	doc := svg.Document("state", titled.Width, titled.Height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccTitle: db.accTitle, AccDescr: db.accDescr}, titled.Content)
	return core.Rendered{SVG: doc, Width: titled.Width, Height: titled.Height, Links: links}
}

// titleX picks where a composite's title sits in its title bar so that edges entering from
// above do not cross it.
func titleX(c *layout.CNode, width float64, crossings map[float64][]float64) float64 {
	xs, ok := crossings[jsmath.Round(c.Y-float64(c.H/2))]
	if !ok || width == 0 {
		return c.X
	}
	left := c.X - float64(c.W/2)
	half := float64(width / 2)
	best := c.X
	bestGap := -1.0
	for _, x := range [...]float64{c.X, left + groupPad + half, left + c.W - groupPad - half} {
		gap := math.Inf(1)
		for _, cx := range xs {
			if cx > left && cx < left+c.W {
				gap = min(gap, math.Abs(cx-x)-half)
			}
		}
		if gap >= 6 {
			return x
		}
		if gap > bestGap+0.5 {
			bestGap = gap
			best = x
		}
	}
	return best
}
