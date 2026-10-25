package gantt

import (
	"math"
	"slices"
	"strings"
	"time"

	"github.com/Broderick-Westrope/pele-go/internal/core"
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsdate"
	"github.com/Broderick-Westrope/pele-go/internal/jsmath"
	"github.com/Broderick-Westrope/pele-go/internal/jsobj"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/omap"
	"github.com/Broderick-Westrope/pele-go/internal/svg"
	"github.com/Broderick-Westrope/pele-go/internal/text"
	"github.com/Broderick-Westrope/pele-go/internal/util"
)

const (
	chartWidth = 800
	minPlot    = 240
	labelGap   = 8
)

func ganttConfig(config *jsobj.Object, key string) (any, bool) {
	section, _ := config.Get("gantt")
	obj, ok := section.(*jsobj.Object)
	if !ok {
		return nil, false
	}
	return obj.Get(key)
}

// setting is a number from the gantt config, clamped, or false when there is none.
func setting(config *jsobj.Object, key string, lo, hi float64) (float64, bool) {
	v, _ := ganttConfig(config, key)
	n, ok := v.(float64)
	if !ok || math.IsInf(n, 0) || math.IsNaN(n) {
		return 0, false
	}
	return math.Min(hi, math.Max(lo, n)), true
}

func settingOr(config *jsobj.Object, key string, lo, hi, def float64) float64 {
	if v, ok := setting(config, key, lo, hi); ok {
		return v
	}
	return def
}

func configText(config *jsobj.Object, key string) (string, bool) {
	v, _ := ganttConfig(config, key)
	s, ok := v.(string)
	return s, ok
}

func flag(config *jsobj.Object, key string) bool {
	v, _ := ganttConfig(config, key)
	return v == true
}

// replaceBreaks is raw.replace(/<br\s*\/?>/gi, ' ').
func replaceBreaks(raw string) string {
	var b strings.Builder
	for i := 0; i < len(raw); {
		if raw[i] == '<' && i+2 < len(raw) && (raw[i+1]|0x20) == 'b' && (raw[i+2]|0x20) == 'r' {
			j := i + 3
			for j < len(raw) {
				r, n := jsstr.Decode(raw, j)
				if !jsstr.IsSpace(r) {
					break
				}
				j += n
			}
			if j < len(raw) && raw[j] == '/' {
				j++
			}
			if j < len(raw) && raw[j] == '>' {
				b.WriteByte(' ')
				i = j + 1
				continue
			}
		}
		b.WriteByte(raw[i])
		i++
	}
	return b.String()
}

// oneLine lays out a task label, which sits on one row, so only its first line is kept.
func oneLine(raw string, measurer text.Measurer, size float64, icons *text.Icons) text.Label {
	label := text.Layout(replaceBreaks(raw), false, measurer, size, 4000, 0, icons)
	if len(label.Lines) <= 1 {
		return label
	}
	label.Lines = label.Lines[:1]
	label.Widths = label.Widths[:1]
	label.Width = label.Widths[0]
	label.Height = label.LineHeight
	return label
}

// valueOf is +date for a date that may be undefined.
func valueOf(d *jsdate.Date) float64 {
	if d == nil {
		return math.NaN()
	}
	return d.Time()
}

// scheduled reports whether scheduling gave the task a start and an end. One that waits on
// a task that never resolves has neither.
func scheduled(t *task) bool {
	return !math.IsNaN(valueOf(t.startTime)) && !math.IsNaN(valueOf(t.endTime))
}

func sign(v float64) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	}
	return 0
}

// packRows is Mermaid's compact mode: within a section, a task goes on the first row that is
// free when it starts.
func packRows(tasks []*task, offset int, rowOf map[*task]int) int {
	var sorted []*task
	for _, t := range tasks {
		if scheduled(t) {
			sorted = append(sorted, t)
		}
	}
	slices.SortStableFunc(sorted, func(a, b *task) int {
		if d := a.startTime.Time() - b.startTime.Time(); d != 0 {
			return sign(d)
		}
		return a.order - b.order
	})
	var free []float64
	for _, t := range sorted {
		start := t.startTime.Time()
		row := 0
		for row < len(free) && start < free[row] {
			row++
		}
		if row == len(free) {
			free = append(free, 0)
		}
		free[row] = t.endTime.Time()
		rowOf[t] = offset + row
	}
	rows := len(free)
	for _, t := range tasks {
		if _, ok := rowOf[t]; !ok {
			rowOf[t] = offset + rows
			rows++
		}
	}
	return max(rows, 1)
}

type bar struct {
	task   *task
	label  text.Label
	row    int
	x0, x1 float64
	labelX float64
	inside bool
}

func renderGantt(model *ganttDb, config *jsobj.Object, rc *core.Context) core.Rendered {
	options := rc.Options
	size := 16.0
	if options.FontSize != nil {
		size = *options.FontSize
	}
	measurer := rc.Measurer()
	pad := 8.0
	if options.Padding != nil {
		pad = *options.Padding
	}
	loc := options.Location
	if loc == nil {
		loc = time.Local
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	nowMs := float64(now.UnixMilli())

	labelSize := settingOr(config, "fontSize", 4, 72, jsmath.Round(size*0.8125))
	sectionSize := settingOr(config, "sectionFontSize", 4, 72, labelSize)
	tickSize := jsmath.Round(size * 0.75)
	barHeight := settingOr(config, "barHeight", 4, 400, jsmath.Round(labelSize*1.85))
	barGap := settingOr(config, "barGap", 0, 400, jsmath.Round(barHeight/4))
	rowHeight := barHeight + barGap
	sectionStyles := jsmath.Round(settingOr(config, "numberSectionStyles", 1, 1000, 4))
	compact := model.displayMode == "compact"
	topAxis := model.topAxis || flag(config, "topAxis")

	sched := model.getTasks(nowMs, loc)
	tasks := sched.tasks
	var rowTasks, verts []*task
	for _, t := range tasks {
		if !t.vert {
			rowTasks = append(rowTasks, t)
		} else if scheduled(t) {
			verts = append(verts, t)
		}
	}

	var categories omap.Map[string, int]
	for _, t := range rowTasks {
		if _, ok := categories.Get(t.section); !ok {
			categories.Set(t.section, categories.Len())
		}
	}

	rowOf := map[*task]int{}
	var rowSection []string
	if compact {
		var groups omap.Map[string, []*task]
		for _, t := range rowTasks {
			group, _ := groups.Get(t.section)
			groups.Set(t.section, append(group, t))
		}
		for section, group := range groups.All() {
			rows := packRows(group, len(rowSection), rowOf)
			for range rows {
				rowSection = append(rowSection, section)
			}
		}
	} else {
		for _, t := range rowTasks {
			rowOf[t] = len(rowSection)
			rowSection = append(rowSection, t.section)
		}
	}
	rowCount := len(rowSection)

	start, stop := math.Inf(1), math.Inf(-1)
	for _, t := range tasks {
		if !scheduled(t) {
			continue
		}
		start = math.Min(start, t.startTime.Time())
		stop = math.Max(stop, t.endTime.Time())
	}
	hasPlot := start <= stop

	title := text.Layout(model.title, false, measurer, size, 4000, text.Bold, rc.Icons)
	titleHeight := 0.0
	if title.Height > 0 {
		titleHeight = title.Height + 16
	}
	doc := func(width, height float64, content string) string {
		return svg.Document("gantt", width, height, size, svg.DocumentOptions{
			IDPrefix:       options.IDPrefix,
			Responsive:     rc.Responsive,
			FontFamily:     options.FontFamily,
			FontFamilyMono: options.FontFamilyMono,
		}, svg.Accessible{AccTitle: model.accTitle, AccDescr: model.accDescr}, content)
	}
	titleAttrs := ` class="pele-title" font-weight="var(--_tw)"`

	if !hasPlot {
		width := math.Ceil(math.Max(title.Width, 1) + 2*pad)
		height := math.Ceil(math.Max(title.Height, 1) + 2*pad)
		content := svg.LabelSVG(title, width/2, pad+float64(title.Height/2), titleAttrs, nil)
		return core.Rendered{SVG: doc(width, height, content), Width: width, Height: height, Links: []core.Link{}}
	}

	var sectionLabels omap.Map[string, text.Label]
	sectionWidth := 0.0
	for section := range categories.All() {
		label := text.Layout(section, false, measurer, sectionSize, 180, 0, rc.Icons)
		sectionLabels.Set(section, label)
		sectionWidth = math.Max(sectionWidth, label.Width)
	}

	axisFormat := model.axisFormat
	if axisFormat == "" {
		if model.dateFormat == "D" {
			axisFormat = "%d"
		} else if s, ok := configText(config, "axisFormat"); ok {
			axisFormat = s
		} else {
			axisFormat = "%Y-%m-%d"
		}
	}
	tickLabel := func(t float64) string { return timeFormat(axisFormat, jsdate.New(t, loc)) }
	sampleWidth := math.Max(measurer.Width(tickLabel(start), tickSize, 0), measurer.Width(tickLabel(stop), tickSize, 0))

	leftPadding, ok := setting(config, "leftPadding", 0, 2000)
	if !ok {
		leftPadding = math.Max(0, sampleWidth/2)
		if sectionWidth > 0 {
			leftPadding = math.Max(sectionWidth+24, sampleWidth/2)
		}
	}
	left := pad + leftPadding
	rightPadding := settingOr(config, "rightPadding", 0, 2000, math.Max(16, sampleWidth/2))
	target := common.CapWidth(options, settingOr(config, "useWidth", 100, 20000, chartWidth), 280, 0)
	plotWidth := math.Max(minPlot, target-left-rightPadding-pad)
	right := left + plotWidth
	scale := 0.0
	if stop > start {
		scale = plotWidth / (stop - start)
	}
	x := func(t float64) float64 {
		if scale == 0 {
			return left + float64(plotWidth/2)
		}
		return left + float64((t-start)*scale)
	}

	tickHeight := jsmath.Round(tickSize * 1.75)
	topPadding, hasTopPadding := setting(config, "topPadding", 0, 2000)
	gridStart, hasGridStart := setting(config, "gridLineStartPadding", 0, 2000)
	headHeight := pad + titleHeight
	if topAxis {
		headHeight += tickHeight
	}
	plotTop := headHeight
	if hasTopPadding {
		plotTop = math.Max(headHeight, topPadding)
	} else {
		plotTop = math.Max(headHeight, 0)
	}
	// Mermaid starts its grid lines above the first row by the difference between these two
	// settings. Here that only uses room a larger topPadding leaves free.
	gridLead := 0.0
	if hasTopPadding && hasGridStart {
		gridLead = math.Max(0, math.Min(topPadding-gridStart, plotTop-headHeight))
	}
	plotHeight := float64(float64(max(rowCount, 1)) * rowHeight)
	plotBottom := plotTop + plotHeight

	// Bars and where their labels go: inside when they fit, otherwise beside the bar on the
	// side with room.
	var bars []bar
	extent := right + rightPadding
	for _, t := range rowTasks {
		label := oneLine(t.task, measurer, labelSize, rc.Icons)
		row := rowOf[t]
		if !scheduled(t) {
			labelX := left + labelGap + float64(label.Width/2)
			extent = math.Max(extent, labelX+float64(label.Width/2))
			bars = append(bars, bar{task: t, label: label, row: row, x0: math.NaN(), x1: math.NaN(), labelX: labelX})
			continue
		}
		x0 := x(t.startTime.Time())
		end := t.endTime
		if t.renderEndTime != nil {
			end = t.renderEndTime
		}
		minWidth := 2.0
		if t.milestone {
			minWidth = 0
		}
		x1 := math.Max(x(end.Time()), x0+minWidth)
		if t.milestone {
			mid := x0 + float64((x(t.endTime.Time())-x0)/2)
			x0 = mid - float64(barHeight/2)
			x1 = mid + float64(barHeight/2)
		}
		inside := !t.milestone && label.Width+2*labelGap <= x1-x0
		labelX := (x0 + x1) / 2
		if !inside {
			fitsRight := x1+labelGap+label.Width <= right+rightPadding
			fitsLeft := x0-labelGap-label.Width >= left
			if fitsRight || !fitsLeft {
				labelX = x1 + labelGap + float64(label.Width/2)
			} else {
				labelX = x0 - labelGap - float64(label.Width/2)
			}
			extent = math.Max(extent, labelX+float64(label.Width/2))
		}
		bars = append(bars, bar{task: t, label: label, row: row, x0: x0, x1: x1, labelX: labelX, inside: inside})
	}

	interval := ""
	if model.tickInterval != nil && *model.tickInterval != "" {
		interval = *model.tickInterval
	} else if s, ok := configText(config, "tickInterval"); ok && s != "" {
		interval = s
	}
	weekday := "sunday"
	if model.weekday != nil {
		weekday = *model.weekday
	} else if s, ok := configText(config, "weekday"); ok {
		weekday = s
	}
	count := math.Max(2, math.Min(10, math.Floor(plotWidth/(sampleWidth+24))))
	tickTimes, ok := intervalTicks(interval, weekday, start, stop, math.Ceil(plotWidth/3), loc)
	if !ok {
		tickTimes = autoTicks(start, stop, count, loc)
	}

	var grid, axisBottom, axisTop strings.Builder
	lastRight := math.Inf(-1)
	lastLabel := ""
	bottomY := plotBottom + float64(tickHeight/2) + 2
	topY := plotTop - gridLead - float64(tickHeight/2) - 2
	for _, t := range tickTimes {
		tx := x(t)
		grid.WriteString("M" + svg.Num(tx) + "," + svg.Num(plotTop-gridLead) + "V" + svg.Num(plotBottom))
		label := tickLabel(t)
		width := measurer.Width(label, tickSize, 0)
		// A label is skipped when it would run into the one before it or only repeat it; its
		// grid line stays.
		if tx-float64(width/2) < lastRight+8 || label == lastLabel {
			continue
		}
		lastRight = tx + float64(width/2)
		lastLabel = label
		extent = math.Max(extent, lastRight)
		body := `text-anchor="middle">` + svg.Esc(label) + `</text>`
		axisBottom.WriteString(`<text x="` + svg.Num(tx) + `" y="` + svg.Num(bottomY+float64(tickSize*0.35)) + `" ` + body)
		if topAxis {
			axisTop.WriteString(`<text x="` + svg.Num(tx) + `" y="` + svg.Num(topY+float64(tickSize*0.35)) + `" ` + body)
		}
	}

	// Vertical markers take no row. Their labels go under the axis, on as many lines as it
	// takes to keep them apart.
	var vertLines, vertLabels strings.Builder
	var vertEnds []float64
	vertTop := plotBottom + tickHeight + 4
	vertLine := jsmath.Round(labelSize * 1.5)
	sortedVerts := slices.Clone(verts)
	slices.SortStableFunc(sortedVerts, func(a, b *task) int { return sign(a.startTime.Time() - b.startTime.Time()) })
	for _, t := range sortedVerts {
		vx := x(t.startTime.Time())
		label := oneLine(t.task, measurer, labelSize, rc.Icons)
		cx := math.Max(pad+float64(label.Width/2), vx)
		line := 0
		for line < len(vertEnds) && cx-float64(label.Width/2) < vertEnds[line]+8 {
			line++
		}
		if line == len(vertEnds) {
			vertEnds = append(vertEnds, 0)
		}
		vertEnds[line] = cx + float64(label.Width/2)
		extent = math.Max(extent, vertEnds[line])
		id := svg.EscText(t.id)
		vertLines.WriteString(`<path class="pele-vert" data-id="` + id + `" d="M` + svg.Num(vx) + "," + svg.Num(plotTop-gridLead) +
			"V" + svg.Num(plotBottom) + `"/>`)
		vertLabels.WriteString(svg.LabelSVG(label, cx, vertTop+float64(float64(line)*vertLine)+float64(vertLine/2),
			` class="pele-vert-label" data-id="`+id+`"`, nil))
	}

	width := math.Ceil(math.Max(extent, pad+title.Width) + pad)
	vertHeight := 0.0
	if len(vertEnds) > 0 {
		vertHeight = 4 + float64(float64(len(vertEnds))*vertLine)
	}
	height := math.Ceil(plotBottom + tickHeight + vertHeight + pad)

	// Section bands, one per run of rows that share a section, shaded on alternate sections.
	var bands, sectionOut strings.Builder
	for row := 0; row < rowCount; {
		section := rowSection[row]
		end := row + 1
		for end < rowCount && rowSection[end] == section {
			end++
		}
		category, _ := categories.Get(section)
		index := math.Mod(float64(category), sectionStyles)
		y := plotTop + float64(float64(row)*rowHeight)
		h := float64(end-row) * rowHeight
		if categories.Len() > 1 || section != "" {
			fill := "none"
			if math.Mod(index, 2) == 0 {
				fill = "var(--_s)"
			}
			bands.WriteString(`<rect class="pele-section pele-section-` + num(index) + `" x="0" y="` + svg.Num(y) + `" width="` +
				num(width) + `" height="` + svg.Num(h) + `" fill="` + fill + `"/>`)
		}
		label, _ := sectionLabels.Get(section)
		sectionOut.WriteString(svg.LabelSVG(label, pad+8+float64(label.Width/2), y+float64(h/2),
			` class="pele-section-label" data-id="`+svg.EscText(section)+`"`, rc.Icons))
		row = end
	}

	// Excluded days, as Mermaid marks them: a day at a time from the first task's start, and
	// not at all when the chart spans more than five years.
	var excluded strings.Builder
	if (len(model.excludes) > 0 || len(model.includes) > 0) && add(jsdate.New(start, loc), 6, "y").Time() > stop {
		var from, to *jsdate.Date
		flush := func() {
			if from == nil || to == nil {
				return
			}
			dayStart := *from
			dayStart.SetHours(0, 0, 0, 0)
			dayEnd := *to
			dayEnd.SetHours(23, 59, 59, 999)
			ex0 := math.Max(left, x(dayStart.Time()))
			ex1 := math.Min(right, x(dayEnd.Time()))
			if ex1 > ex0 {
				excluded.WriteString(`<rect x="` + svg.Num(ex0) + `" y="` + svg.Num(plotTop) + `" width="` + svg.Num(ex1-ex0) +
					`" height="` + svg.Num(plotHeight) + `"/>`)
			}
			from, to = nil, nil
		}
		for d := jsdate.New(start, loc); d.Time() <= stop; d = addDays(d, 1) {
			if sched.isInvalidDate(d) {
				day := d
				if from == nil {
					from = &day
				}
				to = &day
			} else {
				flush()
			}
		}
		flush()
	}

	links := []core.Link{}
	var tasksOut strings.Builder
	for _, b := range bars {
		t := b.task
		cy := plotTop + float64(float64(b.row)*rowHeight) + float64(rowHeight/2)
		state := ""
		if t.active {
			state = "active"
		} else if t.done {
			state = "done"
		}
		classes := "pele-task"
		if state != "" {
			classes += " pele-" + state
		}
		if t.crit {
			classes += " pele-crit"
		}
		if t.milestone {
			classes += " pele-milestone"
		}
		if slices.Contains(t.classes, "clickable") {
			classes += " pele-clickable"
		}

		shape := ""
		textFill := ""
		if math.IsNaN(b.x0) {
			classes += " pele-unscheduled"
			textFill = ` fill="var(--_m)"`
		} else {
			// A critical task takes the critical color in place of the accent. Done, it keeps
			// the fill of finished work and only its border is in that color.
			color := "var(--_c)"
			if t.crit {
				color = svg.Critical
			}
			paint := `fill="` + color + `"`
			switch state {
			case "done":
				paint = `fill="var(--_a)"`
			case "active":
				paint = `fill="` + color + `" fill-opacity="0.25"`
			}
			strokeColor := color
			if state == "done" && !t.crit {
				strokeColor = "var(--_b)"
			}
			stroke := `stroke="` + strokeColor + `"`
			if t.milestone {
				cx := float64((b.x0 + b.x1) / 2)
				r := float64(barHeight / 2)
				shape = `<path d="M` + svg.Num(cx) + "," + svg.Num(cy-r) + "L" + svg.Num(cx+r) + "," + svg.Num(cy) + "L" + svg.Num(cx) +
					"," + svg.Num(cy+r) + "L" + svg.Num(cx-r) + "," + svg.Num(cy) + `Z" ` + paint + " " + stroke + ` stroke-linejoin="round"/>`
			} else {
				shape = `<rect x="` + svg.Num(b.x0) + `" y="` + svg.Num(cy-float64(barHeight/2)) + `" width="` + svg.Num(b.x1-b.x0) +
					`" height="` + svg.Num(barHeight) + `" ` + paint + " " + stroke + svg.RoundedStyle + `/>`
			}
			if b.inside {
				if state == "" {
					textFill = ` fill="var(--_bg)"`
				}
			} else if state == "done" {
				textFill = ` fill="var(--_m)"`
			}
		}

		body := shape + svg.LabelSVG(b.label, b.labelX, cy, ` class="pele-label"`+textFill, nil)
		if url, ok := model.links[t.id]; ok {
			body = util.Linked(body, url, t.id, &links, options, "")
		}
		tasksOut.WriteString(`<g class="` + classes + `" data-id="` + svg.EscText(t.id) + `">` + body + `</g>`)
	}

	today := ""
	if model.todayMarker != "off" && nowMs >= start && nowMs <= stop && stop > start {
		tx := x(nowMs)
		top := plotTop - gridLead
		style := ""
		if model.todayMarker != "" {
			style = svg.ResolveStyle(strings.Split(model.todayMarker, ",")).Line
		}
		today = `<g class="pele-today"><path d="M` + svg.Num(tx) + "," + svg.Num(top) + "V" + svg.Num(plotBottom) +
			`" fill="none" stroke="var(--_c)" stroke-width="2"` + style + `/>`
		if style == "" {
			today += `<path d="M` + svg.Num(tx-4) + "," + svg.Num(top-5) + "H" + svg.Num(tx+4) + "L" + svg.Num(tx) + "," +
				svg.Num(top+1) + `Z" fill="var(--_c)"/>`
		}
		today += "</g>"
	}

	axisLine := "M" + svg.Num(left) + "," + svg.Num(plotBottom) + "H" + svg.Num(right)
	if topAxis {
		axisLine += "M" + svg.Num(left) + "," + svg.Num(plotTop-gridLead) + "H" + svg.Num(right)
	}
	var content strings.Builder
	content.WriteString(svg.LabelSVG(title, width/2, pad+float64(title.Height/2), titleAttrs, nil))
	if bands.Len() > 0 {
		content.WriteString(`<g class="pele-sections">` + bands.String() + `</g>`)
	}
	if excluded.Len() > 0 {
		content.WriteString(`<g class="pele-excluded" fill="var(--_a)">` + excluded.String() + `</g>`)
	}
	content.WriteString(`<g class="pele-grid" fill="none"><path d="` + grid.String() + `" stroke="var(--_b)" stroke-dasharray="1 3"/><path d="` +
		axisLine + `" stroke="var(--_b)"/></g>`)
	if vertLines.Len() > 0 {
		content.WriteString(`<g class="pele-verts" fill="none" stroke="var(--_l)" stroke-width="1.5" stroke-dasharray="4 3">` +
			vertLines.String() + `</g>`)
	}
	content.WriteString(today)
	content.WriteString(`<g class="pele-tasks" font-size="` + svg.Num(labelSize) + `">` + tasksOut.String() + `</g>`)
	content.WriteString(`<g class="pele-section-labels" font-size="` + svg.Num(sectionSize) + `">` + sectionOut.String() + `</g>`)
	content.WriteString(`<g class="pele-axis" font-size="` + num(tickSize) + `" fill="var(--_m)">` + axisBottom.String() + axisTop.String() + `</g>`)
	if vertLabels.Len() > 0 {
		content.WriteString(`<g class="pele-vert-labels" font-size="` + svg.Num(labelSize) + `">` + vertLabels.String() + `</g>`)
	}

	return core.Rendered{SVG: doc(width, height, content.String()), Width: width, Height: height, Links: links}
}
