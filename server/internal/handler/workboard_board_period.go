package handler

import (
	"net/url"
	"strings"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/service"
)

// 주·월 요약·칸반 임시안(지시서 §5-B). 정의를 바꿀 때는 이 파일만 고친다.

const (
	boardViewSummary = "summary"

	summaryKindAdm   = "adm"
	summaryKindAS    = "as"
	summaryKindMnt   = "mnt"
	summaryKindWork  = "work"
	summaryKindSales = "sales"
)

var boardSummaryKindOrder = []string{summaryKindAdm, summaryKindAS, summaryKindMnt, summaryKindWork, summaryKindSales}

var boardSummaryKindLabel = map[string]string{
	summaryKindAdm:   "행정/지원",
	summaryKindAS:    "AS",
	summaryKindMnt:   "정기점검",
	summaryKindWork:  "작업",
	summaryKindSales: "영업",
}

type boardSummaryCard struct {
	Key     string
	Title   string
	IsToday bool
	Rows    []boardSummaryRow
}

type boardSummaryRow struct {
	Key   string
	Label string
	N     int
	Done  int
	Total int
}

type boardSummary struct {
	Period      bool
	PrevLabel   string
	WorkLabel   string
	PrevWorkday string
	Cards       []boardSummaryCard
}

func boardPrevWorkday(day string) string {
	t, err := time.ParseInLocation(dateLayout, strings.TrimSpace(day), time.Local)
	if err != nil {
		t = time.Now()
	}
	got, _ := service.PrevWorkday(t, nil, "", false)
	return got
}

func boardSummaryKindOf(t model.WorkTask) string {
	key := strings.TrimSpace(t.WorkType)
	if key == "" {
		key = strings.TrimSpace(t.SourceType)
	}
	switch key {
	case model.WBWorkAS:
		return summaryKindAS
	case model.WBWorkMaintenance:
		return summaryKindMnt
	case model.WBWorkProjectWork:
		return summaryKindWork
	case model.WBWorkSales, model.WBSourceSalesActivity:
		return summaryKindSales
	case model.WBWorkAdmin, model.WBWorkSupport:
		return summaryKindAdm
	default:
		return summaryKindAdm
	}
}

func boardItemDelayState(it boardItem, today string) model.TaskDelay {
	t := it.Task
	if strings.TrimSpace(t.DueDate) == "" {
		switch strings.TrimSpace(t.WorkType) {
		case model.WBWorkAS, model.WBWorkMaintenance:
			t.DueDate = t.WorkDate
		}
	}
	return model.TaskDelayState(t, today)
}

func skipBoardSummaryItem(t model.WorkTask) bool {
	if t.RecurrenceRole == model.RecurrenceRoleParent {
		return true
	}
	if strings.TrimSpace(t.Status) == model.WBTaskCancelled {
		return true
	}
	return false
}

func itemOnDay(it boardItem, day string) bool {
	day = model.NormalizeAppDate(day)
	if day == "" {
		return false
	}
	if model.NormalizeAppDate(it.PlaceDate) == day {
		return true
	}
	if model.NormalizeAppDate(it.Task.WorkDate) == day || model.NormalizeAppDate(it.Task.DueDate) == day {
		return true
	}
	if model.NormalizeAppDate(it.Task.CompleteDate) == day {
		return true
	}
	return model.TaskSpansDate(it.Task.WorkDate, it.Task.DueDate, day)
}

func itemInPeriod(it boardItem, from, toIncl string) bool {
	from = model.NormalizeAppDate(from)
	toIncl = model.NormalizeAppDate(toIncl)
	if from == "" || toIncl == "" {
		return false
	}
	d := model.NormalizeAppDate(it.PlaceDate)
	if d == "" {
		d = model.NormalizeAppDate(it.Task.WorkDate)
	}
	if d == "" {
		d = model.NormalizeAppDate(it.Task.DueDate)
	}
	if d == "" {
		d = model.NormalizeAppDate(it.Task.CompleteDate)
	}
	if d != "" && d >= from && d <= toIncl {
		return true
	}
	cur, err := time.ParseInLocation(dateLayout, from, time.Local)
	end, err2 := time.ParseInLocation(dateLayout, toIncl, time.Local)
	if err != nil || err2 != nil {
		return false
	}
	for !cur.After(end) {
		if model.TaskSpansDate(it.Task.WorkDate, it.Task.DueDate, cur.Format(dateLayout)) {
			return true
		}
		cur = cur.AddDate(0, 0, 1)
	}
	return false
}

func inProgressForSummary(it boardItem, today string) bool {
	st := strings.TrimSpace(it.Task.Status)
	if st == model.WBTaskInProgress || st == model.WBTaskHold || st == model.WBTaskTransfer || st == model.WBTaskReview || st == model.StatusPartialComplete || st == model.WBTaskWaitingFor {
		return true
	}
	w := model.NormalizeAppDate(it.Task.WorkDate)
	due := model.NormalizeAppDate(it.Task.DueDate)
	if w != "" && today != "" && w < today && due != "" && due >= today {
		return true
	}
	return false
}

func emptySummaryRows() []boardSummaryRow {
	out := make([]boardSummaryRow, 0, len(boardSummaryKindOrder))
	for _, k := range boardSummaryKindOrder {
		out = append(out, boardSummaryRow{Key: k, Label: boardSummaryKindLabel[k]})
	}
	return out
}

func bumpRow(rows []boardSummaryRow, kind string, done bool) {
	for i := range rows {
		if rows[i].Key != kind {
			continue
		}
		rows[i].N++
		rows[i].Total++
		if done {
			rows[i].Done++
		}
		return
	}
}

func buildBoardSummary(items []boardItem, view, from, to, today, prev string) boardSummary {
	period := view == boardViewWeek || view == boardViewMonth
	doneTitle, workTitle := "전일 완료", "오늘 업무"
	if period {
		doneTitle, workTitle = "기간 완료", "기간 업무"
	}
	cards := []boardSummaryCard{
		{Key: "done", Title: doneTitle, Rows: emptySummaryRows()},
		{Key: "prog", Title: "진행중", Rows: emptySummaryRows()},
		{Key: "today", Title: workTitle, IsToday: true, Rows: emptySummaryRows()},
		{Key: "delay", Title: "지연 업무", Rows: emptySummaryRows()},
	}
	seen := map[string]bool{}
	for _, it := range items {
		if skipBoardSummaryItem(it.Task) {
			continue
		}
		key := it.DedupKey
		if key == "" {
			key = taskDedupKey(it.Task)
		}
		if seen[key] {
			continue
		}
		kind := boardSummaryKindOf(it.Task)
		done := boardItemDone(it)
		completeDay := model.NormalizeAppDate(it.Task.CompleteDate)
		ds := boardItemDelayState(it, today)
		onToday := itemOnDay(it, today)
		if view == boardViewDay || view == boardViewSummary {
			onToday = itemOnDay(it, from)
		}
		inWin := true
		if period {
			inWin = itemInPeriod(it, from, to)
		}

		switch {
		case done && !period && completeDay == prev:
			seen[key] = true
			bumpRow(cards[0].Rows, kind, true)
		case done && period && completeDay >= from && completeDay <= to:
			seen[key] = true
			bumpRow(cards[0].Rows, kind, true)
		case !done && ds.Delayed:
			seen[key] = true
			bumpRow(cards[3].Rows, kind, false)
		case !done && onToday && !period:
			seen[key] = true
			bumpRow(cards[2].Rows, kind, false)
		case done && onToday && !period:
			seen[key] = true
			bumpRow(cards[2].Rows, kind, true)
		case period && !done && inWin && !ds.Delayed:
			seen[key] = true
			if inProgressForSummary(it, today) && !onPeriodScheduled(it, from, to) {
				bumpRow(cards[1].Rows, kind, false)
			} else {
				bumpRow(cards[2].Rows, kind, false)
			}
		case period && done && inWin:
			seen[key] = true
			bumpRow(cards[2].Rows, kind, true)
		case !done && inProgressForSummary(it, today):
			seen[key] = true
			bumpRow(cards[1].Rows, kind, false)
		}
	}
	return boardSummary{
		Period:      period,
		PrevLabel:   doneTitle,
		WorkLabel:   workTitle,
		PrevWorkday: prev,
		Cards:       cards,
	}
}

func onPeriodScheduled(it boardItem, from, to string) bool {
	d := model.NormalizeAppDate(it.PlaceDate)
	if d == "" {
		d = model.NormalizeAppDate(it.Task.WorkDate)
	}
	if d == "" {
		d = model.NormalizeAppDate(it.Task.DueDate)
	}
	return d >= from && d <= to
}

func uniqueBoardItems(items []boardItem) int {
	seen := map[string]bool{}
	n := 0
	for _, it := range items {
		if skipBoardSummaryItem(it.Task) {
			continue
		}
		key := it.DedupKey
		if key == "" {
			key = taskDedupKey(it.Task)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		n++
	}
	return n
}

func summaryCardTotal(c boardSummaryCard) int {
	n := 0
	for _, r := range c.Rows {
		if c.IsToday {
			n += r.Total
		} else {
			n += r.N
		}
	}
	return n
}

func boardKanbanMapDate(it model.WorkListItem, from, to, selected string) string {
	completeOn := model.NormalizeAppDate(it.CompleteDate)
	if completeOn != "" && completeOn >= from && completeOn <= to {
		return completeOn
	}
	sched := model.NormalizeAppDate(model.WorkItemScheduledDate(it))
	if sched != "" && sched >= from && sched <= to {
		return sched
	}
	if selected != "" {
		return selected
	}
	return from
}

func boardRangeView(period string) string {
	switch period {
	case boardRangeWeek:
		return boardViewWeek
	case boardRangeMonth:
		return boardViewMonth
	default:
		return boardViewDay
	}
}

func boardModeURL(display, period, date string, keep url.Values) string {
	k := cloneBoardQuery(keep)
	if period == boardRangeWeek || period == boardRangeMonth || display == boardDisplaySchedule {
		k.Set("range", period)
	} else {
		k.Del("range")
	}
	return boardURL(boardURLView(display, period), date, k)
}
