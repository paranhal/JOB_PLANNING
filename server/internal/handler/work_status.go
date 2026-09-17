package handler

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	wsKindAction  = "action"  // 조치(완료 실적)
	wsKindReceipt = "receipt" // 접수(당일 신규)
	wsKindPlanned = "planned" // 삭제됨 — 들어오면 조치로 되돌린다. §14.2
)

type WorkStatusHandler struct {
	wb          *repository.WBRepo
	userRepo    *repository.UserRepo
	holidayRepo *repository.HolidayRepo
	sales       *repository.SalesRepo
}

func NewWorkStatusHandler(wb *repository.WBRepo, userRepo *repository.UserRepo, holidayRepo *repository.HolidayRepo, sales *repository.SalesRepo) *WorkStatusHandler {
	return &WorkStatusHandler{wb: wb, userRepo: userRepo, holidayRepo: holidayRepo, sales: sales}
}

// Calendar 업무처리현황 — 월 캘린더(기본). kind=action|receipt. §14.2 · §14.3
func (h *WorkStatusHandler) Calendar(c echo.Context) error {
	kind := strings.TrimSpace(c.QueryParam("kind"))
	if kind == wsKindPlanned {
		q := c.QueryParams()
		q.Del("kind")
		loc := "/work-status"
		if enc := q.Encode(); enc != "" {
			loc += "?" + enc
		}
		return c.Redirect(http.StatusFound, loc)
	}
	if kind != wsKindReceipt {
		kind = wsKindAction
	}

	view := strings.TrimSpace(c.QueryParam("view"))
	if view != regViewWeek && view != regViewDay {
		view = regViewMonth
	}

	today := time.Now().Format(dateLayout)
	base, err := time.ParseInLocation(dateLayout, strings.TrimSpace(c.QueryParam("date")), time.Local)
	if err != nil {
		base = time.Now()
	}
	base = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, time.Local)
	period := buildRegisterPeriod(view, base, today)

	role := currentRole(c)
	roleUnresolved := !model.IsKnownRole(role)
	assigneeFilter, mineParam, scopeAll, showScopeToggle := workStatusResolveScope(c, role, roleUnresolved)
	projectFilter := strings.TrimSpace(c.QueryParam("project"))

	assignees, _ := h.userRepo.ListAssignable()
	var colorNames []string
	for _, u := range assignees {
		colorNames = append(colorNames, u.FullName)
	}

	h.wb.SyncMaintenanceBoard()
	actionTasks, err := h.wb.ListCompletedForStatusPeriod(period.From, period.To)
	if err != nil {
		return err
	}
	asIDs, _ := collectSourceIDs(actionTasks)
	asStatus, _ := h.wb.ASStatusesByIDs(asIDs)
	enrichCompletedStatus(actionTasks, asStatus)

	receiptTasks, err := h.wb.ListASReceivedBetween(period.From, period.To)
	if err != nil {
		return err
	}

	actionTasks = filterTasksByAssignee(actionTasks, assigneeFilter)
	actionTasks = filterTasksByProject(actionTasks, projectFilter)
	receiptTasks = filterTasksByAssignee(receiptTasks, assigneeFilter)
	receiptTasks = filterTasksByProject(receiptTasks, projectFilter)

	var tasks []model.WorkTask
	var cardFn func(model.WorkTask) model.WBCard
	kindNote := "목록 · 전체 기간 · 이관 데이터 포함 (통계 집계와 다름)"
	if kind == wsKindReceipt {
		tasks = receiptTasks
		cardFn = receiptCardFromWork
		kindNote = "그날 신규 접수 건 · 목록 · 전체 기간 · 이관 데이터 포함 (통계 집계와 다름)"
	} else {
		tasks = actionTasks
		cardFn = statusCardFromWork
	}
	for _, t := range actionTasks {
		colorNames = append(colorNames, t.Assignee)
	}
	for _, t := range receiptTasks {
		colorNames = append(colorNames, t.Assignee)
	}
	model.SetAssigneeColorOrder(colorNames)

	scopeNote := kindNote + " · " + registerScopeNote(role, scopeAll, assigneeFilter, roleUnresolved)

	var asCards, mntCards, adminCards []model.WBCard
	for _, t := range tasks {
		card := cardFn(t)
		switch {
		case t.SourceType == model.WBSourceAS || t.WorkType == model.WBWorkAS:
			asCards = append(asCards, card)
		case t.SourceType == model.WBSourceMaintenance || t.WorkType == model.WBWorkMaintenance:
			mntCards = append(mntCards, card)
		default:
			adminCards = append(adminCards, card)
		}
	}

	projects, _ := h.wb.ListProjects(true)
	holi := loadHolidays(h.holidayRepo, period.From, period.To)
	var monthWeeks [][]RegisterMonthDay
	switch view {
	case regViewWeek:
		monthWeeks = buildRegisterWeekDays(period.Columns, today, tasks, cardFn, registerWeekVisibleMax)
		attachStatusDayCounts(monthWeeks, actionTasks, receiptTasks)
		decorateMonthWeeks(monthWeeks, today, holi, nil)
	case regViewMonth:
		monthWeeks = buildRegisterMonthWeeks(base, today, tasks, cardFn)
		attachStatusDayCounts(monthWeeks, actionTasks, receiptTasks)
		decorateMonthWeeks(monthWeeks, today, holi, nil)
	}

	asStats := buildCardStats(asCards, today)
	mntStats := buildCardStats(mntCards, today)
	adminStats := buildCardStats(adminCards, today)

	var monthBanners []model.SalesMonthBanner
	if view == regViewMonth && h.sales != nil {
		monthBanners, _ = h.sales.ListMonthBanners(base.Year(), int(base.Month()))
	}

	dateStr := base.Format(dateLayout)
	queryAssignee := assigneeFilter
	mineForQuery := ""
	if showScopeToggle {
		queryAssignee = ""
		if mineParam == "0" {
			mineForQuery = "0"
		}
	}
	filterQ := workStatusFilterQuery(queryAssignee, projectFilter, kind, mineForQuery)
	plannedURL := registerURLWithProject(view, dateStr, queryAssignee, projectFilter)
	if mineForQuery != "" {
		plannedURL += "&mine=" + url.QueryEscape(mineForQuery)
	}

	missing, _ := h.wb.CountMissingCompleteDates()
	missingAssignee, _ := h.wb.CountMissingAssignees()
	showMissing := strings.TrimSpace(c.QueryParam("missing"))
	var missingItems []model.WorkTask
	missingTitle := ""
	switch showMissing {
	case "complete":
		missingTitle = "완료일 미기록"
		if missing.Total() > 0 {
			missingItems, _ = h.wb.ListMissingCompleteDates()
		}
	case "assignee":
		missingTitle = "담당자 미배정"
		if missingAssignee.Total() > 0 {
			missingItems, _ = h.wb.ListMissingAssignees()
		}
	}

	return c.Render(http.StatusOK, "work_status/timeline.html", map[string]interface{}{
		"Title":           "업무처리현황",
		"Active":          NavWorkStatus,
		"View":            view,
		"ViewLabel":       workStatusViewLabel(view),
		"Kind":            kind,
		"Date":            dateStr,
		"PeriodLabel":     period.Label,
		"PrevDate":        period.Prev,
		"NextDate":        period.Next,
		"Today":           today,
		"MonthWeeks":      monthWeeks,
		"MonthBanners":    monthBanners,
		"WeekdayLabels":   workStatusWeekdayLabels(view),
		"ASCards":         asCards,
		"MntCards":        mntCards,
		"AdminCards":      adminCards,
		"ASStats":         asStats,
		"MntStats":        mntStats,
		"AdminStats":      adminStats,
		"TotalStats":      asStats.add(mntStats).add(adminStats),
		"PlacedCount":     len(tasks),
		"CompleteCount":   len(tasks),
		"Projects":        projects,
		"Assignees":       assignees,
		"AssigneeFilter":  assigneeFilter,
		"ProjectFilter":   projectFilter,
		"FilterQ":         filterQ,
		"PlannedURL":      plannedURL,
		"ScopeNote":       scopeNote,
		"Role":            role,
		"RoleUnresolved":  roleUnresolved,
		"ShowScopeToggle": showScopeToggle,
		"ScopeAll":        scopeAll,
		"MineParam":       mineParam,
		"TeamHref":        workStatusURL(view, dateStr, kind, "", projectFilter, "0"),
		"MineHref":        workStatusURL(view, dateStr, kind, "", projectFilter, ""),
		"MissingComplete": missing,
		"MissingAssignee": missingAssignee,
		"HolidayMissing":  holidayMissingBanner(h.holidayRepo, base.Year()),
		"ShowMissing":     showMissing,
		"MissingTitle":    missingTitle,
		"MissingItems":    missingItems,
	})
}

func workStatusViewLabel(view string) string {
	switch view {
	case regViewWeek:
		return "주간 업무처리현황"
	case regViewMonth:
		return "월간 업무처리현황"
	default:
		return "일일 업무처리현황"
	}
}

func workStatusFilterQuery(assignee, project, kind, mine string) string {
	v := url.Values{}
	if assignee != "" {
		v.Set("assignee", assignee)
	}
	if project != "" {
		v.Set("project", project)
	}
	if kind != "" && kind != wsKindAction {
		v.Set("kind", kind)
	}
	if mine != "" {
		v.Set("mine", mine)
	}
	enc := v.Encode()
	if enc == "" {
		return ""
	}
	return "&" + enc
}

func workStatusURL(view, date, kind, assignee, project, mine string) string {
	v := url.Values{}
	if view != "" {
		v.Set("view", view)
	}
	if date != "" {
		v.Set("date", date)
	}
	if kind != "" && kind != wsKindAction {
		v.Set("kind", kind)
	}
	if assignee != "" {
		v.Set("assignee", assignee)
	}
	if project != "" {
		v.Set("project", project)
	}
	if mine != "" {
		v.Set("mine", mine)
	}
	enc := v.Encode()
	if enc == "" {
		return "/work-status"
	}
	return "/work-status?" + enc
}

func workStatusWeekdayLabels(view string) []string {
	if view == regViewWeek {
		return []string{"월", "화", "수", "목", "금", "토", "일"}
	}
	return []string{"일", "월", "화", "수", "목", "금", "토"}
}

func workStatusResolveScope(c echo.Context, role string, unresolved bool) (assignee, mineParam string, scopeAll, showToggle bool) {
	mineParam = c.QueryParam("mine")
	assignee = normalizeAssigneeFilter(c.QueryParam("assignee"))
	scopeAll = true
	if unresolved {
		return
	}
	if role == model.RoleAdmin || role == model.RoleOffice {
		scopeAll = assignee == ""
		return
	}
	if role == model.RoleTech || role == model.RoleSales || role == model.RoleObserver {
		showToggle = true
		if mineParam == "0" {
			scopeAll = true
			return
		}
		scopeAll = false
		if assignee == "" {
			assignee = currentUserDisplayName(c)
		}
	}
	return
}

func attachStatusDayCounts(weeks [][]RegisterMonthDay, action, receipt []model.WorkTask) {
	ac := countTasksByWorkDate(action)
	rc := countTasksByWorkDate(receipt)
	for wi := range weeks {
		for di := range weeks[wi] {
			d := weeks[wi][di].Date
			if d == "" {
				continue
			}
			weeks[wi][di].ActionCount = ac[d]
			weeks[wi][di].ReceiptCount = rc[d]
		}
	}
}

func countTasksByWorkDate(tasks []model.WorkTask) map[string]int {
	m := map[string]int{}
	for _, t := range tasks {
		d := strings.TrimSpace(t.WorkDate)
		if d != "" {
			m[d]++
		}
	}
	return m
}

func collectSourceIDs(tasks []model.WorkTask) (asIDs, mntIDs []string) {
	seenAS, seenM := map[string]bool{}, map[string]bool{}
	for _, t := range tasks {
		id := strings.TrimSpace(t.SourceID)
		if id == "" {
			continue
		}
		switch t.SourceType {
		case model.WBSourceAS:
			if !seenAS[id] {
				seenAS[id] = true
				asIDs = append(asIDs, id)
			}
		case model.WBSourceMaintenance:
			if !seenM[id] {
				seenM[id] = true
				mntIDs = append(mntIDs, id)
			}
		}
	}
	return asIDs, mntIDs
}

func filterCompletedWorkTasks(tasks []model.WorkTask, asStatus map[string]string, mntDone map[string]bool) []model.WorkTask {
	var out []model.WorkTask
	for _, t := range tasks {
		if isCompletedWorkTask(t, asStatus, mntDone) {
			out = append(out, t)
		}
	}
	return out
}

func isCompletedWorkTask(t model.WorkTask, asStatus map[string]string, mntDone map[string]bool) bool {
	switch t.SourceType {
	case model.WBSourceAS:
		return model.IsStatsCompletedStatus(asStatus[t.SourceID])
	case model.WBSourceMaintenance:
		return mntDone[t.SourceID]
	default:
		return t.Status == model.WBTaskComplete
	}
}

func enrichCompletedStatus(tasks []model.WorkTask, asStatus map[string]string) {
	for i := range tasks {
		switch tasks[i].SourceType {
		case model.WBSourceAS:
			if st := asStatus[tasks[i].SourceID]; st != "" {
				tasks[i].Status = st
			} else {
				tasks[i].Status = "completed"
			}
		case model.WBSourceMaintenance:
			tasks[i].Status = model.WBTaskComplete
		default:
			tasks[i].Status = model.WBTaskComplete
		}
	}
}

func statusCardFromWork(t model.WorkTask) model.WBCard {
	card := taskCardFromWork(t)
	if href := strings.TrimSpace(t.BoardHref); href != "" {
		card.ActionHref = href
		card.SourceHref = href
	}
	switch t.SourceType {
	case model.WBSourceAS:
		card.Status = t.Status
		card.StatusLabel = model.ASStatusDisplayLabel(t.Status)
		if card.StatusLabel == "" {
			card.StatusLabel = "완료"
		}
	case model.WBSourceMaintenance:
		card.Status = model.WBTaskComplete
		card.StatusLabel = "방문완료"
	default:
		card.Status = model.WBTaskComplete
		card.StatusLabel = "완료"
	}
	return card
}

// wbCardStats 단위 업무별 현황 머리말 집계 — 기간 전체·오늘·남은(미완료) 건수
type wbCardStats struct {
	Total  int
	Today  int
	Remain int
}

func (s wbCardStats) add(o wbCardStats) wbCardStats {
	return wbCardStats{Total: s.Total + o.Total, Today: s.Today + o.Today, Remain: s.Remain + o.Remain}
}

func buildCardStats(cards []model.WBCard, today string) wbCardStats {
	s := wbCardStats{Total: len(cards)}
	for _, c := range cards {
		day := strings.TrimSpace(c.WorkDate)
		if day == "" {
			day = strings.TrimSpace(c.PlannedDay)
		}
		if day == today {
			s.Today++
		}
		if !wbCardSettled(c) {
			s.Remain++
		}
	}
	return s
}

// wbCardSettled 더 처리할 게 없는 카드(완료·종료·취소). AS는 원본 접수 상태로 판단한다.
func wbCardSettled(c model.WBCard) bool {
	st := strings.TrimSpace(c.Status)
	if c.Category == model.WBSourceAS {
		return model.IsStatsCompletedStatus(st) || st == "cancelled"
	}
	return st == model.WBTaskComplete
}

func filterPlannedWorkTasks(tasks []model.WorkTask, asStatus map[string]string, mntDone map[string]bool) []model.WorkTask {
	var out []model.WorkTask
	for _, t := range tasks {
		if !isCompletedWorkTask(t, asStatus, mntDone) {
			out = append(out, t)
		}
	}
	return out
}

// enrichPlannedStatus AS는 접수 상태(진행중·보류 등)를 그대로 보여 준다.
func enrichPlannedStatus(tasks []model.WorkTask, asStatus map[string]string) {
	for i := range tasks {
		if tasks[i].SourceType != model.WBSourceAS {
			continue
		}
		if st := asStatus[tasks[i].SourceID]; st != "" {
			tasks[i].Status = st
		}
	}
}

// plannedCardFromWork 예정업무 카드 — 조치 버튼 없이 상태만 보여 준다.
func plannedCardFromWork(t model.WorkTask) model.WBCard {
	card := taskCardFromWork(t)
	if href := strings.TrimSpace(t.BoardHref); href != "" {
		card.SourceHref = href
		card.ActionHref = href
	}
	card.Status = t.Status
	if t.SourceType == model.WBSourceAS {
		card.StatusLabel = model.ASStatusDisplayLabel(t.Status)
	} else {
		card.StatusLabel = model.WBTaskStatusLabel(t.Status)
	}
	if card.StatusLabel == "" {
		card.StatusLabel = "예정"
	}
	if t.SourceType == model.WBSourceMaintenance {
		// visit_id 대신 점검 번호(방문일 · 점검대상)를 보여 준다.
		card.SourceNumber = strings.TrimSpace(t.Tags)
	}
	return card
}

func receiptCardFromWork(t model.WorkTask) model.WBCard {
	card := taskCardFromWork(t)
	if t.Tags != "" {
		card.SourceNumber = t.Tags
	}
	card.Status = t.Status
	card.StatusLabel = "접수"
	if href := strings.TrimSpace(t.BoardHref); href != "" {
		card.SourceHref = href
	}
	return card
}
