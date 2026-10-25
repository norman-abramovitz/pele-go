package sequence

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

var signalOf = func() [tSTICK_ARROW_BOTTOM_REVERSE_DOTTED + 1]int {
	var s [tSTICK_ARROW_BOTTOM_REVERSE_DOTTED + 1]int
	for i := range s {
		s[i] = -1
	}
	s[tSOLID_OPEN_ARROW] = lineSolidOpen
	s[tDOTTED_OPEN_ARROW] = lineDottedOpen
	s[tSOLID_ARROW] = lineSolid
	s[tDOTTED_ARROW] = lineDotted
	s[tBIDIRECTIONAL_SOLID_ARROW] = lineBidirectionalSolid
	s[tBIDIRECTIONAL_DOTTED_ARROW] = lineBidirectionalDotted
	s[tSOLID_CROSS] = lineSolidCross
	s[tDOTTED_CROSS] = lineDottedCross
	s[tSOLID_POINT] = lineSolidPoint
	s[tDOTTED_POINT] = lineDottedPoint
	s[tSOLID_ARROW_TOP] = lineSolidTop
	s[tSOLID_ARROW_BOTTOM] = lineSolidBottom
	s[tSTICK_ARROW_TOP] = lineStickTop
	s[tSTICK_ARROW_BOTTOM] = lineStickBottom
	s[tSOLID_ARROW_TOP_DOTTED] = lineSolidTopDotted
	s[tSOLID_ARROW_BOTTOM_DOTTED] = lineSolidBottomDotted
	s[tSTICK_ARROW_TOP_DOTTED] = lineStickTopDotted
	s[tSTICK_ARROW_BOTTOM_DOTTED] = lineStickBottomDotted
	s[tSOLID_ARROW_TOP_REVERSE] = lineSolidArrowTopReverse
	s[tSOLID_ARROW_BOTTOM_REVERSE] = lineSolidArrowBottomReverse
	s[tSTICK_ARROW_TOP_REVERSE] = lineStickArrowTopReverse
	s[tSTICK_ARROW_BOTTOM_REVERSE] = lineStickArrowBottomReverse
	s[tSOLID_ARROW_TOP_REVERSE_DOTTED] = lineSolidArrowTopReverseDotted
	s[tSOLID_ARROW_BOTTOM_REVERSE_DOTTED] = lineSolidArrowBottomReverseDotted
	s[tSTICK_ARROW_TOP_REVERSE_DOTTED] = lineStickArrowTopReverseDotted
	s[tSTICK_ARROW_BOTTOM_REVERSE_DOTTED] = lineStickArrowBottomReverseDotted
	return s
}()

// Block kinds.
const (
	kTop = iota
	kBox
	kLoop
	kRect
	kOpt
	kBreak
	kAlt
	kPar
	kCritical
)

var blockOf = map[int]int{
	tLoop:     kLoop,
	tRect:     kRect,
	tOpt:      kOpt,
	tBreak:    kBreak,
	tAlt:      kAlt,
	tPar:      kPar,
	tParOver:  kPar,
	tCritical: kCritical,
	tBox:      kBox,
}

const signalTokens = "'SOLID_OPEN_ARROW', 'DOTTED_OPEN_ARROW', 'SOLID_ARROW', 'DOTTED_ARROW', 'SOLID_CROSS', 'DOTTED_CROSS', 'SOLID_POINT', 'DOTTED_POINT'"

// frame is an open block. Upstream nests each block's statements in lists, which apply then
// walks in order; the port appends every statement to one list in that same order, so a block
// only needs its kind and text until its end.
type frame struct {
	kind int
	text string
}

func parseSequence(src common.Src, db *seqDb, o *perr.Origin) {
	tk := tokenize(src)
	types, texts, starts := tk.types, tk.texts, tk.starts
	i := 0

	typeAt := func(k int) int {
		if k < len(types) {
			return types[k]
		}
		return -1
	}
	fail := func(expected string) {
		t := types[i]
		if t == tERROR {
			perr.Throw(perr.SyntaxError(o, "sequence", src, starts[i], "", true))
		}
		perr.Throw(perr.SyntaxError(o, "sequence", src, starts[i], "Expecting "+expected+", got '"+tokenNames[t]+"'", false))
	}
	expect := func(typ int) string {
		if types[i] != typ {
			fail("'" + tokenNames[typ] + "'")
		}
		i++
		return texts[i-1]
	}
	actor := func() *seqStatement {
		t := types[i]
		if t != tACTOR && t != tLink {
			fail("'ACTOR'")
		}
		i++
		return &seqStatement{typ: "addParticipant", actor: texts[i-1]}
	}
	text2 := func() *seqText {
		t := db.parseMessage(jsstr.Trim(expect(tTXT))[1:])
		return &t
	}
	parseMessage := func(s string) *seqText {
		t := db.parseMessage(s)
		return &t
	}

	var list []*seqStatement
	add := func(items ...*seqStatement) { list = append(list, items...) }

	participant := func() *seqStatement {
		t := types[i]
		i++
		if t == tDestroy {
			node := actor()
			expect(tNEWLINE)
			node.typ = "destroyParticipant"
			return node
		}
		node := actor()
		if types[i] == tCONFIG_START && types[i-1] == tACTOR {
			i++
			content := jsstr.Trim(expect(tCONFIG_CONTENT))
			expect(tCONFIG_END)
			node.config = &content
		}
		node.draw = pick(t == tParticipant, "participant", "actor")
		switch types[i] {
		case tAS:
			i++
			alias := expect(tRestOfLine)
			expect(tNEWLINE)
			node.description = parseMessage(alias)
		case tNEWLINE:
			i++
		default:
			fail("'AS', 'NEWLINE'")
		}
		return node
	}

	signal := func(from *seqStatement) {
		centralFrom := false
		if types[i] == tCENTRAL {
			centralFrom = true
			i++
		}
		signalType := -1
		if t := types[i]; t < len(signalOf) {
			signalType = signalOf[t]
		}
		if signalType < 0 {
			fail(signalTokens)
		}
		i++
		mark := types[i]
		if mark == tCENTRAL || (!centralFrom && (mark == tPLUS || mark == tMINUS)) {
			i++
		}
		to := actor()
		msg := text2()
		message := &seqStatement{typ: "addMessage", from: from.actor, to: to.actor, signalType: signalType, msg: msg}
		add(from, to, message)
		switch {
		case centralFrom:
			dual := mark == tCENTRAL
			message.activate = dual
			message.centralConnection = pick(dual, lineCentralConnectionDual, lineCentralConnectionReverse)
			if dual {
				add(&seqStatement{typ: "centralConnection", signalType: lineCentralConnection, actor: to.actor})
			}
			add(&seqStatement{typ: "centralConnectionReverse", signalType: lineCentralConnectionReverse, actor: from.actor})
		case mark == tPLUS:
			message.activate = true
			add(&seqStatement{typ: "activeStart", signalType: lineActiveStart, actor: to.actor})
		case mark == tMINUS:
			add(&seqStatement{typ: "activeEnd", signalType: lineActiveEnd, actor: from.actor})
		case mark == tCENTRAL:
			message.activate = true
			message.centralConnection = lineCentralConnection
			add(&seqStatement{typ: "centralConnection", signalType: lineCentralConnection, actor: to.actor})
		}
	}

	menu := func(typ string) {
		i++
		who := actor()
		add(who, &seqStatement{typ: typ, actor: who.actor, text: text2()})
	}

	note := func() {
		i++
		t := types[i]
		if t == tOver {
			i++
			first := actor()
			who := []*seqStatement{first}
			ids := []string{first.actor, first.actor}
			if types[i] == tCOMMA {
				i++
				second := actor()
				who = append(who, second)
				ids = []string{first.actor, second.actor}
			}
			add(who...)
			add(&seqStatement{typ: "addNote", placement: placementOver, actors: ids, text: text2()})
			return
		}
		if t != tLeftOf && t != tRightOf {
			fail("'over', 'left_of', 'right_of'")
		}
		i++
		who := actor()
		placement := pick(t == tLeftOf, placementLeftOf, placementRightOf)
		add(who, &seqStatement{typ: "addNote", placement: placement, actor: who.actor, text: text2()})
	}

	autonumber := func() {
		i++
		node := &seqStatement{typ: "sequenceIndex", sequenceVisible: true, signalType: lineAutonumber}
		switch types[i] {
		case tNUM:
			node.sequenceIndex = jsnum.Parse(texts[i])
			i++
			node.sequenceIndexStep = 1
			if types[i] == tNUM {
				node.sequenceIndexStep = jsnum.Parse(texts[i])
				i++
			}
		case tOff:
			i++
			node.sequenceVisible = false
		}
		if types[i] != tNEWLINE {
			fail("'NEWLINE', 'NUM', 'off'")
		}
		i++
		add(node)
	}

	// openBlock writes a block's start entry, which upstream writes when the block closes.
	// Its text alone decides it, so writing it early changes nothing apply sees.
	openBlock := func(t int, text string) {
		switch blockOf[t] {
		case kBox:
			data := db.parseBoxData(text)
			add(&seqStatement{typ: "boxStart", boxData: &data})
		case kLoop:
			add(&seqStatement{typ: "loopStart", text: parseMessage(text), signalType: lineLoopStart})
		case kRect:
			add(&seqStatement{typ: "rectStart", text: parseMessage(text), signalType: lineRectStart})
		case kOpt:
			add(&seqStatement{typ: "optStart", text: parseMessage(text), signalType: lineOptStart})
		case kBreak:
			add(&seqStatement{typ: "breakStart", text: parseMessage(text), signalType: lineBreakStart})
		case kAlt:
			add(&seqStatement{typ: "altStart", text: parseMessage(text), signalType: lineAltStart})
		case kPar:
			add(&seqStatement{typ: "parStart", text: parseMessage(text), signalType: pick(t == tParOver, lineParOverStart, lineParStart)})
		default:
			add(&seqStatement{typ: "criticalStart", text: parseMessage(text), signalType: lineCriticalStart})
		}
	}

	closeBlock := func(f frame) {
		switch f.kind {
		case kBox:
			add(&seqStatement{typ: "boxEnd"})
		case kLoop:
			add(&seqStatement{typ: "loopEnd", signalType: lineLoopEnd})
		case kRect:
			add(&seqStatement{typ: "rectEnd", signalType: lineRectEnd})
		case kOpt:
			add(&seqStatement{typ: "optEnd", signalType: lineOptEnd})
		case kBreak:
			add(&seqStatement{typ: "breakEnd", signalType: lineBreakEnd})
		case kAlt:
			add(&seqStatement{typ: "altEnd", signalType: lineAltEnd})
		case kPar:
			add(&seqStatement{typ: "parEnd", signalType: lineParEnd})
		default:
			add(&seqStatement{typ: "criticalEnd", signalType: lineCriticalEnd})
		}
	}

	for types[i] == tNEWLINE {
		i++
	}
	expect(tSD)

	var stack []frame
	cur := frame{kind: kTop}
	end := func() {
		i++
		closeBlock(cur)
		cur = stack[len(stack)-1]
		stack = stack[:len(stack)-1]
	}

	for {
		t := types[i]
		if cur.kind == kBox {
			switch t {
			case tParticipant, tParticipantActor, tDestroy:
				add(participant())
			case tNEWLINE:
				i++
			case tEnd:
				end()
			default:
				fail("'end', 'NEWLINE', 'participant', 'participant_actor', 'destroy'")
			}
			continue
		}
		switch t {
		case tNEWLINE, tINVALID:
			i++
		case tParticipant, tParticipantActor, tDestroy:
			add(participant())
		case tCreate:
			i++
			next := types[i]
			if next != tParticipant && next != tParticipantActor && next != tDestroy {
				fail("'participant', 'participant_actor', 'destroy'")
			}
			node := participant()
			node.typ = "createParticipant"
			add(node)
		case tACTOR:
			signal(actor())
			expect(tNEWLINE)
		case tLink:
			next := typeAt(i + 1)
			if next == tACTOR || next == tLink {
				menu("addALink")
			} else {
				signal(actor())
			}
			expect(tNEWLINE)
		case tLinks:
			menu("addLinks")
			expect(tNEWLINE)
		case tProperties:
			menu("addProperties")
			expect(tNEWLINE)
		case tDetails:
			menu("addDetails")
			expect(tNEWLINE)
		case tNote:
			note()
			expect(tNEWLINE)
		case tAutonumber:
			autonumber()
		case tActivate, tDeactivate:
			i++
			who := actor()
			expect(tNEWLINE)
			if t == tActivate {
				add(&seqStatement{typ: "activeStart", signalType: lineActiveStart, actor: who.actor})
			} else {
				add(&seqStatement{typ: "activeEnd", signalType: lineActiveEnd, actor: who.actor})
			}
		case tTitle, tLegacyTitle:
			title := jsstr.Slice(texts[i], pick(t == tTitle, 6, 7), jsstr.Len(texts[i]))
			i++
			db.setDiagramTitle(title)
		case tAccTitle:
			i++
			db.setAccTitle(jsstr.Trim(expect(tAccTitleValue)))
		case tAccDescr:
			i++
			db.setAccDescription(jsstr.Trim(expect(tAccDescrValue)))
		case tAccDescrMultilineValue:
			db.setAccDescription(jsstr.Trim(texts[i]))
			i++
		case tLoop, tRect, tOpt, tBreak, tAlt, tPar, tParOver, tCritical, tBox:
			i++
			text := expect(tRestOfLine)
			stack = append(stack, cur)
			cur = frame{kind: blockOf[t], text: text}
			openBlock(t, text)
		case tElse, tAnd, tOption:
			want := kCritical
			if t == tElse {
				want = kAlt
			} else if t == tAnd {
				want = kPar
			}
			if cur.kind != want {
				fail("'end'")
			}
			i++
			text := expect(tRestOfLine)
			switch t {
			case tElse:
				add(&seqStatement{typ: "else", text: parseMessage(text), signalType: lineAltElse})
			case tAnd:
				add(&seqStatement{typ: "and", text: parseMessage(text), signalType: lineParAnd})
			default:
				add(&seqStatement{typ: "option", text: parseMessage(text), signalType: lineCriticalOption})
			}
		case tEnd:
			if cur.kind == kTop {
				fail("'NEWLINE', 'EOF'")
			}
			end()
		case tEND:
			if cur.kind != kTop {
				fail("'end'")
			}
			db.apply(list)
			return
		default:
			fail("'NEWLINE', 'ACTOR', 'participant', 'note', 'end'")
		}
	}
}
