package pele_test

import (
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	pele "github.com/Broderick-Westrope/pele-go"
	_ "github.com/Broderick-Westrope/pele-go/all"
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsjson"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/oracle"
)

var fixedNow = time.Date(2026, time.October, 20, 9, 30, 0, 0, time.UTC)

// lengthMeasurer is a measurer unlike Pele's default, so a test that passes it shows the
// measurer reaches layout on both sides.
type lengthMeasurer struct{}

func (lengthMeasurer) Width(text string, size float64, style pele.Style) float64 {
	return float64(jsstr.Len(text))*size*0.55 + float64(style)*0.25
}

type jsMeasurer struct{ m pele.TextMeasurer }

func (j jsMeasurer) Width(text string, size float64, style int) float64 {
	return j.m.Width(text, size, pele.Style(style))
}

func newOracle(t testing.TB) *oracle.Oracle {
	t.Helper()
	o, err := oracle.New()
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func corpus(t testing.TB, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("internal", "oracle", "testdata", "corpus", name))
	if err != nil {
		t.Fatal(err)
	}
	// jsjson keeps lone surrogates, which encoding/json would replace.
	parsed, err := jsjson.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	var sources []string
	for _, entry := range parsed.([]any) {
		switch entry := entry.(type) {
		case string:
			sources = append(sources, entry)
		case *jsobj.Object:
			src, _ := entry.Get("src")
			sources = append(sources, src.(string))
		}
	}
	return sources
}

var infoInputs = []string{
	"---\nconfig:\n  info:\n    useMaxWidth: true\n---\ninfo",
	"info",
	"info\n",
	"  info showInfo",
	"info\ntitle My title",
	"info\ntitle   spaced    out  ",
	"info\ntitle",
	"info\ntitle x\ttab",
	"info\naccTitle: Hi there\naccDescr: a description",
	"info\naccTitle:",
	"info\naccDescr {\n  multi\n   line\n\n\n  description  \n}",
	"info\naccDescr {\n never closed",
	"info unsupported",
	"info\ntitle inline %% comment",
	"info\n%% comment\ntitle t",
	"%%{init: {\"theme\":\"dark\"}}%%\ninfo",
	"%%{init: {'theme': 'dark'}}%%\ninfo\ntitle quoted",
	"%%{wrap}%%\ninfo",
	"---\ntitle: Front title\n---\ninfo",
	"---\ntitle: Front title\nconfig:\n  info:\n    useMaxWidth: false\n---\ninfo",
	"---\ntitle: [unclosed\n---\ninfo",
	"---\n  bad: indent\n wrong: yes\n---\ninfo",
	"info\ntitle <b>Bold</b> & <i>it</i>",
	"info\ntitle A #35; entity &amp; &lt; #quot;",
	"info\ntitle 中文标题，长一些。",
	"info\ntitle \U0001F600 emoji",
	"info\ntitle fa:fa-car icon",
	"info\ntitle a<br>b<br/>c",
	"info\ntitle " + strings.Repeat("lengthy words ", 400),
	"info\ntitle " + strings.Repeat("长", 600),
	"info\r\ntitle crlf\r\n",
	"info\n\n\ntitle x\n\n",
	"info\ntitle a\x01b\x7f",
	"info\ntitle a\"quote'",
	"info\ntitle x\ntitle y",
	"info\ntitle x\naccTitle: y\nshowInfo",
	"info showInfo extra",
	"",
	"nothing to see",
	"INFO",
	"infox",
	"\u3000info",
	strings.Repeat("x", 50001),
}

// handInputs are hand-written sources by diagram type, checked with the corpus.
var handInputs = map[string][]string{
	"info": infoInputs,
	"flowchart": {
		// A group whose only class is default.
		"flowchart TD\nsubgraph X\nA\nend\nclass X default\nB:::default",
	},
	"agentflow": {
		"agentflow-beta TB\n  flow X[\"x\"]\n    a[\"A\"]\n  end\n  class X default\n",
	},
	"wardley": {
		// An empty second stage name is falsy in JS, so no " / " joins it.
		"wardley-beta\nevolution \"A\" -> \"B\"@0.5 / \"\" -> C\n",
	},
}

// v8Inputs are hand-written sources by diagram type that only V8 can judge.
var v8Inputs = map[string][]string{
	"info":          loneSurrogateInputs,
	"flowchart":     {"flowchart LR\n  A[a" + hi + "] --> B(" + lo + "b)\n  B -->|e" + hi + "| C", "---\ntitle: \"\\uD800 x\"\n---\nflowchart TD\n  A"},
	"sequence":      {"sequenceDiagram\n  participant A as A" + hi + "\n  A->>B: hi" + lo + "\n  Note over A: n" + hi},
	"pie":           {"pie title T" + hi + "\n  \"x" + lo + "\" : 1\n  \"y\" : 2"},
	"mindmap":       {"mindmap\n  root" + hi + "\n    child" + lo + "\n    other"},
	"class":         {"classDiagram\n  class A[\"L" + hi + "\"]\n  class B {\n    +f" + lo + "()\n  }\n  A --> B : uses" + hi},
	"journey":       {"journey\n  title T" + hi + "\n  section S" + lo + "\n    task" + hi + ": 5: Me"},
	"gantt":         {"gantt\n  title G" + hi + "\n  dateFormat YYYY-MM-DD\n  section S" + lo + "\n    task" + hi + " :a1, 2026-01-01, 3d"},
	"state":         {"stateDiagram-v2\n  [*] --> S" + "\n  S : s" + hi + "\n  S --> T : go" + lo},
	"timeline":      {"timeline\n  title T" + hi + "\n  2026 : e" + lo + " : f"},
	"quadrantChart": {"quadrantChart\n  title Q" + hi + "\n  x-axis L" + lo + " --> H\n  P" + hi + ": [0.3, 0.6]"},
}

// hi and lo are a lone high and a lone low surrogate.
var (
	hi = jsstr.FromUTF16([]uint16{0xd83d})
	lo = jsstr.FromUTF16([]uint16{0xdc01})
)

// corpusFileTypes names the diagram type of each corpus file whose name is not
// "<type>-docs.json".
var corpusFileTypes = map[string]string{
	"agentflow-reference.json": "agentflow",
	"git-docs.json":            "gitGraph",
	"quadrant-docs.json":       "quadrantChart",
	"treeview-docs.json":       "treeView",
}

// corpusByType returns every corpus source, by the diagram type of the file it is in.
func corpusByType(t testing.TB) map[string][]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("internal", "oracle", "testdata", "corpus", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	byType := map[string][]string{}
	for _, file := range files {
		name := filepath.Base(file)
		typ, ok := corpusFileTypes[name]
		if !ok {
			typ, ok = strings.CutSuffix(name, "-docs.json")
		}
		if _, err := os.Stat(shardPath(typ)); !ok || err != nil {
			t.Fatalf("corpus file %s: no diagram type %q with a captured shard; add it to corpusFileTypes", name, typ)
		}
		byType[typ] = append(byType[typ], corpus(t, name)...)
	}
	return byType
}

// portable drops the sources of a type that is not registered, which only fail in the port.
func portable(sources []string) []string {
	return slices.DeleteFunc(slices.Clone(sources), func(src string) bool {
		typ, ok := core.DetectType(src)
		return ok && !core.Registered(typ)
	})
}

// TestCorpusParity checks every corpus source of each registered type, and the hand-written
// inputs, against Pele in goja and in Node, across the option sets and both measurers.
// Types run in parallel, each with its own goja runtime and node process, except the zoned
// types: goja reads the zone from time.Local, which they set, so they run first and alone.
func TestCorpusParity(t *testing.T) {
	byType := corpusByType(t)
	for typ, inputs := range handInputs {
		byType[typ] = append(byType[typ], inputs...)
	}
	for _, typ := range slices.Sorted(maps.Keys(byType)) {
		t.Run(typ, func(t *testing.T) {
			if !core.Registered(typ) {
				t.Skip("not registered")
			}
			if !zonedTypes[typ] {
				t.Parallel()
			}
			sources := portable(byType[typ])
			var o *oracle.Oracle
			nodes := map[string]*oracle.Node{}
			t.Cleanup(func() {
				for _, node := range nodes {
					_ = node.Close()
				}
			})
			inEachZone(t, typ, func(t *testing.T, zone string) {
				t.Run("goja", func(t *testing.T) {
					if o == nil {
						o = newOracle(t)
					}
					loc := setLocal(t, zone)
					for _, src := range sources {
						for _, set := range optionSets {
							assertParity(t, o, src, set.in(loc), nil)
							assertParity(t, o, src, set.in(loc), lengthMeasurer{})
						}
					}
				})
				t.Run("v8", func(t *testing.T) {
					if nodes[zone] == nil {
						nodes[zone] = startNodeIn(t, zone)
					}
					loc := location(t, zone)
					for _, src := range append(sources, v8Inputs[typ]...) {
						for _, set := range optionSets {
							assertParityV8(t, nodes[zone], src, set.in(loc), "")
							assertParityV8(t, nodes[zone], src, set.in(loc), "length")
						}
					}
				})
			})
		})
	}
}

// zonedTypes are the diagram types whose output depends on the local time zone. Their cases
// run in each of testZones, with goja, Node and the port all in that zone; other types run
// in the process's own zone.
var zonedTypes = map[string]bool{"gantt": true}

var testZones = []string{"UTC", "Australia/Sydney"}

// refereedTypes are the diagram types whose output goja does not compute as V8 does: gantt,
// as goja's Date parses strings and resolves local times unlike V8, and usecase, whose errors
// quote the message of JSON.parse, which goja words unlike V8, and railroad and wardley,
// whose repeat counts and annotation numbers print numbers past 2^64, which goja formats
// unlike V8, and sankey and sequence, whose titles and value affixes keep lone surrogates
// from YAML and JSON escapes as U+FFFD in goja, where V8 keeps them for the port to drop.
var refereedTypes = map[string]bool{"gantt": true, "usecase": true, "railroad": true, "wardley": true, "sankey": true, "sequence": true}

// zonesOf returns the zones a type's cases run in: "" alone, the process's own, for a type
// the zone does not reach.
func zonesOf(typ string) []string {
	if zonedTypes[typ] {
		return testZones
	}
	return []string{""}
}

// inEachZone runs fn in a subtest per zone for a zoned type, and once with zone "" for
// any other.
func inEachZone(t *testing.T, typ string, fn func(t *testing.T, zone string)) {
	if !zonedTypes[typ] {
		fn(t, "")
		return
	}
	for _, zone := range testZones {
		t.Run(zone, func(t *testing.T) { fn(t, zone) })
	}
}

// location loads zone, or returns nil, the port's default, for "".
func location(t testing.TB, zone string) *time.Location {
	t.Helper()
	if zone == "" {
		return nil
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// setLocal makes zone time.Local, which goja's Date reads, until the test ends.
func setLocal(t testing.TB, zone string) *time.Location {
	t.Helper()
	loc := location(t, zone)
	if loc != nil {
		saved := time.Local
		time.Local = loc
		t.Cleanup(func() { time.Local = saved })
	}
	return loc
}

// in returns the option set with the port's time zone set to loc.
func (set optionSet) in(loc *time.Location) optionSet {
	apply := set.apply
	set.apply = func(o *pele.Options) {
		apply(o)
		o.Location = loc
	}
	return set
}

// startNode starts the V8 oracle, skipping the test when node is not installed.
func startNode(t testing.TB) *oracle.Node { t.Helper(); return startNodeIn(t, "") }

// startNodeIn is startNode with node in zone; "" leaves the process's own. With
// PELE_REQUIRE_NODE=1, as CI sets it, a missing node fails the test instead, so the V8
// checks cannot pass by skipping.
func startNodeIn(t testing.TB, zone string) *oracle.Node {
	t.Helper()
	node, err := oracle.NewNodeIn(zone)
	if errors.Is(err, exec.ErrNotFound) {
		if os.Getenv("PELE_REQUIRE_NODE") == "1" {
			t.Fatal("node is not installed, and PELE_REQUIRE_NODE=1 requires it")
		}
		t.Skip("node is not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func TestDetectTypeParity(t *testing.T) {
	o := newOracle(t)
	byType := corpusByType(t)
	var sources []string
	for _, typ := range slices.Sorted(maps.Keys(byType)) {
		sources = append(sources, byType[typ]...)
	}
	for _, typ := range slices.Sorted(maps.Keys(handInputs)) {
		sources = append(sources, handInputs[typ]...)
	}
	for _, src := range sources {
		want, err := o.DetectType(src)
		if err != nil {
			want = ""
		}
		got, ok := pele.DetectType(src)
		if !ok {
			got = ""
		}
		if string(got) != want {
			t.Errorf("DetectType(%q) = %q, want %q", src, got, want)
		}
	}
}

// fuzzType is the diagram type FuzzParity and FuzzParityV8 fuzz: PELE_FUZZ_TYPE, or info.
func fuzzType(f *testing.F) string {
	typ := os.Getenv("PELE_FUZZ_TYPE")
	if typ == "" {
		typ = "info"
	}
	if !core.Registered(typ) {
		f.Fatalf("PELE_FUZZ_TYPE=%s is not a registered diagram type", typ)
	}
	return typ
}

// fuzzSeeds are the inputs of a type the parity gates know: hand-written, from the corpus,
// and from upstream's test suite as captured.
func fuzzSeeds(f *testing.F, typ string, extra ...string) {
	seeds := append(slices.Clone(handInputs[typ]), extra...)
	seeds = append(seeds, corpusByType(f)[typ]...)
	for _, rec := range readShard(f, typ) {
		if src, ok := rec.Get("src"); ok {
			seeds = append(seeds, src.(string))
		}
	}
	for _, src := range seeds {
		if len(src) < 2000 {
			f.Add(src)
		}
	}
}

// FuzzParity fuzzes the type PELE_FUZZ_TYPE names against Pele in goja.
func FuzzParity(f *testing.F) {
	typ := fuzzType(f)
	fuzzSeeds(f, typ)
	o := newOracle(f)
	// For a type goja cannot judge alone V8 referees: an input the two engines disagree on is
	// left to FuzzParityV8.
	referees := map[string]*oracle.Node{}
	if refereedTypes[typ] {
		for _, zone := range zonesOf(typ) {
			referees[zone] = startNodeIn(f, zone)
			f.Cleanup(func() { _ = referees[zone].Close() })
		}
	}
	f.Fuzz(func(t *testing.T, src string) {
		if got, err := o.DetectType(src); err == nil && got != "" && got != typ {
			t.Skipf("not a %s diagram", typ)
		}
		for _, zone := range zonesOf(typ) {
			set := optionSets[0].in(setLocal(t, zone))
			if node := referees[zone]; node != nil && !enginesAgree(t, o, node, src, set) {
				continue
			}
			assertParity(t, o, src, set, nil)
		}
	})
}

// enginesAgree reports whether Pele gives the same result for src in goja as in Node.
func enginesAgree(t *testing.T, o *oracle.Oracle, node *oracle.Node, src string, set optionSet) bool {
	t.Helper()
	gojaRes, gojaErr, err := o.Render(src, oracle.Options{Now: fixedNow, Extra: set.js})
	if err != nil {
		t.Fatalf("oracle render %q: %v", src, err)
	}
	v8Res, v8Err, err := node.Render(src, oracle.NodeOptions{Options: set.js, Now: float64(fixedNow.UnixMilli())})
	if err != nil {
		t.Fatalf("node render %q: %v", src, err)
	}
	if gojaErr != nil || v8Err != nil {
		return gojaErr != nil && v8Err != nil && *gojaErr == *v8Err
	}
	return gojaRes.SVG == v8Res.SVG && sameNumber(gojaRes.Width, v8Res.Width) && sameNumber(gojaRes.Height, v8Res.Height) &&
		slices.Equal(gojaRes.Links, v8Res.Links)
}

// optionSet is one set of render options, as Go options and as the JS object that means
// the same.
type optionSet struct {
	name  string
	js    map[string]any
	apply func(*pele.Options)
}

func ptr[T any](v T) *T { return &v }

func mustConfig(text string) *pele.Config {
	c, err := pele.ParseConfig(text)
	if err != nil {
		panic(err)
	}
	return c
}

var optionSets = []optionSet{
	{name: "defaults", apply: func(*pele.Options) {}},
	{
		name: "sized",
		js:   map[string]any{"fontSize": 13.5, "padding": 3, "idPrefix": `d1 "x"`},
		apply: func(o *pele.Options) {
			o.FontSize, o.Padding, o.IDPrefix = ptr(13.5), ptr(3.0), ptr(`d1 "x"`)
		},
	},
	{
		name: "fixed fonts",
		js:   map[string]any{"responsive": false, "fontFamily": `Inter, "Helvetica Neue", sans-serif`, "fontFamilyMono": "bad;}"},
		apply: func(o *pele.Options) {
			o.Responsive, o.FontFamily, o.FontFamilyMono = ptr(false), ptr(`Inter, "Helvetica Neue", sans-serif`), ptr("bad;}")
		},
	},
	{
		name:  "zeroes",
		js:    map[string]any{"padding": 0, "fontSize": 0, "idPrefix": ""},
		apply: func(o *pele.Options) { o.Padding, o.FontSize, o.IDPrefix = ptr(0.0), ptr(0.0), ptr("") },
	},
	{
		name: "host config",
		js:   map[string]any{"config": map[string]any{"info": map[string]any{"useMaxWidth": false}, "wrap": true}},
		apply: func(o *pele.Options) {
			o.Config = mustConfig(`{"info": {"useMaxWidth": false}, "wrap": true}`)
		},
	},
	{
		name:  "limits",
		js:    map[string]any{"limit": 12, "outputLimit": 1600},
		apply: func(o *pele.Options) { o.Limit, o.OutputLimit = ptr(12.0), ptr(1600.0) },
	},
	{
		// JS compares lengths with these as numbers and writes them into the message.
		name:  "fractional limits",
		js:    map[string]any{"limit": 9.5, "outputLimit": 1600.5},
		apply: func(o *pele.Options) { o.Limit, o.OutputLimit = ptr(9.5), ptr(1600.5) },
	},
}

func (set optionSet) options(m pele.TextMeasurer) pele.Options {
	opts := pele.Options{Measurer: m, Now: fixedNow}
	set.apply(&opts)
	return opts
}

// assertParity compares the port with Pele in goja for one source and option set. goja
// turns lone surrogates into U+FFFD in some string operations where V8 keeps them, so a
// case whose output holds one, from the source or from an escape, is left to the V8 test.
func assertParity(t *testing.T, o *oracle.Oracle, src string, set optionSet, m pele.TextMeasurer) {
	t.Helper()
	if hasLoneSurrogate(src) || outputHasLoneSurrogate(src, set.options(m)) {
		return
	}
	jsOpts := oracle.Options{Now: fixedNow, Extra: set.js}
	if m != nil {
		jsOpts.Measurer = jsMeasurer{m}
	}
	want, wantErr, err := o.Render(src, jsOpts)
	if err != nil {
		t.Fatalf("oracle render %q: %v", src, err)
	}
	compare(t, "goja/"+set.name, src, set.options(m), want, wantErr)
}

// compare renders src with the port and checks both the lossless and the public results
// against what upstream returned.
func compare(t *testing.T, label, src string, opts pele.Options, want *oracle.Result, wantErr *oracle.Error) {
	t.Helper()
	lossless, gotErr := core.Render(context.Background(), src, opts)
	public, publicErr := pele.Render(context.Background(), src, opts)

	if wantErr != nil {
		for name, got := range map[string]error{"lossless": gotErr, "public": publicErr} {
			var pe *pele.Error
			if !errors.As(got, &pe) {
				t.Errorf("%s: render %q (%s): got error %v, want PeleError %+v", label, src, name, got, *wantErr)
				return
			}
			want := *wantErr
			gotFields := oracle.Error{
				Code: string(pe.Code), Message: pe.Message, Type: pe.Type, MermaidType: pe.MermaidType,
				Line: pe.Line, Column: pe.Column, Snippet: pe.Snippet,
			}
			if name == "public" {
				want.Message, want.Snippet = jsstr.WellFormed(want.Message), jsstr.WellFormed(want.Snippet)
			}
			if gotFields != want {
				t.Errorf("%s: render %q (%s) error:\n got %+v\nwant %+v", label, src, name, gotFields, want)
			}
		}
		return
	}
	if gotErr != nil || publicErr != nil {
		t.Errorf("%s: render %q: got errors %v / %v, want success", label, src, gotErr, publicErr)
		return
	}
	if lossless.SVG != want.SVG {
		t.Errorf("%s: render %q SVG (lossless):\n got %q\nwant %q", label, src, lossless.SVG, want.SVG)
	}
	if public.SVG != jsstr.WellFormed(want.SVG) {
		t.Errorf("%s: render %q SVG (public):\n got %s\nwant %s", label, src, public.SVG, jsstr.WellFormed(want.SVG))
	}
	if !sameNumber(public.Width, want.Width) || !sameNumber(public.Height, want.Height) {
		t.Errorf("%s: render %q size = %vx%v, want %vx%v", label, src, public.Width, public.Height, want.Width, want.Height)
	}
	if string(public.Type) != want.Type || public.MermaidType != want.MermaidType {
		t.Errorf("%s: render %q type = %s/%s, want %s/%s", label, src, public.Type, public.MermaidType, want.Type, want.MermaidType)
	}
	wantPublic := make([]oracle.Link, len(want.Links))
	for i, l := range want.Links {
		wantPublic[i] = oracle.Link{ID: jsstr.WellFormed(l.ID), Href: jsstr.WellFormed(l.Href), Internal: l.Internal}
	}
	if got := oracleLinks(lossless.Links); !slices.Equal(got, want.Links) {
		t.Errorf("%s: render %q links (lossless) = %+v, want %+v", label, src, got, want.Links)
	}
	if got := oracleLinks(public.Links); !slices.Equal(got, wantPublic) {
		t.Errorf("%s: render %q links (public) = %+v, want %+v", label, src, got, wantPublic)
	}
}

// loneSurrogateInputs carry lone surrogates into the output, directly or through YAML
// escapes. goja cannot judge them; V8 can.
var loneSurrogateInputs = []string{
	"info\ntitle a" + jsstr.FromUTF16([]uint16{0xd83d}),
	"info\ntitle " + jsstr.FromUTF16([]uint16{0xdf82}) + " low",
	"info\naccTitle: " + jsstr.FromUTF16([]uint16{0xdc00, 'x', 0xd800}),
	"info" + jsstr.FromUTF16([]uint16{0xdb78}),
	"---\ntitle: \"\\uD83D lone\"\n---\ninfo",
	"---\ntitle: \"\\uD83D\\uDE00 pair\"\n---\ninfo",
	"info\ntitle " + strings.Repeat("lone "+jsstr.FromUTF16([]uint16{0xdc01})+" ", 300),
}

// assertParityV8 compares the port with Pele in Node, which keeps lone surrogates where goja
// does not. measurer names a measurer the driver defines, or "" for the default.
func assertParityV8(t *testing.T, node *oracle.Node, src string, set optionSet, measurer string) {
	t.Helper()
	want, wantErr, err := node.Render(src, oracle.NodeOptions{
		Options: set.js, Measurer: measurer, Now: float64(fixedNow.UnixMilli()),
	})
	if err != nil {
		t.Fatalf("node render %q: %v", src, err)
	}
	var m pele.TextMeasurer
	if measurer == "length" {
		m = lengthMeasurer{}
	}
	compare(t, "v8/"+set.name+"/"+measurer, src, set.options(m), want, wantErr)
}

func oracleLinks(links []pele.Link) []oracle.Link {
	out := make([]oracle.Link, len(links))
	for i, l := range links {
		out[i] = oracle.Link{ID: l.ID, Href: l.Href, Internal: l.Internal}
	}
	return out
}

func outputHasLoneSurrogate(src string, opts pele.Options) bool {
	res, err := core.Render(context.Background(), src, opts)
	if pe := (*pele.Error)(nil); errors.As(err, &pe) {
		return hasLoneSurrogate(pe.Message) || hasLoneSurrogate(pe.Snippet)
	}
	return hasLoneSurrogate(res.SVG)
}

func hasLoneSurrogate(s string) bool {
	return jsstr.WellFormed(jsstr.Normalize(s)) != jsstr.Normalize(s)
}

// FuzzParityV8 fuzzes the type PELE_FUZZ_TYPE names against Pele in Node, which is slower
// than goja but judges lone surrogates correctly.
func FuzzParityV8(f *testing.F) {
	typ := fuzzType(f)
	fuzzSeeds(f, typ, v8Inputs[typ]...)
	nodes := map[string]*oracle.Node{}
	for _, zone := range zonesOf(typ) {
		nodes[zone] = startNodeIn(f, zone)
		f.Cleanup(func() { _ = nodes[zone].Close() })
	}
	f.Fuzz(func(t *testing.T, src string) {
		if got, ok := pele.DetectType(src); ok && string(got) != typ {
			t.Skipf("not a %s diagram", typ)
		}
		for _, zone := range zonesOf(typ) {
			assertParityV8(t, nodes[zone], src, optionSets[0].in(location(t, zone)), "")
		}
	})
}

// TestFontFamilyFollowsV8Unicode checks font names against V8's Unicode tables, which are
// newer than Go's: U+1C89 is a letter in Unicode 16 but not in Go's Unicode 15.
func TestFontFamilyFollowsV8Unicode(t *testing.T) {
	node := startNode(t)
	defer node.Close()
	for _, family := range []string{"\u1c89 Sans", "Noto \U0001E5D0", "Inter"} {
		want, wantErr, err := node.Render("info", oracle.NodeOptions{Options: map[string]any{"fontFamily": family}})
		if err != nil {
			t.Fatal(err)
		}
		compare(t, "v8/fontFamily", "info", pele.Options{FontFamily: ptr(family)}, want, wantErr)
	}
}

// sameNumber reports whether two sizes are the same JS number, NaN included.
func sameNumber(a, b float64) bool {
	return a == b || a != a && b != b
}
