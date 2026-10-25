// Package sequence is the sequence diagram type.
package sequence

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
)

// Diagram is the sequence diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "sequence",
	Math: true,
	Parse: func(source string, config *jsobj.Object, title *string, _ int, _ core.Limits) any {
		var options seqDbOptions
		if wrap, ok := config.Get("wrap"); ok {
			if b, ok := wrap.(bool); ok {
				options.wrap = &b
			}
		}
		if seq, ok := config.Get("sequence"); ok {
			if o, ok := seq.(*jsobj.Object); ok {
				wrap, _ := o.Get("wrap")
				options.sequenceWrap = wrap == true
			}
		}
		db := newSeqDb(options)
		if title != nil && *title != "" {
			db.setDiagramTitle(*title)
		}
		parseSequence(jsstr.UTF16(source), db)
		return db
	},
	Render: func(model any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderSequence(model.(*seqDb), config, rc)
	},
}
