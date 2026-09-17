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

const assignNoticeShownCookie = "assign_notice_shown"

type assignNoticeHook struct {
	repo  *repository.AssignNoticeRepo
	users *repository.UserRepo
}

func newAssignNoticeHook(dbRepo *repository.AssignNoticeRepo, users *repository.UserRepo) *assignNoticeHook {
	if dbRepo == nil {
		return nil
	}
	return &assignNoticeHook{repo: dbRepo, users: users}
}

func (h *assignNoticeHook) Record(c echo.Context, sourceType, sourceID, newName, newUID, oldName, oldUID string) {
	if h == nil || h.repo == nil {
		return
	}
	sourceType = strings.TrimSpace(sourceType)
	sourceID = strings.TrimSpace(sourceID)
	if sourceType == "" || sourceID == "" {
		return
	}
	byUID := strings.TrimSpace(ctxString(c, "user_id"))
	byName := strings.TrimSpace(ctxString(c, "user_name"))
	byUser := strings.TrimSpace(ctxString(c, "username"))
	if strings.TrimSpace(newName) == "" && strings.TrimSpace(newUID) == "" {
		_ = h.repo.RecordAssignment(sourceType, sourceID, "", byUID)
		return
	}
	to := h.resolveUser(newUID, newName)
	if to == nil {
		return
	}
	if isSelfAssign(byUID, byName, byUser, to.UserID, to.FullName, to.Username) {
		_ = h.repo.RecordAssignment(sourceType, sourceID, to.UserID, to.UserID)
		return
	}
	_ = h.repo.RecordAssignment(sourceType, sourceID, to.UserID, byUID)
}

func (h *assignNoticeHook) resolveUser(uid, name string) *model.User {
	if h == nil || h.users == nil {
		return nil
	}
	if u := h.users.FindAssignable(uid); u != nil {
		return u
	}
	return h.users.FindAssignable(name)
}

func (h *assignNoticeHook) viewerID(c echo.Context) string {
	uid := strings.TrimSpace(ctxString(c, "user_id"))
	name := strings.TrimSpace(ctxString(c, "user_name"))
	user := strings.TrimSpace(ctxString(c, "username"))
	if u := h.resolveUser(uid, name); u != nil {
		return u.UserID
	}
	if u := h.resolveUser("", user); u != nil {
		return u.UserID
	}
	return uid
}

func recordTaskNotice(h *assignNoticeHook, c echo.Context, t *model.WorkTask, oldAssignee string) {
	if h == nil || t == nil || strings.TrimSpace(t.TaskID) == "" {
		return
	}
	st := strings.TrimSpace(t.SourceType)
	if st == model.WBSourceAS || st == model.WBSourceMaintenance {
		return
	}
	if strings.TrimSpace(t.Assignee) == "" && strings.TrimSpace(oldAssignee) == "" {
		return
	}
	h.Record(c, model.AssignNoticeSourceTask, t.TaskID, t.Assignee, "", oldAssignee, "")
}

func isSelfAssign(byUID, byName, byUsername, toUID, toName, toUsername string) bool {
	byUID, toUID = strings.TrimSpace(byUID), strings.TrimSpace(toUID)
	if byUID != "" && toUID != "" && byUID == toUID {
		return true
	}
	toName = strings.TrimSpace(toName)
	toUsername = strings.TrimSpace(toUsername)
	byName = strings.TrimSpace(byName)
	byUsername = strings.TrimSpace(byUsername)
	if toName != "" && (toName == byName || toName == byUsername) {
		return true
	}
	if toUsername != "" && (toUsername == byName || toUsername == byUsername) {
		return true
	}
	return false
}

func assignNoticeShownToday(c echo.Context) bool {
	ck, err := c.Cookie(assignNoticeShownCookie)
	if err != nil || ck == nil {
		return false
	}
	return strings.TrimSpace(ck.Value) == time.Now().Format("2006-01-02")
}

func setAssignNoticeShownCookie(c echo.Context) {
	today := time.Now()
	tomorrow := time.Date(today.Year(), today.Month(), today.Day()+1, 0, 0, 0, 0, today.Location())
	c.SetCookie(&http.Cookie{
		Name:     assignNoticeShownCookie,
		Value:    today.Format("2006-01-02"),
		Path:     "/",
		Expires:  tomorrow,
		MaxAge:   int(time.Until(tomorrow).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// InjectAssignNotices 사이드바 뱃지와 하루 한 번 모달을 넣는다. 조회가 UPDATE 를 돌리지 않는다. §42.5
func (h *Handler) InjectAssignNotices(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if h == nil || h.notices == nil || h.notices.repo == nil {
			return next(c)
		}
		uid := h.notices.viewerID(c)
		if uid == "" {
			return next(c)
		}
		path := c.Request().URL.Path
		if path == "/login" || path == "/logout" || path == "/version" || strings.HasPrefix(path, "/static") || strings.HasPrefix(path, "/api") {
			return next(c)
		}
		n, err := h.notices.repo.CountUnseen(uid)
		if err != nil {
			return next(c)
		}
		c.Set("assign_notice_unread", n)
		showModal := n > 0 && c.Request().Method == http.MethodGet &&
			c.Request().Header.Get("HX-Request") != "true" && !assignNoticeShownToday(c)
		if showModal {
			items, err := h.notices.repo.ListUnseen(uid)
			if err == nil && len(items) > 0 {
				c.Set("assign_notices", items)
				c.Set("show_assign_notice_modal", true)
				if h.notices.users != nil {
					if users, e := h.notices.users.ListAssignable(); e == nil {
						c.Set("assign_notice_assignees", users)
					}
				}
				setAssignNoticeShownCookie(c)
			}
		}
		return next(c)
	}
}

func injectAssignNoticeView(c echo.Context, data map[string]interface{}) {
	if data == nil {
		return
	}
	data["AssignNoticeUnread"] = 0
	data["ShowAssignNoticeModal"] = false
	if v := c.Get("assign_notice_unread"); v != nil {
		data["AssignNoticeUnread"] = v
	}
	if v := c.Get("show_assign_notice_modal"); v != nil {
		data["ShowAssignNoticeModal"] = v
	}
	if v := c.Get("assign_notices"); v != nil {
		data["AssignNotices"] = v
	}
	if v := c.Get("assign_notice_assignees"); v != nil {
		data["AssignNoticeAssignees"] = v
	}
	if _, ok := data["AssignNoticeDate"]; !ok {
		data["AssignNoticeDate"] = time.Now().Format("2006-01-02")
	}
}

func assignNoticeBack(c echo.Context) string {
	back := strings.TrimSpace(c.FormValue("back"))
	if back == "" {
		if ref := strings.TrimSpace(c.Request().Header.Get("Referer")); ref != "" {
			if u, err := url.Parse(ref); err == nil && u.Path != "" && u.Path != "/login" {
				if u.RawQuery != "" {
					return u.Path + "?" + u.RawQuery
				}
				return u.Path
			}
		}
	}
	if back == "" {
		return "/work"
	}
	return back
}

func assignNoticeIDs(c echo.Context) []string {
	ids := c.Request().Form["notice_id"]
	if len(ids) == 0 {
		if params, err := c.FormParams(); err == nil {
			ids = params["notice_id"]
		}
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// AssignNoticeLater 「나중에」— 그날 모달을 다시 띄우지 않는다. 안 본 건수는 남긴다. §42.5
func (h *WorkHandler) AssignNoticeLater(c echo.Context) error {
	setAssignNoticeShownCookie(c)
	return c.Redirect(http.StatusSeeOther, assignNoticeBack(c))
}

// AssignNoticeAdd 고른 알림을 일일 업무에 넣는다. §42.5
func (h *WorkHandler) AssignNoticeAdd(c echo.Context) error {
	if h == nil || h.notices == nil || h.notices.repo == nil {
		return c.Redirect(http.StatusSeeOther, assignNoticeBack(c))
	}
	date, err := model.ParseAppDate(c.FormValue("work_date"))
	if err != nil || date == "" {
		return c.Redirect(http.StatusSeeOther, assignNoticeBack(c)+qjoin(assignNoticeBack(c), "err=date"))
	}
	uid := h.notices.viewerID(c)
	var done []string
	for _, id := range assignNoticeIDs(c) {
		n, err := h.notices.repo.GetOwned(id, uid)
		if err != nil || n == nil {
			continue
		}
		if err := h.applyNoticeAdd(n, date); err != nil {
			continue
		}
		done = append(done, id)
	}
	_ = h.notices.repo.MarkSeenAndActed(done, uid)
	setAssignNoticeShownCookie(c)
	return c.Redirect(http.StatusSeeOther, assignNoticeBack(c))
}

// AssignNoticeTransfer 고른 알림을 다른 담당자에게 넘긴다. 이력 표는 §42.6. §42.5
func (h *WorkHandler) AssignNoticeTransfer(c echo.Context) error {
	if h == nil || h.notices == nil || h.notices.repo == nil {
		return c.Redirect(http.StatusSeeOther, assignNoticeBack(c))
	}
	to := h.notices.resolveUser(c.FormValue("assignee_user_id"), c.FormValue("assignee"))
	if to == nil {
		return c.Redirect(http.StatusSeeOther, assignNoticeBack(c)+qjoin(assignNoticeBack(c), "err=assignee"))
	}
	uid := h.notices.viewerID(c)
	var done []string
	for _, id := range assignNoticeIDs(c) {
		n, err := h.notices.repo.GetOwned(id, uid)
		if err != nil || n == nil {
			continue
		}
		if err := h.applyNoticeTransfer(c, n, to); err != nil {
			continue
		}
		done = append(done, id)
	}
	_ = h.notices.repo.MarkSeenAndActed(done, uid)
	setAssignNoticeShownCookie(c)
	return c.Redirect(http.StatusSeeOther, assignNoticeBack(c))
}

func (h *WorkHandler) applyNoticeAdd(n *model.AssignNotice, date string) error {
	if h.repo == nil || n == nil {
		return nil
	}
	key := ""
	switch n.SourceType {
	case model.AssignNoticeSourceAS:
		key = "as:" + n.SourceID
	case model.AssignNoticeSourceMaintenance:
		key = "mnt:" + n.SourceID
	case model.AssignNoticeSourceTask:
		key = "task:" + n.SourceID
	default:
		return nil
	}
	if err := h.repo.AssignUnplannedDate(key, date, "", ""); err != nil {
		return err
	}
	switch n.SourceType {
	case model.AssignNoticeSourceAS:
		h.syncASPlannedFromUnplanned(n.SourceID)
	case model.AssignNoticeSourceMaintenance:
		if h.wbRepo != nil {
			_, _ = h.wbRepo.EnsureMaintenanceTasks()
			_, _ = h.wbRepo.SyncMaintenanceTaskFromVisit(n.SourceID)
		}
	}
	return nil
}

func (h *WorkHandler) applyNoticeTransfer(c echo.Context, n *model.AssignNotice, to *model.User) error {
	if n == nil || to == nil {
		return nil
	}
	switch n.SourceType {
	case model.AssignNoticeSourceAS:
		if h.asRepo == nil {
			return nil
		}
		as, err := h.asRepo.GetByID(n.SourceID)
		if err != nil || as == nil {
			return err
		}
		as.AssignedTo = to.FullName
		as.AssignedUserID = to.UserID
		if err := h.asRepo.Update(as); err != nil {
			return err
		}
		h.syncASPlannedFromUnplanned(as.ASID)
		h.notices.Record(c, model.AssignNoticeSourceAS, as.ASID, to.FullName, to.UserID, n.UserID, n.UserID)
	case model.AssignNoticeSourceMaintenance:
		if h.mntRepo == nil {
			return nil
		}
		v, err := h.mntRepo.GetVisit(n.SourceID)
		if err != nil || v == nil {
			return err
		}
		v.Assignee = to.FullName
		if err := h.mntRepo.UpdateVisit(*v); err != nil {
			return err
		}
		if h.wbRepo != nil {
			_, _ = h.wbRepo.SyncMaintenanceTaskFromVisit(v.VisitID)
		}
		h.notices.Record(c, model.AssignNoticeSourceMaintenance, v.VisitID, to.FullName, to.UserID, n.UserID, n.UserID)
	case model.AssignNoticeSourceTask:
		if h.wbRepo == nil {
			return nil
		}
		if err := h.wbRepo.SetTaskAssignee(n.SourceID, to.FullName); err != nil {
			return err
		}
		h.notices.Record(c, model.AssignNoticeSourceTask, n.SourceID, to.FullName, to.UserID, n.UserID, n.UserID)
	}
	return nil
}
