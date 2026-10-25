// Package fontmeasure measures text with the advance widths and kerning of real fonts, for
// hosts that draw Pele's SVG with known fonts and want labels laid out to fit them.
//
// The caller supplies the fonts; none are embedded. Pass the measurer as
// pele.Options.Measurer:
//
//	m, err := fontmeasure.New(regular, bold, italic, boldItalic)
//	if err != nil { ... }
//	res, err := pele.Render(ctx, src, pele.Options{Measurer: m})
//
// This package depends on golang.org/x/image; the root pele package does not.
package fontmeasure

import (
	"fmt"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	pele "github.com/Broderick-Westrope/pele-go"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// smaller is the divisor for the pele.Sub and pele.Sup style bits, which the SVG draws at
// font-size="smaller". It is upstream's approximation.
const smaller = 1.2

// measurePPEM is large so that rounding advances to 26.6 fixed point is negligible; widths
// scale linearly from it.
const measurePPEM = 1024

// face is one parsed font and the glyph that stands in for characters it lacks.
type face struct {
	font     *sfnt.Font
	fallback sfnt.GlyphIndex
}

// measurer holds four faces per family, indexed by the Bold and Italic style bits.
type measurer struct {
	sans [4]face
	mono *[4]face
	bufs sync.Pool
}

// Option configures a measurer made by New.
type Option func(*options)

type options struct {
	mono [][]byte
}

// WithMono sets the faces used for text with the pele.Mono style bit, such as code spans.
// Without it, monospace text is measured with the faces given to New.
func WithMono(regular, bold, italic, boldItalic []byte) Option {
	return func(o *options) { o.mono = [][]byte{regular, bold, italic, boldItalic} }
}

var faceNames = [4]string{"regular", "bold", "italic", "bold italic"}

// New returns a measurer for the given TrueType or OpenType font files, one per style. The
// byte slices must not be modified afterwards. The measurer is safe for concurrent use.
//
// Width is the sum of the glyph advances plus kerning between adjacent glyphs, measured
// without hinting. Kerning is what golang.org/x/image/font/sfnt reads: pair adjustments of
// the GPOS kern feature for the latn script (or DFLT), else the legacy kern table. Other
// GPOS features, GSUB substitutions such as ligatures, and shaping are not applied, so
// widths can differ from a browser's for scripts that need them. Anvil's measure.go, which
// this generalises, used the same calls, so with the same fonts the widths are equal. A
// character the face has no glyph for is measured as 'n'. Lone surrogates in text are
// measured as U+FFFD, the character pele.Render writes in their place. A soft hyphen
// (U+00AD) measures zero, as a browser draws it where the line does not break there. Text
// with the pele.Sub or pele.Sup bit is measured at the size divided by 1.2.
func New(regular, bold, italic, boldItalic []byte, opts ...Option) (pele.TextMeasurer, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	m := &measurer{}
	m.bufs.New = func() any { return new(sfnt.Buffer) }
	var buf sfnt.Buffer
	if err := parseFamily(&m.sans, [][]byte{regular, bold, italic, boldItalic}, "", &buf); err != nil {
		return nil, err
	}
	if o.mono != nil {
		m.mono = new([4]face)
		if err := parseFamily(m.mono, o.mono, "mono ", &buf); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func parseFamily(dst *[4]face, data [][]byte, prefix string, buf *sfnt.Buffer) error {
	for i, d := range data {
		f, err := sfnt.Parse(d)
		if err != nil {
			return fmt.Errorf("parse %s%s face: %w", prefix, faceNames[i], err)
		}
		n, err := f.GlyphIndex(buf, 'n')
		if err != nil {
			return fmt.Errorf("parse %s%s face: %w", prefix, faceNames[i], err)
		}
		dst[i] = face{font: f, fallback: n}
	}
	return nil
}

func (m *measurer) Width(text string, size float64, style pele.Style) float64 {
	family := &m.sans
	if style&pele.Mono != 0 && m.mono != nil {
		family = m.mono
	}
	f := family[style&(pele.Bold|pele.Italic)]

	buf := m.bufs.Get().(*sfnt.Buffer)
	defer m.bufs.Put(buf)

	ppem := fixed.I(measurePPEM)
	var total fixed.Int26_6
	prev := sfnt.GlyphIndex(0)
	for _, r := range jsstr.WellFormed(text) {
		if r == 0xad {
			continue
		}
		g, err := f.font.GlyphIndex(buf, r)
		if err != nil || g == 0 {
			g = f.fallback
		}
		if prev != 0 {
			if k, err := f.font.Kern(buf, prev, g, ppem, font.HintingNone); err == nil {
				total += k
			}
		}
		if adv, err := f.font.GlyphAdvance(buf, g, ppem, font.HintingNone); err == nil {
			total += adv
		}
		prev = g
	}
	if style&(pele.Sub|pele.Sup) != 0 {
		size /= smaller
	}
	return float64(total) / 64 / measurePPEM * size
}
