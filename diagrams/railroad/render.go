package railroad

import (
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

type boxKind int

const (
	kTerminal boxKind = iota
	kNonTerminal
	kSpecial
	kSequence
	kChoice
	kBypass
	kLoop
	kFolded
)

// box is a laid-out element. The track enters at the left edge and leaves at the right edge
// on one baseline, with up above it and down below it.
type box struct {
	kind  boxKind
	width float64
	up    float64
	down  float64
	kids  []*box
	// text is a leaf's text, or a loop's count.
	text string
	// id is the name a non-terminal refers to.
	id string
	// reach is how far a bypass rises above the baseline, or a loop drops below it.
	reach float64
}

type metrics struct {
	measurer text.Measurer
	size     float64
	padding  float64
	height   float64
	// gap is the space between elements in a sequence, and sep between stacked branches.
	gap float64
	sep float64
	arc float64
}

const (
	lead      = 12
	headGap   = 6
	ruleGap   = 18
	countSize = 12
	arrow     = 7
)

func section(config *jsobj.Object) *jsobj.Object {
	v, _ := config.Get("railroad")
	obj, _ := v.(*jsobj.Object)
	return obj
}

func option(config *jsobj.Object, key string, fallback, maxValue float64) float64 {
	v, _ := section(config).Get(key)
	if n, ok := v.(float64); ok && n >= 0 {
		return min(n, maxValue)
	}
	return fallback
}

var controlEscapes = strings.NewReplacer("\n", `\n`, "\r", `\r`, "\t", `\t`)

// shown is text as drawn. Text is literal here: a terminal such as "<br>" or "*" is itself,
// not markup. Mermaid's `#35;` codes still stand for characters, and escapes that would be
// invisible are written out.
func shown(s string) string {
	if strings.Contains(s, "¶ß") {
		s = text.DecodeEntities(strings.ReplaceAll(s, "&", "&amp;"))
	}
	return controlEscapes.Replace(s)
}

func reverse[T any](list []T, from int) {
	for i, j := from, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
}

func even(value float64) float64 {
	return math.Ceil(value/2) * 2
}

func leaf(kind boxKind, s, id string, style text.Style, m *metrics) *box {
	half := m.height / 2
	width := max(even(m.measurer.Width(s, m.size, style)+2*m.padding), m.height)
	return &box{kind: kind, width: width, up: half, down: half, text: s, id: id}
}

func count(lo, hi float64) string {
	if hi == inf && lo <= 1 {
		return ""
	}
	format := func(value float64) string {
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return "∞"
		}
		return jsnum.Format(value)
	}
	if lo == hi {
		return format(lo) + "×"
	}
	if hi == inf {
		return "≥" + format(lo) + "×"
	}
	if lo == 0 {
		return "≤" + format(hi) + "×"
	}
	return format(lo) + "–" + format(hi) + "×"
}

// below is where the next branch of a choice sits below the baseline, given the one above it.
func below(offset float64, above, branch *box, m *metrics) float64 {
	return max(offset+above.down+m.sep+branch.up, 2*m.arc)
}

func bypass(kid *box, m *metrics) *box {
	reach := max(kid.up+m.sep, 2*m.arc)
	return &box{kind: kBypass, width: kid.width + float64(4*m.arc), up: reach, down: kid.down, kids: []*box{kid}, reach: reach}
}

func isAllSpaces(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			return false
		}
	}
	return true
}

func measure(root *node, m *metrics, room float64) *box {
	budgets := map[*node]float64{}
	type budget struct {
		n     *node
		width float64
	}
	pending := []budget{{root, room}}
	for i := 0; room != inf && i < len(pending); i++ {
		n, width := pending[i].n, pending[i].width
		budgets[n] = width
		// A sequence that folds puts its rows inside rails, so its parts are sized for those rows.
		around := 0.0
		switch n.typ {
		case nChoice, nOptional:
			around = float64(4 * m.arc)
		case nRepetition:
			if n.min == 0 {
				around = float64(6 * m.arc)
			} else {
				around = float64(2 * m.arc)
			}
		case nSequence:
			around = float64(6 * m.arc)
		}
		for _, child := range n.kids {
			pending = append(pending, budget{child, max(40, width-around)})
		}
	}
	return fold(root, func(n *node) []*node { return n.kids }, func(n *node, kids []*box) *box {
		room, ok := budgets[n]
		if !ok {
			room = inf
		}
		switch n.typ {
		case nTerminal:
			s := shown(n.value)
			if isAllSpaces(s) {
				s = strings.Repeat("␣", len(s))
			}
			return leaf(kTerminal, s, "", text.Mono, m)
		case nNonTerminal:
			return leaf(kNonTerminal, shown(n.value), n.value, 0, m)
		case nSpecial:
			return leaf(kSpecial, "? "+shown(n.value)+" ?", "", text.Italic, m)
		case nSequence:
			width := float64(float64(max(0, len(kids)-1)) * m.gap)
			up, down := 0.0, 0.0
			for _, kid := range kids {
				width += kid.width
				up = max(up, kid.up)
				down = max(down, kid.down)
			}
			seq := &box{kind: kSequence, width: width, up: up, down: down, kids: kids}
			if width <= room || len(kids) < 2 {
				return seq
			}
			var rows []*box
			var row []*box
			rowW := 0.0
			flush := func() {
				up, down := 0.0, 0.0
				for _, kid := range row {
					up = max(up, kid.up)
					down = max(down, kid.down)
				}
				rows = append(rows, &box{kind: kSequence, width: rowW, up: up, down: down, kids: row})
				row = nil
				rowW = 0
			}
			limit := max(40, room-float64(6*m.arc))
			for _, kid := range kids {
				if len(row) > 0 && rowW+m.gap+kid.width > limit {
					flush()
				}
				if len(row) > 0 {
					rowW += m.gap + kid.width
				} else {
					rowW += kid.width
				}
				row = append(row, kid)
			}
			flush()
			if len(rows) < 2 {
				return seq
			}
			foldedWidth, offset := 0.0, 0.0
			for i, r := range rows {
				foldedWidth = max(foldedWidth, r.width)
				if i > 0 {
					offset += rows[i-1].down + max(m.sep, 2*m.arc) + r.up
				}
			}
			return &box{kind: kFolded, width: foldedWidth + float64(6*m.arc), up: rows[0].up, down: offset + rows[len(rows)-1].down, kids: rows}
		case nChoice:
			inner, offset := 0.0, 0.0
			for i, kid := range kids {
				inner = max(inner, kid.width)
				if i > 0 {
					offset = below(offset, kids[i-1], kid, m)
				}
			}
			b := &box{kind: kChoice, width: inner + float64(4*m.arc), kids: kids}
			if len(kids) > 0 {
				b.up = kids[0].up
				b.down = offset + kids[len(kids)-1].down
			}
			return b
		case nOptional:
			return bypass(kids[0], m)
		default:
			kid := kids[0]
			if n.min == 1 && n.max == 1 {
				return kid
			}
			s := count(n.min, n.max)
			reach := max(kid.down+m.sep, 2*m.arc)
			labelWidth := 0.0
			extra := float64(arrow)/2 + 1
			if s != "" {
				labelWidth = even(m.measurer.Width(s, countSize, 0))
				extra = countSize + 6
			}
			loop := &box{kind: kLoop, width: max(kid.width, labelWidth) + 2*m.arc, up: kid.up, down: reach + extra, kids: []*box{kid}, text: s, reach: reach}
			if n.min == 0 {
				return bypass(loop, m)
			}
			return loop
		}
	})
}

func renderRailroad(db *railroadDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	return common.Tighten(rc.Options, func(tight bool) core.Rendered {
		return draw(db, config, rc, tight)
	})
}

func draw(db *railroadDb, config *jsobj.Object, rc *core.Context, tight bool) core.Rendered {
	options := rc.Options
	base := 16.0
	if options.FontSize != nil {
		base = *options.FontSize
	}
	fontSize := option(config, "fontSize", 0, 200)
	size := fontSize
	if size == 0 {
		size = jsmath.Round(base * 0.875)
	}
	headSize := fontSize
	if headSize == 0 {
		headSize = base
	}
	padding := option(config, "padding", -1, 200)
	lineHeight := jsmath.Round(size * 1.5)
	m := &metrics{
		measurer: rc.Measurer(),
		size:     size,
		padding:  padding,
		sep:      option(config, "verticalSeparation", 10, 500),
	}
	if padding < 0 {
		m.padding = 12
		m.height = even(lineHeight + 9)
	} else {
		m.height = even(lineHeight + 2*padding)
	}
	m.gap = option(config, "horizontalSeparation", 16, 500)
	m.arc = option(config, "arcRadius", 10, 200)
	if tight {
		m.gap = min(m.gap, 12)
		m.arc = min(m.arc, 6)
	}
	radius := 0.0
	if v, _ := section(config).Get("showMarkers"); v != false {
		radius = option(config, "markerRadius", 4, 50)
	}
	strokeWidth := option(config, "strokeWidth", 0, 20)
	stroke := ""
	if strokeWidth > 0 {
		stroke = ` stroke-width="` + svg.Num(strokeWidth) + `"`
	}
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	arc := m.arc
	num := svg.Num

	a := num(arc)
	turn := func(sweep, dx, dy string) string {
		return "a" + a + "," + a + " 0 0 " + sweep + " " + dx + a + "," + dy + a
	}
	eastToSouth := turn("1", "", "")
	southToEast := turn("0", "", "")
	eastToNorth := turn("0", "", "-")
	northToEast := turn("1", "", "-")
	loopDown := turn("0", "-", "")
	loopUp := turn("0", "-", "-")

	titleRoom := 4000.0
	if tight {
		titleRoom = common.TitleRoom(options)
	}
	title := text.Layout(db.title, false, m.measurer, base, titleRoom, text.Bold, rc.Icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 16
	}
	headHeight := jsmath.Round(headSize * 1.5)
	textShift := float64(size * 0.35)
	boxTop := m.height / 2

	var track, arrows, nodes strings.Builder

	line := func(x1, y, x2 float64) {
		if x2 > x1 {
			track.WriteString("M" + num(x1) + "," + num(y) + "H" + num(x2))
		}
	}

	place := func(root *box, left, baseline float64) {
		boxes := []*box{root}
		at := []float64{left, baseline}
		for len(boxes) > 0 {
			b := boxes[len(boxes)-1]
			boxes = boxes[:len(boxes)-1]
			y := at[len(at)-1]
			x := at[len(at)-2]
			at = at[:len(at)-2]
			kids := b.kids
			right := x + b.width
			switch b.kind {
			case kSequence:
				kx := right
				for i := len(kids) - 1; i >= 0; i-- {
					kx -= kids[i].width
					boxes = append(boxes, kids[i])
					at = append(at, kx, y)
					if i > 0 {
						line(kx-m.gap, y, kx)
					}
					kx -= m.gap
				}
			case kFolded:
				start := x + 2*arc
				from := len(boxes)
				offset := 0.0
				line(x, y, start)
				for i, kid := range kids {
					ky := y + offset
					boxes = append(boxes, kid)
					at = append(at, ky, start)
					if i+1 < len(kids) {
						next := kids[i+1]
						nextOffset := offset + kid.down + max(m.sep, 2*arc) + next.up
						mid := y + offset + kid.down + float64(max(m.sep, 2*arc)/2)
						track.WriteString("M" + num(start+kid.width) + "," + num(ky) + "H" + num(right-2*arc) + "V" + num(mid) + "H" + num(x+arc) +
							"V" + num(y+nextOffset) + "H" + num(start))
						arrows.WriteString("M" + num(start-arrow) + "," + num(y+nextOffset-float64(arrow)/2) + "l7,3.5l-7,3.5z")
						offset = nextOffset
					} else {
						track.WriteString("M" + num(start+kid.width) + "," + num(ky) + "H" + num(right-arc) + "V" + num(y) + "H" + num(right))
					}
				}
				reverse(boxes, from)
				reverse(at, 2*from)
			case kChoice:
				inner := b.width - float64(4*arc)
				railIn := x + arc
				railOut := right - arc
				from := len(boxes)
				offset := 0.0
				for i, kid := range kids {
					if i > 0 {
						offset = below(offset, kids[i-1], kid, m)
					}
					ky := y + offset
					kx := x + float64(2*arc) + float64((inner-kid.width)/2)
					last := i == len(kids)-1
					if i == 0 {
						line(x, y, kx)
						line(kx+kid.width, y, right)
					} else {
						if last {
							track.WriteString("M" + num(x) + "," + num(y) + eastToSouth + "V" + num(ky-arc))
						} else {
							track.WriteString("M" + num(railIn) + "," + num(ky-arc))
						}
						track.WriteString(southToEast)
						if kx > railIn+arc {
							track.WriteString("H" + num(kx))
						}
						track.WriteString("M" + num(kx+kid.width) + "," + num(ky))
						if kx > railIn+arc {
							track.WriteString("H" + num(railOut-arc))
						}
						track.WriteString(eastToNorth)
						if last {
							track.WriteString("V" + num(y+arc) + northToEast)
						}
					}
					boxes = append(boxes, kid)
					at = append(at, ky, kx)
				}
				// Reversed, so that the branches come off the stack top to bottom.
				reverse(boxes, from)
				reverse(at, 2*from)
			case kBypass:
				kid := kids[0]
				kx := x + 2*arc
				line(x, y, kx)
				line(kx+kid.width, y, right)
				track.WriteString("M" + num(x) + "," + num(y) + eastToNorth + "V" + num(y-b.reach+arc) + northToEast + "H" + num(right-2*arc) +
					eastToSouth + "V" + num(y-arc) + southToEast)
				boxes = append(boxes, kid)
				at = append(at, kx, y)
			case kLoop:
				kid := kids[0]
				kx := x + float64((b.width-kid.width)/2)
				loopY := y + b.reach
				middle := x + float64(b.width/2)
				line(x, y, kx)
				line(kx+kid.width, y, right)
				track.WriteString("M" + num(x+arc) + "," + num(y) + loopDown + "V" + num(loopY-arc) + southToEast + "H" + num(right-arc) +
					eastToNorth + "V" + num(y+arc) + loopUp)
				arrows.WriteString("M" + num(middle-float64(arrow)/2) + "," + num(loopY) + "l7,-3.5v7z")
				if b.text != "" {
					nodes.WriteString(`<text class="pele-edge-label pele-count" x="` + num(middle) + `" y="` + num(loopY+countSize+3) +
						`" font-size="12" fill="var(--_m)">` + svg.Esc(b.text) + `</text>`)
				}
				boxes = append(boxes, kid)
				at = append(at, kx, y)
			default:
				s := b.text
				space := ""
				if strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") || strings.Contains(s, "  ") {
					space = ` xml:space="preserve"`
				}
				shape := `<rect x="` + num(x) + `" y="` + num(y-boxTop) + `" width="` + num(b.width) + `" height="` + num(m.height) + `"`
				label := `<text x="` + num(x+float64(b.width/2)) + `" y="` + num(y+textShift) + `"` + space
				switch b.kind {
				case kTerminal:
					nodes.WriteString(`<g class="pele-node pele-terminal">` + shape + ` rx="` + num(boxTop) + `" fill="var(--_s)" stroke="var(--_b)"` + stroke + `/>` +
						label + ` font-family="` + svg.FontMono + `">` + svg.Esc(s) + `</text></g>`)
				case kNonTerminal:
					nodes.WriteString(`<g class="pele-node pele-nonterminal" data-id="` + svg.EscText(b.id) + `">` + shape +
						` fill="var(--_bg)" stroke="var(--_b)"` + stroke + svg.RoundedStyle + `/>` + label + `>` + svg.Esc(s) + `</text></g>`)
				default:
					nodes.WriteString(`<g class="pele-node pele-special">` + shape + ` fill="var(--_bg)" stroke="var(--_b)" stroke-dasharray="4 3"` + stroke + svg.RoundedStyle + `/>` +
						label + ` font-style="italic"><tspan fill="var(--_m)">? </tspan>` + svg.Esc(s[2:len(s)-2]) + `<tspan fill="var(--_m)"> ?</tspan></text></g>`)
				}
			}
		}
	}

	var rules strings.Builder
	width := title.Width
	top := pad + titleHeight
	bottom := pad + title.Height
	for _, r := range db.rules {
		room := inf
		if tight {
			maxWidth := 320.0
			if options.MaxWidth != nil {
				maxWidth = *options.MaxWidth
			}
			room = max(80, maxWidth-float64(2*pad)-float64(4*radius)-2*lead-1)
		}
		b := measure(r.definition, m, room)
		name := shown(r.name)
		baseline := top + headHeight + headGap + b.up
		start := pad + 2*radius
		left := start + lead
		end := left + b.width + lead
		track.Reset()
		arrows.Reset()
		nodes.Reset()
		line(start, baseline, left)
		line(left+b.width, baseline, end)
		place(b, left, baseline)
		rules.WriteString(`<g class="pele-rule" data-id="` + svg.EscText(r.name) + `">` +
			`<text class="pele-rule-name" x="` + num(pad) + `" y="` + num(top+float64(headHeight/2)+float64(headSize*0.35)) + `" font-size="` + num(headSize) +
			`" font-weight="var(--_hw)" text-anchor="start">` + svg.Esc(name) + `</text>` +
			`<path class="pele-edge pele-track" d="` + track.String() + `" fill="none" stroke="var(--_l)"` + stroke + `/>`)
		if arrows.Len() > 0 {
			rules.WriteString(`<path class="pele-marker" d="` + arrows.String() + `" fill="var(--_l)"/>`)
		}
		if radius > 0 {
			rules.WriteString(`<circle class="pele-marker pele-start" cx="` + num(pad+radius) + `" cy="` + num(baseline) + `" r="` + num(radius) + `" fill="var(--_l)"/>` +
				`<circle class="pele-marker pele-end" cx="` + num(end+radius) + `" cy="` + num(baseline) + `" r="` + num(radius) + `" fill="var(--_l)"/>`)
		}
		rules.WriteString(nodes.String() + "</g>")
		width = max(width, end+2*radius-pad, m.measurer.Width(name, headSize, text.Bold))
		bottom = baseline + b.down
		top = bottom + m.sep + ruleGap
	}

	totalWidth := math.Ceil(width + 2*pad)
	totalHeight := math.Ceil(bottom + pad)
	content := svg.LabelSVG(title, totalWidth/2, pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil)
	if rules.Len() > 0 {
		content += `<g class="pele-rules" transform="translate(0.5,0.5)" font-size="` + num(size) + `" text-anchor="middle">` + rules.String() + `</g>`
	}

	doc := svg.Document("railroad", totalWidth, totalHeight, base, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccTitle: db.accTitle, AccDescr: db.accDescr}, content)
	return core.Rendered{SVG: doc, Width: totalWidth, Height: totalHeight, Links: []core.Link{}}
}
