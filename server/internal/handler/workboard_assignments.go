package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

const assignSeenCookie = "assign_seen"

// 95-225: 업무 배정 — 팝업 데이터(세션 첫 확인은 대기+보류, 이후 폴링은 대기만), 수락·거부·보류 처리, 보류 그룹 카드 데이터.

// AssignmentsPopup GET /workboard/assignments/popup?first=1
func (h *WorkboardHandler) AssignmentsPopup(c echo.Context) error {
	uid := strings.TrimSpace(currentUserID(c))
	if h.assign == nil || uid == "" {
		return c.JSON(http.StatusOK, map[string]interface{}{"items": []repository.AssignmentItem{}, "pending": 0, "held": 0})
	}
	// 로그인 세션의 첫 확인인지는 세션 쿠키로 판정한다(로그아웃 때 지워지므로 다음 로그인에 보류 항목을 다시 알린다).
	first := true
	if ck, err := c.Cookie(assignSeenCookie); err == nil && ck.Value == uid {
		first = false
	}
	items, err := h.assign.PopupItems(uid, first)
	if err != nil {
		return err
	}
	if first {
		c.SetCookie(&http.Cookie{Name: assignSeenCookie, Value: uid, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	}
	if items == nil {
		items = []repository.AssignmentItem{}
	}
	p, hd := h.assign.CountOpenForUser(uid)
	return c.JSON(http.StatusOK, map[string]interface{}{"items": items, "pending": p, "held": hd, "first": first})
}

// AssignmentDecide POST /workboard/assignments/:id/decide  (action=accept|reject|hold, reason)
func (h *WorkboardHandler) AssignmentDecide(c echo.Context) error {
	uid := strings.TrimSpace(currentUserID(c))
	if h.assign == nil || uid == "" {
		return echo.ErrForbidden
	}
	if isReadOnly(c) {
		return echo.ErrForbidden
	}
	fail := func(code int, msg string) error {
		if wantsJSON(c) || wantsJSONError(c) {
			return c.JSON(code, map[string]string{"error": msg})
		}
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+msg))
	}
	item, err := h.assign.Decide(c.Param("id"), uid, strings.TrimSpace(c.FormValue("action")), c.FormValue("reason"))
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrAssignNotMine):
			return fail(http.StatusForbidden, err.Error())
		case errors.Is(err, repository.ErrAssignReason), errors.Is(err, repository.ErrAssignTransition):
			return fail(http.StatusBadRequest, err.Error())
		case errors.Is(err, repository.ErrAssignClosed):
			return fail(http.StatusConflict, err.Error())
		}
		return fail(http.StatusBadRequest, err.Error())
	}
	if wantsJSON(c) || wantsJSONError(c) {
		p, hd := h.assign.CountOpenForUser(uid)
		return c.JSON(http.StatusOK, map[string]interface{}{"item": item, "pending": p, "held": hd})
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=assign"))
}

// heldAssignmentsFor 업무 화면의 「보류」 그룹 카드: 내가 보류한 배정(0건이면 카드를 숨긴다).
func (h *WorkboardHandler) heldAssignmentsFor(c echo.Context) []repository.AssignmentItem {
	uid := strings.TrimSpace(currentUserID(c))
	if h.assign == nil || uid == "" {
		return nil
	}
	list, _ := h.assign.ListForUser(uid, repository.AssignHeld)
	return list
}

type assignUserOption struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OrgName string `json:"org_name"`
}

// AssignmentUsers GET /workboard/assignments/users — 배정 대상 선택 목록(활성 사용자, 타 조직 포함). 이름과 조직명만 준다.
func (h *WorkboardHandler) AssignmentUsers(c echo.Context) error {
	if !canWriteWorkboard(c) || isReadOnly(c) {
		return echo.ErrForbidden
	}
	me := strings.TrimSpace(currentUserID(c))
	users, err := h.userRepo.ListAll()
	if err != nil {
		return err
	}
	orgNames := h.assign.OrgNames()
	out := []assignUserOption{}
	for _, u := range users {
		if !u.IsActive || u.UserID == me || u.IsReadOnly || u.IsTest || strings.TrimSpace(u.FullName) == "" {
			continue
		}
		out = append(out, assignUserOption{ID: u.UserID, Name: u.FullName, OrgName: orgNames[strings.TrimSpace(u.OrgID)]})
	}
	return c.JSON(http.StatusOK, out)
}

// createAssignmentsFromForm 작업 등록 폼의 「배정」 줄(assign_type · assign_target · assign_to)을 읽어 배정 요청을 만든다.
// 업무는 이미 저장된 뒤이므로 일부 줄이 실패해도 업무는 남기고, 실패 사유를 모아 돌려준다.
func (h *WorkboardHandler) createAssignmentsFromForm(c echo.Context, taskID string) (made int, problems []string) {
	if h.assign == nil || strings.TrimSpace(taskID) == "" {
		return 0, nil
	}
	_ = c.Request().ParseForm()
	types, targets, tos := c.Request().Form["assign_type"], c.Request().Form["assign_target"], c.Request().Form["assign_to"]
	from := strings.TrimSpace(currentUserID(c))
	for i := range tos {
		if i >= len(types) || i >= len(targets) || strings.TrimSpace(tos[i]) == "" || strings.TrimSpace(targets[i]) == "" {
			continue
		}
		if _, err := h.assign.Create(taskID, strings.TrimSpace(types[i]), strings.TrimSpace(targets[i]), from, strings.TrimSpace(tos[i])); err != nil {
			problems = append(problems, err.Error())
			continue
		}
		made++
	}
	return made, problems
}

// applyCollabFromForm 등록 폼의 「공동 작업」(collab=1, collab_scope=org|members, collab_member[]=사용자 ID)을 저장한다.
func (h *WorkboardHandler) applyCollabFromForm(c echo.Context, taskID string) error {
	if h.assign == nil || strings.TrimSpace(c.FormValue("collab")) != "1" {
		return nil
	}
	scope := strings.TrimSpace(c.FormValue("collab_scope"))
	if scope == "" {
		scope = repository.CollabOrg
	}
	_ = c.Request().ParseForm()
	return h.assign.SetCollab(taskID, scope, c.Request().Form["collab_member"], currentUserID(c))
}

// UpdateTaskCollab POST /workboard/tasks/:id/collab — 공동 작업 범위 변경·해제. 등록자·조직관리자만 할 수 있다.
func (h *WorkboardHandler) UpdateTaskCollab(c echo.Context) error {
	t, err := h.repo.GetTask(strings.TrimSpace(c.Param("id")))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return err
	}
	if !canManageCollab(c, t) {
		return echo.ErrForbidden
	}
	scope := strings.TrimSpace(c.FormValue("collab_scope"))
	if c.FormValue("collab") != "1" {
		scope = repository.CollabNone
	}
	_ = c.Request().ParseForm()
	if err := h.assign.SetCollab(t.TaskID, scope, c.Request().Form["collab_member"], currentUserID(c)); err != nil {
		if wantsJSON(c) || wantsJSONError(c) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+err.Error()))
	}
	if wantsJSON(c) || wantsJSONError(c) {
		return c.JSON(http.StatusOK, map[string]string{"collab_scope": scope})
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=collab"))
}

type assignSiteRow struct {
	CustomerID string
	Name       string
	Status     string
	WorkDate   string
}

// AssignmentShow GET /workboard/assignments/:id — 배정받은 범위만 보여 주는 화면. 수락자와 배정자만 열 수 있다.
// 사업 전체 정보는 주지 않는다: 사업명(또는 구분), 배정자, 대상, 그리고 대상 안의 사이트 체크만.
func (h *WorkboardHandler) AssignmentShow(c echo.Context) error {
	uid := strings.TrimSpace(currentUserID(c))
	if h.assign == nil || uid == "" {
		return echo.ErrNotFound
	}
	a, err := h.assign.Get(strings.TrimSpace(c.Param("id")))
	if err != nil || (a.ToUserID != uid && a.FromUserID != uid) {
		return echo.ErrNotFound
	}
	mine := a.ToUserID == uid
	var rows []assignSiteRow
	canCheck := mine && a.Status == repository.AssignAccepted && !isReadOnly(c)
	if canCheck && (a.TargetType == repository.AssignTargetSite || a.TargetType == repository.AssignTargetSub) {
		sites, _ := h.repo.ListTaskSites(a.TaskID)
		checks, _ := h.repo.ListSiteChecks(a.TaskID)
		last := map[string]repository.SiteProgressCell{}
		for _, ck := range checks {
			if cur, ok := last[ck.CustomerID]; !ok || ck.WorkDate >= cur.WorkDate {
				last[ck.CustomerID] = ck
			}
		}
		for _, s := range sites {
			if a.TargetType == repository.AssignTargetSite && s.SiteID != a.TargetID {
				continue
			}
			ck := last[s.SiteID]
			rows = append(rows, assignSiteRow{CustomerID: s.SiteID, Name: s.SiteName, Status: ck.Status, WorkDate: ck.WorkDate})
		}
	}
	return c.Render(http.StatusOK, "workboard/assignment_show.html", map[string]interface{}{
		"Title": "배정받은 업무", "Active": NavWorkPlan, "Item": a, "Mine": mine, "CanCheck": canCheck,
		"Sites": rows, "Today": time.Now().Format(dateLayout),
		"FlashOK": c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
	})
}
