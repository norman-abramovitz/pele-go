package railroad

import (
	"errors"
	"strings"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// dump writes a rule definition compactly: "t" for a terminal, n for a non-terminal, ?s? for
// a special, and seq, alt, opt and rep(min,max) around their parts.
func dump(n *node) string {
	switch n.typ {
	case nTerminal:
		return `"` + n.value + `"`
	case nNonTerminal:
		return n.value
	case nSpecial:
		return "?" + n.value + "?"
	}
	parts := make([]string, len(n.kids))
	for i, kid := range n.kids {
		parts[i] = dump(kid)
	}
	inner := strings.Join(parts, " ")
	switch n.typ {
	case nSequence:
		return "seq(" + inner + ")"
	case nChoice:
		return "alt(" + inner + ")"
	case nOptional:
		return "opt(" + inner + ")"
	}
	return "rep" + jsnum.Format(n.min) + "," + jsnum.Format(n.max) + "(" + inner + ")"
}

func dumpRules(m *railroadDb) string {
	var lines []string
	for _, r := range m.rules {
		lines = append(lines, r.name+" = "+dump(r.definition))
	}
	return strings.Join(lines, "\n")
}

func TestParseRules(t *testing.T) {
	tests := map[string]struct {
		src      string
		notation string
		want     string
	}{
		"ir functions": {
			src:      "railroad-beta\nrule = sequence(terminal(\"a\"), nonterminal(\"b\"), choice(special(\"c\"), optional(terminal(\"d\")))) ;",
			notation: "railroad",
			want:     `rule = seq("a" b alt(?c? opt("d")))`,
		},
		"ir repetitions": {
			src:      "railroad-beta\nr1 = oneOrMore(terminal(\"b\")) ;\nr2 = zeroOrMore(terminal(\"c\\n\")) ;",
			notation: "railroad",
			want:     "r1 = rep1,Infinity(\"b\")\nr2 = rep0,Infinity(\"c\n\")",
		},
		"ebnf postfixes and exceptions": {
			src:      "railroad-ebnf-beta\nr ::= a?, b*, c+, d - e ;\ns = [f] {g} (h | i) ? special ? ;",
			notation: "ebnf",
			want:     "r = seq(opt(a) rep0,Infinity(b) rep1,Infinity(c) seq(d \"-\" e))\ns = seq(opt(f) rep0,Infinity(g) alt(h i) ?special?)",
		},
		"abnf repeats": {
			src:      "railroad-abnf-beta\nr = *a 1*b 2c 0*1d *4e 2*3f %x41 \"s\" / [g] ;",
			notation: "abnf",
			want:     `r = alt(seq(rep0,Infinity(a) rep1,Infinity(b) rep2,2(c) opt(d) rep0,4(e) rep2,3(f) "%x41" "s") opt(g))`,
		},
		"peg predicates": {
			src:      "railroad-peg-beta\nr <- &a !\"b\" !(c d) e? f* g+ . / h ;",
			notation: "peg",
			want:     `r = alt(seq(?&a? ?!"b"? ?!(...)? opt(e) rep0,Infinity(f) rep1,Infinity(g) ?.?) h)`,
		},
		"duplicate rules": {
			src:      "railroad-ebnf-beta\nr = a ;\nr = b ;",
			notation: "ebnf",
			want:     "r = a\nr = b",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := parseRailroad(tc.src, nil, nil)
			if m.notation != tc.notation {
				t.Errorf("notation = %q, want %q", m.notation, tc.notation)
			}
			if got := dumpRules(m); got != tc.want {
				t.Errorf("rules:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestParseMetadata(t *testing.T) {
	m := parseRailroad("railroad-beta\ntitle Example Grammar\naccTitle: Accessible Railroad\naccDescr: Railroad description\nrule = terminal(\"a\") ;", nil, nil)
	if m.title != "Example Grammar" || m.accTitle != "Accessible Railroad" || m.accDescr != "Railroad description" {
		t.Errorf("metadata = %q, %q, %q", m.title, m.accTitle, m.accDescr)
	}
	quoted := parseRailroad("railroad-ebnf-beta\ntitle \"Quoted\\tTitle\"\nr = a ;", nil, nil)
	if quoted.title != "Quoted\tTitle" {
		t.Errorf("quoted title = %q", quoted.title)
	}
}

func TestParseErrors(t *testing.T) {
	tests := map[string]struct {
		src  string
		want string
	}{
		"no keyword":        {src: "rule = \"a\" ;", want: "No railroad diagram keyword found."},
		"missing semicolon": {src: "railroad-ebnf-beta\nrule = \"a\"", want: "Expecting token of type ';' but found end of input."},
		"unclosed group":    {src: "railroad-ebnf-beta\nrule = ( a ;", want: "Expecting token of type ')' but found `;`."},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				var e *perr.Error
				if err, ok := recover().(error); !ok || !errors.As(err, &e) {
					t.Fatalf("no PeleError")
				}
				if !strings.Contains(e.Message, tc.want) {
					t.Errorf("message = %q, want %q", e.Message, tc.want)
				}
			}()
			parseRailroad(tc.src, nil, nil)
		})
	}
}
