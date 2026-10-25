package kanban

import (
	"math"
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

const (
	columnGap = 12
	columnPad = 8
	cardPad   = 10
	cardGap   = 8
	metaGap   = 4
	iconGap   = 6
)

// priorityStroke is upstream's PRIORITY. Mermaid names five priorities and gives Medium no
// color. Matching here ignores case. The high ones are marked in the critical color and the
// low ones in the muted one, the nearer to Medium the fainter. They are not series colors, as
// a theme may put those in any order.
func priorityStroke(priority string) (string, bool) {
	switch priority {
	case "very high":
		return `stroke="` + svg.Critical + `"`, true
	case "high":
		return `stroke="` + svg.Critical + `" opacity="0.5"`, true
	case "low":
		return `stroke="var(--_m)" opacity="0.5"`, true
	case "very low":
		return `stroke="var(--_m)"`, true
	}
	return "", false
}

// lowerPriority is s.trim().toLowerCase() as far as priorityStroke can tell: only an ASCII
// string lowers to one of its keys, so any other is returned as it is.
func lowerPriority(s string) string {
	s = jsstr.Trim(s)
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return s
		}
	}
	return strings.ToLower(s)
}

// fitLabel lays out text for a fixed width. A word too long for a line is broken between
// characters, because a card cannot grow sideways to hold it.
func fitLabel(raw string, markdown bool, measurer text.Measurer, size, max float64, base text.Style, icons *text.Icons) text.Label {
	label := text.Layout(raw, markdown, measurer, size, max, base, icons)
	if label.Width <= max {
		return label
	}
	var lines [][]text.Span
	var widths []float64
	widest := 0.0
	var line []text.Span
	width := 0.0
	commit := func() {
		lines = append(lines, line)
		widths = append(widths, width)
		widest = math.Max(widest, width)
		line = nil
		width = 0
	}
	for i := range label.Lines {
		if label.Widths[i] <= max {
			line = label.Lines[i]
			width = label.Widths[i]
			commit()
			continue
		}
		for _, span := range label.Lines[i] {
			if span.Icon != "" || span.Math != nil {
				if width > 0 && width+span.Width > max {
					commit()
				}
				line = append(line, span)
				width += span.Width
				continue
			}
			var chars []uint16
			from := width
			units := jsstr.UTF16(span.Text)
			for k := 0; k < len(units); {
				n := 1
				if units[k] >= 0xd800 && units[k] <= 0xdbff && k+1 < len(units) && units[k+1] >= 0xdc00 && units[k+1] <= 0xdfff {
					n = 2
				}
				ch := units[k : k+n]
				k += n
				w := measurer.Width(jsstr.FromUTF16(ch), size, span.Style)
				if width > 0 && width+w > max {
					if len(chars) > 0 {
						line = append(line, text.Span{Text: jsstr.FromUTF16(chars), Style: span.Style, Width: width - from})
					}
					commit()
					chars = nil
					from = 0
				}
				chars = append(chars, ch...)
				width += w
			}
			if len(chars) > 0 {
				line = append(line, text.Span{Text: jsstr.FromUTF16(chars), Style: span.Style, Width: width - from})
			}
		}
		commit()
	}
	label.Lines = lines
	label.Widths = widths
	label.Width = widest
	label.Height = float64(len(lines)) * label.LineHeight
	if label.Heights != nil {
		return text.WithHeights(label)
	}
	return label
}

// textBlock writes a label with every line starting at x, or ending there, and the first
// line's box starting at y.
func textBlock(label text.Label, x, y float64, attrs string, icons *text.Icons, end bool) string {
	var out strings.Builder
	for i, line := range label.Lines {
		cy := y + text.LineMiddle(label, i)
		if len(line) == 1 && line[0].Icon == "" && line[0].Math == nil && line[0].Style&(text.Sub|text.Sup|text.Underline) == 0 {
			anchor := ""
			if end {
				anchor = ` text-anchor="end"`
			}
			out.WriteString(`<text` + svg.TextStyle(attrs, line[0].Style) + ` x="` + svg.Num(x) + `" y="` + svg.Num(cy+float64(label.Size*0.35)) + `"` + anchor + `>` +
				svg.Esc(line[0].Text) + `</text>`)
		} else {
			width := label.Widths[i]
			shift := width
			if end {
				shift = -width
			}
			out.WriteString(svg.LabelSVG(text.LabelLine(label, i), x+float64(shift/2), cy, attrs, icons))
		}
	}
	return out.String()
}

type card struct {
	item             *kanbanNode
	label            text.Label
	ticket, assigned text.Label
	// Height of the rows under the text that hold the ticket and the assignee.
	meta   float64
	height float64
}

func renderKanban(model *kanbanDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	opts := rc.Options
	size := 16.0
	if opts.FontSize != nil {
		size = *opts.FontSize
	}
	small := jsmath.Round(size * 0.875)
	line := jsmath.Round(size * 1.5)
	smallLine := jsmath.Round(small * 1.5)
	measurer := rc.Measurer()
	pad := 8.0
	if opts.Padding != nil {
		pad = *opts.Padding
	}
	icons := rc.Icons
	has := func(name string) bool { return name != "" && icons.Markup(name) != "" }
	section, _ := config.Get("kanban")
	conf, _ := section.(*jsobj.Object)
	glyph := float64(small * 1.25)
	// Math can't break across lines, so columns widen to hold the widest expression.
	mathWidth := 0.0
	holds := func(raw string, fontSize, reserved float64) {
		if raw == "" || !strings.Contains(raw, "$$") {
			return
		}
		for _, spans := range text.Layout(raw, true, measurer, fontSize, math.Inf(1), 0, icons).Lines {
			for _, span := range spans {
				if span.Math != nil {
					mathWidth = max(mathWidth, span.Math.Width+reserved)
				}
			}
		}
	}
	for _, section := range model.sections {
		count := measurer.Width(strconv.Itoa(len(section.items)), small, 0)
		extra := 0.0
		if has(section.icon) {
			extra = glyph + iconGap
		}
		holds(section.label, size, 2*columnPad+8+count+12+extra)
		for _, item := range section.items {
			reserved := float64(2 * (columnPad + cardPad))
			extra := 0.0
			if has(item.icon) {
				extra = glyph + iconGap
			}
			holds(item.label, small, reserved+extra)
			holds(item.ticket, small, reserved)
			holds(item.assigned, small, reserved)
		}
	}
	ownWidth := 200.0
	if v, _ := conf.Get("sectionWidth"); v != nil {
		if sectionWidth, ok := v.(float64); ok && sectionWidth > 0 {
			ownWidth = min(max(sectionWidth, 80), 2000)
		}
	}
	ownWidth = max(ownWidth, math.Ceil(mathWidth))
	// A board wider than the width the host has is folded into rows of columns, which then
	// share the row between them.
	room := math.Inf(1)
	if opts.MaxWidth != nil && *opts.MaxWidth > 0 {
		room = *opts.MaxWidth - float64(2*pad)
	}
	count := len(model.sections)
	fits := float64(float64(count)*(ownWidth+columnGap))-columnGap <= room
	var perRowF, columnWidth float64
	if fits {
		perRowF = float64(max(count, 1))
		columnWidth = ownWidth
	} else {
		perRowF = max(1, math.Floor((room+columnGap)/(ownWidth+columnGap)))
		columnWidth = max(min(ownWidth, room), min(math.Floor((room+columnGap)/perRowF)-columnGap, ownWidth*2), math.Ceil(mathWidth))
	}
	// perRow is a whole number of at least 1; past the number of columns it acts as that number.
	perRow := int(min(perRowF, float64(max(count, 1))))
	baseURL := ""
	if v, ok := conf.Get("ticketBaseUrl"); ok {
		if s, ok := v.(string); ok {
			baseURL = s
		}
	}
	cardWidth := columnWidth - 2*columnPad
	textWidth := cardWidth - 2*cardPad
	links := []core.Link{}

	sections := model.sections
	titles := make([]text.Label, 0, count)
	countWidths := make([]float64, 0, count)
	cards := make([][]card, 0, count)
	headHeight := line
	bodyHeights := make([]float64, 0, count)
	for _, section := range sections {
		countWidth := measurer.Width(strconv.Itoa(len(section.items)), small, 0)
		reserved := countWidth + 12
		if has(section.icon) {
			reserved = countWidth + 12 + (glyph + iconGap)
		}
		title := fitLabel(section.label, true, measurer, size, cardWidth-8-reserved, text.Bold, icons)
		headHeight = max(headHeight, title.Height)
		countWidths = append(countWidths, countWidth)
		titles = append(titles, title)

		list := []card{}
		height := 0.0
		for _, item := range section.items {
			room := textWidth
			if has(item.icon) {
				room = textWidth - (glyph + iconGap)
			}
			label := fitLabel(item.label, true, measurer, small, room, 0, icons)
			ticket := fitLabel(item.ticket, false, measurer, small, textWidth, 0, icons)
			assigned := fitLabel(item.assigned, false, measurer, small, textWidth, 0, icons)
			meta := max(ticket.Height, assigned.Height)
			if ticket.Width+assigned.Width+8 > textWidth {
				meta = ticket.Height + assigned.Height
			}
			cardHeight := 2*cardPad + max(label.Height, smallLine)
			if meta != 0 && !math.IsNaN(meta) {
				cardHeight = 2*cardPad + max(label.Height, smallLine) + (metaGap + meta)
			}
			list = append(list, card{item: item, label: label, ticket: ticket, assigned: assigned, meta: meta, height: cardHeight})
			height += cardHeight + cardGap
		}
		cards = append(cards, list)
		bodyHeights = append(bodyHeights, height)
	}
	headHeight += 2 * columnPad
	var rowHeights, rowTops []float64
	for first, y := 0, 0.0; first < count; first += perRow {
		tallest := float64(cardGap)
		for index := first; index < min(first+perRow, count); index++ {
			tallest = max(tallest, bodyHeights[index])
		}
		rowTops = append(rowTops, y)
		rowHeights = append(rowHeights, headHeight+tallest)
		y += headHeight + tallest + columnGap
	}
	boardHeight := 0.0
	if len(rowHeights) > 0 {
		boardHeight = rowTops[len(rowTops)-1] + rowHeights[len(rowHeights)-1]
	}

	title := svg.TitleLabel(model.title, measurer, size, common.TitleRoom(opts), icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 16
	}
	boardWidth := max(float64(float64(min(count, perRow))*(columnWidth+columnGap))-columnGap, 0)
	width := max(boardWidth, title.Width)
	left := pad + float64((width-boardWidth)/2)
	edge := columnWidth - columnPad - 4

	var out strings.Builder
	for index, section := range sections {
		row := index / perRow
		var body strings.Builder
		body.WriteString(`<rect width="` + jsnum.Format(columnWidth) + `" height="` + svg.Num(rowHeights[row]) + `" fill="var(--_s)"` + svg.RoundedStyle + `/>` +
			textBlock(titles[index], columnPad+4, columnPad, ` class="pele-cluster-label" font-weight="var(--_hw)"`, icons, false) +
			`<text class="pele-count" x="` + jsnum.Format(edge) + `" y="` + svg.Num(columnPad+float64(line/2)+float64(small*0.35)) +
			`" text-anchor="end" font-size="` + jsnum.Format(small) + `" fill="var(--_m)">` + strconv.Itoa(len(section.items)) + `</text>`)
		if has(section.icon) {
			body.WriteString(common.IconSVG(section.icon, edge-countWidths[index]-iconGap-glyph, columnPad+float64((line-glyph)/2), glyph, icons))
		}

		y := headHeight
		for _, c := range cards[index] {
			item := c.item
			priority := lowerPriority(item.priority)
			color, colored := priorityStroke(priority)
			inner := `<rect width="` + jsnum.Format(cardWidth) + `" height="` + svg.Num(c.height) + `" fill="var(--_bg)" stroke="var(--_b)"` + svg.RoundedStyle + `/>`
			if colored {
				inner += `<path class="pele-priority" d="M2.5,5V` + svg.Num(c.height-5) + `" fill="none" ` + color + ` stroke-width="3" stroke-linecap="round"/>`
			}
			inner += textBlock(c.label, cardPad, cardPad, ` class="pele-label"`, icons, false)
			if has(item.icon) {
				inner += common.IconSVG(item.icon, cardWidth-cardPad-glyph, cardPad+float64((smallLine-glyph)/2), glyph, icons)
			}
			bottom := c.height - cardPad
			if item.ticket != "" {
				top := bottom - c.meta
				href, linked := "", false
				if baseURL != "" {
					href, linked = util.LinkURL(util.SafeURL(strings.Replace(baseURL, "#TICKET#", item.ticket, 1)), opts)
				}
				if !linked {
					inner += textBlock(c.ticket, cardPad, top, ` class="pele-ticket" fill="var(--_m)"`, icons, false)
				} else {
					inner += `<a href="` + svg.Esc(href) + `" target="_blank"` + util.RelAttr(opts) + `>` +
						textBlock(c.ticket, cardPad, top, ` class="pele-ticket" fill="var(--_c)"`, icons, false) +
						`</a>`
					links = append(links, core.Link{ID: item.id, Href: href, Internal: false})
				}
			}
			if item.assigned != "" {
				inner += textBlock(c.assigned, cardWidth-cardPad, bottom-c.assigned.Height, ` class="pele-assigned" fill="var(--_m)"`, icons, true)
			}
			class := ""
			if colored {
				class = " pele-priority-" + strings.Replace(priority, " ", "-", 1)
			}
			body.WriteString(`<g class="pele-node pele-card` + class + svg.ClassNames(item.cssClasses) + `" data-id="` + svg.EscText(item.id) +
				`" transform="translate(` + strconv.Itoa(columnPad) + `,` + svg.Num(y) + `)" font-size="` + jsnum.Format(small) + `">` + inner + `</g>`)
			y += c.height + cardGap
		}

		out.WriteString(`<g class="pele-cluster pele-column` + svg.ClassNames(section.cssClasses) + `" data-id="` + svg.EscText(section.id) +
			`" transform="translate(` + svg.Num(left+float64(float64(index%perRow)*(columnWidth+columnGap))) + `,` + svg.Num(pad+titleHeight+rowTops[row]) + `)">` +
			body.String() + `</g>`)
	}

	totalWidth := math.Ceil(width + float64(2*pad))
	totalHeight := math.Ceil(boardHeight + titleHeight + float64(2*pad))
	content := svg.LabelSVG(title, float64(totalWidth/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil) + out.String()

	return core.Rendered{
		SVG: svg.Document("kanban", totalWidth, totalHeight, size, svg.DocumentOptions{
			IDPrefix:       opts.IDPrefix,
			Responsive:     rc.Responsive,
			FontFamily:     opts.FontFamily,
			FontFamilyMono: opts.FontFamilyMono,
		}, svg.Accessible{}, content),
		Width:  totalWidth,
		Height: totalHeight,
		Links:  links,
	}
}
