package common

import "github.com/Broderick-Westrope/pele-go/internal/util"

// Lexer rules that Mermaid's journey and timeline grammars have in common, ported from the
// compiled Jison rules: comments, whitespace, and the accTitle and accDescr statements.

// TextTokens are tokens as types, texts and start offsets in parallel (upstream's Tokens in
// scan.ts and outline.ts).
type TextTokens struct {
	Types  []int
	Texts  []string
	Starts []int
}

// LineEnd returns the index of the next line feed at or after from, or the end of src.
func LineEnd(src Src, from int) int {
	if at := src.Index("\n", from); at != -1 {
		return at
	}
	return len(src)
}

// orNaN32 is c | 32 for a code unit read with charCodeAt, whose NaN past the end ORs to 32.
func orNaN32(c int) int {
	if c < 0 {
		return 32
	}
	return c | 32
}

// KeywordAt reports whether word, lower-case ASCII letters, is at p in either case; the
// grammars are case-insensitive.
func KeywordAt(src Src, p int, word string) bool {
	for i := 0; i < len(word); i++ {
		if orNaN32(src.At(p+i)) != int(word[i]) {
			return false
		}
	}
	return true
}

// SkipTrivia skips comments and whitespace. It stops at a line break that starts a token,
// which is a NEWLINE. Jison compiles `\%%` to `%`, so a single `%` starts a comment.
func SkipTrivia(src Src, from int) int {
	n := len(src)
	p := from
	for p < n {
		c := int(src[p])
		switch {
		case c == 37 && src.At(p+1) != 123:
			p = LineEnd(src, p)
		case c == 10:
			return p
		case util.IsSpace(c):
			for {
				p++
				if !(p < n && util.IsSpace(int(src[p]))) {
					break
				}
			}
		case c == 35:
			p = LineEnd(src, p)
		default:
			return p
		}
	}
	return p
}

func skipSpace(src Src, from int) int {
	p := from
	for util.IsSpace(src.At(p)) {
		p++
	}
	return p
}

// Accessibility lexes an accTitle or accDescr statement at p and returns where it ends, or
// p if there is none. The token types acc_title, acc_title_value, acc_descr,
// acc_descr_value and acc_descr_multiline_value must be numbered consecutively from first.
func Accessibility(src Src, p int, out *TextTokens, first int) int {
	descr := KeywordAt(src, p, "accdescr")
	if !descr && !KeywordAt(src, p, "acctitle") {
		return p
	}
	n := len(src)
	q := skipSpace(src, p+8)
	c := src.At(q)
	emit := func(typ, s, e int) {
		out.Types = append(out.Types, typ)
		out.Texts = append(out.Texts, src.String(s, e))
		out.Starts = append(out.Starts, s)
	}
	if c == 58 {
		q = skipSpace(src, q+1)
		end := LineEnd(src, q)
		if descr {
			emit(first+2, p, q)
			emit(first+3, q, end)
		} else {
			emit(first, p, q)
			emit(first+1, q, end)
		}
		return end
	}
	if !descr || c != 123 {
		return p
	}
	q = skipSpace(src, q+1)
	for src.At(q) != 125 {
		end := src.Index("}", q)
		if end == -1 {
			end = n
		}
		emit(first+4, q, end)
		if q == n {
			return n
		}
		q = end
	}
	return q + 1
}
