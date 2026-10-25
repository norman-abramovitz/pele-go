package svg

import (
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/omap"
)

// RootStyle holds the private aliases set once on the root element, so each use is short
// and the public --pele-* tokens keep their fallbacks.
const RootStyle = "--_bg:var(--pele-bg,#fff);--_fg:var(--pele-fg,#1f1f1f);--_m:var(--pele-muted,#6e6e6e);" +
	"--_l:var(--pele-line,#1f1f1f);--_s:var(--pele-surface,#f3f3f3);--_a:var(--pele-surface-alt,#e6e6e6);" +
	"--_b:var(--pele-border,#8a8a8a);--_c:var(--pele-accent,#5b7bd5);color:var(--_fg)"

// Theme values used across diagram types.
const (
	FontMono = "var(--_fm)"
	Radius   = "var(--pele-radius,4px)"
	Critical = "var(--pele-critical,#e45756)"
	// RoundedStyle rounds a rect's corners. Older browsers take a variable for rx only in CSS.
	// The default goes first so diagram styles can override it.
	RoundedStyle = ` style="rx:var(--_r)"`
)

// Rounded adds the corner radius to a rect's attributes.
func Rounded(attrs string) string {
	at := strings.Index(attrs, ` style="`)
	if at < 0 {
		return attrs + RoundedStyle
	}
	return attrs[:at+8] + "rx:var(--_r);" + attrs[at+8:]
}

func isFamilyQuoted(r rune) bool {
	return !strings.ContainsRune("\"'\\\n<>{}();:@", r)
}

type span struct{ lo, hi rune }

func inSpans(spans []span, r rune) bool {
	lo, hi := 0, len(spans)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		switch {
		case r < spans[mid].lo:
			hi = mid - 1
		case r > spans[mid].hi:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// isFamilyStart is [\p{L}\p{N}_-], with V8's Unicode tables (see unicode.go).
func isFamilyStart(r rune) bool {
	return inSpans(letters, r) || inSpans(numbers, r) || r == '_' || r == '-'
}

func isFamilyRest(r rune) bool {
	return isFamilyStart(r) || r == ' ' || r == '.'
}

func skip(s string, i int, ok func(rune) bool) int {
	for i < len(s) {
		r, n := jsstr.Decode(s, i)
		if !ok(r) {
			break
		}
		i += n
	}
	return i
}

// family matches one font name at i: quoted, or a run of letters, digits and _ .- and
// spaces. It returns -1 when there is none.
func family(s string, i int) int {
	if i >= len(s) {
		return -1
	}
	if q := s[i]; q == '"' || q == '\'' {
		end := skip(s, i+1, isFamilyQuoted)
		if end < len(s) && s[end] == q {
			return end + 1
		}
		return -1
	}
	if r, n := jsstr.Decode(s, i); isFamilyStart(r) {
		return skip(s, i+n, isFamilyRest)
	}
	return -1
}

// isFamilies reports whether list is a list of font names, quoted or not, and nothing that
// could end the declaration it is put in:
// /^\s*FAMILY(?:\s*,\s*FAMILY)*\s*$/u
func isFamilies(list string) bool {
	i := family(list, skip(list, 0, jsstr.IsSpace))
	if i == -1 {
		return false
	}
	for {
		j := skip(list, i, jsstr.IsSpace)
		if j < len(list) && list[j] == ',' {
			if next := family(list, skip(list, j+1, jsstr.IsSpace)); next != -1 {
				i = next
				continue
			}
		}
		return j == len(list)
	}
}

func families(list *string, generic string) string {
	if list != nil && *list != "" && isFamilies(*list) {
		return *list
	}
	return generic
}

// FontStyle returns the root's font declarations. The fonts the labels were measured in are
// the ones they are drawn in, unless the page names others.
func FontStyle(size float64, family, mono *string) string {
	return ";--_fm:var(--pele-font-mono," + families(mono, "monospace") + ");font:" + jsnum.Format(size) + "px sans-serif;" +
		"letter-spacing:normal;word-spacing:normal;text-transform:none;direction:ltr;" +
		"font-family:var(--pele-font," + families(family, "sans-serif") + ")"
}

var shapeProps = map[string]bool{
	"fill": true, "fill-opacity": true, "stroke": true, "stroke-width": true, "stroke-dasharray": true,
	"stroke-dashoffset": true, "stroke-linecap": true, "stroke-linejoin": true, "stroke-opacity": true,
	"opacity": true, "rx": true, "ry": true,
}

var textProps = map[string]bool{
	"color": true, "font-size": true, "font-family": true, "font-weight": true, "font-style": true,
	"text-decoration": true, "letter-spacing": true, "word-spacing": true, "opacity": true,
}

// hasPrefixFold reports whether s has the lower-case ASCII word at i, as a regex with the
// i flag and without u matches it: ASCII letters in either case.
func hasPrefixFold(s string, i int, word string) bool {
	if i+len(word) > len(s) {
		return false
	}
	for k := 0; k < len(word); k++ {
		c := s[i+k]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != word[k] {
			return false
		}
	}
	return true
}

// spaceThenParen matches \s*\( at i.
func spaceThenParen(s string, i int) bool {
	i = skip(s, i, jsstr.IsSpace)
	return i < len(s) && s[i] == '('
}

// isUnsafe is /url\s*\(|expression\s*\(|javascript:|[<>{};\\]|@import|image-set|\/\*/i.
func isUnsafe(value string) bool {
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '<', '>', '{', '}', ';', '\\':
			return true
		}
		if hasPrefixFold(value, i, "url") && spaceThenParen(value, i+3) ||
			hasPrefixFold(value, i, "expression") && spaceThenParen(value, i+10) ||
			hasPrefixFold(value, i, "javascript:") ||
			hasPrefixFold(value, i, "@import") ||
			hasPrefixFold(value, i, "image-set") ||
			strings.HasPrefix(value[i:], "/*") {
			return true
		}
	}
	return false
}

// dropImportant is value.replace(/!important/gi, ”).
func dropImportant(value string) string {
	var b strings.Builder
	last := 0
	for i := 0; i < len(value); {
		if hasPrefixFold(value, i, "!important") {
			b.WriteString(value[last:i])
			i += len("!important")
			last = i
			continue
		}
		i++
	}
	if last == 0 {
		return value
	}
	return b.String() + value[last:]
}

// propertyName lower-cases a property for matching against the property lists. Of every
// character only A-Z and the Kelvin sign lower-case to ASCII in JS; the rest cannot match.
func propertyName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 'a' - 'A')
		case r == '\u212a':
			b.WriteByte('k')
		case r < 0x80:
			b.WriteRune(r)
		default:
			// Stands for any character that keeps the name from matching.
			b.WriteByte(0)
		}
	}
	return b.String()
}

// isPixelSize is /^[\d.]+(px)?$/.
func isPixelSize(s string) bool {
	body := strings.TrimSuffix(s, "px")
	if body == "" {
		return false
	}
	for i := 0; i < len(body); i++ {
		if c := body[i]; c != '.' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// ResolvedStyle is Mermaid style declarations split into what applies to a shape, to the
// lines of a shape without its fill, and to its text. Stroke is "" and FontSize 0 where
// upstream's are undefined: neither can otherwise be empty or zero.
type ResolvedStyle struct {
	Shape, Line, Text string
	Stroke            string
	Bold, Italic      bool
	FontSize          float64
}

// ResolveStyle splits Mermaid style declarations into what applies to a shape and what
// applies to its text.
func ResolveStyle(declarations []string) ResolvedStyle {
	if len(declarations) == 0 {
		return ResolvedStyle{}
	}
	var shape, txt omap.Map[string, string]
	for _, decl := range declarations {
		at := strings.IndexByte(decl, ':')
		if at <= 0 {
			continue
		}
		prop := propertyName(jsstr.Trim(decl[:at]))
		value := jsstr.Trim(dropImportant(decl[at+1:]))
		if value == "" || isUnsafe(value) {
			continue
		}
		if shapeProps[prop] {
			shape.Set(prop, value)
		}
		if textProps[prop] {
			if prop == "color" {
				prop = "fill"
			}
			txt.Set(prop, value)
		}
	}
	if shape.Len() == 0 && txt.Len() == 0 {
		return ResolvedStyle{}
	}
	var shapeCSS, lineCSS, textCSS string
	for prop, value := range shape.All() {
		shapeCSS += prop + ":" + value + ";"
		if prop != "fill" && prop != "fill-opacity" {
			lineCSS += prop + ":" + value + ";"
		}
	}
	for prop, value := range txt.All() {
		textCSS += prop + ":" + value + ";"
	}
	attr := func(css string) string {
		if css == "" {
			return ""
		}
		return ` style="` + Esc(css) + `"`
	}
	weight, hasWeight := txt.Get("font-weight")
	style, _ := txt.Get("font-style")
	stroke, _ := shape.Get("stroke")
	r := ResolvedStyle{
		Shape:  attr(shapeCSS),
		Line:   attr(lineCSS),
		Text:   attr(textCSS),
		Stroke: stroke,
		Bold:   weight == "bold" || weight == "bolder" || hasWeight && jsnum.Parse(weight) >= 600,
		Italic: style == "italic",
	}
	if size, ok := txt.Get("font-size"); ok && isPixelSize(size) {
		if px := jsnum.ParseFloat(size); px > 0 && px <= 200 {
			r.FontSize = px
		}
	}
	return r
}

// StripDefault is s.replace(/^default\s?/, ”): a class list without the default class every
// node starts with.
func StripDefault(s string) string {
	rest, ok := strings.CutPrefix(s, "default")
	if !ok {
		return s
	}
	if rest == "" {
		return rest
	}
	if r, n := jsstr.Decode(rest, 0); jsstr.IsSpace(r) {
		rest = rest[n:]
	}
	return rest
}

// ClassNames turns a class list as written into one for an attribute, with a leading space,
// or "" when it has none.
func ClassNames(raw string) string {
	var b strings.Builder
	inRun := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			b.WriteByte(c)
			inRun = false
		} else if !inRun {
			b.WriteByte(' ')
			inRun = true
		}
	}
	cleaned := jsstr.Trim(b.String())
	if cleaned == "" {
		return ""
	}
	return " " + cleaned
}

var series = [...]string{"#4c78a8", "#f58518", "#54a24b", "#e45756", "#72b7b2", "#eeca3b", "#b279a2", "#9d755d"}

// SeriesColor is the color for the nth data series in a chart, cycling through eight tokens.
func SeriesColor(index int) string {
	k := ((index % 8) + 8) % 8
	return "var(--pele-series-" + strconv.Itoa(k+1) + "," + series[k] + ")"
}
