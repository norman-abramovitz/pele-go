package git

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsnum"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
)

const (
	dot  = 7
	step = 44
	lane = 44
	// Shorter than a step, so a curve between lanes never runs through a commit on a lane it
	// crosses.
	curve    = 36
	gap      = 8
	labelGap = 20
	tail     = 20
	notch    = 7
	tagGap   = 6
	half     = math.Sqrt2 / 2
)

var classes = [...]string{"normal", "reverse", "highlight", "merge", "cherry-pick"}

type view struct {
	commit *gitCommit
	index  int
	lane   int
	m      float64
	// label is nil where upstream's is undefined.
	label *text.Label
	tags  []text.Label
}

func flag(config *jsobj.Object, key string, fallback bool) bool {
	v, _ := config.Get("gitGraph")
	git, _ := v.(*jsobj.Object)
	if b, ok := git.Get(key); ok {
		if b, ok := b.(bool); ok {
			return b
		}
	}
	return fallback
}

func tagWidth(tag text.Label) float64 {
	return notch + 4 + tag.Width + 6
}

func symbol(typ int, x, y float64, color string) string {
	at := `cx="` + svg.Num(x) + `" cy="` + svg.Num(y) + `"`
	if typ == commitHighlight {
		return `<rect x="` + svg.Num(x-dot-1) + `" y="` + svg.Num(y-dot-1) + `" width="` + strconv.Itoa(2*dot+2) + `" height="` + strconv.Itoa(2*dot+2) + `" rx="2" fill="` + color + `"/>`
	}
	if typ == commitMerge {
		return `<circle ` + at + ` r="` + strconv.Itoa(dot-1) + `" fill="var(--_bg)" stroke="` + color + `" stroke-width="2"/><circle ` + at + ` r="2.5" fill="` + color + `"/>`
	}
	d := `<circle ` + at + ` r="` + strconv.Itoa(dot) + `" fill="` + color + `"/>`
	if typ == commitReverse {
		return d +
			`<path d="M` + svg.Num(x-3) + `,` + svg.Num(y-3) + `l6,6M` + svg.Num(x-3) + `,` + svg.Num(y+3) + `l6,-6" fill="none" stroke="var(--_bg)" stroke-width="1.5" stroke-linecap="round"/>`
	}
	if typ == commitCherryPick {
		return d +
			`<path d="M` + svg.Num(x-2.6) + `,` + svg.Num(y+1) + `L` + svg.Num(x+0.8) + `,` + svg.Num(y-4.2) + `L` + svg.Num(x+2.6) + `,` + svg.Num(y+1) + `" fill="none" stroke="var(--_bg)" stroke-linejoin="round"/>` +
			`<circle cx="` + svg.Num(x-2.6) + `" cy="` + svg.Num(y+2.2) + `" r="1.8" fill="var(--_bg)"/>` +
			`<circle cx="` + svg.Num(x+2.6) + `" cy="` + svg.Num(y+2.2) + `" r="1.8" fill="var(--_bg)"/>`
	}
	return d
}

func renderGit(model *gitModel, config *jsobj.Object, rc *core.Context) core.Rendered {
	if model.direction != "LR" {
		return draw(model, config, rc, model.direction)
	}
	return common.TurnToFit(rc.Options, func(down bool) core.Rendered {
		if down {
			return draw(model, config, rc, "TB")
		}
		return draw(model, config, rc, "LR")
	})
}

func draw(model *gitModel, config *jsobj.Object, rc *core.Context, direction string) core.Rendered {
	opts := rc.Options
	size := 16.0
	if opts.FontSize != nil {
		size = *opts.FontSize
	}
	small := jsmath.Round(float64(size * 0.75))
	measurer := rc.Measurer()
	pad := 8.0
	if opts.Padding != nil {
		pad = *opts.Padding
	}

	showBranches := flag(config, "showBranches", true)
	showCommitLabel := flag(config, "showCommitLabel", true)
	rotate := flag(config, "rotateCommitLabel", true)
	parallel := flag(config, "parallelCommits", false)
	vertical := direction != "LR"
	flip := direction == "BT"

	laneOf := map[string]int{}
	for index, name := range model.lanes {
		laneOf[name] = index
	}
	laneCount := len(model.lanes)
	medium := jsmath.Round(float64(size * 0.875))
	// Mermaid writes branch names, tags, and commit ids as plain text, so tags in them show as
	// written.
	names := make([]text.Label, laneCount)
	for i, name := range model.lanes {
		if !showBranches {
			name = ""
		}
		names[i] = common.PlainLabel(name, measurer, medium, 4000, 0)
	}

	var views []*view
	byID := map[string]*view{}
	ordered := collectCommits(model)
	slices.SortStableFunc(ordered, func(a, b *gitCommit) int { return cmp.Compare(a.seq, b.seq) })
	for _, commit := range ordered {
		labelled := showCommitLabel &&
			commit.typ != commitCherryPick &&
			(commit.typ != commitMerge || commit.customID)
		tags := []text.Label{}
		for _, tag := range commit.tags {
			label := common.PlainLabel(tag, measurer, small, 4000, 0)
			if len(label.Lines) > 0 {
				tags = append(tags, label)
			}
		}
		v := &view{
			commit: commit,
			index:  len(views),
			lane:   laneOf[commit.branch],
			tags:   tags,
		}
		if labelled {
			label := common.PlainLabel(commit.id, measurer, small, 4000, 0)
			v.label = &label
		}
		views = append(views, v)
		byID[commit.id] = v
	}

	// Room each commit needs: along the main axis on either side, and across it towards the
	// previous lane (lo) and the next one (hi). Tags sit on one side of a lane and ids on the
	// other.
	halfOf := make([]float64, len(views))
	tagHalf := make([]float64, len(views))
	lo := make([]float64, laneCount)
	hi := make([]float64, laneCount)
	for i := range laneCount {
		lo[i] = dot
		hi[i] = dot
	}
	for k, v := range views {
		tagsMain := 0.0
		tagsCross := 0.0
		for _, tag := range v.tags {
			w := tagWidth(tag)
			if vertical {
				tagsMain = max(tagsMain, tag.Height)
				tagsCross += w + 4
			} else {
				tagsMain = max(tagsMain, w)
				tagsCross += tag.Height + 3
			}
		}
		labelMain := 0.0
		labelCross := 0.0
		if v.label != nil && len(v.label.Lines) > 0 {
			if rotate {
				labelCross = 3 + float64(half*(v.label.Width+v.label.Height))
			} else {
				if vertical {
					labelMain = v.label.Height
					labelCross = 6 + v.label.Width
				} else {
					labelMain = v.label.Width
					labelCross = 6 + v.label.Height
				}
			}
		}
		halfOf[k] = max(dot, float64(tagsMain/2), float64(labelMain/2))
		tagHalf[k] = tagsMain / 2
		tagSide := float64(dot)
		if len(v.tags) > 0 {
			tagSide = dot + (tagGap + tagsCross)
		}
		labelSide := dot + labelCross
		if vertical {
			lo[v.lane] = max(lo[v.lane], labelSide)
			hi[v.lane] = max(hi[v.lane], tagSide)
		} else {
			lo[v.lane] = max(lo[v.lane], tagSide)
			hi[v.lane] = max(hi[v.lane], labelSide)
		}
	}

	// In a left-to-right graph the tags stand over their commit. A connector that comes down
	// from a lane above joins the commit's lane this far before the commit, clear of the tags,
	// and the commit is placed far enough along to leave room for that.
	lead := make([]float64, len(views))
	if !vertical {
		for k, v := range views {
			if tagHalf[k] == 0 {
				continue
			}
			for _, id := range v.commit.parents {
				if parent, ok := byID[id]; ok && parent.lane < v.lane {
					lead[k] = tagHalf[k] + 4
				}
			}
		}
	}
	space := func(p, k int) float64 {
		around := 0.0
		if lead[k] > 0 {
			around = halfOf[p] + curve + lead[k]
		}
		return max(step, halfOf[p]+halfOf[k]+gap, around)
	}

	for k, v := range views {
		if !parallel {
			if k > 0 {
				v.m = views[k-1].m + space(k-1, k)
			}
			continue
		}
		// Mermaid's parallelCommits: a commit sits one step after its furthest parent, so
		// branches that grow side by side share positions instead of taking turns.
		m := 0.0
		for _, id := range v.commit.parents {
			parent, ok := byID[id]
			if !ok || parent.index >= k {
				continue
			}
			m = max(m, parent.m+space(parent.index, k))
		}
		v.m = m
	}
	end := 0.0
	for _, v := range views {
		end = max(end, v.m)
	}

	cross := make([]float64, laneCount)
	for i := range laneCount {
		name := names[i]
		reach := 0.0
		if len(name.Lines) > 0 {
			if vertical {
				reach = float64(name.Width/2) + 8
			} else {
				reach = float64(name.Height/2) + 2
			}
		}
		lo[i] = max(lo[i], reach)
		hi[i] = max(hi[i], reach)
		if i > 0 {
			cross[i] = cross[i-1] + max(lane, hi[i-1]+lo[i]+gap)
		}
	}

	px := func(m, c float64) float64 {
		if vertical {
			return c
		}
		return m
	}
	py := func(m, c float64) float64 {
		if !vertical {
			return c
		}
		if flip {
			return -m
		}
		return m
	}
	point := func(m, c float64) string { return svg.Num(px(m, c)) + "," + svg.Num(py(m, c)) }

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	grow := func(x0, y0, x1, y1 float64) {
		if x0 < minX {
			minX = x0
		}
		if y0 < minY {
			minY = y0
		}
		if x1 > maxX {
			maxX = x1
		}
		if y1 > maxY {
			maxY = y1
		}
	}

	var branches strings.Builder
	if showBranches {
		for i := range laneCount {
			name := names[i]
			color := svg.SeriesColor(i)
			c := cross[i]
			w := name.Width + 16
			h := name.Height + 4
			along := w
			if vertical {
				along = h
			}
			from := float64(-labelGap)
			if len(name.Lines) > 0 {
				from = -labelGap - along
			}
			cx := px(from+float64(along/2), c)
			cy := py(from+float64(along/2), c)
			grow(px(-labelGap, c), py(-labelGap, c), px(-labelGap, c), py(-labelGap, c))
			grow(px(end+tail, c), py(end+tail, c), px(end+tail, c), py(end+tail, c))
			branches.WriteString(`<g class="pele-branch" data-id="` + svg.EscText(model.lanes[i]) + `">` +
				`<path class="pele-lane" d="M` + point(-labelGap, c) + `L` + point(end+tail, c) + `" stroke="` + color + `" stroke-dasharray="2 4"/>`)
			if len(name.Lines) > 0 {
				grow(cx-float64(w/2), cy-float64(h/2), cx+float64(w/2), cy+float64(h/2))
				branches.WriteString(`<rect x="` + svg.Num(cx-float64(w/2)) + `" y="` + svg.Num(cy-float64(h/2)) + `" width="` + svg.Num(w) + `" height="` + svg.Num(h) + `" fill="` + color + `"` + svg.RoundedStyle + `/>` +
					svg.LabelSVG(name, cx, cy, ` class="pele-label" fill="var(--_bg)"`, nil))
			}
			branches.WriteString(`</g>`)
		}
	}

	// Positions of the commits on each lane, to tell whether a connector may run along it.
	taken := make([][]float64, laneCount)
	for _, v := range views {
		taken[v.lane] = append(taken[v.lane], v.m)
	}
	if parallel {
		for _, list := range taken {
			slices.SortStableFunc(list, cmp.Compare[float64])
		}
	}
	busy := func(lane int, from, to float64) bool {
		list := taken[lane]
		low, high := 0, len(list)
		for low < high {
			mid := (low + high) >> 1
			if list[mid] <= from {
				low = mid + 1
			} else {
				high = mid
			}
		}
		return low < len(list) && list[low] < to
	}

	var curves, lines strings.Builder
	for _, v := range views {
		commit := v.commit
		c2 := cross[v.lane]
		m2 := v.m
		for j, id := range commit.parents {
			parent, ok := byID[id]
			if !ok || parent == v {
				continue
			}
			c1 := cross[parent.lane]
			m1 := parent.m
			if parent.lane == v.lane {
				lines.WriteString(`<path class="pele-edge" d="M` + point(m1, c1) + `L` + point(m2, c2) + `" stroke="` + svg.SeriesColor(v.lane) + `"/>`)
				continue
			}
			merged := j > 0 && commit.typ == commitMerge
			picked := j > 0 && !merged
			d := "M" + point(m1, c1)
			if m2 <= m1 {
				mid := (m1 + m2) / 2
				d += "C" + point(mid, c1) + " " + point(mid, c2) + " " + point(m2, c2)
			} else if !merged && !busy(v.lane, m1, m2) {
				// Leaves the parent at once and runs along the child's lane.
				s := min(curve, m2-m1)
				d += "C" + point(m1+float64(s/2), c1) + " " + point(m1+float64(s/2), c2) + " " + point(m1+s, c2) + "L" + point(m2, c2)
			} else {
				// Turns in just before the child, or before its tags when it comes from their side.
				to := m2
				if parent.lane < v.lane && m2-lead[v.index] > m1 {
					to = m2 - lead[v.index]
				}
				s := min(curve, to-m1)
				along := c1
				if !merged || busy(parent.lane, m1, m2) {
					// Neither lane is free to run along. Between two lanes, past the ids of one
					// and before the tags of the next, a strip stays empty, and the connector
					// takes that.
					if c1 > c2 {
						along = c2 + hi[v.lane] + float64(gap/2)
					} else {
						along = c2 - lo[v.lane] - float64(gap/2)
					}
					s = min(curve, (to-m1)/2)
					d += "C" + point(m1+float64(s/2), c1) + " " + point(m1+float64(s/2), along) + " " + point(m1+s, along)
				}
				d += "L" + point(to-s, along) + "C" + point(to-float64(s/2), along) + " " + point(to-float64(s/2), c2) + " " + point(to, c2)
				if to < m2 {
					d += "L" + point(m2, c2)
				}
			}
			class := ""
			if merged {
				class = " pele-edge-merge"
			} else if picked {
				class = " pele-edge-cherry-pick"
			}
			colorLane := v.lane
			if merged {
				colorLane = parent.lane
			}
			dash := ""
			if picked {
				dash = ` stroke-dasharray="4 3"`
			}
			curves.WriteString(`<path class="pele-edge` + class + `" d="` + d + `" ` +
				`stroke="` + svg.SeriesColor(colorLane) + `"` + dash + `/>`)
		}
	}

	var commits strings.Builder
	for _, v := range views {
		commit, label, tags := v.commit, v.label, v.tags
		x := px(v.m, cross[v.lane])
		y := py(v.m, cross[v.lane])
		typ := commit.typ
		if commit.customType != nil {
			typ = *commit.customType
		}
		grow(x-dot-1, y-dot-1, x+dot+1, y+dot+1)
		// Merges and cherry-picks carry a message Mermaid makes up; only a written one is worth
		// showing.
		var body strings.Builder
		if commit.typ < commitMerge && commit.message != "" {
			body.WriteString(`<title>` + svg.EscText(commit.message) + `</title>`)
		}
		body.WriteString(symbol(typ, x, y, svg.SeriesColor(v.lane)))

		if label != nil && len(label.Lines) > 0 {
			w := label.Width
			h := label.Height
			back := func(cx, cy float64) string {
				return `<rect x="` + svg.Num(cx-float64(w/2)-2) + `" y="` + svg.Num(cy-float64(h/2)+2) + `" width="` + svg.Num(w+4) + `" height="` + svg.Num(h-4) + `" rx="3" fill="var(--_bg)"/>`
			}
			if rotate {
				// The text ends beside the commit and trails back the way the history came.
				k := float64(float64(half*h) / 2)
				ax := x + 3
				ay := y + dot + 3 + k
				if vertical {
					ax = x - dot - 3 - k
					ay = y
				}
				reach := float64(half*w) + k
				if flip {
					grow(ax-reach, ay-reach, ax+k, ay+k)
				} else {
					grow(ax-reach, ay-k, ax+k, ay+reach)
				}
				angle := "-45"
				if flip {
					angle = "45"
				}
				body.WriteString(`<g class="pele-commit-label" transform="rotate(` + angle + ` ` + svg.Num(ax) + ` ` + svg.Num(ay) + `)">` +
					back(ax-float64(w/2), ay) +
					svg.LabelSVG(*label, ax-float64(w/2), ay, "", nil) +
					`</g>`)
			} else {
				cx := x
				cy := y + dot + 6 + float64(h/2)
				if vertical {
					cx = x - dot - 6 - float64(w/2)
					cy = y
				}
				grow(cx-float64(w/2), cy-float64(h/2), cx+float64(w/2), cy+float64(h/2))
				body.WriteString(`<g class="pele-commit-label">` + back(cx, cy) + svg.LabelSVG(*label, cx, cy, "", nil) + `</g>`)
			}
		}

		// Beside the lane the tags form a row; above it they stack, the first one furthest out.
		offset := float64(dot + tagGap)
		first, by := len(tags)-1, -1
		if vertical {
			first, by = 0, 1
		}
		for t := first; t >= 0 && t < len(tags); t += by {
			tag := tags[t]
			w := tagWidth(tag)
			h := tag.Height
			left := x - float64(w/2)
			cy := y - offset - float64(h/2)
			if vertical {
				left = x + offset
				cy = y
			}
			if vertical {
				offset += w + 4
			} else {
				offset += h + 3
			}
			grow(left, cy-float64(h/2), left+w, cy+float64(h/2))
			body.WriteString(`<g class="pele-tag">` +
				`<path d="M` + svg.Num(left) + `,` + svg.Num(cy) + `L` + svg.Num(left+notch) + `,` + svg.Num(cy-float64(h/2)) + `H` + svg.Num(left+w) + `V` + svg.Num(cy+float64(h/2)) + `H` + svg.Num(left+notch) + `Z" fill="var(--_s)" stroke="var(--_b)" stroke-linejoin="round"/>` +
				`<circle cx="` + svg.Num(left+notch) + `" cy="` + svg.Num(cy) + `" r="1.5" fill="var(--_b)"/>` +
				svg.LabelSVG(tag, left+notch+4+float64(tag.Width/2), cy, ` fill="var(--_fg)"`, nil) +
				`</g>`)
		}

		merge := ""
		if commit.typ == commitMerge && typ != commitMerge {
			merge = " pele-commit-merge"
		}
		class := classes[0]
		if typ >= 0 && typ < len(classes) {
			class = classes[typ]
		}
		commits.WriteString(`<g class="pele-commit pele-commit-` + class + merge + `" data-id="` + svg.EscText(commit.id) + `">` + body.String() + `</g>`)
	}

	if minX > maxX {
		grow(0, 0, 0, 0)
	}
	title := text.Layout(model.title, false, measurer, size, 4000, text.Bold, rc.Icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 16
	}
	inner := max(maxX-minX, title.Width)
	width := math.Ceil(inner + 2*pad)
	height := math.Ceil(maxY - minY + titleHeight + 2*pad)
	dx := pad - minX + float64((inner-(maxX-minX))/2)
	dy := pad + titleHeight - minY

	var content strings.Builder
	content.WriteString(svg.LabelSVG(title, float64(width/2), pad+float64(title.Height/2), ` class="pele-title" font-weight="var(--_tw)"`, nil))
	content.WriteString(`<g transform="translate(` + svg.Num(dx) + `,` + svg.Num(dy) + `)">`)
	if branches.Len() > 0 {
		content.WriteString(`<g class="pele-branches" fill="none" font-size="` + jsnum.Format(medium) + `">` + branches.String() + `</g>`)
	}
	if curves.Len() > 0 || lines.Len() > 0 {
		content.WriteString(`<g class="pele-edges" fill="none" stroke-width="2" stroke-linecap="round">` + curves.String() + lines.String() + `</g>`)
	}
	if commits.Len() > 0 {
		content.WriteString(`<g class="pele-commits" font-size="` + jsnum.Format(small) + `" fill="var(--_m)">` + commits.String() + `</g>`)
	}
	content.WriteString(`</g>`)

	doc := svg.Document("gitGraph", width, height, size, svg.DocumentOptions{
		IDPrefix:       opts.IDPrefix,
		Responsive:     rc.Responsive,
		FontFamily:     opts.FontFamily,
		FontFamilyMono: opts.FontFamilyMono,
	}, svg.Accessible{AccTitle: model.accTitle, AccDescr: model.accDescr}, content.String())
	return core.Rendered{SVG: doc, Width: width, Height: height, Links: []core.Link{}}
}

func collectCommits(model *gitModel) []*gitCommit {
	var out []*gitCommit
	for _, c := range model.commits.All() {
		out = append(out, c)
	}
	return out
}
