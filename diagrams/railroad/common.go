package railroad

import (
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"math"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// What the four railroad grammars share: the title and accessibility tokens (which, unlike
// the ones in Mermaid's common grammar, do not take leading blanks), the hidden tokens, the
// statements before the first rule, string values, and the walk from a syntax tree to the
// rule model.

var inf = math.Inf(1)

func isSpace(c int) bool { return util.IsSpace(c) }

const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// The common matchers skip leading blanks first; with these first characters they are only
// tried where there are none, so they match as upstream's railroad patterns do.
var (
	titleToken    = &common.TokenType{Name: "TITLE", First: "t", Match: common.Title.Match}
	accTitleToken = &common.TokenType{Name: "ACC_TITLE", First: "a", Match: common.AccTitle.Match}
)

func accDescrToken() *common.TokenType {
	t := *common.AccDescrScanner("ACC_DESCR")
	t.First = "a"
	return &t
}

func literal(name string, longer ...*common.TokenType) *common.TokenType {
	return &common.TokenType{Name: name, Literal: name, Longer: longer}
}

// opening is the diagram's keyword. A name that starts with it and goes on is a name.
func opening(word string, name *common.TokenType) *common.TokenType {
	t := common.Keyword(word)
	t.Longer = []*common.TokenType{name}
	return t
}

// identifier is /[A-Z_a-z][\w-]*/y.
func identifier(name string) *common.TokenType {
	return &common.TokenType{Name: name, First: letters + "_", Match: func(src common.Src, at int) int {
		c := src.At(at)
		if !(util.IsLetter(c) || c == '_') {
			return -1
		}
		i := at + 1
		for util.IsWord(src.At(i)) || src.At(i) == '-' {
			i++
		}
		return i
	}}
}

// quoted is /"([^"\\]|\\.)*"|'([^'\\]|\\.)*'/y.
func quoted(name string) *common.TokenType {
	return &common.TokenType{Name: name, First: `"'`, Match: common.String.Match}
}

// hiddenTokens are whitespace, front matter, directive, and `%%` comment, in the order every
// railroad grammar lists them.
func hiddenTokens(prefix string) []*common.TokenType {
	yaml := *common.Yaml
	yaml.Name = prefix + "_YAML"
	directive := *common.Directive
	directive.Name = prefix + "_DIRECTIVE"
	comment := *common.SingleLineComment
	comment.Name = prefix + "_SINGLE_LINE_COMMENT"
	return []*common.TokenType{
		{Name: prefix + "_WHITESPACE", First: "\t \r\n", Hidden: true, Match: func(src common.Src, at int) int {
			i := at
			for c := src.At(i); c == '\t' || c == ' ' || c == '\r' || c == '\n'; c = src.At(i) {
				i++
			}
			if i == at {
				return -1
			}
			return i
		}},
		&yaml,
		&directive,
		&comment,
	}
}

// blockComment matches from open to the first close after it, as /open[\s\S]*?close/y with
// the opener /open/y.
func blockComment(name, open, close string) *common.TokenType {
	first := open[:1]
	return &common.TokenType{
		Name:   name,
		Hidden: true,
		First:  first,
		Opener: func(src common.Src, at int) int {
			if !src.HasPrefix(at, open) {
				return -1
			}
			return at + len(open)
		},
		Match: func(src common.Src, at int) int {
			if !src.HasPrefix(at, open) {
				return -1
			}
			end := src.Index(close, at+len(open))
			if end == -1 {
				return -1
			}
			return end + len(close)
		},
	}
}

// lineComment is /lead[^\n\r]*/y.
func lineComment(name string, lead byte) *common.TokenType {
	return &common.TokenType{Name: name, Hidden: true, First: string(lead), Match: func(src common.Src, at int) int {
		if src.At(at) != int(lead) {
			return -1
		}
		i := at + 1
		for i < len(src) && src[i] != '\n' && src[i] != '\r' {
			i++
		}
		return i
	}}
}

// unescape is Mermaid's railroad value converter: it knows three escapes; any other escaped
// character stands for itself.
func unescape(text string) string {
	units := jsstr.UTF16(text)
	end := len(units) - 1
	if !containsBackslash(units) {
		return sliceInner(text)
	}
	out := make([]uint16, 0, len(units))
	for i := 1; i < end; i++ {
		c := units[i]
		if c == '\\' && i+1 < end {
			i++
			c = units[i]
			switch c {
			case 'n':
				c = '\n'
			case 'r':
				c = '\r'
			case 't':
				c = '\t'
			}
		}
		out = append(out, c)
	}
	return jsstr.FromUTF16(out)
}

// sliceInner is text.slice(1, -1).
func sliceInner(text string) string {
	return jsstr.Slice(text, 1, jsstr.Len(text)-1)
}

func containsBackslash(units []uint16) bool {
	for _, c := range units {
		if c == '\\' {
			return true
		}
	}
	return false
}

// syntax is a node of any of the four syntax trees. parts are what upstream's parts()
// returns for it, in that order.
type syntax struct {
	kind int
	// text is a leaf's value, or the operator or repeat written with the node ("" for none).
	text  string
	parts []*syntax
	// postfixes are an EBNF term's postfix operators, as token kinds.
	postfixes []int
}

type syntaxRule struct {
	name       string
	definition *syntax
}

// parseGrammar parses the keyword, then any titles and accessibility statements, then rules
// to the end of the text. Every table starts with its keyword and has TITLE, ACC_TITLE, and
// ACC_DESCR in a row from meta.
func parseGrammar(src string, o *perr.Origin, lexer *common.Lexer, meta int, unquote func(string) string, rule func(r *common.Reader) syntaxRule) (parsed, []syntaxRule) {
	r := common.NewReader(lexer.Tokenize(jsstr.UTF16(src), "railroad", o), lexer.Types(), "railroad", o)
	var ast parsed
	var rules []syntaxRule
	r.Expect(0)
	for {
		kind := r.Kind()
		if kind == meta {
			title := common.TitleValue(r.Take())
			if title != "" && (title[0] == '"' || title[0] == '\'') && title[len(title)-1] == title[0] {
				ast.title = unquote(title)
			} else {
				ast.title = title
			}
		} else if kind == meta+1 {
			ast.accTitle = common.AccTitleValue(r.Take())
		} else if kind == meta+2 {
			ast.accDescr = common.AccDescrValue(r.Take())
		} else {
			break
		}
	}
	for r.Kind() != -1 {
		rules = append(rules, rule(r))
	}
	return ast, rules
}

func terminal(value string) *node   { return &node{typ: nTerminal, value: value} }
func nonterminal(name string) *node { return &node{typ: nNonTerminal, value: name} }
func special(text string) *node     { return &node{typ: nSpecial, value: text} }
func optional(element *node) *node  { return &node{typ: nOptional, kids: []*node{element}} }
func repetition(element *node, min, max float64) *node {
	return &node{typ: nRepetition, kids: []*node{element}, min: min, max: max}
}

// sequence and choice of one are that one, in every notation.
func sequence(elements []*node) *node {
	if len(elements) == 1 {
		return elements[0]
	}
	return &node{typ: nSequence, kids: elements}
}

func choice(alternatives []*node) *node {
	if len(alternatives) == 1 {
		return alternatives[0]
	}
	return &node{typ: nChoice, kids: alternatives}
}

// toRules turns each rule's syntax tree into the common model, given how one node is built
// from its parts.
func toRules(ast parsed, rules []syntaxRule, build func(part *syntax, kids []*node) *node) parsed {
	ast.rules = make([]rule, len(rules))
	for i, r := range rules {
		ast.rules[i] = rule{name: r.name, definition: fold(r.definition, func(s *syntax) []*syntax { return s.parts }, build)}
	}
	return ast
}

// fold rebuilds a tree bottom-up without recursion, so that nesting depth is bounded by
// memory, not by the stack.
func fold[T, R any](root T, children func(T) []T, build func(T, []R) R) R {
	nodes := []T{root}
	lists := [][]T{children(root)}
	done := [][]R{nil}
	for {
		top := len(nodes) - 1
		results := done[top]
		if len(results) < len(lists[top]) {
			child := lists[top][len(results)]
			nodes = append(nodes, child)
			lists = append(lists, children(child))
			done = append(done, nil)
			continue
		}
		value := build(nodes[top], results)
		if top == 0 {
			return value
		}
		nodes = nodes[:top]
		lists = lists[:top]
		done = done[:top]
		done[top-1] = append(done[top-1], value)
	}
}
