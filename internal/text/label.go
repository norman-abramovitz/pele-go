package text

import (
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// Span is a run of text in one style, an icon, or typeset math.
type Span struct {
	Text  string
	Style Style
	// Icon is the icon's name, or "" for text.
	Icon  string
	Math  *MathMarkup
	Width float64
}

// Label is measured, wrapped text.
type Label struct {
	Lines      [][]Span
	Widths     []float64
	Width      float64
	Height     float64
	Size       float64
	LineHeight float64
	// Heights is set only when math needs extra line height.
	Heights []float64
}

// LineHeight is the line height as a multiple of the font size.
const LineHeight = 1.5

// Lines of one wrapped label sit closer, so the label reads as one block.
const wrappedLineHeight = 1.3

// tagStyle is upstream's TAG_STYLES.
func tagStyle(tag string) Style {
	switch tag {
	case "b", "strong":
		return Bold
	case "i", "em", "cite", "dfn", "var":
		return Italic
	case "code", "kbd", "samp", "tt":
		return Mono
	case "sub":
		return Sub
	case "sup":
		return Sup
	case "u", "ins":
		return Underline
	}
	return 0
}

const shift = Sub | Sup

var htmlTags = sync.OnceValue(func() map[string]bool {
	set := map[string]bool{}
	for _, tag := range strings.Split("a abbr address article aside audio b bdi bdo big blockquote body br button caption center cite code col "+
		"colgroup data dd del details dfn dialog div dl dt em embed fieldset figcaption figure font footer form h1 h2 "+
		"h3 h4 h5 h6 head header hr html i iframe img input ins kbd label legend li link main map mark meta meter nav "+
		"noscript object ol optgroup option output p picture pre progress q rp rt ruby s samp script section select "+
		"small source span strike strong style sub summary sup table tbody td template textarea tfoot th thead time "+
		"title tr track tt u ul var video wbr", " ") {
		set[tag] = true
	}
	return set
})

var empty = Label{}

// Unique placeholders protect TeX from markup parsing and keep expressions aligned when tags
// are removed.
const (
	mark     = 0xe000
	maxMarks = 0x1900
)

type marks struct {
	tex  []string
	math []*MathMarkup
}

// isMark is /[\ue000-\uf8ff]/.
func isMark(r rune) bool { return r >= 0xe000 && r <= 0xf8ff }

func hasMark(s string) bool {
	for i := 0; i < len(s); {
		r, n := jsstr.Decode(s, i)
		if isMark(r) {
			return true
		}
		i += n
	}
	return false
}

// mathAt matches /\$\$(.+?)\$\$/ at i and returns the end of the TeX and of the match.
func mathAt(s string, i int) (int, int, bool) {
	if !strings.HasPrefix(s[i:], "$$") {
		return 0, 0, false
	}
	p := i + 2
	for q := p; q < len(s); {
		r, n := jsstr.Decode(s, q)
		if jsstr.IsLineEnd(r) {
			return 0, 0, false
		}
		q += n
		if strings.HasPrefix(s[q:], "$$") {
			return q, q + 2, true
		}
	}
	return 0, 0, false
}

// markMath replaces each $$...$$ in raw with a mark, recording its TeX in found.
func markMath(raw string, found *marks) string {
	var out strings.Builder
	last := 0
	for i := 0; i < len(raw); {
		end, stop, ok := mathAt(raw, i)
		if !ok {
			i++
			continue
		}
		if len(found.tex) == maxMarks {
			i = stop
			continue
		}
		found.tex = append(found.tex, raw[i+2:end])
		out.WriteString(raw[last:i])
		out.WriteRune(rune(mark + len(found.tex) - 1))
		last = stop
		i = stop
	}
	return out.String() + raw[last:]
}

// isCJK reports whether r is a Chinese or Japanese character, between any two of which a
// line may break. Hangul is left out: Korean puts spaces between words.
func isCJK(r rune) bool {
	return (r >= 0x2e80 && r <= 0x9fff) || (r >= 0xf900 && r <= 0xfaff) || (r >= 0xff00 && r <= 0xff60)
}

const (
	closePunct = "、。，．：；！？）］｝」』】〕〉》〙〗ー…‥・ゝゞぁぃぅぇぉっゃゅょゎァィゥェォッャュョヮ,.:;!?)]}"
	openPunct  = "（［｛「『【〔〈《〘〖([{"
)

func isClose(r rune) bool { return strings.ContainsRune(closePunct, r) }
func isOpen(r rune) bool  { return strings.ContainsRune(openPunct, r) }

// isHint is a break hint: a zero-width space, and a soft hyphen, which shows as '-' where a
// line breaks.
func isHint(r rune) bool { return r == 0x200b || r == 0xad }

const softHyphen = "\u00ad"

func hasHint(s string) bool { return strings.Contains(s, "\u200b") || strings.Contains(s, softHyphen) }

func hasCJK(s string) bool {
	for i := 0; i < len(s); {
		r, n := jsstr.Decode(s, i)
		if isCJK(r) {
			return true
		}
		i += n
	}
	return false
}

// hasMarkup is /[<\\\n*_]|fa[bklrs]?:fa-/.
func hasMarkup(s string) bool {
	if strings.ContainsAny(s, "<\\\n*_") {
		return true
	}
	for _, p := range []string{"fa:fa-", "fab:fa-", "fak:fa-", "fal:fa-", "far:fa-", "fas:fa-"} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func runeAt(s string, i int) (rune, int) {
	if i >= len(s) {
		return -1, 0
	}
	return jsstr.Decode(s, i)
}

func skipWhile(s string, i int, ok func(rune) bool) int {
	for i < len(s) {
		r, n := jsstr.Decode(s, i)
		if !ok(r) {
			break
		}
		i += n
	}
	return i
}

// pieceAt matches one piece of the wrap pattern at i and returns its end:
// \s*(?:[OPEN]*[CJK][CLOSE]*|[^\sCJK HINT]+[HINT]*[CLOSE]*|[HINT]+)\s*|\s+
// The leading \s* backs off when what follows does not match, which matters for U+3000:
// it is both whitespace and a CJK character.
func pieceAt(s string, i int) int {
	var spaces []int
	for p := i; ; {
		spaces = append(spaces, p)
		r, n := runeAt(s, p)
		if n == 0 || !jsstr.IsSpace(r) {
			break
		}
		p += n
	}
	for k := len(spaces) - 1; k >= 0; k-- {
		if end := wordAt(s, spaces[k]); end != -1 {
			return skipWhile(s, end, jsstr.IsSpace)
		}
	}
	if end := skipWhile(s, i, jsstr.IsSpace); end > i {
		return end
	}
	return -1
}

// wordAt matches [OPEN]*[CJK][CLOSE]*, [^\sCJK HINT]+[HINT]*[CLOSE]* or [HINT]+ at p.
func wordAt(s string, p int) int {
	var opens []int
	for q := p; ; {
		opens = append(opens, q)
		r, n := runeAt(s, q)
		if n == 0 || !isOpen(r) {
			break
		}
		q += n
	}
	for k := len(opens) - 1; k >= 0; k-- {
		if r, n := runeAt(s, opens[k]); n > 0 && isCJK(r) {
			return skipWhile(s, opens[k]+n, isClose)
		}
	}
	end := skipWhile(s, p, func(r rune) bool { return !jsstr.IsSpace(r) && !isCJK(r) && !isHint(r) })
	if end == p {
		if end = skipWhile(s, p, isHint); end == p {
			return -1
		}
		return end
	}
	return skipWhile(s, skipWhile(s, end, isHint), isClose)
}

// pieces splits text where a line may wrap, as text.match(RE_PIECE) does.
func pieces(text string) []string {
	var out []string
	for i := 0; i < len(text); {
		end := pieceAt(text, i)
		if end <= i {
			_, n := jsstr.Decode(text, i)
			i += n
			continue
		}
		out = append(out, text[i:end])
		i = end
	}
	return out
}

func dedent(text string) string {
	lines := strings.Split(text, "\n")
	minIndent := math.MaxInt
	for _, line := range lines {
		if jsstr.Trim(line) == "" {
			continue
		}
		if indent := jsstr.Len(line) - jsstr.Len(jsstr.TrimStart(line)); indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent == math.MaxInt || minIndent == 0 {
		return text
	}
	for i, line := range lines {
		indent := jsstr.Len(line) - jsstr.Len(jsstr.TrimStart(line))
		lines[i] = jsstr.Slice(line, min(minIndent, indent), jsstr.Len(line))
	}
	return strings.Join(lines, "\n")
}

func plain(span *Span) bool {
	return span.Icon == "" && span.Math == nil
}

func pushText(line *[]Span, text string, style Style, found *marks) {
	if text == "" {
		return
	}
	if found != nil && hasMark(text) {
		last := 0
		for i := 0; i < len(text); {
			r, n := jsstr.Decode(text, i)
			if !isMark(r) {
				i += n
				continue
			}
			if last < i {
				pushPlain(line, text[last:i], style)
			}
			at := int(r) - mark
			if math := found.math[at]; math != nil {
				*line = append(*line, Span{Style: style, Math: math, Width: math.Width})
			} else {
				pushPlain(line, "$$"+found.tex[at]+"$$", style)
			}
			i += n
			last = i
		}
		if last < len(text) {
			pushPlain(line, text[last:], style)
		}
		return
	}
	pushPlain(line, text, style)
}

func pushPlain(line *[]Span, text string, style Style) {
	decoded := DecodeEntities(text)
	if n := len(*line); n > 0 && plain(&(*line)[n-1]) && (*line)[n-1].Style == style {
		(*line)[n-1].Text += decoded
		return
	}
	*line = append(*line, Span{Text: decoded, Style: style})
}

// iconAt matches /(fa[bklrs]?):fa-([\w-]+)/ at i and returns the icon name and match end.
func iconAt(s string, i int) (string, int, bool) {
	if !strings.HasPrefix(s[i:], "fa") {
		return "", 0, false
	}
	p := i + 2
	try := func(q int) (string, int, bool) {
		if !strings.HasPrefix(s[q:], ":fa-") {
			return "", 0, false
		}
		start := q + 4
		end := start
		for end < len(s) && (isWordByte(s[end]) || s[end] == '-') {
			end++
		}
		if end == start {
			return "", 0, false
		}
		return s[i:q] + ":fa-" + s[start:end], end, true
	}
	if p < len(s) && strings.IndexByte("bklrs", s[p]) >= 0 {
		if name, end, ok := try(p + 1); ok {
			return name, end, true
		}
	}
	return try(p)
}

func pushIcons(line *[]Span, text string, style Style, icons *Icons, found *marks) {
	if !strings.Contains(text, ":fa-") {
		pushText(line, text, style, found)
		return
	}
	last := 0
	for i := 0; i < len(text); {
		icon, end, ok := iconAt(text, i)
		if !ok {
			i++
			continue
		}
		pushText(line, text[last:i], style, found)
		last = end
		i = end
		switch {
		case icons.Markup(icon) != "":
			*line = append(*line, Span{Style: style, Icon: icon})
		case last < len(text) && text[last] == ' ':
			// An icon the host does not have leaves no gap, nor the space that set it off from the text.
			last++
		case last == len(text):
			if n := len(*line); n > 0 && plain(&(*line)[n-1]) {
				(*line)[n-1].Text = jsstr.TrimEnd((*line)[n-1].Text)
			}
		}
	}
	pushText(line, text[last:], style, found)
}

// tagAt matches /<\/?([a-zA-Z][\w-]*)(?:\s[^<>]*)?\/?>/ at i and returns the tag name
// and match end.
func tagAt(s string, i int) (string, int, bool) {
	p := i + 1
	if p < len(s) && s[p] == '/' {
		p++
	}
	if p >= len(s) || !(s[p] >= 'a' && s[p] <= 'z' || s[p] >= 'A' && s[p] <= 'Z') {
		return "", 0, false
	}
	nameStart := p
	p++
	for p < len(s) && (isWordByte(s[p]) || s[p] == '-') {
		p++
	}
	name := s[nameStart:p]
	if r, n := runeAt(s, p); n > 0 && jsstr.IsSpace(r) {
		q := p + n
		for q < len(s) && s[q] != '<' && s[q] != '>' {
			q++
		}
		if q < len(s) && s[q] == '>' {
			return name, q + 1, true
		}
		return "", 0, false
	}
	if p < len(s) && s[p] == '/' {
		p++
	}
	if p < len(s) && s[p] == '>' {
		return name, p + 1, true
	}
	return "", 0, false
}

func isWordRune(r rune) bool {
	return r < 128 && r >= 0 && isWordByte(byte(r))
}

type openTags struct {
	style Style
	// Sub/sup stack; only the innermost tag shifts text.
	shifts []Style
}

// inline splits inline HTML and, for markdown labels, emphasis markers into styled spans.
func inline(line *[]Span, source string, markdown bool, base Style, open *openTags, icons *Icons, found *marks) {
	style := open.style
	var marked Style
	var text strings.Builder
	flush := func() {
		var top Style
		if k := len(open.shifts); k > 0 {
			top = open.shifts[k-1]
		}
		pushIcons(line, text.String(), style|top, icons, found)
		text.Reset()
	}
	n := len(source)
	for i := 0; i < n; {
		ch := source[i]
		if ch == '<' {
			if name, end, ok := tagAt(source, i); ok {
				tag := strings.ToLower(name)
				closing := i+1 < n && source[i+1] == '/'
				bit := tagStyle(tag)
				if bit&shift != 0 {
					flush()
					at := -1
					for k := len(open.shifts) - 1; k >= 0; k-- {
						if open.shifts[k] == bit {
							at = k
							break
						}
					}
					if !closing {
						open.shifts = append(open.shifts, bit)
					} else if at != -1 {
						open.shifts = slices.Delete(open.shifts, at, at+1)
					}
				} else if bit != 0 {
					flush()
					if closing {
						open.style = (open.style &^ bit) | (base & bit)
						style = (style &^ bit) | (base & bit)
					} else {
						open.style |= bit
						style |= bit
					}
				}
				// Other HTML tags are dropped and their text kept. Anything else shaped like a
				// tag, such as the type in List<T>, is text.
				if bit != 0 || htmlTags()[tag] {
					i = end
					continue
				}
			}
		} else if markdown && (ch == '*' || ch == '_') {
			double := i+1 < n && source[i+1] == ch
			bit, size := Italic, 1
			if double {
				bit, size = Bold, 2
			}
			opening := marked&bit == 0
			prev := rune(' ')
			if i > 0 {
				prev, _ = jsstr.DecodeLast(source, i)
			}
			next := rune(' ')
			if i+size < n {
				next, _ = jsstr.Decode(source, i+size)
			}
			marker := source[i : i+size]
			canOpen := opening && !jsstr.IsSpace(next) && strings.Contains(source[i+size:], marker)
			canClose := !opening && !jsstr.IsSpace(prev)
			wordy := ch == '_' && isWordRune(prev) && isWordRune(next)
			if (canOpen || canClose) && !wordy {
				flush()
				if canOpen {
					marked |= bit
					style |= bit
				} else {
					marked &^= bit
					style = (style &^ bit) | (base & bit)
				}
				i += size
				continue
			}
		}
		_, size := jsstr.Decode(source, i)
		text.WriteString(source[i : i+size])
		i += size
	}
	flush()
}

func measureSpans(line []Span, m Measurer, size float64) float64 {
	total := 0.0
	for i := range line {
		if line[i].Icon != "" {
			line[i].Width = size * 1.25
		} else if line[i].Math != nil {
			line[i].Width = line[i].Math.Width
		} else {
			line[i].Width = m.Width(line[i].Text, size, line[i].Style)
		}
		total += line[i].Width
	}
	return total
}

func wrap(line []Span, m Measurer, size, maxWidth float64, out *[][]Span, widths *[]float64) {
	var current []Span
	width := 0.0
	commit := func(hyphen bool) {
		if n := len(current); n > 0 && plain(&current[n-1]) {
			last := &current[n-1]
			trimmed := jsstr.TrimEnd(last.Text)
			if hyphen && strings.HasSuffix(last.Text, softHyphen) {
				trimmed = strings.TrimSuffix(trimmed, softHyphen) + "-"
			}
			if trimmed != last.Text {
				last.Text = trimmed
				w := m.Width(trimmed, size, last.Style)
				width += w - last.Width
				last.Width = w
			}
		}
		*out = append(*out, current)
		*widths = append(*widths, width)
		current = nil
		width = 0
	}
	for si, span := range line {
		if span.Icon != "" || span.Math != nil {
			w := float64(size * 1.25)
			if span.Math != nil {
				w = span.Math.Width
			}
			if width > 0 && width+w > maxWidth {
				commit(false)
			}
			span.Width = w
			current = append(current, span)
			width += w
			continue
		}
		parts := pieces(span.Text)
		// A piece ending in a soft hyphen fits if its '-' does, or if the rest of the word does,
		// which hides the hyphen. The line only grows along a word, so no later hyphen in it fits
		// either, and the remaining width of each word makes this one check.
		soft := strings.Contains(span.Text, softHyphen)
		n := 0
		if soft {
			n = len(parts)
		}
		own := make([]float64, n)
		rest := make([]float64, n+1)
		for q := n - 1; q >= 0; q-- {
			// A soft hyphen before a space is no break of its own: the space is.
			shy := strings.HasSuffix(parts[q], softHyphen)
			end := jsstr.TrimEnd(parts[q])
			if shy {
				end = strings.TrimSuffix(end, softHyphen)
			}
			own[q] = m.Width(end, size, span.Style)
			switch {
			case !shy:
				rest[q] = own[q]
			case q == n-1:
				// Word width beyond this span is unknown.
				if si == len(line)-1 {
					rest[q] = own[q]
				} else {
					rest[q] = math.Inf(1)
				}
			default:
				rest[q] = own[q] + rest[q+1]
			}
		}
		for p, part := range parts {
			var partWidth float64
			if soft {
				partWidth = own[p]
			} else {
				partWidth = m.Width(jsstr.TrimEnd(part), size, span.Style)
			}
			var fits bool
			if !soft || !strings.HasSuffix(part, softHyphen) {
				fits = width+partWidth <= maxWidth
			} else {
				fits = width+m.Width(strings.TrimSuffix(part, softHyphen)+"-", size, span.Style) <= maxWidth || width+rest[p] <= maxWidth
			}
			if width > 0 && !fits {
				// Breaking at whitespace leaves a preceding soft hyphen hidden.
				commit(jsstr.TrimStart(part) == part)
				part = jsstr.TrimStart(part)
			}
			if k := len(current); k > 0 && plain(&current[k-1]) && current[k-1].Style == span.Style {
				last := &current[k-1]
				last.Text += part
				// Remeasuring for each zero-width piece would take quadratic time.
				if partWidth > 0 || jsstr.TrimEnd(part) != part {
					before := last.Width
					last.Width = m.Width(last.Text, size, last.Style)
					width += last.Width - before
				}
			} else {
				w := m.Width(part, size, span.Style)
				current = append(current, Span{Text: part, Style: span.Style, Width: w})
				width += w
			}
		}
	}
	if len(current) > 0 || len(*out) == 0 {
		commit(false)
	}
}

// splitBreaks splits on /<br\s*\/?>|\\n|\n/gi.
func splitBreaks(s string) []string {
	var parts []string
	last := 0
	for i := 0; i < len(s); {
		end := -1
		switch {
		case s[i] == '\n':
			end = i + 1
		case s[i] == '\\' && i+1 < len(s) && (s[i+1] == 'n' || s[i+1] == 'N'):
			end = i + 2
		case s[i] == '<' && i+2 < len(s) && (s[i+1] == 'b' || s[i+1] == 'B') && (s[i+2] == 'r' || s[i+2] == 'R'):
			p := skipWhile(s, i+3, jsstr.IsSpace)
			if p < len(s) && s[p] == '/' {
				p++
			}
			if p < len(s) && s[p] == '>' {
				end = p + 1
			}
		}
		if end == -1 {
			i++
			continue
		}
		parts = append(parts, s[last:i])
		last = end
		i = end
	}
	return append(parts, s[last:])
}

// Layout measures raw and wraps it to maxWidth. base is the style every span starts in.
func Layout(raw string, markdown bool, m Measurer, size, maxWidth float64, base Style, icons *Icons) Label {
	if raw == "" {
		return empty
	}
	lineHeight := jsmath.Round(size * LineHeight)

	if icons.MathActive() && strings.Contains(raw, "$$") && !hasMark(raw) {
		found := &marks{}
		marked := markMath(raw, found)
		if len(found.tex) > 0 {
			for _, tex := range found.tex {
				found.math = append(found.math, icons.typeset(DecodeEntities(tex), size))
			}
			return layoutMarkup(marked, markdown, m, size, maxWidth, base, lineHeight, icons, found)
		}
	}

	if !hasMarkup(raw) && !strings.Contains(raw, "&") && !strings.Contains(raw, "ﬂ") {
		text := jsstr.Trim(raw)
		width := m.Width(text, size, base)
		if width <= maxWidth || !(strings.Contains(text, " ") || hasCJK(text) || hasHint(text)) {
			return Label{
				Lines:      [][]Span{{{Text: text, Style: base, Width: width}}},
				Widths:     []float64{width},
				Width:      width,
				Height:     lineHeight,
				Size:       size,
				LineHeight: lineHeight,
			}
		}
		var lines [][]Span
		var widths []float64
		wrap([]Span{{Text: text, Style: base, Width: width}}, m, size, maxWidth, &lines, &widths)
		widest := 0.0
		for _, w := range widths {
			if w > widest {
				widest = w
			}
		}
		snug := snugLineHeight(len(lines), size, lineHeight)
		return Label{Lines: lines, Widths: widths, Width: widest, Height: float64(len(lines)) * snug, Size: size, LineHeight: snug}
	}
	return layoutMarkup(raw, markdown, m, size, maxWidth, base, lineHeight, icons, nil)
}

func layoutMarkup(raw string, markdown bool, m Measurer, size, maxWidth float64, base Style, lineHeight float64, icons *Icons, found *marks) Label {
	label := layoutLines(raw, markdown, m, size, maxWidth, base, lineHeight, icons, found)
	if found != nil {
		return WithHeights(label)
	}
	return label
}

func layoutLines(raw string, markdown bool, m Measurer, size, maxWidth float64, base Style, lineHeight float64, icons *Icons, found *marks) Label {
	source := jsstr.Trim(raw)
	if markdown {
		source = jsstr.Trim(dedent(strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(raw)))
	}
	var lines [][]Span
	var widths []float64
	// HTML tags can span explicit breaks; Markdown emphasis stays local to each line.
	open := &openTags{style: base}
	for _, part := range splitBreaks(source) {
		var line []Span
		inline(&line, jsstr.Trim(part), markdown, base, open, icons, found)
		w := measureSpans(line, m, size)
		if w > maxWidth {
			wrap(line, m, size, maxWidth, &lines, &widths)
		} else {
			lines = append(lines, line)
			widths = append(widths, w)
		}
	}
	width := 0.0
	for _, w := range widths {
		if w > width {
			width = w
		}
	}
	snug := snugLineHeight(len(lines), size, lineHeight)
	return Label{Lines: lines, Widths: widths, Width: width, Height: float64(len(lines)) * snug, Size: size, LineHeight: snug}
}

func snugLineHeight(lines int, size, lineHeight float64) float64 {
	if lines > 1 {
		return jsmath.Round(size * wrappedLineHeight)
	}
	return lineHeight
}

// WithHeights sets the label's line heights for the math it holds.
func WithHeights(label Label) Label {
	var heights []float64
	for i, line := range label.Lines {
		h := label.LineHeight
		for _, span := range line {
			if span.Math != nil {
				h = max(h, math.Ceil(span.Math.Height+float64(label.Size/4)))
			}
		}
		if h != label.LineHeight && heights == nil {
			heights = make([]float64, len(label.Lines))
			for k := range heights {
				heights[k] = label.LineHeight
			}
		}
		if heights != nil {
			heights[i] = h
		}
	}
	if heights == nil {
		if label.Heights == nil {
			return label
		}
		label.Heights = nil
		label.Height = float64(len(label.Lines)) * label.LineHeight
		return label
	}
	height := 0.0
	for _, h := range heights {
		height += h
	}
	label.Heights = heights
	label.Height = height
	return label
}

// HasMath reports whether a line holds typeset math.
func HasMath(line []Span) bool {
	for _, span := range line {
		if span.Math != nil {
			return true
		}
	}
	return false
}

// LineMiddle is the middle of line i, from the top of the label.
func LineMiddle(label Label, i int) float64 {
	heights := label.Heights
	if heights == nil {
		return float64((float64(i) + 0.5) * label.LineHeight)
	}
	top := 0.0
	for k := 0; k < i; k++ {
		top += heights[k]
	}
	return top + float64(heights[i]/2)
}

// LabelLine is line i of the label on its own.
func LabelLine(label Label, i int) Label {
	width := label.Widths[i]
	height := label.LineHeight
	if label.Heights != nil {
		height = label.Heights[i]
	}
	line := Label{Lines: [][]Span{label.Lines[i]}, Widths: []float64{width}, Width: width, Height: height, Size: label.Size, LineHeight: label.LineHeight}
	if height != label.LineHeight {
		line.Heights = []float64{height}
	}
	return line
}
