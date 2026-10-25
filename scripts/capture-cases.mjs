// Runs upstream's Vitest suite with scripts/capture/capture-setup.ts and writes every
// public parse() and render() call it made to internal/oracle/testdata/captured/<type>.jsonl.gz,
// one file per detected diagram type, with a manifest.
//
// The capture is checked before anything is written: every test file must have run, the
// tests that fail with capture on must be the ones the manifest already allows, no fewer
// tests may pass than last time, and no more calls may be unreplayable. A first capture
// records these as the baseline.
//
// Usage: node scripts/capture-cases.mjs <pele checkout at pele.UpstreamVersion, with npm ci run>
import { execFileSync } from 'node:child_process';
import { copyFileSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';
import { gzipSync } from 'node:zlib';

const [, , peleDir] = process.argv;
if (!peleDir) throw new Error('usage: capture-cases.mjs <pele checkout>');

const here = fileURLToPath(new URL('.', import.meta.url));
const testdata = join(here, '../internal/oracle/testdata');
const out = join(testdata, 'captured');
const setup = join(peleDir, 'tests/support/__capture-setup.ts');
const config = join(peleDir, 'vitest.capture.config.ts');
const dir = mkdtempSync(join(tmpdir(), 'pele-capture-'));
const report = join(dir, 'vitest.json');

function fail(message) {
  console.error(`capture rejected: ${message}`);
  process.exit(1);
}

// entries.test.ts resets modules to test registration, so its calls' outcomes depend on
// which types were registered at the time and can never be replayed.
copyFileSync(join(here, 'capture/capture-setup.ts'), setup);
writeFileSync(
  config,
  `import base from './vitest.config.ts';
import { defineConfig, mergeConfig } from 'vitest/config';
export default mergeConfig(base, defineConfig({
  test: { setupFiles: ['./tests/support/__capture-setup.ts'], exclude: ['tests/website/**', 'tests/entries.test.ts', 'node_modules/**'] },
}));
`
);
try {
  execFileSync('npx', ['vitest', 'run', '--config', config, '--reporter=dot', '--reporter=json', `--outputFile.json=${report}`], {
    cwd: peleDir,
    env: { ...process.env, PELE_CAPTURE_DIR: dir },
    stdio: 'inherit',
  });
} catch {
  // Judged from the report below.
} finally {
  rmSync(setup);
  rmSync(config);
}

let baseline;
try {
  baseline = JSON.parse(readFileSync(join(out, 'manifest.json'), 'utf8'));
} catch {
  baseline = undefined;
}

const results = JSON.parse(readFileSync(report, 'utf8'));
const unloaded = results.testResults.filter((f) => f.status === 'failed' && f.assertionResults.length === 0);
if (unloaded.length > 0) fail(`test files did not run: ${unloaded.map((f) => f.name).join(', ')}`);
const failing = results.testResults
  .flatMap((f) => f.assertionResults.filter((a) => a.status === 'failed').map((a) => `${f.name.slice(peleDir.length)} > ${a.fullName}`))
  .sort();
const suite = { files: results.numTotalTestSuites, passed: results.numPassedTests, failing };
if (baseline?.suite) {
  const allowed = new Set(baseline.suite.failing);
  const unexpected = failing.filter((name) => !allowed.has(name));
  if (unexpected.length > 0) fail(`tests failed that the baseline allows to pass:\n  ${unexpected.join('\n  ')}`);
  if (suite.files < baseline.suite.files) fail(`${suite.files} test files ran, the baseline had ${baseline.suite.files}`);
  if (suite.passed < baseline.suite.passed) fail(`${suite.passed} tests passed, the baseline had ${baseline.suite.passed}`);
}

// Shard by detected diagram type, so each type's cases can be committed when it is ported.
vm.runInThisContext(readFileSync(join(testdata, 'pele.global.js'), 'utf8'));
const typeOf = (src) => {
  try {
    return globalThis.pele.detectType(src) ?? 'none';
  } catch {
    return 'none';
  }
};

const seen = new Set();
const shards = {};
const manifest = { suite, calls: 0, unique: 0, replayable: 0, unsupported: {}, byType: {} };
for (const file of readdirSync(dir).filter((f) => f.endsWith('.jsonl')).sort()) {
  for (const line of readFileSync(join(dir, file), 'utf8').split('\n')) {
    if (!line) continue;
    manifest.calls++;
    if (seen.has(line)) continue;
    seen.add(line);
    const record = JSON.parse(line);
    const reason = record.unsupported ?? record.parse?.unsupported ?? (record.outcome.startsWith('thrown:') ? record.outcome : undefined);
    if (reason) {
      manifest.unsupported[reason] = (manifest.unsupported[reason] ?? 0) + 1;
      continue;
    }
    (shards[typeOf(record.src ?? record.parse.src)] ??= []).push(line);
  }
}
manifest.unique = seen.size;

// Calls that cannot be replayed are a gap in the parity gate. More of them than last time
// fails the capture instead of quietly narrowing the gate.
const total = (m) => Object.values(m.unsupported).reduce((a, b) => a + b, 0);
if (baseline && total(manifest) > total(baseline)) fail(`unreplayable calls rose from ${total(baseline)} to ${total(manifest)}`);
if (baseline && manifest.calls < baseline.calls) fail(`${manifest.calls} calls captured, the baseline had ${baseline.calls}`);

// Publish only now that the capture has been checked.
const staged = join(dir, 'staged');
mkdirSync(staged);
for (const [type, lines] of Object.entries(shards).sort()) {
  lines.sort();
  manifest.replayable += lines.length;
  manifest.byType[type] = lines.length;
  writeFileSync(join(staged, `${type}.jsonl.gz`), gzipSync(lines.join('\n') + '\n', { level: 9 }));
}
writeFileSync(join(staged, 'manifest.json'), JSON.stringify(manifest, null, 1) + '\n');
mkdirSync(out, { recursive: true });
for (const file of readdirSync(out).filter((f) => f.endsWith('.jsonl.gz'))) rmSync(join(out, file));
for (const file of readdirSync(staged)) renameSync(join(staged, file), join(out, file));
rmSync(dir, { recursive: true });
console.log({ ...manifest, suite: { ...suite, failing: failing.length } });
