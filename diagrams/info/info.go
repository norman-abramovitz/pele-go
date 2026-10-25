// Package info is the info diagram type, which shows the version of Pele the port follows.
package info

import (
	"math"

	pele "github.com/Broderick-Westrope/pele-go"
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

// Diagram is the info diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "info",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		return buildInfo(parseInfo(source, limits.Origin), title)
	},
	Render: func(m any, _ *jsobj.Object, rc *core.Context) core.Rendered {
		return renderInfo(m.(model), rc)
	},
}

type model struct {
	// Empty strings stand for undefined: every use treats the two alike.
	title, accTitle, accDescr string
}

func buildInfo(a ast, title *string) model {
	var m model
	if a.title != nil && *a.title != "" {
		m.title = *a.title
	} else if title != nil {
		m.title = *title
	}
	if a.accTitle != nil && *a.accTitle != "" {
		m.accTitle = jsstr.TrimStart(*a.accTitle)
	}
	if a.accDescr != nil && *a.accDescr != "" {
		m.accDescr = common.CollapseIndentedBreaks(*a.accDescr)
	}
	return m
}

func renderInfo(m model, rc *core.Context) core.Rendered {
	opts := rc.Options
	size := 16.0
	if opts.FontSize != nil {
		size = *opts.FontSize
	}
	pad := 8.0
	if opts.Padding != nil {
		pad = *opts.Padding
	}
	measurer := rc.Measurer()

	title := text.Layout(m.title, false, measurer, size, 4000, text.Bold, rc.Icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 8
	}
	version := text.Layout("Pele v"+pele.UpstreamVersion, false, measurer, size, 4000, 0, rc.Icons)

	width := math.Ceil(max(title.Width, version.Width) + 2*pad)
	height := math.Ceil(titleHeight + version.Height + 2*pad)
	content := svg.LabelSVG(title, width/2, pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil) +
		svg.LabelSVG(version, width/2, pad+titleHeight+float64(version.Height/2), ` class="pele-version" fill="var(--_m)"`, nil)

	doc := svg.Document("info", width, height, size, svg.DocumentOptions{
		IDPrefix:       opts.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     opts.FontFamily,
		FontFamilyMono: opts.FontFamilyMono,
	}, svg.Accessible{AccTitle: m.accTitle, AccDescr: m.accDescr}, content)
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: []core.Link{}}
}
