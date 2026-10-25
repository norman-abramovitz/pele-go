package requirement

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// values are what the keyword tokens stand for, from REQUIREMENT to TRACES in token order.
var values = [...]string{
	requirementTypeRequirement,
	requirementTypeFunctionalRequirement,
	requirementTypeInterfaceRequirement,
	requirementTypePerformanceRequirement,
	requirementTypePhysicalRequirement,
	requirementTypeDesignConstraint,
	riskLow,
	riskMedium,
	riskHigh,
	verifyAnalysis,
	verifyDemonstration,
	verifyInspection,
	verifyTest,
	"",
	relContains,
	relCopies,
	relDerives,
	relSatisfies,
	relVerifies,
	relRefines,
	relTraces,
}

func isStylePart(t int) bool {
	return t == tAlpha || t == tColon || t == tBrkt || t == tMinus || t == tSemicolon
}

// parseRequirement panics with a *perr.Error on a syntax error.
func parseRequirement(source string, db *requirementDb, o *perr.Origin) {
	src := common.Src(jsstr.UTF16(source))
	tk := tokenize(src)
	types, texts, starts := tk.types, tk.texts, tk.starts
	i := 0
	var fields []int
	var vals []string

	// fail fails with Jison's wording, naming the token types from to to and any others as
	// expected.
	fail := func(from, to int, others ...int) {
		t := types[i]
		if t == tError {
			perr.Throw(perr.SyntaxError(o, "requirement", src, starts[i], "", true))
		}
		names := make([]string, 0, to-from+1+len(others))
		for k := from; k <= to; k++ {
			names = append(names, "'"+tokenNames[k]+"'")
		}
		for _, k := range others {
			names = append(names, "'"+tokenNames[k]+"'")
		}
		perr.Throw(perr.SyntaxError(o, "requirement", src, starts[i],
			"Expecting "+strings.Join(names, ", ")+", got '"+tokenNames[t]+"'", false))
	}

	expect := func(typ int) string {
		if types[i] != typ {
			fail(typ, typ)
		}
		i++
		return texts[i-1]
	}

	// keyword is the value of the next token if it is a keyword in the given range of token
	// types.
	keyword := func(from, to int) string {
		t := types[i]
		if t < from || t > to {
			fail(from, to)
		}
		i++
		return values[t-tRequirement]
	}

	str := func() string {
		t := types[i]
		if t != tUnqString && t != tQString {
			fail(tQString, tUnqString)
		}
		i++
		return texts[i-1]
	}

	directive := func() bool {
		switch types[i] {
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
			return false
		}
		return true
	}

	idList := func() []string {
		var list []string
		for {
			t := types[i]
			if t != tAlpha && t != tUnqString && t != tQString {
				fail(tQString, tUnqString, tAlpha)
			}
			list = append(list, texts[i])
			i++
			if types[i] != tComma {
				return list
			}
			i++
		}
	}

	stylesOpt := func() []string {
		var styles []string
		for {
			if !isStylePart(types[i]) {
				fail(tAlpha, tSemicolon, tMinus, tBrkt)
			}
			var style strings.Builder
			style.WriteString(texts[i])
			i++
			for isStylePart(types[i]) {
				style.WriteString(texts[i])
				i++
			}
			styles = append(styles, style.String())
			if types[i] != tComma {
				return styles
			}
			i++
		}
	}

	classList := func() []string {
		var classes []string
		switch types[i] {
		case tStyleSeparator:
			i++
			classes = idList()
			expect(tStructStart)
		case tStructStart:
			i++
		default:
			fail(tStructStart, tStructStart, tStyleSeparator)
		}
		expect(tNewline)
		return classes
	}

	// body reads a body up to its closing brace. The grammar is right recursive, so Mermaid
	// applies the fields last to first and the first of two repeated fields wins.
	body := func(isRequirement bool) {
		fields = fields[:0]
		vals = vals[:0]
		for {
			t := types[i]
			if t == tStructStop {
				i++
				break
			}
			if t == tNewline {
				i++
				continue
			}
			if isRequirement {
				if t < tID || t > tVerifyMthd {
					fail(tID, tVerifyMthd, tNewline, tStructStop)
				}
			} else if t != tType && t != tDocRef {
				fail(tType, tDocRef, tNewline, tStructStop)
			}
			i++
			expect(tColonSep)
			var value string
			switch t {
			case tRisk:
				value = keyword(tLowRisk, tHighRisk)
			case tVerifyMthd:
				value = keyword(tVerifyAnalysis, tVerifyTest)
			default:
				value = str()
			}
			vals = append(vals, value)
			fields = append(fields, t)
			expect(tNewline)
		}
		for k := len(fields) - 1; k >= 0; k-- {
			value := vals[k]
			switch fields[k] {
			case tID:
				db.setNewReqID(value)
			case tText:
				db.setNewReqText(value)
			case tRisk:
				db.setNewReqRisk(value)
			case tVerifyMthd:
				db.setNewReqVerifyMethod(value)
			case tType:
				db.setNewElementType(value)
			default:
				db.setNewElementDocRef(value)
			}
		}
	}

	for directive() {
		if types[i] == tNewline {
			i++
		}
	}
	if types[i] != tRD {
		fail(tRD, tRD, tAccTitle, tAccDescr, tAccDescrMultilineValue)
	}
	i++
	expect(tNewline)

	for {
		t := types[i]
		switch {
		case t == tEOF:
			return
		case t == tNewline:
			i++
		case directive():
		case t >= tDirectionTB && t <= tDirectionLR:
			i++
			db.setDirection([...]string{"TB", "BT", "RL", "LR"}[t-tDirectionTB])
		case t >= tRequirement && t <= tDesignConstraint:
			typ := keyword(tRequirement, tDesignConstraint)
			name := str()
			classes := classList()
			body(true)
			db.addRequirement(name, typ)
			if classes != nil {
				db.setClass([]string{name}, classes)
			}
		case t == tElement:
			i++
			name := str()
			classes := classList()
			body(false)
			db.addElement(name)
			if classes != nil {
				db.setClass([]string{name}, classes)
			}
		case t == tStyle:
			i++
			ids := idList()
			db.setCssStyle(ids, stylesOpt())
		case t == tClassDef:
			i++
			ids := idList()
			db.defineClass(ids, stylesOpt())
		case t == tClass:
			i++
			ids := idList()
			db.setClass(ids, idList())
		case t == tUnqString || t == tQString:
			id := texts[i]
			i++
			next := types[i]
			switch next {
			case tStyleSeparator:
				i++
				db.setClass([]string{id}, idList())
			case tEndArrowL, tLine:
				i++
				relationship := keyword(tContains, tTraces)
				if next == tLine {
					expect(tEndArrowR)
				} else {
					expect(tLine)
				}
				other := str()
				if next == tLine {
					db.addRelationship(relationship, id, other)
				} else {
					db.addRelationship(relationship, other, id)
				}
			default:
				fail(tStyleSeparator, tStyleSeparator, tEndArrowL, tLine)
			}
		default:
			fail(tNewline, tEOF, tAccTitle, tAccDescr, tAccDescrMultilineValue, tDirectionTB, tDirectionBT, tDirectionRL,
				tDirectionLR, tRequirement, tFunctionalRequirement, tInterfaceRequirement, tPerformanceRequirement,
				tPhysicalRequirement, tDesignConstraint, tElement, tClassDef, tClass, tStyle, tUnqString, tQString)
		}
	}
}
