// Package detect is upstream's src/detect.ts: Mermaid's diagram detectors, in the order
// Mermaid registers them.
package detect

import (
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// Detected is a diagram type, with the name Mermaid reports for it.
type Detected struct {
	Type        string
	MermaidType string
}

type detector struct {
	typ, mermaidType string
	match            func(cleaned string) bool
}

func isWord(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

// afterSpace is what follows the whitespace ^\s* matches.
var afterSpace = jsstr.TrimStart

// hasPrefixFold compares ASCII letters without case, as a non-unicode /i regex does.
func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		a, b := s[i], prefix[i]
		if 'A' <= a && a <= 'Z' {
			a += 'a' - 'A'
		}
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		if a != b {
			return false
		}
	}
	return true
}

// prefix is /^\s*word/.
func prefix(word string) func(string) bool {
	return func(s string) bool { return strings.HasPrefix(afterSpace(s), word) }
}

// prefixFold is /^\s*word/i.
func prefixFold(word string) func(string) bool {
	return func(s string) bool { return hasPrefixFold(afterSpace(s), word) }
}

// wordEnd is /^\s*word\b/, optionally case-insensitive. Every word here ends in a word
// character, so \b means the next character is not one.
func wordEnd(word string, fold bool) func(string) bool {
	return func(s string) bool {
		rest := afterSpace(s)
		ok := strings.HasPrefix(rest, word)
		if fold {
			ok = hasPrefixFold(rest, word)
		}
		return ok && (len(rest) == len(word) || !isWord(rest[len(word)]))
	}
}

// followedBy is /^\s*word(?:[\s...]|$)/: the word, then the end or a character next allows.
func followedBy(word string, next func(rune) bool) func(string) bool {
	return func(s string) bool {
		rest := afterSpace(s)
		if !strings.HasPrefix(rest, word) {
			return false
		}
		if len(rest) == len(word) {
			return true
		}
		r, _ := jsstr.Decode(rest, len(word))
		return next(r)
	}
}

func isSpaceOrColon(r rune) bool { return jsstr.IsSpace(r) || r == ':' }

// c4 is /^\s*(?:C4Context|C4Container|C4Component|C4Dynamic|C4Deployment)/.
func c4(s string) bool {
	rest := afterSpace(s)
	for _, name := range []string{"C4Context", "C4Container", "C4Component", "C4Dynamic", "C4Deployment"} {
		if strings.HasPrefix(rest, name) {
			return true
		}
	}
	return false
}

func either(matchers ...func(string) bool) func(string) bool {
	return func(s string) bool {
		for _, m := range matchers {
			if m(s) {
				return true
			}
		}
		return false
	}
}

// detectors are Mermaid's, in the order Mermaid registers them. The first match wins.
var detectors = []detector{
	{"flowchart", "flowchart-elk", prefix("flowchart-elk")},
	{"mindmap", "mindmap", prefix("mindmap")},
	{"architecture", "architecture", prefix("architecture")},
	{"agentflow", "agentflow", wordEnd("agentflow-beta", false)},
	{"c4", "c4", c4},
	{"kanban", "kanban", prefix("kanban")},
	{"class", "classDiagram", prefix("classDiagram")},
	{"er", "er", prefix("erDiagram")},
	{"gantt", "gantt", prefix("gantt")},
	{"info", "info", prefix("info")},
	{"pie", "pie", prefix("pie")},
	{"requirement", "requirement", prefix("requirement")},
	{"sequence", "sequence", prefix("sequenceDiagram")},
	{"swimlane", "swimlane", wordEnd("swimlane-beta", false)},
	{"flowchart", "flowchart-v2", either(prefix("graph"), prefix("flowchart"))},
	{"timeline", "timeline", prefix("timeline")},
	{"gitGraph", "gitGraph", prefix("gitGraph")},
	{"state", "stateDiagram", prefix("stateDiagram")},
	{"journey", "journey", prefix("journey")},
	{"quadrantChart", "quadrantChart", prefix("quadrantChart")},
	{"sankey", "sankey", prefix("sankey")},
	{"packet", "packet", prefix("packet")},
	{"xychart", "xychart", prefix("xychart")},
	{"block", "block", prefix("block")},
	{"eventmodeling", "eventmodeling", prefix("eventmodeling")},
	{"treeView", "treeView", prefix("treeView-beta")},
	{"radar", "radar", prefix("radar-beta")},
	// /^\s*ishikawa(-beta)?\b/i: when -beta is not followed by a boundary, the regex backs
	// off to ishikawa, which is always followed by one there.
	{"ishikawa", "ishikawa", wordEnd("ishikawa", true)},
	{"treemap", "treemap", prefix("treemap")},
	{"railroad", "railroad", prefixFold("railroad-beta")},
	{"railroad", "railroadEbnf", prefixFold("railroad-ebnf-beta")},
	{"railroad", "railroadAbnf", prefixFold("railroad-abnf-beta")},
	{"railroad", "railroadPeg", prefixFold("railroad-peg-beta")},
	{"venn", "venn", prefix("venn-beta")},
	{"wardley", "wardley", prefixFold("wardley-beta")},
	{"cynefin", "cynefin", followedBy("cynefin-beta", isSpaceOrColon)},
	{"usecase", "usecase", followedBy("usecase-beta", jsstr.IsSpace)},
}

// Detect returns the diagram type of preprocessed text.
func Detect(cleaned string) (Detected, bool) {
	for _, d := range detectors {
		if d.match(cleaned) {
			return Detected{Type: d.typ, MermaidType: d.mermaidType}, true
		}
	}
	return Detected{}, false
}
