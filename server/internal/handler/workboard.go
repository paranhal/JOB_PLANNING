package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
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
	settingsRepo *repository.SettingsRepo
	unlockRepo   *repository.ASUnlockRepo
	attach       *AttachmentHandler
}

func NewWorkboardHandler(
	repo *repository.WBRepo,
	userRepo *repository.UserRepo,
	customerRepo *repository.CustomerRepo,
	contactRepo *repository.ContactRepo,
	codeRepo *repository.CodeRepo,
	asRepo *repository.ASRepo,
	mntRepo *repository.MaintenanceRepo,
	settingsRepo *repository.SettingsRepo,
	unlockRepo *repository.ASUnlockRepo,
) *WorkboardHandler {
	return &WorkboardHandler{
		repo: repo, userRepo: userRepo,
		customerRepo: customerRepo, contactRepo: contactRepo, codeRepo: codeRepo,
		asRepo: asRepo, mntRepo: mntRepo,
		settingsRepo: settingsRepo, unlockRepo: unlockRepo,
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
	Label       string // 월 / 7/1주
	Sub         string // 8/4
	Date        string // 이 열에 카드를 놓았을 때 배치될 날짜
	From        string // 열이 포함하는 기간 (월간은 한 주)
	To          string
	IsToday     bool
	IsOffDay    bool // 근무일이 아님
	IsSunday    bool
	IsSaturday  bool
	HolidayName string
	HolidayKind string
	DateTitle   string
	DayClass    string
	CellClass   string
	DayStyle    string
	CellStyle   string
	Leaves      LeaveBadgeGroup
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

	// 정기점검 방문 ↔ work_tasks 동기화(기존 데이터 보정)
	h.syncMaintenanceTasks()

	// 예정일이 있는 미배치 업무를 해당 날짜 07:00부터 차례로 올린다.
	// 지난날 잠금이 켜진 경우에만 오늘 이후만 자동 배치한다.
	if c.QueryParam("auto") != "0" {
		autoFrom := period.From
		if registerPastDayLockEnabled && autoFrom < today {
			autoFrom = today
		}
		if autoFrom <= period.To {
			if _, err := h.autoPlacePlanned(autoFrom, period.To); err != nil {
				return err
			}
		}
	}

	placed, err := h.repo.ListTasksBetween(period.From, period.To)
	if err != nil {
		return err
	}
	// 시각 미기재 확정 건도 시간표에 보이도록 표시용 시각만 채운다.
	placed = ensureTimelineDisplayTimes(placed)

	assigneeFilter := normalizeAssigneeFilter(c.QueryParam("assignee"))
	projectFilter := strings.TrimSpace(c.QueryParam("project"))
	memberMap := map[string][]model.WorkTaskMember{}
	if ids := taskIDsOf(placed); len(ids) > 0 {
		if m, err := h.repo.ListMembersByTaskIDs(ids); err == nil {
			memberMap = m
		}
	}
	if view == regViewDay {
		placed = filterTasksByParticipants(placed, memberMap, assigneeFilter)
	} else {
		placed = filterTasksByAssignee(placed, assigneeFilter)
	}
	placed = filterTasksByProject(placed, projectFilter)
	asCards, mntCards, adminCards, err := h.registerPalette()
	if err != nil {
		return err
	}
	asCards = filterCardsByAssignee(asCards, assigneeFilter)
	mntCards = filterCardsByAssignee(mntCards, assigneeFilter)
	adminCards = filterCardsByAssignee(adminCards, assigneeFilter)
	asCards = filterCardsByProject(asCards, projectFilter)
	mntCards = filterCardsByProject(mntCards, projectFilter)
	adminCards = filterCardsByProject(adminCards, projectFilter)

	projects, _ := h.repo.ListProjects(true)
	assignees, _ := h.userRepo.ListAssignable()
	customers, _ := h.customerRepo.ListAll()
	var colorNames []string
	for _, u := range assignees {
		colorNames = append(colorNames, u.FullName)
	}
	for _, t := range placed {
		colorNames = append(colorNames, t.Assignee)
	}
	for _, c := range asCards {
		colorNames = append(colorNames, c.Assignee)
	}
	for _, c := range mntCards {
		colorNames = append(colorNames, c.Assignee)
	}
	for _, c := range adminCards {
		colorNames = append(colorNames, c.Assignee)
	}
	model.SetAssigneeColorOrder(colorNames)

	slotTimes := registerSlotTimes()
	dateStr := base.Format(dateLayout)
	var dayCols []RegisterDayColumn
	if view == regViewDay {
		var order []string
		for _, u := range assignees {
			order = append(order, u.FullName)
		}
		sort.Strings(order)
		dateCol := RegisterColumn{Date: dateStr, From: dateStr, To: dateStr}
		if len(period.Columns) > 0 {
			dateCol = period.Columns[0]
		}
		dayCols = buildRegisterAssigneeColumnsMembers(dateCol, placed, h.taskCard, order, assigneeFilter, memberMap)
	} else {
		dayCols = buildRegisterDayColumns(period.Columns, placed, h.taskCard)
	}
	holi := loadHolidays(h.mntRepo.Holidays(), period.From, period.To)
	var leaveItems []model.StaffLeave
	if h.mntRepo != nil {
		if lr := h.mntRepo.Leaves(); lr != nil {
			leaveItems, _ = lr.ListInRange(period.From, period.To)
		}
	}
	leaveMap := leaveBadgesByDate(leaveItems, currentUserID(c))
	decorateRegisterColumns(period.Columns, today, holi)
	attachColumnLeaves(period.Columns, leaveMap)
	decorateRegisterDayColumns(dayCols, today, holi)
	var dayLeaveNames []string
	leaveHit := map[string]bool{}
	for _, b := range leaveMap[dateStr] {
		if b.Name != "" && !leaveHit[b.Name] {
			leaveHit[b.Name] = true
			dayLeaveNames = append(dayLeaveNames, b.Name)
		}
	}
	if view == regViewDay {
		for i := range dayCols {
			if leaveHit[strings.TrimSpace(dayCols[i].Assignee)] {
				dayCols[i].OnLeave = true
			}
		}
	} else {
		for i := range dayCols {
			if dayCols[i].HolidayName == "" {
				dayCols[i].Leaves = groupLeaveBadges(leaveMap[dayCols[i].Date])
			}
		}
	}
	// 드롭 존용 빈 행(카드는 DayColumns.Blocks에 절대 배치)
	rows := h.buildRegisterRows(period.Columns, nil)
	assigneeLegend := buildAssigneeLegend(placed, assignees, asCards, mntCards, adminCards)
	gridHeightStyle, slotTopStyles := registerGridStyles(len(slotTimes))
	var monthWeeks [][]RegisterMonthDay
	if view == regViewMonth {
		monthWeeks = buildRegisterMonthWeeks(base, today, placed, h.taskCard)
		decorateMonthWeeks(monthWeeks, today, holi, leaveMap)
	}

	filterQ := registerFilterQuery(assigneeFilter, projectFilter)
	regBase := registerURLWithProject(view, dateStr, assigneeFilter, projectFilter)

	writeBase := canWriteWorkboard(c)
	dayPast := registerPastDayLockEnabled && view == regViewDay && dateStr < today
	unlocked, unlockExp := h.registerDayUnlocked(c, dateStr)
	canWrite := writeBase && (!dayPast || unlocked)
	canUnlock := registerPastDayLockEnabled && writeBase && isAdminRole(c) && dayPast && !unlocked
	unlockExpLabel := ""
	if !unlockExp.IsZero() {
		unlockExpLabel = unlockExp.Format("15:04")
	}

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
		"Customers":       customers,
		"AssigneeFilter":  assigneeFilter,
		"ProjectFilter":   projectFilter,
		"AssigneeQ":       filterQ, // 하위호환: 템플릿 링크용 (&assignee=&project=)
		"FilterQ":         filterQ,
		"ExtraAssignees":  extraAssigneesForDay(view, assigneeFilter, assignees, dayCols),
		"DayLeaveNames":   dayLeaveNames,
		"AssigneeColMin":  registerAssigneeColMinPx,
		"ModalRedirect":   regBase,
		"CanWrite":        canWrite,
		"PastDayLocked":   dayPast && !unlocked,
		"CanUnlockPast":   canUnlock,
		"EditUnlocked":    unlocked && dayPast,
		"UnlockExpires":   unlockExpLabel,
		"UnlockRedirect":  regBase,
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
	return registerFilterQuery(assignee, "")
}

func registerFilterQuery(assignee, project string) string {
	v := url.Values{}
	if assignee != "" {
		v.Set("assignee", assignee)
	}
	if project != "" {
		v.Set("project", project)
	}
	enc := v.Encode()
	if enc == "" {
		return ""
	}
	return "&" + enc
}

func registerURL(view, date, assignee string) string {
	return registerURLWithProject(view, date, assignee, "")
}

func registerURLWithProject(view, date, assignee, project string) string {
	return fmt.Sprintf("/workboard/register?view=%s&date=%s%s", view, date, registerFilterQuery(assignee, project))
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

func filterTasksByParticipants(tasks []model.WorkTask, members map[string][]model.WorkTaskMember, assignee string) []model.WorkTask {
	assignee = strings.TrimSpace(assignee)
	if assignee == "" {
		return tasks
	}
	var out []model.WorkTask
	for _, t := range tasks {
		if strings.TrimSpace(t.Assignee) == assignee {
			out = append(out, t)
			continue
		}
		for _, m := range members[t.TaskID] {
			if strings.TrimSpace(m.Assignee) == assignee {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

func filterTasksByProject(tasks []model.WorkTask, projectID string) []model.WorkTask {
	if projectID == "" {
		return tasks
	}
	var out []model.WorkTask
	for _, t := range tasks {
		if strings.TrimSpace(t.ProjectID) == projectID {
			out = append(out, t)
		}
	}
	return out
}

func filterCardsByProject(cards []model.WBCard, projectID string) []model.WBCard {
	if projectID == "" {
		return cards
	}
	var out []model.WBCard
	for _, c := range cards {
		if strings.TrimSpace(c.ProjectID) == projectID {
			out = append(out, c)
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
				card.ActionHref = mntVisitActionHref(*v)
				card.ProductType = v.ProductType
			}
		}
		if card.ProductType == "" {
			card.ProductType = productFromMaintDesc(t.Description)
		}
	}
	return card
}

// syncMaintenanceTasks 정기점검 일정(방문)을 기준으로 일일업무의 상태·일자·담당자를 맞춘다.
// 점검 일정 화면에서 날짜를 바꿔도 일일 업무 등록에 그대로 보이도록 화면 진입 때마다 보정한다.
func (h *WorkboardHandler) syncMaintenanceTasks() {
	if h == nil || h.repo == nil {
		return
	}
	h.repo.SyncMaintenanceBoard()
}

// syncVisitDateFromTask 시간표에서 정기점검 카드를 다른 날짜로 옮기면 점검 일정도 함께 옮긴다.
func (h *WorkboardHandler) syncVisitDateFromTask(sourceType, sourceID, workDate string) {
	if h == nil || h.mntRepo == nil || sourceType != model.WBSourceMaintenance {
		return
	}
	_ = h.mntRepo.SetVisitDate(sourceID, workDate)
}

// registerPalette 아직 시간표에 올리지 않은 AS·정기점검·행정관련 업무 카드
func (h *WorkboardHandler) registerPalette() (as, mnt, admin []model.WBCard, err error) {
	tasks, err := h.repo.ListTasks()
	if err != nil {
		return nil, nil, nil, err
	}
	usedSource := map[string]bool{}
	for _, t := range tasks {
		// 일일 업무로 일자(WorkDate)가 확정된 원본은 우측 대기 목록에서 뺀다.
		// DueDate만 있고 WorkDate가 비면 아직 미확정이므로 팔레트에 남긴다.
		if t.SourceType != "" && t.SourceID != "" && strings.TrimSpace(t.WorkDate) != "" {
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
	if !h.canEditRegisterDate(c, date) {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=past"))
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
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+url.QueryEscape(assigneeOverlapMessage(assignee, "07:00", "20:00"))))
		}
		start = s
		_ = e
	}
	end, dur := parseDurationForm(start, c.FormValue("end_time"), c.FormValue("duration_min"), durFallback)
	occ := []string{assignee}
	if excludeID != "" {
		if ms, err := h.repo.ListMembers(excludeID); err == nil {
			var supports []model.WorkTaskMember
			for _, m := range ms {
				if m.IsSupport() {
					supports = append(supports, m)
				}
			}
			occ = occupancyNames(assignee, supports)
		}
	}
	if msg := h.firstParticipantOverlap(date, start, end, excludeID, occ); msg != "" {
		return overlapRedirect(c, msg)
	}

	fromColumn := strings.TrimSpace(c.FormValue("assignee_from_column")) == "1"

	var openedTaskID string
	switch kind {
	case "task":
		if err := h.repo.PlaceTask(refID, date, start, end); err != nil {
			return err
		}
		if t, _ := h.repo.GetTask(refID); t != nil {
			h.syncVisitDateFromTask(t.SourceType, t.SourceID, date)
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
			h.syncVisitDateFromTask(kind, refID, date)
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
		h.syncVisitDateFromTask(kind, refID, date)
		openedTaskID = t.TaskID
	default:
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=place"))
	}
	if fromColumn && openedTaskID != "" {
		if err := h.repo.SetTaskAssignee(openedTaskID, assignee); err != nil {
			return err
		}
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
		if v.Completed {
			t.Status = model.WBTaskComplete
			t.Progress = 100
		}
	}
	if titleOverride != "" {
		t.Title = titleOverride
	}
	return t, nil
}

// autoPlacePlanned 기간 안 배정일(work_date)이 있고 시간만 비어 있는 업무를 07:00부터 올린다.
// ×로 배정일을 비운 건·아직 올리지 않은 건은 대기열에 두고 다시 올리지 않는다.
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
		planned := strings.TrimSpace(t.WorkDate)
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
	if t, _ := h.repo.GetTask(taskID); t != nil {
		d := t.WorkDate
		if d == "" {
			d = t.DueDate
		}
		if d != "" && !h.canEditRegisterDate(c, d) {
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=past"))
		}
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

// waitingActionChecks 다음 확인일이 오늘인 회신 대기 행동 → 칸반 「할 일」. 시간표에는 올리지 않는다(§13.8).
func waitingActionChecks(today string, repo *repository.WBRepo, seenTask map[string]bool) []model.WorkTask {
	if repo == nil {
		return nil
	}
	actions, err := repo.ListWaitingActionsDueCheck(today)
	if err != nil || len(actions) == 0 {
		return nil
	}
	var out []model.WorkTask
	for _, a := range actions {
		parent, _ := repo.GetTask(a.TaskID)
		if parent == nil || !model.IsAdminGTDTask(*parent) {
			continue
		}
		if parent.Status == model.WBTaskCancelled || parent.Status == model.WBTaskComplete {
			continue
		}
		title := "확인: " + a.Title
		if a.WaitParty != "" {
			title += " · " + a.WaitParty
		}
		out = append(out, model.WorkTask{
			TaskID:      "wa:" + a.ActionID,
			WorkType:    parent.WorkType,
			Title:       title,
			Description: a.WaitRequest,
			DueDate:     today,
			WorkDate:    today,
			Status:      model.WBTaskWaiting,
			Priority:    model.WBPriorityNormal,
			Assignee:    firstNonEmpty(a.Assignee, parent.Assignee),
			BoardHref:   "/workboard/tasks/" + parent.TaskID,
		})
	}
	return out
}

func (h *WorkboardHandler) boardData(c echo.Context, view string) (map[string]interface{}, error) {
	today := time.Now().Format(dateLayout)
	tasks, err := h.repo.ListTasksOnDate(today)
	if err != nil {
		return nil, err
	}

	_ = h.repo.BackfillMaintenanceTaskStatuses()

	// AS·정기점검 원본 상태로 표시 상태 보정
	var asIDs, mntIDs []string
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
	asStatus, _ := h.repo.ASStatusesByIDs(asIDs)
	mntDone, _ := h.repo.MaintenanceCompletedByIDs(mntIDs)

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
		} else if t.SourceType == model.WBSourceMaintenance && mntDone[t.SourceID] {
			t.Status = model.WBTaskComplete
		}
		if model.IsAdminGTDTask(t) && t.Status == model.WBTaskCancelled {
			continue
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
	board = append(board, waitingActionChecks(today, h.repo, seenTask)...)

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
		model.WBTaskComplete:   {},
	}
	for _, t := range board {
		statusBucket := model.WBKanbanBucket(t.Status)
		if statusBucket == "" {
			continue
		}
		summary.TotalTasks++
		byStatus[statusBucket] = append(byStatus[statusBucket], t)
		switch statusBucket {
		case model.WBTaskWaiting:
			summary.Waiting++
		case model.WBTaskInProgress:
			summary.InProgress++
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
	p := ParseWorkProjectForm(c)
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
	if !h.canEditRegisterDate(c, workDate) {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=past"))
	}

	// AS·정기점검 일일업무 — 원본을 골라 업무 고유번호를 부여하고 업무 화면으로 이동
	if workType == model.WBWorkAS || workType == model.WBWorkMaintenance {
		kind := workType
		if sourceID == "" {
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=task"))
		}
		if existing, _ := h.repo.GetTaskBySource(kind, sourceID); existing != nil {
			owner := strings.TrimSpace(c.FormValue("assignee"))
			if owner == "" {
				owner = strings.TrimSpace(existing.Assignee)
			}
			supports := parseSupportMembers(c, owner)
			if msg := h.firstParticipantOverlap(workDate, start, end, existing.TaskID, occupancyNames(owner, supports)); msg != "" {
				return overlapRedirect(c, msg)
			}
			if err := h.repo.PlaceTask(existing.TaskID, workDate, start, end); err != nil {
				return err
			}
			if dueDate != "" {
				_ = h.repo.SetTaskDueDate(existing.TaskID, dueDate)
			}
			if owner != strings.TrimSpace(existing.Assignee) {
				_ = h.repo.SetTaskAssignee(existing.TaskID, owner)
			}
			_ = h.saveSupportMembers(c, existing.TaskID, owner)
			h.syncVisitDateFromTask(kind, sourceID, workDate)
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
		if msg := h.firstParticipantOverlap(workDate, start, end, "", occupancyNames(t.Assignee, parseSupportMembers(c, t.Assignee))); msg != "" {
			return overlapRedirect(c, msg)
		}
		if err := h.repo.CreateTask(t); err != nil {
			return err
		}
		_ = h.saveSupportMembers(c, t.TaskID, t.Assignee)
		h.syncVisitDateFromTask(kind, sourceID, workDate)
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
	applyCustomerForm(t, c)
	if t.Title == "" || t.DueDate == "" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=task"))
	}
	if t.WorkType == model.WBWorkSupport && t.ProjectID == "" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=project_required"))
	}
	supports := parseSupportMembers(c, t.Assignee)
	if len(supports) > 0 && t.Assignee == "" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=owner"))
	}
	if msg := h.firstParticipantOverlap(t.WorkDate, t.StartTime, t.EndTime, "", occupancyNames(t.Assignee, supports)); msg != "" {
		return overlapRedirect(c, msg)
	}
	if err := h.repo.CreateTask(t); err != nil {
		return err
	}
	_ = h.saveSupportMembers(c, t.TaskID, t.Assignee)
	if mode := strings.TrimSpace(c.FormValue("sub_mode")); mode != "" {
		dates, err := collectDatesFromForm(mode, c.FormValue("daily_from"), c.FormValue("daily_to"), c.FormValue("sub_dates"))
		if err == nil && len(dates) > 0 {
			_, _ = h.repo.CreateSubtasks(t, dates)
		}
	}
	if applied, code := applyRecurrenceFromForm(c, h.repo, t, h.holidayYearMissing()); code != "" {
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t.TaskID+"?err="+code)
	} else if applied {
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t.TaskID+"?ok=rec_gen")
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=task"))
}

// parseRequiredSchedule 업무 배정일·시작·종료(필수). 예정일(due)은 비어 있을 수 있다(원본 예정일 사용).
func parseRequiredSchedule(c echo.Context) (workDate, dueDate, start, end string, dur int, ok bool) {
	workDate = strings.TrimSpace(c.FormValue("work_date"))
	dueDate = strings.TrimSpace(c.FormValue("due_date"))
	if workDate != "" {
		if d, err := model.ParseAppDate(workDate); err != nil {
			return "", "", "", "", 0, false
		} else {
			workDate = d
		}
	}
	if dueDate != "" {
		if d, err := model.ParseAppDate(dueDate); err != nil {
			return "", "", "", "", 0, false
		} else {
			dueDate = d
		}
	}
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
	adminGTD := model.IsAdminGTDTask(*existing)
	if !schedOK {
		if !adminGTD {
			return c.Redirect(http.StatusSeeOther,
				"/workboard/tasks/"+id+"?err=time&back="+url.QueryEscape(strings.TrimSpace(c.FormValue("back"))))
		}
		dueDate = strings.TrimSpace(c.FormValue("due_date"))
		workDate = strings.TrimSpace(c.FormValue("work_date"))
		start = strings.TrimSpace(c.FormValue("start_time"))
		end = strings.TrimSpace(c.FormValue("end_time"))
		dur = existing.DurationMin
		if start != "" && end != "" {
			end, dur = parseDurationForm(start, end, c.FormValue("duration_min"), dur)
		} else {
			start, end, dur = existing.StartTime, existing.EndTime, existing.DurationMin
			if workDate == "" {
				workDate = existing.WorkDate
			}
		}
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
	// AS는 구분·원본 고정. 정기점검은 사업명 변경 가능. 행정/지원은 구분·사업명 변경
	if existing.SourceType == model.WBSourceMaintenance {
		projectID = strings.TrimSpace(c.FormValue("project_id"))
	} else if existing.SourceType == "" {
		workType = strings.TrimSpace(c.FormValue("work_type"))
		if workType != model.WBWorkSupport {
			workType = model.WBWorkAdmin
		}
		projectID = strings.TrimSpace(c.FormValue("project_id"))
		if workType == model.WBWorkSupport && projectID == "" {
			return c.Redirect(http.StatusSeeOther,
				"/workboard/tasks/"+id+"?err=project_required&back="+url.QueryEscape(strings.TrimSpace(c.FormValue("back"))))
		}
	}
	t := &model.WorkTask{
		TaskID:       existing.TaskID,
		WorkType:     workType,
		ProjectID:    projectID,
		Title:        title,
		Description:  strings.TrimSpace(c.FormValue("description")),
		DueDate:      dueDate,
		WorkDate:     workDate,
		StartTime:    start,
		EndTime:      end,
		DurationMin:  dur,
		Status:       strings.TrimSpace(c.FormValue("status")),
		Priority:     strings.TrimSpace(c.FormValue("priority")),
		Assignee:     strings.TrimSpace(c.FormValue("assignee")),
		Tags:         strings.TrimSpace(c.FormValue("tags")),
		Progress:     progress,
		SourceType:   existing.SourceType,
		SourceID:     existing.SourceID,
		ParentTaskID: existing.ParentTaskID,
		CustomerID:   existing.CustomerID,
		CustomerName: existing.CustomerName,
	}
	if existing.SourceType == "" {
		applyCustomerForm(t, c)
	}
	if adminGTD {
		applyAdminGTDForm(t, c)
		if code := h.adminGTDErr(id, t.Status, t, c); code != "" {
			return c.Redirect(http.StatusSeeOther,
				"/workboard/tasks/"+id+"?err="+code+"&back="+url.QueryEscape(strings.TrimSpace(c.FormValue("back"))))
		}
	} else {
		copyAdminGTDFields(t, existing)
	}
	if t.DueDate == "" && !adminGTD {
		t.DueDate = workDate
	}
	if t.DueDate == "" && t.Status != model.WBTaskInbox {
		return c.Redirect(http.StatusSeeOther,
			"/workboard/tasks/"+id+"?err=task&back="+url.QueryEscape(strings.TrimSpace(c.FormValue("back"))))
	}
	supports := parseSupportMembers(c, t.Assignee)
	if len(supports) > 0 && t.Assignee == "" {
		return c.Redirect(http.StatusSeeOther,
			"/workboard/tasks/"+id+"?err=owner&back="+url.QueryEscape(strings.TrimSpace(c.FormValue("back"))))
	}
	if code := h.recurrenceCompleteErr(existing, t, c); code != "" {
		loc := "/workboard/tasks/" + id + "?err=" + code
		if code == "rec_open" {
			open, _ := h.repo.CountOpenOccurrences(id)
			loc += "&n=" + fmt.Sprintf("%d", open)
		}
		back := strings.TrimSpace(c.FormValue("back"))
		if back != "" {
			loc += "&back=" + url.QueryEscape(back)
		}
		return c.Redirect(http.StatusSeeOther, loc)
	}
	if t.StartTime != "" {
		if msg := h.firstParticipantOverlap(t.WorkDate, t.StartTime, t.EndTime, t.TaskID, occupancyNames(t.Assignee, supports)); msg != "" {
			return c.Redirect(http.StatusSeeOther,
				"/workboard/tasks/"+id+"?err="+url.QueryEscape(msg)+"&back="+url.QueryEscape(strings.TrimSpace(c.FormValue("back"))))
		}
	}
	if err := h.repo.UpdateTask(t); err != nil {
		return err
	}
	if err := h.saveOccurrenceStatus(existing, t, c); err != nil {
		code := "rec_rule"
		if strings.Contains(err.Error(), "제외 사유") {
			code = "rec_skip_reason"
		} else if strings.Contains(err.Error(), "다음 조치일") {
			code = "rec_defer_date"
		}
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+id+"?err="+code)
	}
	_ = h.saveSupportMembers(c, t.TaskID, t.Assignee)
	if adminGTD {
		if t.Status == model.WBTaskComplete && existing.Status != model.WBTaskComplete {
			note := t.CompleteNote
			if strings.TrimSpace(c.FormValue("force_complete")) == "1" {
				if r := strings.TrimSpace(c.FormValue("force_reason")); r != "" {
					note = strings.TrimSpace(note + "\n관리자 강제 완료: " + r)
				}
			}
			h.addCompleteActivity(t.TaskID, note, ctxString(c, "user_name"))
		}
		h.ensureWaitingAction(t)
	}
	if existing.SourceType == model.WBSourceMaintenance && h.mntRepo != nil && existing.SourceID != "" {
		if v, err := h.mntRepo.GetVisit(existing.SourceID); err == nil && v != nil {
			v.ProjectID = projectID
			_ = h.mntRepo.UpdateVisit(*v)
		}
	}
	h.syncVisitDateFromTask(t.SourceType, t.SourceID, t.WorkDate)
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
	today := time.Now().Format(dateLayout)
	_, _ = h.repo.MarkPastOccurrencesOverdue(today)
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
		// 쓰기 가능한 경우만 「조치/조치 완료」버튼 (옵저버는 can* 가 false)
		canAction = canProcessAS(c) || canReceiveAS(c) || canWriteWorkboard(c)
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
			actionHref = mntVisitActionHref(*v)
			actionLabel = "조치"
			canAction = canEditMaintenanceSchedule(c) || canWriteWorkboard(c)
			if v.Completed {
				actionLabel = "조치 완료"
				t.Status = model.WBTaskComplete
				if t.Progress < 100 {
					t.Progress = 100
				}
				_ = h.repo.SyncMaintenanceTaskStatus(t.SourceID, true)
			}
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
	customers, _ := h.customerRepo.ListAll()
	supportMembers, _ := h.repo.ListMembers(t.TaskID)
	var supportOnly []model.WorkTaskMember
	for _, m := range supportMembers {
		if m.IsSupport() {
			supportOnly = append(supportOnly, m)
		}
	}
	var taskLeaveNames []string
	if h.mntRepo != nil {
		if lr := h.mntRepo.Leaves(); lr != nil && t.WorkDate != "" {
			if items, err := lr.ListInRange(t.WorkDate, t.WorkDate); err == nil {
				hit := map[string]bool{}
				for _, it := range items {
					n := strings.TrimSpace(it.UserName)
					if n != "" && !hit[n] {
						hit[n] = true
						taskLeaveNames = append(taskLeaveNames, n)
					}
				}
			}
		}
	}
	back := strings.TrimSpace(c.QueryParam("back"))
	if back == "" {
		back = "/workboard/register"
	}
	active := "work_register"
	if strings.Contains(back, "/admin-work/stats") {
		active = "admin_work_stats"
	} else if strings.Contains(back, "/admin-work") {
		active = "admin_work"
	}
	data := map[string]interface{}{
		"Title":          t.Title,
		"Active":         active,
		"Task":           t,
		"Children":       children,
		"Parent":         parent,
		"SourceHref":     sourceHref,
		"SourceLabel":    sourceLabel,
		"ActionHref":     actionHref,
		"ActionLabel":    actionLabel,
		"CanAction":      canAction,
		"ASClosed":       asClosed,
		"ASSource":       asSource,
		"CompleteLocal":  completeLocal,
		"Assignees":      assignees,
		"SupportMembers": supportOnly,
		"DayLeaveNames":  taskLeaveNames,
		"Projects":       projects,
		"Customers":      customers,
		"CanWrite":       canWriteWorkboard(c),
		"FlashOK":        c.QueryParam("ok"),
		"FlashErr":       c.QueryParam("err"),
		"FlashErrMsg":    gtdFlashMsg(c.QueryParam("err"), c.QueryParam("n")),
		"Today":          time.Now().Format(dateLayout),
		"BackURL":        back,
	}
	rule := defaultRecurrenceForm(t, data["Today"].(string))
	archived := false
	if saved, _ := h.repo.GetRecurrence(t.TaskID); saved != nil {
		rule = *saved
		archived = saved.Archived
	}
	if form, ok := c.Get("recurrence_form").(*model.WorkRecurrence); ok && form != nil {
		rule = *form
		rule.Archived = archived
	}
	var preview *model.OccurrencePreview
	if p, ok := c.Get("recurrence_preview").(*model.OccurrencePreview); ok {
		preview = p
	}
	occN, _ := h.repo.CountOccurrences(t.TaskID)
	holidayBanners := []string{}
	if preview != nil {
		holidayBanners = preview.HolidayBanners
	} else {
		missing := h.holidayYearMissing()
		for _, y := range recurrenceYears(rule.StartDate, rule.EndDate) {
			if missing(y) {
				holidayBanners = append(holidayBanners, model.HolidayMissingBanner(y))
			}
		}
	}
	data["Recurrence"] = rule
	data["WeekdayOn"] = model.ParseWeekdays(rule.Weekdays)
	data["RecurrencePreview"] = preview
	data["OccurrenceCount"] = occN
	data["RecurrenceArchived"] = archived
	completeOcc, _ := h.repo.CountCompleteOccurrences(t.TaskID)
	data["CompleteOccurrenceCount"] = completeOcc
	changeMode := model.RecurrenceChangeFuture
	if m, ok := c.Get("recurrence_change_mode").(string); ok && m != "" {
		changeMode = model.NormalizeRecurrenceChangeMode(m)
	}
	data["ChangeMode"] = changeMode
	if preview != nil && occN > 0 {
		model.AnnotateOccurrenceChange(preview, children, changeMode, today)
	}
	data["HolidayBanners"] = holidayBanners
	data["FlashN"] = c.QueryParam("n")
	data["FlashReport"] = c.QueryParam("report")
	prog := model.CalcOccurrenceProgress(children, rule.ProgressIncludeFuture, today)
	data["OccProgress"] = prog
	data["OpenOccurrenceCount"] = prog.Open
	data["IsOccurrence"] = t.RecurrenceRole == model.RecurrenceRoleOccurrence
	data["NeedFinalResult"] = occN > 0 &&
		model.NormalizeCompletePolicy(rule.CompletePolicy) == model.CompletePolicyRequireResult &&
		prog.Open == 0 && strings.TrimSpace(rule.FinalResult) == "" &&
		t.Status != model.WBTaskComplete
	nextOcc := ""
	for _, ch := range children {
		if ch.RecurrenceRole != model.RecurrenceRoleOccurrence || !model.OccurrenceOpen(ch) {
			continue
		}
		d := strings.TrimSpace(ch.WorkDate)
		if d == "" {
			d = strings.TrimSpace(ch.DueDate)
		}
		if d >= today {
			nextOcc = d
			break
		}
		if nextOcc == "" {
			nextOcc = d
		}
	}
	data["NextOccurrence"] = nextOcc
	h.renderTaskGTD(c, data, t)
	return c.Render(http.StatusOK, "workboard/task_show.html", data)
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

func applyCustomerForm(t *model.WorkTask, c echo.Context) {
	if t == nil {
		return
	}
	t.CustomerID = strings.TrimSpace(c.FormValue("customer_id"))
	t.CustomerName = strings.TrimSpace(c.FormValue("customer_name"))
	if t.CustomerID != "" {
		t.CustomerName = ""
	}
}

func canWriteWorkboard(c echo.Context) bool {
	if isObserverRole(c) {
		return false
	}
	return hasPerm(c, model.PermWorkboard)
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
