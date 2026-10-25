package handler

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

const (
	loginBriefingCookie    = "login_briefing"
	loginBriefingOffCookie = "login_briefing_off"

	briefingScopeMe  = "me"
	briefingScopeOrg = "org"

	briefingHintBefore10 = "before10"
	briefingHintAfter10  = "after10"
	briefingHintNone     = "none"

	briefingStartRegister = "업무 시작하기"
	briefingStartView     = "업무 보기"
)

var briefingNow = time.Now

// LoginBriefingData 로그인 직후 브리핑 팝업.
type LoginBriefingData struct {
	Greeting        string
	Name            string
	Scope           string
	OrgName         string
	DailyCount      int
	UnassignedCount int
	UnplannedCount  int
	HintKind        string
	Hint            string
	StartHref       string
	StartLabel      string
	CanRegister     bool
	DailyHref       string
	UnassignedHref  string
}

func issueLoginBriefingCookie(c echo.Context) {
	c.SetCookie(sessionCookie(loginBriefingCookie, "1", 0))
}

func clearLoginBriefingCookie(c echo.Context) {
	c.SetCookie(sessionCookie(loginBriefingCookie, "", -1))
}

func loginBriefingOffToday(c echo.Context, now time.Time) bool {
	ck, err := c.Cookie(loginBriefingOffCookie)
	if err != nil || ck == nil {
		return false
	}
	return strings.TrimSpace(ck.Value) == now.In(now.Location()).Format(dateLayout)
}

func setLoginBriefingOffCookie(c echo.Context, now time.Time) {
	today := now.In(now.Location())
	tomorrow := time.Date(today.Year(), today.Month(), today.Day()+1, 0, 0, 0, 0, today.Location())
	ck := sessionCookie(loginBriefingOffCookie, today.Format(dateLayout), int(time.Until(tomorrow).Seconds()))
	ck.Expires = tomorrow
	c.SetCookie(ck)
}

func loginBriefingHome(path string) bool {
	return path == "/" || path == "/work" || path == pathSalesDashboard
}

func loginBriefingOrgRole(role string) bool {
	switch model.NormalizeRole(role) {
	case model.RoleOrgAdmin, model.RoleVisionAdmin:
		return true
	default:
		return false
	}
}

func briefingGreeting(name string, now time.Time) string {
	name = strings.TrimSpace(name)
	h := now.Hour()
	switch {
	case h < 12:
		return "좋은 아침이에요, " + name + " 님"
	case h < 18:
		return "좋은 오후예요, " + name + " 님"
	default:
		return "오늘도 수고 많으셨어요, " + name + " 님"
	}
}

func briefingHint(unassigned int, now time.Time) (kind, text string) {
	if unassigned <= 0 {
		return briefingHintNone, "미배정 업무가 없습니다. 오늘도 좋은 하루 되세요."
	}
	if now.Hour() < 10 {
		return briefingHintBefore10, "오전 10시까지 우선순위가 높은 미배정 업무를 확인해 주세요."
	}
	return briefingHintAfter10, "오늘 안에 미배정 업무의 날짜·시간을 정해 주세요."
}

func (h *Handler) InjectLoginBriefing(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if h == nil {
			return next(c)
		}
		req := c.Request()
		if req.Method != http.MethodGet || req.Header.Get("HX-Request") == "true" {
			return next(c)
		}
		if isSalesHomeRedirect(c) {
			return next(c)
		}
		if viewAsActive(c) || currentRole(c) == model.RoleCLevel {
			return next(c)
		}
		if !loginBriefingHome(req.URL.Path) {
			return next(c)
		}
		ck, err := c.Cookie(loginBriefingCookie)
		if err != nil || ck == nil || strings.TrimSpace(ck.Value) != "1" {
			return next(c)
		}
		now := briefingNow()
		if loginBriefingOffToday(c, now) {
			return next(c)
		}
		data, err := h.buildLoginBriefing(c, now)
		if err != nil {
			log.Printf("login briefing: %v", err)
			return next(c)
		}
		clearLoginBriefingCookie(c)
		c.Set("login_briefing", data)
		c.Set("show_assign_notice_modal", false)
		return next(c)
	}
}

func injectLoginBriefingView(c echo.Context, data map[string]interface{}) {
	if data == nil {
		return
	}
	if v := c.Get("login_briefing"); v != nil {
		data["LoginBriefing"] = v
	}
}

func (h *Handler) buildLoginBriefing(c echo.Context, now time.Time) (*LoginBriefingData, error) {
	name := strings.TrimSpace(ctxString(c, "user_name"))
	out := &LoginBriefingData{
		Name:        name,
		Greeting:    briefingGreeting(name, now),
		Scope:       briefingScopeMe,
		CanRegister: boardCanWrite(c),
		DailyHref:   "/workboard",
	}
	orgRole := loginBriefingOrgRole(currentRole(c))
	if orgRole {
		out.Scope = briefingScopeOrg
		var orgs []model.Org
		if v := c.Get("org_switch_list"); v != nil {
			if list, ok := v.([]model.Org); ok {
				orgs = list
			}
		}
		out.OrgName = orgViewLabel(c, orgs)
		daily, unassigned, unplanned, err := h.loginBriefingOrgCounts(c, now)
		if err != nil {
			return nil, err
		}
		out.DailyCount = daily
		out.UnassignedCount = unassigned
		out.UnplannedCount = unplanned
		canTeam, _ := meetingTeamScope(c)
		if canTeam {
			out.StartHref = "/meeting"
		} else {
			out.StartHref = "/workboard"
		}
		out.UnassignedHref = "/work"
		out.StartLabel = briefingStartRegister
	} else {
		if h.Workboard == nil {
			return nil, fmt.Errorf("workboard missing")
		}
		daily, unplaced, err := h.Workboard.loginBriefingStaffCounts(c, now)
		if err != nil {
			return nil, err
		}
		out.DailyCount = daily
		out.UnassignedCount = unplaced
		out.UnassignedHref = "/workboard#unplanned"
		if out.CanRegister {
			out.StartHref = "/workboard?new=1"
			out.StartLabel = briefingStartRegister
		} else {
			out.StartHref = "/workboard"
			out.StartLabel = briefingStartView
		}
	}
	out.HintKind, out.Hint = briefingHint(out.UnassignedCount, now)
	return out, nil
}

func (h *WorkboardHandler) loginBriefingStaffCounts(c echo.Context, now time.Time) (daily, unplaced int, err error) {
	if h == nil {
		return 0, 0, nil
	}
	day := now.Format(dateLayout)
	scope := resolveBoardScope(c)
	entries, err := h.collectBoardEntriesAuto(c, day, day, scope, false)
	if err != nil {
		return 0, 0, err
	}
	n := len(entries.Overdue)
	for _, it := range entries.Placed {
		d := strings.TrimSpace(it.Task.WorkDate)
		if d == "" {
			d = strings.TrimSpace(it.Task.DueDate)
		}
		if d == day {
			n++
		}
	}
	return n, len(entries.Unplaced), nil
}

func (h *Handler) loginBriefingOrgCounts(c echo.Context, now time.Time) (daily, unassigned, unplanned int, err error) {
	day := now.Format(dateLayout)
	org := currentOrg(c)
	if h.workBoard == nil {
		return 0, 0, 0, fmt.Errorf("work board missing")
	}
	todayItems, err := h.workBoard.ListScheduledOn(day, "", nil, 300)
	if err != nil {
		return 0, 0, 0, err
	}
	if h.Meeting != nil {
		todayItems = h.Meeting.filterMeetingOrg(c, todayItems)
	}
	daily = len(todayItems)

	raw, err := h.workBoard.CollectUnassigned(org)
	if err != nil {
		return 0, 0, 0, err
	}
	skip := dismissedKeys(c)
	for _, it := range raw {
		if skip[workAssignKey(it)] {
			continue
		}
		unassigned++
	}

	up, _, err := h.workBoard.ListUnplannedForOrg("", nil, "", org)
	if err != nil {
		return 0, 0, 0, err
	}
	var need []model.WorkListItem
	for _, u := range up {
		if !u.NeedPlanKind() {
			continue
		}
		need = append(need, u.WorkListItem)
	}
	if h.Meeting != nil {
		need = h.Meeting.filterMeetingOrg(c, need)
	}
	unplanned = len(need)
	return daily, unassigned, unplanned, nil
}

func (h *Handler) LoginBriefingOff(c echo.Context) error {
	if !requireCSRF(c) {
		return echo.NewHTTPError(http.StatusForbidden, "csrf")
	}
	setLoginBriefingOffCookie(c, briefingNow())
	return c.NoContent(http.StatusNoContent)
}
