package block

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
	gapDefault = 12
	wrapWidth  = 200
	corner     = 8
	shapeAttrs = ` fill="var(--_s)" stroke="var(--_b)"`
	lineAttrs  = ` stroke="var(--_b)"`
	// Checking every edge against every block is quadratic, so large diagrams route without it.
	obstacleBudget = 2_000_000
	// sqrt1_2 is Math.SQRT1_2.
	sqrt1_2 = math.Sqrt2 / 2
)

var shapes = map[string]string{
	"square":              "rect",
	"round":               "rounded",
	"circle":              "circle",
	"doublecircle":        "dbl-circ",
	"diamond":             "diam",
	"hexagon":             "hex",
	"stadium":             "stadium",
	"subroutine":          "fr-rect",
	"cylinder":            "cyl",
	"rect_left_inv_arrow": "odd",
	"lean_right":          "lean-r",
	"lean_left":           "lean-l",
	"trapezoid":           "trap-b",
	"inv_trapezoid":       "trap-t",
}

// fixed is the shapes that keep their own size in a larger cell, where the others stretch to
// fill it.
var fixed = map[string]bool{"circle": true, "dbl-circ": true, "diam": true}

// wide is the shapes that only widen, because their slants and curves are set by their height.
var wide = map[string]bool{"stadium": true, "hex": true, "odd": true, "lean-r": true, "lean-l": true, "trap-b": true, "trap-t": true}

type kind int

const (
	kindLeaf kind = iota
	kindArrow
	kindComposite
	kindSpace
)

type view struct {
	cell
	block *blockNode
	kind  kind
	index int
	shape string
	label text.Label
	style svg.ResolvedStyle
	mask  int
	dx    float64
	dy    float64
	// bw and bh are the size as drawn, which for a fixed shape is smaller than its cell.
	bw float64
	bh float64
}

type route struct {
	edge *blockNode
	// points holds two points for a straight line, three for a bend, none for a self-loop.
	points []float64
	loop   int
	label  text.Label
	lx, ly float64
}

// orOne is `v || 1`.
func orOne(v float64) float64 {
	if v == 0 || math.IsNaN(v) {
		return 1
	}
	return v
}

func isRound(v *view) bool {
	return v.kind == kindLeaf && (v.shape == "circle" || v.shape == "dbl-circ")
}

// edgeY is where a vertical line at x meets the outline, below the center (sign 1) or above
// it (sign -1).
func edgeY(v *view, x, sign float64) float64 {
	hw := float64(v.bw / 2)
	hh := float64(v.bh / 2)
	dx := math.Abs(x - v.x)
	reach := hh
	switch {
	case isRound(v):
		reach = math.Sqrt(max(float64(hw*hw)-float64(dx*dx), 0))
	case v.kind == kindLeaf && v.shape == "diam":
		reach = float64(hh * max(1-dx/hw, 0))
	case v.kind == kindLeaf:
		side := 0
		if sign > 0 {
			side = 2
		}
		reach = hh - svg.ShapeInset(v.shape, v.bw, v.bh, side, 0)
	case v.kind == kindArrow && arrowAxis(v.mask) == "x":
		reach = hh - overhang
	}
	return v.y + float64(sign*reach)
}

// edgeX is where a horizontal line at y meets the outline, right of the center (sign 1) or
// left of it (sign -1).
func edgeX(v *view, y, sign float64) float64 {
	hw := float64(v.bw / 2)
	hh := float64(v.bh / 2)
	dy := math.Abs(y - v.y)
	reach := hw
	switch {
	case isRound(v):
		reach = math.Sqrt(max(float64(hh*hh)-float64(dy*dy), 0))
	case v.kind == kindLeaf && v.shape == "diam":
		reach = float64(hw * max(1-dy/hh, 0))
	case v.kind == kindLeaf:
		side := 3
		if sign > 0 {
			side = 1
		}
		reach = hw - svg.ShapeInset(v.shape, v.bw, v.bh, side, 0)
	case v.kind == kindArrow && arrowAxis(v.mask) == "y":
		reach = hw - overhang
	}
	return v.x + float64(sign*reach)
}

// border appends where the line from the center toward (tx, ty) leaves the outline.
func border(v *view, tx, ty float64, out []float64) []float64 {
	dx := tx - v.x
	dy := ty - v.y
	hw := float64(v.bw / 2)
	hh := float64(v.bh / 2)
	var k float64
	switch {
	case isRound(v):
		k = hw / orOne(jsmath.Hypot(dx, dy))
	case v.kind == kindLeaf && v.shape == "diam":
		k = 1 / orOne(math.Abs(dx)/hw+math.Abs(dy)/hh)
	default:
		kx, ky := math.Inf(1), math.Inf(1)
		if dx != 0 {
			kx = hw / math.Abs(dx)
		}
		if dy != 0 {
			ky = hh / math.Abs(dy)
		}
		k = min(kx, ky)
	}
	if math.IsInf(k, 1) {
		k = 0
	}
	return append(out, v.x+float64(dx*k), v.y+float64(dy*k))
}

func crossesBox(x1, y1, x2, y2 float64, v *view) bool {
	l := v.x - float64(v.bw/2) + 1
	r := v.x + float64(v.bw/2) - 1
	t := v.y - float64(v.bh/2) + 1
	b := v.y + float64(v.bh/2) - 1
	if (x1 < l && x2 < l) || (x1 > r && x2 > r) || (y1 < t && y2 < t) || (y1 > b && y2 > b) {
		return false
	}
	if x1 == x2 || y1 == y2 {
		return true
	}
	// The segment is diagonal and its bounding box overlaps: the box is hit unless all four corners are on one side.
	dx := x2 - x1
	dy := y2 - y1
	side := func(x, y float64) float64 { return jsmath.Sign(float64(dx*(y-y1)) - float64(dy*(x-x1))) }
	first := side(l, t)
	return first == 0 || side(r, t) != first || side(r, b) != first || side(l, b) != first
}

func hits(points []float64, s, t *view, obstacles []*view) int {
	count := 0
	for _, v := range obstacles {
		if v == s || v == t {
			continue
		}
		for i := 0; i+3 < len(points); i += 2 {
			if crossesBox(points[i], points[i+1], points[i+2], points[i+3], v) {
				count++
				break
			}
		}
	}
	return count
}

// routeEdge is a straight line where the two blocks share a row or a column, otherwise one
// bend between facing sides. A nil obstacles routes without checking for blocks in the way.
func routeEdge(s, t *view, shift float64, obstacles []*view) []float64 {
	sl := s.x - float64(s.bw/2)
	sr := s.x + float64(s.bw/2)
	st := s.y - float64(s.bh/2)
	sb := s.y + float64(s.bh/2)
	tl := t.x - float64(t.bw/2)
	tr := t.x + float64(t.bw/2)
	tt := t.y - float64(t.bh/2)
	tb := t.y + float64(t.bh/2)
	x1 := max(sl, tl)
	x2 := min(sr, tr)
	y1 := max(st, tt)
	y2 := min(sb, tb)
	overX := x2-x1 > 6
	overY := y2-y1 > 6

	if overX && overY {
		// One block is inside the other: join the inner one to the nearest side of the outer one.
		inner, outer := t, s
		if float64(s.bw*s.bh) <= float64(t.bw*t.bh) {
			inner, outer = s, t
		}
		up := inner.y - float64(inner.bh/2) - (outer.y - float64(outer.bh/2))
		down := outer.y + float64(outer.bh/2) - (inner.y + float64(inner.bh/2))
		left := inner.x - float64(inner.bw/2) - (outer.x - float64(outer.bw/2))
		right := outer.x + float64(outer.bw/2) - (inner.x + float64(inner.bw/2))
		least := min(up, down, left, right)
		var points []float64
		switch least {
		case down:
			points = []float64{inner.x, edgeY(inner, inner.x, 1), inner.x, outer.y + float64(outer.bh/2)}
		case up:
			points = []float64{inner.x, edgeY(inner, inner.x, -1), inner.x, outer.y - float64(outer.bh/2)}
		case right:
			points = []float64{edgeX(inner, inner.y, 1), inner.y, outer.x + float64(outer.bw/2), inner.y}
		default:
			points = []float64{edgeX(inner, inner.y, -1), inner.y, outer.x - float64(outer.bw/2), inner.y}
		}
		if inner == s {
			return points
		}
		return []float64{points[2], points[3], points[0], points[1]}
	}
	if overX {
		margin := min(6, float64((x2-x1)/2))
		x := min(max(float64((x1+x2)/2)+shift, x1+margin), x2-margin)
		down := -1.0
		if t.y > s.y {
			down = 1
		}
		return []float64{x, edgeY(s, x, down), x, edgeY(t, x, -down)}
	}
	if overY {
		margin := min(6, float64((y2-y1)/2))
		y := min(max(float64((y1+y2)/2)+shift, y1+margin), y2-margin)
		right := -1.0
		if t.x > s.x {
			right = 1
		}
		return []float64{edgeX(s, y, right), y, edgeX(t, y, -right), y}
	}

	right := -1.0
	if t.x > s.x {
		right = 1
	}
	down := -1.0
	if t.y > s.y {
		down = 1
	}
	across := []float64{edgeX(s, s.y, right), s.y, t.x, s.y, t.x, edgeY(t, t.x, -down)}
	along := []float64{s.x, edgeY(s, s.x, down), s.x, t.y, edgeX(t, t.y, -right), t.y}
	isWide := math.Abs(t.x-s.x) >= math.Abs(t.y-s.y)
	best, other := along, across
	if isWide {
		best, other = across, along
	}
	if obstacles == nil {
		return best
	}
	least := hits(best, s, t, obstacles)
	if least == 0 {
		return best
	}
	if count := hits(other, s, t, obstacles); count < least {
		best = other
		least = count
	}
	if least == 0 {
		return best
	}
	direct := border(s, t.x, t.y, nil)
	direct = border(t, s.x, s.y, direct)
	if hits(direct, s, t, obstacles) < least {
		return direct
	}
	return best
}

// trace builds the path through two or three points, rounding the bend and leaving room for
// markers.
func trace(p []float64, startTrim, endTrim float64) svg.EdgePath {
	n := len(p)
	sx := p[0]
	sy := p[1]
	ex := p[n-2]
	ey := p[n-1]
	ax := p[2] - sx
	ay := p[3] - sy
	first := jsmath.Hypot(ax, ay)
	ax /= orOne(first)
	ay /= orOne(first)
	bx := ex - p[n-4]
	by := ey - p[n-3]
	last := jsmath.Hypot(bx, by)
	bx /= orOne(last)
	by /= orOne(last)
	if n == 4 && startTrim+endTrim > first {
		scale := first / (startTrim + endTrim)
		startTrim *= scale
		endTrim *= scale
	}
	startTrim = min(startTrim, first)
	endTrim = min(endTrim, last)
	d := "M" + svg.Num(sx+float64(ax*startTrim)) + "," + svg.Num(sy+float64(ay*startTrim))
	if n == 6 {
		r := max(0, min(corner, first-startTrim, last-endTrim))
		d += "L" + svg.Num(p[2]-float64(ax*r)) + "," + svg.Num(p[3]-float64(ay*r)) + "Q" + svg.Num(p[2]) + "," + svg.Num(p[3]) + " " +
			svg.Num(p[2]+float64(bx*r)) + "," + svg.Num(p[3]+float64(by*r))
	}
	d += "L" + svg.Num(ex-float64(bx*endTrim)) + "," + svg.Num(ey-float64(by*endTrim))
	return svg.EdgePath{D: d, SX: sx, SY: sy, SDX: -ax, SDY: -ay, EX: ex, EY: ey, EDX: bx, EDY: by}
}

// loopArc gives a self-loop's arc: around the top right corner of the block, or around the
// matching point on a round or diamond outline. It returns the arc's center and radius and
// the angles where it leaves and returns.
func loopArc(v *view, k int) (px, py, r, a1, a2 float64) {
	hw := float64(v.bw / 2)
	hh := float64(v.bh / 2)
	r = max(1, min(10+float64(5*float64(k-1)), hw, hh))
	if isRound(v) {
		px = v.x + float64(hw*sqrt1_2)
		py = v.y - float64(hw*sqrt1_2)
		half := float64(2 * jsmath.Asin(min(1, r/float64(2*hw))))
		angle := func(at float64) float64 {
			return jsmath.Atan2(v.y-float64(hw*jsmath.Sin(at))-py, v.x+float64(hw*jsmath.Cos(at))-px)
		}
		return px, py, r, angle(math.Pi/4 + half), angle(math.Pi/4 - half)
	}
	if v.kind == kindLeaf && v.shape == "diam" {
		slope := jsmath.Atan2(hh, hw)
		return v.x + float64(hw/2), v.y - float64(hh/2), r, slope + math.Pi, slope
	}
	return v.x + hw, v.y - hh, r, math.Pi, math.Pi / 2
}

func loopPath(v *view, k int, startTrim, endTrim float64) svg.EdgePath {
	px, py, r, a1, a2 := loopArc(v, k)
	from := a1 + startTrim/r
	to := a2 - endTrim/r
	sweep := math.Mod(to-from, 2*math.Pi)
	if sweep <= 0 {
		sweep += 2 * math.Pi
	}
	x1 := px + float64(r*jsmath.Cos(from))
	y1 := py + float64(r*jsmath.Sin(from))
	x2 := px + float64(r*jsmath.Cos(to))
	y2 := py + float64(r*jsmath.Sin(to))
	sx := px + float64(r*jsmath.Cos(a1))
	sy := py + float64(r*jsmath.Sin(a1))
	ex := px + float64(r*jsmath.Cos(a2))
	ey := py + float64(r*jsmath.Sin(a2))
	// Each marker lies along the chord it covers, so it follows the curve instead of the tangent at its tip.
	sl := jsmath.Hypot(sx-x1, sy-y1)
	el := jsmath.Hypot(ex-x2, ey-y2)
	large := "0"
	if sweep > math.Pi {
		large = "1"
	}
	path := svg.EdgePath{
		D:  "M" + svg.Num(x1) + "," + svg.Num(y1) + "A" + svg.Num(r) + "," + svg.Num(r) + " 0 " + large + " 1 " + svg.Num(x2) + "," + svg.Num(y2),
		SX: sx, SY: sy, EX: ex, EY: ey,
	}
	if sl > 0 {
		path.SDX, path.SDY = (sx-x1)/sl, (sy-y1)/sl
	} else {
		path.SDX, path.SDY = jsmath.Sin(a1), -jsmath.Cos(a1)
	}
	if el > 0 {
		path.EDX, path.EDY = (ex-x2)/el, (ey-y2)/el
	} else {
		path.EDX, path.EDY = -jsmath.Sin(a2), jsmath.Cos(a2)
	}
	return path
}

var spots = [...]float64{0.5, 0.3, 0.7, 0.15, 0.85}

// labelSpot picks the point on a leg where a label of the given size covers no block other
// than the two it joins.
func labelSpot(p []float64, at int, w, h float64, s, t *view, obstacles []*view) float64 {
	if obstacles == nil {
		return 0.5
	}
	for _, spot := range spots {
		x := p[at] + float64((p[at+2]-p[at])*spot)
		y := p[at+1] + float64((p[at+3]-p[at+1])*spot)
		clear := true
		for _, v := range obstacles {
			if v != s && v != t && math.Abs(x-v.x) < (w+v.bw)/2 && math.Abs(y-v.y) < (h+v.bh)/2 {
				clear = false
				break
			}
		}
		if clear {
			return spot
		}
	}
	return 0.5
}

func renderBlock(model *blockDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	return common.Tighten(rc.Options, func(tight bool) core.Rendered { return draw(model, config, rc, tight) })
}

func draw(model *blockDb, config *jsobj.Object, rc *core.Context, tight bool) core.Rendered {
	options := rc.Options
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	edgeSize := jsmath.Round(size * 0.875)
	measurer := rc.Measurer()
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	icons := rc.Icons
	gap := float64(gapDefault)
	if section, ok := config.Get("block"); ok {
		if obj, isObj := section.(*jsobj.Object); isObj {
			if configured, has := obj.Get("padding"); has {
				if v, isNum := configured.(float64); isNum && v >= 0 {
					gap = min(v, 200)
				}
			}
		}
	}
	wrap := float64(wrapWidth)
	if tight {
		gap = min(gap, 6)
		wrap = 80
	}

	styleOf := func(block *blockNode) svg.ResolvedStyle {
		var declarations []string
		// A style list is split on commas only, so one entry can still hold several declarations.
		add := func(styles []string) {
			for _, style := range styles {
				declarations = append(declarations, strings.Split(style, ";")...)
			}
		}
		names := block.classes
		if len(names) == 0 {
			names = []string{"default"}
		}
		for _, name := range names {
			if def, ok := model.classes[name]; ok {
				add(def.styles)
			}
		}
		add(block.styles)
		return svg.ResolveStyle(declarations)
	}

	newView := func(block *blockNode, index int) *view {
		typ := block.typ
		var style svg.ResolvedStyle
		if typ != "space" {
			style = styleOf(block)
		}
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
		v := &view{block: block, index: index, style: style}
		var nw, nh, head float64
		switch typ {
		case "space":
			v.kind = kindSpace
		case "composite":
			v.kind = kindComposite
			room := 4000.0
			if tight {
				room = common.TitleRoom(options)
			}
			v.label = text.Layout(block.label, false, measurer, fontSize, room, textStyle, icons)
			if v.label.Height > 0 {
				head = v.label.Height + 4
			}
			nw = v.label.Width + float64(2*gap)
			nh = head + float64(2*gap)
		case "block_arrow":
			v.kind = kindArrow
			v.label = text.Layout(block.label, false, measurer, fontSize, wrap, textStyle, icons)
			blank := true
			for _, line := range v.label.Lines {
				for _, part := range line {
					if part.Icon != "" || jsstr.Trim(part.Text) != "" {
						blank = false
					}
				}
			}
			v.mask = arrowMask(block.directions)
			var s arrowBox
			if blank {
				s = arrowSize(v.mask, 0, 0)
			} else {
				s = arrowSize(v.mask, v.label.Width, v.label.Height)
			}
			nw, nh, v.dx, v.dy = s.w, s.h, s.dx, s.dy
		default:
			v.shape = "rect"
			if shape, ok := shapes[typ]; ok {
				v.shape = shape
			}
			v.label = text.Layout(block.label, false, measurer, fontSize, wrap, textStyle, icons)
			s := svg.ShapeSizeOf(v.shape, v.label.Width, v.label.Height)
			nw = s.W
			// A cylinder's caps deepen as it widens, so it is sized for the deepest ones.
			nh = s.H
			if v.shape == "cyl" {
				nh = max(s.H, v.label.Height+40)
			}
			v.dy = s.DY
		}
		v.columns = -1
		if v.kind == kindComposite {
			v.columns = block.columns
		}
		v.span = block.widthInColumns
		if v.kind == kindSpace {
			v.span = 1
		}
		v.space = v.kind == kindSpace
		v.head, v.nw, v.nh = head, nw, nh
		return v
	}

	root := newView(model.root, 0)
	root.nw = 0
	root.nh = 0
	order := []*view{root}
	byId := map[string]*view{}
	for i := 0; i < len(order); i++ {
		parent := order[i]
		if parent.kind != kindComposite {
			continue
		}
		for _, child := range parent.block.children {
			v := newView(child, len(order))
			parent.children = append(parent.children, &v.cell)
			order = append(order, v)
			if v.kind != kindSpace {
				byId[child.id] = v
			}
		}
	}

	contentWidth, contentHeight := layout(&root.cell, gap)

	obstacles := []*view{}
	for _, v := range order {
		switch {
		case v.kind == kindLeaf && fixed[v.shape]:
			v.bw = v.nw
			v.bh = v.nh
		case v.kind == kindArrow:
			v.bw = v.nw
			if v.span > 1 {
				v.bw = v.w
			}
			v.bh = v.nh
		default:
			v.bw = v.w
			v.bh = v.h
			if v.kind == kindLeaf && wide[v.shape] {
				v.bh = v.nh
			}
		}
		if v.kind == kindLeaf && v.shape == "cyl" {
			v.dy = min(12, max(6, v.bw/14)) / 2
		}
		if v.kind == kindLeaf || v.kind == kindArrow {
			obstacles = append(obstacles, v)
		}
	}

	// Edges between the same two blocks are spread apart instead of drawn on top of each other.
	pairKey := func(s, t *view) int { return min(s.index, t.index)*len(order) + max(s.index, t.index) }
	pairs := map[int]int{}
	labelled := map[int]bool{}
	var ends [][2]*view
	var drawn []*blockNode
	for _, edge := range model.edges {
		s, okS := byId[edge.start]
		t, okT := byId[edge.end]
		if !okS || !okT {
			continue
		}
		key := pairKey(s, t)
		pairs[key]++
		if edge.label != "" {
			labelled[key] = true
		}
		ends = append(ends, [2]*view{s, t})
		drawn = append(drawn, edge)
	}
	check := obstacles
	if float64(len(drawn))*float64(len(obstacles)) > obstacleBudget {
		check = nil
	}
	seen := map[int]int{}
	routes := make([]route, 0, len(drawn))
	minX, minY := 0.0, 0.0
	maxX, maxY := contentWidth, contentHeight
	grow := func(x, y, w, h float64) {
		if x-float64(w/2) < minX {
			minX = x - float64(w/2)
		}
		if x+float64(w/2) > maxX {
			maxX = x + float64(w/2)
		}
		if y-float64(h/2) < minY {
			minY = y - float64(h/2)
		}
		if y+float64(h/2) > maxY {
			maxY = y + float64(h/2)
		}
	}
	for i, edge := range drawn {
		s, t := ends[i][0], ends[i][1]
		key := pairKey(s, t)
		total := pairs[key]
		k := seen[key]
		seen[key] = k + 1
		label := text.Layout(edge.label, false, measurer, edgeSize, wrapWidth, 0, icons)
		var points []float64
		var lx, ly float64
		if s == t {
			px, py, r, _, _ := loopArc(s, k+1)
			grow(px, py, float64(2*r)+2, float64(2*r)+2)
			lx = px + r + 6 + float64(label.Width/2)
			ly = py - float64(r/2)
		} else {
			step := 14.0
			if labelled[key] {
				step = jsmath.Round(edgeSize*1.5) + 2
			}
			points = routeEdge(s, t, float64((float64(k)-float64(float64(total-1)/2))*step), check)
			n := len(points)
			// The label goes on the longer leg.
			firstLeg := math.Abs(points[2]-points[0]) + math.Abs(points[3]-points[1])
			lastLeg := math.Abs(points[n-2]-points[n-4]) + math.Abs(points[n-1]-points[n-3])
			at := 0
			if n == 6 && lastLeg > firstLeg {
				at = 2
			}
			spot := 0.5
			if label.Width > 0 {
				spot = labelSpot(points, at, label.Width+8, label.Height, s, t, check)
			}
			lx = points[at] + float64((points[at+2]-points[at])*spot)
			ly = points[at+1] + float64((points[at+3]-points[at+1])*spot)
		}
		if label.Width > 0 {
			grow(lx, ly, label.Width+8, label.Height)
		}
		loop := 0
		if s == t {
			loop = k + 1
		}
		routes = append(routes, route{edge: edge, points: points, loop: loop, label: label, lx: lx, ly: ly})
	}

	titleRoom := 4000.0
	if tight {
		titleRoom = common.TitleRoom(options)
	}
	titleText := ""
	if model.title != nil {
		titleText = *model.title
	}
	title := svg.TitleLabel(titleText, measurer, size, titleRoom, icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}
	inner := max(maxX-minX, title.Width)
	ox := pad - minX + float64((inner-(maxX-minX))/2)
	oy := pad + titleHeight - minY
	for _, v := range order {
		v.x += ox
		v.y += oy
	}
	width := math.Ceil(inner + float64(2*pad))
	height := math.Ceil(maxY - minY + titleHeight + float64(2*pad))

	var clusters, nodes strings.Builder
	for _, v := range order[1:] {
		if v.kind == kindSpace {
			continue
		}
		classes := svg.ClassNames(strings.Join(v.block.classes, " "))
		id := svg.EscText(v.block.id)
		if v.kind == kindComposite {
			top := v.y - float64(v.h/2)
			clusters.WriteString(`<g class="pele-cluster` + classes + `" data-id="` + id + `">` +
				`<rect x="` + svg.Num(v.x-float64(v.w/2)) + `" y="` + svg.Num(top) + `" width="` + svg.Num(v.w) + `" height="` + svg.Num(v.h) +
				`" fill="var(--_a)" fill-opacity="0.5" stroke="var(--_b)"` + svg.Rounded(v.style.Shape) + `/>` +
				svg.LabelSVG(v.label, v.x, top+float64((v.head+gap)/2)+2, ` class="pele-cluster-label" fill="var(--_m)"`+v.style.Text, icons) +
				`</g>`)
			continue
		}
		arrow := v.kind == kindArrow
		shapeClass := v.shape
		var body string
		if arrow {
			shapeClass = "block-arrow"
			body = drawArrow(v.mask, v.bw, v.bh, shapeAttrs+v.style.Shape)
		} else {
			body = svg.DrawShape(v.shape, v.bw, v.bh, shapeAttrs+v.style.Shape, lineAttrs+v.style.Line)
		}
		nodes.WriteString(`<g class="pele-node pele-shape-` + shapeClass + classes + `" data-id="` + id + `" transform="translate(` + svg.Num(v.x) + `,` + svg.Num(v.y) + `)">` +
			body +
			svg.LabelSVG(v.label, v.dx, v.dy, ` class="pele-label"`+v.style.Text, icons) +
			`</g>`)
	}

	var edges, labels strings.Builder
	for i, rt := range routes {
		edge := rt.edge
		startType := edge.arrowTypeStart
		endType := edge.arrowTypeEnd
		var path svg.EdgePath
		if rt.loop > 0 {
			path = loopPath(ends[i][0], rt.loop, svg.MarkerTrim(startType), svg.MarkerTrim(endType))
		} else {
			points := rt.points
			for k := 0; k < len(points); k += 2 {
				points[k] += ox
				points[k+1] += oy
			}
			path = trace(points, svg.MarkerTrim(startType), svg.MarkerTrim(endType))
		}
		id := svg.EscText(edge.id)
		extra := ""
		if edge.thickness == "thick" {
			extra += ` stroke-width="2.5"`
		}
		if edge.pattern == "dotted" {
			extra += ` stroke-dasharray="3 4"`
		}
		edges.WriteString(`<g class="pele-edge" data-id="` + id + `">` +
			`<path d="` + path.D + `"` + extra + `/>` +
			svg.Marker(startType, path.SX, path.SY, path.SDX, path.SDY, "var(--_l)") +
			svg.Marker(endType, path.EX, path.EY, path.EDX, path.EDY, "var(--_l)") +
			`</g>`)
		if rt.label.Width > 0 {
			labels.WriteString(svg.EdgeLabelSVG(edge.id, rt.label, rt.lx+ox, rt.ly+oy, "", icons))
		}
	}

	content := svg.LabelSVG(title, float64(width/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil)
	if clusters.Len() > 0 {
		content += `<g class="pele-clusters">` + clusters.String() + `</g>`
	}
	if edges.Len() > 0 {
		content += `<g class="pele-edges" fill="none" stroke="var(--_l)" stroke-linecap="round">` + edges.String() + `</g>`
	}
	content += `<g class="pele-nodes">` + nodes.String() + `</g>`
	if labels.Len() > 0 {
		content += `<g class="pele-edge-labels" font-size="` + jsnum.Format(edgeSize) + `">` + labels.String() + `</g>`
	}

	doc := svg.Document("block", width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{}, content)
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: []core.Link{}}
}
