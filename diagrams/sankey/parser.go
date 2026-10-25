package sankey

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

func isBlank(c uint16) bool {
	if c == 10 || c == 13 {
		return false
	}
	if c < 128 {
		return c == 32 || c == 9 || c == 11 || c == 12
	}
	return c == 0xa0 ||
		c == 0x1680 ||
		(c >= 0x2000 && c <= 0x200a) ||
		c == 0x2028 ||
		c == 0x2029 ||
		c == 0x202f ||
		c == 0x205f ||
		c == 0x3000 ||
		c == 0xfeff
}

// prepareTextForParsing is Mermaid's sankeyUtils: strip blanks at both ends of the text,
// then collapse runs of line breaks so that no row is empty.
func prepareTextForParsing(text string) string {
	units := jsstr.UTF16(text)
	start, end := 0, len(units)
	for start < end && isBlank(units[start]) {
		start++
	}
	for end > start && isBlank(units[end-1]) {
		end--
	}
	out := make([]uint16, 0, end-start)
	for i := start; i < end; i++ {
		c := units[i]
		if c == 10 || c == 13 {
			for i+1 < end && (units[i+1] == 10 || units[i+1] == 13) {
				i++
			}
			c = 10
		}
		out = append(out, c)
	}
	return jsstr.Trim(jsstr.FromUTF16(out))
}

func parseSankey(source string, db *sankeyDb) {
	src := common.Src(jsstr.UTF16(source))
	toks := tokenize(src)
	types, texts, starts := toks.types, toks.texts, toks.starts
	i := 0

	fail := func(expected string) {
		t := types[i]
		got := tokenNames[t]
		if t == tStuck {
			got = "undefined"
			if i > 0 {
				got = tokenNames[types[i-1]]
			}
		}
		perr.Throw(perr.SyntaxError("sankey", src, starts[i], "Expecting "+expected+", got '"+got+"'", false))
	}

	expect := func(typ int, also string) {
		if types[i] != typ {
			fail("'" + tokenNames[typ] + "'" + also)
		}
		i++
	}

	field := func() string {
		if types[i] == tNonEscapedText {
			i++
			return texts[i-1]
		}
		if types[i] != tDQuote {
			fail("'DQUOTE', 'NON_ESCAPED_TEXT'")
		}
		i++
		if types[i] != tEscapedText {
			fail("'ESCAPED_TEXT'")
		}
		text := texts[i]
		i++
		expect(tDQuote, "")
		return text
	}

	name := func() string {
		start := starts[i]
		value := strings.ReplaceAll(jsstr.Trim(field()), `""`, `"`)
		// Entity codes can encode characters excluded by the lexer.
		if hasInvalidXML(text.DecodeEntities(value)) {
			perr.Throw(perr.SyntaxError("sankey", src, start, "Invalid XML character in node name", false))
		}
		return value
	}

	expect(tSankey, "")
	expect(tNewline, "")
	for {
		from := name()
		expect(tComma, ", 'NEWLINE', 'EOF'")
		to := name()
		expect(tComma, ", 'NEWLINE', 'EOF'")
		amount := field()
		source := db.findOrCreateNode(from)
		target := db.findOrCreateNode(to)
		db.addLink(source, target, jsnum.ParseFloat(jsstr.Trim(amount)))
		if types[i] != tNewline {
			break
		}
		i++
	}
	if types[i] == tEOF {
		i++
	}
	if types[i] != tEnd {
		fail("'NEWLINE', 'EOF'")
	}
}

// hasInvalidXML is /[\x00-\x08\x0b\x0c\x0e-\x1f\ud800-\udfff\ufffe\uffff]/u.test(s): with the u
// flag a surrogate pair is one character, so only lone surrogates match.
func hasInvalidXML(s string) bool {
	units := jsstr.UTF16(s)
	for k := 0; k < len(units); k++ {
		c := units[k]
		if c >= 0xd800 && c <= 0xdbff && k+1 < len(units) && units[k+1] >= 0xdc00 && units[k+1] <= 0xdfff {
			k++
			continue
		}
		if c <= 0x08 || c == 0x0b || c == 0x0c || (c >= 0x0e && c <= 0x1f) || (c >= 0xd800 && c <= 0xdfff) || c == 0xfffe || c == 0xffff {
			return true
		}
	}
	return false
}
