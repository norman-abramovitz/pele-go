# Porting guide

How to port a piece of upstream Pele to Go in this repo. Read this before porting anything.
The design is `plans/design-2026-10-20-pele-go-port.md`.

## The contract

Output must be **byte-identical** to Pele 1.9.1 (tag `1.9.1`, commit 3052498; see
`pele.UpstreamVersion` and `pele.UpstreamCommit`) running under V8, for the same source,
options, `now` and measurer: the SVG, width, height, links, and on failure the error code,
message, type, line, column and snippet. "Looks the same" is a failure.

The scripts and parts of this guide need a checkout of upstream at that tag, with its
`node_modules` installed. Put it anywhere outside this repo; below it is `$PELE_UPSTREAM`:

```sh
git clone --branch 1.9.1 --depth 1 https://github.com/obsidianmd/pele.git "$PELE_UPSTREAM"
(cd "$PELE_UPSTREAM" && npm ci)
```

Upstream's TypeScript is in `$PELE_UPSTREAM/src` and its tests in `$PELE_UPSTREAM/tests`.
Every `scripts/*.mjs` that reads upstream takes the checkout as its first argument. The
Go tests don't need it: they use the vendored bundle and fixtures under `testdata`.

Port file for file: `src/diagrams/<type>/<file>.ts` becomes `diagrams/<type>/<file>.go`;
shared code goes under `internal/` (`internal/diagrams/common`, `internal/svg`,
`internal/text`, `internal/layout`, `internal/util`). Keep upstream's function names
(Go-cased) and order so release diffs map across. Mark intentional differences with a
short comment.

## How a diagram type plugs in

- A public package `diagrams/<type>` exports `var Diagram = &core.Diagram{Type, Section,
  KeepComments, Parse, Render}` (see `diagrams/info`). `Section` and `KeepComments` copy
  upstream's `index.ts`. The type string is upstream's `DiagramType` (`gitGraph`,
  `quadrantChart`, `treeView`, ...).
- Register it in its own file `all/<type>.go` (`func init() { pele.Register(<pkg>.Diagram) }`),
  one file per type so parallel ports never conflict.
- `Parse(source, config, title, lineOffset, limits)` returns the model; `Render(model, config,
  rc)` returns `core.Rendered`. Read options from `rc.Options` (pointer fields: nil means
  unset, use upstream's `??` default) and `rc.Responsive`; measure with `rc.Measurer()`;
  icons via `rc.Icons`. `limits.MaxEdges` is a float64.

## JavaScript semantics you must reproduce

Use the helpers; never approximate with Go's standard behaviour.

| JS | Go |
|---|---|
| `String(n)`, template `${n}` | `jsnum.Format` |
| `Number(s)`, `parseFloat`-like reads | `jsnum.Parse` (write a helper next to it for `parseFloat`/`parseInt` semantics if needed, with a test against goja/V8) |
| `Math.round` | `jsmath.Round` (Go's `math.Round` differs on .5 and -0) |
| `Math.sin`, `Math.cos`, `Math.hypot` | `jsmath.Sin`, `jsmath.Cos`, `jsmath.Hypot` (Go's differ in the last bit). Other fdlibm functions (`atan2`, `acos`, `asin`, `pow`, `log10`, `log2`, `exp`) need the same: port V8's `src/base/ieee754.cc` with clang's contraction as `math.FMA`, checked against Node as `internal/jsmath/v8_test.go` does |
| `num()` coordinate formatting | `svg.Num` |
| `s.length`, `s.slice`, `charCodeAt` | `jsstr.Len`, `jsstr.Slice`, or work on `[]uint16` (`jsstr.UTF16`) in lexers |
| `trim`/`trimStart`/`trimEnd`, `\s` | `jsstr.Trim*`, `jsstr.IsSpace` (JS whitespace, not Go's) |
| `.` in a regex | anything but `jsstr.IsLineEnd` |
| `Map` / `Set` | `omap.Map` / `omap.Set` (insertion order; Go maps are random) |
| plain object used as a map, `Object.keys/entries` | `jsobj.Object` (integer-like keys first, ascending) |
| `a < b` on strings, default `sort()` | `jsstr.Compare` (UTF-16 order) |
| `arr.sort(cmp)` | `slices.SortStableFunc` (JS sort is stable). Never `sort.Slice`. |
| `JSON.parse` | `jsjson.Parse` |
| throw `PeleError` | `perr.Throw(...)` / `perr.SyntaxError(...)`; a JS `RangeError` is `perr.ThrowRange()` |
| config values | `*jsobj.Object` / `jsobj.Truthy` / `jsobj.ToString` |

Strings are WTF-8 Go strings (they can hold lone surrogates). Lexers work on
`common.Src` (`[]uint16`) so offsets and error columns are UTF-16, as upstream's are.

**Regexes:** Go's `regexp` differs from JS (`\s`, `.`, `/i` case folding, no lookaround,
leftmost-first vs backtracking order). Use Go `regexp` only for ASCII-only patterns whose
meaning is the same in both. Otherwise hand-write a matcher and quote the JS regex in its
comment. Think about backtracking: greedy optional groups are tried first (see
`common.Yaml`), `\s*` before an alternative can back off, `$`/`^` with and without `m`.

**Numbers:** keep float64 everywhere upstream has a number, and keep the order of float
operations exactly (`a * b / c` is not `a * (b / c)`). Go may fuse a product into a sum, even
across statements and for `x/2`, into one FMA instruction on arm64, which rounds once where JS
rounds twice: write `float64(a*b) + c` (`TestNoFusedMultiplyAdd` finds the places). `Math.max/min` with NaN or -0 match
Go's `max`/`min` builtins; check other Math functions before relying on them. Integer
division and `| 0` / `>>` truncate to int32 in JS.

**Dates (gantt):** upstream uses JS `Date` in local time. Use `rc.Options.Now` and
`rc.Options.Location` (nil means `time.Local`), and reproduce JS `Date` semantics (month
overflow, `setDate`, DST) in a helper package `internal/jsdate` with tests against V8.

**Recursion:** upstream walks nested input (subgraphs, blocks, boundaries, groups, JSON)
without recursion that hostile input can overflow, and V8 renders deep nesting (checked to
100,000 levels). Don't add a depth counter: there is no fixed depth limit, and
`TestDeepNestingRenders` checks that 20,000-deep sources render. Go's stacks grow, so a
port that recurses where upstream does still renders them. Upstream reports every JS `RangeError` as "Diagram is nested
too deeply." (code `limit`); where a built-in would throw one (for example
`String.fromCodePoint` given a fraction, `internal/text/entities.go`), call
`perr.ThrowRange()` and the core reports the same.

**Concurrency:** no mutable package-level state except tables built with `sync.OnceValue`.
Per-render state goes on the model or `core.Context`.

## Testing (test first)

1. `go test ./...` runs everything. These gates pick up a registered type automatically:
   - `TestCorpusParity`: every source in `internal/oracle/testdata/corpus/<type>-docs.json`
     against goja (several option sets and measurers) and against Node/V8.
   - `TestCapturedParity`: every call upstream's own test suite made, replayed from
     `internal/oracle/testdata/captured/<type>.jsonl.gz` and checked against V8's hash.
   - `TestDetectTypeParity`.
   Before a type is registered these skip it; once registered they must all pass.
2. Fuzz for at least 60 s per type: `go test -run '^$' -fuzz '^FuzzParity$' -fuzztime 60s .`
   with `PELE_FUZZ_TYPE=<type>` (goja), and `FuzzParityV8` the same way
   (`scripts/fuzz-all.sh <target> <time> [type...]` loops over types). Keep failing
   inputs the fuzzer writes under `testdata/fuzz`.
   Then run `PELE_GEN=2000 PELE_GEN_TYPE=<type> go test -run '^TestGeneratedParity$' .`
   (and again with `PELE_GEN_BIG=1`), which builds big valid diagrams from the corpus and
   checks them against V8; mismatches land in `PELE_GEN_OUT`. `PELE_PROBE=<file>` with
   `-run '^TestProbe$'` checks one source.
3. Port upstream's model-level assertions from `tests/compat/<type>/` as package-internal
   Go tests where they explain behaviour a parity failure would not.
4. Debugging a mismatch: `go test -run 'TestCapturedParity/<type>' .` names the source and
   options. Render it with `oracle.NewNode()` or `oracle.New()` to see upstream's SVG and
   diff it against the port's. `internal/oracle` has both.
5. Shared modules (`internal/util`, `internal/svg`, `internal/diagrams/common`,
   `internal/jsmath`) have no oracle of their own: their tests replay calls of upstream's
   functions under Node, written by `node scripts/gen-shared-cases.mjs "$PELE_UPSTREAM"`
   into each package's `testdata/cases.json` (`internal/fixture` runs them). Add cases there
   when you change or use one of them in a new way.
6. goja is not a faithful JS engine for lone surrogates, numbers past 2^64 and `JSON.parse`
   of out-of-range numbers. When goja and V8 disagree, V8 wins.

`go vet ./...`, `gofmt` and `go test -race ./...` must be clean. Run
`go run honnef.co/go/tools/cmd/staticcheck@latest ./...` too.

The V8 checks skip when `node` is not on `PATH`; `PELE_REQUIRE_NODE=1` makes them fail
instead. CI (`.github/workflows/ci.yml`) sets it and runs vet, gofmt, staticcheck and
`go test -race -timeout 30m ./...` with Node 22, fuzzes every type for 10 s per engine on
each PR, and for 60 s per engine nightly. `TestCorpusParity` and `TestCapturedParity` run
their types in parallel, each with its own goja runtime and node process, so keep per-type
state inside the subtest; zoned types (which set `time.Local` for goja) run alone first.

## Style

Go code follows the repo's existing files. Comments only for non-obvious decisions,
differences from upstream and preserved quirks. Errors use `perr`. Commit with a short
imperative subject per logical step, staging files by explicit path.
