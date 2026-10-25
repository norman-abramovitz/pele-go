package perr

import (
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

func TestInputOffset(t *testing.T) {
	// "  a#35;b": the leading blanks trimmed, then the code encoded as "aﬂ°°35¶ßb".
	steps := [][]Edit{{{From: 0, To: 2, Length: 0}}, {{From: 1, To: 5, Length: 7}}}
	for _, tc := range []struct{ at, want int }{{0, 2}, {1, 3}, {4, 6}, {7, 6}, {8, 7}, {9, 8}} {
		if got := InputOffset(steps, tc.at); got != tc.want {
			t.Errorf("InputOffset(%d) = %d, want %d", tc.at, got, tc.want)
		}
	}
}

func TestWrittenOnlyMapsTheParsedText(t *testing.T) {
	source := jsstr.UTF16("x\nab\n")
	o := NewOrigin("ab\n", func(at int) ([]uint16, int) { return source, at + 2 })
	if loc := Locate(o, jsstr.UTF16("ab\n"), 1); loc.Line != 2 || loc.Column != 2 || loc.Snippet != "ab\n-^" {
		t.Errorf("Locate(parsed, 1) = %+v", loc)
	}
	if loc := Locate(o, jsstr.UTF16("ab"), 1); loc.Line != 1 || loc.Column != 2 {
		t.Errorf("Locate(other, 1) = %+v", loc)
	}
	if loc := Locate(nil, jsstr.UTF16("ab\n"), 1); loc.Line != 1 || loc.Column != 2 {
		t.Errorf("Locate without an Origin = %+v", loc)
	}
}
