package flowchart

import (
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// GraphNode is a node as drawn. Empty strings and zero numbers stand for upstream's
// undefined, as in FlowNode; ColorIndex is -1 when unset.
type GraphNode struct {
	ID        string
	Label     string
	LabelType LabelType
	Shape     string
	IsGroup   bool
	// ParentID is the group the node is in, when HasParent: a subgraph's id can be "".
	ParentID          string
	HasParent         bool
	CSSStyles         []string
	CSSCompiledStyles []string
	CSSClasses        string
	Dir               string
	Link              string
	LinkTarget        string
	Tooltip           string
	Icon              string
	Pos               string
	Img               string
	AssetWidth        float64
	AssetHeight       float64
	Constraint        string
	ColorIndex        int
	Metadata          *jsobj.Object
}

// GraphEdge is an edge as drawn. Curve is "" unless it names a curve.
type GraphEdge struct {
	ID                string
	IsUserDefinedID   bool
	Start             string
	End               string
	Type              string
	Label             string
	LabelType         LabelType
	Thickness         string
	Pattern           string
	Minlen            float64
	ArrowTypeStart    string
	ArrowTypeEnd      string
	CSSCompiledStyles []string
	Style             []string
	Classes           string
	Animate           any
	Animation         any
	Curve             string
}

// FlowGraph is what a flowchart draws.
type FlowGraph struct {
	Nodes []*GraphNode
	Edges []*GraphEdge
}

func shapeOf(vertex *FlowNode) string {
	if vertex.Img != "" {
		return "imageSquare"
	}
	if vertex.Icon != "" {
		switch vertex.Form {
		case "circle":
			return "iconCircle"
		case "square":
			return "iconSquare"
		case "rounded":
			return "iconRounded"
		}
		return "icon"
	}
	switch vertex.Type {
	case "square", "":
		return "squareRect"
	case "round":
		return "roundedRect"
	}
	return vertex.Type
}

func arrows(typ string) (string, string) {
	switch typ {
	case "arrow_point", "arrow_circle", "arrow_cross":
		return "none", typ
	case "double_arrow_point", "double_arrow_circle", "double_arrow_cross":
		single := strings.Replace(typ, "double_", "", 1)
		return single, single
	}
	return "none", "arrow_point"
}

func isCollapsedView(metadata *jsobj.Object) bool {
	if metadata == nil {
		return false
	}
	v, _ := metadata.Get("view")
	return v == "collapsed"
}

// firstTruthy is a || b || c over upstream's values: the first truthy one, or the last.
func firstTruthy(values ...any) any {
	for _, v := range values[:len(values)-1] {
		if jsobj.Truthy(v) {
			return v
		}
	}
	return values[len(values)-1]
}

// buildFlowGraph resolves a parsed flowchart into the nodes and edges that are actually
// drawn: group membership, collapsed subgraphs, class styles and edge defaults. defaultCurve
// is "" when unset.
func buildFlowGraph(model *FlowDb, defaultCurve string) *FlowGraph {
	graph := &FlowGraph{}
	subgraphs := model.Subgraphs

	compiled := func(classNames []string) []string {
		out := []string{}
		for _, name := range classNames {
			def := model.Classes[name]
			if def == nil {
				continue
			}
			for _, s := range def.Styles {
				out = append(out, jsstr.Trim(s))
			}
			for _, s := range def.TextStyles {
				out = append(out, jsstr.Trim(s))
			}
		}
		return out
	}

	subgraphParent := map[string]string{}
	for _, sg := range subgraphs {
		for _, childID := range sg.Nodes {
			if model.Subgraph(childID) != nil {
				subgraphParent[childID] = sg.ID
			}
		}
	}

	declarationIndex := map[string]int{}
	childrenOf := map[string][]string{}
	for _, sg := range subgraphs {
		if parent, ok := subgraphParent[sg.ID]; ok {
			childrenOf[parent] = append(childrenOf[parent], sg.ID)
		}
	}
	nextIndex := 0
	for _, sg := range subgraphs {
		if _, ok := subgraphParent[sg.ID]; ok {
			continue
		}
		pending := []string{sg.ID}
		for len(pending) > 0 {
			id := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if _, done := declarationIndex[id]; done {
				continue
			}
			declarationIndex[id] = nextIndex
			nextIndex++
			kids := childrenOf[id]
			for i := len(kids) - 1; i >= 0; i-- {
				pending = append(pending, kids[i])
			}
		}
	}

	isCollapsed := func(id string) bool {
		sg := model.Subgraph(id)
		return sg != nil && isCollapsedView(sg.Metadata)
	}
	anyCollapsed := false
	for _, sg := range subgraphs {
		if isCollapsedView(sg.Metadata) {
			anyCollapsed = true
		}
	}
	// The outermost collapsed subgraph around each subgraph, resolved once per chain of
	// parents. "" is none: a subgraph id is never empty once it is collapsed into.
	type found struct {
		id string
		ok bool
	}
	outermost := map[string]found{}
	outermostCollapsed := func(id string) found {
		var chain []string
		seen := map[string]bool{}
		current, has := id, true
		for has {
			if _, done := outermost[current]; done || seen[current] {
				break
			}
			seen[current] = true
			chain = append(chain, current)
			current, has = subgraphParent[current]
		}
		var result found
		if has {
			result = outermost[current]
		}
		for i := len(chain) - 1; i >= 0; i-- {
			if !result.ok && isCollapsed(chain[i]) {
				result = found{chain[i], true}
			}
			outermost[chain[i]] = result
		}
		return result
	}

	hidden := map[string]bool{}
	collapsedInto := map[string]string{}
	if anyCollapsed {
		for _, sg := range subgraphs {
			ancestor := outermostCollapsed(sg.ID)
			if !ancestor.ok {
				continue
			}
			if sg.ID != ancestor.id {
				hidden[sg.ID] = true
				collapsedInto[sg.ID] = ancestor.id
			}
			for _, childID := range sg.Nodes {
				if childID == ancestor.id {
					continue
				}
				hidden[childID] = true
				collapsedInto[childID] = ancestor.id
			}
		}
	}

	parentOf := map[string]string{}
	groups := map[string]bool{}
	for i := len(subgraphs) - 1; i >= 0; i-- {
		sg := subgraphs[i]
		if hidden[sg.ID] {
			continue
		}
		if len(sg.Nodes) > 0 {
			groups[sg.ID] = true
		}
		for _, id := range sg.Nodes {
			parentOf[id] = sg.ID
		}
	}

	byID := map[string]*GraphNode{}
	for i := len(subgraphs) - 1; i >= 0; i-- {
		sg := subgraphs[i]
		if hidden[sg.ID] {
			continue
		}
		collapsed := isCollapsedView(sg.Metadata)
		parent, hasParent := parentOf[sg.ID]
		colorIndex, ok := declarationIndex[sg.ID]
		if !ok {
			colorIndex = -1
		}
		node := &GraphNode{
			ID:                sg.ID,
			Label:             sg.Title,
			LabelType:         sg.LabelType,
			ParentID:          parent,
			HasParent:         hasParent,
			CSSStyles:         []string{},
			CSSCompiledStyles: compiled(sg.Classes),
			CSSClasses:        strings.Join(sg.Classes, " "),
			Shape:             "rect",
			Dir:               sg.Dir,
			IsGroup:           !collapsed,
			ColorIndex:        colorIndex,
		}
		if collapsed {
			node.Shape = "collapsedGroup"
		} else {
			node.Metadata = sg.Metadata
		}
		graph.Nodes = append(graph.Nodes, node)
		byID[sg.ID] = node
	}

	for _, vertex := range model.Nodes.All() {
		if hidden[vertex.ID] {
			continue
		}
		if existing := byID[vertex.ID]; existing != nil {
			existing.CSSStyles = vertex.Styles
			existing.CSSCompiledStyles = compiled(vertex.Classes)
			existing.CSSClasses = strings.Join(vertex.Classes, " ")
			continue
		}
		isGroup := groups[vertex.ID]
		parent, hasParent := parentOf[vertex.ID]
		node := &GraphNode{
			ID:                vertex.ID,
			Label:             vertex.Text,
			LabelType:         vertex.LabelType,
			ParentID:          parent,
			HasParent:         hasParent,
			CSSStyles:         vertex.Styles,
			CSSCompiledStyles: compiled(append([]string{"default", "node"}, vertex.Classes...)),
			CSSClasses:        "default " + strings.Join(vertex.Classes, " "),
			Dir:               vertex.Dir,
			Link:              vertex.Link,
			LinkTarget:        vertex.LinkTarget,
			Tooltip:           model.Tooltips[vertex.ID],
			Icon:              vertex.Icon,
			Pos:               vertex.Pos,
			Img:               vertex.Img,
			AssetWidth:        vertex.AssetWidth,
			AssetHeight:       vertex.AssetHeight,
			Constraint:        vertex.Constraint,
			IsGroup:           isGroup,
			ColorIndex:        -1,
			Shape:             "rect",
		}
		if !isGroup {
			node.Shape = shapeOf(vertex)
		}
		graph.Nodes = append(graph.Nodes, node)
	}

	for index, raw := range model.Edges {
		start, startCollapsed := collapsedInto[raw.Start]
		if !startCollapsed {
			start = raw.Start
		}
		end, endCollapsed := collapsedInto[raw.End]
		if !endCollapsed {
			end = raw.End
		}
		if start == end && (startCollapsed || endCollapsed) {
			continue
		}

		arrowStart, arrowEnd := arrows(raw.Type)
		styles := append([]string{}, model.DefaultEdgeStyle...)
		styles = append(styles, raw.Style...)
		plain := raw.Stroke == "invisible" || raw.Type == "arrow_open"
		id := raw.ID
		if id == "" {
			id = "L_" + start + "_" + end + "_" + strconv.Itoa(index)
		}
		edge := &GraphEdge{
			ID:                id,
			IsUserDefinedID:   raw.IsUserDefinedID,
			Start:             start,
			End:               end,
			Type:              raw.Type,
			Label:             raw.Text,
			LabelType:         raw.LabelType,
			Thickness:         raw.Stroke,
			Pattern:           raw.Stroke,
			Minlen:            raw.Length,
			Classes:           "edge-thickness-normal edge-pattern-solid flowchart-link",
			ArrowTypeStart:    arrowStart,
			ArrowTypeEnd:      arrowEnd,
			CSSCompiledStyles: compiled(raw.Classes),
			Style:             styles,
			Animate:           raw.Animate,
			Animation:         raw.Animation,
		}
		if edge.Type == "" {
			edge.Type = "normal"
		}
		if raw.Stroke == "invisible" {
			edge.Classes = ""
		}
		if plain {
			edge.ArrowTypeStart, edge.ArrowTypeEnd = "none", "none"
		}
		var defaultInterpolate, curve any
		if model.DefaultInterpolate != "" {
			defaultInterpolate = model.DefaultInterpolate
		}
		if defaultCurve != "" {
			curve = defaultCurve
		}
		edge.Curve, _ = firstTruthy(raw.Interpolate, defaultInterpolate, curve).(string)
		graph.Edges = append(graph.Edges, edge)
	}

	return graph
}
