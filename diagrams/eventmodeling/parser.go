package eventmodeling

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

func isLetter(c int) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' }

const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz"

// emID is /[_a-zA-Z][\w_]*/y.
var emID = &common.TokenType{Name: "EM_ID", First: letters, Match: func(src common.Src, at int) int {
	if !isLetter(src.At(at)) {
		return -1
	}
	i := at + 1
	for c := src.At(i); isLetter(c) || util.IsDigit(c); c = src.At(i) {
		i++
	}
	return i
}}

// word is a keyword that is also an identifier: Langium gives it the identifier as a longer
// alternative.
func word(text string) *common.TokenType {
	return &common.TokenType{Name: text, Literal: text, Longer: []*common.TokenType{emID}}
}

func sign(text string) *common.TokenType {
	return &common.TokenType{Name: text, Literal: text}
}

func renamed(t *common.TokenType, name string) *common.TokenType {
	c := *t
	c.Name = name
	return &c
}

var words = []string{
	"resetframe", "readmodel", "processor", "timeframe", "command", "entity", "event", "jsobj", "figma", "given", "json", "salt", "html",
	"text", "data", "note", "when", "then", "rmo", "cmd", "evt", "pcr", "uri",
}

// lastOnLine returns the index of the last c after from and before the line ends, or -1.
func lastOnLine(src common.Src, from int, c uint16) int {
	last := -1
	for i := from; i < len(src) && !util.IsLineEnd(int(src[i])); i++ {
		if src[i] == c {
			last = i
		}
	}
	return last
}

// dataInline is /\{(.*)\}|"(.*)"|'(.*)'/y.
var dataInline = &common.TokenType{Name: "EM_DATA_INLINE", First: `{"'`, Match: func(src common.Src, at int) int {
	var closer uint16
	switch src.At(at) {
	case '{':
		closer = '}'
	case '"':
		closer = '"'
	case '\'':
		closer = '\''
	default:
		return -1
	}
	if end := lastOnLine(src, at+1, closer); end != -1 {
		return end + 1
	}
	return -1
}}

// lineEndOrSpace matches (?:\r?\n|(?!\S)) at i.
func lineEndOrSpace(src common.Src, i int) int {
	if src.At(i) == '\r' && src.At(i+1) == '\n' {
		return i + 2
	}
	if src.At(i) == '\n' {
		return i + 1
	}
	if i >= len(src) || util.IsSpace(src.At(i)) {
		return i
	}
	return -1
}

// dataBlock is /\{[\t ]*\r?\n(?:[\S\s]*?\r?\n)?\}(?:\r?\n|(?!\S))/y.
var dataBlock = &common.TokenType{Name: "EM_DATA_BLOCK", First: "{", Match: func(src common.Src, at int) int {
	if src.At(at) != '{' {
		return -1
	}
	i := at + 1
	for src.At(i) == ' ' || src.At(i) == '\t' {
		i++
	}
	if src.At(i) == '\r' {
		i++
	}
	if src.At(i) != '\n' {
		return -1
	}
	body := i + 1
	closeAt := func(j int) int {
		if src.At(j) != '}' {
			return -1
		}
		return lineEndOrSpace(src, j+1)
	}
	// The optional body group is greedy, so a closing brace after a line break inside the
	// body is preferred to one straight after the opening line.
	for nl := body; nl < len(src); nl++ {
		if src[nl] != '\n' {
			continue
		}
		if end := closeAt(nl + 1); end != -1 {
			return end
		}
	}
	return closeAt(body)
}}

// emWS is /\s+/y.
var emWS = &common.TokenType{Name: "EM_WS", First: "\t\n\v\f\r ", Hidden: true, Match: func(src common.Src, at int) int {
	i := at
	for util.IsSpace(src.At(i)) {
		i++
	}
	if i == at {
		return -1
	}
	return i
}}

// mlComment is /\/\*[\s\S]*?\*\//y.
var mlComment = &common.TokenType{Name: "EM_ML_COMMENT", First: "/", Hidden: true, Match: func(src common.Src, at int) int {
	if !src.HasPrefix(at, "/*") {
		return -1
	}
	if end := src.Index("*/", at+2); end != -1 {
		return end + 2
	}
	return -1
}}

// slComment is /\/\/[^\n\r]*/y.
var slComment = &common.TokenType{Name: "EM_SL_COMMENT", First: "/", Hidden: true, Match: func(src common.Src, at int) int {
	if !src.HasPrefix(at, "//") {
		return -1
	}
	i := at + 2
	for i < len(src) && src[i] != '\n' && src[i] != '\r' {
		i++
	}
	return i
}}

var emLexer = func() *common.Lexer {
	keyword := common.Keyword("eventmodeling")
	keyword.Longer = []*common.TokenType{emID}
	types := []*common.TokenType{keyword}
	for _, w := range words {
		types = append(types, word(w))
	}
	types = append(types,
		sign("->>"),
		word("gwt"),
		word("ui"),
		word("md"),
		word("tf"),
		sign("[["),
		sign("]]"),
		word("rf"),
		sign("`"),
		sign("."),
		emID,
		&common.TokenType{Name: "EM_FID", First: "0123456789", Match: func(src common.Src, at int) int {
			i := at
			for i < at+3 && util.IsDigit(src.At(i)) {
				i++
			}
			if i == at {
				return -1
			}
			return i
		}},
		dataInline,
		dataBlock,
		common.AccDescrScanner("EM_ACC_DESCR"),
		renamed(common.AccTitle, "EM_ACC_TITLE"),
		renamed(common.Title, "EM_TITLE"),
		emWS,
		renamed(common.Yaml, "EM_YAML"),
		renamed(common.Directive, "EM_DIRECTIVE"),
		renamed(common.SingleLineComment, "EM_SINGLE_LINE_COMMENT"),
		mlComment,
		slComment,
	)
	return common.NewLexer(types)
}()

func index(name string) int {
	for k, t := range emLexer.Types() {
		if t.Name == name {
			return k
		}
	}
	return -1
}

func kinds(names ...string) map[int]bool {
	set := map[int]bool{}
	for _, name := range names {
		set[index(name)] = true
	}
	return set
}

var (
	tKeyword   = 0
	tArrow     = index("->>")
	tOpen      = index("[[")
	tClose     = index("]]")
	tTick      = index("`")
	tDot       = index(".")
	tID        = index("EM_ID")
	tFrameID   = index("EM_FID")
	tInline    = index("EM_DATA_INLINE")
	tBlock     = index("EM_DATA_BLOCK")
	tAccDescr  = index("EM_ACC_DESCR")
	tAccTitle  = index("EM_ACC_TITLE")
	tTitle     = index("EM_TITLE")
	tEntity    = index("entity")
	tData      = index("data")
	tNote      = index("note")
	tGwt       = index("gwt")
	tGiven     = index("given")
	tWhen      = index("when")
	tThen      = index("then")
	timeFrame  = kinds("tf", "timeframe")
	resetFrame = kinds("rf", "resetframe")
	entityType = kinds("rmo", "readmodel", "ui", "cmd", "command", "evt", "event", "pcr", "processor")
	dataType   = kinds("json", "jsobj", "figma", "salt", "uri", "md", "html", "text")
)

type emFrame struct {
	reset            bool
	name             string
	modelEntityType  string
	entityIdentifier string
	sourceFrames     []string
	// Nil pointers are upstream's undefined.
	dataReference   *string
	dataType        *string
	dataInlineValue *string
	// offset of the frame in the source, for error messages.
	offset int
}

type emDataEntity struct {
	name           string
	dataType       *string
	dataBlockValue string
}

type emNoteEntity struct {
	sourceFrame    string
	dataType       *string
	dataBlockValue string
}

type emGwt struct {
	sourceFrame                                     string
	givenStatements, whenStatements, thenStatements []string
}

type eventModelAst struct {
	// The whole token, keyword included: the grammar's own terminals get no value conversion.
	title, accTitle, accDescr *string
	modelEntities             []string
	frames                    []emFrame
	dataEntities              []emDataEntity
	noteEntities              []emNoteEntity
	gwtEntities               []emGwt
	source                    common.Src
}

func parseEventModel(source string, o *perr.Origin) eventModelAst {
	src := jsstr.UTF16(source)
	r := common.NewReader(emLexer.Tokenize(src, "eventmodeling", o), emLexer.Types(), "eventmodeling", o)
	a := eventModelAst{source: src}
	at := func(kind int) bool { return r.Kind() == kind }
	one := func(set map[int]bool, expected string) string {
		if !set[r.Kind()] {
			r.Fail(expected)
		}
		return r.Take()
	}
	qualifiedName := func() string {
		name := r.Expect(tID)
		for r.Accept(tDot) {
			name += "." + r.Expect(tID)
		}
		return name
	}
	readDataType := func() *string {
		if !r.Accept(tTick) {
			return nil
		}
		typ := one(dataType, "a data type")
		r.Expect(tTick)
		return &typ
	}
	statements := func() []string {
		var out []string
		for {
			one(entityType, "an entity type")
			out = append(out, r.Expect(tID))
			if !entityType[r.Kind()] {
				return out
			}
		}
	}

	r.Expect(tKeyword)
	for !at(-1) {
		kind := r.Kind()
		switch {
		case kind == tAccDescr:
			a.accDescr = new(r.Take())
		case kind == tAccTitle:
			a.accTitle = new(r.Take())
		case kind == tTitle:
			a.title = new(r.Take())
		case kind == tEntity:
			r.I++
			a.modelEntities = append(a.modelEntities, qualifiedName())
		case timeFrame[kind] || resetFrame[kind]:
			frame := emFrame{reset: !timeFrame[kind], offset: r.Tokens.Starts[r.I]}
			r.I++
			frame.name = r.Expect(tFrameID)
			frame.modelEntityType = one(entityType, "an entity type")
			frame.entityIdentifier = qualifiedName()
			for r.Accept(tArrow) {
				frame.sourceFrames = append(frame.sourceFrames, r.Expect(tFrameID))
			}
			if r.Accept(tOpen) {
				frame.dataReference = new(r.Expect(tID))
				r.Expect(tClose)
			}
			if at(tTick) || at(tInline) {
				frame.dataType = readDataType()
				frame.dataInlineValue = new(r.Expect(tInline))
			}
			a.frames = append(a.frames, frame)
		case kind == tData:
			r.I++
			entity := emDataEntity{name: r.Expect(tID)}
			entity.dataType = readDataType()
			entity.dataBlockValue = r.Expect(tBlock)
			a.dataEntities = append(a.dataEntities, entity)
		case kind == tNote:
			r.I++
			note := emNoteEntity{sourceFrame: r.Expect(tFrameID)}
			note.dataType = readDataType()
			note.dataBlockValue = r.Expect(tBlock)
			a.noteEntities = append(a.noteEntities, note)
		case kind == tGwt:
			r.I++
			gwt := emGwt{sourceFrame: r.Expect(tFrameID)}
			r.Expect(tGiven)
			gwt.givenStatements = statements()
			if r.Accept(tWhen) {
				gwt.whenStatements = statements()
			}
			r.Expect(tThen)
			gwt.thenStatements = statements()
			a.gwtEntities = append(a.gwtEntities, gwt)
		default:
			r.Fail("a frame, an entity, a data block, a note, or a specification")
		}
	}
	return a
}
