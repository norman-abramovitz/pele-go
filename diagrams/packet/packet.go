// Package packet is the packet diagram type: the fields of a network packet, in rows of bits.
package packet

import (
	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
)

// Diagram is the packet diagram type. Register it with pele.Register.
var Diagram = &core.Diagram{
	Type: "packet",
	Parse: func(source string, config *jsobj.Object, title *string, _ int, limits core.Limits) any {
		return buildPacket(parsePacket(source, limits.Origin), config, title)
	},
	Render: func(m any, config *jsobj.Object, rc *core.Context) core.Rendered {
		return renderPacket(m.(model), config, rc)
	},
}
