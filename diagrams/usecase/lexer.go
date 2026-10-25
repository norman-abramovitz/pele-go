package usecase

import (
	"strconv"
	"sync"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

type tokens struct {
	types  []int
	starts []int
	ends   []int
}

const (
	classWord        = 1
	classPunctuation = 2
)

var charClass = sync.OnceValue(func() [128]uint8 {
	var class [128]uint8
	for c := 48; c <= 57; c++ {
		class[c] = classWord
	}
	for c := 65; c <= 90; c++ {
		class[c] = classWord
		class[c+32] = classWord
	}
	class[95] = classWord
	for _, ch := range "!#$&*+/=?^|~" {
		class[ch] = classPunctuation
	}
	return class
})

var keywordLength = [...]int{0, 12, 5, 14, 3, 9, 2, 2, 2, 2, 2, 4, 3, 4, 8, 5, 5, 7, 6, 4, 5}

func isWord(c int) bool {
	return c >= 0 && c < 128 && charClass()[c]&classWord != 0
}

// startsWithNoCase: Mermaid's INCLUDE and EXTEND are the only keywords matched without regard
// to case.
func startsWithNoCase(src common.Src, at int, word string) bool {
	for k := 0; k < len(word); k++ {
		if src.At(at+k)|32 != int(word[k]) {
			return false
		}
	}
	return true
}

// keywordAt is the keyword that is a prefix of the text at at, tried in the order of Mermaid's
// token list.
func keywordAt(src common.Src, at, c int) int {
	pick := func(word string, t int) int {
		if src.HasPrefix(at, word) {
			return t
		}
		return 0
	}
	switch c {
	case 97:
		return pick("actor", tActor)
	case 115:
		if t := pick("systemBoundary", tSystemBoundary); t != 0 {
			return t
		}
		return pick("style", tStyle)
	case 101:
		if t := pick("end", tEnd); t != 0 {
			return t
		}
		if startsWithNoCase(src, at, "extend") {
			return tExtend
		}
		return 0
	case 69:
		if startsWithNoCase(src, at, "extend") {
			return tExtend
		}
		return 0
	case 100:
		return pick("direction", tDirection)
	case 84:
		if t := pick("TD", tTD); t != 0 {
			return t
		}
		return pick("TB", tTB)
	case 66:
		return pick("BT", tBT)
	case 76:
		return pick("LR", tLR)
	case 82:
		return pick("RL", tRL)
	case 110:
		return pick("note", tNote)
	case 102:
		if t := pick("for", tFor); t != 0 {
			return t
		}
		return pick("false", tFalse)
	case 106:
		return pick("json", tJSON)
	case 99:
		if t := pick("classDef", tClassDef); t != 0 {
			return t
		}
		return pick("class", tClass)
	case 105, 73:
		if startsWithNoCase(src, at, "include") {
			return tInclude
		}
		return 0
	case 116:
		return pick("true", tTrue)
	case 117:
		return pick("usecase-beta", tUsecase)
	}
	return 0
}

func lineColumn(o *perr.Origin, src common.Src, offset int) (int, int) {
	src, offset = o.Written(src, offset)
	line := 1
	lineStart := 0
	for i := 0; i < offset; i++ {
		c := src.At(i)
		if c != 10 && c != 13 {
			continue
		}
		if c == 13 && i+1 < offset && src.At(i+1) == 10 {
			i++
		}
		line++
		lineStart = i + 1
	}
	return line, offset - lineStart + 1
}

func locationText(o *perr.Origin, src common.Src, start, end int) string {
	line, column := lineColumn(o, src, start)
	_, from := o.Written(src, start)
	_, to := o.Written(src, end)
	return "line " + strconv.Itoa(line) + ", column " + strconv.Itoa(column) + " [" + strconv.Itoa(from) + "," + strconv.Itoa(to) + ")"
}

// stringEnd is the end of a quoted string that opens at at, or -1 when the line ends first.
func stringEnd(src common.Src, at, quote int) int {
	for k := at + 1; k < len(src); k++ {
		c := int(src[k])
		if c == quote {
			return k + 1
		}
		if c == 10 || c == 13 {
			return -1
		}
	}
	return -1
}

func startsToken(src common.Src, at int) bool {
	c := src.At(at)
	if c != 34 && c != 39 {
		return true
	}
	return (c == 34 && src.At(at+1) == 96) || stringEnd(src, at, c) != -1
}

// charAt is String.prototype.charAt: the code unit at i, or "" past the end.
func charAt(src common.Src, i int) string {
	if i < 0 || i >= len(src) {
		return ""
	}
	return src.String(i, i+1)
}

// lexError: Mermaid reports the first character no token starts with, and how many characters
// its lexer skipped before one did.
func lexError(o *perr.Origin, src common.Src, offset, length int) *perr.Error {
	line, column := lineColumn(o, src, offset)
	_, from := o.Written(src, offset)
	_, to := o.Written(src, offset+length)
	return &perr.Error{
		Code: perr.Syntax,
		Message: "Error lexing usecase diagram: unexpected character: ->" + charAt(src, offset) + "<- at offset: " +
			strconv.Itoa(from) + ", skipped " + strconv.Itoa(to-from) + " characters. at line " + strconv.Itoa(line) +
			", column " + strconv.Itoa(column) + " [" + strconv.Itoa(from) + "," + strconv.Itoa(to) + ")",
		Type:   "usecase",
		Line:   line,
		Column: column,
	}
}

// tokenize produces the tokens of Mermaid's Chevrotain lexer for use case diagrams: the first
// pattern in its list that matches wins, a keyword gives way to a longer identifier, and
// `json id@` and `<<` switch to modes that read a JSON object and a stereotype.
func tokenize(src common.Src, o *perr.Origin) tokens {
	var out tokens
	push := func(t, start, end int) {
		out.types = append(out.types, t)
		out.starts = append(out.starts, start)
		out.ends = append(out.ends, end)
	}
	n := len(src)
	i := 0
	// Whether only blanks came before on this line, where comments and accessibility lines may start.
	lineStart := true
	noBrace := false

scan:
	for i < n {
		c := int(src[i])
		if c == 32 || c == 9 {
			i++
			continue
		}
		var typ int
		end := i + 1
		fresh := false

		switch {
		case c < 128 && charClass()[c]&classWord != 0:
			typ = 0
			if c == 97 && lineStart && src.HasPrefix(i, "acc") {
				descr := src.HasPrefix(i+3, "Descr")
				if descr || src.HasPrefix(i+3, "Title") {
					j := i + 8
					for util.IsBlank(src.At(j)) {
						j++
					}
					next := src.At(j)
					if next == 58 {
						typ = tAccTitleLine
						if descr {
							typ = tAccDescrLine
						}
						end = j + 1
						for end < n && src[end] != 10 && src[end] != 13 {
							end++
						}
					} else if next == 123 && descr && !noBrace {
						if close := src.Index("}", j+1); close == -1 {
							noBrace = true
						} else {
							typ = tAccDescrBlock
							end = close + 1
						}
					}
				}
			} else if c == 106 && src.HasPrefix(i, "json") && util.IsBlank(src.At(i+4)) {
				j := i + 5
				for util.IsBlank(src.At(j)) {
					j++
				}
				id := j
				for isWord(src.At(j)) {
					j++
				}
				if j > id {
					for util.IsBlank(src.At(j)) {
						j++
					}
					if src.At(j) == 64 {
						j++
						for util.IsBlank(src.At(j)) {
							j++
						}
						if src.At(j) == 123 {
							push(tJSONDeclarationStart, i, j)
							depth := 0
							quoted := false
							escaped := false
							k := j
							for ; k < n; k++ {
								ch := src[k]
								if quoted {
									if escaped {
										escaped = false
									} else if ch == 92 {
										escaped = true
									} else if ch == 34 {
										quoted = false
									}
								} else if ch == 34 {
									quoted = true
								} else if ch == 123 {
									depth++
								} else if ch == 125 {
									depth--
									if depth == 0 {
										break
									}
								}
							}
							literal := tUnclosedJSONObjectLiteral
							i = n
							if k < n {
								literal = tJSONObjectLiteral
								i = k + 1
							}
							push(literal, j, i)
							lineStart = false
							continue scan
						}
					}
				}
			}
			if typ == 0 {
				for end < n && isWord(int(src[end])) {
					end++
				}
				keyword := keywordAt(src, i, c)
				switch {
				case keyword != 0:
					// A keyword that is only the start of a longer word is an identifier.
					if keyword == tUsecase || end-i == keywordLength[keyword] {
						typ = keyword
						end = i + keywordLength[keyword]
					} else {
						typ = tIdentifier
					}
				case (c == 111 || c == 120) && src.At(i+1) == 45 && src.At(i+2) == 45:
					typ = tBackwardCircle
					if c == 120 {
						typ = tBackwardCross
					}
					end = i + 3
				case util.IsDigit(c):
					// `1mg` is an identifier, and `1.5px` is a number because the identifier stops at the dot.
					digits := i + 1
					for util.IsDigit(src.At(digits)) {
						digits++
					}
					if src.At(digits) == 46 && util.IsDigit(src.At(digits+1)) {
						typ = tNumber
						end = digits + 2
						for util.IsDigit(src.At(end)) {
							end++
						}
						for util.IsLetter(src.At(end)) {
							end++
						}
					} else {
						typ = tIdentifier
					}
				default:
					typ = tIdentifier
					for src.At(end) == 45 && isWord(src.At(end+1)) {
						typ = tCSSIdentifier
						end += 2
						for isWord(src.At(end)) {
							end++
						}
					}
				}
			}
		case c == 10:
			typ = tNewline
			fresh = true
		case c == 13:
			typ = tNewline
			fresh = true
			if src.At(end) == 10 {
				end++
			}
		case c == 34 || c == 39:
			if c == 34 && src.At(i+1) == 96 {
				close := src.Index("`\"", i+2)
				if close == -1 {
					typ = tUnclosedMarkdownString
					end = n
				} else {
					typ = tMarkdownString
					end = close + 2
				}
			} else {
				typ = tPlainString
				end = stringEnd(src, i, c)
				if end == -1 {
					resume := i + 1
					for resume < n && !startsToken(src, resume) {
						resume++
					}
					perr.Throw(lexError(o, src, i, resume-i))
				}
			}
		case c == 37:
			if lineStart && src.At(end) == 37 {
				typ = tComment
				for end < n && src[end] != 10 && src[end] != 13 {
					end++
				}
			} else {
				typ = tPercent
			}
		case c == 45:
			if src.At(end) != 45 {
				typ = tDash
			} else {
				for src.At(end) == 45 {
					end++
				}
				next := src.At(end)
				two := end-i == 2
				switch {
				case two && next == 124 && src.At(end+1) == 62:
					typ = tGeneralization
					end += 2
				case next == 62:
					typ = tForwardSolid
					end++
				case two && (next == 111 || next == 120):
					typ = tForwardCircle
					if next == 120 {
						typ = tForwardCross
					}
					end++
				default:
					typ = tMarkerlessSolid
				}
			}
		case c == 46:
			next := src.At(end)
			switch {
			case next == 46 && src.At(end+1) == 62:
				typ = tDependencyArrow
				end += 2
			case util.IsDigit(next):
				typ = tNumber
				for util.IsDigit(src.At(end)) {
					end++
				}
				for util.IsLetter(src.At(end)) {
					end++
				}
			default:
				typ = tDot
			}
		case c == 60:
			switch {
			case src.At(end) == 60:
				push(tStereotypeStart, i, i+2)
				lineStart = false
				i += 2
				if i >= n {
					break scan
				}
				if src.At(i) == 62 && src.At(i+1) == 62 {
					typ = tStereotypeEnd
					end = i + 2
				} else {
					close := i
					found := false
					for ; close < n; close++ {
						ch := src[close]
						if ch == 10 || ch == 13 {
							break
						}
						if ch == 62 && src.At(close+1) == 62 {
							found = true
							break
						}
					}
					if found && jsstr.Trim(src.String(i, close)) != "" {
						push(tStereotypeText, i, close)
						typ = tStereotypeEnd
						i = close
						end = close + 2
					} else {
						for close < n && src[close] != 10 && src[close] != 13 {
							close++
						}
						if close == i {
							for close < n && (src[close] == 10 || src[close] == 13) {
								close++
							}
							perr.Throw(lexError(o, src, i, close-i))
						}
						typ = tUnclosedStereotypeText
						end = close
					}
				}
			case src.At(end) == 45 && src.At(end+1) == 45:
				typ = tBackwardSolid
				end += 2
				for src.At(end) == 45 {
					end++
				}
			default:
				typ = tLabelPunctuation
			}
		case c == 58:
			if src.At(end) == 58 && src.At(end+1) == 58 {
				typ = tClassSeparator
				end += 2
			} else {
				typ = tColon
			}
		case c == 64:
			if src.At(end) == 123 {
				typ = tMetadataStart
				end++
			} else {
				typ = tAt
			}
		case c == 123:
			typ = tLBrace
		case c == 125:
			typ = tRBrace
		case c == 91:
			typ = tLBracket
		case c == 93:
			typ = tRBracket
		case c == 40:
			typ = tLParen
		case c == 41:
			typ = tRParen
		case c == 44:
			typ = tComma
		case c == 35:
			for util.IsHex(src.At(end)) {
				end++
			}
			typ = tCSSPunctuation
			if end > i+1 {
				typ = tHashColor
			}
		case c == 92:
			if src.At(end) == 44 {
				typ = tCSSEscapedComma
				end++
			} else {
				typ = tLabelPunctuation
			}
		case c < 127 && c > 32:
			typ = tLabelPunctuation
			if charClass()[c]&classPunctuation != 0 {
				typ = tCSSPunctuation
			}
		default:
			typ = tLabelSymbol
			for end < n {
				ch := src[end]
				if (ch >= 32 && ch < 127) || ch == 9 || ch == 10 || ch == 13 {
					break
				}
				end++
			}
		}

		push(typ, i, end)
		i = end
		lineStart = fresh
	}

	for range 3 {
		push(tEOF, n, n)
	}
	return out
}
