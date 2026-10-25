// Writes internal/layout/testdata/cases.jsonl.gz: inputs to upstream's layered, compoundLayout
// and laneLayout with what each returns under V8, for the Go port to replay. Cases come from
// seeded random and structured graphs, and from the layouts upstream's own diagrams ask for when
// rendering the corpus and the captured test sources of every type that uses layout.
//
// Usage: node --experimental-transform-types scripts/gen-layout-cases.mjs <pele checkout at pele.UpstreamVersion>
// Set PELE_LAYOUT_FULL=1 to keep whole outputs rather than hashes when debugging a mismatch.
import { createHash } from 'node:crypto';
import { readdirSync, readFileSync, realpathSync, writeFileSync } from 'node:fs';
import { register } from 'node:module';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { gunzipSync, gzipSync } from 'node:zlib';

const [, , peleArg] = process.argv;
if (!peleArg) throw new Error('usage: gen-layout-cases.mjs <pele checkout>');
// The resolved URLs that the hooks match are real paths.
const peleDir = realpathSync(peleArg);
const src = (path) => pathToFileURL(join(peleDir, 'src', path)).href;

// Diagrams import compoundLayout and laneLayout from their modules, whose exports cannot be
// replaced, so a resolve hook points those imports at wrappers that record each call.
const wrapper = (file, name) =>
  'data:text/javascript,' +
  encodeURIComponent(`import * as real from ${JSON.stringify(src(file) + '?real')};
export * from ${JSON.stringify(src(file) + '?real')};
export function ${name}(...args) {
  const input = structuredClone(args);
  const out = real.${name}(...args);
  globalThis.__layoutCall?.(${JSON.stringify(name)}, input, args, out);
  return out;
}`);
const hooks = `
const redirect = ${JSON.stringify({ [src('layout/compound.ts')]: wrapper('layout/compound.ts', 'compoundLayout'), [src('layout/lanes.ts')]: wrapper('layout/lanes.ts', 'laneLayout') })};
export async function resolve(specifier, context, next) {
  let result;
  try {
    result = await next(specifier, context);
  } catch (error) {
    // Upstream imports its TypeScript files by their .js names.
    if (!specifier.endsWith('.js')) throw error;
    result = await next(specifier.slice(0, -3) + '.ts', context);
  }
  const to = redirect[result.url];
  return to ? { ...result, url: to } : result;
}
// The info diagram imports a field of package.json by name, as its bundler allows.
export async function load(url, context, next) {
  if (!url.endsWith('/package.json')) return next(url, context);
  const { readFileSync } = await import('node:fs');
  const { fileURLToPath } = await import('node:url');
  const { version } = JSON.parse(readFileSync(fileURLToPath(url), 'utf8'));
  return { format: 'module', source: 'export const version = ' + JSON.stringify(version) + ';', shortCircuit: true };
}`;
register('data:text/javascript,' + encodeURIComponent(hooks));

const { layered, lnode, ledge } = await import(src('layout/layered.ts') + '?real');
const { compoundLayout, cnode } = await import(src('layout/compound.ts') + '?real');
const { laneLayout } = await import(src('layout/lanes.ts') + '?real');
const { render } = await import(src('index.ts'));

// Numbers JSON cannot hold, and -0, travel as strings.
const replacer = (_k, v) => (typeof v === 'number' && (!Number.isFinite(v) || Object.is(v, -0)) ? (Object.is(v, -0) ? '-0' : String(v)) : v);
const num = (v) => (Object.is(v, -0) ? '-0' : String(v));
const list = (a) => [a.length, ...a.map(num)].join(' ');

function describeLayered(nodes, edges, result) {
  const out = [num(result.width), num(result.height)];
  for (const n of nodes) out.push(num(n.x), num(n.y), num(n.rank));
  for (const e of edges) out.push(e.reversed ? 'r' : 'f', num(e.tailDx), num(e.headDx), num(e.labelX), num(e.labelY), list(e.points));
  return out.join(' ');
}
function describeCompound(nodes, edges, result) {
  const out = [num(result.width), num(result.height)];
  for (const n of nodes) out.push(num(n.x), num(n.y), num(n.w), num(n.h), num(n.parent), num(n.seq), num(n.padTop));
  for (const e of edges) out.push(num(e.src), num(e.dst), num(e.labelX), num(e.labelY), list(e.route));
  return out.join(' ');
}

// Inputs are written positionally, with only what the layout reads, and each case runs on what
// its input decodes to, so that the fixture holds everything that decided the output.
const DIR_CODES = [undefined, 'TB', 'BT', 'LR', 'RL'];
const flag = (b) => (b ? 1 : 0);
const encodeOpt = (o) => [o.nodeSep, o.edgeSep, o.rankSep, o.portSep, flag(o.tracks), flag(o.straight), flag(o.apart), o.headRoom ?? 0];
const decodeOpt = ([nodeSep, edgeSep, rankSep, portSep, tracks, straight, apart, headRoom]) => {
  const opt = { nodeSep, edgeSep, rankSep, portSep };
  if (tracks) opt.tracks = true;
  if (straight) opt.straight = true;
  if (apart) opt.apart = true;
  if (headRoom !== 0) opt.headRoom = headRoom;
  return opt;
};
const encodeCNodes = (nodes) => nodes.map((c) => [c.w, c.h, c.parent, flag(c.isGroup), DIR_CODES.indexOf(c.dir), c.padX, c.padTop, c.padBottom, c.minW, c.seq, c.span, c.x, c.y]);
const decodeCNodes = (rows) =>
  rows.map(([w, h, parent, isGroup, dir, padX, padTop, padBottom, minW, seq, span, x, y]) => ({ w, h, parent, isGroup: isGroup === 1, dir: DIR_CODES[dir], padX, padTop, padBottom, minW, seq, span, x, y }));
const encodeCEdges = (edges) => edges.map((e) => [e.src, e.dst, e.minlen, e.labelW, e.labelH, e.labelX, e.labelY]);
const decodeCEdges = (rows) => rows.map(([src, dst, minlen, labelW, labelH, labelX, labelY]) => ({ src, dst, minlen, labelW, labelH, route: [], labelX, labelY }));
const encodeLNodes = (nodes) => nodes.map((n) => [n.w, n.h, n.kind, n.pin, n.span]);
const decodeLNodes = (rows) => rows.map(([w, h, kind, pin, span]) => ({ ...lnode(w, h, kind), pin, span }));
const encodeLEdges = (edges) => edges.map((e) => [e.tail, e.head, e.minlen, e.labelW, e.labelH, e.tailDx, e.headDx, flag(e.tailFixed), flag(e.headFixed)]);
const decodeLEdges = (rows) =>
  rows.map(([tail, head, minlen, labelW, labelH, tailDx, headDx, tailFixed, headFixed]) => ({ ...ledge(tail, head, minlen, labelW, labelH), tailDx, headDx, tailFixed: tailFixed === 1, headFixed: headFixed === 1 }));
const revive = (_k, v) => (v === '-0' ? -0 : v === 'Infinity' ? Infinity : v === '-Infinity' ? -Infinity : v === 'NaN' ? NaN : v);

const cases = [];
const seen = new Set();
// Outputs are kept as hashes, which keeps the fixture small. PELE_LAYOUT_FULL=1 keeps them
// whole instead, for debugging a mismatch: the Go test then shows where the two part.
const FULL = process.env.PELE_LAYOUT_FULL === '1';
function add(kind, encoded, run) {
  const input = JSON.stringify(encoded, replacer);
  if (seen.has(kind + input)) return;
  seen.add(kind + input);
  let output;
  try {
    output = run(JSON.parse(input, revive));
  } catch (error) {
    output = `throws ${error.name}: ${error.message}`;
  }
  const record = { kind, input: JSON.parse(input) };
  if (FULL) record.output = output;
  else record.sha256 = createHash('sha256').update(output).digest('hex').slice(0, 32);
  cases.push(record);
}
const addLayered = (nodes, edges, opt) =>
  add('layered', { nodes: encodeLNodes(nodes), edges: encodeLEdges(edges), opt: encodeOpt(opt) }, (a) => {
    const [ns, es] = [decodeLNodes(a.nodes), decodeLEdges(a.edges)];
    return describeLayered(ns, es, layered(ns, es, decodeOpt(a.opt)));
  });
const addCompound = (nodes, edges, dir, opt) =>
  add('compound', { nodes: encodeCNodes(nodes), edges: encodeCEdges(edges), dir, opt: encodeOpt(opt) }, (a) => {
    const [ns, es] = [decodeCNodes(a.nodes), decodeCEdges(a.edges)];
    return describeCompound(ns, es, compoundLayout(ns, es, a.dir, decodeOpt(a.opt)));
  });
const addLanes = (nodes, edges, dir, opt, lanePad) =>
  add('lanes', { nodes: encodeCNodes(nodes), edges: encodeCEdges(edges), dir, opt: encodeOpt(opt), lanePad: lanePad ?? 24 }, (a) => {
    const [ns, es] = [decodeCNodes(a.nodes), decodeCEdges(a.edges)];
    return describeCompound(ns, es, laneLayout(ns, es, a.dir, decodeOpt(a.opt), a.lanePad));
  });

// The diagrams' own layouts, recorded as they render.
const layoutTypes = ['flowchart', 'graph', 'state', 'class', 'er', 'requirement', 'usecase', 'agentflow', 'swimlane'];
globalThis.__layoutCall = (name, input) => {
  if (name === 'compoundLayout') addCompound(input[0], input[1], input[2], input[3]);
  else addLanes(input[0], input[1], input[2], input[3], input[4]);
};
const sources = [];
const corpusDir = join(peleDir, 'tests/corpus');
for (const file of readdirSync(corpusDir).sort()) {
  if (!layoutTypes.some((t) => file.startsWith(t + '-'))) continue;
  for (const source of JSON.parse(readFileSync(join(corpusDir, file), 'utf8'))) sources.push([source, {}]);
}
const capturedDir = new URL('../internal/oracle/testdata/captured/', import.meta.url);
for (const type of layoutTypes) {
  let text;
  try {
    text = gunzipSync(readFileSync(new URL(`${type}.jsonl.gz`, capturedDir))).toString('utf8');
  } catch {
    continue;
  }
  for (const line of text.split('\n')) {
    if (!line) continue;
    const call = JSON.parse(line);
    if (typeof call.src !== 'string') continue;
    const options = {};
    for (const key of ['fontSize', 'fontFamily', 'padding', 'autoDirection', 'maxWidth', 'directionBreakpoint', 'config']) {
      if (call.options?.[key] !== undefined) options[key] = call.options[key];
    }
    sources.push([call.src, options]);
  }
}
const before = cases.length;
const progress = (label) => process.stderr.write(`${label}: ${cases.length} cases, ${Math.round(performance.now() / 1000)}s\n`);
progress(`rendering ${sources.length} sources`);
for (const [i, [source, options]] of sources.entries()) {
  if (i % 1000 === 0) progress(`source ${i}`);
  try {
    render(source, { now: 0, ...options });
  } catch {
    // Failures to parse or render are not layout's concern.
  }
}
globalThis.__layoutCall = undefined;
const fromDiagrams = cases.length - before;

// Seeded random numbers, as upstream's tests make them.
function random(seed) {
  let state = seed >>> 0;
  return () => {
    state = (Math.imul(state, 1664525) + 1013904223) >>> 0;
    return state / 4294967296;
  };
}

const DIRS = ['TB', 'BT', 'LR', 'RL'];
const OPTIONS = { nodeSep: 40, edgeSep: 16, rankSep: 48, portSep: 20 };

function options(rnd) {
  const int = (n) => Math.floor(rnd() * n);
  const opt = rnd() < 0.5 ? { ...OPTIONS } : { nodeSep: 10 + int(60), edgeSep: 4 + int(30), rankSep: 20 + int(80), portSep: 8 + int(30) };
  if (rnd() < 0.25) opt.tracks = true;
  if (rnd() < 0.2) opt.straight = true;
  if (rnd() < 0.2) opt.apart = true;
  if (rnd() < 0.4) opt.headRoom = [0, 8, 10, 13.5][int(4)];
  if (rnd() < 0.1) opt.nodeSep += 0.25;
  return opt;
}

// A size: mostly whole, sometimes as fractional as measured text is.
function size(rnd, base, spread) {
  const v = base + Math.floor(rnd() * spread);
  const r = rnd();
  if (r < 0.15) return v + Math.floor(rnd() * 1000) / 1000;
  if (r < 0.25) return v + rnd();
  return v;
}

function compoundGraph(seed) {
  const rnd = random(seed);
  const int = (n) => Math.floor(rnd() * n);
  const style = seed % 4;
  const nodes = [];
  const groups = [];
  const groupCount = int(style === 3 ? 9 : 5);
  for (let g = 0; g < groupCount; g++) {
    const node = cnode(0, 0, groups.length > 0 && rnd() < 0.5 ? groups[int(groups.length)] : -1);
    node.isGroup = true;
    node.padX = [20, 8, 0, 12.5][int(4)];
    node.padTop = [40, 24, 0, 31.25][int(4)];
    node.padBottom = [20, 8, 0][int(3)];
    node.minW = [60, 0, 200][int(3)];
    node.dir = rnd() < 0.3 ? DIRS[int(4)] : undefined;
    node.seq = rnd() < 0.8 ? 1000 + g : int(40);
    if (style === 0) node.span = Infinity;
    else if (style === 2 && rnd() < 0.5) node.span = int(200);
    groups.push(nodes.length);
    nodes.push(node);
  }
  const leaves = 1 + int(style === 3 ? 50 : 30);
  for (let i = 0; i < leaves; i++) {
    const parent = groups.length > 0 && rnd() < 0.5 ? groups[int(groups.length)] : -1;
    const node = cnode(size(rnd, 20, 140), size(rnd, 20, 70), parent);
    node.seq = rnd() < 0.9 ? i : int(leaves);
    if (style === 0) node.span = Math.max(0, Math.min(node.w, node.h) - 24);
    else if (style === 2) node.span = rnd() < 0.5 ? node.w - 24 : rnd() < 0.5 ? Infinity : 0;
    nodes.push(node);
  }
  // Now and then a group that holds itself, groups that hold each other, or a parent that is no group.
  if (rnd() < 0.1 && groups.length > 0) nodes[groups[0]].parent = groups[0];
  if (rnd() < 0.1 && groups.length > 1) {
    nodes[groups[0]].parent = groups[1];
    nodes[groups[1]].parent = groups[0];
  }
  if (rnd() < 0.05) nodes[nodes.length - 1].parent = nodes.length + 3;
  if (rnd() < 0.05 && nodes.length > 2) nodes[nodes.length - 1].parent = nodes.length - 2;
  const edges = [];
  const count = int(leaves * (style === 3 ? 3 : 2)) + (rnd() < 0.1 ? 0 : 1);
  for (let i = 0; i < count; i++) {
    const labelled = rnd() < 0.3;
    const src = rnd() < 0.8 ? groups.length + int(leaves) : int(nodes.length);
    let dst = rnd() < 0.8 ? groups.length + int(leaves) : int(nodes.length);
    if (rnd() < 0.05) dst = src;
    const e = { src, dst, minlen: rnd() < 0.85 ? 1 : 1 + int(3), labelW: labelled ? size(rnd, 10, 80) : 0, labelH: labelled ? [24, 18, 20.5][int(3)] : 0, route: [], labelX: 0, labelY: 0 };
    edges.push(e);
    if (rnd() < 0.08) edges.push({ ...e, route: [] });
  }
  return { nodes, edges, dir: DIRS[int(4)], opt: options(rnd) };
}

function laneGraph(seed) {
  const rnd = random(seed);
  const int = (n) => Math.floor(rnd() * n);
  const group = (parent = -1) => {
    const node = cnode(0, 0, parent);
    node.isGroup = true;
    node.padX = [20, 8, 30][int(3)];
    node.padTop = [36, 20, 50][int(3)];
    node.padBottom = [20, 8][int(2)];
    node.minW = [60, 0, 300][int(3)];
    return node;
  };
  const nodes = [];
  const leaves = [];
  const lanes = [];
  const inner = [];
  const leafCount = 1 + int(30);
  const laneCount = int(6);
  for (let i = 0; i < leafCount; i++) {
    nodes.push(cnode(size(rnd, 30, 120), size(rnd, 24, 60)));
    leaves.push(i);
  }
  for (let k = 0; k < laneCount; k++) {
    lanes.push(nodes.length);
    nodes.push(group());
  }
  if (lanes.length > 0) {
    for (let k = 0, c = int(5); k < c; k++) {
      const parent = inner.length > 0 && rnd() < 0.4 ? inner[int(inner.length)] : lanes[int(laneCount)];
      inner.push(nodes.length);
      nodes.push(group(parent));
    }
  }
  for (const i of leaves) {
    if (inner.length > 0 && rnd() < 0.25) nodes[i].parent = inner[int(inner.length)];
    else if (lanes.length > 0 && rnd() < 0.95) nodes[i].parent = lanes[int(laneCount)];
  }
  const edges = [];
  for (let k = 0, count = int(leafCount * 2.2); k < count; k++) {
    const labelled = rnd() < 0.3;
    let src = int(leafCount);
    let dst = int(leafCount);
    if (rnd() < 0.05) dst = src;
    if (rnd() < 0.05) src = int(nodes.length);
    const e = { src, dst, minlen: rnd() < 0.9 ? 1 : 2 + int(2), labelW: labelled ? size(rnd, 20, 80) : 0, labelH: labelled ? 18 : 0, route: [], labelX: 0, labelY: 0 };
    edges.push(e);
    if (rnd() < 0.05) edges.push({ ...e, route: [] });
  }
  if (lanes.length > 0 && rnd() < 0.3) edges.push({ src: lanes[0], dst: leaves[0], minlen: 1, labelW: 0, labelH: 0, route: [], labelX: 0, labelY: 0 });
  // Now and then two groups that hold each other, which no lane's items can be drawn around.
  if (inner.length >= 2 && rnd() < 0.08) {
    nodes[inner[0]].parent = inner[1];
    nodes[inner[1]].parent = inner[0];
  }
  const lanePad = [24, 12, 24, 30][int(4)];
  return { nodes, edges, dir: DIRS[int(4)], opt: options(rnd), lanePad };
}

function layeredGraph(seed) {
  const rnd = random(seed);
  const int = (n) => Math.floor(rnd() * n);
  const nodes = [];
  const count = 1 + int(40);
  for (let i = 0; i < count; i++) {
    const node = lnode(size(rnd, 0, 150), size(rnd, 0, 80));
    if (rnd() < 0.1) node.pin = rnd() < 0.5 ? -1 : 1;
    if (rnd() < 0.5) node.span = rnd() < 0.3 ? Infinity : Math.max(0, node.w - 24);
    nodes.push(node);
  }
  const edges = [];
  for (let k = 0, m = int(count * 2.5); k < m; k++) {
    const labelled = rnd() < 0.25;
    const tail = int(count);
    // No self-loops: both callers keep them out of layered, which cannot lay them out.
    const head = int(count);
    if (head === tail) continue;
    const e = ledge(tail, head, rnd() < 0.85 ? 1 : 1 + int(3), labelled ? size(rnd, 10, 60) : 0, labelled ? 20 : 0);
    if (rnd() < 0.15) e.tailDx = int(41) - 20;
    if (rnd() < 0.15) e.headDx = int(41) - 20;
    if (rnd() < 0.1) e.tailFixed = true;
    if (rnd() < 0.1) e.headFixed = true;
    edges.push(e);
  }
  // Ports on a group's border, each with the one edge that passes it.
  for (let k = 0, ports = rnd() < 0.3 ? int(6) : 0; k < ports; k++) {
    const start = rnd() < 0.5;
    const port = nodes.length;
    nodes.push(lnode(0, 0, start ? 3 : 4));
    const inner = int(count);
    const e = start ? ledge(port, inner) : ledge(inner, port);
    if (rnd() < 0.5) {
      if (start) e.headDx = int(21) - 10;
      else e.tailDx = int(21) - 10;
    }
    edges.push(e);
  }
  return { nodes, edges, opt: options(rnd) };
}

// Structured graphs: chains, fans, grids, complete bipartite and long skips.
function structured() {
  const out = [];
  const node = (w = 60, h = 40, span = 0) => {
    const c = cnode(w, h);
    c.span = span;
    return c;
  };
  const edge = (src, dst, labelW = 0, labelH = 0) => ({ src, dst, minlen: 1, labelW, labelH, route: [], labelX: 0, labelY: 0 });
  const seq = (nodes) => nodes.forEach((n, i) => (n.seq = i));
  for (const span of [0, 36, 76, Infinity]) {
    for (const k of [1, 2, 3, 5, 7, 12, 25, 40]) {
      const nodes = [node(100, 40, span)];
      const edges = [];
      for (let i = 1; i <= k; i++) {
        nodes.push(node(100, 40, span));
        edges.push(edge(0, i));
      }
      seq(nodes);
      out.push({ nodes, edges });
      const back = structuredClone({ nodes, edges });
      back.edges = back.edges.map((e) => edge(e.dst, e.src, 30, 20));
      out.push(back);
    }
    for (const k of [2, 3, 4, 6]) {
      const nodes = Array.from({ length: 2 * k }, () => node(80, 40, span));
      const edges = [];
      for (let i = 0; i < k; i++) for (let j = k; j < 2 * k; j++) edges.push(edge(i, j));
      seq(nodes);
      out.push({ nodes, edges });
    }
    for (const k of [3, 8, 20]) {
      const nodes = Array.from({ length: k }, () => node(80, 40, span));
      const edges = [];
      for (let i = 0; i + 1 < k; i++) edges.push(edge(i, i + 1));
      for (let i = 0; i + 3 < k; i += 2) edges.push(edge(i, i + 3, i % 4 === 0 ? 40 : 0, 20));
      edges.push(edge(k - 1, 0));
      seq(nodes);
      out.push({ nodes, edges });
    }
    {
      const nodes = Array.from({ length: 16 }, () => node(50, 30, span));
      const edges = [];
      for (let r = 0; r < 4; r++)
        for (let c = 0; c < 4; c++) {
          if (c < 3) edges.push(edge(r * 4 + c, r * 4 + c + 1));
          if (r < 3) edges.push(edge(r * 4 + c, r * 4 + c + 4));
        }
      seq(nodes);
      out.push({ nodes, edges });
    }
    {
      const nodes = [node(60, 40, span), node(60, 40, span)];
      seq(nodes);
      for (const edges of [[edge(0, 1), edge(0, 1)], [edge(0, 1), edge(1, 0)], [edge(0, 1, 40, 20), edge(0, 1, 50, 20), edge(0, 1)], [edge(0, 0, 30, 20), edge(0, 1)]]) out.push({ nodes: structuredClone(nodes), edges });
    }
  }
  return out;
}

// Graphs large enough to reach the limits: many dummies, more than 600 nodes, crowded gaps, many ports.
function large(seed) {
  const rnd = random(seed);
  const int = (n) => Math.floor(rnd() * n);
  const shape = seed % 4;
  const nodes = [];
  const edges = [];
  const edge = (src, dst, minlen = 1) => ({ src, dst, minlen, labelW: 0, labelH: 0, route: [], labelX: 0, labelY: 0 });
  if (shape === 0) {
    // A long chain with many edges that skip most of it: past MAX_DUMMIES.
    const n = 260;
    for (let i = 0; i < n; i++) nodes.push(cnode(40, 30));
    for (let i = 0; i + 1 < n; i++) edges.push(edge(i, i + 1));
    for (let k = 0; k < 120; k++) edges.push(edge(int(20), n - 1 - int(20)));
  } else if (shape === 1) {
    // Wide: more than 600 nodes, so a single crossing run.
    for (let i = 0; i < 700; i++) nodes.push(cnode(20 + int(40), 20));
    for (let i = 1; i < 700; i++) edges.push(edge(int(i), i));
    for (let k = 0; k < 200; k++) edges.push(edge(int(700), int(700)));
  } else if (shape === 2) {
    // More than 400 edges across one gap.
    const top = 25;
    const bottom = 25;
    for (let i = 0; i < top + bottom; i++) {
      const c = cnode(60, 30);
      c.span = 40;
      nodes.push(c);
    }
    for (let i = 0; i < top; i++) for (let j = 0; j < bottom; j++) if (rnd() < 0.8) edges.push(edge(i, top + j));
  } else {
    // Deep groups with edges across them: past MAX_PORTS.
    let parent = -1;
    for (let d = 0; d < 60; d++) {
      const g = cnode(0, 0, parent);
      g.isGroup = true;
      g.padX = 4;
      g.padTop = 4;
      g.padBottom = 4;
      parent = nodes.length;
      nodes.push(g);
    }
    const first = nodes.length;
    for (let i = 0; i < 40; i++) nodes.push(cnode(30, 20, i % 2 === 0 ? -1 : 59));
    for (let k = 0; k < 800; k++) edges.push(edge(first + int(40), first + int(40)));
  }
  nodes.forEach((n, i) => (n.seq = i));
  return { nodes, edges, dir: DIRS[int(4)], opt: options(rnd) };
}

progress('diagrams done');
for (let seed = 1; seed <= 2500; seed++) {
  const g = compoundGraph(seed);
  addCompound(g.nodes, g.edges, g.dir, g.opt);
}
for (let seed = 1; seed <= 1500; seed++) {
  const g = laneGraph(seed);
  addLanes(g.nodes, g.edges, g.dir, g.opt, g.lanePad);
}
for (let seed = 1; seed <= 1500; seed++) {
  const g = layeredGraph(seed);
  addLayered(g.nodes, g.edges, g.opt);
}
progress('random done');
let k = 0;
for (const g of structured()) {
  for (const dir of DIRS) {
    k++;
    addCompound(structuredClone(g.nodes), structuredClone(g.edges), dir, k % 3 === 0 ? { ...OPTIONS, tracks: true } : k % 3 === 1 ? { ...OPTIONS, headRoom: 8 } : OPTIONS);
  }
}
progress('structured done');
for (let seed = 1; seed <= 12; seed++) {
  progress(`large ${seed}`);
  const g = large(seed);
  addCompound(g.nodes, g.edges, g.dir, g.opt);
}
// Lanes large enough to reach the routing budgets and the dummy limit.
for (const [leaves, laneCount, count, chain] of [[120, 4, 260, false], [300, 4, 800, false], [300, 2, 100, true]]) {
  const rnd = random(leaves * 31 + count);
  const int = (n) => Math.floor(rnd() * n);
  const nodes = [];
  for (let i = 0; i < leaves; i++) nodes.push(cnode(40 + int(80), 30 + int(20)));
  const lanes = [];
  for (let k = 0; k < laneCount; k++) {
    const lane = cnode(0, 0);
    lane.isGroup = true;
    lane.padX = 20;
    lane.padTop = 36;
    lane.padBottom = 20;
    lane.minW = 60;
    lanes.push(nodes.length);
    nodes.push(lane);
  }
  for (let i = 0; i < leaves; i++) nodes[i].parent = lanes[int(laneCount)];
  const edges = [];
  const edge = (src, dst) => ({ src, dst, minlen: 1, labelW: rnd() < 0.2 ? 40 : 0, labelH: 18, route: [], labelX: 0, labelY: 0 });
  if (chain) {
    for (let i = 0; i + 1 < leaves; i++) edges.push(edge(i, i + 1));
    for (let k = 0; k < count; k++) edges.push(edge(int(20), leaves - 1 - int(20)));
  } else {
    for (let k = 0; k < count; k++) edges.push(edge(int(leaves), int(leaves)));
  }
  addLanes(nodes, edges, DIRS[int(4)], OPTIONS, 24);
}
// Upstream's own tests' graphs.
for (let seed = 1; seed <= 300; seed++) addCompound(...upstreamCompound(seed), OPTIONS);
for (let seed = 1; seed <= 300; seed++) for (const lanePad of [24, 12]) addLanes(...upstreamLanes(seed), OPTIONS, lanePad);

function upstreamCompound(seed) {
  const rnd = random(seed);
  const int = (n) => Math.floor(rnd() * n);
  const nodes = [];
  const groups = [];
  const groupCount = int(5);
  for (let g = 0; g < groupCount; g++) {
    const node = cnode(0, 0, groups.length > 0 && rnd() < 0.4 ? groups[int(groups.length)] : -1);
    node.isGroup = true;
    node.padX = 20;
    node.padTop = 40;
    node.padBottom = 20;
    node.minW = 60;
    node.dir = rnd() < 0.3 ? DIRS[int(4)] : undefined;
    node.seq = 1000 + g;
    if (seed % 2 === 0) node.span = Infinity;
    groups.push(nodes.length);
    nodes.push(node);
  }
  const leaves = 3 + int(30);
  for (let i = 0; i < leaves; i++) {
    const parent = groups.length > 0 && rnd() < 0.5 ? groups[int(groups.length)] : -1;
    const node = cnode(30 + int(120), 30 + int(50), parent);
    node.seq = i;
    if (seed % 2 === 0) node.span = Math.min(node.w, node.h) - 24;
    nodes.push(node);
  }
  const edges = [];
  const count = int(leaves * 2);
  for (let i = 0; i < count; i++) {
    const labelled = rnd() < 0.3;
    edges.push({ src: int(nodes.length), dst: int(nodes.length), minlen: 1, labelW: labelled ? 20 + int(60) : 0, labelH: labelled ? 24 : 0, route: [], labelX: 0, labelY: 0 });
  }
  return [nodes, edges, DIRS[int(4)]];
}

function upstreamLanes(seed) {
  const rnd = random(seed);
  const int = (n) => Math.floor(rnd() * n);
  const group = (parent = -1) => {
    const node = cnode(0, 0, parent);
    node.isGroup = true;
    node.padX = 20;
    node.padTop = 36;
    node.padBottom = 20;
    node.minW = 60;
    return node;
  };
  const nodes = [];
  const leaves = [];
  const lanes = [];
  const leafCount = 1 + int(30);
  const laneCount = 1 + int(5);
  const inner = [];
  for (let i = 0; i < leafCount; i++) {
    nodes.push(cnode(30 + int(120), 24 + int(60)));
    leaves.push(i);
  }
  for (let k = 0; k < laneCount; k++) {
    lanes.push(nodes.length);
    nodes.push(group());
  }
  for (let k = 0; k < int(4); k++) {
    const lane = lanes[int(laneCount)];
    inner.push(nodes.length);
    nodes.push(group(lane));
  }
  for (const i of leaves) {
    if (inner.length > 0 && rnd() < 0.25) nodes[i].parent = inner[int(inner.length)];
    else nodes[i].parent = lanes[int(laneCount)];
  }
  const edges = [];
  for (let k = 0, count = int(leafCount * 2); k < count; k++) {
    const labelled = rnd() < 0.3;
    edges.push({ src: int(leafCount), dst: int(leafCount), minlen: 1, labelW: labelled ? 20 + int(80) : 0, labelH: labelled ? 18 : 0, route: [], labelX: 0, labelY: 0 });
  }
  if (rnd() < 0.3) edges.push({ src: lanes[0], dst: leaves[0], minlen: 1, labelW: 0, labelH: 0, route: [], labelX: 0, labelY: 0 });
  return [nodes, edges, DIRS[int(4)]];
}

// Math.hypot, which the layout uses to tell how near two curves pass.
const hypot = [];
{
  const rnd = random(7);
  for (let i = 0; i < 4000; i++) {
    const a = (rnd() - 0.5) * 10 ** Math.floor(rnd() * 8 - 2);
    const b = i % 5 === 0 ? 0 : (rnd() - 0.5) * 10 ** Math.floor(rnd() * 8 - 2);
    hypot.push([num(a), num(b), num(Math.hypot(a, b))]);
  }
}
cases.push({ kind: 'hypot', input: hypot });

const body = cases.map((c) => JSON.stringify(c)).join('\n') + '\n';
writeFileSync(new URL('../internal/layout/testdata/cases.jsonl.gz', import.meta.url), gzipSync(body, { level: 9 }));
const counts = {};
for (const c of cases) counts[c.kind] = (counts[c.kind] ?? 0) + 1;
console.log(JSON.stringify({ ...counts, fromDiagrams, bytes: body.length }));
