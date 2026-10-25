package flowchart

import (
	"math"
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/omap"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
	"github.com/Broderick-Westrope/pele-go/internal/yaml"
)

// flowDbOptions configure a FlowDb. Upstream's warn callback and direction are never set by
// a diagram type, so they are left out.
type flowDbOptions struct {
	MaxEdges   float64
	InheritDir bool
}

func labelType(typ any) LabelType {
	switch typ {
	case "markdown", "string", "text":
		return LabelType(typ.(string))
	}
	return "markdown"
}

// unquote is upstream's: substring(1, length - 1), which leaves a lone `"` as it is.
func unquote(text string) string {
	if len(text) >= 2 && strings.HasPrefix(text, `"`) && strings.HasSuffix(text, `"`) {
		return text[1 : len(text)-1]
	}
	return text
}

type linkEnds struct {
	typ, stroke string
	length      float64
}

func destructStartLink(raw string) linkEnds {
	str := jsstr.Trim(raw)
	typ := "arrow_open"
	if str != "" {
		switch str[0] {
		case '<':
			typ = "arrow_point"
			str = str[1:]
		case 'x':
			typ = "arrow_cross"
			str = str[1:]
		case 'o':
			typ = "arrow_circle"
			str = str[1:]
		}
	}
	stroke := "normal"
	if strings.Contains(str, "=") {
		stroke = "thick"
	}
	if strings.Contains(str, ".") {
		stroke = "dotted"
	}
	return linkEnds{typ: typ, stroke: stroke}
}

// destructEndLink reads a link token, which is ASCII once trimmed.
func destructEndLink(raw string) linkEnds {
	str := jsstr.Trim(raw)
	line := str[:max(0, len(str)-1)]
	typ := "arrow_open"
	last := byte(0)
	if str != "" {
		last = str[len(str)-1]
	}
	double := func(first string) {
		if strings.HasPrefix(str, first) {
			typ = "double_" + typ
			line = line[min(1, len(line)):]
		}
	}
	switch last {
	case 'x':
		typ = "arrow_cross"
		double("x")
	case '>':
		typ = "arrow_point"
		double("<")
	case 'o':
		typ = "arrow_circle"
		double("o")
	}
	stroke := "normal"
	length := float64(len(line) - 1)
	if strings.HasPrefix(line, "=") {
		stroke = "thick"
	}
	if strings.HasPrefix(line, "~") {
		stroke = "invisible"
	}
	if dots := strings.Count(line, "."); dots > 0 {
		stroke = "dotted"
		length = float64(dots)
	}
	return linkEnds{typ: typ, stroke: stroke, length: length}
}

// FlowDb is the flowchart model the parser builds.
type FlowDb struct {
	Direction          string
	Nodes              omap.Map[string, *FlowNode]
	Edges              []*FlowEdge
	Subgraphs          []*FlowSubgraph
	Classes            map[string]*FlowClassDef
	Tooltips           map[string]string
	DefaultEdgeStyle   []string
	DefaultInterpolate string
	// Title, AccTitle and AccDescr are "" when unset: every use treats the two alike.
	Title    string
	AccTitle string
	AccDescr string

	subgraphLookup map[string]*FlowSubgraph
	members        map[string]bool
	edgeByID       map[string]*FlowEdge
	pairCount      map[string]int
	subCount       int
	options        flowDbOptions
}

// newFlowDb returns an empty model.
func newFlowDb(options flowDbOptions) *FlowDb {
	return &FlowDb{
		Classes:        map[string]*FlowClassDef{},
		Tooltips:       map[string]string{},
		subgraphLookup: map[string]*FlowSubgraph{},
		members:        map[string]bool{},
		edgeByID:       map[string]*FlowEdge{},
		pairCount:      map[string]int{},
		options:        options,
	}
}

// toNumber is Number(v) for a YAML value.
func toNumber(v any) float64 {
	switch v := v.(type) {
	case float64:
		return v
	case bool:
		if v {
			return 1
		}
		return 0
	case nil:
		return 0
	case *jsobj.Object:
		return math.NaN()
	}
	return jsnum.Parse(jsobj.ToString(v))
}

func flowError(message string, code perr.Code) {
	e := perr.New(message, code)
	e.Type = "flowchart"
	perr.Throw(e)
}

// addVertex adds or updates a vertex. text and metadata are nil, and typ "", when not given.
func (db *FlowDb) addVertex(id string, text *FlowText, typ string, style []string, metadata *string) {
	if id == "" || jsstr.Trim(id) == "" {
		return
	}

	var doc *jsobj.Object
	if metadata != nil {
		src := "{\n" + *metadata + "\n}"
		if strings.Contains(*metadata, "\n") {
			src = *metadata + "\n"
		}
		parsed, err := yaml.Parse(src)
		if err != nil {
			flowError(`Metadata of "`+id+`" is not valid YAML. `+err.Error(), perr.Syntax)
		}
		doc, _ = parsed.(*jsobj.Object)
	}

	if subgraph := db.subgraphLookup[id]; subgraph != nil && doc != nil {
		merged := jsobj.New()
		for _, m := range []*jsobj.Object{subgraph.Metadata, doc} {
			if m == nil {
				continue
			}
			for _, k := range m.Keys() {
				v, _ := m.Get(k)
				merged.Set(k, v)
			}
		}
		subgraph.Metadata = merged
		return
	}

	if edge := db.edgeByID[id]; edge != nil {
		if doc != nil {
			if v, ok := doc.Get("animate"); ok {
				edge.Animate = v
			}
			if v, ok := doc.Get("animation"); ok {
				edge.Animation = v
			}
			if v, ok := doc.Get("curve"); ok {
				edge.Interpolate = v
			}
		}
		return
	}

	vertex, ok := db.Nodes.Get(id)
	if !ok {
		vertex = &FlowNode{ID: id, LabelType: "text", Styles: []string{}, Classes: []string{}}
		db.Nodes.Set(id, vertex)
	}

	if text != nil {
		vertex.LabelType = text.Type
		vertex.Text = unquote(jsstr.Trim(text.Text))
	} else if !ok {
		vertex.Text = id
	}
	if typ != "" {
		vertex.Type = typ
	}
	vertex.Styles = append(vertex.Styles, style...)

	if doc == nil {
		return
	}

	get := func(key string) (any, bool) {
		v, _ := doc.Get(key)
		return v, jsobj.Truthy(v)
	}
	if v, ok := get("shape"); ok {
		shape := jsobj.ToString(v)
		if jsstr.ChangesWhenLowered(shape) || strings.Contains(shape, "_") {
			flowError("No such shape: "+shape+". Shape names should be lowercase.", perr.Semantic)
		}
		if !IsValidShape(shape) {
			flowError("No such shape: "+shape+".", perr.Semantic)
		}
		vertex.Type = shape
	}
	labelValue, _ := doc.Get("label")
	label, isString := labelValue.(string)
	noLabel := !isString || jsstr.Trim(label) == ""
	if v, ok := get("label"); ok {
		vertex.Text = jsobj.ToString(v)
		lt, _ := doc.Get("labelType")
		vertex.LabelType = labelType(lt)
	}
	if v, ok := get("icon"); ok {
		vertex.Icon = jsobj.ToString(v)
		if noLabel && vertex.Text == id {
			vertex.Text = ""
		}
	}
	if v, ok := get("form"); ok {
		vertex.Form = jsobj.ToString(v)
	}
	if v, ok := get("pos"); ok {
		vertex.Pos = jsobj.ToString(v)
	}
	if v, ok := get("img"); ok {
		vertex.Img = jsobj.ToString(v)
		if noLabel && vertex.Text == id {
			vertex.Text = ""
		}
	}
	if v, ok := get("constraint"); ok {
		vertex.Constraint = jsobj.ToString(v)
	}
	if v, ok := get("w"); ok {
		vertex.AssetWidth = toNumber(v)
	}
	if v, ok := get("h"); ok {
		vertex.AssetHeight = toNumber(v)
	}
}

func (db *FlowDb) addSingleLink(start, end string, info *LinkInfo, id string) {
	edge := &FlowEdge{
		Start:       start,
		End:         end,
		LabelType:   "text",
		Classes:     []string{},
		Interpolate: nil,
	}
	if db.DefaultInterpolate != "" {
		edge.Interpolate = db.DefaultInterpolate
	}
	if info.Text != nil {
		edge.Text = unquote(jsstr.Trim(info.Text.Text))
		edge.LabelType = labelType(string(info.Text.Type))
	}
	edge.Type = info.Type
	edge.Stroke = info.Stroke
	edge.Length = min(info.Length, 10)
	pair := start + "\x00" + end
	if _, taken := db.edgeByID[id]; id != "" && !taken {
		edge.ID = id
		edge.IsUserDefinedID = true
	} else {
		existing := db.pairCount[pair]
		n := 0
		if existing != 0 {
			n = existing + 1
		}
		edge.ID = "L_" + start + "_" + end + "_" + strconv.Itoa(n)
	}

	if limit := db.options.MaxEdges; float64(len(db.Edges)) >= limit {
		flowError("Edge limit exceeded. The limit is "+jsnum.Format(limit)+".", perr.Limit)
	}
	db.Edges = append(db.Edges, edge)
	db.pairCount[pair]++
	if _, taken := db.edgeByID[edge.ID]; !taken {
		db.edgeByID[edge.ID] = edge
	}
}

// addLink adds an edge from each of starts to each of ends.
func (db *FlowDb) addLink(starts, ends []string, info *LinkInfo) {
	id := strings.Replace(info.ID, "@", "", 1)
	lastStart := starts[len(starts)-1]
	firstEnd := ends[0]
	for _, start := range starts {
		for _, end := range ends {
			edgeID := ""
			if start == lastStart && end == firstEnd {
				edgeID = id
			}
			db.addSingleLink(start, end, info, edgeID)
		}
	}
}

// updateLinkInterpolate sets the curve of the edges at positions, or the default one.
func (db *FlowDb) updateLinkInterpolate(positions []string, interpolate string) {
	for _, pos := range positions {
		if pos == "default" {
			db.DefaultInterpolate = interpolate
		} else {
			db.edgeAt(pos).Interpolate = interpolate
		}
	}
}

// updateLink sets the style of the edges at positions, or the default one.
func (db *FlowDb) updateLink(positions []string, style []string) {
	// Upstream gives every edge the one array, and adds fill:none to it at the first edge.
	var styled []*FlowEdge
	for _, pos := range positions {
		if pos == "default" {
			db.DefaultEdgeStyle = style
			continue
		}
		edge := db.edgeAt(pos)
		styled = append(styled, edge)
		hasFill := false
		for _, s := range style {
			if strings.HasPrefix(s, "fill") {
				hasFill = true
			}
		}
		if len(style) > 0 && !hasFill {
			style = append(style, "fill:none")
		}
		for _, e := range styled {
			e.Style = style
		}
	}
}

func (db *FlowDb) edgeAt(pos string) *FlowEdge {
	i := jsnum.Parse(pos)
	if i >= 0 && i < float64(len(db.Edges)) && i == float64(int(i)) {
		return db.Edges[int(i)]
	}
	flowError("The index "+pos+" for linkStyle is out of bounds. Valid indices for linkStyle are between 0 and "+
		strconv.Itoa(len(db.Edges)-1)+". (Help: Ensure that the index is within the range of existing edges.)", perr.Semantic)
	return nil
}

// addClass defines the classes ids, a comma-separated list.
func (db *FlowDb) addClass(ids string, styles []string) {
	joined := strings.Join(styles, ",")
	joined = strings.ReplaceAll(joined, `\,`, "§§§")
	joined = strings.ReplaceAll(joined, ",", ";")
	joined = strings.ReplaceAll(joined, "§§§", ",")
	style := strings.Split(joined, ";")
	for _, id := range strings.Split(ids, ",") {
		classNode := db.Classes[id]
		if classNode == nil {
			classNode = &FlowClassDef{ID: id, Styles: []string{}, TextStyles: []string{}}
			db.Classes[id] = classNode
		}
		for _, s := range style {
			if strings.Contains(s, "color") {
				classNode.TextStyles = append(classNode.TextStyles, strings.Replace(s, "fill", "bgFill", 1))
			}
			classNode.Styles = append(classNode.Styles, s)
		}
	}
}

// setDirection sets the direction the diagram runs in.
func (db *FlowDb) setDirection(dir string) {
	d := jsstr.Trim(dir)
	if strings.Contains(d, "<") {
		d = "RL"
	}
	if strings.Contains(d, "^") {
		d = "BT"
	}
	if strings.Contains(d, ">") {
		d = "LR"
	}
	if strings.Contains(d, "v") {
		d = "TB"
	}
	if d == "TD" {
		d = "TB"
	}
	db.Direction = d
}

// setClass gives the nodes, edges and subgraphs ids, a comma-separated list, a class.
func (db *FlowDb) setClass(ids, className string) {
	for _, id := range strings.Split(ids, ",") {
		if v, ok := db.Nodes.Get(id); ok {
			v.Classes = append(v.Classes, className)
		}
		if e := db.edgeByID[id]; e != nil {
			e.Classes = append(e.Classes, className)
		}
		if sg := db.subgraphLookup[id]; sg != nil {
			sg.Classes = append(sg.Classes, className)
		}
	}
}

// setTooltip gives the nodes ids a tooltip.
func (db *FlowDb) setTooltip(ids, tooltip string) {
	for _, id := range strings.Split(ids, ",") {
		db.Tooltips[id] = tooltip
	}
}

// setLink links the nodes ids to link. target is "" when not given.
func (db *FlowDb) setLink(ids, link, target string) {
	url := jsstr.Trim(link)
	for _, id := range strings.Split(ids, ",") {
		if vertex, ok := db.Nodes.Get(id); ok {
			vertex.Link = ""
			if url != "" {
				vertex.Link = util.SafeURL(url)
			}
			vertex.LinkTarget = target
		}
	}
	db.setClass(ids, "clickable")
}

// setClickEvent marks the nodes clickable. Callbacks are never executed, as in Mermaid's
// strict mode.
func (db *FlowDb) setClickEvent(ids string) {
	db.setClass(ids, "clickable")
}

// setAccTitle sets the accessible title.
func (db *FlowDb) setAccTitle(text string) {
	db.AccTitle = jsstr.TrimStart(text)
}

// setAccDescription sets the accessible description.
func (db *FlowDb) setAccDescription(text string) {
	db.AccDescr = common.CollapseIndentedBreaks(text)
}

func hasSpace(s string) bool {
	for i := 0; i < len(s); {
		r, n := jsstr.Decode(s, i)
		if jsstr.IsSpace(r) {
			return true
		}
		i += n
	}
	return false
}

// addSubGraph adds a subgraph and returns its id. idText and titleText are nil when not
// given; sameText is upstream's idText === titleText.
func (db *FlowDb) addSubGraph(idText *FlowText, list []docItem, titleText *FlowText, sameText bool) string {
	var id *string
	if idText != nil {
		trimmed := jsstr.Trim(idText.Text)
		id = &trimmed
	}
	title := ""
	if titleText != nil {
		title = titleText.Text
	}
	if idText != nil && sameText && hasSpace(title) {
		id = nil
	}

	seen := map[string]bool{}
	var nodeList []string
	dir := ""
	for _, entry := range list {
		if entry.dir != "" {
			dir = entry.dir
			continue
		}
		for _, item := range entry.ids {
			if jsstr.Trim(item) != "" && !seen[item] {
				seen[item] = true
				nodeList = append(nodeList, item)
			}
		}
	}
	if dir == "" && db.options.InheritDir {
		dir = db.Direction
	}

	if id == nil {
		generated := "subGraph" + strconv.Itoa(db.subCount)
		id = &generated
	}
	db.subCount++

	var lt any
	if titleText != nil {
		lt = string(titleText.Type)
	}
	subgraph := &FlowSubgraph{
		ID:        *id,
		Title:     jsstr.Trim(title),
		Classes:   []string{},
		Dir:       dir,
		LabelType: labelType(lt),
	}
	// A node belongs to the first subgraph that lists it, and a subgraph never contains itself.
	subgraph.Nodes = []string{}
	for _, nodeID := range nodeList {
		if !db.members[nodeID] && nodeID != *id {
			subgraph.Nodes = append(subgraph.Nodes, nodeID)
		}
	}
	for _, nodeID := range subgraph.Nodes {
		db.members[nodeID] = true
	}

	if existing := db.subgraphLookup[*id]; existing != nil {
		existing.Nodes = append(existing.Nodes, subgraph.Nodes...)
	} else {
		db.Subgraphs = append(db.Subgraphs, subgraph)
		db.subgraphLookup[*id] = subgraph
	}
	return *id
}

func (db *FlowDb) destructLink(end, start string) LinkInfo {
	info := destructEndLink(end)
	if start == "" {
		return LinkInfo{Type: info.typ, Stroke: info.stroke, Length: info.length}
	}
	startInfo := destructStartLink(start)
	if startInfo.stroke != info.stroke {
		return LinkInfo{Type: "INVALID", Stroke: "INVALID"}
	}
	if startInfo.typ == "arrow_open" {
		startInfo.typ = info.typ
	} else {
		if startInfo.typ != info.typ {
			return LinkInfo{Type: "INVALID", Stroke: "INVALID"}
		}
		startInfo.typ = "double_" + startInfo.typ
	}
	if startInfo.typ == "double_arrow" {
		startInfo.typ = "double_arrow_point"
	}
	return LinkInfo{Type: startInfo.typ, Stroke: startInfo.stroke, Length: info.length}
}

// Subgraph returns the subgraph id, or nil.
func (db *FlowDb) Subgraph(id string) *FlowSubgraph {
	return db.subgraphLookup[id]
}
