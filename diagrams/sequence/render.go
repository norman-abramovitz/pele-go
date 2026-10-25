package sequence

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/omap"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

// Arrow heads.
const (
	headNone = iota
	headArrow
	headCross
	headOpen
	headHalfSolid
	headHalfStick
)

type lineStyle struct {
	dotted bool
	head   int
	// atStart and atEnd say where the heads are: at the receiver, at the sender only (the
	// reverse arrows), or at both.
	atStart, atEnd bool
	bottom         bool
}

var styles = func() map[int]lineStyle {
	s := map[int]lineStyle{}
	style := func(typ int, dotted bool, head, ends int, bottom bool) {
		s[typ] = lineStyle{dotted: dotted, head: head, atStart: ends != 1, atEnd: ends != 0, bottom: bottom}
	}
	style(lineSolid, false, headArrow, 1, false)
	style(lineDotted, true, headArrow, 1, false)
	style(lineSolidCross, false, headCross, 1, false)
	style(lineDottedCross, true, headCross, 1, false)
	style(lineSolidOpen, false, headNone, 1, false)
	style(lineDottedOpen, true, headNone, 1, false)
	style(lineSolidPoint, false, headOpen, 1, false)
	style(lineDottedPoint, true, headOpen, 1, false)
	style(lineBidirectionalSolid, false, headArrow, 2, false)
	style(lineBidirectionalDotted, true, headArrow, 2, false)
	for _, dotted := range []bool{false, true} {
		d := pick(dotted, 10, 0)
		style(lineSolidTop+d, dotted, headHalfSolid, 1, false)
		style(lineSolidBottom+d, dotted, headHalfSolid, 1, true)
		style(lineStickTop+d, dotted, headHalfStick, 1, false)
		style(lineStickBottom+d, dotted, headHalfStick, 1, true)
		style(lineSolidArrowTopReverse+d, dotted, headHalfSolid, 0, false)
		style(lineSolidArrowBottomReverse+d, dotted, headHalfSolid, 0, true)
		style(lineStickArrowTopReverse+d, dotted, headHalfStick, 0, false)
		style(lineStickArrowBottomReverse+d, dotted, headHalfStick, 0, true)
	}
	return s
}()

var frameKinds = map[int]string{
	lineLoopStart:     "loop",
	lineAltStart:      "alt",
	lineOptStart:      "opt",
	lineParStart:      "par",
	lineParOverStart:  "par",
	lineCriticalStart: "critical",
	lineBreakStart:    "break",
	lineRectStart:     "rect",
}

const (
	msgPadDefault = 12
	selfW         = 28
	selfH         = 22
	actW          = 10
	actStep       = 4
	actMin        = 14
	noteGap       = 10
	noteOverWide  = 16
	framePadWide  = 20
	frameNestWide = 10
	tabH          = 20
	frameText     = 240
	badgeH        = 16
	centralR      = 4
	dash          = ` stroke-dasharray="4 4"`
)

type numbering struct {
	// index is the number each message line carries, whether or not it is shown. NaN for
	// everything else.
	index []float64
	shown []bool
	// visible says whether numbers are on after the last statement.
	visible bool
}

func isLineMessage(typ int) bool {
	_, ok := styles[typ]
	return ok
}

// numberMessages counts message lines the way Mermaid's renderer does while it draws.
func numberMessages(messages []seqMessage, always bool) numbering {
	index := make([]float64, len(messages))
	shown := make([]bool, len(messages))
	current := 1.0
	step := 1.0
	visible := false
	for i, m := range messages {
		index[i] = math.NaN()
		if m.typ == lineAutonumber {
			spec := m.numbering
			if spec.start != 0 && spec.start == spec.start {
				current = spec.start
			}
			if spec.step != 0 && spec.step == spec.step {
				step = spec.step
			}
			visible = spec.visible
		} else if isLineMessage(m.typ) {
			index[i] = current
			shown[i] = visible || always
			current = jsmath.Round((current+step)*100) / 100
		}
	}
	return numbering{index: index, shown: shown, visible: visible}
}

type column struct {
	id          string
	actor       *seqActor
	kind        string
	label       text.Label
	size        actorSizeT
	x           float64
	createAt    int
	destroyAt   int
	top         float64
	lifeStart   float64
	lifeEnd     float64
	activations []float64
}

const (
	viewLine = iota
	viewSelf
	viewNote
)

type view struct {
	kind  int
	a, b  int
	label text.Label
	x, w  float64
}

type seqFrame struct {
	kind          string
	x0, x1        float64
	top           float64
	title         text.Label
	tabW          float64
	fill          string
	dividers      []int
	dividerY      []float64
	dividerLabels []text.Label
}

type boxRun struct {
	box         *seqBox
	first, last int
	title       text.Label
	grow        float64
}

// option is a number from sequence config when it is in range, and fallback otherwise.
func option(conf *jsobj.Object, key string, fallback float64) float64 {
	v, _ := conf.Get(key)
	if n, ok := v.(float64); ok && n >= 0 && n <= 2000 {
		return n
	}
	return fallback
}

func half(x, y, dir, up float64, stick bool) string {
	bx := svg.Num(x - float64(dir*svg.Arrow))
	if stick {
		return `<path class="pele-marker" d="M` + svg.Num(x) + `,` + svg.Num(y) + `L` + bx + `,` + svg.Num(y+float64(up*5)) + `"/>`
	}
	return `<path class="pele-marker" d="M` + svg.Num(x) + `,` + svg.Num(y) + `L` + bx + `,` + svg.Num(y+float64(up*4.5)) + `V` + svg.Num(y) +
		`Z" fill="var(--_l)" stroke="none"/>`
}

// head draws the head whose tip is at (x, y) and points along dir. up is the side a half
// head sits on.
func head(typ int, x, y, dir, up float64) string {
	switch typ {
	case headArrow:
		return svg.Marker("arrow_point", x, y, dir, 0, "var(--_l)")
	case headCross:
		return svg.Marker("arrow_cross", x, y, dir, 0, "var(--_l)")
	case headOpen:
		bx := svg.Num(x - float64(dir*svg.Arrow))
		return `<path class="pele-marker" d="M` + bx + `,` + svg.Num(y-4.5) + `L` + svg.Num(x) + `,` + svg.Num(y) + `L` + bx + `,` + svg.Num(y+4.5) +
			`" stroke-linejoin="round"/>`
	case headHalfSolid:
		return half(x, y, dir, up, false)
	case headHalfStick:
		return half(x, y, dir, up, true)
	}
	return ""
}

func trim(typ int) float64 {
	switch typ {
	case headArrow:
		return svg.Arrow - 1
	case headCross:
		return 4
	}
	return 0
}

func renderSequence(db *seqDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	return common.Tighten(rc.Options, func(tight bool) core.Rendered { return draw(db, config, rc, tight) })
}

func draw(db *seqDb, config *jsobj.Object, rc *core.Context, tight bool) core.Rendered {
	options := rc.Options
	seqConf, _ := config.Get("sequence")
	conf, _ := seqConf.(*jsobj.Object)
	confIs := func(key string, want any) bool {
		v, _ := conf.Get(key)
		return v == want
	}
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	small := jsmath.Round(size * 0.875)
	tiny := jsmath.Round(size * 0.75)
	measurer := rc.Measurer()
	icons := rc.Icons
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	inf := math.Inf(1)
	mirror := !confIs("mirrorActors", false)
	rightAngles := confIs("rightAngles", true)
	actorMargin := min(option(conf, "actorMargin", 40), pick(tight, 8, inf))
	minW := min(option(conf, "width", 64), pick(tight, 48, inf))
	minH := option(conf, "height", jsmath.Round(size*1.5)+20)
	wrapW := option(conf, "width", 160)
	if tight {
		maxWidth := 320.0
		if options.MaxWidth != nil {
			maxWidth = *options.MaxWidth
		}
		wrapW = min(wrapW, max(32, (maxWidth-float64(2*pad))/max(1, float64(db.actors.Len()))-actorMargin-24))
	}
	// Mermaid's default of 35 corresponds to Pele's default gap. Other values move it by the difference.
	msgGap := max(4, 14+option(conf, "messageMargin", 35)-35)
	notePad := min(option(conf, "noteMargin", 10), pick(tight, 6, inf))
	boxPad := min(option(conf, "boxMargin", 10), pick(tight, 6, inf))
	align := 0
	if confIs("messageAlign", "left") {
		align = -1
	} else if confIs("messageAlign", "right") {
		align = 1
	}

	msgPad := pick(tight, 6.0, msgPadDefault)
	noteOver := pick(tight, 8.0, noteOverWide)
	framePad := pick(tight, 12.0, framePadWide)
	frameNest := pick(tight, 6.0, frameNestWide)
	messages := db.messages
	count := len(messages)
	nums := numberMessages(messages, confIs("showSequenceNumbers", true))
	layout := func(raw string, fontSize float64, wrap bool, width float64, base text.Style) text.Label {
		label := text.Layout(raw, false, measurer, fontSize, pick(tight || wrap, width, inf), base, icons)
		if tight {
			return common.BreakLabel(label, measurer, max(120, width))
		}
		return label
	}
	label := func(raw string, fontSize float64, wrap bool) text.Label {
		return layout(raw, fontSize, wrap, wrapW, 0)
	}

	var used map[string]bool
	if confIs("hideUnusedParticipants", true) {
		used = map[string]bool{}
		for _, m := range messages {
			used[m.from] = true
			used[m.to] = true
		}
	}

	nextLine := make([]int, count+1)
	nextLine[count] = -1
	for i := count - 1; i >= 0; i-- {
		if isLineMessage(messages[i].typ) {
			nextLine[i] = i
		} else {
			nextLine[i] = nextLine[i+1]
		}
	}

	var cols []*column
	colOf := map[string]int{}
	paddingX := pick(tight, 8.0, actorPadX)
	for id, actor := range db.actors.All() {
		if used != nil && !used[id] {
			continue
		}
		kind := actorKind(actor.typ)
		lbl := label(actor.description, size, actor.wrap)
		createAt, destroyAt := -1, -1
		if created, ok := db.createdActors.Get(id); ok {
			createAt = nextLine[min(created, count)]
		}
		if createAt >= 0 && messages[createAt].to != id {
			createAt = -1
		}
		if destroyed, ok := db.destroyedActors.Get(id); ok {
			destroyAt = nextLine[min(destroyed, count)]
		}
		if destroyAt >= 0 && messages[destroyAt].to != id && messages[destroyAt].from != id {
			destroyAt = -1
		}
		colOf[id] = len(cols)
		cols = append(cols, &column{
			id:        id,
			actor:     actor,
			kind:      kind,
			label:     lbl,
			size:      actorSize(kind, lbl, minW, minH, paddingX),
			createAt:  createAt,
			destroyAt: destroyAt,
			lifeEnd:   -1,
		})
	}
	last := len(cols) - 1

	// Widths between lifelines. Every message and note says how far apart the lifelines it touches must be.
	var needs omap.Map[int, float64]
	need := func(lo, hi int, width float64) {
		if lo < 0 || hi > last {
			return
		}
		key := lo*len(cols) + hi
		if have, _ := needs.Get(key); have < width {
			needs.Set(key, width)
		}
	}
	badgeWidth := func(i int) float64 {
		if !nums.shown[i] {
			return 0
		}
		return max(badgeH, measurer.Width(jsnum.Format(nums.index[i]), tiny, 0)+8)
	}

	views := make([]*view, count)
	for i := range messages {
		m := &messages[i]
		if m.from == "" || m.to == "" || m.numbering != nil {
			continue
		}
		a, okA := colOf[m.from]
		b, okB := colOf[m.to]
		if !okA || !okB {
			continue
		}
		if m.typ == lineNote {
			lbl := label(m.message, small, m.wrap)
			w := lbl.Width + float64(2*notePad)
			switch {
			case m.placement == placementLeftOf:
				need(a-1, a, w+2*noteGap+actW)
			case m.placement == placementRightOf:
				need(a, a+1, w+2*noteGap+actW)
			case a == b:
				need(a-1, a, float64(w/2)+noteGap)
				need(a, a+1, float64(w/2)+noteGap)
			default:
				need(min(a, b), max(a, b), w-float64(2*noteOver))
			}
			views[i] = &view{kind: viewNote, a: a, b: b, label: lbl, w: w}
		} else if isLineMessage(m.typ) {
			lbl := label(m.message, small, m.wrap)
			badge := badgeWidth(i)
			if a == b {
				need(a, a+1, selfW+8+lbl.Width+16+actW+badge)
				views[i] = &view{kind: viewSelf, a: a, b: b, label: lbl}
			} else {
				created := 0.0
				if cols[b].createAt == i {
					created = float64(cols[b].size.w / 2)
				}
				central := 0.0
				if m.centralConnection != 0 {
					central = 4 * centralR
				}
				width := lbl.Width
				if tight {
					width = min(lbl.Width, max(16, wrapW-16))
				}
				need(min(a, b), max(a, b), width+float64(2*msgPad)+actW+badge+created+central)
				views[i] = &view{kind: viewLine, a: a, b: b, label: lbl}
			}
		}
	}

	var runs []*boxRun
	for i := 0; i <= last; i++ {
		box := cols[i].actor.box
		var run *boxRun
		if len(runs) > 0 {
			run = runs[len(runs)-1]
		}
		if box != nil && run != nil && run.box == box && run.last == i-1 {
			run.last = i
		} else if box != nil {
			runs = append(runs, &boxRun{box: box, first: i, last: i, title: label(box.name, small, box.wrap)})
		}
	}

	gaps := make([]float64, max(last, 0))
	for i := 0; i < last; i++ {
		left := cols[i]
		right := cols[i+1]
		boxed := 0.0
		if left.actor.box != right.actor.box {
			boxed = pick(left.actor.box != nil, boxPad, 0) + pick(right.actor.box != nil, boxPad, 0)
		}
		have, _ := needs.Get(i*len(cols) + i + 1)
		gaps[i] = max(float64(left.size.w/2)+float64(right.size.w/2)+actorMargin+boxed, have)
	}
	// A message across several lifelines widens each gap it crosses by an equal share of what is missing.
	type span struct {
		lo, hi int
		width  float64
	}
	var spans []span
	for key, width := range needs.All() {
		lo := key / len(cols)
		hi := key % len(cols)
		if hi-lo > 1 {
			spans = append(spans, span{lo, hi, width})
		}
	}
	slices.SortStableFunc(spans, func(p, q span) int {
		if d := p.hi - p.lo - (q.hi - q.lo); d != 0 {
			return d
		}
		return p.lo - q.lo
	})
	for _, s := range spans {
		total := 0.0
		for i := s.lo; i < s.hi; i++ {
			total += gaps[i]
		}
		if total >= s.width {
			continue
		}
		share := (s.width - total) / float64(s.hi-s.lo)
		for i := s.lo; i < s.hi; i++ {
			gaps[i] += share
		}
	}
	place := func() {
		for i := 1; i <= last; i++ {
			cols[i].x = cols[i-1].x + gaps[i-1]
		}
	}
	place()
	for _, run := range runs {
		inner := cols[run.last].x + float64(cols[run.last].size.w/2) - (cols[run.first].x - float64(cols[run.first].size.w/2))
		run.grow = max(0, float64((run.title.Width-inner)/2))
		if run.grow == 0 {
			continue
		}
		if run.first > 0 {
			gaps[run.first-1] += run.grow
		}
		if run.last < last {
			gaps[run.last] += run.grow
		}
	}
	place()

	minX := inf
	maxX := -inf
	extend := func(x0, x1 float64) {
		if x0 < minX {
			minX = x0
		}
		if x1 > maxX {
			maxX = x1
		}
	}
	for _, col := range cols {
		extend(col.x-float64(col.size.w/2), col.x+float64(col.size.w/2))
	}
	for _, run := range runs {
		extend(cols[run.first].x-float64(cols[run.first].size.w/2)-boxPad-run.grow, cols[run.last].x+float64(cols[run.last].size.w/2)+boxPad+run.grow)
	}

	// Horizontal extents of every block, from what it holds.
	frames := make([]*seqFrame, count)
	var open []*seqFrame
	spanAll := func() (float64, float64) {
		if last < 0 {
			return 0, 120
		}
		return cols[0].x - framePad, cols[last].x + framePad
	}
	for i := range messages {
		m := &messages[i]
		typ := m.typ
		frameKind, isFrame := frameKinds[typ]
		v := views[i]
		switch {
		case isFrame:
			f := &seqFrame{kind: frameKind, x0: inf, x1: -inf, dividers: []int{i}}
			if frameKind == "rect" && m.numbering == nil {
				f.fill = m.message
			}
			frames[i] = f
			open = append(open, f)
		case typ == lineAltElse || typ == lineParAnd || typ == lineCriticalOption:
			if len(open) > 0 {
				f := open[len(open)-1]
				f.dividers = append(f.dividers, i)
			}
		case typ == lineLoopEnd || typ == lineAltEnd || typ == lineOptEnd || typ == lineParEnd ||
			typ == lineCriticalEnd || typ == lineBreakEnd || typ == lineRectEnd:
			if len(open) == 0 {
				continue
			}
			f := open[len(open)-1]
			open = open[:len(open)-1]
			frames[i] = f
			if f.x0 > f.x1 {
				f.x0, f.x1 = spanAll()
			}
			if f.kind != "rect" {
				f.tabW = measurer.Width(f.kind, tiny, text.Bold) + 14
				// A block grows to the right to give its texts up to frameText of width before they wrap.
				for k, d := range f.dividers {
					raw := messages[d].message
					tab := 0.0
					if k == 0 {
						tab = f.tabW + 8
					}
					room := f.x1 - f.x0 - 16 - tab
					var lbl text.Label
					if raw != "" {
						lbl = layout("["+raw+"]", small, true, max(room, pick(tight, min(frameText, wrapW), frameText)), 0)
					}
					if lbl.Width > room {
						f.x1 += lbl.Width - room
					}
					if k == 0 {
						f.title = lbl
					} else {
						f.dividerLabels = append(f.dividerLabels, lbl)
					}
				}
			}
			extend(f.x0, f.x1)
			if len(open) > 0 {
				parent := open[len(open)-1]
				parent.x0 = min(parent.x0, f.x0-frameNest)
				parent.x1 = max(parent.x1, f.x1+frameNest)
			}
		case v != nil:
			a := cols[v.a]
			b := cols[v.b]
			var x0, x1 float64
			switch v.kind {
			case viewNote:
				switch {
				case m.placement == placementLeftOf:
					v.x = a.x - noteGap - v.w
				case m.placement == placementRightOf:
					v.x = a.x + noteGap
				case v.a == v.b:
					v.x = a.x - float64(v.w/2)
				default:
					lo := min(a.x, b.x) - noteOver
					hi := max(a.x, b.x) + noteOver
					if tight || m.wrap {
						v.label = layout(m.message, small, true, max(wrapW, hi-lo-float64(2*notePad)), 0)
					}
					v.w = max(hi-lo, v.label.Width+float64(2*notePad))
					v.x = float64((lo + hi - v.w) / 2)
				}
				x0 = v.x
				x1 = v.x + v.w
			case viewSelf:
				x0 = a.x
				x1 = a.x + actW + selfW + 8 + v.label.Width + badgeWidth(i)
			default:
				x0 = min(a.x, b.x)
				x1 = max(a.x, b.x)
				if tight || m.wrap {
					v.label = layout(m.message, small, true, max(wrapW, x1-x0-float64(2*msgPad)-actW), 0)
				}
				if b.createAt == i {
					if b.x > a.x {
						x1 += float64(b.size.w / 2)
					} else {
						x0 -= float64(b.size.w / 2)
					}
				}
			}
			extend(x0, x1)
			if len(open) > 0 {
				f := open[len(open)-1]
				f.x0 = min(f.x0, x0-framePad)
				f.x1 = max(f.x1, x1+framePad)
			}
		}
	}

	// Vertical placement, top to bottom, drawing as it goes.
	boxTitleH := 0.0
	for _, run := range runs {
		boxTitleH = max(boxTitleH, run.title.Height)
	}
	boxTop := 0.0
	if len(runs) > 0 {
		boxTop = boxPad + pick(boxTitleH > 0, boxTitleH+4, 0)
	}
	rowH := 0.0
	for _, col := range cols {
		if col.createAt < 0 {
			rowH = max(rowH, col.size.h)
		}
	}
	headBottom := boxTop + rowH
	for _, col := range cols {
		col.top = headBottom - col.size.h
		col.lifeStart = headBottom
	}

	xs := make([]float64, len(cols))
	for i, col := range cols {
		xs[i] = col.x
	}
	// crosses reports whether a lifeline runs between two x positions, where text would be
	// struck through.
	crosses := func(x0, x1 float64) bool {
		lo, hi := 0, len(xs)
		for lo < hi {
			mid := (lo + hi) >> 1
			if xs[mid] <= x0 {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		return lo < len(xs) && xs[lo] < x1
	}
	backed := func(lbl text.Label, cx, cy float64, attrs string) string {
		if lbl.Width == 0 {
			return ""
		}
		l := cx - float64(lbl.Width/2)
		back := ""
		if crosses(l, l+lbl.Width) {
			back = `<rect x="` + svg.Num(l-3) + `" y="` + svg.Num(cy-float64(lbl.Height/2)+2) + `" width="` + svg.Num(lbl.Width+6) +
				`" height="` + svg.Num(lbl.Height-4) + `" rx="3" fill="var(--_bg)" stroke="none"/>`
		}
		return back + svg.LabelSVG(lbl, cx, cy, attrs, icons)
	}
	leftEdge := func(col *column, extra int) float64 {
		if len(col.activations)+extra > 0 {
			return col.x - actW/2
		}
		return col.x
	}
	rightEdge := func(col *column, extra int) float64 {
		depth := len(col.activations) + extra
		if depth > 0 {
			return col.x + actW/2 + float64((depth-1)*actStep)
		}
		return col.x
	}

	// Shaded blocks and activations close from the inside out. They are written in reverse, so that
	// the inner ones end up on top.
	var rectsOut, activationsOut []string
	var framesOut, edgesOut, labelsOut, notesOut, actorsOut, crossesOut strings.Builder
	endActivation := func(col *column, end float64) float64 {
		start := col.activations[len(col.activations)-1]
		col.activations = col.activations[:len(col.activations)-1]
		bottom := max(end, start+actMin)
		x := col.x - actW/2 + float64(len(col.activations)*actStep)
		extend(x, x+actW)
		activationsOut = append(activationsOut, `<rect class="pele-activation" data-id="`+svg.EscText(col.id)+`" x="`+svg.Num(x)+`" y="`+svg.Num(start)+
			`" width="`+strconv.Itoa(actW)+`" height="`+svg.Num(bottom-start)+`" rx="2"/>`)
		return bottom
	}
	// destroy ends the lifeline of a destroyed participant in a cross a little below the
	// message that ends it.
	destroy := func(col *column, lineY float64) float64 {
		cx := col.x
		cy := lineY + 16
		col.lifeEnd = cy
		crossesOut.WriteString(`<path class="pele-destroy" data-id="` + svg.EscText(col.id) + `" d="M` + svg.Num(cx-6) + `,` + svg.Num(cy-6) + `L` + svg.Num(cx+6) + `,` + svg.Num(cy+6) +
			`M` + svg.Num(cx+6) + `,` + svg.Num(cy-6) + `L` + svg.Num(cx-6) + `,` + svg.Num(cy+6) + `"/>`)
		return cy + 6
	}
	actorSvg := func(col *column, top float64, footer bool) string {
		classes := ""
		if cls, ok := col.actor.properties.Get("class"); ok {
			if s, ok := cls.(string); ok {
				classes = svg.ClassNames(s)
			}
		}
		return `<g class="pele-node pele-actor pele-actor-` + col.kind + pick(footer, " pele-actor-footer", "") + classes + `" data-id="` + svg.EscText(col.id) + `">` +
			drawActor(col.kind, col.x, top, col.size, col.label, icons) +
			`</g>`
	}
	badge := func(i int, cx, cy, w float64) string {
		return `<g class="pele-sequence-number"><rect x="` + svg.Num(cx-float64(w/2)) + `" y="` + svg.Num(cy-badgeH/2) + `" width="` + svg.Num(w) +
			`" height="` + strconv.Itoa(badgeH) + `" rx="` + strconv.Itoa(badgeH/2) + `" fill="var(--_l)" stroke="none"/><text x="` + svg.Num(cx) +
			`" y="` + svg.Num(cy+float64(tiny*0.35)) + `" text-anchor="middle" font-size="` + jsnum.Format(tiny) + `" fill="var(--_bg)" stroke="none">` +
			svg.Esc(jsnum.Format(nums.index[i])) + `</text></g>`
	}

	y := headBottom + 6
	var stack []*seqFrame
	for i := range messages {
		m := &messages[i]
		typ := m.typ
		v := views[i]
		f := frames[i]

		switch {
		case typ == lineActiveStart:
			if col, ok := colOf[m.from]; ok && m.from != "" {
				cols[col].activations = append(cols[col].activations, y)
			}
		case typ == lineActiveEnd:
			if col, ok := colOf[m.from]; ok && m.from != "" && len(cols[col].activations) > 0 {
				y = max(y, endActivation(cols[col], y))
			}
		case f != nil && f.dividers[0] == i:
			stack = append(stack, f)
			if f.kind == "rect" {
				f.top = y + 8
				y = f.top
			} else {
				f.top = y + 12
				y = f.top + max(tabH, f.title.Height+2)
			}
		case f != nil:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			bottom := y + 12
			x := svg.Num(f.x0)
			w := svg.Num(f.x1 - f.x0)
			if f.kind == "rect" {
				fill := ""
				if f.fill != "" {
					fill = svg.ResolveStyle([]string{"fill:" + f.fill}).Shape
				}
				rectsOut = append(rectsOut, `<rect class="pele-rect" x="`+x+`" y="`+svg.Num(f.top)+`" width="`+w+`" height="`+svg.Num(bottom-f.top)+`"`+
					svg.Rounded(pick(fill != "", "", ` fill-opacity="0.5"`)+fill)+`/>`)
			} else {
				var inner strings.Builder
				inner.WriteString(`<rect x="` + x + `" y="` + svg.Num(f.top) + `" width="` + w + `" height="` + svg.Num(bottom-f.top) + `" fill="none"` + svg.RoundedStyle + `/>`)
				for k, dy := range f.dividerY {
					lbl := f.dividerLabels[k]
					inner.WriteString(`<path d="M` + x + `,` + svg.Num(dy) + `H` + svg.Num(f.x1) + `"` + dash + `/>` +
						backed(lbl, f.x0+8+float64(lbl.Width/2), dy+2+float64(lbl.Height/2), ` class="pele-frame-label" fill="var(--_m)" stroke="none"`))
				}
				inner.WriteString(`<rect x="` + x + `" y="` + svg.Num(f.top) + `" width="` + svg.Num(f.tabW) + `" height="` + strconv.Itoa(tabH) + `" fill="var(--_s)"` + svg.RoundedStyle + `/>` +
					`<text class="pele-frame-kind" x="` + svg.Num(f.x0+float64(f.tabW/2)) + `" y="` + svg.Num(f.top+tabH/2+float64(tiny*0.35)) +
					`" text-anchor="middle" font-size="` + jsnum.Format(tiny) + `" font-weight="var(--_hw)" fill="var(--_m)" stroke="none">` + f.kind + `</text>` +
					backed(f.title, f.x0+f.tabW+8+float64(f.title.Width/2), f.top+float64(max(tabH, f.title.Height)/2), ` class="pele-frame-label" fill="var(--_m)" stroke="none"`))
				framesOut.WriteString(`<g class="pele-frame pele-frame-` + f.kind + `">` + inner.String() + `</g>`)
			}
			y = bottom
		case typ == lineAltElse || typ == lineParAnd || typ == lineCriticalOption:
			if len(stack) > 0 {
				current := stack[len(stack)-1]
				lbl := current.dividerLabels[len(current.dividerY)]
				current.dividerY = append(current.dividerY, y+12)
				y += 12 + pick(lbl.Height > 0, lbl.Height, 0)
			}
		case v != nil && v.kind == viewNote:
			a := cols[v.a]
			h := v.label.Height + float64(2*max(4, notePad-4))
			top := y + 10
			x := v.x
			if m.placement == placementRightOf {
				x += rightEdge(a, 0) - a.x
			} else if m.placement == placementLeftOf {
				x += leftEdge(a, 0) - a.x
			}
			extend(x, x+v.w)
			notesOut.WriteString(`<g class="pele-note" data-id="i` + svg.EscText(m.id) + `">` +
				`<rect x="` + svg.Num(x) + `" y="` + svg.Num(top) + `" width="` + svg.Num(v.w) + `" height="` + svg.Num(h) + `"` + svg.RoundedStyle + `/>` +
				svg.LabelSVG(v.label, x+float64(v.w/2), top+float64(h/2), ` class="pele-label" stroke="none" fill="var(--_fg)"`, icons) +
				`</g>`)
			y = top + h
		case v != nil:
			st := styles[typ]
			a := cols[v.a]
			b := cols[v.b]
			lbl := v.label
			bw := badgeWidth(i)
			// The activation a `+` starts belongs to this message, so the arrow stops at its edge.
			starting := 0
			for k := i + 1; k < count; k++ {
				t := messages[k].typ
				if t == lineActiveStart {
					if messages[k].from == m.to {
						starting++
					}
				} else if t != lineActiveEnd && t != lineCentralConnection && t != lineCentralConnectionReverse {
					break
				}
			}
			up := pick(st.bottom, 1.0, -1)
			attrs := pick(st.dotted, dash, "")
			cc := m.centralConnection
			circleTo := cc == lineCentralConnection || cc == lineCentralConnectionDual
			circleFrom := cc == lineCentralConnectionReverse || cc == lineCentralConnectionDual
			circle := func(cx, cy float64) string {
				return `<circle class="pele-central" cx="` + svg.Num(cx) + `" cy="` + svg.Num(cy) + `" r="` + strconv.Itoa(centralR) + `" fill="var(--_l)" stroke="none"/>`
			}
			tailAtEnd := st.atStart && !st.atEnd
			headTrim := trim(st.head)
			inner := ""
			var lineY float64

			if v.kind == viewSelf {
				top := y + msgGap + max(0, float64((lbl.Height-selfH)/2))
				bottom := top + selfH
				x0 := rightEdge(a, 0)
				if circleFrom {
					x0 = a.x + centralR
				}
				x1 := rightEdge(a, starting)
				if circleTo {
					x1 = a.x + centralR
				}
				out := max(x0, x1) + selfW + float64(bw/2)
				if circleFrom {
					inner += circle(a.x, top)
				}
				if circleTo {
					inner += circle(a.x, bottom)
				}
				if bw > 0 {
					inner += badge(i, pick(tailAtEnd, x1, x0), pick(tailAtEnd, bottom, top), bw)
					if tailAtEnd {
						x1 += float64(bw / 2)
					} else {
						x0 += float64(bw / 2)
					}
				}
				// The loop leaves to the right and comes back heading left, which turns a half head over.
				heads := ""
				if st.atStart {
					heads += head(st.head, x0, top, -1, up)
				}
				if st.atEnd {
					heads += head(st.head, x1, bottom, -1, -up)
				}
				from := svg.Num(x0 + pick(st.atStart, headTrim, 0))
				to := svg.Num(x1 + pick(st.atEnd, headTrim, 0))
				var d string
				if rightAngles {
					d = `M` + from + `,` + svg.Num(top) + `H` + svg.Num(out) + `V` + svg.Num(bottom) + `H` + to
				} else {
					d = `M` + from + `,` + svg.Num(top) + `H` + svg.Num(out-6) + `A6,6 0 0 1 ` + svg.Num(out) + `,` + svg.Num(top+6) + `V` + svg.Num(bottom-6) +
						`A6,6 0 0 1 ` + svg.Num(out-6) + `,` + svg.Num(bottom) + `H` + to
				}
				inner = `<path d="` + d + `"` + attrs + `/>` + heads + inner
				extend(a.x, out+8+lbl.Width)
				if lbl.Width > 0 {
					labelsOut.WriteString(`<g class="pele-edge-label" data-id="i` + svg.EscText(m.id) + `">` +
						svg.LabelSVG(lbl, out+8+float64(lbl.Width/2), float64((top+bottom)/2), "", icons) + `</g>`)
				}
				// A label taller than the loop reaches below it, so the next row starts under the label.
				lineY = max(bottom, float64((top+bottom)/2)+float64(lbl.Height/2))
			} else {
				dir := pick(b.x > a.x, 1.0, -1)
				creating := b.createAt == i
				lineY = y + msgGap + max(pick(lbl.Height > 0, lbl.Height+2, 6), pick(creating, float64(b.size.h/2)-msgGap+6, 0))
				var x0, x1 float64
				if dir > 0 {
					x0, x1 = rightEdge(a, 0), leftEdge(b, starting)
				} else {
					x0, x1 = leftEdge(a, 0), rightEdge(b, starting)
				}
				if creating {
					b.top = lineY - float64(b.size.h/2)
					b.lifeStart = b.top + b.size.h
					x1 = b.x - float64(dir*pick(isFigure(b.kind), 18, float64(b.size.w/2)+1))
				}
				if circleFrom {
					inner += circle(a.x, lineY)
					x0 = a.x + float64(dir*centralR)
				}
				if circleTo {
					inner += circle(b.x, lineY)
					x1 = b.x - float64(dir*centralR)
				}
				if bw > 0 {
					// The number sits on the lifeline at the tail of the arrow, past a connection circle if there is one.
					if tailAtEnd {
						cx := x1
						if circleTo {
							cx = x1 - float64(dir*(float64(bw/2)+2))
						}
						inner += badge(i, cx, lineY, bw)
						x1 = cx - float64(float64(dir*bw)/2)
					} else {
						cx := x0
						if circleFrom {
							cx = x0 + float64(dir*(float64(bw/2)+2))
						}
						inner += badge(i, cx, lineY, bw)
						x0 = cx + float64(float64(dir*bw)/2)
					}
				}
				heads := ""
				if st.atStart {
					heads += head(st.head, x0, lineY, -dir, up*dir)
				}
				if st.atEnd {
					heads += head(st.head, x1, lineY, dir, up*dir)
				}
				from := x0 + pick(st.atStart, float64(dir*headTrim), 0)
				to := x1 - pick(st.atEnd, float64(dir*headTrim), 0)
				inner = `<path d="M` + svg.Num(from) + `,` + svg.Num(lineY) + `H` + svg.Num(to) + `"` + attrs + `/>` + heads + inner
				if lbl.Width > 0 {
					l := min(x0, x1)
					r := max(x0, x1)
					var cx float64
					switch {
					case align < 0:
						cx = l + msgPad + float64(lbl.Width/2)
					case align > 0:
						cx = r - msgPad - float64(lbl.Width/2)
					default:
						cx = float64((l + r) / 2)
					}
					extend(cx-float64(lbl.Width/2), cx+float64(lbl.Width/2))
					labelsOut.WriteString(`<g class="pele-edge-label" data-id="i` + svg.EscText(m.id) + `">` + backed(lbl, cx, lineY-4-float64(lbl.Height/2), "") + `</g>`)
				}
				if creating {
					y = lineY + float64(b.size.h/2) - 6
				}
			}
			edgesOut.WriteString(`<g class="pele-edge pele-message" data-id="i` + svg.EscText(m.id) + `">` + inner + `</g>`)
			y = max(y, lineY)
			if a.destroyAt == i {
				y = max(y, destroy(a, lineY))
			}
			if b.destroyAt == i && b != a {
				y = max(y, destroy(b, lineY))
			}
		}
	}

	y += 20
	for _, col := range cols {
		for len(col.activations) > 0 {
			endActivation(col, y)
		}
	}
	bottom := y
	if mirror {
		footH := 0.0
		for _, col := range cols {
			if col.lifeEnd >= 0 {
				continue
			}
			footH = max(footH, col.size.h)
			actorsOut.WriteString(actorSvg(col, y, true))
		}
		bottom = y + footH
	}
	var lifelines strings.Builder
	for _, col := range cols {
		actorsOut.WriteString(actorSvg(col, col.top, false))
		end := y
		if col.lifeEnd >= 0 {
			end = col.lifeEnd
		}
		if end > col.lifeStart {
			lifelines.WriteString(`<path class="pele-lifeline" data-id="` + svg.EscText(col.id) + `" d="M` + svg.Num(col.x) + `,` + svg.Num(col.lifeStart) + `V` + svg.Num(end) + `"/>`)
		}
	}

	var boxesOut strings.Builder
	for _, run := range runs {
		x0 := cols[run.first].x - float64(cols[run.first].size.w/2) - boxPad - run.grow
		x1 := cols[run.last].x + float64(cols[run.last].size.w/2) + boxPad + run.grow
		fill := ""
		if run.box.fill != "transparent" {
			fill = svg.ResolveStyle([]string{"fill:" + run.box.fill}).Shape
		}
		dataID := ""
		if run.box.name != "" {
			dataID = ` data-id="` + svg.EscText(run.box.name) + `"`
		}
		boxesOut.WriteString(`<g class="pele-cluster pele-box"` + dataID + `>` +
			`<rect x="` + svg.Num(x0) + `" y="0" width="` + svg.Num(x1-x0) + `" height="` + svg.Num(bottom+boxPad) + `" fill="var(--_a)" stroke="var(--_b)"` +
			svg.Rounded(pick(fill != "", "", ` fill-opacity="0.5"`)+fill) + `/>` +
			svg.LabelSVG(run.title, float64((x0+x1)/2), float64(boxPad/2)+2+float64(run.title.Height/2), ` class="pele-cluster-label" fill="var(--_m)"`, icons) +
			`</g>`)
	}
	if len(runs) > 0 {
		bottom += boxPad
	}

	minY := 0.0
	titleOut := ""
	room := common.TitleRoom(options)
	title := svg.TitleLabel(db.title, measurer, size, pick(tight, room, inf), icons)
	if tight {
		title = common.BreakLabel(title, measurer, max(120, room))
	}
	if minX > maxX {
		extend(0, max(title.Width, 40))
	}
	if title.Width > 0 {
		cx := float64((minX + maxX) / 2)
		extend(cx-float64(title.Width/2), cx+float64(title.Width/2))
		minY = -title.Height - 12
		titleOut = svg.LabelSVG(title, cx, minY+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, icons)
	}

	width := math.Ceil(maxX - minX + float64(2*pad))
	height := math.Ceil(bottom - minY + float64(2*pad))
	group := func(open string, content string) string {
		if content == "" {
			return ""
		}
		return open + content + `</g>`
	}
	slices.Reverse(rectsOut)
	slices.Reverse(activationsOut)
	content := `<g transform="translate(` + svg.Num(pad-minX) + `,` + svg.Num(pad-minY) + `)">` +
		titleOut +
		group(`<g class="pele-boxes" font-size="`+jsnum.Format(small)+`">`, boxesOut.String()) +
		group(`<g class="pele-rects" fill="var(--_a)">`, strings.Join(rectsOut, "")) +
		group(`<g class="pele-lifelines" fill="none" stroke="var(--_b)">`, lifelines.String()) +
		group(`<g class="pele-activations" fill="var(--_s)" stroke="var(--_b)">`, strings.Join(activationsOut, "")) +
		group(`<g class="pele-frames" font-size="`+jsnum.Format(small)+`" stroke="var(--_b)">`, framesOut.String()) +
		group(`<g class="pele-edges" fill="none" stroke="var(--_l)" stroke-linecap="round">`, edgesOut.String()) +
		group(`<g class="pele-destroys" fill="none" stroke="var(--_l)" stroke-width="1.5" stroke-linecap="round">`, crossesOut.String()) +
		group(`<g class="pele-edge-labels" font-size="`+jsnum.Format(small)+`">`, labelsOut.String()) +
		group(`<g class="pele-notes" font-size="`+jsnum.Format(small)+`" fill="var(--_a)" stroke="var(--_b)">`, notesOut.String()) +
		group(`<g class="pele-nodes">`, actorsOut.String()) +
		`</g>`
	doc := svg.Document("sequence", width, height, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccTitle: db.accTitle, AccDescr: db.accDescr}, content)
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: []core.Link{}}
}
