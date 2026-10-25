package usecase

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/omap"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

type draftLabel struct {
	text string
	typ  string
	span span
}

// draftProperty: keySpan is also where an error about the property points. value is a string
// or a bool.
type draftProperty struct {
	key       string
	value     any
	span      span
	keySpan   span
	valueSpan span
}

// named is what every mention of an id carries: where it is, and whether the id was made from
// a label.
type named struct {
	id        string
	label     draftLabel
	span      span
	generated bool
}

type draftElement struct {
	named
	kind   string
	parent *draftBoundary
	shape  string
	// metadata is nil where upstream's is undefined; an empty block is an empty, non-nil list.
	metadata   []draftProperty
	stereotype string
	classes    []string
}

type draftBoundary struct {
	named
	classes []string
}

type draftJSON struct {
	id            string
	value         *jsobj.Object
	propertyOrder map[string][]string
	span          span
	classes       []string
}

type draftEndpoint struct {
	named
	// Whether this mention declares the element, and whether it carries a `:::` suffix.
	declares bool
	classed  bool
}

type draftRelationship struct {
	source draftEndpoint
	target draftEndpoint
	span   span
	// id is "" where upstream's is undefined: an explicit id is never empty.
	id        string
	idSpan    span
	typ       string
	arrowType int
	label     *draftLabel
	minlen    int
}

type draftNote struct {
	target string
	span   span
	label  draftLabel
}

type draftMetadata struct {
	target     string
	span       span
	properties []draftProperty
}

type draftTarget struct {
	id   string
	span span
}

type draftClasses struct {
	targets []draftTarget
	classes []string
}

type draftStyle struct {
	target string
	span   span
	styles []string
}

type draftClassDef struct {
	ids    []string
	styles []string
}

// drafts is everything the parser read, in source order, before any of it is resolved.
type drafts struct {
	elements       []*draftElement
	boundaries     []*draftBoundary
	jsons          []*draftJSON
	relationships  []*draftRelationship
	notes          []draftNote
	metadata       []draftMetadata
	classes        []draftClasses
	styles         []draftStyle
	classDefs      []draftClassDef
	direction      string
	accTitle       string
	accDescription string
}

type origin struct {
	kind      string
	span      span
	generated bool
	// The label a generated id was derived from.
	label string
}

type elementState struct {
	named
	kind           string
	parent         *draftBoundary
	shape          string
	stereotype     string
	stereotypeSpan *span
	classes        []string
	// actorType, icon and business are nil where upstream's are undefined, which a conflict
	// tells apart from an empty icon or false.
	actorType *string
	icon      *string
	business  *bool
}

type boundaryState struct {
	draftBoundary
	typ     string
	members []string
}

func pushUnique(target *[]string, values []string) {
	if len(values) == 0 {
		return
	}
	seen := map[string]bool{}
	for _, v := range *target {
		seen[v] = true
	}
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		*target = append(*target, v)
	}
}

// inferKind is which kind of element the keys of a metadata block could only belong to, for
// the error message.
func inferKind(properties []draftProperty) string {
	var possible omap.Set[string]
	for _, k := range []string{"actor", "usecase", "boundary", "edge"} {
		possible.Add(k)
	}
	only := func(kind string) {
		possible = omap.Set[string]{}
		if kind != "" {
			possible.Add(kind)
		}
	}
	for _, p := range properties {
		switch p.key {
		case "icon":
			only("actor")
		case "animate", "animation":
			only("edge")
		case "type":
			switch p.value {
			case "rect", "package":
				only("boundary")
			case "normal", "hollow", "awesome":
				only("actor")
			default:
				only("")
			}
		case "business":
			possible.Delete("boundary")
			possible.Delete("edge")
		default:
			return ""
		}
	}
	if possible.Len() == 1 {
		for k := range possible.All() {
			return k
		}
	}
	return ""
}

func strPtr(s string) *string { return &s }

// resolveUsecase resolves what the parser read into a model, once the whole diagram is read:
// which ids are actors, use cases, boundaries, JSON nodes or edges, in what order they appear,
// and whether the statements about them agree. Ports Mermaid's UsecaseModelBuilder, with its
// messages.
func resolveUsecase(src common.Src, d *drafts, title string, o *perr.Origin) *usecaseModel {
	model := emptyModel()
	actors := &model.actors
	useCases := &model.useCases
	systemBoundaries := &model.systemBoundaries
	symbols := map[string]*origin{}
	var elements omap.Map[string, *elementState]
	var boundaries omap.Map[string, *boundaryState]
	edges := map[string]*relationship{}
	// The offset at which each id is first mentioned, which decides the order of the model.
	first := map[string]int{}

	at := func(s span) string { return locationText(o, src, s[0], s[1]) }

	newError := func(message string, s span) *perr.Error {
		line, column := lineColumn(o, src, s[0])
		return &perr.Error{Code: perr.Semantic, Message: message, Type: "usecase", Line: line, Column: column}
	}

	fail := func(message string, s span) {
		perr.Throw(newError(message+" at "+at(s), s))
	}

	labelSuffix := func(label *string) string {
		if label == nil {
			return ""
		}
		return ` (label "` + *label + `")`
	}
	conflict := func(message string, current, previous span, currentLabel, previousLabel *string) {
		perr.Throw(newError(message+" at "+at(current)+labelSuffix(currentLabel)+"; previous declaration at "+
			at(previous)+labelSuffix(previousLabel), current))
	}

	invalid := func(id, kind string, p draftProperty) {
		fail("Metadata property '"+p.key+"' is invalid for "+kind+" '"+id+"'", p.keySpan)
	}

	mention := func(id string, offset int) {
		if previous, ok := first[id]; !ok || offset < previous {
			first[id] = offset
		}
	}

	firstOf := func(id string) int { return first[id] }

	register := func(n *named, kind string) {
		symbols[n.id] = &origin{kind: kind, span: n.span, generated: n.generated, label: n.label.text}
	}

	originLabel := func(o *origin) *string {
		if o.generated {
			return strPtr(o.label)
		}
		return nil
	}

	// claim: an id means one thing in the whole diagram, and an id made from a label may be
	// declared once.
	claim := func(n *named, kind string) {
		o, ok := symbols[n.id]
		if !ok {
			return
		}
		var label *string
		if n.generated {
			label = strPtr(n.label.text)
		}
		if o.kind != kind {
			conflict("ID '"+n.id+"' is declared as both "+o.kind+" and "+kind, n.span, o.span, label, originLabel(o))
		}
		if o.generated || n.generated {
			conflict("Generated ID '"+n.id+"' collides with another declaration", n.span, o.span, label, originLabel(o))
		}
	}

	unique := func(id, kind string, s span) {
		if previous, ok := symbols[id]; ok {
			conflict("ID '"+id+"' is declared more than once ("+previous.kind+" and "+kind+")", s, previous.span, nil, originLabel(previous))
		}
		symbols[id] = &origin{kind: kind, span: s}
	}

	// actorProperty: a metadata statement replaces what a declaration said; two declarations
	// have to agree.
	actorProperty := func(state *elementState, p draftProperty, replace bool) {
		clash := func(previous any) {
			if !replace && previous != nil && previous != p.value {
				conflict("Actor '"+state.id+"' has conflicting "+p.key+" metadata", p.keySpan, state.span, nil, nil)
			}
		}
		str, isString := p.value.(string)
		b, isBool := p.value.(bool)
		switch {
		case p.key == "type" && (str == "normal" || str == "hollow" || str == "awesome"):
			if state.actorType != nil {
				clash(*state.actorType)
			}
			state.actorType = strPtr(str)
		case p.key == "icon" && isString:
			if state.icon != nil {
				clash(*state.icon)
			}
			state.icon = strPtr(str)
		case p.key == "business" && isBool:
			if state.business != nil {
				clash(*state.business)
			}
			state.business = &b
		default:
			invalid(state.id, "actor", p)
		}
	}

	elementMetadata := func(state *elementState, properties []draftProperty, replace bool) {
		for _, p := range properties {
			if state.kind == "actor" {
				actorProperty(state, p, replace)
			} else if b, ok := p.value.(bool); p.key == "business" && ok {
				if !replace && state.business != nil && *state.business != b {
					conflict("Use case '"+state.id+"' has conflicting business metadata", p.keySpan, state.span, nil, nil)
				}
				state.business = &b
			} else {
				invalid(state.id, "usecase", p)
			}
		}
	}

	publish := func(state *elementState) {
		business := state.business != nil && *state.business
		parentID := ""
		if state.parent != nil {
			parentID = state.parent.id
		}
		if state.kind == "actor" {
			useCases.Delete(state.id)
			a := &actor{
				id:        state.id,
				label:     state.label.text,
				labelType: state.label.typ,
				business:  business,
				classes:   slices.Clone(state.classes),
				styles:    []string{},
			}
			switch {
			case state.icon != nil && *state.icon != "":
				a.typ = "icon"
				a.icon = *state.icon
			case state.actorType != nil:
				a.typ = *state.actorType
			default:
				a.typ = "normal"
			}
			if prev, ok := actors.Get(state.id); ok {
				a.styles = prev.styles
			}
			a.stereotype = state.stereotype
			a.parentID = parentID
			actors.Set(state.id, a)
			return
		}
		actors.Delete(state.id)
		u := &useCase{
			id:         state.id,
			label:      state.label.text,
			labelType:  state.label.typ,
			business:   business,
			classes:    slices.Clone(state.classes),
			shape:      state.shape,
			styles:     []string{},
			stereotype: state.stereotype,
			parentID:   parentID,
		}
		if u.shape == "" {
			u.shape = "ellipse"
		}
		if prev, ok := useCases.Get(state.id); ok {
			u.styles = prev.styles
		}
		useCases.Set(state.id, u)
	}

	declareElement := func(draft *draftElement) {
		claim(&draft.named, draft.kind)
		var parent *draftBoundary
		if draft.parent != nil && draft.parent.id != "" {
			parent = draft.parent
		}
		existing, ok := elements.Get(draft.id)
		if !ok {
			state := &elementState{
				named:      draft.named,
				kind:       draft.kind,
				classes:    slices.Clone(draft.classes),
				parent:     parent,
				shape:      draft.shape,
				stereotype: draft.stereotype,
			}
			if draft.stereotype != "" {
				state.stereotypeSpan = &draft.span
			}
			elementMetadata(state, draft.metadata, false)
			elements.Set(draft.id, state)
			register(&draft.named, draft.kind)
			return
		}
		if existing.label.text != draft.label.text || existing.label.typ != draft.label.typ {
			conflict("ID '"+draft.id+"' has conflicting labels", draft.span, existing.span, nil, nil)
		}
		if draft.shape != "" && existing.shape != "" && draft.shape != existing.shape {
			conflict("Use case '"+draft.id+"' has conflicting shapes", draft.span, existing.span, nil, nil)
		}
		// A boundary whose title makes an empty id is no parent, but it still takes the place
		// of one.
		if parent != nil && existing.parent != nil && existing.parent.id != "" && parent.id != existing.parent.id {
			conflict("Element '"+draft.id+"' belongs to more than one system boundary", parent.span, existing.parent.span, nil, nil)
		}
		if draft.stereotype != "" && existing.stereotype != "" && draft.stereotype != existing.stereotype {
			previous := existing.span
			if existing.stereotypeSpan != nil {
				previous = *existing.stereotypeSpan
			}
			conflict("Element '"+draft.id+"' has conflicting stereotypes", draft.span, previous, nil, nil)
		}
		if existing.shape == "" {
			existing.shape = draft.shape
		}
		if existing.parent == nil {
			existing.parent = draft.parent
		}
		if existing.stereotype == "" {
			existing.stereotype = draft.stereotype
		}
		if existing.stereotypeSpan == nil && draft.stereotype != "" {
			existing.stereotypeSpan = &draft.span
		}
		pushUnique(&existing.classes, draft.classes)
		elementMetadata(existing, draft.metadata, false)
	}

	declareBoundary := func(draft *draftBoundary) {
		claim(&draft.named, "boundary")
		if existing, ok := boundaries.Get(draft.id); ok {
			if existing.label.text != draft.label.text || existing.label.typ != draft.label.typ {
				conflict("Boundary '"+draft.id+"' has conflicting titles", draft.span, existing.span, nil, nil)
			}
			pushUnique(&existing.classes, draft.classes)
			return
		}
		state := &boundaryState{draftBoundary: *draft, members: []string{}}
		state.classes = slices.Clone(draft.classes)
		boundaries.Set(draft.id, state)
		register(&draft.named, "boundary")
	}

	// endpoint: an end of a relationship that nothing declares becomes a use case.
	endpoint := func(end *draftEndpoint) string {
		if end.classed && !end.declares {
			fail("Relationship endpoint '"+end.id+"' uses ::: without declaring the node", end.span)
		}
		if o, ok := symbols[end.id]; ok {
			return o.kind
		}
		elements.Set(end.id, &elementState{named: end.named, kind: "usecase", shape: "ellipse", classes: []string{}})
		register(&end.named, "usecase")
		mention(end.id, end.span[0])
		return "usecase"
	}

	stylable := func(id string, s span) (classes, styles *[]string) {
		var kind string
		if o, ok := symbols[id]; ok {
			kind = o.kind
		}
		switch kind {
		case "actor":
			if a, ok := actors.Get(id); ok {
				return &a.classes, &a.styles
			}
		case "usecase":
			if u, ok := useCases.Get(id); ok {
				return &u.classes, &u.styles
			}
		case "boundary":
			if b, ok := systemBoundaries.Get(id); ok {
				return &b.classes, &b.styles
			}
		case "json":
			if j, ok := model.jsonNodes.Get(id); ok {
				return &j.classes, &j.styles
			}
		default:
			if e, ok := edges[id]; ok {
				return &e.classes, &e.styles
			}
		}
		fail("Class/style target '"+id+"' is unresolved or anonymous", s)
		return nil, nil
	}

	type declaration struct {
		offset int
		run    func()
	}
	var declarations []declaration
	for _, r := range d.relationships {
		mention(r.source.id, r.source.span[0])
		mention(r.target.id, r.target.span[0])
	}
	for _, draft := range d.elements {
		mention(draft.id, draft.span[0])
		declarations = append(declarations, declaration{draft.span[0], func() { declareElement(draft) }})
	}
	for _, draft := range d.boundaries {
		mention(draft.id, draft.span[0])
		declarations = append(declarations, declaration{draft.span[0], func() { declareBoundary(draft) }})
	}
	for _, draft := range d.jsons {
		mention(draft.id, draft.span[0])
		declarations = append(declarations, declaration{draft.span[0], func() { unique(draft.id, "json", draft.span) }})
	}
	for _, r := range d.relationships {
		if r.id != "" {
			declarations = append(declarations, declaration{r.idSpan[0], func() { unique(r.id, "edge", r.idSpan) }})
		}
	}
	slices.SortStableFunc(declarations, func(a, b declaration) int { return cmp.Compare(a.offset, b.offset) })
	for _, decl := range declarations {
		decl.run()
	}

	elementsInOrder := func() []*elementState {
		var list []*elementState
		for _, s := range elements.All() {
			list = append(list, s)
		}
		slices.SortStableFunc(list, func(a, b *elementState) int { return cmp.Compare(firstOf(a.id), firstOf(b.id)) })
		return list
	}

	for _, state := range elementsInOrder() {
		publish(state)
	}
	var boundaryList []*boundaryState
	for _, s := range boundaries.All() {
		boundaryList = append(boundaryList, s)
	}
	slices.SortStableFunc(boundaryList, func(a, b *boundaryState) int { return cmp.Compare(firstOf(a.id), firstOf(b.id)) })
	for _, state := range boundaryList {
		systemBoundaries.Set(state.id, &systemBoundary{
			id:        state.id,
			label:     state.label.text,
			labelType: state.label.typ,
			typ:       "rect",
			members:   []string{},
			classes:   []string{},
			styles:    []string{},
		})
	}
	jsonList := slices.Clone(d.jsons)
	slices.SortStableFunc(jsonList, func(a, b *draftJSON) int { return cmp.Compare(firstOf(a.id), firstOf(b.id)) })
	for _, draft := range jsonList {
		model.jsonNodes.Set(draft.id, &usecaseJSONNode{
			id:            draft.id,
			value:         draft.value,
			propertyOrder: draft.propertyOrder,
			classes:       slices.Clone(draft.classes),
			styles:        []string{},
		})
	}

	related := func(kind string) bool { return kind == "actor" || kind == "usecase" || kind == "json" }
	anonymous := 0
	for _, draft := range d.relationships {
		source, target, typ := &draft.source, &draft.target, draft.typ
		sourceKind := endpoint(source)
		targetKind := endpoint(target)
		if !related(sourceKind) {
			conflict("Relationship source '"+source.id+"' cannot be "+sourceKind, source.span, symbols[source.id].span, nil, nil)
		}
		if !related(targetKind) {
			conflict("Relationship target '"+target.id+"' cannot be "+targetKind, target.span, symbols[target.id].span, nil, nil)
		}
		if (typ == "include" || typ == "extend") && (sourceKind != "usecase" || targetKind != "usecase") {
			fail(typ+" relationship requires use-case endpoints", draft.span)
		}
		if typ == "generalization" && (sourceKind == "json" || sourceKind != targetKind) {
			fail("Generalization requires actor-to-actor or use-case-to-use-case endpoints", draft.span)
		}
		if typ == "association" && (sourceKind == "json" || targetKind == "json") && draft.arrowType > 2 {
			fail("JSON relationship '"+source.id+"' to '"+target.id+"' permits only point, reversed-point, or markerless solid association", draft.span)
		}
		id := draft.id
		if id == "" {
			id = "edge-" + strconv.Itoa(anonymous)
			anonymous++
		}
		r := &relationship{
			id:         id,
			explicitID: draft.id != "",
			source:     source.id,
			target:     target.id,
			typ:        typ,
			arrowType:  draft.arrowType,
			minlen:     draft.minlen,
			classes:    []string{},
			styles:     []string{},
		}
		if draft.label != nil {
			r.label = draft.label.text
			r.labelType = draft.label.typ
		}
		model.relationships = append(model.relationships, r)
		edges[id] = r
	}

	// The relationships may have added use cases; put all elements back in the order of first
	// mention.
	declaredActors := map[string]*actor{}
	for id, a := range actors.All() {
		declaredActors[id] = a
	}
	declaredUseCases := map[string]*useCase{}
	for id, u := range useCases.All() {
		declaredUseCases[id] = u
	}
	*actors = omap.Map[string, *actor]{}
	*useCases = omap.Map[string, *useCase]{}
	for _, state := range elementsInOrder() {
		if a, ok := declaredActors[state.id]; ok {
			actors.Set(state.id, a)
		} else if u, ok := declaredUseCases[state.id]; ok {
			useCases.Set(state.id, u)
		} else {
			publish(state)
		}
	}

	for _, m := range d.metadata {
		o, ok := symbols[m.target]
		if !ok {
			inferred := inferKind(m.properties)
			suffix := ""
			if inferred != "" {
				suffix = " (metadata implies " + inferred + ")"
			}
			fail("Metadata target '"+m.target+"' is unresolved"+suffix, m.span)
		}
		kind := o.kind
		if kind == "actor" || kind == "usecase" {
			state, _ := elements.Get(m.target)
			elementMetadata(state, m.properties, true)
			continue
		}
		boundary, hasBoundary := boundaries.Get(m.target)
		edge := edges[m.target]
		for _, p := range m.properties {
			str, isString := p.value.(string)
			b, isBool := p.value.(bool)
			switch {
			case kind == "boundary" && hasBoundary && p.key == "type" && isString && (str == "rect" || str == "package"):
				boundary.typ = str
			case kind == "edge" && edge != nil && p.key == "animate" && isBool:
				edge.animate = b
			case kind == "edge" && edge != nil && p.key == "animation" && isString && (str == "fast" || str == "slow"):
				edge.animation = str
				edge.animate = true
			default:
				invalid(m.target, kind, p)
			}
		}
	}
	for _, r := range model.relationships {
		if r.animation != "" {
			r.animate = true
		}
	}

	for _, state := range elements.All() {
		if state.kind == "actor" {
			hasIcon := state.icon != nil && *state.icon != ""
			if hasIcon && state.actorType != nil && *state.actorType != "normal" {
				fail("Actor '"+state.id+"' cannot combine icon with type '"+*state.actorType+"'", state.span)
			}
			if state.business != nil && *state.business && (hasIcon || state.actorType != nil && *state.actorType == "awesome") {
				fail("Business actor '"+state.id+"' must use normal or hollow geometry", state.span)
			}
		} else if state.shape == "rect" && state.business != nil && *state.business {
			fail("Rectangular use case '"+state.id+"' cannot be a business use case", state.span)
		}
		publish(state)
	}

	byOffset := slices.Clone(d.elements)
	slices.SortStableFunc(byOffset, func(a, b *draftElement) int { return cmp.Compare(a.span[0], b.span[0]) })
	for _, draft := range byOffset {
		if draft.parent == nil || draft.parent.id == "" {
			continue
		}
		b, _ := boundaries.Get(draft.parent.id)
		if !slices.Contains(b.members, draft.id) {
			b.members = append(b.members, draft.id)
		}
	}
	for _, state := range boundaries.All() {
		b, _ := systemBoundaries.Get(state.id)
		b.typ = state.typ
		if b.typ == "" {
			b.typ = "rect"
		}
		b.members = state.members
		b.classes = slices.Clone(state.classes)
	}

	notes := 0
	for _, draft := range d.notes {
		o, ok := symbols[draft.target]
		if !ok {
			fail("Note target '"+draft.target+"' is unresolved", draft.span)
		}
		if o.kind != "actor" && o.kind != "usecase" {
			conflict("Note target '"+draft.target+"' must be an actor or use case, not "+o.kind, draft.span, o.span, nil, nil)
		}
		id := "note-" + strconv.Itoa(notes)
		notes++
		model.notes.Set(id, &usecaseNote{id: id, target: draft.target, label: draft.label.text, labelType: draft.label.typ})
	}

	for _, def := range d.classDefs {
		for _, id := range def.ids {
			model.classDefs.Set(id, &classDef{id: id, styles: slices.Clone(def.styles)})
		}
	}
	for _, assignment := range d.classes {
		for _, target := range assignment.targets {
			classes, _ := stylable(target.id, target.span)
			pushUnique(classes, assignment.classes)
		}
	}
	for _, assignment := range d.styles {
		_, styles := stylable(assignment.target, assignment.span)
		*styles = append(*styles, assignment.styles...)
	}

	if d.direction != "" {
		model.direction = d.direction
	}
	model.title = title
	model.accTitle = d.accTitle
	model.accDescr = common.CollapseIndentedBreaks(d.accDescription)
	return model
}
