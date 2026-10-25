// Package core is upstream's src/core.ts: the registry of diagram types and the render
// pipeline from source text to SVG.
package core

import (
	"context"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Broderick-Westrope/pele-go/internal/detect"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/preprocess"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

// Options are the render options. Pointer fields are unset when nil, so a zero value can be
// told from no value, as upstream tells undefined from 0 or "".
type Options struct {
	// Measurer measures label text. Nil uses the built-in Arial table. It must be safe for
	// concurrent use and must not panic.
	Measurer       text.Measurer
	FontFamily     *string
	FontFamilyMono *string
	FontSize       *float64
	IDPrefix       *string
	// Responsive makes the SVG scale to its container. Nil leaves it on unless the diagram's
	// config sets useMaxWidth: false.
	Responsive          *bool
	MaxWidth            *float64
	AutoDirection       *bool
	DirectionBreakpoint *float64
	Padding             *float64
	// Limit is the longest source accepted, in UTF-16 code units. Nil means 50,000. The
	// limits are numbers, as upstream's are: math.Inf(1) means no limit.
	Limit *float64
	// OutputLimit is the longest SVG returned, in UTF-16 code units. Nil means 4,000,000.
	OutputLimit *float64
	// MaxEdges is the most edges a flowchart may have. Nil means 5,000.
	MaxEdges *float64
	// LinkSchemes and ImageSchemes are the schemes a link or an image may use besides a
	// relative address. Nil leaves upstream's defaults; an empty, non-nil list allows none.
	LinkSchemes  []string
	ImageSchemes []string
	Links        *bool
	Images       *bool
	LinkRel      *string
	// Icons returns SVG markup for an icon name, or "" when the host has none. It must be
	// safe for concurrent use and must not panic.
	Icons func(name string) string
	// Math typesets the TeX of a $$...$$ expression in a label at a font size in pixels, or
	// returns nil to leave the expression as written. Only the types Mermaid typesets math
	// in ask it. It must be safe for concurrent use and must not panic.
	Math func(tex string, size float64) *MathMarkup
	// Now is the time gantt charts treat as now. The zero time means the current time.
	Now time.Time
	// Location is the time zone gantt charts read dates in. Nil means time.Local.
	Location *time.Location
	// Config is Mermaid configuration from the host. The diagram's own front matter and
	// directives win over it, key by key at the top level.
	Config *jsobj.Object
}

// MathMarkup is typeset math: SVG markup, or HTML markup when HTML is set, which then needs
// the typesetter's CSS. Width and Height are in pixels and must be positive and finite.
type MathMarkup = text.MathMarkup

// Link is an element of a rendered diagram that links somewhere.
type Link struct {
	ID       string
	Href     string
	Internal bool
}

// Rendered is what a diagram type's renderer returns.
type Rendered struct {
	SVG    string
	Width  float64
	Height float64
	Links  []Link
}

// Result is a rendered diagram. SVG is WTF-8: lone surrogates upstream would put in the
// output are kept.
type Result struct {
	Rendered
	Type        string
	MermaidType string
}

// Limits are set by the host. A diagram's own configuration cannot change them.
type Limits struct {
	MaxEdges float64
	// Origin locates errors in the source as written. It is not a limit, but rides along
	// with them to every parser; upstream keeps it in a module variable during the parse.
	Origin *perr.Origin
}

// Context is what a diagram type's renderer gets besides its model: the options as the core
// resolved them, and per-render state upstream keeps in module variables.
type Context struct {
	Ctx     context.Context
	Options Options
	// Responsive is options.responsive after the core defaulted it.
	Responsive bool
	Icons      *text.Icons
}

// CheckCanceled stops the render with a Canceled error once its context is done. Upstream has
// no equivalent; renderers call it at the heads of loops that can run long, as layout does.
func (c *Context) CheckCanceled() {
	checkContext(c.Ctx)
}

// Measurer returns the measurer to lay labels out with.
func (c *Context) Measurer() text.Measurer {
	if c.Options.Measurer != nil {
		return c.Options.Measurer
	}
	return text.Metrics
}

// Diagram is what a diagram type provides. Parse is given text that has been through
// preprocessing and entity encoding and ends with a newline, as Mermaid's parsers expect.
// Both report failures by panicking with a *perr.Error, as upstream throws them.
type Diagram struct {
	Type string
	// Section is the type's section in Mermaid's config, when it is not the type's name.
	Section string
	// KeepComments asks for the text with its %% comment lines kept, for a type that
	// reports source positions.
	KeepComments bool
	// Math enables $$...$$ in labels.
	Math   bool
	Parse  func(source string, config *jsobj.Object, title *string, lineOffset int, limits Limits) any
	Render func(model any, config *jsobj.Object, rc *Context) Rendered
}

var (
	registryMu sync.Mutex
	registry   = map[string]*Diagram{}
	// sealed is set by the first render. Registering after it is a programming error.
	sealed atomic.Bool
)

// Register makes diagram types available to Render. It must be called before the first
// render, normally from init.
func Register(diagrams ...*Diagram) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if sealed.Load() {
		panic("pele: Register called after the first Render")
	}
	for _, d := range diagrams {
		registry[d.Type] = d
	}
}

func lookup(typ string) (*Diagram, bool) {
	if !sealed.Load() {
		registryMu.Lock()
		sealed.Store(true)
		registryMu.Unlock()
	}
	d, ok := registry[typ]
	return d, ok
}

// Registered reports whether a diagram type is available. Unlike a render, it does not seal
// the registry.
func Registered(typ string) bool {
	registryMu.Lock()
	defer registryMu.Unlock()
	_, ok := registry[typ]
	return ok
}

// Upstream's defaults.
const (
	// DefaultLimit is Mermaid's default maxTextSize.
	DefaultLimit = 50000
	// DefaultOutputLimit bounds the SVG: a source within the limit can still describe a
	// drawing of many megabytes.
	DefaultOutputLimit = 4_000_000
	// DefaultMaxEdges bounds flowchart edges, which `A & B --> C & D` multiplies.
	DefaultMaxEdges = 5000
)

// DetectType returns the diagram type of text, or false when none is detected or the front
// matter cannot be read.
func DetectType(text string) (typ string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, isPeleErr := r.(*perr.Error); !isPeleErr {
				if _, isRange := r.(perr.RangeError); !isRange {
					panic(r)
				}
			}
			typ, ok = "", false
		}
	}()
	d, found := detect.Detect(preprocess.Preprocess(jsstr.Normalize(text), nil).Text)
	return d.Type, found
}

// Supports reports whether text is of a registered diagram type.
func Supports(text string) bool {
	typ, ok := DetectType(text)
	return ok && Registered(typ)
}

type parsed struct {
	detect.Detected
	diagram *Diagram
	model   any
	config  *jsobj.Object
}

func optNumber(p *float64, def float64) float64 {
	if p == nil {
		return def
	}
	return *p
}

func parseSource(ctx context.Context, source string, opts Options) parsed {
	limit := optNumber(opts.Limit, DefaultLimit)
	if float64(jsstr.Len(source)) > limit {
		perr.Throw(perr.New("Diagram source is longer than the limit of "+jsnum.Format(limit)+" characters.", perr.Limit))
	}
	pre := preprocess.Preprocess(source, nil)
	found, ok := detect.Detect(pre.Text)
	if !ok {
		perr.Throw(perr.New("No diagram type detected for the given text.", perr.UnsupportedDiagram))
	}
	diagram, ok := lookup(found.Type)
	if !ok {
		e := perr.New(`Diagram type "`+found.Type+`" is not supported yet.`, perr.UnsupportedDiagram)
		e.Type, e.MermaidType = found.Type, found.MermaidType
		perr.Throw(e)
	}
	checkContext(ctx)
	config := pre.Config
	if opts.Config != nil {
		// {...extra, ...pre.config}. Deep-copied so a render never changes the caller's
		// config, which other renders may be reading.
		config = opts.Config.DeepClone()
		for _, k := range pre.Config.Keys() {
			v, _ := pre.Config.Get(k)
			config.Set(k, v)
		}
	}
	src := pre.Text
	if diagram.KeepComments {
		src = pre.WithComments
	}
	parsedText := preprocess.EncodeEntities(src, nil) + "\n"
	origin := perr.NewOrigin(parsedText, inSource(source, diagram.KeepComments))
	var model any
	tagErrors(found, func() {
		model = diagram.Parse(parsedText, config, pre.Title, pre.LineOffset,
			Limits{MaxEdges: optNumber(opts.MaxEdges, DefaultMaxEdges), Origin: origin})
	})
	return parsed{Detected: found, diagram: diagram, model: model, config: config}
}

// inSource records preprocessing edits only when a source position is needed.
func inSource(text string, keepComments bool) func(at int) ([]uint16, int) {
	var steps *preprocess.Steps
	var source []uint16
	return func(at int) ([]uint16, int) {
		if steps == nil {
			steps = &preprocess.Steps{}
			pre := preprocess.Preprocess(text, steps)
			src := pre.WithComments
			if !keepComments {
				src = preprocess.CleanupComments(pre.WithComments, steps)
			}
			preprocess.EncodeEntities(src, steps)
			// Match the line break appended for parsing.
			source = jsstr.UTF16(preprocess.NormalizeLineBreaks(text) + "\n")
		}
		return source, perr.InputOffset(*steps, at)
	}
}

func checkContext(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		perr.Throw(perr.Cancel(err))
	}
}

// tagErrors runs fn and gives errors it throws the detected type, turning a RangeError into
// the limit error.
func tagErrors(found detect.Detected, fn func()) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		switch e := r.(type) {
		case perr.RangeError:
			err := perr.New("Diagram is nested too deeply.", perr.Limit)
			err.Type, err.MermaidType = found.Type, found.MermaidType
			panic(err)
		case *perr.Error:
			if e.Type == "" {
				e.Type = found.Type
			}
			if e.MermaidType == "" {
				e.MermaidType = found.MermaidType
			}
			e.Message = text.WrittenCodes(e.Message)
			e.Snippet = text.WrittenCodes(e.Snippet)
			panic(e)
		default:
			panic(r)
		}
	}()
	fn()
}

// recoverInto turns a thrown error into err, as upstream's guarded() does at the edge of
// the public API. Any other panic, from a bug or from the host's measurer or icon resolver,
// becomes an Internal error rather than crash the host.
func recoverInto(err *error) {
	r := recover()
	if r == nil {
		return
	}
	switch e := r.(type) {
	case *perr.Error:
		e.Message, e.Snippet = jsstr.Canonical(e.Message), jsstr.Canonical(e.Snippet)
		*err = e
	case perr.RangeError:
		*err = perr.New("Diagram is nested too deeply.", perr.Limit)
	default:
		*err = perr.Panic(r, debug.Stack())
	}
}

// orBackground returns ctx, or context.Background() for a nil ctx.
func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// Parsed is a parsed diagram, ready to render.
type Parsed struct {
	p parsed
}

// Parse parses source with the options read at parse time (Limit, MaxEdges, Config), as
// upstream's parse() does. Render options given to RenderParsed later do not change them.
// A nil ctx is context.Background().
func Parse(ctx context.Context, source string, opts Options) (res *Parsed, err error) {
	defer recoverInto(&err)
	ctx = orBackground(ctx)
	checkContext(ctx)
	return &Parsed{p: parseSource(ctx, jsstr.Normalize(source), opts)}, nil
}

// Render renders Mermaid source to SVG. The SVG keeps lone surrogates; see Result.
func Render(ctx context.Context, source string, opts Options) (Result, error) {
	parsed, err := Parse(ctx, source, opts)
	if err != nil {
		return Result{}, err
	}
	return RenderParsed(ctx, parsed, opts)
}

// RenderParsed renders a diagram Parse returned, as upstream's render(model) does. A nil ctx
// is context.Background().
func RenderParsed(ctx context.Context, parsed *Parsed, opts Options) (res Result, err error) {
	defer recoverInto(&err)
	ctx = orBackground(ctx)
	p := parsed.p

	section := p.diagram.Section
	if section == "" {
		section = p.Type
	}
	fixed := false
	if v, ok := p.config.Get(section); ok {
		if obj, isObj := v.(*jsobj.Object); isObj {
			if useMaxWidth, has := obj.Get("useMaxWidth"); has && useMaxWidth == false {
				fixed = true
			}
		}
	}
	responsive := !fixed
	if opts.Responsive != nil {
		responsive = *opts.Responsive
	}
	resolve := opts.Icons
	var icons func(string) string
	if resolve != nil {
		icons = func(name string) string { return resolve(text.DecodeEntities(name)) }
	}
	labelIcons := text.NewIcons(icons)
	if p.diagram.Math {
		labelIcons = labelIcons.WithMath(text.NewMath(opts.Math))
	}
	rc := &Context{Ctx: ctx, Options: opts, Responsive: responsive, Icons: labelIcons}
	rc.Options.Responsive = &responsive
	checkContext(ctx)
	var rendered Rendered
	tagErrors(p.Detected, func() { rendered = p.diagram.Render(p.model, p.config, rc) })

	outputLimit := optNumber(opts.OutputLimit, DefaultOutputLimit)
	if float64(jsstr.Len(rendered.SVG)) > outputLimit {
		e := perr.New("Diagram output is longer than the limit of "+jsnum.Format(outputLimit)+" characters.", perr.Limit)
		e.Type, e.MermaidType = p.Type, p.MermaidType
		return Result{}, e
	}
	// Output is joined from many strings; a surrogate pair split across two of them is
	// written as the one character JS would have.
	rendered.SVG = jsstr.Canonical(rendered.SVG)
	for i := range rendered.Links {
		rendered.Links[i].ID = jsstr.Canonical(text.DecodeEntities(rendered.Links[i].ID))
		rendered.Links[i].Href = jsstr.Canonical(rendered.Links[i].Href)
	}
	if rendered.Links == nil {
		rendered.Links = []Link{}
	}
	return Result{Rendered: rendered, Type: p.Type, MermaidType: p.MermaidType}, nil
}
