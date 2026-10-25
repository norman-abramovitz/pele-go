// Package usecase is the use case diagram type.
package usecase

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the use case diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "usecase",
	Math: true,
	Parse: func(source string, _ *jsobj.Object, title *string, _ int, limits core.Limits) any {
		t := ""
		if title != nil {
			t = *title
		}
		return parseUsecase(source, t, limits.Origin)
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderUsecase(m.(*usecaseModel), config, rc)
	},
}
