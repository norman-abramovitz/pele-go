package packet

import (
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

func renderPacket(m model, config *jsobj.Object, rc *core.Context) core.Rendered {
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered { return draw(m, config, rc, down) })
}

// floorPow2 is 2 ** Math.floor(Math.log2(count)) for a whole count of at least 1, without
// Math.log2: it is exact up to 8192, and past that it only has to exceed any row, which is
// at most 4096 bits.
func floorPow2(count float64) float64 {
	if count != count {
		return count
	}
	p := 1.0
	for p < 8192 && p*2 <= count {
		p *= 2
	}
	return p
}

type fitKey struct {
	label string
	max   float64
}

type fitted struct {
	common.Fitted
	ok bool
}

func draw(m model, config *jsobj.Object, rc *core.Context, down bool) core.Rendered {
	options := rc.Options
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	labelSize := jsmath.Round(size * 0.875)
	bitSize := jsmath.Round(size * 0.625)
	measurer := rc.Measurer()
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}

	section, _ := config.Get("packet")
	packet, _ := section.(*jsobj.Object)
	rowHeight := packetOption(config, "rowHeight", 32, 1, 400)
	bitWidth := packetOption(config, "bitWidth", 32, 1, 400)
	paddingX := packetOption(config, "paddingX", 5, 0, 400)
	paddingY := packetOption(config, "paddingY", 5, 0, 400)
	showBits, _ := packet.Get("showBits")
	showBitsOn := showBits != false
	bitOrder, _ := packet.Get("bitOrder")
	descending := bitOrder == "descending"
	bitsPerRow := m.bitsPerRow
	rows := m.rows
	if down {
		maxWidth := 320.0
		if options.MaxWidth != nil {
			maxWidth = *options.MaxWidth
		}
		count := max(1, math.Floor((maxWidth-float64(2*pad))/min(bitWidth, 18)))
		// A byte is the shortest row. Below that every label is one character to a line, and a long
		// packet is a row to a bit.
		bitsPerRow = min(bitsPerRow, max(8, floorPow2(count)))
		last := 0.0
		if len(m.rows) > 0 {
			if lastRow := m.rows[len(m.rows)-1]; len(lastRow) > 0 {
				last = lastRow[len(lastRow)-1].end
			}
		}
		if math.Ceil((last+1)/bitsPerRow) > 10000 {
			bitsPerRow = m.bitsPerRow
		}
		bitWidth = min(bitWidth, max(18, (maxWidth-float64(2*pad))/bitsPerRow))
		// The model has split each field at its own rows. Its pieces are joined again first, so that
		// a field is only split where one of the new rows ends. A piece counts one bit short.
		piece := func(b block) bool { return b.bits == b.end-b.start }
		var whole []block
		for _, row := range m.rows {
			for _, b := range row {
				if n := len(whole); n > 0 {
					prev := &whole[n-1]
					if piece(*prev) && piece(b) && prev.label == b.label && b.start == prev.end+1 && math.Mod(b.start, m.bitsPerRow) == 0 {
						prev.end = b.end
						prev.bits = prev.end - prev.start
						continue
					}
				}
				whole = append(whole, b)
			}
		}
		rows = nil
		for _, b := range whole {
			for start := b.start; start <= b.end; {
				end := min(b.end, float64((math.Floor(start/bitsPerRow)+1)*bitsPerRow)-1)
				index := int(math.Floor(start / bitsPerRow))
				for len(rows) <= index {
					rows = append(rows, nil)
				}
				piece := b
				piece.start, piece.end = start, end
				rows[index] = append(rows[index], piece)
				start = end + 1
			}
		}
	}
	var labels [][]text.Label
	if down {
		labels = make([][]text.Label, len(rows))
		for r, row := range rows {
			labels[r] = make([]text.Label, len(row))
			for k, b := range row {
				room := max(1, float64((b.end-b.start+1)*bitWidth)-min(paddingX, bitWidth-1)-6)
				labels[r][k] = common.LiteralLabel(jsstr.Trim(text.DecodeEntities(b.label)), measurer, labelSize, room, 0)
			}
		}
	}
	// Bit numbers sit above each row and need room of their own.
	numbers := 0.0
	if showBitsOn {
		numbers = bitSize + 4
	}
	rowHeights := make([]float64, len(rows))
	for r, row := range rows {
		height := rowHeight
		if down {
			for k := range row {
				height = max(height, labels[r][k].Height+10)
			}
		}
		rowHeights[r] = height
	}
	tops := make([]float64, len(rows))
	rowsHeight := 0.0
	for r, height := range rowHeights {
		tops[r] = rowsHeight
		rowsHeight += numbers + height + paddingY
	}
	if len(rows) > 0 {
		rowsHeight -= paddingY
	}

	gap := min(paddingX, bitWidth-1)
	width := 0.0
	if len(rows) > 0 {
		width = float64(bitsPerRow*bitWidth) - gap
	}
	titleWidth := 4000.0
	if down {
		titleWidth = common.TitleRoom(options)
	}
	title := text.Layout(m.title, false, measurer, size, titleWidth, text.Bold, rc.Icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height
		if rowsHeight > 0 {
			titleHeight += 12
		}
	}

	// The pieces of a field split across rows share its label, so each width is fitted once.
	fits := map[fitKey]fitted{}
	fit := func(label string, most float64) fitted {
		key := fitKey{label, most}
		if f, ok := fits[key]; ok {
			return f
		}
		f, ok := common.FitText(jsstr.Trim(text.DecodeEntities(label)), most, labelSize, jsmath.Round(size*0.5), measurer, 0)
		fits[key] = fitted{f, ok}
		return fits[key]
	}

	left := pad + max(0, (title.Width-width)/2)
	var fields, bits strings.Builder
	for index, row := range rows {
		y := pad + tops[index] + numbers
		rowHeight := rowHeights[index]
		for k, b := range row {
			count := b.end - b.start + 1
			first := math.Mod(b.start, bitsPerRow)
			slot := first
			if descending {
				slot = bitsPerRow - first - count
			}
			x := left + float64(slot*bitWidth)
			w := float64(count*bitWidth) - gap
			fields.WriteString(`<g class="pele-node" data-id="` + jsnum.Format(b.start) + "-" + jsnum.Format(b.end) + `">` +
				`<rect x="` + svg.Num(x) + `" y="` + svg.Num(y) + `" width="` + svg.Num(w) + `" height="` + svg.Num(rowHeight) + `" fill="var(--_s)" stroke="var(--_b)"` + svg.RoundedStyle + `/>`)
			if down {
				fields.WriteString(svg.LabelSVG(labels[index][k], x+float64(w/2), y+float64(rowHeight/2), "", nil))
			} else if t := fit(b.label, w-6); t.ok && t.Size <= rowHeight {
				fontSize := ""
				if t.Size != labelSize {
					fontSize = ` font-size="` + svg.Num(t.Size) + `"`
				}
				fields.WriteString(`<text x="` + svg.Num(x+float64(w/2)) + `" y="` + svg.Num(y+float64(rowHeight/2)+float64(t.Size*0.35)) + `"` + fontSize + `>` + svg.Esc(t.Text) + `</text>`)
			}
			fields.WriteString("</g>")
			if !showBitsOn {
				continue
			}
			leading, trailing := b.start, b.end
			if descending {
				leading, trailing = b.end, b.start
			}
			if count == 1 {
				bits.WriteString(`<text x="` + svg.Num(x+float64(w/2)) + `" y="` + svg.Num(y-4) + `" text-anchor="middle">` + jsnum.Format(leading) + `</text>`)
			} else {
				bits.WriteString(`<text x="` + svg.Num(x) + `" y="` + svg.Num(y-4) + `">` + jsnum.Format(leading) + `</text>` +
					`<text x="` + svg.Num(x+w) + `" y="` + svg.Num(y-4) + `" text-anchor="end">` + jsnum.Format(trailing) + `</text>`)
			}
		}
	}

	totalWidth := math.Ceil(max(width, title.Width) + float64(2*pad))
	totalHeight := math.Ceil(rowsHeight + titleHeight + float64(2*pad))
	content := ""
	if fields.Len() > 0 {
		content += `<g class="pele-packet-fields" font-size="` + jsnum.Format(labelSize) + `" text-anchor="middle">` + fields.String() + `</g>`
	}
	if bits.Len() > 0 {
		content += `<g class="pele-packet-bits" font-size="` + jsnum.Format(bitSize) + `" fill="var(--_m)">` + bits.String() + `</g>`
	}
	// Mermaid puts a packet's title under it.
	content += svg.LabelSVG(title, totalWidth/2, totalHeight-pad-float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil)

	doc := svg.Document("packet", totalWidth, totalHeight, size, svg.DocumentOptions{
		IDPrefix:       options.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     options.FontFamily,
		FontFamilyMono: options.FontFamilyMono,
	}, svg.Accessible{AccTitle: m.accTitle, AccDescr: m.accDescr}, content)
	return core.Rendered{SVG: doc, Width: totalWidth, Height: totalHeight, Links: []core.Link{}}
}
