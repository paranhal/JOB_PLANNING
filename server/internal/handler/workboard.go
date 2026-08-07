package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type WorkboardHandler struct {
	repo         *repository.WBRepo
	userRepo     *repository.UserRepo
	customerRepo *repository.CustomerRepo
	contactRepo  *repository.ContactRepo
	codeRepo     *repository.CodeRepo
	asRepo       *repository.ASRepo
	mntRepo      *repository.MaintenanceRepo
}

func NewWorkboardHandler(
	repo *repository.WBRepo,
	userRepo *repository.UserRepo,
	customerRepo *repository.CustomerRepo,
	contactRepo *repository.ContactRepo,
	codeRepo *repository.CodeRepo,
	asRepo *repository.ASRepo,
	mntRepo *repository.MaintenanceRepo,
) *WorkboardHandler {
	return &WorkboardHandler{
		repo: repo, userRepo: userRepo,
		customerRepo: customerRepo, contactRepo: contactRepo, codeRepo: codeRepo,
		asRepo: asRepo, mntRepo: mntRepo,
	}
}

func (h *WorkboardHandler) Index(c echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/workboard/kanban")
}

func (h *WorkboardHandler) Kanban(c echo.Context) error {
	data, err := h.boardData(c, "kanban")
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "workboard/kanban.html", data)
}

func (h *WorkboardHandler) List(c echo.Context) error {
	data, err := h.boardData(c, "list")
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "workboard/list.html", data)
}

// 업무 등록 화면의 기간 단위
const (
	regViewDay   = "day"
	regViewWeek  = "week"
	regViewMonth = "month"
)

const (
	workdayStartHour = 7
	workdayEndHour   = 20
	dateLayout       = "2006-01-02"
)

// RegisterColumn 시간표의 세로 열. 일간은 1개, 주간은 요일 7개, 월간은 주차별로 만든다.
type RegisterColumn struct {
	Label   string // 월 / 7/1주
	Sub     string // 8/4
	Date    string // 이 열에 카드를 놓았을 때 배치될 날짜
	From    string // 열이 포함하는 기간 (월간은 한 주)
	To      string
	IsToday bool
}

// RegisterCell 시간표 한 칸 (열 × 시각)
type RegisterCell struct {
	Column  int
	Date    string
	Time    string
	IsToday bool
	Cards   []model.WBCard
}

// RegisterRow 시간표 한 줄 (15분)
type RegisterRow struct {
	Time   string
	IsHour bool
	Cells  []RegisterCell
}

// Register 업무 등록 화면 — 왼쪽 15분 단위 시간표(07:00~20:00)에
// 오른쪽 AS·정기점검·행정관련 업무 카드를 끌어다 배치한다.
func (h *WorkboardHandler) Register(c echo.Context) error {
	view := strings.TrimSpace(c.QueryParam("view"))
	if view != regViewWeek && view != regViewMonth {
		view = regViewDay
	}

	today := time.Now().Format(dateLayout)
	base, err := time.ParseInLocation(dateLayout, strings.TrimSpace(c.QueryParam("date")), time.Local)
	if err != nil {
		base = time.Now()
	}
	base = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, time.Local)

	period := buildRegisterPeriod(view, base, today)

	// 예정일이 있는 미배치 업무를 해당 날짜 07:00부터 차례로 올린다.
	if c.QueryParam("auto") != "0" {
		if _, err := h.autoPlacePlanned(period.From, period.To); err != nil {
			return err
		}
	}

	placed, err := h.repo.ListTasksBetween(period.From, period.To)
	if err != nil {
		return err
	}

	assigneeFilter := normalizeAssigneeFilter(c.QueryParam("assignee"))
	placed = filterTasksByAssignee(placed, assigneeFilter)
	asCards, mntCards, adminCards, err := h.registerPalette()
	if err != nil {
		return err
	}
	asCards = filterCardsByAssignee(asCards, assigneeFilter)
	mntCards = filterCardsByAssignee(mntCards, assigneeFilter)
	adminCards = filterCardsByAssignee(adminCards, assigneeFilter)

	projects, _ := h.repo.ListProjects(false)
	assignees, _ := h.userRepo.ListAssignable()

	slotTimes := registerSlotTimes()
	dayCols := buildRegisterDayColumns(period.Columns, placed, h.taskCard)
	// 드롭 존용 빈 행(카드는 DayColumns.Blocks에 절대 배치)
	rows := h.buildRegisterRows(period.Columns, nil)
	assigneeLegend := buildAssigneeLegend(placed, assignees, asCards, mntCards, adminCards)
	gridHeightStyle, slotTopStyles := registerGridStyles(len(slotTimes))
	var monthWeeks [][]RegisterMonthDay
	if view == regViewMonth {
		monthWeeks = buildRegisterMonthWeeks(base, today, placed, h.taskCard)
	}

	dateStr := base.Format(dateLayout)
	regBase := registerURL(view, dateStr, assigneeFilter)

	return c.Render(http.StatusOK, "workboard/register.html", map[string]interface{}{
		"Title":           "일일 업무 등록",
		"Active":          "work_register",
		"View":            view,
		"ViewLabel":       registerViewLabel(view),
		"Date":            dateStr,
		"PeriodLabel":     period.Label,
		"PrevDate":        period.Prev,
		"NextDate":        period.Next,
		"Today":           today,
		"Columns":         period.Columns,
		"DayColumns":      dayCols,
		"MonthWeeks":      monthWeeks,
		"SlotTimes":       slotTimes,
		"SlotRem":         registerSlotRem,
		"GridHeightStyle": gridHeightStyle,
		"SlotTopStyles":   slotTopStyles,
		"Rows":            rows,
		"AssigneeLegend":  assigneeLegend,
		"ASCards":         asCards,
		"MntCards":        mntCards,
		"AdminCards":      adminCards,
		"PlacedCount":     len(placed),
		"Projects":        projects,
		"Assignees":       assignees,
		"AssigneeFilter":  assigneeFilter,
		"AssigneeQ":       assigneeQuerySuffix(assigneeFilter),
		"ModalRedirect":   regBase,
		"CanWrite":        canWriteWorkboard(c),
		"FlashOK":         c.QueryParam("ok"),
		"FlashErr":        c.QueryParam("err"),
	})
}

// normalizeAssigneeFilter 빈 값·all·팀전체 → 팀전체(필터 없음).
func normalizeAssigneeFilter(raw string) string {
	s := strings.TrimSpace(raw)
	switch s {
	case "", "all", "팀전체":
		return ""
	default:
		return s
	}
}

func assigneeQuerySuffix(assignee string) string {
	if assignee == "" {
		return ""
	}
	return "&assignee=" + url.QueryEscape(assignee)
}

func registerURL(view, date, assignee string) string {
	return fmt.Sprintf("/workboard/register?view=%s&date=%s%s", view, date, assigneeQuerySuffix(assignee))
}

func filterTasksByAssignee(tasks []model.WorkTask, assignee string) []model.WorkTask {
	if assignee == "" {
		return tasks
	}
	var out []model.WorkTask
	for _, t := range tasks {
		if strings.TrimSpace(t.Assignee) == assignee {
			out = append(out, t)
		}
	}
	return out
}

func filterCardsByAssignee(cards []model.WBCard, assignee string) []model.WBCard {
	if assignee == "" {
		return cards
	}
	var out []model.WBCard
	for _, c := range cards {
		if strings.TrimSpace(c.Assignee) == assignee {
			out = append(out, c)
		}
	}
	return out
}

func registerViewLabel(view string) string {
	switch view {
	case regViewWeek:
		return "주간 업무 등록"
	case regViewMonth:
		return "월간 업무 등록"
	default:
		return "일일 업무 등록"
	}
}

type registerPeriod struct {
	Columns    []RegisterColumn
	From, To   string
	Prev, Next string
	Label      string
}

var weekdayKR = [...]string{"일", "월", "화", "수", "목", "금", "토"}

func buildRegisterPeriod(view string, base time.Time, today string) registerPeriod {
	switch view {
	case regViewWeek:
		mon := base.AddDate(0, 0, -mondayOffset(base))
		p := registerPeriod{
			From:  mon.Format(dateLayout),
			To:    mon.AddDate(0, 0, 6).Format(dateLayout),
			Prev:  mon.AddDate(0, 0, -7).Format(dateLayout),
			Next:  mon.AddDate(0, 0, 7).Format(dateLayout),
			Label: fmt.Sprintf("%s ~ %s", mon.Format("2006.01.02"), mon.AddDate(0, 0, 6).Format("01.02")),
		}
		for i := 0; i < 7; i++ {
			d := mon.AddDate(0, 0, i)
			ds := d.Format(dateLayout)
			p.Columns = append(p.Columns, RegisterColumn{
				Label:   weekdayKR[int(d.Weekday())],
				Sub:     d.Format("1/2"),
				Date:    ds,
				From:    ds,
				To:      ds,
				IsToday: ds == today,
			})
		}
		return p

	case regViewMonth:
		first := time.Date(base.Year(), base.Month(), 1, 0, 0, 0, 0, base.Location())
		last := first.AddDate(0, 1, -1)
		// 월간은 캘린더 그리드로 렌더(Columns는 비움 — MonthWeeks 사용)
		return registerPeriod{
			From:  first.Format(dateLayout),
			To:    last.Format(dateLayout),
			Prev:  first.AddDate(0, -1, 0).Format(dateLayout),
			Next:  first.AddDate(0, 1, 0).Format(dateLayout),
			Label: first.Format("2006년 01월"),
		}

	default:
		ds := base.Format(dateLayout)
		return registerPeriod{
			From: ds, To: ds,
			Prev:  base.AddDate(0, 0, -1).Format(dateLayout),
			Next:  base.AddDate(0, 0, 1).Format(dateLayout),
			Label: fmt.Sprintf("%s (%s)", base.Format("2006.01.02"), weekdayKR[int(base.Weekday())]),
			Columns: []RegisterColumn{{
				Label:   weekdayKR[int(base.Weekday())] + "요일",
				Sub:     base.Format("1/2"),
				Date:    ds,
				From:    ds,
				To:      ds,
				IsToday: ds == today,
			}},
		}
	}
}

// mondayOffset 월요일을 주의 시작으로 보고 그 주의 시작까지 며칠 전인지 반환한다.
func mondayOffset(t time.Time) int {
	return (int(t.Weekday()) + 6) % 7
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (h *WorkboardHandler) buildRegisterRows(cols []RegisterColumn, placed []model.WorkTask) []RegisterRow {
	return buildRegisterRowsWith(cols, placed, h.taskCard)
}

func buildRegisterRows(cols []RegisterColumn, placed []model.WorkTask) []RegisterRow {
	return buildRegisterRowsWith(cols, placed, taskCardFromWork)
}

func buildRegisterRowsWith(cols []RegisterColumn, placed []model.WorkTask, toCard func(model.WorkTask) model.WBCard) []RegisterRow {
	rows := make([]RegisterRow, 0, ((workdayEndHour-workdayStartHour)*60)/15+1)
	for hour := workdayStartHour; hour <= workdayEndHour; hour++ {
		for m := 0; m < 60; m += 15 {
			if hour == workdayEndHour && m > 0 {
				break
			}
			hhmm := fmt.Sprintf("%02d:%02d", hour, m)
			row := RegisterRow{Time: hhmm, IsHour: m == 0}
			for i, col := range cols {
				cell := RegisterCell{Column: i, Date: col.Date, Time: hhmm, IsToday: col.IsToday}
				for _, t := range placed {
					if t.StartTime != hhmm || t.WorkDate < col.From || t.WorkDate > col.To {
						continue
					}
					cell.Cards = append(cell.Cards, toCard(t))
				}
				row.Cells = append(row.Cells, cell)
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func (h *WorkboardHandler) taskCard(t model.WorkTask) model.WBCard {
	card := taskCardFromWork(t)
	switch t.SourceType {
	case model.WBSourceAS:
		card.SourceNumber = t.SourceID
		card.SourceHref = "/as/" + t.SourceID
	case model.WBSourceMaintenance:
		if h.mntRepo != nil {
			if v, _ := h.mntRepo.GetVisit(t.SourceID); v != nil {
				card.SourceNumber = mntVisitNumber(*v)
				card.SourceHref = mntVisitHref(*v)
				card.ProductType = v.ProductType
			}
		}
		if card.ProductType == "" {
			card.ProductType = productFromMaintDesc(t.Description)
		}
	}
	return card
}

// registerPalette 아직 시간표에 올리지 않은 AS·정기점검·행정관련 업무 카드
func (h *WorkboardHandler) registerPalette() (as, mnt, admin []model.WBCard, err error) {
	tasks, err := h.repo.ListTasks()
	if err != nil {
		return nil, nil, nil, err
	}
	usedSource := map[string]bool{}
	for _, t := range tasks {
		// 이미 업무로 만들어진 원본은 팔레트에서 뺀다(시간 재배정은 시간표에서).
		if t.SourceType != "" {
			usedSource[t.SourceType+":"+t.SourceID] = true
		}
	}

	if h.asRepo != nil {
		items, _, e := h.asRepo.ListFiltered("open", "", "", nil, "", "", 1, 200)
		if e != nil {
			return nil, nil, nil, e
		}
		// 이미 배치된 건은 빼고, 같은 고객·예정일 건수로 점(.)을 붙인다.
		var open []model.ASListItem
		for _, it := range items {
			if usedSource[model.WBSourceAS+":"+it.ASID] {
				continue
			}
			open = append(open, it)
		}
		titles := buildASTitleMap(open)
		for _, it := range open {
			as = append(as, model.WBCard{
				Kind:         model.WBSourceAS,
				RefID:        it.ASID,
				Category:     model.WBSourceAS,
				Title:        titles[it.ASID],
				SubTitle:     it.Symptom,
				SourceNumber: it.ASNumber,
				SourceHref:   "/as/" + it.ASID,
				Assignee:     it.AssignedTo,
				PlannedDay:   it.VisitScheduledDate,
				DurationMin:  30,
			})
		}
	}

	if h.mntRepo != nil {
		now := time.Now()
		visits, e := h.mntRepo.ListVisitsBetween(
			now.AddDate(0, 0, -7).Format(dateLayout), now.AddDate(0, 0, 45).Format(dateLayout))
		if e != nil {
			return nil, nil, nil, e
		}
		for _, v := range visits {
			if v.Completed || usedSource[model.WBSourceMaintenance+":"+v.VisitID] {
				continue
			}
			name := v.ShortName
			if name == "" {
				name = v.OrgName
			}
			num := mntVisitNumber(v)
			mnt = append(mnt, model.WBCard{
				Kind:         model.WBSourceMaintenance,
				RefID:        v.VisitID,
				Category:     model.WBSourceMaintenance,
				Title:        model.FormatMaintenanceWorkTitle(name, num),
				SubTitle:     model.FormatMaintenanceDescription(v.ProductType, name),
				ProductType:  v.ProductType,
				SourceNumber: num,
				SourceHref:   mntVisitHref(v),
				Assignee:     v.Assignee,
				PlannedDay:   v.VisitDate,
				DurationMin:  30,
			})
		}
	}

	adminTasks, err := h.repo.ListUnplacedAdminTasks()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, t := range adminTasks {
		planned := t.DueDate
		if t.WorkDate != "" {
			planned = t.WorkDate
		}
		title := t.Title
		if t.ParentTaskID != "" {
			title = "└ " + title
		}
		cat := model.WBWorkAdmin
		if t.WorkType == model.WBWorkSupport {
			cat = model.WBWorkSupport
		}
		admin = append(admin, model.WBCard{
			Kind:         "task",
			RefID:        t.TaskID,
			TaskID:       t.TaskID,
			Category:     cat,
			Title:        title,
			SubTitle:     t.Description,
			SourceNumber: t.TaskID,
			SourceHref:   "/workboard/tasks/" + t.TaskID,
			Assignee:     t.Assignee,
			PlannedDay:   planned,
			DurationMin:  t.DurationMin,
			ParentTaskID: t.ParentTaskID,
		})
	}
	return as, mnt, admin, nil
}

// Schedule 카드를 시간표의 날짜·시각에 배치한다. AS·정기점검 카드는 업무로 복사해 원본과 연결한다.
func (h *WorkboardHandler) Schedule(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	kind := strings.TrimSpace(c.FormValue("kind"))
	refID := strings.TrimSpace(c.FormValue("ref_id"))
	date := strings.TrimSpace(c.FormValue("work_date"))
	start := strings.TrimSpace(c.FormValue("start_time"))
	if refID == "" || date == "" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=place"))
	}
	durFallback := 30
	if d := strings.TrimSpace(c.FormValue("duration_min")); d != "" {
		fmtScanInt(d, &durFallback)
	}
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	excludeID := ""
	if kind == "task" {
		excludeID = refID
		if existing, _ := h.repo.GetTask(refID); existing != nil && assignee == "" {
			assignee = strings.TrimSpace(existing.Assignee)
		}
	} else if kind == model.WBSourceAS || kind == model.WBSourceMaintenance {
		if existing, _ := h.repo.GetTaskBySource(kind, refID); existing != nil {
			excludeID = existing.TaskID
			if assignee == "" {
				assignee = strings.TrimSpace(existing.Assignee)
			}
		} else if assignee == "" {
			assignee = h.sourceAssignee(kind, refID)
		}
	}
	// 시작시각이 비면 같은 담당자 기준으로 빈 자리에 올린다(일·주 동일).
	if start == "" {
		s, e, err := h.repo.NextFreeSlotForAssignee(date, assignee, durFallback, excludeID)
		if err != nil {
			return err
		}
		if s == "" {
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=overlap"))
		}
		start = s
		_ = e
	}
	end, dur := parseDurationForm(start, c.FormValue("end_time"), c.FormValue("duration_min"), durFallback)
	if ok, err := h.repo.AssigneeTimeOverlaps(date, assignee, start, end, excludeID); err != nil {
		return err
	} else if ok {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=overlap"))
	}

	var openedTaskID string
	switch kind {
	case "task":
		if err := h.repo.PlaceTask(refID, date, start, end); err != nil {
			return err
		}
		openedTaskID = refID
	case model.WBSourceAS, model.WBSourceMaintenance:
		existing, err := h.repo.GetTaskBySource(kind, refID)
		if err != nil {
			return err
		}
		if existing != nil {
			if err := h.repo.PlaceTask(existing.TaskID, date, start, end); err != nil {
				return err
			}
			openedTaskID = existing.TaskID
			break
		}
		t, err := h.buildSourceTask(kind, refID, date, start, end, dur,
			assignee, strings.TrimSpace(c.FormValue("title")))
		if err != nil {
			return err
		}
		if err := h.repo.CreateTask(t); err != nil {
			return err
		}
		openedTaskID = t.TaskID
	default:
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=place"))
	}
	// AS·정기점검은 배치 후 업무 등록 화면으로 이동한다.
	if (kind == model.WBSourceAS || kind == model.WBSourceMaintenance) && openedTaskID != "" {
		back := redirectBack(c, "")
		return c.Redirect(http.StatusSeeOther,
			"/workboard/tasks/"+openedTaskID+"?ok=place&back="+url.QueryEscape(back))
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=place"))
}

func (h *WorkboardHandler) sourceAssignee(kind, refID string) string {
	switch kind {
	case model.WBSourceAS:
		if as, err := h.asRepo.GetByID(refID); err == nil && as != nil {
			return strings.TrimSpace(as.AssignedTo)
		}
	case model.WBSourceMaintenance:
		if v, err := h.mntRepo.GetVisit(refID); err == nil && v != nil {
			return strings.TrimSpace(v.Assignee)
		}
	}
	return ""
}

// buildSourceTask AS·정기점검 원본에서 업무 행을 만든다.
func (h *WorkboardHandler) buildSourceTask(kind, refID, date, start, end string, dur int, assignee, titleOverride string) (*model.WorkTask, error) {
	t := &model.WorkTask{
		WorkDate:    date,
		StartTime:   start,
		EndTime:     end,
		DurationMin: dur,
		Status:      model.WBTaskWaiting,
		Priority:    model.WBPriorityNormal,
		Assignee:    assignee,
		SourceType:  kind,
		SourceID:    refID,
	}
	switch kind {
	case model.WBSourceAS:
		t.WorkType = model.WBWorkAS
		as, err := h.asRepo.GetByID(refID)
		if err != nil || as == nil {
			return nil, echo.ErrNotFound
		}
		t.Title = model.FormatASWorkTitle(as.OrgName, as.ASNumber)
		t.Description = as.Symptom
		t.DueDate = as.VisitScheduledDate
		if t.DueDate == "" {
			t.DueDate = date
		}
		if t.Assignee == "" {
			t.Assignee = as.AssignedTo
		}
	case model.WBSourceMaintenance:
		t.WorkType = model.WBWorkMaintenance
		v, err := h.mntRepo.GetVisit(refID)
		if err != nil || v == nil {
			return nil, echo.ErrNotFound
		}
		name := v.ShortName
		if name == "" {
			name = v.OrgName
		}
		num := mntVisitNumber(*v)
		t.Title = model.FormatMaintenanceWorkTitle(name, num)
		t.Description = model.FormatMaintenanceDescription(v.ProductType, name)
		t.DueDate = v.VisitDate
		if t.DueDate == "" {
			t.DueDate = date
		}
		if t.Assignee == "" {
			t.Assignee = v.Assignee
		}
	}
	if titleOverride != "" {
		t.Title = titleOverride
	}
	return t, nil
}

// autoPlacePlanned 기간 안 예정일이 있는 미배치 업무(이미 work_tasks 행이 있는 것만)를 07:00부터 올린다.
// AS·정기점검 팔레트에서 새 업무를 자동 생성하지 않는다(×로 내린 뒤 즉시 다시 올라가는 것을 막기 위함).
func (h *WorkboardHandler) autoPlacePlanned(from, to string) (int, error) {
	n := 0
	all, err := h.repo.ListTasks()
	if err != nil {
		return 0, err
	}
	for _, t := range all {
		if t.StartTime != "" || t.Status == model.WBTaskComplete {
			continue
		}
		planned := t.DueDate
		if t.WorkDate != "" {
			planned = t.WorkDate
		}
		if planned == "" || planned < from || planned > to {
			continue
		}
		dur := t.DurationMin
		if dur <= 0 {
			dur = 30
		}
		start, end, err := h.repo.NextFreeSlotForAssignee(planned, t.Assignee, dur, t.TaskID)
		if err != nil {
			return n, err
		}
		if start == "" {
			continue // 같은 담당자 일정 포화 — 겹쳐 올리지 않음
		}
		if err := h.repo.PlaceTask(t.TaskID, planned, start, end); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Unschedule 시간표에서 카드를 내린다.
func (h *WorkboardHandler) Unschedule(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	taskID := strings.TrimSpace(c.FormValue("task_id"))
	if taskID == "" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=place"))
	}
	if err := h.repo.UnplaceTask(taskID); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=unplace"))
}

// addMinutesHHMM HH:MM 에 분을 더한다. 업무시간 끝을 넘지 않는다.
func addMinutesHHMM(hhmm string, add int) string {
	var hh, mm int
	if _, err := fmt.Sscanf(hhmm, "%d:%d", &hh, &mm); err != nil {
		return hhmm
	}
	total := hh*60 + mm + add
	if max := workdayEndHour * 60; total > max {
		total = max
	}
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

// todayWaitingFromPalette 오늘 예정일(PlannedDay)인 AS·정기점검·행정/지원을 오늘예정업무로 합친다.
func todayWaitingFromPalette(today string, as, mnt, admin []model.WBCard, seenSource, seenTask map[string]bool) []model.WorkTask {
	var out []model.WorkTask
	add := func(c model.WBCard) {
		if strings.TrimSpace(c.PlannedDay) != today {
			return
		}
		if c.TaskID != "" && seenTask[c.TaskID] {
			return
		}
		key := ""
		switch c.Kind {
		case model.WBSourceAS:
			key = model.WBSourceAS + ":" + c.RefID
		case model.WBSourceMaintenance:
			key = model.WBSourceMaintenance + ":" + c.RefID
		}
		if key != "" && seenSource[key] {
			return
		}
		t := model.WBCardAsWaitingTask(c)
		out = append(out, t)
		if c.TaskID != "" {
			seenTask[c.TaskID] = true
		}
		if key != "" {
			seenSource[key] = true
		}
	}
	for _, c := range as {
		add(c)
	}
	for _, c := range mnt {
		add(c)
	}
	for _, c := range admin {
		add(c)
	}
	return out
}

func (h *WorkboardHandler) boardData(c echo.Context, view string) (map[string]interface{}, error) {
	today := time.Now().Format(dateLayout)
	tasks, err := h.repo.ListTasksOnDate(today)
	if err != nil {
		return nil, err
	}

	// AS 원본 상태로 표시 상태 보정(보류·이관 → 검토 열)
	var asIDs []string
	for _, t := range tasks {
		if t.SourceType == model.WBSourceAS && t.SourceID != "" {
			asIDs = append(asIDs, t.SourceID)
		}
	}
	asStatus, _ := h.repo.ASStatusesByIDs(asIDs)

	var board []model.WorkTask
	seenSource := map[string]bool{}
	seenTask := map[string]bool{}
	for _, t := range tasks {
		if t.SourceType == model.WBSourceAS {
			mapped := model.MapASStatusToWB(asStatus[t.SourceID])
			if mapped == "" {
				continue // 취소 등 제외
			}
			t.Status = mapped
		}
		board = append(board, t)
		seenTask[t.TaskID] = true
		if t.SourceType != "" && t.SourceID != "" {
			seenSource[t.SourceType+":"+t.SourceID] = true
		}
	}

	// 오늘 예정인데 아직 work_tasks에 없거나 날짜가 비어 빠진 AS·정기점검·행정/지원 합치기
	asCards, mntCards, adminCards, err := h.registerPalette()
	if err != nil {
		return nil, err
	}
	board = append(board, todayWaitingFromPalette(today, asCards, mntCards, adminCards, seenSource, seenTask)...)

	projects, err := h.repo.ListProjects(false)
	if err != nil {
		return nil, err
	}

	summary := model.WBSummary{}
	byWorkType := map[string][]model.WorkTask{
		model.WBWorkMaintenance: {},
		model.WBWorkAS:          {},
		model.WBWorkAdmin:       {},
	}
	byStatus := map[string][]model.WorkTask{
		model.WBTaskWaiting:    {},
		model.WBTaskInProgress: {},
		model.WBTaskReview:     {},
		model.WBTaskComplete:   {},
	}
	for _, t := range board {
		summary.TotalTasks++
		statusBucket := model.WBKanbanBucket(t.Status)
		byStatus[statusBucket] = append(byStatus[statusBucket], t)
		switch statusBucket {
		case model.WBTaskWaiting:
			summary.Waiting++
		case model.WBTaskInProgress:
			summary.InProgress++
		case model.WBTaskReview:
			summary.Review++
		case model.WBTaskComplete:
			summary.Complete++
		}
		// 완료 건은 「완료」열에만 두고, 유형 열에는 미완료만 둔다
		if statusBucket != model.WBTaskComplete {
			typeBucket := model.WBWorkTypeBucket(t.WorkType)
			byWorkType[typeBucket] = append(byWorkType[typeBucket], t)
			switch typeBucket {
			case model.WBWorkAS:
				summary.AS++
			case model.WBWorkMaintenance:
				summary.Maintenance++
			default:
				summary.Admin++
			}
		}
		if t.Priority == model.WBPriorityUrgent {
			summary.Urgent++
		}
	}
	for _, p := range projects {
		if p.Status != model.WBProjectArchived {
			summary.ProjectCount++
		}
	}
	assignees, _ := h.userRepo.ListAssignable()
	customers, _ := h.customerRepo.ListAll()
	contractTypes, _ := h.codeRepo.ActiveByGroup("project_contract_type")
	billingTypes, _ := h.codeRepo.ActiveByGroup("project_billing_type")

	modalRedirect := "/workboard/kanban"
	if view == "list" {
		modalRedirect = "/workboard/tasks"
	}
	return map[string]interface{}{
		"Title":         "일일 업무 현황",
		"Active":        "work_plan",
		"View":          view,
		"Summary":       summary,
		"Tasks":         board,
		"ByWorkType":    byWorkType,
		"ByStatus":      byStatus,
		"Projects":      projects,
		"Assignees":     assignees,
		"Customers":     customers,
		"ContractTypes": contractTypes,
		"BillingTypes":  billingTypes,
		"ASCards":       asCards,
		"MntCards":      mntCards,
		"Date":          today,
		"TodayLabel":    today,
		"ModalRedirect": modalRedirect,
		"CanWrite":      canWriteWorkboard(c),
		"FlashOK":       c.QueryParam("ok"),
		"FlashErr":      c.QueryParam("err"),
	}, nil
}

func (h *WorkboardHandler) CreateProject(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	contactID := strings.TrimSpace(c.FormValue("contact_id"))
	if contactID == "__new__" {
		contactID = ""
	}
	orderingPartyID := strings.TrimSpace(c.FormValue("ordering_party_id"))
	orderingParty := ""
	if orderingPartyID == "custom" {
		orderingPartyID = ""
		orderingParty = strings.TrimSpace(c.FormValue("ordering_party_custom"))
	}
	contractType := strings.TrimSpace(c.FormValue("contract_type"))
	if contractType == "custom" {
		contractType = strings.TrimSpace(c.FormValue("contract_type_custom"))
	}
	billingType := strings.TrimSpace(c.FormValue("billing_type"))
	if billingType == "custom" {
		billingType = strings.TrimSpace(c.FormValue("billing_type_custom"))
	}
	p := &model.WorkProject{
		Name:            strings.TrimSpace(c.FormValue("name")),
		OrderingPartyID: orderingPartyID,
		OrderingParty:   orderingParty,
		CustomerID:      strings.TrimSpace(c.FormValue("customer_id")),
		ContractType:    contractType,
		BillingType:     billingType,
		StartDate:       strings.TrimSpace(c.FormValue("start_date")),
		EndDate:         strings.TrimSpace(c.FormValue("end_date")),
		Notes:           strings.TrimSpace(c.FormValue("notes")),
		ContactID:       contactID,
		Color:           strings.TrimSpace(c.FormValue("color")),
	}
	if p.Name == "" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=name"))
	}
	if err := h.repo.CreateProject(p); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=project"))
}

func (h *WorkboardHandler) CreateTask(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	workType := strings.TrimSpace(c.FormValue("work_type"))
	sourceID := strings.TrimSpace(c.FormValue("source_id"))

	workDate, dueDate, start, end, dur, schedOK := parseRequiredSchedule(c)
	if !schedOK {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=time"))
	}

	// AS·정기점검 일일업무 — 원본을 골라 업무 고유번호를 부여하고 업무 화면으로 이동
	if workType == model.WBWorkAS || workType == model.WBWorkMaintenance {
		kind := workType
		if sourceID == "" {
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=task"))
		}
		if existing, _ := h.repo.GetTaskBySource(kind, sourceID); existing != nil {
			if err := h.repo.PlaceTask(existing.TaskID, workDate, start, end); err != nil {
				return err
			}
			if dueDate != "" {
				_ = h.repo.SetTaskDueDate(existing.TaskID, dueDate)
			}
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=place"))
		}
		t, err := h.buildSourceTask(kind, sourceID, workDate, start, end, dur,
			strings.TrimSpace(c.FormValue("assignee")), "")
		if err != nil {
			return err
		}
		if dueDate != "" {
			t.DueDate = dueDate
		}
		if t.DueDate == "" {
			t.DueDate = workDate
		}
		if st := strings.TrimSpace(c.FormValue("status")); st != "" {
			t.Status = st
		}
		if pr := strings.TrimSpace(c.FormValue("priority")); pr != "" {
			t.Priority = pr
		}
		if err := h.repo.CreateTask(t); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=task"))
	}

	progress := 0
	if p := strings.TrimSpace(c.FormValue("progress")); p != "" {
		fmtScanInt(p, &progress)
	}
	if progress > 100 {
		progress = 100
	}
	if workType != model.WBWorkSupport {
		workType = model.WBWorkAdmin
	}
	if dueDate == "" {
		dueDate = workDate
	}
	t := &model.WorkTask{
		WorkType:    workType,
		ProjectID:   strings.TrimSpace(c.FormValue("project_id")),
		Title:       strings.TrimSpace(c.FormValue("title")),
		Description: strings.TrimSpace(c.FormValue("description")),
		DueDate:     dueDate,
		WorkDate:    workDate,
		StartTime:   start,
		EndTime:     end,
		DurationMin: dur,
		Status:      strings.TrimSpace(c.FormValue("status")),
		Priority:    strings.TrimSpace(c.FormValue("priority")),
		Assignee:    strings.TrimSpace(c.FormValue("assignee")),
		Tags:        strings.TrimSpace(c.FormValue("tags")),
		Progress:    progress,
	}
	if t.Title == "" || t.DueDate == "" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=task"))
	}
	if t.WorkType == model.WBWorkSupport && t.ProjectID == "" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=project_required"))
	}
	if err := h.repo.CreateTask(t); err != nil {
		return err
	}
	if mode := strings.TrimSpace(c.FormValue("sub_mode")); mode != "" {
		dates, err := collectDatesFromForm(mode, c.FormValue("daily_from"), c.FormValue("daily_to"), c.FormValue("sub_dates"))
		if err == nil && len(dates) > 0 {
			_, _ = h.repo.CreateSubtasks(t, dates)
		}
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=task"))
}

// parseRequiredSchedule 업무 배정일·시작·종료(필수). 예정일(due)은 비어 있을 수 있다(원본 예정일 사용).
func parseRequiredSchedule(c echo.Context) (workDate, dueDate, start, end string, dur int, ok bool) {
	workDate = strings.TrimSpace(c.FormValue("work_date"))
	dueDate = strings.TrimSpace(c.FormValue("due_date"))
	start = strings.TrimSpace(c.FormValue("start_time"))
	end = strings.TrimSpace(c.FormValue("end_time"))
	dur = 30
	if d := strings.TrimSpace(c.FormValue("duration_min")); d != "" {
		fmtScanInt(d, &dur)
	}
	if start == "" || end == "" {
		return "", "", "", "", 0, false
	}
	end, dur = parseDurationForm(start, end, c.FormValue("duration_min"), dur)
	if workDate == "" {
		workDate = dueDate
	}
	if workDate == "" {
		return "", "", "", "", 0, false
	}
	return workDate, dueDate, start, end, dur, true
}

// UpdateTask 업무 화면에서 배정일·시간·상태 등 수정
func (h *WorkboardHandler) UpdateTask(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	existing, err := h.repo.GetTask(id)
	if err != nil || existing == nil {
		return echo.ErrNotFound
	}
	workDate, dueDate, start, end, dur, schedOK := parseRequiredSchedule(c)
	if !schedOK {
		return c.Redirect(http.StatusSeeOther,
			"/workboard/tasks/"+id+"?err=time&back="+url.QueryEscape(strings.TrimSpace(c.FormValue("back"))))
	}
	progress := existing.Progress
	if p := strings.TrimSpace(c.FormValue("progress")); p != "" {
		fmtScanInt(p, &progress)
	}
	if progress > 100 {
		progress = 100
	}
	workType := existing.WorkType
	projectID := existing.ProjectID
	title := strings.TrimSpace(c.FormValue("title"))
	if title == "" {
		title = existing.Title
	}
	// AS·점검은 구분·원본 고정. 행정/지원만 구분·사업명 변경
	if existing.SourceType == "" {
		workType = strings.TrimSpace(c.FormValue("work_type"))
		if workType != model.WBWorkSupport {
			workType = model.WBWorkAdmin
		}
		projectID = strings.TrimSpace(c.FormValue("project_id"))
		if workType == model.WBWorkSupport && projectID == "" {
			return c.Redirect(http.StatusSeeOther,
				"/workboard/tasks/"+id+"?err=project_required&back="+url.QueryEscape(strings.TrimSpace(c.FormValue("back"))))
		}
		if workType == model.WBWorkAdmin {
			projectID = ""
		}
	}
	t := &model.WorkTask{
		TaskID:      existing.TaskID,
		WorkType:    workType,
		ProjectID:   projectID,
		Title:       title,
		Description: strings.TrimSpace(c.FormValue("description")),
		DueDate:     dueDate,
		WorkDate:    workDate,
		StartTime:   start,
		EndTime:     end,
		DurationMin: dur,
		Status:      strings.TrimSpace(c.FormValue("status")),
		Priority:    strings.TrimSpace(c.FormValue("priority")),
		Assignee:    strings.TrimSpace(c.FormValue("assignee")),
		Tags:        strings.TrimSpace(c.FormValue("tags")),
		Progress:    progress,
		SourceType:  existing.SourceType,
		SourceID:    existing.SourceID,
		ParentTaskID: existing.ParentTaskID,
	}
	if t.DueDate == "" {
		t.DueDate = workDate
	}
	if t.DueDate == "" {
		return c.Redirect(http.StatusSeeOther,
			"/workboard/tasks/"+id+"?err=task&back="+url.QueryEscape(strings.TrimSpace(c.FormValue("back"))))
	}
	if err := h.repo.UpdateTask(t); err != nil {
		return err
	}
	back := strings.TrimSpace(c.FormValue("back"))
	loc := "/workboard/tasks/" + id + "?ok=saved"
	if back != "" {
		loc += "&back=" + url.QueryEscape(back)
	}
	return c.Redirect(http.StatusSeeOther, loc)
}

// ShowTask 업무 상세 — AS/점검 원본 링크·하위업무 관리
func (h *WorkboardHandler) ShowTask(c echo.Context) error {
	id := c.Param("id")
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	children, _ := h.repo.ListChildren(id)
	var sourceHref, sourceLabel string
	var actionHref, actionLabel string
	var canAction, asClosed bool
	var asSource *model.ASReceipt
	var completeLocal string
	switch t.SourceType {
	case model.WBSourceAS:
		sourceHref = "/as/" + t.SourceID
		sourceLabel = t.SourceID
		actionHref = "/as/" + t.SourceID + "/action"
		actionLabel = "조치"
		canAction = canReceiveAS(c) || canWriteWorkboard(c) || canProcessAS(c)
		if as, _ := h.asRepo.GetByID(t.SourceID); as != nil {
			asSource = as
			if as.ASNumber != "" {
				sourceLabel = as.ASNumber
			}
			asClosed = isASClosedStatus(as.Status)
			if asClosed {
				actionLabel = "조치 완료"
			}
			if as.CompleteDatetime != nil && !as.CompleteDatetime.IsZero() {
				completeLocal = as.CompleteDatetime.Format("2006-01-02 15:04")
			}
		}
	case model.WBSourceMaintenance:
		if v, _ := h.mntRepo.GetVisit(t.SourceID); v != nil {
			sourceHref = mntVisitHref(*v)
			sourceLabel = mntVisitNumber(*v)
		} else {
			sourceHref = "/maintenance"
			sourceLabel = t.SourceID
		}
	}
	var parent *model.WorkTask
	if t.ParentTaskID != "" {
		parent, _ = h.repo.GetTask(t.ParentTaskID)
	}
	assignees, _ := h.userRepo.ListAssignable()
	projects, _ := h.repo.ListProjects(false)
	back := strings.TrimSpace(c.QueryParam("back"))
	if back == "" {
		back = "/workboard/register"
	}
	return c.Render(http.StatusOK, "workboard/task_show.html", map[string]interface{}{
		"Title":         t.Title,
		"Active":        "work_register",
		"Task":          t,
		"Children":      children,
		"Parent":        parent,
		"SourceHref":    sourceHref,
		"SourceLabel":   sourceLabel,
		"ActionHref":    actionHref,
		"ActionLabel":   actionLabel,
		"CanAction":     canAction,
		"ASClosed":      asClosed,
		"ASSource":      asSource,
		"CompleteLocal": completeLocal,
		"Assignees":     assignees,
		"Projects":      projects,
		"CanWrite":      canWriteWorkboard(c),
		"FlashOK":       c.QueryParam("ok"),
		"FlashErr":      c.QueryParam("err"),
		"Today":         time.Now().Format(dateLayout),
		"BackURL":       back,
	})
}

// CreateSubtasks 행정/지원 업무에 일자별 하위업무 추가
func (h *WorkboardHandler) CreateSubtasks(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	parent, err := h.repo.GetTask(c.Param("id"))
	if err != nil || parent == nil {
		return echo.ErrNotFound
	}
	if parent.SourceType != "" {
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+parent.TaskID+"?err=sub")
	}
	dates, err := collectDatesFromForm(
		c.FormValue("sub_mode"), c.FormValue("daily_from"), c.FormValue("daily_to"), c.FormValue("sub_dates"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+parent.TaskID+"?err=sub")
	}
	n, err := h.repo.CreateSubtasks(parent, dates)
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/workboard/tasks/%s?ok=sub&n=%d", parent.TaskID, n))
}

func canWriteWorkboard(c echo.Context) bool {
	r := currentRole(c)
	return r == "admin" || r == "receipt" || r == "tech"
}

func redirectBack(c echo.Context, q string) string {
	back := strings.TrimSpace(c.FormValue("redirect"))
	if back == "" {
		back = "/workboard/kanban"
	}
	if q != "" {
		if strings.Contains(back, "?") {
			return back + "&" + q
		}
		return back + "?" + q
	}
	return back
}

func fmtScanInt(s string, out *int) {
	var n int
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int(ch-'0')
		}
	}
	*out = n
}
