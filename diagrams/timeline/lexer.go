package timeline

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
	tTimeline
	tTimelineLR
	tTimelineTD
	tTitle
	tAccTitle
	tAccTitleValue
	tAccDescr
	tAccDescrValue
	tAccDescrMultilineValue
	tSection
	tPeriod
	tEvent
)

var tokenNames = [...]string{
	"$end",
	"EOF",
	"INVALID",
	"NEWLINE",
	"timeline",
	"timeline_lr",
	"timeline_td",
	"title",
	"acc_title",
	"acc_title_value",
	"acc_descr",
	"acc_descr_value",
	"acc_descr_multiline_value",
	"section",
	"period",
	"event",
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
		if common.KeywordAt(src, p, "timeline") {
			e = p + 8
			for src.At(e) == 32 || src.At(e) == 9 {
				e++
			}
			lr := common.KeywordAt(src, e, "lr")
			if e > p+8 && (lr || common.KeywordAt(src, e, "td")) && !util.IsWord(src.At(e+2)) {
				if lr {
					emit(tTimelineLR, e+2)
				} else {
					emit(tTimelineTD, e+2)
				}
				continue
			}
			if !util.IsWord(src.At(p + 8)) {
				emit(tTimeline, p+8)
				continue
			}
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
				emit(tTitle, common.LineEnd(src, p+6))
				continue
			}
		}
		e = common.Accessibility(src, p, &out, tAccTitle)
		if e != p {
			p = e
			continue
		}
		// Like Mermaid, a bare section uses the next line as its name.
		if common.KeywordAt(src, p, "section") && util.IsSpace(src.At(p+7)) {
			e = p + 8
			for e < n && src[e] != 58 && src[e] != 10 {
				e++
			}
			if e > p+8 {
				emit(tSection, e)
				continue
			}
		}
		if c != 58 {
			e = p + 1
			for e < n && src[e] != 35 && src[e] != 58 && src[e] != 10 {
				e++
			}
			emit(tPeriod, e)
			continue
		}
		// An event runs to the end of the line or to a colon that is followed by whitespace.
		if util.IsSpace(src.At(p + 1)) {
			e = p + 2
			for ; e < n; e++ {
				k := src[e]
				if k == 10 || (k == 58 && util.IsSpace(src.At(e+1))) {
					break
				}
			}
			if e > p+2 {
				emit(tEvent, e)
				continue
			}
		}
		emit(tInvalid, p+1)
	}

	// Jison marks its lexer done when a rule runs on empty input, so an accessibility value that
	// matches nothing at the very end of the text is followed by no EOF token.
	if len(out.Starts) == 0 || out.Starts[len(out.Starts)-1] < n {
		emit(tEOF, n)
	}
	emit(tEnd, n)
	return out
}
