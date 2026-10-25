package packet

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// Token kinds, in the order of packetTokens.
const (
	kindPacketBeta = iota
	kindPacket
	kindDash
	kindPlus
	kindColon
	kindAccDescr
	kindAccTitle
	kindTitle
	kindInt
	kindString
	kindNewline
)

// matchInt is /0|[1-9][0-9]*(?!\.)/y. A run of digits before a dot backs off by one.
func matchInt(src common.Src, at int) int {
	c := src.At(at)
	if c == '0' {
		return at + 1
	}
	if c < '1' || c > '9' {
		return -1
	}
	end := at + 1
	for d := src.At(end); d >= '0' && d <= '9'; d = src.At(end) {
		end++
	}
	if src.At(end) != '.' {
		return end
	}
	if end-1 > at {
		return end - 1
	}
	return -1
}

var packetLexer = common.NewLexer([]*common.TokenType{
	{Name: "packet-beta", Literal: "packet-beta"},
	common.Keyword("packet"),
	// A front matter block also starts with dashes, so Langium lets it win over a dash.
	{Name: "-", Literal: "-", Longer: []*common.TokenType{common.Yaml}},
	{Name: "+", Literal: "+"},
	{Name: ":", Literal: ":"},
	common.AccDescr,
	common.AccTitle,
	common.Title,
	{Name: "INT", First: "0123456789", Match: matchInt},
	common.String,
	common.Newline,
	common.Whitespace,
	common.Yaml,
	common.Directive,
	common.SingleLineComment,
})

type blockAst struct {
	start, end, bits *float64
	label            string
}

type ast struct {
	title, accTitle, accDescr *string
	blocks                    []blockAst
}

func parsePacket(src string, o *perr.Origin) ast {
	r := common.NewReader(packetLexer.Tokenize(jsstr.UTF16(src), "packet", o), packetLexer.Types(), "packet", o)
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
	if !r.Accept(kindPacketBeta) {
		r.Expect(kindPacket)
	}
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
		case kindInt, kindPlus:
			var b blockAst
			if r.Accept(kindPlus) {
				b.bits = new(jsnum.Parse(r.Expect(kindInt)))
			} else {
				b.start = new(jsnum.Parse(r.Take()))
				if r.Accept(kindDash) {
					b.end = new(jsnum.Parse(r.Expect(kindInt)))
				}
			}
			r.Expect(kindColon)
			b.label = common.StringValue(r.Expect(kindString))
			a.blocks = append(a.blocks, b)
			endOfLine()
		default:
			r.Fail("a field, a title, or a line break")
		}
	}
	return a
}
