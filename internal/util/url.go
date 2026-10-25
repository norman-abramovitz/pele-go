package util

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const blank = "about:blank"

// units is a JS string as UTF-16 code units, which the sanitizer's regular expressions
// read.
type units []uint16

func (u units) at(i int) int {
	if i < 0 || i >= len(u) {
		return -1
	}
	return int(u[i])
}

// hasFold reports whether the lower-case ASCII word is at i, ASCII letters in either case.
func (u units) hasFold(i int, word string) bool {
	if i < 0 || i+len(word) > len(u) {
		return false
	}
	for k := 0; k < len(word); k++ {
		c := u[i+k]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != uint16(word[k]) {
			return false
		}
	}
	return true
}

func (u units) has(i int, word string) bool {
	if i < 0 || i+len(word) > len(u) {
		return false
	}
	for k := 0; k < len(word); k++ {
		if u[i+k] != uint16(word[k]) {
			return false
		}
	}
	return true
}

func (u units) trim() units {
	return units(jsstr.UTF16(jsstr.Trim(jsstr.FromUTF16(u))))
}

// ctrlAt reports whether RE_CTRL,
// /[\u0000-\u001F\u007F-\u009F\u2000-\u200D\uFEFF\uD800-\uDFFF\uFFFE\uFFFF]/gimu, matches
// at u[i], and how many units to step past. The u flag reads a surrogate pair as one code
// point outside the class, so only lone surrogates match.
func ctrlAt(u units, i int) (bool, int) {
	c := u[i]
	if c >= 0xd800 && c <= 0xdbff && i+1 < len(u) && u[i+1] >= 0xdc00 && u[i+1] <= 0xdfff {
		return false, 2
	}
	return c <= 0x1f || c >= 0x7f && c <= 0x9f || c >= 0x2000 && c <= 0x200d || c == 0xfeff ||
		c >= 0xd800 && c <= 0xdfff || c == 0xfffe || c == 0xffff, 1
}

func hasCtrl(u units) bool {
	for i := 0; i < len(u); {
		ctrl, n := ctrlAt(u, i)
		if ctrl {
			return true
		}
		i += n
	}
	return false
}

// decodeURIComponent returns decodeURIComponent(u), or false where it throws a URIError.
func decodeURIComponent(u units) (units, bool) {
	// hex reads the escape at i, or -1.
	hex := func(i int) int {
		if i+2 >= len(u) || u[i] != '%' || !IsHex(int(u[i+1])) || !IsHex(int(u[i+2])) {
			return -1
		}
		v, _ := strconv.ParseUint(string([]byte{byte(u[i+1]), byte(u[i+2])}), 16, 8)
		return int(v)
	}
	out := make(units, 0, len(u))
	for k := 0; k < len(u); k++ {
		if u[k] != '%' {
			out = append(out, u[k])
			continue
		}
		b := hex(k)
		if b == -1 {
			return nil, false
		}
		k += 2
		if b < 0x80 {
			out = append(out, uint16(b))
			continue
		}
		n := 0
		for b<<n&0x80 != 0 {
			n++
		}
		if n == 1 || n > 4 {
			return nil, false
		}
		octets := []byte{byte(b)}
		if k+3*(n-1) >= len(u) {
			return nil, false
		}
		for j := 1; j < n; j++ {
			k++
			c := hex(k)
			if c == -1 || c&0xc0 != 0x80 {
				return nil, false
			}
			k += 2
			octets = append(octets, byte(c))
		}
		r, size := utf8.DecodeRune(octets)
		if size != n {
			return nil, false
		}
		out = append(out, jsstr.UTF16(string(r))...)
	}
	return out, true
}

// decode is decodeURIComponent, keeping u unchanged where that throws.
func decode(u units) units {
	if d, ok := decodeURIComponent(u); ok {
		return d
	}
	return u
}

func removeCtrl(u units) units {
	out := make(units, 0, len(u))
	for i := 0; i < len(u); {
		ctrl, n := ctrlAt(u, i)
		if !ctrl {
			out = append(out, u[i:i+n]...)
		}
		i += n
	}
	return out
}

// entityAt matches RE_HTML_ENTITIES, /&#(\w+)(^\w|;)?/, at i and returns its end and the
// digits, or -1. (^\w can never match after &#, so the group is an optional semicolon.)
func entityAt(u units, i int) (int, units) {
	if !u.has(i, "&#") {
		return -1, nil
	}
	j := i + 2
	for IsWord(u.at(j)) {
		j++
	}
	if j == i+2 {
		return -1, nil
	}
	digits := u[i+2 : j]
	if u.at(j) == ';' {
		j++
	}
	return j, digits
}

// fromCharCode is String.fromCharCode(Number(s)).
func fromCharCode(s units) uint16 {
	n := jsnum.Parse(jsstr.FromUTF16(s))
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	m := math.Mod(math.Trunc(n), 65536)
	if m < 0 {
		m += 65536
	}
	return uint16(m)
}

func replaceEntities(u units) units {
	out := make(units, 0, len(u))
	for i := 0; i < len(u); {
		if end, digits := entityAt(u, i); end != -1 {
			out = append(out, fromCharCode(digits))
			i = end
			continue
		}
		out = append(out, u[i])
		i++
	}
	return out
}

// ctrlEntityAt matches RE_HTML_CTRL_ENTITY, /&(newline|tab);/i, at i and returns its end, or
// -1.
func ctrlEntityAt(u units, i int) int {
	if u.at(i) != '&' {
		return -1
	}
	for _, word := range [...]string{"newline", "tab"} {
		if u.hasFold(i+1, word) && u.at(i+1+len(word)) == ';' {
			return i + 2 + len(word)
		}
	}
	return -1
}

// wsEscapeAt matches RE_WS_ESCAPE, /(\\|%5[cC])((%(6[eE]|72|74))|[nrt])/, at i and returns
// its end, or -1.
func wsEscapeAt(u units, i int) int {
	j := -1
	switch {
	case u.at(i) == '\\':
		j = i + 1
	case u.has(i, "%5c") || u.has(i, "%5C"):
		j = i + 3
	default:
		return -1
	}
	switch {
	case u.has(j, "%6e") || u.has(j, "%6E") || u.has(j, "%72") || u.has(j, "%74"):
		return j + 3
	case u.at(j) == 'n' || u.at(j) == 'r' || u.at(j) == 't':
		return j + 1
	}
	return -1
}

func removeMatches(u units, at func(units, int) int) units {
	out := make(units, 0, len(u))
	for i := 0; i < len(u); {
		if end := at(u, i); end != -1 {
			i = end
			continue
		}
		out = append(out, u[i])
		i++
	}
	return out
}

func anyMatch(u units, at func(units, int) int) bool {
	for i := range u {
		if at(u, i) != -1 {
			return true
		}
	}
	return false
}

// lineStarts are where ^ matches with the m flag: the start and after each line terminator.
func lineStarts(u units) []int {
	starts := []int{0}
	for i, c := range u {
		if IsLineEnd(int(c)) {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// schemeOf is the first match of RE_SCHEME, /^.+(:|&colon;)/im: on the first line that has
// one, everything up to its last colon.
func schemeOf(u units) (units, bool) {
	for _, s := range lineStarts(u) {
		e := s
		for e < len(u) && !IsLineEnd(int(u[e])) {
			e++
		}
		for k := e - 1; k > s; k-- {
			if u[k] == ':' {
				return u[s : k+1], true
			}
			if u.hasFold(k, "&colon;") {
				return u[s : k+7], true
			}
		}
	}
	return nil, false
}

// lower is toLowerCase as far as the scheme checks can tell: of every character only A-Z,
// the Kelvin sign and the dotted capital I lower-case to anything with ASCII in it.
func lower(u units) units {
	out := make(units, 0, len(u))
	for _, c := range u {
		switch {
		case c >= 'A' && c <= 'Z':
			out = append(out, c+'a'-'A')
		case c == 0x212a:
			out = append(out, 'k')
		case c == 0x130:
			out = append(out, 'i', 0x307)
		default:
			out = append(out, c)
		}
	}
	return out
}

// isInvalidProtocol is RE_INVALID_PROTOCOL, /^([^\w]*)(javascript|data|vbscript)/im.
func isInvalidProtocol(u units) bool {
	for _, s := range lineStarts(u) {
		i := s
		for i < len(u) && !IsWord(int(u[i])) {
			i++
		}
		if u.hasFold(i, "javascript") || u.hasFold(i, "data") || u.hasFold(i, "vbscript") {
			return true
		}
	}
	return false
}

// SanitizeURL follows the algorithm of @braintree/sanitize-url, which Mermaid applies to
// click targets.
func SanitizeURL(url string) string {
	if url == "" {
		return blank
	}
	decoded := decode(units(jsstr.UTF16(jsstr.Trim(url))))
	for rounds := 1; ; rounds++ {
		// Each round peels one layer of encoding. A URL wrapped this many times is not a real
		// one.
		if rounds > 32 {
			return blank
		}
		decoded = removeCtrl(decoded)
		decoded = replaceEntities(decoded)
		decoded = removeMatches(decoded, ctrlEntityAt)
		decoded = removeCtrl(decoded)
		decoded = removeMatches(decoded, wsEscapeAt).trim()
		decoded = decode(decoded)
		if !hasCtrl(decoded) && !anyMatch(decoded, func(u units, i int) int { end, _ := entityAt(u, i); return end }) &&
			!anyMatch(decoded, ctrlEntityAt) && !anyMatch(decoded, wsEscapeAt) {
			break
		}
	}

	if len(decoded) == 0 {
		return blank
	}
	if decoded[0] == '.' || decoded[0] == '/' {
		return jsstr.FromUTF16(decoded)
	}
	trimmed := units(jsstr.UTF16(jsstr.TrimStart(jsstr.FromUTF16(decoded))))
	scheme, ok := schemeOf(trimmed)
	if !ok {
		return jsstr.FromUTF16(decoded)
	}
	protocol := lower(scheme).trim()
	if isInvalidProtocol(protocol) {
		return blank
	}
	normalized := strings.ReplaceAll(jsstr.FromUTF16(trimmed), `\`, "/")
	p := jsstr.FromUTF16(protocol)
	if p == "mailto:" || strings.Contains(p, "://") {
		return normalized
	}
	if p == "http:" || p == "https:" {
		parsed, ok := parseURL(normalized)
		if !ok {
			return blank
		}
		// Setting the protocol and hostname to their lower case changes nothing: the parser
		// has lower-cased both.
		return parsed
	}
	return normalized
}

// SafeURL is a URL as written in a diagram: entity codes are read first, so they cannot
// hide a scheme.
func SafeURL(url string) string {
	return SanitizeURL(text.DecodeEntities(url))
}

var (
	linkSchemes  = []string{"http", "https", "mailto", "tel"}
	imageSchemes = []string{"http", "https"}
)

// allow keeps a URL only when it is relative or its scheme is in schemes. Stricter than
// Mermaid, which lets any scheme but three through: a diagram in a shared note could
// otherwise open a local file, a network share, or another application.
func allow(url string, schemes []string) (string, bool) {
	// /^[\\/]{2}/
	if len(url) >= 2 && (url[0] == '\\' || url[0] == '/') && (url[1] == '\\' || url[1] == '/') {
		return "", false
	}
	// /^([a-z][a-z0-9+.-]*):/i
	i := 0
	if i < len(url) && IsLetter(int(url[i])) {
		i++
		for i < len(url) && (IsLetter(int(url[i])) || IsDigit(int(url[i])) || url[i] == '+' || url[i] == '.' || url[i] == '-') {
			i++
		}
		if i < len(url) && url[i] == ':' {
			scheme := strings.ToLower(url[:i])
			for _, s := range schemes {
				if s == scheme {
					return url, true
				}
			}
			return "", false
		}
	}
	return url, true
}

func orDefault(list, def []string) []string {
	if list == nil {
		return def
	}
	return list
}

// LinkURL keeps a URL that SanitizeURL has passed only when it is relative or its scheme is
// one the host allows. One that is refused is left out of the drawing: written as
// about:blank, a link would still take the page it is in somewhere when clicked.
func LinkURL(url string, options core.Options) (string, bool) {
	if options.Links != nil && !*options.Links {
		return "", false
	}
	return allow(url, orDefault(options.LinkSchemes, linkSchemes))
}

// ImageURL is LinkURL for an image's address.
func ImageURL(url string, options core.Options) (string, bool) {
	if options.Images != nil && !*options.Images {
		return "", false
	}
	return allow(url, orDefault(options.ImageSchemes, imageSchemes))
}

// InternalLink reports whether an internal link's name may be its href. The href keeps the
// original name, so sanitizing alone is not enough.
func InternalLink(name string, options core.Options) bool {
	url := SanitizeURL(name)
	if url == blank {
		return false
	}
	if _, ok := LinkURL(url, options); !ok {
		return false
	}
	// Match character removal by Esc and the browser.
	seen := strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(svg.DropInvalidXML(name))
	seen = strings.TrimLeft(seen, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f ")
	_, ok := LinkURL(seen, options)
	return ok
}

// Linked wraps body in a link to url, when the host allows it, and records the link.
func Linked(body, url, id string, links *[]core.Link, options core.Options, attrs string) string {
	href, ok := LinkURL(url, options)
	if !ok {
		return body
	}
	*links = append(*links, core.Link{ID: id, Href: href})
	return `<a href="` + svg.Esc(href) + `"` + attrs + RelAttr(options) + `>` + body + `</a>`
}

// RelAttr is a link's rel attribute: noopener by default, none for an empty LinkRel.
func RelAttr(options core.Options) string {
	rel := "noopener"
	if options.LinkRel != nil {
		rel = *options.LinkRel
	}
	if rel == "" {
		return ""
	}
	return ` rel="` + svg.Esc(rel) + `"`
}
