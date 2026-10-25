package treeview

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// Token kinds, in the order of treeViewTokens.
const (
	kindKeyword = iota
	kindAccDescr
	kindAccTitle
	kindTitle
	kindClassAnnotation
	kindIconAnnotation
	kindDescAnnotation
	kindIndentation
	kindQuotedName
	kindWS
	kindComment
	kindNewline
	kindBareName
)

func skipBlanks(src common.Src, i int) int {
	for util.IsBlank(src.At(i)) {
		i++
	}
	return i
}

func annotationAt(src common.Src, at int) bool {
	if src.HasPrefix(at, "##") || src.HasPrefix(at, "icon(") {
		return true
	}
	if !src.HasPrefix(at, ":::") {
		return false
	}
	c := src.At(skipBlanks(src, at+3))
	return (c >= 65 && c <= 90) || (c >= 97 && c <= 122) || c == 95
}

// matchBareName is
// /(?!:::|icon\(|##)[^ \t\n\r"'](?:(?![ \t]+:::[ \t]*[A-Za-z_]|[ \t]+icon\(|[ \t]+##)[^\n\r])*/y.
// The pattern looks ahead from every blank for an annotation, which rescans long runs of
// blanks. This looks once per run.
func matchBareName(src common.Src, at int) int {
	first := src.At(at)
	if util.IsBlank(first) || first == 10 || first == 13 || first == 34 || first == 39 {
		return -1
	}
	if src.HasPrefix(at, ":::") || src.HasPrefix(at, "icon(") || src.HasPrefix(at, "##") {
		return -1
	}
	n := len(src)
	i := at + 1
	for i < n {
		c := int(src[i])
		if c == 10 || c == 13 {
			break
		}
		if util.IsBlank(c) {
			j := skipBlanks(src, i+1)
			if annotationAt(src, j) {
				break
			}
			i = j
		} else {
			i++
		}
	}
	return i
}

var bareName = &common.TokenType{Name: "BARE_NAME", Match: matchBareName}

func isNameChar(c int) bool { return util.IsWord(c) || c == '-' }

// classAnnotation is /[ \t]+:::[ \t]*[A-Za-z_][\w-]*/y.
func classAnnotation(src common.Src, at int) int {
	i := skipBlanks(src, at)
	if i == at || !src.HasPrefix(i, ":::") {
		return -1
	}
	i = skipBlanks(src, i+3)
	if c := src.At(i); !(util.IsLetter(c) || c == '_') {
		return -1
	}
	i++
	for isNameChar(src.At(i)) {
		i++
	}
	return i
}

// iconAnnotation is /[ \t]+icon\([\w-]*(?::[\w-]+)?\)/y.
func iconAnnotation(src common.Src, at int) int {
	i := skipBlanks(src, at)
	if i == at || !src.HasPrefix(i, "icon(") {
		return -1
	}
	i += len("icon(")
	for isNameChar(src.At(i)) {
		i++
	}
	if src.At(i) == ':' && isNameChar(src.At(i+1)) {
		i += 2
		for isNameChar(src.At(i)) {
			i++
		}
	}
	if src.At(i) != ')' {
		return -1
	}
	return i + 1
}

// descAnnotation is /[ \t]+##[^\n\r]*/y.
func descAnnotation(src common.Src, at int) int {
	i := skipBlanks(src, at)
	if i == at || !src.HasPrefix(i, "##") {
		return -1
	}
	i += 2
	for i < len(src) && src[i] != '\n' && src[i] != '\r' {
		i++
	}
	return i
}

// quotedName is /"[^"]*"|'[^']*'/y.
func quotedName(src common.Src, at int) int {
	q := src.At(at)
	if q != '"' && q != '\'' {
		return -1
	}
	for i := at + 1; i < len(src); i++ {
		if int(src[i]) == q {
			return i + 1
		}
	}
	return -1
}

// comment is /\%\%[^\n]*/y.
func comment(src common.Src, at int) int {
	if !src.HasPrefix(at, "%%") {
		return -1
	}
	i := at + 2
	for i < len(src) && src[i] != '\n' {
		i++
	}
	return i
}

var keyword = func() *common.TokenType {
	k := common.Keyword("treeView-beta")
	k.Longer = []*common.TokenType{bareName}
	return k
}()

var treeViewLexer = common.NewLexer([]*common.TokenType{
	keyword,
	common.AccDescrScanner(""),
	common.AccTitle,
	common.Title,
	{Name: "CLASS_ANNOTATION", Match: classAnnotation},
	{Name: "ICON_ANNOTATION", Match: iconAnnotation},
	{Name: "DESC_ANNOTATION", Match: descAnnotation},
	{Name: "INDENTATION", Match: common.Whitespace.Match},
	{Name: "QUOTED_NAME", Match: quotedName},
	{Name: "WS", Match: common.Whitespace.Match, Hidden: true},
	{Name: "ML_COMMENT", Match: comment, Hidden: true},
	{Name: "NL", Match: common.Newline.Match, Hidden: true},
	bareName,
})

type treeNodeAst struct {
	indent                                          *int
	name                                            string
	classAnnotation, iconAnnotation, descAnnotation *string
}

type treeViewAst struct {
	title, accTitle, accDescr *string
	nodes                     []treeNodeAst
}

func trimBlanksEnd(text string) string {
	end := len(text)
	for end > 0 && util.IsBlank(int(text[end-1])) {
		end--
	}
	return text[:end]
}

// convertValue is Mermaid's TreeViewValueConverter for the string terminals, by name.
func convertValue(rule, input string) string {
	switch rule {
	case "QUOTED_NAME":
		return input[1 : len(input)-1]
	case "BARE_NAME":
		return trimBlanksEnd(input)
	case "CLASS_ANNOTATION":
		return jsstr.Trim(jsstr.Trim(input)[3:])
	case "ICON_ANNOTATION":
		trimmed := jsstr.Trim(input)
		return trimmed[5 : len(trimmed)-1]
	case "DESC_ANNOTATION":
		return jsstr.Trim(jsstr.Trim(input)[2:])
	}
	return ""
}

func parseTreeView(src string, o *perr.Origin) treeViewAst {
	r := common.NewReader(treeViewLexer.Tokenize(jsstr.UTF16(src), "treeView", o), treeViewLexer.Types(), "treeView", o)
	var ast treeViewAst
	r.Expect(kindKeyword)
	for {
		switch r.Kind() {
		case kindAccDescr:
			ast.accDescr = new(common.AccDescrValue(r.Take()))
			continue
		case kindAccTitle:
			ast.accTitle = new(common.AccTitleValue(r.Take()))
			continue
		case kindTitle:
			ast.title = new(common.TitleValue(r.Take()))
			continue
		}
		break
	}
	for r.Kind() != -1 {
		var node treeNodeAst
		if r.Kind() == kindIndentation {
			node.indent = new(len(r.Take()))
		}
		switch r.Kind() {
		case kindQuotedName:
			node.name = convertValue("QUOTED_NAME", r.Take())
		case kindBareName:
			node.name = convertValue("BARE_NAME", r.Take())
		default:
			r.Fail("a name")
		}
	annotations:
		for {
			switch r.Kind() {
			case kindClassAnnotation:
				node.classAnnotation = new(convertValue("CLASS_ANNOTATION", r.Take()))
			case kindIconAnnotation:
				node.iconAnnotation = new(convertValue("ICON_ANNOTATION", r.Take()))
			case kindDescAnnotation:
				node.descAnnotation = new(convertValue("DESC_ANNOTATION", r.Take()))
			default:
				break annotations
			}
		}
		ast.nodes = append(ast.nodes, node)
	}
	return ast
}
