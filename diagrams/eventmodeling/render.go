package eventmodeling

import (
	"math"
	"slices"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/omap"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

// series is the series slot for each kind of entity. A UI has none and is drawn plain.
var series = map[string]int{
	"command":   0,
	"event":     1,
	"readmodel": 2,
	"processor": 6,
}

const (
	boxPadX     = 12
	boxPadY     = 10
	boxMinWidth = 96
	boxGap      = 16
	stagger     = 80
	lanePad     = 12
	laneGap     = 8
	dataColumns = 48
	noteColumns = 80
	cardGap     = 8
)

type placedBox struct {
	box  *emBox
	name text.Label
	data []string
	x, y float64
	w, h float64
}

type band struct {
	lane  *emLane
	label text.Label
	right float64
	y, h  float64
}

// hardWrap splits text into lines of at most columns characters, keeping its own line breaks.
func hardWrap(raw string, columns int) []string {
	var out []string
	for _, line := range strings.Split(text.DecodeEntities(raw), "\n") {
		n := jsstr.Len(line)
		if n <= columns {
			out = append(out, line)
			continue
		}
		for i := 0; i < n; i += columns {
			out = append(out, jsstr.Slice(line, i, min(i+columns, n)))
		}
	}
	return out
}

func lines(rows []string, x, y, step float64, attrs string) string {
	var body strings.Builder
	for i, row := range rows {
		if row != "" {
			body.WriteString(`<tspan x="` + svg.Num(x) + `" y="` + svg.Num(y+float64(float64(i)*step)) + `">` + svg.Esc(row) + `</tspan>`)
		}
	}
	if body.Len() == 0 {
		return ""
	}
	return `<text` + attrs + ` xml:space="preserve">` + body.String() + `</text>`
}

func hasData(box *emBox) bool { return box.data != nil && *box.data != "" }

func renderEventModel(model *eventModelingModel, rc *core.Context) core.Rendered {
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered { return draw(model, rc, down) })
}

func fontSize(rc *core.Context) float64 {
	if rc.Options.FontSize != nil {
		return *rc.Options.FontSize
	}
	return 16
}

func padding(rc *core.Context) float64 {
	if rc.Options.Padding != nil {
		return *rc.Options.Padding
	}
	return 8
}

func document(model *eventModelingModel, rc *core.Context, width, height, size float64, content string) core.Rendered {
	opts := rc.Options
	doc := svg.Document("eventmodeling", width, height, size, svg.DocumentOptions{
		IDPrefix:       opts.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     opts.FontFamily,
		FontFamilyMono: opts.FontFamilyMono,
	}, svg.Accessible{AccTitle: model.accTitle, AccDescr: model.accDescr}, content)
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: []core.Link{}}
}

func draw(model *eventModelingModel, rc *core.Context, down bool) core.Rendered {
	if down {
		return verticalEventModel(model, rc)
	}
	size := fontSize(rc)
	nameSize := jsmath.Round(float64(size * 0.875))
	small := jsmath.Round(float64(size * 0.8125))
	mono := jsmath.Round(float64(size * 0.75))
	monoStep := jsmath.Round(float64(mono * 1.4))
	smallStep := jsmath.Round(float64(small * 1.5))
	measurer := rc.Measurer()
	pad := padding(rc)
	widest := func(rows []string, fontSize float64, style text.Style) float64 {
		width := 0.0
		for _, row := range rows {
			width = max(width, measurer.Width(row, fontSize, style))
		}
		return width
	}

	bands := &omap.Map[*emLane, *band]{}
	labelColumn := 0.0
	for _, lane := range model.lanes {
		label := text.Layout(lane.label, false, measurer, small, 4000, text.Bold, rc.Icons)
		labelColumn = max(labelColumn, label.Width)
		bands.Set(lane, &band{lane: lane, label: label, right: math.Inf(-1)})
	}
	labelColumn += 2 * lanePad

	// Time runs left to right. A frame in a new lane starts under the end of the one before
	// it, so the arrow between them can drop straight down; a frame in the same lane follows on.
	var placed []*placedBox
	byBox := map[*emBox]*placedBox{}
	var previous *placedBox
	for _, box := range model.boxes {
		b, _ := bands.Get(box.lane)
		name := text.Layout(box.name, false, measurer, nameSize, 4000, text.Bold, rc.Icons)
		var data []string
		if hasData(box) {
			data = hardWrap(*box.data, dataColumns)
		}
		w := max(boxMinWidth, max(name.Width, widest(data, mono, text.Mono))+2*boxPadX)
		h := 2*boxPadY + name.Height
		if len(data) > 0 {
			h += 6 + float64(float64(len(data))*monoStep)
		}
		x := 0.0
		if previous != nil {
			staggered := previous.x + max(float64(previous.w/2), previous.w-stagger)
			if previous.box.lane == box.lane {
				x = b.right + boxGap
			} else {
				x = max(staggered, b.right+boxGap)
			}
		}
		item := &placedBox{box: box, name: name, data: data, x: x, w: w, h: h}
		b.right = x + w
		b.h = max(b.h, h)
		placed = append(placed, item)
		byBox[box] = item
		previous = item
	}

	contentRight := 0.0
	for _, item := range placed {
		contentRight = max(contentRight, item.x+item.w)
	}
	laneWidth := labelColumn + contentRight + lanePad

	title := text.Layout(model.title, false, measurer, size, 4000, text.Bold, rc.Icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 12
	}
	left := pad
	top := pad + titleHeight

	y := top
	var lanes strings.Builder
	for _, b := range bands.All() {
		b.y = y
		b.h += 2 * lanePad
		lanes.WriteString(`<g class="pele-cluster pele-em-lane" data-id="` + svg.EscText(b.lane.label) + `">` +
			`<rect x="` + svg.Num(left) + `" y="` + svg.Num(y) + `" width="` + svg.Num(laneWidth) + `" height="` + svg.Num(b.h) + `" fill="var(--_s)"` + svg.RoundedStyle + `/>` +
			svg.LabelSVG(b.label, left+lanePad+float64(b.label.Width/2), y+float64(b.h/2), ` class="pele-cluster-label" font-size="`+jsnum.Format(small)+`" font-weight="var(--_hw)" fill="var(--_m)"`, nil) +
			`</g>`)
		y += b.h + laneGap
	}
	bottom := top
	if bands.Len() > 0 {
		bottom = y - laneGap
	}

	var nodes strings.Builder
	for _, item := range placed {
		b, _ := bands.Get(item.box.lane)
		item.x += left + labelColumn
		item.y = b.y + float64((b.h-item.h)/2)
		frame := `x="` + svg.Num(item.x) + `" y="` + svg.Num(item.y) + `" width="` + svg.Num(item.w) + `" height="` + svg.Num(item.h) + `"`
		var shape string
		if s, ok := series[item.box.kind]; ok {
			shape = `<rect ` + frame + ` fill="var(--_bg)"` + svg.RoundedStyle + `/><rect ` + frame + ` fill="` + svg.SeriesColor(s) + `" fill-opacity="0.22" stroke="` + svg.SeriesColor(s) + `"` + svg.RoundedStyle + `/>`
		} else {
			shape = `<rect ` + frame + ` fill="var(--_bg)" stroke="var(--_b)"` + svg.RoundedStyle + `/>`
		}
		nameY := item.y + boxPadY + float64(item.name.Height/2)
		body := svg.LabelSVG(item.name, item.x+float64(item.w/2), nameY, ` class="pele-label" font-size="`+jsnum.Format(nameSize)+`" font-weight="var(--_hw)"`, nil) +
			lines(
				item.data,
				item.x+boxPadX,
				item.y+boxPadY+item.name.Height+6+float64(monoStep/2)+float64(mono*0.35),
				monoStep,
				` class="pele-em-data" font-family="`+svg.FontMono+`" font-size="`+jsnum.Format(mono)+`"`,
			)
		reset := ""
		if item.box.reset {
			reset = " pele-em-reset"
		}
		nodes.WriteString(`<g class="pele-node pele-em-` + item.box.kind + reset + `" data-id="` + svg.EscText(item.box.id) + `">` + shape + body + `</g>`)
	}

	var edges strings.Builder
	trim := svg.MarkerTrim("arrow_point")
	for _, relation := range model.relations {
		s, sok := byBox[relation.source]
		t, tok := byBox[relation.target]
		if !sok || !tok || s == t {
			continue
		}
		var d, head string
		if s.box.lane == t.box.lane {
			// Along the lane, from the side of one box to the side of the other.
			forward := t.x >= s.x
			sx, tx := s.x, t.x+t.w
			if forward {
				sx, tx = s.x+s.w, t.x
			}
			sy := s.y + float64(s.h/2)
			ty := t.y + float64(t.h/2)
			dir := -1.0
			if tx >= sx {
				dir = 1
			}
			d = "M" + svg.Num(sx) + "," + svg.Num(sy) + "L" + svg.Num(tx-float64(dir*trim)) + "," + svg.Num(ty)
			head = svg.Marker("arrow_point", tx, ty, dir, 0, "var(--_l)")
		} else {
			// Between lanes, straight down or up where the boxes overlap, else in an S.
			down := t.y > s.y
			from := max(s.x, t.x)
			to := min(s.x+s.w, t.x+t.w)
			inset := func(w float64) float64 { return min(24, w/4) }
			var sx, tx float64
			switch {
			case to-from >= 16:
				sx = (from + to) / 2
				tx = sx
			case t.x > s.x:
				sx = s.x + s.w - inset(s.w)
				tx = t.x + inset(t.w)
			default:
				sx = s.x + inset(s.w)
				tx = t.x + t.w - inset(t.w)
			}
			sy, ty, dir := s.y, t.y+t.h, -1.0
			if down {
				sy, ty, dir = s.y+s.h, t.y, 1
			}
			end := ty - float64(dir*trim)
			mid := (sy + end) / 2
			if sx == tx {
				d = "M" + svg.Num(sx) + "," + svg.Num(sy) + "V" + svg.Num(end)
			} else {
				d = "M" + svg.Num(sx) + "," + svg.Num(sy) + "C" + svg.Num(sx) + "," + svg.Num(mid) + " " + svg.Num(tx) + "," + svg.Num(mid) + " " + svg.Num(tx) + "," + svg.Num(end)
			}
			head = svg.Marker("arrow_point", tx, ty, 0, dir, "var(--_l)")
		}
		edges.WriteString(`<g class="pele-edge" data-id="` + svg.EscText(relation.source.id) + `-` + svg.EscText(relation.target.id) + `"><path d="` + d + `" fill="none" stroke="var(--_l)"/>` + head + `</g>`)
	}

	// Notes and given/when/then specifications, listed under the lanes with the frame they
	// belong to.
	names := map[string]string{}
	for _, box := range model.boxes {
		names[box.id] = box.name
	}
	var cards strings.Builder
	cardsRight := 0.0
	card := func(kind, heading, frame string, rows []string) {
		label := heading + " · " + frame
		if name, ok := names[frame]; ok {
			label += " " + name
		}
		head := text.Layout(label, false, measurer, small, 4000, text.Bold, rc.Icons)
		w := max(head.Width, widest(rows, small, 0)) + 2*boxPadX
		h := 2*boxPadY + head.Height + float64(float64(len(rows))*smallStep)
		cy := bottom + cardGap
		if cards.Len() == 0 && bands.Len() == 0 {
			cy = bottom
		}
		cards.WriteString(`<g class="pele-em-` + kind + `" data-id="` + svg.EscText(frame) + `">` +
			`<rect x="` + svg.Num(left) + `" y="` + svg.Num(cy) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) + `" fill="var(--_bg)" stroke="var(--_b)"` + svg.RoundedStyle + `/>` +
			svg.LabelSVG(head, left+boxPadX+float64(head.Width/2), cy+boxPadY+float64(head.Height/2), ` font-size="`+jsnum.Format(small)+`" font-weight="var(--_hw)"`, nil) +
			lines(rows, left+boxPadX, cy+boxPadY+head.Height+float64(smallStep/2)+float64(small*0.35), smallStep, ` font-size="`+jsnum.Format(small)+`"`) +
			`</g>`)
		bottom = cy + h
		cardsRight = max(cardsRight, w)
	}
	for _, note := range model.notes {
		card("note", "Note", note.frame, hardWrap(note.text, noteColumns))
	}
	for _, spec := range model.specifications {
		var rows []string
		for _, row := range specRows(spec) {
			rows = append(rows, hardWrap(row, noteColumns)...)
		}
		card("specification", "Given / When / Then", spec.frame, rows)
	}

	width := max(0, cardsRight, title.Width)
	if bands.Len() > 0 {
		width = max(laneWidth, cardsRight, title.Width)
	}
	totalWidth := math.Ceil(width + 2*pad)
	totalHeight := math.Ceil(bottom + pad)
	content := svg.LabelSVG(title, float64(totalWidth/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil) +
		lanes.String() + edges.String() + nodes.String() + cards.String()
	return document(model, rc, totalWidth, totalHeight, size, content)
}

func specRows(spec emSpecification) []string {
	rows := []string{"Given  " + strings.Join(spec.given, ", ")}
	if len(spec.when) > 0 {
		rows = append(rows, "When  "+strings.Join(spec.when, ", "))
	}
	return append(rows, "Then  "+strings.Join(spec.then, ", "))
}

const (
	railGap   = 8
	cardMin   = 120
	arrowRoom = 20
)

type verticalBox struct {
	y, h, w float64
	index   int
	lane    float64
}

func labelText(label text.Label) []string {
	out := make([]string, len(label.Lines))
	for i, line := range label.Lines {
		var b strings.Builder
		for _, span := range line {
			b.WriteString(span.Text)
		}
		out[i] = b.String()
	}
	return out
}

func verticalEventModel(model *eventModelingModel, rc *core.Context) core.Rendered {
	size := fontSize(rc)
	small := jsmath.Round(float64(size * 0.875))
	mono := jsmath.Round(float64(size * 0.75))
	pad := padding(rc)
	measurer := rc.Measurer()
	maxWidth := 320.0
	if rc.Options.MaxWidth != nil {
		maxWidth = *rc.Options.MaxWidth
	}
	room := max(160, maxWidth-2*pad)
	// A relation that skips a card runs down a rail beside the cards. Relations into one card
	// share a rail; others whose runs overlap take the next rail out, so that no two follow
	// the same line.
	order := map[*emBox]int{}
	for index, box := range model.boxes {
		order[box] = index
	}
	runs := &omap.Map[*emBox, *[2]int]{}
	// A relation between neighbouring cards passes the lower card's lane heading.
	passed := map[*emBox]bool{}
	for _, relation := range model.relations {
		s, sok := order[relation.source]
		t, tok := order[relation.target]
		if !sok || !tok || s == t {
			continue
		}
		if s-t < 2 && t-s < 2 {
			passed[model.boxes[max(s, t)]] = true
			continue
		}
		if run, ok := runs.Get(relation.target); ok {
			run[0] = min(run[0], s)
			run[1] = max(run[1], s)
		} else {
			runs.Set(relation.target, &[2]int{min(s, t), max(s, t)})
		}
	}
	type targetRun struct {
		target *emBox
		run    *[2]int
	}
	var sortedRuns []targetRun
	for target, run := range runs.All() {
		sortedRuns = append(sortedRuns, targetRun{target, run})
	}
	slices.SortStableFunc(sortedRuns, func(a, b targetRun) int { return a.run[0] - b.run[0] })
	railOf := map[*emBox]int{}
	var railEnds []int
	for _, tr := range sortedRuns {
		rail := slices.IndexFunc(railEnds, func(end int) bool { return end < tr.run[0] })
		if rail < 0 {
			rail = len(railEnds)
			railEnds = append(railEnds, 0)
		}
		railEnds[rail] = tr.run[1]
		railOf[tr.target] = rail
	}
	gutter := float64(24 + railGap*max(0, len(railEnds)-1))
	// Rails are never shared to save room: past what the width allows, the drawing is wider.
	cardWidth := max(cardMin, room-gutter)
	textRoom := cardWidth - 2*boxPadX
	title := text.Layout(model.title, false, measurer, size, room, text.Bold, rc.Icons)
	y := pad
	if title.Height != 0 {
		y = pad + (title.Height + 12)
	}
	bottom := pad + title.Height
	width := max(title.Width, cardWidth+gutter)
	var nodes, edges, cards strings.Builder
	placed := map[*emBox]*verticalBox{}
	literal := func(raw string, fontSize float64, style text.Style) text.Label {
		return common.LiteralLabel(text.DecodeEntities(raw), measurer, fontSize, textRoom, style)
	}
	for index, box := range model.boxes {
		laneRoom := textRoom
		if passed[box] {
			laneRoom = textRoom - arrowRoom
		}
		lane := common.BreakLabel(text.Layout(box.lane.label, false, measurer, small, laneRoom, text.Bold, rc.Icons), measurer, laneRoom)
		name := text.Layout(box.name, false, measurer, small, textRoom-36, text.Bold, rc.Icons)
		var data *text.Label
		if hasData(box) {
			l := literal(*box.data, mono, text.Mono)
			data = &l
		}
		dataWidth := 0.0
		if data != nil {
			dataWidth = data.Width
		}
		w := max(cardWidth, lane.Width+2*boxPadX, name.Width+2*boxPadX+36, dataWidth+2*boxPadX)
		headH := lane.Height + 12
		h := 2*boxPadY + name.Height
		if data != nil {
			h += data.Height + 6
		}
		cy := y + headH
		frame := `x="` + svg.Num(pad) + `" y="` + svg.Num(cy) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) + `"`
		reset := ""
		if box.reset {
			reset = " pele-em-reset"
		}
		nodes.WriteString(`<g class="pele-cluster pele-em-lane" data-id="` + svg.EscText(box.lane.label) + `">` +
			common.LeftLabelSVG(lane, pad+boxPadX, y, ` class="pele-cluster-label" font-size="`+jsnum.Format(small)+`" font-weight="var(--_hw)" fill="var(--_m)"`) +
			`<g class="pele-node pele-em-` + box.kind + reset + `" data-id="` + svg.EscText(box.id) + `"><rect ` + frame + ` fill="var(--_bg)" stroke="var(--_b)"` + svg.RoundedStyle + `/>`)
		if s, ok := series[box.kind]; ok {
			nodes.WriteString(`<rect ` + frame + ` fill="` + svg.SeriesColor(s) + `" fill-opacity="0.22" stroke="` + svg.SeriesColor(s) + `"` + svg.RoundedStyle + `/>`)
		}
		nodes.WriteString(svg.LabelSVG(name, pad+boxPadX+float64(name.Width/2), cy+boxPadY+float64(name.Height/2), ` class="pele-label" font-size="`+jsnum.Format(small)+`" font-weight="var(--_hw)"`, nil) +
			`<text class="pele-em-frame" x="` + svg.Num(pad+w-boxPadX) + `" y="` + svg.Num(cy+boxPadY+small) + `" text-anchor="end" font-size="` + jsnum.Format(mono) + `" fill="var(--_m)">` + svg.Esc(box.id) + `</text>`)
		if data != nil {
			nodes.WriteString(lines(labelText(*data), pad+boxPadX, cy+boxPadY+name.Height+6+float64(data.LineHeight/2)+float64(mono*0.35), data.LineHeight,
				` class="pele-em-data" font-family="`+svg.FontMono+`" font-size="`+jsnum.Format(mono)+`"`))
		}
		nodes.WriteString(`</g></g>`)
		placed[box] = &verticalBox{y: cy, h: h, w: w, index: index, lane: lane.Width}
		width = max(width, w+gutter)
		bottom = cy + h
		y = bottom + 28
	}
	trim := svg.MarkerTrim("arrow_point")
	for _, relation := range model.relations {
		s, sok := placed[relation.source]
		t, tok := placed[relation.target]
		if !sok || !tok || s == t {
			continue
		}
		down := t.y > s.y
		dir := -1.0
		sy, ty := s.y, t.y+t.h
		if down {
			dir = 1
			sy, ty = s.y+s.h, t.y
		}
		var d, head string
		if s.index-t.index == 1 || t.index-s.index == 1 {
			// The lower card's lane heading sits in the gap, and wraps to leave the arrow room
			// beside it.
			lower := s
			if down {
				lower = t
			}
			cx := pad + max(float64(min(s.w, t.w)/2), boxPadX+lower.lane+16)
			d = "M" + svg.Num(cx) + "," + svg.Num(sy) + "V" + svg.Num(ty-float64(dir*trim))
			head = svg.Marker("arrow_point", cx, ty, 0, dir, "var(--_l)")
		} else {
			rail := pad + width - gutter + 12 + float64(railGap*railOf[relation.target])
			// A card can send one relation and receive another: they leave above and arrive
			// below its middle.
			from := s.y + float64(s.h/2) - 5
			to := t.y + float64(t.h/2) + 5
			d = "M" + svg.Num(pad+s.w) + "," + svg.Num(from) + "H" + svg.Num(rail) + "V" + svg.Num(to) + "H" + svg.Num(pad+t.w+trim)
			head = svg.Marker("arrow_point", pad+t.w, to, -1, 0, "var(--_l)")
		}
		edges.WriteString(`<g class="pele-edge" data-id="` + svg.EscText(relation.source.id) + `-` + svg.EscText(relation.target.id) + `"><path d="` + d + `" fill="none" stroke="var(--_l)"/>` + head + `</g>`)
	}
	names := map[string]string{}
	for _, box := range model.boxes {
		names[box.id] = box.name
	}
	card := func(kind, heading, frame, raw string) {
		label := heading + " · " + frame
		if name, ok := names[frame]; ok {
			label += " " + name
		}
		head := text.Layout(label, false, measurer, small, textRoom, text.Bold, rc.Icons)
		body := literal(raw, small, 0)
		w := max(cardWidth, head.Width+2*boxPadX, body.Width+2*boxPadX)
		h := 2*boxPadY + head.Height + body.Height
		cards.WriteString(`<g class="pele-em-` + kind + `" data-id="` + svg.EscText(frame) + `"><rect x="` + svg.Num(pad) + `" y="` + svg.Num(y) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) + `" fill="var(--_bg)" stroke="var(--_b)"` + svg.RoundedStyle + `/>` +
			svg.LabelSVG(head, pad+boxPadX+float64(head.Width/2), y+boxPadY+float64(head.Height/2), ` font-size="`+jsnum.Format(small)+`" font-weight="var(--_hw)"`, nil) +
			lines(labelText(body), pad+boxPadX, y+boxPadY+head.Height+float64(body.LineHeight/2)+float64(small*0.35), body.LineHeight, ` font-size="`+jsnum.Format(small)+`"`) + `</g>`)
		bottom = y + h
		y = bottom + cardGap
		width = max(width, w)
	}
	for _, note := range model.notes {
		card("note", "Note", note.frame, note.text)
	}
	for _, spec := range model.specifications {
		card("specification", "Given / When / Then", spec.frame, strings.Join(specRows(spec), "\n"))
	}
	totalWidth := math.Ceil(width + 2*pad)
	height := math.Ceil(bottom + pad)
	content := svg.LabelSVG(title, float64(totalWidth/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil) +
		edges.String() + nodes.String() + cards.String()
	return document(model, rc, totalWidth, height, size, content)
}
