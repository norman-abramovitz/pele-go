package treeview

import (
	"strings"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

// Ported from tests/compat/treeview/boxDrawingPreprocessor.spec.ts.
func TestPreprocessBoxDrawing(t *testing.T) {
	tests := map[string]struct {
		input   []string
		want    []string
		lineMap map[int]int
	}{
		"simple tree": {
			input:   []string{"treeView-beta", "root/", "├── src/", "│   └── index.js", "└── README.md"},
			want:    []string{"treeView-beta", "root/", "    src/", "        index.js", "    README.md"},
			lineMap: map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5},
		},
		"decoration lines skipped": {
			input:   []string{"treeView-beta", "├── a.txt", "│", "└── b.txt"},
			want:    []string{"treeView-beta", "    a.txt", "    b.txt"},
			lineMap: map[int]int{1: 1, 2: 2, 3: 4},
		},
		"heavy variants with extra dashes": {
			input: []string{"treeView-beta", "┣━━━ src/", "┃   ┗━ a.ts"},
			want:  []string{"treeView-beta", "    src/", "        a.ts"},
		},
		"tabs": {
			input: []string{"treeView-beta", "├── src/", "│\t├── index.ts", "│\t└── utils.ts"},
			want:  []string{"treeView-beta", "    src/", "        index.ts", "        utils.ts"},
		},
		"metadata passes": {
			input: []string{"treeView-beta", "title My Tree", "accTitle: T", "├── file.txt"},
			want:  []string{"treeView-beta", "title My Tree", "accTitle: T", "    file.txt"},
		},
		"indent-based input unchanged": {
			input: []string{"treeView-beta", "    src/", "        a.ts"},
			want:  []string{"treeView-beta", "    src/", "        a.ts"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, lineMap, _ := preprocessBoxDrawing(strings.Join(tc.input, "\n"), nil)
			if want := strings.Join(tc.want, "\n"); got != want {
				t.Errorf("text = %q, want %q", got, want)
			}
			for out, original := range tc.lineMap {
				if lineMap[out] != original {
					t.Errorf("lineMap[%d] = %d, want %d", out, lineMap[out], original)
				}
			}
		})
	}
}

func TestPreprocessBoxDrawingErrors(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"empty content": {
			input: "treeView-beta\nroot/\n├── a\n└──",
			want:  "Line 4: Empty node — expected a filename or directory name after the box-drawing prefix",
		},
		"indent without box chars": {
			input: "treeView-beta\n├── a\n    b",
			want:  "Line 3: Unexpected indentation without box-drawing characters. In box-drawing format, use ├── or └── prefixes for indented nodes.",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				e, ok := recover().(*perr.Error)
				if !ok || e.Message != tc.want {
					t.Fatalf("threw %v, want %q", e, tc.want)
				}
			}()
			preprocessBoxDrawing(tc.input, nil)
		})
	}
}

// Errors in box-drawing input name the line of the original text.
func TestReadTreeViewRemapsErrorLines(t *testing.T) {
	defer func() {
		e, ok := recover().(*perr.Error)
		if !ok {
			t.Fatalf("threw %v, want a *perr.Error", e)
		}
		if e.Line != 5 || !strings.HasPrefix(e.Message, "Parse error on line 5:\n") {
			t.Fatalf("line %d, message %q; want line 5", e.Line, e.Message)
		}
	}()
	readTreeView("treeView-beta\n├── a\n│\n└── b\ntitle x\n", nil)
}

// Ported from tests/compat/treeview/treeViewValueConverter.test.ts.
func TestConvertValue(t *testing.T) {
	tests := map[string]struct {
		rule, input, want string
	}{
		"double quotes":     {rule: "QUOTED_NAME", input: `"my file.js"`, want: "my file.js"},
		"single quotes":     {rule: "QUOTED_NAME", input: `'my folder/'`, want: "my folder/"},
		"empty quotes":      {rule: "QUOTED_NAME", input: `""`, want: ""},
		"class":             {rule: "CLASS_ANNOTATION", input: " :::highlight", want: "highlight"},
		"class with hyphen": {rule: "CLASS_ANNOTATION", input: " :::my-class", want: "my-class"},
		"class trimmed":     {rule: "CLASS_ANNOTATION", input: "  :::highlight  ", want: "highlight"},
		"bare inner spaces": {rule: "BARE_NAME", input: "But  _  _ton💓.tsx", want: "But  _  _ton💓.tsx"},
		"bare trailing":     {rule: "BARE_NAME", input: "index.js  ", want: "index.js"},
		"icon":              {rule: "ICON_ANNOTATION", input: " icon(folder)", want: "folder"},
		"icon with pack":    {rule: "ICON_ANNOTATION", input: " icon(logos:react)", want: "logos:react"},
		"empty icon":        {rule: "ICON_ANNOTATION", input: " icon()", want: ""},
		"description":       {rule: "DESC_ANNOTATION", input: " ## main entry ", want: "main entry"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := convertValue(tc.rule, tc.input); got != tc.want {
				t.Errorf("convertValue(%s, %q) = %q, want %q", tc.rule, tc.input, got, tc.want)
			}
		})
	}
}

// Ported from tests/compat/treeview/db.spec.ts: indentation nests nodes under the nearest
// shallower one.
func TestTreeViewDb(t *testing.T) {
	db := newTreeViewDb()
	db.addNode(0, "src", nodeDirectory, "", "", "")
	db.addNode(4, "index.ts", nodeFile, "", "", "")
	db.addNode(8, "deep.ts", nodeFile, "", "", "")
	db.addNode(4, "util.ts", nodeFile, "", "", "")
	db.addNode(0, "README.md", nodeFile, "", "", "")
	root := db.root
	if db.count != 6 || len(root.children) != 2 {
		t.Fatalf("count %d, %d top-level nodes; want 6, 2", db.count, len(root.children))
	}
	src := root.children[0]
	if len(src.children) != 2 || src.children[0].name != "index.ts" || src.children[1].name != "util.ts" {
		t.Fatalf("src children %v", src.children)
	}
	if len(src.children[0].children) != 1 || root.children[1].name != "README.md" {
		t.Fatalf("unexpected tree under %v", root.children)
	}
}

// Ported from tests/compat/treeview/icons.spec.ts.
func TestGetNodeIcon(t *testing.T) {
	tests := map[string]struct {
		node   treeNode
		config iconSettings
		want   string
	}{
		"annotation wins":     {node: treeNode{name: "a.ts", icon: "logos:react"}, want: "logos:react"},
		"none hides":          {node: treeNode{name: "a.ts", icon: "none"}, config: iconSettings{showIcons: true}, want: ""},
		"hidden by default":   {node: treeNode{name: "a.ts"}, want: ""},
		"builtin file":        {node: treeNode{name: "a.ts"}, config: iconSettings{showIcons: true}, want: "mermaid-treeview:file"},
		"builtin folder":      {node: treeNode{name: "src", nodeType: nodeDirectory}, config: iconSettings{showIcons: true}, want: "mermaid-treeview:folder"},
		"glyph stays builtin": {node: treeNode{name: "a", icon: "folder"}, config: iconSettings{defaultIconPack: "pack"}, want: "mermaid-treeview:folder"},
		"default pack":        {node: treeNode{name: "a", icon: "docker"}, config: iconSettings{defaultIconPack: "pack"}, want: "pack:docker"},
		"no pack is builtin":  {node: treeNode{name: "a", icon: "docker"}, want: "mermaid-treeview:docker"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := getNodeIcon(&tc.node, tc.config); got != tc.want {
				t.Errorf("getNodeIcon = %q, want %q", got, tc.want)
			}
		})
	}
}
