package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

const (
	boardViewList  = "list"
	boardViewMonth = "month"
	boardViewWeek  = "week"
	boardViewDay   = "day"

	boardDayStartHour = 7
	boardDayEndHour   = 21
	boardDayPxPerHour = 64
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
	Task            model.WorkTask
	Href            string
	Type            boardTypeStyle
	Status          boardStatusView
	CustomerLine    string
	PlaceLine       string
	DateLabel       string
	TimeLabel       string
	HasTime         bool
	StartMin        int
	EndMin          int
	AssigneeName    string
	AssigneeExtra   string
	AssigneeTitle   string
	AssigneeInitial string
	ChipTime        string
	DayPosTop       int
	DayPosHeight    int
	DayCol          int
	DayCols         int
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
	view := data["View"].(string)
	tpl := "workboard/board_list.html"
	switch view {
	case boardViewMonth:
		tpl = "workboard/board_month.html"
	case boardViewWeek:
		tpl = "workboard/board_week.html"
	case boardViewDay:
		tpl = "workboard/board_day.html"
	}
	return c.Render(http.StatusOK, tpl, data)
}

func parseBoardView(raw string) string {
	switch strings.TrimSpace(raw) {
	case boardViewMonth, boardViewWeek, boardViewDay, boardViewList:
		return strings.TrimSpace(raw)
	default:
		return boardViewList
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
	day := strings.TrimSpace(t.WorkDate)
	if day == "" {
		day = strings.TrimSpace(t.DueDate)
	}
	if st != model.WBTaskCancelled && day != "" && day < today {
		return boardStatusView{Key: "late", Label: "지연", Class: "board-st-late"}
	}
	if st == model.WBTaskInProgress || st == model.WBTaskHold || st == model.WBTaskTransfer || st == model.WBTaskReview {
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
	primary := t.Assignee
	if primary == "" && len(names) > 0 {
		primary = names[0]
	}
	extra := ""
	if len(names) > 1 {
		extra = fmt.Sprintf(" 외 %d명", len(names)-1)
	}
	cust := strings.TrimSpace(t.OrgName)
	if cust == "" {
		cust = strings.TrimSpace(t.CustomerName)
	}
	if cust == "" {
		cust = strings.TrimSpace(t.SiteLine)
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
		AssigneeName:    primary,
		AssigneeExtra:   extra,
		AssigneeTitle:   strings.Join(names, ", "),
		AssigneeInitial: firstRune(primary),
		ChipTime:        chipTime,
	}
	return it
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
	q := strings.TrimSpace(c.QueryParam("q"))
	fromT, toT := boardRange(view, base)
	from, to := fromT.Format(dateLayout), toT.Format(dateLayout)
	org := currentOrg(c)

	syncFrom, syncTo := registerSyncWindow(from, to)
	h.syncMaintenanceTasks(syncFrom, syncTo, org)

	tasks, err := h.repo.ListTasksBetweenForOrg(from, to, org)
	if err != nil {
		return nil, err
	}
	asIDs, mntIDs := collectASMntIDs(tasks)
	asStatus, _ := h.repo.ASStatusesByIDs(asIDs)
	mntDone, _ := h.repo.MaintenanceCompletedByIDs(mntIDs)
	var visible []model.WorkTask
	for _, t := range tasks {
		ok := false
		t, ok = applyWorkboardSourceStatus(t, asStatus, mntDone)
		if !ok {
			continue
		}
		if t.Status == model.WBTaskCancelled {
			continue
		}
		visible = append(visible, t)
	}

	ids := taskIDsOf(visible)
	memberMap := map[string][]model.WorkTaskMember{}
	if len(ids) > 0 {
		if m, err := h.repo.ListMembersByTaskIDs(ids); err == nil {
			memberMap = m
		}
	}
	visible = filterTasksForPersonalAssignee(visible, memberMap, currentUserID(c), currentUserDisplayName(c))
	ids = taskIDsOf(visible)

	siteLines := map[string]string{}
	if len(ids) > 0 {
		if m, err := h.repo.TaskSiteLinesByTaskIDs(ids); err == nil {
			siteLines = m
		}
	}
	var custIDs []string
	for i := range visible {
		if line := siteLines[visible[i].TaskID]; line != "" {
			visible[i].SiteLine = line
		}
		if cid := strings.TrimSpace(visible[i].CustomerID); cid != "" {
			custIDs = append(custIDs, cid)
		}
	}
	addrMap := map[string]string{}
	if h.customerRepo != nil && len(custIDs) > 0 {
		if m, err := h.customerRepo.AddressesByIDs(custIDs); err == nil {
			addrMap = m
		}
	}

	items := make([]boardItem, 0, len(visible))
	for _, t := range visible {
		it := buildBoardItem(t, memberMap[t.TaskID], addrMap[strings.TrimSpace(t.CustomerID)], today)
		if view == boardViewList && !boardSearchHit(it, q) {
			continue
		}
		items = append(items, it)
	}
	sortBoardItems(items)

	byDate := map[string][]boardItem{}
	for _, it := range items {
		d := strings.TrimSpace(it.Task.WorkDate)
		if d == "" {
			d = strings.TrimSpace(it.Task.DueDate)
		}
		byDate[d] = append(byDate[d], it)
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
					MoreHref: "/workboard/board?view=day&date=" + ds,
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
	if view == boardViewDay {
		ds := base.Format(dateLayout)
		list := byDate[ds]
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
		placed := layoutDayEvents(evs)
		h0 := boardDayStartHour * 60
		h1 := (boardDayEndHour + 1) * 60
		for i := range timed {
			p := placed[i]
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

	prev := boardPeriodShift(view, base, -1).Format(dateLayout)
	next := boardPeriodShift(view, base, 1).Format(dateLayout)
	calHref := "/workboard/board?view=month&date=" + base.Format(dateLayout)
	listHref := "/workboard/board?view=list&date=" + base.Format(dateLayout)
	if q != "" && view == boardViewList {
		listHref += "&q=" + q
	}

	title := "일일 업무"
	if view != boardViewList {
		title = "일정 캘린더"
	}
	return map[string]interface{}{
		"Title":         title,
		"Active":        NavWorkBoard,
		"View":          view,
		"Date":          base.Format(dateLayout),
		"Today":         today,
		"DateLabel":     formatDotDate(base.Format(dateLayout)),
		"PeriodLabel":   boardPeriodLabel(view, base, fromT, toT),
		"PrevDate":      prev,
		"NextDate":      next,
		"Q":             q,
		"Items":         items,
		"Count":         len(items),
		"MonthWeeks":    monthWeeks,
		"WeekCols":      weekCols,
		"DayTimed":      dayTimed,
		"DayUntimed":    dayUntimed,
		"DayHours":      dayHours,
		"DayNowTop":     nowTop,
		"ShowNowLine":   nowTop >= 0,
		"DayBodyHeight": (boardDayEndHour - boardDayStartHour + 1) * boardDayPxPerHour,
		"CalHref":       calHref,
		"ListHref":      listHref,
		"Weekdays":      weekdayKR,
	}, nil
}
