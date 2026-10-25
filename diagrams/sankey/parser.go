package sankey

import (
	"math"
	"slices"
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
// then collapse runs of line breaks so that no row is empty. steps, when not nil, records
// what each of its steps changed.
func prepareTextForParsing(text string, steps *[][]perr.Edit) string {
	units := jsstr.UTF16(text)
	start, end := 0, len(units)
	for start < end && isBlank(units[start]) {
		start++
	}
	for end > start && isBlank(units[end-1]) {
		end--
	}
	var runs []perr.Edit
	collapsed := make([]uint16, 0, end-start)
	for i := start; i < end; i++ {
		c := units[i]
		if c == 10 || c == 13 {
			from := i
			for i+1 < end && (units[i+1] == 10 || units[i+1] == 13) {
				i++
			}
			runs = append(runs, perr.Edit{From: from - start, To: i + 1 - start, Length: 1})
			c = 10
		}
		collapsed = append(collapsed, c)
	}
	lead, tail := 0, len(collapsed)
	for lead < tail && jsstr.IsSpace(rune(collapsed[lead])) {
		lead++
	}
	for tail > lead && jsstr.IsSpace(rune(collapsed[tail-1])) {
		tail--
	}
	if steps != nil {
		*steps = append(*steps,
			[]perr.Edit{{From: 0, To: start, Length: 0}, {From: end, To: len(units), Length: 0}},
			runs,
			[]perr.Edit{{From: 0, To: lead, Length: 0}, {From: tail, To: len(collapsed), Length: 0}})
	}
	return jsstr.FromUTF16(collapsed[lead:tail])
}

// parseSankey parses src, the text after prepareTextForParsing, and reports errors in
// source, the text before it, with o.
func parseSankey(prepared string, db *sankeyDb, source string, o *perr.Origin) {
	src := common.Src(jsstr.UTF16(prepared))
	toks := tokenize(src)
	types, texts, starts := toks.types, toks.texts, toks.starts
	i := 0

	syntaxError := func(at int, detail string) {
		var steps [][]perr.Edit
		units := jsstr.UTF16(source)
		if !slices.Equal(units, src) {
			prepareTextForParsing(source, &steps)
		}
		// Map EOF before the trimmed trailing blanks.
		offset := perr.InputOffset(steps, at)
		if at > 0 && at >= len(src) {
			offset = perr.InputOffset(steps, len(src)-1) + 1
		}
		perr.Throw(perr.SyntaxError(o, "sankey", units, offset, detail, false))
	}

	fail := func(expected string) {
		t := types[i]
		got := tokenNames[t]
		if t == tStuck {
			got = "undefined"
			if i > 0 {
				got = tokenNames[types[i-1]]
			}
		}
		syntaxError(starts[i], "Expecting "+expected+", got '"+got+"'")
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
			syntaxError(start, "Invalid XML character in node name")
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
		start := starts[i]
		// Keep Mermaid's numeric prefixes, but reject non-finite values.
		value := jsnum.ParseFloat(field())
		if math.IsNaN(value) || math.IsInf(value, 0) {
			syntaxError(start, "Expecting a number")
		}
		source := db.findOrCreateNode(from)
		target := db.findOrCreateNode(to)
		db.addLink(source, target, value)
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
