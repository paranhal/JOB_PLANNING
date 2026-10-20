package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const workDismissCookie = "work_notice_dismiss"

// 브라우저 쿠키 한도는 보통 4096바이트. 오늘 하루만 제외한 키를 이 한도 안에서 보관한다.
const workDismissCookieMaxBytes = 3500

func workAssignKey(it model.WorkListItem) string {
	id := strings.TrimSpace(it.RefID)
	if strings.Contains(it.Href, "/as/work/") {
		return "wi:" + id
	}
	switch it.Prefix {
	case model.WorkPrefixAS:
		return "as:" + id
	case model.WorkPrefixMaintenance:
		return "mnt:" + id
	default:
		return "task:" + id
	}
}

func workEmbedHref(it model.WorkListItem) string {
	href := strings.TrimSpace(it.Href)
	if href == "" {
		return ""
	}
	sep := "?"
	if strings.Contains(href, "?") {
		sep = "&"
	}
	return href + sep + "embed=1"
}

func dismissedKeys(c echo.Context) map[string]bool {
	out := map[string]bool{}
	ck, err := c.Cookie(workDismissCookie)
	if err != nil || ck == nil {
		return out
	}
	for _, p := range strings.Split(ck.Value, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out[p] = true
		}
	}
	return out
}

func addDismissedKey(c echo.Context, key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	seen := dismissedKeys(c)
	seen[key] = true
	parts := make([]string, 0, len(seen))
	for k := range seen {
		parts = append(parts, k)
	}
	sort.Strings(parts)
	val := strings.Join(parts, ",")
	for len(val) > workDismissCookieMaxBytes && len(parts) > 1 {
		parts = parts[1:]
		val = strings.Join(parts, ",")
	}
	today := time.Now()
	tomorrow := time.Date(today.Year(), today.Month(), today.Day()+1, 0, 0, 0, 0, today.Location())
	http.SetCookie(c.Response(), &http.Cookie{
		Name: workDismissCookie, Value: val, Path: "/",
		Expires: tomorrow, MaxAge: int(time.Until(tomorrow).Seconds()),
		SameSite: http.SameSiteLaxMode,
	})
}

func filterSameOrgUsers(users []model.User, orgID string) []model.User {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == repository.OrgAll {
		return users
	}
	var out []model.User
	for _, u := range users {
		if strings.TrimSpace(u.OrgID) == orgID || u.Role == model.RoleVisionAdmin {
			out = append(out, u)
		}
	}
	return out
}

func (h *WorkHandler) AssignOne(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	key := strings.TrimSpace(c.FormValue("key"))
	date := strings.TrimSpace(c.FormValue("work_date"))
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	uid := strings.TrimSpace(c.FormValue("assignee_user_id"))
	if key == "" {
		return h.finishAssignNotice(c, "assign", "")
	}
	if date == "" {
		return h.finishAssignNotice(c, "date", "")
	}
	if err := h.requireAssigneeInOrg(c, assignee, uid); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he
		}
		return h.finishAssignNotice(c, unplannedErrQuery(err), "")
	}
	if err := h.repo.AssignUnplannedDate(key, date, assignee, uid); err != nil {
		return h.finishAssignNotice(c, unplannedErrQuery(err), "")
	}
	h.markNoticeActed(c)
	return h.finishAssignNotice(c, "", "")
}

func (h *WorkHandler) TransferOne(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	key := strings.TrimSpace(c.FormValue("key"))
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	uid := strings.TrimSpace(c.FormValue("assignee_user_id"))
	if key == "" {
		return h.finishAssignNotice(c, "assign", "")
	}
	if err := h.requireAssigneeInOrg(c, assignee, uid); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he
		}
		return h.finishAssignNotice(c, unplannedErrQuery(err), "")
	}
	if err := h.repo.TransferUnplanned(key, assignee, uid, c.FormValue("work_date")); err != nil {
		return h.finishAssignNotice(c, unplannedErrQuery(err), "")
	}
	h.markNoticeActed(c)
	return h.finishAssignNotice(c, "", "")
}

func parseAssignKey(key string) (kind, id string) {
	i := strings.Index(key, ":")
	if i < 0 {
		return "", key
	}
	return key[:i], key[i+1:]
}

func (h *WorkHandler) transferByKey(c echo.Context, kind, id string, to *model.User) error {
	if to == nil {
		return fmt.Errorf("담당자가 필요합니다")
	}
	return h.repo.TransferUnplanned(kind+":"+id, to.FullName, to.UserID, c.FormValue("work_date"))
}

func (h *WorkHandler) HoldOne(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	key := strings.TrimSpace(c.FormValue("key"))
	reason := strings.TrimSpace(c.FormValue("hold_reason"))
	if key == "" {
		return h.finishAssignNotice(c, "assign", "")
	}
	if reason == "" {
		return h.finishAssignNotice(c, "hold_reason", "")
	}
	if err := h.repo.HoldUnplanned(key, reason, c.FormValue("work_date")); err != nil {
		return h.finishAssignNotice(c, unplannedErrQuery(err), "")
	}
	h.markNoticeActed(c)
	return h.finishAssignNotice(c, "", "")
}

func (h *WorkHandler) DismissOne(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	key := strings.TrimSpace(c.FormValue("key"))
	h.markNoticeActed(c)
	if key != "" {
		addDismissedKey(c, key)
	}
	return h.finishAssignNotice(c, "", key)
}

func (h *WorkHandler) CreateAssign(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	kind := strings.TrimSpace(c.FormValue("kind"))
	if kind == "as" || kind == model.WorkPrefixAS {
		return c.Redirect(http.StatusSeeOther, "/as/new")
	}
	title := strings.TrimSpace(c.FormValue("title"))
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	date := strings.TrimSpace(c.FormValue("work_date"))
	if title == "" {
		return c.Redirect(http.StatusSeeOther, "/"+qjoin("/", "err=title"))
	}
	if date == "" {
		return c.Redirect(http.StatusSeeOther, "/"+qjoin("/", "err=date"))
	}
	if err := h.requireAssigneeInOrg(c, assignee, ""); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he
		}
		return c.Redirect(http.StatusSeeOther, "/"+qjoin("/", "err="+unplannedErrQuery(err)))
	}
	wt := model.WBWorkAdmin
	if kind == "sales" || kind == model.WorkPrefixSales {
		wt = model.WBWorkSales
	} else if kind == "support" || kind == model.WBWorkSupport {
		wt = model.WBWorkSupport
	} else if kind == "maintenance" {
		wt = model.WBWorkMaintenance
	}
	t := &model.WorkTask{
		WorkType: wt,
		Title:    title,
		Assignee: assignee,
		WorkDate: date,
		DueDate:  date,
		Status:   model.WBTaskWaiting,
		OrgID:    currentOrg(c),
	}
	if h.wbRepo == nil {
		return c.Redirect(http.StatusSeeOther, "/"+qjoin("/", "err=assign"))
	}
	if err := h.wbRepo.CreateTask(t); err != nil {
		return c.Redirect(http.StatusSeeOther, "/"+qjoin("/", "err=task"))
	}
	return c.Redirect(http.StatusSeeOther, "/"+qjoin("/", "ok=task"))
}

func (h *WorkHandler) requireAssigneeInOrg(c echo.Context, name, uid string) error {
	name, uid = strings.TrimSpace(name), strings.TrimSpace(uid)
	if name == "" && uid == "" {
		return fmt.Errorf("담당자가 필요합니다")
	}
	return checkAssigneeInOrg(h.userRepo, assignableOrgID(c), name, uid, "")
}

func (h *WorkHandler) markNoticeActed(c echo.Context) {
	nid := strings.TrimSpace(c.FormValue("notice_id"))
	if nid == "" || h.notices == nil || h.notices.repo == nil {
		return
	}
	_ = h.notices.repo.MarkSeenAndActed([]string{nid}, h.notices.viewerID(c))
}

func unplannedErrQuery(err error) string {
	if err == nil {
		return "assign"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "날짜"):
		return "date"
	case strings.Contains(msg, "보류 사유"):
		return "hold_reason"
	case strings.Contains(msg, "조직"):
		return "org"
	case strings.Contains(msg, "담당자"):
		return "assignee"
	case strings.Contains(msg, "지원하지"):
		return "unsupported"
	default:
		return "assign"
	}
}

func (h *WorkHandler) finishAssignNotice(c echo.Context, errCode, extraDismiss string) error {
	back := assignNoticeKeepPath(c)
	if errCode != "" {
		return c.Redirect(http.StatusSeeOther, back+qjoin(back, "assign=1&err="+errCode))
	}
	if h.assignNoticeStillOpen(c, extraDismiss) {
		return c.Redirect(http.StatusSeeOther, back+qjoin(back, "assign=1"))
	}
	setAssignNoticeShownCookie(c)
	return c.Redirect(http.StatusSeeOther, stripAssignOpen(back))
}

func assignNoticeKeepPath(c echo.Context) string {
	back := assignNoticeBack(c)
	if u := strings.TrimSpace(back); strings.Contains(u, "assign=1") {
		return u
	}
	return back
}

func stripAssignOpen(path string) string {
	path = strings.ReplaceAll(path, "?assign=1&", "?")
	path = strings.ReplaceAll(path, "&assign=1", "")
	path = strings.TrimSuffix(path, "?assign=1")
	return path
}

func (h *WorkHandler) assignNoticeStillOpen(c echo.Context, extraDismiss string) bool {
	if sliceLen(c.Get("login_helps")) > 0 {
		return true
	}
	if h.notices != nil && h.notices.repo != nil {
		uid := h.notices.viewerID(c)
		if n, err := h.notices.repo.CountUnseen(uid); err == nil && n > 0 {
			return true
		}
	}
	if h.repo == nil || !canWriteWorkboard(c) {
		return false
	}
	items, err := h.repo.CollectUnassigned(currentOrg(c))
	if err != nil {
		return false
	}
	skip := dismissedKeys(c)
	if extraDismiss != "" {
		skip[extraDismiss] = true
	}
	for _, it := range items {
		if !skip[workAssignKey(it)] {
			return true
		}
	}
	return false
}
