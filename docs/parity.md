# Parity with upstream

pele-go's output is byte-identical to Pele 1.10.0's for the same source, options and text
measurer. This page covers how that's checked and where the port deliberately differs.

## How parity is checked

Byte-identical is checked by string comparison against upstream itself, not by eye:

- **goja:** `internal/oracle` runs the npm `pele@1.10.0` bundle (checksum pinned) in
  [goja](https://github.com/dop251/goja), with the same Go measurer on both sides.
- **V8:** the same bundle under Node, when `node` is on `PATH`. Where goja and V8 disagree
  (lone surrogates, numbers past 2^64), V8 wins.
- **Upstream's corpus:** every source in Pele's `tests/corpus`, with several option sets and
  measurers, and gantt charts in both UTC and `Australia/Sydney` (`TestCorpusParity`).
- **Upstream's test suite:** every public `parse`/`render` call Pele's own Vitest suite
  makes, captured with V8's result and replayed (`TestCapturedParity`).
- **Shared modules:** fixtures of upstream function calls recorded under V8 and replayed
  per package.
- **Fuzzing:** `FuzzParity` (goja) and `FuzzParityV8` (Node) per diagram type:

  ```sh
  PELE_FUZZ_TYPE=flowchart go test -run '^$' -fuzz '^FuzzParity$' -fuzztime 60s .
  ```

goja is test-only; it doesn't end up in your binary.

## Differences from upstream

- Browser-only APIs aren't ported: `mount`, zoom, `pele/lazy`, `renderAsync`, the canvas
  measurer and anything that reads `window` or the DOM.
- There's no public `parse` or typed model. Render is the API.
- Public strings are well-formed UTF-8, as described in
  [Using pele-go](usage.md#cancellation-concurrency-and-strings). Internally the port keeps
  upstream's UTF-16 semantics, including lone surrogates.
- Go additions: a `context.Context` on `Render` with `CodeCanceled`, `CodeInternal` for a
  recovered panic, `Options.Location` for gantt's time zone, and a per-render text-width
  cache in place of upstream's global one (`forgetTextWidths` is gone). None of these
  change output.
- There is no depth limit. Deeply nested sources render as they do in V8, which walks them
  without overflowing (checked to 100,000 levels); the port renders them too (tested at
  20,000). Where upstream's JS throws a `RangeError`, such as
  `String.fromCodePoint` given a fraction, the port returns the same `CodeLimit` error.

