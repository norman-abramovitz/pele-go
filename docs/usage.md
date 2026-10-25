# Using pele-go

The Go API in detail. For a first render, see the [README](../README.md).

## Render

```go
import (
	pele "github.com/Broderick-Westrope/pele-go"
	_ "github.com/Broderick-Westrope/pele-go/all" // registers every diagram type
)

res, err := pele.Render(ctx, "flowchart LR\n  A[Mermaid text] --> B[SVG]", pele.Options{})
if err != nil {
	return err
}
fmt.Println(res.SVG, res.Width, res.Height)
```

`Result` also carries `Links` (elements that link somewhere), `Type` (`flowchart`) and
`MermaidType` (`flowchart-v2`). Theme the SVG with the same CSS variables as upstream
(`--pele-fg`, `--pele-font` and so on, see [Theming](https://pele.run/theming)).

To keep binaries small, import single types instead of `all` and register them yourself:

```go
import "github.com/Broderick-Westrope/pele-go/diagrams/flowchart"

func init() { pele.Register(flowchart.Diagram) }
```

`Register` must run before the first `Render`, normally from `init`; it panics afterwards.
`DetectType` and `Supports` tell you a source's type without rendering it, and don't count
as a render for `Register`.

## Options

`Options` mirrors upstream's `RenderOptions`. The zero value renders exactly as upstream
does with no options. Fields where JS tells "unset" apart from a zero value are pointers,
and `pele.Ptr` makes one inline:

```go
config, err := pele.ParseConfig(`{"flowchart": {"curve": "linear"}}`)
if err != nil {
	return err
}
res, err := pele.Render(ctx, src, pele.Options{
	FontSize:   pele.Ptr(14.0),
	Responsive: pele.Ptr(false),
	MaxWidth:   pele.Ptr(480.0),
	LinkRel:    pele.Ptr(""), // omit rel; nil means "noopener"
	Config:     config,
	Now:        time.Now(), // what gantt charts treat as today
})
```

`ParseConfig` reads Mermaid configuration from JSON and keeps JavaScript's key order, which
upstream's output can depend on. Front matter and directives in the diagram win over it.
The defaults for every field are in the `Options` doc comment.

Gantt charts read dates in `Options.Location` (default `time.Local`), standing in for the
time zone of the JS process. Set it and `Now` if you need output that doesn't depend on the
machine.

### Math

`Options.Math` typesets `$$...$$` in labels, as Mermaid does with KaTeX. Pele has no
typesetter of its own: you get the TeX between the `$$` and the font size in pixels, and
return the markup to draw with its size, or `nil` to leave the expression as written:

```go
res, err := pele.Render(ctx, "flowchart LR\n  A[\"$$x^2$$\"] --> B", pele.Options{
	Math: func(tex string, size float64) *pele.MathMarkup {
		svg, w, h, ok := typeset(tex, size) // your typesetter
		if !ok {
			return nil
		}
		return &pele.MathMarkup{Markup: svg, Width: w, Height: h}
	},
})
```

Set `HTML: true` for HTML markup (drawn in a `foreignObject`; the page needs the
typesetter's CSS). The markup is trusted and goes into the SVG as given. Only the types
Mermaid typesets math in ask (flowchart, swimlane, sequence, class, state, er, requirement,
block, mindmap, kanban and usecase), never for diagram titles, and once per expression and
size in a render. Markup whose width or height isn't positive and finite is dropped.

## Measurers

Layout depends on how wide text is, so output is only byte-identical to upstream's when both
use the same measurer. There are two built in:

| Measurer | Use it when |
|---|---|
| `pele.DefaultMeasurer()` (what a nil `Options.Measurer` uses) | You want upstream's output with no options. It's upstream's Arial advance table, ported exactly. |
| `fontmeasure.New(regular, bold, italic, boldItalic)` | You know which fonts will draw the SVG (for example you rasterise it) and want labels sized to them. |

```go
import "github.com/Broderick-Westrope/pele-go/fontmeasure"

m, err := fontmeasure.New(regular, bold, italic, boldItalic) // TTF/OTF bytes you supply
res, err := pele.Render(ctx, src, pele.Options{Measurer: m})
```

`fontmeasure` sums glyph advances and kerning (the GPOS `kern` feature, else the legacy
`kern` table; no ligatures or shaping) at 1024 ppem, measures a missing glyph as
`n`, and takes `fontmeasure.WithMono(...)` for code spans. It embeds no fonts, and it's the
only package that pulls in `golang.org/x/image`. You can also implement `pele.TextMeasurer`
yourself; it must be safe for concurrent use and must not panic (as must `Options.Icons` and `Options.Math`).

## Errors

Failures are `*pele.Error`. Read one with `errors.As`:

```go
var pe *pele.Error
if errors.As(err, &pe) {
	log.Printf("%s at %d:%d: %s\n%s", pe.Code, pe.Line, pe.Column, pe.Message, pe.Snippet)
}
```

| Code | Meaning |
|---|---|
| `CodeUnsupportedDiagram` | No type detected, or the type isn't registered |
| `CodeSyntax` | The source doesn't parse |
| `CodeSemantic` | It parses but asks for something that doesn't exist |
| `CodeLimit` | Past `Limit`, `OutputLimit` or `MaxEdges`, or where upstream's JS throws a `RangeError` ("Diagram is nested too deeply.") |
| `CodeCanceled` | The context was done (Go only, see below) |
| `CodeInternal` | A panic during the render: a bug, or your `TextMeasurer`, `Icons` or `Math` panicked (Go only). It wraps a `*pele.PanicError` with the value and stack |

Messages, lines and columns are upstream's. Columns count UTF-16 code units, as upstream's
do.

## Cancellation, concurrency and strings

- `Render` checks its context between phases and inside layout loops. A cancelled render
  returns `CodeCanceled` wrapping `ctx.Err()`, so `errors.Is(err, context.DeadlineExceeded)`
  works, and leaves no goroutine behind. Use a deadline for untrusted input.
- `Render` is safe for concurrent use. There's no global cache and no mutable package state
  beyond the registry, which is sealed by the first render.
- Every string in `Result` and `Error` is valid UTF-8. Where upstream would emit a lone
  surrogate (say, from a `\ud800` escape in YAML config), you get U+FFFD, as encoding the JS
  string with `TextEncoder` would.

