package block

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

type nodeInfo struct {
	id, label, typeStr string
	directions         []string
}

// A statement list is kept flat: upstream's lists of statements, where a link statement is a
// nested list, are only ever read flattened.
type openFrame struct {
	list []*blockNode
	// header is the block statement that opened the composite, nil for a bare `block`.
	header *blockNode
	// chained is set when the header was a link statement, which leaves the composite
	// none of its fields.
	chained bool
}

const statementStart = "'NODE_ID', 'COLUMNS', 'SPACE_BLOCK', 'BLOCK_DIAGRAM_KEY', 'id-block', 'classDef', 'class', 'style'"

func parseBlock(source string, db *blockDb, o *perr.Origin) {
	src := common.Src(jsstr.UTF16(source))
	toks := tokenize(src)
	types, texts, starts := toks.types, toks.texts, toks.starts
	i := 0

	fail := func(expected string) {
		t := types[i]
		if t == tError {
			perr.Throw(perr.SyntaxError(o, "block", src, starts[i], "", true))
		}
		perr.Throw(perr.SyntaxError(o, "block", src, starts[i], "Expecting "+expected+", got '"+tokenNames[t]+"'", false))
	}

	expect := func(typ int) string {
		if types[i] != typ {
			fail("'" + tokenNames[typ] + "'")
		}
		i++
		return texts[i-1]
	}

	node := func() nodeInfo {
		id := expect(tNodeID)
		if types[i] == tNodeDStart {
			open := texts[i]
			i++
			label := expect(tStr)
			return nodeInfo{id: id, label: label, typeStr: open + expect(tNodeDEnd)}
		}
		if types[i] == tBlockArrowStart {
			open := texts[i]
			i++
			label := expect(tStr)
			dirs := []string{expect(tDir)}
			for types[i] == tDir {
				dirs = append(dirs, texts[i])
				i++
			}
			return nodeInfo{id: id, label: label, typeStr: open + expect(tBlockArrowEnd), directions: dirs}
		}
		return nodeInfo{id: id}
	}

	newBlock := func(info nodeInfo) *blockNode {
		return &blockNode{
			id: info.id, hasID: true, label: info.label, typ: typeStr2Type(info.typeStr), directions: info.directions,
			columns: -1, widthInColumns: 1, colorIndex: -1,
		}
	}

	// nodeStatement returns the statement's blocks, and whether it was a link statement.
	nodeStatement := func() ([]*blockNode, bool) {
		first := node()
		size := 1.0
		if types[i] == tSize {
			size = jsnum.Parse(texts[i])
			i++
		}
		from := newBlock(first)
		from.widthInColumns = size
		var chain []*blockNode
		for types[i] == tLink || types[i] == tStartLink {
			label := ""
			i++
			if types[i-1] == tStartLink {
				expect(tLinkLabel)
				label = expect(tStr)
				expect(tLink)
			}
			link := texts[i-1]
			to := node()
			arrowTypeEnd := edgeStrToEdgeData(link)
			arrowTypeStart := edgeStrToEdgeStartData(link)
			thickness := edgeStrToThickness(link)
			pattern := edgeStrToPattern(link)
			// Mermaid keeps only the last link of `A --> B --> C` and loses its start. Here every link counts.
			if chain == nil {
				head := newBlock(first)
				head.typ = from.typ
				chain = []*blockNode{head}
			}
			from = newBlock(to)
			prev := chain[len(chain)-1].id
			chain = append(chain, &blockNode{
				id: prev + "-" + to.id, hasID: true, start: prev, end: to.id, label: label, typ: "edge",
				thickness: thickness, pattern: pattern, directions: to.directions,
				arrowTypeEnd: arrowTypeEnd, arrowTypeStart: arrowTypeStart,
				columns: -1, widthInColumns: 1, colorIndex: -1,
			}, from)
		}
		if chain != nil {
			return chain, true
		}
		return []*blockNode{from}, false
	}

	expect(tKey)
	// Composites nest without recursion: each open one keeps the statement list of its parent.
	var open []openFrame
	var list []*blockNode
	for {
		switch types[i] {
		case tNodeID:
			blocks, _ := nodeStatement()
			list = append(list, blocks...)
		case tColumns:
			list = append(list, &blockNode{typ: "column-setting", columns: jsnum.Parse(texts[i]), widthInColumns: 1, colorIndex: -1})
			i++
		case tSpaceBlock:
			width := jsnum.Parse(texts[i])
			i++
			list = append(list, &blockNode{id: db.generateId(), hasID: true, typ: "space", width: width, columns: -1, widthInColumns: 1, colorIndex: -1})
		case tKey:
			i++
			open = append(open, openFrame{list: list})
			list = nil
		case tIDBlock:
			i++
			if types[i] != tNodeID {
				fail("'NODE_ID'")
			}
			blocks, chained := nodeStatement()
			open = append(open, openFrame{list: list, header: blocks[0], chained: chained})
			list = nil
		case tEnd:
			if len(open) == 0 || len(list) == 0 {
				fail(statementStart)
			}
			f := open[len(open)-1]
			open = open[:len(open)-1]
			i++
			id := db.generateId()
			children := list
			list = f.list
			switch {
			case f.header == nil:
				list = append(list, &blockNode{id: id, hasID: true, typ: "composite", children: children, columns: -1, widthInColumns: 1, colorIndex: -1})
			case f.chained:
				list = append(list, &blockNode{typ: "composite", children: children, columns: -1, colorIndex: -1, widthInColumns: 1})
			default:
				composite := *f.header
				composite.typ = "composite"
				composite.children = children
				list = append(list, &composite)
			}
		case tClassDef:
			i++
			id := jsstr.Trim(expect(tClassDefID))
			list = append(list, &blockNode{typ: "classDef", id: id, hasID: true, css: jsstr.Trim(expect(tClassDefStyleOpts)), columns: -1, widthInColumns: 1, colorIndex: -1})
		case tClass:
			i++
			id := jsstr.Trim(expect(tClassEntityIDs))
			list = append(list, &blockNode{typ: "applyClass", id: id, hasID: true, styleClass: jsstr.Trim(expect(tStyleClass)), columns: -1, widthInColumns: 1, colorIndex: -1})
		case tStyle:
			i++
			id := jsstr.Trim(expect(tStyleEntityIDs))
			list = append(list, &blockNode{typ: "applyStyles", id: id, hasID: true, stylesStr: jsstr.Trim(expect(tStyleDefinitionData)), columns: -1, widthInColumns: 1, colorIndex: -1})
		case tEOF:
			if len(open) > 0 {
				fail(statementStart + ", 'end'")
			}
			if len(list) == 0 {
				fail(statementStart)
			}
			db.setHierarchy(list)
			return
		default:
			switch {
			case len(list) == 0:
				fail(statementStart)
			case len(open) > 0:
				fail(statementStart + ", 'end'")
			default:
				fail(statementStart + ", 'EOF'")
			}
		}
	}
}
