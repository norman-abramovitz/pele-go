package gantt

import (
	"github.com/Broderick-Westrope/pele-go/internal/diagrams/common"
	"github.com/Broderick-Westrope/pele-go/internal/jsstr"
	"github.com/Broderick-Westrope/pele-go/internal/perr"
)

const lineTokens = "'EOF', 'SPACE', 'NL', 'weekday_monday', 'weekday_tuesday', 'weekday_wednesday', 'weekday_thursday', " +
	"'weekday_friday', 'weekday_saturday', 'weekday_sunday', 'weekend_friday', 'weekend_saturday', 'dateFormat', " +
	"'inclusiveEndDates', 'topAxis', 'axisFormat', 'tickInterval', 'excludes', 'includes', 'todayMarker', 'title', " +
	"'acc_title', 'acc_descr', 'acc_descr_multiline_value', 'section', 'taskTxt', 'click'"

// startsLine holds the tokens that can start a line, which are also the only ones that can
// follow a complete statement.
var startsLine = func() [tEOF + 1]bool {
	var s [tEOF + 1]bool
	for t := tWeekdayMonday; t <= tWeekendSaturday; t++ {
		s[t] = true
	}
	for _, t := range []int{tEOF, tNL, tDateFormat, tInclusiveEndDates, tTopAxis, tAxisFormat, tTickInterval, tExcludes,
		tIncludes, tTodayMarker, tTitle, tAccTitle, tAccDescr, tAccDescrMultilineValue, tSection, tTaskTxt, tClick} {
		s[t] = true
	}
	return s
}()

func parseGantt(source string, db *ganttDb, o *perr.Origin) {
	src := common.Src(jsstr.UTF16(source))
	toks := tokenize(src)
	i := 0
	text := func(k int) string { return jsstr.FromUTF16(src[toks.starts[k]:toks.ends[k]]) }
	// after is a token's text without its first n code units, as upstream's text.slice(n).
	after := func(k, n int) string { return jsstr.FromUTF16(src[min(toks.starts[k]+n, toks.ends[k]):toks.ends[k]]) }

	fail := func(expected string) {
		t := toks.types[i]
		got := "1"
		if t != tEND {
			got = tokenNames[t]
		}
		perr.Throw(perr.SyntaxError(o, "gantt", src, toks.starts[i], "Expecting "+expected+", got '"+got+"'", false))
	}
	// Mermaid's parser runs a statement's action only after seeing that the next token can
	// follow it.
	end := func(also string) {
		if !startsLine[toks.types[i]] {
			fail(lineTokens + also)
		}
	}

	if toks.types[i] != tGantt {
		fail("'gantt'")
	}
	i++

	for {
		t := toks.types[i]
		k := i
		if !startsLine[t] {
			fail(lineTokens)
		}
		i++
		switch t {
		case tEOF:
			if toks.types[i] == tEND {
				return
			}
			end("")
		case tNL:
		case tDateFormat:
			end("")
			db.setDateFormat(after(k, 11))
		case tInclusiveEndDates:
			end("")
			db.inclusiveEndDates = true
		case tTopAxis:
			end("")
			db.topAxis = true
		case tAxisFormat:
			end("")
			db.axisFormat = after(k, 11)
		case tTickInterval:
			end("")
			db.tickInterval = new(after(k, 13))
		case tExcludes:
			end("")
			db.excludes = mergeTokens(db.excludes, after(k, 9))
		case tIncludes:
			end("")
			db.includes = mergeTokens(db.includes, after(k, 9))
		case tTodayMarker:
			end("")
			db.todayMarker = after(k, 12)
		case tWeekendFriday:
			end("")
			db.weekend = "friday"
		case tWeekendSaturday:
			end("")
			db.weekend = "saturday"
		case tTitle:
			end("")
			db.title = after(k, 6)
		case tAccTitle:
			if toks.types[i] != tAccTitleValue {
				fail("'acc_title_value'")
			}
			value := text(i)
			i++
			end("")
			db.setAccTitle(jsstr.Trim(value))
		case tAccDescr:
			if toks.types[i] != tAccDescrValue {
				fail("'acc_descr_value'")
			}
			value := text(i)
			i++
			end("")
			db.setAccDescription(jsstr.Trim(value))
		case tAccDescrMultilineValue:
			end("")
			db.setAccDescription(jsstr.Trim(text(k)))
		case tSection:
			end("")
			db.addSection(after(k, 8))
		case tTaskTxt:
			if toks.types[i] != tTaskData {
				fail("'taskData'")
			}
			data := text(i)
			i++
			end("")
			db.addTask(text(k), data)
		case tClick:
			var name, href *string
			hasArgs := false
			switch toks.types[i] {
			case tCallbackname:
				name = new(text(i))
				i++
				if toks.types[i] == tCallbackargs {
					hasArgs = true
					i++
				}
				if toks.types[i] == tHref {
					href = new(text(i))
					i++
					end("")
				} else if hasArgs {
					end(", 'href'")
				} else {
					end(", 'callbackargs', 'href'")
				}
			case tHref:
				href = new(text(i))
				i++
				if toks.types[i] == tCallbackname {
					name = new(text(i))
					i++
					if toks.types[i] == tCallbackargs {
						i++
						end("")
					} else {
						end(", 'callbackargs'")
					}
				} else {
					end(", 'callbackname'")
				}
			default:
				fail("'callbackname', 'href'")
			}
			// Callbacks are recognized and never run; the task is only marked clickable, as in
			// Mermaid.
			if name != nil {
				db.setClass(text(k), "clickable")
			}
			if href != nil {
				db.setLink(text(k), *href)
			}
		default:
			end("")
			db.weekday = new(days[t-tWeekdayMonday])
		}
	}
}
