package common_test

import (
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/fixture"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

func src(s string) common.Src { return common.Src(jsstr.UTF16(s)) }

func ints(list []int) []any {
	out := make([]any, len(list))
	for i, v := range list {
		out[i] = float64(v)
	}
	return out
}

func textTokens(t common.TextTokens) map[string]any {
	texts := make([]any, len(t.Texts))
	for i, s := range t.Texts {
		texts[i] = s
	}
	return map[string]any{"types": ints(t.Types), "texts": texts, "starts": ints(t.Starts)}
}

func labelJSON(l text.Label) map[string]any {
	lines := []any{}
	for _, line := range l.Lines {
		spans := []any{}
		for _, s := range line {
			span := map[string]any{"text": s.Text, "style": float64(s.Style), "width": s.Width}
			if s.Icon != "" {
				span["icon"] = s.Icon
			}
			spans = append(spans, span)
		}
		lines = append(lines, spans)
	}
	widths := []any{}
	for _, w := range l.Widths {
		widths = append(widths, w)
	}
	return map[string]any{"lines": lines, "widths": widths, "width": l.Width, "height": l.Height, "size": l.Size, "lineHeight": l.LineHeight}
}

// recorder is an outline DB that records the calls the parser makes.
type recorder struct{ calls []any }

func (r *recorder) GetType(start, end string) int {
	r.calls = append(r.calls, []any{"getType", start, end})
	return common.GetType(start, end)
}

func (r *recorder) AddNode(level int, id, descr string, typ int, shapeData *string) {
	call := []any{"addNode", float64(level), id, descr, float64(typ)}
	if shapeData != nil {
		call = append(call, *shapeData)
	}
	r.calls = append(r.calls, call)
}

func (r *recorder) DecorateNode(d common.Decoration) {
	m := map[string]any{}
	if d.Icon != nil {
		m["icon"] = *d.Icon
	}
	if d.Class != nil {
		m["class"] = *d.Class
	}
	r.calls = append(r.calls, []any{"decorateNode", m})
}

func options(o *jsobj.Object) core.Options {
	var opts core.Options
	get := func(key string) *float64 {
		if v, ok := o.Get(key); ok {
			f := v.(float64)
			return &f
		}
		return nil
	}
	opts.MaxWidth, opts.Padding, opts.DirectionBreakpoint = get("maxWidth"), get("padding"), get("directionBreakpoint")
	if v, ok := o.Get("autoDirection"); ok {
		b := v.(bool)
		opts.AutoDirection = &b
	}
	return opts
}

// draw is the generator's synthetic chart: as wide as its plot scaled, plus extra when flag
// is set, plus 40.
func draw(scale, extra float64) func(plot float64, flag bool) core.Rendered {
	return func(plot float64, flag bool) core.Rendered {
		e := 0.0
		if flag {
			e = extra
		}
		flagText := "false"
		if flag {
			flagText = "true"
		}
		return core.Rendered{SVG: jsnum.Format(plot) + "|" + flagText, Width: float64(plot*scale) + e + 40, Height: 1}
	}
}

func rendered(r core.Rendered) map[string]any {
	return map[string]any{"svg": r.SVG, "width": r.Width, "height": r.Height, "links": []any{}}
}

func word() *common.TokenType {
	return &common.TokenType{Name: "WORD", First: "abcdefghijklmnopqrstuvwxyz", Match: func(s common.Src, at int) int {
		e := at
		for e < len(s) && s[e] >= 'a' && s[e] <= 'z' {
			e++
		}
		if e == at {
			return -1
		}
		return e
	}}
}

// TestCases replays upstream's results from testdata/cases.json (scripts/gen-shared-cases.mjs).
func TestCases(t *testing.T) {
	lexer := common.NewLexer([]*common.TokenType{
		common.AccDescrScanner("DESCR"), common.AccTitle, common.Title, common.String, common.Newline, common.Whitespace,
		common.Yaml, common.Directive, common.SingleLineComment, common.Keyword("end"), word(),
	})
	fixture.Run(t, "testdata/cases.json", map[string]fixture.Func{
		"tokenize": func(a fixture.Args) any {
			r := lexer.Tokenize(src(a.Str(0)), "test", nil)
			return map[string]any{"kinds": ints(r.Kinds), "starts": ints(r.Starts), "ends": ints(r.Ends)}
		},
		"stringValue": func(a fixture.Args) any { return common.StringValue(a.Str(0)) },
		"wordAt":      func(a fixture.Args) any { return common.WordAt(src(a.Str(0)), int(a.Num(1)), a.Str(2)) },
		"skipTrivia":  func(a fixture.Args) any { return float64(common.SkipTrivia(src(a.Str(0)), int(a.Num(1)))) },
		"lineEnd":     func(a fixture.Args) any { return float64(common.LineEnd(src(a.Str(0)), int(a.Num(1)))) },
		"keywordAt":   func(a fixture.Args) any { return common.KeywordAt(src(a.Str(0)), int(a.Num(1)), a.Str(2)) },
		"accessibility": func(a fixture.Args) any {
			var out common.TextTokens
			end := common.Accessibility(src(a.Str(0)), int(a.Num(1)), &out, 10)
			return map[string]any{"end": float64(end), "out": textTokens(out)}
		},
		"tokenizeOutline": func(a fixture.Args) any { return textTokens(common.TokenizeOutline(src(a.Str(0)), a.Bool(1))) },
		"parseOutline": func(a fixture.Args) any {
			var r recorder
			common.ParseOutline(src(a.Str(0)), &r, a.Bool(1), nil)
			return r.calls
		},
		"parseGenericTypes": func(a fixture.Args) any { return common.ParseGenericTypes(a.Str(0)) },
		"iconSvg": func(a fixture.Args) any {
			icons := text.NewIcons(func(name string) string {
				if name == "car" {
					return `<path d="M0 0"/>`
				}
				return ""
			})
			return common.IconSVG(a.Str(0), a.Num(1), a.Num(2), a.Num(3), icons)
		},
		"plainLabel": func(a fixture.Args) any {
			return labelJSON(common.PlainLabel(a.Str(0), text.Metrics, a.Num(1), a.Num(2), text.Style(a.Num(3))))
		},
		"fitLabel": func(a fixture.Args) any {
			return labelJSON(common.FitLabel(a.Str(0), text.Metrics, a.Num(1), a.Num(2), text.Style(a.Num(3))))
		},
		"fitText": func(a fixture.Args) any {
			f, ok := common.FitText(a.Str(0), a.Num(1), a.Num(2), a.Num(3), text.Metrics, text.Style(a.Num(4)))
			if !ok {
				return nil
			}
			return map[string]any{"text": f.Text, "size": f.Size, "width": f.Width}
		},
		"fitWidth": func(a fixture.Args) any {
			return rendered(common.FitWidth(options(a.Obj(0)), a.Num(1), a.Num(2), draw(a.Num(3), a.Num(4))))
		},
		"titleRoom": func(a fixture.Args) any { return common.TitleRoom(options(a.Obj(0))) },
		"capWidth":  func(a fixture.Args) any { return common.CapWidth(options(a.Obj(0)), a.Num(1), a.Num(2), a.Num(3)) },
		"turnToFit": func(a fixture.Args) any {
			d := draw(a.Num(2), a.Num(3))
			return rendered(common.TurnToFit(options(a.Obj(0)), func(down bool) core.Rendered { return d(a.Num(1), down) }))
		},
		"tighten": func(a fixture.Args) any {
			d := draw(a.Num(2), a.Num(3))
			return rendered(common.Tighten(options(a.Obj(0)), func(tight bool) core.Rendered { return d(a.Num(1), tight) }))
		},
		"runsAcross": func(a fixture.Args) any { return common.RunsAcross(a.Str(0)) },
		"literalLabel": func(a fixture.Args) any {
			return labelJSON(common.LiteralLabel(a.Str(0), text.Metrics, a.Num(1), a.Num(2), text.Style(a.Num(3))))
		},
		"breakLabel": func(a fixture.Args) any {
			l := text.Layout(a.Str(0), true, text.Metrics, a.Num(1), a.Num(2), 0, nil)
			return labelJSON(common.BreakLabel(l, text.Metrics, a.Num(3)))
		},
		"leftLabelSvg": func(a fixture.Args) any {
			l := text.Layout(a.Str(0), true, text.Metrics, a.Num(1), a.Num(2), 0, nil)
			return common.LeftLabelSVG(l, a.Num(3), a.Num(4), ` class="l"`)
		},
	})
}

func TestCollapseIndentedBreaks(t *testing.T) {
	tests := map[string]struct {
		in, want string
	}{
		"indented lines":       {in: "a\n   b\n\tc", want: "a\nb\nc"},
		"blank lines collapse": {in: "a\n\n\n  b", want: "a\nb"},
		"unicode space":        {in: "a\n\u3000\u2028b", want: "a\nb"},
		"trailing break":       {in: "a\n", want: "a\n"},
		"no break":             {in: "  a  ", want: "  a  "},
		"lone surrogate kept":  {in: "a\n\xed\xa0\x80", want: "a\n\xed\xa0\x80"},
		"space before break":   {in: "a  \n b", want: "a  \nb"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := common.CollapseIndentedBreaks(tc.in); got != tc.want {
				t.Errorf("CollapseIndentedBreaks(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
