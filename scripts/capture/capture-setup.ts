// A Vitest setup file that records every call to upstream's public parse() and render()
// while upstream's own tests run, so the Go port can replay them. scripts/capture-cases.mjs
// copies it into a Pele checkout and runs the suite with it.
//
// Each call becomes one JSON line in $PELE_CAPTURE_DIR: the source, the options, and a
// SHA-256 of what V8 returned or threw, in a form tests/capture_test.go recomputes.
import { createHash } from 'node:crypto';
import { appendFileSync } from 'node:fs';
import { threadId } from 'node:worker_threads';
import { vi } from 'vitest';

// The instant a render with no `now` is given, so gantt output does not depend on when the
// suite ran.
const FIXED_NOW = Date.UTC(2026, 9, 20, 9, 30);

// A render given an invalid `now` reads the clock instead. Stopping it at FIXED_NOW during
// the call keeps those outcomes the same however and whenever the suite runs.
function atFixedNow(run) {
  const clock = Date.now;
  Date.now = () => FIXED_NOW;
  try {
    return run();
  } finally {
    Date.now = clock;
  }
}

const out = `${process.env.PELE_CAPTURE_DIR}/capture-${process.pid}-${threadId}.jsonl`;

const units = (s) => Array.from({ length: s.length }, (_, i) => s.charCodeAt(i));

// The outcome as the Go side writes it before hashing. Strings are code-unit arrays so lone
// surrogates count, and type is null when no type was detected.
function outcomeText(result, error) {
  if (error) {
    return JSON.stringify({
      error: {
        code: error.code,
        message: units(error.message),
        type: error.type ?? null,
        mermaidType: error.mermaidType ?? null,
        line: error.line,
        column: error.column,
        snippet: units(error.snippet),
      },
    });
  }
  return JSON.stringify({
    svg: units(result.svg),
    width: result.width,
    height: result.height,
    links: result.links.map((l) => ({ id: units(l.id), href: units(l.href), internal: l.internal })),
    type: result.type,
    mermaidType: result.mermaidType,
  });
}

const hash = (text) => createHash('sha256').update(text).digest('hex');

class Unserializable extends Error {}

// Options as JSON. Non-finite numbers, which JSON cannot hold, become {"$number": "..."}.
function encode(value) {
  if (value === null || typeof value === 'boolean' || typeof value === 'string') return value;
  if (typeof value === 'number') return Number.isFinite(value) ? value : { $number: String(value) };
  if (Array.isArray(value)) return value.map(encode);
  if (typeof value === 'object' && Object.getPrototypeOf(value) === Object.prototype) {
    const o = {};
    // An undefined property reads as unset everywhere upstream looks (with ??), and JSON
    // drops it the same way.
    for (const k of Object.keys(value)) if (value[k] !== undefined) o[k] = encode(value[k]);
    return o;
  }
  throw new Unserializable(typeof value);
}

function mathRecord(table) {
  if (table === undefined) return {};
  try {
    return { math: encode(Object.assign({}, table)) };
  } catch (e) {
    if (e instanceof Unserializable) return { unsupported: `math answer of type ${e.message}` };
    throw e;
  }
}

function write(record) {
  appendFileSync(out, JSON.stringify(record) + '\n');
}

vi.mock('../../src/core.js', async (importOriginal) => {
  const mod = await importOriginal();
  const { metricsMeasurer } = await import('../../src/text/measurer.js');
  const fromParse = new WeakMap();

  // Splits options into what JSON can carry and how the measurer was chosen. Returns null
  // when something cannot be replayed.
  function describe(options) {
    const { measurer, icons, math, now, ...rest } = options ?? {};
    let kind = 'default';
    if (measurer === metricsMeasurer) kind = 'metrics';
    else if (measurer !== undefined) return { unsupported: 'custom measurer' };
    try {
      const encoded = encode(rest);
      if (now !== undefined) encoded.now = now instanceof Date ? now.getTime() : now;
      return { options: encoded, measurer: kind, icons: icons !== undefined, ...(math !== undefined && { mathResolver: true }) };
    } catch (e) {
      if (e instanceof Unserializable) return { unsupported: `option of type ${e.message}` };
      throw e;
    }
  }

  function parse(text, options) {
    const record = { call: 'parse', src: text, ...describe(options) };
    try {
      const model = atFixedNow(() => mod.parse(text, options));
      fromParse.set(model, { src: text, ...describe(options) });
      write({ ...record, outcome: 'ok' });
      return model;
    } catch (error) {
      write({ ...record, outcome: hash(outcomeText(null, error)) });
      throw error;
    }
  }

  function render(source, options) {
    const opts = { ...options };
    if (opts.now === undefined) opts.now = FIXED_NOW;
    // Icon names the render asks for, with the host's answers, for the replay to give back.
    // A null prototype keeps an icon named __proto__ as a key instead of a prototype.
    const iconTable = Object.create(null);
    if (typeof opts.icons === 'function') {
      const host = opts.icons;
      opts.icons = (name) => {
        const answer = host(name);
        iconTable[name] = answer ?? null;
        return answer;
      };
    }
    // TeX the render asks the math resolver for, keyed as upstream caches it (size, newline,
    // TeX), with the host's answers. Upstream asks once per key per render, so the table
    // answers every question the replay asks.
    let mathTable;
    if (typeof opts.math === 'function') {
      const host = opts.math;
      mathTable = Object.create(null);
      opts.math = (tex, size) => {
        const answer = host(tex, size);
        mathTable[size + '\n' + tex] = answer ?? null;
        return answer;
      };
    }
    let record;
    if (typeof source === 'string') record = { call: 'render', src: source, ...describe(opts) };
    else if (fromParse.has(source)) record = { call: 'render-model', parse: fromParse.get(source), ...describe(opts) };
    else record = { call: 'render-foreign', unsupported: 'model not returned by parse' };
    try {
      const result = atFixedNow(() => mod.render(source, opts));
      write({ ...record, icons: iconTable, ...mathRecord(mathTable), outcome: hash(outcomeText(result, null)) });
      return result;
    } catch (error) {
      const outcome = error && error.name === 'PeleError' ? hash(outcomeText(null, error)) : `thrown: ${error && error.name}`;
      write({ ...record, icons: iconTable, ...mathRecord(mathTable), outcome });
      throw error;
    }
  }

  return { ...mod, parse, render };
});
