package treemap

import (
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// Token kinds, in the order of treemapLexer's types.
const (
	kindAccDescr = iota
	kindAccTitle
	kindTitle
	kindKeyword
	kindClassDef
	kindStyleSeparator
	kindSeparator
	kindComma
	kindIndentation
	kindWhitespace
	kindComment
	kindNewline
	kindID
	kindNumber
	kindString
)

// matchClassDef is /classDef\s+([a-zA-Z_][a-zA-Z0-9_]+)(?:\s+([^;\r\n]*))?(?:;)?/ at at. It
// returns the end of the match, or -1, and the class name and style text (nil when the group
// did not take part).
func matchClassDef(src common.Src, at int) (end int, name string, style *string) {
	if !src.HasPrefix(at, "classDef") {
		return -1, "", nil
	}
	i := at + len("classDef")
	j := i
	for util.IsSpace(src.At(j)) {
		j++
	}
	if j == i {
		return -1, "", nil
	}
	start := j
	if c := src.At(j); !(util.IsLetter(c) || c == '_') {
		return -1, "", nil
	}
	j++
	if !util.IsWord(src.At(j)) {
		return -1, "", nil
	}
	for util.IsWord(src.At(j)) {
		j++
	}
	name = src.String(start, j)
	k := j
	for util.IsSpace(src.At(k)) {
		k++
	}
	if k > j {
		from := k
		for k < len(src) && src[k] != ';' && src[k] != '\r' && src[k] != '\n' {
			k++
		}
		s := src.String(from, k)
		style = &s
		j = k
	}
	if src.At(j) == ';' {
		j++
	}
	return j, name, style
}

func blanks(src common.Src, at int) int {
	i := at
	for util.IsBlank(src.At(i)) {
		i++
	}
	if i == at {
		return -1
	}
	return i
}

// Mermaid's treemap grammar has its own terminals. Every run of spaces or tabs is an
// INDENTATION token, since that comes before WS, and line breaks are hidden, so rows are told
// apart by their tokens alone and the indentation of a row is whatever whitespace precedes
// it.
var treemapLexer = common.NewLexer([]*common.TokenType{
	common.AccDescr,
	common.AccTitle,
	common.Title,
	// /treemap-beta|treemap/y
	{Name: "TREEMAP_KEYWORD", Match: func(src common.Src, at int) int {
		if src.HasPrefix(at, "treemap-beta") {
			return at + len("treemap-beta")
		}
		if src.HasPrefix(at, "treemap") {
			return at + len("treemap")
		}
		return -1
	}},
	{Name: "CLASS_DEF", Match: func(src common.Src, at int) int {
		end, _, _ := matchClassDef(src, at)
		return end
	}},
	{Name: "STYLE_SEPARATOR", Literal: ":::"},
	{Name: "SEPARATOR", Literal: ":"},
	{Name: "COMMA", Literal: ","},
	{Name: "INDENTATION", Match: blanks},
	{Name: "WS", Match: blanks, Hidden: true},
	// /\%\%[^\n]*/y
	{Name: "ML_COMMENT", Hidden: true, Match: func(src common.Src, at int) int {
		if !src.HasPrefix(at, "%%") {
			return -1
		}
		i := at + 2
		for i < len(src) && src[i] != '\n' {
			i++
		}
		return i
	}},
	{Name: "NL", Match: common.Newline.Match, Hidden: true},
	// /[a-zA-Z_][a-zA-Z0-9_]*/y
	{Name: "ID2", Match: func(src common.Src, at int) int {
		if c := src.At(at); !(util.IsLetter(c) || c == '_') {
			return -1
		}
		i := at + 1
		for util.IsWord(src.At(i)) {
			i++
		}
		return i
	}},
	// /[0-9_\.\,]+/y
	{Name: "NUMBER2", Match: func(src common.Src, at int) int {
		i := at
		for c := src.At(i); util.IsDigit(c) || c == '_' || c == '.' || c == ','; c = src.At(i) {
			i++
		}
		if i == at {
			return -1
		}
		return i
	}},
	// /"[^"]*"|'[^']*'/y
	{Name: "STRING2", Match: func(src common.Src, at int) int {
		q := src.At(at)
		if q != '"' && q != '\'' {
			return -1
		}
		for i := at + 1; i < len(src); i++ {
			if int(src[i]) == q {
				return i + 1
			}
		}
		return -1
	}},
})

type itemAst struct {
	leaf          bool
	name          string
	value         float64
	classSelector *string
}

// rowAst is a TreemapRow, or a ClassDefStatement when classDef is set.
type rowAst struct {
	indent    *int
	item      itemAst
	classDef  bool
	className string
	styleText *string
}

type treemapAst struct {
	title, accTitle, accDescr *string
	rows                      []rowAst
}

func parseTreemap(source string, o *perr.Origin) treemapAst {
	src := common.Src(jsstr.UTF16(source))
	r := common.NewReader(treemapLexer.Tokenize(src, "treemap", o), treemapLexer.Types(), "treemap", o)
	kinds := r.Tokens.Kinds
	var ast treemapAst

	r.Expect(kindKeyword)
	for r.Kind() != -1 {
		kind := r.Kind()
		switch kind {
		case kindAccDescr:
			ast.accDescr = new(common.AccDescrValue(r.Take()))
			continue
		case kindAccTitle:
			ast.accTitle = new(common.AccTitleValue(r.Take()))
			continue
		case kindTitle:
			ast.title = new(common.TitleValue(r.Take()))
			continue
		}
		if kind != kindIndentation && kind != kindString && kind != kindClassDef {
			r.Fail("a node, a class definition, or a title")
		}

		var indent *int
		if kind == kindIndentation {
			indent = new(len(r.Take()))
		}
		if r.Kind() == kindClassDef {
			_, name, style := matchClassDef(common.Src(jsstr.UTF16(r.Take())), 0)
			row := rowAst{indent: indent, classDef: true, className: name}
			if style != nil && *style != "" {
				row.styleText = style
			}
			ast.rows = append(ast.rows, row)
			continue
		}

		text := r.Expect(kindString)
		name := text[1 : len(text)-1]
		// A leaf is a name followed by a colon or comma, with or without whitespace before it.
		after := r.I
		if kinds[r.I] == kindIndentation {
			after = r.I + 1
		}
		var item itemAst
		if kinds[after] == kindSeparator || kinds[after] == kindComma {
			r.I = after + 1
			r.Accept(kindIndentation)
			item = itemAst{leaf: true, name: name, value: jsnum.ParseFloat(strings.ReplaceAll(r.Expect(kindNumber), ",", ""))}
		} else {
			item = itemAst{name: name}
		}
		if r.Accept(kindStyleSeparator) {
			item.classSelector = new(r.Expect(kindID))
		}
		ast.rows = append(ast.rows, rowAst{indent: indent, item: item})
	}
	return ast
}
