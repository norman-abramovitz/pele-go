package railroad

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// `railroad-ebnf-beta`: EBNF in the W3C and ISO 14977 styles.

// Token kinds, in the order of ebnfLexer's types.
const (
	ebnfKeyword = iota
	ebnfDefines
	ebnfEquals
	ebnfSemicolon
	ebnfBar
	ebnfComma
	ebnfGroup
	ebnfGroupEnd
	ebnfOption
	ebnfOptionEnd
	ebnfRepeat
	ebnfRepeatEnd
	ebnfQuestion
	ebnfStar
	ebnfPlus
	ebnfMinus
	ebnfTitle
	ebnfAccTitle
	ebnfAccDescr
	ebnfID
	ebnfString
	ebnfSpecial
)

// Syntax node kinds besides the tokens that start a primary.
const (
	ebnfChoiceNode = iota + 100
	ebnfSequenceNode
	ebnfTermNode
)

// specialSequence is /\?(?=[^?;]*[^?\s;][^?;]*\?)[^?;]*\?/y: a question mark, text with
// something other than blanks in it and no `?` or `;`, and a question mark.
func specialSequence(src common.Src, at int) int {
	if src.At(at) != '?' {
		return -1
	}
	solid := false
	for i := at + 1; i < len(src); i++ {
		c := int(src[i])
		if c == '?' {
			if solid {
				return i + 1
			}
			return -1
		}
		if c == ';' {
			return -1
		}
		solid = solid || !isSpace(c)
	}
	return -1
}

var ebnfLexer = func() *common.Lexer {
	id := identifier("EBNF_ID")
	special := &common.TokenType{Name: "EBNF_SPECIAL_SEQUENCE", First: "?", Match: specialSequence}
	hidden := hiddenTokens("EBNF")
	isoComment := blockComment("EBNF_ISO_COMMENT", "(*", "*)")
	types := []*common.TokenType{
		opening("railroad-ebnf-beta", id),
		literal("::="),
		literal("="),
		literal(";"),
		literal("|"),
		literal(","),
		literal("(", isoComment),
		literal(")"),
		literal("["),
		literal("]"),
		literal("{"),
		literal("}"),
		literal("?", special),
		literal("*"),
		literal("+"),
		literal("-", hidden[1]),
		titleToken,
		accTitleToken,
		accDescrToken(),
		id,
		quoted("EBNF_STRING"),
		special,
	}
	types = append(types, hidden...)
	types = append(types, blockComment("EBNF_BLOCK_COMMENT", "/*", "*/"), isoComment)
	return common.NewLexer(types)
}()

// ebnfFrame is one open bracket, or the rule itself at the bottom of the stack.
type ebnfFrame struct {
	choice   *syntax
	sequence *syntax
	term     *syntax
	// node is the bracket's node, close its closing token, and except whether it is the
	// right side of an exception.
	node   *syntax
	close  int
	except bool
}

func newEbnfFrame(n, choice *syntax, close int, except bool) *ebnfFrame {
	sequence := &syntax{kind: ebnfSequenceNode}
	choice.parts = append(choice.parts, sequence)
	return &ebnfFrame{choice: choice, sequence: sequence, node: n, close: close, except: except}
}

func isEbnfStart(k int) bool {
	return k == ebnfString || k == ebnfID || k == ebnfSpecial || k == ebnfGroup || k == ebnfOption || k == ebnfRepeat
}

func ebnfDefinition(r *common.Reader) *syntax {
	var stack []*ebnfFrame
	top := newEbnfFrame(nil, &syntax{kind: ebnfChoiceNode}, -2, false)
	except := false
	for {
		var primary *syntax
		switch kind := r.Kind(); kind {
		case ebnfString:
			primary = &syntax{kind: kind, text: unescape(r.Take())}
		case ebnfID:
			primary = &syntax{kind: kind, text: r.Take()}
		case ebnfSpecial:
			primary = &syntax{kind: kind, text: jsstr.Trim(sliceInner(r.Take()))}
		case ebnfGroup, ebnfOption, ebnfRepeat:
			r.I++
			element := &syntax{kind: ebnfChoiceNode}
			stack = append(stack, top)
			top = newEbnfFrame(&syntax{kind: kind, parts: []*syntax{element}}, element, kind+1, except)
			except = false
			continue
		default:
			r.Fail("a terminal, a name, or an opening bracket")
		}
		for {
			if except {
				top.term.parts = append(top.term.parts, primary)
				top.term.postfixes = append(top.term.postfixes, ebnfMinus)
				except = false
			} else {
				top.term = &syntax{kind: ebnfTermNode, parts: []*syntax{primary}}
				top.sequence.parts = append(top.sequence.parts, top.term)
			}
			for {
				k := r.Kind()
				if k != ebnfQuestion && k != ebnfStar && k != ebnfPlus {
					break
				}
				r.I++
				top.term.postfixes = append(top.term.postfixes, k)
			}
			k := r.Kind()
			if k == top.close {
				r.I++
				primary = top.node
				except = top.except
				top = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				continue
			}
			switch {
			case k == ebnfMinus:
				r.I++
				except = true
			case k == ebnfComma:
				r.I++
			case k == ebnfBar:
				r.I++
				top.sequence = &syntax{kind: ebnfSequenceNode}
				top.choice.parts = append(top.choice.parts, top.sequence)
			case !isEbnfStart(k):
				if len(stack) == 0 {
					return top.choice
				}
				r.Fail("token of type '" + r.Types[top.close].Name + "'")
			}
			break
		}
	}
}

func parseEbnf(src string, o *perr.Origin) (parsed, []syntaxRule) {
	return parseGrammar(src, o, ebnfLexer, ebnfTitle, unescape, func(r *common.Reader) syntaxRule {
		name := r.Expect(ebnfID)
		if !r.Accept(ebnfEquals) {
			r.Expect(ebnfDefines)
		}
		definition := ebnfDefinition(r)
		r.Expect(ebnfSemicolon)
		return syntaxRule{name: name, definition: definition}
	})
}

func ebnfNode(part *syntax, kids []*node) *node {
	switch part.kind {
	case ebnfChoiceNode:
		return choice(kids)
	case ebnfSequenceNode:
		return sequence(kids)
	case ebnfTermNode:
		// Each postfix operator applies to everything before it in the term.
		result := kids[0]
		next := 1
		for _, postfix := range part.postfixes {
			switch postfix {
			case ebnfQuestion:
				result = optional(result)
			case ebnfMinus:
				result = &node{typ: nSequence, kids: []*node{result, terminal("-"), kids[next]}}
				next++
			default:
				min := 0.0
				if postfix == ebnfPlus {
					min = 1
				}
				result = repetition(result, min, inf)
			}
		}
		return result
	case ebnfString:
		return terminal(part.text)
	case ebnfID:
		return nonterminal(part.text)
	case ebnfSpecial:
		return special(part.text)
	case ebnfGroup:
		return kids[0]
	case ebnfOption:
		return optional(kids[0])
	default:
		return repetition(kids[0], 0, inf)
	}
}
