package eventmodeling

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/omap"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

type emLane struct {
	// Mermaid's ordering key: 0 for UI/Automation, 100 for Command/Read Model, 200 for Events,
	// and the next free number after those for each namespace.
	index int
	label string
	group string
}

type emBox struct {
	// The frame number as written, such as "01".
	id     string
	kind   string
	entity string
	name   string
	reset  bool
	lane   *emLane
	// Example data from the frame or from the data block it refers to; nil where upstream's
	// is undefined.
	data *string
}

type emRelation struct {
	source, target *emBox
}

type emNote struct {
	frame string
	text  string
}

type emSpecification struct {
	frame             string
	given, when, then []string
}

type eventModelingModel struct {
	// Empty strings stand for undefined: every use treats the two alike.
	title, accTitle, accDescr string
	boxes                     []*emBox
	// Sorted from top to bottom.
	lanes          []*emLane
	relations      []emRelation
	notes          []emNote
	specifications []emSpecification
	entities       []string
}

var kindNames = map[string]string{
	"ui":        "ui",
	"pcr":       "processor",
	"processor": "processor",
	"rmo":       "readmodel",
	"readmodel": "readmodel",
	"cmd":       "command",
	"command":   "command",
	"evt":       "event",
	"event":     "event",
}

type groupInfo struct {
	first         int
	label, prefix string
}

var groups = map[string]groupInfo{
	"trigger": {0, "UI/Automation", "UI/A: "},
	"state":   {100, "Command/Read Model", "C/RM: "},
	"stream":  {200, "Events", "Stream: "},
}

func groupOf(kind string) string {
	switch kind {
	case "ui", "processor":
		return "trigger"
	case "event":
		return "stream"
	}
	return "state"
}

// blockText is the text between the outer braces, without the indentation its lines share.
func blockText(raw string) string {
	open := strings.IndexByte(raw, '{')
	closeAt := strings.LastIndexByte(raw, '}')
	inner := ""
	if open != -1 && closeAt > open {
		inner = raw[open+1 : closeAt]
	} else if open == -1 && jsstr.Len(raw) >= 2 {
		inner = jsstr.Slice(raw, 1, jsstr.Len(raw)-1)
	}
	lines := strings.Split(strings.ReplaceAll(inner, "\r", ""), "\n")
	for len(lines) > 0 && jsstr.Trim(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && jsstr.Trim(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	indent := math.MaxInt
	for _, line := range lines {
		if jsstr.Trim(line) == "" {
			continue
		}
		indent = min(indent, jsstr.Len(line)-jsstr.Len(jsstr.TrimStart(line)))
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		n := jsstr.Len(line)
		out[i] = jsstr.TrimEnd(jsstr.Slice(line, min(indent, n), n))
	}
	return strings.Join(out, "\n")
}

func buildEventModel(a eventModelAst, title *string, o *perr.Origin) *eventModelingModel {
	ids := map[string]bool{}
	for _, frame := range a.frames {
		if ids[frame.name] {
			line := perr.Locate(o, a.source, frame.offset).Line
			e := perr.New(`Duplicate event modeling frame ID "`+frame.name+`" on line `+strconv.Itoa(line), perr.Semantic)
			e.Type = "eventmodeling"
			e.Line = line
			perr.Throw(e)
		}
		ids[frame.name] = true
	}

	data := map[string]string{}
	for _, entity := range a.dataEntities {
		if _, ok := data[entity.name]; !ok {
			data[entity.name] = entity.dataBlockValue
		}
	}

	// One lane per group, and one more per namespace within a group.
	laneOf := &omap.Map[string, *emLane]{}
	next := map[string]int{}
	lane := func(kind string, namespace *string) *emLane {
		group := groupOf(kind)
		key := group
		if namespace != nil {
			key = group + " " + *namespace
		}
		found, ok := laneOf.Get(key)
		if !ok {
			g := groups[group]
			index := g.first
			label := g.label
			if namespace != nil {
				index = g.first + 1
				if n, ok := next[group]; ok {
					index = n
				}
				next[group] = index + 1
				label = g.prefix + *namespace
			}
			found = &emLane{index: index, label: label, group: group}
			laneOf.Set(key, found)
		}
		return found
	}

	var boxes []*emBox
	var relations []emRelation
	for index, frame := range a.frames {
		// Only a two-part name has a namespace.
		parts := strings.Split(frame.entityIdentifier, ".")
		var namespace *string
		name := frame.entityIdentifier
		if len(parts) == 2 {
			namespace = &parts[0]
			name = parts[1]
		}
		kind, ok := kindNames[frame.modelEntityType]
		if !ok {
			kind = "event"
		}
		var text *string
		if frame.dataInlineValue != nil {
			text = new(blockText(*frame.dataInlineValue))
		}
		if frame.dataReference != nil {
			if block, ok := data[*frame.dataReference]; ok {
				text = new(blockText(block))
			}
		}
		box := &emBox{
			id:     frame.name,
			kind:   kind,
			entity: frame.entityIdentifier,
			name:   name,
			reset:  frame.reset,
			lane:   lane(kind, namespace),
			data:   text,
		}
		boxes = append(boxes, box)

		if len(frame.sourceFrames) > 0 {
			// Sources connect in frame order, and only frames that already have a box.
			for _, source := range boxes {
				if slices.Contains(frame.sourceFrames, source.id) {
					relations = append(relations, emRelation{source, box})
				}
			}
		} else if index > 0 && !box.reset {
			// Otherwise the nearest earlier frame in another lane feeds this one.
			for i := index - 1; i >= 0; i-- {
				if boxes[i].lane != box.lane {
					relations = append(relations, emRelation{boxes[i], box})
					break
				}
			}
		}
	}

	m := &eventModelingModel{boxes: boxes, relations: relations}
	if a.title != nil {
		m.title = common.TitleValue(*a.title)
	}
	if m.title == "" && title != nil {
		m.title = *title
	}
	if a.accTitle != nil {
		m.accTitle = jsstr.TrimStart(common.AccTitleValue(*a.accTitle))
	}
	if a.accDescr != nil {
		m.accDescr = common.CollapseIndentedBreaks(common.AccDescrValue(*a.accDescr))
	}
	for _, l := range laneOf.All() {
		m.lanes = append(m.lanes, l)
	}
	slices.SortStableFunc(m.lanes, func(a, b *emLane) int { return cmp.Compare(a.index, b.index) })
	for _, note := range a.noteEntities {
		m.notes = append(m.notes, emNote{frame: note.sourceFrame, text: blockText(note.dataBlockValue)})
	}
	for _, gwt := range a.gwtEntities {
		m.specifications = append(m.specifications, emSpecification{
			frame: gwt.sourceFrame,
			given: gwt.givenStatements,
			when:  gwt.whenStatements,
			then:  gwt.thenStatements,
		})
	}
	m.entities = a.modelEntities
	return m
}
