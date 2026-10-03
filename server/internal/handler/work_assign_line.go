package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const workDismissCookie = "work_notice_dismiss"

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
	var parts []string
	for k := range seen {
		parts = append(parts, k)
	}
	http.SetCookie(c.Response(), &http.Cookie{
		Name: workDismissCookie, Value: strings.Join(parts, ","), Path: "/",
		MaxAge: 86400, SameSite: http.SameSiteLaxMode,
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
	if key == "" || date == "" {
		return c.Redirect(http.StatusSeeOther, assignNoticeBack(c)+qjoin(assignNoticeBack(c), "err=date"))
	}
	if err := h.repo.AssignUnplannedDate(key, date, assignee, uid); err != nil {
		return c.Redirect(http.StatusSeeOther, assignNoticeBack(c)+qjoin(assignNoticeBack(c), "err=assign"))
	}
	if nid := strings.TrimSpace(c.FormValue("notice_id")); nid != "" && h.notices != nil && h.notices.repo != nil {
		_ = h.notices.repo.MarkSeenAndActed([]string{nid}, h.notices.viewerID(c))
	}
	return c.Redirect(http.StatusSeeOther, assignNoticeBack(c))
}

func (h *WorkHandler) TransferOne(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	key := strings.TrimSpace(c.FormValue("key"))
	to := h.notices.resolveUser(c.FormValue("assignee_user_id"), c.FormValue("assignee"))
	if to == nil || key == "" {
		return c.Redirect(http.StatusSeeOther, assignNoticeBack(c)+qjoin(assignNoticeBack(c), "err=assignee"))
	}
	org := currentOrg(c)
	if org != repository.OrgAll && strings.TrimSpace(to.OrgID) != "" && strings.TrimSpace(to.OrgID) != org && to.Role != model.RoleVisionAdmin {
		return c.Redirect(http.StatusSeeOther, assignNoticeBack(c)+qjoin(assignNoticeBack(c), "err=org"))
	}
	kind, id := parseAssignKey(key)
	if err := h.transferByKey(c, kind, id, to); err != nil {
		return c.Redirect(http.StatusSeeOther, assignNoticeBack(c)+qjoin(assignNoticeBack(c), "err=transfer"))
	}
	if nid := strings.TrimSpace(c.FormValue("notice_id")); nid != "" && h.notices != nil && h.notices.repo != nil {
		_ = h.notices.repo.MarkSeenAndActed([]string{nid}, h.notices.viewerID(c))
	}
	return c.Redirect(http.StatusSeeOther, assignNoticeBack(c))
}

func parseAssignKey(key string) (kind, id string) {
	i := strings.Index(key, ":")
	if i < 0 {
		return "", key
	}
	return key[:i], key[i+1:]
}

func (h *WorkHandler) transferByKey(c echo.Context, kind, id string, to *model.User) error {
	switch kind {
	case "as":
		if h.asRepo == nil {
			return nil
		}
		as, err := h.asRepo.GetByID(currentOrg(c), id)
		if err != nil || as == nil {
			return err
		}
		as.AssignedTo = to.FullName
		as.AssignedUserID = to.UserID
		return h.asRepo.Update(as)
	case "mnt":
		if h.mntRepo == nil {
			return nil
		}
		v, err := h.mntRepo.GetVisit(id)
		if err != nil || v == nil {
			return err
		}
		v.Assignee = to.FullName
		return h.mntRepo.UpdateVisit(*v)
	case "task", "wi":
		if kind == "wi" {
			return nil
		}
		if h.wbRepo == nil {
			return nil
		}
		return h.wbRepo.SetTaskAssignee(id, to.FullName)
	default:
		return nil
	}
}

func (h *WorkHandler) HoldOne(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	key := strings.TrimSpace(c.FormValue("key"))
	reason := strings.TrimSpace(c.FormValue("hold_reason"))
	if key == "" || reason == "" {
		return c.Redirect(http.StatusSeeOther, assignNoticeBack(c)+qjoin(assignNoticeBack(c), "err=hold"))
	}
	kind, id := parseAssignKey(key)
	switch kind {
	case "as":
		if h.asRepo != nil {
			as, err := h.asRepo.GetByID(currentOrg(c), id)
			if err == nil && as != nil {
				as.Status = "hold"
				_ = h.asRepo.Update(as)
			}
		}
	case "task":
		if h.wbRepo != nil {
			t, err := h.wbRepo.GetTask(id)
			if err == nil && t != nil {
				t.Status = model.WBTaskHold
				t.HoldReason = reason
				t.WorkDate = ""
				_ = h.wbRepo.UpdateTask(t)
			}
		}
	case "mnt":
		if h.mntRepo != nil {
			v, err := h.mntRepo.GetVisit(id)
			if err == nil && v != nil {
				if v.Notes != "" {
					v.Notes = v.Notes + "\n"
				}
				v.Notes += "보류: " + reason
				_ = h.mntRepo.UpdateVisit(*v)
			}
		}
	}
	if nid := strings.TrimSpace(c.FormValue("notice_id")); nid != "" && h.notices != nil && h.notices.repo != nil {
		_ = h.notices.repo.MarkSeenAndActed([]string{nid}, h.notices.viewerID(c))
	}
	return c.Redirect(http.StatusSeeOther, assignNoticeBack(c))
}

func (h *WorkHandler) DismissOne(c echo.Context) error {
	key := strings.TrimSpace(c.FormValue("key"))
	if nid := strings.TrimSpace(c.FormValue("notice_id")); nid != "" && h.notices != nil && h.notices.repo != nil {
		_ = h.notices.repo.MarkSeenAndActed([]string{nid}, h.notices.viewerID(c))
	}
	if key != "" {
		addDismissedKey(c, key)
	}
	return c.Redirect(http.StatusSeeOther, assignNoticeBack(c))
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
	if h.wbRepo != nil {
		_ = h.wbRepo.CreateTask(t)
		if t.TaskID != "" && assignee != "" {
			_ = h.wbRepo.SetTaskAssignee(t.TaskID, assignee)
		}
	}
	return c.Redirect(http.StatusSeeOther, "/")
}
