package common

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

func isLineBreak(c int) bool { return c == '\n' || c == '\r' }

func skipBlanks(src Src, i int) int {
	for util.IsBlank(src.At(i)) {
		i++
	}
	return i
}

// lineRest matches [^\n\r]*?(?=%%)|[^\n\r]* from i: up to the first %% on the line, or else
// to the end of the line.
func lineRest(src Src, i int) int {
	for j := i; j < len(src) && !isLineBreak(int(src[j])); j++ {
		if src.HasPrefix(j, "%%") {
			return j
		}
	}
	end := i
	for end < len(src) && !isLineBreak(int(src[end])) {
		end++
	}
	return end
}

// AccDescr is /[\t ]*accDescr(?:[\t ]*:([^\n\r]*?(?=%%)|[^\n\r]*)|\s*{([^}]*)})/y.
var AccDescr = &TokenType{Name: "ACC_DESCR", First: "\t a", Match: func(src Src, at int) int {
	i := skipBlanks(src, at)
	if !src.HasPrefix(i, "accDescr") {
		return -1
	}
	i += len("accDescr")
	if j := skipBlanks(src, i); src.At(j) == ':' {
		return lineRest(src, j+1)
	}
	j := i
	for util.IsSpace(src.At(j)) {
		j++
	}
	if src.At(j) != '{' {
		return -1
	}
	if end := src.Index("}", j+1); end != -1 {
		return end + 1
	}
	return -1
}}

// AccTitle is /[\t ]*accTitle[\t ]*:(?:[^\n\r]*?(?=%%)|[^\n\r]*)/y.
var AccTitle = &TokenType{Name: "ACC_TITLE", First: "\t a", Match: func(src Src, at int) int {
	i := skipBlanks(src, at)
	if !src.HasPrefix(i, "accTitle") {
		return -1
	}
	i = skipBlanks(src, i+len("accTitle"))
	if src.At(i) != ':' {
		return -1
	}
	return lineRest(src, i+1)
}}

// Title is /[\t ]*title(?:[\t ][^\n\r]*?(?=%%)|[\t ][^\n\r]*|)/y.
var Title = &TokenType{Name: "TITLE", First: "\t t", Match: func(src Src, at int) int {
	i := skipBlanks(src, at)
	if !src.HasPrefix(i, "title") {
		return -1
	}
	i += len("title")
	if util.IsBlank(src.At(i)) {
		return lineRest(src, i+1)
	}
	return i
}}

// String is /"([^"\\]|\\.)*"|'([^'\\]|\\.)*'/y: a quoted string, in which a backslash takes
// the next character with it unless that ends the line.
var String = &TokenType{Name: "STRING", First: `"'`, Match: func(src Src, at int) int {
	q := src.At(at)
	if q != '"' && q != '\'' {
		return -1
	}
	for i := at + 1; i < len(src); {
		switch c := int(src[i]); {
		case c == q:
			return i + 1
		case c == '\\':
			if next := src.At(i + 1); next == -1 || util.IsLineEnd(next) {
				return -1
			}
			i += 2
		default:
			i++
		}
	}
	return -1
}}

// Newline is /\r?\n/y.
var Newline = &TokenType{Name: "NEWLINE", First: "\r\n", Match: func(src Src, at int) int {
	i := at
	if src.At(i) == '\r' {
		i++
	}
	if src.At(i) == '\n' {
		return i + 1
	}
	return -1
}}

// Whitespace is /[\t ]+/y.
var Whitespace = &TokenType{Name: "WHITESPACE", First: "\t ", Hidden: true, Match: func(src Src, at int) int {
	if end := skipBlanks(src, at); end > at {
		return end
	}
	return -1
}}

// lineEndOrSpace matches (?:\r?\n|(?!\S)) at i: a line break, or nothing before whitespace
// or the end.
func lineEndOrSpace(src Src, i int) int {
	if src.At(i) == '\r' && src.At(i+1) == '\n' {
		return i + 2
	}
	if src.At(i) == '\n' {
		return i + 1
	}
	if i >= len(src) || util.IsSpace(src.At(i)) {
		return i
	}
	return -1
}

// yamlOpener is /---[\t ]*\r?\n/y.
func yamlOpener(src Src, at int) int {
	if !src.HasPrefix(at, "---") {
		return -1
	}
	return Newline.Match(src, skipBlanks(src, at+3))
}

// Yaml is /---[\t ]*\r?\n(?:[\S\s]*?\r?\n)?---(?:\r?\n|(?!\S))/y.
var Yaml = &TokenType{Name: "YAML", First: "-", Hidden: true, Opener: yamlOpener, Match: func(src Src, at int) int {
	body := yamlOpener(src, at)
	if body == -1 {
		return -1
	}
	closeAt := func(i int) int {
		if !src.HasPrefix(i, "---") {
			return -1
		}
		return lineEndOrSpace(src, i+3)
	}
	// The optional body group is greedy, so a closing fence after a line break inside the
	// body is preferred to one straight after the opening line.
	for nl := src.Index("\n", body); nl != -1; nl = src.Index("\n", nl+1) {
		if end := closeAt(nl + 1); end != -1 {
			return end
		}
	}
	return closeAt(body)
}}

// directiveOpener is /[\t ]*%%{/y.
func directiveOpener(src Src, at int) int {
	i := skipBlanks(src, at)
	if !src.HasPrefix(i, "%%{") {
		return -1
	}
	return i + 3
}

// Directive is /[\t ]*%%{[\S\s]*?}%%(?:\r?\n|(?!\S))/y.
var Directive = &TokenType{Name: "DIRECTIVE", First: "\t %", Hidden: true, Opener: directiveOpener, Match: func(src Src, at int) int {
	body := directiveOpener(src, at)
	if body == -1 {
		return -1
	}
	for c := src.Index("}%%", body); c != -1; c = src.Index("}%%", c+1) {
		if end := lineEndOrSpace(src, c+3); end != -1 {
			return end
		}
	}
	return -1
}}

// SingleLineComment is /[\t ]*%%[^\n\r]*/y.
var SingleLineComment = &TokenType{Name: "SINGLE_LINE_COMMENT", First: "\t %", Hidden: true, Match: func(src Src, at int) int {
	i := skipBlanks(src, at)
	if !src.HasPrefix(i, "%%") {
		return -1
	}
	i += 2
	for i < len(src) && !isLineBreak(int(src[i])) {
		i++
	}
	return i
}}

// Keyword matches word where Mermaid ends a keyword: at whitespace, a comment, or the end.
func Keyword(word string) *TokenType {
	return &TokenType{Name: word, First: word[:1], Match: func(src Src, at int) int {
		if !src.HasPrefix(at, word) {
			return -1
		}
		end := at + len(word)
		if src.HasPrefix(end, "%%") || end >= len(src) || util.IsSpace(src.At(end)) {
			return end
		}
		return -1
	}}
}

// single trims text and collapses runs of blanks, as text.trim().replace(/[\t ]{2,}/gm, ' ').
func single(text string) string {
	return collapseRuns(jsstr.Trim(text), func(c byte) bool { return c == ' ' || c == '\t' }, " ")
}

// collapseRuns replaces each run of two or more bytes for which in is true with repl.
func collapseRuns(text string, in func(byte) bool, repl string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if !in(text[i]) {
			b.WriteByte(text[i])
			i++
			continue
		}
		j := i
		for j < len(text) && in(text[j]) {
			j++
		}
		if j-i >= 2 {
			b.WriteString(repl)
		} else {
			b.WriteString(text[i:j])
		}
		i = j
	}
	return b.String()
}

// TitleValue reads the title from a TITLE token: /title([\t ][^\n\r]*|)/ group 1.
func TitleValue(text string) string {
	at := strings.Index(text, "title")
	rest := text[at+len("title"):]
	if rest == "" || !(rest[0] == ' ' || rest[0] == '\t') {
		return single("")
	}
	return single(rest[:lineEndIndex(rest)])
}

func lineEndIndex(s string) int {
	if i := strings.IndexAny(s, "\n\r"); i != -1 {
		return i
	}
	return len(s)
}

// AccTitleValue reads an ACC_TITLE token: /accTitle[\t ]*:([^\n\r]*)/ group 1.
func AccTitleValue(text string) string {
	at := strings.Index(text, "accTitle")
	rest := strings.TrimLeft(text[at+len("accTitle"):], " \t")
	rest = rest[1:] // the colon
	return single(rest[:lineEndIndex(rest)])
}

// AccDescrValue reads an ACC_DESCR token, either form.
func AccDescrValue(text string) string {
	at := strings.Index(text, "accDescr")
	rest := text[at+len("accDescr"):]
	if trimmed := strings.TrimLeft(rest, " \t"); strings.HasPrefix(trimmed, ":") {
		trimmed = trimmed[1:]
		return single(trimmed[:lineEndIndex(trimmed)])
	}
	open := strings.IndexByte(rest, '{')
	body := rest[open+1:]
	body = body[:strings.IndexByte(body, '}')]
	body = removeLeadingSpaceMultiline(body)
	body = removeTrailingSpaceMultiline(body)
	body = collapseRuns(body, func(c byte) bool { return c == ' ' || c == '\t' }, " ")
	return collapseRuns(body, func(c byte) bool { return c == '\n' || c == '\r' }, "\n")
}

func isLineStart(s string, i int) bool {
	if i == 0 {
		return true
	}
	r, _ := jsstr.DecodeLast(s, i)
	return jsstr.IsLineEnd(r)
}

func skipJSSpace(s string, i int) int {
	for i < len(s) {
		r, n := jsstr.Decode(s, i)
		if !jsstr.IsSpace(r) {
			break
		}
		i += n
	}
	return i
}

// removeLeadingSpaceMultiline is s.replace(/^\s*/gm, ”). \s* can run over line breaks,
// so blank lines go with the indent after them.
func removeLeadingSpaceMultiline(s string) string {
	var b strings.Builder
	last := 0
	for i := 0; i <= len(s); {
		if isLineStart(s, i) {
			if end := skipJSSpace(s, i); end > i {
				b.WriteString(s[last:i])
				last = end
				i = end
				continue
			}
		}
		if i == len(s) {
			break
		}
		_, n := jsstr.Decode(s, i)
		i += n
	}
	return b.String() + s[last:]
}

// removeTrailingSpaceMultiline is s.replace(/\s+$/gm, ”): at each position, the longest
// run of whitespace that ends at a line end or the end of the text.
func removeTrailingSpaceMultiline(s string) string {
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); {
		r, n := jsstr.Decode(s, i)
		if !jsstr.IsSpace(r) {
			i += n
			continue
		}
		end := skipJSSpace(s, i)
		// Back off to the last place in the run where $ holds.
		match := -1
		for j := end; j > i; {
			if j == len(s) {
				match = j
				break
			}
			if next, _ := jsstr.Decode(s, j); jsstr.IsLineEnd(next) {
				match = j
				break
			}
			_, m := jsstr.DecodeLast(s, j)
			j -= m
		}
		if match == -1 {
			i += n
			continue
		}
		b.WriteString(s[last:i])
		last = match
		i = match
	}
	return b.String() + s[last:]
}

// escapes are the escapes StringValue reads; any other escaped character stands for itself.
var escapes = map[uint16]uint16{'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v', '0': 0}

// StringValue reads a STRING token: the text between the quotes, with escapes read.
func StringValue(text string) string {
	units := jsstr.UTF16(text)
	if !strings.Contains(text, "\\") {
		from := min(1, len(units))
		return jsstr.FromUTF16(units[from:max(from, len(units)-1)])
	}
	out := make([]uint16, 0, len(units))
	for i := 1; i < len(units)-1; i++ {
		c := units[i]
		if c != '\\' {
			out = append(out, c)
			continue
		}
		i++
		next := units[i]
		if e, ok := escapes[next]; ok {
			next = e
		}
		out = append(out, next)
	}
	return jsstr.FromUTF16(out)
}

// Reader reads tokens for a hand-written parser, with Chevrotain-style messages on failure.
type Reader struct {
	I       int
	Tokens  Tokens
	Types   []*TokenType
	Diagram string
	Origin  *perr.Origin
}

// NewReader returns a Reader at the first token, locating errors with o.
func NewReader(tokens Tokens, types []*TokenType, diagram string, o *perr.Origin) *Reader {
	return &Reader{Tokens: tokens, Types: types, Diagram: diagram, Origin: o}
}

// Kind returns the kind of the current token, -1 at the end.
func (r *Reader) Kind() int { return r.Tokens.Kinds[r.I] }

// Text returns the text of the token at index.
func (r *Reader) Text(index int) string {
	return r.Tokens.Src.String(r.Tokens.Starts[index], r.Tokens.Ends[index])
}

// Take returns the current token's text and moves past it.
func (r *Reader) Take() string {
	t := r.Text(r.I)
	r.I++
	return t
}

// Accept moves past the current token if it is of kind.
func (r *Reader) Accept(kind int) bool {
	if r.Tokens.Kinds[r.I] != kind {
		return false
	}
	r.I++
	return true
}

// Expect takes a token of kind or fails.
func (r *Reader) Expect(kind int) string {
	if r.Tokens.Kinds[r.I] != kind {
		r.Fail("token of type '" + r.Types[kind].Name + "'")
	}
	return r.Take()
}

// Fail throws a parse error at the current token.
func (r *Reader) Fail(expected string) {
	found := "end of input"
	if r.Kind() != -1 {
		found = "`" + r.Text(r.I) + "`"
	}
	perr.Throw(perr.SyntaxError(r.Origin, r.Diagram, r.Tokens.Src, r.Tokens.Starts[r.I], "Expecting "+expected+" but found "+found+".", false))
}
