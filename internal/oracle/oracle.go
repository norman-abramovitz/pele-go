// Package oracle runs upstream Pele, as published to npm, in goja. Parity tests compare the
// Go port's output with it.
package oracle

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/dop251/goja"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// Version is the upstream release the bundle was published as. It must equal
// pele.UpstreamVersion.
const Version = "1.9.0"

// bundleSHA256 is the checksum of dist/pele.global.js in the npm package pele@1.9.0.
const bundleSHA256 = "323c16de7844d4b609743f70449ebce041a531e4f48f23b1e9e7ec9d3b917287"

//go:embed testdata/pele.global.js
var bundle string

// Measurer is a text measurer to pass to Pele in place of its own.
type Measurer interface {
	Width(text string, size float64, style int) float64
}

// Options are the RenderOptions to pass. A nil Measurer leaves Pele's default.
type Options struct {
	Measurer Measurer
	Now      time.Time
	// Extra holds the other RenderOptions, by their JS names.
	Extra map[string]any
}

// Link is one entry of RenderResult.links.
type Link struct {
	ID       string
	Href     string
	Internal bool
}

// Result is a successful render. Strings are WTF-8, so lone surrogates survive.
type Result struct {
	SVG         string
	Width       float64
	Height      float64
	Links       []Link
	Type        string
	MermaidType string
}

// Error is a PeleError thrown by render. A null type is "".
type Error struct {
	Code        string
	Message     string
	Type        string
	MermaidType string
	Line        int
	Column      int
	Snippet     string
}

// Oracle is one goja runtime with Pele loaded. It is not safe for concurrent use.
type Oracle struct {
	vm         *goja.Runtime
	render     goja.Callable
	detectType goja.Callable
}

// New loads the bundle into a fresh runtime.
func New() (*Oracle, error) {
	sum := sha256.Sum256([]byte(bundle))
	if got := hex.EncodeToString(sum[:]); got != bundleSHA256 {
		return nil, fmt.Errorf("pele.global.js checksum %s, want %s", got, bundleSHA256)
	}
	vm := goja.New()
	if err := installURL(vm); err != nil {
		return nil, err
	}
	if _, err := vm.RunScript("pele.global.js", stickyBoundaries.Replace(bundle)); err != nil {
		return nil, fmt.Errorf("load pele: %w", err)
	}
	pele := vm.Get("pele").ToObject(vm)
	render, ok := goja.AssertFunction(pele.Get("render"))
	if !ok {
		return nil, errors.New("pele.render is not a function")
	}
	detectType, ok := goja.AssertFunction(pele.Get("detectType"))
	if !ok {
		return nil, errors.New("pele.detectType is not a function")
	}
	return &Oracle{vm: vm, render: render, detectType: detectType}, nil
}

func (o *Oracle) jsString(s string) goja.Value {
	return o.vm.ToValue(goja.StringFromUTF16(jsstr.UTF16(s)))
}

func goString(v goja.Value) string {
	s, ok := v.ToString().(goja.String)
	if !ok {
		return v.String()
	}
	units := make([]uint16, s.Length())
	for i := range units {
		units[i] = s.CharAt(i)
	}
	return jsstr.FromUTF16(units)
}

func optional(v goja.Value) string {
	if v == nil || goja.IsNull(v) || goja.IsUndefined(v) {
		return ""
	}
	return goString(v)
}

// DetectType returns pele.detectType(src), or "" for null. A thrown error is returned.
func (o *Oracle) DetectType(src string) (string, error) {
	v, err := o.detectType(goja.Undefined(), o.jsString(src))
	if err != nil {
		return "", err
	}
	return optional(v), nil
}

// Render calls pele.render. A PeleError comes back as *Error; anything else thrown is err.
func (o *Oracle) Render(src string, opts Options) (*Result, *Error, error) {
	js := o.vm.NewObject()
	for k, v := range opts.Extra {
		if err := js.Set(k, v); err != nil {
			return nil, nil, fmt.Errorf("set option %s: %w", k, err)
		}
	}
	if !opts.Now.IsZero() {
		if err := js.Set("now", float64(opts.Now.UnixMilli())); err != nil {
			return nil, nil, err
		}
	}
	if opts.Measurer != nil {
		m := o.vm.NewObject()
		width := func(call goja.FunctionCall) goja.Value {
			text := goString(call.Argument(0))
			return o.vm.ToValue(opts.Measurer.Width(text, call.Argument(1).ToFloat(), int(call.Argument(2).ToInteger())))
		}
		if err := m.Set("width", width); err != nil {
			return nil, nil, err
		}
		if err := js.Set("measurer", m); err != nil {
			return nil, nil, err
		}
	}

	v, err := o.render(goja.Undefined(), o.jsString(src), js)
	if err != nil {
		var ex *goja.Exception
		if errors.As(err, &ex) {
			if obj, ok := ex.Value().(*goja.Object); ok && optional(obj.Get("name")) == "PeleError" {
				return nil, &Error{
					Code:        optional(obj.Get("code")),
					Message:     optional(obj.Get("message")),
					Type:        optional(obj.Get("type")),
					MermaidType: optional(obj.Get("mermaidType")),
					Line:        int(obj.Get("line").ToInteger()),
					Column:      int(obj.Get("column").ToInteger()),
					Snippet:     optional(obj.Get("snippet")),
				}, nil
			}
		}
		return nil, nil, err
	}
	obj := v.ToObject(o.vm)
	res := &Result{
		SVG:         goString(obj.Get("svg")),
		Width:       obj.Get("width").ToFloat(),
		Height:      obj.Get("height").ToFloat(),
		Type:        optional(obj.Get("type")),
		MermaidType: optional(obj.Get("mermaidType")),
	}
	links := obj.Get("links").ToObject(o.vm)
	for i := range int(links.Get("length").ToInteger()) {
		l := links.Get(fmt.Sprint(i)).ToObject(o.vm)
		res.Links = append(res.Links, Link{
			ID:       goString(l.Get("id")),
			Href:     goString(l.Get("href")),
			Internal: l.Get("internal").ToBoolean(),
		})
	}
	return res, nil, nil
}
