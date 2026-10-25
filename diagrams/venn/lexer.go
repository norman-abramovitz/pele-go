package venn

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

const (
	tEnd = iota
	tError
	tEOF
	tNewline
	tVenn
	tTitle
	tSet
	tUnion
	tText
	tIndentText
	tStyle
	tBracketLabel
	tNumeric
	tHexColor
	tRGBColor
	tRGBAColor
	tIdentifier
	tString
	tComma
	tColon
)

var tokenNames = [...]string{
	"$end",
	"INVALID",
	"EOF",
	"NEWLINE",
	"VENN",
	"TITLE",
	"SET",
	"UNION",
	"TEXT",
	"INDENT_TEXT",
	"STYLE",
	"BRACKET_LABEL",
	"NUMERIC",
	"HEXCOLOR",
	"RGBCOLOR",
	"RGBACOLOR",
	"IDENTIFIER",
	"STRING",
	"COMMA",
	"COLON",
}

var keywords = [...]struct {
	word string
	typ  int
}{
	{"venn-beta", tVenn},
	{"set", tSet},
	{"union", tUnion},
	{"text", tText},
	{"style", tStyle},
}

// vennLexer reads tokens one at a time, as Mermaid's Jison lexer for venn diagrams does:
// every rule ignores case, the first rule that matches wins, and a keyword must end at a
// word boundary. An indented `text` line is a different token after a set or union than
// anywhere else, and the parser is what knows which came before, so the lexer asks the db.
type vennLexer struct {
	src common.Src
	db  *vennDb
	// The text and position of the token next returned last.
	text      string
	start     int
	at        int
	lineStart bool
	ended     bool
}

func (l *vennLexer) word(at int, word string) bool {
	for k := 0; k < len(word); k++ {
		c := l.src.At(at + k)
		w := int(word[k])
		if c != w && (c+32 != w || c < 65 || c > 90) {
			return false
		}
	}
	return true
}

func (l *vennLexer) keyword(at int, word string) bool {
	return l.word(at, word) && !util.IsWord(l.src.At(at+len(word)))
}

// color returns the end of `rgb(1, 2, 3)` or `rgba(1, 2, 3, 0.5)` whose parenthesis opens
// at at, or -1.
func (l *vennLexer) color(at, parts int) int {
	src := l.src
	i := at + 1
	for part := 0; part < parts; part++ {
		for util.IsSpace(src.At(i)) {
			i++
		}
		from := i
		for util.IsDigit(src.At(i)) || src.At(i) == 46 {
			i++
		}
		if i == from {
			return -1
		}
		for util.IsSpace(src.At(i)) {
			i++
		}
		want := 44
		if part == parts-1 {
			want = 41
		}
		if src.At(i) != want {
			return -1
		}
		i++
	}
	return i
}

func (l *vennLexer) afterLineFeed(at int) bool {
	k := at - 1
	for l.src.At(k) == 32 || l.src.At(k) == 9 {
		k--
	}
	return l.src.At(k) == 10
}

// blank is s.trim() === ”.
func blank(s common.Src) bool {
	for _, c := range s {
		if !util.IsSpace(int(c)) {
			return false
		}
	}
	return true
}

func (l *vennLexer) next() int {
	src := l.src
	n := len(src)
	for {
		i := l.at
		l.start = i
		l.text = ""
		if i >= n {
			if l.ended {
				return tEnd
			}
			l.ended = true
			return tEOF
		}
		c := int(src[i])
		if c == 37 && src.At(i+1) == 37 && src.At(i+2) != 123 {
			l.at = common.LineEnd(src, i+2)
			continue
		}
		if l.lineStart && l.word(i, "title") && l.afterLineFeed(i) && blank(src[i+5:common.LineEnd(src, i+5)]) {
			// Skip a bare title like a comment, so it doesn't end indented text.
			j := common.LineEnd(src, i+5)
			for src.At(j) == 10 || src.At(j) == 13 {
				j++
			}
			l.at = j
			continue
		}
		if l.lineStart {
			if c == 32 || c == 9 {
				j := i + 1
				for src.At(j) == 32 || src.At(j) == 9 {
					j++
				}
				l.at = j
				if l.keyword(j, "text") && l.db.indent {
					// The word `text` itself is dropped; the indentation stands for it.
					l.at = j + 4
					l.lineStart = false
					l.text = src.String(i, j)
					return tIndentText
				}
				continue
			}
			if c != 10 && c != 13 {
				l.db.indent = false
				l.lineStart = false
			}
		}
		if c == 10 || c == 13 {
			i++
			for src.At(i) == 10 || src.At(i) == 13 {
				i++
			}
			l.lineStart = true
			return l.token(tNewline, i)
		}
		if c == 37 && src.At(i+1) == 37 {
			l.at = common.LineEnd(src, i)
			continue
		}
		if c == 32 || c == 9 {
			i++
			for src.At(i) == 32 || src.At(i) == 9 {
				i++
			}
			l.at = i
			continue
		}

		if (c|32 >= 97 && c|32 <= 122) || c == 95 {
			if sep := src.At(i + 5); l.word(i, "title") && util.IsSpace(sep) && sep != 10 && sep != 13 {
				end := i + 6
				for end < n {
					ch := src[end]
					if ch == 35 || ch == 10 || ch == 59 {
						break
					}
					end++
				}
				if end > i+6 {
					return l.token(tTitle, end)
				}
			}
			for _, kw := range keywords {
				if l.keyword(i, kw.word) {
					return l.token(kw.typ, i+len(kw.word))
				}
			}
			if l.word(i, "rgb") {
				alpha := src.At(i+3)|32 == 97
				open := i + 3
				parts := 3
				if alpha {
					open, parts = i+4, 4
				}
				end := -1
				if src.At(open) == 40 {
					end = l.color(open, parts)
				}
				if end != -1 {
					if alpha {
						return l.token(tRGBAColor, end)
					}
					return l.token(tRGBColor, end)
				}
			}
			i++
			for util.IsWord(src.At(i)) || src.At(i) == 45 {
				i++
			}
			return l.token(tIdentifier, i)
		}
		if c == 91 {
			if src.At(i+1) == 34 {
				quote := src.Index(`"`, i+2)
				if quote == -1 || src.At(quote+1) != 93 {
					return tError
				}
				l.at = quote + 2
				l.text = src.String(i+2, quote)
				return tBracketLabel
			}
			end := i + 1
			for end < n && src[end] != 93 && src[end] != 34 {
				end++
			}
			if end == i+1 || src.At(end) != 93 {
				return tError
			}
			l.at = end + 1
			l.text = jsstr.Trim(src.String(i+1, end))
			return tBracketLabel
		}
		if util.IsDigit(c) || c == 46 || c == 43 || c == 45 {
			if c == 43 || c == 45 {
				i++
			}
			from := i
			for util.IsDigit(src.At(i)) {
				i++
			}
			if i > from {
				if src.At(i) == 46 && util.IsDigit(src.At(i+1)) {
					i += 2
					for util.IsDigit(src.At(i)) {
						i++
					}
				}
			} else if src.At(i) == 46 && util.IsDigit(src.At(i+1)) {
				i += 2
				for util.IsDigit(src.At(i)) {
					i++
				}
			} else {
				return tError
			}
			return l.token(tNumeric, i)
		}
		if c == 35 {
			end := i + 1
			for end < i+9 && util.IsHex(src.At(end)) {
				end++
			}
			if end >= i+4 {
				return l.token(tHexColor, end)
			}
			return tError
		}
		if c == 34 {
			quote := src.Index(`"`, i+1)
			if quote == -1 {
				return tError
			}
			return l.token(tString, quote+1)
		}
		if c == 44 {
			return l.token(tComma, i+1)
		}
		if c == 58 {
			return l.token(tColon, i+1)
		}
		return tError
	}
}

func (l *vennLexer) token(typ, end int) int {
	l.text = l.src.String(l.at, end)
	l.at = end
	return typ
}
