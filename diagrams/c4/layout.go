package c4

import (
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const (
	shapeW    = 216
	shapeMinH = 64
	padX      = 14
	padY      = 12
	// Room between two columns for the label of a line across them, clear of an arrowhead on
	// either side, with an arrowhead's length to spare for a word a little wider than the
	// label room.
	gapX = narrow + labelPad + 3*svg.Arrow
	gapY = 88
	// Room for a relation to run along the inside of a boundary.
	groupPad = lane + 8
	groupGap = 48
	// Where a boundary's heading starts, from its top-left corner.
	headLeft = 16
	headTop  = 10
)

// Shape keywords Mermaid accepts in $shape, $sprite or $tags, as names of the shared shapes.
var forms = map[string]string{
	"person":    "person",
	"box":       "rect",
	"rounded":   "rect",
	"component": "rect",
	"cylinder":  "cyl",
	"database":  "cyl",
	"db":        "cyl",
	"queue":     "h-cyl",
	"pipe":      "h-cyl",
}

// formNamed is FORMS.get(s.toLowerCase()). Only an ASCII string lowers to a key: the one
// other character JS lowers to ASCII is the Kelvin sign, to a k no key has, while Go would
// also lower U+0130 to i.
func formNamed(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return ""
		}
	}
	return forms[strings.ToLower(s)]
}

type node struct {
	box
	shape *element
	// Type, name, technology, description.
	labels []text.Label
	textH  float64
}

type group struct {
	box
	boundary *element
	kids     []*group
	nodes    []*node
	// Name, type, description.
	labels []text.Label
	head   float64
	headW  float64
	headX  float64
}

type layout struct {
	// The boundaries that are drawn, parents before children. The first is the unnamed top
	// level.
	order []*group
	boxes map[*element]*box
}

func formOf(shape *element) string {
	if explicit := formNamed(shape.shape); explicit != "" {
		return explicit
	}
	if explicit := formNamed(shape.sprite); explicit != "" {
		return explicit
	}
	if shape.tags != "" {
		for _, tag := range strings.Split(shape.tags, ",") {
			if form := formNamed(jsstr.Trim(tag)); form != "" {
				return form
			}
		}
	}
	kind := shape.kind
	switch {
	case strings.HasSuffix(kind, "person"):
		return "person"
	case strings.HasSuffix(kind, "_db"):
		return "cyl"
	case strings.HasSuffix(kind, "_queue"):
		return "h-cyl"
	}
	return "rect"
}

func measure(shape *element, m text.Measurer, icons *text.Icons, size, small, width float64) *node {
	form := formOf(shape)
	capW := 0.0
	if form == "h-cyl" {
		capW = 36
	}
	capH := 0.0
	if form == "person" {
		capH = head
	} else if form == "cyl" {
		capH = 24
	}
	inner := width - 2*padX - capW
	techn := ""
	if shape.techn != "" {
		techn = "[" + shape.techn + "]"
	}
	labels := []text.Label{
		text.Layout("«"+strings.ReplaceAll(shape.kind, "_", " ")+"»", false, m, small-1, inner, 0, icons),
		text.Layout(shape.label, false, m, size, inner, text.Bold, icons),
		text.Layout(techn, false, m, small, inner, 0, icons),
		text.Layout(shape.descr, false, m, small, inner, 0, icons),
	}
	tw := 0.0
	textH := 0.0
	if labels[3].Height > 0 {
		textH = 4
	}
	for _, label := range labels {
		tw = max(tw, label.Width)
		textH += label.Height
	}
	return &node{
		box: box{
			form: form,
			w:    max(width, tw+2*padX+capW),
			h:    max(shapeMinH, textH+2*padY+capH),
		},
		shape:  shape,
		labels: labels,
		textH:  textH,
	}
}

// rows places boxes left to right in rows of perRow, from (0, 0), and returns the width
// and height the rows take. Shapes are stretched to the height of their row; boundaries
// keep their own. perRow is at least 1 and may be Infinity.
func rows(boxes []*box, perRow, gapX, gapY float64, stretch bool) (float64, float64) {
	step := len(boxes)
	if perRow < float64(len(boxes)) {
		step = int(perRow)
	}
	width := 0.0
	y := 0.0
	for from := 0; from < len(boxes); from += step {
		to := min(len(boxes), from+step)
		x := 0.0
		rowH := 0.0
		for i := from; i < to; i++ {
			rowH = max(rowH, boxes[i].h)
		}
		for i := from; i < to; i++ {
			b := boxes[i]
			b.x = x
			b.y = y
			if stretch {
				b.h = rowH
			}
			x += b.w + gapX
		}
		width = max(width, x-gapX)
		y += rowH + gapY
	}
	if len(boxes) == 0 {
		return width, 0
	}
	return width, y - gapY
}

// layoutC4 is Mermaid's grid: in each boundary, its shapes in rows of c4ShapeInRow, then
// the boundaries inside it in rows of c4BoundaryInRow, each sized around its own content.
// room is used when hasRoom is set.
func layoutC4(db *c4Db, m text.Measurer, icons *text.Icons, size, small, room float64, hasRoom bool, gap float64) layout {
	pad := float64(groupPad)
	if hasRoom {
		pad = 12
	}
	groups := make(map[*element]*group, len(db.boundaries))
	for _, boundary := range db.boundaries {
		labelRoom := 4000.0
		if hasRoom {
			labelRoom = max(80, room-2*headLeft-2*groupPad*2)
		}
		var labels []text.Label
		if boundary.parent != nil {
			typ := ""
			if boundary.typ != "" {
				typ = "[" + boundary.typ + "]"
			}
			labels = []text.Label{
				text.Layout(boundary.label, false, m, size, labelRoom, text.Bold, icons),
				text.Layout(typ, false, m, small, labelRoom, 0, icons),
				text.Layout(boundary.descr, false, m, small, labelRoom, 0, icons),
			}
		}
		h := 0.0
		headW := 0.0
		for _, label := range labels {
			h += label.Height
			headW = max(headW, label.Width)
		}
		g := &group{boundary: boundary, labels: labels, head: pad, headW: headW, headX: headLeft}
		g.form = "rect"
		if h > 0 {
			g.head = h + headTop + 6 + lane
		}
		groups[boundary] = g
	}
	root := groups[db.boundaries[0]]
	for _, boundary := range db.boundaries {
		if parent := boundary.parent; parent != nil {
			if p := groups[parent]; p != nil {
				p.kids = append(p.kids, groups[boundary])
			}
		}
	}
	// A boundary that ends up inside itself is not reachable from the top, as in Mermaid.
	order := []*group{root}
	depths := map[*element]float64{root.boundary: 0}
	for i := 0; i < len(order); i++ {
		for _, kid := range order[i].kids {
			depths[kid.boundary] = depths[order[i].boundary] + 1
			order = append(order, kid)
		}
	}
	for _, shape := range db.shapes {
		if shape.parent != nil {
			depth := depths[shape.parent]
			nodeWidth := float64(shapeW)
			if hasRoom {
				nodeWidth = max(100, min(shapeW, room-float64(2*depth*pad)-32))
			}
			if g := groups[shape.parent]; g != nil {
				g.nodes = append(g.nodes, measure(shape, m, icons, size, small, nodeWidth))
			}
		}
	}

	perRow := max(1, db.shapeInRow)
	groupsPerRow := max(1, db.boundaryInRow)
	kidsTop := make(map[*group]float64, len(order))
	for i := len(order) - 1; i >= 0; i-- {
		g := order[i]
		gx := float64(gapX)
		if hasRoom {
			gx = gap
		}
		count := func(boxes []*box, limit, gap float64) float64 {
			if !hasRoom {
				return limit
			}
			widest := 1.0
			for _, b := range boxes {
				widest = max(widest, b.w)
			}
			return max(1, min(limit, math.Floor((room+gap-2*pad)/(widest+gap))))
		}
		nodeBoxes := make([]*box, len(g.nodes))
		for k, n := range g.nodes {
			nodeBoxes[k] = &n.box
		}
		kidBoxes := make([]*box, len(g.kids))
		for k, kid := range g.kids {
			kidBoxes[k] = &kid.box
		}
		gridW, gridH := rows(nodeBoxes, count(nodeBoxes, perRow, gx), gx, gapY, true)
		kidsW, kidsH := rows(kidBoxes, count(kidBoxes, groupsPerRow, groupGap), groupGap, groupGap, false)
		between := 0.0
		if gridH > 0 && kidsH > 0 {
			between = groupGap
		}
		frame := pad
		if g == root {
			frame = 0
		}
		kidsTop[g] = gridH + between
		g.w = max(gridW, kidsW, g.headW+2*headLeft-float64(2*frame)) + float64(2*frame)
		top := 0.0
		if frame != 0 {
			top = g.head
		}
		g.h = top + gridH + between + kidsH + frame
	}

	boxes := map[*element]*box{}
	for _, g := range order {
		left, top := g.x, g.y
		if g != root {
			left += pad
			top += g.head
		}
		for _, n := range g.nodes {
			n.x += left
			n.y += top
			boxes[n.shape] = &n.box
		}
		for _, kid := range g.kids {
			kid.x += left
			kid.y += top + kidsTop[g]
		}
		if g != root {
			boxes[g.boundary] = &g.box
		}
	}
	return layout{order: order, boxes: boxes}
}
