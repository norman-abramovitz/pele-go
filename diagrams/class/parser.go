package class

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func isAlphaNum(t int) bool {
	return t == tUnicodeText || t == tAlpha || t == tMinus
}

func isStylePart(t int) bool {
	switch t {
	case tAlpha, tColon, tBrkt, tStyle, tPct, tLabel:
		return true
	}
	return false
}

// endType is the relation end a token marks, or relationNone.
func endType(t int) int {
	switch t {
	case tAggregation:
		return relationAggregation
	case tExtension:
		return relationExtension
	case tComposition:
		return relationComposition
	case tDependency:
		return relationDependency
	case tLollipop:
		return relationLollipop
	}
	return relationNone
}

const nameTokens = "'UNICODE_TEXT', 'NUM', 'ALPHA', 'MINUS', 'BQUOTE_STR'"

type frame struct {
	id      string
	classes []string
	notes   []string
}

func parseClassDiagram(source string, db *classDb, o *perr.Origin) {
	src := common.Src(jsstr.UTF16(source))
	toks := tokenize(src)
	types, texts, starts := toks.types, toks.texts, toks.starts
	i := 0
	kind := func(k int) int {
		if k < len(types) {
			return types[k]
		}
		return -1
	}

	fail := func(expected string) {
		t := types[i]
		if t == tError {
			perr.Throw(perr.SyntaxError(o, "class", src, starts[i], "", true))
		}
		perr.Throw(perr.SyntaxError(o, "class", src, starts[i], "Expecting "+expected+", got '"+tokenNames[t]+"'", false))
	}

	expect := func(typ int) string {
		if types[i] != typ {
			fail("'" + tokenNames[typ] + "'")
		}
		i++
		return texts[i-1]
	}

	take := func() string {
		i++
		return texts[i-1]
	}

	alphaNum := func() string {
		if !isAlphaNum(types[i]) {
			fail("'UNICODE_TEXT', 'NUM', 'ALPHA', 'MINUS'")
		}
		return take()
	}

	name := func(generic bool) string {
		out := ""
		for {
			if types[i] == tBquoteStr {
				out += take()
				break
			}
			if !isAlphaNum(types[i]) {
				fail(nameTokens)
			}
			out += take()
			t := types[i]
			if t == tDot {
				out += "."
				i++
			} else if !isAlphaNum(t) && t != tBquoteStr {
				break
			}
		}
		if generic && types[i] == tGenerictype {
			out += "~" + take() + "~"
		}
		return out
	}

	classLabel := func() string {
		i++
		label := expect(tStr)
		expect(tSqe)
		return label
	}

	// members returns the members first to last; the grammar collects them last to first,
	// and addMembers turns them back.
	members := func() []string {
		if types[i] != tMember {
			fail("'STRUCT_STOP', 'MEMBER'")
		}
		var list []string
		for types[i] == tMember {
			list = append(list, take())
		}
		expect(tStructStop)
		for l, r := 0, len(list)-1; l < r; l, r = l+1, r-1 {
			list[l], list[r] = list[r], list[l]
		}
		return list
	}

	classStatement := func() string {
		i++
		id := name(true)
		db.addClass(id)
		if types[i] == tSqs {
			db.setClassLabel(id, classLabel())
		}
		switch types[i] {
		case tStyleSeparator:
			i++
			css := alphaNum()
			db.setCSSClass(id, css)
			if types[i] == tStructStart {
				i++
				db.addMembers(id, members())
			}
		case tStructStart:
			i++
			if types[i] == tStructStop {
				i++
			} else {
				db.addMembers(id, members())
			}
		case tAnnotationStart:
			i++
			annotation := alphaNum()
			expect(tAnnotationEnd)
			db.addAnnotation(id, annotation)
			if types[i] == tStructStart {
				i++
				if types[i] == tStructStop {
					i++
				} else {
					db.addMembers(id, members())
				}
			}
		}
		return id
	}

	noteStatement := func() string {
		i++
		if types[i-1] == tNote {
			return db.addNote(expect(tStr), nil)
		}
		id := name(true)
		return db.addNote(expect(tStr), &id)
	}

	// Namespaces nest, so they are parsed with a stack of open ones.
	namespaceStatement := func() {
		var open []*frame
		for {
			i++
			ns := name(false)
			var id string
			if types[i] == tSqs {
				id = db.addNamespace(ns, classLabel())
			} else {
				id = db.addNamespace(ns, "")
			}
			expect(tStructStart)
			if types[i] == tNewline {
				i++
			}
			open = append(open, &frame{id: id})
			item := true
			for item {
				f := open[len(open)-1]
				t := types[i]
				if t == tNamespace {
					break
				}
				switch t {
				case tClass:
					f.classes = append(f.classes, classStatement())
				case tNote, tNoteFor:
					f.notes = append(f.notes, noteStatement())
				default:
					fail("'NAMESPACE', 'CLASS', 'NOTE_FOR', 'NOTE'")
				}
				for {
					if types[i] == tNewline {
						next := kind(i + 1)
						i++
						if next == tNamespace || next == tClass || next == tNote || next == tNoteFor {
							break
						}
					}
					expect(tStructStop)
					done := open[len(open)-1]
					open = open[:len(open)-1]
					db.addClassesToNamespace(done.id, done.classes, done.notes)
					db.popNamespace()
					if len(open) == 0 {
						item = false
						break
					}
				}
			}
			if len(open) == 0 {
				return
			}
		}
	}

	relationEnd := func() int {
		typ := endType(types[i])
		if typ != relationNone {
			i++
		}
		return typ
	}

	classOrRelation := func() {
		id1 := name(true)
		t := types[i]
		if t == tLabel {
			db.addMember(id1, cleanupLabel(take()))
			return
		}
		title1 := "none"
		if t == tStr {
			title1 = take()
		} else if endType(t) == relationNone && t != tLine && t != tDottedLine {
			return
		}
		type1 := relationEnd()
		t = types[i]
		if t != tLine && t != tDottedLine {
			fail("'AGGREGATION', 'EXTENSION', 'COMPOSITION', 'DEPENDENCY', 'LOLLIPOP', 'LINE', 'DOTTED_LINE'")
		}
		i++
		type2 := relationEnd()
		title2 := "none"
		if types[i] == tStr {
			title2 = take()
		}
		id2 := name(true)
		r := &relation{id1: id1, id2: id2, type1: type1, type2: type2, relationTitle1: title1, relationTitle2: title2}
		if t != tLine {
			r.lineType = 1
		}
		if types[i] == tLabel {
			r.title = cleanupLabel(take())
		}
		db.addRelation(r)
	}

	styles := func() []string {
		var out []string
		for {
			if !isStylePart(types[i]) {
				fail("'ALPHA', 'NUM', 'COLON', 'UNIT', 'SPACE', 'BRKT', 'STYLE', 'PCT', 'LABEL'")
			}
			s := take()
			for isStylePart(types[i]) {
				s += take()
			}
			out = append(out, s)
			if types[i] != tComma {
				return out
			}
			i++
		}
	}

	tooltip := func(id string) {
		if types[i] == tStr {
			db.setTooltip(id, take())
		}
	}

	linkStatement := func(id string) {
		href := expect(tStr)
		var tip *string
		if types[i] == tStr {
			s := take()
			tip = &s
		}
		if types[i] == tLinkTarget {
			target := take()
			db.setLink(id, href, &target)
		} else {
			db.setLink(id, href, nil)
		}
		if tip != nil {
			db.setTooltip(id, *tip)
		}
	}

	statement := func() {
		t := types[i]
		switch t {
		case tUnicodeText, tAlpha, tMinus, tBquoteStr:
			classOrRelation()
			return
		case tNamespace:
			namespaceStatement()
			return
		case tClass:
			classStatement()
			return
		case tMember:
			i++
			return
		case tAnnotationStart:
			i++
			annotation := alphaNum()
			expect(tAnnotationEnd)
			db.addAnnotation(name(true), annotation)
			return
		case tCallback:
			i++
			id := name(true)
			expect(tStr)
			db.setClickEvent(id)
			tooltip(id)
			return
		case tLink:
			i++
			linkStatement(name(true))
			return
		case tClick:
			i++
			id := name(true)
			if types[i] == tHref {
				i++
				linkStatement(id)
				return
			}
			expect(tCallbackName)
			if types[i] == tCallbackArgs {
				i++
			}
			db.setClickEvent(id)
			tooltip(id)
			return
		case tStyle:
			i++
			id := expect(tAlpha)
			db.setCSSStyle(id, styles())
			return
		case tCssclass:
			i++
			ids := expect(tStr)
			db.setCSSClass(ids, expect(tAlpha))
			return
		case tNote, tNoteFor:
			noteStatement()
			return
		case tClassdef:
			i++
			ids := []string{expect(tAlpha)}
			for types[i] == tComma {
				i++
				ids = append(ids, expect(tAlpha))
			}
			db.defineClass(ids, styles())
			return
		case tDirectionTb, tDirectionBt, tDirectionRl, tDirectionLr:
			i++
			db.setDirection(directionWords[t-tDirectionTb])
			return
		case tAccTitle:
			i++
			db.setAccTitle(jsstr.Trim(expect(tAccTitleValue)))
			return
		case tAccDescr:
			i++
			db.setAccDescription(jsstr.Trim(expect(tAccDescrValue)))
			return
		case tAccDescrMultilineValue:
			db.setAccDescription(jsstr.Trim(take()))
			return
		}
		fail("'acc_title', 'acc_descr', 'acc_descr_multiline_value', 'NAMESPACE', 'ANNOTATION_START', 'CLASS', 'MEMBER', 'NOTE_FOR', 'NOTE', 'CLASSDEF', 'direction_tb', 'CALLBACK', 'LINK', 'CLICK', 'STYLE', 'CSSCLASS', 'ALPHA', 'MINUS', 'BQUOTE_STR'")
	}

	// Mermaid's grammar also accepts statements with no `classDiagram` line before them.
	header := types[i] == tClassDiagram
	if header {
		i++
		expect(tNewline)
	}
	for {
		statement()
		if types[i] != tNewline {
			break
		}
		i++
		if types[i] == tEof || types[i] == tEnd {
			break
		}
	}
	if header {
		expect(tEof)
	}
	expect(tEnd)
}
