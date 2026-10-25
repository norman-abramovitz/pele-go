package architecture

import (
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// Token kinds, in the order of architectureTokens.
const (
	kindHeader = iota
	kindJunction
	kindService
	kindColumn
	kindGroup
	kindAlign
	kindRow
	kindDashes
	kindIn
	kindColon
	kindDash
	kindDirection
	kindArrowGroup
	kindInto
	kindAccDescr
	kindAccTitle
	kindTitle
	kindString
	kindID
	kindNewline
	kindWhitespace
	kindYaml
	kindDirective
	kindComment
	kindIcon
	kindLabel
)

// idToken is /[\w]([-\w]*\w)?/y.
var idToken = &common.TokenType{Name: "ID", Match: func(src common.Src, at int) int {
	if !util.IsWord(src.At(at)) {
		return -1
	}
	end := at + 1
	for i := end; util.IsWord(src.At(i)) || src.At(i) == '-'; i++ {
		if src.At(i) != '-' {
			end = i + 1
		}
	}
	return end
}}

func word(text string) *common.TokenType {
	return &common.TokenType{Name: text, Literal: text, Longer: []*common.TokenType{idToken}}
}

func oneOf(name, chars string) *common.TokenType {
	return &common.TokenType{Name: name, Match: func(src common.Src, at int) int {
		if c := src.At(at); c >= 0 && c < 128 && strings.IndexByte(chars, byte(c)) >= 0 {
			return at + 1
		}
		return -1
	}}
}

// quotedLabel matches the quote alternatives of ARCH_TITLE, /"([^"\\]|\\.)*"/ and its
// single-quoted twin, from the quote at at, followed by the closing bracket.
func quotedLabel(src common.Src, at int, quote int) int {
	i := at + 1
	for i < len(src) {
		c := int(src[i])
		if c == quote {
			break
		}
		if c == '\\' {
			if i+1 < len(src) && !jsstr.IsLineEnd(rune(src[i+1])) {
				i += 2
				continue
			}
			return -1
		}
		i++
	}
	if src.At(i) == quote && src.At(i+1) == ']' {
		return i + 2
	}
	return -1
}

// The direction letters come before identifiers and have no longer alternative, so Mermaid
// reads an id that starts with L, R, T or B as a direction followed by the rest.
var architectureLexer = common.NewLexer([]*common.TokenType{
	word("architecture-beta"),
	word("junction"),
	word("service"),
	word("column"),
	word("group"),
	word("align"),
	word("row"),
	{Name: "--", Literal: "--", Longer: []*common.TokenType{common.Yaml}},
	word("in"),
	{Name: ":", Literal: ":"},
	{Name: "-", Literal: "-", Longer: []*common.TokenType{common.Yaml}},
	oneOf("ARROW_DIRECTION", "LRTB"),
	{Name: "ARROW_GROUP", Match: func(src common.Src, at int) int {
		if src.HasPrefix(at, "{group}") {
			return at + len("{group}")
		}
		return -1
	}},
	oneOf("ARROW_INTO", "<>"),
	common.AccDescr,
	common.AccTitle,
	common.Title,
	common.String,
	idToken,
	common.Newline,
	common.Whitespace,
	common.Yaml,
	common.Directive,
	common.SingleLineComment,
	// /\([\w-:]+\)/y
	{Name: "ARCH_ICON", Match: func(src common.Src, at int) int {
		if src.At(at) != '(' {
			return -1
		}
		i := at + 1
		for c := src.At(i); util.IsWord(c) || c == '-' || c == ':'; c = src.At(i) {
			i++
		}
		if i == at+1 || src.At(i) != ')' {
			return -1
		}
		return i + 1
	}},
	// /\[(?:"([^"\\]|\\.)*"|'([^'\\]|\\.)*'|[^\[\]\r\n]+)\]/y
	{Name: "ARCH_TITLE", Match: func(src common.Src, at int) int {
		if src.At(at) != '[' {
			return -1
		}
		if q := src.At(at + 1); q == '"' || q == '\'' {
			if end := quotedLabel(src, at+1, q); end != -1 {
				return end
			}
		}
		i := at + 1
		for c := src.At(i); c != -1 && c != '[' && c != ']' && c != '\r' && c != '\n'; c = src.At(i) {
			i++
		}
		if i == at+1 || src.At(i) != ']' {
			return -1
		}
		return i + 1
	}},
})

type side = byte

type groupAst struct {
	// Empty strings stand for undefined: an icon and an id are never empty, and an empty
	// title draws as no title.
	id, icon, title, in string
}

type serviceAst struct {
	id, icon, iconText, title, in string
}

type junctionAst struct {
	id, in string
}

type edgeAst struct {
	lhsID    string
	lhsGroup bool
	lhsDir   side
	lhsInto  bool
	title    string
	rhsInto  bool
	rhsDir   side
	rhsID    string
	rhsGroup bool
}

type alignmentAst struct {
	direction string
	members   []string
}

type architectureAst struct {
	title, accTitle, accDescr string
	groups                    []groupAst
	services                  []serviceAst
	junctions                 []junctionAst
	edges                     []edgeAst
	alignments                []alignmentAst
}

// labelValue is Mermaid's ArchitectureValueConverter for ARCH_TITLE.
func labelValue(text string) string {
	result := jsstr.Trim(text[1 : len(text)-1])
	if strings.HasPrefix(result, `"`) && strings.HasSuffix(result, `"`) || strings.HasPrefix(result, "'") && strings.HasSuffix(result, "'") {
		if len(result) < 2 {
			result = ""
		} else {
			result = result[1 : len(result)-1]
		}
		result = strings.ReplaceAll(strings.ReplaceAll(result, `\"`, `"`), `\'`, "'")
	}
	return jsstr.Trim(result)
}

func parseArchitecture(src string, o *perr.Origin) architectureAst {
	r := common.NewReader(architectureLexer.Tokenize(jsstr.UTF16(src), "architecture", o), architectureLexer.Types(), "architecture", o)
	var a architectureAst

	is := func(kind int) bool { return r.Kind() == kind }
	endOfLine := func() {
		if r.Kind() == -1 {
			return
		}
		if r.Kind() != kindNewline {
			r.Fail("a line break or the end of input")
		}
		for r.Kind() == kindNewline {
			r.I++
		}
	}

	for r.Kind() == kindNewline {
		r.I++
	}
	r.Expect(kindHeader)
	for r.Kind() != -1 {
		switch r.Kind() {
		case kindNewline:
			r.I++
			continue
		case kindAccDescr:
			a.accDescr = common.AccDescrValue(r.Take())
		case kindAccTitle:
			a.accTitle = common.AccTitleValue(r.Take())
		case kindTitle:
			a.title = common.TitleValue(r.Take())
		case kindGroup:
			r.I++
			group := groupAst{id: r.Expect(kindID)}
			if is(kindIcon) {
				t := r.Take()
				group.icon = t[1 : len(t)-1]
			}
			if is(kindLabel) {
				group.title = labelValue(r.Take())
			}
			if r.Accept(kindIn) {
				group.in = r.Expect(kindID)
			}
			a.groups = append(a.groups, group)
		case kindService:
			r.I++
			service := serviceAst{id: r.Expect(kindID)}
			if is(kindString) {
				service.iconText = common.StringValue(r.Take())
			} else if is(kindIcon) {
				t := r.Take()
				service.icon = t[1 : len(t)-1]
			}
			if is(kindLabel) {
				service.title = labelValue(r.Take())
			}
			if r.Accept(kindIn) {
				service.in = r.Expect(kindID)
			}
			a.services = append(a.services, service)
		case kindJunction:
			r.I++
			junction := junctionAst{id: r.Expect(kindID)}
			if r.Accept(kindIn) {
				junction.in = r.Expect(kindID)
			}
			a.junctions = append(a.junctions, junction)
		case kindID:
			var e edgeAst
			e.lhsID = r.Take()
			e.lhsGroup = r.Accept(kindArrowGroup)
			r.Expect(kindColon)
			e.lhsDir = r.Expect(kindDirection)[0]
			e.lhsInto = r.Accept(kindInto)
			if !r.Accept(kindDashes) {
				if !is(kindDash) {
					r.Fail("'--' or '-'")
				}
				r.I++
				e.title = labelValue(r.Expect(kindLabel))
				r.Expect(kindDash)
			}
			e.rhsInto = r.Accept(kindInto)
			e.rhsDir = r.Expect(kindDirection)[0]
			r.Expect(kindColon)
			e.rhsID = r.Expect(kindID)
			e.rhsGroup = r.Accept(kindArrowGroup)
			a.edges = append(a.edges, e)
		case kindAlign:
			r.I++
			if !is(kindRow) && !is(kindColumn) {
				r.Fail("'row' or 'column'")
			}
			direction := r.Take()
			members := []string{r.Expect(kindID), r.Expect(kindID)}
			for is(kindID) {
				members = append(members, r.Take())
			}
			a.alignments = append(a.alignments, alignmentAst{direction: direction, members: members})
		default:
			r.Fail("a group, a service, a junction, an edge, an alignment, a title, or a line break")
		}
		endOfLine()
	}
	return a
}
