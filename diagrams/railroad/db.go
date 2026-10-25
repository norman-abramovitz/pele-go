package railroad

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// railroadDb is upstream's RailroadDb. Empty strings stand for undefined: every use treats
// the two alike. Upstream's lookup by name serves only its tests and is left out.
type railroadDb struct {
	// notation is which of Mermaid's four grammars the text was written in: railroad, ebnf,
	// abnf or peg.
	notation                  string
	title, accTitle, accDescr string
	// rules are every rule in source order. A name defined twice appears twice.
	rules []rule
}

func (db *railroadDb) setTitle(text string) {
	db.title = text
}

func (db *railroadDb) setAccTitle(text string) {
	db.accTitle = jsstr.TrimStart(text)
}

func (db *railroadDb) setAccDescription(text string) {
	db.accDescr = common.CollapseIndentedBreaks(text)
}

func (db *railroadDb) addRule(r rule) {
	db.rules = append(db.rules, r)
}

// notationOf is /^\s*railroad-(ebnf-|abnf-|peg-)?beta/i: Mermaid has a detector per
// keyword, each matching without regard to case. "" is upstream's undefined.
func notationOf(source string) string {
	i := 0
	for i < len(source) {
		r, n := jsstr.Decode(source, i)
		if !jsstr.IsSpace(r) {
			break
		}
		i += n
	}
	rest := source[i:]
	if !hasPrefixFold(rest, "railroad-") {
		return ""
	}
	rest = rest[len("railroad-"):]
	for _, infix := range [...]string{"ebnf", "abnf", "peg"} {
		if hasPrefixFold(rest, infix+"-") && hasPrefixFold(rest[len(infix)+1:], "beta") {
			return infix
		}
	}
	if hasPrefixFold(rest, "beta") {
		return "railroad"
	}
	return ""
}

// hasPrefixFold compares ASCII letters without regard to case, as a regex with /i and no
// u flag does.
func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if s[i]|0x20 != prefix[i]|0x20 || (s[i] != prefix[i] && !isASCIILetter(prefix[i])) {
			return false
		}
	}
	return true
}

func isASCIILetter(c byte) bool {
	return c|0x20 >= 'a' && c|0x20 <= 'z'
}

type parsed struct {
	title, accTitle, accDescr string
	rules                     []rule
}

func grammar(source, notation string, o *perr.Origin) parsed {
	switch notation {
	case "ebnf":
		ast, rules := parseEbnf(source, o)
		return toRules(ast, rules, ebnfNode)
	case "abnf":
		ast, rules := parseAbnf(source, o)
		return toRules(ast, rules, abnfNode)
	case "peg":
		ast, rules := parsePeg(source, o)
		return toRules(ast, rules, pegNode)
	default:
		ast, rules := parseIr(source, o)
		return toRules(ast, rules, irNode)
	}
}

// populate parses the text in one notation and loads the result into the model, as
// Mermaid's four parsers do.
func populate(db *railroadDb, source, notation string, o *perr.Origin) {
	ast := grammar(source, notation, o)
	db.notation = notation
	if ast.accDescr != "" {
		db.setAccDescription(ast.accDescr)
	}
	if ast.accTitle != "" {
		db.setAccTitle(ast.accTitle)
	}
	if ast.title != "" {
		db.setTitle(ast.title)
	}
	for _, r := range ast.rules {
		db.addRule(r)
	}
}

func parseRailroad(source string, title *string, o *perr.Origin) *railroadDb {
	notation := notationOf(source)
	if notation == "" {
		perr.Throw(&perr.Error{Code: perr.Syntax, Message: "No railroad diagram keyword found.", Type: "railroad"})
	}
	db := &railroadDb{notation: "railroad"}
	if title != nil && *title != "" {
		db.title = *title
	}
	populate(db, source, notation, o)
	return db
}
