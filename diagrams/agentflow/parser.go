package agentflow

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

func tokenSet(types ...int) (set [tEOF + 1]bool) {
	for _, t := range types {
		set[t] = true
	}
	return set
}

var (
	idTokens       = tokenSet(tNum, tNodeString, tDown, tMinus, tDefault, tComma, tColon, tAmp, tBrkt, tMult, tUnicodeText)
	alphaNumTokens = tokenSet(tNum, tUnicodeText, tNodeString, tDir, tDown, tMinus, tComma, tColon, tAmp, tBrkt, tMult)
	noTagsTokens   = tokenSet(tNum, tNodeString, tSpace, tMinus, tAmp, tUnicodeText, tColon, tMult, tBrkt, tStyle,
		tLinkstyle, tClassdef, tClass, tClick, tGraph, tDir, tFlow, tConnector, tGlobal, tEnd, tDown, tUp, tStartLink)
	styleTokens     = tokenSet(tNum, tNodeString, tColon, tSpace, tBrkt, tStyle)
	separatorTokens = tokenSet(tNewline, tSemi, tEOF, tComment)
)

// frame is an open block: a flow or a global block, or the header of a connector, with the
// list and collect flag of its parent.
type frame struct {
	global bool
	// at is the token index of the keyword.
	at       int
	id       *flowText
	title    *flowText
	data     *string
	dataFrom int
	dataTo   int
	list     []docItem
	collect  bool
}

const (
	textTokens = "'UNICODE_TEXT', 'TEXT', 'TAGSTART'"
	separators = "'COMMENT', 'SEMI', 'NEWLINE', 'EOF'"
)

var directions = [...]string{"TB", "BT", "RL", "LR", "TD"}

const statementStart = "'COMMENT', 'SEMI', 'NEWLINE', 'SPACE', 'EOF', 'flow', 'global', 'connector', 'acc_title', 'acc_descr', 'acc_descr_multiline_value', 'STYLE', 'LINKSTYLE', 'CLASSDEF', 'CLASS', 'CLICK', 'NODE_STRING'"

// braceTrails returns the matches of /}[^\S\n]*\n/g in src, each replaced by the two
// characters `}\n`.
func braceTrails(src common.Src) []perr.Edit {
	var edits []perr.Edit
	for i := 0; i < len(src); i++ {
		if src[i] != '}' {
			continue
		}
		j := i + 1
		for j < len(src) && src[j] != '\n' && util.IsSpace(int(src[j])) {
			j++
		}
		if j >= len(src) || src[j] != '\n' {
			continue
		}
		edits = append(edits, perr.Edit{From: i, To: j + 1, Length: 2})
		i = j
	}
	return edits
}

// trimBraceTrail is source.replace(/}[^\S\n]*\n/g, '}\n'): Mermaid trims spaces and tabs
// between a closing brace and the end of its line before parsing.
func trimBraceTrail(src common.Src, edits []perr.Edit) common.Src {
	if edits == nil {
		return src
	}
	var out common.Src
	last := 0
	for _, e := range edits {
		out = append(out, src[last:e.From+1]...)
		out = append(out, '\n')
		last = e.To
	}
	return append(out, src[last:]...)
}

type parser struct {
	src common.Src
	// source is the text before trimBraceTrail, edits what it changed, and origin locates
	// errors in source.
	source      common.Src
	edits       []perr.Edit
	origin      *perr.Origin
	toks        tokens
	i           int
	db          *agentflowDb
	pendingData *string
	dataFrom    int
	dataTo      int
}

func (p *parser) kind() int { return p.toks.types[p.i] }

func (p *parser) fail(expected string) {
	t := p.toks.types[p.i]
	at := perr.InputOffset([][]perr.Edit{p.edits}, p.toks.starts[p.i])
	if t == tError {
		perr.Throw(perr.SyntaxError(p.origin, "agentflow", p.source, at, "", true))
	}
	perr.Throw(perr.SyntaxError(p.origin, "agentflow", p.source, at, "Expecting "+expected+", got '"+tokenNames[t]+"'", false))
}

// span is the span from the start of token a to the end of token b.
func (p *parser) span(a, b int) *loc {
	l := p.toks.locs
	return &loc{firstLine: l[4*a], firstColumn: l[4*a+1], lastLine: l[4*b+2], lastColumn: l[4*b+3]}
}

func (p *parser) take() string {
	s := p.toks.texts[p.i]
	p.i++
	return s
}

func (p *parser) expect(typ int, also string) string {
	if p.kind() != typ {
		detail := "'" + tokenNames[typ] + "'"
		if also != "" {
			detail += ", " + also
		}
		p.fail(detail)
	}
	return p.take()
}

func (p *parser) separator() {
	if !separatorTokens[p.kind()] {
		p.fail(separators)
	}
	p.i++
}

// run joins the texts of the tokens in set from here, at least one.
func (p *parser) run(set *[tEOF + 1]bool, expected string) string {
	if !set[p.kind()] {
		p.fail(expected)
	}
	s := p.take()
	for set[p.kind()] {
		s += p.take()
	}
	return s
}

func (p *parser) idString() string { return p.run(&idTokens, "'NODE_STRING'") }

func (p *parser) alphaNum() string { return p.run(&alphaNumTokens, "'NODE_STRING'") }

func isTextToken(t int) bool {
	return t == tText || t == tTagstart || t == tTagend || t == tUnicodeText
}

func (p *parser) text() *flowText {
	t := p.kind()
	var typ labelKind
	switch {
	case t == tStr:
		typ = "string"
	case t == tMdStr:
		typ = "markdown"
	case isTextToken(t):
		typ = "text"
	default:
		p.fail(textTokens + ", 'TAGEND', 'STR', 'MD_STR'")
	}
	s := p.take()
	for isTextToken(p.kind()) {
		s += p.take()
	}
	return &flowText{text: s, typ: typ}
}

func (p *parser) closed(typ int) {
	p.expect(typ, textTokens)
}

func (p *parser) edgeText() *flowText {
	var typ labelKind
	switch p.kind() {
	case tStr:
		typ = "string"
	case tMdStr:
		typ = "markdown"
	case tEdgeText, tUnicodeText:
		typ = "text"
	default:
		p.fail("'EDGE_TEXT', 'STR', 'MD_STR'")
	}
	s := p.take()
	for p.kind() == tEdgeText || p.kind() == tUnicodeText {
		s += p.take()
	}
	return &flowText{text: s, typ: typ}
}

func (p *parser) textNoTags() *flowText {
	t := p.kind()
	var typ labelKind
	switch {
	case t == tStr:
		typ = "text"
	case t == tMdStr:
		typ = "markdown"
	case noTagsTokens[t]:
		typ = "text"
	default:
		p.fail("'NODE_STRING', 'STR', 'MD_STR'")
	}
	s := p.take()
	for noTagsTokens[p.kind()] {
		s += p.take()
	}
	return &flowText{text: s, typ: typ}
}

func (p *parser) shapeData() string {
	p.dataFrom = p.i
	s := p.take()
	for p.kind() == tShapeData {
		s += p.take()
	}
	p.dataTo = p.i - 1
	return s
}

func (p *parser) attach(id, data string, from, to int) {
	l := p.span(from, to)
	p.db.addVertex(id, nil, "", nil, nil, &data, l)
	p.db.extendVertexMapping(id, l)
}

func (p *parser) attachData(id string, f *frame) {
	if f.data == nil {
		return
	}
	p.db.addVertex(id, nil, "", nil, nil, f.data, p.span(f.dataFrom, f.dataTo))
}

// bracketShapes are the shapes by the tokens that open and close them, for the shapes with
// nothing more to them.
var bracketShapes = map[int]struct {
	end   int
	shape string
}{
	tSqs:               {tSqe, "square"},
	tDoublecirclestart: {tDoublecircleend, "doublecircle"},
	tEllipseStart:      {tEllipseEnd, "ellipse"},
	tStadiumstart:      {tStadiumend, "stadium"},
	tSubroutinestart:   {tSubroutineend, "subroutine"},
	tCylinderstart:     {tCylinderend, "cylinder"},
	tTagend:            {tSqe, "odd"},
}

func (p *parser) vertex() string {
	from := p.i
	id := p.idString()
	var label *flowText
	typ := ""
	var props *jsobj.Object
	open := p.kind()
	if b, ok := bracketShapes[open]; ok {
		p.i++
		label = p.text()
		p.closed(b.end)
		typ = b.shape
	}
	switch open {
	case tPs:
		p.i++
		if p.kind() == tPs {
			p.i++
			label = p.text()
			p.closed(tPe)
			p.expect(tPe, "")
			typ = "circle"
		} else {
			label = p.text()
			p.closed(tPe)
			typ = "round"
		}
	case tVertexWithPropsStart:
		p.i++
		field := p.expect(tNodeString, "")
		p.expect(tColon, "")
		value := p.expect(tNodeString, "")
		p.expect(tPipe, "")
		label = p.text()
		p.closed(tSqe)
		typ = "rect"
		props = jsobj.New()
		props.Set(field, value)
	case tDiamondStart:
		p.i++
		if p.kind() == tDiamondStart {
			p.i++
			label = p.text()
			p.closed(tDiamondStop)
			p.expect(tDiamondStop, "")
			typ = "hexagon"
		} else {
			label = p.text()
			p.closed(tDiamondStop)
			typ = "diamond"
		}
	case tTrapstart:
		p.i++
		label = p.text()
		if p.kind() == tInvtrapend {
			p.i++
			typ = "lean_right"
		} else {
			p.closed(tTrapend)
			typ = "trapezoid"
		}
	case tInvtrapstart:
		p.i++
		label = p.text()
		if p.kind() == tTrapend {
			p.i++
			typ = "lean_left"
		} else {
			p.closed(tInvtrapend)
			typ = "inv_trapezoid"
		}
	}
	p.db.addVertex(id, label, typ, nil, props, nil, nil)
	p.db.addVertexMapping(id, typ, p.span(from, p.i-1))
	return id
}

func (p *parser) styledVertex() string {
	id := p.vertex()
	if p.kind() == tStyleSeparator {
		p.i++
		p.db.setClass(id, p.idString())
	}
	return id
}

func (p *parser) node() []string {
	ids := []string{p.styledVertex()}
	p.pendingData = nil
	for {
		var data *string
		if p.kind() == tShapeData {
			d := p.shapeData()
			data = &d
		}
		from, to := p.dataFrom, p.dataTo
		if p.kind() != tSpace {
			p.pendingData = data
			return ids
		}
		for p.i++; p.kind() == tSpace; p.i++ {
		}
		if p.kind() != tAmp {
			if data != nil {
				p.fail("'AMP'")
			}
			return ids
		}
		p.i++
		if p.kind() != tSpace {
			p.fail("'SPACE'")
		}
		for p.i++; p.kind() == tSpace; p.i++ {
		}
		next := p.styledVertex()
		if data != nil {
			p.attach(ids[len(ids)-1], *data, from, to)
		}
		ids = append(ids, next)
	}
}

func (p *parser) link() *linkInfo {
	id := ""
	if p.kind() == tLinkID {
		id = p.take()
	}
	switch p.kind() {
	case tLink:
		info := p.db.destructLink(p.take(), "")
		info.id = id
		if p.kind() == tPipe {
			p.i++
			info.text = p.text()
			p.closed(tPipe)
			if p.kind() == tSpace {
				p.i++
			}
		}
		return &info
	case tStartLink:
		start := p.take()
		label := p.edgeText()
		end := p.expect(tLink, "'EDGE_TEXT'")
		info := p.db.destructLink(end, start)
		info.text = label
		info.id = id
		return &info
	}
	p.fail("'LINK', 'START_LINK'")
	return nil
}

func (p *parser) vertexStatement() []string {
	from := p.i
	stmt := p.node()
	if p.pendingData != nil {
		p.attach(stmt[len(stmt)-1], *p.pendingData, p.dataFrom, p.dataTo)
	}
	nodes := stmt
	for {
		if t := p.kind(); t != tLink && t != tStartLink && t != tLinkID {
			return nodes
		}
		info := p.link()
		next := p.node()
		if p.pendingData != nil {
			p.attach(next[len(next)-1], *p.pendingData, p.dataFrom, p.dataTo)
		}
		p.db.addLink(stmt, next, info)
		p.db.addEdgeMapping(next, p.span(from, p.i-1))
		nodes = append(append([]string{}, next...), nodes...)
		stmt = next
	}
}

func (p *parser) style() string {
	return p.run(&styleTokens, "'NODE_STRING', 'NUM', 'COLON', 'BRKT'")
}

func (p *parser) stylesOpt() []string {
	out := []string{p.style()}
	for p.kind() == tComma {
		p.i++
		out = append(out, p.style())
	}
	return out
}

func (p *parser) linkStyleStatement() {
	db := p.db
	p.i++
	p.expect(tSpace, "")
	var positions []string
	if p.kind() == tDefault {
		positions = []string{p.take()}
	} else {
		positions = []string{p.expect(tNum, "'DEFAULT'")}
		for p.kind() == tComma {
			p.i++
			positions = append(positions, p.expect(tNum, ""))
		}
	}
	p.expect(tSpace, "")
	if p.kind() != tInterpolate {
		db.updateLink(positions, p.stylesOpt())
		return
	}
	p.i++
	p.expect(tSpace, "")
	interpolate := p.alphaNum()
	if p.kind() == tSpace {
		p.i++
		styles := p.stylesOpt()
		db.updateLinkInterpolate(positions, interpolate)
		db.updateLink(positions, styles)
	} else {
		db.updateLinkInterpolate(positions, interpolate)
	}
}

func (p *parser) linkTail(id, url string) {
	db := p.db
	if p.kind() != tSpace {
		db.setLink(id, url, nil)
		return
	}
	p.i++
	if p.kind() == tLinkTarget {
		target := p.take()
		db.setLink(id, url, &target)
		return
	}
	tooltip := p.expect(tStr, "'LINK_TARGET'")
	if p.kind() == tSpace {
		p.i++
		target := p.expect(tLinkTarget, "")
		db.setLink(id, url, &target)
	} else {
		db.setLink(id, url, nil)
	}
	db.setTooltip(id, tooltip)
}

func (p *parser) clickStatement() {
	db := p.db
	id := p.take()
	switch p.kind() {
	case tCallbackname:
		p.i++
		if p.kind() == tCallbackargs {
			p.i++
		}
		var tooltip *string
		if p.kind() == tSpace {
			p.i++
			t := p.expect(tStr, "")
			tooltip = &t
		}
		db.setClickEvent(id)
		if tooltip != nil {
			db.setTooltip(id, *tooltip)
		}
	case tHref:
		p.i++
		p.linkTail(id, p.expect(tStr, ""))
	case tStr:
		p.linkTail(id, p.take())
	default:
		p.alphaNum()
		if p.kind() == tSpace {
			p.i++
			tooltip := p.expect(tStr, "")
			db.setClickEvent(id)
			db.setTooltip(id, tooltip)
		} else {
			db.setClickEvent(id)
		}
	}
}

// header reads `flow` or `connector` up to the separator: an id, an optional bracketed
// title, optional metadata.
func (p *parser) header(at int) *frame {
	p.expect(tSpace, separators)
	id := p.textNoTags()
	title := &flowText{text: "", typ: "text"}
	if p.kind() == tSqs {
		p.i++
		title = p.text()
		p.closed(tSqe)
	}
	var data *string
	if p.kind() == tShapeData {
		d := p.shapeData()
		data = &d
	}
	if !separatorTokens[p.kind()] {
		p.fail(separators + ", 'SHAPE_DATA'")
	}
	return &frame{at: at, id: id, title: title, data: data, dataFrom: p.dataFrom, dataTo: p.dataTo}
}

// statement parses a statement and returns what it adds to an open block's list, or nothing.
func (p *parser) statement() (docItem, bool) {
	db := p.db
	t := p.kind()
	switch t {
	case tStyle:
		p.i++
		p.expect(tSpace, "")
		id := p.idString()
		p.expect(tSpace, "")
		db.addVertex(id, nil, "", p.stylesOpt(), nil, nil, nil)
		p.separator()
		return docItem{}, false
	case tLinkstyle:
		p.linkStyleStatement()
		p.separator()
		return docItem{}, false
	case tClassdef:
		p.i++
		p.expect(tSpace, "")
		id := p.idString()
		p.expect(tSpace, "")
		db.addClass(id, p.stylesOpt())
		p.separator()
		return docItem{}, false
	case tClass:
		p.i++
		p.expect(tSpace, "")
		ids := p.idString()
		p.expect(tSpace, "")
		db.setClass(ids, p.idString())
		p.separator()
		return docItem{}, false
	case tClick:
		p.clickStatement()
		p.separator()
		return docItem{}, false
	case tConnector:
		at := p.i
		p.i++
		f := p.header(at)
		sep := p.i
		p.i++
		id := db.addConnector(f.id, f.title)
		p.attachData(id, f)
		db.addConnectorMapping(f.id, p.span(f.at, f.at), p.span(sep, sep))
		return docItem{ids: []string{id}}, true
	case tDirectionTB, tDirectionBT, tDirectionRL, tDirectionLR, tDirectionTD:
		p.i++
		return docItem{dir: directions[t-tDirectionTB]}, true
	case tAccTitle:
		p.i++
		value := jsstr.Trim(p.expect(tAccTitleValue, ""))
		db.setAccTitle(value)
		return docItem{ids: []string{value}}, true
	case tAccDescr:
		p.i++
		value := jsstr.Trim(p.expect(tAccDescrValue, ""))
		db.setAccDescription(value)
		return docItem{ids: []string{value}}, true
	case tAccDescrMultilineValue:
		value := jsstr.Trim(p.take())
		db.setAccDescription(value)
		return docItem{ids: []string{value}}, true
	}
	if !idTokens[t] {
		p.fail(statementStart)
	}
	nodes := p.vertexStatement()
	p.separator()
	return docItem{ids: nodes}, len(nodes) > 0
}

// document parses the statements. Blocks nest without recursion: each open one keeps the list
// and collect flag of its parent.
func (p *parser) document() {
	db := p.db
	var open []*frame
	var list []docItem
	collect := false
loop:
	for {
		t := p.kind()
		switch {
		case t == tInputEnd:
			break loop
		case t == tEnd:
			if len(open) == 0 {
				break loop
			}
			f := open[len(open)-1]
			open = open[:len(open)-1]
			at := p.i
			p.i++
			if f.global {
				db.addGlobal(list)
				list = f.list
				collect = f.collect
				continue
			}
			id := db.addSubGraph(f.id, list, f.title)
			p.attachData(id, f)
			db.addSubgraphMapping(f.id, p.span(f.at, f.at), p.span(at, at))
			list = f.list
			collect = f.collect
			if collect {
				list = append(list, docItem{ids: []string{id}})
			}
		case t == tSemi || t == tNewline || t == tSpace || t == tEOF:
			if collect {
				list = append(list, docItem{ids: []string{p.toks.texts[p.i]}})
			}
			p.i++
		case t == tComment:
			p.i++
		case t == tFlow || t == tGlobal:
			at := p.i
			p.i++
			var f *frame
			if t == tGlobal || separatorTokens[p.kind()] {
				f = &frame{global: t == tGlobal, at: at}
				p.separator()
			} else {
				f = p.header(at)
				p.i++
			}
			f.list = list
			f.collect = collect
			open = append(open, f)
			list = nil
			collect = true
		default:
			item, ok := p.statement()
			if collect && ok {
				list = append(list, item)
			}
		}
	}
	if len(open) > 0 {
		p.fail("'end'")
	}
}

// parseAgentflow parses source into db. raw parses the text as it is, the way Mermaid's
// generated parser does without its wrapper.
func parseAgentflow(source string, db *agentflowDb, raw bool, o *perr.Origin) {
	text := common.Src(jsstr.UTF16(source))
	src := text
	var edits []perr.Edit
	if !raw && src.Index("}", 0) >= 0 {
		edits = braceTrails(src)
		src = trimBraceTrail(src, edits)
	}
	p := &parser{src: src, source: text, edits: edits, origin: o, toks: tokenize(src), db: db}

	for p.kind() == tSpace || p.kind() == tNewline || p.kind() == tComment {
		p.i++
	}
	p.expect(tGraph, "'SPACE', 'NEWLINE', 'COMMENT'")
	if p.kind() == tNodir {
		p.i++
		db.setDirection("TB")
	} else {
		dir := p.expect(tDir, "'NODIR'")
		t := p.kind()
		if t == tSpace {
			for p.i++; p.kind() == tSpace; p.i++ {
			}
			if p.kind() != tNewline {
				p.fail("'NEWLINE', 'SPACE'")
			}
		} else if t != tSemi && t != tNewline && t != tComment {
			p.fail("'COMMENT', 'SEMI', 'NEWLINE', 'SPACE'")
		}
		p.i++
		db.setDirection(dir)
	}
	p.document()
	if p.kind() != tInputEnd {
		p.fail(statementStart)
	}
}
