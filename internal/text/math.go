package text

import (
	"math"

	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
)

// MathMarkup is a typeset $$...$$ expression. Markup is SVG, or HTML that needs the
// typesetter's CSS when HTML is set. Width and Height are in pixels.
type MathMarkup struct {
	Markup string
	HTML   bool
	Width  float64
	Height float64
}

// Math typesets TeX for one render. Upstream keeps the resolver and what it returned in
// module variables for the length of a render (withMath); here they are carried by Icons.
type Math struct {
	resolve func(tex string, size float64) *MathMarkup
	found   map[string]*MathMarkup
}

// NewMath returns the typesetter for a render. A nil resolve means the host has none.
func NewMath(resolve func(tex string, size float64) *MathMarkup) *Math {
	if resolve == nil {
		return nil
	}
	return &Math{resolve: resolve, found: map[string]*MathMarkup{}}
}

// WithMath returns icons that also typeset math with m.
func (ic *Icons) WithMath(m *Math) *Icons {
	if ic == nil {
		ic = NewIcons(nil)
	}
	out := *ic
	out.math = m
	return &out
}

// WithoutMath returns icons that leave $$ as written, as upstream's withoutMath does.
func (ic *Icons) WithoutMath() *Icons {
	if ic == nil || ic.math == nil {
		return ic
	}
	out := *ic
	out.math = nil
	return &out
}

// MathActive reports whether labels typeset math.
func (ic *Icons) MathActive() bool {
	return ic != nil && ic.math != nil
}

func validSize(size float64) bool {
	return !math.IsInf(size, 0) && !math.IsNaN(size) && size > 0
}

func (ic *Icons) typeset(tex string, size float64) *MathMarkup {
	if !ic.MathActive() {
		return nil
	}
	m := ic.math
	key := jsnum.Format(size) + "\n" + tex
	if markup, ok := m.found[key]; ok {
		return markup
	}
	markup := m.resolve(tex, size)
	if markup != nil && !(validSize(markup.Width) && validSize(markup.Height)) {
		markup = nil
	}
	m.found[key] = markup
	return markup
}
