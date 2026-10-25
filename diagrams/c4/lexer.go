package c4

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// tokens holds token i as src[starts[i]:ends[i]].
type tokens struct {
	types  []int
	starts []int
	ends   []int
}

// Lexer start conditions.
const (
	sINITIAL = iota
	sStruct
	sAttribute
	sString
	sStringKV
	sStringKVKey
	sStringKVValue
	sAcc
	sAccDescrMultiline
)

var keywords = map[string]int{
	"C4Context":           tC4Context,
	"C4Container":         tC4Container,
	"C4Component":         tC4Component,
	"C4Dynamic":           tC4Dynamic,
	"C4Deployment":        tC4Deployment,
	"Person":              tPERSON,
	"Person_Ext":          tPersonExt,
	"System":              tSYSTEM,
	"SystemDb":            tSystemDB,
	"SystemQueue":         tSystemQueue,
	"System_Ext":          tSystemExt,
	"SystemDb_Ext":        tSystemExtDB,
	"SystemQueue_Ext":     tSystemExtQueue,
	"Container":           tCONTAINER,
	"ContainerDb":         tContainerDB,
	"ContainerQueue":      tContainerQueue,
	"Container_Ext":       tContainerExt,
	"ContainerDb_Ext":     tContainerExtDB,
	"ContainerQueue_Ext":  tContainerExtQueue,
	"Component":           tCOMPONENT,
	"ComponentDb":         tComponentDB,
	"ComponentQueue":      tComponentQueue,
	"Component_Ext":       tComponentExt,
	"ComponentDb_Ext":     tComponentExtDB,
	"ComponentQueue_Ext":  tComponentExtQueue,
	"Enterprise_Boundary": tEnterpriseBoundary,
	"System_Boundary":     tSystemBoundary,
	"Boundary":            tBOUNDARY,
	"Container_Boundary":  tContainerBoundary,
	"Deployment_Node":     tNODE,
	"Node":                tNODE,
	"Node_L":              tNodeL,
	"Node_R":              tNodeR,
	"Rel":                 tREL,
	"BiRel":               tBIREL,
	"Rel_Up":              tRelU,
	"Rel_U":               tRelU,
	"Rel_Down":            tRelD,
	"Rel_D":               tRelD,
	"Rel_Left":            tRelL,
	"Rel_L":               tRelL,
	"Rel_Right":           tRelR,
	"Rel_R":               tRelR,
	"Rel_Back":            tRelB,
	"RelIndex":            tRelIndex,
	"UpdateElementStyle":  tUpdateElStyle,
	"UpdateRelStyle":      tUpdateRelStyle,
	"UpdateLayoutConfig":  tUpdateLayoutConfig,
}

var directions = [...]string{"TB", "BT", "RL", "LR"}

// matchDirection is the sticky /.*direction\s+XX[^\n]*/y at p, where lineEnd is the first
// line terminator at or after p: it returns the end of the match, or -1. `.*` backs off
// from the end of the line, so the last "direction" on it followed by whitespace (which
// may run onto later lines) and XX is the one matched.
func matchDirection(src common.Src, p, lineEnd int, xx string) int {
	for s := lineEnd - len("direction"); s >= p; s-- {
		if !src.HasPrefix(s, "direction") {
			continue
		}
		q := s + len("direction")
		ws := q
		for util.IsSpace(src.At(q)) {
			q++
		}
		if q == ws || !src.HasPrefix(q, xx) {
			continue
		}
		q += len(xx)
		for q < len(src) && src[q] != '\n' {
			q++
		}
		return q
	}
	return -1
}

// tokenize follows the lexer Jison compiles from c4Diagram.jison: its rule order, its start
// conditions, and what it returns once the input runs out in each of them.
func tokenize(src common.Src) tokens {
	n := len(src)
	var out tokens
	state := sINITIAL
	accValue := tAccTitleValue
	p := 0
	done := false
	// Where the next `direction` is; -2 until the first search.
	dirNext := -2
	lineEnd := -1
	dirFailed := 0

	emit := func(typ, s, e int) {
		out.types = append(out.types, typ)
		out.starts = append(out.starts, s)
		out.ends = append(out.ends, e)
		p = e
	}
	skipWs := func(q int) int {
		for q < n && util.IsSpace(int(src[q])) {
			q++
		}
		return q
	}
	upTo := func(ch string, from int) int {
		if q := src.Index(ch, from); q != -1 {
			return q
		}
		return n
	}
	// `word\s[^#\n;]+`
	statement := func(word string, typ int) bool {
		q := p + len(word)
		if !src.HasPrefix(p, word) || !util.IsSpace(src.At(q)) {
			return false
		}
		q++
		from := q
		for q < n {
			c := src[q]
			if c == 35 || c == 10 || c == 59 {
				break
			}
			q++
		}
		if q == from {
			return false
		}
		emit(typ, p, q)
		return true
	}

	for {
		// Jison notes the end of input one call before it reports it, so a rule that
		// matches nothing gets one last turn.
		if done {
			emit(tEND, n, n)
			break
		}
		if p >= n {
			done = true
		}
		c := src.At(p)

		if state == sINITIAL {
			if p >= n {
				emit(tEOF, n, n)
				continue
			}
			if dirNext != -1 && p >= dirFailed {
				if dirNext < p {
					dirNext = src.Index("direction", p)
				}
				if dirNext != -1 {
					if lineEnd < p {
						lineEnd = p
						for lineEnd < n && !util.IsLineEnd(int(src[lineEnd])) {
							lineEnd++
						}
					}
					if dirNext < lineEnd {
						matched := false
						for i := 0; i < 4 && !matched; i++ {
							if e := matchDirection(src, p, lineEnd, directions[i]); e != -1 {
								emit(tDirectionTB+i, p, e)
								matched = true
							}
						}
						if matched {
							continue
						}
						// No direction statement starts anywhere in the rest of this line either.
						dirFailed = lineEnd
					}
				}
			}
			if c == 116 {
				if e := bareTitle(src, p); e >= 0 {
					p = e
					continue
				}
				if statement("title", tTitle) {
					continue
				}
			}
			if c == 97 {
				if statement("accDescription", tAccDescription) {
					continue
				}
				title := src.HasPrefix(p, "accTitle")
				if title || src.HasPrefix(p, "accDescr") {
					q := skipWs(p + 8)
					d := src.At(q)
					if d == 58 {
						if title {
							emit(tAccTitle, p, skipWs(q+1))
							accValue = tAccTitleValue
						} else {
							emit(tAccDescr, p, skipWs(q+1))
							accValue = tAccDescrValue
						}
						state = sAcc
						continue
					}
					if d == 123 && !title {
						p = skipWs(q + 1)
						state = sAccDescrMultiline
						continue
					}
				}
			}
			if c == 37 && src.At(p+1) == 37 {
				// A comment takes the line breaks after it, so no NEWLINE follows.
				q := upTo("\n", p+2)
				for q < n && (src[q] == 10 || src[q] == 13) {
					q++
				}
				p = q
				continue
			}
			if util.IsSpace(c) {
				q := p
				last := -1
				for q < n {
					d := src[q]
					if !util.IsSpace(int(d)) {
						break
					}
					if d == 10 {
						last = q
					}
					q++
				}
				if last != -1 {
					emit(tNEWLINE, p, last+1)
				} else {
					p = q
				}
				continue
			}
			if c == 123 {
				emit(tLBRACE, p, p+1)
				continue
			}
			if c == 125 {
				emit(tRBRACE, p, p+1)
				continue
			}
			if util.IsWord(c) {
				q := p + 1
				for q < n && util.IsWord(int(src[q])) {
					q++
				}
				if q-p <= 19 {
					if typ, ok := keywords[src.String(p, q)]; ok {
						if typ > tC4Deployment {
							state = sStruct
						}
						emit(typ, p, q)
						continue
					}
				}
			}
			emit(tERROR, p, p)
			break
		}

		switch state {
		case sStruct:
			switch {
			case p >= n:
				emit(tEOFInStruct, n, n)
			case c == 40:
				q := p + 1
				for src.At(q) == 32 {
					q++
				}
				if src.At(q) == 44 {
					emit(tAttributeEmpty, p, q+1)
				} else {
					p++
				}
				state = sAttribute
			case c == 41:
				p++
				state = sINITIAL
			default:
				emit(tERROR, p, p)
				return out
			}

		case sAttribute:
			if p >= n {
				continue
			}
			switch c {
			case 41:
				p++
				state = sINITIAL
			case 44:
				if src.At(p+1) == 44 {
					emit(tAttributeEmpty, p, p+2)
				} else {
					p++
				}
			default:
				q := p
				for src.At(q) == 32 {
					q++
				}
				switch d := src.At(q); {
				case d == 34:
					if src.At(q+1) == 34 {
						emit(tAttributeEmpty, p, q+2)
					} else {
						p = q + 1
						state = sString
					}
				case d == 36:
					p = q + 1
					state = sStringKV
				default:
					// An unquoted value runs to the next comma, through closing parentheses
					// and line breaks.
					emit(tSTR, p, upTo(",", p))
				}
			}

		case sString:
			if c == 34 {
				p++
				state = sAttribute
			} else {
				emit(tSTR, p, upTo(`"`, p))
			}

		case sStringKV:
			emit(tStrKey, p, upTo("=", p))
			state = sStringKVKey

		case sStringKVKey:
			if p >= n {
				continue
			}
			q := p + 1
			if c == 61 {
				for src.At(q) == 32 {
					q++
				}
			}
			if c != 61 || src.At(q) != 34 {
				emit(tERROR, p, p)
				return out
			}
			p = q + 1
			state = sStringKVValue

		case sStringKVValue:
			if p >= n {
				continue
			}
			if c == 34 {
				p++
				state = sAttribute
			} else {
				emit(tStrValue, p, upTo(`"`, p))
			}

		case sAcc:
			emit(accValue, p, upTo("\n", p))
			state = sINITIAL

		default:
			if c == 125 {
				p++
				state = sINITIAL
			} else {
				emit(tAccDescrMultilineValue, p, upTo("}", p))
			}
		}
	}
	return out
}

// bareTitle matches /title[^\S\r\n]*(?=[\r\n]|$)\s*/y, returning its end or -1. It consumes
// line breaks too: the grammar rejects blank statements.
func bareTitle(src common.Src, p int) int {
	if !src.HasPrefix(p, "title") {
		return -1
	}
	e := p + len("title")
	for e < len(src) && src[e] != '\r' && src[e] != '\n' && util.IsSpace(int(src[e])) {
		e++
	}
	if e < len(src) && src[e] != '\r' && src[e] != '\n' {
		return -1
	}
	for e < len(src) && util.IsSpace(int(src[e])) {
		e++
	}
	return e
}
