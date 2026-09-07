package handler

import (
	"html/template"
	"net/http"
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

	yesterday, err := h.work.ListCompletedOn(prevStr, "", nil, 300)
	if err != nil {
		return err
	}
	todayItems, err := h.work.ListScheduledOn(dateStr, "", nil, 300)
	if err != nil {
		return err
	}
	yesterday = filterWorkItemsByAssignee(yesterday, selected)
	todayItems = filterWorkItemsByAssignee(todayItems, selected)

	mineKeys := []string(nil)
	if selected != "" {
		mineKeys = []string{selected}
	}
	unplannedRaw, _, err := h.work.ListUnplanned("", mineKeys, "")
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
	progress, err := h.work.ListBucketOn(model.WorkBucketInProgress, dateStr, "", mineKeys, 300)
	if err != nil {
		return err
	}

	kanban := model.FillMeetingKanban(yesterday, progress, todayItems, unplannedNeed)
	display := model.ParseDisplay(c.QueryParam("display"), c.QueryParam("view"))
	navQ := meetingNavQuery(selected, display)

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
		"TodayN":          len(todayItems),
		"UnplannedN":      len(unplannedRaw),
		"UnplannedHref":   planUnplannedURL(false, currentRole(c), ""),
		"PrevCol":         prevCol,
		"CurCol":          curCol,
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
		"KanbanHint":      "전일 완료 → 진행중 → 오늘 예정 → 미계획 순으로 한 열에만 넣습니다. 카드를 누르면 원본으로 갑니다.",
		"CanWrite":        false,
		"Role":            currentRole(c),
		"ScopeNote":       meetingScopeNote(selected),
		"HolidayBanner":   meetingHolidayBanner(dateStr),
	})
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
