package railroad

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// `railroad-peg-beta`: parsing expression grammars.

// Token kinds, in the order of pegLexer's types.
const (
	pegKeyword = iota
	pegArrow
	pegSemicolon
	pegSlash
	pegAnd
	pegNot
	pegQuestion
	pegStar
	pegPlus
	pegGroup
	pegGroupEnd
	pegDot
	pegTitle
	pegAccTitle
	pegAccDescr
	pegID
	pegString
)

// Syntax node kinds besides the tokens that start a primary.
const (
	pegChoiceNode = iota + 100
	pegSequenceNode
	pegPrefixNode
	pegSuffixNode
)

var pegLexer = func() *common.Lexer {
	id := identifier("PEG_ID")
	types := []*common.TokenType{
		opening("railroad-peg-beta", id),
		literal("<-"),
		literal(";"),
		literal("/"),
		literal("&"),
		literal("!"),
		literal("?"),
		literal("*"),
		literal("+"),
		literal("("),
		literal(")"),
		literal("."),
		titleToken,
		accTitleToken,
		accDescrToken(),
		id,
		quoted("PEG_STRING"),
	}
	types = append(types, hiddenTokens("PEG")...)
	types = append(types, lineComment("PEG_LINE_COMMENT", '#'))
	return common.NewLexer(types)
}()

// pegFrame is one open group, or the rule itself at the bottom of the stack.
type pegFrame struct {
	choice   *syntax
	sequence *syntax
	// node is the group's node and operator the predicate written before it ("" for none).
	node     *syntax
	operator string
}

func newPegFrame(n, choice *syntax, operator string) *pegFrame {
	sequence := &syntax{kind: pegSequenceNode}
	choice.parts = append(choice.parts, sequence)
	return &pegFrame{choice: choice, sequence: sequence, node: n, operator: operator}
}

func pegDefinition(r *common.Reader) *syntax {
	var stack []*pegFrame
	top := newPegFrame(nil, &syntax{kind: pegChoiceNode}, "")
	for {
		operator := ""
		if r.Kind() == pegAnd || r.Kind() == pegNot {
			operator = r.Take()
		}
		var primary *syntax
		switch kind := r.Kind(); kind {
		case pegString:
			primary = &syntax{kind: kind, text: unescape(r.Take())}
		case pegID, pegDot:
			primary = &syntax{kind: kind, text: r.Take()}
		case pegGroup:
			r.I++
			element := &syntax{kind: pegChoiceNode}
			stack = append(stack, top)
			top = newPegFrame(&syntax{kind: kind, parts: []*syntax{element}}, element, operator)
			continue
		default:
			r.Fail("a literal, a name, a dot, or an opening parenthesis")
		}
		for {
			suffix := &syntax{kind: pegSuffixNode, parts: []*syntax{primary}}
			if k := r.Kind(); k == pegQuestion || k == pegStar || k == pegPlus {
				suffix.text = r.Take()
			}
			top.sequence.parts = append(top.sequence.parts, &syntax{kind: pegPrefixNode, text: operator, parts: []*syntax{suffix}})
			k := r.Kind()
			if k == pegGroupEnd && len(stack) > 0 {
				r.I++
				primary = top.node
				operator = top.operator
				top = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				continue
			}
			if k == pegSlash {
				r.I++
				top.sequence = &syntax{kind: pegSequenceNode}
				top.choice.parts = append(top.choice.parts, top.sequence)
			} else if k != pegAnd && k != pegNot && k != pegString && k != pegID && k != pegDot && k != pegGroup {
				if len(stack) == 0 {
					return top.choice
				}
				r.Fail("token of type ')'")
			}
			break
		}
	}
}

func parsePeg(src string, o *perr.Origin) (parsed, []syntaxRule) {
	return parseGrammar(src, o, pegLexer, pegTitle, unescape, func(r *common.Reader) syntaxRule {
		name := r.Expect(pegID)
		r.Expect(pegArrow)
		definition := pegDefinition(r)
		r.Expect(pegSemicolon)
		return syntaxRule{name: name, definition: definition}
	})
}

// label is what a predicate shows of the expression it looks ahead for.
func label(n *node) string {
	switch n.typ {
	case nTerminal:
		return `"` + n.value + `"`
	case nNonTerminal, nSpecial:
		return n.value
	default:
		return "(...)"
	}
}

func pegNode(part *syntax, kids []*node) *node {
	var kid *node
	if len(kids) > 0 {
		kid = kids[0]
	}
	switch part.kind {
	case pegChoiceNode:
		return choice(kids)
	case pegSequenceNode:
		return sequence(kids)
	case pegPrefixNode:
		if part.text != "" {
			return special(part.text + label(kid))
		}
		return kid
	case pegSuffixNode:
		switch part.text {
		case "":
			return kid
		case "?":
			return optional(kid)
		case "+":
			return repetition(kid, 1, inf)
		default:
			return repetition(kid, 0, inf)
		}
	case pegString:
		return terminal(part.text)
	case pegID:
		return nonterminal(part.text)
	case pegDot:
		return special(part.text)
	default:
		return kid
	}
}
