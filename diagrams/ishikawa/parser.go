package ishikawa

import (
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

const stop = "'SPACELINE', 'NL', 'EOF'"

// parseIshikawa follows the parser Jison builds from Mermaid's grammar: blank lines and
// comments may come first, then the keyword and at most one line break, then statements. A
// statement is a line of text, with or without indentation, or a blank line; each is ended by
// a blank line, a line break or the end of the text, and any number of line breaks after
// that.
func parseIshikawa(source string, db *ishikawaDb, o *perr.Origin) {
	src := jsstr.UTF16(source)
	t := tokenize(src)
	types, starts, ends := t.types, t.starts, t.ends
	i := 0

	fail := func(expected string) {
		perr.Throw(perr.SyntaxError(o, "ishikawa", src, starts[i], "Expecting "+expected+", got '"+tokenNames[types[i]]+"'", false))
	}

	if types[i] == tSpaceline {
		i++
		for types[i] == tSpaceline || types[i] == tNL {
			i++
		}
		if types[i] != tIshikawa {
			fail("'SPACELINE', 'NL', 'ISHIKAWA'")
		}
	} else if types[i] != tIshikawa {
		fail("'SPACELINE', 'ISHIKAWA'")
	}
	i++
	if types[i] == tNL {
		i++
	}

	for {
		typ := types[i]
		switch {
		case typ == tText || (typ == tSpacelist && types[i+1] == tText):
			text := i
			if typ != tText {
				text = i + 1
			}
			i = text + 1
			if types[i] != tSpaceline && types[i] != tNL && types[i] != tEOF {
				fail(stop)
			}
			level := 0
			if typ != tText {
				level = ends[text-1] - starts[text-1]
			}
			db.addNode(level, jsstr.Trim(jsstr.FromUTF16(src[starts[text]:ends[text]])))
		case typ == tSpaceline || typ == tSpacelist:
			i++
			if types[i] != tSpaceline && types[i] != tNL && types[i] != tEOF {
				if typ == tSpacelist {
					fail(stop + ", 'TEXT'")
				}
				fail(stop)
			}
		default:
			fail("'SPACELINE', 'SPACELIST', 'TEXT'")
		}
		i++
		for types[i] == tNL || types[i] == tEOF {
			i++
		}
		if types[i] == tEnd {
			return
		}
	}
}
