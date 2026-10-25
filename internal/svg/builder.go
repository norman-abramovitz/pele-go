// Package svg writes SVG markup the way upstream's src/svg does.
package svg

import (
	"math"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

func isControl(c byte) bool {
	return c <= 0x08 || c == 0x0b || c == 0x0c || (c >= 0x0e && c <= 0x1f) || c == 0x7f
}

// Esc escapes text for XML and drops the control characters XML does not allow.
func Esc(s string) string {
	needs := false
	for i := 0; i < len(s); i++ {
		if c := s[i]; c == '&' || c == '<' || c == '>' || c == '"' || isControl(c) {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '&':
			b.WriteString("&amp;")
		case c == '<':
			b.WriteString("&lt;")
		case c == '>':
			b.WriteString("&gt;")
		case c == '"':
			b.WriteString("&quot;")
		case isControl(c):
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// EscText escapes text that is only ever read as text: a tooltip, an accessible name, a
// data-id. Entity codes are turned into their characters first.
func EscText(s string) string {
	return Esc(text.DecodeEntities(s))
}

// fraction holds the two decimals of a coordinate, by hundredths.
var fraction = func() [100]string {
	var f [100]string
	for i := 1; i < 100; i++ {
		switch {
		case i%10 == 0:
			f[i] = "." + jsnum.Format(float64(i/10))
		case i < 10:
			f[i] = ".0" + jsnum.Format(float64(i))
		default:
			f[i] = "." + jsnum.Format(float64(i))
		}
	}
	return f
}()

// Num writes a coordinate rounded to two decimals.
func Num(value float64) string {
	n := jsmath.Round(value * 100)
	if n == 0 {
		return "0"
	}
	if n > 0 {
		if n < 1e11 {
			q := math.Floor(n / 100)
			return jsnum.Format(q) + fraction[int(n-float64(q*100))]
		}
	} else if n > -1e11 {
		m := -n
		q := math.Floor(m / 100)
		return "-" + jsnum.Format(q) + fraction[int(m-float64(q*100))]
	}
	return jsnum.Format(n / 100)
}

const (
	boldAttr      = ` font-weight="var(--_w)"`
	italicAttr    = ` font-style="italic"`
	underlineAttr = ` text-decoration="underline"`
	monoAttr      = ` font-family="var(--_fm)"`
	subAttr       = ` baseline-shift="sub" font-size="smaller"`
	supAttr       = ` baseline-shift="super" font-size="smaller"`
)

// SpanStyle returns the attributes for a span's emphasis.
func SpanStyle(style text.Style) string {
	s := ""
	if style&text.Bold != 0 {
		s += boldAttr
	}
	if style&text.Italic != 0 {
		s += italicAttr
	}
	if style&text.Mono != 0 {
		s += monoAttr
	}
	if style&text.Underline != 0 {
		s += underlineAttr
	}
	if style&text.Sub != 0 {
		s += subAttr
	} else if style&text.Sup != 0 {
		s += supAttr
	}
	return s
}

// SharedStyle returns the emphasis every span of a label has. It is written once, on the
// text element, where a CSS rule for the label reaches it.
func SharedStyle(label text.Label) text.Style {
	shared := text.Bold | text.Italic | text.Mono
	any := false
	for _, line := range label.Lines {
		for _, span := range line {
			if span.Icon != "" || span.Text == "" {
				continue
			}
			shared &= span.Style
			any = true
		}
	}
	if !any {
		return 0
	}
	return shared
}

// TextStyle adds that emphasis to a text element's attributes, unless they give a weight of
// their own.
func TextStyle(attrs string, shared text.Style) string {
	if shared&text.Bold != 0 && !strings.Contains(attrs, ` font-weight="`) {
		attrs += boldAttr
	}
	if shared&text.Italic != 0 && !strings.Contains(attrs, ` font-style="`) {
		attrs += italicAttr
	}
	if shared&text.Mono != 0 && !strings.Contains(attrs, ` font-family="`) {
		attrs += monoAttr
	}
	return attrs
}

// LabelSVG writes a measured label as SVG text centered on (cx, cy).
func LabelSVG(label text.Label, cx, cy float64, attrs string, icons *text.Icons) string {
	lines := label.Lines
	if len(lines) == 0 {
		return ""
	}
	lh := label.LineHeight
	heights := label.Heights
	shift := float64(label.Size * 0.35)
	top := cy - float64(label.Height/2)

	if len(lines) == 1 && len(lines[0]) == 1 {
		if only := lines[0][0]; only.Icon == "" && only.Math == nil && only.Style == 0 {
			return `<text` + attrs + ` x="` + Num(cx) + `" y="` + Num(top+float64(lh/2)+shift) + `" text-anchor="middle">` + Esc(only.Text) + `</text>`
		}
	}

	shared := SharedStyle(label)
	given := attrs
	var look *mathLook
	attrs = TextStyle(attrs, shared)
	var body, extra strings.Builder
	for i, line := range lines {
		h := lh
		if heights != nil {
			h = heights[i]
		}
		mid := top + float64(h/2)
		y := mid + shift
		placed := false
		for _, span := range line {
			if span.Icon != "" || span.Math != nil {
				placed = true
			}
		}
		if placed {
			x := cx - float64(label.Widths[i]/2)
			for _, span := range line {
				if span.Math != nil {
					box := `x="` + Num(x) + `" y="` + Num(mid-float64(span.Math.Height/2)) + `" width="` + Num(span.Math.Width) + `" height="` + Num(span.Math.Height) + `"`
					if look == nil {
						l := newMathLook(given)
						look = &l
					}
					if look.halo == nil {
						extra.WriteString(mathSVG(span.Math, box, label.Size, look.attrs))
					} else {
						extra.WriteString(`<rect ` + box + ` rx="2" fill="` + *look.halo + `"/>`)
					}
				} else if span.Icon != "" {
					side := float64(label.Size * 1.1)
					inner := icons.Resolve(span.Icon)
					extra.WriteString(`<svg class="pele-icon" data-icon="` + EscText(span.Icon) + `" x="` + Num(x) + `" y="` + Num(mid-float64(side/2)) +
						`" width="` + Num(side) + `" height="` + Num(side) +
						`" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
						inner + `</svg>`)
				} else if span.Text != "" {
					body.WriteString(`<tspan x="` + Num(x) + `" y="` + Num(y) + `" text-anchor="start"` + SpanStyle(span.Style&^shared) + `>` + Esc(span.Text) + `</tspan>`)
				}
				x += span.Width
			}
		} else {
			for j, span := range line {
				pos := ""
				if j == 0 {
					pos = ` x="` + Num(cx) + `" y="` + Num(y) + `"`
				}
				body.WriteString(`<tspan` + pos + SpanStyle(span.Style&^shared) + `>` + Esc(span.Text) + `</tspan>`)
			}
			if len(line) == 0 {
				body.WriteString(`<tspan x="` + Num(cx) + `" y="` + Num(y) + `"> </tspan>`)
			}
		}
		top += h
	}
	return `<text` + attrs + ` text-anchor="middle" xml:space="preserve">` + body.String() + `</text>` + extra.String()
}

// mathLook copies only color and opacity, to preserve measured math dimensions. A cluster
// title's outline gets a solid patch (halo) to hide edges behind math.
type mathLook struct {
	attrs string
	halo  *string
}

// attrValue is /<prefix>([^"]*)"/ on s: the value of the first such attribute.
func attrValue(s, prefix string) (string, bool) {
	at := strings.Index(s, prefix)
	if at < 0 {
		return "", false
	}
	rest := s[at+len(prefix):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// styleValue is /(?:^|;)\s*<name>:([^;]*)/ on a style.
func styleValue(style, name string) (string, bool) {
	match := func(p int) (string, bool) {
		for p < len(style) {
			r, n := jsstr.Decode(style, p)
			if !jsstr.IsSpace(r) {
				break
			}
			p += n
		}
		if !strings.HasPrefix(style[p:], name+":") {
			return "", false
		}
		rest := style[p+len(name)+1:]
		if end := strings.IndexByte(rest, ';'); end >= 0 {
			rest = rest[:end]
		}
		return rest, true
	}
	if v, ok := match(0); ok {
		return v, true
	}
	for at := 0; at < len(style); at++ {
		if style[at] == ';' {
			if v, ok := match(at + 1); ok {
				return v, true
			}
		}
	}
	return "", false
}

func newMathLook(attrs string) mathLook {
	style, _ := attrValue(attrs, ` style="`)
	fill, ok := styleValue(style, "fill")
	if !ok {
		fill, _ = attrValue(attrs, ` fill="`)
	}
	fill = jsstr.Trim(fill)
	if fill == "none" {
		halo, ok := attrValue(attrs, ` stroke="`)
		if !ok {
			halo = "none"
		}
		return mathLook{halo: &halo}
	}
	out := ""
	if fill != "" {
		out = ` color="` + fill + `"`
	}
	if opacity, ok := styleValue(style, "opacity"); ok {
		if opacity = jsstr.Trim(opacity); opacity != "" {
			out += ` opacity="` + opacity + `"`
		}
	}
	return mathLook{attrs: out}
}

// mathSVG writes typeset math in box. The host's markup is trusted.
func mathSVG(math *text.MathMarkup, box string, size float64, attrs string) string {
	if !math.HTML {
		return `<svg class="pele-math" ` + box + ` overflow="visible"` + attrs + `>` + math.Markup + `</svg>`
	}
	return `<foreignObject class="pele-math" ` + box + ` overflow="visible"` + attrs + `>` +
		`<div xmlns="http://www.w3.org/1999/xhtml" style="display:flex;align-items:center;justify-content:center;width:100%;height:100%;font-size:` + Num(size) +
		`px;line-height:normal;white-space:nowrap">` + math.Markup + `</div>` +
		`</foreignObject>`
}
