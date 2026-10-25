package svg

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
)

// weights are the weights of bold text, each set only in a diagram that has such text.
var weights = [][2]string{
	{"--_tw", "title"},
	{"--_hw", "heading"},
	{"--_w", "bold"},
}

// Accessible is a diagram's accessible title and description.
type Accessible struct {
	AccTitle string
	AccDescr string
}

// DocumentOptions are the render options the root element reads.
type DocumentOptions struct {
	IDPrefix       *string
	Responsive     bool
	FontFamily     *string
	FontFamilyMono *string
}

// Document wraps content in the root svg element.
func Document(typ string, width, height, fontSize float64, opts DocumentOptions, acc Accessible, content string) string {
	prefix := "pele"
	if opts.IDPrefix != nil {
		prefix = *opts.IDPrefix
	}
	prefix = Esc(prefix)
	head, aria := "", ""
	if acc.AccTitle != "" {
		head += `<title id="` + prefix + `-title">` + EscText(acc.AccTitle) + `</title>`
		aria += ` aria-labelledby="` + prefix + `-title"`
	}
	if acc.AccDescr != "" {
		head += `<desc id="` + prefix + `-desc">` + EscText(acc.AccDescr) + `</desc>`
		aria += ` aria-describedby="` + prefix + `-desc"`
	}
	fit := "max-width:100%;height:auto;"
	if !opts.Responsive {
		fit = ""
	}
	w := ""
	if strings.Contains(content, "var(--_r)") {
		w = ";--_r:" + Radius
	}
	for _, pair := range weights {
		if strings.Contains(content, "var("+pair[0]+")") {
			w += ";" + pair[0] + ":var(--pele-" + pair[1] + "-weight,bold)"
		}
	}
	return `<svg xmlns="http://www.w3.org/2000/svg" class="pele pele-` + typ + `" viewBox="0 0 ` + jsnum.Format(width) + ` ` + jsnum.Format(height) +
		`" width="` + jsnum.Format(width) + `" height="` + jsnum.Format(height) + `" style="` + fit + RootStyle + w +
		Esc(FontStyle(fontSize, opts.FontFamily, opts.FontFamilyMono)) +
		`" fill="var(--_fg)" role="graphics-document document" aria-roledescription="` + typ + `"` + aria + `>` +
		head + content + `</svg>`
}
