package svg

import (
	"math"

	"github.com/Broderick-Westrope/pele-go/internal/text"
)

// TitleLabel lays a diagram title out. Mermaid draws diagram titles as text, whatever they
// hold, so math is not typeset.
func TitleLabel(title string, measurer text.Measurer, size, maxWidth float64, icons *text.Icons) text.Label {
	return text.Layout(title, false, measurer, size, maxWidth, text.Bold, icons.WithoutMath())
}

// Titled is content with a title put above it.
type Titled struct {
	Width, Height float64
	Content       string
}

// WithTitle puts a diagram title above content that was laid out without one. An empty
// title is upstream's undefined.
func WithTitle(title string, measurer text.Measurer, size, pad, width, height float64, content string, icons *text.Icons) Titled {
	label := TitleLabel(title, measurer, size, 4000, icons)
	if label.Height == 0 {
		return Titled{Width: width, Height: height, Content: content}
	}
	shift := label.Height + 12
	full := max(width, math.Ceil(label.Width+float64(2*pad)))
	return Titled{
		Width:  full,
		Height: math.Ceil(height + shift),
		Content: LabelSVG(label, full/2, pad+float64(label.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil) +
			`<g transform="translate(` + Num((full-width)/2) + `,` + Num(shift) + `)">` + content + `</g>`,
	}
}
