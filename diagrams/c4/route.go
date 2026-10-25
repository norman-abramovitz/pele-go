package c4

import (
	"math"

	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
)

// box is a laid out shape or boundary: top-left corner, size, and the shared shape it is
// drawn as. Boxes are compared by identity, as upstream's objects are.
type box struct {
	x, y, w, h float64
	form       string
}

type route struct {
	d string
	// Where the markers go: the tip, and the way it points.
	sx, sy, sdx, sdy float64
	ex, ey, edx, edy float64
	// The stretch the label may sit on.
	x0, y0, x1, y1 float64
	// The side of that stretch the label is pushed to, as a unit vector; zero centers it on
	// the stretch.
	nx, ny float64
	// When set, the label stays over the line and moves that way by at most this much.
	lean float64
	// The widest the label should be.
	room float64
	// The corners of the line, as x, y pairs.
	points []float64
}

const (
	loop     = 28
	lane     = 20
	laneStep = 7
	corner   = 8
	// A wide label on an upright lane leans this far out from it, to the middle of the gap
	// between columns.
	lean = 30
	// Label widths that fit the gap between two columns, and where there is room to spare.
	narrow = 88
	wide   = 160
	// The space a label's background adds to the width of its text.
	labelPad = 8
	// The shared person shape: a head of radius 13 centered 14 below the top, over a body
	// that starts 26 below it.
	head = 26
)

// The cap radii DrawShape uses for the two cylinders.
func cylRy(w float64) float64   { return min(12, max(6, w/14)) }
func queueRx(h float64) float64 { return min(12, h/4) }

// truthy is a number's truth in JS: neither zero nor NaN.
func truthy(v float64) bool { return v != 0 && v == v }

// hitRect is how far along the segment from (px, py) by (dx, dy) it first meets the
// rectangle; Infinity when it misses.
func hitRect(x0, y0, x1, y1, px, py, dx, dy float64) float64 {
	enter := 0.0
	leave := 1.0
	if dx == 0 {
		if px < x0 || px > x1 {
			return math.Inf(1)
		}
	} else {
		a := (x0 - px) / dx
		b := (x1 - px) / dx
		enter = max(enter, min(a, b))
		leave = min(leave, max(a, b))
	}
	if dy == 0 {
		if py < y0 || py > y1 {
			return math.Inf(1)
		}
	} else {
		a := (y0 - py) / dy
		b := (y1 - py) / dy
		enter = max(enter, min(a, b))
		leave = min(leave, max(a, b))
	}
	if enter <= leave {
		return enter
	}
	return math.Inf(1)
}

func hitEllipse(cx, cy, rx, ry, px, py, dx, dy float64) float64 {
	ux := dx / rx
	uy := dy / ry
	vx := (px - cx) / rx
	vy := (py - cy) / ry
	a := float64(ux*ux) + float64(uy*uy)
	b := 2 * (float64(ux*vx) + float64(uy*vy))
	c := float64(vx*vx) + float64(vy*vy) - 1
	disc := float64(b*b) - float64(4*a*c)
	if a == 0 || disc < 0 {
		return math.Inf(1)
	}
	t := (-b - math.Sqrt(disc)) / (2 * a)
	if t >= 0 && t <= 1 {
		return t
	}
	if c <= 0 {
		return 0
	}
	return math.Inf(1)
}

// hit is where a segment from outside first meets the outline of a shape, as a fraction of
// the segment.
func hit(b *box, px, py, qx, qy float64) float64 {
	x, y, w, h := b.x, b.y, b.w, b.h
	dx := qx - px
	dy := qy - py
	switch b.form {
	case "person":
		return min(hitRect(x, y+head, x+w, y+h, px, py, dx, dy), hitEllipse(x+float64(w/2), y+14, 13, 13, px, py, dx, dy))
	case "cyl":
		ry := cylRy(w)
		return min(
			hitRect(x, y+ry, x+w, y+h-ry, px, py, dx, dy),
			hitEllipse(x+float64(w/2), y+ry, w/2, ry, px, py, dx, dy),
			hitEllipse(x+float64(w/2), y+h-ry, w/2, ry, px, py, dx, dy),
		)
	case "h-cyl":
		rx := queueRx(h)
		return min(
			hitRect(x+rx, y, x+w-rx, y+h, px, py, dx, dy),
			hitEllipse(x+rx, y+float64(h/2), rx, h/2, px, py, dx, dy),
			hitEllipse(x+w-rx, y+float64(h/2), rx, h/2, px, py, dx, dy),
		)
	}
	return hitRect(x, y, x+w, y+h, px, py, dx, dy)
}

func crosses(points []float64, x0, y0, x1, y1 float64) bool {
	for i := 2; i < len(points); i += 2 {
		px := points[i-2]
		py := points[i-1]
		if hitRect(x0, y0, x1, y1, px, py, points[i]-px, points[i+1]-py) <= 1 {
			return true
		}
	}
	return false
}

func contains(outer, inner *box) bool {
	return outer.x <= inner.x &&
		outer.y <= inner.y &&
		outer.x+outer.w >= inner.x+inner.w &&
		outer.y+outer.h >= inner.y+inner.h
}

// connect is the straight line from a to b: x, y of its start, then of its end.
func connect(a, b *box, shift float64) (float64, float64, float64, float64) {
	ax := a.x + float64(a.w/2)
	ay := a.y + float64(a.h/2)
	bx := b.x + float64(b.w/2)
	by := b.y + float64(b.h/2)
	down := contains(a, b)
	if down || contains(b, a) {
		// One holds the other: a short line from the nearest side of the outer box.
		outer, inner := b, a
		if down {
			outer, inner = a, b
		}
		ix := inner.x + float64(inner.w/2)
		iy := inner.y + float64(inner.h/2)
		left := inner.x - outer.x
		right := outer.x + outer.w - inner.x - inner.w
		top := inner.y - outer.y
		bottom := outer.y + outer.h - inner.y - inner.h
		least := min(left, right, top, bottom)
		ox := ix
		if least == left {
			ox = outer.x
		} else if least == right {
			ox = outer.x + outer.w
		}
		oy := outer.y
		if least == left || least == right {
			oy = iy
		} else if least == bottom {
			oy = outer.y + outer.h
		}
		t := min(hit(inner, ox, oy, ix, iy), 1)
		mx := ox + float64((ix-ox)*t)
		my := oy + float64((iy-oy)*t)
		if down {
			return ox, oy, mx, my
		}
		return mx, my, ox, oy
	}

	px, py, qx, qy := ax, ay, bx, by
	// Shapes that share a column or a row are joined by an upright or level line through the
	// shared span.
	x0 := max(a.x, b.x)
	x1 := min(a.x+a.w, b.x+b.w)
	y0 := max(a.y, b.y)
	y1 := min(a.y+a.h, b.y+b.h)
	if x0 < x1 {
		px = (x0 + x1) / 2
		qx = px
	} else if y0 < y1 {
		py = (y0 + y1) / 2
		qy = py
	}
	length := jsmath.Hypot(qx-px, qy-py)
	if !truthy(length) {
		length = 1
	}
	nx := float64(float64(-(qy-py)/length) * shift)
	ny := float64(float64((qx-px)/length) * shift)
	ta := hit(a, qx+nx, qy+ny, px+nx, py+ny)
	tb := hit(b, px+nx, py+ny, qx+nx, qy+ny)
	if ta <= 1 && tb <= 1 {
		px += nx
		py += ny
		qx += nx
		qy += ny
	} else {
		px, py, qx, qy = ax, ay, bx, by
		ta = min(hit(a, qx, qy, px, py), 1)
		tb = min(hit(b, px, py, qx, qy), 1)
	}
	return qx + float64((px-qx)*ta), qy + float64((py-qy)*ta), px + float64((qx-px)*tb), py + float64((qy-py)*tb)
}

// Parallel stretches closer than near read as one line, and a line frameNear to a
// boundary's frame reads as part of it.
const (
	near      = 6
	frameNear = 3
	portStep  = 12
)

// lanes is what the routes drawn so far take up, so that the next keeps apart from them:
// upright stretches as x, top, bottom; level ones as y, left, right; and where lines leave
// or reach each side of a shape.
type lanes struct {
	upright []float64
	level   []float64
	frames  [2][]float64
	taken   map[*box][]float64
	// Routes that found no lane of their own: a line left straight behind a shape, or a
	// lane that runs along another.
	squeezed int
}

func newLanes() *lanes {
	return &lanes{taken: map[*box][]float64{}}
}

func (l *lanes) free(upright bool, at, a0, a1 float64) bool {
	lo := min(a0, a1)
	hi := max(a0, a1)
	nearAny := func(list []float64, gap float64) bool {
		for i := 0; i < len(list); i += 3 {
			if math.Abs(list[i]-at) < gap && list[i+1] < hi && list[i+2] > lo {
				return true
			}
		}
		return false
	}
	list, frames := l.level, l.frames[1]
	if upright {
		list, frames = l.upright, l.frames[0]
	}
	return !nearAny(list, near) && !nearAny(frames, frameNear)
}

func (l *lanes) crossings(points []float64) int {
	count := 0
	for i := 2; i < len(points); i += 2 {
		upright := points[i] == points[i-2]
		var at, lo, hi float64
		var list []float64
		if upright {
			at = points[i]
			lo = min(points[i-1], points[i+1])
			hi = max(points[i-1], points[i+1])
			list = l.level
		} else {
			at = points[i+1]
			lo = min(points[i-2], points[i])
			hi = max(points[i-2], points[i])
			list = l.upright
		}
		for j := 0; j < len(list); j += 3 {
			if list[j] > lo && list[j] < hi && list[j+1] < at && list[j+2] > at {
				count++
			}
		}
	}
	return count
}

func (l *lanes) frame(b *box) {
	l.frames[0] = append(l.frames[0], b.x, b.y, b.y+b.h, b.x+b.w, b.y, b.y+b.h)
	l.frames[1] = append(l.frames[1], b.y, b.x, b.x+b.w, b.y+b.h, b.x, b.x+b.w)
}

func (l *lanes) take(points []float64) {
	for i := 2; i < len(points); i += 2 {
		if points[i] == points[i-2] {
			l.upright = append(l.upright, points[i], min(points[i-1], points[i+1]), max(points[i-1], points[i+1]))
		} else if points[i+1] == points[i-1] {
			l.level = append(l.level, points[i+1], min(points[i-2], points[i]), max(points[i-2], points[i]))
		}
	}
}

// stepAt is (k & 1 ? 1 : -1) * Math.ceil(k / 2): 0, -1, 1, -2, 2 and so on.
func stepAt(k int) float64 {
	s := float64((k + 1) / 2)
	if k&1 == 0 {
		return -s
	}
	return s
}

// ports is where on one side of a shape (0 left, 1 right, 2 top, 3 bottom) a line can leave
// or arrive, between lo and hi: the places clear of the lines already there, nearest want
// first.
func (l *lanes) ports(b *box, side int, want, lo, hi float64) []float64 {
	used := l.taken[b]
	var out []float64
	for k := 0; k < 9 && len(out) < 4; k++ {
		at := want + float64(stepAt(k)*portStep)
		if at < lo || at > hi {
			continue
		}
		clear := true
		for i := 0; i < len(used); i += 2 {
			if used[i] == float64(side) && math.Abs(used[i+1]-at) < portStep {
				clear = false
			}
		}
		if clear {
			out = append(out, at)
		}
	}
	if len(out) == 0 {
		return []float64{want}
	}
	return out
}

func (l *lanes) claim(b *box, side int, at float64) {
	l.taken[b] = append(l.taken[b], float64(side), at)
}

func path(points []float64, startTrim, endTrim float64) string {
	n := len(points)
	ux := jsmath.Sign(points[2] - points[0])
	uy := jsmath.Sign(points[3] - points[1])
	d := "M" + svg.Num(points[0]+float64(ux*startTrim)) + "," + svg.Num(points[1]+float64(uy*startTrim))
	for i := 2; i < n-2; i += 2 {
		x := points[i]
		y := points[i+1]
		ix := jsmath.Sign(x - points[i-2])
		iy := jsmath.Sign(y - points[i-1])
		ox := jsmath.Sign(points[i+2] - x)
		oy := jsmath.Sign(points[i+3] - y)
		r := min(
			corner,
			math.Abs(x-points[i-2]+y-points[i-1])/2,
			math.Abs(points[i+2]-x+points[i+3]-y)/2,
		)
		if truthy(ix) {
			d += "H" + svg.Num(x-float64(ix*r))
		} else {
			d += "V" + svg.Num(y-float64(iy*r))
		}
		d += "Q" + svg.Num(x) + "," + svg.Num(y) + " " + svg.Num(x+float64(ox*r)) + "," + svg.Num(y+float64(oy*r))
	}
	lx := jsmath.Sign(points[n-2] - points[n-4])
	ly := jsmath.Sign(points[n-1] - points[n-3])
	if truthy(lx) {
		return d + "H" + svg.Num(points[n-2]-float64(lx*endTrim))
	}
	return d + "V" + svg.Num(points[n-1]-float64(ly*endTrim))
}

func clearOf(points []float64, others []*box, a, b *box) bool {
	for _, other := range others {
		if other != a && other != b && crosses(points, other.x, other.y, other.x+other.w, other.y+other.h) {
			return false
		}
	}
	return true
}

// bent is the route for a line with corners through points, from a's outline to b's. along
// is the stretch its label sits on; label sets the label fields given in set.
func bent(points []float64, along int, startTrim, endTrim float64, set func(r *route)) route {
	n := len(points)
	r := route{
		d:    path(points, startTrim, endTrim),
		sx:   points[0],
		sy:   points[1],
		sdx:  -jsmath.Sign(points[2] - points[0]),
		sdy:  -jsmath.Sign(points[3] - points[1]),
		ex:   points[n-2],
		ey:   points[n-1],
		edx:  jsmath.Sign(points[n-2] - points[n-4]),
		edy:  jsmath.Sign(points[n-1] - points[n-3]),
		x0:   points[along],
		y0:   points[along+1],
		x1:   points[along+2],
		y1:   points[along+3],
		room: wide,
	}
	set(&r)
	r.points = points
	return r
}

func across(lo, hi float64) []float64 {
	var out []float64
	mid := float64((lo + hi) / 2)
	step := max(laneStep, (hi-lo)/4)
	for k := 0; k < 9; k++ {
		at := mid + float64(stepAt(k)*step)
		if at >= lo+near && at <= hi-near {
			out = append(out, at)
		}
	}
	return out
}

type candidate struct {
	points  []float64
	cost    float64
	upright bool
	sides   [2]int
}

func elbow(a, b *box, others []*box, ls *lanes, startTrim, endTrim float64) (route, bool) {
	ax := a.x + float64(a.w/2)
	ay := a.y + float64(a.h/2)
	bx := b.x + float64(b.w/2)
	by := b.y + float64(b.h/2)
	var best *candidate
	consider := func(points []float64, upright bool, sides [2]int) {
		if !clearOf(points, others, a, b) {
			return
		}
		for i := 2; i < len(points); i += 2 {
			up := points[i] == points[i-2]
			var ok bool
			if up {
				ok = ls.free(up, points[i], points[i-1], points[i+1])
			} else {
				ok = ls.free(up, points[i+1], points[i-2], points[i])
			}
			if !ok {
				return
			}
		}
		cost := float64(ls.crossings(points)) * 1e6
		for i := 2; i < len(points); i += 2 {
			cost += math.Abs(points[i]-points[i-2]) + math.Abs(points[i+1]-points[i-1])
		}
		if best == nil || cost < best.cost {
			best = &candidate{points, cost, upright, sides}
		}
	}
	if a.x+a.w <= b.x || b.x+b.w <= a.x {
		right := bx > ax
		down := jsmath.Sign(by - ay)
		sides := [2]int{0, 1}
		lanesAcross := across(b.x+b.w, a.x)
		if right {
			sides = [2]int{1, 0}
			lanesAcross = across(a.x+a.w, b.x)
		}
	search1:
		for _, y0 := range ls.ports(a, sides[0], ay+float64(float64(down*a.h)/4), a.y+8, a.y+a.h-8) {
			for _, y1 := range ls.ports(b, sides[1], by-float64(float64(down*b.h)/4), b.y+8, b.y+b.h-8) {
				for _, at := range lanesAcross {
					x0 := at + float64((ax-at)*min(hit(a, at, y0, ax, y0), 1))
					x1 := at + float64((bx-at)*min(hit(b, at, y1, bx, y1), 1))
					before := best
					consider([]float64{x0, y0, at, y0, at, y1, x1, y1}, true, sides)
					if best != before {
						break search1
					}
				}
			}
		}
	}
	if a.y+a.h <= b.y || b.y+b.h <= a.y {
		below := by > ay
		sideways := jsmath.Sign(bx - ax)
		sides := [2]int{2, 3}
		lanesAcross := across(b.y+b.h, a.y)
		if below {
			sides = [2]int{3, 2}
			lanesAcross = across(a.y+a.h, b.y)
		}
	search2:
		for _, x0 := range ls.ports(a, sides[0], ax+float64(float64(sideways*a.w)/4), a.x+8, a.x+a.w-8) {
			for _, x1 := range ls.ports(b, sides[1], bx-float64(float64(sideways*b.w)/4), b.x+8, b.x+b.w-8) {
				for _, at := range lanesAcross {
					y0 := at + float64((ay-at)*min(hit(a, x0, at, x0, ay), 1))
					y1 := at + float64((by-at)*min(hit(b, x1, at, x1, by), 1))
					before := best
					consider([]float64{x0, y0, x0, at, x1, at, x1, y1}, false, sides)
					if best != before {
						break search2
					}
				}
			}
		}
	}
	if best == nil {
		return route{}, false
	}
	points, upright, sides := best.points, best.upright, best.sides
	ls.take(points)
	if upright {
		ls.claim(a, sides[0], points[1])
		ls.claim(b, sides[1], points[7])
	} else {
		ls.claim(a, sides[0], points[0])
		ls.claim(b, sides[1], points[6])
	}
	// The label sits on the middle stretch, across the gap.
	return bent(points, 2, startTrim, endTrim, func(r *route) {
		if upright {
			r.room = narrow
		} else {
			r.room = wide
		}
	}), true
}

type way struct {
	points []float64
	side   int
	p0, p1 float64
	step   int
}

// routeRel routes a relation from a to b. shift moves the line sideways when another runs
// the opposite way. others are the shapes to steer clear of; nil skips that check (careful
// is false then). ls holds what the routes before it take up, so that this one keeps apart
// from them.
func routeRel(a, b *box, shift float64, others []*box, careful bool, ls *lanes, startTrim, endTrim float64) route {
	if a == b {
		spread := min(float64(a.h/2)-4, 10)
		length := jsmath.Hypot(loop, spread)
		x := a.x + a.w
		cy := a.y + float64(a.h/2)
		ux := loop / length
		uy := spread / length
		return route{
			d: "M" + svg.Num(x+float64(ux*startTrim)) + "," + svg.Num(cy-spread-float64(uy*startTrim)) +
				"C" + svg.Num(x+loop) + "," + svg.Num(cy-float64(spread*2)) + " " +
				svg.Num(x+loop) + "," + svg.Num(cy+float64(spread*2)) + " " +
				svg.Num(x+float64(ux*endTrim)) + "," + svg.Num(cy+spread+float64(uy*endTrim)),
			sx:     x,
			sy:     cy - spread,
			sdx:    -ux,
			sdy:    uy,
			ex:     x,
			ey:     cy + spread,
			edx:    -ux,
			edy:    -uy,
			x0:     x + loop + 4,
			y0:     cy,
			x1:     x + loop + 4,
			y1:     cy,
			nx:     1,
			room:   64,
			points: []float64{},
		}
	}

	sx, sy, ex, ey := connect(a, b, shift)
	upright := sx == ex
	blocked := false
	if careful && !contains(a, b) && !contains(b, a) {
		for _, other := range others {
			if other != a && other != b && hitRect(other.x, other.y, other.x+other.w, other.y+other.h, sx, sy, ex-sx, ey-sy) <= 1 {
				blocked = true
			}
		}
	}
	if careful && blocked && !upright && sy != ey {
		if around, ok := elbow(a, b, others, ls, startTrim, endTrim); ok {
			return around
		}
		ls.squeezed++
	}
	if careful && blocked && (upright || sy == ey) {
		laneOn := func(forward bool) float64 {
			at := math.Inf(1)
			if forward {
				at = math.Inf(-1)
			}
			widen := func(b *box) {
				edge, size := b.y, b.h
				if upright {
					edge, size = b.x, b.w
				}
				if forward {
					at = max(at, edge+size)
				} else {
					at = min(at, edge)
				}
			}
			widen(a)
			widen(b)
			for _, other := range others {
				if other != a && other != b && hitRect(other.x, other.y, other.x+other.w, other.y+other.h, sx, sy, ex-sx, ey-sy) <= 1 {
					widen(other)
				}
			}
			return at
		}
		var lo, hi float64
		if upright {
			lo = min(a.y, b.y)
			hi = max(a.y+a.h, b.y+b.h)
		} else {
			lo = min(a.x, b.x)
			hi = max(a.x+a.w, b.x+b.w)
		}
		outside := func(forward bool, at float64) bool {
			for _, other := range others {
				if other == a || other == b {
					continue
				}
				var isNear bool
				var start, end float64
				if upright {
					isNear = other.y < hi && other.y+other.h > lo
					start = other.x
					end = start + other.w
				} else {
					isNear = other.x < hi && other.x+other.w > lo
					start = other.y
					end = start + other.h
				}
				if isNear && (forward && end > at || !forward && start < at) {
					return false
				}
			}
			return true
		}
		ax := a.x + float64(a.w/2)
		ay := a.y + float64(a.h/2)
		bx := b.x + float64(b.w/2)
		by := b.y + float64(b.h/2)
		towards := jsmath.Sign(bx - ax)
		if upright {
			towards = jsmath.Sign(by - ay)
		}
		// The route on one side: each end leaves its shape a quarter of the way from the
		// middle towards the other, clear of the straight lines that meet the middle of that
		// side and of other routes there. The lane runs a little further out for each route
		// already along that gap, and never behind a shape. step is how many lanes out it
		// went; 5 when none was free.
		along := func(forward bool) way {
			base := laneOn(forward)
			sgn := -1.0
			if forward {
				sgn = 1
			}
			var side int
			var p0, p1 float64
			if upright {
				side = 0
				if forward {
					side = 1
				}
				p0 = ls.ports(a, side, ay+float64(float64(towards*a.h)/4), a.y+8, a.y+a.h-8)[0]
				p1 = ls.ports(b, side, by-float64(float64(towards*b.h)/4), b.y+8, b.y+b.h-8)[0]
			} else {
				side = 2
				if forward {
					side = 3
				}
				p0 = ls.ports(a, side, ax+float64(float64(towards*a.w)/4), a.x+8, a.x+a.w-8)[0]
				p1 = ls.ports(b, side, bx-float64(float64(towards*b.w)/4), b.x+8, b.x+b.w-8)[0]
			}
			pointsAt := func(at float64) []float64 {
				if upright {
					x0 := at + float64((ax-at)*min(hit(a, at, p0, ax, p0), 1))
					x1 := at + float64((bx-at)*min(hit(b, at, p1, bx, p1), 1))
					return []float64{x0, p0, at, p0, at, p1, x1, p1}
				}
				y0 := at + float64((ay-at)*min(hit(a, p0, at, p0, ay), 1))
				y1 := at + float64((by-at)*min(hit(b, p1, at, p1, by), 1))
				return []float64{p0, y0, p0, at, p1, at, p1, y1}
			}
			points := pointsAt(base + float64(sgn*lane))
			for k := 0; k <= 4; k++ {
				at := base + float64(sgn*(lane+float64(float64(k)*laneStep)))
				tried := pointsAt(at)
				if !clearOf(tried, others, a, b) {
					continue
				}
				points = tried
				if ls.free(upright, at, p0, p1) {
					return way{points, side, p0, p1, k}
				}
			}
			return way{points, side, p0, p1, 5}
		}
		// A detour takes the outside of the drawing when only one side is outside, leaving
		// the gaps between columns and rows to the relations that cross them. Otherwise it
		// takes the side where it crosses fewer lines, then the one where it runs nearer its
		// shapes, so that detours spread over both sides.
		natural := ex < sx
		if upright {
			natural = ey > sy
		}
		out := outside(natural, laneOn(natural))
		back := outside(!natural, laneOn(!natural))
		first := along(natural)
		var other *way
		if out == back {
			w := along(!natural)
			other = &w
		}
		cost := func(w way) int { return ls.crossings(w.points)*8 + w.step }
		forward := natural
		if back && !out {
			forward = !natural
		} else if other != nil && cost(*other) < cost(first) {
			forward = !natural
		}
		chosen := first
		if forward != natural {
			if other != nil {
				chosen = *other
			} else {
				chosen = along(!natural)
			}
		}
		sgn := -1.0
		if forward {
			sgn = 1
		}
		if chosen.step > 4 {
			ls.squeezed++
		}
		ls.take(chosen.points)
		ls.claim(a, chosen.side, chosen.p0)
		ls.claim(b, chosen.side, chosen.p1)
		return bent(chosen.points, 2, startTrim, endTrim, func(r *route) {
			if upright {
				r.nx, r.lean, r.room = sgn, lean, narrow
			} else {
				r.room = wide
			}
		})
	}

	length := jsmath.Hypot(ex-sx, ey-sy)
	if !truthy(length) {
		length = 1
	}
	dx := (ex - sx) / length
	dy := (ey - sy) / length
	if upright || sy == ey {
		ls.take([]float64{sx, sy, ex, ey})
	}
	r := route{
		d:      "M" + svg.Num(sx+float64(dx*startTrim)) + "," + svg.Num(sy+float64(dy*startTrim)) + "L" + svg.Num(ex-float64(dx*endTrim)) + "," + svg.Num(ey-float64(dy*endTrim)),
		sx:     sx,
		sy:     sy,
		sdx:    -dx,
		sdy:    -dy,
		ex:     ex,
		ey:     ey,
		edx:    dx,
		edy:    dy,
		x0:     sx,
		y0:     sy,
		x1:     ex,
		y1:     ey,
		room:   wide,
		points: []float64{sx, sy, ex, ey},
	}
	if truthy(shift) {
		r.nx, r.ny = -dy, dx
	}
	if math.Abs(dx) > math.Abs(dy) {
		r.room = narrow
	}
	return r
}
