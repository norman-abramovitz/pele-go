package packet

import (
	"slices"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func build(src string) (m model, err *perr.Error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*perr.Error)
			if !ok {
				panic(r)
			}
			err = e
		}
	}()
	return buildPacket(parsePacket(src, nil), jsobj.New(), nil), nil
}

// Ported from tests/compat/packet/packet.spec.ts.
func TestRows(t *testing.T) {
	tests := map[string]struct {
		src  string
		want [][]block
	}{
		"beta":        {src: "packet-beta\n", want: nil},
		"single bits": {src: "packet\n0-10: \"test\"\n11: \"single\"\n", want: [][]block{{{0, 10, 11, "test"}, {11, 11, 1, "single"}}}},
		"bit counts":  {src: "packet\n+8: \"byte\"\n+16: \"word\"\n", want: [][]block{{{0, 7, 8, "byte"}, {8, 23, 16, "word"}}}},
		"split": {src: "packet\n0-10: \"test\"\n11-90: \"multiple\"\n", want: [][]block{
			{{0, 10, 11, "test"}, {11, 31, 20, "multiple"}},
			{{32, 63, 31, "multiple"}},
			{{64, 90, 26, "multiple"}},
		}},
		"split at row end": {src: "packet\n0-16: \"test\"\n17-63: \"multiple\"\n", want: [][]block{
			{{0, 16, 17, "test"}, {17, 31, 14, "multiple"}},
			{{32, 63, 31, "multiple"}},
		}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := build(tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.EqualFunc(m.rows, tc.want, slices.Equal) {
				t.Errorf("rows %v, want %v", m.rows, tc.want)
			}
		})
	}
}

func TestInvalid(t *testing.T) {
	tests := map[string]struct{ src, want string }{
		"gap":        {src: "packet\n0-10: \"test\"\n12-20: \"x\"\n", want: "Packet block 12 - 20 is not contiguous. It should start from 11."},
		"gap counts": {src: "packet\n+8: \"test\"\n9-20: \"x\"\n", want: "Packet block 9 - 20 is not contiguous. It should start from 8."},
		"gap single": {src: "packet\n0-10: \"test\"\n12: \"x\"\n", want: "Packet block 12 - 12 is not contiguous. It should start from 11."},
		"backwards":  {src: "packet\n0-10: \"test\"\n20-12: \"x\"\n", want: "Packet block 20 - 12 is invalid. End must be greater than start."},
		"zero bits":  {src: "packet\n+0: \"test\"\n", want: "Packet block 0 is invalid. Cannot have a zero bit field."},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := build(tc.src)
			if err == nil || err.Message != tc.want || err.Code != perr.Semantic {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}
