package mindmap

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func parseMindmap(src string, db common.OutlineDB, o *perr.Origin) {
	common.ParseOutline(jsstr.UTF16(src), db, false, o)
}
