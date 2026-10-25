package layout

import "math"

type part struct {
	nodes, edges             []int
	left, top, right, bottom float64
}

func (p *part) include(x, y, w, h float64) {
	p.left = min(p.left, x-float64(w/2))
	p.top = min(p.top, y-float64(h/2))
	p.right = max(p.right, x+float64(w/2))
	p.bottom = max(p.bottom, y+float64(h/2))
}

// WrapComponents moves disconnected parts into rows without changing the layout inside a
// part. It reports false, and moves nothing, where that would not make the layout narrower.
func WrapComponents(nodes []CNode, edges []CEdge, width, gap float64) (CompoundResult, bool) {
	parent := make([]int, len(nodes))
	for i := range parent {
		parent[i] = i
	}
	root := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	join := func(a, b int) {
		ra := root(a)
		parent[ra] = root(b)
	}
	for i := range nodes {
		if nodes[i].Parent >= 0 {
			join(i, nodes[i].Parent)
		}
	}
	for _, e := range edges {
		join(e.Src, e.Dst)
	}

	var parts []*part
	byRoot := map[int]*part{}
	for i := range nodes {
		id := root(i)
		p := byRoot[id]
		if p == nil {
			p = &part{left: math.Inf(1), top: math.Inf(1), right: math.Inf(-1), bottom: math.Inf(-1)}
			byRoot[id] = p
			parts = append(parts, p)
		}
		c := &nodes[i]
		p.nodes = append(p.nodes, i)
		p.include(c.X, c.Y, c.W, c.H)
	}
	if len(parts) < 2 {
		return CompoundResult{}, false
	}
	for k := range edges {
		e := &edges[k]
		p := byRoot[root(e.Src)]
		p.edges = append(p.edges, k)
		for i := 0; i < len(e.Route); i += 3 {
			p.include(e.Route[i], e.Route[i+1], 0, 0)
		}
		if e.LabelW > 0 {
			p.include(e.LabelX, e.LabelY, e.LabelW, e.LabelH)
		}
	}

	x, y, rowHeight, right, from, to := 0.0, 0.0, 0.0, 0.0, math.Inf(1), math.Inf(-1)
	shifts := make([]float64, 0, 2*len(parts))
	for _, p := range parts {
		w, h := p.right-p.left, p.bottom-p.top
		if x > 0 && x+w > width {
			y += rowHeight + gap
			x = 0
			rowHeight = 0
		}
		shifts = append(shifts, x-p.left, y-p.top)
		from = min(from, p.left)
		to = max(to, p.right)
		right = max(right, x+w)
		rowHeight = max(rowHeight, h)
		x += w + gap
	}
	if right >= to-from {
		return CompoundResult{}, false
	}
	for k, p := range parts {
		ox, oy := shifts[2*k], shifts[2*k+1]
		for _, i := range p.nodes {
			nodes[i].X += ox
			nodes[i].Y += oy
		}
		for _, i := range p.edges {
			e := &edges[i]
			for j := 0; j < len(e.Route); j += 3 {
				e.Route[j] += ox
				e.Route[j+1] += oy
			}
			e.LabelX += ox
			e.LabelY += oy
		}
	}
	return CompoundResult{Width: right, Height: y + rowHeight}, true
}
