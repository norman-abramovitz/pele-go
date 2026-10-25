# Versioning and upstream releases

## Versioning

pele-go has its own semver tags, which change with changes here, separately from upstream's
version numbers. The upstream release it matches is in `pele.UpstreamVersion` and the
commit that release's tag points to is in `pele.UpstreamCommit`. Currently that's Pele 1.10.0,
commit `4eff76f98c73b6817800a05773517f35832e4e92`.

## Following upstream releases

Each Pele release becomes one change here: port the diff between tags file for file (the Go
files map one-to-one to upstream's `src/`), then bump the pinned oracle and regenerate the
fixtures:

- `internal/oracle`: the npm bundle, its checksum and `Version`; `UpstreamVersion` and
  `UpstreamCommit` in `pele.go` (a test fails if `UpstreamVersion` and `Version` differ);
  `tests/corpus` into `internal/oracle/testdata/corpus`.
- `node scripts/capture-cases.mjs <pele checkout>` re-captures upstream's suite.
- `node scripts/gen-shared-cases.mjs <pele checkout>` and the other `scripts/gen-*.mjs`
  regenerate the shared-module fixtures.

[The porting guide](porting.md) has the rules: the JS semantics that need helpers (number
formatting, `Math.round`, UTF-16 strings, object key order, regexes, fused multiply-add on
arm64) and how to debug a mismatch.

