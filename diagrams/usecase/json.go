package usecase

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsjson"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	entities "github.com/Broderick-Westrope/pele-go/internal/text"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

type orderedJSON struct {
	value         *jsobj.Object
	propertyOrder map[string][]string
}

// walkError is thrown with the offset of the first character that is not valid JSON.
type walkError struct{ offset int }

type frame struct {
	object  bool
	pointer string
	// keys is shared with the order map, as upstream's array is.
	keys *[]string
	// For each key read so far, the stretch of created its value added.
	ranges map[string][2]int
	key    string
	from   int
	index  int
}

const (
	stateValue = iota
	stateKey
	stateAfter
)

// pointerKey escapes a key for a JSON Pointer.
func pointerKey(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

// propertyOrder walks JSON text that JSON.parse has accepted, to recover the order its keys
// were written in, which JSON.parse loses for integer-like keys. On text JSON.parse rejected
// it finds where. Nesting is kept on a stack of its own, so depth is limited by memory and not
// by the call stack.
func propertyOrder(text common.Src) map[string][]string {
	order := map[string]*[]string{}
	var created []string
	var stack []*frame
	n := len(text)
	at := 0
	pointer := ""
	state := stateValue
	throw := func(offset int) { panic(walkError{offset}) }

	skip := func() {
		for at < n {
			c := text[at]
			if c != 32 && c != 9 && c != 10 && c != 13 {
				return
			}
			at++
		}
	}

	readString := func(decode bool) string {
		at++
		var value []uint16
		for at < n {
			start := at
			ch := text[at]
			at++
			if ch == '"' {
				return jsstr.FromUTF16(value)
			}
			if ch < 0x20 {
				throw(start)
			}
			if ch != '\\' {
				if decode {
					value = append(value, ch)
				}
				continue
			}
			escapeAt := at
			escape := text.At(at)
			at++
			var decoded uint16
			switch escape {
			case '"', '\\', '/':
				decoded = uint16(escape)
			case 'b':
				decoded = '\b'
			case 'f':
				decoded = '\f'
			case 'n':
				decoded = '\n'
			case 'r':
				decoded = '\r'
			case 't':
				decoded = '\t'
			case 'u':
				// /^[\dA-Fa-f]{4}$/ on text.slice(at, at + 4).
				if at+4 > n {
					throw(at)
				}
				var unit uint16
				for k := range 4 {
					c := int(text[at+k])
					if !util.IsHex(c) {
						throw(at)
					}
					unit = unit<<4 | uint16(hexDigit(c))
				}
				decoded = unit
				at += 4
			default:
				throw(escapeAt)
			}
			if decode {
				value = append(value, decoded)
			}
		}
		throw(at)
		return ""
	}

	literal := func(word string) {
		for k := 0; k < len(word); k++ {
			if text.At(at+k) != int(word[k]) {
				throw(at + k)
			}
		}
		at += len(word)
	}

	number := func() {
		if text.At(at) == 45 {
			at++
		}
		if text.At(at) == 48 {
			at++
		} else if util.IsDigit(text.At(at)) {
			for util.IsDigit(text.At(at)) {
				at++
			}
		} else {
			throw(at)
		}
		if text.At(at) == 46 {
			at++
			if !util.IsDigit(text.At(at)) {
				throw(at)
			}
			for util.IsDigit(text.At(at)) {
				at++
			}
		}
		if text.At(at)|32 == 101 {
			at++
			if text.At(at) == 43 || text.At(at) == 45 {
				at++
			}
			if !util.IsDigit(text.At(at)) {
				throw(at)
			}
			for util.IsDigit(text.At(at)) {
				at++
			}
		}
	}

	skip()
	for {
		if state == stateValue {
			skip()
			c := text.At(at)
			state = stateAfter
			switch {
			case c == 123:
				at++
				keys := &[]string{}
				order[pointer] = keys
				created = append(created, pointer)
				skip()
				if text.At(at) == 125 {
					at++
				} else {
					stack = append(stack, &frame{object: true, pointer: pointer, keys: keys, ranges: map[string][2]int{}})
					state = stateKey
				}
			case c == 91:
				at++
				skip()
				if text.At(at) == 93 {
					at++
				} else {
					stack = append(stack, &frame{object: false, pointer: pointer, keys: &[]string{}, ranges: map[string][2]int{}})
					pointer += "/0"
					state = stateValue
				}
			case c == 34:
				readString(false)
			case c == 116:
				literal("true")
			case c == 102:
				literal("false")
			case c == 110:
				literal("null")
			case c == 45 || util.IsDigit(c):
				number()
			default:
				throw(at)
			}
		} else if state == stateKey {
			f := stack[len(stack)-1]
			if text.At(at) != 34 {
				throw(at)
			}
			key := readString(true)
			// A repeated key keeps its first place in the order and takes the nested order of
			// its last value.
			if r, ok := f.ranges[key]; !ok {
				*f.keys = append(*f.keys, key)
			} else {
				for k := r[0]; k < r[1]; k++ {
					delete(order, created[k])
				}
			}
			f.key = key
			f.from = len(created)
			skip()
			if text.At(at) != 58 {
				throw(at)
			}
			at++
			pointer = f.pointer + "/" + pointerKey(key)
			state = stateValue
		} else {
			if len(stack) == 0 {
				break
			}
			f := stack[len(stack)-1]
			if f.object {
				f.ranges[f.key] = [2]int{f.from, len(created)}
			} else {
				f.index++
			}
			skip()
			c := text.At(at)
			closer := 93
			if f.object {
				closer = 125
			}
			if c == closer {
				at++
				stack = stack[:len(stack)-1]
			} else if c != 44 {
				throw(at)
			} else {
				at++
				skip()
				pointer = f.pointer + "/" + jsnum.Format(float64(f.index))
				if f.object {
					state = stateKey
				} else {
					state = stateValue
				}
			}
		}
	}
	skip()
	if at != n {
		throw(at)
	}
	out := make(map[string][]string, len(order))
	for k, v := range order {
		out[k] = *v
	}
	return out
}

func hexDigit(c int) int {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	return c - 'A' + 10
}

func locationAt(text common.Src, offset float64, line, column float64) (float64, float64) {
	end := int(min(max(offset, 0), float64(len(text))))
	for i := 0; i < end; i++ {
		c := text[i]
		if c == 13 {
			line++
			column = 1
		} else if c == 10 {
			if i == 0 || text[i-1] != 13 {
				line++
				column = 1
			}
		} else {
			column++
		}
	}
	return line, column
}

// boundaryAt is \b at byte i of s: JS's \b sees only ASCII word characters, and the bytes of
// other characters are none.
func boundaryAt(s string, i int) bool {
	before := i > 0 && util.IsWord(int(s[i-1]))
	after := i < len(s) && util.IsWord(int(s[i]))
	return before != after
}

// wordThenDigits matches /\b<word> (\d+)\b/ at byte i of s, returning the digits' value as
// parseInt reads them and where they end.
func wordThenDigits(s string, i int, word string) (float64, int, bool) {
	if !boundaryAt(s, i) || !strings.HasPrefix(s[i:], word+" ") {
		return 0, 0, false
	}
	start := i + len(word) + 1
	end := start
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	// \d+ backtracks to find a boundary, but a run of digits followed by a word character
	// has none inside it either.
	if end == start || !boundaryAt(s, end) {
		return 0, 0, false
	}
	return jsnum.Parse(s[start:end]), end, true
}

// hasFold reports whether s contains the lower-case ASCII word as a JS /i regex without u
// finds it: an ASCII letter matches only its own other case.
func hasFold(s, word string) bool {
	for i := 0; i+len(word) <= len(s); i++ {
		match := true
		for k := 0; k < len(word) && match; k++ {
			c := s[i+k]
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			match = c == word[k]
		}
		if match {
			return true
		}
	}
	return false
}

// locationFromMessage: engines word JSON.parse errors differently, some give an offset, some a
// line and column.
func locationFromMessage(message string, text common.Src, line, column float64) (float64, float64, bool) {
	for i := 0; i < len(message); i++ {
		if position, _, ok := wordThenDigits(message, i, "position"); ok {
			l, c := locationAt(text, position, line, column)
			return l, c, true
		}
	}
	for i := 0; i < len(message); i++ {
		localLine, end, ok := wordThenDigits(message, i, "line")
		if !ok {
			continue
		}
		if end >= len(message) || message[end] != ' ' {
			continue
		}
		localColumn, _, ok := wordThenDigits(message, end+1, "column")
		if !ok {
			continue
		}
		if localLine == 1 {
			return line + localLine - 1, column + localColumn - 1, true
		}
		return line + localLine - 1, localColumn, true
	}
	if hasFold(message, "unexpected end") || hasFold(message, "end of json input") {
		l, c := locationAt(text, float64(len(text)), line, column)
		return l, c, true
	}
	return 0, 0, false
}

func jsonError(message string, line, column float64) *perr.Error {
	return &perr.Error{
		Code:    perr.Syntax,
		Message: message + " (line " + jsnum.Format(line) + ", column " + jsnum.Format(column) + ")",
		Type:    "usecase",
		Line:    int(line),
		Column:  int(column),
	}
}

// parseOrderedJSON parses the body of a `json` node. origin gives the line and column the text
// starts at in the diagram.
func parseOrderedJSON(text common.Src, origin func() (int, int)) orderedJSON {
	str := jsstr.FromUTF16(text)
	if message := jsjson.V8Message(str); message != "" {
		// Reparse restored entity codes so error messages and positions match the source.
		shown := text
		if written := entities.WrittenCodes(str); written != str {
			if again := jsjson.V8Message(written); again != "" {
				message = again
			}
			shown = jsstr.UTF16(written)
		}
		ol, oc := origin()
		line, column, ok := locationFromMessage(message, shown, float64(ol), float64(oc))
		if !ok {
			offset := 0
			func() {
				defer func() {
					if r := recover(); r != nil {
						w, isWalk := r.(walkError)
						if !isWalk {
							panic(r)
						}
						offset = w.offset
					}
				}()
				propertyOrder(shown)
			}()
			line, column = locationAt(shown, float64(offset), float64(ol), float64(oc))
		}
		perr.Throw(jsonError("Invalid JSON: "+message, line, column))
	}
	parsed, err := jsjson.Parse(str)
	if err != nil {
		panic("jsjson.Parse rejects text V8 accepts: " + err.Error())
	}
	value, ok := parsed.(*jsobj.Object)
	if !ok {
		line, column := origin()
		perr.Throw(jsonError("JSON value must have an object root", float64(line), float64(column)))
	}
	return orderedJSON{value: value, propertyOrder: propertyOrder(text)}
}
