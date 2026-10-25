package radar

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// Token kinds, in the order of radarTokens.
const (
	tRadarColon = iota
	tRadar
	tShowLegend
	tGraticule
	tCurve
	tTicks
	tAxis
	tMax
	tMin
	tColon
	tComma
	tOpen
	tClose
	tOpenBrace
	tCloseBrace
	tGraticuleValue
	tBoolean
	tAccDescr
	tAccTitle
	tTitle
	tNumber
	tString
	tID
	tNewline
)

func digits(src common.Src, at int) int {
	end := at
	for util.IsDigit(src.At(end)) {
		end++
	}
	return end
}

// newMatchNumber is the NUMBER pattern without its backtracking. As a regular expression it
// reads to the end of a run of digits from every position in the run, which is quadratic
// for a long run of zeros.
func newMatchNumber() common.Matcher {
	runFrom, runEnd := 0, 0
	return func(src common.Src, at int) int {
		if at < runFrom || at >= runEnd {
			runFrom = at
			runEnd = digits(src, at)
		}
		whole := runEnd
		if whole == at {
			return -1
		}
		dot := src.At(whole) == 46
		if dot {
			fraction := digits(src, whole+1)
			length := fraction - whole - 1
			if length > 0 && src.At(fraction) != 46 {
				return fraction
			}
			if length > 1 {
				return fraction - 1
			}
		}
		if src.At(at) == 48 {
			return at + 1
		}
		if !dot {
			return whole
		}
		if whole-1 > at {
			return whole - 1
		}
		return -1
	}
}

// matchBlock is /[\t ]*accDescr\s*{/y.
func matchBlock(src common.Src, at int) int {
	i := at
	for util.IsBlank(src.At(i)) {
		i++
	}
	if !src.HasPrefix(i, "accDescr") {
		return -1
	}
	i += len("accDescr")
	for util.IsSpace(src.At(i)) {
		i++
	}
	if src.At(i) != 123 {
		return -1
	}
	return i + 1
}

// newMatchAccDescr: in the other grammars an `accDescr {` that never closes ends the lexing.
// Here `accDescr` is also a name and `{` a token, so each unclosed one would read to the end
// of the text again.
func newMatchAccDescr() common.Matcher {
	lastBrace := -2
	return func(src common.Src, at int) int {
		if end := matchBlock(src, at); end != -1 {
			if lastBrace == -2 {
				lastBrace = -1
				for k := len(src) - 1; k >= 0; k-- {
					if src[k] == 125 {
						lastBrace = k
						break
					}
				}
			}
			if end > lastBrace {
				return -1
			}
		}
		return common.AccDescr.Match(src, at)
	}
}

// idType is /[\w]([-\w]*\w)?/y.
var idType = &common.TokenType{Name: "ID", Match: func(src common.Src, at int) int {
	if !util.IsWord(src.At(at)) {
		return -1
	}
	end := at + 1
	for k := at + 1; ; k++ {
		c := src.At(k)
		if util.IsWord(c) {
			end = k + 1
		} else if c != 45 {
			break
		}
	}
	return end
}}

func word(text string) *common.TokenType {
	return &common.TokenType{Name: text, Literal: text, Longer: []*common.TokenType{idType}}
}

func mark(text string) *common.TokenType { return &common.TokenType{Name: text, Literal: text} }

// alternatives matches the first of words that is at at.
func alternatives(name string, words ...string) *common.TokenType {
	return &common.TokenType{Name: name, Match: func(src common.Src, at int) int {
		for _, w := range words {
			if src.HasPrefix(at, w) {
				return at + len(w)
			}
		}
		return -1
	}}
}

var radarLexer = common.NewLexer(func() []*common.TokenType {
	radar := *common.Keyword("radar-beta")
	radar.Longer = []*common.TokenType{idType}
	accDescr := *common.AccDescr
	accDescr.Match = nil
	accDescr.NewMatch = newMatchAccDescr
	return []*common.TokenType{
		mark("radar-beta:"),
		&radar,
		word("showLegend"),
		word("graticule"),
		word("curve"),
		word("ticks"),
		word("axis"),
		word("max"),
		word("min"),
		mark(":"),
		mark(","),
		mark("["),
		mark("]"),
		mark("{"),
		mark("}"),
		alternatives("GRATICULE", "circle", "polygon"),
		alternatives("BOOLEAN", "true", "false"),
		&accDescr,
		common.AccTitle,
		common.Title,
		{Name: "NUMBER", NewMatch: newMatchNumber},
		common.String,
		idType,
		common.Newline,
		common.Whitespace,
		common.Yaml,
		common.Directive,
		common.SingleLineComment,
	}
}())

type axisAst struct {
	name  string
	label *string
}

type entryAst struct {
	// axis is the text of the reference Langium leaves unresolved; nil when the entry has
	// none.
	axis  *string
	value float64
}

type curveAst struct {
	name    string
	label   *string
	entries []entryAst
}

type optionAst struct {
	name    string
	boolean bool
	number  float64
	text    string
}

type radarAst struct {
	title, accTitle, accDescr *string
	axes                      []axisAst
	curves                    []curveAst
	options                   []optionAst
}

func parseRadar(source string, o *perr.Origin) radarAst {
	tokens := radarLexer.Tokenize(jsstr.UTF16(source), "radar", o)
	r := common.NewReader(tokens, radarLexer.Types(), "radar", o)
	var ast radarAst

	newlines := func() {
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
		newlines()
	}

	label := func() *string {
		if !r.Accept(tOpen) {
			return nil
		}
		text := common.StringValue(r.Expect(tString))
		r.Expect(tClose)
		return &text
	}

	entry := func(detailed bool) entryAst {
		if !detailed {
			return entryAst{value: jsnum.Parse(r.Expect(tNumber))}
		}
		axis := r.Expect(tID)
		r.Accept(tColon)
		return entryAst{axis: &axis, value: jsnum.Parse(r.Expect(tNumber))}
	}

	curve := func() curveAst {
		node := curveAst{name: r.Expect(tID)}
		node.label = label()
		r.Expect(tOpenBrace)
		newlines()
		if r.Kind() != tNumber && r.Kind() != tID {
			r.Fail("a number or an axis name")
		}
		detailed := r.Kind() == tID
		node.entries = append(node.entries, entry(detailed))
		for r.Accept(tComma) {
			newlines()
			node.entries = append(node.entries, entry(detailed))
		}
		newlines()
		r.Expect(tCloseBrace)
		return node
	}

	axis := func() axisAst {
		node := axisAst{name: r.Expect(tID)}
		node.label = label()
		return node
	}

	option := func() optionAst {
		kind := r.Kind()
		node := optionAst{name: r.Take()}
		switch kind {
		case tShowLegend:
			node.boolean = r.Expect(tBoolean) == "true"
		case tGraticule:
			node.text = r.Expect(tGraticuleValue)
		default:
			node.number = jsnum.Parse(r.Expect(tNumber))
		}
		return node
	}

	isOption := func() bool {
		k := r.Kind()
		return k == tShowLegend || k == tTicks || k == tMax || k == tMin || k == tGraticule
	}

	newlines()
	if !r.Accept(tRadarColon) {
		r.Expect(tRadar)
		r.Accept(tColon)
	}
	newlines()
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
		case tAxis:
			r.I++
			ast.axes = append(ast.axes, axis())
			for r.Accept(tComma) {
				ast.axes = append(ast.axes, axis())
			}
		case tCurve:
			r.I++
			ast.curves = append(ast.curves, curve())
			for r.Accept(tComma) {
				ast.curves = append(ast.curves, curve())
			}
		default:
			if !isOption() {
				r.Fail("an axis, a curve, an option, a title, or a line break")
			}
			ast.options = append(ast.options, option())
			for r.Accept(tComma) {
				if !isOption() {
					r.Fail("an option")
				}
				ast.options = append(ast.options, option())
			}
		}
	}
	return ast
}
