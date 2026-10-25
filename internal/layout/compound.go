package layout

import (
	"context"
	"math"
	"slices"

	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
)

// Dir is the direction ranks run in.
type Dir string

// The directions. A CNode's Dir of "" is upstream's undefined: the group takes its parent's.
const (
	TB Dir = "TB"
	BT Dir = "BT"
	LR Dir = "LR"
	RL Dir = "RL"
)

// Direction is the direction a diagram names, or top to bottom for anything else.
func Direction(dir string) Dir {
	if dir == "BT" || dir == "LR" || dir == "RL" {
		return Dir(dir)
	}
	return TB
}

// CNode is a node of a compound graph. X and Y are outputs, and so are W and H for a group.
type CNode struct {
	W       float64
	H       float64
	Parent  int
	IsGroup bool
	Dir     Dir
	PadX    float64
	PadTop  float64
	// PadBottom is the room under a group's members.
	PadBottom float64
	MinW      float64
	// Declaration order; a group takes the smallest value among its members.
	Seq float64
	// How much of a side its edges may spread along, measured across the flow; 0 keeps them at its middle.
	Span float64
	X    float64
	Y    float64
}

// CEdge is an edge of a compound graph. Route, LabelX and LabelY are outputs.
type CEdge struct {
	Src    int
	Dst    int
	Minlen float64
	LabelW float64
	LabelH float64
	// x, y, axis triples; axis is 0 where the edge runs vertically into the point, 1 horizontally.
	Route  []float64
	LabelX float64
	LabelY float64
}

// CompoundResult is the size of a layout.
type CompoundResult struct {
	Width  float64
	Height float64
}

type side int

const (
	sideTop side = iota
	sideRight
	sideBottom
	sideLeft
)

type portEdge struct {
	e     int
	inner int
	out   bool
	side  side
	// Cross-flow position outside the group from a previous layout; NaN if unknown.
	hint  float64
	px    float64
	py    float64
	route []float64
}

type levelEdge struct {
	e int
	a int
	b int
}

type level struct {
	id     int
	dir    Dir
	items  []int
	edges  []levelEdge
	ports  []*portEdge
	portOf map[int]*portEdge
	routes map[int][]float64
	x      float64
	y      float64
}

const maxPorts = 40000

// Limit repeated layouts to small graphs.
const (
	maxOrderedPorts = 400
	maxOrderedEdges = 500
)

// Crossing tolerance before trying independent group ordering.
const (
	flatSlack = 1.5
	flatSpare = 4
)

type snapshotState struct {
	nodes  []float64
	routes [][]float64
	labels []float64
	levels []float64
	total  CompoundResult
}

func snapshot(nodes []CNode, edges []CEdge, levels []*level, total CompoundResult) snapshotState {
	at := []float64{}
	for i := range nodes {
		c := &nodes[i]
		at = append(at, c.X, c.Y, c.W, c.H)
	}
	labels := []float64{}
	routes := make([][]float64, len(edges))
	for i := range edges {
		labels = append(labels, edges[i].LabelX, edges[i].LabelY)
		routes[i] = edges[i].Route
	}
	places := []float64{}
	for _, l := range levels {
		places = append(places, l.x, l.y)
		for _, port := range l.ports {
			places = append(places, port.px, port.py)
		}
	}
	return snapshotState{nodes: at, routes: routes, labels: labels, levels: places, total: total}
}

func restore(s snapshotState, nodes []CNode, edges []CEdge, levels []*level) CompoundResult {
	for i := range nodes {
		c := &nodes[i]
		c.X = s.nodes[4*i]
		c.Y = s.nodes[4*i+1]
		c.W = s.nodes[4*i+2]
		c.H = s.nodes[4*i+3]
	}
	for i := range edges {
		e := &edges[i]
		e.Route = s.routes[i]
		e.LabelX = s.labels[2*i]
		e.LabelY = s.labels[2*i+1]
	}
	k := 0
	for _, l := range levels {
		l.x = s.levels[k]
		l.y = s.levels[k+1]
		k += 2
		for _, port := range l.ports {
			port.px = s.levels[k]
			port.py = s.levels[k+1]
			k += 2
		}
	}
	return s.total
}

// Ignore crossings within this distance of a shared node.
const fan = 24

// Tolerance for collinear points.
const compoundInLine = 0.5

const (
	curvePieces  = 3
	cornerPieces = 2
	// Allow for a step, an S-curve, or a straight segment plus a rounded corner.
	stepPieces = max(3, curvePieces, cornerPieces+1)
	// Match the renderer's corner handles.
	handle    = 0.62
	maxInsert = 128
)

// Style is how the renderer draws routes between their points.
type Style int

// The route styles.
const (
	StyleCurved Style = iota
	StyleStepped
	StyleLinear
)

// StyleOf is the route style the options draw.
func StyleOf(opt LayeredOptions) Style {
	if opt.Tracks {
		return StyleStepped
	}
	if opt.Straight {
		return StyleLinear
	}
	return StyleCurved
}

// RouteCrossings counts the places where routes drawn in style cross, leaving out those near a
// node that the two edges share.
func RouteCrossings(nodes []CNode, edges []CEdge, style Style) int {
	size := 0
	for i := range edges {
		e := &edges[i]
		if e.Src != e.Dst && len(e.Route) > 3 {
			size += (len(e.Route)/3 - 1) * stepPieces
		}
	}
	ax := make([]float64, size)
	ay := make([]float64, size)
	bx := make([]float64, size)
	by := make([]float64, size)
	low := make([]float64, size)
	high := make([]float64, size)
	owner := make([]int, size)
	// Endpoint flags: 1 for the first segment, 2 for the last.
	ends := make([]int, size)
	count := 0
	top := math.Inf(1)
	bottom := math.Inf(-1)
	fromX := math.NaN()
	fromY := math.NaN()
	toX := math.NaN()
	toY := math.NaN()
	add := func(ei, part int, x0, y0, x1, y1 float64) {
		p := count - 1
		on := x0 == toX && y0 == toY && ((y1 == y0 && fromY == y0) || (x1 == x0 && fromX == x0))
		if on && owner[p] == ei && ends[p] == part && float64((x0-fromX)*(x1-x0))+float64((y0-fromY)*(y1-y0)) > 0 {
			count--
			x0 = fromX
			y0 = fromY
		}
		fromX = x0
		fromY = y0
		toX = x1
		toY = y1
		ends[count] = part
		ax[count] = min(x0, x1)
		bx[count] = max(x0, x1)
		if x0 <= x1 {
			ay[count] = y0
			by[count] = y1
		} else {
			ay[count] = y1
			by[count] = y0
		}
		low[count] = min(y0, y1)
		high[count] = max(y0, y1)
		if low[count] < top {
			top = low[count]
		}
		if high[count] > bottom {
			bottom = high[count]
		}
		owner[count] = ei
		count++
	}
	for ei := range edges {
		e := &edges[ei]
		if e.Src == e.Dst {
			continue
		}
		r := e.Route
		if len(r) < 3 {
			continue
		}
		aligned := func(k int) bool { return math.Abs(r[k]-r[k-3]) < 0.01 || math.Abs(r[k+1]-r[k-2]) < 0.01 }
		length := func(k int) float64 { return math.Abs(r[k]-r[k-3]) + math.Abs(r[k+1]-r[k-2]) }
		turns := func(k int) bool {
			return style == StyleCurved &&
				k > 0 &&
				k < len(r)-3 &&
				aligned(k) &&
				aligned(k+3) &&
				length(k) >= 0.01 &&
				length(k+3) >= 0.01 &&
				(math.Abs(r[k]-r[k-3]) < 0.01) != (math.Abs(r[k+3]-r[k]) < 0.01)
		}
		atX := r[0]
		atY := r[1]
		for k := 3; k < len(r); k += 3 {
			part := 0
			if k == 3 {
				part |= 1
			}
			if k == len(r)-3 {
				part |= 2
			}
			x0 := atX
			y0 := atY
			x1 := r[k]
			y1 := r[k+1]
			atX = x1
			atY = y1
			if aligned(k) || style == StyleLinear {
				if !turns(k) {
					add(ei, part, x0, y0, x1, y1)
					continue
				}
				// Adjacent corners share a leg, so each gets half.
				before := length(k)
				if turns(k - 3) {
					before = length(k) / 2
				}
				after := length(k + 3)
				if turns(k + 3) {
					after = length(k+3) / 2
				}
				ux := (x1 - r[k-3]) / length(k)
				uy := (y1 - r[k-2]) / length(k)
				vx := (r[k+3] - x1) / length(k+3)
				vy := (r[k+4] - y1) / length(k+3)
				sx := x1 - float64(ux*before)
				sy := y1 - float64(uy*before)
				add(ei, part, x0, y0, sx, sy)
				c1x := x1 - float64(ux*before*(1-handle))
				c1y := y1 - float64(uy*before*(1-handle))
				c2x := x1 + float64(vx*after*(1-handle))
				c2y := y1 + float64(vy*after*(1-handle))
				atX = x1 + float64(vx*after)
				atY = y1 + float64(vy*after)
				turn := part
				if k+3 == len(r)-3 {
					turn |= 2
				}
				px := sx
				py := sy
				for i := 1; i <= cornerPieces; i++ {
					t := float64(float64(i) / cornerPieces)
					u := 1 - t
					x := float64(float64(float64(u*u*u*sx)+float64(3*u*u*t*c1x))+float64(3*u*t*t*c2x)) + float64(t*t*t*atX)
					y := float64(float64(float64(u*u*u*sy)+float64(3*u*u*t*c1y))+float64(3*u*t*t*c2y)) + float64(t*t*t*atY)
					add(ei, turn, px, py, x, y)
					px = x
					py = y
				}
				continue
			}
			if style == StyleStepped {
				down := r[k+2] == 0
				mx := float64((x0 + x1) / 2)
				my := y0
				if down {
					mx = x0
					my = float64((y0 + y1) / 2)
				}
				nx, ny := mx, y1
				if down {
					nx, ny = x1, my
				}
				add(ei, part, x0, y0, mx, my)
				add(ei, part, mx, my, nx, ny)
				add(ei, part, nx, ny, x1, y1)
				continue
			}
			// Approximate the rendered S-curve with straight segments.
			down := r[k+2] == 0
			c := Ease(y1-y0, x1-x0)
			if down {
				c = Ease(x1-x0, y1-y0)
			}
			px := x0
			py := y0
			for i := 1; i <= curvePieces; i++ {
				t := float64(i) / curvePieces
				u := 1 - t
				across := t * t * (3 - float64(2*t))
				along := float64(float64(float64(3*c*t*u*u)+float64(3*(1-c)*t*t*u)) + float64(t*t*t))
				fx, fy := along, across
				if down {
					fx, fy = across, along
				}
				x := x0 + float64((x1-x0)*fx)
				y := y0 + float64((y1-y0)*fy)
				add(ei, part, px, py, x, y)
				px = x
				py = y
			}
		}
	}
	// Count each pair only in the first band shared by both segments.
	bands := 1.0
	scale := 0.0
	if bottom > top {
		bands = jsmath.Round(math.Sqrt(float64(count)))
		scale = bands / (bottom - top)
	}
	bandOf := func(y float64) float64 { return min(bands-1, math.Floor((y-top)*scale)) }
	nb := int(bands)
	first := make([]int, count)
	bandStart := make([]int, nb+1)
	for p := 0; p < count; p++ {
		first[p] = toInt32(bandOf(low[p]))
		for b, last := first[p], bandOf(high[p]); float64(b) <= last; b++ {
			bandStart[b+1]++
		}
	}
	for b := 0; b < nb; b++ {
		bandStart[b+1] += bandStart[b]
	}
	fill := slices.Clone(bandStart[:nb])
	inBand := make([]int, bandStart[nb])
	for p := 0; p < count; p++ {
		for b, last := first[p], bandOf(high[p]); float64(b) <= last; b++ {
			inBand[fill[b]] = p
			fill[b]++
		}
	}
	// Edges merging into a shared endpoint run do not count as crossings.
	runs := make([]float64, len(edges)*8)
	inLineAt := func(r []float64, a, b, c int) bool {
		return (math.Abs(r[a]-r[b]) < compoundInLine && math.Abs(r[a]-r[c]) < compoundInLine) ||
			(math.Abs(r[a+1]-r[b+1]) < compoundInLine && math.Abs(r[a+1]-r[c+1]) < compoundInLine)
	}
	for ei := range edges {
		r := edges[ei].Route
		if len(r) < 6 {
			continue
		}
		k := 3
		for k+3 < len(r) && inLineAt(r, 0, k, k+3) {
			k += 3
		}
		last := len(r) - 3
		j := last - 3
		for j >= 3 && inLineAt(r, last, j, j-3) {
			j -= 3
		}
		copy(runs[ei*8:], []float64{r[0], r[1], r[k], r[k+1], r[j], r[j+1], r[last], r[last+1]})
	}
	onRun := func(ei, end int, x, y float64) bool {
		o := ei*8 + 4
		if end == 1 {
			o = ei * 8
		}
		x0 := runs[o]
		y0 := runs[o+1]
		dx := runs[o+2] - x0
		dy := runs[o+3] - y0
		length := float64(dx*dx) + float64(dy*dy)
		t := 0.0
		if length > 0 {
			t = max(0, min(1, (float64((x-x0)*dx)+float64((y-y0)*dy))/length))
		}
		return jsmath.Hypot(x-x0-float64(t*dx), y-y0-float64(t*dy)) < compoundInLine
	}
	total := 0
	for band := 0; band < nb; band++ {
		bandEnd := bandStart[band+1]
		if bandEnd-bandStart[band] > maxInsert {
			slices.SortStableFunc(inBand[bandStart[band]:bandEnd], func(p, q int) int {
				if c := sign(ax[p] - ax[q]); c != 0 {
					return c
				}
				return p - q
			})
		} else {
			for i := bandStart[band] + 1; i < bandEnd; i++ {
				p := inBand[i]
				j := i
				for ; j > bandStart[band] && ax[inBand[j-1]] > ax[p]; j-- {
					inBand[j] = inBand[j-1]
				}
				inBand[j] = p
			}
		}
		for s := bandStart[band]; s < bandEnd; s++ {
			p := inBand[s]
			dx := bx[p] - ax[p]
			dy := by[p] - ay[p]
			here := first[p] == band
			for t := s + 1; t < bandEnd; t++ {
				q := inBand[t]
				if ax[q] > bx[p] {
					break
				}
				if high[q] < low[p] || low[q] > high[p] || owner[p] == owner[q] || (!here && first[q] != band) {
					continue
				}
				d1 := float64(dx*(ay[q]-ay[p])) - float64(dy*(ax[q]-ax[p]))
				d2 := float64(dx*(by[q]-ay[p])) - float64(dy*(bx[q]-ax[p]))
				// Treat points on the line as one side to count segment-boundary crossings once.
				if (d1 > 0) == (d2 > 0) {
					continue
				}
				ex := bx[q] - ax[q]
				ey := by[q] - ay[q]
				d3 := float64(ex*(ay[p]-ay[q])) - float64(ey*(ax[p]-ax[q]))
				d4 := float64(ex*(by[p]-ay[q])) - float64(ey*(bx[p]-ax[q]))
				if (d3 > 0) == (d4 > 0) {
					continue
				}
				ep := &edges[owner[p]]
				eq := &edges[owner[q]]
				at := d3 / (d3 - d4)
				x := ax[p] + float64(at*dx)
				y := ay[p] + float64(at*dy)
				// Exclude fan-in and fan-out at shared nodes.
				fanned := func(v, pe, qe int) bool {
					return (ends[p]&pe != 0 && ends[q]&qe != 0) ||
						(math.Abs(x-nodes[v].X) < float64(nodes[v].W/2)+fan && math.Abs(y-nodes[v].Y) < float64(nodes[v].H/2)+fan) ||
						(onRun(owner[p], pe, x, y) && onRun(owner[q], qe, x, y))
				}
				if ep.Src == eq.Src && fanned(ep.Src, 1, 1) {
					continue
				}
				if ep.Src == eq.Dst && fanned(ep.Src, 1, 2) {
					continue
				}
				if ep.Dst == eq.Src && fanned(ep.Dst, 2, 1) {
					continue
				}
				if ep.Dst == eq.Dst && fanned(ep.Dst, 2, 2) {
					continue
				}
				total++
			}
		}
	}
	return total
}

// ShiftLayout moves a laid-out graph by (ox, oy).
func ShiftLayout(nodes []CNode, edges []CEdge, ox, oy float64) {
	for i := range nodes {
		c := &nodes[i]
		c.X += ox
		c.Y += oy
	}
	for i := range edges {
		e := &edges[i]
		for k := 0; k < len(e.Route); k += 3 {
			e.Route[k] += ox
			e.Route[k+1] += oy
		}
		e.LabelX += ox
		e.LabelY += oy
	}
}

// NewCNode is upstream's cnode(); a parent of -1 is the root.
func NewCNode(w, h float64, parent int) CNode {
	return CNode{W: w, H: h, Parent: parent}
}

// sameValue is JS's Object.is.
func sameValue(a, b float64) bool {
	if math.IsNaN(a) {
		return math.IsNaN(b)
	}
	return a == b && math.Signbit(a) == math.Signbit(b)
}

func startSide(dir Dir) side {
	switch dir {
	case TB:
		return sideTop
	case BT:
		return sideBottom
	case LR:
		return sideLeft
	}
	return sideRight
}

func opposite(s side) side {
	return (s + 2) & 3
}

// CompoundLayout lays out a graph whose nodes may be nested in groups. Each group is laid out on
// its own, in its own direction, and then placed in its parent as a single box. An edge that
// crosses a group's border passes through a port on the side facing its other end.
func CompoundLayout(ctx context.Context, nodes []CNode, edges []CEdge, rootDir Dir, opt LayeredOptions) CompoundResult {
	n := len(nodes)
	levelOf := map[int]*level{}
	order := []*level{}
	makeLevel := func(id int, dir Dir) *level {
		l := &level{id: id, dir: dir, portOf: map[int]*portEdge{}, routes: map[int][]float64{}}
		levelOf[id] = l
		return l
	}
	root := makeLevel(-1, rootDir)

	children := make([][]int, n)
	rootItems := []int{}
	for i := 0; i < n; i++ {
		p := nodes[i].Parent
		if p >= 0 && p < n && p != i && nodes[p].IsGroup {
			children[p] = append(children[p], i)
		} else {
			nodes[i].Parent = -1
			rootItems = append(rootItems, i)
		}
	}
	root.items = rootItems
	order = append(order, root)
	depth := make([]int, n)
	placed := make([]uint8, n)
	descend := func(from int) {
		for q := from; q < len(order); q++ {
			checkCancel(ctx)
			l := order[q]
			for _, item := range l.items {
				placed[item] = 1
				if l.id < 0 {
					depth[item] = 0
				} else {
					depth[item] = depth[l.id] + 1
				}
				if nodes[item].IsGroup {
					dir := nodes[item].Dir
					if dir == "" {
						dir = l.dir
					}
					child := makeLevel(item, dir)
					child.items = children[item]
					order = append(order, child)
				}
			}
		}
	}
	descend(0)
	// Groups that contain each other are unreachable from the root; lift them out.
	for i := 0; i < n; i++ {
		if placed[i] != 0 {
			continue
		}
		former := nodes[i].Parent
		kept := []int{}
		for _, c := range children[former] {
			if c != i {
				kept = append(kept, c)
			}
		}
		children[former] = kept
		nodes[i].Parent = -1
		root.items = append(root.items, i)
		placed[i] = 1
		depth[i] = 0
		if nodes[i].IsGroup {
			from := len(order)
			dir := nodes[i].Dir
			if dir == "" {
				dir = rootDir
			}
			child := makeLevel(i, dir)
			child.items = children[i]
			order = append(order, child)
			descend(from)
		}
	}

	for q := len(order) - 1; q > 0; q-- {
		l := order[q]
		seq := math.Inf(1)
		for _, item := range l.items {
			if nodes[item].Seq < seq {
				seq = nodes[item].Seq
			}
		}
		if !math.IsInf(seq, 1) {
			nodes[l.id].Seq = seq
		}
	}
	for _, l := range order {
		slices.SortStableFunc(l.items, func(a, b int) int {
			if c := sign(nodes[a].Seq - nodes[b].Seq); c != 0 {
				return c
			}
			return a - b
		})
	}

	// Every group border an edge crosses costs a port. Past the budget, an edge is drawn between
	// the groups that hold its ends, at the level they share.
	ports := 0
	for i := range edges {
		e := &edges[i]
		a := e.Src
		b := e.Dst
		crossed := 0
		for ; depth[a] > depth[b]; crossed++ {
			a = nodes[a].Parent
		}
		for ; depth[b] > depth[a]; crossed++ {
			b = nodes[b].Parent
		}
		for ; a != b && nodes[a].Parent != nodes[b].Parent; crossed += 2 {
			a = nodes[a].Parent
			b = nodes[b].Parent
		}
		if a == b {
			continue
		}
		if ports+crossed > maxPorts {
			e.Src = a
			e.Dst = b
		} else {
			ports += crossed
		}
	}

	loops := []int{}
	stray := []int{}
	for ei := range edges {
		checkCancel(ctx)
		e := &edges[ei]
		e.Route = []float64{}
		if e.Src == e.Dst {
			loops = append(loops, ei)
			continue
		}
		a := e.Src
		b := e.Dst
		aPath := []int{}
		bPath := []int{}
		for depth[a] > depth[b] {
			aPath = append(aPath, a)
			a = nodes[a].Parent
		}
		for depth[b] > depth[a] {
			bPath = append(bPath, b)
			b = nodes[b].Parent
		}
		if a == b {
			stray = append(stray, ei)
			continue
		}
		for nodes[a].Parent != nodes[b].Parent {
			aPath = append(aPath, a)
			bPath = append(bPath, b)
			a = nodes[a].Parent
			b = nodes[b].Parent
		}
		shared := levelOf[nodes[a].Parent]
		shared.edges = append(shared.edges, levelEdge{e: ei, a: a, b: b})
		for _, path := range [2]struct {
			items []int
			out   bool
		}{{aPath, true}, {bPath, false}} {
			for _, inner := range path.items {
				l := levelOf[nodes[inner].Parent]
				port := &portEdge{e: ei, inner: inner, out: path.out, side: sideTop, hint: math.NaN(), route: []float64{}}
				l.ports = append(l.ports, port)
				key := ei * 2
				if !path.out {
					key++
				}
				l.portOf[key] = port
			}
		}
	}

	local := make([]int, n)
	endpointBelow := func(item int, e *CEdge, out bool) bool {
		if out {
			return e.Src != item
		}
		return e.Dst != item
	}
	portKey := func(ei int, out bool) int {
		if out {
			return ei * 2
		}
		return ei*2 + 1
	}

	// Set from a flat layout of the whole drawing, below.
	key := make([]float64, n)
	for i := range key {
		key[i] = math.NaN()
	}
	keyed := false
	var farHint func(l *level, port *portEdge) float64

	build := func(l *level, sized bool) ([]LNode, []LEdge, []int) {
		vertical := l.dir == TB || l.dir == BT
		start := startSide(l.dir)
		lnodes := make([]LNode, 0, len(l.items))
		ledges := make([]LEdge, 0, len(l.edges)+len(l.ports))
		for i, item := range l.items {
			local[item] = i
			node := &nodes[item]
			ln := NewLNode(0, 0, KindNode)
			if sized {
				if vertical {
					ln = NewLNode(node.W, node.H, KindNode)
				} else {
					ln = NewLNode(node.H, node.W, KindNode)
				}
			}
			if node.IsGroup {
				ln.Span = min(node.Span, ln.W-float64(2*node.PadX))
			} else {
				ln.Span = node.Span
			}
			if keyed {
				ln.Key = key[item]
			}
			lnodes = append(lnodes, ln)
		}
		held := func(item, ei int, out bool) bool {
			return nodes[item].IsGroup && endpointBelow(item, &edges[ei], out)
		}
		offset := func(item, ei int, out bool) float64 {
			if !sized || !nodes[item].IsGroup || !endpointBelow(item, &edges[ei], out) {
				return 0
			}
			port := levelOf[item].portOf[portKey(ei, out)]
			if vertical {
				return port.px - float64(nodes[item].W/2)
			}
			return port.py - float64(nodes[item].H/2)
		}
		for _, le := range l.edges {
			e := &edges[le.e]
			edge := NewLEdge(local[le.a], local[le.b], e.Minlen, 0, 0)
			if sized {
				if vertical {
					edge.LabelW, edge.LabelH = e.LabelW, e.LabelH
				} else {
					edge.LabelW, edge.LabelH = e.LabelH, e.LabelW
				}
				edge.TailDx = offset(le.a, le.e, true)
				edge.HeadDx = offset(le.b, le.e, false)
				edge.TailFixed = held(le.a, le.e, true)
				edge.HeadFixed = held(le.b, le.e, false)
			}
			ledges = append(ledges, edge)
		}
		dummies := fillInt(len(l.ports), -1)
		for i, port := range l.ports {
			if port.side == start || port.side == opposite(start) {
				d := len(lnodes)
				kind := KindEndPort
				if port.side == start {
					kind = KindStartPort
				}
				ln := NewLNode(0, 0, kind)
				if keyed {
					if port.out {
						ln.Key = key[edges[port.e].Dst]
					} else {
						ln.Key = key[edges[port.e].Src]
					}
				}
				if math.IsNaN(port.hint) && keyed && farHint != nil {
					ln.Hint = farHint(l, port)
				} else {
					ln.Hint = port.hint
				}
				lnodes = append(lnodes, ln)
				dummies[i] = d
				var edge LEdge
				if port.out {
					edge = NewLEdge(local[port.inner], d, 1, 0, 0)
					edge.TailDx = offset(port.inner, port.e, true)
					edge.TailFixed = held(port.inner, port.e, true)
				} else {
					edge = NewLEdge(d, local[port.inner], 1, 0, 0)
					edge.HeadDx = offset(port.inner, port.e, false)
					edge.HeadFixed = held(port.inner, port.e, false)
				}
				ledges = append(ledges, edge)
			} else if port.side == sideTop || port.side == sideLeft {
				lnodes[local[port.inner]].Pin = -1
			} else {
				lnodes[local[port.inner]].Pin = 1
			}
		}
		return lnodes, ledges, dummies
	}

	ordered := ports > 1 && ports <= maxOrderedPorts && len(edges) <= maxOrderedEdges

	// Use a flat layout to coordinate node and port order across groups.
	flatCrossings := 0
	flat := len(order) > 1 && ordered
	for _, l := range order {
		if l.dir != rootDir {
			flat = false
		}
	}
	if flat {
		vertical := rootDir == TB || rootDir == BT
		leaf := fillInt(n, -1)
		group := fillInt(n, -1)
		lnodes := []LNode{}
		of := []int{}
		parent := []int{}
		for i := 0; i < n; i++ {
			if !nodes[i].IsGroup {
				continue
			}
			group[i] = len(parent)
			parent = append(parent, -1)
		}
		for i := 0; i < n; i++ {
			p := nodes[i].Parent
			if nodes[i].IsGroup {
				if p >= 0 {
					parent[group[i]] = group[p]
				} else {
					parent[group[i]] = -1
				}
			} else {
				leaf[i] = len(lnodes)
				if vertical {
					lnodes = append(lnodes, NewLNode(nodes[i].W, nodes[i].H, KindNode))
				} else {
					lnodes = append(lnodes, NewLNode(nodes[i].H, nodes[i].W, KindNode))
				}
				if p >= 0 {
					of = append(of, group[p])
				} else {
					of = append(of, -1)
				}
			}
		}
		ledges := []LEdge{}
		ledgeOf := fillInt(len(edges), -1)
		for ei := range edges {
			e := &edges[ei]
			if e.Src == e.Dst || leaf[e.Src] < 0 || leaf[e.Dst] < 0 {
				continue
			}
			ledgeOf[ei] = len(ledges)
			if vertical {
				ledges = append(ledges, NewLEdge(leaf[e.Src], leaf[e.Dst], e.Minlen, e.LabelW, e.LabelH))
			} else {
				ledges = append(ledges, NewLEdge(leaf[e.Src], leaf[e.Dst], e.Minlen, e.LabelH, e.LabelW))
			}
		}
		flatOpt := opt
		flatOpt.Groups = &Groups{Of: of, Parent: parent}
		flatOpt.OrderOnly = true
		flatLayout := Layered(ctx, lnodes, ledges, flatOpt)
		flatCrossings = min(flatLayout.Crossings, flatLayout.Declared)
		sum := make([]float64, n)
		count := make([]int, n)
		// Last rank occupied by each group.
		last := fillInt(n, -1)
		for i := 0; i < n; i++ {
			if leaf[i] < 0 {
				continue
			}
			key[i] = lnodes[leaf[i]].X
			for p := nodes[i].Parent; p >= 0; p = nodes[p].Parent {
				sum[p] += key[i]
				count[p]++
				last[p] = max(last[p], lnodes[leaf[i]].Rank)
			}
		}
		for i := 0; i < n; i++ {
			if count[i] > 0 {
				key[i] = sum[i] / float64(count[i])
			}
		}
		// Order outgoing ports at the group's last rank; break ties using the next rank.
		farHint = func(l *level, port *portEdge) float64 {
			le := ledgeOf[port.e]
			if le < 0 || l.id < 0 || !port.out || port.side != opposite(startSide(l.dir)) {
				return math.NaN()
			}
			points := ledges[le].Points
			inside := key[edges[port.e].Src]
			outside := 0.0
			for k := 0; k < len(points); k += 2 {
				if points[k] == float64(last[l.id]) {
					inside = points[k+1]
				}
				if points[k] == float64(last[l.id]+1) {
					outside = points[k+1]
				}
			}
			return inside + float64(outside*1e-6)
		}
	}

	// Reuse unchanged levels across layout attempts.
	type laidOut struct {
		given  []float64
		lnodes []LNode
		ledges []LEdge
		size   LayeredResult
	}
	laid := map[int][]*laidOut{}
	layOut := func(l *level, lnodes []LNode, ledges []LEdge) ([]LNode, []LEdge, LayeredResult) {
		if !ordered {
			return lnodes, ledges, Layered(ctx, lnodes, ledges, opt)
		}
		given := []float64{}
		for i := range lnodes {
			ln := &lnodes[i]
			given = append(given, ln.W, ln.H, float64(ln.Kind), ln.Pin, ln.Hint, ln.Key, ln.Span)
		}
		for i := range ledges {
			le := &ledges[i]
			given = append(given, float64(le.Tail), float64(le.Head), le.Minlen, le.LabelW, le.LabelH, le.TailDx, le.HeadDx,
				float64(b2u(le.TailFixed)), float64(b2u(le.HeadFixed)))
		}
		for _, p := range laid[l.id] {
			same := len(p.given) == len(given)
			for i := 0; same && i < len(given); i++ {
				same = sameValue(p.given[i], given[i])
			}
			if same {
				return p.lnodes, p.ledges, p.size
			}
		}
		done := &laidOut{given: given, lnodes: lnodes, ledges: ledges, size: Layered(ctx, lnodes, ledges, opt)}
		laid[l.id] = append(laid[l.id], done)
		return done.lnodes, done.ledges, done.size
	}

	total := CompoundResult{}
	place := func() {
		// Rank parents first to determine which group sides edges cross.
		for _, l := range order {
			checkCancel(ctx)
			for _, port := range l.ports {
				if nodes[port.inner].IsGroup && endpointBelow(port.inner, &edges[port.e], port.out) {
					levelOf[port.inner].portOf[portKey(port.e, port.out)].side = port.side
				}
			}
			if len(l.edges) == 0 {
				continue
			}
			needed := false
			for _, le := range l.edges {
				e := &edges[le.e]
				if (nodes[le.a].IsGroup && e.Src != le.a) || (nodes[le.b].IsGroup && e.Dst != le.b) {
					needed = true
				}
			}
			if !needed {
				continue
			}
			lnodes, ledges, _ := build(l, false)
			orient(ctx, lnodes, ledges)
			rank(ctx, lnodes, ledges, 1)
			start := startSide(l.dir)
			for i, le := range l.edges {
				e := &edges[le.e]
				aAbove := !ledges[i].Reversed
				if nodes[le.a].IsGroup && e.Src != le.a {
					s := start
					if aAbove {
						s = opposite(start)
					}
					levelOf[le.a].portOf[le.e*2].side = s
				}
				if nodes[le.b].IsGroup && e.Dst != le.b {
					s := opposite(start)
					if aAbove {
						s = start
					}
					levelOf[le.b].portOf[le.e*2+1].side = s
				}
			}
		}

		// Lay out children first so their sizes are known.
		for q := len(order) - 1; q >= 0; q-- {
			l := order[q]
			var group *CNode
			if l.id >= 0 {
				group = &nodes[l.id]
			}
			vertical := l.dir == TB || l.dir == BT
			flip := l.dir == BT || l.dir == RL
			lnodes, ledges, dummies := build(l, true)
			lnodes, ledges, size := layOut(l, lnodes, ledges)
			contentW, contentH := size.Height, size.Width
			if vertical {
				contentW, contentH = size.Width, size.Height
			}
			var padX, padTop, padBottom, minW float64
			if group != nil {
				padX, padTop, padBottom, minW = group.PadX, group.PadTop, group.PadBottom, group.MinW
			}
			boxW := max(contentW+float64(2*padX), minW)
			boxH := contentH + padTop + padBottom
			offX := float64((boxW - contentW) / 2)
			offY := padTop
			tx := func(lx, ly float64) float64 {
				if vertical {
					return offX + lx
				}
				if flip {
					return offX + (size.Height - ly)
				}
				return offX + ly
			}
			ty := func(lx, ly float64) float64 {
				if !vertical {
					return offY + lx
				}
				if flip {
					return offY + (size.Height - ly)
				}
				return offY + ly
			}
			axis := 1.0
			if vertical {
				axis = 0
			}
			convert := func(points []float64) []float64 {
				out := make([]float64, 0, len(points)/2*3)
				for i := 0; i < len(points); i += 2 {
					out = append(out, tx(points[i], points[i+1]), ty(points[i], points[i+1]), axis)
				}
				return out
			}

			for i, item := range l.items {
				nodes[item].X = tx(lnodes[i].X, lnodes[i].Y)
				nodes[item].Y = ty(lnodes[i].X, lnodes[i].Y)
			}
			for i, le := range l.edges {
				edge := &ledges[i]
				l.routes[le.e] = convert(edge.Points)
				e := &edges[le.e]
				if e.LabelW > 0 {
					e.LabelX = tx(edge.LabelX, edge.LabelY)
					e.LabelY = ty(edge.LabelX, edge.LabelY)
				}
			}
			k := len(l.edges)
			for i, port := range l.ports {
				item := &nodes[port.inner]
				if dummies[i] >= 0 {
					route := convert(ledges[k].Points)
					k++
					at := 0
					if port.out {
						at = len(route) - 3
					}
					bx := route[at]
					by := route[at+1]
					switch port.side {
					case sideTop:
						by = 0
					case sideBottom:
						by = boxH
					case sideLeft:
						bx = 0
					default:
						bx = boxW
					}
					if port.out {
						route = append(route, bx, by, axis)
					} else {
						route = append([]float64{bx, by, axis}, route...)
					}
					port.px = bx
					port.py = by
					port.route = route
					continue
				}
				ix := item.X
				iy := item.Y
				if item.IsGroup && endpointBelow(port.inner, &edges[port.e], port.out) {
					inner := levelOf[port.inner].portOf[portKey(port.e, port.out)]
					ix += inner.px - float64(item.W/2)
					iy += inner.py - float64(item.H/2)
				} else {
					switch port.side {
					case sideTop:
						iy -= float64(item.H / 2)
					case sideBottom:
						iy += float64(item.H / 2)
					case sideLeft:
						ix -= float64(item.W / 2)
					default:
						ix += float64(item.W / 2)
					}
				}
				bx := ix
				by := iy
				switch port.side {
				case sideTop:
					by = 0
				case sideBottom:
					by = boxH
				case sideLeft:
					bx = 0
				default:
					bx = boxW
				}
				a := 1.0
				if port.side == sideTop || port.side == sideBottom {
					a = 0
				}
				port.px = bx
				port.py = by
				if port.out {
					port.route = []float64{ix, iy, a, bx, by, a}
				} else {
					port.route = []float64{bx, by, a, ix, iy, a}
				}
			}

			if group != nil {
				group.W = boxW
				group.H = boxH
			} else {
				total = CompoundResult{Width: boxW, Height: boxH}
			}
		}

		for _, l := range order {
			ox := l.x
			oy := l.y
			for _, item := range l.items {
				node := &nodes[item]
				node.X += ox
				node.Y += oy
				if node.IsGroup {
					child := levelOf[item]
					child.x = node.X - float64(node.W/2)
					child.y = node.Y - float64(node.H/2)
				}
			}
		}

		appendRoute := func(route, part []float64, ox, oy float64) []float64 {
			for i := 0; i < len(part); i += 3 {
				x := part[i] + ox
				y := part[i+1] + oy
				n := len(route)
				if n >= 3 && math.Abs(route[n-3]-x) < 0.01 && math.Abs(route[n-2]-y) < 0.01 {
					continue
				}
				route = append(route, x, y, part[i+2])
			}
			return route
		}

		for _, l := range order {
			checkCancel(ctx)
			for _, le := range l.edges {
				e := &edges[le.e]
				route := []float64{}
				outward := []*level{}
				for item := e.Src; nodes[item].Parent != l.id; item = nodes[item].Parent {
					outward = append(outward, levelOf[nodes[item].Parent])
				}
				for _, lv := range outward {
					route = appendRoute(route, lv.portOf[le.e*2].route, lv.x, lv.y)
				}
				route = appendRoute(route, l.routes[le.e], l.x, l.y)
				inward := []*level{}
				for item := e.Dst; nodes[item].Parent != l.id; item = nodes[item].Parent {
					inward = append(inward, levelOf[nodes[item].Parent])
				}
				for i := len(inward) - 1; i >= 0; i-- {
					route = appendRoute(route, inward[i].portOf[le.e*2+1].route, inward[i].x, inward[i].y)
				}
				e.Route = route
				if e.LabelW > 0 {
					e.LabelX += l.x
					e.LabelY += l.y
				}
			}
		}
	}
	// The flat layout ignores group boxes, so independent ordering may yield fewer crossings.
	style := StyleOf(opt)
	keyed = flat
	place()
	crossings := -1
	if flat {
		crossings = RouteCrossings(nodes, edges, style)
		if float64(crossings) > float64(float64(flatCrossings)*flatSlack)+flatSpare {
			first := snapshot(nodes, edges, order, total)
			keyed = false
			place()
			own := RouteCrossings(nodes, edges, style)
			if own >= crossings {
				keyed = true
				total = restore(first, nodes, edges, order)
			} else {
				crossings = own
			}
		}
	}

	// Refine port order using routes outside each group. Skip this if independent ordering won;
	// it rarely helps enough to justify another layout.
	if keyed == flat && ordered {
		again := false
		for _, l := range order {
			if l.id < 0 {
				continue
			}
			box := &nodes[l.id]
			vertical := l.dir == TB || l.dir == BT
			end := opposite(startSide(l.dir))
			seen := [][2]float64{}
			for _, port := range l.ports {
				if port.side != end {
					continue
				}
				// Skip the straight run out of the port to find where the edge turns.
				route := edges[port.e].Route
				bx := l.x + port.px
				by := l.y + port.py
				at := -1
				for k := 0; k < len(route) && (at < 0 || port.out); k += 3 {
					if math.Abs(route[k]-bx) < 0.01 && math.Abs(route[k+1]-by) < 0.01 {
						at = k
					}
				}
				if at < 0 {
					continue
				}
				a := 1
				if vertical {
					a = 0
				}
				stepBy := -3
				if port.out {
					stepBy = 3
				}
				next := at + stepBy
				for next >= 0 && next < len(route) && math.Abs(route[next+a]-route[at+a]) < compoundInLine {
					next += stepBy
				}
				if next < 0 || next >= len(route) {
					continue
				}
				if vertical {
					port.hint = route[next+a] - (box.X - float64(box.W/2))
					seen = append(seen, [2]float64{port.px, port.hint})
				} else {
					port.hint = route[next+a] - (box.Y - float64(box.H/2))
					seen = append(seen, [2]float64{port.py, port.hint})
				}
			}
			slices.SortStableFunc(seen, func(p, q [2]float64) int { return sign(p[0] - q[0]) })
			for i := 1; i < len(seen); i++ {
				if seen[i-1][1] > seen[i][1] {
					again = true
				}
			}
		}
		if again {
			before := crossings
			if crossings < 0 {
				before = RouteCrossings(nodes, edges, style)
			}
			if before > 0 {
				first := snapshot(nodes, edges, order, total)
				place()
				if RouteCrossings(nodes, edges, style) >= before {
					total = restore(first, nodes, edges, order)
				}
			}
		}
	}

	for _, ei := range stray {
		e := &edges[ei]
		a := &nodes[e.Src]
		b := &nodes[e.Dst]
		e.Route = []float64{a.X, a.Y, 0, b.X, b.Y, 0}
		e.LabelX = float64((a.X + b.X) / 2)
		e.LabelY = float64((a.Y + b.Y) / 2)
	}
	for _, ei := range loops {
		e := &edges[ei]
		node := &nodes[e.Src]
		e.LabelX = node.X + float64(node.W/2) + 28 + float64(e.LabelW/2)
		e.LabelY = node.Y
	}

	return total
}
