package quadrant

import (
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
	sTitle
	sAccTitle
	sAccDescr
	sAccDescrMultiline
	sString
	sMDString
	sClassName
	sPointStart
	sPointX
	sPointY
)

// atEnd is the token each state gives for the empty rest of the input, where it gives one.
var atEnd = [...]int{tEOF, tTitleValue, tAccTitleValue, tAccDescrValue, tAccDescrMultilineValue, tStr}

var single = func() (out [128]int) {
	for _, pair := range []struct {
		chars string
		typ   int
	}{
		{":", tColon}, {"+", tPlus}, {",", tComma}, {"=", tEquals}, {"*", tMult}, {"#", tBrkt},
		{"_", tUnderscore}, {".", tDot}, {"&", tAmp}, {"-", tMinus}, {" \t\v\f", tSpace}, {";", tSemi},
		{"!$%'`?\\/", tPunctuation},
	} {
		for k := 0; k < len(pair.chars); k++ {
			out[pair.chars[k]] = pair.typ
		}
	}
	return out
}()

func tokenize(src common.Src) tokens {
	n := len(src)
	var out tokens
	var stack []int
	state := sInitial
	p := 0
	// The last run scanned of each kind, so that a long run is not scanned again from each
	// of its characters.
	spaceFrom, spaceTo := 0, 0
	dashFrom, dashTo := 0, 0
	wsFrom, wsTo := 0, 0
	gapFrom, gapTo := 0, 0

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
	spaces := func(q int) int {
		for src.At(q) == 32 {
			q++
		}
		return q
	}
	run := func(q int) int {
		for q < n && util.IsSpace(int(src[q])) {
			q++
		}
		return q
	}
	// Mermaid's `(1)|(0(.\d+)?)`, where the dot is any character within a line.
	coordinate := func(q int) int {
		c := src.At(q)
		if c == 49 {
			return q + 1
		}
		if c != 48 {
			return -1
		}
		e := q + 1
		if e < n && !util.IsLineEnd(int(src[e])) && util.IsDigit(src.At(e+1)) {
			e += 2
			for util.IsDigit(src.At(e)) {
				e++
			}
		}
		return e
	}

lexing:
	for p < n {
		c := int(src[p])
		next := src.At(p + 1)

		switch state {
		case sTitle, sAccTitle, sAccDescr:
			emit(atEnd[state], p, lineEnd(p))
			pop()
			continue

		case sAccDescrMultiline:
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

		case sString:
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

		case sMDString:
			if c == 96 && next == 34 {
				p += 2
				pop()
				continue
			}
			if c != 96 && c != 34 {
				e := p + 1
				for e < n && src[e] != 96 && src[e] != 34 {
					e++
				}
				emit(tMDStr, p, e)
				continue
			}
			break lexing

		case sClassName:
			if util.IsWord(c) {
				e := p + 1
				for util.IsWord(src.At(e)) {
					e++
				}
				emit(tClassName, p, e)
				pop()
				continue
			}
			break lexing

		case sPointStart:
			if e := coordinate(p); e != -1 {
				emit(tPointX, p, e)
				push(sPointX)
				continue
			}
			if k := run(p); src.At(k) == 93 {
				p = spaces(k + 1)
				pop()
				continue
			}
			break lexing

		case sPointX:
			if k := run(p); src.At(k) == 44 {
				p = run(k + 1)
				state = sPointY
				continue
			}
			break lexing

		case sPointY:
			if e := coordinate(p); e != -1 {
				emit(tPointY, p, e)
				pop()
				continue
			}
			break lexing
		}

		if c == 37 && next == 37 {
			p = lineEnd(p + 2)
			continue
		}
		if c == 10 || c == 13 {
			e := p + 1
			for d := src.At(e); d == 10 || d == 13; d = src.At(e) {
				e++
			}
			emit(tNewline, p, e)
			continue
		}

		lower := c | 32
		if lower == 116 && common.WordAt(src, p, "title") && !util.IsWord(src.At(p+5)) {
			emit(tTitle, p, p+5)
			push(sTitle)
			continue
		}
		if lower == 97 && common.WordAt(src, p, "acc") {
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
					continue
				}
				if d == 123 && descr {
					p = run(k + 1)
					push(sAccDescrMultiline)
					continue
				}
			}
		}

		// Axis, quadrant and chart keywords and the axis arrow take the spaces around them.
		j := p
		if c == 32 {
			if p < spaceFrom || p >= spaceTo {
				spaceFrom = p
				spaceTo = spaces(p)
			}
			j = spaceTo
		}
		d := src.At(j)
		dl := d | 32
		typ, end := 0, 0
		switch {
		case dl == 120 && common.WordAt(src, j, "x-axis"):
			typ, end = tXAxis, j+6
		case dl == 121 && common.WordAt(src, j, "y-axis"):
			typ, end = tYAxis, j+6
		case d == 45:
			if j < dashFrom || j >= dashTo {
				dashFrom = j
				dashTo = j
				for src.At(dashTo) == 45 {
					dashTo++
				}
			}
			if dashTo-j >= 2 && src.At(dashTo) == 62 {
				typ, end = tDelimiter, dashTo+1
			}
		case dl == 113 && common.WordAt(src, j, "quadrant"):
			q := src.At(j + 9)
			if src.At(j+8) == 45 && q >= 49 && q <= 52 {
				typ, end = tQuadrant1+q-49, j+10
			} else if common.WordAt(src, j+8, "chart") {
				typ, end = tQuadrant, j+13
			}
		}
		if typ != 0 {
			emit(typ, p, spaces(end))
			continue
		}

		if lower == 99 && common.WordAt(src, p, "classdef") && !util.IsWord(src.At(p+8)) {
			emit(tClassDef, p, p+8)
			continue
		}
		if c == 34 {
			if next == 96 {
				p += 2
				push(sMDString)
			} else {
				p++
				push(sString)
			}
			continue
		}
		if c == 58 && next == 58 && src.At(p+2) == 58 {
			p += 3
			push(sClassName)
			continue
		}
		// A point's `: [`, with any whitespace before, between and after, line breaks
		// included.
		if c == 58 || util.IsSpace(c) {
			k := p
			if c != 58 {
				if p < wsFrom || p >= wsTo {
					wsFrom = p
					wsTo = run(p)
				}
				k = wsTo
			}
			if src.At(k) == 58 {
				if k+1 < gapFrom || k+1 >= gapTo {
					gapFrom = k + 1
					gapTo = run(k + 1)
				}
				if src.At(gapTo) == 91 {
					emit(tPointStart, p, run(gapTo+1))
					push(sPointStart)
					continue
				}
			}
		}

		if util.IsLetter(c) {
			e := p + 1
			for util.IsLetter(src.At(e)) {
				e++
			}
			emit(tAlpha, p, e)
			continue
		}
		if c >= 128 {
			e := p + 1
			for src.At(e) >= 128 {
				e++
			}
			emit(tUnicodeText, p, e)
			continue
		}
		if util.IsDigit(c) {
			e := p + 1
			for util.IsDigit(src.At(e)) {
				e++
			}
			emit(tNum, p, e)
			continue
		}
		if single[c] != 0 {
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
	if state < len(atEnd) {
		out.types = append(out.types, atEnd[state])
		out.texts = append(out.texts, "")
		out.starts = append(out.starts, n)
	}
	out.types = append(out.types, tEnd)
	out.texts = append(out.texts, "")
	out.starts = append(out.starts, n)
	return out
}
