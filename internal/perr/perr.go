// Package perr is PeleError: the error every failure to parse or render is reported as.
package perr

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// Code classifies an Error.
type Code string

// The codes upstream uses, plus Canceled for a render stopped through its context and
// Internal for a panic that was not an Error.
const (
	UnsupportedDiagram Code = "unsupported-diagram"
	Syntax             Code = "syntax"
	Semantic           Code = "semantic"
	Limit              Code = "limit"
	Canceled           Code = "canceled"
	Internal           Code = "internal"
)

// Error is a failure to parse or render a diagram. Line and Column are 1-based and count
// UTF-16 code units, as upstream does; they are 0 when the error has no position.
type Error struct {
	Code    Code
	Message string
	// Type and MermaidType are the detected diagram type, or "" when none was detected.
	Type        string
	MermaidType string
	Line        int
	Column      int
	// Snippet is the offending line with a caret under the column.
	Snippet string
	err     error
}

func (e *Error) Error() string { return e.Message }

// Unwrap returns the context error of a Canceled error, or the *PanicError of an Internal
// one.
func (e *Error) Unwrap() error { return e.err }

// New returns an Error with no position.
func New(message string, code Code) *Error {
	return &Error{Code: code, Message: message}
}

// Cancel returns the Error for a render stopped by its context.
func Cancel(cause error) *Error {
	return &Error{Code: Canceled, Message: "Render canceled: " + cause.Error(), err: cause}
}

// PanicError is a panic recovered during a render that was not an Error: a bug in the port,
// or a panic in a host's measurer or icon resolver.
type PanicError struct {
	// Value is what was passed to panic.
	Value any
	// Stack is the stack of the goroutine that panicked, as runtime/debug.Stack writes it.
	Stack []byte
}

func (p *PanicError) Error() string { return fmt.Sprintf("panic: %v", p.Value) }

// Unwrap returns Value when it is an error.
func (p *PanicError) Unwrap() error {
	err, _ := p.Value.(error)
	return err
}

// Panic returns the Internal Error for a recovered panic value and its stack.
func Panic(value any, stack []byte) *Error {
	cause := &PanicError{Value: value, Stack: stack}
	return &Error{Code: Internal, Message: fmt.Sprintf("Render panicked: %v", value), err: cause}
}

// Throw panics with e. Parsers report errors this way, as upstream throws them; the core
// recovers the panic and returns e.
func Throw(e *Error) {
	panic(e)
}

// RangeError stands for a JS RangeError: a stack overflow, or a built-in such as
// String.fromCodePoint rejecting its argument. Upstream reports every RangeError as the
// limit error "Diagram is nested too deeply.", tagged with the diagram type when one was
// detected, and the core does the same with this.
type RangeError struct{}

// ThrowRange panics with a RangeError.
func ThrowRange() {
	panic(RangeError{})
}

// Location is a position in a source, as upstream's locate() computes it.
type Location struct {
	Line    int
	Column  int
	Snippet string
}

// Edit is an input range [From, To) replaced by Length output characters, all in UTF-16
// code units.
type Edit struct{ From, To, Length int }

func stepInput(edits []Edit, at int) int {
	shift := 0
	for _, e := range edits {
		start := e.From - shift
		if at < start {
			break
		}
		if at < start+e.Length {
			return e.From + min(at-start, e.To-e.From-1)
		}
		shift += e.To - e.From - e.Length
	}
	return at + shift
}

// InputOffset maps an offset in the output of steps, applied in order, back to their input.
func InputOffset(steps [][]Edit, at int) int {
	for s := len(steps) - 1; s >= 0; s-- {
		at = stepInput(steps[s], at)
	}
	return at
}

// Origin is upstream's locatingIn() state: the text a diagram type parses, and how to find an
// offset into it in the source as written. Upstream keeps it in a module variable for the
// length of the parse; here the core hands it to Parse, and parsers pass it to Locate.
type Origin struct {
	parsed string
	units  []uint16
	find   func(at int) ([]uint16, int)
}

// NewOrigin returns the Origin for parsed, whose offsets find maps into the source.
func NewOrigin(parsed string, find func(at int) ([]uint16, int)) *Origin {
	return &Origin{parsed: parsed, find: find}
}

// Written is upstream's written(): it maps only when src is exactly the text being parsed.
// A nil Origin maps nothing.
func (o *Origin) Written(src []uint16, offset int) ([]uint16, int) {
	if o == nil {
		return src, offset
	}
	if o.units == nil {
		o.units = jsstr.UTF16(o.parsed)
	}
	if slices.Equal(src, o.units) {
		return o.find(offset)
	}
	return src, offset
}

// Locate finds the line, column and snippet for a UTF-16 offset into src, in the source as
// written when src is the text o parses.
func Locate(o *Origin, src []uint16, offset int) Location {
	src, offset = o.Written(src, offset)
	line := 1
	lineStart := 0
	for i := indexOf(src, '\n', 0); i != -1 && i < offset; i = indexOf(src, '\n', i+1) {
		line++
		lineStart = i + 1
	}
	lineEnd := indexOf(src, '\n', offset)
	if lineEnd == -1 {
		lineEnd = len(src)
	}
	column := offset - lineStart + 1
	text := jsstr.FromUTF16(src[lineStart:max(lineStart, lineEnd)])
	return Location{Line: line, Column: column, Snippet: text + "\n" + strings.Repeat("-", max(0, column-1)) + "^"}
}

func indexOf(src []uint16, c uint16, from int) int {
	for i := max(from, 0); i < len(src); i++ {
		if src[i] == c {
			return i
		}
	}
	return -1
}

// SyntaxError builds upstream's syntaxError(): a lexical error, or a parse error that ends
// with detail.
func SyntaxError(o *Origin, diagramType string, src []uint16, offset int, detail string, lexical bool) *Error {
	loc := Locate(o, src, offset)
	var message string
	if lexical {
		message = fmt.Sprintf("Lexical error on line %d. Unrecognized text.\n%s", loc.Line, loc.Snippet)
	} else {
		message = fmt.Sprintf("Parse error on line %d:\n%s\n%s", loc.Line, loc.Snippet, detail)
	}
	return &Error{
		Code: Syntax, Message: message, Type: diagramType,
		Line: loc.Line, Column: loc.Column, Snippet: loc.Snippet,
	}
}
