# pele-go

Mermaid diagrams to SVG in pure Go. It's a port of [Pele](https://github.com/obsidianmd/pele)
that produces the same output as Pele 1.8.0 byte for byte, with all 33 diagram types, no
cgo and no JavaScript runtime.

![A flowchart from Mermaid source through pele-go and resvg to PNG](raster/cmd/pele/examples/pipeline.png)

![A dark sequence diagram of the CLI calling pele-go and resvg](raster/cmd/pele/examples/sequence.png)

## Install

```sh
go get github.com/Broderick-Westrope/pele-go
```

Go 1.26 or later. The root package uses only the standard library, so it builds with
`CGO_ENABLED=0`.

## Render a diagram

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

## Try it from a shell

The images above came from `pele`, a small CLI in the [`raster`](raster/README.md) module,
which draws pele-go's SVG to PNG with resvg in WebAssembly:

```sh
cd raster
go run ./cmd/pele -theme dark -o sequence.png cmd/pele/examples/sequence.mmd
```

Its [README](raster/cmd/pele/README.md) has the flags and more examples.

## Diagram types

Each type is its own package under [`diagrams`](diagrams), so you can import only the ones
you need (see [Using pele-go](docs/usage.md#render)):

agentflow, architecture, block, c4, class, cynefin, er, eventmodeling, flowchart, gantt, git,
info, ishikawa, journey, kanban, mindmap, packet, pie, quadrant, radar, railroad, requirement,
sankey, sequence, state, swimlane, timeline, treemap, treeview, usecase, venn, wardley and
xychart.

## Docs

| | |
|---|---|
| [Using pele-go](docs/usage.md) | Options, text measurers, errors, cancellation and concurrency |
| [Parity with upstream](docs/parity.md) | How byte-for-byte output is checked, and where the port differs |
| [Versioning and upstream releases](docs/releasing.md) | Tags, and how a new Pele release is ported |
| [Porting guide](docs/porting.md) | The rules for porting upstream code and debugging a mismatch |
| [raster](raster/README.md) | Mermaid to PNG in pure Go |

## Licence

MIT, see [LICENSE](LICENSE). Pele is MIT licensed by kepano, and its copyright notice is
kept there. The oracle's copy of Pele's bundle is under
[`internal/oracle/testdata/PELE_LICENSE`](internal/oracle/testdata/PELE_LICENSE), and the
DejaVu fonts used to test `fontmeasure` under
[`fontmeasure/testdata/LICENSE`](fontmeasure/testdata/LICENSE). Neither is compiled into
the library.
