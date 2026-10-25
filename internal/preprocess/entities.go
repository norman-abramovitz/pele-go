package preprocess

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// lineEnd returns the index of the first line terminator at or after from, and its length,
// or len(text) and 0.
func lineEnd(text string, from int) (int, int) {
	for i := from; i < len(text); {
		r, n := jsstr.Decode(text, i)
		if jsstr.IsLineEnd(r) {
			return i, n
		}
		i += n
	}
	return len(text), 0
}

// dropColorSemicolons does what Mermaid's text.replace(/keyword.*:\S*#.*;/g, drop the last
// character) does: on a line where the keyword is followed by a colon, a color-like #, and
// later a semicolon, the last semicolon goes.
func dropColorSemicolons(text, keyword string, steps *Steps) string {
	var edits [][3]int
	defer func() { steps.push(text, edits) }()
	from := strings.Index(text, keyword)
	if from == -1 {
		return text
	}
	var out strings.Builder
	last := 0
	for from != -1 {
		end, breakLen := lineEnd(text, from)
		afterColon := false
		hash := -1
		for j := from + len(keyword); j < end; {
			r, n := jsstr.Decode(text, j)
			switch {
			case r == ':':
				afterColon = true
			case r == '#':
				if afterColon {
					hash = j
				}
			case jsstr.IsSpace(r):
				afterColon = false
			}
			if hash != -1 {
				break
			}
			j += n
		}
		if hash != -1 {
			if semi := strings.LastIndexByte(text[:end], ';'); semi > hash {
				edits = append(edits, [3]int{semi, semi + 1, 0})
				out.WriteString(text[last:semi])
				last = semi + 1
			}
		}
		if end >= len(text) {
			from = -1
		} else {
			from = indexFrom(text, keyword, end+breakLen)
		}
	}
	if last == 0 {
		return text
	}
	return out.String() + text[last:]
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// EncodeEntities hides #name; entity codes behind Mermaid's placeholder characters so # and
// ; survive parsing.
func EncodeEntities(text string, steps *Steps) string {
	if !strings.Contains(text, "#") {
		return text
	}
	text = dropColorSemicolons(dropColorSemicolons(text, "style", steps), "classDef", steps)
	var out strings.Builder
	var edits [][3]int
	defer func() { steps.push(text, edits) }()
	last := 0
	for i := 0; i < len(text); i++ {
		if text[i] != '#' {
			continue
		}
		w := wordEnd(text, i+1)
		if w == i+1 || w >= len(text) || text[w] != ';' {
			continue
		}
		inner := text[i+1 : w]
		out.WriteString(text[last:i])
		if isDigits(inner) {
			out.WriteString("ﬂ°°")
			edits = append(edits, [3]int{i, w + 1, 3 + len(inner) + 2})
		} else {
			out.WriteString("ﬂ°")
			edits = append(edits, [3]int{i, w + 1, 2 + len(inner) + 2})
		}
		out.WriteString(inner)
		out.WriteString("¶ß")
		last = w + 1
		i = w
	}
	if last == 0 {
		return text
	}
	return out.String() + text[last:]
}
