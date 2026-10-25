package xychart

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func flags(toks ...int) (out [tEOF + 1]bool) {
	for _, t := range toks {
		out[t] = true
	}
	return out
}

var (
	wordTokens = flags(tAmp, tNum, tAlpha, tPlus, tEquals, tMult, tDot, tBrkt, tMinus, tUnderscore)
	eolTokens  = flags(tNewline, tSemi, tEOF)
)

func parseXyChart(source string, db *xyChartDb, o *perr.Origin) {
	src := common.Src(jsstr.UTF16(source))
	toks := tokenize(src)
	types, texts, starts := toks.types, toks.texts, toks.starts
	i := 0

	fail := func(expected string) {
		t := types[i]
		if t == tError {
			perr.Throw(perr.SyntaxError(o, "xychart", src, starts[i], "", true))
		}
		perr.Throw(perr.SyntaxError(o, "xychart", src, starts[i], "Expecting "+expected+", got '"+tokenNames[t]+"'", false))
	}

	expect := func(typ int) string {
		if types[i] != typ {
			fail("'" + tokenNames[typ] + "'")
		}
		i++
		return texts[i-1]
	}

	isText := func(t int) bool { return t == tStr || wordTokens[t] }

	text := func() string {
		if !isText(types[i]) {
			fail("'STR', 'ALPHA', 'NUM'")
		}
		s := texts[i]
		i++
		if types[i-1] != tStr {
			for wordTokens[types[i]] {
				s += texts[i]
				i++
			}
		}
		return s
	}

	rangeOf := func() (float64, float64) {
		lo := jsnum.Parse(expect(tNumberWithDecimal))
		expect(tArrowDelimiter)
		return lo, jsnum.Parse(expect(tNumberWithDecimal))
	}

	points := func() []dataPoint {
		var out []dataPoint
		expect(tSquareBracesStart)
		for {
			value := jsnum.Parse(expect(tNumberWithDecimal))
			label := ""
			if types[i] == tStr {
				label = texts[i]
				i++
			}
			out = append(out, dataPoint{value: value, label: label})
			if types[i] != tComma {
				break
			}
			i++
		}
		expect(tSquareBracesEnd)
		return out
	}

	for {
		if eolTokens[types[i]] {
			i++
		} else if types[i] != tXYChart {
			break
		} else {
			i++
			if types[i] == tChartOrientation {
				db.setOrientation(texts[i])
				i++
			}
		}
	}

	for types[i] != tEnd {
		t := types[i]
		i++
		switch t {
		case tTitle:
			db.setDiagramTitle(jsstr.Trim(text()))
		case tXAxis, tYAxis:
			var title *string
			if isText(types[i]) {
				s := text()
				title = &s
			}
			if t == tXAxis && types[i] == tSquareBracesStart {
				var categories []string
				for {
					i++
					categories = append(categories, text())
					if types[i] != tComma {
						break
					}
				}
				expect(tSquareBracesEnd)
				db.setXAxisBand(categories)
			} else if types[i] == tNumberWithDecimal || title == nil {
				lo, hi := rangeOf()
				if t == tXAxis {
					db.setXAxisRangeData(lo, hi)
				} else {
					db.setYAxisRangeData(lo, hi)
				}
			}
			name := ""
			if title != nil {
				name = *title
			}
			if t == tXAxis {
				db.setXAxisTitle(name)
			} else {
				db.setYAxisTitle(name)
			}
		case tLine, tBar:
			title := ""
			if types[i] != tSquareBracesStart {
				title = text()
			}
			data := points()
			if t == tLine {
				db.setLineData(title, data)
			} else {
				db.setBarData(title, data)
			}
		case tAccTitle:
			db.setAccTitle(jsstr.Trim(expect(tAccTitleValue)))
		case tAccDescr:
			db.setAccDescription(jsstr.Trim(expect(tAccDescrValue)))
		case tAccDescrMultilineValue:
			db.setAccDescription(jsstr.Trim(texts[i-1]))
		default:
			i--
			fail("'title', 'X_AXIS', 'Y_AXIS', 'LINE', 'BAR', 'acc_title', 'acc_descr', 'acc_descr_multiline_value'")
		}
		for eolTokens[types[i]] {
			i++
		}
	}
}
