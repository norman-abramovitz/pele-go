package pie

import (
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func parse(t *testing.T, src string) (m model, err *perr.Error) {
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
	return buildPie(parsePie(src, nil), nil), nil
}

// Ported from tests/compat/pie/pie.spec.ts.
func TestSections(t *testing.T) {
	tests := map[string]struct {
		src      string
		showData bool
		title    string
		want     [][2]any
	}{
		"simple":           {src: "pie\n\"ash\" : 60\n\"bat\" : 40\n", want: [][2]any{{"ash", 60.0}, {"bat", 40.0}}},
		"show data":        {src: "pie showData\n\"ash\" : 60\n", showData: true, want: [][2]any{{"ash", 60.0}}},
		"title":            {src: "pie title a 60/40 pie\n\"ash\" : 60\n", title: "a 60/40 pie", want: [][2]any{{"ash", 60.0}}},
		"first value wins": {src: "pie\n\"ash\" : 60\n\"ash\" : 40\n", want: [][2]any{{"ash", 60.0}}},
		"decimals":         {src: "pie\n\"ash\" : 60.67\n\"bat\" : 40\n", want: [][2]any{{"ash", 60.67}, {"bat", 40.0}}},
		"zero":             {src: "pie\n\"ash\" : 0\n", want: [][2]any{{"ash", 0.0}}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := parse(t, tc.src)
			if err != nil {
				t.Fatal(err)
			}
			var got [][2]any
			for label, value := range m.sections.All() {
				got = append(got, [2]any{label, value})
			}
			if len(got) != len(tc.want) {
				t.Fatalf("sections %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("section %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
			if m.showData != tc.showData || m.title != tc.title {
				t.Errorf("showData %v title %q, want %v %q", m.showData, m.title, tc.showData, tc.title)
			}
		})
	}
}

func TestNegativeValue(t *testing.T) {
	_, err := parse(t, "pie\n\"ash\" : -60\n")
	want := `"ash" has invalid value: -60. Negative values are not allowed in pie charts. All slice values must be >= 0.`
	if err == nil || err.Message != want || err.Code != perr.Semantic {
		t.Fatalf("got %v, want %q", err, want)
	}
}
