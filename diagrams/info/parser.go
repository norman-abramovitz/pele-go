package info

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// Token kinds, in the order of infoTokens.
const (
	kindShowInfo = iota
	kindInfo
	kindAccDescr
	kindAccTitle
	kindTitle
	kindNewline
)

var infoLexer = common.NewLexer([]*common.TokenType{
	common.Keyword("showInfo"),
	common.Keyword("info"),
	common.AccDescr,
	common.AccTitle,
	common.Title,
	common.Newline,
	common.Whitespace,
	common.Yaml,
	common.Directive,
	common.SingleLineComment,
})

type ast struct {
	title, accTitle, accDescr *string
}

func parseInfo(src string, o *perr.Origin) ast {
	tokens := infoLexer.Tokenize(jsstr.UTF16(src), "info", o)
	r := common.NewReader(tokens, infoLexer.Types(), "info", o)
	var a ast

	skipNewlines := func() {
		for r.Kind() == kindNewline {
			r.I++
		}
	}
	skipNewlines()
	r.Expect(kindInfo)
	skipNewlines()
	if r.Accept(kindShowInfo) {
		skipNewlines()
	}
	for r.Kind() != -1 {
		switch r.Kind() {
		case kindAccDescr:
			a.accDescr = new(common.AccDescrValue(r.Take()))
		case kindAccTitle:
			a.accTitle = new(common.AccTitleValue(r.Take()))
		case kindTitle:
			a.title = new(common.TitleValue(r.Take()))
		default:
			r.Fail("a title or the end of input")
		}
		if r.Kind() == -1 {
			break
		}
		if r.Kind() != kindNewline {
			r.Fail("a line break or the end of input")
		}
		skipNewlines()
	}
	return a
}
