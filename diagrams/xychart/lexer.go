package xychart

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

type tokens struct {
	types  []int
	texts  []string
	starts []int
}

// Lexer states.
const (
	sInitial = iota
	sAxisData
	sAxisBandData
	sData
	sDataInner
	sString
	sAccTitle
	sAccDescr
	sAccDescrMultiline
)

// atEnd is the token each state gives for the empty rest of the input.
var atEnd = [...]int{tEOF, tEOF, tEOF, tEOF, tEOF, tStr, tAccTitleValue, tAccDescrValue, tAccDescrMultilineValue}

// brokenRule: Mermaid's grammar means to open a markdown string at `"` followed by a
// backtick, but its quoting turns the rule into one that matches this text and leaves the
// current state. Markdown strings are never read.
var brokenRule = "`)" + strings.Repeat(" ", 36) +
	"{ this.pushstate(md_string); }\n<md_string>(?:(?!`\").)+" + strings.Repeat(" ", 18) +
	"{ return md_str; }\n<md_string>(?:`"

var single = func() (out [128]int) {
	for c, typ := range map[byte]int{
		':': tColon, '+': tPlus, ',': tComma, '=': tEquals, '*': tMult, '#': tBrkt,
		'_': tUnderscore, '.': tDot, '&': tAmp, '-': tMinus, ';': tSemi,
	} {
		out[c] = typ
	}
	return out
}()

func tokenize(src common.Src) tokens {
	n := len(src)
	var out tokens
	var stack []int
	state := sInitial
	p := 0

	emit := func(typ, s, e int) {
		out.types = append(out.types, typ)
		out.texts = append(out.texts, src.String(s, e))
		out.starts = append(out.starts, s)
		p = e
	}
	push := func(s int) {
		stack = append(stack, state)
		state = s
	}
	pop := func() {
		if len(stack) > 0 {
			state = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		}
	}
	lineEnd := func(q int) int {
		if e := src.Index("\n", q); e != -1 {
			return e
		}
		return n
	}
	run := func(q int) int {
		for q < n && util.IsSpace(int(src[q])) {
			q++
		}
		return q
	}
	digits := func(q int) int {
		for util.IsDigit(src.At(q)) {
			q++
		}
		return q
	}
	keyword := func(word string) bool {
		return common.WordAt(src, p, word) && !util.IsWord(src.At(p+len(word)))
	}

	for p < n {
		c := int(src[p])

		if state == sString {
			if c == 34 {
				p++
				pop()
			} else {
				e := src.Index(`"`, p)
				if e == -1 {
					e = n
				}
				emit(tStr, p, e)
			}
			continue
		}
		if state == sAccTitle || state == sAccDescr {
			emit(atEnd[state], p, lineEnd(p))
			pop()
			continue
		}
		if state == sAccDescrMultiline {
			if c == 125 {
				p++
				pop()
			} else {
				e := src.Index("}", p)
				if e == -1 {
					e = n
				}
				emit(tAccDescrMultilineValue, p, e)
			}
			continue
		}

		next := src.At(p + 1)
		if c == 37 && next == 37 {
			p = lineEnd(p + 2)
			continue
		}
		if c == 10 || c == 13 {
			// Only a line break that directly follows an axis or a series ends it; one after
			// spaces is skipped with them, and the state carries on to the next line.
			if (state == sAxisData || state == sData) && (c == 10 || next == 10) {
				if c == 10 {
					emit(tNewline, p, p+1)
				} else {
					emit(tNewline, p, p+2)
				}
				pop()
				continue
			}
			e := p + 1
			for d := src.At(e); d == 10 || d == 13; d = src.At(e) {
				e++
			}
			emit(tNewline, p, e)
			continue
		}

		matched := true
		switch c | 32 {
		case 116:
			if keyword("title") {
				emit(tTitle, p, p+5)
			} else {
				matched = false
			}
		case 97:
			matched = false
			if common.WordAt(src, p, "acc") {
				descr := common.WordAt(src, p+3, "descr")
				if descr || common.WordAt(src, p+3, "title") {
					k := run(p + 8)
					d := src.At(k)
					if d == 58 {
						if descr {
							emit(tAccDescr, p, run(k+1))
							push(sAccDescr)
						} else {
							emit(tAccTitle, p, run(k+1))
							push(sAccTitle)
						}
						matched = true
					} else if d == 123 && descr {
						p = run(k + 1)
						push(sAccDescrMultiline)
						matched = true
					}
				}
			}
		case 120:
			switch {
			case keyword("xychart-beta"):
				emit(tXYChart, p, p+12)
			case keyword("xychart"):
				emit(tXYChart, p, p+7)
			case keyword("x-axis"):
				emit(tXAxis, p, p+6)
				push(sAxisData)
			default:
				matched = false
			}
		case 121:
			if keyword("y-axis") {
				emit(tYAxis, p, p+6)
				push(sAxisData)
			} else {
				matched = false
			}
		case 118:
			if common.WordAt(src, p, "vertical") {
				emit(tChartOrientation, p, p+8)
			} else {
				matched = false
			}
		case 104:
			if common.WordAt(src, p, "horizontal") {
				emit(tChartOrientation, p, p+10)
			} else {
				matched = false
			}
		case 108:
			if keyword("line") {
				emit(tLine, p, p+4)
				push(sData)
			} else {
				matched = false
			}
		case 98:
			if keyword("bar") {
				emit(tBar, p, p+3)
				push(sData)
			} else {
				matched = false
			}
		default:
			matched = false
		}
		if matched {
			continue
		}

		if c == 91 {
			if state == sAxisData {
				push(sAxisBandData)
			} else if state == sData {
				push(sDataInner)
			}
			emit(tSquareBracesStart, p, p+1)
			continue
		}
		if c == 93 {
			if state == sDataInner || state == sAxisBandData {
				pop()
			}
			emit(tSquareBracesEnd, p, p+1)
			continue
		}
		if state == sAxisData && c == 45 && next == 45 && src.At(p+2) == 62 {
			emit(tArrowDelimiter, p, p+3)
			continue
		}
		if state == sAxisData || state == sDataInner {
			q := p
			if c == 43 || c == 45 {
				q = p + 1
			}
			d := src.At(q)
			e := -1
			if util.IsDigit(d) {
				e = digits(q + 1)
				if src.At(e) == 46 && util.IsDigit(src.At(e+1)) {
					e = digits(e + 2)
				}
			} else if d == 46 && util.IsDigit(src.At(q+1)) {
				e = digits(q + 2)
			}
			if e != -1 {
				emit(tNumberWithDecimal, p, e)
				continue
			}
		}
		if c == 96 && common.WordAt(src, p, brokenRule) {
			p += len(brokenRule)
			pop()
			continue
		}
		if c == 34 {
			p++
			push(sString)
			continue
		}
		if util.IsLetter(c) {
			e := p + 1
			for util.IsLetter(src.At(e)) {
				e++
			}
			emit(tAlpha, p, e)
			continue
		}
		if util.IsDigit(c) {
			emit(tNum, p, digits(p+1))
			continue
		}
		if util.IsSpace(c) {
			p = run(p + 1)
			continue
		}
		if c < 128 && single[c] != 0 {
			emit(single[c], p, p+1)
			continue
		}
		break
	}

	if p < n {
		out.types = append(out.types, tError)
		out.texts = append(out.texts, "")
		out.starts = append(out.starts, p)
		return out
	}
	// Jison tries the rules of the current state once more against the empty rest of the
	// input.
	out.types = append(out.types, atEnd[state], tEnd)
	out.texts = append(out.texts, "", "")
	out.starts = append(out.starts, n, n)
	return out
}
