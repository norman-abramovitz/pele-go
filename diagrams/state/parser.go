package state

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

var directions = [...]string{"TB", "BT", "RL", "LR"}

const statement = "'NL', 'ID', 'HIDE_EMPTY', 'scale', 'COMPOSIT_STATE', 'STATE_DESCR', 'FORK', 'JOIN', 'CHOICE', 'CONCURRENT', 'note', " +
	"'acc_title', 'acc_descr', 'acc_descr_multiline_value', 'CLICK', 'classDef', 'style', 'class', 'direction_tb', " +
	"'direction_bt', 'direction_rl', 'direction_lr', 'EDGE_STATE'"

func parseState(source string, db *stateDb, o *perr.Origin) []stmt {
	src := common.Src(jsstr.UTF16(source))
	tk := tokenize(src)
	types, texts, starts := tk.types, tk.texts, tk.starts
	i := 0

	fail := func(expected string) {
		t := types[i]
		if t == tError {
			if tk.hasErr {
				loc := perr.Locate(o, src, starts[i])
				perr.Throw(&perr.Error{
					Code: perr.Syntax, Message: tk.err, Type: "state",
					Line: loc.Line, Column: loc.Column, Snippet: loc.Snippet,
				})
			}
			perr.Throw(perr.SyntaxError(o, "state", src, starts[i], "", true))
		}
		perr.Throw(perr.SyntaxError(o, "state", src, starts[i], "Expecting "+expected+", got '"+tokenNames[t]+"'", false))
	}
	expect := func(typ int) string {
		if types[i] != typ {
			fail("'" + tokenNames[typ] + "'")
		}
		i++
		return texts[i-1]
	}
	idStatement := func() *stateStmt {
		t := types[i]
		if t != tID && t != tEdgeState {
			fail("'ID', 'EDGE_STATE'")
		}
		id := jsstr.Trim(texts[i])
		i++
		if types[i] != tStyleSeparator {
			return &stateStmt{id: id, typ: "default"}
		}
		i++
		return &stateStmt{id: id, classes: []string{jsstr.Trim(expect(tID))}, typ: "default"}
	}

	for types[i] == tNL {
		i++
	}
	if types[i] != tSD {
		fail("'NL', 'SD'")
	}
	i++

	doc := &[]stmt{}
	var outer []*[]stmt
	// The grammar drops a statement whose value is the string it uses for a blank line.
	add := func(value stmt) {
		if value != textStmt("nl") {
			*doc = append(*doc, value)
		}
	}
	open := func(id string, description string) {
		i++
		inner := &[]stmt{}
		*doc = append(*doc, &stateStmt{id: id, typ: "default", description: descr{s: description}, doc: inner})
		outer = append(outer, doc)
		doc = inner
	}

	for {
		t := types[i]
		switch t {
		case tNL:
			i++
		case tID, tEdgeState:
			first := idStatement()
			switch types[i] {
			case tDescr:
				first.description = descr{s: db.trimColon(texts[i])}
				i++
				add(first)
			case tArrow:
				i++
				second := idStatement()
				if types[i] == tDescr {
					add(&relationStmt{state1: first, state2: second, description: db.trimColon(texts[i])})
					i++
				} else {
					add(&relationStmt{state1: first, state2: second})
				}
			default:
				add(first)
			}
		case tHideEmpty:
			add(textStmt(texts[i]))
			i++
		case tScale:
			i++
			expect(tWidth)
			add(textStmt(texts[i-2]))
		case tCompositState:
			if types[i+1] == tStructStart {
				i++
				open(texts[i-1], "")
			} else {
				add(textStmt(texts[i]))
				i++
			}
		case tStateDescr:
			description := texts[i]
			i++
			expect(tAs)
			id := expect(tID)
			if types[i] == tStructStart {
				open(id, description)
			} else if strings.Contains(id, ":") {
				parts := strings.Split(id, ":")
				add(&stateStmt{id: parts[0], typ: "default", description: descr{isList: true, list: []string{jsstr.Trim(description), parts[1]}}})
			} else {
				add(&stateStmt{id: id, typ: "default", description: descr{s: jsstr.Trim(description)}})
			}
		case tFork:
			add(&stateStmt{id: texts[i], typ: "fork"})
			i++
		case tJoin:
			add(&stateStmt{id: texts[i], typ: "join"})
			i++
		case tChoice:
			add(&stateStmt{id: texts[i], typ: "choice"})
			i++
		case tConcurrent:
			i++
			add(&stateStmt{id: db.getDividerID(), typ: "divider"})
		case tNote:
			keyword := texts[i]
			i++
			if types[i] == tLeftOf || types[i] == tRightOf {
				position := jsstr.Trim(texts[i])
				i++
				id := jsstr.Trim(expect(tID))
				add(&stateStmt{id: id, note: &stateNote{position: position, text: jsstr.Trim(expect(tNoteText))}})
			} else {
				if types[i] != tNoteText {
					fail("'NOTE_TEXT', 'left_of', 'right_of'")
				}
				i++
				expect(tAs)
				expect(tID)
				add(textStmt(keyword))
			}
		case tDirectionTB, tDirectionBT, tDirectionRL, tDirectionLR:
			value := directions[t-tDirectionTB]
			i++
			db.setDirection(value)
			add(&directionStmt{value: value})
		case tAccTitle:
			i++
			value := jsstr.Trim(expect(tAccTitleValue))
			db.setAccTitle(value)
			add(textStmt(value))
		case tAccDescr:
			i++
			value := jsstr.Trim(expect(tAccDescrValue))
			db.setAccDescription(value)
			add(textStmt(value))
		case tAccDescrMultilineValue:
			value := jsstr.Trim(texts[i])
			i++
			db.setAccDescription(value)
			add(textStmt(value))
		case tClick:
			i++
			id := idStatement()
			var url string
			tooltip := ""
			if types[i] == tHref {
				i++
				url = expect(tString)
			} else {
				if types[i] != tString {
					fail("'STRING', 'HREF'")
				}
				url = texts[i]
				i++
				tooltip = expect(tString)
			}
			expect(tNL)
			add(&clickStmt{id: id, url: url, tooltip: tooltip})
		case tClassDef:
			i++
			id := jsstr.Trim(expect(tClassDefID))
			add(&classDefStmt{id: id, classes: jsstr.Trim(expect(tClassDefStyleOpts))})
		case tStyle:
			i++
			id := jsstr.Trim(expect(tStyleIDs))
			add(&styleStmt{id: id, styleClass: jsstr.Trim(expect(tStyleDefStyleOpts))})
		case tClass:
			i++
			id := jsstr.Trim(expect(tClassEntityIDs))
			add(&applyClassStmt{id: id, styleClass: jsstr.Trim(expect(tStyleClass))})
		case tStructStop:
			if len(outer) == 0 {
				fail("'$end', " + statement)
			}
			i++
			doc = outer[len(outer)-1]
			outer = outer[:len(outer)-1]
		case tEnd:
			if len(outer) > 0 {
				fail("'STRUCT_STOP', " + statement)
			}
			db.setRootDoc(*doc)
			return *doc
		default:
			if len(outer) > 0 {
				fail("'STRUCT_STOP', " + statement)
			}
			fail("'$end', " + statement)
		}
	}
}
