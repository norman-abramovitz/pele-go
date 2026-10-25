package svg

import (
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/layout"
)

// ShapeSize is a node shape's size for a label of a given size.
type ShapeSize struct {
	W, H float64
	// DY is the vertical shift of the label from the shape's center.
	DY float64
}

// The padding around a label in a box.
const (
	PadX = 18
	PadY = 12
)

type (
	sizer  func(tw, th float64) ShapeSize
	drawer func(w, h float64, a, line string) string
	// inset is the distance from the bounding box to the outline on a side: 0 top, 1 right,
	// 2 bottom, 3 left. t is how far along the side from its middle, to the right or down.
	insetFn func(w, h float64, side int, t float64) float64
	// span is how much of a side edges may spread along, centered on its middle: the top and
	// bottom, or the left and right when sideways. A shape without one keeps its edges at
	// its middle.
	spanFn func(w, h float64, sideways bool) float64
)

type shapeDef struct {
	size    sizer
	draw    drawer
	inset   insetFn
	span    spanFn
	noLabel bool
}

// shapeCorner is kept clear at each end of a straight side, for the corner.
const shapeCorner = 12

func flat(w, h float64, sideways bool) float64 {
	if sideways {
		return h - 2*shapeCorner
	}
	return w - 2*shapeCorner
}

// flatAcross is straight along the top and bottom except for cut(h) at each end, and not
// straight at the sides.
func flatAcross(cut func(h float64) float64) spanFn {
	return func(w, h float64, sideways bool) float64 {
		if sideways {
			return 0
		}
		return w - float64(2*cut(h)) - 2*shapeCorner
	}
}

// bulge is how far an ellipse's outline has fallen away from its bounding box, t from the
// middle of a side half long.
func bulge(t, half, depth float64) float64 {
	k := min(1, math.Abs(t)/half)
	return depth * (1 - math.Sqrt(1-float64(k*k)))
}

func roundInset(w, h float64, side int, t float64) float64 {
	if side&1 != 0 {
		return bulge(t, h/2, w/2)
	}
	return bulge(t, w/2, h/2)
}

func roundSpan(w, h float64, sideways bool) float64 {
	if sideways {
		return h * 0.6
	}
	return w * 0.6
}

func capInset(w, _ float64, side int, t float64) float64 {
	if side&1 != 0 {
		return 0
	}
	return bulge(t, w/2, min(12, max(6, w/14)))
}

func fixed(w, h float64) sizer {
	return func(_, _ float64) ShapeSize { return ShapeSize{W: w, H: h} }
}

func triSize(tw, th float64, flip bool) ShapeSize {
	w := max(float64(1.6*tw)+40, 80)
	h := (th + 10) / (1 - (tw+8)/w)
	dy := float64(h/2) - 10 - float64(th/2)
	if flip {
		dy = -dy
	}
	return ShapeSize{W: w, H: h, DY: dy}
}

func path(d, a string) string {
	return `<path d="` + d + `"` + a + `/>`
}

func leanInset(_, h float64, side int, _ float64) float64 {
	if side&1 != 0 {
		return h / 8
	}
	return 0
}

func box(tw, th float64) ShapeSize {
	return ShapeSize{W: max(tw+2*PadX, 2*PadX+12), H: th + 2*PadY}
}

func poly(points []float64, a string) string {
	var d strings.Builder
	for i := 0; i < len(points); i += 2 {
		if i > 0 {
			d.WriteByte(' ')
		}
		d.WriteString(Num(points[i]) + "," + Num(points[i+1]))
	}
	return `<polygon points="` + d.String() + `"` + a + `/>`
}

func rect(w, h float64, a, rx string) string {
	return `<rect x="` + Num(-w/2) + `" y="` + Num(-h/2) + `" width="` + Num(w) + `" height="` + Num(h) + `"` + rx + a + `/>`
}

// sqrt1_2 is Math.SQRT1_2.
const sqrt1_2 = 0.7071067811865476

// padded is the size of a shape that is the label padded on every side, plus extra width
// and height.
func padded(extraW, extraH, dy float64) sizer {
	return func(tw, th float64) ShapeSize {
		return ShapeSize{W: tw + 2*PadX + extraW, H: th + 2*PadY + extraH, DY: dy}
	}
}

// heightShare is the size of a shape whose width grows by a share of its height.
func heightShare(share float64) sizer {
	return func(tw, th float64) ShapeSize {
		h := th + 2*PadY
		return ShapeSize{W: tw + 2*PadX + float64(h*share), H: h}
	}
}

var shapes = map[string]shapeDef{
	"rect": {size: box, span: flat, draw: func(w, h float64, a, _ string) string { return rect(w, h, Rounded(a), "") }},
	"rounded": {size: box, span: flat, draw: func(w, h float64, a, _ string) string {
		return rect(w, h, a, ` rx="`+Num(min(12, h/2))+`"`)
	}},
	"stadium": {
		inset: func(_, h float64, side int, t float64) float64 {
			if side&1 != 0 {
				return bulge(t, h/2, h/2)
			}
			return 0
		},
		span: func(w, h float64, sideways bool) float64 {
			if sideways {
				return h * 0.6
			}
			return w - h
		},
		size: func(tw, th float64) ShapeSize {
			h := th + 2*PadY
			return ShapeSize{W: tw + float64(h*0.5) + 2*PadX - 8, H: h}
		},
		draw: func(w, h float64, a, _ string) string { return rect(w, h, a, ` rx="`+Num(h/2)+`"`) },
	},
	"fr-rect": {
		span: flat,
		size: padded(16, 0, 0),
		draw: func(w, h float64, a, line string) string {
			x := float64(w/2) - 8
			return rect(w, h, Rounded(a), "") +
				`<path d="M` + Num(-x) + "," + Num(-h/2) + "V" + Num(h/2) + "M" + Num(x) + "," + Num(-h/2) + "V" + Num(h/2) + `" fill="none"` + line + `/>`
		},
	},
	"cyl": {
		inset: capInset,
		span: func(w, h float64, sideways bool) float64 {
			if sideways {
				return h - 2*shapeCorner - 12
			}
			return w * 0.6
		},
		size: func(tw, th float64) ShapeSize {
			w := max(tw+2*PadX, 56)
			ry := min(12, max(6, w/14))
			return ShapeSize{W: w, H: th + 2*PadY + float64(2*ry), DY: ry / 2}
		},
		draw: func(w, h float64, a, line string) string {
			rx := float64(w / 2)
			ry := min(12, max(6, w/14))
			top := float64(-h/2) + ry
			bottom := float64(h/2) - ry
			return `<path d="M` + Num(-rx) + "," + Num(top) + "A" + Num(rx) + "," + Num(ry) + " 0 0 1 " + Num(rx) + "," + Num(top) +
				"V" + Num(bottom) + "A" + Num(rx) + "," + Num(ry) + " 0 0 1 " + Num(-rx) + "," + Num(bottom) + `Z"` + a + `/>` +
				`<path d="M` + Num(-rx) + "," + Num(top) + "A" + Num(rx) + "," + Num(ry) + " 0 0 0 " + Num(rx) + "," + Num(top) + `" fill="none"` + line + `/>`
		},
	},
	"circle": {
		inset: roundInset,
		span:  roundSpan,
		size: func(tw, th float64) ShapeSize {
			d := max(math.Sqrt(float64(tw*tw)+float64(th*th))+16, 44)
			return ShapeSize{W: d, H: d}
		},
		draw: func(w, _ float64, a, _ string) string { return `<circle r="` + Num(w/2) + `"` + a + `/>` },
	},
	"dbl-circ": {
		inset: roundInset,
		span:  roundSpan,
		size: func(tw, th float64) ShapeSize {
			d := max(math.Sqrt(float64(tw*tw)+float64(th*th))+26, 54)
			return ShapeSize{W: d, H: d}
		},
		draw: func(w, _ float64, a, line string) string {
			return `<circle r="` + Num(w/2) + `"` + a + `/><circle r="` + Num(float64(w/2)-5) + `" fill="none"` + line + `/>`
		},
	},
	"ellipse": {
		inset: roundInset,
		span:  roundSpan,
		size: func(tw, th float64) ShapeSize {
			return ShapeSize{W: float64(tw*1.3) + 2*PadX, H: float64(th*1.2) + 2*PadY}
		},
		draw: func(w, h float64, a, _ string) string {
			return `<ellipse rx="` + Num(w/2) + `" ry="` + Num(h/2) + `"` + a + `/>`
		},
	},
	"diam": {
		inset: func(w, h float64, side int, t float64) float64 {
			if side&1 != 0 {
				return (math.Abs(t) * w) / h
			}
			return (math.Abs(t) * h) / w
		},
		span: roundSpan,
		size: func(tw, th float64) ShapeSize {
			a := tw + 16
			b := th + 8
			return ShapeSize{W: a + float64(1.6*b), H: a/1.6 + b}
		},
		draw: func(w, h float64, a, _ string) string {
			return poly([]float64{0, -h / 2, w / 2, 0, 0, h / 2, -w / 2, 0}, a)
		},
	},
	"hex": {
		inset: func(_, _ float64, side int, t float64) float64 {
			if side&1 != 0 {
				return math.Abs(t) / 2
			}
			return 0
		},
		span: func(w, h float64, sideways bool) float64 {
			if sideways {
				return h * 0.5
			}
			return w - float64(h/2) - shapeCorner
		},
		size: heightShare(0.5),
		draw: func(w, h float64, a, _ string) string {
			m := float64(h / 4)
			return poly([]float64{float64(-w/2) + m, -h / 2, float64(w/2) - m, -h / 2, w / 2, 0, float64(w/2) - m, h / 2, float64(-w/2) + m, h / 2, -w / 2, 0}, a)
		},
	},
	"odd": {
		inset: func(_, h float64, side int, _ float64) float64 {
			if side == 3 {
				return h / 4
			}
			return 0
		},
		span: flatAcross(func(float64) float64 { return 0 }),
		size: heightShare(0.25),
		draw: func(w, h float64, a, _ string) string {
			notch := float64(h / 4)
			return poly([]float64{-w / 2, -h / 2, w / 2, -h / 2, w / 2, h / 2, -w / 2, h / 2, float64(-w/2) + notch, 0}, a)
		},
	},
	"lean-r": {
		inset: leanInset,
		span:  flatAcross(func(h float64) float64 { return h / 4 }),
		size:  heightShare(0.5),
		draw: func(w, h float64, a, _ string) string {
			s := float64(h / 4)
			return poly([]float64{float64(-w/2) + s, -h / 2, w / 2, -h / 2, float64(w/2) - s, h / 2, -w / 2, h / 2}, a)
		},
	},
	"lean-l": {
		inset: leanInset,
		span:  flatAcross(func(h float64) float64 { return h / 4 }),
		size:  heightShare(0.5),
		draw: func(w, h float64, a, _ string) string {
			s := float64(h / 4)
			return poly([]float64{-w / 2, -h / 2, float64(w/2) - s, -h / 2, w / 2, h / 2, float64(-w/2) + s, h / 2}, a)
		},
	},
	"trap-b": {
		inset: leanInset,
		span:  flatAcross(func(h float64) float64 { return h / 4 }),
		size:  heightShare(0.5),
		draw: func(w, h float64, a, _ string) string {
			s := float64(h / 4)
			return poly([]float64{float64(-w/2) + s, -h / 2, float64(w/2) - s, -h / 2, w / 2, h / 2, -w / 2, h / 2}, a)
		},
	},
	"trap-t": {
		inset: leanInset,
		span:  flatAcross(func(h float64) float64 { return h / 4 }),
		size:  heightShare(0.5),
		draw: func(w, h float64, a, _ string) string {
			s := float64(h / 4)
			return poly([]float64{-w / 2, -h / 2, w / 2, -h / 2, float64(w/2) - s, h / 2, float64(-w/2) + s, h / 2}, a)
		},
	},
	"text": {
		span: flat,
		size: func(tw, th float64) ShapeSize { return ShapeSize{W: tw + 8, H: th + 8} },
		draw: func(w, h float64, _, _ string) string { return rect(w, h, ` fill="none" stroke="none"`, "") },
	},
	"sm-circ": {
		noLabel: true,
		inset:   roundInset,
		span:    roundSpan,
		size:    fixed(14, 14),
		draw:    func(w, _ float64, a, _ string) string { return `<circle r="` + Num(w/2) + `"` + a + `/>` },
	},
	"f-circ": {
		noLabel: true,
		inset:   roundInset,
		span:    roundSpan,
		size:    fixed(14, 14),
		draw: func(w, _ float64, _, line string) string {
			return `<circle r="` + Num(w/2) + `" fill="var(--_l)"` + line + `/>`
		},
	},
	"fr-circ": {
		noLabel: true,
		inset:   roundInset,
		span:    roundSpan,
		size:    fixed(18, 18),
		draw: func(w, _ float64, a, line string) string {
			return `<circle r="` + Num(w/2) + `"` + a + `/><circle r="` + Num(float64(w/2)-4) + `" fill="var(--_l)"` + line + `/>`
		},
	},
	"fork": {
		noLabel: true,
		span: func(w, h float64, sideways bool) float64 {
			if sideways {
				return h - 8
			}
			return w - 8
		},
		size: fixed(72, 8),
		draw: func(w, h float64, _, line string) string { return rect(w, h, ` fill="var(--_l)"`+line, ` rx="2"`) },
	},
	"notch-rect": {
		size: box,
		span: flat,
		draw: func(w, h float64, a, _ string) string {
			const c = 10
			return poly([]float64{float64(-w/2) + c, -h / 2, w / 2, -h / 2, w / 2, h / 2, -w / 2, h / 2, -w / 2, float64(-h/2) + c}, a)
		},
	},
	"lin-rect": {
		span: flat,
		size: padded(8, 0, 0),
		draw: func(w, h float64, a, line string) string {
			return rect(w, h, Rounded(a), "") + `<path d="M` + Num(float64(-w/2)+8) + "," + Num(-h/2) + "V" + Num(h/2) + `" fill="none"` + line + `/>`
		},
	},
	"div-rect": {
		span: flat,
		size: padded(0, 8, 4),
		draw: func(w, h float64, a, line string) string {
			return rect(w, h, Rounded(a), "") + `<path d="M` + Num(-w/2) + "," + Num(float64(-h/2)+8) + "H" + Num(w/2) + `" fill="none"` + line + `/>`
		},
	},
	"win-pane": {
		span: flat,
		size: padded(8, 8, 4),
		draw: func(w, h float64, a, line string) string {
			return rect(w, h, Rounded(a), "") + `<path d="M` + Num(float64(-w/2)+8) + "," + Num(-h/2) + "V" + Num(h/2) + "M" + Num(-w/2) + "," + Num(float64(-h/2)+8) +
				"H" + Num(w/2) + `" fill="none"` + line + `/>`
		},
	},
	"tri": {
		inset: func(w, _ float64, side int, _ float64) float64 {
			if side&1 != 0 {
				return w / 4
			}
			return 0
		},
		size: func(tw, th float64) ShapeSize { return triSize(tw, th, false) },
		draw: func(w, h float64, a, _ string) string {
			return poly([]float64{0, -h / 2, w / 2, h / 2, -w / 2, h / 2}, a)
		},
	},
	"flip-tri": {
		inset: func(w, _ float64, side int, _ float64) float64 {
			if side&1 != 0 {
				return w / 4
			}
			return 0
		},
		size: func(tw, th float64) ShapeSize { return triSize(tw, th, true) },
		draw: func(w, h float64, a, _ string) string {
			return poly([]float64{-w / 2, -h / 2, w / 2, -h / 2, 0, h / 2}, a)
		},
	},
	"sl-rect": {
		inset: func(_, _ float64, side int, _ float64) float64 {
			if side == 0 {
				return 5
			}
			return 0
		},
		size: padded(0, 10, 5),
		draw: func(w, h float64, a, _ string) string {
			return poly([]float64{-w / 2, float64(-h/2) + 10, w / 2, -h / 2, w / 2, h / 2, -w / 2, h / 2}, a)
		},
	},
	"notch-pent": {
		span: flat,
		size: padded(0, 0, 0),
		draw: func(w, h float64, a, _ string) string {
			const c = 10
			return poly([]float64{float64(-w/2) + c, -h / 2, float64(w/2) - c, -h / 2, w / 2, float64(-h/2) + c, w / 2, h / 2, -w / 2, h / 2, -w / 2, float64(-h/2) + c}, a)
		},
	},
	"hourglass": {
		noLabel: true,
		inset: func(w, _ float64, side int, _ float64) float64 {
			if side&1 != 0 {
				return w / 2
			}
			return 0
		},
		size: fixed(32, 40),
		draw: func(w, h float64, a, _ string) string {
			return poly([]float64{-w / 2, -h / 2, w / 2, -h / 2, -w / 2, h / 2, w / 2, h / 2}, a)
		},
	},
	"delay": {
		size: heightShare(0.25),
		draw: func(w, h float64, a, _ string) string {
			r := float64(h / 2)
			return `<path d="M` + Num(-w/2) + "," + Num(-r) + "H" + Num(float64(w/2)-r) + "A" + Num(r) + "," + Num(r) + " 0 0 1 " +
				Num(float64(w/2)-r) + "," + Num(r) + "H" + Num(-w/2) + `Z"` + a + `/>`
		},
	},
	"h-cyl": {
		size: func(tw, th float64) ShapeSize {
			h := th + 2*PadY
			rx := min(12, h/4)
			return ShapeSize{W: tw + 2*PadX + float64(2*rx), H: h}
		},
		draw: func(w, h float64, a, line string) string {
			ry := float64(h / 2)
			rx := min(12, h/4)
			l := float64(-w/2) + rx
			r := float64(w/2) - rx
			return `<path d="M` + Num(l) + "," + Num(-ry) + "H" + Num(r) + "A" + Num(rx) + "," + Num(ry) + " 0 0 1 " + Num(r) + "," + Num(ry) +
				"H" + Num(l) + "A" + Num(rx) + "," + Num(ry) + " 0 0 1 " + Num(l) + "," + Num(-ry) + `Z"` + a + `/>` +
				`<path d="M` + Num(r) + "," + Num(-ry) + "A" + Num(rx) + "," + Num(ry) + " 0 0 0 " + Num(r) + "," + Num(ry) + `" fill="none"` + line + `/>`
		},
	},
	"cross-circ": {
		noLabel: true,
		inset:   roundInset,
		span:    roundSpan,
		size:    fixed(44, 44),
		draw: func(w, _ float64, a, line string) string {
			r := float64(w / 2)
			k := float64(r * sqrt1_2)
			return `<circle r="` + Num(r) + `"` + a + `/>` +
				`<path d="M` + Num(-k) + "," + Num(-k) + "L" + Num(k) + "," + Num(k) + "M" + Num(k) + "," + Num(-k) + "L" + Num(-k) + "," + Num(k) + `" fill="none"` + line + `/>`
		},
	},
	"st-rect": {
		span: func(w, h float64, sideways bool) float64 {
			if sideways {
				return h - 2*shapeCorner - 8
			}
			return w - 2*shapeCorner - 8
		},
		size: padded(8, 8, 4),
		draw: func(w, h float64, a, _ string) string {
			bw := w - 8
			bh := h - 8
			var out strings.Builder
			for i := range 3 {
				x := float64(-w/2) + float64((2-i)*4)
				y := float64(-h/2) + float64(i*4)
				out.WriteString(`<rect x="` + Num(x) + `" y="` + Num(y) + `" width="` + Num(bw) + `" height="` + Num(bh) + `"` + Rounded(a) + `/>`)
			}
			return out.String()
		},
	},
	"doc": {
		inset: func(_, _ float64, side int, _ float64) float64 {
			if side == 2 {
				return 10
			}
			return 0
		},
		size: padded(0, 10, -5),
		draw: func(w, h float64, a, _ string) string { return wave(w, h, a) },
	},
	"flag": {
		inset: func(_, _ float64, side int, _ float64) float64 {
			if side&1 != 0 {
				return 0
			}
			return 8
		},
		size: padded(0, 16, 0),
		draw: func(w, h float64, a, _ string) string {
			x := float64(w / 2)
			y := float64(h/2) - 8
			q := float64(w / 4)
			return `<path d="M` + Num(-x) + "," + Num(-y) + "q" + Num(q) + ",-10 " + Num(float64(2*q)) + ",0t" + Num(float64(2*q)) + ",0V" + Num(y) +
				"q" + Num(-q) + ",10 " + Num(float64(-2*q)) + ",0t" + Num(float64(-2*q)) + `,0Z"` + a + `/>`
		},
	},
	"datastore": {
		size: box,
		span: flat,
		draw: func(w, h float64, a, line string) string {
			return rect(w, h, strings.Replace(a, `stroke="var(--_b)"`, `stroke="none"`, 1), "") +
				`<path d="M` + Num(-w/2) + "," + Num(-h/2) + "H" + Num(w/2) + "M" + Num(-w/2) + "," + Num(h/2) + "H" + Num(w/2) + `" fill="none"` + line + `/>`
		},
	},
	"folder": {
		size: func(tw, th float64) ShapeSize { return ShapeSize{W: max(tw+2*PadX, 64), H: th + 2*PadY + 8, DY: 4} },
		draw: func(w, h float64, a, _ string) string {
			x := float64(w / 2)
			y := float64(h / 2)
			tab := min(float64(w*0.4), 48)
			return path("M"+Num(-x)+","+Num(-y)+"H"+Num(-x+tab)+"L"+Num(-x+tab+8)+","+Num(-y+8)+"H"+Num(x)+"V"+Num(y)+"H"+Num(-x)+"Z", a)
		},
	},
	"bucket": {
		size: padded(20, 10, 3),
		inset: func(_, h float64, side int, _ float64) float64 {
			if side&1 != 0 {
				return h / 12
			}
			return 0
		},
		draw: func(w, h float64, a, line string) string {
			x := float64(w / 2)
			y := float64(h / 2)
			const ry = 5
			taper := float64(h / 6)
			return path("M"+Num(-x)+","+Num(-y+ry)+"A"+Num(x)+","+Num(ry)+" 0 0 1 "+Num(x)+","+Num(-y+ry)+"L"+Num(x-taper)+","+Num(y-ry)+
				"A"+Num(x-taper)+","+Num(ry)+" 0 0 1 "+Num(-x+taper)+","+Num(y-ry)+"Z", a) +
				`<path d="M` + Num(-x) + "," + Num(-y+ry) + "A" + Num(x) + "," + Num(ry) + " 0 0 0 " + Num(x) + "," + Num(-y+ry) + `" fill="none"` + line + `/>`
		},
	},
	"console": {
		span: flat,
		size: func(tw, th float64) ShapeSize { return ShapeSize{W: max(tw+2*PadX, 72), H: th + 2*PadY + 12, DY: 6} },
		draw: func(w, h float64, a, line string) string {
			return rect(w, h, Rounded(a), "") +
				`<path d="M` + Num(-w/2) + "," + Num(float64(-h/2)+12) + "H" + Num(w/2) + "M" + Num(float64(-w/2)+7) + "," + Num(float64(-h/2)+4) + "l3,2.5l-3,2.5M" +
				Num(float64(-w/2)+13) + "," + Num(float64(-h/2)+9) + `h4" fill="none"` + line + `/>`
		},
	},
	"browser": {
		span: flat,
		size: func(tw, th float64) ShapeSize { return ShapeSize{W: max(tw+2*PadX, 72), H: th + 2*PadY + 12, DY: 6} },
		draw: func(w, h float64, a, line string) string {
			var dots strings.Builder
			for i := range 3 {
				dots.WriteString(`<circle cx="` + Num(float64(-w/2)+8+float64(i*7)) + `" cy="` + Num(float64(-h/2)+6) + `" r="1.5" fill="none"` + line + `/>`)
			}
			return rect(w, h, Rounded(a), "") + `<path d="M` + Num(-w/2) + "," + Num(float64(-h/2)+12) + "H" + Num(w/2) + `" fill="none"` + line + `/>` + dots.String()
		},
	},
	"person": {
		size: func(tw, th float64) ShapeSize { return ShapeSize{W: max(tw+2*PadX, 56), H: th + 2*PadY + 26, DY: 13} },
		draw: func(w, h float64, a, _ string) string {
			top := float64(-h / 2)
			return `<rect x="` + Num(-w/2) + `" y="` + Num(top+26) + `" width="` + Num(w) + `" height="` + Num(h-26) + `" rx="12"` + a + `/>` +
				`<circle cy="` + Num(top+14) + `" r="13"` + a + `/>`
		},
	},
	"bang": {
		size:  padded(28, 24, 0),
		inset: func(float64, float64, int, float64) float64 { return 3 },
		draw: func(w, h float64, a, _ string) string {
			const spikes = 14
			pi := math.Pi
			pts := make([]float64, 0, 4*spikes)
			for i := range spikes * 2 {
				angle := float64(pi*float64(i))/spikes - pi/2
				k := 0.82
				if i%2 == 0 {
					k = 1
				}
				pts = append(pts, float64(jsmath.Cos(angle)*(w/2))*k, float64(jsmath.Sin(angle)*(h/2))*k)
			}
			return poly(pts, a)
		},
	},
	"cloud": {
		size: padded(32, 24, 0),
		inset: func(_, _ float64, side int, _ float64) float64 {
			if side&1 != 0 {
				return 2
			}
			return 1
		},
		draw: func(w, h float64, a, _ string) string {
			x := float64(w / 2)
			y := float64(h / 2)
			r := float64(y * 0.55)
			return path("M"+Num(-x+r)+","+Num(y)+"A"+Num(r)+","+Num(r)+" 0 0 1 "+Num(-x+float64(r*0.6))+","+Num(-y*0.1)+
				"A"+Num(x*0.42)+","+Num(y*0.7)+" 0 0 1 "+Num(-x*0.1)+","+Num(-y*0.72)+
				"A"+Num(x*0.4)+","+Num(y*0.62)+" 0 0 1 "+Num(x*0.62)+","+Num(-y*0.42)+
				"A"+Num(r*1.05)+","+Num(r*1.05)+" 0 0 1 "+Num(x-r)+","+Num(y)+"Z", a)
		},
	},
	"brace": {
		size: padded(12, 0, 0),
		draw: func(w, h float64, _, line string) string {
			return rect(w, h, ` fill="none" stroke="none"`, "") + brace(float64(-w/2)+12, h, -1, line)
		},
	},
	"brace-r": {
		size: padded(12, 0, 0),
		draw: func(w, h float64, _, line string) string {
			return rect(w, h, ` fill="none" stroke="none"`, "") + brace(float64(w/2)-12, h, 1, line)
		},
	},
	"braces": {
		size: padded(24, 0, 0),
		draw: func(w, h float64, _, line string) string {
			return rect(w, h, ` fill="none" stroke="none"`, "") + brace(float64(-w/2)+12, h, -1, line) + brace(float64(w/2)-12, h, 1, line)
		},
	},
	"bolt": {
		noLabel: true,
		size:    fixed(36, 56),
		inset: func(w, _ float64, side int, _ float64) float64 {
			if side&1 != 0 {
				return w / 4
			}
			return 0
		},
		draw: func(w, h float64, a, _ string) string {
			return poly([]float64{w * 0.15, -h / 2, -w / 2, h * 0.08, -w * 0.05, h * 0.08, -w * 0.15, h / 2, w / 2, -h * 0.08, w * 0.05, -h * 0.08}, a)
		},
	},
	"lin-cyl": {
		inset: capInset,
		span: func(w, h float64, sideways bool) float64 {
			if sideways {
				return h - 2*shapeCorner - 20
			}
			return w * 0.6
		},
		size: func(tw, th float64) ShapeSize {
			w := max(tw+2*PadX, 56)
			ry := min(12, max(6, w/14))
			return ShapeSize{W: w, H: th + 2*PadY + float64(2*ry) + 8, DY: float64(ry/2) + 4}
		},
		draw: func(w, h float64, a, line string) string {
			rx := float64(w / 2)
			ry := min(12, max(6, w/14))
			top := float64(-h/2) + ry
			bottom := float64(h/2) - ry
			arc := func(y float64) string {
				return "M" + Num(-rx) + "," + Num(y) + "A" + Num(rx) + "," + Num(ry) + " 0 0 0 " + Num(rx) + "," + Num(y)
			}
			return path("M"+Num(-rx)+","+Num(top)+"A"+Num(rx)+","+Num(ry)+" 0 0 1 "+Num(rx)+","+Num(top)+"V"+Num(bottom)+
				"A"+Num(rx)+","+Num(ry)+" 0 0 1 "+Num(-rx)+","+Num(bottom)+"Z", a) +
				`<path d="` + arc(top) + arc(top+8) + `" fill="none"` + line + `/>`
		},
	},
	"curv-trap": {
		size: heightShare(0.6),
		draw: func(w, h float64, a, _ string) string {
			x := float64(w / 2)
			y := float64(h / 2)
			k := float64(h * 0.3)
			return path("M"+Num(-x+k)+","+Num(-y)+"H"+Num(x-k)+"A"+Num(k)+","+Num(y)+" 0 0 1 "+Num(x-k)+","+Num(y)+"H"+Num(-x+k)+"L"+Num(-x)+",0Z", a)
		},
	},
	"docs": {
		size: padded(8, 18, -1),
		inset: func(_, _ float64, side int, _ float64) float64 {
			if side == 2 {
				return 10
			}
			return 0
		},
		draw: func(w, h float64, a, _ string) string {
			var out strings.Builder
			for i := range 3 {
				out.WriteString(`<g transform="translate(` + jsnum.Format(float64((1-i)*4)) + "," + jsnum.Format(float64((i-1)*4)) + `)">` + wave(w-8, h-8, a) + `</g>`)
			}
			return out.String()
		},
	},
	"bow-rect": {
		size: heightShare(0.4),
		inset: func(_, h float64, side int, _ float64) float64 {
			if side == 1 {
				return h * 0.2
			}
			return 0
		},
		draw: func(w, h float64, a, _ string) string {
			x := float64(w / 2)
			y := float64(h / 2)
			k := float64(h * 0.2)
			return path("M"+Num(-x+k)+","+Num(-y)+"H"+Num(x)+"A"+Num(k)+","+Num(y)+" 0 0 0 "+Num(x)+","+Num(y)+"H"+Num(-x+k)+
				"A"+Num(k)+","+Num(y)+" 0 0 1 "+Num(-x+k)+","+Num(-y)+"Z", a)
		},
	},
	"tag-rect": {
		size: box,
		span: flat,
		draw: func(w, h float64, a, line string) string {
			return rect(w, h, Rounded(a), "") + `<path d="M` + Num(float64(w/2)-12) + "," + Num(h/2) + "L" + Num(w/2) + "," + Num(float64(h/2)-12) + `" fill="none"` + line + `/>`
		},
	},
	"tag-doc": {
		size: padded(0, 10, -5),
		inset: func(_, _ float64, side int, _ float64) float64 {
			if side == 2 {
				return 10
			}
			return 0
		},
		draw: func(w, h float64, a, line string) string {
			return wave(w, h, a) + `<path d="M` + Num(float64(w/2)-12) + "," + Num(float64(h/2)-14) + "L" + Num(w/2) + "," + Num(float64(h/2)-26) + `" fill="none"` + line + `/>`
		},
	},
	"lin-doc": {
		size: padded(8, 10, -5),
		inset: func(_, _ float64, side int, _ float64) float64 {
			if side == 2 {
				return 10
			}
			return 0
		},
		draw: func(w, h float64, a, line string) string {
			return wave(w, h, a) + `<path d="M` + Num(float64(-w/2)+8) + "," + Num(-h/2) + "V" + Num(float64(h/2)-6) + `" fill="none"` + line + `/>`
		},
	},
	"collapsedGroup": {
		size: padded(8, 8, -2),
		draw: func(w, h float64, a, _ string) string {
			return `<rect x="` + Num(float64(-w/2)+6) + `" y="` + Num(float64(-h/2)+8) + `" width="` + Num(w-12) + `" height="` + Num(h-8) + `"` + Rounded(a) + `/>` +
				`<rect x="` + Num(-w/2) + `" y="` + Num(-h/2) + `" width="` + Num(w) + `" height="` + Num(h-4) + `"` + Rounded(a) + `/>`
		},
	},
}

func brace(x, h, dir float64, line string) string {
	y := float64(h / 2)
	k := float64(6 * dir)
	return `<path d="M` + Num(x) + "," + Num(-y) + "q" + Num(k) + ",0 " + Num(k) + ",6V-6q0,6 " + Num(k) + ",6q" + Num(-k) + ",0 " + Num(-k) + ",6V" +
		Num(y-6) + "q0,6 " + Num(-k) + `,6" fill="none"` + line + `/>`
}

func wave(w, h float64, a string) string {
	x := float64(w / 2)
	top := float64(-h / 2)
	base := float64(h/2) - 10
	q := float64(w / 4)
	return `<path d="M` + Num(-x) + "," + Num(top) + "H" + Num(x) + "V" + Num(base) + "q" + Num(-q) + ",-12 " + Num(float64(-2*q)) + ",0t" + Num(float64(-2*q)) + `,0Z"` + a + `/>`
}

func shapeOrRect(shape string) shapeDef {
	if def, ok := shapes[shape]; ok {
		return def
	}
	return shapes["rect"]
}

// ShapeSizeOf is the size of a shape for a label tw by th. An unknown shape is a rect.
func ShapeSizeOf(shape string, tw, th float64) ShapeSize {
	return shapeOrRect(shape).size(tw, th)
}

// DrawShape draws a shape w by h centred on the origin, with attributes a and line for the
// lines drawn inside it.
func DrawShape(shape string, w, h float64, a, line string) string {
	return shapeOrRect(shape).draw(w, h, a, line)
}

// ShapeInset is the distance from the bounding box to the outline on a side (0 top, 1 right,
// 2 bottom, 3 left), t from the middle of the side.
func ShapeInset(shape string, w, h float64, side int, t float64) float64 {
	if def, ok := shapes[shape]; ok && def.inset != nil {
		return def.inset(w, h, side, t)
	}
	return 0
}

// ShapeSpan is how much of a side of the shape edges may spread along, when the flow runs
// down or, if sideways, across.
func ShapeSpan(shape string, w, h float64, sideways bool) float64 {
	if def, ok := shapes[shape]; ok && def.span != nil {
		return max(0, def.span(w, h, sideways))
	}
	return 0
}

// ShapeHasLabel reports whether a shape shows its label.
func ShapeHasLabel(shape string) bool {
	def, ok := shapes[shape]
	return !ok || !def.noLabel
}

// EndSide is the side of a node's layout box that a route end lies on: 0 top, 1 right, 2
// bottom, 3 left, or -1 for none.
func EndSide(route []float64, at int, c *layout.CNode) int {
	dx := route[at] - c.X
	dy := route[at+1] - c.Y
	if math.Abs(math.Abs(dy)-float64(c.H/2)) < 0.5 {
		if dy < 0 {
			return 0
		}
		return 2
	}
	if math.Abs(math.Abs(dx)-float64(c.W/2)) < 0.5 {
		if dx < 0 {
			return 3
		}
		return 1
	}
	return -1
}

// InsetRoute moves a route end from a node's layout box onto the outline of the shape drawn
// in it, shape by w by h.
func InsetRoute(route []float64, at int, shape string, w, h float64, c *layout.CNode) {
	if c.IsGroup {
		return
	}
	side := EndSide(route, at, c)
	if side == 0 || side == 2 {
		amount := ShapeInset(shape, w, h, side, route[at]-c.X) + float64((c.H-h)/2)
		if side == 0 {
			route[at+1] += amount
		} else {
			route[at+1] -= amount
		}
	} else if side >= 0 {
		amount := ShapeInset(shape, w, h, side, route[at+1]-c.Y) + float64((c.W-w)/2)
		if side == 3 {
			route[at] += amount
		} else {
			route[at] -= amount
		}
	}
}
