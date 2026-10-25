// Writes the differential fixtures for the shared modules (internal/util, internal/svg,
// internal/diagrams/common, internal/jsmath): calls of upstream's functions under V8 and what
// they returned. Each Go package's tests replay its file through internal/fixture.
//
// Usage: node scripts/gen-shared-cases.mjs <pele checkout at pele.UpstreamVersion>
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const [, , peleDir] = process.argv;
if (!peleDir) throw new Error('usage: gen-shared-cases.mjs <pele checkout>');

// The modules import each other as .js; esbuild resolves them to the .ts sources.
const esbuild = createRequire(join(peleDir, 'package.json'))('esbuild');
const modules = {
  chars: 'src/util/chars.ts',
  unicode: 'src/util/unicode.ts',
  url: 'src/util/url.ts',
  reference: '@braintree/sanitize-url',
  builder: 'src/svg/builder.ts',
  theme: 'src/svg/theme.ts',
  title: 'src/svg/title.ts',
  cluster: 'src/svg/cluster.ts',
  edges: 'src/svg/edges.ts',
  shapes: 'src/svg/shapes.ts',
  measurer: 'src/text/measurer.ts',
  label: 'src/text/label.ts',
  tokens: 'src/diagrams/common/tokens.ts',
  accDescr: 'src/diagrams/common/accDescr.ts',
  caseless: 'src/diagrams/common/caseless.ts',
  scan: 'src/diagrams/common/scan.ts',
  outline: 'src/diagrams/common/outline.ts',
  generics: 'src/diagrams/common/generics.ts',
  icon: 'src/diagrams/common/icon.ts',
  fit: 'src/diagrams/common/fit.ts',
  fitText: 'src/diagrams/common/fit-text.ts',
  fitWidth: 'src/diagrams/common/fit-width.ts',
  literalLabel: 'src/diagrams/common/literal-label.ts',
};
const entry = Object.entries(modules)
  .map(([name, path]) => `export * as ${name} from ${JSON.stringify(path.startsWith('src/') ? './' + path : path)};`)
  .join('\n');
const out = join(mkdtempSync(join(tmpdir(), 'pele-cases-')), 'bundle.mjs');
await esbuild.build({
  stdin: { contents: entry, resolveDir: peleDir, loader: 'ts' },
  bundle: true,
  format: 'esm',
  platform: 'node',
  outfile: out,
  logLevel: 'error',
});
const m = await import(pathToFileURL(out).href);

// A seeded generator, so the fixtures only change when upstream does.
let seed = 0x2545f491;
function random() {
  seed ^= seed << 13;
  seed ^= seed >>> 17;
  seed ^= seed << 5;
  return (seed >>> 0) / 0x100000000;
}
const pick = (list) => list[Math.floor(random() * list.length)];
// Strings joined from pieces, for fuzz-like inputs.
function compose(pieces, max) {
  let s = '';
  const n = 1 + Math.floor(random() * max);
  for (let i = 0; i < n; i++) s += pick(pieces);
  return s;
}

function write(path, cases) {
  const file = new URL('../' + path, import.meta.url);
  mkdirSync(dirname(fileURLToPath(file)), { recursive: true });
  writeFileSync(file, JSON.stringify(cases) + '\n');
}

// What a call returned, as JSON can carry it: undefined becomes null, and a thrown error its message.
function call(fn, name, args) {
  let want;
  try {
    want = fn(...args);
  } catch (e) {
    return { fn: name, args, throws: String(e && e.message) };
  }
  return { fn: name, args, want: want === undefined ? null : want };
}

const sections = {};
const section = (name, build) => (sections[name] = build);

section('util', () => {
  const cases = [];
  const letters = [];
  for (let c = 0; c <= 0x10000; c++) {
    if (!m.unicode.isUnicodeLetter(c)) continue;
    const last = letters[letters.length - 1];
    if (last && last[1] === c - 1) last[1] = c;
    else letters.push([c, c]);
  }
  cases.push({ fn: 'letters', args: [], want: letters });
  // Every code unit any of the tests holds for, with a bit per test.
  const tests = [m.chars.isSpace, m.chars.isBlank, m.chars.isLineEnd, m.chars.isDigit, m.chars.isLetter, m.chars.isHex, m.chars.isWord];
  const chars = [];
  for (let c = 0; c <= 0x10000; c++) {
    const mask = tests.reduce((acc, f, k) => acc | (f(c) ? 1 << k : 0), 0);
    if (mask) chars.push([c, mask]);
  }
  chars.push([-1, tests.reduce((acc, f, k) => acc | (f(NaN) ? 1 << k : 0), 0)]);
  cases.push({ fn: 'chars', args: [], want: chars });

  const reference = m.reference.sanitizeUrl;
  const urlPieces = ['http:', 'HTTP:', 'https:', 'https://', 'http://', 'javascript:', 'JaVaScRiPt:', ' ', '\n', '\t', '&#106;', '&#x6A;', '&#0000106',
    '&#55357;&#56832;', '&colon;', '&newline;', '&TAB;', '%6A', '%25', '%2525', '%E0%A4%A', '%C3%A4', '%0A', '%20', '%2e', '%2E%2e', '\\', '%5Cn', '\\t',
    'example.com', 'EXAMPLE.com', 'Ex.A_b-c', '/path', '/a/../b', '/./c', '?q=1', '#frag', 'mailto:', 'tel:', 'data:', 'vbscript:', 'user@',
    'a@b@', 'u:p@', '0x7f.1', '192.168.0.1', '1.2.3.4.5', '256', '0x', '08', '..', '.', '/', '//', 'ä', '😀', '\u2028', '\u0130', '\u212a',
    '[::1]', ':80', ':0443', ':99999', '^', '|', '{', '`', '"', '<', "'", 'obsidian://open', '\ud800', 'note', 'C:\\x', '%EF%BB%BF'];
  for (let i = 0; i < 4000; i++) {
    const url = (random() < 0.6 ? pick(['http:', 'https:', 'HTTP:', ' http:']) : '') + compose(urlPieces, 6);
    cases.push(call(m.url.sanitizeUrl, 'sanitizeUrl', [url]));
  }
  for (const url of ['https://example.com', 'http://EXAMPLE.com/Path?q=1#frag', 'click.html', './relative/path', '/absolute/path',
    'mailto:someone@example.com', 'obsidian://open?vault=Notes&file=Idea', 'javascript:alert(1)', 'JaVaScRiPt:alert(1)', '  javascript:alert(1)',
    'java\nscript:alert(1)', 'jav&#x09;ascript:alert(1)', '&#106;avascript:alert(1)', 'data:text/html,<script>alert(1)</script>',
    'vbscript:msgbox(1)', 'javascript&colon;alert(1)', '%6A%61%76%61%73%63%72%69%70%74:alert(1)', 'https://example.com/a b', 'http://',
    'ftp://example.com/file', 'www.example.com', 'C:\\Users\\file.txt', '\\\\server\\share', 'note name with spaces', '#heading', '?query=1', '',
    'about:blank', 'tel:+123456', 'https://example.com/%E0%A4%A']) {
    cases.push(call(m.url.sanitizeUrl, 'sanitizeUrl', [url]));
    // Upstream's tests/util/url.test.ts: the same answers as @braintree/sanitize-url.
    if (m.url.sanitizeUrl(url) !== reference(url)) throw new Error('upstream differs from the reference for ' + url);
  }
  for (const url of ['http:münchen.de', 'http:MÜNCHEN.de/a', 'https:bücher.example/x?y', 'http:例え.jp', 'http:xn--mnchen-3ya.de', 'http:пример.рф']) {
    cases.push(call(m.url.sanitizeUrl, 'sanitizeUrl', [url]));
  }
  for (let i = 0; i < 300; i++) cases.push(call(m.url.safeUrl, 'safeUrl', [compose(['&#106;', 'javascript:', '&colon;', 'http:', 'x.com', '&amp;', '#35;', '/p'], 4)]));

  const optionSets = [{}, { links: false }, { images: false }, { linkSchemes: [] }, { linkSchemes: ['obsidian'], imageSchemes: ['data'] }, { linkRel: '' },
    { linkRel: 'noopener "x"' }];
  const linkInputs = ['http://x', 'HTTPS://x', 'mailto:a', 'tel:1', 'obsidian://open', 'data:x', 'file:///x', '//server', '\\\\share', '/\\x', 'rel/path',
    '#a', 'a:b', '1a:b', 'a+b.c-d:x', 'é:x', 'about:blank'];
  for (const options of optionSets) {
    for (const url of linkInputs) {
      cases.push(call((u, o) => m.url.linkUrl(u, o), 'linkUrl', [url, options]));
      cases.push(call((u, o) => m.url.imageUrl(u, o), 'imageUrl', [url, options]));
      cases.push(call((u, o) => { const links = []; const svg = m.url.linked('<b/>', u, 'id"1', links, o, ' target="_blank"'); return { svg, links }; },
        'linked', [url, options]));
    }
    cases.push(call((o) => m.url.relAttr(o), 'relAttr', [options]));
  }
  return cases;
});

section('jsmath', () => {
  const cases = [];
  const args = [0, -0, 1, -1, Math.PI, Math.PI / 2, -Math.PI / 4, 1e-9, 1e300, -1e22, 2 ** 19 * Math.PI];
  for (let i = 0; i < 28; i++) args.push((Math.PI * i) / 14 - Math.PI / 2);
  for (let i = 0; i < 2500; i++) args.push((random() - 0.5) * 2 ** (Math.floor(random() * 90) - 30));
  for (let i = 0; i < 300; i++) args.push((random() - 0.5) * 2 ** (Math.floor(random() * 2000) - 1000));
  for (let i = 1; i < 100; i++) args.push((i * Math.PI) / 4);
  for (const x of args) cases.push(call(Math.sin, 'sin', [x]), call(Math.cos, 'cos', [x]));
  for (let i = 0; i < 1000; i++) {
    const n = Math.floor(random() * 4);
    const xs = [];
    for (let k = 0; k < n; k++) xs.push((random() - 0.5) * 2 ** (Math.floor(random() * 60) - 30));
    cases.push(call(Math.hypot, 'hypot', xs));
  }
  cases.push(call(Math.hypot, 'hypot', [3, 4]), call(Math.hypot, 'hypot', [1e200, 1e200]), call(Math.hypot, 'hypot', [0, -0]));
  const unit = [0, 1, -1, 0.5, -0.5, 0.975, 0.98, -0.98, 1e-9, 2 ** -27, 2 ** -28, 1.5, -2, 0.4999999999999999, 0.9999999999999999];
  for (let i = 0; i < 2000; i++) unit.push((random() * 2 - 1) * (random() < 0.2 ? 2 ** -Math.floor(random() * 40) : 1));
  for (const x of unit) cases.push(call(Math.asin, 'asin', [x]));
  const sizes = () => (random() - 0.5) * 2 ** (Math.floor(random() * 140) - 70);
  const pairs = [[0, 1], [0, -1], [1, 0], [-1, 0], [1, 1], [-1, -1], [3, 1], [1e300, 1e-300], [1e-300, -1e300], [5, -2], [0.3, 0.7]];
  for (let i = 0; i < 2500; i++) pairs.push(random() < 0.5 ? [sizes(), sizes()] : [(random() - 0.5) * 200, (random() - 0.5) * 200]);
  for (let i = 0; i < 200; i++) pairs.push([(random() - 0.5) * 20, 1]);
  for (const [y, x] of pairs) cases.push(call(Math.atan2, 'atan2', [y, x]));
  const logs = [0, -0, -1, 1, Infinity, NaN, 5e-324, 2.2250738585072014e-308, 1 - 2 ** -53, 1 + 2 ** -52, Math.E];
  for (let p = -310; p <= 308; p++) logs.push(Number(`1e${p}`), Number(`1e${p}`) * (1 - 2 ** -53), Number(`1e${p}`) * (1 + 2 ** -52));
  for (let i = 0; i < 2000; i++) logs.push(random() * 2 ** (Math.floor(random() * 120) - 60));
  for (let i = 0; i < 300; i++) logs.push(1 + (random() - 0.5) * 2 ** -(Math.floor(random() * 30)));
  for (const x of logs) cases.push(call(Math.log10, 'log10', [x]));
  return cases;
});

const metrics = m.measurer.metricsMeasurer;
const int = (lo, hi) => lo + Math.floor(random() * (hi - lo + 1));
const coord = () => pick([0, 10, 20, 48, 100, -30]) + (random() < 0.5 ? 0 : Math.round((random() - 0.5) * 2000) / 10);
const labelTexts = ['', 'Hi', 'Hello world', 'A much longer label that has to wrap somewhere', '<b>bold</b> & <i>it</i>', '中文标签', 'a<br>b', '**md**'];

section('svg', () => {
  const cases = [];
  const props = ['fill', 'FILL', 'Stroke', 'stroke-width', 'stroke-dasharray', 'color', 'font-size', 'font-weight', 'font-style',
    'opacity', 'rx', 'fill-opacity', 'text-decoration', 'unknown', 'stro\u212Ae', ' fill ', 'f\u0130ll', '', 'font-family'];
  const values = ['red', '#fff', '12px', '12', '2.5px', '300', 'bold', 'bolder', '600', '700', ' 0x258 ', 'italic', 'url(x)', 'URL (x)',
    'javascript:alert(1)', 'a;b', 'x !important', 'x !IMPORTANT', '!important', '/* c */', 'image-set(a)', '@IMPORT', '', '  ',
    '0', '201px', '200px', '..5', '1e2', 'expression\u00a0(x)', 'ur l(x)', 'a<b', 'a\\b', '"q"&amp;', '5 5'];
  for (let i = 0; i < 1500; i++) {
    const decls = [];
    for (let n = int(0, 5); n > 0; n--) decls.push(pick(props) + pick([':', ' : ', '', '::']) + pick(values));
    cases.push(call(m.theme.resolveStyle, 'resolveStyle', [decls]));
  }
  for (let i = 0; i < 300; i++) cases.push(call(m.theme.classNames, 'classNames', [compose(['a', 'B-c', '_', ' ', ',', '.x', 'é', '😀', '\t', '--', '9', '!!', ''], 6)]));
  for (let i = -20; i <= 20; i++) cases.push(call(m.theme.seriesColor, 'seriesColor', [i]));

  for (let i = 0; i < 120; i++) {
    const args = [pick([...labelTexts, 'Quarterly results for the whole company and more']), pick([14, 16, 13.5]), pick([8, 0, 12.5]),
      pick([0, 100, 333.3]), pick([0, 50, 77.7]), '<g/>'];
    cases.push(call((t, size, pad, w, h, c) => m.title.withTitle(t || undefined, metrics, size, pad, w, h, c), 'withTitle', args));
  }

  for (let i = 0; i < 600; i++) {
    const routes = [];
    for (let r = int(0, 4); r > 0; r--) {
      const route = [];
      for (let p = int(1, 4); p > 0; p--) route.push(coord(), pick([-40, -39.6, 0, 20, coord()]), int(0, 1));
      routes.push(route);
    }
    const c = { x: pick([0, 30, 100.5]), y: pick([0, 40, 20]), w: pick([80, 200, 33.3]), h: pick([80, 40, 61]) };
    const width = pick([0, 30, 60, 120.5]);
    const pad = pick([12, 8, 20]);
    const crossings = new Map();
    for (const route of routes) m.cluster.markCrossings(crossings, route);
    const x = m.cluster.clusterTitleX(c, width, crossings, pad);
    const other = coord();
    cases.push({ fn: 'cluster', args: [c, width, routes, pad, other], want: [x, m.cluster.titleCrossed(c, width, x, crossings), m.cluster.titleCrossed(c, width, other, crossings)] });
  }
  for (let i = 0; i < 60; i++) {
    const parents = [-1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 0, -1];
    const args = [pick(labelTexts), coord(), coord(), pick(['', ' class="t"']), pick(['id1', 'a&b']), parents, int(0, parents.length - 1)];
    cases.push(call((t, x, y, attrs, id, ps, index) =>
      m.cluster.struckTitle(m.label.layoutLabel(t, false, metrics, 14, 200), x, y, attrs, id, ps.map((parent) => ({ parent })), index), 'struckTitle', args));
  }

  const curves = [null, 'linear', 'step', 'stepBefore', 'stepAfter', 'basis'];
  for (let i = 0; i < 1500; i++) {
    const route = [];
    let x = coord();
    let y = coord();
    for (let p = int(2, 6); p > 0; p--) {
      route.push(x, y, int(0, 1));
      if (random() < 0.4) x = random() < 0.5 ? x : coord();
      else x = coord();
      y = random() < 0.4 ? y : coord();
    }
    const startTrim = pick([0, 0, 7, 8, 4]);
    const endTrim = pick([0, 7, 7, 8, 4]);
    const reach = random() < 0.5;
    const args = [route, pick(curves), startTrim, endTrim, reach ? pick([0, 13, 14, 20]) : null, reach ? pick([0, 13, 14, 20]) : null];
    cases.push(call((r, c, st, et, sr, er) => m.edges.routePath(r, c ?? undefined, st, et, sr ?? undefined, er ?? undefined), 'routePath', args));
  }
  for (const typ of ['arrow_point', 'arrow_circle', 'arrow_cross', 'none', '']) {
    cases.push(call(m.edges.markerTrim, 'markerTrim', [typ]));
    for (let i = 0; i < 20; i++) {
      const a = random() * 7;
      cases.push(call(m.edges.marker, 'marker', [typ, coord(), coord(), Math.cos(a), Math.sin(a), pick(['var(--_l)', 'red'])]));
    }
  }
  for (let i = 0; i < 40; i++) {
    const args = [pick(['e1', 'a&lt;b']), pick(labelTexts), coord(), coord(), pick(['', ' fill="red"'])];
    cases.push(call((id, t, x, y, attrs) => m.edges.edgeLabelSvg(id, m.label.layoutLabel(t, false, metrics, 14, 200), x, y, attrs), 'edgeLabelSvg', args));
  }
  for (let i = 0; i < 200; i++) {
    const args = [coord(), coord(), pick([40, 80, 120.5, 10]), pick([30, 44, 61.3, 6]), int(0, 4), random() < 0.5, pick([20, 30]), pick([0, 7, 8]), pick([0, 40.5]), pick([0, 21])];
    cases.push(call((x, y, w, h, k, sideways, base, trim, labelW, labelH) => {
      const e = { labelW, labelH, labelX: 0, labelY: 0 };
      const path = m.edges.loopPath({ x, y }, w, h, k, sideways, base, trim, e);
      return { path, labelX: e.labelX, labelY: e.labelY };
    }, 'loopPath', args));
  }
  for (let i = 0; i < 50; i++) {
    const ops = [];
    for (let n = int(1, 8); n > 0; n--) ops.push([int(0, 3), int(-1, 3), pick(['arrow_point', 'arrow_circle', 'arrow_cross', 'none'])]);
    const queries = [];
    for (let n = 0; n < 6; n++) queries.push([int(0, 3), int(-1, 3)]);
    cases.push(call((adds, qs) => {
      const reach = m.edges.sideReach();
      for (const [node, side, typ] of adds) reach.add(node, side, typ);
      return qs.map(([node, side]) => reach.of(node, side));
    }, 'sideReach', [ops, queries]));
  }

  const source = readFileSync(join(peleDir, 'src/svg/shapes.ts'), 'utf8');
  const names = [...source.matchAll(/^  '?([\w-]+)'?: \{/gm)].map((x) => x[1]);
  names.push('unknown', 'constructor', '');
  for (const name of names) {
    for (const [tw, th] of [[0, 0], [40, 21], [123.4, 42], [7, 63]]) cases.push(call(m.shapes.shapeSize, 'shapeSize', [name, tw, th]));
    for (const [w, h] of [[100, 40], [44, 44], [181.3, 77.7]]) {
      cases.push(call(m.shapes.drawShape, 'drawShape', [name, w, h, ' fill="var(--_s)" stroke="var(--_b)"', ' stroke="red"']));
      for (let side = 0; side < 4; side++) for (const t of [0, 5, -13.3, 100]) cases.push(call(m.shapes.shapeInset, 'shapeInset', [name, w, h, side, t]));
      cases.push(call(m.shapes.shapeSpan, 'shapeSpan', [name, w, h, false]), call(m.shapes.shapeSpan, 'shapeSpan', [name, w, h, true]));
    }
    cases.push(call(m.shapes.shapeHasLabel, 'shapeHasLabel', [name]));
  }
  for (let i = 0; i < 400; i++) {
    const c = { x: pick([0, 50]), y: pick([0, 30]), w: pick([100, 60]), h: pick([40, 80]), isGroup: random() < 0.2 };
    const route = [c.x + pick([-c.w / 2, c.w / 2, 0, 10, -c.w / 2 + 0.3]), c.y + pick([-c.h / 2, c.h / 2, 0, 5, c.h / 2 - 0.4]), 0];
    const view = { shape: pick(names), w: c.w - pick([0, 10]), h: c.h - pick([0, 6]) };
    cases.push(call((r, box, v) => {
      const side = m.shapes.endSide(r, 0, box);
      const copy = r.slice();
      m.shapes.insetRoute(copy, 0, v, box);
      return [side, copy];
    }, 'insetRoute', [route, c, view]));
  }
  return cases;
});

section('common', () => {
  const cases = [];
  const t = m.tokens;
  const WORD = { name: 'WORD', pattern: /[a-z]+/y, first: 'abcdefghijklmnopqrstuvwxyz' };
  const types = [m.accDescr.accDescr('DESCR'), t.ACC_TITLE, t.TITLE, t.STRING, t.NEWLINE, t.WHITESPACE, t.YAML, t.DIRECTIVE, t.SINGLE_LINE_COMMENT, t.keyword('end'), WORD];
  const pieces = ['"a b"', "'c'", '"x\\"y"', '"\\n"', '"open', "'\\\n'", '"\u2028"', ' ', '\n', 'end', 'endx', 'word', 'accDescr {', '}',
    'accDescr: d %% c', 'accTitle: t', 'title T', '%% c', '%%{init}%%', '---\n', '"\\', '😀', '\t', '\r\n', 'é'];
  for (let i = 0; i < 1000; i++) {
    const src = compose(pieces, 8);
    cases.push(call((x) => { const r = t.tokenize(x, types, 'test'); return { kinds: r.kinds, starts: r.starts, ends: r.ends }; }, 'tokenize', [src]));
  }
  const strPieces = ['a', '\\n', '\\t', '\\b', '\\f', '\\r', '\\v', '\\0', '\\\\', '\\"', '\\x', '\\😀', 'é', '"', "'", '\\'];
  for (let i = 0; i < 400; i++) cases.push(call(t.stringValue, 'stringValue', [compose(strPieces, 6)]));
  for (const x of ['', '"', '""', "'a'", '"\\"', 'x']) cases.push(call(t.stringValue, 'stringValue', [x]));
  for (let i = 0; i < 200; i++) {
    const src = compose(['Graph', 'graph', 'GRAPH', 'gr', 'x', 'ɢraph', ' '], 3);
    cases.push(call(m.caseless.wordAt, 'wordAt', [src, int(0, 2), pick(['graph', 'gr', 'x'])]));
  }
  const triviaPieces = ['%% c\n', '% c\n', 'x%%y\n', '}%%z', '%{', ' ', '\u00a0', '\t', '\n', '# h\n', 'a', 'accTitle: hi\n', 'accdescr : d\n',
    'accDescr { multi\nline }', 'ACCDESCR{x', 'acctitle', 'accDescr {', '}', 'section'];
  for (let i = 0; i < 600; i++) {
    const src = compose(triviaPieces, 6);
    const p = int(0, Math.max(0, src.length - 1));
    cases.push(call(m.scan.skipTrivia, 'skipTrivia', [src, p]));
    cases.push(call(m.scan.lineEnd, 'lineEnd', [src, p]));
    cases.push(call(m.scan.keywordAt, 'keywordAt', [src, p, pick(['accdescr', 'section', 'a'])]));
    cases.push(call((x, q) => { const out = { types: [], texts: [], starts: [] }; const end = m.scan.accessibility(x, q, out, 10); return { end, out }; },
      'accessibility', [src, pick([0, p])]));
  }

  const outlinePieces = ['mindmap', 'kanban', '\n', '  ', '    ', '\t', 'root', 'id', '((circle))', '(round)', '[sq]', '))bang((', ')cloud(', '{{hex}}',
    '("str")', '["`md`"]', '::icon(fa fa-x)', ':::cls', '%% c', '@{ shape: rect, label: "a\n  b" }', '@', '-)x(-', '}', 'é', '\u00a0', '\r\n', '^', '"', '`'];
  for (let i = 0; i < 1000; i++) {
    const kanban = random() < 0.5;
    const src = (random() < 0.8 ? (kanban ? 'kanban\n' : 'mindmap\n') : '') + compose(outlinePieces, 10) + '\n';
    cases.push(call((x, k) => m.outline.tokenize(x, k), 'tokenizeOutline', [src, kanban]));
    cases.push(call((x, k) => {
      const calls = [];
      m.outline.parseOutline(x, {
        getType: (a, b) => { calls.push(['getType', a, b]); return m.outline.getType(a, b); },
        addNode: (...args) => calls.push(['addNode', ...args]),
        decorateNode: (d) => calls.push(['decorateNode', d]),
      }, k);
      return calls;
    }, 'parseOutline', [src, kanban]));
  }

  const genericPieces = ['List', '~', 'int', ',', ' ', 'Map', 'K', 'V', '~~', 'é'];
  for (let i = 0; i < 500; i++) cases.push(call(m.generics.parseGenericTypes, 'parseGenericTypes', [compose(genericPieces, 8)]));

  for (const name of ['car', 'none', 'a&amp;b', '"q"']) {
    cases.push(call((n, x, y, side) => m.icon.iconSvg(n, x, y, side, (k) => (k === 'car' ? '<path d="M0 0"/>' : k === 'none' ? null : undefined)),
      'iconSvg', [name, coord(), coord(), pick([14, 15.4])]));
  }

  const labelPieces = ['word', ' ', 'longerwordwithoutbreaks', '<br>', '<BR />', '<b>', '</b>', '&amp;', '&#35;', 'ﬂ°°lt¶ß', 'ﬂ°x¶ß', ':fa-car', 'fa:fa-car',
    '中文', '😀', '\t', 'a<c', '<br/ >', '&', ';'];
  for (let i = 0; i < 600; i++) {
    const raw = compose(labelPieces, 8);
    const args = [raw, pick([12, 14, 16.5]), pick([20, 60, 150, 400]), pick([0, 1, 2])];
    cases.push(call((r, size, max, base) => m.fit.plainLabel(r, metrics, size, max, base), 'plainLabel', args));
    cases.push(call((r, size, max, base) => m.fit.fitLabel(r, metrics, size, max, base), 'fitLabel', args));
  }
  for (let i = 0; i < 500; i++) {
    const args = [compose(['word', ' ', 'W', '中', '😀', 'iiii', '  '], 6), pick([0, 10, 30, 80, 300]), pick([14, 16, 20]), pick([6, 8, 10]), pick([0, 1, 3])];
    cases.push(call((x, max, size, min, style) => m.fitText.fitText(x, max, size, min, metrics, style), 'fitText', args));
  }

  const draw = (scale, extra) => (plot, flag) => ({ svg: `${plot}|${flag}`, width: plot * scale + (flag ? extra : 0) + 40, height: 1, links: [] });
  for (let i = 0; i < 300; i++) {
    const options = {};
    if (random() < 0.8) options.maxWidth = pick([0, -1, 100, 320, 500, 700, 1000, 333.3]);
    if (random() < 0.3) options.padding = pick([0, 4, 16]);
    if (random() < 0.3) options.autoDirection = random() < 0.5;
    if (random() < 0.3) options.directionBreakpoint = pick([0, 300, 800]);
    const natural = pick([200, 400, 800, 1200]);
    const least = pick([50, 100, 300, 2000]);
    const scale = pick([1, 1.3, 0.7]);
    const extra = pick([0, 30, -100]);
    cases.push(call((o, n, l, sc, ex) => m.fitWidth.fitWidth(o, n, l, draw(sc, ex)), 'fitWidth', [options, natural, least, scale, extra]));
    cases.push(call(m.fitWidth.titleRoom, 'titleRoom', [options]));
    cases.push(call((o, n, l, a) => m.fitWidth.capWidth(o, n, l, a), 'capWidth', [options, natural, least, pick([0, 40])]));
    cases.push(call((o, n, sc, ex) => m.fitWidth.turnToFit(o, (down) => draw(sc, ex)(n, down)), 'turnToFit', [options, natural, scale, extra]));
    cases.push(call((o, n, sc, ex) => m.fitWidth.tighten(o, (tight) => draw(sc, ex)(n, tight)), 'tighten', [options, natural, scale, extra]));
  }
  for (const d of ['LR', 'RL', 'TB', '', null]) cases.push(call((x) => m.fitWidth.runsAcross(x ?? undefined), 'runsAcross', [d]));

  const literalPieces = ['file', '.', 'name', ' ', '/', 'averyveryverylongsegment', '\n', '😀', '中文', '\u00a0', '  ', 'x'];
  for (let i = 0; i < 400; i++) {
    const args = [compose(literalPieces, 8), pick([12, 14]), pick([10, 40, 90, 300]), pick([0, 1, 2, 4])];
    cases.push(call((x, size, room, style) => m.literalLabel.literalLabel(x, metrics, size, room, style), 'literalLabel', args));
    const markup = [compose(labelPieces, 8), pick([12, 14]), pick([30, 80, 400]), pick([20, 60, 200])];
    cases.push(call((x, size, max, room) => m.literalLabel.breakLabel(m.label.layoutLabel(x, true, metrics, size, max), metrics, room), 'breakLabel', markup));
    cases.push(call((x, size, max, left, top) => m.literalLabel.leftLabelSvg(m.label.layoutLabel(x, true, metrics, size, max), left, top, ' class="l"'),
      'leftLabelSvg', [markup[0], markup[1], markup[2], coord(), coord()]));
  }
  return cases;
});

for (const [name, build] of Object.entries(sections)) {
  // Each section starts from the same seed, so adding cases to one leaves the others as they are.
  seed = 0x2545f491;
  const target = { util: 'internal/util', svg: 'internal/svg', common: 'internal/diagrams/common', jsmath: 'internal/jsmath' }[name];
  write(`${target}/testdata/cases.json`, build());
}
