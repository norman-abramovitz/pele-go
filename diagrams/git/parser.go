package git

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// Token kinds, in the order of gitTokens.
const (
	tCherryPick = iota
	tGitGraphColon
	tHighlight
	tGitGraph
	tCheckout
	tReverse
	tParent
	tCommit
	tNormal
	tBranch
	tOrder
	tSwitch
	tType
	tMerge
	tMsg
	tTag
	tID
	tLR
	tTB
	tBT
	tColon
	tAccDescr
	tAccTitle
	tTitle
	tInt
	tString
	tNewline
	tReference = 31
)

const wordChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz"

// reference is /\w([-\./\w]*[-\w])?/y: the run of name characters, backed off to its last
// character that may end a name.
var reference = &common.TokenType{Name: "REFERENCE", First: wordChars, Match: func(src common.Src, at int) int {
	if !util.IsWord(src.At(at)) {
		return -1
	}
	end := at + 1
	for i := at + 1; ; i++ {
		c := src.At(i)
		if c == '-' || util.IsWord(c) {
			end = i + 1
		} else if c != '.' && c != '/' {
			break
		}
	}
	return end
}}

var names = []*common.TokenType{reference}

// word is a keyword that is the start of a longer name is part of that name: `commits` is a
// branch.
func word(name string) *common.TokenType {
	return &common.TokenType{Name: name, Literal: name, Longer: names}
}

func sign(name string) *common.TokenType {
	return &common.TokenType{Name: name, Literal: name}
}

// accDescrOpener is /[\t ]*accDescr\s*{/y.
func accDescrOpener(src common.Src, at int) int {
	i := at
	for src.At(i) == ' ' || src.At(i) == '\t' {
		i++
	}
	if !src.HasPrefix(i, "accDescr") {
		return -1
	}
	i += len("accDescr")
	for c := src.At(i); c >= 0 && jsstr.IsSpace(rune(c)); c = src.At(i) {
		i++
	}
	if src.At(i) != '{' {
		return -1
	}
	return i + 1
}

// intToken is /0|[1-9][0-9]*(?!\.)/y.
var intToken = &common.TokenType{Name: "INT", First: "0123456789", Match: func(src common.Src, at int) int {
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
	// Backing off one digit leaves a digit after the match.
	if end-1 > at {
		return end - 1
	}
	return -1
}}

var gitLexer = func() *common.Lexer {
	gitGraph := *common.Keyword("gitGraph")
	gitGraph.Longer = names
	accDescr := *common.AccDescr
	accDescr.Opener = accDescrOpener
	return common.NewLexer([]*common.TokenType{
		word("cherry-pick"),
		sign("gitGraph:"),
		word("HIGHLIGHT"),
		&gitGraph,
		word("checkout"),
		word("REVERSE"),
		sign("parent:"),
		word("commit"),
		word("NORMAL"),
		word("branch"),
		sign("order:"),
		word("switch"),
		sign("type:"),
		word("merge"),
		sign("msg:"),
		sign("tag:"),
		sign("id:"),
		word("LR"),
		word("TB"),
		word("BT"),
		sign(":"),
		// Without a closing brace the block form scans to the end of the text, and a brace is
		// not a token on its own, so the first failure is final.
		&accDescr,
		common.AccTitle,
		common.Title,
		intToken,
		common.String,
		common.Newline,
		common.Whitespace,
		common.Yaml,
		common.Directive,
		common.SingleLineComment,
		reference,
	})
}()

type statementKind int

const (
	stCommit statementKind = iota
	stBranch
	stMerge
	stCheckout
	stCherryPicking
)

// statementAst is upstream's StatementAst union; kind picks the fields that apply. Nil
// pointers are upstream's undefined.
type statementAst struct {
	kind    statementKind
	id      *string
	message *string
	tags    []string
	typ     *string
	// name is a branch's name, or the branch a merge or checkout names.
	name   string
	order  *float64
	parent *string
}

type gitGraphAst struct {
	dir                       string
	title, accTitle, accDescr *string
	statements                []statementAst
}

func parseGit(src string, o *perr.Origin) gitGraphAst {
	r := common.NewReader(gitLexer.Tokenize(jsstr.UTF16(src), "gitGraph", o), gitLexer.Types(), "gitGraph", o)
	var a gitGraphAst

	endOfLine := func() {
		if r.Kind() == -1 {
			return
		}
		if r.Kind() != tNewline {
			r.Fail("a line break or the end of input")
		}
		for r.Kind() == tNewline {
			r.I++
		}
	}
	name := func() string {
		switch r.Kind() {
		case tReference:
			return r.Take()
		case tString:
			return common.StringValue(r.Take())
		}
		r.Fail("a branch name")
		return ""
	}
	str := func() string { return common.StringValue(r.Expect(tString)) }
	commitType := func() string {
		if k := r.Kind(); k != tNormal && k != tReverse && k != tHighlight {
			r.Fail("NORMAL, REVERSE, or HIGHLIGHT")
		}
		return r.Take()
	}

	for r.Kind() == tNewline {
		r.I++
	}
	if !r.Accept(tGitGraphColon) {
		r.Expect(tGitGraph)
		next := -1
		if r.I+1 < len(r.Tokens.Kinds) {
			next = r.Tokens.Kinds[r.I+1]
		}
		if k := r.Kind(); (k == tLR || k == tTB || k == tBT) && next == tColon {
			a.dir = r.Take()
			r.I++
		} else {
			r.Accept(tColon)
		}
	}

	for r.Kind() != -1 {
		switch r.Kind() {
		case tNewline:
			r.I++
		case tAccDescr:
			a.accDescr = new(common.AccDescrValue(r.Take()))
			endOfLine()
		case tAccTitle:
			a.accTitle = new(common.AccTitleValue(r.Take()))
			endOfLine()
		case tTitle:
			a.title = new(common.TitleValue(r.Take()))
			endOfLine()
		case tCommit:
			r.I++
			node := statementAst{kind: stCommit, tags: []string{}}
			for {
				if r.Accept(tID) {
					node.id = new(str())
				} else if r.Accept(tTag) {
					node.tags = append(node.tags, str())
				} else if r.Accept(tType) {
					node.typ = new(commitType())
				} else if r.Accept(tMsg) || r.Kind() == tString {
					node.message = new(str())
				} else {
					break
				}
			}
			endOfLine()
			a.statements = append(a.statements, node)
		case tBranch:
			r.I++
			node := statementAst{kind: stBranch, name: name()}
			if r.Accept(tOrder) {
				node.order = new(jsnum.ParseInt(r.Expect(tInt)))
			}
			endOfLine()
			a.statements = append(a.statements, node)
		case tMerge:
			r.I++
			node := statementAst{kind: stMerge, name: name(), tags: []string{}}
			for {
				if r.Accept(tID) {
					node.id = new(str())
				} else if r.Accept(tTag) {
					node.tags = append(node.tags, str())
				} else if r.Accept(tType) {
					node.typ = new(commitType())
				} else {
					break
				}
			}
			endOfLine()
			a.statements = append(a.statements, node)
		case tCheckout, tSwitch:
			r.I++
			node := statementAst{kind: stCheckout, name: name()}
			endOfLine()
			a.statements = append(a.statements, node)
		case tCherryPick:
			r.I++
			node := statementAst{kind: stCherryPicking, tags: []string{}}
			for {
				if r.Accept(tID) {
					node.id = new(str())
				} else if r.Accept(tTag) {
					node.tags = append(node.tags, str())
				} else if r.Accept(tParent) {
					node.parent = new(str())
				} else {
					break
				}
			}
			endOfLine()
			a.statements = append(a.statements, node)
		default:
			r.Fail("a statement, a title, or a line break")
		}
	}
	return a
}
