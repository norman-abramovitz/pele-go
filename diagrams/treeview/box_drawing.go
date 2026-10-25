package treeview

import (
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

// Mermaid's box-drawing preprocessor: turns `├──`, `└──`, `│` (and the heavy variants) into the
// indentation the grammar reads. Every box character is one UTF-16 code unit, so lines are
// read as code units, as upstream's regexes index them.

const indentUnit = "    "

// isBoxChar is /[─━│┃└┗├┣]/ on one code unit.
func isBoxChar(c uint16) bool {
	return isBranchChar(c) || isDashChar(c) || isBarChar(c)
}

// isBranchChar is /[└┗├┣]/.
func isBranchChar(c uint16) bool { return c == '└' || c == '┗' || c == '├' || c == '┣' }

// isDashChar is /[─━]/.
func isDashChar(c uint16) bool { return c == '─' || c == '━' }

func isBarChar(c uint16) bool { return c == '│' || c == '┃' }

func isSpaceUnit(c uint16) bool { return util.IsSpace(int(c)) }

func hasBoxChar(units []uint16) bool {
	for _, c := range units {
		if isBoxChar(c) {
			return true
		}
	}
	return false
}

func branchIndex(units []uint16) int {
	for i, c := range units {
		if isBranchChar(c) {
			return i
		}
	}
	return -1
}

// onlyOf is /^[...]+$/ for a set of code units.
func onlyOf(units []uint16, in func(uint16) bool) bool {
	if len(units) == 0 {
		return false
	}
	for _, c := range units {
		if !in(c) {
			return false
		}
	}
	return true
}

// decorationOnly is /^[\s│┃]+$/.
func decorationOnly(units []uint16) bool {
	return onlyOf(units, func(c uint16) bool { return isSpaceUnit(c) || isBarChar(c) })
}

// boxOnly is /^[\s─━│┃└┗├┣]+$/.
func boxOnly(units []uint16) bool {
	return onlyOf(units, func(c uint16) bool { return isSpaceUnit(c) || isBoxChar(c) })
}

func skipSpace(units []uint16) int {
	i := 0
	for i < len(units) && isSpaceUnit(units[i]) {
		i++
	}
	return i
}

func hasASCIIPrefix(units []uint16, at int, word string) bool {
	if at+len(word) > len(units) {
		return false
	}
	for k := range len(word) {
		if units[at+k] != uint16(word[k]) {
			return false
		}
	}
	return true
}

// metadataLine is /^\s*(title[\t ]|accTitle[\t ]*:|accDescr[\t ]*[:{])/.
func metadataLine(units []uint16) bool {
	i := skipSpace(units)
	blank := func(at int) bool { return at < len(units) && (units[at] == ' ' || units[at] == '\t') }
	skipBlank := func(at int) int {
		for blank(at) {
			at++
		}
		return at
	}
	switch {
	case hasASCIIPrefix(units, i, "title"):
		return blank(i + len("title"))
	case hasASCIIPrefix(units, i, "accTitle"):
		j := skipBlank(i + len("accTitle"))
		return j < len(units) && units[j] == ':'
	case hasASCIIPrefix(units, i, "accDescr"):
		j := skipBlank(i + len("accDescr"))
		return j < len(units) && (units[j] == ':' || units[j] == '{')
	}
	return false
}

// commentLine is /^\s*%%/.
func commentLine(units []uint16) bool {
	return hasASCIIPrefix(units, skipSpace(units), "%%")
}

func expandTabs(units []uint16) []uint16 {
	var out []uint16
	for _, c := range units {
		if c == '\t' {
			out = append(out, ' ', ' ', ' ', ' ')
		} else {
			out = append(out, c)
		}
	}
	return out
}

func inferSegmentWidth(contentLines [][]uint16) int {
	for _, line := range contentLines {
		if index := branchIndex(line); index > 0 {
			return index
		}
	}
	return 4
}

// remapErrorLines is message.replace(/\bline\s+(\d+)\b/gi, ...), naming the original line for
// each output line the map knows.
func remapErrorLines(message string, lineMap map[int]int) string {
	var b strings.Builder
	last := 0
	for i := 0; i+4 <= len(message); i++ {
		// Without the u flag, /i folds no character outside ASCII onto these letters.
		if !asciiEqualFold(message[i:i+4], "line") || i > 0 && util.IsWord(int(message[i-1])) {
			continue
		}
		j := i + 4
		spaces := j
		for j < len(message) {
			r, n := jsstr.Decode(message, j)
			if !jsstr.IsSpace(r) {
				break
			}
			j += n
		}
		if j == spaces {
			continue
		}
		digits := j
		for j < len(message) && util.IsDigit(int(message[j])) {
			j++
		}
		if j == digits || j < len(message) && util.IsWord(int(message[j])) {
			continue
		}
		n, err := strconv.Atoi(message[digits:j])
		if err != nil {
			continue
		}
		original, ok := lineMap[n]
		if !ok {
			continue
		}
		b.WriteString(message[last:i])
		b.WriteString("line " + strconv.Itoa(original))
		last = j
		i = j - 1
	}
	if last == 0 {
		return message
	}
	b.WriteString(message[last:])
	return b.String()
}

// lineStart is the offset of a line, from 1, in text. Like upstream's indexOf loop, it starts
// over from 0 past the last line.
func lineStart(text []uint16, line int) int {
	at := 0
	for n := 1; n < line; n++ {
		at = indexOf(text, '\n', at) + 1
	}
	return at
}

func indexOf(text []uint16, c uint16, from int) int {
	for i := from; i < len(text); i++ {
		if text[i] == c {
			return i
		}
	}
	return -1
}

func fail(o *perr.Origin, input []uint16, index int, message string) {
	line := perr.Locate(o, input, lineStart(input, index+1)).Line
	perr.Throw(&perr.Error{Code: perr.Syntax, Message: "Line " + strconv.Itoa(line) + ": " + message, Type: "treeView", Line: line})
}

// beforeTabs undoes four-space tab expansion.
func beforeTabs(line []uint16, at int) int {
	i := 0
	for n := 0; i < len(line) && n < at; i++ {
		if line[i] == '\t' {
			n += 4
		} else {
			n++
		}
	}
	return i
}

// preprocessBoxDrawing returns the text the grammar reads and, when it changed the text, a map
// from its line numbers to the original's, both from 1, and per output line a map from its
// indices to the original's.
func preprocessBoxDrawing(input string, o *perr.Origin) (string, map[int]int, map[int]func(int) int) {
	lineMap := map[int]int{}
	columns := map[int]func(int) int{}
	units := jsstr.UTF16(input)
	if !hasBoxChar(units) {
		return input, lineMap, columns
	}
	var lines [][]uint16
	from := 0
	for i, c := range units {
		if c == '\n' {
			lines = append(lines, units[from:i])
			from = i + 1
		}
	}
	lines = append(lines, units[from:])

	keywordIdx := -1
	for i, line := range lines {
		if jsstr.Trim(jsstr.FromUTF16(line)) == "treeView-beta" {
			keywordIdx = i
			break
		}
	}
	if keywordIdx == -1 {
		return input, lineMap, columns
	}

	passes := func(line []uint16) bool {
		return skipSpace(line) == len(line) || commentLine(line) || metadataLine(line)
	}

	var contentLines [][]uint16
	for _, line := range lines[keywordIdx+1:] {
		if passes(line) || decorationOnly(line) {
			continue
		}
		contentLines = append(contentLines, expandTabs(line))
	}
	contentHasBox := false
	for _, line := range contentLines {
		if hasBoxChar(line) {
			contentHasBox = true
			break
		}
	}
	if !contentHasBox {
		return input, lineMap, columns
	}

	segmentWidth := inferSegmentWidth(contentLines)
	var out []string
	keep := func(text string, index int, column func(int) int) {
		out = append(out, text)
		lineMap[len(out)] = index + 1
		if column != nil {
			columns[len(out)] = column
		}
	}

	for i := 0; i <= keywordIdx; i++ {
		keep(jsstr.FromUTF16(lines[i]), i, nil)
	}
	for i := keywordIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if passes(line) {
			keep(jsstr.FromUTF16(line), i, nil)
			continue
		}
		if decorationOnly(line) {
			continue
		}

		normalized := expandTabs(line)
		if branch := branchIndex(normalized); branch != -1 {
			depth := int(jsmath.Round(float64(branch)/float64(segmentWidth))) + 1
			pos := branch + 1
			for pos < len(normalized) && isDashChar(normalized[pos]) {
				pos++
			}
			for pos < len(normalized) && normalized[pos] == ' ' {
				pos++
			}
			content := jsstr.TrimEnd(jsstr.FromUTF16(normalized[pos:]))
			if content == "" {
				fail(o, units, i, "Empty node — expected a filename or directory name after the box-drawing prefix")
			}
			indent := strings.Repeat(indentUnit, depth)
			keep(indent+content, i, func(at int) int { return beforeTabs(line, pos+max(at-len(indent), 0)) })
		} else if boxOnly(normalized) {
			continue
		} else if hasBoxChar(normalized) {
			// A name that happens to contain a box character, such as "Section ─ A.txt", is a root item.
			keep(jsstr.FromUTF16(line), i, nil)
		} else if len(normalized) > 0 && isSpaceUnit(normalized[0]) {
			fail(o, units, i, "Unexpected indentation without box-drawing characters. In box-drawing format, use ├── or └── prefixes for indented nodes.")
		} else {
			keep(jsstr.FromUTF16(line), i, nil)
		}
	}
	return strings.Join(out, "\n"), lineMap, columns
}

func asciiEqualFold(s, lower string) bool {
	for i := range len(lower) {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}
