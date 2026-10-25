package common

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

// isSpaceChar is /\s/.test(char) for one code point.
func isSpaceChar(char string) bool {
	r, _ := jsstr.Decode(char, 0)
	return jsstr.IsSpace(r)
}

// LiteralLabel lays out a literal label, such as a filename or a protocol field, which must
// keep every character but the spaces a line breaks at. Long words can break between code
// points; measurement stays bounded by one line of text.
func LiteralLabel(s string, measurer text.Measurer, size, room float64, style text.Style) text.Label {
	var lines []string
	for _, raw := range strings.Split(s, "\n") {
		chars := codePoints(raw)
		if len(chars) == 0 {
			lines = append(lines, "")
		}
		from := 0
		for from < len(chars) {
			fits := func(length int) bool {
				return measurer.Width(strings.Join(chars[from:from+length], ""), size, style) <= room
			}
			hi := 1
			for hi < len(chars)-from && fits(hi) {
				hi = min(len(chars)-from, hi*2)
			}
			lo := 1
			for lo < hi {
				mid := (lo + hi + 1) / 2
				if fits(mid) {
					lo = mid
				} else {
					hi = mid - 1
				}
			}
			take := lo
			wraps := from+take < len(chars)
			if wraps && !isSpaceChar(chars[from+take]) {
				for i := take - 1; i > 0; i-- {
					if isSpaceChar(chars[from+i]) {
						take = i + 1
						break
					}
				}
			}
			line := strings.Join(chars[from:from+take], "")
			if trimmed := jsstr.TrimEnd(line); wraps && trimmed != "" {
				line = trimmed
			}
			lines = append(lines, line)
			from += take
			for wraps && from < len(chars) && isSpaceChar(chars[from]) {
				from++
			}
		}
	}
	if s == "" {
		lines = nil
	}
	label := text.Label{Size: size, LineHeight: jsmath.Round(float64(size * 1.5))}
	for _, line := range lines {
		w := measurer.Width(line, size, style)
		label.Widths = append(label.Widths, w)
		label.Lines = append(label.Lines, []text.Span{{Text: line, Style: style, Width: w}})
		label.Width = max(label.Width, w)
	}
	label.Height = float64(len(lines)) * label.LineHeight
	return label
}

// BreakLabel cuts the lines of a label that are wider than room between characters.
func BreakLabel(label text.Label, measurer text.Measurer, room float64) text.Label {
	if label.Width <= room {
		return label
	}
	var lines [][]text.Span
	var widths []float64
	for i, spans := range label.Lines {
		if label.Widths[i] <= room {
			lines = append(lines, spans)
			widths = append(widths, label.Widths[i])
			continue
		}
		var current []text.Span
		width := 0.0
		commit := func() {
			lines = append(lines, current)
			widths = append(widths, width)
			current = nil
			width = 0
		}
		for _, span := range spans {
			if span.Icon != "" || span.Math != nil {
				if len(current) > 0 && width+span.Width > room {
					commit()
				}
				current = append(current, span)
				width += span.Width
				continue
			}
			for _, char := range codePoints(span.Text) {
				var last *text.Span
				if len(current) > 0 {
					last = &current[len(current)-1]
				}
				joined := last != nil && last.Icon == "" && last.Math == nil && last.Style == span.Style
				var added float64
				if joined {
					added = measurer.Width(last.Text+char, label.Size, span.Style) - last.Width
				} else {
					added = measurer.Width(char, label.Size, span.Style)
				}
				if len(current) > 0 && width+added > room {
					commit()
					last = nil
					added = measurer.Width(char, label.Size, span.Style)
				}
				if last != nil && last.Icon == "" && last.Math == nil && last.Style == span.Style {
					last.Text += char
					last.Width += added
				} else {
					current = append(current, text.Span{Text: char, Style: span.Style, Width: added})
				}
				width += added
			}
		}
		if len(current) > 0 || len(spans) == 0 {
			commit()
		}
	}
	out := label
	out.Lines, out.Widths, out.Width = lines, widths, 0
	for _, w := range widths {
		out.Width = max(out.Width, w)
	}
	out.Height = float64(len(lines)) * label.LineHeight
	if label.Heights != nil {
		return text.WithHeights(out)
	}
	return out
}

// LeftLabelSVG writes a label with every line starting at x. A line is anchored at its
// start and keeps its spaces, so that where it begins does not depend on how its text was
// measured.
func LeftLabelSVG(label text.Label, x, top float64, attrs string) string {
	shared := svg.SharedStyle(label)
	open := "<text" + svg.TextStyle(attrs, shared)
	var body strings.Builder
	for i, spans := range label.Lines {
		cy := top + float64((float64(i)+0.5)*label.LineHeight)
		hasIcon := false
		for _, span := range spans {
			if span.Icon != "" {
				hasIcon = true
			}
		}
		if hasIcon {
			width := label.Widths[i]
			line := label
			line.Lines, line.Widths, line.Width, line.Height = [][]text.Span{spans}, []float64{width}, width, label.LineHeight
			body.WriteString(svg.LabelSVG(line, x+float64(width/2), cy, attrs, nil))
			continue
		}
		var t strings.Builder
		for _, span := range spans {
			if own := svg.SpanStyle(span.Style &^ shared); own != "" {
				t.WriteString("<tspan" + own + ">" + svg.Esc(span.Text) + "</tspan>")
			} else {
				t.WriteString(svg.Esc(span.Text))
			}
		}
		if t.Len() > 0 {
			body.WriteString(open + ` x="` + svg.Num(x) + `" y="` + svg.Num(cy+float64(label.Size*0.35)) + `" text-anchor="start" xml:space="preserve">` + t.String() + "</text>")
		}
	}
	return body.String()
}
