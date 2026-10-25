// Package preprocess is upstream's src/preprocess.ts: line endings, inline HTML attributes,
// front matter, %%{ }%% directives and %% comments, and the entity encoding Mermaid applies
// before parsing.
package preprocess

import (
	"regexp"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsjson"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/yaml"
)

// Preprocessed is the text a diagram type parses, with what was read from around it.
type Preprocessed struct {
	Text string
	// Title is the front matter title; nil when there is none.
	Title  *string
	Config *jsobj.Object
	// WithComments is the text before its comment lines are removed; LineOffset is the
	// number of front matter lines before it.
	WithComments string
	LineOffset   int
}

var unsafeKeys = map[string]bool{"__proto__": true, "constructor": true, "prototype": true}

func isLineTerminator(r rune) bool { return jsstr.IsLineEnd(r) }

// Steps records the edits preprocessing makes, one []perr.Edit per step, so that an offset
// into its output can be found in its input. A nil *Steps records nothing.
type Steps [][]perr.Edit

// push records a step whose edits are given as byte offsets into text, with lengths in
// UTF-16 code units.
func (s *Steps) push(text string, edits [][3]int) {
	if s == nil {
		return
	}
	out := make([]perr.Edit, len(edits))
	units, at := 0, 0
	conv := func(b int) int {
		units += jsstr.Len(text[at:b])
		at = b
		return units
	}
	for i, e := range edits {
		out[i] = perr.Edit{From: conv(e[0]), To: conv(e[1]), Length: e[2]}
	}
	*s = append(*s, out)
}

// NormalizeLineBreaks turns \r\n and \r into \n.
func NormalizeLineBreaks(text string) string {
	if strings.Contains(text, "\r") {
		return reCRLF.ReplaceAllString(text, "\n")
	}
	return text
}

// CleanupComments removes %% comment lines, as Mermaid's /^\s*%%(?!{)[^\n]+\n?/gm does,
// then leading whitespace.
func CleanupComments(text string, steps *Steps) string {
	if strings.Contains(text, "%%") {
		text = removeComments(text, steps)
	}
	trimmed := jsstr.TrimStart(text)
	steps.push(text, [][3]int{{0, len(text) - len(trimmed), 0}})
	return trimmed
}

func removeComments(text string, steps *Steps) string {
	var out strings.Builder
	var edits [][3]int
	last := 0
	for p := 0; p < len(text); {
		if p == 0 || prevIsLineTerminator(text, p) {
			if end := commentAt(text, p); end > p {
				out.WriteString(text[last:p])
				edits = append(edits, [3]int{p, end, 0})
				last = end
				p = end
				continue
			}
		}
		_, n := jsstr.Decode(text, p)
		p += n
	}
	steps.push(text, edits)
	if last == 0 {
		return text
	}
	return out.String() + text[last:]
}

func prevIsLineTerminator(text string, p int) bool {
	r, _ := jsstr.DecodeLast(text, p)
	return isLineTerminator(r)
}

// commentAt returns the end of a comment match starting at p, or -1.
func commentAt(text string, p int) int {
	i := skipSpace(text, p)
	if !strings.HasPrefix(text[i:], "%%") {
		return -1
	}
	i += 2
	if i < len(text) && text[i] == '{' {
		return -1
	}
	j := i
	for j < len(text) && text[j] != '\n' {
		j++
	}
	if j == i {
		return -1
	}
	if j < len(text) {
		j++
	}
	return j
}

func skipSpace(text string, i int) int {
	for i < len(text) {
		r, n := jsstr.Decode(text, i)
		if !jsstr.IsSpace(r) {
			break
		}
		i += n
	}
	return i
}

func isLineBreak(r rune) bool { return r == '\n' || r == '\r' }

type frontMatter struct {
	indent string
	body   string
	length int
}

// lastIndexAtOrBefore is JS lastIndexOf(needle, from) for from >= 0.
func lastIndexAtOrBefore(text, needle string, from int) int {
	return strings.LastIndex(text[:min(len(text), from+len(needle))], needle)
}

func indexFrom(text, needle string, from int) int {
	if from > len(text) {
		return -1
	}
	i := strings.Index(text[from:], needle)
	if i == -1 {
		return -1
	}
	return from + i
}

// matchFrontMatter finds a --- fenced YAML block at the start of the text the way Mermaid's
// front matter regex does.
func matchFrontMatter(text string) *frontMatter {
	cursor := 0
	for cursor < len(text) {
		r, n := jsstr.Decode(text, cursor)
		if !jsstr.IsSpace(r) || isLineBreak(r) {
			break
		}
		cursor += n
	}
	indent := text[:cursor]
	if !strings.HasPrefix(text[cursor:], "---") {
		return nil
	}

	var openingEnds []int
	for i := cursor + 3; i < len(text); {
		r, n := jsstr.Decode(text, i)
		if !jsstr.IsSpace(r) {
			break
		}
		if isLineBreak(r) {
			openingEnds = append(openingEnds, i)
		}
		i += n
	}
	if len(openingEnds) == 0 {
		return nil
	}

	fence := indent + "---"
	closingEnd := func(i int) int {
		if !(text[i] == '\n' || text[i] == '\r') || !strings.HasPrefix(text[i+1:], fence) {
			return -1
		}
		last := -1
		for j := i + 1 + len(fence); j < len(text); {
			r, n := jsstr.Decode(text, j)
			if !jsstr.IsSpace(r) {
				break
			}
			if isLineBreak(r) {
				last = j
			}
			j += n
		}
		return last
	}

	needle := "\n" + fence
	lastFence := -1
	for i := strings.LastIndex(text, needle); i > openingEnds[0]; i = lastIndexAtOrBefore(text, needle, i-1) {
		if closingEnd(i) != -1 {
			lastFence = i
			break
		}
	}
	if lastFence == -1 {
		return nil
	}

	openingEnd := openingEnds[0]
	for _, candidate := range openingEnds {
		if candidate < lastFence {
			openingEnd = candidate
		}
	}

	bodyStart := openingEnd + 1
	firstAt := 0
	if bodyStart > 0 {
		firstAt = bodyStart - 1
	}
	for i := indexFrom(text, needle, firstAt); i != -1 && i <= lastFence; i = indexFrom(text, needle, i+1) {
		if i < bodyStart {
			continue
		}
		if end := closingEnd(i); end != -1 {
			return &frontMatter{indent: indent, body: text[bodyStart:i], length: end + 1}
		}
	}
	return nil
}

func isRecord(v any) (*jsobj.Object, bool) {
	o, ok := v.(*jsobj.Object)
	return o, ok
}

// Merge copies source into target, merging nested objects, as upstream's merge() does.
func Merge(target, source *jsobj.Object) *jsobj.Object {
	for _, key := range source.Keys() {
		if unsafeKeys[key] {
			continue
		}
		value, _ := source.Get(key)
		current, _ := target.Get(key)
		valueObj, valueIsObj := isRecord(value)
		currentObj, currentIsObj := isRecord(current)
		switch {
		case valueIsObj && currentIsObj:
			Merge(currentObj, valueObj)
		case valueIsObj:
			target.Set(key, Merge(jsobj.New(), valueObj))
		default:
			target.Set(key, value)
		}
	}
	return target
}

var (
	reCRLF    = regexp.MustCompile(`\r\n?`)
	reHTMLTag = regexp.MustCompile(`<(\w+)([^>]*)>`)
	reAttr    = regexp.MustCompile(`="([^"]*)"`)
)

// Preprocess prepares source for detection and parsing. steps maps the source, with its line
// breaks normalized, to WithComments.
func Preprocess(source string, steps *Steps) Preprocessed {
	text := NormalizeLineBreaks(source)
	// A tag needs a closing bracket, so nothing after the last one can match.
	lastClose := -1
	if strings.Contains(text, "<") {
		lastClose = strings.LastIndex(text, ">")
	}
	if lastClose > 0 {
		head := reHTMLTag.ReplaceAllStringFunc(text[:lastClose+1], func(m string) string {
			sub := reHTMLTag.FindStringSubmatch(m)
			return "<" + sub[1] + reAttr.ReplaceAllString(sub[2], "='${1}'") + ">"
		})
		text = head + text[lastClose+1:]
	}

	config := jsobj.New()
	var title *string
	lineOffset := 0

	var front *frontMatter
	if strings.Contains(text, "---") {
		front = matchFrontMatter(text)
	}
	if front != nil {
		body := front.body
		if front.indent != "" {
			lines := strings.Split(front.body, "\n")
			for i, line := range lines {
				lines[i] = strings.TrimPrefix(line, front.indent)
			}
			body = strings.Join(lines, "\n")
		}
		parsed, err := yaml.Parse(body)
		if err != nil {
			perr.Throw(perr.New("Front matter is not valid YAML. "+err.Error(), perr.Syntax))
		}
		if record, ok := isRecord(parsed); ok {
			if t, _ := record.Get("title"); jsobj.Truthy(t) {
				s := jsobj.ToString(t)
				title = &s
			}
			if c, _ := record.Get("config"); c != nil {
				if cfg, ok := isRecord(c); ok {
					Merge(config, cfg)
				}
			}
			if mode, _ := record.Get("displayMode"); jsobj.Truthy(mode) {
				current, _ := config.Get("gantt")
				gantt, ok := isRecord(current)
				if !ok {
					gantt = jsobj.New()
					config.Set("gantt", gantt)
				}
				gantt.Set("displayMode", jsobj.ToString(mode))
			}
		}
		for i := strings.IndexByte(text, '\n'); i != -1 && i < front.length; i = indexFrom(text, "\n", i+1) {
			lineOffset++
		}
		steps.push(text, [][3]int{{0, front.length, 0}})
		text = text[front.length:]
	}

	text = directives(text, config, steps)
	return Preprocessed{Text: CleanupComments(text, nil), Title: title, Config: config, WithComments: text, LineOffset: lineOffset}
}

type directive struct {
	end      int
	typ      string
	args     string
	argsSeen bool
}

func wordEnd(s string, i int) int {
	for i < len(s) && isWordByte(s[i]) {
		i++
	}
	return i
}

func isWordByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

// directiveAt matches Mermaid's directive pattern at p:
// /%{2}{\s*(?:(\w+)\s*:|(\w+))\s*(?:(\w+)|((?:(?!}%{2}).|\r?\n)*))?\s*(?:}%{2})?/gi
func directiveAt(s string, p int) (directive, bool) {
	if !strings.HasPrefix(s[p:], "%%{") {
		return directive{}, false
	}
	i := skipSpace(s, p+3)
	w := wordEnd(s, i)
	if w == i {
		return directive{}, false
	}
	var d directive
	if j := skipSpace(s, w); j < len(s) && s[j] == ':' {
		d.typ = s[i:w]
		i = j + 1
	} else {
		d.typ = s[i:w]
		i = w
	}
	i = skipSpace(s, i)
	if w := wordEnd(s, i); w > i {
		i = w
	} else {
		start := i
		for i < len(s) && !strings.HasPrefix(s[i:], "}%%") {
			r, n := jsstr.Decode(s, i)
			if r == '\r' && i+1 < len(s) && s[i+1] == '\n' {
				i += 2
				continue
			}
			if r == '\n' {
				i++
				continue
			}
			if isLineTerminator(r) {
				break
			}
			i += n
		}
		d.args = s[start:i]
		d.argsSeen = true
	}
	i = skipSpace(s, i)
	if strings.HasPrefix(s[i:], "}%%") {
		i += 3
	}
	d.end = i
	return d, true
}

// eachDirective calls fn for each directive match in s, scanning as a global regex does.
func eachDirective(s string, fn func(start int, d directive)) {
	for p := 0; p < len(s); {
		at := strings.Index(s[p:], "%%{")
		if at == -1 {
			return
		}
		p += at
		d, ok := directiveAt(s, p)
		if !ok {
			p++
			continue
		}
		fn(p, d)
		p = d.end
	}
}

func directives(text string, config *jsobj.Object, steps *Steps) string {
	if !strings.Contains(text, "%%{") {
		return text
	}
	normalized := strings.ReplaceAll(jsstr.Trim(text), "'", "\"")
	eachDirective(normalized, func(_ int, d directive) {
		switch {
		case d.typ == "wrap":
			config.Set("wrap", true)
		case (d.typ == "init" || d.typ == "initialize") && d.argsSeen && d.args != "":
			// Mermaid ignores an init directive whose body is not valid JSON.
			if args, err := jsjson.Parse(jsstr.Trim(d.args)); err == nil {
				if obj, ok := isRecord(args); ok {
					Merge(config, obj)
				}
			}
		}
	})
	var out strings.Builder
	var edits [][3]int
	last := 0
	eachDirective(text, func(start int, d directive) {
		out.WriteString(text[last:start])
		edits = append(edits, [3]int{start, d.end, 0})
		last = d.end
	})
	steps.push(text, edits)
	return out.String() + text[last:]
}
