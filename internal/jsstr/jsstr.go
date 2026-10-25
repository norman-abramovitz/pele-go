// Package jsstr gives Go strings the behaviour of JavaScript strings where the two differ.
//
// Strings inside the port are WTF-8: UTF-8 that can also hold a lone surrogate, encoded as
// the three bytes UTF-8 would use for its code point. JavaScript strings are UTF-16 and can
// hold lone surrogates, so this keeps every JS string representable and makes conversions in
// both directions lossless.
//
// Joining two WTF-8 strings can leave a high and a low surrogate next to each other, each
// encoded on its own, where JS would have one character. UTF16 and Len read such a run as
// the pair JS has; Canonical rewrites it as the one character, as output must be.
package jsstr

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Decode returns the code point that starts at s[i] and its length in bytes. A WTF-8 lone
// surrogate decodes to its surrogate code point. An invalid byte decodes to U+FFFD with
// length 1.
func Decode(s string, i int) (rune, int) {
	c := s[i]
	if c < utf8.RuneSelf {
		return rune(c), 1
	}
	if c == 0xed && i+2 < len(s) && s[i+1] >= 0xa0 && s[i+1] <= 0xbf && s[i+2]&0xc0 == 0x80 {
		return 0xd000 | rune(s[i+1]&0x3f)<<6 | rune(s[i+2]&0x3f), 3
	}
	return utf8.DecodeRuneInString(s[i:])
}

// DecodeLast returns the code point that ends at s[:end] and its length in bytes, as Decode
// would read it.
func DecodeLast(s string, end int) (rune, int) {
	start := end - 1
	for start > 0 && end-start < utf8.UTFMax && !utf8.RuneStart(s[start]) {
		start--
	}
	if r, n := Decode(s, start); start+n == end {
		return r, n
	}
	return utf8.RuneError, 1
}

func isSurrogate(r rune) bool { return r >= 0xd800 && r <= 0xdfff }

// UTF16 returns the UTF-16 code units of s.
func UTF16(s string) []uint16 {
	units := make([]uint16, 0, len(s))
	for i := 0; i < len(s); {
		r, n := Decode(s, i)
		i += n
		if r >= 0x10000 {
			hi, lo := utf16.EncodeRune(r)
			units = append(units, uint16(hi), uint16(lo))
		} else {
			units = append(units, uint16(r))
		}
	}
	return units
}

// FromUTF16 returns the WTF-8 string for UTF-16 code units, pairing surrogates where they
// form a pair and keeping lone ones.
func FromUTF16(units []uint16) string {
	var b strings.Builder
	b.Grow(len(units))
	for i := 0; i < len(units); i++ {
		r := rune(units[i])
		if r >= 0xd800 && r < 0xdc00 && i+1 < len(units) {
			if pair := utf16.DecodeRune(r, rune(units[i+1])); pair != utf8.RuneError {
				b.WriteRune(pair)
				i++
				continue
			}
		}
		writeRune(&b, r)
	}
	return b.String()
}

func writeRune(b *strings.Builder, r rune) {
	if isSurrogate(r) {
		b.WriteByte(0xe0 | byte(r>>12))
		b.WriteByte(0x80 | byte(r>>6)&0x3f)
		b.WriteByte(0x80 | byte(r)&0x3f)
		return
	}
	b.WriteRune(r)
}

// Normalize returns s with every invalid byte turned into U+FFFD, as converting s to a JS
// string does. Lone surrogates already in WTF-8 are kept, and a high and a low one encoded
// next to each other become the character JS reads them as.
func Normalize(s string) string {
	for i := 0; i < len(s); {
		r, n := Decode(s, i)
		if r == utf8.RuneError && n == 1 {
			return FromUTF16(UTF16(s))
		}
		i += n
	}
	return Canonical(s)
}

// Len returns s.length: the number of UTF-16 code units in s.
func Len(s string) int {
	n := 0
	for i := 0; i < len(s); {
		r, size := Decode(s, i)
		i += size
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return n
}

// hasSurrogate reports whether s holds an encoded surrogate, paired or not.
func hasSurrogate(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == 0xed && s[i+1] >= 0xa0 && s[i+1] <= 0xbf {
			return true
		}
	}
	return false
}

// Canonical returns s with each high surrogate that is followed by a low one written as the
// character they make, as JS reads them.
func Canonical(s string) string {
	if !hasSurrogate(s) {
		return s
	}
	return FromUTF16(UTF16(s))
}

// WellFormed returns s with each lone surrogate replaced by U+FFFD, as encoding a JS string
// as UTF-8 does.
func WellFormed(s string) string {
	s = Canonical(s)
	for i := 0; i < len(s); {
		r, n := Decode(s, i)
		if isSurrogate(r) {
			var b strings.Builder
			b.Grow(len(s))
			b.WriteString(s[:i])
			for ; i < len(s); i += n {
				r, n = Decode(s, i)
				if isSurrogate(r) {
					r = utf8.RuneError
				}
				b.WriteRune(r)
			}
			return b.String()
		}
		i += n
	}
	return s
}

// IsSpace reports whether r is whitespace to JavaScript: what \s matches and trim removes.
func IsSpace(r rune) bool {
	if r < 128 {
		return r == ' ' || (r >= '\t' && r <= '\r')
	}
	return r == 0xa0 || r == 0x1680 || (r >= 0x2000 && r <= 0x200a) || r == 0x2028 || r == 0x2029 ||
		r == 0x202f || r == 0x205f || r == 0x3000 || r == 0xfeff
}

// IsLineEnd reports whether r ends a line to JavaScript: what . does not match.
func IsLineEnd(r rune) bool {
	return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029
}

// TrimStart returns s without leading JS whitespace.
func TrimStart(s string) string {
	i := 0
	for i < len(s) {
		r, n := Decode(s, i)
		if !IsSpace(r) {
			break
		}
		i += n
	}
	return s[i:]
}

// TrimEnd returns s without trailing JS whitespace.
func TrimEnd(s string) string {
	end := len(s)
	for end > 0 {
		r, n := DecodeLast(s, end)
		if !IsSpace(r) {
			break
		}
		end -= n
	}
	return s[:end]
}

// Trim returns s without leading or trailing JS whitespace.
func Trim(s string) string {
	return TrimEnd(TrimStart(s))
}

// Slice returns s.slice(from, to) for non-negative UTF-16 offsets. An offset inside a
// surrogate pair splits it, leaving lone surrogates, as JS does.
func Slice(s string, from, to int) string {
	if to <= from {
		return ""
	}
	units := UTF16(s)
	from = min(from, len(units))
	to = min(to, len(units))
	return FromUTF16(units[from:to])
}

// Compare orders a and b by UTF-16 code unit, as JS's < and default sort() do. Go's string
// comparison orders by code point, which puts characters outside the BMP after U+E000 to
// U+FFFF where JS puts them before.
func Compare(a, b string) int {
	i, j := 0, 0
	var pendingA, pendingB rune = -1, -1
	next := func(s string, k *int, pending *rune) (rune, bool) {
		if *pending >= 0 {
			u := *pending
			*pending = -1
			return u, true
		}
		if *k >= len(s) {
			return 0, false
		}
		r, n := Decode(s, *k)
		*k += n
		if r >= 0x10000 {
			hi, lo := utf16.EncodeRune(r)
			*pending = lo
			return hi, true
		}
		return r, true
	}
	for {
		ua, okA := next(a, &i, &pendingA)
		ub, okB := next(b, &j, &pendingB)
		switch {
		case !okA && !okB:
			return 0
		case !okA:
			return -1
		case !okB:
			return 1
		case ua != ub:
			if ua < ub {
				return -1
			}
			return 1
		}
	}
}
