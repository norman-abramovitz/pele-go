package venn

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

const (
	statementNames = "'EOF', 'NEWLINE', 'TITLE', 'SET', 'UNION', 'TEXT', 'INDENT_TEXT', 'STYLE'"
	valueNames     = "'NUMERIC', 'IDENTIFIER', 'STRING', 'HEXCOLOR', 'RGBCOLOR', 'RGBACOLOR'"
)

// parseVenn ports Mermaid's venn grammar. A token is read only when the grammar has to look
// at one to decide, because the lexer asks the model whether an indented `text` continues a
// set, and the statements are what tell the model so.
func parseVenn(source string, db *vennDb, o *perr.Origin) {
	src := common.Src(jsstr.UTF16(source))
	lexer := &vennLexer{src: src, db: db}
	typ := -1

	peek := func() int {
		if typ == -1 {
			typ = lexer.next()
		}
		return typ
	}

	fail := func(expected string) {
		if peek() == tError {
			perr.Throw(perr.SyntaxError(o, "venn", src, lexer.start, "", true))
		}
		perr.Throw(perr.SyntaxError(o, "venn", src, lexer.start, "Expecting "+expected+", got '"+tokenNames[typ]+"'", false))
	}

	take := func() string {
		typ = -1
		return lexer.text
	}

	expect := func(wanted int) string {
		if peek() != wanted {
			fail("'" + tokenNames[wanted] + "'")
		}
		return take()
	}

	identifier := func() string {
		if peek() != tIdentifier && typ != tString {
			fail("'IDENTIFIER', 'STRING'")
		}
		return take()
	}

	identifiers := func() []string {
		list := []string{identifier()}
		for peek() == tComma {
			take()
			list = append(list, identifier())
		}
		return list
	}

	subset := func(ids []string, union bool) {
		label := ""
		if peek() == tBracketLabel {
			label = take()
		}
		var size *float64
		if peek() == tColon {
			take()
			v := jsnum.ParseFloat(expect(tNumeric))
			size = &v
		}
		if union {
			if len(ids) < 2 {
				throwSemantic("union requires multiple identifiers")
			}
			db.validateUnionIdentifiers(ids)
		}
		db.addSubsetData(ids, label, size)
		db.indent = true
	}

	// The id of a text, and the label in brackets that an id which is not a number may carry.
	text := func(sets func() []string) {
		kind := peek()
		if kind != tIdentifier && kind != tString && kind != tNumeric {
			fail("'NUMERIC', 'IDENTIFIER', 'STRING'")
		}
		id := take()
		label := ""
		if kind != tNumeric && peek() == tBracketLabel {
			label = take()
		}
		db.addTextData(sets(), id, label)
	}

	style := func() {
		ids := identifiers()
		var styles [][2]string
		for {
			key := expect(tIdentifier)
			expect(tColon)
			var value string
			if peek() == tString {
				value = take()
			} else {
				var parts []string
				for peek() == tIdentifier || (typ >= tNumeric && typ <= tRGBAColor) {
					parts = append(parts, take())
				}
				if len(parts) == 0 {
					fail(valueNames)
				}
				value = strings.Join(parts, " ")
			}
			styles = append(styles, [2]string{key, value})
			if peek() != tComma {
				break
			}
			take()
		}
		db.addStyleData(ids, styles)
	}

	for peek() == tNewline {
		take()
	}
	expect(tVenn)
	for {
		statement := peek()
		if statement == tEOF {
			return
		}
		if statement < tNewline || statement > tStyle || statement == tVenn {
			fail(statementNames)
		}
		take()
		switch statement {
		case tTitle:
			db.setDiagramTitle(jsstr.Slice(lexer.text, 6, jsstr.Len(lexer.text)))
		case tSet:
			subset([]string{identifier()}, false)
		case tUnion:
			subset(identifiers(), true)
		case tText:
			ids := identifiers()
			text(func() []string { return ids })
		case tIndentText:
			text(func() []string {
				if db.current == nil {
					throwSemantic("text requires set")
				}
				return db.current
			})
		case tStyle:
			style()
		}
	}
}
