package kanban

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

func parseKanban(src common.Src, db common.OutlineDB, o *perr.Origin) {
	common.ParseOutline(src, db, true, o)
}
