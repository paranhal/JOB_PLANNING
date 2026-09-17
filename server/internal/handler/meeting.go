package handler

import (
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type MeetingHandler struct {
	work     *repository.WorkBoardRepo
	stats    *repository.StatsRepo
	userRepo *repository.UserRepo
}

func NewMeetingHandler(work *repository.WorkBoardRepo, stats *repository.StatsRepo, userRepo *repository.UserRepo) *MeetingHandler {
	return &MeetingHandler{work: work, stats: stats, userRepo: userRepo}
}

// Show GET /meeting?date=YYYY-MM-DD&assignee=
func (h *MeetingHandler) Show(c echo.Context) error {
	today := parseMeetingAnchor(c.QueryParam("date"))
	dateStr := today.Format("2006-01-02")
	prevStr := today.AddDate(0, 0, -1).Format("2006-01-02")

	var users []model.User
	if h.userRepo != nil {
		users, _ = h.userRepo.ListAssignable()
	}
	selected := resolveMeetingAssignee(c, users)
	order := assignableNameOrder(users)
	assigneeQ := meetingAssigneeQuery(selected)
	mineUID, mineKeys := meetingAssigneeKeys(selected, users)

	yesterday, err := h.work.ListCompletedOn(prevStr, mineUID, mineKeys, 300)
	if err != nil {
		return err
	}
	todayDone, err := h.work.ListCompletedOn(dateStr, mineUID, mineKeys, 300)
	if err != nil {
		return err
	}
	todayItems, err := h.work.ListScheduledOn(dateStr, mineUID, mineKeys, 300)
	if err != nil {
		return err
	}

	unplannedRaw, _, err := h.work.ListUnplanned(mineUID, mineKeys, "")
	if err != nil {
		return err
	}
	var unplannedNeed []model.WorkListItem
	for _, u := range unplannedRaw {
		if !u.NeedPlanKind() {
			continue
		}
		it := u.WorkListItem
		it.NeedPlan = true
		if it.ProductType == "" {
			it.ProductType = u.ProductType
		}
		unplannedNeed = append(unplannedNeed, it)
	}
	progress, err := h.work.ListBucketOn(model.WorkBucketInProgress, dateStr, mineUID, mineKeys, 300)
	if err != nil {
		return err
	}
	progress = meetingProgressExcludingScheduled(progress, dateStr)

	kanban := model.FillMeetingKanban(yesterday, todayDone, progress, todayItems, unplannedNeed)
	plannedN := model.MeetingKanbanScheduledSum(kanban)
	logMeetingPlannedIdentity(dateStr, selected, plannedN, kanban)
	display := model.ParseDisplay(c.QueryParam("display"), c.QueryParam("view"))
	navQ := meetingNavQuery(selected, display)
	overviewQ := meetingOverviewQuery(dateStr, selected, len(yesterday), plannedN, len(unplannedRaw))

	return c.Render(http.StatusOK, "meeting/show.html", map[string]interface{}{
		"Title":           "일일 업무 회의",
		"Active":          NavMeeting,
		"Date":            dateStr,
		"PrevDate":        prevStr,
		"NextDate":        today.AddDate(0, 0, 1).Format("2006-01-02"),
		"Yesterday":       yesterday,
		"TodayItems":      todayItems,
		"YesterdayGroups": groupWorkItemsByAssignee(yesterday, order),
		"TodayGroups":     groupWorkItemsByAssignee(todayItems, order),
		"YesterdayN":      len(yesterday),
		"TodayN":          plannedN,
		"PlannedN":        plannedN,
		"UnplannedN":      len(unplannedRaw),
		"UnplannedHref":   planUnplannedURL(false, currentRole(c), ""),
		"OverviewReady":   false,
		"OverviewURL":     "/meeting/overview?" + overviewQ,
		"Users":           users,
		"Assignee":        selected,
		"AssigneeQ":       template.URL(assigneeQ),
		"NavQ":            template.URL(navQ),
		"Display":         display,
		"KanbanHref":      "/meeting?" + meetingFilterQuery(dateStr, selected, "kanban"),
		"ListHref":        "/meeting?" + meetingFilterQuery(dateStr, selected, "list"),
		"KanbanColumns":   kanban.Columns,
		"KanbanTotal":     kanban.Total,
		"KanbanDrag":      false,
		"KanbanDrop":      "",
		"KanbanHint":      "전일 완료 → 오늘 완료 → 진행중 → 오늘 예정 → 미계획 순으로 한 열에만 넣습니다. 카드를 누르면 원본으로 갑니다.",
		"CanWrite":        false,
		"Role":            currentRole(c),
		"ScopeNote":       meetingScopeNote(selected),
		"HolidayBanner":   meetingHolidayBanner(dateStr),
	})
}

// Overview GET /meeting/overview — 요약 카드만. 목록은 Show 가 먼저 그린다. §39.1
func (h *MeetingHandler) Overview(c echo.Context) error {
	today := parseMeetingAnchor(c.QueryParam("date"))
	dateStr := today.Format("2006-01-02")
	prevStr := today.AddDate(0, 0, -1).Format("2006-01-02")

	var users []model.User
	if h.userRepo != nil {
		users, _ = h.userRepo.ListAssignable()
	}
	selected := resolveMeetingAssignee(c, users)

	cols := repository.BuildStatsPeriodColumns(model.StatsViewDay, today)
	filter := repository.ParseMeetingFilter(model.StatsScopeTeam, "", "")
	if selected != "" {
		filter = repository.ParseMeetingFilter(model.StatsScopeAssignee, selected, "")
	}
	_ = h.stats.FillPeriodOverview(cols, filter)

	var prevCol, curCol *model.StatsPeriodColumn
	for i := range cols {
		switch cols[i].Key {
		case "prev":
			prevCol = &cols[i]
		case "current":
			curCol = &cols[i]
		}
	}
	yesterdayN := atoiDefault(c.QueryParam("yesterday_n"), 0)
	unplannedN := atoiDefault(c.QueryParam("unplanned_n"), 0)
	plannedN := atoiDefault(c.QueryParam("planned_n"), 0)
	if strings.TrimSpace(c.QueryParam("planned_n")) == "" && curCol != nil {
		plannedN = curCol.Counts.PlannedTotal()
	}

	c.Request().Header.Set("HX-Request", "true")
	return c.Render(http.StatusOK, "meeting/overview.html", map[string]interface{}{
		"Date":          dateStr,
		"PrevDate":      prevStr,
		"YesterdayN":    yesterdayN,
		"PlannedN":      plannedN,
		"UnplannedN":    unplannedN,
		"UnplannedHref": planUnplannedURL(false, currentRole(c), ""),
		"PrevCol":       prevCol,
		"CurCol":        curCol,
		"OverviewReady": true,
	})
}

func meetingOverviewQuery(date, selected string, yesterdayN, plannedN, unplannedN int) string {
	v := url.Values{}
	if date != "" {
		v.Set("date", date)
	}
	if selected == "" {
		v.Set("assignee", meetingAssigneeAll)
	} else {
		v.Set("assignee", selected)
	}
	v.Set("yesterday_n", strconv.Itoa(yesterdayN))
	v.Set("planned_n", strconv.Itoa(plannedN))
	v.Set("unplanned_n", strconv.Itoa(unplannedN))
	return v.Encode()
}

func atoiDefault(s string, fallback int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func meetingProgressExcludingScheduled(items []model.WorkListItem, date string) []model.WorkListItem {
	var out []model.WorkListItem
	for _, it := range items {
		if model.WorkItemScheduledDate(it) == date {
			continue
		}
		out = append(out, it)
	}
	return out
}

func logMeetingPlannedIdentity(date, assignee string, planned int, view model.KanbanView) {
	sum := model.MeetingKanbanScheduledSum(view)
	if sum == planned {
		return
	}
	todayN, doneN := 0, 0
	for _, c := range view.Columns {
		switch c.Key {
		case model.WorkBucketToday:
			todayN = c.Count
		case model.WorkBucketCompletedToday:
			doneN = c.Count
		}
	}
	log.Printf("meeting planned identity mismatch: date=%s assignee=%q planned=%d today_sched=%d today_complete=%d sum=%d",
		date, assignee, planned, todayN, doneN, sum)
}

func parseMeetingAnchor(s string) time.Time {
	s = strings.TrimSpace(s)
	if s != "" {
		if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		}
	}
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
}

func meetingScopeNote(assignee string) string {
	if assignee != "" {
		return assignee + " 기준"
	}
	return "전체 업무 기준"
}
