package treeview

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

type nodeType int

const (
	nodeFile nodeType = iota
	nodeDirectory
)

type treeNode struct {
	id       int
	level    int
	name     string
	nodeType nodeType
	// icon is an icon name as written (`pack:name`, `file`, `folder`), or `none` to hide the
	// icon. "" is upstream's undefined, as are cssClass and description.
	icon        string
	cssClass    string
	description string
	children    []*treeNode
}

type treeViewModel struct {
	// Empty strings stand for undefined: every use treats the two alike.
	title, accTitle, accDescr string
	// root is a directory named `/` at level -1 that holds the top-level entries.
	root  *treeNode
	count int
}

// treeViewDb is Mermaid's treeView database: nodes arrive in source order with their
// indentation.
type treeViewDb struct {
	count int
	root  *treeNode
	stack []*treeNode
}

func newTreeViewDb() *treeViewDb {
	root := &treeNode{level: -1, name: "/", nodeType: nodeDirectory}
	return &treeViewDb{count: 1, root: root, stack: []*treeNode{root}}
}

func (db *treeViewDb) addNode(level int, name string, typ nodeType, cssClass, icon, description string) {
	for level <= db.stack[len(db.stack)-1].level {
		db.stack = db.stack[:len(db.stack)-1]
	}
	node := &treeNode{id: db.count, level: level, name: name, nodeType: typ, icon: icon, cssClass: cssClass, description: description}
	db.count++
	parent := db.stack[len(db.stack)-1]
	parent.children = append(parent.children, node)
	db.stack = append(db.stack, node)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func populate(ast treeViewAst, db *treeViewDb) {
	for _, node := range ast.nodes {
		level := 0
		if node.indent != nil {
			level = *node.indent
		}
		name := node.name
		isDirectory := strings.HasSuffix(name, "/")
		if isDirectory {
			name = name[:len(name)-1]
		}
		icon := ""
		if node.iconAnnotation != nil {
			icon = *node.iconAnnotation
			if icon == "" {
				icon = "none"
			}
		}
		typ := nodeFile
		if isDirectory {
			typ = nodeDirectory
		}
		db.addNode(level, name, typ, deref(node.classAnnotation), icon, deref(node.descAnnotation))
	}
}

// readTreeView reads the source to a syntax tree, accepting box-drawing input. Errors name
// lines of the original text.
func readTreeView(source string, o *perr.Origin) treeViewAst {
	text, lineMap, columns := preprocessBoxDrawing(source, o)
	// The parser already located errors in unchanged text.
	if text == source {
		return parseTreeView(text, o)
	}
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		e, ok := r.(*perr.Error)
		if !ok {
			panic(r)
		}
		line, ok := lineMap[e.Line]
		if !ok {
			panic(r)
		}
		at := 0
		if e.Column > 0 {
			if column, ok := columns[e.Line]; ok {
				at = column(e.Column - 1)
			} else {
				at = e.Column - 1
			}
		}
		units := jsstr.UTF16(source)
		written := perr.Locate(o, units, lineStart(units, line)+at)
		head := strings.IndexByte(e.Message, '\n')
		if head == -1 {
			head = len(e.Message)
		}
		rest := e.Message[head:]
		if e.Snippet != "" {
			rest = strings.Replace(rest, e.Snippet, written.Snippet, 1)
		}
		message := remapErrorLines(e.Message[:head], map[int]int{e.Line: written.Line}) + rest
		column, snippet := 0, ""
		if e.Column > 0 {
			column = written.Column
		}
		if e.Snippet != "" {
			snippet = written.Snippet
		}
		perr.Throw(&perr.Error{Code: e.Code, Message: message, Type: e.Type, Line: written.Line, Column: column, Snippet: snippet})
	}()
	return parseTreeView(text, o)
}

func buildTreeView(ast treeViewAst, title *string) treeViewModel {
	db := newTreeViewDb()
	populate(ast, db)
	m := treeViewModel{root: db.root, count: db.count}
	if ast.title != nil && *ast.title != "" {
		m.title = *ast.title
	} else if title != nil {
		m.title = *title
	}
	if ast.accTitle != nil && *ast.accTitle != "" {
		m.accTitle = jsstr.TrimStart(*ast.accTitle)
	}
	if ast.accDescr != nil && *ast.accDescr != "" {
		m.accDescr = common.CollapseIndentedBreaks(*ast.accDescr)
	}
	return m
}
