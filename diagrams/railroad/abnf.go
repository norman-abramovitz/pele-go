package railroad

import (
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// `railroad-abnf-beta`: ABNF as in RFC 5234, with each rule closed by a semicolon.

// Token kinds, in the order of abnfLexer's types.
const (
	abnfKeyword = iota
	abnfEquals
	abnfSemicolon
	abnfSlash
	abnfGroup
	abnfGroupEnd
	abnfOption
	abnfOptionEnd
	abnfTitle
	abnfAccTitle
	abnfAccDescr
	abnfName
	abnfString
	abnfNumber
	abnfRepeat
	abnfExact
)

// Syntax node kinds besides the tokens that start a primary.
const (
	abnfAlternationNode = iota + 100
	abnfConcatenationNode
	abnfElementNode
)

const digits = "0123456789"

func runOf(src common.Src, i int, in func(int) bool) int {
	for in(src.At(i)) {
		i++
	}
	return i
}

var abnfLexer = func() *common.Lexer {
	name := &common.TokenType{Name: "ABNF_RULENAME", First: letters, Match: func(src common.Src, at int) int {
		if !util.IsLetter(src.At(at)) {
			return -1
		}
		return runOf(src, at+1, func(c int) bool { return util.IsLetter(c) || util.IsDigit(c) || c == '-' })
	}}
	comment := lineComment("ABNF_COMMENT", ';')
	types := []*common.TokenType{
		opening("railroad-abnf-beta", name),
		literal("="),
		// A semicolon closes a rule only at the end of its line; with anything after it, it
		// starts a comment.
		literal(";", comment),
		literal("/"),
		literal("("),
		literal(")"),
		literal("["),
		literal("]"),
		titleToken,
		accTitleToken,
		accDescrToken(),
		name,
		// /"[^"]*"/y
		{Name: "ABNF_STRING", First: `"`, Match: func(src common.Src, at int) int {
			if src.At(at) != '"' {
				return -1
			}
			end := src.Index(`"`, at+1)
			if end == -1 {
				return -1
			}
			return end + 1
		}},
		// /%[xXdDbB][0-9A-Fa-f]+(?:-[0-9A-Fa-f]+|\.[0-9A-Fa-f]+)*/y
		{Name: "ABNF_NUMVAL", First: "%", Match: func(src common.Src, at int) int {
			if src.At(at) != '%' || !strings.ContainsRune("xXdDbB", rune(max(src.At(at+1), 0))) {
				return -1
			}
			i := runOf(src, at+2, util.IsHex)
			if i == at+2 {
				return -1
			}
			for c := src.At(i); (c == '-' || c == '.') && util.IsHex(src.At(i+1)); c = src.At(i) {
				i = runOf(src, i+1, util.IsHex)
			}
			return i
		}},
		// /[0-9]*\*[0-9]*/y
		{Name: "ABNF_REPEAT", First: digits + "*", Match: func(src common.Src, at int) int {
			i := runOf(src, at, util.IsDigit)
			if src.At(i) != '*' {
				return -1
			}
			return runOf(src, i+1, util.IsDigit)
		}},
		// /[0-9]+/y
		{Name: "ABNF_EXACT_REPEAT", First: digits, Match: func(src common.Src, at int) int {
			if i := runOf(src, at, util.IsDigit); i > at {
				return i
			}
			return -1
		}},
	}
	types = append(types, hiddenTokens("ABNF")...)
	types = append(types, comment)
	return common.NewLexer(types)
}()

// abnfFrame is one open bracket, or the rule itself at the bottom of the stack.
type abnfFrame struct {
	alternation   *syntax
	concatenation *syntax
	// node is the bracket's node, close its closing token, and repeat the repeat written
	// before it ("" for none).
	node   *syntax
	close  int
	repeat string
}

func newAbnfFrame(n, alternation *syntax, close int, repeat string) *abnfFrame {
	concatenation := &syntax{kind: abnfConcatenationNode}
	alternation.parts = append(alternation.parts, concatenation)
	return &abnfFrame{alternation: alternation, concatenation: concatenation, node: n, close: close, repeat: repeat}
}

func isAbnfStart(k int) bool {
	return k == abnfString || k == abnfNumber || k == abnfName || k == abnfGroup || k == abnfOption || k == abnfRepeat || k == abnfExact
}

func abnfAlternation(r *common.Reader) *syntax {
	var stack []*abnfFrame
	top := newAbnfFrame(nil, &syntax{kind: abnfAlternationNode}, -2, "")
	for {
		repeat := ""
		if r.Kind() == abnfRepeat || r.Kind() == abnfExact {
			repeat = r.Take()
		}
		var primary *syntax
		switch kind := r.Kind(); kind {
		case abnfString:
			primary = &syntax{kind: kind, text: sliceInner(r.Take())}
		case abnfNumber, abnfName:
			primary = &syntax{kind: kind, text: r.Take()}
		case abnfGroup, abnfOption:
			r.I++
			element := &syntax{kind: abnfAlternationNode}
			stack = append(stack, top)
			top = newAbnfFrame(&syntax{kind: kind, parts: []*syntax{element}}, element, kind+1, repeat)
			continue
		default:
			r.Fail("a string, a number, a name, or an opening bracket")
		}
		for {
			top.concatenation.parts = append(top.concatenation.parts, &syntax{kind: abnfElementNode, text: repeat, parts: []*syntax{primary}})
			k := r.Kind()
			if k == top.close {
				r.I++
				primary = top.node
				repeat = top.repeat
				top = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				continue
			}
			if k == abnfSlash {
				r.I++
				top.concatenation = &syntax{kind: abnfConcatenationNode}
				top.alternation.parts = append(top.alternation.parts, top.concatenation)
			} else if !isAbnfStart(k) {
				if len(stack) == 0 {
					return top.alternation
				}
				r.Fail("token of type '" + r.Types[top.close].Name + "'")
			}
			break
		}
	}
}

func parseAbnf(src string, o *perr.Origin) (parsed, []syntaxRule) {
	return parseGrammar(src, o, abnfLexer, abnfTitle, sliceInner, func(r *common.Reader) syntaxRule {
		name := r.Expect(abnfName)
		r.Expect(abnfEquals)
		definition := abnfAlternation(r)
		r.Expect(abnfSemicolon)
		return syntaxRule{name: name, definition: definition}
	})
}

func abnfNode(part *syntax, kids []*node) *node {
	switch part.kind {
	case abnfAlternationNode:
		return choice(kids)
	case abnfConcatenationNode:
		return sequence(kids)
	case abnfElementNode:
		repeat := part.text
		if repeat == "" {
			return kids[0]
		}
		star := strings.IndexByte(repeat, '*')
		exact := star == -1
		min := 0.0
		if star > 0 || exact {
			min = jsnum.ParseInt(repeat)
		}
		max := inf
		if exact {
			max = min
		} else if star < len(repeat)-1 {
			max = jsnum.ParseInt(repeat[star+1:])
		}
		if min == 0 && max == 1 {
			return optional(kids[0])
		}
		return repetition(kids[0], min, max)
	case abnfString, abnfNumber:
		return terminal(part.text)
	case abnfName:
		return nonterminal(part.text)
	case abnfGroup:
		return kids[0]
	default:
		return optional(kids[0])
	}
}
