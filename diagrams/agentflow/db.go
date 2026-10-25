package agentflow

import (
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

// agentflowDbOptions configure an agentflowDb. Upstream's log callback is never set by the
// diagram type, so it is left out.
type agentflowDbOptions struct {
	maxEdges   float64
	inheritDir bool
	// direction is "" when unset.
	direction string
	// lineOffset is the lines of front matter before the text the parser sees, added to
	// every reported line.
	lineOffset int
	// source is the text the parser sees, where errors are located with origin; nil when
	// unset.
	source *string
	origin *perr.Origin
}

// presentationKeys are the metadata keys left out of the semantic model.
var presentationKeys = setOf("shape", "view", "icon", "img", "form", "pos", "w", "h", "class", "style", "labelType")

var toolShapes = setOf("subroutine", "subprocess", "subproc", "framed-rectangle", "tool")

func isToolDefinition(vertex *agentNode) bool {
	return toolShapes[vertex.typ]
}

func labelType(typ any) labelKind {
	switch typ {
	case "markdown", "string", "text":
		return labelKind(typ.(string))
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

// stripPrototypeKeys drops `__proto__`, `constructor` and `prototype` keys at every level, so
// that a consumer merging metadata into its own objects cannot be made to pollute a
// prototype. The YAML reader nests at most 64 levels, so this may recurse.
func stripPrototypeKeys(value any) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, entry := range v {
			out[i] = stripPrototypeKeys(entry)
		}
		return out
	case *jsobj.Object:
		out := jsobj.New()
		for _, key := range v.Keys() {
			if key == "__proto__" || key == "constructor" || key == "prototype" {
				continue
			}
			entry, _ := v.Get(key)
			out.Set(key, stripPrototypeKeys(entry))
		}
		return out
	}
	return value
}

// merge adds what a metadata block holds to an element's metadata. Mermaid spreads the
// parsed YAML whatever it is, so a list adds its indexes as keys and a string its characters.
func merge(into *jsobj.Object, doc any) *jsobj.Object {
	if into == nil {
		into = jsobj.New()
	}
	switch d := doc.(type) {
	case *jsobj.Object:
		for _, key := range d.Keys() {
			v, _ := d.Get(key)
			into.Set(key, v)
		}
	case []any:
		for i, v := range d {
			into.Set(strconv.Itoa(i), v)
		}
	case string:
		for i, unit := range jsstr.UTF16(d) {
			into.Set(strconv.Itoa(i), jsstr.FromUTF16([]uint16{unit}))
		}
	}
	return into
}

// isBlockScalarLine matches /:\s*[>|][\d+-]*\s*$/: every part is a run of characters the
// next cannot hold, so reading back from the end finds the one way it can match.
func isBlockScalarLine(code string) bool {
	end := len(jsstr.TrimEnd(code))
	for end > 0 && (code[end-1] >= '0' && code[end-1] <= '9' || code[end-1] == '+' || code[end-1] == '-') {
		end--
	}
	if end == 0 || (code[end-1] != '>' && code[end-1] != '|') {
		return false
	}
	before := jsstr.TrimEnd(code[:end-1])
	return strings.HasSuffix(before, ":")
}

// dropTrailingComma is code.replace(/,([\t ]*)$/, '$1').
func dropTrailingComma(code string) string {
	end := len(code)
	for end > 0 && (code[end-1] == ' ' || code[end-1] == '\t') {
		end--
	}
	if end > 0 && code[end-1] == ',' {
		return code[:end-1] + code[end:]
	}
	return code
}

// indentOf is line.length - line.trimStart().length.
func indentOf(line string) int {
	return jsstr.Len(line) - jsstr.Len(jsstr.TrimStart(line))
}

// stripLineTrailingCommas removes the comma that ends a line of a multi-line metadata body,
// which block YAML does not allow. Commas inside quoted scalars, flow collections and block
// scalars are content and stay. The scan reads bytes where upstream reads UTF-16 units: it
// only looks for ASCII, which neither encoding uses inside another character.
func stripLineTrailingCommas(body string) string {
	inSingle, inDouble := false, false
	flowDepth := 0
	blockScalarIndent := -1
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if blockScalarIndent >= 0 {
			if jsstr.Trim(line) == "" || indentOf(line) > blockScalarIndent {
				out = append(out, line)
				continue
			}
			blockScalarIndent = -1
		}
		commentStart := -1
	scan:
		for i := 0; i < len(line); i++ {
			ch := line[i]
			switch {
			case inDouble:
				if ch == '\\' {
					i++
				} else if ch == '"' {
					inDouble = false
				}
			case inSingle:
				if ch == '\'' {
					inSingle = false
				}
			case ch == '"':
				inDouble = true
			case ch == '\'':
				inSingle = true
			case ch == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
				commentStart = i
				break scan
			case ch == '[' || ch == '{':
				flowDepth++
			case ch == ']' || ch == '}':
				flowDepth = max(0, flowDepth-1)
			}
		}
		if inSingle || inDouble || flowDepth > 0 {
			out = append(out, line)
			continue
		}
		code, comment := line, ""
		if commentStart >= 0 {
			code, comment = line[:commentStart], line[commentStart:]
		}
		if isBlockScalarLine(code) {
			blockScalarIndent = indentOf(line)
			out = append(out, line)
			continue
		}
		out = append(out, dropTrailingComma(code)+comment)
	}
	return strings.Join(out, "\n")
}

func edgeSemantic(typ, stroke string) string {
	switch {
	case typ == "arrow_point" && stroke == "normal":
		return "sequence"
	case typ == "arrow_cross" && stroke == "normal":
		return "failure"
	case typ == "arrow_open" && stroke == "dotted":
		return "reference"
	}
	return ""
}

// destructEndLink reads a link token, which is ASCII once trimmed.
func destructEndLink(raw string) linkInfo {
	str := jsstr.Trim(raw)
	line := str[:max(0, len(str)-1)]
	typ := "arrow_open"
	if str != "" {
		switch str[len(str)-1] {
		case 'x':
			typ = "arrow_cross"
		case '>':
			typ = "arrow_point"
		case '-', '.':
			line = str
		}
	}
	stroke := "normal"
	length := float64(len(line) - 1)
	if dots := strings.Count(line, "."); dots > 0 {
		stroke = "dotted"
		length = float64(dots)
	}
	return linkInfo{typ: typ, stroke: stroke, length: length}
}

// semanticMetadata is metadata without its presentation-only keys, or nil when none is left.
func semanticMetadata(metadata *jsobj.Object) *jsobj.Object {
	if metadata == nil {
		return nil
	}
	out := jsobj.New()
	for _, key := range metadata.Keys() {
		if presentationKeys[key] {
			continue
		}
		v, _ := metadata.Get(key)
		out.Set(key, v)
	}
	if out.Len() == 0 {
		return nil
	}
	return out
}

// stringify is JSON.stringify for a YAML value.
func stringify(v any) string {
	switch v := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if v != v || v > 1.7976931348623157e308 || v < -1.7976931348623157e308 {
			return "null"
		}
		return jsnum.Format(v)
	case string:
		return quoteJSON(v)
	case []any:
		parts := make([]string, len(v))
		for i, e := range v {
			parts[i] = stringify(e)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case *jsobj.Object:
		var parts []string
		for _, k := range v.Keys() {
			e, _ := v.Get(k)
			parts = append(parts, quoteJSON(k)+":"+stringify(e))
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	return "null"
}

// quoteJSON quotes s as JSON.stringify does, writing lone surrogates as escapes.
func quoteJSON(s string) string {
	const hex = "0123456789abcdef"
	units := jsstr.UTF16(s)
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u == '"':
			b.WriteString(`\"`)
		case u == '\\':
			b.WriteString(`\\`)
		case u == '\b':
			b.WriteString(`\b`)
		case u == '\f':
			b.WriteString(`\f`)
		case u == '\n':
			b.WriteString(`\n`)
		case u == '\r':
			b.WriteString(`\r`)
		case u == '\t':
			b.WriteString(`\t`)
		case u < 0x20:
			b.WriteString(`\u00` + string(hex[u>>4]) + string(hex[u&15]))
		case u >= 0xd800 && u <= 0xdbff && i+1 < len(units) && units[i+1] >= 0xdc00 && units[i+1] <= 0xdfff:
			b.WriteString(jsstr.FromUTF16(units[i : i+2]))
			i++
		case u >= 0xd800 && u <= 0xdfff:
			b.WriteString(`\u` + string(hex[u>>12]) + string(hex[u>>8&15]) + string(hex[u>>4&15]) + string(hex[u&15]))
		default:
			b.WriteString(jsstr.FromUTF16(units[i : i+1]))
		}
	}
	b.WriteByte('"')
	return b.String()
}

// agentflowDb is the agentflow model the parser builds.
type agentflowDb struct {
	// direction is "" when unset.
	direction          string
	nodes              omap.Map[string, *agentNode]
	edges              []*agentEdge
	subgraphs          []*agentSubgraph
	connectors         omap.Map[string, *agentNode]
	classes            omap.Map[string, *agentClassDef]
	tooltips           map[string]string
	defaultEdgeStyle   []string
	defaultInterpolate string
	// mappings is where each statement sits in the source, in the order the statements
	// were read.
	mappings    []*elementMapping
	diagnostics []*diagnostic
	// title, accTitle and accDescr are "" when unset; accTitle and accDescr are nil when unset.
	title    string
	accTitle *string
	accDescr *string
	// graph is the graph built at parse time, which render draws.
	graph *agentGraph

	lineOffset     int
	subgraphLookup map[string]*agentSubgraph
	globalNodes    map[string]bool
	members        map[string]bool
	edgeByID       map[string]*agentEdge
	pairCount      map[string]int
	subCount       int
	bareMappings   map[*elementMapping]bool
	firstMapping   map[string]*elementMapping
	reported       map[string][]*diagnostic
	options        agentflowDbOptions
}

func newAgentflowDb(options agentflowDbOptions) *agentflowDb {
	return &agentflowDb{
		tooltips:       map[string]string{},
		lineOffset:     options.lineOffset,
		subgraphLookup: map[string]*agentSubgraph{},
		globalNodes:    map[string]bool{},
		members:        map[string]bool{},
		edgeByID:       map[string]*agentEdge{},
		pairCount:      map[string]int{},
		bareMappings:   map[*elementMapping]bool{},
		firstMapping:   map[string]*elementMapping{},
		reported:       map[string][]*diagnostic{},
		options:        options,
	}
}

func (db *agentflowDb) errorAt(l *loc) (line, column int) {
	if l == nil {
		return 0, 0
	}
	if db.options.source == nil {
		return l.firstLine + db.lineOffset, l.firstColumn + 1
	}
	source := jsstr.UTF16(*db.options.source)
	start := 0
	for n := 1; n < l.firstLine; n++ {
		start = indexOf(source, '\n', start) + 1
	}
	at := perr.Locate(db.options.origin, source, start+l.firstColumn)
	return at.Line, at.Column
}

func indexOf(text []uint16, c uint16, from int) int {
	for i := from; i < len(text); i++ {
		if text[i] == c {
			return i
		}
	}
	return -1
}

func (db *agentflowDb) located(message string, code perr.Code, l *loc) {
	e := perr.New(message, code)
	e.Type = "agentflow"
	e.Line, e.Column = db.errorAt(l)
	perr.Throw(e)
}

func (db *agentflowDb) metadata(id, source string, l *loc) any {
	multiline := strings.Contains(source, "\n")
	text := "{\n" + source + "\n}"
	if multiline {
		text = source + "\n"
	}
	parsed, err := yaml.Parse(text)
	if err != nil {
		// Block YAML rejects the comma-separated style of the single-line form, so try once
		// more without the commas.
		stripped := source
		if multiline {
			stripped = stripLineTrailingCommas(source)
		}
		retried := false
		if stripped != source {
			if v, retryErr := yaml.Parse(stripped + "\n"); retryErr == nil {
				parsed, retried = v, true
			}
		}
		if !retried {
			at := ""
			if l != nil {
				line, column := db.errorAt(l)
				at = " (" + strconv.Itoa(line) + ":" + strconv.Itoa(column) + ")"
			}
			db.located(`Metadata of "`+id+`" is not valid YAML. `+err.Error()+at, perr.Syntax, l)
		}
	}
	if parsed == nil {
		parsed = jsobj.New()
	}
	return stripPrototypeKeys(parsed)
}

// addVertex adds or updates a vertex. text, props and metadata are nil, and typ "", when not
// given. Upstream's classes and dir parameters are never passed, so they are left out.
func (db *agentflowDb) addVertex(id string, text *flowText, typ string, style []string, props *jsobj.Object, metadata *string, metadataLoc *loc) {
	if id == "" || jsstr.Trim(id) == "" {
		return
	}

	var doc any
	var fields *jsobj.Object
	authored, hasAuthored := "", false
	if metadata != nil {
		doc = db.metadata(id, *metadata, metadataLoc)
		if obj, ok := doc.(*jsobj.Object); ok {
			fields = obj
			if s, ok := getString(obj, "shape"); ok {
				authored, hasAuthored = s, true
				obj.Set("shape", resolveShapeAlias(s))
			}
		}
	}
	hasDoc := jsobj.Truthy(doc)

	if subgraph := db.subgraphLookup[id]; subgraph != nil && hasDoc {
		subgraph.metadata = merge(subgraph.metadata, doc)
		return
	}
	if connector, ok := db.connectors.Get(id); ok && hasDoc {
		connector.metadata = merge(connector.metadata, doc)
		return
	}
	// Mermaid reserves this id and creates no node for it.
	if id == "connectors" {
		return
	}
	if edge := db.edgeByID[id]; edge != nil {
		if v, ok := fields.Get("animate"); ok {
			edge.animate = v
		}
		if v, ok := fields.Get("animation"); ok {
			edge.animation = v
		}
		if v, ok := fields.Get("curve"); ok {
			edge.interpolate, edge.hasInterpolate = v, true
		}
		if hasDoc {
			edge.metadata = merge(edge.metadata, doc)
		}
		return
	}

	vertex, ok := db.nodes.Get(id)
	if !ok {
		vertex = &agentNode{id: id, labelType: "text", styles: []string{}, classes: []string{}}
		db.nodes.Set(id, vertex)
	}
	if text != nil {
		vertex.labelType = text.typ
		vertex.text = unquote(jsstr.Trim(text.text))
	} else if !ok {
		vertex.text = id
	}
	if typ != "" {
		vertex.typ = typ
	}
	vertex.styles = append(vertex.styles, style...)
	if props == nil {
		props = jsobj.New()
	}
	if vertex.props == nil {
		vertex.props = props
	} else {
		for _, k := range props.Keys() {
			v, _ := props.Get(k)
			vertex.props.Set(k, v)
		}
	}

	if !hasDoc {
		return
	}
	vertex.metadata = merge(vertex.metadata, doc)
	if fields == nil {
		return
	}

	if shapeValue, _ := fields.Get("shape"); jsobj.Truthy(shapeValue) {
		if !hasAuthored {
			db.located("No such shape: "+stringify(shapeValue)+".", perr.Semantic, metadataLoc)
		}
		if jsstr.ChangesWhenLowered(authored) || strings.Contains(authored, "_") {
			db.located("No such shape: "+authored+". Shape names should be lowercase.", perr.Semantic, metadataLoc)
		}
		shape := shapeValue.(string)
		if !isValidShape(shape) {
			db.located("No such shape: "+shape+".", perr.Semantic, metadataLoc)
		}
		vertex.typ = shape
	}
	if label, _ := fields.Get("label"); jsobj.Truthy(label) {
		vertex.text = jsobj.ToString(label)
		lt, _ := fields.Get("labelType")
		vertex.labelType = labelType(lt)
	}
}

func getString(obj *jsobj.Object, key string) (string, bool) {
	v, _ := obj.Get(key)
	s, ok := v.(string)
	return s, ok
}

func (db *agentflowDb) addSingleLink(start, end string, info *linkInfo, id string) {
	edge := &agentEdge{start: start, end: end, labelType: "text", classes: []string{}}
	if db.defaultInterpolate != "" {
		edge.interpolate, edge.hasInterpolate = db.defaultInterpolate, true
	}
	if info.text != nil {
		edge.text = unquote(jsstr.Trim(info.text.text))
		edge.labelType = labelType(string(info.text.typ))
	}
	edge.typ = info.typ
	edge.stroke = info.stroke
	edge.length = min(info.length, 10)
	edge.edgeSemantic = info.edgeSemantic
	pair := start + "\x00" + end
	existing := db.pairCount[pair]
	if _, taken := db.edgeByID[id]; id != "" && !taken {
		edge.id = id
		edge.isUserDefinedID = true
	} else {
		n := 0
		if existing != 0 {
			n = existing + 1
		}
		edge.id = "L_" + start + "_" + end + "_" + strconv.Itoa(n)
	}

	if limit := db.options.maxEdges; float64(len(db.edges)) >= limit {
		e := perr.New("Edge limit exceeded. The limit is "+jsnum.Format(limit)+".", perr.Limit)
		e.Type = "agentflow"
		perr.Throw(e)
	}
	db.edges = append(db.edges, edge)
	db.pairCount[pair] = existing + 1
	if _, taken := db.edgeByID[edge.id]; !taken {
		db.edgeByID[edge.id] = edge
	}
}

// addLink adds an edge from each of starts to each of ends.
func (db *agentflowDb) addLink(starts, ends []string, info *linkInfo) {
	id := strings.Replace(info.id, "@", "", 1)
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

func (db *agentflowDb) updateLinkInterpolate(positions []string, interpolate string) {
	for _, pos := range positions {
		if pos == "default" {
			db.defaultInterpolate = interpolate
		} else {
			edge := db.edgeAt(pos)
			edge.interpolate, edge.hasInterpolate = interpolate, true
		}
	}
}

func (db *agentflowDb) updateLink(positions []string, style []string) {
	// Upstream gives every edge the one array, and adds fill:none to it at the first edge.
	var styled []*agentEdge
	for _, pos := range positions {
		if pos == "default" {
			db.defaultEdgeStyle = style
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
			e.style = style
		}
	}
}

func (db *agentflowDb) edgeAt(pos string) *agentEdge {
	i := jsnum.Parse(pos)
	if i >= 0 && i < float64(len(db.edges)) && i == float64(int(i)) {
		return db.edges[int(i)]
	}
	e := perr.New("The index "+pos+" for linkStyle is out of bounds. Valid indices for linkStyle are between 0 and "+
		strconv.Itoa(len(db.edges)-1)+". (Help: Ensure that the index is within the range of existing edges.)", perr.Semantic)
	e.Type = "agentflow"
	perr.Throw(e)
	return nil
}

func (db *agentflowDb) addClass(ids string, styles []string) {
	joined := strings.Join(styles, ",")
	joined = strings.ReplaceAll(joined, `\,`, "§§§")
	joined = strings.ReplaceAll(joined, ",", ";")
	joined = strings.ReplaceAll(joined, "§§§", ",")
	style := strings.Split(joined, ";")
	for _, id := range strings.Split(ids, ",") {
		classNode, ok := db.classes.Get(id)
		if !ok {
			classNode = &agentClassDef{id: id, styles: []string{}, textStyles: []string{}}
			db.classes.Set(id, classNode)
		}
		for _, s := range style {
			if strings.Contains(s, "color") {
				classNode.textStyles = append(classNode.textStyles, strings.Replace(s, "fill", "bgFill", 1))
			}
			classNode.styles = append(classNode.styles, s)
		}
	}
}

func (db *agentflowDb) setDirection(dir string) {
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
	db.direction = d
}

func (db *agentflowDb) setClass(ids, className string) {
	for _, id := range strings.Split(ids, ",") {
		if v, ok := db.nodes.Get(id); ok {
			v.classes = append(v.classes, className)
		}
		if e := db.edgeByID[id]; e != nil {
			e.classes = append(e.classes, className)
		}
		if sg := db.subgraphLookup[id]; sg != nil {
			sg.classes = append(sg.classes, className)
		}
	}
}

func (db *agentflowDb) setTooltip(ids, tooltip string) {
	for _, id := range strings.Split(ids, ",") {
		db.tooltips[id] = tooltip
	}
}

// setLink links the nodes ids to link. target is nil when not given.
func (db *agentflowDb) setLink(ids, link string, target *string) {
	url := jsstr.Trim(link)
	for _, id := range strings.Split(ids, ",") {
		if vertex, ok := db.nodes.Get(id); ok {
			vertex.link = nil
			if url != "" {
				safe := util.SafeURL(url)
				vertex.link = &safe
			}
			vertex.linkTarget = target
		}
	}
	db.setClass(ids, "clickable")
}

// setClickEvent marks the nodes clickable. Callbacks are never executed, as in Mermaid's
// strict mode.
func (db *agentflowDb) setClickEvent(ids string) {
	db.setClass(ids, "clickable")
}

func (db *agentflowDb) setAccTitle(text string) {
	v := jsstr.TrimStart(text)
	db.accTitle = &v
}

func (db *agentflowDb) setAccDescription(text string) {
	v := common.CollapseIndentedBreaks(text)
	db.accDescr = &v
}

// addSubGraph adds a flow and returns its id. idText and titleText are nil when not given;
// the parser never passes the one object as both, as Mermaid's flowchart can.
func (db *agentflowDb) addSubGraph(idText *flowText, list []docItem, titleText *flowText) string {
	var id *string
	if idText != nil {
		trimmed := jsstr.Trim(idText.text)
		id = &trimmed
	}
	title := ""
	if titleText != nil {
		title = titleText.text
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
	if dir == "" && db.options.inheritDir {
		dir = db.direction
		if dir == "" {
			dir = db.options.direction
		}
	}

	if id == nil {
		generated := "subGraph" + strconv.Itoa(db.subCount)
		id = &generated
	}
	db.subCount++

	// A flow never contains itself, a globally scoped node, or a node another flow already holds.
	nodes := []string{}
	for _, nodeID := range nodeList {
		if nodeID != *id && !db.globalNodes[nodeID] && !db.members[nodeID] {
			nodes = append(nodes, nodeID)
		}
	}
	for _, nodeID := range nodes {
		db.members[nodeID] = true
	}

	if existing := db.subgraphLookup[*id]; existing != nil {
		// A repeated id adds its members to the first flow; its title and direction win.
		existing.nodes = append(existing.nodes, nodes...)
		if t := jsstr.Trim(title); t != "" {
			existing.title = t
		}
		if dir != "" {
			existing.dir = dir
		}
		return *id
	}
	var lt any
	if titleText != nil {
		lt = string(titleText.typ)
	}
	subgraph := &agentSubgraph{
		id:        *id,
		nodes:     nodes,
		title:     jsstr.Trim(title),
		classes:   []string{},
		dir:       dir,
		labelType: labelType(lt),
	}
	db.subgraphs = append(db.subgraphs, subgraph)
	db.subgraphLookup[*id] = subgraph
	return *id
}

// addGlobal keeps every node named inside a `global` block out of the flows that mention it.
func (db *agentflowDb) addGlobal(list []docItem) {
	release := false
	for _, entry := range list {
		for _, item := range entry.ids {
			id := jsstr.Trim(item)
			if id == "" {
				continue
			}
			db.globalNodes[id] = true
			if db.members[id] {
				delete(db.members, id)
				release = true
			}
		}
	}
	if !release {
		return
	}
	for _, subgraph := range db.subgraphs {
		kept := []string{}
		for _, nodeID := range subgraph.nodes {
			if !db.globalNodes[nodeID] {
				kept = append(kept, nodeID)
			}
		}
		subgraph.nodes = kept
	}
}

func (db *agentflowDb) addConnector(textObj, titleObj *flowText) string {
	if textObj == nil {
		return ""
	}
	id := jsstr.Trim(textObj.text)
	if id == "" {
		return ""
	}
	title := ""
	if titleObj != nil {
		title = jsstr.Trim(titleObj.text)
	}
	connector, ok := db.connectors.Get(id)
	if !ok {
		created := &agentNode{id: id, labelType: "text", styles: []string{}, classes: []string{}, isConnector: true, text: id}
		if title != "" {
			created.text = title
		}
		db.connectors.Set(id, created)
		db.nodes.Set(id, created)
	} else if title != "" {
		connector.text = title
	}
	return id
}

// destructLink reads a link, end and the start of a split one, or "" for start.
func (db *agentflowDb) destructLink(end, start string) linkInfo {
	info := destructEndLink(end)
	if start != "" {
		// The first half of a split arrow only says whether the line is dotted.
		stroke := "normal"
		if strings.Contains(start, ".") {
			stroke = "dotted"
		}
		if stroke != info.stroke {
			return linkInfo{typ: "INVALID", stroke: "INVALID"}
		}
	}
	info.edgeSemantic = edgeSemantic(info.typ, info.stroke)
	return info
}

func (db *agentflowDb) subgraph(id string) *agentSubgraph {
	return db.subgraphLookup[id]
}

func (db *agentflowDb) vertexKind(vertex *agentNode) string {
	if isToolDefinition(vertex) {
		return "tool"
	}
	switch resolveShapeAlias(vertex.typ) {
	case "hexagon", "hex":
		return "action"
	case "lean-right", "lean_right":
		return "input"
	case "lin-doc", "lined-document":
		return "refdoc"
	case "diamond":
		return "decision"
	}
	return "task"
}

func (db *agentflowDb) position(l *loc) *elementPosition {
	var startLine, startColumn int
	endLine, endColumn := 0, 0
	if l != nil {
		startLine, startColumn = l.firstLine, l.firstColumn
		endLine, endColumn = l.lastLine, l.lastColumn
	} else {
		endLine, endColumn = startLine, startColumn
	}
	// Mermaid builds its parser without ranges, so the indexes are always zero.
	return &elementPosition{
		startLine:   startLine + db.lineOffset,
		startColumn: startColumn,
		endLine:     endLine + db.lineOffset,
		endColumn:   endColumn,
	}
}

func (db *agentflowDb) pushMapping(id, typ string, position *elementPosition) *elementMapping {
	mapping := &elementMapping{id: id, typ: typ, position: position}
	db.mappings = append(db.mappings, mapping)
	if _, ok := db.firstMapping[id]; !ok {
		db.firstMapping[id] = mapping
	}
	return mapping
}

// addVertexMapping records a vertex. A bare reference, with no shape, becomes an attachment
// if a metadata block follows it.
func (db *agentflowDb) addVertexMapping(id, shape string, l *loc) {
	if id == "" {
		return
	}
	mapping := db.pushMapping(id, "vertex", db.position(l))
	if shape == "" {
		db.bareMappings[mapping] = true
	}
}

// extendVertexMapping widens a node's span over the metadata block that follows it.
func (db *agentflowDb) extendVertexMapping(id string, l *loc) {
	if id == "" || l == nil {
		return
	}
	end := db.position(l)
	for i := len(db.mappings) - 1; i >= 0; i-- {
		m := db.mappings[i]
		if m.typ != "vertex" || m.id != id {
			continue
		}
		if db.bareMappings[m] {
			delete(db.bareMappings, m)
			m.typ = "attachment"
		}
		p := m.position
		if end.endLine > p.endLine || (end.endLine == p.endLine && end.endColumn > p.endColumn) {
			p.endLine = end.endLine
			p.endColumn = end.endColumn
		}
		return
	}
	db.pushMapping(id, "attachment", end)
}

func (db *agentflowDb) addEdgeMapping(toNodes []string, l *loc) {
	var ids []string
	for _, n := range toNodes {
		if n != "" {
			ids = append(ids, n)
		}
	}
	id := "edge"
	if len(ids) > 0 {
		id = strings.Join(ids, ">")
	}
	db.pushMapping(id, "edge", db.position(l))
}

func (db *agentflowDb) blockMapping(id, typ string, startLoc, endLoc *loc) {
	if id == "" {
		return
	}
	start := db.position(startLoc)
	end := start
	if endLoc != nil {
		end = db.position(endLoc)
	}
	db.pushMapping(id, typ, &elementPosition{
		startLine:   start.startLine,
		startColumn: start.startColumn,
		endLine:     end.endLine,
		endColumn:   end.endColumn,
	})
}

// addSubgraphMapping records a flow by its id as written, untrimmed.
func (db *agentflowDb) addSubgraphMapping(id *flowText, startLoc, endLoc *loc) {
	text := ""
	if id != nil {
		text = id.text
	}
	db.blockMapping(text, "subgraph", startLoc, endLoc)
}

func (db *agentflowDb) addConnectorMapping(textObj *flowText, startLoc, endLoc *loc) {
	text := ""
	if textObj != nil {
		text = jsstr.Trim(textObj.text)
	}
	db.blockMapping(text, "connector", startLoc, endLoc)
}

// emit records a diagnostic about nodeID, "" for none. Upstream also takes an edge id, which
// nothing passes.
func (db *agentflowDb) emit(id, severity, message, nodeID string) {
	d := &diagnostic{id: id, severity: severity, message: message, nodeID: nodeID}
	if nodeID != "" {
		if mapping := db.firstMapping[nodeID]; mapping != nil {
			d.position = mapping.position
		}
	}
	// The graph may be built more than once; each finding is recorded once.
	for _, same := range db.reported[nodeID] {
		if same.id == id && same.severity == severity && same.message == message && same.nodeID == d.nodeID && same.edgeID == d.edgeID {
			return
		}
	}
	db.reported[nodeID] = append(db.reported[nodeID], d)
	db.diagnostics = append(db.diagnostics, d)
}

func (db *agentflowDb) emitWarning(id, message, nodeID string) {
	db.emit(id, "warning", message, nodeID)
}

func (db *agentflowDb) emitError(id, message, nodeID string) {
	db.emit(id, "error", message, nodeID)
}

func (p *elementPosition) object() *jsobj.Object {
	o := jsobj.New()
	o.Set("startLine", float64(p.startLine))
	o.Set("startColumn", float64(p.startColumn))
	o.Set("endLine", float64(p.endLine))
	o.Set("endColumn", float64(p.endColumn))
	o.Set("startIndex", float64(p.startIndex))
	o.Set("endIndex", float64(p.endIndex))
	return o
}

func (d *diagnostic) object() *jsobj.Object {
	o := jsobj.New()
	o.Set("id", d.id)
	o.Set("severity", d.severity)
	o.Set("message", d.message)
	if d.nodeID != "" {
		o.Set("nodeId", d.nodeID)
	} else if d.edgeID != "" {
		o.Set("edgeId", d.edgeID)
	}
	if d.position != nil {
		o.Set("position", d.position.object())
	}
	return o
}

func anyList(list []string) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}

// semanticModel is the diagram with presentation-only controls left out, for tools that read
// what it means, as JSON values.
func (db *agentflowDb) semanticModel() *jsobj.Object {
	vertices := []any{}
	connectors := []any{}
	for id, v := range db.nodes.All() {
		if db.subgraphLookup[id] != nil {
			continue
		}
		metadata := semanticMetadata(v.metadata)
		if v.isConnector {
			connector := jsobj.New()
			connector.Set("id", id)
			if v.text != id {
				connector.Set("title", v.text)
			}
			if metadata != nil {
				connector.Set("metadata", metadata)
			}
			connectors = append(connectors, connector)
			continue
		}
		vertex := jsobj.New()
		vertex.Set("id", id)
		vertex.Set("label", v.text)
		if shape := resolveShapeAlias(v.typ); shape != "" {
			vertex.Set("shape", shape)
		}
		vertex.Set("vertexKind", db.vertexKind(v))
		if metadata != nil {
			vertex.Set("metadata", metadata)
		}
		vertices = append(vertices, vertex)
	}

	edges := []any{}
	for _, e := range db.edges {
		edge := jsobj.New()
		edge.Set("start", e.start)
		edge.Set("end", e.end)
		if e.id != "" {
			edge.Set("id", e.id)
		}
		if e.text != "" {
			edge.Set("label", e.text)
		}
		if e.typ != "" {
			edge.Set("type", e.typ)
		}
		if e.stroke != "" {
			edge.Set("stroke", e.stroke)
		}
		if e.edgeSemantic != "" {
			edge.Set("edgeSemantic", e.edgeSemantic)
		}
		if e.length != 0 {
			edge.Set("length", e.length)
		}
		if e.metadata.Len() > 0 {
			edge.Set("metadata", e.metadata.Clone())
		}
		edges = append(edges, edge)
	}

	subGraphs := []any{}
	for _, sg := range db.subgraphs {
		out := jsobj.New()
		out.Set("id", sg.id)
		out.Set("nodes", anyList(sg.nodes))
		out.Set("type", "flow")
		out.Set("title", sg.title)
		if sg.dir != "" {
			out.Set("direction", sg.dir)
		}
		if metadata := semanticMetadata(sg.metadata); metadata != nil {
			out.Set("metadata", metadata)
		}
		subGraphs = append(subGraphs, out)
	}

	diagnostics := []any{}
	for _, d := range db.diagnostics {
		diagnostics = append(diagnostics, d.object())
	}
	model := jsobj.New()
	model.Set("vertices", vertices)
	model.Set("edges", edges)
	model.Set("subGraphs", subGraphs)
	model.Set("connectors", connectors)
	model.Set("diagnostics", diagnostics)
	if db.direction != "" {
		model.Set("direction", db.direction)
	}
	return model
}
