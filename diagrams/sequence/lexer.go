package sequence

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

type tokens struct {
	types  []int
	texts  []string
	starts []int
}

// Lexer states.
const (
	sINITIAL = iota
	sID
	sALIAS
	sLINE
	sCONFIG
	sAccTitle
	sAccDescr
	sAccDescrMultiline
)

// Character classes.
const (
	cActorStart = 1
	cActorRest  = 2
	cID         = 4
	cIDWide     = 8
	cLine       = 16
	cMenu       = 32
)

func contains(set string, c byte) bool {
	for i := 0; i < len(set); i++ {
		if set[i] == c {
			return true
		}
	}
	return false
}

// classTable holds the character classes of Mermaid's lexer rules, for ASCII. Other
// characters are tested by util.IsSpace.
var classTable = func() [128]uint8 {
	var t [128]uint8
	for c := 0; c < 128; c++ {
		ch := byte(c)
		ws := c == 32 || (c >= 9 && c <= 13)
		bits := uint8(0)
		if !contains("/\\+()<->:\n,;", ch) {
			bits |= cActorStart
		}
		if !contains("+<->:\n,;", ch) {
			bits |= cActorRest
		}
		if !contains("<>:\n,;@", ch) {
			bits |= cIDWide
			if !ws {
				bits |= cID
			}
		}
		if !contains("#\n;", ch) {
			bits |= cLine
		}
		if !contains("/\\+()<>:\n,;-", ch) && !ws {
			bits |= cMenu
		}
		t[c] = bits
	}
	return t
}()

func tokenize(src common.Src) tokens {
	n := len(src)
	var out tokens
	var stack []int
	state := sINITIAL
	p := 0
	nextNl := -1

	// at is src.charCodeAt(q), with -1 for upstream's NaN past the end.
	at := func(q int) int {
		if q < 0 || q >= n {
			return -1
		}
		return int(src[q])
	}
	slice := func(s, e int) string {
		return jsstr.FromUTF16(src[s:e])
	}
	emitText := func(typ, s, e int, text string) {
		out.types = append(out.types, typ)
		out.texts = append(out.texts, text)
		out.starts = append(out.starts, s)
		p = e
	}
	emit := func(typ, s, e int) {
		emitText(typ, s, e, slice(s, e))
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
	indexOf := func(c uint16, from int) int {
		for q := from; q < n; q++ {
			if src[q] == c {
				return q
			}
		}
		return -1
	}
	eol := func(q int) int {
		if nextNl < q {
			nextNl = indexOf('\n', q)
			if nextNl == -1 {
				nextNl = n
			}
		}
		return nextNl
	}
	has := func(q int, bit uint8) bool {
		c := at(q)
		if c >= 0 && c < 128 {
			return classTable[c]&bit != 0
		}
		if bit == cID || bit == cMenu {
			return !util.IsSpace(c)
		}
		return true
	}
	// ci matches a lowercase literal without regard to the case of the source.
	ci := func(q int, word string) bool {
		for k := 0; k < len(word); k++ {
			w := int(word[k])
			c := at(q + k)
			if w >= 97 && w <= 122 {
				if c|32 != w {
					return false
				}
			} else if c != w {
				return false
			}
		}
		return true
	}
	word := func(lit string) bool { return ci(p, lit) && !util.IsWord(at(p+len(lit))) }
	skipWs := func(q int) int {
		for q < n && util.IsSpace(int(src[q])) {
			q++
		}
		return q
	}
	skipSameLine := func() bool {
		q := p
		for q < n {
			c := int(src[q])
			if c == 10 || !util.IsSpace(c) {
				break
			}
			q++
		}
		if q == p {
			return false
		}
		p = q
		return true
	}
	lineRun := func(q int) int {
		for q < n && has(q, cLine) {
			q++
		}
		return q
	}
	begin := func(typ, length, next int) {
		emit(typ, p, p+length)
		push(next)
	}
	keyword := func(lit string, typ, next int) bool {
		if !word(lit) {
			return false
		}
		emit(typ, p, p+len(lit))
		if next >= 0 {
			push(next)
		}
		return true
	}
	// menu matches an actor menu keyword, which counts only when spaces and the start of a
	// name follow.
	menu := func(lit string, typ int) bool {
		if !ci(p, lit) {
			return false
		}
		q := p + len(lit)
		c := at(q)
		if c != 32 && c != 9 {
			return false
		}
		for c == 32 || c == 9 {
			q++
			c = at(q)
		}
		if q >= n || !has(q, cMenu) {
			return false
		}
		emit(typ, p, p+len(lit))
		return true
	}

	arrowAhead := func(q int) bool {
		c := at(q)
		d := at(q + 1)
		if c == 45 {
			if d == 120 || d == 88 || d == 41 || d == 92 || d == 47 || d == 45 {
				return true
			}
			e := at(q + 2)
			return d == 124 && (e == 92 || e == 47)
		}
		if c == 47 || c == 92 {
			return (d == 124 || d == c) && at(q+2) == 45
		}
		return c == 40 && d == 41
	}

	actor := func() {
		q := p + 1
		for q < n && has(q, cActorStart) {
			q++
		}
		for q < n && !arrowAhead(q) {
			r := q
			for at(r) == 45 {
				r++
			}
			if r >= n || !has(r, cActorRest) {
				break
			}
			r++
			for r < n && has(r, cActorRest) {
				r++
			}
			q = r
		}
		emitText(tACTOR, p, q, jsstr.Trim(slice(p, q)))
	}

	number := func() bool {
		q := p
		for util.IsDigit(at(q)) {
			q++
		}
		whole := q > p
		if at(q) == 46 && util.IsDigit(at(q+1)) {
			r := q + 2
			if util.IsDigit(at(q + 2)) {
				r = q + 3
			}
			after := at(r)
			if after == 32 || after == 10 {
				emit(tNUM, p, r)
				return true
			}
		}
		after := at(q)
		if whole && (after == 32 || after == 10) {
			emit(tNUM, p, q)
			return true
		}
		return false
	}

	dash := func() {
		d := at(p + 1)
		e := at(p + 2)
		switch {
		case d == 62:
			if e == 62 {
				emit(tSOLID_ARROW, p, p+3)
			} else {
				emit(tSOLID_OPEN_ARROW, p, p+2)
			}
		case d == 120 || d == 88:
			emit(tSOLID_CROSS, p, p+2)
		case d == 41:
			emit(tSOLID_POINT, p, p+2)
		case d == 124 && e == 92:
			emit(tSOLID_ARROW_TOP, p, p+3)
		case d == 124 && e == 47:
			emit(tSOLID_ARROW_BOTTOM, p, p+3)
		case d == 92 && e == 92:
			emit(tSTICK_ARROW_TOP, p, p+3)
		case d == 47 && e == 47:
			emit(tSTICK_ARROW_BOTTOM, p, p+3)
		case d == 45:
			f := at(p + 3)
			switch {
			case e == 62:
				if f == 62 {
					emit(tDOTTED_ARROW, p, p+4)
				} else {
					emit(tDOTTED_OPEN_ARROW, p, p+3)
				}
			case e == 120 || e == 88:
				emit(tDOTTED_CROSS, p, p+3)
			case e == 41:
				emit(tDOTTED_POINT, p, p+3)
			case e == 124 && f == 92:
				emit(tSOLID_ARROW_TOP_DOTTED, p, p+4)
			case e == 124 && f == 47:
				emit(tSOLID_ARROW_BOTTOM_DOTTED, p, p+4)
			case e == 92 && f == 92:
				emit(tSTICK_ARROW_TOP_DOTTED, p, p+4)
			case e == 47 && f == 47:
				emit(tSTICK_ARROW_BOTTOM_DOTTED, p, p+4)
			default:
				emit(tMINUS, p, p+1)
			}
		default:
			emit(tMINUS, p, p+1)
		}
	}

	reverse := func(c int) bool {
		d := at(p + 1)
		if (d != 124 && d != c) || at(p+2) != 45 {
			return false
		}
		dotted := at(p+3) == 45
		top := c == 47
		var typ int
		switch {
		case d == 124 && dotted:
			typ = pick(top, tSOLID_ARROW_TOP_REVERSE_DOTTED, tSOLID_ARROW_BOTTOM_REVERSE_DOTTED)
		case d == 124:
			typ = pick(top, tSOLID_ARROW_TOP_REVERSE, tSOLID_ARROW_BOTTOM_REVERSE)
		case dotted:
			typ = pick(top, tSTICK_ARROW_TOP_REVERSE_DOTTED, tSTICK_ARROW_BOTTOM_REVERSE_DOTTED)
		default:
			typ = pick(top, tSTICK_ARROW_TOP_REVERSE, tSTICK_ARROW_BOTTOM_REVERSE)
		}
		emit(typ, p, p+pick(dotted, 4, 3))
		return true
	}

	title := func() bool {
		if !ci(p, "title") {
			return false
		}
		q := p + 5
		legacy := at(q) == 58
		if legacy {
			q++
		}
		e := q
		for e < n && at(e) != 10 && util.IsSpace(at(e)) {
			e++
		}
		if e == n || at(e) == 10 {
			p = e
			return true
		}
		if sep := at(q); !util.IsSpace(sep) || sep == 13 || q+1 >= n || !has(q+1, cLine) {
			return false
		}
		emit(pick(legacy, tLegacyTitle, tTitle), p, lineRun(q+1))
		return true
	}

	acc := func() bool {
		descr := ci(p, "accdescr")
		if !descr && !ci(p, "acctitle") {
			return false
		}
		q := skipWs(p + 8)
		c := at(q)
		if c == 58 {
			emit(pick(descr, tAccDescr, tAccTitle), p, skipWs(q+1))
			push(pick(descr, sAccDescr, sAccTitle))
			return true
		}
		if descr && c == 123 {
			p = skipWs(q + 1)
			push(sAccDescrMultiline)
			return true
		}
		return false
	}

	keywords := func(c int) bool {
		switch c | 32 {
		case 97:
			return keyword("actor", tParticipantActor, sID) ||
				keyword("alt", tAlt, sLINE) ||
				keyword("and", tAnd, sLINE) ||
				keyword("activate", tActivate, sID) ||
				acc() ||
				keyword("autonumber", tAutonumber, -1)
		case 98:
			return keyword("box", tBox, sLINE) || keyword("break", tBreak, sLINE)
		case 99:
			return keyword("create", tCreate, -1) || keyword("critical", tCritical, sLINE)
		case 100:
			return keyword("destroy", tDestroy, sID) || menu("details", tDetails) || keyword("deactivate", tDeactivate, sID)
		case 101:
			return keyword("else", tElse, sLINE) || keyword("end", tEnd, -1)
		case 108:
			return keyword("loop", tLoop, sLINE) || keyword("left of", tLeftOf, -1) || menu("links", tLinks) || menu("link", tLink)
		case 110:
			return keyword("note", tNote, -1)
		case 111:
			return keyword("opt", tOpt, sLINE) || keyword("option", tOption, sLINE) || keyword("over", tOver, -1) || keyword("off", tOff, -1)
		case 112:
			return keyword("participant", tParticipant, sID) ||
				keyword("par", tPar, sLINE) ||
				keyword("par_over", tParOver, sLINE) ||
				menu("properties", tProperties)
		case 114:
			return keyword("rect", tRect, sLINE) || keyword("right of", tRightOf, -1)
		case 115:
			return keyword("sequencediagram", tSD, -1)
		case 116:
			return title()
		}
		return false
	}

	startsWith := func(lit string) bool {
		for k := 0; k < len(lit); k++ {
			if at(p+k) != int(lit[k]) {
				return false
			}
		}
		return true
	}

	initial := func() {
		c := at(p)
		if c == 10 {
			q := p + 1
			for at(q) == 10 {
				q++
			}
			emit(tNEWLINE, p, q)
			return
		}
		if util.IsSpace(c) {
			p = skipWs(p + 1)
			return
		}
		if c == 35 || (c == 37 && at(p+1) != 123) {
			p = eol(p)
			return
		}
		if (util.IsDigit(c) || c == 46) && number() {
			return
		}
		if c < 128 && c|32 >= 97 && c|32 <= 122 && keywords(c) {
			return
		}
		switch {
		case c == 44:
			emit(tCOMMA, p, p+1)
		case c == 59:
			emit(tNEWLINE, p, p+1)
		case has(p, cActorStart):
			actor()
		case c == 45:
			dash()
		case c == 60 && startsWith("<<->>"):
			emit(tBIDIRECTIONAL_SOLID_ARROW, p, p+5)
		case c == 60 && startsWith("<<-->>"):
			emit(tBIDIRECTIONAL_DOTTED_ARROW, p, p+6)
		case (c == 47 || c == 92) && reverse(c):
		case c == 58:
			emit(tTXT, p, lineRun(p+1))
		case c == 43:
			emit(tPLUS, p, p+1)
		case c == 40 && at(p+1) == 41:
			emit(tCENTRAL, p, p+2)
		default:
			emit(tINVALID, p, p+1)
		}
	}

	// id reads a participant name, up to an alias, a config object, or the end of the
	// statement.
	id := func() bool {
		if skipSameLine() {
			return true
		}
		c := at(p)
		if c == 35 {
			p = eol(p)
			return true
		}
		if c == 64 && at(p+1) == 123 {
			begin(tCONFIG_START, 2, sCONFIG)
			return true
		}
		a := p
		for a < n && has(a, cID) {
			a++
		}
		if a > p {
			w := skipWs(a)
			if at(w) == 64 && at(w+1) == 123 {
				emit(tACTOR, p, a)
				return true
			}
			if w > a && ci(w, "as") && util.IsSpace(at(w+2)) {
				emit(tACTOR, p, a)
				push(sALIAS)
				return true
			}
		}
		b := a
		for b < n && has(b, cIDWide) {
			b++
		}
		if b > p {
			end := at(b)
			// Before any other delimiter the name must be followed by a comment, so it ends
			// at the last one.
			cut := b
			if b < n && end != 10 && end != 59 {
				cut--
				for cut > p && at(cut) != 35 {
					cut--
				}
			}
			if cut > p {
				emitText(tACTOR, p, cut, jsstr.Trim(slice(p, cut)))
				pop()
				return true
			}
		}
		if at(b) == 60 {
			emit(tINVALID, p, eol(p))
			pop()
			return true
		}
		if c != 10 {
			e := eol(p)
			emitText(tINVALID, p, e, jsstr.Trim(slice(p, e)))
			pop()
			return true
		}
		return false
	}

	step := func() bool {
		switch state {
		case sINITIAL:
			initial()
			return true
		case sID:
			return id()
		case sALIAS:
			if skipSameLine() {
				return true
			}
			if at(p) == 35 {
				p = eol(p)
				return true
			}
			pop()
			pop()
			if word("as") {
				begin(tAS, 2, sLINE)
			} else {
				emit(tNEWLINE, p, p)
			}
			return true
		case sLINE:
			if skipSameLine() {
				return true
			}
			if at(p) == 35 {
				p = eol(p)
				return true
			}
			emit(tRestOfLine, p, lineRun(p))
			pop()
			return true
		case sCONFIG:
			if at(p) != 125 {
				q := indexOf('}', p)
				if q == -1 {
					q = n
				}
				emit(tCONFIG_CONTENT, p, q)
				return true
			}
			w := skipWs(p + 1)
			alias := w > p+1 && ci(w, "as") && util.IsSpace(at(w+2))
			emit(tCONFIG_END, p, p+1)
			pop()
			if alias {
				push(sALIAS)
			} else {
				pop()
			}
			return true
		case sAccTitle, sAccDescr:
			emit(pick(state == sAccTitle, tAccTitleValue, tAccDescrValue), p, eol(p))
			pop()
			return true
		default:
			if at(p) == 125 {
				p++
				pop()
			} else {
				q := indexOf('}', p)
				if q == -1 {
					q = n
				}
				emit(tAccDescrMultilineValue, p, q)
			}
			return true
		}
	}

	for p < n {
		if !step() {
			out.types = append(out.types, tERROR)
			out.texts = append(out.texts, "")
			out.starts = append(out.starts, p)
			return out
		}
	}

	// Mermaid's lexer tries its rules once more on the empty remainder before reporting the end.
	switch state {
	case sINITIAL, sALIAS:
		emit(tNEWLINE, n, n)
	case sLINE:
		emit(tRestOfLine, n, n)
	case sAccTitle:
		emit(tAccTitleValue, n, n)
	case sAccDescr:
		emit(tAccDescrValue, n, n)
	case sAccDescrMultiline:
		emit(tAccDescrMultilineValue, n, n)
	}
	out.types = append(out.types, tEND)
	out.texts = append(out.texts, "")
	out.starts = append(out.starts, n)
	return out
}

func pick[T any](cond bool, yes, no T) T {
	if cond {
		return yes
	}
	return no
}
