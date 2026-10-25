package usecase

import (
	"strconv"
	"strings"
	"sync"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// Mermaid parses use case diagrams with a Chevrotain parser, which decides each choice by
// looking at most three tokens ahead and reports the first token that fits nowhere. This
// parser makes the same decisions from the same lookahead, so it accepts the same text and
// names the same token in its errors. It builds the model directly, where Mermaid builds a
// syntax tree and then visits it.

const (
	flagLine        = 1
	flagRelation    = 2
	flagStylePart   = 4
	flagFollowsName = 8
	flagBoundary    = 16
	flagClosesLabel = 32
	flagMetadata    = 64
)

const (
	arrowNames     = "DEPENDENCY_ARROW|GENERALIZATION|FORWARD_SOLID|BACKWARD_SOLID|MARKERLESS_SOLID|FORWARD_CIRCLE|BACKWARD_CIRCLE|FORWARD_CROSS|BACKWARD_CROSS"
	nameNames      = "IDENTIFIER|PLAIN_STRING|MARKDOWN_STRING"
	labelNames     = "PLAIN_STRING|MARKDOWN_STRING|LABEL_TEXT"
	statementNames = "ACC_TITLE_LINE|ACC_DESCR_LINE|ACC_DESCR_BLOCK|DIRECTION|ACTOR|SYSTEM_BOUNDARY|NOTE|JSON_DECLARATION_START|CLASS_DEF|CLASS|STYLE"
	stylePartNames = "WORD|PLAIN_STRING|NUMBER|HASH_COLOR|CSS_IDENTIFIER|CSS_ESCAPED_COMMA|DASH|DOT|PERCENT|CSS_PUNCTUATION|COLON|LPAREN|RPAREN|LBRACKET|RBRACKET|LBRACE|RBRACE|AT|MARKERLESS_SOLID"
)

var tokenFlags = sync.OnceValue(func() [tRParen + 1]uint8 {
	var flags [tRParen + 1]uint8
	flag := func(names string, bits uint8) {
		for _, name := range strings.Split(names, "|") {
			// WORD is a category: every keyword and IDENTIFIER.
			if name == "WORD" {
				for t := tUsecase; t <= tIdentifier; t++ {
					flags[t] |= bits
				}
				continue
			}
			for t, n := range tokenNames {
				if n == name {
					flags[t] |= bits
				}
			}
		}
	}
	flag("NEWLINE|COMMENT|"+statementNames+"|"+nameNames, flagLine)
	flag("IDENTIFIER|"+arrowNames, flagRelation)
	flag(stylePartNames, flagStylePart)
	flag("STEREOTYPE_START|CLASS_SEPARATOR|IDENTIFIER|"+arrowNames+"|NEWLINE|EOF", flagFollowsName)
	flag("NEWLINE|COMMENT|ACTOR|"+nameNames, flagBoundary)
	flag("FORWARD_SOLID|MARKERLESS_SOLID|FORWARD_CIRCLE|FORWARD_CROSS", flagClosesLabel)
	flag("NEWLINE|IDENTIFIER|PLAIN_STRING|RBRACE", flagMetadata)
	return flags
})

func hasFlag(t int, bits uint8) bool { return tokenFlags()[t]&bits != 0 }

var forbidden = map[string]bool{"allowmixing": true, "newpage": true, "package": true, "rectangle": true, "skinparam": true}

type item struct {
	id        string
	label     draftLabel
	span      span
	generated bool
	shape     string
	// metadata is nil where upstream's is undefined.
	metadata       []draftProperty
	stereotype     string
	stereotypeSpan *span
	classes        []string
	classSpans     []span
	// Whether this mention says something about the element, and so declares it.
	explicit bool
	nodeSpan span
}

type tail struct {
	explicitID     string
	explicitIDSpan span
	typ            string
	arrowType      int
	label          *draftLabel
	minlen         int
	target         *item
	span           span
}

// parser holds what upstream keeps in module variables for the length of a parse.
type parser struct {
	src         common.Src
	types       []int
	starts      []int
	ends        []int
	p           int
	drafts      *drafts
	parent      *draftBoundary
	pendingJSON []pendingJSON
	origin      *perr.Origin
}

type pendingJSON struct {
	json    *draftJSON
	literal int
}

func (ps *parser) image(k int) string { return ps.src.String(ps.starts[k], ps.ends[k]) }

func (ps *parser) span(k int) span { return span{ps.starts[k], ps.ends[k]} }

func (ps *parser) fail(detail string) {
	line, column := lineColumn(ps.origin, ps.src, ps.starts[ps.p])
	perr.Throw(&perr.Error{
		Code:    perr.Syntax,
		Message: "Error parsing usecase diagram: " + detail + " at " + locationText(ps.origin, ps.src, ps.starts[ps.p], ps.ends[ps.p]),
		Type:    "usecase",
		Line:    line,
		Column:  column,
	})
}

func (ps *parser) mismatch(name string) {
	ps.fail("Expecting token of type --> " + name + " <-- but found --> '" + ps.image(ps.p) + "' <--")
}

func (ps *parser) expect(typ int) int {
	if ps.types[ps.p] != typ {
		ps.mismatch(tokenNames[typ])
	}
	ps.p++
	return ps.p - 1
}

// noAlternative: Chevrotain lists every token sequence that would have been accepted.
func (ps *parser) noAlternative(expected string) {
	var list []string
	for i, sequence := range strings.Split(expected, "|") {
		list = append(list, "  "+strconv.Itoa(i+1)+". ["+sequence+"]")
	}
	ps.fail("Expecting: one of these possible Token sequences:\n" + strings.Join(list, "\n") + "\nbut found: '" + ps.image(ps.p) + "'")
}

func statementSequences() string {
	names := strings.Split(nameNames, "|")
	var metadata []string
	for _, name := range names {
		for _, next := range []string{"NEWLINE", "IDENTIFIER", "PLAIN_STRING", "RBRACE"} {
			metadata = append(metadata, name+", METADATA_START, "+next)
		}
	}
	follows := strings.Split("STEREOTYPE_START|CLASS_SEPARATOR|IDENTIFIER|"+arrowNames+"|NEWLINE|EOF", "|")
	var entities []string
	for i, name := range names {
		nexts := follows
		if i == 0 {
			nexts = append([]string{"LPAREN", "LBRACKET"}, follows...)
		}
		for _, next := range nexts {
			entities = append(entities, name+", "+next)
		}
	}
	all := append([]string{statementNames}, metadata...)
	all = append(all, entities...)
	all = append(all, metadata...)
	return strings.Join(all, "|")
}

func isWordToken(t int) bool { return t >= tUsecase && t <= tIdentifier }

func isLabelText(t int) bool { return t >= tUsecase && t <= tLabelSymbol }

func (ps *parser) lineEnd() {
	switch ps.types[ps.p] {
	case tNewline:
		ps.p++
	case tEOF:
	default:
		ps.noAlternative("NEWLINE|EOF")
	}
}

// content is the text of a name or string token, without its quotes.
func (ps *parser) content(k int) span {
	trim := 0
	switch ps.types[k] {
	case tMarkdownString:
		trim = 2
	case tPlainString:
		trim = 1
	}
	return span{ps.starts[k] + trim, ps.ends[k] - trim}
}

func (ps *parser) tokenLabel(k int) draftLabel {
	at := ps.content(k)
	typ := "text"
	if ps.types[k] == tMarkdownString {
		typ = "markdown"
	}
	return draftLabel{text: ps.src.String(at[0], at[1]), typ: typ, span: at}
}

// generatedID is label.replace(/\W/g, '_'), which replaces each code unit of a character
// outside ASCII.
func generatedID(label string) string {
	units := jsstr.UTF16(label)
	for i, u := range units {
		if !isWord(int(u)) {
			units[i] = '_'
		}
	}
	return jsstr.FromUTF16(units)
}

// label: an unquoted label runs to the next delimiter and is taken from the source as written.
func (ps *parser) label() draftLabel {
	typ := ps.types[ps.p]
	if typ == tPlainString || typ == tMarkdownString {
		ps.p++
		return ps.tokenLabel(ps.p - 1)
	}
	if !isLabelText(typ) {
		ps.noAlternative(labelNames)
	}
	first := ps.p
	for isLabelText(ps.types[ps.p]) {
		ps.p++
	}
	at := span{ps.starts[first], ps.ends[ps.p-1]}
	return draftLabel{text: ps.src.String(at[0], at[1]), typ: "text", span: at}
}

func isKey(t int) bool { return t == tIdentifier || t == tPlainString }

func (ps *parser) property() draftProperty {
	key := ps.p
	if !isKey(ps.types[ps.p]) {
		ps.noAlternative("IDENTIFIER|PLAIN_STRING")
	}
	ps.p++
	ps.expect(tColon)
	value := ps.p
	typ := ps.types[ps.p]
	if !isKey(typ) && typ != tTrue && typ != tFalse {
		ps.noAlternative("IDENTIFIER|PLAIN_STRING|TRUE|FALSE")
	}
	ps.p++
	keySpan := ps.content(key)
	valueSpan := ps.content(value)
	var v any
	switch typ {
	case tTrue:
		v = true
	case tFalse:
		v = false
	default:
		v = ps.src.String(valueSpan[0], valueSpan[1])
	}
	return draftProperty{
		key:       ps.src.String(keySpan[0], keySpan[1]),
		value:     v,
		span:      span{ps.starts[key], ps.ends[value]},
		keySpan:   keySpan,
		valueSpan: valueSpan,
	}
}

// separatorAhead is whether another property follows. A comma or line breaks before the
// closing brace do not count, but Chevrotain looks only three tokens ahead, so a comma and two
// line breaks, or three line breaks, are taken for a separator and the brace after them is then
// an error.
func (ps *parser) separatorAhead() bool {
	a, b, c := ps.types[ps.p], ps.types[ps.p+1], ps.types[ps.p+2]
	if a == tComma {
		return isKey(b) || (b == tNewline && (c == tNewline || isKey(c)))
	}
	if a != tNewline {
		return false
	}
	return b == tComma || isKey(b) || (b == tNewline && (c == tNewline || c == tComma || isKey(c)))
}

func (ps *parser) skipNewlines() {
	for ps.types[ps.p] == tNewline {
		ps.p++
	}
}

func (ps *parser) metadata() []draftProperty {
	ps.expect(tMetadataStart)
	properties := []draftProperty{}
	ps.skipNewlines()
	if isKey(ps.types[ps.p]) {
		properties = append(properties, ps.property())
		for ps.separatorAhead() {
			if ps.types[ps.p] == tComma {
				ps.p++
			} else {
				ps.skipNewlines()
				if ps.types[ps.p] == tComma {
					ps.p++
				}
			}
			ps.skipNewlines()
			properties = append(properties, ps.property())
		}
		if ps.types[ps.p] == tComma {
			ps.p++
		}
		ps.skipNewlines()
	}
	ps.expect(tRBrace)
	return properties
}

func (ps *parser) classSuffix(classes *[]string, spans *[]span) {
	for {
		ps.p++
		id := ps.expect(tIdentifier)
		*classes = append(*classes, ps.image(id))
		*spans = append(*spans, ps.span(id))
		if ps.types[ps.p] != tComma {
			return
		}
	}
}

// name is an id with an optional label in parentheses or brackets, or a string that stands
// for both.
func (ps *parser) name(brackets bool) *item {
	k := ps.p
	typ := ps.types[ps.p]
	it := &item{span: ps.content(k), generated: typ != tIdentifier, classes: []string{}}
	switch typ {
	case tIdentifier:
		ps.p++
		it.id = ps.image(k)
		open := ps.types[ps.p]
		if open == tLParen || (brackets && open == tLBracket) {
			ps.p++
			it.label = ps.label()
			if open == tLParen {
				ps.expect(tRParen)
				it.shape = "ellipse"
			} else {
				ps.expect(tRBracket)
				it.shape = "rect"
			}
		} else {
			it.label = ps.tokenLabel(k)
		}
	case tPlainString, tMarkdownString:
		ps.p++
		it.label = ps.tokenLabel(k)
		it.id = generatedID(it.label.text)
	default:
		ps.noAlternative(nameNames)
	}
	return it
}

func (ps *parser) element(actor bool) *item {
	first := ps.p
	it := ps.name(!actor)
	if actor {
		it.shape = ""
	}
	if ps.types[ps.p] == tMetadataStart {
		it.metadata = ps.metadata()
	}
	if ps.types[ps.p] == tStereotypeStart {
		ps.p++
		t := ps.expect(tStereotypeText)
		ps.expect(tStereotypeEnd)
		it.stereotype = jsstr.Trim(ps.image(t))
		s := ps.span(t)
		it.stereotypeSpan = &s
	}
	if ps.types[ps.p] == tClassSeparator {
		ps.classSuffix(&it.classes, &it.classSpans)
	}
	it.explicit = it.shape != "" || it.metadata != nil || it.stereotype != ""
	it.nodeSpan = span{ps.starts[first], ps.ends[ps.p-1]}
	return it
}

func (ps *parser) draft(it *item, kind string) *draftElement {
	return &draftElement{
		named:      named{id: it.id, label: it.label, span: it.span, generated: it.generated},
		kind:       kind,
		parent:     ps.parent,
		shape:      it.shape,
		metadata:   it.metadata,
		stereotype: it.stereotype,
		classes:    it.classes,
	}
}

func endpointOf(it *item, declares bool) draftEndpoint {
	return draftEndpoint{
		named:    named{id: it.id, label: it.label, span: it.span, generated: it.generated},
		declares: declares,
		classed:  len(it.classes) > 0,
	}
}

// labeledRight is whether a label and then one of the operators that can close it come next.
func (ps *parser) labeledRight(closers uint8) bool {
	if ps.types[ps.p] < tUsecase || ps.types[ps.p] > tMarkdownString {
		return false
	}
	for k := ps.p + 1; ; k++ {
		typ := ps.types[k]
		if typ == tMarkerlessSolid || hasFlag(typ, closers) {
			return true
		}
		if typ < tUsecase || typ > tMarkdownString {
			return false
		}
	}
}

// minimumLength: each dash beyond the second asks for one more rank between the two ends.
func (ps *parser) minimumLength(k int) int {
	dashes := ps.ends[k] - ps.starts[k]
	if ps.types[k] != tMarkerlessSolid {
		dashes--
	}
	return max(1, dashes-1)
}

func (ps *parser) relationTail() *tail {
	first := ps.p
	t := &tail{typ: "association", arrowType: arrowSolidArrow, minlen: 1}
	if ps.types[ps.p] == tIdentifier && ps.types[ps.p+1] == tAt {
		t.explicitID = ps.image(ps.p)
		t.explicitIDSpan = ps.span(ps.p)
		ps.p += 2
	}
	k := ps.p
	typ := ps.types[ps.p]
	ps.p++
	length := ps.ends[k] - ps.starts[k]
	switch typ {
	case tDependencyArrow:
		ps.expect(tColon)
		if ps.types[ps.p] != tInclude && ps.types[ps.p] != tExtend {
			ps.noAlternative("INCLUDE|EXTEND")
		}
		t.typ = "extend"
		if ps.types[ps.p] == tInclude {
			t.typ = "include"
		}
		t.label = &draftLabel{text: t.typ, typ: "text", span: ps.span(ps.p)}
		ps.p++
	case tGeneralization:
		t.typ = "generalization"
	case tForwardSolid:
		t.minlen = ps.minimumLength(k)
	case tBackwardSolid:
		t.arrowType = arrowBackArrow
		// Only the shortest operator can open a label: `A <-- label -- B`.
		if length == 3 && ps.labeledRight(0) {
			l := ps.label()
			t.label = &l
			t.minlen = ps.minimumLength(ps.expect(tMarkerlessSolid))
		} else {
			t.minlen = ps.minimumLength(k)
		}
	case tMarkerlessSolid:
		t.arrowType = arrowLineSolid
		if length == 2 && ps.labeledRight(flagClosesLabel) {
			l := ps.label()
			t.label = &l
			switch ps.types[ps.p] {
			case tForwardSolid:
				t.arrowType = arrowSolidArrow
			case tForwardCircle:
				t.arrowType = arrowCircleArrow
			case tForwardCross:
				t.arrowType = arrowCrossArrow
			case tMarkerlessSolid:
			default:
				ps.noAlternative("FORWARD_SOLID|MARKERLESS_SOLID|FORWARD_CIRCLE|FORWARD_CROSS")
			}
			if t.arrowType == arrowSolidArrow || t.arrowType == arrowLineSolid {
				t.minlen = ps.minimumLength(ps.p)
			}
			ps.p++
		} else {
			t.minlen = ps.minimumLength(k)
		}
	case tForwardCircle:
		t.arrowType = arrowCircleArrow
	case tForwardCross:
		t.arrowType = arrowCrossArrow
	case tBackwardCircle, tBackwardCross:
		t.arrowType = arrowCrossArrowReversed
		if typ == tBackwardCircle {
			t.arrowType = arrowCircleArrowReversed
		}
		if ps.labeledRight(0) {
			l := ps.label()
			t.label = &l
			ps.expect(tMarkerlessSolid)
		}
	default:
		ps.p--
		ps.noAlternative(arrowNames)
	}
	t.target = ps.element(false)
	t.span = span{ps.starts[first], ps.ends[ps.p-1]}
	return t
}

func (ps *parser) relationship(source *item, declaration bool, t *tail) {
	target := t.target
	if target.explicit {
		ps.drafts.elements = append(ps.drafts.elements, ps.draft(target, "usecase"))
	}
	ps.drafts.relationships = append(ps.drafts.relationships, &draftRelationship{
		source:    endpointOf(source, declaration),
		target:    endpointOf(target, target.explicit),
		span:      t.span,
		id:        t.explicitID,
		idSpan:    t.explicitIDSpan,
		typ:       t.typ,
		arrowType: t.arrowType,
		label:     t.label,
		minlen:    t.minlen,
	})
}

// actors: `actor A, B` declares several actors; at the top level `actor A --> B` declares one
// and relates it.
func (ps *parser) actors(top bool) {
	ps.p++
	items := []*item{ps.element(true)}
	var t *tail
	if ps.types[ps.p] == tComma {
		for ps.types[ps.p] == tComma {
			ps.p++
			items = append(items, ps.element(true))
		}
	} else if top && hasFlag(ps.types[ps.p], flagRelation) {
		t = ps.relationTail()
	}
	ps.lineEnd()
	for _, it := range items {
		ps.drafts.elements = append(ps.drafts.elements, ps.draft(it, "actor"))
	}
	if t != nil {
		ps.relationship(items[0], true, t)
	}
}

func (ps *parser) entity(top bool) {
	source := ps.element(false)
	var t *tail
	if top && hasFlag(ps.types[ps.p], flagRelation) {
		t = ps.relationTail()
	}
	ps.lineEnd()
	if t == nil {
		source.explicit = true
		if source.shape == "" {
			source.shape = "ellipse"
		}
	}
	if source.explicit {
		ps.drafts.elements = append(ps.drafts.elements, ps.draft(source, "usecase"))
	}
	if t != nil {
		ps.relationship(source, source.explicit, t)
	}
}

func (ps *parser) boundary() {
	ps.p++
	n := ps.name(true)
	var inline []draftProperty
	if ps.types[ps.p] == tMetadataStart {
		inline = ps.metadata()
	}
	if ps.types[ps.p] == tClassSeparator {
		ps.classSuffix(&n.classes, &n.classSpans)
	}
	group := &draftBoundary{
		named:   named{id: n.id, label: n.label, span: n.span, generated: n.generated},
		classes: n.classes,
	}
	ps.lineEnd()
	ps.drafts.boundaries = append(ps.drafts.boundaries, group)
	ps.parent = group
	for hasFlag(ps.types[ps.p], flagBoundary) {
		ps.line(false)
	}
	ps.parent = nil
	ps.expect(tEnd)
	ps.lineEnd()
	if inline != nil {
		ps.drafts.metadata = append(ps.drafts.metadata, draftMetadata{target: group.id, span: group.span, properties: inline})
	}
}

func (ps *parser) style() string {
	first := ps.p
	typ := ps.types[ps.p]
	switch {
	case isWordToken(typ) || typ == tCSSIdentifier:
		ps.p++
	case typ == tMarkerlessSolid:
		ps.p++
		if !isWordToken(ps.types[ps.p]) {
			ps.mismatch("WORD")
		}
		ps.p++
	default:
		ps.noAlternative("WORD|CSS_IDENTIFIER|MARKERLESS_SOLID")
	}
	ps.expect(tColon)
	if !hasFlag(ps.types[ps.p], flagStylePart) {
		ps.fail("Expecting: expecting at least one iteration which starts with one of these possible Token sequences::\n  <[" +
			strings.ReplaceAll(stylePartNames, "|", "] ,[") + "]>\nbut found: '" + ps.image(ps.p) + "'")
	}
	for hasFlag(ps.types[ps.p], flagStylePart) {
		ps.p++
	}
	// The value is taken from the source, so the spaces inside `1px solid red` survive.
	return strings.ReplaceAll(ps.src.String(ps.starts[first], ps.ends[ps.p-1]), `\,`, ",")
}

func (ps *parser) styles() []string {
	out := []string{ps.style()}
	for ps.types[ps.p] == tComma {
		ps.p++
		out = append(out, ps.style())
	}
	return out
}

func (ps *parser) identifiers() []int {
	out := []int{ps.expect(tIdentifier)}
	for ps.types[ps.p] == tComma {
		ps.p++
		out = append(out, ps.expect(tIdentifier))
	}
	return out
}

func (ps *parser) images(ks []int) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = ps.image(k)
	}
	return out
}

func (ps *parser) topStatement() {
	k := ps.p
	typ := ps.types[ps.p]
	switch typ {
	case tAccTitleLine, tAccDescrLine, tAccDescrBlock:
		ps.p++
		t := ps.image(k)
		switch typ {
		case tAccDescrBlock:
			ps.drafts.accDescription = jsstr.Trim(t[strings.IndexByte(t, '{')+1 : strings.LastIndexByte(t, '}')])
		case tAccDescrLine:
			ps.drafts.accDescription = jsstr.Trim(t[strings.IndexByte(t, ':')+1:])
		default:
			ps.drafts.accTitle = jsstr.Trim(t[strings.IndexByte(t, ':')+1:])
		}
	case tDirection:
		ps.p++
		value := ps.types[ps.p]
		if value < tTD || value > tRL {
			ps.noAlternative("TD|TB|BT|LR|RL")
		}
		if value == tTD {
			ps.drafts.direction = "TB"
		} else {
			ps.drafts.direction = ps.image(ps.p)
		}
		ps.p++
	case tActor:
		ps.actors(true)
		return
	case tSystemBoundary:
		ps.boundary()
		return
	case tNote:
		ps.p++
		ps.expect(tFor)
		target := ps.expect(tIdentifier)
		l := ps.label()
		ps.drafts.notes = append(ps.drafts.notes, draftNote{target: ps.image(target), span: ps.span(target), label: l})
	case tJSONDeclarationStart:
		ps.p++
		literal := ps.expect(tJSONObjectLiteral)
		classes := []string{}
		var classSpans []span
		if ps.types[ps.p] == tClassSeparator {
			ps.classSuffix(&classes, &classSpans)
		}
		from := ps.starts[k] + 4
		for c := ps.src.At(from); c == 32 || c == 9; c = ps.src.At(from) {
			from++
		}
		to := from
		for isWord(ps.src.At(to)) {
			to++
		}
		j := &draftJSON{id: ps.src.String(from, to), span: span{from, to}, classes: classes}
		ps.drafts.jsons = append(ps.drafts.jsons, j)
		ps.pendingJSON = append(ps.pendingJSON, pendingJSON{j, literal})
	case tClassDef:
		ps.p++
		ids := ps.identifiers()
		ps.drafts.classDefs = append(ps.drafts.classDefs, draftClassDef{ids: ps.images(ids), styles: ps.styles()})
	case tClass:
		ps.p++
		targets := ps.identifiers()
		classes := ps.identifiers()
		var list []draftTarget
		for _, id := range targets {
			list = append(list, draftTarget{id: ps.image(id), span: ps.span(id)})
		}
		ps.drafts.classes = append(ps.drafts.classes, draftClasses{targets: list, classes: ps.images(classes)})
	case tStyle:
		ps.p++
		target := ps.expect(tIdentifier)
		ps.drafts.styles = append(ps.drafts.styles, draftStyle{target: ps.image(target), span: ps.span(target), styles: ps.styles()})
	default:
		next := ps.types[ps.p+1]
		if next == tMetadataStart {
			if !hasFlag(ps.types[ps.p+2], flagMetadata) {
				ps.noAlternative(statementSequences())
			}
			ps.p++
			at := ps.content(k)
			id := ps.image(k)
			if typ != tIdentifier {
				id = generatedID(ps.src.String(at[0], at[1]))
			}
			assigned := ps.metadata()
			ps.drafts.metadata = append(ps.drafts.metadata, draftMetadata{target: id, span: at, properties: assigned})
		} else {
			follows := hasFlag(next, flagFollowsName) || (typ == tIdentifier && (next == tLParen || next == tLBracket))
			// Mermaid turns away the PlantUML statements people are most likely to try.
			if !follows || (typ == tIdentifier && forbidden[strings.ToLower(ps.image(k))]) {
				ps.noAlternative(statementSequences())
			}
			ps.entity(true)
			return
		}
	}
	ps.lineEnd()
}

func (ps *parser) line(top bool) {
	switch typ := ps.types[ps.p]; {
	case typ == tNewline:
		ps.p++
	case typ == tComment:
		ps.p++
		ps.lineEnd()
	case top:
		ps.topStatement()
	case typ == tActor:
		ps.actors(false)
	default:
		ps.entity(false)
	}
}

func parseUsecase(source string, title string, o *perr.Origin) *usecaseModel {
	src := common.Src(jsstr.UTF16(source))
	toks := tokenize(src, o)
	ps := &parser{src: src, origin: o, types: toks.types, starts: toks.starts, ends: toks.ends, drafts: &drafts{}}
	ps.expect(tUsecase)
	ps.lineEnd()
	for hasFlag(ps.types[ps.p], flagLine) {
		ps.line(true)
	}
	if ps.types[ps.p] != tEOF {
		ps.fail("Redundant input, expecting EOF but found: " + ps.image(ps.p))
	}
	// The whole text has to be well formed before the JSON in it is looked at, as in Mermaid.
	for _, pending := range ps.pendingJSON {
		literal := pending.literal
		parsed := parseOrderedJSON(src[ps.starts[literal]:ps.ends[literal]], func() (int, int) {
			return lineColumn(o, src, ps.starts[literal])
		})
		pending.json.value = parsed.value
		pending.json.propertyOrder = parsed.propertyOrder
	}
	return resolveUsecase(src, ps.drafts, title, o)
}
