// Package pele renders Mermaid diagrams to SVG. It is a Go port of Pele
// (https://github.com/obsidianmd/pele) that produces the same SVG as the upstream release it
// follows, byte for byte, given the same options and text measurer.
//
// Diagram types are registered by importing them: import
// github.com/Broderick-Westrope/pele-go/all for every type, or register single types from
// the diagrams packages with Register.
//
// Render is safe for concurrent use and stops when its context is done. Labels are measured
// with the measurer DefaultMeasurer returns unless Options.Measurer is set; the fontmeasure
// package measures with real fonts.
package pele

import (
	"context"
	"errors"
	"fmt"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsjson"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

// UpstreamVersion and UpstreamCommit are the Pele release this package matches and the
// commit its tag points to. This module's own version is separate: it moves with changes
// here, and a new upstream release only changes these.
const (
	UpstreamVersion = "1.9.0"
	UpstreamCommit  = "e7f092a4de147cc32210913c20e36eaa5fcd0ef1"
)

// Options are the render options, upstream's RenderOptions. The zero value renders as
// upstream does with no options. Pointer fields are unset when nil, so that a zero value can
// be told from no value; set them with Ptr. The fields are:
//
//   - Measurer measures label text. Nil means the measurer DefaultMeasurer returns. It must
//     be safe for concurrent use and must not panic; see TextMeasurer.
//   - FontFamily and FontFamilyMono are the CSS font families the SVG names where the page
//     sets no --pele-font or --pele-font-mono. Nil means sans-serif and monospace.
//   - FontSize is the label size in pixels. Nil means 16.
//   - IDPrefix prefixes the ids in the SVG, so several diagrams can share a page. Nil means
//     "pele".
//   - Responsive makes the SVG scale to its container. Nil leaves it on unless the
//     diagram's config sets useMaxWidth: false.
//   - MaxWidth is the width the host has for the diagram, in pixels. A type that can draw
//     itself narrower does.
//   - AutoDirection lets a diagram that runs across be drawn running down when it is wider
//     than MaxWidth and MaxWidth is under DirectionBreakpoint. Nil means on and 640.
//   - Padding is the space around the drawing in pixels. Nil means 8.
//   - Limit is the longest source accepted, in UTF-16 code units. Nil means 50,000.
//     OutputLimit is the longest SVG returned, nil meaning 4,000,000. MaxEdges is the most
//     edges a flowchart may have, nil meaning 5,000. The limits are numbers, as upstream's
//     are: math.Inf(1) means no limit.
//   - LinkSchemes and ImageSchemes are the schemes a link or an image may use besides a
//     relative address. Nil means http, https, mailto and tel for links and http and https
//     for images; an empty, non-nil list allows none.
//   - Links and Images, when false, drop every link or image. Nil means true.
//   - LinkRel is the rel attribute of links. Nil means "noopener"; "" omits it.
//   - Icons returns SVG markup for an icon name, or "" when the host has none. It must be
//     safe for concurrent use if Render is called concurrently with it, and must not panic:
//     Render recovers a panic in it as a CodeInternal error, but the render is lost.
//   - Math typesets a $$...$$ expression in a label: it gets the TeX between the $$ and the
//     font size in pixels, and returns the markup to draw, or nil to leave the expression as
//     written. Only the types Mermaid typesets math in ask it (flowchart, swimlane, sequence,
//     class, state, er, requirement, block, mindmap, kanban and usecase); diagram titles stay
//     text. It is asked once per expression and size in a render; markup whose width or
//     height is not positive and finite is dropped. Nil means none. It must be safe for
//     concurrent use if Render is called concurrently with it, and must not panic: Render
//     recovers a panic in it as a CodeInternal error, but the render is lost.
//   - Config is Mermaid configuration from the host, made with ParseConfig. The diagram's
//     own front matter and directives win over it, key by key at the top level.
//   - Now is the time gantt charts treat as now. The zero time means the current time.
//   - Location is the time zone gantt charts read dates in, standing in for the JS process
//     time zone. Nil means time.Local.
type Options = core.Options

// Config is Mermaid configuration, an object in JavaScript's key order. Make one with
// ParseConfig. Its values are what JSON.parse gives: nil, bool, float64, string, []any and
// *Config. Read them with Get and Keys; Set adds or replaces a key.
type Config = jsobj.Object

// ParseConfig reads Mermaid configuration from JSON text, which must hold an object. Keys
// keep their order, as they would in a JS object.
func ParseConfig(jsonText string) (*Config, error) {
	v, err := jsjson.Parse(jsonText)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	obj, ok := v.(*jsobj.Object)
	if !ok {
		return nil, errors.New("parse config: not a JSON object")
	}
	return obj, nil
}

// Ptr returns a pointer to a copy of v, for setting the pointer fields of Options:
//
//	pele.Options{FontSize: pele.Ptr(14.0), Responsive: pele.Ptr(false)}
func Ptr[T any](v T) *T {
	return &v
}

// MathMarkup is math typeset by Options.Math. Markup is SVG drawn in the expression's box,
// or, when HTML is set, HTML drawn in a foreignObject, which needs the typesetter's CSS on
// the page. Width and Height are its size in pixels. The markup is trusted: it goes into the
// SVG as given.
type MathMarkup = core.MathMarkup

// Link is an element of a rendered diagram that links somewhere. ID is the id of the
// element in the SVG, Href is where it links, and Internal reports a link to another
// element of the same diagram, such as a flowchart node's click target.
type Link = core.Link

// Result is a rendered diagram.
type Result struct {
	// SVG is the drawing. A lone surrogate the source carried into the drawing is written as
	// U+FFFD, as encoding a JS string as UTF-8 does.
	SVG string
	// Width and Height are the size of the drawing in pixels, as its viewBox has them.
	Width  float64
	Height float64
	// Links are the elements of the drawing that link somewhere, in document order.
	Links []Link
	Type  DiagramType
	// MermaidType is the name Mermaid reports for the type, such as flowchart-v2 for graph TD.
	MermaidType string
}

// DiagramType names a diagram type, such as "flowchart" or "info".
type DiagramType string

// Error is a failure to parse or render a diagram. Use errors.As to read one. Its fields
// are:
//
//   - Code classifies the failure; see the Code constants.
//   - Message is upstream's message, which Error returns.
//   - Type and MermaidType are the detected diagram type, or "" when none was detected.
//   - Line and Column locate the failure in the source. They are 1-based and count UTF-16
//     code units, as upstream does, and are 0 when the error has no position.
//   - Snippet is the offending line with a caret under the column, or "".
//
// An Error with CodeCanceled wraps the context's error, so errors.Is(err,
// context.DeadlineExceeded) works. An Error with CodeInternal wraps a *PanicError.
type Error = perr.Error

// PanicError is what an Error with CodeInternal wraps: a panic Render recovered that was
// not one of its own errors, from a bug in this package or from the host's TextMeasurer or
// Options.Icons. Value is the panic value and Stack the panicking goroutine's stack.
// Unwrap returns Value when it is an error.
type PanicError = perr.PanicError

// ErrorCode classifies an Error.
type ErrorCode = perr.Code

// The error codes. All but CodeCanceled and CodeInternal are upstream's.
const (
	// CodeUnsupportedDiagram reports source of no known or no registered diagram type.
	CodeUnsupportedDiagram = perr.UnsupportedDiagram
	// CodeSyntax reports source that does not parse.
	CodeSyntax = perr.Syntax
	// CodeSemantic reports source that parses but asks for something that does not exist,
	// such as an unknown flowchart shape.
	CodeSemantic = perr.Semantic
	// CodeLimit reports source past one of the Options limits, or nested too deeply.
	CodeLimit = perr.Limit
	// CodeCanceled reports a render stopped by its context. The Error wraps the context's
	// error.
	CodeCanceled = perr.Canceled
	// CodeInternal reports a panic during the render that was not one of its errors. The
	// Error wraps a *PanicError, and its message names the panic value.
	CodeInternal = perr.Internal
)

// TextMeasurer measures the advance width of text in pixels:
//
//	Width(text string, size float64, style Style) float64
//
// size is the font size in pixels and style the emphasis the text is drawn with. text may
// hold lone surrogates, encoded as UTF-8 would encode their code points. Output is
// byte-identical to upstream's only when the two measurers return the same widths.
// Implementations must be safe for concurrent use and must not panic: Render recovers a
// panic in Width as a CodeInternal error, but the render is lost.
type TextMeasurer = text.Measurer

// Style is a set of emphasis bits passed to a TextMeasurer.
type Style = text.Style

// The Style bits.
const (
	Bold   = text.Bold
	Italic = text.Italic
	// Mono marks monospace text, such as a code span.
	Mono = text.Mono
	// Sub and Sup mark subscript and superscript text, which the SVG draws at
	// font-size="smaller". A measurer should measure it at that smaller size: the built-in
	// ones divide the size by 1.2, as upstream does.
	Sub = text.Sub
	Sup = text.Sup
	// Underline marks underlined text. It does not change the width.
	Underline = text.Underline
)

// DefaultMeasurer returns the measurer Render uses when Options.Measurer is nil: upstream's
// metricsMeasurer, which estimates widths from a table of Arial advances. Passing it as
// Options.Measurer renders the same as leaving Measurer nil. It is safe for concurrent use.
func DefaultMeasurer() TextMeasurer {
	return text.Metrics
}

// Diagram is a diagram type, as the diagrams packages export them in their Diagram
// variable. Its parser and renderer use internal types, so diagram types can only come from
// this module.
type Diagram = core.Diagram

// Register makes diagram types available to Render. Call it before the first Render,
// normally from init; it panics afterwards.
func Register(diagrams ...*Diagram) {
	core.Register(diagrams...)
}

// Render renders Mermaid source to SVG. Failures are returned as *Error. Render is safe for
// concurrent use. Strings in the result and in errors are valid UTF-8. A nil ctx is
// context.Background().
func Render(ctx context.Context, source string, opts Options) (Result, error) {
	res, err := core.Render(ctx, source, opts)
	if err != nil {
		var pe *perr.Error
		if errors.As(err, &pe) {
			wellFormed := *pe
			wellFormed.Message = jsstr.WellFormed(pe.Message)
			wellFormed.Snippet = jsstr.WellFormed(pe.Snippet)
			return Result{}, &wellFormed
		}
		return Result{}, err
	}
	links := make([]Link, len(res.Links))
	for i, l := range res.Links {
		links[i] = Link{ID: jsstr.WellFormed(l.ID), Href: jsstr.WellFormed(l.Href), Internal: l.Internal}
	}
	return Result{
		SVG:         jsstr.WellFormed(res.SVG),
		Width:       res.Width,
		Height:      res.Height,
		Links:       links,
		Type:        DiagramType(res.Type),
		MermaidType: res.MermaidType,
	}, nil
}

// DetectType returns the diagram type of source, whether or not it is registered. It
// returns false when no type is detected or the front matter cannot be read.
func DetectType(source string) (DiagramType, bool) {
	typ, ok := core.DetectType(source)
	return DiagramType(typ), ok
}

// Supports reports whether source is of a registered diagram type. Unlike Render, it does
// not stop Register from being called afterwards.
func Supports(source string) bool {
	return core.Supports(source)
}
