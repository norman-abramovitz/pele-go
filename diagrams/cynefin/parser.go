package cynefin

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// Token kinds, in the order of cynefinTokens.
const (
	tKeywordColon = iota
	tKeyword
	tArrow
	tColon
	tDomain
	tAccDescr
	tAccTitle
	tTitle
	tString
	tNewline
)

var domainNames = [...]string{"complex", "complicated", "clear", "chaotic", "confusion"}

var cynefinTokens = []*common.TokenType{
	{Name: "cynefin-beta:", Literal: "cynefin-beta:"},
	common.Keyword("cynefin-beta"),
	{Name: "-->", Literal: "-->"},
	{Name: ":", Literal: ":"},
	// /complex|complicated|clear|chaotic|confusion/y, with no first characters given, as
	// upstream gives none.
	{Name: "DOMAIN_NAME", Match: func(src common.Src, at int) int {
		for _, name := range domainNames {
			if src.HasPrefix(at, name) {
				return at + len(name)
			}
		}
		return -1
	}},
	common.AccDescr,
	common.AccTitle,
	common.Title,
	common.String,
	common.Newline,
	common.Whitespace,
	common.Yaml,
	common.Directive,
	common.SingleLineComment,
}

var cynefinLexer = common.NewLexer(cynefinTokens)

type domainBlockAst struct {
	domain string
	items  []string
}

type transitionAst struct {
	from, to string
	label    *string
}

type cynefinAst struct {
	title, accTitle, accDescr *string
	domains                   []domainBlockAst
	transitions               []transitionAst
}

func parseCynefin(src string, o *perr.Origin) cynefinAst {
	r := common.NewReader(cynefinLexer.Tokenize(common.Src(jsstr.UTF16(src)), "cynefin", o), cynefinTokens, "cynefin", o)
	var ast cynefinAst

	skipLines := func() {
		for r.Kind() == tNewline {
			r.I++
		}
	}
	endOfLine := func() {
		if r.Kind() == -1 {
			return
		}
		if r.Kind() != tNewline {
			r.Fail("a line break or the end of input")
		}
		skipLines()
	}

	skipLines()
	if r.Kind() != tKeyword && r.Kind() != tKeywordColon {
		r.Fail("'cynefin-beta'")
	}
	r.I++
	for r.Kind() != -1 {
		switch r.Kind() {
		case tNewline:
			r.I++
		case tAccDescr:
			ast.accDescr = new(common.AccDescrValue(r.Take()))
			endOfLine()
		case tAccTitle:
			ast.accTitle = new(common.AccTitleValue(r.Take()))
			endOfLine()
		case tTitle:
			ast.title = new(common.TitleValue(r.Take()))
			endOfLine()
		case tDomain:
			name := r.Take()
			if r.Accept(tArrow) {
				transition := transitionAst{from: name, to: r.Expect(tDomain)}
				if r.Accept(tColon) {
					transition.label = new(common.StringValue(r.Expect(tString)))
				}
				endOfLine()
				ast.transitions = append(ast.transitions, transition)
				break
			}
			block := domainBlockAst{domain: name}
			skipLines()
			for r.Kind() == tString {
				block.items = append(block.items, common.StringValue(r.Take()))
				skipLines()
			}
			ast.domains = append(ast.domains, block)
		default:
			r.Fail("a domain, a transition, a title, or a line break")
		}
	}
	return ast
}
