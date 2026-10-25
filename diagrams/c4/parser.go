package c4

import (
	"slices"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// Mermaid's typeC4Shape for each shape keyword, indexed from tPERSON.
var kinds = [...]string{
	"person",
	"external_person",
	"system",
	"system_db",
	"system_queue",
	"external_system",
	"external_system_db",
	"external_system_queue",
	"container",
	"container_db",
	"container_queue",
	"external_container",
	"external_container_db",
	"external_container_queue",
	"component",
	"component_db",
	"component_queue",
	"external_component",
	"external_component_db",
	"external_component_queue",
}

var (
	relTypes  = [...]string{"rel", "birel", "rel_u", "rel_d", "rel_l", "rel_r", "rel_b", "rel"}
	nodeTypes = [...]string{"node", "nodeL", "nodeR"}
)

const (
	headers    = "'direction_tb', 'direction_bt', 'direction_rl', 'direction_lr', 'C4_CONTEXT', 'C4_CONTAINER', 'C4_COMPONENT', 'C4_DYNAMIC', 'C4_DEPLOYMENT'"
	attributes = "'STR', 'STR_KEY', 'ATTRIBUTE', 'ATTRIBUTE_EMPTY'"
)

// Parser states.
const (
	// A statement is required.
	pItem = iota
	// Only a diagram statement may come next: the first one in a boundary, or one written
	// straight after a title.
	pDiagram
	pItemOrEnd
	pAfterOther
	pAfterDiagram
)

func isOther(t int) bool {
	return t == tTitle || t == tAccDescription || t == tAccTitle || t == tAccDescr || t == tAccDescrMultilineValue
}

func isDiagram(t int) bool {
	return t >= tPERSON && t <= tUpdateLayoutConfig
}

// parseC4 accepts what the parser Jison generates from c4Diagram.jison accepts, and calls the
// model builder in the order its semantic actions do. Boundaries nest through a counter, not
// recursion.
func parseC4(source string, db *c4Db, o *perr.Origin) {
	src := common.Src(jsstr.UTF16(source))
	toks := tokenize(src)
	types, starts, ends := toks.types, toks.starts, toks.ends
	i := 0

	text := func(at int) string { return src.String(starts[at], ends[at]) }
	fail := func(expected string) {
		t := types[i]
		if t == tERROR {
			perr.Throw(perr.SyntaxError(o, "c4", src, starts[i], "", true))
		}
		perr.Throw(perr.SyntaxError(o, "c4", src, starts[i], "Expecting "+expected+", got '"+tokenNames[t]+"'", false))
	}
	expect := func(typ int) {
		if types[i] != typ {
			fail("'" + tokenNames[typ] + "'")
		}
		i++
	}

	readAttributes := func() []attr {
		var attrs []attr
		for {
			t := types[i]
			if t == tSTR {
				attrs = append(attrs, attr{s: jsstr.Trim(text(i))})
				i++
			} else if t == tAttributeEmpty {
				attrs = append(attrs, attr{})
				i++
			} else if t == tStrKey {
				i++
				if types[i] != tStrValue {
					fail("'STR_VALUE'")
				}
				attrs = append(attrs, attr{n: &named{key: jsstr.Trim(text(i - 1)), value: jsstr.Trim(text(i))}})
				i++
			} else {
				break
			}
		}
		if len(attrs) == 0 {
			fail(attributes)
		}
		return attrs
	}

	other := func(t int) {
		switch t {
		case tTitle:
			db.setTitle(src.String(starts[i]+6, ends[i]))
			i++
		case tAccDescription:
			db.setAccDescription(src.String(starts[i]+15, ends[i]))
			i++
		case tAccDescrMultilineValue:
			db.setAccDescription(jsstr.Trim(text(i)))
			i++
		default:
			i++
			if t == tAccTitle {
				if types[i] != tAccTitleValue {
					fail("'acc_title_value'")
				}
				// The grammar hands `accTitle:` to setTitle, so it becomes the visible title.
				db.setTitle(jsstr.Trim(text(i)))
			} else {
				if types[i] != tAccDescrValue {
					fail("'acc_descr_value'")
				}
				db.setAccDescription(jsstr.Trim(text(i)))
			}
			i++
		}
	}

	// diagram reports whether the statement opens a boundary.
	diagram := func(t int) bool {
		i++
		attrs := readAttributes()
		switch {
		case t <= tSystemExtQueue:
			db.addPersonOrSystem(kinds[t-tPERSON], attrs)
		case t <= tContainerExtQueue:
			db.addContainer(kinds[t-tPERSON], attrs)
		case t <= tComponentExtQueue:
			db.addComponent(kinds[t-tPERSON], attrs)
		case t <= tNodeR:
			// attrs.splice(2, 0, kind) inserts at the end of a shorter list.
			at := min(2, len(attrs))
			switch t {
			case tEnterpriseBoundary:
				attrs = slices.Insert(attrs, at, attr{s: "ENTERPRISE"})
			case tSystemBoundary:
				attrs = slices.Insert(attrs, at, attr{s: "SYSTEM"})
			case tContainerBoundary:
				attrs = slices.Insert(attrs, at, attr{s: "CONTAINER"})
			}
			if t >= tNODE {
				db.addDeploymentNode(nodeTypes[t-tNODE], attrs)
			} else if t == tContainerBoundary {
				db.addContainerBoundary(attrs)
			} else {
				db.addPersonOrSystemBoundary(attrs, "system")
			}
			switch types[i] {
			case tLBRACE:
				i++
				expect(tNEWLINE)
			case tNEWLINE:
				i++
				expect(tLBRACE)
				if types[i] == tNEWLINE {
					i++
				}
			default:
				fail("'NEWLINE', 'LBRACE'")
			}
			return true
		case t <= tRelIndex:
			// RelIndex takes a sequence number first, which Mermaid ignores.
			if t == tRelIndex {
				attrs = attrs[1:]
			}
			db.addRel(relTypes[t-tREL], attrs)
		case t == tUpdateElStyle:
			db.updateElStyle(attrs)
		case t == tUpdateRelStyle:
			db.updateRelStyle(attrs)
		default:
			db.updateLayoutConfig(attrs)
		}
		return false
	}

	if header := types[i]; header < tC4Context || header > tC4Deployment {
		fail(headers)
	}
	i++
	expect(tNEWLINE)

	depth := 0
	state := pItem
	for {
		t := types[i]
		if state == pAfterOther || state == pAfterDiagram {
			if t == tNEWLINE {
				i++
				state = pItemOrEnd
				continue
			}
			if state == pAfterOther && isDiagram(t) {
				state = pDiagram
				continue
			}
		} else if isDiagram(t) {
			if diagram(t) {
				depth++
				state = pDiagram
			} else {
				state = pAfterDiagram
			}
			continue
		} else if isOther(t) && state != pDiagram {
			other(t)
			state = pAfterOther
			continue
		} else if state != pItemOrEnd {
			fail("'title', 'accDescription', 'acc_title', 'acc_descr', 'acc_descr_multiline_value', 'PERSON', 'REL'")
		}

		if depth > 0 {
			if t != tRBRACE {
				fail("'NEWLINE', 'RBRACE'")
			}
			i++
			db.popBoundaryParseStack()
			depth--
			state = pAfterDiagram
		} else {
			if t != tEOF {
				fail("'NEWLINE', 'EOF'")
			}
			i++
			expect(tEND)
			db.setC4Type(text(0))
			return
		}
	}
}
