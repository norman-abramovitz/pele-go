package wardley

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// Token kinds, in the order of wardleyTokens.
const (
	tLbracket = iota
	tComma
	tRbracket
	tAt
	tSlash
	tLparen
	tRparen
	tMinus
	tLbrace
	tRbrace
	tNumber
	tArrow
	tLinkPort
	tLinkArrow
	tLinkLabel
	tStrategy
	tWardley
	tSize
	tEvolution
	tAnchor
	tComponent
	tLabel
	tInertia
	tEvolve
	tPipeline
	tNote
	tAnnotations
	tAnnotation
	tAccelerator
	tDeaccelerator
	tName
	tWs
	tAccDescr
	tAccTitle
	tTitle
	tInt
	tString
	tID
	tNewline
)

func digits(src common.Src, from int) int {
	i := from
	for util.IsDigit(src.At(i)) {
		i++
	}
	return i
}

// numberScanner answers as WARDLEY_NUMBER's pattern /[0-9]+\.[0-9]+/y does. The pattern reads
// a whole run of digits before it finds there is no fraction, and would do so again from
// every digit of a long run.
func numberScanner() common.Matcher {
	from, end := 0, 0
	return func(src common.Src, p int) int {
		if p < from || p >= end {
			from = p
			end = digits(src, p)
		}
		if end == p || src.At(end) != '.' {
			return -1
		}
		stop := digits(src, end+1)
		if stop > end+1 {
			return stop
		}
		return -1
	}
}

// arrowToken is /->/y.
var arrowToken = &common.TokenType{Name: "ARROW", Literal: "->"}

// quotedArrow matches +'[^']*' at at and returns the index after the closing quote, or -1.
func quotedArrow(src common.Src, at int) int {
	if src.At(at) != '+' || src.At(at+1) != '\'' {
		return -1
	}
	close := src.Index("'", at+2)
	if close == -1 {
		return -1
	}
	return close + 1
}

// linkArrowToken is /-->|-\.->|>|\+'[^']*'<>|\+'[^']*'<|\+'[^']*'>/y.
var linkArrowToken = &common.TokenType{Name: "LINK_ARROW", First: "->+", Match: func(src common.Src, at int) int {
	switch {
	case src.HasPrefix(at, "-->"):
		return at + 3
	case src.HasPrefix(at, "-.->"):
		return at + 4
	case src.At(at) == '>':
		return at + 1
	}
	q := quotedArrow(src, at)
	switch {
	case q == -1:
		return -1
	case src.HasPrefix(q, "<>"):
		return q + 2
	case src.At(q) == '<' || src.At(q) == '>':
		return q + 1
	}
	return -1
}}

// word is a keyword matched as a plain prefix, with no boundary after it.
func word(name, text string) *common.TokenType {
	return &common.TokenType{Name: name, First: text[:1], Match: func(src common.Src, at int) int {
		if src.HasPrefix(at, text) {
			return at + len(text)
		}
		return -1
	}}
}

// nameRun matches (?:[A-Za-z0-9_()&]|-(?!>))* from i.
func nameRun(src common.Src, i int) int {
	for {
		c := src.At(i)
		if util.IsLetter(c) || util.IsDigit(c) || c == '_' || c == '(' || c == ')' || c == '&' || c == '-' && src.At(i+1) != '>' {
			i++
			continue
		}
		return i
	}
}

// nameWithSpaces is
// /(?!title\s|accTitle|accDescr)[A-Za-z](?:[A-Za-z0-9_()&]|-(?!>))*(?:[ \t]+[A-Za-z(](?:[A-Za-z0-9_()&]|-(?!>))*)*/y.
// The runs cannot hold a blank, so the pattern never backtracks into them.
func nameWithSpaces(src common.Src, at int) int {
	if src.HasPrefix(at, "title") && src.At(at+5) >= 0 && jsstr.IsSpace(rune(src.At(at+5))) ||
		src.HasPrefix(at, "accTitle") || src.HasPrefix(at, "accDescr") {
		return -1
	}
	if !util.IsLetter(src.At(at)) {
		return -1
	}
	end := nameRun(src, at+1)
	for {
		k := end
		for src.At(k) == ' ' || src.At(k) == '\t' {
			k++
		}
		if k == end || !(util.IsLetter(src.At(k)) || src.At(k) == '(') {
			return end
		}
		end = nameRun(src, k+1)
	}
}

// intToken is /0|[1-9][0-9]*(?!\.)/y: a run followed by a dot backs off one digit.
func intToken(src common.Src, at int) int {
	c := src.At(at)
	if c == '0' {
		return at + 1
	}
	if c < '1' || c > '9' {
		return -1
	}
	end := digits(src, at+1)
	if src.At(end) != '.' {
		return end
	}
	if end-1 > at {
		return end - 1
	}
	return -1
}

// idToken is /[\w]([-\w]*\w)?/y: it ends at the last word character of the run.
func idToken(src common.Src, at int) int {
	if !util.IsWord(src.At(at)) {
		return -1
	}
	end := at + 1
	for i := at + 1; ; i++ {
		c := src.At(i)
		if util.IsWord(c) {
			end = i + 1
		} else if c != '-' {
			return end
		}
	}
}

var wardleyTokens = []*common.TokenType{
	{Name: "[", Literal: "["},
	{Name: ",", Literal: ","},
	{Name: "]", Literal: "]"},
	{Name: "@", Literal: "@"},
	{Name: "/", Literal: "/"},
	{Name: "(", Literal: "("},
	{Name: ")", Literal: ")"},
	{Name: "-", Literal: "-", Longer: []*common.TokenType{arrowToken, linkArrowToken, common.Yaml}},
	{Name: "{", Literal: "{"},
	{Name: "}", Literal: "}"},
	{Name: "WARDLEY_NUMBER", First: "0123456789", NewMatch: numberScanner},
	arrowToken,
	{Name: "LINK_PORT", First: "+", Match: func(src common.Src, at int) int {
		switch {
		case src.HasPrefix(at, "+<>"):
			return at + 3
		case src.HasPrefix(at, "+>"), src.HasPrefix(at, "+<"):
			return at + 2
		}
		return -1
	}},
	linkArrowToken,
	{Name: "LINK_LABEL", First: ";", Match: func(src common.Src, at int) int {
		if src.At(at) != ';' {
			return -1
		}
		i := at + 1
		for i < len(src) && src[i] != '\n' && src[i] != '\r' {
			i++
		}
		if i == at+1 {
			return -1
		}
		return i
	}},
	{Name: "STRATEGY", First: "bom", Match: func(src common.Src, at int) int {
		for _, s := range [...]string{"build", "buy", "outsource", "market"} {
			if src.HasPrefix(at, s) {
				return at + len(s)
			}
		}
		return -1
	}},
	word("KW_WARDLEY", "wardley-beta"),
	word("KW_SIZE", "size"),
	word("KW_EVOLUTION", "evolution"),
	word("KW_ANCHOR", "anchor"),
	word("KW_COMPONENT", "component"),
	word("KW_LABEL", "label"),
	word("KW_INERTIA", "inertia"),
	word("KW_EVOLVE", "evolve"),
	word("KW_PIPELINE", "pipeline"),
	word("KW_NOTE", "note"),
	word("KW_ANNOTATIONS", "annotations"),
	word("KW_ANNOTATION", "annotation"),
	word("KW_ACCELERATOR", "accelerator"),
	word("KW_DEACCELERATOR", "deaccelerator"),
	{Name: "NAME_WITH_SPACES", First: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", Match: nameWithSpaces},
	{Name: "WS", First: "\t ", Hidden: true, Match: func(src common.Src, at int) int {
		i := at
		for src.At(i) == ' ' || src.At(i) == '\t' {
			i++
		}
		if i == at {
			return -1
		}
		return i
	}},
	common.AccDescrScanner(""),
	common.AccTitle,
	common.Title,
	{Name: "INT", First: "0123456789", Match: intToken},
	common.String,
	{Name: "ID", First: "_0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", Match: idToken},
	common.Newline,
	common.Whitespace,
	common.Yaml,
	common.Directive,
	common.SingleLineComment,
}

var wardleyLexer = common.NewLexer(wardleyTokens)

type labelAst struct {
	negX    bool
	offsetX float64
	negY    bool
	offsetY float64
}

type stageAst struct {
	name       string
	boundary   *float64
	secondName *string
}

type componentAst struct {
	name       string
	visibility float64
	evolution  float64
	label      *labelAst
	// strategy is the decorator's strategy, "" when there is none.
	strategy string
	inertia  bool
}

type pipelineComponentAst struct {
	name      string
	evolution float64
	label     *labelAst
}

type linkAst struct {
	from      string
	fromPort  string
	arrow     *string
	to        string
	toPort    string
	linkLabel *string
}

type anchorAst struct {
	name                  string
	visibility, evolution float64
}

type evolveAst struct {
	component string
	target    float64
}

type pipelineAst struct {
	parent     string
	components []pipelineComponentAst
}

type noteAst struct {
	text                  string
	visibility, evolution float64
}

type annotationAst struct {
	number, x, y float64
	text         string
}

type placedAst struct {
	name string
	x, y float64
}

type sizeAst struct{ width, height float64 }

type wardleyAst struct {
	title, accTitle, accDescr *string
	size                      *sizeAst
	evolution                 []stageAst
	anchors                   []anchorAst
	components                []componentAst
	links                     []linkAst
	evolves                   []evolveAst
	pipelines                 []pipelineAst
	notes                     []noteAst
	annotations               [][2]float64
	annotation                []annotationAst
	accelerators              []placedAst
	deaccelerators            []placedAst
}

func parseWardley(src string, o *perr.Origin) wardleyAst {
	r := common.NewReader(wardleyLexer.Tokenize(common.Src(jsstr.UTF16(src)), "wardley", o), wardleyTokens, "wardley", o)
	var ast wardleyAst

	at := func(kind int) bool { return r.Tokens.Kinds[r.I] == kind }
	isName := func() bool { return at(tString) || at(tID) || at(tName) }
	name := func() string {
		if at(tString) {
			return common.StringValue(r.Take())
		}
		if at(tID) || at(tName) {
			return r.Take()
		}
		r.Fail("a name")
		return ""
	}
	number := func() float64 { return jsnum.Parse(r.Expect(tNumber)) }
	integer := func() float64 { return jsnum.ParseInt(r.Expect(tInt)) }
	coordinate := func() float64 {
		if at(tNumber) {
			return jsnum.Parse(r.Take())
		}
		return jsnum.ParseInt(r.Expect(tInt))
	}
	endOfLine := func() {
		if at(-1) {
			return
		}
		if !at(tNewline) {
			r.Fail("a line break or the end of input")
		}
		for at(tNewline) {
			r.I++
		}
	}
	pair := func(read func() float64) (float64, float64) {
		r.Expect(tLbracket)
		first := read()
		r.Expect(tComma)
		second := read()
		r.Expect(tRbracket)
		return first, second
	}
	label := func() *labelAst {
		r.Expect(tLabel)
		r.Expect(tLbracket)
		var l labelAst
		l.negX = r.Accept(tMinus)
		l.offsetX = integer()
		r.Expect(tComma)
		l.negY = r.Accept(tMinus)
		l.offsetY = integer()
		r.Expect(tRbracket)
		return &l
	}
	stage := func() stageAst {
		out := stageAst{name: name()}
		if r.Accept(tAt) {
			out.boundary = new(number())
		}
		if r.Accept(tSlash) {
			out.secondName = new(name())
		}
		return out
	}

	for at(tNewline) {
		r.I++
	}
	r.Expect(tWardley)
	for !at(-1) {
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
		case tSize:
			r.I++
			width, height := pair(integer)
			endOfLine()
			ast.size = &sizeAst{width, height}
		case tEvolution:
			r.I++
			stages := []stageAst{stage()}
			r.Expect(tArrow)
			stages = append(stages, stage())
			for r.Accept(tArrow) {
				stages = append(stages, stage())
			}
			endOfLine()
			ast.evolution = stages
		case tAnchor:
			r.I++
			anchor := name()
			visibility, evolution := pair(number)
			endOfLine()
			ast.anchors = append(ast.anchors, anchorAst{anchor, visibility, evolution})
		case tComponent:
			r.I++
			component := componentAst{name: name()}
			component.visibility, component.evolution = pair(number)
			if at(tLabel) {
				component.label = label()
			}
			if at(tLparen) && r.Tokens.Kinds[r.I+1] == tStrategy {
				r.I++
				component.strategy = r.Take()
				r.Expect(tRparen)
			}
			if r.Accept(tInertia) {
				component.inertia = true
			} else if r.Accept(tLparen) {
				r.Expect(tInertia)
				r.Expect(tRparen)
				component.inertia = true
			}
			endOfLine()
			ast.components = append(ast.components, component)
		case tEvolve:
			r.I++
			component := name()
			target := number()
			endOfLine()
			ast.evolves = append(ast.evolves, evolveAst{component, target})
		case tPipeline:
			r.I++
			parent := name()
			r.Expect(tLbrace)
			r.Expect(tNewline)
			for at(tNewline) {
				r.I++
			}
			var components []pipelineComponentAst
			for {
				r.Expect(tComponent)
				component := pipelineComponentAst{name: name()}
				r.Expect(tLbracket)
				component.evolution = number()
				r.Expect(tRbracket)
				if at(tLabel) {
					component.label = label()
				}
				endOfLine()
				components = append(components, component)
				if !at(tComponent) {
					break
				}
			}
			r.Expect(tRbrace)
			endOfLine()
			ast.pipelines = append(ast.pipelines, pipelineAst{parent, components})
		case tNote:
			r.I++
			text := common.StringValue(r.Expect(tString))
			visibility, evolution := pair(number)
			endOfLine()
			ast.notes = append(ast.notes, noteAst{text, visibility, evolution})
		case tAnnotations:
			r.I++
			x, y := pair(coordinate)
			endOfLine()
			ast.annotations = append(ast.annotations, [2]float64{x, y})
		case tAnnotation:
			r.I++
			n := integer()
			r.Expect(tComma)
			x, y := pair(coordinate)
			text := common.StringValue(r.Expect(tString))
			endOfLine()
			ast.annotation = append(ast.annotation, annotationAst{n, x, y, text})
		case tAccelerator, tDeaccelerator:
			slow := r.Kind() == tDeaccelerator
			r.I++
			force := name()
			x, y := pair(number)
			endOfLine()
			if slow {
				ast.deaccelerators = append(ast.deaccelerators, placedAst{force, x, y})
			} else {
				ast.accelerators = append(ast.accelerators, placedAst{force, x, y})
			}
		default:
			if !isName() {
				r.Fail("a statement")
			}
			link := linkAst{from: name()}
			if at(tLinkPort) {
				link.fromPort = r.Take()
			}
			if at(tLinkArrow) || at(tArrow) {
				link.arrow = new(r.Take())
			}
			link.to = name()
			if at(tLinkPort) {
				link.toPort = r.Take()
			}
			if at(tLinkLabel) {
				link.linkLabel = new(jsstr.Trim(r.Take()[1:]))
			}
			endOfLine()
			ast.links = append(ast.links, link)
		}
	}
	return ast
}
