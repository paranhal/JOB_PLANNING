package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

const (
	boardViewList   = "list"
	boardViewMonth  = "month"
	boardViewWeek   = "week"
	boardViewDay    = "day"
	boardViewKanban = "kanban"
	boardRangeDay   = "day"
	boardRangeWeek  = "week"
	boardRangeMonth = "month"

	boardDayStartHour = 7
	boardDayEndHour   = 21
	boardDayPxPerHour = 64

	boardColsPerson = "person"
	boardColsFlat   = "flat"
)

type boardTypeStyle struct {
	Key    string
	Label  string
	Letter string
	Text   string
	Bg     string
}

type boardStatusView struct {
	Key   string
	Label string
	Class string
}

type boardItem struct {
	Task                 model.WorkTask
	Href                 string
	Type                 boardTypeStyle
	Status               boardStatusView
	CustomerLine         string
	PlaceLine            string
	DateLabel            string
	TimeLabel            string
	HasTime              bool
	StartMin             int
	EndMin               int
	AssigneeName         string
	AssigneeExtra        string
	AssigneeTitle        string
	AssigneeInitial      string
	AssigneeLine         string
	ChipTime             string
	DayPosTop            int
	DayPosHeight         int
	DayCol               int
	DayCols              int
	Unplaced             bool
	Overdue              bool
	OverdueDays          int
	DedupKey             string
	PlaceKind            string
	PlaceRef             string
	PlaceAssignee        string
	PlaceDur             int
	PlaceDue             string
	CanPlace             bool
	DueTag               string
	DueLate              bool
	CanDelete            bool
	PlanEditHref         string
	SegLabel             string
	SegIndex             int
	SegCount             int
	TravelAfter          bool
	Fragment             bool
	ReturnPending        bool
	DeleteKey            string
	DeleteDisabledReason string
	CustomerPlace        string
	ScheduleLine         string
	PlaceDate            string
	CanCarry             bool
	DelayReason          string
	DelayReasonFull      string
	ChipUntil            string
	SpanLabel            string
	ItemKindLine         string
	ItemProgress         string
	KanbanKey            string
	TodaySchedule        bool
	KanbanBadgeToday     bool
	RemainItemN          int
	FollowHref           string
	FollowLabel          string
	FollowMissing        bool
	NextWorkday          string
	CarrySkipNotice      string
	Members              []model.WorkTaskMember
	IsSupport            bool
	OverlapWarn          bool
	ExtraCount           int
	EditHref             string
	ActionHref           string
	Done                 bool
	CanMove              bool
	CanResize            bool
	CanUnschedule        bool
	StartHHMM            string
	EndHHMM              string
}

type boardMonthCell struct {
	Date     string
	Day      int
	InMonth  bool
	IsToday  bool
	Items    []boardItem
	MoreN    int
	MoreHref string
}

type boardWeekCol struct {
	Date    string
	Dow     string
	Day     int
	IsToday bool
	Items   []boardItem
}

func (h *WorkboardHandler) Board(c echo.Context) error {
	data, err := h.personalBoardData(c)
	if err != nil {
		return err
	}
	if msg := visibleQueryErr(c.QueryParam("err")); msg != "" {
		data["FlashErr"] = c.QueryParam("err")
		data["FlashErrMsg"] = msg
		if _, ok := data["FlashPopup"]; !ok {
			data["FlashPopup"] = msg
		}
	} else if msg := boardScheduleErrMsg(c.QueryParam("err")); msg != "" {
		data["FlashErr"] = c.QueryParam("err")
		data["FlashErrMsg"] = msg
		data["FlashPopup"] = msg
	} else if msg := boardKanbanErrMsg(c.QueryParam("err")); msg != "" {
		data["FlashErr"] = c.QueryParam("err")
		data["FlashErrMsg"] = msg
		data["FlashPopup"] = msg
	}
	view := data["View"].(string)
	tpl := "workboard/board_list.html"
	switch view {
	case boardViewMonth:
		tpl = "workboard/board_month.html"
	case boardViewWeek:
		tpl = "workboard/board_week.html"
	case boardViewDay:
		tpl = "workboard/board_day.html"
	case boardViewKanban:
		tpl = "workboard/board_kanban.html"
	}
	return c.Render(http.StatusOK, tpl, data)
}

func parseBoardView(raw string) string {
	switch strings.TrimSpace(raw) {
	case boardViewMonth, boardViewWeek, boardViewDay, boardViewList, boardViewKanban:
		return strings.TrimSpace(raw)
	default:
		return boardViewList
	}
}

func parseListRangeParam(raw string) string {
	switch strings.TrimSpace(raw) {
	case boardRangeWeek, boardRangeMonth:
		return strings.TrimSpace(raw)
	default:
		return boardRangeDay
	}
}

func parseBoardDate(raw string, today time.Time) time.Time {
	base, err := time.ParseInLocation(dateLayout, strings.TrimSpace(raw), time.Local)
	if err != nil {
		base = today
	}
	return time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, time.Local)
}

func boardRange(view string, base time.Time) (from, to time.Time) {
	switch view {
	case boardViewWeek:
		mon := base.AddDate(0, 0, -mondayOffset(base))
		return mon, mon.AddDate(0, 0, 6)
	case boardViewMonth:
		first := time.Date(base.Year(), base.Month(), 1, 0, 0, 0, 0, base.Location())
		start := first.AddDate(0, 0, -int(first.Weekday()))
		return start, start.AddDate(0, 0, 41)
	default:
		return base, base
	}
}

func boardPeriodShift(view string, base time.Time, dir int) time.Time {
	switch view {
	case boardViewWeek:
		return base.AddDate(0, 0, 7*dir)
	case boardViewMonth:
		return time.Date(base.Year(), base.Month(), 1, 0, 0, 0, 0, base.Location()).AddDate(0, dir, 0)
	default:
		return base.AddDate(0, 0, dir)
	}
}

func boardPeriodLabel(view string, base, from, to time.Time) string {
	weekdayKR := []string{"일", "월", "화", "수", "목", "금", "토"}
	switch view {
	case boardViewMonth:
		return fmt.Sprintf("%d년 %d월", base.Year(), int(base.Month()))
	case boardViewWeek:
		if from.Month() == to.Month() {
			return fmt.Sprintf("%d년 %d월 %d일 – %d일", from.Year(), int(from.Month()), from.Day(), to.Day())
		}
		return fmt.Sprintf("%d년 %d월 %d일 – %d월 %d일", from.Year(), int(from.Month()), from.Day(), int(to.Month()), to.Day())
	case boardViewDay:
		return fmt.Sprintf("%d년 %d월 %d일 (%s)", base.Year(), int(base.Month()), base.Day(), weekdayKR[int(base.Weekday())])
	default:
		return fmt.Sprintf("%04d. %02d. %02d", base.Year(), int(base.Month()), base.Day())
	}
}

func boardTypeStyleOf(t model.WorkTask) boardTypeStyle {
	key := strings.TrimSpace(t.WorkType)
	if key == "" {
		key = strings.TrimSpace(t.SourceType)
	}
	switch key {
	case model.WBWorkAS:
		return boardTypeStyle{Key: "as", Label: "AS 조치", Letter: "W", Text: "board-t-as", Bg: "board-b-as"}
	case model.WBWorkMaintenance:
		return boardTypeStyle{Key: "mnt", Label: "정기점검 방문", Letter: "C", Text: "board-t-mnt", Bg: "board-b-mnt"}
	case model.WBWorkProjectWork:
		return boardTypeStyle{Key: "work", Label: "작업 수행", Letter: "H", Text: "board-t-work", Bg: "board-b-work"}
	case model.WBWorkSales, model.WBSourceSalesActivity:
		return boardTypeStyle{Key: "sales", Label: "영업 활동", Letter: "T", Text: "board-t-sales", Bg: "board-b-sales"}
	case model.WBWorkAdmin, model.WBWorkSupport:
		return boardTypeStyle{Key: "adm", Label: "행정/지원 업무", Letter: "D", Text: "board-t-adm", Bg: "board-b-adm"}
	default:
		return boardTypeStyle{Key: "etc", Label: "기타 업무", Letter: "E", Text: "board-t-adm", Bg: "board-b-adm"}
	}
}

func boardStatus(t model.WorkTask, today string) boardStatusView {
	st := strings.TrimSpace(t.Status)
	if st == model.WBTaskComplete {
		return boardStatusView{Key: "done", Label: "완료", Class: "board-st-done"}
	}
	ds := model.TaskDelayState(t, today)
	if ds.Delayed {
		return boardStatusView{Key: "late", Label: "지연", Class: "board-st-late"}
	}
	if st == model.WBTaskInProgress || st == model.WBTaskHold || st == model.WBTaskTransfer || st == model.WBTaskReview || st == model.StatusPartialComplete {
		return boardStatusView{Key: "prog", Label: "진행중", Class: "board-st-prog"}
	}
	w := strings.TrimSpace(t.WorkDate)
	due := strings.TrimSpace(t.DueDate)
	if w != "" && today != "" && w < today && due != "" && due >= today {
		return boardStatusView{Key: "prog", Label: "진행중", Class: "board-st-prog"}
	}
	return boardStatusView{Key: "plan", Label: "예정", Class: "board-st-plan"}
}

func taskAssignedToUser(t model.WorkTask, mems []model.WorkTaskMember, userID, displayName string) bool {
	userID = strings.TrimSpace(userID)
	displayName = strings.TrimSpace(displayName)
	hasID := strings.TrimSpace(t.AssigneeUserID) != ""
	if userID != "" && strings.TrimSpace(t.AssigneeUserID) == userID {
		return true
	}
	for _, m := range mems {
		if strings.TrimSpace(m.UserID) != "" {
			hasID = true
		}
		if userID != "" && strings.TrimSpace(m.UserID) == userID {
			return true
		}
	}
	if hasID {
		return false
	}
	if displayName == "" {
		return false
	}
	if strings.TrimSpace(t.Assignee) == displayName {
		return true
	}
	for _, m := range mems {
		if strings.TrimSpace(m.Assignee) == displayName {
			return true
		}
	}
	return false
}

func filterTasksForPersonalAssignee(tasks []model.WorkTask, members map[string][]model.WorkTaskMember, userID, displayName string) []model.WorkTask {
	var out []model.WorkTask
	for _, t := range tasks {
		if taskAssignedToUser(t, members[t.TaskID], userID, displayName) {
			out = append(out, t)
		}
	}
	return out
}

func boardSearchHit(it boardItem, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	blob := strings.ToLower(strings.Join([]string{
		it.Task.Title, it.Task.CustomerName, it.Task.OrgName, it.Task.SiteLine, it.Task.ProjectName,
		it.CustomerLine, it.PlaceLine,
	}, " "))
	return strings.Contains(blob, q)
}

func parseClockMin(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 4 {
		return 0, false
	}
	t, err := time.Parse("15:04", s[:5])
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

func formatDotDate(ymd string) string {
	t, err := time.ParseInLocation(dateLayout, ymd, time.Local)
	if err != nil {
		return ymd
	}
	return fmt.Sprintf("%04d. %02d. %02d", t.Year(), int(t.Month()), t.Day())
}

func firstRune(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "?"
	}
	r, _ := utf8.DecodeRuneInString(s)
	return string(r)
}

func isBoardDisplayName(n string) bool {
	n = strings.TrimSpace(n)
	if n == "" {
		return false
	}
	return !strings.HasPrefix(strings.ToUpper(n), "USR-")
}

func boardAssigneeFields(primary string, names []string) (name, extra, title, initial, line string) {
	var shown []string
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if !isBoardDisplayName(n) || seen[n] {
			continue
		}
		seen[n] = true
		shown = append(shown, n)
	}
	primary = strings.TrimSpace(primary)
	if !isBoardDisplayName(primary) {
		primary = ""
	}
	if primary == "" && len(shown) > 0 {
		primary = shown[0]
	}
	if primary == "" {
		return "미배정", "", "미배정", "?", "미배정"
	}
	if len(shown) > 1 {
		extra = fmt.Sprintf(" 외 %d명", len(shown)-1)
	}
	title = strings.Join(shown, ", ")
	if title == "" {
		title = primary
	}
	return primary, extra, title, firstRune(primary), primary + extra
}

func buildBoardItem(t model.WorkTask, mems []model.WorkTaskMember, addr string, today string) boardItem {
	names := []string{}
	seen := map[string]bool{}
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		names = append(names, n)
	}
	add(t.Assignee)
	for _, m := range mems {
		add(m.Assignee)
	}
	name, extra, title, initial, line := boardAssigneeFields(t.Assignee, names)
	cust := strings.TrimSpace(t.SiteLine)
	if cust == "" {
		cust = strings.TrimSpace(t.OrgName)
	}
	if cust == "" {
		cust = strings.TrimSpace(t.CustomerName)
	}
	place := strings.TrimSpace(addr)
	if place == "" {
		place = strings.TrimSpace(t.SiteLine)
	}
	if place == "" {
		place = strings.TrimSpace(t.ProjectName)
	}
	if place == cust {
		place = ""
	}
	day := strings.TrimSpace(t.WorkDate)
	if day == "" {
		day = strings.TrimSpace(t.DueDate)
	}
	start, hasStart := parseClockMin(t.StartTime)
	end, hasEnd := parseClockMin(t.EndTime)
	hasTime := hasStart
	if hasTime && !hasEnd {
		end = start + 60
		if end <= start {
			end = start + 30
		}
	}
	timeLabel := ""
	chipTime := ""
	if hasTime {
		if hasEnd {
			timeLabel = t.StartTime[:5] + " – " + t.EndTime[:5]
		} else {
			timeLabel = t.StartTime[:5]
		}
		chipTime = t.StartTime[:5]
	}
	it := boardItem{
		Task:            t,
		Href:            t.DetailHref(),
		Type:            boardTypeStyleOf(t),
		Status:          boardStatus(t, today),
		CustomerLine:    cust,
		PlaceLine:       place,
		DateLabel:       formatDotDate(day),
		TimeLabel:       timeLabel,
		HasTime:         hasTime,
		StartMin:        start,
		EndMin:          end,
		AssigneeName:    name,
		AssigneeExtra:   extra,
		AssigneeTitle:   title,
		AssigneeInitial: initial,
		AssigneeLine:    line,
		ChipTime:        chipTime,
		CustomerPlace:   customerPlaceLine(cust, place),
		ScheduleLine:    boardOneLineSchedule(day, timeLabel),
	}
	if ds := model.TaskDelayState(t, today); ds.Delayed {
		it.Overdue = true
		it.OverdueDays = ds.Days
	}
	if model.TaskIsSpan(t.WorkDate, t.DueDate) {
		it.ChipUntil = "~" + model.FormatMonthDay(t.DueDate)
		it.SpanLabel = "진행중 · 시작 " + model.FormatMonthDay(t.WorkDate) + " → 종료 " + model.FormatMonthDay(t.DueDate)
		if strings.TrimSpace(today) != "" && model.NormalizeAppDate(t.WorkDate) != model.NormalizeAppDate(today) &&
			model.TaskSpansDate(t.WorkDate, t.DueDate, today) {
			it.ScheduleLine = it.SpanLabel
			it.HasTime = false
		}
	}
	return it
}

func customerPlaceLine(cust, place string) string {
	cust, place = strings.TrimSpace(cust), strings.TrimSpace(place)
	if cust != "" && place != "" {
		return cust + " · " + place
	}
	if cust != "" {
		return cust
	}
	return place
}

func boardOneLineSchedule(day, timeLabel string) string {
	day = strings.TrimSpace(day)
	if day == "" {
		return "날짜 미정"
	}
	t, err := time.ParseInLocation(dateLayout, day, time.Local)
	if err != nil {
		if strings.TrimSpace(timeLabel) != "" {
			return strings.TrimSpace(timeLabel)
		}
		return day
	}
	wd := []string{"일", "월", "화", "수", "목", "금", "토"}
	s := fmt.Sprintf("%d/%02d (%s)", int(t.Month()), t.Day(), wd[int(t.Weekday())])
	clock := strings.TrimSpace(timeLabel)
	if clock == "" {
		return s
	}
	clock = strings.ReplaceAll(clock, "–", "~")
	clock = strings.ReplaceAll(clock, " - ", "~")
	clock = strings.Join(strings.Fields(clock), "")
	s += " " + clock
	return s
}

func sortBoardItems(items []boardItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].HasTime != items[j].HasTime {
			return items[i].HasTime
		}
		if items[i].HasTime && items[i].StartMin != items[j].StartMin {
			return items[i].StartMin < items[j].StartMin
		}
		return items[i].Task.Title < items[j].Task.Title
	})
}

func collectASMntIDs(tasks []model.WorkTask) (asIDs, mntIDs []string) {
	for _, t := range tasks {
		switch t.SourceType {
		case model.WBSourceAS:
			if t.SourceID != "" {
				asIDs = append(asIDs, t.SourceID)
			}
		case model.WBSourceMaintenance:
			if t.SourceID != "" {
				mntIDs = append(mntIDs, t.SourceID)
			}
		}
	}
	return
}

func (h *WorkboardHandler) personalBoardData(c echo.Context) (map[string]interface{}, error) {
	view := parseBoardView(c.QueryParam("view"))
	now := time.Now()
	today := now.Format(dateLayout)
	base := parseBoardDate(c.QueryParam("date"), now)
	listRange := parseListRangeParam(c.QueryParam("range"))
	if view == boardViewKanban {
		base = parseBoardDate(today, now)
		listRange = boardRangeDay
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	rangeView := view
	if view == boardViewList {
		rangeView = listRange
	}
	fromT, toT := boardRange(rangeView, base)
	from, to := fromT.Format(dateLayout), toT.Format(dateLayout)
	scope := h.withBoardMemberFilter(c, resolveBoardScope(c))
	keep := boardKeepQuery(c)
	keep.Del("members")
	for _, id := range scope.MemberIDs {
		keep.Add("members", id)
	}
	showPlan, showAct := model.ParseScheduleShow(c.QueryParam("show"))
	keep.Set("show", model.EncodeScheduleShow(showPlan, showAct))

	entries, err := h.collectBoardEntries(c, from, to, scope)
	if err != nil {
		return nil, err
	}
	mergeKanbanOpen(h, c, &entries, currentOrg(c), scope, today)

	filterHit := func(it boardItem) bool {
		if !boardSearchHit(it, q) {
			return false
		}
		return true
	}
	var placed, overdue, unplaced []boardItem
	for _, it := range entries.Placed {
		if filterHit(it) {
			placed = append(placed, it)
		}
	}
	for _, it := range entries.Overdue {
		if filterHit(it) {
			overdue = append(overdue, it)
		}
	}
	for _, it := range entries.Unplaced {
		if filterHit(it) {
			unplaced = append(unplaced, it)
		}
	}

	var calItems []boardItem
	if showPlan {
		for _, it := range placed {
			it.PlaceDate = model.TaskPlanDate(it.Task)
			calItems = append(calItems, it)
		}
		for _, it := range overdue {
			it.PlaceDate = model.TaskPlanDate(it.Task)
			calItems = append(calItems, it)
		}
	}
	if showAct {
		same := map[string]bool{}
		if showPlan {
			for _, it := range calItems {
				d := it.PlaceDate
				if d == "" {
					d = model.TaskPlanDate(it.Task)
				}
				if d != "" && d == model.TaskExecDate(it.Task) {
					same[taskDedupKey(it.Task)] = true
				}
			}
		}
		for _, it := range entries.Exec {
			if !filterHit(it) {
				continue
			}
			ed := strings.TrimSpace(it.PlaceDate)
			if ed == "" {
				ed = model.TaskExecDate(it.Task)
			}
			if showPlan && same[taskDedupKey(it.Task)] {
				continue
			}
			it.PlaceDate = ed
			calItems = append(calItems, it)
		}
	}

	calItems = h.expandBoardSegments(calItems, showPlan, showAct)
	stampBoardItemKinds(h, calItems)

	var items []boardItem
	var listGroups []boardListGroup
	if view == boardViewList {
		if listRange == boardRangeDay {
			day := base.Format(dateLayout)
			for _, it := range calItems {
				if it.Overdue && showPlan {
					items = append(items, it)
					continue
				}
				if it.PlaceDate == day || model.TaskSpansDate(it.Task.WorkDate, it.Task.DueDate, day) {
					items = append(items, it)
				}
			}
		} else {
			items = append(items, calItems...)
		}
		sortListBoardItems(items)
		h.annotateListDelete(c, items)
		listGroups = groupBoardListItems(items, listRange, fromT, toT, base)
	} else {
		items = append(items, calItems...)
		sortBoardItems(items)
	}

	byDate := map[string][]boardItem{}
	for _, it := range calItems {
		d := strings.TrimSpace(it.PlaceDate)
		if d == "" {
			continue
		}
		byDate[d] = append(byDate[d], it)
	}
	for d := range byDate {
		sortBoardItems(byDate[d])
	}

	weekdayKR := []string{"일", "월", "화", "수", "목", "금", "토"}
	var monthWeeks [][]boardMonthCell
	if view == boardViewMonth {
		for w := 0; w < 6; w++ {
			row := make([]boardMonthCell, 7)
			for d := 0; d < 7; d++ {
				day := fromT.AddDate(0, 0, w*7+d)
				ds := day.Format(dateLayout)
				list := byDate[ds]
				more := 0
				shown := list
				if len(list) > 3 {
					shown = list[:3]
					more = len(list) - 3
				}
				row[d] = boardMonthCell{
					Date:     ds,
					Day:      day.Day(),
					InMonth:  day.Month() == base.Month(),
					IsToday:  ds == today,
					Items:    shown,
					MoreN:    more,
					MoreHref: boardURL(boardViewDay, ds, keep),
				}
			}
			monthWeeks = append(monthWeeks, row)
		}
	}

	var weekCols []boardWeekCol
	if view == boardViewWeek {
		for i := 0; i < 7; i++ {
			day := fromT.AddDate(0, 0, i)
			ds := day.Format(dateLayout)
			weekCols = append(weekCols, boardWeekCol{
				Date:    ds,
				Dow:     weekdayKR[int(day.Weekday())],
				Day:     day.Day(),
				IsToday: ds == today,
				Items:   byDate[ds],
			})
		}
	}

	var dayTimed, dayUntimed []boardItem
	nowTop := -1
	dayHours := []string{}
	colsMode := parseBoardCols(c.QueryParam("cols"))
	var dayPersonCols []boardPersonCol
	if view == boardViewDay {
		ds := base.Format(dateLayout)
		list := byDate[ds]
		for _, it := range calItems {
			if strings.TrimSpace(it.PlaceDate) == ds {
				continue
			}
			if !model.TaskSpansDate(it.Task.WorkDate, it.Task.DueDate, ds) {
				continue
			}
			it.HasTime = false
			if it.SpanLabel != "" {
				it.ScheduleLine = it.SpanLabel
			}
			dayUntimed = append(dayUntimed, it)
		}
		var evs []DayLayoutEvent
		var timed []boardItem
		for _, it := range list {
			if !it.HasTime {
				dayUntimed = append(dayUntimed, it)
				continue
			}
			timed = append(timed, it)
			evs = append(evs, DayLayoutEvent{StartMin: it.StartMin, EndMin: it.EndMin})
		}
		placedLay := layoutDayEvents(evs)
		h0 := boardDayStartHour * 60
		h1 := (boardDayEndHour + 1) * 60
		for i := range timed {
			p := placedLay[i]
			s := p.StartMin
			e := p.EndMin
			if s < h0 {
				s = h0
			}
			if e > h1 {
				e = h1
			}
			top := (s-h0)*boardDayPxPerHour/60 + 2
			hgt := (e-s)*boardDayPxPerHour/60 - 4
			if hgt < 40 {
				hgt = 40
			}
			timed[i].DayPosTop = top
			timed[i].DayPosHeight = hgt
			timed[i].DayCol = p.Col
			timed[i].DayCols = p.Cols
			dayTimed = append(dayTimed, timed[i])
		}
		for hr := boardDayStartHour; hr <= boardDayEndHour; hr++ {
			dayHours = append(dayHours, fmt.Sprintf("%02d:00", hr))
		}
		if ds == today {
			m := now.Hour()*60 + now.Minute()
			if m >= h0 && m <= h1 {
				nowTop = (m - h0) * boardDayPxPerHour / 60
			}
		}
	}

	shiftView := view
	if view == boardViewList {
		shiftView = listRange
	}
	prev := boardPeriodShift(shiftView, base, -1).Format(dateLayout)
	next := boardPeriodShift(shiftView, base, 1).Format(dateLayout)
	calHref := boardURL(boardViewDay, base.Format(dateLayout), keep)
	listHref := boardURL(boardViewList, base.Format(dateLayout), keep)
	kanbanHref := boardURL(boardViewKanban, today, keep)
	ds := base.Format(dateLayout)
	title := "일일 업무"
	if view != boardViewList {
		title = "일정 캘린더"
	}
	if view == boardViewKanban {
		title = "칸반 보기"
	}
	writeBase := boardCanWrite(c)
	dsLock := base.Format(dateLayout)
	dayPast := registerPastDayLockEnabled && view == boardViewDay && dsLock < today
	unlocked, unlockExp := h.registerDayUnlocked(c, dsLock)
	write := writeBase && (!dayPast || unlocked)
	canUnlock := registerPastDayLockEnabled && writeBase && isAdminRole(c) && dayPast && !unlocked
	unlockExpLabel := ""
	if !unlockExp.IsZero() {
		unlockExpLabel = unlockExp.Format("15:04")
	}
	me := currentUserDisplayName(c)
	mineKeep := cloneBoardQuery(keep)
	allKeep := cloneBoardQuery(keep)
	if scope.ShowToggle {
		mineKeep.Del("mine")
		mineKeep.Del("assignee")
		allKeep.Set("mine", "0")
		allKeep.Del("assignee")
	} else {
		mineKeep.Del("mine")
		mineKeep.Set("assignee", me)
		allKeep.Del("mine")
		allKeep.Del("assignee")
	}
	mineEmptyN := 0
	if !scope.ScopeAll && len(items)+len(unplaced) == 0 {
		mineEmptyN = boardMineEmptyAllCount(h, c, from, to)
	}
	unplacedN := len(unplaced)
	planN, actN := 0, 0
	for _, it := range placed {
		if filterHit(it) {
			planN++
		}
	}
	for _, it := range entries.Exec {
		if filterHit(it) {
			actN++
		}
	}
	curShow := model.EncodeScheduleShow(showPlan, showAct)
	planKeep := cloneBoardQuery(keep)
	stampBoardPlanEdit(c, items)
	stampBoardPlanEdit(c, unplaced)
	stampBoardPlanEdit(c, dayTimed)
	stampBoardPlanEdit(c, dayUntimed)
	stampBoardDelayReasons(h, items)
	stampBoardDelayReasons(h, unplaced)
	stampBoardDelayReasons(h, dayTimed)
	stampBoardDelayReasons(h, dayUntimed)
	stampBoardItemKinds(h, items)
	stampBoardItemKinds(h, unplaced)
	stampBoardItemKinds(h, dayTimed)
	stampBoardItemKinds(h, dayUntimed)
	for i := range weekCols {
		stampBoardPlanEdit(c, weekCols[i].Items)
	}
	for i := range monthWeeks {
		for j := range monthWeeks[i] {
			stampBoardPlanEdit(c, monthWeeks[i][j].Items)
			stampBoardMove(c, monthWeeks[i][j].Items, write)
		}
	}
	stampBoardMove(c, items, write)
	stampBoardMove(c, unplaced, write)
	stampBoardMove(c, dayTimed, write)
	stampBoardMove(c, dayUntimed, write)
	for i := range weekCols {
		stampBoardMove(c, weekCols[i].Items, write)
	}
	if view == boardViewDay && colsMode == boardColsPerson {
		assignees := listAssignableForPicker(h.userRepo, c)
		order := make([]string, 0, len(assignees))
		regNames := map[string]bool{}
		for _, u := range assignees {
			n := strings.TrimSpace(u.FullName)
			if n == "" {
				continue
			}
			order = append(order, n)
			regNames[n] = true
		}
		leaveHit := map[string]bool{}
		for _, n := range boardDayLeaveNames(h, c, ds) {
			leaveHit[n] = true
		}
		dayPersonCols = buildBoardPersonCols(dayTimed, order, scope.AssigneeFilter, leaveHit, regNames)
	}
	planKeep.Set("show", model.ToggleScheduleShow(curShow, model.ScheduleShowPlan))
	actKeep := cloneBoardQuery(keep)
	actKeep.Set("show", model.ToggleScheduleShow(curShow, model.ScheduleShowAct))
	personKeep := cloneBoardQuery(keep)
	personKeep.Del("cols")
	flatKeep := cloneBoardQuery(keep)
	flatKeep.Set("cols", boardColsFlat)
	assignees := listAssignableForPicker(h.userRepo, c)
	extraAssignees := extraAssigneesForBoard(assignees, dayPersonCols)
	if view != boardViewDay || colsMode != boardColsPerson || strings.TrimSpace(scope.AssigneeFilter) != "" {
		extraAssignees = nil
	}
	var legendTasks []model.WorkTask
	for _, it := range calItems {
		legendTasks = append(legendTasks, it.Task)
	}
	projects, _ := h.repo.ListProjects(true)
	wgItems, _ := h.workGroupPicks(c)
	data := map[string]interface{}{
		"Title":            title,
		"Active":           NavWorkBoard,
		"View":             view,
		"Date":             ds,
		"Today":            today,
		"DateLabel":        formatDotDate(ds),
		"PeriodLabel":      boardPeriodLabel(shiftView, base, fromT, toT),
		"ListRange":        listRange,
		"ListGroups":       listGroups,
		"KanbanHref":       kanbanHref,
		"PrevDate":         prev,
		"NextDate":         next,
		"Q":                q,
		"Items":            items,
		"Count":            uniqueTaskCount(items),
		"Unplaced":         unplaced,
		"UnplacedN":        unplacedN,
		"OtherUnplacedN":   entries.OtherUnplacedN,
		"MineEmptyAllN":    mineEmptyN,
		"ScopeAll":         scope.ScopeAll,
		"ShowScopeToggle":  true,
		"MineHref":         boardURL(view, ds, mineKeep),
		"AllHref":          boardURL(view, ds, allKeep),
		"MonthWeeks":       monthWeeks,
		"WeekCols":         weekCols,
		"DayTimed":         dayTimed,
		"DayUntimed":       dayUntimed,
		"DayHours":         dayHours,
		"DayNowTop":        nowTop,
		"ShowNowLine":      nowTop >= 0,
		"DayBodyHeight":    (boardDayEndHour - boardDayStartHour + 1) * boardDayPxPerHour,
		"CalHref":          calHref,
		"ListHref":         listHref,
		"PrevHref":         boardURL(view, prev, keep),
		"NextHref":         boardURL(view, next, keep),
		"TodayHref":        boardURL(view, today, keep),
		"MonthViewHref":    boardURL(boardViewMonth, ds, keep),
		"WeekViewHref":     boardURL(boardViewWeek, ds, keep),
		"DayViewHref":      boardURL(boardViewDay, ds, keep),
		"ListDayHref":      boardListRangeURL(ds, boardRangeDay, keep),
		"ListWeekHref":     boardListRangeURL(ds, boardRangeWeek, keep),
		"ListMonthHref":    boardListRangeURL(ds, boardRangeMonth, keep),
		"MineParam":        scope.MineParam,
		"AssigneeFilter":   scope.AssigneeFilter,
		"ShowMemberFilter": showBoardMemberFilter(scope.Role, view),
		"TeamMembers":      boardMemberOpts(scope),
		"MemberFilterOn":   len(scope.MemberIDs) > 0,
		"MemberAllHref":    boardURL(view, ds, memberAllKeep(keep)),
		"MembersParam":     strings.Join(scope.MemberIDs, ","),
		"Weekdays":         weekdayKR,
		"CanWrite":         write,
		"PastDayLocked":    dayPast && !unlocked,
		"CanUnlockPast":    canUnlock,
		"EditUnlocked":     unlocked && dayPast,
		"UnlockExpires":    unlockExpLabel,
		"UnlockRedirect":   boardURL(view, ds, keep),
		"Cols":             colsMode,
		"DayPersonCols":    dayPersonCols,
		"AssigneeColMin":   registerAssigneeColMinPx,
		"PersonHref":       boardURL(view, ds, personKeep),
		"FlatHref":         boardURL(view, ds, flatKeep),
		"ExtraAssignees":   extraAssignees,
		"DayLeaveNames":    boardDayLeaveNames(h, c, ds),
		"AssigneeLegend":   buildAssigneeLegend(legendTasks, assignees),
		"Projects":         projects,
		"WorkGroups":       wgItems,
		"ProjectFilter":    strings.TrimSpace(c.QueryParam("project")),
		"WorkGroupFilter":  strings.TrimSpace(c.QueryParam("work_group_id")),
		"CurrentUserName":  me,
		"BoardHref":        boardURL(view, ds, keep),
		"ScheduleRedirect": boardURL(view, ds, keep),
		"ShowPlan":         showPlan,
		"ShowAct":          showAct,
		"ShowParam":        curShow,
		"PlanCount":        planN,
		"ActCount":         actN,
		"ShowPlanHref":     boardURL(view, ds, planKeep),
		"ShowActHref":      boardURL(view, ds, actKeep),
	}
	if view == boardViewKanban {
		kanbanItems := append([]boardItem{}, calItems...)
		stampBoardPlanEdit(c, kanbanItems)
		stampBoardDelayReasons(h, kanbanItems)
		stampBoardItemKinds(h, kanbanItems)
		stampBoardMove(c, kanbanItems, write)
		kcols := classifyKanban(kanbanItems, today)
		stampKanbanExtras(h, kcols)
		data["KanbanDelay"] = kcols[kanbanColDelay]
		data["KanbanOff"] = kcols[kanbanColOff]
		data["KanbanPlan"] = kcols[kanbanColPlan]
		data["KanbanProgress"] = kcols[kanbanColProgress]
		data["KanbanPartial"] = kcols[kanbanColPartial]
		data["KanbanDone"] = kcols[kanbanColDone]
		data["KanbanDelayN"] = len(kcols[kanbanColDelay])
		data["KanbanOffN"] = len(kcols[kanbanColOff])
		data["KanbanPlanN"] = len(kcols[kanbanColPlan])
		data["KanbanProgressN"] = len(kcols[kanbanColProgress])
		data["KanbanPartialN"] = len(kcols[kanbanColPartial])
		data["KanbanDoneN"] = len(kcols[kanbanColDone])
	}
	mergeCarryModal(data)
	data["ItemOrgID"] = currentOrg(c)
	attachWorkItemView(data, h, nil, false)
	return attachViewGroup(c, data), nil
}

type boardListGroup struct {
	Date  string
	Label string
	Items []boardItem
}

func boardListRangeURL(date, rng string, keep url.Values) string {
	k := cloneBoardQuery(keep)
	if rng == boardRangeDay {
		k.Del("range")
	} else {
		k.Set("range", rng)
	}
	return boardURL(boardViewList, date, k)
}

func groupBoardListItems(items []boardItem, rng string, from, to, base time.Time) []boardListGroup {
	if rng == boardRangeDay {
		if len(items) == 0 {
			return nil
		}
		return []boardListGroup{{Date: base.Format(dateLayout), Label: formatDotDate(base.Format(dateLayout)), Items: items}}
	}
	by := map[string][]boardItem{}
	var overdue []boardItem
	for _, it := range items {
		if it.Overdue {
			overdue = append(overdue, it)
			continue
		}
		d := strings.TrimSpace(it.PlaceDate)
		if d == "" {
			d = model.NormalizeAppDate(it.Task.WorkDate)
		}
		if d == "" {
			d = model.NormalizeAppDate(it.Task.DueDate)
		}
		if d == "" {
			continue
		}
		by[d] = append(by[d], it)
	}
	var out []boardListGroup
	if len(overdue) > 0 {
		out = append(out, boardListGroup{Date: "", Label: "지연 업무", Items: overdue})
	}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		ds := d.Format(dateLayout)
		list := by[ds]
		if len(list) == 0 {
			continue
		}
		out = append(out, boardListGroup{Date: ds, Label: formatDotDate(ds), Items: list})
	}
	return out
}

func cloneBoardQuery(v url.Values) url.Values {
	out := url.Values{}
	for k, vs := range v {
		out[k] = append([]string{}, vs...)
	}
	return out
}

type boardMemberOpt struct {
	UserID  string
	Name    string
	Checked bool
}

func boardMemberOpts(scope boardScope) []boardMemberOpt {
	want := map[string]bool{}
	for _, id := range scope.MemberIDs {
		want[id] = true
	}
	var out []boardMemberOpt
	for _, u := range scope.TeamMembers {
		id := strings.TrimSpace(u.UserID)
		if id == "" {
			continue
		}
		out = append(out, boardMemberOpt{UserID: id, Name: u.FullName, Checked: want[id]})
	}
	return out
}

func memberAllKeep(keep url.Values) url.Values {
	out := cloneBoardQuery(keep)
	out.Del("members")
	return out
}

func (h *WorkboardHandler) expandBoardSegments(items []boardItem, showPlan, showAct bool) []boardItem {
	if h == nil || h.repo == nil || len(items) == 0 {
		return items
	}
	ids := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		id := strings.TrimSpace(it.Task.TaskID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	segMap, _ := h.repo.ListSegmentsByTaskIDs(ids)
	pending := map[string]bool{}
	if deps, err := h.repo.ListPendingReturns(); err == nil {
		for _, d := range deps {
			pending[d.TaskID] = true
		}
	}
	var out []boardItem
	for _, it := range items {
		it.ReturnPending = pending[it.Task.TaskID]
		kind := model.SegKindPlan
		execDay := model.TaskExecDate(it.Task)
		planDay := model.TaskPlanDate(it.Task)
		if showAct && (!showPlan || (execDay != "" && it.PlaceDate == execDay && execDay != planDay)) {
			kind = model.SegKindActual
		}
		var use []model.WorkTaskSegment
		for _, s := range segMap[it.Task.TaskID] {
			if model.NormalizeSegmentKind(s.Kind) == kind && model.NormalizeSegmentScope(s.Scope) == model.SegScopeCommon {
				use = append(use, s)
			}
		}
		if len(use) == 0 && kind == model.SegKindPlan {
			v := model.VirtualPlanSegment(it.Task)
			if v.StartTime != "" {
				use = []model.WorkTaskSegment{v}
			}
		}
		if len(use) <= 1 {
			if len(use) == 1 {
				applySegTimes(&it, use[0], 1, 1)
			}
			out = append(out, it)
			continue
		}
		for i, s := range use {
			bar := it
			applySegTimes(&bar, s, i+1, len(use))
			if i > 0 {
				bar.Fragment = true
			}
			out = append(out, bar)
		}
	}
	return out
}

func applySegTimes(it *boardItem, s model.WorkTaskSegment, idx, n int) {
	st, en := strings.TrimSpace(s.StartTime), strings.TrimSpace(s.EndTime)
	if len(st) >= 5 {
		st = st[:5]
	}
	if len(en) >= 5 {
		en = en[:5]
	}
	sm, okS := parseClockMin(st)
	em, okE := parseClockMin(en)
	if okS {
		it.HasTime = true
		it.StartMin = sm
		if okE {
			it.EndMin = em
			it.TimeLabel = st + " – " + en
		} else {
			it.EndMin = sm + 30
			it.TimeLabel = st
		}
		it.ChipTime = st
		if n > 1 {
			it.ChipTime = fmt.Sprintf("%s · %d/%d", st, idx, n)
		}
		day := strings.TrimSpace(s.WorkDate)
		if day == "" {
			day = strings.TrimSpace(it.PlaceDate)
		}
		if day == "" {
			day = strings.TrimSpace(it.Task.WorkDate)
		}
		it.ScheduleLine = boardOneLineSchedule(day, it.TimeLabel)
		if s.WorkDate != "" {
			it.PlaceDate = s.WorkDate
			it.DateLabel = formatDotDate(s.WorkDate)
		}
	}
	it.SegIndex, it.SegCount = idx, n
	if n > 1 {
		it.SegLabel = fmt.Sprintf("%d/%d", idx, n)
	}
	it.TravelAfter = s.TravelAfterMin > 0
}
