package flowchart

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
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
		tLinkstyle, tClassdef, tClass, tClick, tGraph, tDir, tSubgraph, tEnd, tDown, tUp, tStartLink)
	styleTokens = tokenSet(tNum, tNodeString, tColon, tSpace, tBrkt, tStyle)
)

// frame is an open subgraph: its header, and the list and collect flag of its parent.
type frame struct {
	id, title *FlowText
	sameText  bool
	list      []docItem
	collect   bool
}

const textTokens = "'UNICODE_TEXT', 'TEXT', 'TAGSTART'"

var directions = [...]string{"TB", "BT", "RL", "LR", "TD"}

const statementStart = "'SEMI', 'NEWLINE', 'SPACE', 'EOF', 'subgraph', 'acc_title', 'acc_descr', 'acc_descr_multiline_value', 'STYLE', 'LINKSTYLE', 'CLASSDEF', 'CLASS', 'CLICK', 'NODE_STRING'"

// braceTrails returns the matches of /}\s*\n/g in src, each replaced by the two characters
// `}\n`: \s* backs off to the last line feed in the whitespace after a closing brace.
func braceTrails(src common.Src) []perr.Edit {
	var edits []perr.Edit
	for i := 0; i < len(src); i++ {
		if src[i] != '}' {
			continue
		}
		end := -1
		for j := i + 1; j < len(src) && util.IsSpace(int(src[j])); j++ {
			if src[j] == '\n' {
				end = j
			}
		}
		if end < 0 {
			continue
		}
		edits = append(edits, perr.Edit{From: i, To: end + 1, Length: 2})
		i = end
	}
	return edits
}

// trimBraceLines is source.replace(/}\s*\n/g, '}\n'), which Mermaid does before parsing.
func trimBraceLines(src common.Src, edits []perr.Edit) common.Src {
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
	// source is the text before trimBraceLines, edits what it changed, and origin locates
	// errors in source.
	source      common.Src
	edits       []perr.Edit
	origin      *perr.Origin
	toks        tokens
	i           int
	db          *FlowDb
	pendingData *string
}

func (p *parser) kind() int { return p.toks.types[p.i] }

func (p *parser) fail(expected string) {
	t := p.toks.types[p.i]
	at := perr.InputOffset([][]perr.Edit{p.edits}, p.toks.starts[p.i])
	if t == tError {
		perr.Throw(perr.SyntaxError(p.origin, "flowchart", p.source, at, "", true))
	}
	perr.Throw(perr.SyntaxError(p.origin, "flowchart", p.source, at, "Expecting "+expected+", got '"+tokenNames[t]+"'", false))
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
	if t := p.kind(); t != tNewline && t != tSemi && t != tEOF {
		p.fail("'SEMI', 'NEWLINE', 'EOF'")
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

func (p *parser) text() *FlowText {
	t := p.kind()
	var typ LabelType
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
	return &FlowText{Text: s, Type: typ}
}

func (p *parser) closed(typ int) {
	p.expect(typ, textTokens)
}

func (p *parser) edgeText() *FlowText {
	t := p.kind()
	var typ LabelType
	switch t {
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
	return &FlowText{Text: s, Type: typ}
}

func (p *parser) textNoTags() *FlowText {
	t := p.kind()
	var typ LabelType
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
	return &FlowText{Text: s, Type: typ}
}

func (p *parser) shapeData() string {
	s := p.take()
	for p.kind() == tShapeData {
		s += p.take()
	}
	return s
}

// shapes by the tokens that open and close them, for the shapes with nothing more to them.
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
	db := p.db
	id := p.idString()
	open := p.kind()
	if b, ok := bracketShapes[open]; ok {
		p.i++
		label := p.text()
		p.closed(b.end)
		db.addVertex(id, label, b.shape, nil, nil)
		return id
	}
	switch open {
	case tPs:
		p.i++
		if p.kind() == tPs {
			p.i++
			label := p.text()
			p.closed(tPe)
			p.expect(tPe, "")
			db.addVertex(id, label, "circle", nil, nil)
		} else {
			label := p.text()
			p.closed(tPe)
			db.addVertex(id, label, "round", nil, nil)
		}
	case tVertexWithPropsStart:
		p.i++
		p.expect(tNodeString, "")
		p.expect(tColon, "")
		p.expect(tNodeString, "")
		p.expect(tPipe, "")
		label := p.text()
		p.closed(tSqe)
		// Upstream also records the property, which nothing reads.
		db.addVertex(id, label, "rect", nil, nil)
	case tDiamondStart:
		p.i++
		if p.kind() == tDiamondStart {
			p.i++
			label := p.text()
			p.closed(tDiamondStop)
			p.expect(tDiamondStop, "")
			db.addVertex(id, label, "hexagon", nil, nil)
		} else {
			label := p.text()
			p.closed(tDiamondStop)
			db.addVertex(id, label, "diamond", nil, nil)
		}
	case tTrapstart:
		p.i++
		label := p.text()
		if p.kind() == tInvtrapend {
			p.i++
			db.addVertex(id, label, "lean_right", nil, nil)
		} else {
			p.closed(tTrapend)
			db.addVertex(id, label, "trapezoid", nil, nil)
		}
	case tInvtrapstart:
		p.i++
		label := p.text()
		if p.kind() == tTrapend {
			p.i++
			db.addVertex(id, label, "lean_left", nil, nil)
		} else {
			p.closed(tInvtrapend)
			db.addVertex(id, label, "inv_trapezoid", nil, nil)
		}
	default:
		db.addVertex(id, nil, "", nil, nil)
	}
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
			p.db.addVertex(ids[len(ids)-1], nil, "", nil, data)
		}
		ids = append(ids, next)
	}
}

func (p *parser) link() *LinkInfo {
	id := ""
	if p.kind() == tLinkID {
		id = p.take()
	}
	switch p.kind() {
	case tLink:
		info := p.db.destructLink(p.take(), "")
		info.ID = id
		if p.kind() == tPipe {
			p.i++
			info.Text = p.text()
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
		info.Text = label
		info.ID = id
		return &info
	}
	p.fail("'LINK', 'START_LINK'")
	return nil
}

func (p *parser) vertexStatement() []string {
	stmt := p.node()
	if p.pendingData != nil {
		p.db.addVertex(stmt[len(stmt)-1], nil, "", nil, p.pendingData)
	}
	nodes := stmt
	for {
		if t := p.kind(); t != tLink && t != tStartLink && t != tLinkID {
			return nodes
		}
		info := p.link()
		next := p.node()
		if p.pendingData != nil {
			p.db.addVertex(next[len(next)-1], nil, "", nil, p.pendingData)
		}
		p.db.addLink(stmt, next, info)
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
		db.setLink(id, url, "")
		return
	}
	p.i++
	if p.kind() == tLinkTarget {
		db.setLink(id, url, p.take())
		return
	}
	tooltip := p.expect(tStr, "'LINK_TARGET'")
	if p.kind() == tSpace {
		p.i++
		db.setLink(id, url, p.expect(tLinkTarget, ""))
	} else {
		db.setLink(id, url, "")
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

func (p *parser) subgraphHeader() *frame {
	p.i++
	t := p.kind()
	if t == tNewline || t == tSemi || t == tEOF {
		p.i++
		return &frame{}
	}
	p.expect(tSpace, "'SEMI', 'NEWLINE', 'EOF'")
	id := p.textNoTags()
	title := id
	same := true
	if p.kind() == tSqs {
		p.i++
		title = p.text()
		same = false
		p.closed(tSqe)
	}
	if t := p.kind(); t != tNewline && t != tSemi && t != tEOF {
		p.fail("'SEMI', 'NEWLINE', 'EOF', 'SQS'")
	}
	p.i++
	return &frame{id: id, title: title, sameText: same}
}

// statement parses a statement and returns what it adds to an open subgraph's list, or
// nothing.
func (p *parser) statement() (docItem, bool) {
	db := p.db
	t := p.kind()
	switch t {
	case tStyle:
		p.i++
		p.expect(tSpace, "")
		id := p.idString()
		p.expect(tSpace, "")
		db.addVertex(id, nil, "", p.stylesOpt(), nil)
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

// document parses the statements. Subgraphs nest without recursion: each open one keeps the
// list and collect flag of its parent.
func (p *parser) document() {
	var open []*frame
	var list []docItem
	collect := false
loop:
	for {
		t := p.kind()
		if t == tInputEnd {
			break
		}
		switch {
		case t == tEnd:
			if len(open) == 0 {
				break loop
			}
			f := open[len(open)-1]
			open = open[:len(open)-1]
			p.i++
			id := p.db.addSubGraph(f.id, list, f.title, f.sameText)
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
		case t == tSubgraph:
			f := p.subgraphHeader()
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

// parseFlowchart parses source into db, locating errors with o.
func parseFlowchart(source string, db *FlowDb, o *perr.Origin) {
	text := common.Src(jsstr.UTF16(source))
	src := text
	var edits []perr.Edit
	if src.Index("}", 0) >= 0 {
		edits = braceTrails(src)
		src = trimBraceLines(src, edits)
	}
	p := &parser{src: src, source: text, edits: edits, origin: o, toks: tokenize(src), db: db}

	for p.kind() == tSpace || p.kind() == tNewline {
		p.i++
	}
	p.expect(tGraph, "'SPACE', 'NEWLINE'")
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
		} else if t != tSemi && t != tNewline {
			p.fail("'SEMI', 'NEWLINE', 'SPACE'")
		}
		p.i++
		db.setDirection(dir)
	}
	p.document()
	if p.kind() != tInputEnd {
		p.fail(statementStart)
	}
}
