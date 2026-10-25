// Package ishikawa is the ishikawa diagram type: a fishbone of the causes of an effect.
package ishikawa

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the ishikawa diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "ishikawa",
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		db := &ishikawaDb{title: title}
		parseIshikawa(source, db, limits.Origin)
		return db
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderIshikawa(m.(*ishikawaDb), config, rc)
	},
}
