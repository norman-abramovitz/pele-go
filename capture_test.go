package pele_test

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	pele "github.com/Broderick-Westrope/pele-go"
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsjson"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// TestCapturedParity replays every public parse and render call upstream's own test suite
// made (captured under V8 by scripts/capture-cases.mjs) and checks the port returns what
// upstream did. Each registered type's shard is replayed, and the shard of inputs no type
// was detected in. It runs in parallel with other tests, as it sets no global state.
func TestCapturedParity(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(shardPath("*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		shard := strings.TrimSuffix(filepath.Base(file), ".jsonl.gz")
		t.Run(shard, func(t *testing.T) {
			if shard != "none" && !core.Registered(shard) {
				t.Skip("not registered")
			}
			t.Parallel()
			records := readShard(t, shard)
			if len(records) == 0 {
				t.Fatal("no captured calls")
			}
			for _, rec := range records {
				replay(t, rec)
			}
		})
	}
}

func shardPath(name string) string {
	return filepath.Join("internal", "oracle", "testdata", "captured", name+".jsonl.gz")
}

func readShard(t testing.TB, name string) []*jsobj.Object {
	t.Helper()
	f, err := os.Open(shardPath(name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var records []*jsobj.Object
	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for scanner.Scan() {
		// jsjson keeps lone surrogates, which encoding/json would replace.
		v, err := jsjson.Parse(scanner.Text())
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, v.(*jsobj.Object))
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}

func get[T any](o *jsobj.Object, key string) T {
	v, _ := o.Get(key)
	t, _ := v.(T)
	return t
}

func replay(t *testing.T, rec *jsobj.Object) {
	t.Helper()
	ctx := context.Background()
	want := get[string](rec, "outcome")
	switch call := get[string](rec, "call"); call {
	case "parse":
		src := get[string](rec, "src")
		_, err := core.Parse(ctx, src, capturedOptions(t, rec))
		got := "ok"
		if err != nil {
			got = outcomeHash(t, core.Result{}, err)
		}
		if got != want {
			t.Errorf("parse %q: outcome differs from upstream (%v)", src, err)
		}
	case "render":
		src := get[string](rec, "src")
		opts := capturedOptions(t, rec)
		res, err := core.Render(ctx, src, opts)
		if got := outcomeHash(t, res, err); got != want {
			t.Errorf("render %q with %s: outcome differs from upstream (error %v)", src, describeOptions(rec), err)
		}
		checkPublic(t, src, opts, res, err)
	case "render-model":
		parse := get[*jsobj.Object](rec, "parse")
		src := get[string](parse, "src")
		parsed, err := core.Parse(ctx, src, capturedOptions(t, parse))
		var res core.Result
		if err == nil {
			res, err = core.RenderParsed(ctx, parsed, capturedOptions(t, rec))
		}
		if got := outcomeHash(t, res, err); got != want {
			t.Errorf("render(parse(%q)) with %s: outcome differs from upstream (error %v)", src, describeOptions(rec), err)
		}
	default:
		t.Fatalf("unknown captured call %q", call)
	}
}

// checkPublic checks pele.Render gives the well-formed form of the lossless result.
func checkPublic(t *testing.T, src string, opts pele.Options, lossless core.Result, losslessErr error) {
	t.Helper()
	public, err := pele.Render(context.Background(), src, opts)
	if losslessErr != nil {
		var want, got *pele.Error
		if !errors.As(losslessErr, &want) || !errors.As(err, &got) ||
			got.Message != jsstr.WellFormed(want.Message) || got.Snippet != jsstr.WellFormed(want.Snippet) || got.Code != want.Code {
			t.Errorf("render %q: public error %v does not match lossless %v", src, err, losslessErr)
		}
		return
	}
	if err != nil || public.SVG != jsstr.WellFormed(lossless.SVG) || len(public.Links) != len(lossless.Links) {
		t.Errorf("render %q: public result does not match lossless (error %v)", src, err)
		return
	}
	for i, l := range lossless.Links {
		if public.Links[i].ID != jsstr.WellFormed(l.ID) || public.Links[i].Href != jsstr.WellFormed(l.Href) {
			t.Errorf("render %q: public link %d does not match lossless", src, i)
		}
	}
}

func describeOptions(rec *jsobj.Object) string {
	opts := get[*jsobj.Object](rec, "options")
	return "options " + strings.Join(opts.Keys(), ",") + " measurer " + get[string](rec, "measurer")
}

// number reads a captured number, which non-finite values reach as {"$number": "..."}.
func number(v any) float64 {
	if o, ok := v.(*jsobj.Object); ok {
		return jsnum.Parse(get[string](o, "$number"))
	}
	return v.(float64)
}

// numberOrNaN reads a captured number, or gives NaN for a value that is not one.
func numberOrNaN(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case *jsobj.Object:
		if _, ok := x.Get("$number"); ok {
			return number(x)
		}
	}
	return math.NaN()
}

// capturedNow is the clock a call with an invalid now read during the capture, which
// scripts/capture/capture-setup.ts stops at its FIXED_NOW.
var capturedNow = time.Date(2026, time.October, 20, 9, 30, 0, 0, time.UTC)

// captureZone is the time zone the captured calls ran in, which gantt's dates depend on.
const captureZone = "Australia/Sydney"

// numbers decodes the non-finite numbers nested in a captured value, as number does for one.
func numbers(v any) any {
	switch x := v.(type) {
	case *jsobj.Object:
		if _, ok := x.Get("$number"); ok && x.Len() == 1 {
			return number(x)
		}
		out := jsobj.New()
		for _, k := range x.Keys() {
			e, _ := x.Get(k)
			out.Set(k, numbers(e))
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = numbers(e)
		}
		return out
	}
	return v
}

func capturedOptions(t *testing.T, rec *jsobj.Object) pele.Options {
	t.Helper()
	opts := pele.Options{Location: location(t, captureZone)}
	switch m := get[string](rec, "measurer"); m {
	case "default", "metrics", "":
		// Without a canvas upstream's default is the metrics measurer, and so is the port's.
	default:
		t.Fatalf("captured measurer %q cannot be replayed", m)
	}
	if icons := get[*jsobj.Object](rec, "icons"); icons.Len() > 0 {
		opts.Icons = func(name string) string {
			markup, _ := icons.Get(name)
			s, _ := markup.(string)
			return s
		}
	}
	if _, ok := rec.Get("math"); ok {
		answers := get[*jsobj.Object](rec, "math")
		opts.Math = func(tex string, size float64) *pele.MathMarkup {
			answer, _ := answers.Get(jsnum.Format(size) + "\n" + tex)
			o, ok := answer.(*jsobj.Object)
			if !ok {
				return nil
			}
			markup := &pele.MathMarkup{}
			if v, ok := o.Get("svg"); ok {
				if markup.Markup, ok = v.(string); !ok {
					return nil
				}
			} else if v, ok := o.Get("html"); ok {
				if markup.Markup, ok = v.(string); !ok {
					return nil
				}
				markup.HTML = true
			} else {
				return nil
			}
			width, _ := o.Get("width")
			height, _ := o.Get("height")
			// Upstream drops markup whose size is not a finite number; NaN stands in for a
			// size that is not a number at all.
			markup.Width, markup.Height = numberOrNaN(width), numberOrNaN(height)
			return markup
		}
	}
	raw := get[*jsobj.Object](rec, "options")
	for _, key := range raw.Keys() {
		v, _ := raw.Get(key)
		switch key {
		case "now":
			if v == nil {
				// NaN or an Invalid Date, which upstream replaces with Date.now() as the
				// capture ran.
				opts.Now = capturedNow
				continue
			}
			opts.Now = time.UnixMilli(int64(number(v)))
		case "fontFamily":
			opts.FontFamily = ptr(v.(string))
		case "fontFamilyMono":
			opts.FontFamilyMono = ptr(v.(string))
		case "fontSize":
			opts.FontSize = ptr(number(v))
		case "idPrefix":
			opts.IDPrefix = ptr(v.(string))
		case "responsive":
			opts.Responsive = ptr(v.(bool))
		case "maxWidth":
			opts.MaxWidth = ptr(number(v))
		case "autoDirection":
			opts.AutoDirection = ptr(v.(bool))
		case "directionBreakpoint":
			opts.DirectionBreakpoint = ptr(number(v))
		case "padding":
			opts.Padding = ptr(number(v))
		case "limit":
			opts.Limit = ptr(number(v))
		case "outputLimit":
			opts.OutputLimit = ptr(number(v))
		case "maxEdges":
			opts.MaxEdges = ptr(number(v))
		case "links":
			opts.Links = ptr(v.(bool))
		case "images":
			opts.Images = ptr(v.(bool))
		case "linkRel":
			opts.LinkRel = ptr(v.(string))
		case "linkSchemes", "imageSchemes":
			list := []string{}
			for _, s := range v.([]any) {
				list = append(list, s.(string))
			}
			if key == "linkSchemes" {
				opts.LinkSchemes = list
			} else {
				opts.ImageSchemes = list
			}
		case "config":
			opts.Config = numbers(v).(*jsobj.Object)
		default:
			t.Fatalf("captured option %q has no Go equivalent", key)
		}
	}
	return opts
}

// outcomeHash writes the outcome the way scripts/capture/capture-setup.ts does, as
// JSON.stringify would, and hashes it.
func outcomeHash(t *testing.T, res core.Result, err error) string {
	t.Helper()
	var b strings.Builder
	if err != nil {
		var pe *pele.Error
		if !errors.As(err, &pe) {
			t.Fatalf("render returned %v, not a *pele.Error", err)
		}
		b.WriteString(`{"error":{"code":` + strconv.Quote(string(pe.Code)) +
			`,"message":` + unitsJSON(pe.Message) +
			`,"type":` + nullable(pe.Type) +
			`,"mermaidType":` + nullable(pe.MermaidType) +
			`,"line":` + jsnum.Format(float64(pe.Line)) +
			`,"column":` + jsnum.Format(float64(pe.Column)) +
			`,"snippet":` + unitsJSON(pe.Snippet) + `}}`)
	} else {
		b.WriteString(`{"svg":` + unitsJSON(res.SVG) +
			`,"width":` + jsonNumber(res.Width) +
			`,"height":` + jsonNumber(res.Height) + `,"links":[`)
		for i, l := range res.Links {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`{"id":` + unitsJSON(l.ID) + `,"href":` + unitsJSON(l.Href) + `,"internal":` + strconv.FormatBool(l.Internal) + `}`)
		}
		b.WriteString(`],"type":` + strconv.Quote(res.Type) + `,"mermaidType":` + strconv.Quote(res.MermaidType) + `}`)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func unitsJSON(s string) string {
	units := jsstr.UTF16(s)
	parts := make([]string, len(units))
	for i, u := range units {
		parts[i] = strconv.Itoa(int(u))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func nullable(s string) string {
	if s == "" {
		return "null"
	}
	return strconv.Quote(s)
}

// jsonNumber is JSON.stringify for a number: null for non-finite values.
func jsonNumber(v float64) string {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return "null"
	}
	return jsnum.Format(v)
}

// An empty scheme list allows no schemes, where a missing one allows the defaults, so the
// replay must not turn [] into nil.
func TestCapturedOptionsKeepEmptySchemeLists(t *testing.T) {
	rec, err := jsjson.Parse(`{"measurer":"default","options":{"linkSchemes":[],"imageSchemes":[]}}`)
	if err != nil {
		t.Fatal(err)
	}
	opts := capturedOptions(t, rec.(*jsobj.Object))
	if opts.LinkSchemes == nil || len(opts.LinkSchemes) != 0 || opts.ImageSchemes == nil || len(opts.ImageSchemes) != 0 {
		t.Fatalf("schemes = %#v / %#v, want empty non-nil lists", opts.LinkSchemes, opts.ImageSchemes)
	}
}
