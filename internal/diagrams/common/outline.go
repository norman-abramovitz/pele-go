package common

import (
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// The indented outline grammar that Mermaid's mindmap and kanban diagrams share. Kanban adds
// `@{ ... }` metadata after a node and keeps `@` out of node ids.

// The outline token types (upstream's T in outline.ts).
const (
	OutlineEnd = iota
	OutlineError
	OutlineSpaceLine
	OutlineNL
	OutlineEOF
	OutlineKeyword
	OutlineSpaceList
	OutlineIcon
	OutlineClass
	OutlineNodeDStart
	OutlineNodeID
	OutlineNodeDescr
	OutlineNodeDEnd
	OutlineShapeData
)

// outlineNames are the token names as the generated parser prints them. It has no name for
// the end of input and prints its number.
var outlineNames = [...]string{
	"1", "INVALID", "SPACELINE", "NL", "EOF", "", "SPACELIST", "ICON", "CLASS",
	"NODE_DSTART", "NODE_ID", "NODE_DESCR", "NODE_DEND", "SHAPE_DATA",
}

// OutlineTokenName is a token type's name in parse errors.
func OutlineTokenName(typ int, kanban bool) string {
	if typ == OutlineKeyword {
		if kanban {
			return "KANBAN"
		}
		return "MINDMAP"
	}
	return outlineNames[typ]
}

// The node types the outline delimiters give (upstream's NODE_TYPE).
const (
	NodeTypeDefault     = 0
	NodeTypeNoBorder    = 0
	NodeTypeRoundedRect = 1
	NodeTypeRect        = 2
	NodeTypeCircle      = 3
	NodeTypeCloud       = 4
	NodeTypeBang        = 5
	NodeTypeHexagon     = 6
)

// GetType is the node type of a pair of delimiters. Any opening delimiter pairs with any
// closing one, as in Mermaid.
func GetType(start, end string) int {
	switch start {
	case "[":
		return NodeTypeRect
	case "(":
		if end == ")" {
			return NodeTypeRoundedRect
		}
		return NodeTypeCloud
	case "((":
		return NodeTypeCircle
	case ")":
		return NodeTypeCloud
	case "))":
		return NodeTypeBang
	case "{{":
		return NodeTypeHexagon
	}
	return NodeTypeDefault
}

// Decoration is an icon or a class for the last node. Only one is set.
type Decoration struct {
	Icon, Class *string
}

// OutlineDB receives the outline as the grammar's actions report it. Mindmap and kanban
// each have one.
type OutlineDB interface {
	GetType(start, end string) int
	// AddNode adds a node; shapeData is nil when the node has none.
	AddNode(level int, id, descr string, typ int, shapeData *string)
	DecorateNode(d Decoration)
}

// The lexer's states.
const (
	stateInitial = iota
	stateClass
	stateIcon
	stateNode
	stateNStr
	stateNStr2
	stateData
	stateDataStr
)

// outlineLetters is Jison's case-insensitive match, which folds ASCII letters only.
func outlineLetters(src Src, p int, word string) bool {
	return KeywordAt(src, p, word)
}

// runOf returns the end of a run of at least one code unit from p for which in holds, or -1.
func runOf(src Src, p int, in func(c uint16) bool) int {
	e := p
	for e < len(src) && in(src[e]) {
		e++
	}
	if e == p {
		return -1
	}
	return e
}

func notIn(chars string) func(uint16) bool {
	return func(c uint16) bool {
		for i := 0; i < len(chars); i++ {
			if c == uint16(chars[i]) {
				return false
			}
		}
		return true
	}
}

var (
	// /.+/y
	reLine = func(c uint16) bool { return !util.IsLineEnd(int(c)) }
	// /[^([\n){}]+/y and the kanban form, which keeps @ out
	reID       = notIn("([\n){}")
	reIDKanban = notIn("([\n){}@")
	// /[^)\](}]+/y
	reDescr = notIn(")](}")
	// /[^`"]+/y
	reMD = notIn("`\"")
	// /[^}^"]+/y
	reData = notIn("}^\"")
)

// breakLines is text.replace(/\n\s*/g, '<br/>').
func breakLines(units Src) string {
	out := make([]uint16, 0, len(units))
	for i := 0; i < len(units); {
		if units[i] != '\n' {
			out = append(out, units[i])
			i++
			continue
		}
		i++
		for i < len(units) && util.IsSpace(int(units[i])) {
			i++
		}
		for _, c := range "<br/>" {
			out = append(out, uint16(c))
		}
	}
	return jsstr.FromUTF16(out)
}

// TokenizeOutline lexes an outline.
func TokenizeOutline(src Src, kanban bool) TextTokens {
	n := len(src)
	var toks TextTokens
	keyword := "mindmap"
	id := reID
	if kanban {
		keyword = "kanban"
		id = reIDKanban
	}
	state := stateInitial
	p := 0
	nextBreak := -1

	put := func(typ int, text string, end int) {
		toks.Types = append(toks.Types, typ)
		toks.Texts = append(toks.Texts, text)
		toks.Starts = append(toks.Starts, p)
		p = end
	}
	emit := func(typ, end, next int) {
		put(typ, src.String(p, end), end)
		state = next
	}
	upTo := func(ch string) int {
		if e := src.Index(ch, p); e != -1 {
			return e
		}
		return n
	}

scan:
	for p < n {
		c := int(src[p])
		d := src.At(p + 1)
		switch state {
		case stateInitial:
			w := p
			if c <= 32 || c >= 160 {
				w = max(runOf(src, p, func(c uint16) bool { return util.IsSpace(int(c)) }), p)
			}
			switch {
			case kanban && c == 64 && d == 123:
				put(OutlineShapeData, "", p+2)
				state = stateData
			case src.At(w) == 37 && src.At(w+1) == 37:
				emit(OutlineSpaceLine, runOf(src, w, reLine), stateInitial)
			case w > p:
				if nextBreak < p {
					nextBreak = upTo("\n")
				}
				if nextBreak >= w {
					emit(OutlineSpaceList, w, stateInitial)
				} else if e := lastIndexOf(src, '\n', w-1); e > p {
					emit(OutlineSpaceLine, e+1, stateInitial)
				} else {
					emit(OutlineNL, p+1, stateInitial)
				}
			case outlineLetters(src, p, keyword) && !util.IsWord(src.At(p+len(keyword))):
				emit(OutlineKeyword, p+len(keyword), stateInitial)
			case c == 58 && d == 58 && src.At(p+2) == 58:
				p += 3
				state = stateClass
			case c == 58 && d == 58 && outlineLetters(src, p+2, "icon") && src.At(p+6) == 40:
				p += 7
				state = stateIcon
			case (c == 45 && d == 41) || (c == 40 && (d == 45 || d == 40)) || ((c == 41 || c == 123) && d == c):
				emit(OutlineNodeDStart, p+2, stateNode)
			case c == 41 || c == 40 || c == 91:
				emit(OutlineNodeDStart, p+1, stateNode)
			default:
				e := runOf(src, p, id)
				if e == -1 {
					break scan
				}
				emit(OutlineNodeID, e, stateInitial)
			}
		case stateNode:
			switch {
			case c == 34:
				if d == 96 {
					state = stateNStr2
					p += 2
				} else {
					state = stateNStr
					p++
				}
			case c == 41 || c == 40:
				end := p + 1
				if d == c || (c == 40 && d == 45) {
					end = p + 2
				}
				emit(OutlineNodeDEnd, end, stateInitial)
			case c == 93:
				emit(OutlineNodeDEnd, p+1, stateInitial)
			case (c == 125 && d == 125) || (c == 45 && d == 41):
				emit(OutlineNodeDEnd, p+2, stateInitial)
			default:
				// A lone closing brace takes the rest of its line as text.
				match := reDescr
				if c == 125 {
					match = reLine
				}
				emit(OutlineNodeDescr, runOf(src, p, match), stateNode)
			}
		case stateNStr:
			if c == 34 {
				p++
				state = stateNode
			} else {
				emit(OutlineNodeDescr, upTo(`"`), stateNStr)
			}
		case stateNStr2:
			if c == 96 && d == 34 {
				p += 2
				state = stateNode
			} else if e := runOf(src, p, reMD); e != -1 {
				emit(OutlineNodeDescr, e, stateNStr2)
			} else {
				break scan
			}
		case stateClass:
			if e := runOf(src, p, reLine); e != -1 {
				emit(OutlineClass, e, stateInitial)
			} else if c == 10 {
				p++
				state = stateInitial
			} else {
				break scan
			}
		case stateIcon:
			if c == 41 {
				p++
				state = stateInitial
			} else {
				emit(OutlineIcon, upTo(")"), stateIcon)
			}
		case stateData:
			if c == 34 {
				emit(OutlineShapeData, p+1, stateDataStr)
			} else if c == 125 {
				p++
				state = stateInitial
			} else if e := runOf(src, p, reData); e != -1 {
				emit(OutlineShapeData, e, stateData)
			} else {
				break scan
			}
		default:
			if c == 34 {
				emit(OutlineShapeData, p+1, stateData)
			} else {
				e := upTo(`"`)
				put(OutlineShapeData, breakLines(src[p:e]), e)
			}
		}
	}

	if p < n {
		put(OutlineError, "", p)
	} else {
		if state == stateInitial {
			put(OutlineEOF, "", n)
		}
		put(OutlineEnd, "", n)
	}
	return toks
}

// lastIndexOf is src.lastIndexOf(c, from).
func lastIndexOf(src Src, c uint16, from int) int {
	for i := min(from, len(src)-1); i >= 0; i-- {
		if src[i] == c {
			return i
		}
	}
	return -1
}

const outlineStop = "'SPACELINE', 'NL', 'EOF'"

func outlineStarts(typ int) bool {
	switch typ {
	case OutlineSpaceLine, OutlineSpaceList, OutlineIcon, OutlineClass, OutlineNodeDStart, OutlineNodeID:
		return true
	}
	return false
}

func isOutlineStop(typ int) bool {
	return typ == OutlineSpaceLine || typ == OutlineNL || typ == OutlineEOF
}

// ParseOutline makes the calls on db that the grammar's actions make, in the same order, and
// reports errors in the state where the generated parser would.
func ParseOutline(src Src, db OutlineDB, kanban bool, o *perr.Origin) {
	diagram := "mindmap"
	data := ""
	if kanban {
		diagram = "kanban"
		data = ", 'SHAPE_DATA'"
	}
	toks := TokenizeOutline(src, kanban)
	types, texts, starts := toks.Types, toks.Texts, toks.Starts
	i := 0

	fail := func(expected string) {
		t := types[i]
		if t == OutlineError {
			perr.Throw(perr.SyntaxError(o, diagram, src, starts[i], "", true))
		}
		perr.Throw(perr.SyntaxError(o, diagram, src, starts[i], "Expecting "+expected+", got '"+OutlineTokenName(t, kanban)+"'", false))
	}

	keyword := "'" + OutlineTokenName(OutlineKeyword, kanban) + "'"
	if types[i] == OutlineSpaceLine {
		i++
		for types[i] == OutlineSpaceLine || types[i] == OutlineNL {
			i++
		}
		if types[i] != OutlineKeyword {
			fail("'SPACELINE', 'NL', " + keyword)
		}
	} else if types[i] != OutlineKeyword {
		fail("'SPACELINE', " + keyword)
	}
	i++
	first := "'SPACELINE', 'NL', 'SPACELIST', 'ICON', 'CLASS', 'NODE_DSTART', 'NODE_ID'"
	if types[i] == OutlineNL {
		i++
		first = "'SPACELINE', 'SPACELIST', 'ICON', 'CLASS', 'NODE_DSTART', 'NODE_ID'"
	}

	for {
		t := types[i]
		level := 0
		indented := t == OutlineSpaceList
		if indented {
			level = jsstr.Len(texts[i])
			i++
			t = types[i]
		}
		switch {
		case t == OutlineNodeID || t == OutlineNodeDStart:
			var id *string
			var descr string
			typ := 0
			if t == OutlineNodeID {
				id = &texts[i]
				i++
			}
			if types[i] == OutlineNodeDStart {
				start := texts[i]
				i++
				if types[i] != OutlineNodeDescr {
					fail("'NODE_DESCR'")
				}
				descr = texts[i]
				i++
				if types[i] != OutlineNodeDEnd {
					fail("'NODE_DEND'")
				}
				end := texts[i]
				i++
				if !isOutlineStop(types[i]) && types[i] != OutlineShapeData {
					fail(outlineStop + data)
				}
				typ = db.GetType(start, end)
			} else {
				if !isOutlineStop(types[i]) && types[i] != OutlineShapeData {
					fail(outlineStop + ", 'NODE_DSTART'" + data)
				}
				descr = *id
			}
			nodeID := descr
			if id != nil {
				nodeID = *id
			}
			if types[i] == OutlineShapeData {
				shapeData := texts[i]
				i++
				for types[i] == OutlineShapeData {
					shapeData += texts[i]
					i++
				}
				if !isOutlineStop(types[i]) {
					fail(outlineStop + data)
				}
				db.AddNode(level, nodeID, descr, typ, &shapeData)
			} else {
				db.AddNode(level, nodeID, descr, typ, nil)
			}
		case t == OutlineIcon || t == OutlineClass:
			text := texts[i]
			i++
			if !isOutlineStop(types[i]) {
				fail(outlineStop)
			}
			if t == OutlineIcon {
				db.DecorateNode(Decoration{Icon: &text})
			} else {
				db.DecorateNode(Decoration{Class: &text})
			}
		case indented:
			if !isOutlineStop(t) {
				fail(outlineStop + ", 'ICON', 'CLASS', 'NODE_DSTART', 'NODE_ID'")
			}
		case t == OutlineSpaceLine:
			i++
			if !isOutlineStop(types[i]) {
				fail(outlineStop)
			}
		default:
			fail(first)
		}

		i++
		for types[i] == OutlineNL || types[i] == OutlineEOF {
			i++
		}
		if types[i] == OutlineEnd {
			return
		}
		if !outlineStarts(types[i]) {
			fail(outlineStop + ", 'SPACELIST', 'ICON', 'CLASS', 'NODE_DSTART', 'NODE_ID'")
		}
	}
}
