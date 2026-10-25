package text

import (
	"strconv"
	"sync"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// Style is a set of label emphasis bits, as upstream's Style enum.
type Style int

// The Style bits.
const (
	Bold      Style = 1
	Italic    Style = 2
	Mono      Style = 4
	Sub       Style = 8
	Sup       Style = 16
	Underline Style = 32
)

// smaller is an approximate divisor for CSS font-size="smaller".
const smaller = 1.2

// Measurer measures the advance width of text in pixels. Implementations must be safe for
// concurrent use.
type Measurer interface {
	Width(text string, size float64, style Style) float64
}

func unpack(packed string) []uint16 {
	out := make([]uint16, 95)
	for i := range out {
		v, _ := strconv.ParseUint(packed[i*2:i*2+2], 36, 16)
		out[i] = uint16(v)
	}
	return out
}

var (
	regularTable = sync.OnceValue(func() []uint16 { return unpack(regularMetrics) })
	boldTable    = sync.OnceValue(func() []uint16 { return unpack(boldMetrics) })
)

func isWide(c rune) bool {
	return (c >= 0x1100 && c <= 0x115f) ||
		(c >= 0x2e80 && c <= 0xa4cf) ||
		(c >= 0xac00 && c <= 0xd7a3) ||
		(c >= 0xf900 && c <= 0xfaff) ||
		(c >= 0xfe30 && c <= 0xfe4f) ||
		(c >= 0xff00 && c <= 0xff60) ||
		(c >= 0xffe0 && c <= 0xffe6) ||
		(c >= 0xd800 && c <= 0xdbff)
}

func isZeroWidth(c rune) bool {
	return c == 0xad ||
		(c >= 0x300 && c <= 0x36f) ||
		(c >= 0x200b && c <= 0x200f) ||
		(c >= 0xdc00 && c <= 0xdfff) ||
		(c >= 0xfe00 && c <= 0xfe0f) ||
		c == 0xfeff
}

type metrics struct{}

// Metrics estimates widths from a table of Arial advances. It is upstream's
// metricsMeasurer, the measurer used where no canvas is available.
var Metrics Measurer = metrics{}

func (metrics) Width(text string, size float64, style Style) float64 {
	mono := style&Mono != 0
	eff := size
	if style&(Sub|Sup) != 0 {
		eff = size / smaller
	}
	table := regularTable()
	if style&Bold != 0 {
		table = boldTable()
	}
	units := 0
	add := func(c rune) {
		switch {
		case c < 32:
		case c < 127:
			if mono {
				units += 600
			} else {
				units += int(table[c-32])
			}
		case isZeroWidth(c):
		case isWide(c):
			if mono {
				units += 1200
			} else {
				units += 1000
			}
		default:
			if mono {
				units += 600
			} else {
				units += 580
			}
		}
	}
	// The table is read by UTF-16 code unit, so a character outside the BMP counts as its
	// high surrogate (wide) and its low surrogate (zero width).
	for i := 0; i < len(text); {
		r, n := jsstr.Decode(text, i)
		i += n
		if r >= 0x10000 {
			add(0xd800)
			add(0xdc00)
			continue
		}
		add(r)
	}
	return float64(units) * eff / 1000
}
