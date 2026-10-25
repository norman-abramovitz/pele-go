package er

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

var cardinalities = map[int]cardinality{
	tZeroOrOne:  "ZERO_OR_ONE",
	tZeroOrMore: "ZERO_OR_MORE",
	tOneOrMore:  "ONE_OR_MORE",
	tOnlyOne:    "ONLY_ONE",
	tMdParent:   "MD_PARENT",
}

var directions = [4]string{"TB", "BT", "RL", "LR"}

const (
	expectName  = "'UNICODE_TEXT', 'NUM', 'ENTITY_NAME', 'DECIMAL_NUM', 'ENTITY_ONE'"
	expectCards = "'ZERO_OR_ONE', 'ZERO_OR_MORE', 'ONE_OR_MORE', 'ONLY_ONE', 'MD_PARENT'"
	expectLine  = "'EOF', 'NEWLINE', 'title', 'acc_title', 'acc_descr', 'acc_descr_multiline_value', 'SUBGRAPH', 'direction_tb', " +
		"'direction_bt', 'direction_rl', 'direction_lr', 'CLASSDEF', 'CLASS', 'STYLE', " +
		expectName
)

type frame struct {
	id    string
	title string
	// list is what Mermaid's grammar hands to addSubGraph: the value of every statement inside.
	// Besides entity names that includes accessibility text and the keywords `style`, `class`
	// and `classDef`.
	list []docItem
}

// parseEr panics with a *perr.Error on a syntax error.
func parseEr(source string, db *erDb, o *perr.Origin) {
	src := common.Src(jsstr.UTF16(source))
	tk := tokenize(src)
	types, texts, starts := tk.types, tk.texts, tk.starts
	i := 0

	// kind is types[i]; the lexer ends every list with a token the parser stops at, so it
	// never reads past the end.
	kind := func() int { return types[i] }

	fail := func(expected string) {
		t := kind()
		if t == tError {
			perr.Throw(perr.SyntaxError(o, "er", src, starts[i], "", true))
		}
		got := tokenNames[t]
		if t == tChar {
			got = texts[i]
		}
		perr.Throw(perr.SyntaxError(o, "er", src, starts[i], "Expecting "+expected+", got '"+got+"'", false))
	}

	expect := func(typ int) string {
		if kind() != typ {
			fail("'" + tokenNames[typ] + "'")
		}
		i++
		return texts[i-1]
	}

	separator := func() {
		t := kind()
		if t != tNewline && t != tSemi && t != tEOF {
			fail("'EOF', 'NEWLINE', 'SEMI'")
		}
		i++
	}

	isName := func(t int) bool {
		return t == tUnicodeText || t == tEntityName || t == tNum || t == tDecimalNum || t == tEntityOne
	}

	unquote := func(s string) string { return s[1 : len(s)-1] }

	entityName := func() string {
		t := kind()
		if !isName(t) {
			fail(expectName)
		}
		text := texts[i]
		i++
		if t == tEntityName {
			return unquote(text)
		}
		return text
	}

	idList := func() []string {
		var ids []string
		for {
			t := kind()
			if t != tUnicodeText && t != tStyleText {
				fail("'UNICODE_TEXT', 'STYLE_TEXT'")
			}
			ids = append(ids, texts[i])
			i++
			if kind() != tComma {
				return ids
			}
			i++
		}
	}

	isStyleComponent := func(t int) bool { return t == tStyleText || t == tNum || t == tColon || t == tBrkt }

	stylesOpt := func() []string {
		var styles []string
		for {
			if !isStyleComponent(kind()) {
				fail("'STYLE_TEXT', 'COLON', 'NUM', 'BRKT'")
			}
			style := texts[i]
			i++
			for isStyleComponent(kind()) {
				style += texts[i]
				i++
			}
			styles = append(styles, style)
			if kind() != tComma {
				return styles
			}
			i++
		}
	}

	parseRelSpec := func() relSpec {
		cardB := cardinalities[kind()]
		i++
		r := kind()
		if r != tIdentifying && r != tNonIdentifying {
			fail("'NON_IDENTIFYING', 'IDENTIFYING'")
		}
		relType := identification("NON_IDENTIFYING")
		if r == tIdentifying {
			relType = "IDENTIFYING"
		}
		i++
		cardA, ok := cardinalities[kind()]
		if !ok {
			fail(expectCards)
		}
		i++
		return relSpec{cardA: cardA, relType: relType, cardB: cardB}
	}

	role := func() string {
		t := kind()
		if t != tWord && t != tEntityName && t != tUnicodeText {
			fail("'UNICODE_TEXT', 'ENTITY_NAME', 'WORD'")
		}
		text := texts[i]
		i++
		if t == tUnicodeText {
			return text
		}
		return unquote(text)
	}

	isChar := func(ch string) bool { return kind() == tChar && texts[i] == ch }

	// attributes returns the attributes last to first, as Mermaid's right-recursive rule
	// builds them.
	attributes := func() []erAttribute {
		var list []erAttribute
		for {
			typ := expect(tAttributeWord)
			if isChar("?") {
				typ += texts[i]
				i++
			}
			if kind() != tAttributeWord {
				if strings.HasSuffix(typ, "?") {
					fail("'ATTRIBUTE_WORD'")
				}
				fail("'ATTRIBUTE_WORD', '?'")
			}
			attribute := erAttribute{typ: typ, name: texts[i]}
			i++
			if kind() == tAttributeKey {
				keys := []string{texts[i]}
				i++
				for isChar(",") {
					i++
					keys = append(keys, expect(tAttributeKey))
				}
				attribute.keys = keys
			}
			if kind() == tComment {
				attribute.comment = unquote(texts[i])
				i++
			}
			list = append(list, attribute)
			if kind() == tBlockStop {
				break
			}
		}
		for l, r := 0, len(list)-1; l < r; l, r = l+1, r-1 {
			list[l], list[r] = list[r], list[l]
		}
		return list
	}

	// block parses an optional `{ ... }` block; nil when there is none or it is empty.
	block := func() []erAttribute {
		if kind() != tBlockStart {
			return nil
		}
		i++
		if kind() == tBlockStop {
			i++
			return nil
		}
		list := attributes()
		i++
		return list
	}

	entityStatement := func() []string {
		a := entityName()
		t := kind()
		if t == tSQS {
			i++
			alias := entityName()
			expect(tSQE)
			var classes []string
			if kind() == tStyleSeparator {
				i++
				classes = idList()
			}
			attrs := block()
			db.addEntity(a, alias)
			if attrs != nil {
				db.addAttributes(a, attrs)
			}
			if classes != nil {
				db.setClass([]string{a}, classes)
			}
			return []string{a}
		}
		var classes []string
		if t == tStyleSeparator {
			i++
			classes = idList()
			t = kind()
		}
		if _, ok := cardinalities[t]; ok {
			spec := parseRelSpec()
			b := entityName()
			var classesB []string
			if kind() == tStyleSeparator {
				i++
				classesB = idList()
			}
			expect(tColon)
			label := role()
			db.addEntity(a, "")
			db.addEntity(b, "")
			db.addRelationship(a, label, b, spec)
			if classes != nil {
				db.setClass([]string{a}, classes)
			}
			if classesB != nil {
				db.setClass([]string{b}, classesB)
			}
			return []string{a, b}
		}
		attrs := block()
		db.addEntity(a, "")
		if attrs != nil {
			db.addAttributes(a, attrs)
		}
		if classes != nil {
			db.setClass([]string{a}, classes)
		}
		return []string{a}
	}

	expect(tErDiagram)
	// Subgraphs nest on this stack rather than on the call stack; the first frame is the
	// diagram itself.
	frames := []*frame{{}}
	cur := frames[0]
	nested := false
	push := func(value string) {
		if nested {
			cur.list = append(cur.list, docItem{value: value})
		}
	}

	for {
		t := kind()
		switch t {
		case tNewline:
			i++
		case tEOF:
			i++
			if nested {
				fail("'END', " + expectLine)
			}
			return
		case tEndKw:
			if !nested {
				fail(expectLine)
			}
			i++
			id := db.addSubGraph(cur.id, cur.list, cur.title)
			frames = frames[:len(frames)-1]
			cur = frames[len(frames)-1]
			nested = len(frames) > 1
			push(id)
		case tSubgraph:
			i++
			id := entityName()
			title := id
			if kind() == tSQS {
				i++
				title = entityName()
				for isName(kind()) {
					title += " " + entityName()
				}
				expect(tSQE)
			}
			separator()
			cur = &frame{id: id, title: title}
			frames = append(frames, cur)
			nested = true
		case tAccTitle:
			i++
			value := jsstr.Trim(expect(tAccTitleValue))
			db.setAccTitle(value)
			push(value)
		case tAccDescr:
			i++
			value := jsstr.Trim(expect(tAccDescrValue))
			db.setAccDescription(value)
			push(value)
		case tAccDescrMultilineValue:
			value := jsstr.Trim(texts[i])
			i++
			db.setAccDescription(value)
			push(value)
		case tDirectionTB, tDirectionBT, tDirectionRL, tDirectionLR:
			i++
			value := directions[t-tDirectionTB]
			if nested {
				cur.list = append(cur.list, docItem{value: value, isDir: true})
			} else {
				db.setDirection(value)
			}
		case tClassDef:
			keyword := texts[i]
			i++
			ids := idList()
			styles := stylesOpt()
			separator()
			db.addClass(ids, styles)
			push(keyword)
		case tClass:
			keyword := texts[i]
			i++
			ids := idList()
			db.setClass(ids, idList())
			push(keyword)
		case tStyle:
			keyword := texts[i]
			i++
			ids := idList()
			styles := stylesOpt()
			separator()
			db.addCssStyles(ids, styles)
			push(keyword)
		default:
			if !isName(t) {
				if nested {
					fail("'END', " + expectLine)
				}
				fail(expectLine)
			}
			for _, name := range entityStatement() {
				push(name)
			}
		}
	}
}
