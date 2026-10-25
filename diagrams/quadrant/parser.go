package quadrant

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
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
	wordTokens  = flags(tPunctuation, tAmp, tNum, tAlpha, tComma, tPlus, tEquals, tMult, tDot, tBrkt, tUnderscore, tUnicodeText)
	idTokens    = flags(tAlpha, tNum, tMinus, tComma, tColon, tAmp, tBrkt, tMult, tUnicodeText)
	styleTokens = flags(tAlpha, tNum, tColon, tSpace, tBrkt, tMinus)
	eolTokens   = flags(tNewline, tSemi, tEOF)
)

const textTokenNames = "'STR', 'MD_STR', 'ALPHA', 'NUM', 'UNICODE_TEXT', 'PUNCTUATION'"

func parseQuadrant(source string, db *quadrantDb, o *perr.Origin) {
	src := common.Src(jsstr.UTF16(source))
	toks := tokenize(src)
	types, texts, starts := toks.types, toks.texts, toks.starts
	i := 0

	fail := func(expected string) {
		t := types[i]
		if t == tError {
			perr.Throw(perr.SyntaxError(o, "quadrantChart", src, starts[i], "", true))
		}
		perr.Throw(perr.SyntaxError(o, "quadrantChart", src, starts[i], "Expecting "+expected+", got '"+tokenNames[t]+"'", false))
	}

	expect := func(typ int) string {
		if types[i] != typ {
			fail("'" + tokenNames[typ] + "'")
		}
		i++
		return texts[i-1]
	}

	isText := func(t int) bool { return t == tStr || t == tMDStr || wordTokens[t] }

	text := func() quadrantText {
		first := types[i]
		if !isText(first) {
			fail(textTokenNames)
		}
		s := texts[i]
		i++
		for wordTokens[types[i]] || types[i] == tSpace || types[i] == tMinus {
			s += texts[i]
			i++
		}
		return quadrantText{text: s, markdown: first == tMDStr}
	}

	styles := func() []string {
		var out []string
		for {
			if !styleTokens[types[i]] {
				fail("'ALPHA', 'NUM', 'COLON', 'SPACE', 'BRKT', 'MINUS'")
			}
			s := texts[i]
			i++
			for styleTokens[types[i]] {
				s += texts[i]
				i++
			}
			out = append(out, jsstr.Trim(s))
			if types[i] != tComma {
				return out
			}
			i++
		}
	}

	for eolTokens[types[i]] || types[i] == tSpace {
		i++
	}
	expect(tQuadrant)

	for types[i] != tEnd {
		for types[i] == tSpace {
			i++
		}
		t := types[i]
		switch t {
		case tClassDef:
			i++
			expect(tSpace)
			if !idTokens[types[i]] {
				fail("'ALPHA', 'NUM', 'UNICODE_TEXT'")
			}
			id := texts[i]
			i++
			for idTokens[types[i]] {
				id += texts[i]
				i++
			}
			expect(tSpace)
			db.addClass(id, styles())
		case tXAxis, tYAxis:
			i++
			from := text()
			var to *quadrantText
			if types[i] == tDelimiter {
				i++
				if isText(types[i]) {
					t := text()
					to = &t
				} else {
					from.text += " ⟶ "
				}
			}
			if t == tXAxis {
				db.setXAxisLeftText(from)
				if to != nil {
					db.setXAxisRightText(*to)
				}
			} else {
				db.setYAxisBottomText(from)
				if to != nil {
					db.setYAxisTopText(*to)
				}
			}
		case tQuadrant1, tQuadrant2, tQuadrant3, tQuadrant4:
			i++
			db.setQuadrantText(t-tQuadrant1, text())
		case tTitle:
			i++
			db.setDiagramTitle(jsstr.Trim(expect(tTitleValue)))
		case tAccTitle:
			i++
			db.setAccTitle(jsstr.Trim(expect(tAccTitleValue)))
		case tAccDescr:
			i++
			db.setAccDescription(jsstr.Trim(expect(tAccDescrValue)))
		case tAccDescrMultilineValue:
			db.setAccDescription(jsstr.Trim(texts[i]))
			i++
		default:
			if isText(t) {
				label := text()
				className := ""
				if types[i] == tClassName {
					className = texts[i]
					i++
				}
				expect(tPointStart)
				x := expect(tPointX)
				y := expect(tPointY)
				var pointStyles []string
				if styleTokens[types[i]] {
					pointStyles = styles()
				}
				db.addPoint(label, className, x, y, pointStyles)
			}
		}
		if !eolTokens[types[i]] {
			fail("'NEWLINE', 'SEMI', 'EOF'")
		}
		i++
	}
}
