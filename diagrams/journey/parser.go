package journey

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

const statement = "'EOF', 'SPACE', 'NEWLINE', 'title', 'acc_title', 'acc_descr', 'acc_descr_multiline_value', 'section', 'taskName'"

func parseJourney(src common.Src, db *journeyDb, o *perr.Origin) {
	toks := tokenize(src)
	types, texts, starts := toks.Types, toks.Texts, toks.Starts

	fail := func(i int, expected string) {
		got := tokenNames[types[i]]
		if types[i] == tEnd {
			got = "1"
		}
		perr.Throw(perr.SyntaxError(o, "journey", src, starts[i], "Expecting "+expected+", got '"+got+"'", false))
	}

	if types[0] != tJourney {
		fail(0, "'journey'")
	}

	for i := 1; ; i++ {
		text := texts[i]
		switch types[i] {
		case tNewline:
		case tTitle:
			db.setDiagramTitle(substring(text, 6))
		case tAccTitle:
			i++
			db.setAccTitle(jsstr.Trim(texts[i]))
		case tAccDescr:
			i++
			db.setAccDescription(jsstr.Trim(texts[i]))
		case tAccDescrMultilineValue:
			db.setAccDescription(jsstr.Trim(text))
		case tSection:
			db.addSection(substring(text, 8))
		case tTaskName:
			i++
			if types[i] != tTaskData {
				fail(i, "'taskData'")
			}
			db.addTask(text, texts[i])
		case tEOF:
			return
		default:
			fail(i, statement)
		}
	}
}

// substring is s.substring(from).
func substring(s string, from int) string {
	return jsstr.Slice(s, from, jsstr.Len(s))
}
