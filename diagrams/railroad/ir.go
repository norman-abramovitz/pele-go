package railroad

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// `railroad-beta`: Mermaid's own notation, one constructor call per node.

// Token kinds, in the order of irLexer's types. A syntax node's kind is the token that
// starts it.
const (
	irKeyword = iota
	irNonterminal
	irZeroOrMore
	irOneOrMore
	irSequence
	irOptional
	irTerminal
	irSpecial
	irChoice
	irEquals
	irSemicolon
	irOpen
	irComma
	irClose
	irTitle
	irAccTitle
	irAccDescr
	irID
	irString
)

var irLexer = func() *common.Lexer {
	id := identifier("RR_ID")
	word := func(name string) *common.TokenType { return literal(name, id) }
	types := []*common.TokenType{
		opening("railroad-beta", id),
		word("nonterminal"),
		word("zeroOrMore"),
		word("oneOrMore"),
		word("sequence"),
		word("optional"),
		word("terminal"),
		word("special"),
		word("choice"),
		literal("="),
		literal(";"),
		literal("("),
		literal(","),
		literal(")"),
		titleToken,
		accTitleToken,
		accDescrToken(),
		id,
		quoted("RR_STRING"),
	}
	types = append(types, hiddenTokens("RR")...)
	types = append(types, blockComment("RR_BLOCK_COMMENT", "/*", "*/"))
	return common.NewLexer(types)
}()

func argument(r *common.Reader) string {
	r.I++
	r.Expect(irOpen)
	value := unescape(r.Expect(irString))
	r.Expect(irClose)
	return value
}

func irExpression(r *common.Reader) *syntax {
	var open []*syntax
	for {
		var value *syntax
		switch kind := r.Kind(); kind {
		case irTerminal, irNonterminal, irSpecial:
			value = &syntax{kind: kind, text: argument(r)}
		case irSequence, irChoice, irOptional, irOneOrMore, irZeroOrMore:
			r.I++
			r.Expect(irOpen)
			open = append(open, &syntax{kind: kind})
			continue
		default:
			r.Fail("an expression")
		}
		for {
			if len(open) == 0 {
				return value
			}
			parent := open[len(open)-1]
			parent.parts = append(parent.parts, value)
			if (parent.kind == irSequence || parent.kind == irChoice) && r.Accept(irComma) {
				break
			}
			r.Expect(irClose)
			open = open[:len(open)-1]
			value = parent
		}
	}
}

func parseIr(src string, o *perr.Origin) (parsed, []syntaxRule) {
	return parseGrammar(src, o, irLexer, irTitle, unescape, func(r *common.Reader) syntaxRule {
		name := r.Expect(irID)
		r.Expect(irEquals)
		definition := irExpression(r)
		r.Expect(irSemicolon)
		return syntaxRule{name: name, definition: definition}
	})
}

func irNode(e *syntax, kids []*node) *node {
	switch e.kind {
	case irTerminal:
		return terminal(e.text)
	case irNonterminal:
		return nonterminal(e.text)
	case irSpecial:
		return special(e.text)
	case irSequence:
		return sequence(kids)
	case irChoice:
		return choice(kids)
	case irOptional:
		return optional(kids[0])
	case irOneOrMore:
		return repetition(kids[0], 1, inf)
	default:
		return repetition(kids[0], 0, inf)
	}
}
