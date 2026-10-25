package sequence

import (
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func parse(t *testing.T, src string) (db *seqDb, err *perr.Error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*perr.Error)
			if !ok {
				panic(r)
			}
			err = e
		}
	}()
	db = newSeqDb(seqDbOptions{})
	parseSequence(jsstr.UTF16(src+"\n"), db, nil)
	return db, nil
}

func TestCentralConnections(t *testing.T) {
	tests := map[string]struct {
		arrow   string
		central int
		count   int
	}{
		"dual":    {arrow: "()->>()", central: lineCentralConnectionDual, count: 3},
		"to":      {arrow: "->>()", central: lineCentralConnection, count: 2},
		"reverse": {arrow: "()->>", central: lineCentralConnectionReverse, count: 2},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			db, err := parse(t, "sequenceDiagram\nparticipant Alice\nparticipant Bob\nAlice "+tc.arrow+" Bob: Hello")
			if err != nil {
				t.Fatal(err)
			}
			if len(db.messages) != tc.count {
				t.Fatalf("got %d messages, want %d", len(db.messages), tc.count)
			}
			m := db.messages[0]
			if m.from != "Alice" || m.to != "Bob" || m.message != "Hello" || m.typ != lineSolid || m.centralConnection != tc.central {
				t.Errorf("got %+v", m)
			}
		})
	}
}

func TestAutonumberDecimals(t *testing.T) {
	db, err := parse(t, "sequenceDiagram\nautonumber 10.1 .01\nAlice->Bob: Hi\nBob-->Alice: Hi")
	if err != nil {
		t.Fatal(err)
	}
	nums := numberMessages(db.messages, false)
	if nums.index[1] != 10.1 || nums.index[2] != 10.11 || !nums.shown[1] {
		t.Errorf("got %v %v", nums.index, nums.shown)
	}
	if _, err := parse(t, "sequenceDiagram\nautonumber 10.001\nAlice->Bob: Hi"); err == nil || err.Code != perr.Syntax {
		t.Errorf("thousandths: got %v, want a syntax error", err)
	}
}

func TestSemanticErrors(t *testing.T) {
	tests := map[string]struct {
		src  string
		want string
	}{
		"inactive": {
			src:  "sequenceDiagram\nAlice->>Bob: Hi\ndeactivate Bob",
			want: "Trying to inactivate an inactive participant (Bob)",
		},
		"two boxes": {
			src:  "sequenceDiagram\nbox Aqua\nparticipant A\nend\nbox\nparticipant A\nend",
			want: "A same participant should only be defined in one Box: A can't be in 'undefined' and in 'undefined' at the same time.",
		},
		"created twice": {
			src:  "sequenceDiagram\nparticipant A\ncreate participant A\nB->>A: Hi",
			want: "It is not possible to have actors with the same id, even if one is destroyed before the next is created. Use 'AS' aliases to simulate the behavior",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parse(t, tc.src)
			if err == nil || err.Code != perr.Semantic || err.Message != tc.want {
				t.Errorf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseBoxData(t *testing.T) {
	tests := map[string]struct {
		in   string
		want seqBoxData
	}{
		"keyword":    {in: "Aqua Group", want: seqBoxData{text: "Group", color: "Aqua"}},
		"function":   {in: "rgb(1, 2, 3) Group", want: seqBoxData{text: "Group", color: "rgb(1, 2, 3)"}},
		"no color":   {in: "Group of things", want: seqBoxData{text: "Group of things", color: "transparent"}},
		"color only": {in: "hsl(10deg 20% 30% / 0.5)", want: seqBoxData{color: "hsl(10deg 20% 30% / 0.5)"}},
		"kelvin":     {in: "darK\u212ahaki Group", want: seqBoxData{text: "darK\u212ahaki Group", color: "transparent"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := newSeqDb(seqDbOptions{}).parseBoxData(tc.in)
			if got.text != tc.want.text || got.color != tc.want.color || got.wrap != nil {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestIsCssColor(t *testing.T) {
	tests := map[string]bool{
		"red":                   true,
		"RED":                   true,
		"dar\u212akhaki":        true,
		"l\u0130me":             false,
		"rgb(1 2 3 / 50%)":      true,
		"rgb(1, 2%, 3)":         false,
		"hsl(1turn, 20%, 30%)":  true,
		"hsla(1, 20%, 30%, .5)": true,
		"rgb(none 2 3)":         true,
		"calc(1)":               false,
	}
	for color, want := range tests {
		t.Run(color, func(t *testing.T) {
			if got := isCssColor(color); got != want {
				t.Errorf("isCssColor(%q) = %v, want %v", color, got, want)
			}
		})
	}
}
