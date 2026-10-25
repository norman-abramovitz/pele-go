package journey

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// The token types (upstream's T).
const (
	tEnd = iota
	tEOF
	tInvalid
	tNewline
	tJourney
	tTitle
	tAccTitle
	tAccTitleValue
	tAccDescr
	tAccDescrValue
	tAccDescrMultilineValue
	tSection
	tTaskName
	tTaskData
	tColon
)

var tokenNames = [...]string{
	"$end",
	"EOF",
	"INVALID",
	"NEWLINE",
	"journey",
	"title",
	"acc_title",
	"acc_title_value",
	"acc_descr",
	"acc_descr_value",
	"acc_descr_multiline_value",
	"section",
	"taskName",
	"taskData",
	":",
}

// textEnd finds the end of text from from: `#`, a line break, and `;` end every kind of
// text; a colon also ends a task name or a section.
func textEnd(src common.Src, from int, colon bool) int {
	n := len(src)
	e := from
	for ; e < n; e++ {
		c := src[e]
		if c == 35 || c == 10 || c == 59 || (colon && c == 58) {
			break
		}
	}
	return e
}

func tokenize(src common.Src) common.TextTokens {
	n := len(src)
	var out common.TextTokens
	p := 0

	emit := func(typ, e int) {
		out.Types = append(out.Types, typ)
		out.Texts = append(out.Texts, src.String(p, e))
		out.Starts = append(out.Starts, p)
		p = e
	}

	for {
		p = common.SkipTrivia(src, p)
		if p >= n {
			break
		}
		c := src[p]
		e := p + 1
		if c == 10 {
			for src.At(e) == 10 {
				e++
			}
			emit(tNewline, e)
			continue
		}
		if common.KeywordAt(src, p, "journey") && !util.IsWord(src.At(p+7)) {
			emit(tJourney, p+7)
			continue
		}
		if common.KeywordAt(src, p, "title") {
			// Unlike Mermaid, ignore bare titles.
			e = p + 5
			for src.At(e) != 10 && util.IsSpace(src.At(e)) {
				e++
			}
			if e == n || src.At(e) == 10 {
				p = e
				continue
			}
			if sep := src.At(p + 5); sep != 13 && util.IsSpace(sep) {
				e = textEnd(src, p+6, false)
				if e > p+6 {
					emit(tTitle, e)
					continue
				}
			}
		}
		e = common.Accessibility(src, p, &out, tAccTitle)
		if e != p {
			p = e
			continue
		}
		// Like Mermaid, a bare section uses the next line as its name.
		if common.KeywordAt(src, p, "section") && util.IsSpace(src.At(p+7)) {
			e = textEnd(src, p+8, true)
			if e > p+8 {
				emit(tSection, e)
				continue
			}
		}
		switch c {
		case 58:
			e = textEnd(src, p+1, false)
			if e > p+1 {
				emit(tTaskData, e)
			} else {
				emit(tColon, p+1)
			}
		case 59:
			emit(tInvalid, p+1)
		default:
			emit(tTaskName, textEnd(src, p+1, true))
		}
	}

	// Jison marks its lexer done when a rule runs on empty input, so an accessibility value that
	// matches nothing at the very end of the text is followed by no EOF token.
	if len(out.Starts) == 0 || out.Starts[len(out.Starts)-1] < n {
		emit(tEOF, n)
	}
	emit(tEnd, n)
	return out
}
