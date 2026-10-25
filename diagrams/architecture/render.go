package architecture

import (
	"math"
	"slices"
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
	stub     = 12
	corner   = 6
	labelGap = 4
	dot      = 3
)

var (
	sideDX = [4]float64{-1, 1, 0, 0}
	sideDY = [4]float64{0, 0, -1, 1}
)

func setting(config *jsobj.Object, key string, fallback, lo, hi float64) float64 {
	v, _ := config.Get("architecture")
	section, ok := v.(*jsobj.Object)
	if !ok {
		return fallback
	}
	value, _ := section.Get(key)
	n, ok := value.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return fallback
	}
	return min(hi, max(lo, n))
}

// rank maps the grid lines in use to consecutive indexes, so empty rows and columns take no
// room.
func rank(values []int) map[int]int {
	slices.Sort(values)
	out := map[int]int{}
	for _, value := range values {
		if _, ok := out[value]; !ok {
			out[value] = len(out)
		}
	}
	return out
}

func iconSlot(name string, x, y, side float64, icons *text.Icons) string {
	markup := icons.Resolve(name)
	if markup == "" {
		markup = builtinIcon(name)
	}
	return `<svg class="pele-icon" data-icon="` + svg.EscText(name) + `" x="` + svg.Num(x) + `" y="` + svg.Num(y) +
		`" width="` + svg.Num(side) + `" height="` + svg.Num(side) +
		`" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
		markup + `</svg>`
}

// fit cuts a label down to a box, ending what is left with an ellipsis.
func fit(label text.Label, maxWidth, maxLines float64, measurer text.Measurer) text.Label {
	cut := float64(len(label.Lines)) > maxLines
	if !cut && label.Width <= maxWidth {
		return label
	}
	dots := measurer.Width("…", label.Size, 0)
	var lines [][]text.Span
	var widths []float64
	widest := 0.0
	for i := 0; float64(i) < min(maxLines, float64(len(label.Lines))); i++ {
		line := slices.Clone(label.Lines[i])
		width := label.Widths[i]
		if width > maxWidth || cut && float64(i) == maxLines-1 {
			for {
				if len(line) == 0 {
					line = append(line, text.Span{Text: "…", Width: dots})
					width = dots
					break
				}
				span := &line[len(line)-1]
				rest := width - span.Width
				if span.Icon == "" && rest+dots < maxWidth {
					units := jsstr.UTF16(span.Text)
					lo, hi := 0, len(units)
					for lo < hi {
						mid := (lo + hi + 1) >> 1
						if rest+measurer.Width(jsstr.FromUTF16(units[:mid]), label.Size, span.Style)+dots <= maxWidth {
							lo = mid
						} else {
							hi = mid - 1
						}
					}
					if lo > 0 && units[lo-1] >= 0xd800 && units[lo-1] <= 0xdbff {
						lo--
					}
					span.Text = jsstr.TrimEnd(jsstr.FromUTF16(units[:lo])) + "…"
					span.Width = measurer.Width(span.Text, label.Size, span.Style)
					width = rest + span.Width
					break
				}
				line = line[:len(line)-1]
				width = rest
			}
		}
		lines = append(lines, line)
		widths = append(widths, width)
		widest = max(widest, width)
	}
	label.Lines = lines
	label.Widths = widths
	label.Width = widest
	label.Height = float64(len(lines)) * label.LineHeight
	return label
}

type port struct {
	x, y, dx, dy float64
	// The box the route has to stay out of when it cannot leave straight.
	x0, y0, x1, y1 float64
	// How far short of the port an arrowhead stops: the radius of a junction's dot.
	gap float64
	// Set where a junction is reached through the border of its group: the line carries on to
	// the junction.
	wire bool
}

// route is an orthogonal route from one port to another, each left along its side's normal.
func route(s, e port) []float64 {
	sx, sy, sdx, sdy := s.x, s.y, s.dx, s.dy
	ex, ey, edx, edy := e.x, e.y, e.dx, e.dy
	s1x := sx + float64(sdx*stub)
	s1y := sy + float64(sdy*stub)
	e1x := ex + float64(edx*stub)
	e1y := ey + float64(edy*stub)
	if float64(sdx*edx)+float64(sdy*edy) == 0 {
		cx, cy := sx, ey
		if sdx != 0 {
			cx, cy = ex, sy
		}
		if float64((cx-sx)*sdx)+float64((cy-sy)*sdy) > 0 && float64((cx-ex)*edx)+float64((cy-ey)*edy) > 0 {
			return []float64{sx, sy, cx, cy, ex, ey}
		}
		if sdx != 0 {
			return []float64{sx, sy, s1x, s1y, s1x, e1y, e1x, e1y, ex, ey}
		}
		return []float64{sx, sy, s1x, s1y, e1x, s1y, e1x, e1y, ex, ey}
	}
	sideways := sdx != 0
	offset := ex - sx
	if sideways {
		offset = ey - sy
	}
	if sdx == -edx && sdy == -edy {
		ahead := float64((ex-sx)*sdx) + float64((ey-sy)*sdy)
		if ahead > 0 {
			if math.Abs(offset) < 0.5 {
				return []float64{sx, sy, ex, ey}
			}
			// Turn midway between the two boxes, which is clear of both titles, when there is
			// such a place.
			if sideways {
				lo, hi := e.x1, s.x0
				if sdx > 0 {
					lo, hi = s.x1, e.x0
				}
				mx := (sx + ex) / 2
				if lo < hi {
					mx = (lo + hi) / 2
				}
				return []float64{sx, sy, mx, sy, mx, ey, ex, ey}
			}
			lo, hi := e.y1, s.y0
			if sdy > 0 {
				lo, hi = s.y1, e.y0
			}
			my := (sy + ey) / 2
			if lo < hi {
				my = (lo + hi) / 2
			}
			return []float64{sx, sy, sx, my, ex, my, ex, ey}
		}
	} else if math.Abs(offset) >= 1 {
		if sideways {
			x := min(sx, ex) - stub
			if sdx > 0 {
				x = max(sx, ex) + stub
			}
			return []float64{sx, sy, x, sy, x, ey, ex, ey}
		}
		y := min(sy, ey) - stub
		if sdy > 0 {
			y = max(sy, ey) + stub
		}
		return []float64{sx, sy, sx, y, ex, y, ex, ey}
	}
	// The straight way is blocked by one of the two nodes: go between them, or around both.
	if sideways {
		var lane float64
		switch {
		case s.y1+stub <= e.y0:
			lane = (s.y1 + e.y0) / 2
		case e.y1+stub <= s.y0:
			lane = (e.y1 + s.y0) / 2
		default:
			lane = max(s.y1, e.y1) + stub
		}
		return []float64{sx, sy, s1x, sy, s1x, lane, e1x, lane, e1x, ey, ex, ey}
	}
	var lane float64
	switch {
	case s.x1+stub <= e.x0:
		lane = (s.x1 + e.x0) / 2
	case e.x1+stub <= s.x0:
		lane = (e.x1 + s.x0) / 2
	default:
		lane = max(s.x1, e.x1) + stub
	}
	return []float64{sx, sy, sx, s1y, lane, s1y, lane, e1y, ex, e1y, ex, ey}
}

// simplify drops repeated points and joins runs that continue in a straight line.
func simplify(p []float64) []float64 {
	out := []float64{p[0], p[1]}
	for i := 2; i < len(p); i += 2 {
		x := p[i]
		y := p[i+1]
		n := len(out)
		sameX := math.Abs(x-out[n-2]) < 0.01
		sameY := math.Abs(y-out[n-1]) < 0.01
		if sameX && sameY {
			continue
		}
		if n >= 4 && (sameX && math.Abs(out[n-4]-out[n-2]) < 0.01 || sameY && math.Abs(out[n-3]-out[n-1]) < 0.01) {
			out[n-2] = x
			out[n-1] = y
		} else {
			out = append(out, x, y)
		}
	}
	return out
}

func pathData(p []float64, startTrim, endTrim float64) string {
	n := len(p) / 2
	xs := make([]float64, n)
	ys := make([]float64, n)
	for i := range n {
		xs[i] = p[i*2]
		ys[i] = p[i*2+1]
	}
	trim := func(at, toward int, amount float64) {
		length := jsmath.Hypot(xs[toward]-xs[at], ys[toward]-ys[at])
		t := min(amount, max(0, length-1))
		if length > 0 && t > 0 {
			xs[at] += float64((xs[toward] - xs[at]) / length * t)
			ys[at] += float64((ys[toward] - ys[at]) / length * t)
		}
	}
	trim(0, 1, startTrim)
	trim(n-1, n-2, endTrim)
	d := "M" + svg.Num(xs[0]) + "," + svg.Num(ys[0])
	for i := 1; i < n-1; i++ {
		before := jsmath.Hypot(xs[i]-xs[i-1], ys[i]-ys[i-1])
		after := jsmath.Hypot(xs[i+1]-xs[i], ys[i+1]-ys[i])
		r := min(corner, before/2, after/2)
		if r < 0.5 {
			d += "L" + svg.Num(xs[i]) + "," + svg.Num(ys[i])
			continue
		}
		inX := xs[i] - float64((xs[i]-xs[i-1])/before*r)
		inY := ys[i] - float64((ys[i]-ys[i-1])/before*r)
		outX := xs[i] + float64((xs[i+1]-xs[i])/after*r)
		outY := ys[i] + float64((ys[i+1]-ys[i])/after*r)
		d += "L" + svg.Num(inX) + "," + svg.Num(inY) + "Q" + svg.Num(xs[i]) + "," + svg.Num(ys[i]) + " " + svg.Num(outX) + "," + svg.Num(outY)
	}
	return d + "L" + svg.Num(xs[n-1]) + "," + svg.Num(ys[n-1])
}

func renderArchitecture(model *architectureDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	return common.Tighten(rc.Options, func(tight bool) core.Rendered { return draw(model, config, rc, tight) })
}

// rankOf is a rank's index for value, or 0 where upstream reads a missing one into an
// Int32Array.
func rankOf(ranks map[int]int, value int) int {
	return ranks[value]
}

func draw(model *architectureDb, config *jsobj.Object, rc *core.Context, tight bool) core.Rendered {
	options := rc.Options
	icons := rc.Icons
	iconSize := setting(config, "iconSize", 48, 8, 512)
	fontSize := 16.0
	if options.FontSize != nil {
		fontSize = *options.FontSize
	}
	size := setting(config, "fontSize", fontSize, 4, 128)
	groupPad := setting(config, "padding", 20, 0, 256)
	if tight {
		groupPad = min(groupPad, 12)
	}
	small := jsmath.Round(size * 0.875)
	measurer := rc.Measurer()
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	half := float64(iconSize / 2)

	var nodes []archNode
	for _, node := range model.nodes.All() {
		nodes = append(nodes, node)
	}
	var groups []archGroup
	for _, group := range model.groups.All() {
		groups = append(groups, group)
	}
	N := len(nodes)
	G := len(groups)
	grid := placeOnGrid(nodes, groups, model.edges, model.layoutHints)
	index := map[string]int{}
	for i, node := range nodes {
		index[node.id] = i
	}

	// A service is its icon square with the title beneath; a junction takes the same room.
	labels := make([]text.Label, N)
	boxW := make([]float64, N)
	boxH := make([]float64, N)
	wrap := max(float64(iconSize*2.25), 96)
	if tight {
		wrap = max(iconSize, 64)
	}
	for i, node := range nodes {
		title := node.title
		if node.junction {
			title = ""
		}
		label := text.Layout(title, true, measurer, size, wrap, 0, icons)
		labels[i] = label
		boxW[i] = max(iconSize, 2*math.Ceil(label.Width/2))
		boxH[i] = iconSize
		if label.Height > 0 {
			boxH[i] = iconSize + (labelGap + label.Height)
		}
	}

	var colValues, rowValues []int
	for i := range N {
		colValues = append(colValues, grid.col[i])
		rowValues = append(rowValues, grid.row[i])
	}
	for g := range G {
		if !grid.groupEmpty[g] {
			continue
		}
		colValues = append(colValues, grid.groupCol[g])
		rowValues = append(rowValues, grid.groupRow[g])
	}
	colOf := rank(colValues)
	rowOf := rank(rowValues)
	C := len(colOf)
	R := len(rowOf)
	colW := make([]float64, C)
	rowH := make([]float64, R)
	gap := max(40, jsmath.Round(iconSize*0.75))
	if tight {
		gap = max(24, 2*stub)
	}
	gapX := make([]float64, C)
	for c := range gapX {
		gapX[c] = gap
	}
	gapY := make([]float64, R)
	for r := range gapY {
		gapY[r] = gap
	}
	nodeCol := make([]int, N)
	nodeRow := make([]int, N)
	for i := range N {
		c := rankOf(colOf, grid.col[i])
		r := rankOf(rowOf, grid.row[i])
		nodeCol[i] = c
		nodeRow[i] = r
		colW[c] = max(colW[c], boxW[i])
		rowH[r] = max(rowH[r], boxH[i])
	}
	for g := range G {
		if !grid.groupEmpty[g] {
			continue
		}
		c := rankOf(colOf, grid.groupCol[g])
		r := rankOf(rowOf, grid.groupRow[g])
		colW[c] = max(colW[c], iconSize)
		rowH[r] = max(rowH[r], half)
	}

	// An edge label needs the gap its edge crosses to be wide enough to hold it.
	E := len(model.edges)
	edgeLabels := make([]text.Label, E)
	ends := make([]int, E*2)
	for k, edge := range model.edges {
		a, ok := index[edge.lhsID]
		if !ok {
			a = -1
		}
		b, ok := index[edge.rhsID]
		if !ok {
			b = -1
		}
		ends[k*2] = a
		ends[k*2+1] = b
		label := text.Layout(edge.title, true, measurer, small, 160, 0, icons)
		edgeLabels[k] = label
		if label.Width == 0 || a < 0 || b < 0 {
			continue
		}
		if nodeCol[a] != nodeCol[b] {
			c := min(nodeCol[a], nodeCol[b])
			gapX[c] = max(gapX[c], math.Ceil(label.Width)+28)
		} else {
			colW[nodeCol[a]] = max(colW[nodeCol[a]], float64(2*math.Ceil(label.Width/2))+8)
			if nodeRow[a] != nodeRow[b] {
				r := min(nodeRow[a], nodeRow[b])
				gapY[r] = max(gapY[r], math.Ceil(label.Height)+24)
			}
		}
	}

	// Room around each group, kept on the rows and columns at its edges. A group that shares an
	// edge with a group inside it stands that much further out. Children come after their parents,
	// so walking backwards settles every group before the one that holds it.
	groupLabels := make([]text.Label, G)
	c0 := make([]int, G)
	c1 := make([]int, G)
	r0 := make([]int, G)
	r1 := make([]int, G)
	insL := make([]float64, G)
	insR := make([]float64, G)
	insT := make([]float64, G)
	insB := make([]float64, G)
	headW := make([]float64, G)
	headH := make([]float64, G)
	colL := make([]float64, C)
	colR := make([]float64, C)
	rowT := make([]float64, R)
	rowB := make([]float64, R)
	badge := jsmath.Round(size * 1.25)
	for g := range G {
		c0[g] = rankOf(colOf, grid.groupCol[g])
		c1[g] = rankOf(colOf, grid.groupCol[g]+grid.groupCols[g]-1)
		r0[g] = rankOf(rowOf, grid.groupRow[g])
		r1[g] = rankOf(rowOf, grid.groupRow[g]+grid.groupRows[g]-1)
		room := 320.0
		if tight {
			room = max(80, common.TitleRoom(options)/2)
		}
		label := text.Layout(groups[g].title, true, measurer, size, room, 0, icons)
		groupLabels[g] = label
		icon := 0.0
		if groups[g].icon != "" {
			icon = badge
		}
		headW[g] = label.Width + icon
		if label.Width > 0 && icon > 0 {
			headW[g] += 6
		}
		headH[g] = max(label.Height, icon)
	}
	for g := G - 1; g >= 0; g-- {
		left := groupPad + insL[g]
		right := groupPad + insR[g]
		top := groupPad
		if headH[g] > 0 {
			top = max(groupPad, headH[g]+12)
		}
		top += insT[g]
		bottom := groupPad + insB[g]
		if headW[g] > 0 {
			inner := left + right
			for c := c0[g]; c <= c1[g]; c++ {
				between := 0.0
				if c < c1[g] {
					between = colR[c] + gapX[c] + colL[c+1]
				}
				inner += colW[c] + between
			}
			if headW[g]+20 > inner {
				right += math.Ceil(headW[g] + 20 - inner)
			}
		}
		insL[g] = left
		insR[g] = right
		insT[g] = top
		insB[g] = bottom
		colL[c0[g]] = max(colL[c0[g]], left)
		colR[c1[g]] = max(colR[c1[g]], right)
		rowT[r0[g]] = max(rowT[r0[g]], top)
		rowB[r1[g]] = max(rowB[r1[g]], bottom)
		p := grid.groupParent[g]
		if p < 0 {
			continue
		}
		if c0[p] == c0[g] {
			insL[p] = max(insL[p], left)
		}
		if c1[p] == c1[g] {
			insR[p] = max(insR[p], right)
		}
		if r0[p] == r0[g] {
			insT[p] = max(insT[p], top)
		}
		if r1[p] == r1[g] {
			insB[p] = max(insB[p], bottom)
		}
	}

	X := make([]float64, C)
	Y := make([]float64, R)
	innerW := 0.0
	for c := range C {
		X[c] = innerW + colL[c]
		innerW = X[c] + colW[c] + colR[c]
		if c < C-1 {
			innerW += gapX[c]
		}
	}
	innerH := 0.0
	for r := range R {
		Y[r] = innerH + rowT[r]
		innerH = Y[r] + rowH[r] + rowB[r]
		if r < R-1 {
			innerH += gapY[r]
		}
	}
	// What is drawn, which an edge that has to go around a node can push past the grid.
	minX, minY := 0.0, 0.0
	maxX, maxY := innerW, innerH

	left := func(g int) float64 { return X[c0[g]] - insL[g] }
	right := func(g int) float64 { return X[c1[g]] + colW[c1[g]] + insR[g] }
	top := func(g int) float64 { return Y[r0[g]] - insT[g] }
	bottom := func(g int) float64 { return Y[r1[g]] + rowH[r1[g]] + insB[g] }
	centerX := func(i int) float64 { return X[nodeCol[i]] + float64(colW[nodeCol[i]]/2) }

	portOf := func(i, side int, viaGroup bool) port {
		dx := sideDX[side]
		dy := sideDY[side]
		cx := centerX(i)
		y := Y[nodeRow[i]]
		p := port{x: cx, y: y + half, dx: dx, dy: dy, x0: cx - float64(boxW[i]/2), y0: y, x1: cx + float64(boxW[i]/2), y1: y + boxH[i]}
		switch {
		case nodes[i].junction:
			p.x0, p.x1 = cx, cx
			p.y0, p.y1 = p.y, p.y
			p.gap = dot
		case dx != 0:
			p.x = cx + float64(dx*half)
		case dy < 0:
			p.y = y
		default:
			p.y = y + boxH[i]
			if labels[i].Height > 0 {
				p.y += 2
			}
		}
		g := -1
		if viaGroup {
			g = grid.nodeParent[i]
		}
		if g >= 0 {
			p.x0 = left(g)
			p.x1 = right(g)
			p.y0 = top(g)
			p.y1 = bottom(g)
			p.gap = 0
			p.wire = nodes[i].junction
			switch {
			case dx < 0:
				p.x = p.x0
			case dx > 0:
				p.x = p.x1
			case dy < 0:
				p.y = p.y0
			default:
				p.y = p.y1
			}
		}
		return p
	}

	var edgesOut, labelsOut strings.Builder
	// Vertical runs of the edges, to keep group titles out from under them.
	var runs []float64
	trimLength := svg.MarkerTrim("arrow_point")
	for k, edge := range model.edges {
		a := ends[k*2]
		b := ends[k*2+1]
		if a < 0 || b < 0 {
			continue
		}
		s := portOf(a, sideIndex(edge.lhsDir), edge.lhsGroup)
		e := portOf(b, sideIndex(edge.rhsDir), edge.rhsGroup)
		if s.x == e.x && s.y == e.y && s.dx == e.dx {
			// An edge from a side back to the same side: part the two ends so it shows as a loop.
			s.x -= float64(8 * math.Abs(s.dy))
			s.y -= float64(8 * math.Abs(s.dx))
			e.x += float64(8 * math.Abs(s.dy))
			e.y += float64(8 * math.Abs(s.dx))
		}
		routed := route(s, e)
		if s.wire {
			routed = append([]float64{centerX(a), Y[nodeRow[a]] + half}, routed...)
		}
		if e.wire {
			routed = append(routed, centerX(b), Y[nodeRow[b]]+half)
		}
		points := simplify(routed)
		if len(points) < 4 {
			continue
		}
		startTrim, endTrim := 0.0, 0.0
		if edge.lhsInto && !s.wire {
			startTrim = trimLength + s.gap
		}
		if edge.rhsInto && !e.wire {
			endTrim = trimLength + e.gap
		}
		edgesOut.WriteString(`<g class="pele-edge" data-id="` + svg.EscText(edge.lhsID+"-"+edge.rhsID) + `">` +
			`<path d="` + pathData(points, startTrim, endTrim) + `"/>`)
		if edge.lhsInto {
			edgesOut.WriteString(svg.Marker("arrow_point", s.x+float64(s.dx*s.gap), s.y+float64(s.dy*s.gap), -s.dx, -s.dy, "var(--_l)"))
		}
		if edge.rhsInto {
			edgesOut.WriteString(svg.Marker("arrow_point", e.x+float64(e.dx*e.gap), e.y+float64(e.dy*e.gap), -e.dx, -e.dy, "var(--_l)"))
		}
		edgesOut.WriteString("</g>")
		best, bestLength := -1, -1.0
		flat, flatLength := -1, -1.0
		label := edgeLabels[k]
		for i := 0; i < len(points); i += 2 {
			minX = min(minX, points[i]-4)
			maxX = max(maxX, points[i]+4)
			minY = min(minY, points[i+1]-4)
			maxY = max(maxY, points[i+1]+4)
		}
		for i := 0; i+3 < len(points); i += 2 {
			horizontal := math.Abs(points[i+1]-points[i+3]) < 0.01
			length := math.Abs(points[i]-points[i+2]) + math.Abs(points[i+1]-points[i+3])
			if !horizontal {
				runs = append(runs, points[i], min(points[i+1], points[i+3]), max(points[i+1], points[i+3]))
			}
			if length > bestLength {
				best = i
				bestLength = length
			}
			if horizontal && length > flatLength {
				flat = i
				flatLength = length
			}
		}
		if label.Width > 0 {
			at := best
			if flatLength >= label.Width+8 {
				at = flat
			}
			x := (points[at] + points[at+2]) / 2
			y := (points[at+1] + points[at+3]) / 2
			w := label.Width + 8
			halfW := float64(w / 2)
			halfH := float64(label.Height / 2)
			upright := math.Abs(points[at]-points[at+2]) < 0.01
			// On a short run next to a node the label would land on the node, so it moves off to
			// the side.
			for _, p := range [2]port{s, e} {
				if x+halfW <= p.x0 || x-halfW >= p.x1 || y+halfH <= p.y0 || y-halfH >= p.y1 {
					continue
				}
				switch {
				case upright && x >= (p.x0+p.x1)/2:
					x = p.x1 + halfW + 4
				case upright:
					x = p.x0 - halfW - 4
				case y >= (p.y0+p.y1)/2:
					y = p.y1 + halfH + 4
				default:
					y = p.y0 - halfH - 4
				}
			}
			minX = min(minX, x-halfW)
			maxX = max(maxX, x+halfW)
			minY = min(minY, y-halfH)
			maxY = max(maxY, y+halfH)
			labelsOut.WriteString(svg.EdgeLabelSVG(edge.lhsID+"-"+edge.rhsID, label, x, y, "", icons))
		}
	}

	var clusters strings.Builder
	for g, group := range groups {
		x := left(g)
		y := top(g)
		w := right(g) - x
		label := groupLabels[g]
		head := ""
		if headW[g] > 0 {
			// The title starts at the left, or after the edges that come down through that corner.
			band := y + headH[g] + 12
			var crossings []float64
			for i := 0; i < len(runs); i += 3 {
				if runs[i+1] < band && runs[i+2] > y && runs[i] > x && runs[i] < x+w {
					crossings = append(crossings, runs[i])
				}
			}
			slices.SortStableFunc(crossings, compareNumbers)
			crossings = append(crossings, x+w-4)
			start := x + 10
			for _, crossing := range crossings {
				if crossing-6-start >= headW[g] {
					break
				}
				start = max(start, crossing+6)
			}
			if start+headW[g] > x+w-4 {
				start = x + 10
			}
			cy := y + 6 + float64(headH[g]/2)
			textX := start
			if group.icon != "" {
				head += iconSlot(group.icon, start, cy-float64(badge/2), badge, icons)
				textX += badge + 6
			}
			head += svg.LabelSVG(label, textX+float64(label.Width/2), cy, ` class="pele-cluster-label" fill="var(--_m)"`, icons)
		}
		clusters.WriteString(`<g class="pele-cluster" data-id="` + svg.EscText(group.id) + `">` +
			`<rect x="` + svg.Num(x) + `" y="` + svg.Num(y) + `" width="` + svg.Num(w) + `" height="` + svg.Num(bottom(g)-y) +
			`" fill="var(--_a)" fill-opacity="0.5" stroke="var(--_b)"` + svg.RoundedStyle + `/>` +
			head +
			"</g>")
	}

	var nodesOut strings.Builder
	glyph := float64(iconSize * 0.5)
	textSize := max(6, jsmath.Round(size*0.75))
	for i, node := range nodes {
		at := `data-id="` + svg.EscText(node.id) + `" transform="translate(` + svg.Num(centerX(i)) + "," + svg.Num(Y[nodeRow[i]]) + `)"`
		if node.junction {
			nodesOut.WriteString(`<g class="pele-node pele-junction" ` + at + `><circle cy="` + svg.Num(half) + `" r="` + jsnum.Format(dot) + `" fill="var(--_l)"/></g>`)
			continue
		}
		inner := ""
		if node.icon != "" {
			inner = iconSlot(node.icon, float64(-glyph/2), float64((iconSize-glyph)/2), glyph, icons)
		} else if node.iconText != "" {
			label := text.Layout(node.iconText, false, measurer, textSize, iconSize-8, 0, icons)
			lineHeight := label.LineHeight
			if lineHeight == 0 || math.IsNaN(lineHeight) {
				lineHeight = 1
			}
			lines := max(1, math.Floor((iconSize-4)/lineHeight))
			inner = svg.LabelSVG(fit(label, iconSize-8, lines, measurer), 0, half, ` class="pele-icon-text" font-size="`+jsnum.Format(textSize)+`"`, icons)
		}
		nodesOut.WriteString(`<g class="pele-node pele-service" ` + at + `>` +
			`<rect x="` + svg.Num(-half) + `" width="` + svg.Num(iconSize) + `" height="` + svg.Num(iconSize) +
			`" fill="var(--_s)" stroke="var(--_b)"` + svg.RoundedStyle + `/>` +
			inner +
			svg.LabelSVG(labels[i], 0, iconSize+labelGap+float64(labels[i].Height/2), ` class="pele-label"`, icons) +
			"</g>")
	}

	titleRoom := 4000.0
	if tight {
		titleRoom = common.TitleRoom(options)
	}
	title := text.Layout(model.title, false, measurer, size, titleRoom, text.Bold, icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}
	width := math.Ceil(max(maxX-minX, title.Width) + float64(2*pad))
	height := math.Ceil(maxY - minY + titleHeight + float64(2*pad))
	ox := jsmath.Round(float64((width-(maxX-minX))/2) - minX)
	oy := jsmath.Round(pad + titleHeight - minY)
	body := ""
	if clusters.Len() > 0 {
		body += `<g class="pele-clusters">` + clusters.String() + "</g>"
	}
	if edgesOut.Len() > 0 {
		body += `<g class="pele-edges" fill="none" stroke="var(--_l)" stroke-linecap="round" stroke-linejoin="round">` + edgesOut.String() + "</g>"
	}
	if labelsOut.Len() > 0 {
		body += `<g class="pele-edge-labels" font-size="` + jsnum.Format(small) + `">` + labelsOut.String() + "</g>"
	}
	if nodesOut.Len() > 0 {
		body += `<g class="pele-nodes">` + nodesOut.String() + "</g>"
	}
	content := svg.LabelSVG(title, float64(width/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil)
	if body != "" {
		content += `<g transform="translate(` + jsnum.Format(ox) + "," + jsnum.Format(oy) + `)">` + body + "</g>"
	}
	doc := svg.Document("architecture", width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccTitle: model.accTitle, AccDescr: model.accDescr}, content)
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: []core.Link{}}
}

// compareNumbers is (a, b) => a - b, under which NaN is equal to everything.
func compareNumbers(a, b float64) int {
	switch d := a - b; {
	case d < 0:
		return -1
	case d > 0:
		return 1
	}
	return 0
}
