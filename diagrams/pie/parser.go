package pie

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// Token kinds, in the order of pieTokens.
const (
	kindShowData = iota
	kindPie
	kindColon
	kindNumber
	kindAccDescr
	kindAccTitle
	kindTitle
	kindString
	kindNewline
)

func digits(src common.Src, from int) int {
	i := from
	for c := src.At(i); c >= 48 && c <= 57; c = src.At(i) {
		i++
	}
	return i
}

// numberScanner answers as NUMBER_PIE's pattern does. The pattern reads a whole run of
// digits before it finds there is no fraction, and would do so again from every digit of a
// long run of zeros.
func numberScanner() common.Matcher {
	from, end, fraction := 0, 0, -1
	return func(src common.Src, p int) int {
		at := p
		if src.At(p) == 45 {
			at = p + 1
		}
		if at < from || at >= end {
			from = at
			end = digits(src, at)
			fraction = -1
			if end > at && src.At(end) == 46 {
				stop := digits(src, end+1)
				length := stop - end - 1
				if length > 0 && src.At(stop) != 46 {
					fraction = stop
				} else if length > 1 {
					fraction = stop - 1
				}
			}
		}
		if end == at {
			return -1
		}
		if fraction >= 0 {
			return fraction
		}
		if src.At(at) == 48 {
			if src.At(at+1) == 46 {
				return -1
			}
			return at + 1
		}
		if src.At(end) != 46 {
			return end
		}
		if end-1 > at {
			return end - 1
		}
		return -1
	}
}

var pieLexer = common.NewLexer([]*common.TokenType{
	common.Keyword("showData"),
	common.Keyword("pie"),
	{Name: ":", Literal: ":"},
	// /(?:-?[0-9]+\.[0-9]+(?!\.))|(?:-?(0|[1-9][0-9]*)(?!\.))/y
	{Name: "NUMBER_PIE", NewMatch: numberScanner, First: "-0123456789"},
	common.AccDescr,
	common.AccTitle,
	common.Title,
	common.String,
	common.Newline,
	common.Whitespace,
	common.Yaml,
	common.Directive,
	common.SingleLineComment,
})

type section struct {
	label string
	value float64
}

type ast struct {
	showData                  bool
	title, accTitle, accDescr *string
	sections                  []section
}

func parsePie(src string, o *perr.Origin) ast {
	r := common.NewReader(pieLexer.Tokenize(jsstr.UTF16(src), "pie", o), pieLexer.Types(), "pie", o)
	var a ast

	endOfLine := func() {
		if r.Kind() == -1 {
			return
		}
		if r.Kind() != kindNewline {
			r.Fail("a line break or the end of input")
		}
		for r.Kind() == kindNewline {
			r.I++
		}
	}

	for r.Kind() == kindNewline {
		r.I++
	}
	r.Expect(kindPie)
	a.showData = r.Accept(kindShowData)
	for r.Kind() != -1 {
		switch r.Kind() {
		case kindNewline:
			r.I++
		case kindAccDescr:
			a.accDescr = new(common.AccDescrValue(r.Take()))
			endOfLine()
		case kindAccTitle:
			a.accTitle = new(common.AccTitleValue(r.Take()))
			endOfLine()
		case kindTitle:
			a.title = new(common.TitleValue(r.Take()))
			endOfLine()
		case kindString:
			label := common.StringValue(r.Take())
			r.Expect(kindColon)
			value := jsnum.Parse(r.Expect(kindNumber))
			a.sections = append(a.sections, section{label: label, value: value})
			endOfLine()
		default:
			r.Fail("a section, a title, or a line break")
		}
	}
	return a
}
