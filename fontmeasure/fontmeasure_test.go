package fontmeasure_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pele "github.com/Broderick-Westrope/pele-go"
	_ "github.com/Broderick-Westrope/pele-go/all"
	"github.com/Broderick-Westrope/pele-go/fontmeasure"
	"github.com/Broderick-Westrope/pele-go/internal/oracle"
)

type faces struct{ regular, bold, italic, boldItalic []byte }

func dejaVu(t testing.TB) faces {
	t.Helper()
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	return faces{
		regular:    read("DejaVuSans.ttf"),
		bold:       read("DejaVuSans-Bold.ttf"),
		italic:     read("DejaVuSans-Oblique.ttf"),
		boldItalic: read("DejaVuSans-BoldOblique.ttf"),
	}
}

func newMeasurer(t testing.TB, opts ...fontmeasure.Option) pele.TextMeasurer {
	t.Helper()
	f := dejaVu(t)
	m, err := fontmeasure.New(f.regular, f.bold, f.italic, f.boldItalic, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// The wants are what Anvil's internal/mermaid/measure.go returned for the same fonts.
func TestWidthMatchesAnvil(t *testing.T) {
	m := newMeasurer(t)
	tests := map[string]struct {
		text  string
		size  float64
		style pele.Style
		want  float64
	}{
		"regular":                  {text: "Hello, World", size: 16, want: 96.578125},
		"bold with kerning":        {text: "AVATAR Wave", size: 14, style: pele.Bold, want: 107.4814453125},
		"italic":                   {text: "italic text", size: 12, style: pele.Italic, want: 56.3671875},
		"bold italic":              {text: "Bold Italic", size: 18, style: pele.Bold | pele.Italic, want: 101.865234375},
		"mono without a mono face": {text: "mono bit", size: 16, style: pele.Mono, want: 71.265625},
		"missing glyphs":           {text: "日本語", size: 16, want: 30.421875},
		"empty":                    {text: "", size: 16, want: 0},
		"replacement character":    {text: "\uFFFD", size: 16, want: 10.140625},
		"astral":                   {text: "emoji 😀", size: 16, want: 59.3359375},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := m.Width(tc.text, tc.size, tc.style); got != tc.want {
				t.Errorf("Width(%q, %v, %d) = %v, want %v", tc.text, tc.size, tc.style, got, tc.want)
			}
		})
	}
}

// DejaVu has no kern table: its kerning is only in GPOS, which sfnt reads.
func TestWidthAppliesGPOSKerning(t *testing.T) {
	m := newMeasurer(t)
	pair, apart := m.Width("AV", 16, 0), m.Width("A", 16, 0)+m.Width("V", 16, 0)
	if pair >= apart {
		t.Fatalf("Width(AV) = %v, want less than Width(A)+Width(V) = %v", pair, apart)
	}
}

func TestWidthReadsSurrogatesAsJS(t *testing.T) {
	m := newMeasurer(t)
	tests := map[string]struct {
		text, same string
	}{
		"lone surrogate is one replacement character": {text: "a\xed\xa0\x80b", same: "a\uFFFDb"},
		"split pair is one character":                 {text: "a\xed\xa0\xbd\xed\xb8\x80b", same: "a😀b"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got, want := m.Width(tc.text, 16, 0), m.Width(tc.same, 16, 0); got != want {
				t.Errorf("Width(%q) = %v, want %v as for %q", tc.text, got, want, tc.same)
			}
		})
	}
}

func TestWithMono(t *testing.T) {
	f := dejaVu(t)
	// The bold faces stand in for a monospace family so the switch is visible.
	m := newMeasurer(t, fontmeasure.WithMono(f.bold, f.bold, f.boldItalic, f.boldItalic))
	plain := newMeasurer(t)
	tests := map[string]struct {
		style, sameAs pele.Style
	}{
		"mono":             {style: pele.Mono, sameAs: pele.Bold},
		"mono italic":      {style: pele.Mono | pele.Italic, sameAs: pele.Bold | pele.Italic},
		"no mono bit":      {style: pele.Italic, sameAs: pele.Italic},
		"mono bold italic": {style: pele.Mono | pele.Bold | pele.Italic, sameAs: pele.Bold | pele.Italic},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got, want := m.Width("Wide text", 16, tc.style), plain.Width("Wide text", 16, tc.sameAs); got != want {
				t.Errorf("Width = %v, want %v", got, want)
			}
		})
	}
}

func TestNewRejectsBadFonts(t *testing.T) {
	f := dejaVu(t)
	tests := map[string]struct {
		build func() (pele.TextMeasurer, error)
		want  string
	}{
		"nil face": {
			build: func() (pele.TextMeasurer, error) { return fontmeasure.New(f.regular, nil, f.italic, f.boldItalic) },
			want:  "parse bold face: ",
		},
		"not a font": {
			build: func() (pele.TextMeasurer, error) {
				return fontmeasure.New(f.regular, f.bold, []byte("not a font"), f.boldItalic)
			},
			want: "parse italic face: ",
		},
		"bad mono face": {
			build: func() (pele.TextMeasurer, error) {
				return fontmeasure.New(f.regular, f.bold, f.italic, f.boldItalic,
					fontmeasure.WithMono(f.regular, f.bold, f.italic, []byte{1, 2, 3}))
			},
			want: "parse mono bold italic face: ",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := tc.build()
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("New() = %v, %v; want error starting %q", m, err, tc.want)
			}
		})
	}
}

func TestConcurrentUse(t *testing.T) {
	m := newMeasurer(t)
	texts := []string{"Hello, World", "AVATAR Wave", "日本語", "emoji 😀", "Bold Italic"}
	want := make([]float64, len(texts))
	for i, s := range texts {
		want[i] = m.Width(s, 16, pele.Style(i%4))
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 200 {
				for i, s := range texts {
					if got := m.Width(s, 16, pele.Style(i%4)); got != want[i] {
						t.Errorf("Width(%q) = %v concurrently, want %v", s, got, want[i])
						return
					}
				}
			}
		})
	}
	wg.Wait()
}

type jsMeasurer struct{ m pele.TextMeasurer }

func (j jsMeasurer) Width(text string, size float64, style int) float64 {
	return j.m.Width(text, size, pele.Style(style))
}

// Rendering through upstream in goja and through the port with this measurer must give the
// same SVG, so the measurer is a drop-in for hosts that used one with upstream.
func TestParityWithUpstream(t *testing.T) {
	if testing.Short() {
		t.Skip("loads upstream into goja")
	}
	m := newMeasurer(t)
	o, err := oracle.New()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 20, 9, 30, 0, 0, time.UTC)
	sources := map[string]string{
		"flowchart": "graph TD\n  A[Start **bold**] --> B{Is it *italic*?}\n  B -->|Yes| C[`mono`]\n  B -->|No| D[日本語 😀]",
		"sequence":  "sequenceDiagram\n  Alice->>Bob: Hello AVATAR\n  Bob-->>Alice: Wave back",
		"class":     "classDiagram\n  class Animal {\n    +String name\n    +speak() void\n  }\n  Animal <|-- Duck",
		"pie":       "pie title Pets\n  \"Dogs\" : 386\n  \"Cats\" : 85",
		"mindmap":   "mindmap\n  root((Root))\n    Kerning AV\n    **Bold** child",
	}
	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			want, jsErr, err := o.Render(src, oracle.Options{Measurer: jsMeasurer{m}, Now: now})
			if err != nil || jsErr != nil {
				t.Fatalf("upstream render: %v %+v", err, jsErr)
			}
			got, err := pele.Render(context.Background(), src, pele.Options{Measurer: m, Now: now})
			if err != nil {
				t.Fatal(err)
			}
			if got.SVG != want.SVG || got.Width != want.Width || got.Height != want.Height {
				t.Errorf("SVG differs from upstream\n got: %s\nwant: %s", got.SVG, want.SVG)
			}
		})
	}
}

func TestWidthOfShiftedAndHyphenatedText(t *testing.T) {
	m := newMeasurer(t)
	tests := map[string]struct {
		text  string
		style pele.Style
		want  float64
	}{
		"sub is smaller":           {text: "x2", style: pele.Sub, want: m.Width("x2", 16/1.2, 0)},
		"sup is smaller":           {text: "x2", style: pele.Sup | pele.Bold, want: m.Width("x2", 16/1.2, pele.Bold)},
		"underline keeps width":    {text: "under", style: pele.Underline, want: m.Width("under", 16, 0)},
		"soft hyphen is invisible": {text: "co\u00adop", want: m.Width("coop", 16, 0)},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := m.Width(tc.text, 16, tc.style); got != tc.want {
				t.Errorf("Width(%q, 16, %d) = %v, want %v", tc.text, tc.style, got, tc.want)
			}
		})
	}
}
