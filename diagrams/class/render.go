package class

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
	"github.com/Broderick-Westrope/pele-go/internal/layout"
	"github.com/Broderick-Westrope/pele-go/internal/omap"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

const (
	nodeSep      = 40
	edgeSep      = 16
	rankSep      = 56
	groupPad     = 20
	loop         = 26
	padX         = 12
	titlePad     = 8
	sectionPad   = 6
	emptySection = 10
	minWidth     = 64
	fold         = 8
	lineColor    = "var(--_l)"
)

func isTarget(s string) bool {
	return s == "_self" || s == "_blank" || s == "_parent" || s == "_top"
}

func numberOption(config *jsobj.Object, key string, fallback float64) float64 {
	if v, ok := config.Get(key); ok {
		if value, isNum := v.(float64); isNum && value > 0 {
			return min(value, 2000)
		}
	}
	return fallback
}

type line struct {
	text       string
	width      float64
	classifier string
}

type view struct {
	node        *graphNode
	style       svg.ResolvedStyle
	label       text.Label
	w, h        float64
	annotations []annotation
	attributes  []line
	methods     []line
	titleH      float64
	attributesH float64
	fontSize    float64
}

type end struct {
	route  []float64
	at     int
	toward float64
	order  int
	room   float64
	side   int
	// Whether the text of this end goes on the right of, or below, its line.
	after bool
}

type card struct {
	text   string
	math   *text.Label
	x, y   float64
	w, h   float64
	parent int
}

// annotation is an annotation's text, or its label when it holds typeset math.
type annotation struct {
	text string
	math *text.Label
}

func memberLine(member *classMember, measurer text.Measurer, size float64) line {
	display, _ := member.getDisplayDetails()
	t := jsstr.Trim(text.DecodeEntities(display))
	var style text.Style
	if member.classifier == "*" {
		style = text.Italic
	}
	return line{text: t, width: measurer.Width(t, size, style), classifier: member.classifier}
}

func renderClass(db *classDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	options := rc.Options
	fitted := func(down bool) core.Rendered {
		return common.Tighten(options, func(tight bool) core.Rendered { return draw(db, config, rc, down, tight) })
	}
	if common.RunsAcross(db.direction) {
		return common.TurnToFit(options, fitted)
	}
	return fitted(false)
}

// draw draws the diagram. turned draws a diagram that runs across as one that runs down.
// tight gives up the room that keeps a namespace's name clear of an edge coming in beside it.
func draw(db *classDb, config *jsobj.Object, rc *core.Context, turned, tight bool) core.Rendered {
	options := rc.Options
	var conf *jsobj.Object
	if v, ok := config.Get("class"); ok {
		conf, _ = v.(*jsobj.Object)
	}
	hierarchical := true
	if v, ok := conf.Get("hierarchicalNamespaces"); ok && v == false {
		hierarchical = false
	}
	graph := buildClassGraph(db, hierarchical)
	hideEmpty := false
	if v, ok := conf.Get("hideEmptyMembersBox"); ok && v == true {
		hideEmpty = true
	}
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	small := jsmath.Round(size * 0.875)
	tiny := jsmath.Round(size * 0.8)
	tinyHeight := jsmath.Round(tiny * 1.4)
	measurer := rc.Measurer()
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	rootDir := layout.TB
	if !turned {
		rootDir = layout.Direction(db.direction)
	}
	icons := rc.Icons
	// Keep annotations and cardinalities literal unless math is typeset.
	mathText := func(raw string, fontSize float64) *text.Label {
		if !strings.Contains(raw, "$$") {
			return nil
		}
		label := text.Layout(raw, false, measurer, fontSize, 4000, 0, icons)
		if slices.ContainsFunc(label.Lines, text.HasMath) {
			return &label
		}
		return nil
	}
	cardWidth := func(raw string) float64 {
		if raw == "" {
			return 0
		}
		if math := mathText(raw, tiny); math != nil {
			return math.Width
		}
		return measurer.Width(text.DecodeEntities(raw), tiny, 0)
	}

	// A class may carry the declarations of many classDefs many times over. Each distinct
	// declaration is checked once, and only the last value of each property that has an
	// effect is resolved.
	effect := map[string]string{}
	fromDefs := map[string][][2]string{}
	var kept omap.Map[string, string]
	keep := func(decl string) {
		prop, ok := effect[decl]
		if !ok {
			one := svg.ResolveStyle([]string{decl})
			if one.Shape != "" || one.Text != "" {
				prop = strings.ToLower(jsstr.Trim(decl[:strings.IndexByte(decl, ':')]))
			}
			effect[decl] = prop
		}
		if prop != "" {
			kept.Set(prop, decl)
		}
	}
	classStyle := func(cls *classNode) svg.ResolvedStyle {
		kept = omap.Map[string, string]{}
		base, ok := fromDefs[cls.cssClasses]
		if !ok {
			for _, name := range strings.Split(cls.cssClasses, " ") {
				if def, has := db.styleClasses.Get(name); has {
					for _, s := range def.styles {
						for _, part := range strings.Split(s, ",") {
							keep(part)
						}
					}
				}
			}
			base = [][2]string{}
			for prop, decl := range kept.All() {
				base = append(base, [2]string{prop, decl})
			}
			fromDefs[cls.cssClasses] = base
		} else {
			for _, pair := range base {
				kept.Set(pair[0], pair[1])
			}
		}
		for _, s := range cls.styles {
			keep(s)
		}
		if kept.Len() == 0 {
			return svg.ResolveStyle(nil)
		}
		return svg.ResolveStyle(slices.Collect(func(yield func(string) bool) {
			for _, decl := range kept.All() {
				if !yield(decl) {
					return
				}
			}
		}))
	}

	nodes := graph.nodes
	children := make([]int, len(nodes))
	for _, node := range nodes {
		if node.parent >= 0 {
			children[node.parent]++
		}
	}

	// Namespaces that an edge may cross into are made wide enough for the title to sit
	// beside it.
	crossed := make([]bool, len(nodes))
	for _, edge := range graph.edges {
		if edge.source < 0 || edge.target < 0 {
			continue
		}
		a := nodes[edge.source].parent
		b := nodes[edge.target].parent
		if a == b {
			continue
		}
		for g := a; g >= 0 && !crossed[g]; g = nodes[g].parent {
			crossed[g] = true
		}
		for g := b; g >= 0 && !crossed[g]; g = nodes[g].parent {
			crossed[g] = true
		}
	}

	base := func(node *graphNode, label text.Label, w, h, fontSize float64) *view {
		return &view{node: node, label: label, w: w, h: h, fontSize: fontSize}
	}

	classView := func(node *graphNode, cls *classNode) *view {
		style := classStyle(cls)
		fontSize := size
		if style.FontSize != 0 {
			fontSize = style.FontSize
		}
		memberSize := jsmath.Round(fontSize * 0.875)
		lineHeight := jsmath.Round(memberSize * 1.5)
		// The generic type is escaped so that `<T>` is not read as a tag.
		generic := ""
		if cls.typ != "" {
			g := common.ParseGenericTypes(cls.typ)
			g = strings.ReplaceAll(g, "<", "&lt;")
			g = strings.ReplaceAll(g, ">", "&gt;")
			generic = "&lt;" + g + "&gt;"
		}
		labelStyle := text.Bold
		if style.Italic {
			labelStyle |= text.Italic
		}
		label := text.Layout(cls.label+generic, false, measurer, fontSize, 4000, labelStyle, icons)
		var annotations []annotation
		for _, a := range cls.annotations {
			if math := mathText("«"+jsstr.Trim(a)+"»", tiny); math != nil {
				annotations = append(annotations, annotation{math: math})
			} else {
				annotations = append(annotations, annotation{text: "«" + jsstr.Trim(text.DecodeEntities(a)) + "»"})
			}
		}
		var attributes, methods []line
		for _, m := range cls.members {
			attributes = append(attributes, memberLine(m, measurer, memberSize))
		}
		for _, m := range cls.methods {
			methods = append(methods, memberLine(m, measurer, memberSize))
		}
		inner := label.Width
		annotationsH := 0.0
		for _, a := range annotations {
			if a.math == nil {
				inner = max(inner, measurer.Width(a.text, tiny, 0))
				annotationsH += tinyHeight
			} else {
				inner = max(inner, a.math.Width)
				annotationsH += max(tinyHeight, a.math.Height)
			}
		}
		for _, l := range attributes {
			inner = max(inner, l.width)
		}
		for _, l := range methods {
			inner = max(inner, l.width)
		}
		sections := !(hideEmpty && len(attributes) == 0 && len(methods) == 0)
		section := func(count int) float64 {
			if !sections {
				return 0
			}
			if count > 0 {
				return 2*sectionPad + float64(float64(count)*lineHeight)
			}
			return emptySection
		}
		titleH := 2*titlePad + annotationsH + max(label.Height, jsmath.Round(fontSize*1.5))
		attributesH := section(len(attributes))
		return &view{
			node:        node,
			style:       style,
			label:       label,
			w:           max(inner+2*padX, minWidth),
			h:           titleH + attributesH + section(len(methods)),
			annotations: annotations,
			attributes:  attributes,
			methods:     methods,
			titleH:      titleH,
			attributesH: attributesH,
			fontSize:    fontSize,
		}
	}

	views := make([]*view, 0, len(nodes))
	cnodes := make([]layout.CNode, 0, len(nodes))
	for i, node := range nodes {
		var v *view
		var c layout.CNode
		switch {
		case node.isGroup:
			label := text.Layout(node.label, false, measurer, size, 4000, 0, icons)
			v = base(node, label, 0, 0, size)
			if children[i] > 0 {
				c = layout.NewCNode(0, 0, -1)
				c.IsGroup = true
				c.PadX = groupPad
				c.PadTop = groupPad
				if label.Height > 0 {
					c.PadTop = label.Height + 16
				}
				c.PadBottom = groupPad
				c.MinW = label.Width + 2*groupPad - 12
				if crossed[i] && !tight {
					c.MinW += min(label.Width+24, 240)
				}
			} else {
				v.w = max(label.Width+2*groupPad, 80)
				v.h = label.Height + 2*groupPad
				c = layout.NewCNode(v.w, v.h, -1)
			}
		case node.classNode != nil:
			v = classView(node, node.classNode)
			c = layout.NewCNode(v.w, v.h, -1)
		case node.shape == shapeNote:
			label := text.Layout(node.label, true, measurer, small, 260, 0, icons)
			v = base(node, label, max(label.Width+2*padX, 40), label.Height+2*titlePad, small)
			c = layout.NewCNode(v.w, v.h, -1)
		default:
			label := text.Layout(node.label, false, measurer, small, 200, 0, icons)
			v = base(node, label, max(label.Width, 12)+4, max(label.Height, 12), small)
			c = layout.NewCNode(v.w, v.h, -1)
		}
		c.Parent = node.parent
		c.Seq = float64(i)
		views = append(views, v)
		cnodes = append(cnodes, c)
	}

	var cedges []layout.CEdge
	var drawn []*graphEdge
	var edgeLabels []text.Label
	var startRoom, endRoom []float64
	// Edges leave one side of a class and arrive at the other, so a class with many of them
	// is widened until they, and the text at their ends, fit side by side.
	crowd := make([]float64, len(views)*4)
	meet := func(item, slot int, room float64) {
		crowd[item*4+slot]++
		if room > crowd[item*4+slot+1] {
			crowd[item*4+slot+1] = room
		}
	}
	for _, edge := range graph.edges {
		if edge.source < 0 || edge.target < 0 {
			continue
		}
		label := text.Layout(edge.label, true, measurer, small, 200, 0, icons)
		startW := cardWidth(edge.startLabelRight)
		endW := cardWidth(edge.endLabelLeft)
		labelW, labelH := 0.0, 0.0
		if label.Width > 0 {
			labelW = label.Width + 12
		}
		if label.Height > 0 {
			labelH = label.Height + 4
		}
		if edge.source == edge.target {
			cnodes[edge.source].W += 2 * (loop + labelW)
		} else {
			meet(edge.source, 0, startW)
			meet(edge.target, 2, endW)
		}
		startRoom = append(startRoom, startW)
		endRoom = append(endRoom, endW)
		drawn = append(drawn, edge)
		edgeLabels = append(edgeLabels, label)
		cedges = append(cedges, layout.CEdge{Src: edge.source, Dst: edge.target, Minlen: 1, LabelW: labelW, LabelH: labelH})
	}

	if rootDir == layout.TB || rootDir == layout.BT {
		for i, v := range views {
			if v.node.classNode == nil {
				continue
			}
			need := 0.0
			for slot := 0; slot < 4; slot += 2 {
				count := crowd[i*4+slot]
				room := crowd[i*4+slot+1]
				if count > 1 {
					per := 14.0
					if room > 0 {
						per = room + 24
					}
					need = max(need, float64((count-1)*per)+24)
				}
			}
			need = min(need, 480)
			if need > v.w {
				cnodes[i].W += need - v.w
				v.w = need
			}
		}
	}

	lettered := false
	for i := 0; i < len(drawn) && !lettered; i++ {
		lettered = startRoom[i] > 0 || endRoom[i] > 0
	}
	rankDefault := float64(rankSep)
	if lettered {
		rankDefault = rankSep + 16
	}
	// Cardinalities at both ends of a vertical edge need room for their height between the ranks.
	tallCards := 0.0
	if rootDir == layout.TB || rootDir == layout.BT {
		cardHeight := func(raw string) float64 {
			if raw == "" {
				return 0
			}
			h := 0.0
			if math := mathText(raw, tiny); math != nil {
				h = math.Height
			}
			return max(tinyHeight, h)
		}
		for _, edge := range drawn {
			tallCards = max(tallCards, cardHeight(edge.startLabelRight)+cardHeight(edge.endLabelLeft)-2*tinyHeight)
		}
	}
	lay := layout.CompoundLayout(rc.Ctx, cnodes, cedges, rootDir, layout.LayeredOptions{
		NodeSep: numberOption(conf, "nodeSpacing", nodeSep),
		EdgeSep: edgeSep,
		RankSep: numberOption(conf, "rankSpacing", rankDefault) + tallCards,
		PortSep: 20,
		Apart:   true,
	})

	// Every edge reaches the middle of a side. Where several share a side they are spread
	// along it, in the order of where they come from. An end with text runs straight for the
	// length of the text, which sits beside it on the side the line then bends toward, away
	// from the neighbors that bend the other way. Neighbors are set apart far enough for the
	// text between them.
	var sides omap.Map[int, []*end]
	var cards []card
	attach := func(ce *layout.CEdge, at, item int, room float64, order int) *end {
		route := ce.Route
		c := &cnodes[item]
		side := svg.EndSide(route, at, c)
		if side < 0 {
			return nil
		}
		slack := float64((c.W - views[item].w) / 2)
		if side == 1 {
			route[at] -= slack
		} else if side == 3 {
			route[at] += slack
		}
		across := side & 1
		from := at - 3
		if at == 0 {
			from = 3
		}
		toward := route[from+across]
		bend := toward - route[at+across]
		after := bend >= 0
		if across == 1 {
			after = bend > 0
		}
		e := &end{route: route, at: at, toward: toward, order: order, room: room, side: side, after: after}
		key := item*4 + side
		list, _ := sides.Get(key)
		sides.Set(key, append(list, e))
		return e
	}
	starts := make([]*end, len(cedges))
	ends := make([]*end, len(cedges))
	for i := range cedges {
		ce := &cedges[i]
		if ce.Src != ce.Dst && len(ce.Route) >= 6 {
			starts[i] = attach(ce, 0, ce.Src, startRoom[i], i)
			ends[i] = attach(ce, len(ce.Route)-3, ce.Dst, endRoom[i], i)
		}
	}
	// Whether a point lies on the border of one of the namespaces around a class.
	onBorder := func(item int, x, y float64) bool {
		for g, hops := cnodes[item].Parent, 0; g >= 0 && hops < 10000; g, hops = cnodes[g].Parent, hops+1 {
			c := &cnodes[g]
			dx := math.Abs(x - c.X)
			dy := math.Abs(y - c.Y)
			halfW := float64(c.W / 2)
			halfH := float64(c.H / 2)
			if (math.Abs(dy-halfH) < 0.5 && dx <= halfW+0.5) || (math.Abs(dx-halfW) < 0.5 && dy <= halfH+0.5) {
				return true
			}
		}
		return false
	}
	var gaps []float64
	for key, list := range sides.All() {
		if len(list) < 2 {
			continue
		}
		item := key >> 2
		across := key & 1
		length := views[item].w
		if across == 1 {
			length = views[item].h
		}
		slices.SortStableFunc(list, func(a, b *end) int {
			if d := a.toward - b.toward; d != 0 && !math.IsNaN(d) {
				if d < 0 {
					return -1
				}
				return 1
			}
			return a.order - b.order
		})
		total := 0.0
		gaps = gaps[:0]
		for j := 0; j+1 < len(list); j++ {
			a := list[j]
			b := list[j+1]
			gap := 24.0
			if across == 1 {
				gap = 20
			}
			if a.room > 0 && a.after {
				if across == 1 {
					gap += tinyHeight + 6
				} else {
					gap += a.room
				}
			}
			if b.room > 0 && !b.after {
				if across == 1 {
					gap += tinyHeight + 6
				} else {
					gap += b.room
				}
			}
			gaps = append(gaps, gap)
			total += gap
		}
		scale := min(1, (length-24)/total)
		if scale <= 0 {
			continue
		}
		center := cnodes[item].X
		if across == 1 {
			center = cnodes[item].Y
		}
		at := center - float64(float64(total*scale)/2)
		for j, e := range list {
			route, endAt := e.route, e.at
			// An edge that crosses the border of the class's namespace close by runs straight
			// to it: the point where it crosses moves into line with its end.
			step := -3
			if endAt == 0 {
				step = 3
			}
			first := endAt + step
			border := -1
			for k := first; k >= 0 && k < len(route) && math.Abs(route[k+across]-route[first+across]) < 0.5; k += step {
				if onBorder(item, route[k], route[k+1]) {
					border = k
					break
				}
			}
			if border >= 0 && math.Abs(route[first+across]-at) <= 24 {
				for k := first; k != border+step; k += step {
					route[k+across] = at
				}
			}
			route[endAt+across] = at
			if j < len(gaps) {
				at += float64(gaps[j] * scale)
			}
		}
	}

	straighten := func(route []float64, e *end, room float64, first bool) []float64 {
		if e == nil || room <= 0 {
			return route
		}
		across := e.side & 1
		at := len(route) - 3
		next := at - 3
		if first {
			at, next = 0, 3
		}
		along := at + 1 - across
		reach := math.Abs(route[next+1-across] - route[along])
		length := tinyHeight + 6
		if across == 1 {
			length = room + 10
		}
		length = min(length, reach*0.35)
		if length < 4 {
			return route
		}
		point := []float64{route[at], route[at+1], route[at+2]}
		if e.side == 0 || e.side == 3 {
			point[1-across] -= length
		} else {
			point[1-across] += length
		}
		insert := at
		if first {
			insert = 3
		}
		return slices.Insert(route, insert, point...)
	}
	for i := range cedges {
		cedges[i].Route = straighten(cedges[i].Route, starts[i], startRoom[i], true)
		cedges[i].Route = straighten(cedges[i].Route, ends[i], endRoom[i], false)
	}

	minX, minY := 0.0, 0.0
	maxX, maxY := lay.Width, lay.Height
	addCard := func(parent int, raw string, width, x, y float64, side int, after bool) {
		if raw == "" {
			return
		}
		math := mathText(raw, tiny)
		h := tinyHeight
		if math != nil {
			h = max(tinyHeight, math.Height)
		}
		cx, cy := x, y
		halfW := float64(width / 2)
		halfT := float64(h / 2)
		if side&1 == 1 {
			if side == 3 {
				cx -= 5 + halfW
			} else {
				cx += 5 + halfW
			}
			if after {
				cy += 7 + halfT
			} else {
				cy -= 7 + halfT
			}
		} else {
			if after {
				cx += 9 + halfW
			} else {
				cx -= 9 + halfW
			}
			if side == 0 {
				cy -= 3 + halfT
			} else {
				cy += 3 + halfT
			}
		}
		cards = append(cards, card{text: text.DecodeEntities(raw), math: math, x: cx, y: cy, w: width, h: h, parent: parent})
		minX = min(minX, cx-halfW)
		maxX = max(maxX, cx+halfW)
		minY = min(minY, cy-halfT)
		maxY = max(maxY, cy+halfT)
	}
	loopSeen := map[int]int{}
	loopAt := make([]int, len(cedges))
	for i := range cedges {
		ce := &cedges[i]
		edge := drawn[i]
		route := ce.Route
		if ce.Src == ce.Dst {
			k := loopSeen[ce.Src]
			loopSeen[ce.Src] = k + 1
			loopAt[i] = k
			c := &cnodes[ce.Src]
			v := views[ce.Src]
			x := c.X + float64(v.w/2)
			spread := min(float64(v.h/2)-4, float64(8+k*6))
			addCard(ce.Src, edge.startLabelRight, startRoom[i], x, c.Y-spread, 1, false)
			addCard(ce.Dst, edge.endLabelLeft, endRoom[i], x, c.Y+spread, 1, true)
			ce.LabelX = c.X + float64(v.w/2) + loop + float64(k*8) + 4 + float64(ce.LabelW/2)
			ce.LabelY = c.Y
		} else if len(route) >= 6 {
			last := len(route) - 3
			fromSide, fromAfter := 2, true
			if from := starts[i]; from != nil {
				fromSide, fromAfter = from.side, from.after
			}
			toSide, toAfter := 0, true
			if to := ends[i]; to != nil {
				toSide, toAfter = to.side, to.after
			}
			addCard(ce.Src, edge.startLabelRight, startRoom[i], route[0], route[1], fromSide, fromAfter)
			addCard(ce.Dst, edge.endLabelLeft, endRoom[i], route[last], route[last+1], toSide, toAfter)
		}
	}

	if available := options.MaxWidth; available != nil && *available > 0 && maxX-minX+float64(2*pad) > *available {
		all := slices.Clone(cnodes)
		for _, item := range cards {
			c := layout.NewCNode(item.w, item.h, item.parent)
			c.X, c.Y = item.x, item.y
			all = append(all, c)
		}
		if wrapped, ok := layout.WrapComponents(all, cedges, max(1, *available-float64(2*pad)), numberOption(conf, "nodeSpacing", nodeSep)); ok {
			copy(cnodes, all)
			for i := range cards {
				cards[i].x, cards[i].y = all[len(cnodes)+i].X, all[len(cnodes)+i].Y
			}
			minX, minY = 0, 0
			maxX, maxY = wrapped.Width, wrapped.Height
		}
	}

	ox := pad - minX
	oy := pad - minY
	width := math.Ceil(maxX - minX + float64(2*pad))
	height := math.Ceil(maxY - minY + float64(2*pad))
	links := []core.Link{}

	crossings := svg.Crossings{}
	if len(nodes) > 0 && nodes[0].isGroup {
		for _, e := range cedges {
			svg.MarkCrossings(crossings, e.Route)
		}
	}

	num := svg.Num
	smallAttr := jsnum.Format(small)

	classSvg := func(v *view, cls *classNode, id, at string) string {
		w, h, style := v.w, v.h, v.style
		left := float64(-w / 2)
		top := float64(-h / 2)
		memberSize := jsmath.Round(v.fontSize * 0.875)
		lineHeight := jsmath.Round(memberSize * 1.5)
		body := `<rect x="` + num(left) + `" y="` + num(top) + `" width="` + num(w) + `" height="` + num(h) + `" fill="var(--_s)" stroke="var(--_b)"` + svg.Rounded(style.Shape) + `/>`
		first := top + v.titleH
		second := first + v.attributesH
		if h > v.titleH {
			stroke := ""
			if style.Stroke != "" {
				stroke = ` style="stroke:` + svg.Esc(style.Stroke) + `"`
			}
			body += `<path class="pele-divider" d="M` + num(left) + `,` + num(first) + `H` + num(-left) + `M` + num(left) + `,` + num(second) + `H` + num(-left) + `" fill="none" stroke="var(--_b)"` + stroke + `/>`
		}
		y := top + titlePad
		for _, annotation := range v.annotations {
			if annotation.math == nil {
				body += `<text class="pele-annotation" x="0" y="` + num(y+float64(tinyHeight/2)+float64(tiny*0.35)) + `" text-anchor="middle" font-size="` + jsnum.Format(tiny) + `" fill="var(--_m)">` + svg.Esc(annotation.text) + `</text>`
				y += tinyHeight
			} else {
				h := max(tinyHeight, annotation.math.Height)
				body += svg.LabelSVG(*annotation.math, 0, y+float64(h/2), ` class="pele-annotation" font-size="`+jsnum.Format(tiny)+`" fill="var(--_m)"`, icons)
				y += h
			}
		}
		titleSize := ""
		if v.fontSize != size {
			titleSize = ` font-size="` + num(v.fontSize) + `"`
		}
		body += svg.LabelSVG(v.label, 0, (y+first-titlePad)/2, ` class="pele-label"`+titleSize+` font-weight="var(--_hw)"`+style.Text, icons)
		lines := func(list []line, kind string, from float64) string {
			out := ""
			baseline := from + sectionPad + float64(lineHeight/2) + float64(memberSize*0.35)
			for _, l := range list {
				var mark string
				switch l.classifier {
				case "$":
					if style.Text != "" {
						mark = ` pele-static"` + style.Text[:len(style.Text)-1] + `text-decoration:underline"`
					} else {
						mark = ` pele-static" style="text-decoration:underline"`
					}
				case "*":
					mark = ` pele-abstract" font-style="italic"` + style.Text
				default:
					mark = `"` + style.Text
				}
				out += `<text class="pele-member pele-` + kind + mark + ` x="` + num(left+padX) + `" y="` + num(baseline) + `">` + svg.Esc(l.text) + `</text>`
				baseline += lineHeight
			}
			return out
		}
		if len(v.attributes) > 0 || len(v.methods) > 0 {
			body += `<g class="pele-members" font-size="` + jsnum.Format(memberSize) + `">` + lines(v.attributes, "attribute", first) + lines(v.methods, "method", second) + `</g>`
		}
		if cls.tooltip != "" {
			body = `<title>` + svg.EscText(cls.tooltip) + `</title>` + body
		}
		if cls.link != "" {
			target := ""
			if cls.linkTarget != "" && isTarget(cls.linkTarget) {
				target = ` target="` + cls.linkTarget + `"`
			}
			body = util.Linked(body, cls.link, cls.id, &links, options, target)
		}
		return `<g class="pele-node pele-class` + svg.ClassNames(svg.StripDefault(cls.cssClasses)) + `" data-id="` + id + `"` + at + `>` + body + `</g>`
	}

	var clusters, struckTitles, nodesOut strings.Builder
	parentOf := func(i int) int { return nodes[i].parent }
	for i, v := range views {
		c := &cnodes[i]
		node := v.node
		x := c.X + ox
		y := c.Y + oy
		id := svg.EscText(node.id)
		w, h := v.w, v.h
		if c.IsGroup {
			w, h = c.W, c.H
		}
		if node.isGroup {
			titleX := c.X
			if c.IsGroup {
				titleX = svg.ClusterTitleX(c, v.label.Width, crossings, groupPad)
			}
			titleY := y - float64(h/2) + 8 + float64(v.label.Height/2)
			const titleAttrs = ` class="pele-cluster-label" fill="var(--_m)"`
			struck := c.IsGroup && svg.TitleCrossed(c, v.label.Width, titleX, crossings)
			if struck {
				struckTitles.WriteString(svg.StruckTitle(v.label, titleX+ox, titleY, titleAttrs, id, parentOf, i, nil))
			}
			clusters.WriteString(`<g class="pele-cluster pele-namespace" data-id="` + id + `">` +
				`<rect x="` + num(x-float64(w/2)) + `" y="` + num(y-float64(h/2)) + `" width="` + num(w) + `" height="` + num(h) + `" fill="var(--_a)" fill-opacity="0.5" stroke="var(--_b)"` + svg.RoundedStyle + `/>`)
			if !struck {
				clusters.WriteString(svg.LabelSVG(v.label, titleX+ox, titleY, titleAttrs, nil))
			}
			clusters.WriteString(`</g>`)
			continue
		}
		at := ` transform="translate(` + num(x) + `,` + num(y) + `)"`
		switch {
		case node.classNode != nil:
			nodesOut.WriteString(classSvg(v, node.classNode, id, at))
		case node.shape == shapeNote:
			r := float64(w / 2)
			t := float64(-h / 2)
			nodesOut.WriteString(`<g class="pele-node pele-note" data-id="` + id + `"` + at + `>` +
				`<path d="M` + num(-r) + `,` + num(t) + `H` + num(r-fold) + `L` + num(r) + `,` + num(t+fold) + `V` + num(-t) + `H` + num(-r) + `Z" fill="var(--_a)" stroke="var(--_b)" stroke-linejoin="round"/>` +
				`<path d="M` + num(r-fold) + `,` + num(t) + `V` + num(t+fold) + `H` + num(r) + `" fill="none" stroke="var(--_b)" stroke-linejoin="round"/>` +
				svg.LabelSVG(v.label, 0, 0, ` class="pele-label" font-size="`+smallAttr+`"`, icons) +
				`</g>`)
		default:
			nodesOut.WriteString(`<g class="pele-node pele-interface" data-id="` + id + `"` + at + `>` +
				svg.LabelSVG(v.label, 0, 0, ` class="pele-label" font-size="`+smallAttr+`"`, icons) +
				`</g>`)
		}
	}

	var edgesOut, labelsOut strings.Builder
	for i := range cedges {
		ce := &cedges[i]
		edge := drawn[i]
		label := edgeLabels[i]
		startType := edge.arrowTypeStart
		endType := edge.arrowTypeEnd
		startTrim := classMarkerTrim(startType)
		endTrim := classMarkerTrim(endType)
		var path svg.EdgePath
		if ce.Src == ce.Dst {
			c := &cnodes[ce.Src]
			v := views[ce.Src]
			k := loopAt[i]
			x := c.X + ox + float64(v.w/2)
			y := c.Y + oy
			spread := min(float64(v.h/2)-4, float64(8+k*6))
			reach := float64(loop + k*8)
			path = svg.EdgePath{
				D: `M` + num(x+startTrim) + `,` + num(y-spread) + `C` + num(x+reach+startTrim) + `,` + num(y-spread) + ` ` +
					num(x+reach+endTrim) + `,` + num(y+spread) + ` ` + num(x+endTrim) + `,` + num(y+spread),
				SX: x, SY: y - spread, SDX: -1, SDY: 0,
				EX: x, EY: y + spread, EDX: -1, EDY: 0,
			}
		} else {
			route := ce.Route
			for k := 0; k < len(route); k += 3 {
				route[k] += ox
				route[k+1] += oy
			}
			path = svg.RoutePath(route, "", startTrim, endTrim, svg.DefaultReach(startTrim), svg.DefaultReach(endTrim))
		}
		attrs := ""
		switch edge.pattern {
		case patternDashed:
			attrs = ` stroke-dasharray="6 4"`
		case patternDotted:
			attrs = ` stroke-dasharray="2 4"`
		}
		kind := ` pele-relation`
		if edge.pattern == patternDotted {
			kind = ` pele-note-edge`
		}
		edgesOut.WriteString(`<g class="pele-edge` + kind + `" data-id="` + svg.EscText(edge.id) + `">` +
			`<path d="` + path.D + `"` + attrs + `/>` +
			classMarker(startType, path.SX, path.SY, path.SDX, path.SDY, lineColor) +
			classMarker(endType, path.EX, path.EY, path.EDX, path.EDY, lineColor) +
			`</g>`)
		if label.Width > 0 {
			labelsOut.WriteString(svg.EdgeLabelSVG(edge.id, label, ce.LabelX+ox, ce.LabelY+oy, "", icons))
		}
	}

	var cardsOut strings.Builder
	for _, item := range cards {
		if item.math != nil {
			cardsOut.WriteString(svg.LabelSVG(*item.math, item.x+ox, item.y+oy, ` class="pele-cardinality"`, icons))
		} else {
			cardsOut.WriteString(`<text class="pele-cardinality" x="` + num(item.x+ox) + `" y="` + num(item.y+oy+float64(tiny*0.35)) + `" text-anchor="middle">` + svg.Esc(item.text) + `</text>`)
		}
	}

	var content strings.Builder
	if clusters.Len() > 0 {
		content.WriteString(`<g class="pele-clusters">` + clusters.String() + `</g>`)
	}
	if edgesOut.Len() > 0 {
		content.WriteString(`<g class="pele-edges" fill="none" stroke="` + lineColor + `" stroke-linecap="round">` + edgesOut.String() + `</g>`)
	}
	if struckTitles.Len() > 0 {
		content.WriteString(`<g class="pele-cluster-titles">` + struckTitles.String() + `</g>`)
	}
	if labelsOut.Len() > 0 {
		content.WriteString(`<g class="pele-edge-labels" font-size="` + smallAttr + `">` + labelsOut.String() + `</g>`)
	}
	if cardsOut.Len() > 0 {
		content.WriteString(`<g class="pele-cardinalities" font-size="` + jsnum.Format(tiny) + `" fill="var(--_m)">` + cardsOut.String() + `</g>`)
	}
	content.WriteString(`<g class="pele-nodes">` + nodesOut.String() + `</g>`)

	titled := svg.WithTitle(db.title, measurer, size, pad, width, height, content.String(), icons)
	doc := svg.Document("class", titled.Width, titled.Height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccTitle: db.accTitle, AccDescr: db.accDescr}, titled.Content)
	return core.Rendered{SVG: doc, Width: titled.Width, Height: titled.Height, Links: links}
}
