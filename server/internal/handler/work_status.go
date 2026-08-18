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
	wsKindPlanned = "planned" // 예정업무(아직 완료되지 않은 그날 업무)
)

type WorkStatusHandler struct {
	wb          *repository.WBRepo
	userRepo    *repository.UserRepo
	holidayRepo *repository.HolidayRepo
}

func NewWorkStatusHandler(wb *repository.WBRepo, userRepo *repository.UserRepo, holidayRepo *repository.HolidayRepo) *WorkStatusHandler {
	return &WorkStatusHandler{wb: wb, userRepo: userRepo, holidayRepo: holidayRepo}
}

// Calendar 업무처리현황 — 일일 업무 등록과 동일 UI. kind=action|receipt|planned
func (h *WorkStatusHandler) Calendar(c echo.Context) error {
	view := strings.TrimSpace(c.QueryParam("view"))
	if view != regViewWeek && view != regViewMonth {
		view = regViewDay
	}
	kind := strings.TrimSpace(c.QueryParam("kind"))
	if kind != wsKindReceipt && kind != wsKindPlanned {
		kind = wsKindAction
	}

	today := time.Now().Format(dateLayout)
	base, err := time.ParseInLocation(dateLayout, strings.TrimSpace(c.QueryParam("date")), time.Local)
	if err != nil {
		base = time.Now()
	}
	base = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, time.Local)
	period := buildRegisterPeriod(view, base, today)

	assigneeFilter := normalizeAssigneeFilter(c.QueryParam("assignee"))
	projectFilter := strings.TrimSpace(c.QueryParam("project"))

	assignees, _ := h.userRepo.ListAssignable()
	var colorNames []string
	for _, u := range assignees {
		colorNames = append(colorNames, u.FullName)
	}

	var tasks []model.WorkTask
	var cardFn func(model.WorkTask) model.WBCard
	scopeNote := ""

	if kind == wsKindReceipt {
		tasks, err = h.wb.ListASReceivedBetween(period.From, period.To)
		if err != nil {
			return err
		}
		for _, t := range tasks {
			colorNames = append(colorNames, t.Assignee)
		}
		model.SetAssigneeColorOrder(colorNames)
		cardFn = receiptCardFromWork
		scopeNote = "그날 신규 접수 건"
	} else if kind == wsKindPlanned {
		// 정기점검 일정 변경분을 먼저 일일업무에 반영한 뒤 집계한다.
		h.wb.SyncMaintenanceBoard()
		dated, err := h.wb.ListTasksDatedBetween(period.From, period.To)
		if err != nil {
			return err
		}
		asIDs, mntIDs := collectSourceIDs(dated)
		asStatus, _ := h.wb.ASStatusesByIDs(asIDs)
		mntDone, _ := h.wb.MaintenanceCompletedByIDs(mntIDs)
		tasks = filterPlannedWorkTasks(dated, asStatus, mntDone)
		enrichPlannedStatus(tasks, asStatus)
		// 아직 일일업무로 만들지 않은 예정 건(AS 방문예정·점검 방문)도 합친다.
		extra, err := h.wb.ListPlannedSourcesBetween(period.From, period.To)
		if err != nil {
			return err
		}
		tasks = append(tasks, extra...)
		for _, t := range tasks {
			colorNames = append(colorNames, t.Assignee)
		}
		model.SetAssigneeColorOrder(colorNames)
		cardFn = plannedCardFromWork
		scopeNote = "완료 전 예정·진행 업무"
	} else {
		// 통계「처리」와 같은 완료일 기준(AS 완료일시 · 점검 완료일 · 행정 배정일).
		h.wb.SyncMaintenanceBoard()
		tasks, err = h.wb.ListCompletedForStatusPeriod(period.From, period.To)
		if err != nil {
			return err
		}
		asIDs, _ := collectSourceIDs(tasks)
		asStatus, _ := h.wb.ASStatusesByIDs(asIDs)
		enrichCompletedStatus(tasks, asStatus)
		for _, t := range tasks {
			colorNames = append(colorNames, t.Assignee)
		}
		model.SetAssigneeColorOrder(colorNames)
		cardFn = statusCardFromWork
		scopeNote = "완료 실적(통계 처리와 동일 기준)"
	}

	tasks = filterTasksByAssignee(tasks, assigneeFilter)
	tasks = filterTasksByProject(tasks, projectFilter)
	// 좌(시간표)·우(단위업무)는 같은 집합. 시각이 없는 완료 건도 표시용 시각을 채운다.
	tasks = ensureTimelineDisplayTimes(tasks)

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

	slotTimes := registerSlotTimes()
	dayCols := buildRegisterDayColumns(period.Columns, tasks, cardFn)
	holi := loadHolidays(h.holidayRepo, period.From, period.To)
	decorateRegisterColumns(period.Columns, today, holi)
	decorateRegisterDayColumns(dayCols, today, holi)
	assigneeLegend := buildAssigneeLegend(tasks, assignees, asCards, mntCards, adminCards)
	gridHeightStyle, slotTopStyles := registerGridStyles(len(slotTimes))
	var monthWeeks [][]RegisterMonthDay
	if view == regViewMonth {
		monthWeeks = buildRegisterMonthWeeks(base, today, tasks, cardFn)
		decorateMonthWeeks(monthWeeks, today, holi, nil)
	}

	asStats := buildCardStats(asCards, today)
	mntStats := buildCardStats(mntCards, today)
	adminStats := buildCardStats(adminCards, today)

	dateStr := base.Format(dateLayout)
	filterQ := workStatusFilterQuery(assigneeFilter, projectFilter, kind)
	plannedURL := registerURLWithProject(view, dateStr, assigneeFilter, projectFilter)

	return c.Render(http.StatusOK, "work_status/timeline.html", map[string]interface{}{
		"Title":           "업무처리현황",
		"Active":          "work_status",
		"View":            view,
		"ViewLabel":       workStatusViewLabel(view),
		"Kind":            kind,
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
		"AssigneeLegend":  assigneeLegend,
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

func workStatusFilterQuery(assignee, project, kind string) string {
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
	enc := v.Encode()
	if enc == "" {
		return ""
	}
	return "&" + enc
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
	switch t.SourceType {
	case model.WBSourceAS:
		card.Status = t.Status
		card.StatusLabel = model.ASStatusDisplayLabel(t.Status)
		if card.StatusLabel == "" {
			card.StatusLabel = "완료"
		}
	case model.WBSourceMaintenance:
		card.Status = model.WBTaskComplete
		card.StatusLabel = "완료"
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
