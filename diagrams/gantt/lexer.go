package gantt

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// tokens are the lexer's output: a type, the source span and its start, per token.
type tokens struct {
	types  []int
	starts []int
	ends   []int
}

// Lexer states, as upstream's S.
const (
	sINITIAL = iota
	sAccTitle
	sAccDescr
	sAccDescrMultiline
	sHref
	sCallbackname
	sCallbackargs
	sClick
)

var days = [...]string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}

func skipSpace(src common.Src, p int) int {
	for util.IsSpace(src.At(p)) {
		p++
	}
	return p
}

// keywordLine matches /word\s[^...]+/iy, where stops are the characters the line may not
// contain: the keyword, one whitespace character, and the rest of the line up to a stop.
func keywordLine(src common.Src, p int, word string, stops string) int {
	if !common.WordAt(src, p, word) || !util.IsSpace(src.At(p+len(word))) {
		return -1
	}
	start := p + len(word) + 1
	e := start
	for e < len(src) && !containsUnit(stops, src[e]) {
		e++
	}
	if e == start {
		return -1
	}
	return e
}

// isBlank is [^\S\r\n]: whitespace but a line break.
func isBlank(c int) bool { return c != '\r' && c != '\n' && util.IsSpace(c) }

// bareTitle matches /title[^\S\r\n]*(?=[\r\n]|$)/iy.
func bareTitle(src common.Src, p int) int {
	if !common.WordAt(src, p, "title") {
		return -1
	}
	e := p + len("title")
	for isBlank(src.At(e)) {
		e++
	}
	if e < len(src) && src[e] != '\r' && src[e] != '\n' {
		return -1
	}
	return e
}

// titleLine matches /title[^\S\r\n][^\n]+/iy.
func titleLine(src common.Src, p int) int {
	if !common.WordAt(src, p, "title") || !isBlank(src.At(p+len("title"))) {
		return -1
	}
	start := p + len("title") + 1
	e := start
	for e < len(src) && src[e] != '\n' {
		e++
	}
	if e == start {
		return -1
	}
	return e
}

func containsUnit(set string, c uint16) bool {
	for i := 0; i < len(set); i++ {
		if uint16(set[i]) == c {
			return true
		}
	}
	return false
}

// keywordWord matches /word\b/iy.
func keywordWord(src common.Src, p int, word string) int {
	if !common.WordAt(src, p, word) || util.IsWord(src.At(p+len(word))) {
		return -1
	}
	return p + len(word)
}

// keywordThen matches /word\s*X\s*/iy (or \s+ before X when spaced), returning the end.
func keywordThen(src common.Src, p int, word string, mark int) int {
	if !common.WordAt(src, p, word) {
		return -1
	}
	e := skipSpace(src, p+len(word))
	if src.At(e) != mark {
		return -1
	}
	return skipSpace(src, e+1)
}

// keywordSpaced matches /word\s+/iy.
func keywordSpaced(src common.Src, p int, word string) int {
	if !common.WordAt(src, p, word) {
		return -1
	}
	e := skipSpace(src, p+len(word))
	if e == p+len(word) {
		return -1
	}
	return e
}

// dayAfter matches /keyword\s+(choice...)\b/iy, returning the end and the index of the choice.
func dayAfter(src common.Src, p int, word string, choices []string) (int, int) {
	e := keywordSpaced(src, p, word)
	if e < 0 {
		return -1, -1
	}
	for i, day := range choices {
		if common.WordAt(src, e, day) && !util.IsWord(src.At(e+len(day))) {
			return e + len(day), i
		}
	}
	return -1, -1
}

// keywordRule is one of upstream's KEYWORDS: a statement that starts with a keyword.
type keywordRule struct {
	match func(src common.Src, p int) int
	typ   int
}

func line(word, stops string) func(common.Src, int) int {
	return func(src common.Src, p int) int { return keywordLine(src, p, word, stops) }
}

func word(w string) func(common.Src, int) int {
	return func(src common.Src, p int) int { return keywordWord(src, p, w) }
}

// keywordRules are grouped by the keyword's first letter and kept in the grammar's rule
// order.
var keywordRules = map[int][]keywordRule{
	'a': {{line("axisformat", "#\n;"), tAxisFormat}, {line("accdescription", "#\n;"), tAccDescription}},
	'g': {{word("gantt"), tGantt}},
	'd': {{line("dateformat", "#\n;"), tDateFormat}},
	'i': {{word("inclusiveenddates"), tInclusiveEndDates}, {line("includes", "#\n;"), tIncludes}},
	't': {
		{word("topaxis"), tTopAxis},
		{line("tickinterval", "#\n;"), tTickInterval},
		{line("todaymarker", "\n;"), tTodayMarker},
		{titleLine, tTitle},
	},
	'e': {{line("excludes", "#\n;"), tExcludes}},
	's': {{line("section", "\n"), tSection}},
}

func tokenize(src common.Src) tokens {
	n := len(src)
	var out tokens
	state := sINITIAL
	p := 0

	emit := func(typ, s, e int) {
		out.types = append(out.types, typ)
		out.starts = append(out.starts, s)
		out.ends = append(out.ends, min(e, n))
		p = e
	}
	until := func(ch uint16, from int) int {
		for i := max(from, 0); i < n; i++ {
			if src[i] == ch {
				return i
			}
		}
		return n
	}

	keyword := func(c int) bool {
		var e int
		switch c | 32 {
		case 'a':
			if e = keywordThen(src, p, "acctitle", ':'); e >= 0 {
				state = sAccTitle
				emit(tAccTitle, p, e)
				return true
			}
			if e = keywordThen(src, p, "accdescr", ':'); e >= 0 {
				state = sAccDescr
				emit(tAccDescr, p, e)
				return true
			}
			if e = keywordThen(src, p, "accdescr", '{'); e >= 0 {
				state = sAccDescrMultiline
				p = e
				return true
			}
		case 'h':
			if e = keywordSpaced(src, p, "href"); e >= 0 && src.At(e) == '"' {
				state = sHref
				p = e + 1
				return true
			}
		case 'c':
			if e = keywordSpaced(src, p, "call"); e >= 0 {
				state = sCallbackname
				p = e
				return true
			}
			if e = keywordSpaced(src, p, "click"); e >= 0 {
				state = sClick
				p = e
				return true
			}
		case 't':
			if e = bareTitle(src, p); e >= 0 {
				p = e
				return true
			}
		case 'w':
			if e, day := dayAfter(src, p, "weekday", days[:]); e >= 0 {
				emit(tWeekdayMonday+day, p, e)
				return true
			}
			if e, day := dayAfter(src, p, "weekend", []string{"friday", "saturday"}); e >= 0 {
				emit(tWeekendFriday+day, p, e)
				return true
			}
		}
		for _, rule := range keywordRules[c|32] {
			if e = rule.match(src, p); e >= 0 {
				emit(rule.typ, p, e)
				return true
			}
		}
		return false
	}

	isDate := func() bool {
		for k, c := range "dddd-dd-dd" {
			u := src.At(p + k)
			if c == 'd' && !(u >= '0' && u <= '9') || c == '-' && u != '-' {
				return false
			}
		}
		return !util.IsWord(src.At(p + 10))
	}

	for {
		if p >= n {
			typ := tEOF
			switch state {
			case sAccTitle:
				typ = tAccTitleValue
			case sAccDescr:
				typ = tAccDescrValue
			case sAccDescrMultiline:
				typ = tAccDescrMultilineValue
			case sHref:
				typ = tHref
			case sCallbackname:
				typ = tCallbackname
			case sCallbackargs:
				typ = tCallbackargs
			case sClick:
				typ = tClick
			}
			emit(typ, n, n)
			break
		}

		c := src.At(p)
		switch state {
		case sINITIAL:
			d := src.At(p + 1)
			switch {
			case c == '%':
				if d != '%' {
					e := until('\n', p+1)
					for src.At(e) == '\n' {
						e++
					}
					p = e
				} else if src.At(p+2) == '{' {
					// The grammar enters a lexer state it never defines, so nothing can follow.
					emit(tOpenDirective, p, p+3)
					emit(tERROR, p, p)
					return out
				} else {
					p = until('\n', p+2)
				}
			case c == '\n':
				e := p + 1
				for src.At(e) == '\n' {
					e++
				}
				emit(tNL, p, e)
			case util.IsSpace(c):
				p++
				for p < n && util.IsSpace(src.At(p)) {
					p++
				}
			case keyword(c):
			case c >= '0' && c <= '9' && isDate():
				emit(tDate, p, p+10)
			case c != ':':
				e := p + 1
				for e < n && src[e] != ':' && src[e] != '\n' {
					e++
				}
				emit(tTaskTxt, p, e)
			default:
				e := p + 1
				for e < n && src[e] != '#' && src[e] != '\n' && src[e] != ';' {
					e++
				}
				if e > p+1 {
					emit(tTaskData, p, e)
				} else {
					emit(tCOLON, p, e)
				}
			}

		case sAccTitle, sAccDescr:
			typ := tAccTitleValue
			if state == sAccDescr {
				typ = tAccDescrValue
			}
			state = sINITIAL
			emit(typ, p, until('\n', p))

		case sAccDescrMultiline:
			if c == '}' {
				state = sINITIAL
				p++
			} else {
				emit(tAccDescrMultilineValue, p, until('}', p))
			}

		case sHref:
			if c == '"' {
				state = sINITIAL
				p++
			} else {
				emit(tHref, p, until('"', p))
			}

		case sCallbackname:
			if c == '(' {
				if e := skipSpace(src, p+1); src.At(e) == ')' {
					state = sINITIAL
					p = e + 1
				} else {
					state = sCallbackargs
					p++
				}
			} else {
				emit(tCallbackname, p, until('(', p))
			}

		case sCallbackargs:
			if c == ')' {
				state = sINITIAL
				p++
			} else {
				emit(tCallbackargs, p, until(')', p))
			}

		case sClick:
			if util.IsSpace(c) {
				state = sINITIAL
				p++
			} else {
				e := p + 1
				for e < n && !util.IsSpace(src.At(e)) {
					e++
				}
				emit(tClick, p, e)
			}
		}
	}

	out.types = append(out.types, tEND)
	out.starts = append(out.starts, n)
	out.ends = append(out.ends, n)
	return out
}
