// Writes internal/yaml/testdata/cases.json: each input with what upstream's parseYaml returns
// for it under V8, or the message it throws.
//
// Usage: node --experimental-transform-types scripts/gen-yaml-cases.mjs <pele checkout at pele.UpstreamVersion>
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const [, , peleDir] = process.argv;
if (!peleDir) throw new Error('usage: gen-yaml-cases.mjs <pele checkout>');
const { parseYaml } = await import(pathToFileURL(join(peleDir, 'src/util/yaml.ts')).href);

// Code units, so lone surrogates survive JSON.
const units = (s) => Array.from({ length: s.length }, (_, i) => s.charCodeAt(i));

function describe(v) {
  if (v === null) return 'null';
  if (Array.isArray(v)) return '[' + v.map(describe).join(',') + ']';
  if (typeof v === 'object') return '{' + Object.keys(v).map((k) => JSON.stringify(units(k)) + ':' + describe(v[k])).join(',') + '}';
  if (typeof v === 'string') return 's' + JSON.stringify(units(v));
  return typeof v + ':' + String(v);
}

const inputs = JSON.parse(readFileSync(new URL('../internal/yaml/testdata/inputs.json', import.meta.url), 'utf8'));
const cases = inputs.map((input) => {
  try {
    return { input, value: describe(parseYaml(input)) };
  } catch (error) {
    return { input, error: `${error.name}: ${error.message}` };
  }
});
writeFileSync(new URL('../internal/yaml/testdata/cases.json', import.meta.url), JSON.stringify(cases, null, 1) + '\n');
