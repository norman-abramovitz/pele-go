package sequence

import (
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const (
	actorPadX = 16
	actorPadY = 10
	glyph     = 34
	glyphGap  = 4
	stackOff  = 5
	capW      = 6
	shapeAttr = ` fill="var(--_s)" stroke="var(--_b)"`
	lineAttr  = ` fill="none" stroke="var(--_b)"`
)

var actorKinds = map[string]bool{
	"participant": true, "actor": true, "boundary": true, "control": true,
	"entity": true, "database": true, "collections": true, "queue": true,
}

var figures = map[string]bool{"actor": true, "boundary": true, "control": true, "entity": true, "database": true}

type actorSizeT struct {
	w, h float64
}

func actorKind(typ string) string {
	if actorKinds[typ] {
		return typ
	}
	return "participant"
}

// isFigure reports whether a kind stands above its label. The other kinds hold the label
// inside.
func isFigure(kind string) bool {
	return figures[kind]
}

func actorSize(kind string, label text.Label, minW, minH, paddingX float64) actorSizeT {
	if isFigure(kind) {
		h := float64(glyph)
		if label.Height > 0 {
			h += glyphGap + label.Height
		}
		return actorSizeT{w: max(40, label.Width+8), h: h}
	}
	extra := 0.0
	if kind == "collections" {
		extra = stackOff
	} else if kind == "queue" {
		extra = 2 * capW
	}
	stack := 0.0
	if kind == "collections" {
		stack = stackOff
	}
	return actorSizeT{
		w: max(minW, label.Width+float64(2*paddingX)) + extra,
		h: max(minH, label.Height+2*actorPadY) + stack,
	}
}

func figure(kind string, cx, top float64) string {
	x := svg.Num(cx)
	y := func(dy float64) string { return svg.Num(top + dy) }
	switch kind {
	case "actor":
		return `<circle cx="` + x + `" cy="` + y(7) + `" r="6"` + shapeAttr + `/>` +
			`<path d="M` + x + `,` + y(13) + `V` + y(24) + `M` + svg.Num(cx-10) + `,` + y(17) + `H` + svg.Num(cx+10) +
			`M` + svg.Num(cx-8) + `,` + y(34) + `L` + x + `,` + y(24) + `L` + svg.Num(cx+8) + `,` + y(34) + `"` + lineAttr + `/>`
	case "boundary":
		return `<path d="M` + svg.Num(cx-19) + `,` + y(6) + `V` + y(28) + `M` + svg.Num(cx-19) + `,` + y(17) + `H` + svg.Num(cx-8) + `"` + lineAttr + `/>` +
			`<circle cx="` + svg.Num(cx+3) + `" cy="` + y(17) + `" r="11"` + shapeAttr + `/>`
	case "control":
		return `<circle cx="` + x + `" cy="` + y(21) + `" r="11"` + shapeAttr + `/>` +
			`<path d="M` + svg.Num(cx+5) + `,` + y(5) + `L` + svg.Num(cx-1) + `,` + y(10) + `L` + svg.Num(cx+5) + `,` + y(15) + `"` + lineAttr + `/>`
	case "entity":
		return `<circle cx="` + x + `" cy="` + y(15) + `" r="11"` + shapeAttr + `/>` +
			`<path d="M` + svg.Num(cx-11) + `,` + y(30) + `H` + svg.Num(cx+11) + `"` + lineAttr + `/>`
	}
	l := svg.Num(cx - 14)
	r := svg.Num(cx + 14)
	return `<path d="M` + l + `,` + y(7) + `V` + y(27) + `A14,5 0 0 0 ` + r + `,` + y(27) + `V` + y(7) + `A14,5 0 0 0 ` + l + `,` + y(7) + `Z"` + shapeAttr + `/>` +
		`<path d="M` + l + `,` + y(7) + `A14,5 0 0 0 ` + r + `,` + y(7) + `"` + lineAttr + `/>`
}

// drawActor draws a participant whose bounding box has its top center at (cx, top).
func drawActor(kind string, cx, top float64, size actorSizeT, label text.Label, icons *text.Icons) string {
	w, h := size.w, size.h
	const attrs = ` class="pele-label"`
	if isFigure(kind) {
		return figure(kind, cx, top) + svg.LabelSVG(label, cx, top+glyph+glyphGap+float64(label.Height/2), attrs, icons)
	}
	left := cx - float64(w/2)
	if kind == "collections" {
		bw := w - stackOff
		bh := h - stackOff
		back := `<rect x="` + svg.Num(left+stackOff) + `" y="` + svg.Num(top) + `" width="` + svg.Num(bw) + `" height="` + svg.Num(bh) + `"` + shapeAttr + svg.RoundedStyle + `/>`
		front := `<rect x="` + svg.Num(left) + `" y="` + svg.Num(top+stackOff) + `" width="` + svg.Num(bw) + `" height="` + svg.Num(bh) + `"` + shapeAttr + svg.RoundedStyle + `/>`
		return back + front + svg.LabelSVG(label, left+float64(bw/2), top+stackOff+float64(bh/2), attrs, icons)
	}
	if kind == "queue" {
		l := svg.Num(left + capW)
		r := svg.Num(left + w - capW)
		b := svg.Num(top + h)
		ry := svg.Num(h / 2)
		const c = "6"
		return `<path d="M` + l + `,` + svg.Num(top) + `H` + r + `A` + c + `,` + ry + ` 0 0 1 ` + r + `,` + b + `H` + l + `A` + c + `,` + ry + ` 0 0 1 ` + l + `,` + svg.Num(top) + `Z"` + shapeAttr + `/>` +
			`<path d="M` + r + `,` + svg.Num(top) + `A` + c + `,` + ry + ` 0 0 0 ` + r + `,` + b + `"` + lineAttr + `/>` +
			svg.LabelSVG(label, cx-capW/2, top+float64(h/2), attrs, icons)
	}
	return `<rect x="` + svg.Num(left) + `" y="` + svg.Num(top) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) + `"` + shapeAttr + svg.RoundedStyle + `/>` +
		svg.LabelSVG(label, cx, top+float64(h/2), attrs, icons)
}
