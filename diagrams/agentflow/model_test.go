package agentflow

import (
	"context"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/preprocess"
)

func init() { core.Register(Diagram) }

// parseSource parses src as upstream's parse(src, { limit: Infinity }) does, returning the
// model or the error.
func parseSource(src string) (*agentflowDb, *perr.Error) {
	inf := math.Inf(1)
	if _, err := core.Parse(context.Background(), src, core.Options{Limit: &inf}); err != nil {
		return nil, err.(*perr.Error)
	}
	pre := preprocess.Preprocess(jsstr.Normalize(src), nil)
	model := Diagram.Parse(preprocess.EncodeEntities(pre.WithComments, nil)+"\n", pre.Config, pre.Title, pre.LineOffset,
		core.Limits{MaxEdges: core.DefaultMaxEdges})
	return model.(*agentflowDb), nil
}

// parseRaw is the compat specs' parser: the text as it is, with no preprocessing. raw also
// skips the trimming after a closing brace. It returns the error the parser throws, if any.
func parseRaw(src string, raw bool) (db *agentflowDb, err *perr.Error) {
	db = newAgentflowDb(agentflowDbOptions{maxEdges: 500})
	if err := catch(func() { parseAgentflow(src, db, raw, nil) }); err != nil {
		return nil, err
	}
	return db, nil
}

func mustParse(t *testing.T, src string) *agentflowDb {
	t.Helper()
	db, err := parseRaw(src, false)
	if err != nil {
		t.Fatalf("parse %q: %s", src, err.Message)
	}
	return db
}

func strs(list []string) []any { return anyList(list) }

func setIf(o *jsobj.Object, key string, value any, ok bool) {
	if ok {
		o.Set(key, value)
	}
}

func ptrValue(p *string) (any, bool) {
	if p == nil {
		return nil, false
	}
	return *p, true
}

// project is the reference's projection of a model: the same fields Mermaid's AgentFlowDB
// gives, from the graph built with Mermaid's palette of twelve and its basis curve.
func project(db *agentflowDb) *jsobj.Object {
	graph := buildAgentGraph(db, 12, "basis")
	out := jsobj.New()
	out.Set("direction", jsstr.Trim(db.direction))
	acc, _ := ptrValue(db.accTitle)
	if acc == nil {
		acc = ""
	}
	out.Set("accTitle", acc)
	descr, _ := ptrValue(db.accDescr)
	if descr == nil {
		descr = ""
	}
	out.Set("accDescr", descr)

	vertices := []any{}
	for _, v := range db.nodes.All() {
		o := jsobj.New()
		o.Set("id", v.id)
		o.Set("text", v.text)
		o.Set("labelType", string(v.labelType))
		setIf(o, "type", v.typ, v.typ != "")
		o.Set("styles", strs(v.styles))
		o.Set("classes", strs(v.classes))
		link, ok := ptrValue(v.link)
		setIf(o, "link", link, ok)
		target, ok := ptrValue(v.linkTarget)
		setIf(o, "linkTarget", target, ok)
		setIf(o, "metadata", v.metadata, v.metadata != nil)
		setIf(o, "isConnector", true, v.isConnector)
		setIf(o, "props", v.props, v.props != nil)
		vertices = append(vertices, o)
	}
	out.Set("vertices", vertices)

	edges := []any{}
	for _, e := range db.edges {
		o := jsobj.New()
		o.Set("start", e.start)
		o.Set("end", e.end)
		o.Set("type", e.typ)
		o.Set("text", e.text)
		o.Set("labelType", string(e.labelType))
		o.Set("classes", strs(e.classes))
		o.Set("isUserDefinedId", e.isUserDefinedID)
		setIf(o, "interpolate", e.interpolate, e.hasInterpolate)
		o.Set("stroke", e.stroke)
		setIf(o, "length", e.length, e.length != 0)
		setIf(o, "edgeSemantic", e.edgeSemantic, e.edgeSemantic != "")
		o.Set("id", e.id)
		setIf(o, "style", strs(e.style), e.style != nil)
		// A null animation from YAML is left out with an unset one; the reference has none.
		setIf(o, "animate", e.animate, e.animate != nil)
		setIf(o, "animation", e.animation, e.animation != nil)
		setIf(o, "metadata", e.metadata, e.metadata != nil)
		edges = append(edges, o)
	}
	out.Set("edges", edges)
	setIf(out, "defaultStyle", strs(db.defaultEdgeStyle), db.defaultEdgeStyle != nil)
	setIf(out, "defaultInterpolate", db.defaultInterpolate, db.defaultInterpolate != "")

	subGraphs := []any{}
	for _, sg := range db.subgraphs {
		o := jsobj.New()
		o.Set("id", sg.id)
		o.Set("nodes", strs(sg.nodes))
		o.Set("title", sg.title)
		o.Set("classes", strs(sg.classes))
		setIf(o, "dir", sg.dir, sg.dir != "")
		o.Set("labelType", string(sg.labelType))
		o.Set("type", "flow")
		setIf(o, "metadata", sg.metadata, sg.metadata != nil)
		subGraphs = append(subGraphs, o)
	}
	out.Set("subGraphs", subGraphs)

	classes := []any{}
	for _, c := range db.classes.All() {
		o := jsobj.New()
		o.Set("id", c.id)
		o.Set("styles", strs(c.styles))
		o.Set("textStyles", strs(c.textStyles))
		classes = append(classes, o)
	}
	out.Set("classes", classes)

	spaces := regexp.MustCompile(`\s+`)
	nodes := []any{}
	for _, n := range graph.nodes {
		o := jsobj.New()
		o.Set("id", n.id)
		o.Set("label", n.label)
		o.Set("labelType", string(n.labelType))
		o.Set("shape", n.shape)
		o.Set("isGroup", n.isGroup)
		setIf(o, "parentId", n.parentID, n.hasParent)
		o.Set("cssCompiledStyles", strs(n.cssCompiledStyles))
		// Mermaid marks a node's kind with a class; Pele keeps the kind on the node.
		classes := n.cssClasses
		if n.kind != "" {
			classes += " af-kind-" + n.kind
		}
		o.Set("cssClasses", strings.TrimSpace(spaces.ReplaceAllString(classes, " ")))
		setIf(o, "dir", n.dir, n.dir != "")
		link, ok := ptrValue(n.link)
		setIf(o, "link", link, ok)
		target, ok := ptrValue(n.linkTarget)
		setIf(o, "linkTarget", target, ok)
		tooltip, ok := ptrValue(n.tooltip)
		setIf(o, "tooltip", tooltip, ok)
		setIf(o, "colorIndex", float64(n.colorIndex), n.colorIndex >= 0)
		setIf(o, "metadata", n.metadata, n.metadata != nil)
		nodes = append(nodes, o)
	}
	out.Set("nodes", nodes)

	layoutEdges := []any{}
	for _, e := range graph.edges {
		o := jsobj.New()
		o.Set("id", e.id)
		o.Set("start", e.start)
		o.Set("end", e.end)
		o.Set("type", e.typ)
		o.Set("label", e.label)
		o.Set("labelType", string(e.labelType))
		o.Set("thickness", e.thickness)
		setIf(o, "minlen", e.minlen, e.minlen != 0)
		o.Set("arrowTypeStart", e.arrowTypeStart)
		o.Set("arrowTypeEnd", e.arrowTypeEnd)
		o.Set("cssCompiledStyles", strs(e.cssCompiledStyles))
		o.Set("style", strs(e.style))
		setIf(o, "curve", e.curve, e.curve != nil)
		setIf(o, "metadata", e.metadata, e.metadata != nil)
		o.Set("isUserDefinedId", e.isUserDefinedID)
		layoutEdges = append(layoutEdges, o)
	}
	out.Set("layoutEdges", layoutEdges)

	mappings := []any{}
	for _, m := range db.mappings {
		p := m.position
		mappings = append(mappings, []any{m.id, m.typ, float64(p.startLine), float64(p.startColumn), float64(p.endLine),
			float64(p.endColumn), float64(p.startIndex), float64(p.endIndex)})
	}
	out.Set("mappings", mappings)

	diagnostics := []any{}
	for _, d := range db.diagnostics {
		diagnostics = append(diagnostics, d.object())
	}
	out.Set("diagnostics", diagnostics)
	out.Set("semantic", db.semanticModel())
	return out
}

var markup = regexp.MustCompile(`[<>&]`)

// comparable is the reference's: Mermaid passes text through DOMPurify, which rewrites
// markup, so text holding any is not compared; Mermaid's DOM ids and click bookkeeping have no
// counterpart in Pele.
func comparable(v any) any {
	switch v := v.(type) {
	case string:
		if markup.MatchString(v) {
			return "(markup)"
		}
		return v
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = comparable(e)
		}
		return out
	case *jsobj.Object:
		out := jsobj.New()
		keys := v.Keys()
		slices.Sort(keys)
		for _, k := range keys {
			if k == "domId" || k == "haveCallback" {
				continue
			}
			e, _ := v.Get(k)
			out.Set(k, comparable(e))
		}
		return out
	}
	return v
}

// difference describes the first place a and b differ, or "" when they are equal.
func difference(path string, a, b any) string {
	switch a := a.(type) {
	case *jsobj.Object:
		bo, ok := b.(*jsobj.Object)
		if !ok {
			return path + ": got " + stringify(a) + ", want " + stringify(b)
		}
		ka, kb := a.Keys(), bo.Keys()
		slices.Sort(ka)
		slices.Sort(kb)
		if !slices.Equal(ka, kb) {
			return path + ": keys " + strings.Join(ka, ",") + ", want " + strings.Join(kb, ",")
		}
		for _, k := range ka {
			va, _ := a.Get(k)
			vb, _ := bo.Get(k)
			if d := difference(path+"."+k, va, vb); d != "" {
				return d
			}
		}
		return ""
	case []any:
		bl, ok := b.([]any)
		if !ok || len(a) != len(bl) {
			return path + ": got " + stringify(a) + ", want " + stringify(b)
		}
		for i := range a {
			if d := difference(path+"["+strconv.Itoa(i)+"]", a[i], bl[i]); d != "" {
				return d
			}
		}
		return ""
	}
	if a != b {
		return path + ": got " + stringify(a) + ", want " + stringify(b)
	}
	return ""
}

// catch runs fn and returns the error it throws, or nil.
func catch(fn func()) (err *perr.Error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*perr.Error)
			if !ok {
				panic(r)
			}
			err = e
		}
	}()
	fn()
	return nil
}
