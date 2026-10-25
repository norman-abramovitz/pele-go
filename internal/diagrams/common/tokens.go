// Package common holds what upstream shares between diagram types in src/diagrams/common.
package common

import (
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// Src is diagram source as UTF-16 code units. Lexers read it the way upstream's read a JS
// string, so token offsets, and the line and column of errors, match.
type Src []uint16

// At returns the code unit at i, or -1 past the end.
func (s Src) At(i int) int {
	if i < 0 || i >= len(s) {
		return -1
	}
	return int(s[i])
}

// HasPrefix reports whether s has the ASCII word at i.
func (s Src) HasPrefix(i int, word string) bool {
	if i+len(word) > len(s) {
		return false
	}
	for k := range len(word) {
		if s[i+k] != uint16(word[k]) {
			return false
		}
	}
	return true
}

// Index returns the first index of the ASCII word at or after from, or -1.
func (s Src) Index(word string, from int) int {
	for i := max(from, 0); i+len(word) <= len(s); i++ {
		if s.HasPrefix(i, word) {
			return i
		}
	}
	return -1
}

// String returns s[from:to] as a string.
func (s Src) String(from, to int) string {
	return jsstr.FromUTF16(s[from:to])
}

// Matcher returns the end of a match at at, or -1.
type Matcher func(src Src, at int) int

// TokenType is a terminal: a literal or a matcher standing in for upstream's sticky regex.
// Types are tried in order and the first match wins, as in Chevrotain.
type TokenType struct {
	Name string
	// Literal, when set, is matched exactly. Otherwise Match is used.
	Literal string
	Match   Matcher
	// NewMatch makes a matcher once per text, for a pattern that remembers what it has
	// scanned (upstream's scanner).
	NewMatch func() Matcher
	// Opener is the start of a construct that needs a closer. When it matches but the whole
	// pattern does not, no closer exists in the rest of the text, so the pattern is not
	// tried again.
	Opener Matcher
	Hidden bool
	// First lists every ASCII character a match can start with, when that is known. The
	// type is then tried only at those characters and at characters outside ASCII.
	First string
	// Longer is Chevrotain's LONGER_ALT: types tried at the same place when this one
	// matches, the first that matches more text becoming the token.
	Longer []*TokenType
}

// Tokens is the lexed text: kinds, starts and ends in parallel, ending with kind -1.
type Tokens struct {
	Kinds  []int
	Starts []int
	Ends   []int
	Src    Src
}

type table struct {
	starting [128][]int
	every    []int
	longer   [][]int
}

func prepare(types []*TokenType) *table {
	t := &table{}
	for k := range types {
		t.every = append(t.every, k)
	}
	for c := range 128 {
		for k, typ := range types {
			if typ.Literal != "" {
				if int(typ.Literal[0]) == c {
					t.starting[c] = append(t.starting[c], k)
				}
			} else if typ.First == "" || containsByte(typ.First, byte(c)) {
				t.starting[c] = append(t.starting[c], k)
			}
		}
	}
	t.longer = make([][]int, len(types))
	for k, typ := range types {
		for _, alt := range typ.Longer {
			for j, other := range types {
				if other == alt {
					t.longer[k] = append(t.longer[k], j)
				}
			}
		}
	}
	return t
}

func containsByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

// Lexer tokenizes with a fixed list of token types.
type Lexer struct {
	types []*TokenType
	table *table
}

// NewLexer prepares types for tokenizing.
func NewLexer(types []*TokenType) *Lexer {
	return &Lexer{types: types, table: prepare(types)}
}

// Types returns the lexer's token types, indexed by kind.
func (l *Lexer) Types() []*TokenType { return l.types }

// Tokenize lexes src, throwing upstream's lexical error at the first text no type matches.
func (l *Lexer) Tokenize(src Src, diagram string, o *perr.Origin) Tokens {
	var toks Tokens
	toks.Src = src
	dead := make([]bool, len(l.types))
	scanners := make([]Matcher, len(l.types))
	match := func(k, p int) int {
		if dead[k] {
			return -1
		}
		typ := l.types[k]
		switch {
		case typ.NewMatch != nil:
			if scanners[k] == nil {
				scanners[k] = typ.NewMatch()
			}
			return scanners[k](src, p)
		case typ.Literal != "":
			if src.HasPrefix(p, typ.Literal) {
				return p + len(typ.Literal)
			}
			return -1
		}
		if end := typ.Match(src, p); end != -1 {
			return end
		}
		if typ.Opener != nil && typ.Opener(src, p) != -1 {
			dead[k] = true
		}
		return -1
	}

	n := len(src)
	p := 0
scan:
	for p < n {
		c := src[p]
		candidates := l.table.every
		if c < 128 {
			candidates = l.table.starting[c]
		}
		for _, k := range candidates {
			end := match(k, p)
			if end <= p {
				continue
			}
			kind := k
			for _, alt := range l.table.longer[k] {
				if altEnd := match(alt, p); altEnd > end {
					kind = alt
					end = altEnd
					break
				}
			}
			if !l.types[kind].Hidden {
				toks.Kinds = append(toks.Kinds, kind)
				toks.Starts = append(toks.Starts, p)
				toks.Ends = append(toks.Ends, end)
			}
			p = end
			continue scan
		}
		perr.Throw(perr.SyntaxError(o, diagram, src, p, "", true))
	}
	toks.Kinds = append(toks.Kinds, -1)
	toks.Starts = append(toks.Starts, n)
	toks.Ends = append(toks.Ends, n)
	return toks
}
