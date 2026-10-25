package journey

import (
	"math"
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const (
	padX      = 10
	padY      = 8
	minText   = 124
	maxText   = 200
	gap       = 12
	bandPad   = 6
	gutter    = 24
	level     = 26
	faceR     = 11
	dot       = 5
	dotStep   = 13
	legendGap = 20
)

const eyes = `<circle cx="-3.6" cy="-2.8" r="1.3"/><circle cx="3.6" cy="-2.8" r="1.3"/>`

var mouths = map[string]string{"happy": "M-5,2A6,6 0 0 0 5,2", "neutral": "M-4.5,4.5H4.5", "sad": "M-5,6.5A6,6 0 0 1 5,6.5"}

func face(x, y float64, mood string) string {
	return `<g class="pele-face pele-face-` + mood + `" transform="translate(` + svg.Num(x) + `,` + svg.Num(y) + `)">` +
		`<circle r="` + strconv.Itoa(faceR) + `" fill="var(--_bg)" stroke="var(--_l)"/>` +
		`<g fill="var(--_l)">` + eyes + `</g>` +
		`<path d="` + mouths[mood] + `" fill="none" stroke="var(--_l)" stroke-linecap="round"/>` +
		`</g>`
}

func moodOf(score float64) string {
	if score > 3 {
		return "happy"
	}
	if score < 3 {
		return "sad"
	}
	return "neutral"
}

type settings struct {
	size, small, tiny, pad float64
	measurer               text.Measurer
}

func settingsOf(rc *core.Context) settings {
	opts := rc.Options
	s := settings{size: 16, pad: 8, measurer: rc.Measurer()}
	if opts.FontSize != nil {
		s.size = *opts.FontSize
	}
	if opts.Padding != nil {
		s.pad = *opts.Padding
	}
	s.small = jsmath.Round(s.size * 0.875)
	s.tiny = jsmath.Round(s.size * 0.75)
	return s
}

func document(width, height, size float64, rc *core.Context, model *journeyDb, content string) string {
	opts := rc.Options
	return svg.Document("journey", width, height, size, svg.DocumentOptions{
		IDPrefix:       opts.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     opts.FontFamily,
		FontFamilyMono: opts.FontFamilyMono,
	}, svg.Accessible{AccTitle: model.accTitle, AccDescr: model.accDescr}, content)
}

func renderJourney(model *journeyDb, rc *core.Context) core.Rendered {
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered { return draw(model, rc, down) })
}

type legendItem struct {
	name    string
	label   text.Label
	x, y, h float64
}

type run struct {
	start, count int
	label        *text.Label
}

func nonEmpty(names []string) []string {
	out := []string{}
	for _, name := range names {
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func draw(model *journeyDb, rc *core.Context, down bool) core.Rendered {
	s := settingsOf(rc)
	size, small, tiny, measurer, pad := s.size, s.small, s.tiny, s.measurer, s.pad
	if down {
		return verticalJourney(model, rc)
	}
	tasks := model.tasks
	n := len(tasks)

	// `Task: 5:` names an actor with no name; Mermaid gives it a dot and an empty legend row.
	names := nonEmpty(journeyActors(tasks))
	actor := map[string]int{}
	for i, name := range names {
		actor[name] = i
	}

	// Cards widen, up to a point, for a word that would not fit; past that the word is cut.
	textW := float64(minText)
	labels := make([]text.Label, n)
	for i, task := range tasks {
		labels[i] = common.PlainLabel(task.task, measurer, small, textW, 0)
	}
	widest := 0.0
	for _, label := range labels {
		widest = max(widest, label.Width)
	}
	if widest > textW {
		textW = min(math.Ceil(widest), maxText)
		for i, task := range tasks {
			labels[i] = common.FitLabel(task.task, measurer, small, textW, 0)
		}
	}

	cardW := textW + 2*padX
	pitch := cardW + gap
	plotW := 0.0
	if n > 0 {
		plotW = float64(float64(n)*pitch) - gap
	}

	lineH := jsmath.Round(small * 1.5)
	limit := max(plotW, 240)
	var legend []legendItem
	legendW, legendH, lx := 0.0, 0.0, 0.0
	for _, name := range names {
		label := common.FitLabel(name, measurer, small, limit-2*dot-6, 0)
		w := 2*dot + 6 + label.Width
		if lx > 0 && lx+w > limit {
			lx = 0
			legendH += 4
		}
		y := legendH
		if lx > 0 {
			y = legend[len(legend)-1].y
		}
		h := max(label.Height, lineH)
		legend = append(legend, legendItem{name: name, label: label, x: lx, y: y, h: h})
		lx += w
		legendW = max(legendW, lx)
		legendH = max(legendH, y+h)
		lx += legendGap
	}

	bodyW := 0.0
	legendSpan := 0.0
	if n > 0 {
		bodyW = gutter + plotW
		legendSpan = gutter + legendW
	}
	title := common.FitLabel(model.title, measurer, size, max(bodyW, 320), text.Bold)
	titleH := 0.0
	if title.Height > 0 {
		titleH = title.Height + 16
	}
	width := max(bodyW, title.Width, legendSpan)
	left := pad + float64((width-bodyW)/2) + gutter
	top := pad + titleH

	// A band covers a run of tasks that share a section, as in Mermaid; a section with no tasks draws nothing.
	var runs []run
	bandH := 0.0
	for i := 0; i < n; {
		end := i + 1
		for end < n && tasks[end].section == tasks[i].section {
			end++
		}
		var label *text.Label
		if tasks[i].section != "" {
			l := common.FitLabel(tasks[i].section, measurer, small, float64(float64(end-i)*pitch)-gap-2*padX, text.Bold)
			label = &l
			bandH = max(bandH, max(l.Height, lineH)+2*bandPad)
		}
		runs = append(runs, run{start: i, count: end - i, label: label})
		i = end
	}

	textH := lineH
	for _, label := range labels {
		textH = max(textH, label.Height)
	}
	dotsH := 0.0
	if len(names) > 0 {
		dotsH = 2*dot + 6
	}
	cardH := textH + dotsH + 2*padY
	cardsY := top
	if bandH > 0 {
		cardsY = top + (bandH + bandPad)
	}
	chartTop := cardsY + cardH + faceR + 16
	chartBottom := chartTop + 4*level
	right := left + plotW

	var body strings.Builder
	bottom := top
	if n > 0 {
		grid := ""
		scale := ""
		for score := 5; score >= 1; score-- {
			y := chartTop + float64((5-score)*level)
			grid += "M" + svg.Num(left-6) + "," + svg.Num(y) + "H" + svg.Num(right)
			scale += `<text x="` + svg.Num(left-12) + `" y="` + svg.Num(y+float64(tiny*0.35)) + `">` + strconv.Itoa(score) + `</text>`
		}
		body.WriteString(`<path class="pele-grid" d="` + grid + `" fill="none" stroke="var(--_a)"/>` +
			`<g class="pele-scale" font-size="` + jsnum.Format(tiny) + `" text-anchor="end" fill="var(--_m)">` + scale + `</g>`)

		var line strings.Builder
		var stems strings.Builder
		var faces strings.Builder
		cards := make([]string, 0, n)
		for i, task := range tasks {
			x := left + float64(float64(i)*pitch)
			cx := x + float64(cardW/2)
			score := task.score
			lvl := 3.0
			if !math.IsNaN(score) {
				lvl = min(5, max(1, score))
			}
			y := chartTop + float64((5-lvl)*level)
			cmd := "L"
			if i == 0 {
				cmd = "M"
			}
			line.WriteString(cmd + svg.Num(cx) + "," + svg.Num(y))
			stems.WriteString("M" + svg.Num(cx) + "," + svg.Num(cardsY+cardH) + "V" + svg.Num(y-faceR))
			faces.WriteString(face(cx, y, moodOf(score)))

			var dots strings.Builder
			people := nonEmpty(task.people)
			step := min(dotStep, (cardW-2*padX-2*dot)/float64(max(len(people)-1, 1)))
			for k, name := range people {
				offset := float64(k) - float64(float64(len(people)-1)/2)
				dots.WriteString(`<circle class="pele-actor" cx="` + svg.Num(cx+float64(offset*step)) + `" cy="` + svg.Num(cardsY+cardH-padY-dot) +
					`" r="` + strconv.Itoa(dot) + `" fill="` + svg.SeriesColor(actor[name]) + `" stroke="var(--_s)">` +
					`<title>` + svg.EscText(name) + `</title></circle>`)
			}
			cards = append(cards, `<g class="pele-node pele-task" data-id="`+svg.EscText(jsstr.Trim(task.task))+`">`+
				`<rect x="`+svg.Num(x)+`" y="`+svg.Num(cardsY)+`" width="`+svg.Num(cardW)+`" height="`+svg.Num(cardH)+`" fill="var(--_s)" stroke="var(--_b)"`+svg.RoundedStyle+`/>`+
				svg.LabelSVG(labels[i], cx, cardsY+padY+float64(textH/2), ` font-size="`+jsnum.Format(small)+`"`, nil)+
				dots.String()+
				`</g>`)
		}
		body.WriteString(`<path class="pele-stems" d="` + stems.String() + `" fill="none" stroke="var(--_b)" stroke-dasharray="2 3"/>`)
		if n > 1 {
			body.WriteString(`<path class="pele-edge" d="` + line.String() + `" fill="none" stroke="var(--_l)" stroke-linejoin="round"/>`)
		}
		body.WriteString(faces.String())

		for _, r := range runs {
			var members strings.Builder
			for _, card := range cards[r.start : r.start+r.count] {
				members.WriteString(card)
			}
			if r.label == nil {
				body.WriteString(members.String())
				continue
			}
			x := left + float64(float64(r.start)*pitch)
			w := float64(float64(r.count)*pitch) - gap
			body.WriteString(`<g class="pele-cluster pele-section" data-id="` + svg.EscText(jsstr.Trim(tasks[r.start].section)) + `">` +
				`<rect x="` + svg.Num(x) + `" y="` + svg.Num(top) + `" width="` + svg.Num(w) + `" height="` + svg.Num(bandH) + `" fill="var(--_a)"` + svg.RoundedStyle + `/>` +
				svg.LabelSVG(*r.label, x+float64(w/2), top+float64(bandH/2), ` font-size="`+jsnum.Format(small)+`" font-weight="var(--_hw)"`, nil) +
				members.String() +
				`</g>`)
		}
		bottom = chartBottom + faceR

		if len(legend) > 0 {
			x0 := left + float64((plotW-legendW)/2)
			y0 := bottom + 16
			var items strings.Builder
			for _, item := range legend {
				cy := y0 + item.y + float64(item.h/2)
				items.WriteString(`<g class="pele-legend-item" data-id="` + svg.EscText(item.name) + `">` +
					`<circle cx="` + svg.Num(x0+item.x+dot) + `" cy="` + svg.Num(cy) + `" r="` + strconv.Itoa(dot) + `" fill="` + svg.SeriesColor(actor[item.name]) + `"/>` +
					svg.LabelSVG(item.label, x0+item.x+2*dot+6+float64(item.label.Width/2), cy, "", nil) +
					`</g>`)
			}
			body.WriteString(`<g class="pele-legend" font-size="` + jsnum.Format(small) + `">` + items.String() + `</g>`)
			bottom = y0 + legendH
		}
	}

	totalWidth := math.Ceil(width + float64(2*pad))
	totalHeight := math.Ceil(max(bottom, pad) + pad)
	content := svg.LabelSVG(title, pad+float64(width/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil) + body.String()
	return core.Rendered{
		SVG:    document(totalWidth, totalHeight, size, rc, model, content),
		Width:  totalWidth,
		Height: totalHeight,
		Links:  []core.Link{},
	}
}

type person struct {
	name  string
	label text.Label
}

func verticalJourney(model *journeyDb, rc *core.Context) core.Rendered {
	s := settingsOf(rc)
	size, small, measurer, pad := s.size, s.small, s.measurer, s.pad
	maxWidth := 320.0
	if rc.Options.MaxWidth != nil {
		maxWidth = *rc.Options.MaxWidth
	}
	room := max(140, maxWidth-float64(2*pad))
	title := common.FitLabel(model.title, measurer, size, room, text.Bold)
	names := nonEmpty(journeyActors(model.tasks))
	actors := map[string]int{}
	for i, name := range names {
		actors[name] = i
	}
	y := pad
	if title.Height != 0 {
		y = pad + (title.Height + 16)
	}
	// Upstream starts from undefined, which no section equals.
	var section *string
	var body strings.Builder
	width := title.Width
	for index, task := range model.tasks {
		if section == nil || task.section != *section {
			section = &task.section
			head := common.FitLabel(*section, measurer, size, room-16, text.Bold)
			if head.Height != 0 {
				body.WriteString(svg.LabelSVG(head, pad+8+float64(head.Width/2), y+float64(head.Height/2), ` class="pele-cluster-label" font-weight="var(--_hw)"`, nil))
				width = max(width, head.Width+16)
				y += head.Height + 8
			}
		}
		label := common.FitLabel(task.task, measurer, small, room-72, 0)
		var people []person
		for _, name := range nonEmpty(task.people) {
			people = append(people, person{name: name, label: common.FitLabel(name, measurer, small, room-44, 0)})
		}
		textH := max(label.Height, 42)
		h := 2*padY + textH
		for _, p := range people {
			h += max(16, p.label.Height) + 4
		}
		w := max(room, label.Width+72)
		body.WriteString(`<g class="pele-node pele-journey-step" data-id="` + strconv.Itoa(index) + `"><rect x="` + svg.Num(pad) + `" y="` + svg.Num(y) +
			`" width="` + svg.Num(w) + `" height="` + svg.Num(h) + `" fill="var(--_s)" stroke="var(--_b)"` + svg.RoundedStyle + `/>`)
		body.WriteString(svg.LabelSVG(label, pad+padX+float64(label.Width/2), y+padY+float64(label.Height/2), ` class="pele-label"`, nil))
		cx := pad + w - 28
		body.WriteString(face(cx, y+padY+faceR, moodOf(task.score)))
		if !math.IsNaN(task.score) {
			body.WriteString(`<text class="pele-journey-score" x="` + svg.Num(cx) + `" y="` + svg.Num(y+padY+2*faceR+small) + `" font-size="` + jsnum.Format(small) +
				`" text-anchor="middle">` + svg.Num(min(5, max(1, task.score))) + `</text>`)
		}
		py := y + padY + textH + 4
		for _, p := range people {
			ph := max(16, p.label.Height)
			body.WriteString(`<circle class="pele-actor" cx="` + svg.Num(pad+16) + `" cy="` + svg.Num(py+float64(ph/2)) + `" r="` + strconv.Itoa(dot) +
				`" fill="` + svg.SeriesColor(actors[p.name]) + `"/>` +
				svg.LabelSVG(p.label, pad+28+float64(p.label.Width/2), py+float64(ph/2), ` class="pele-journey-actor" font-size="`+jsnum.Format(small)+`"`, nil))
			py += ph + 4
		}
		body.WriteString(`</g>`)
		y += h
		if index+1 < len(model.tasks) {
			body.WriteString(`<path class="pele-edge" d="M` + svg.Num(pad+float64(w/2)) + `,` + svg.Num(y) + `v16" fill="none" stroke="var(--_l)"/>`)
		}
		y += 16
		width = max(width, w)
	}
	totalWidth := math.Ceil(width + float64(2*pad))
	tail := 0.0
	if len(model.tasks) > 0 {
		tail = 16
	}
	height := math.Ceil(max(pad+title.Height, y-tail) + pad)
	content := svg.LabelSVG(title, float64(totalWidth/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil) + body.String()
	return core.Rendered{SVG: document(totalWidth, height, size, rc, model, content), Width: totalWidth, Height: height, Links: []core.Link{}}
}
