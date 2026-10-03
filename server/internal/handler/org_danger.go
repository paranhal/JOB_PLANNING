package handler

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/backup"
	"customer-support/internal/repository"
)

func (h *OrgHandler) checkVisionPassword(c echo.Context, pw string) bool {
	pw = strings.TrimSpace(pw)
	if pw == "" {
		return false
	}
	if h.settings != nil {
		hash, _ := h.settings.Get(repository.SettingVisionAdminPassword)
		if strings.TrimSpace(hash) != "" {
			return verifyAndUpgradeSetting(h.settings, repository.SettingVisionAdminPassword, pw)
		}
	}
	return false
}

func (h *OrgHandler) snapshotOrg(orgID, prefix, kind string) (string, error) {
	org, err := h.repo.Get(orgID)
	if err != nil {
		return "", err
	}
	if org == nil {
		return "", echo.ErrNotFound
	}
	name, tmp, final, err := backup.PrepareOrgDir(h.dataDir, prefix, org.OrgName, time.Now())
	if err != nil {
		return "", err
	}
	if _, err := h.repo.WriteOrgSnapshot(tmp, orgID, kind); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	if err := backup.PromoteDir(tmp, final); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	audit.Use(h.db)
	audit.RecordBackup(kind, name, org.OrgName)
	return name, nil
}

func (h *OrgHandler) DeleteForm(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	org, err := h.repo.Get(id)
	if err != nil {
		return err
	}
	if org == nil {
		return echo.ErrNotFound
	}
	counts, err := h.repo.CountOrg(id)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "admin/org_delete.html", map[string]interface{}{
		"Title": "조직 숨기기", "Active": NavOrgs, "Org": org, "Counts": counts,
		"FlashErr": strings.TrimSpace(c.QueryParam("err")),
	})
}

func (h *OrgHandler) Delete(c echo.Context) error {
	id := strings.TrimSpace(c.FormValue("org_id"))
	org, err := h.repo.Get(id)
	if err != nil {
		return err
	}
	if org == nil {
		return echo.ErrNotFound
	}
	if strings.TrimSpace(c.FormValue("confirm_name")) != org.OrgName {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/delete?err="+url.QueryEscape("조직명을 그대로 입력하세요"))
	}
	if !h.checkVisionPassword(c, c.FormValue("vision_password")) {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/delete?err="+url.QueryEscape("비밀번호가 올바르지 않습니다"))
	}
	folder, err := h.snapshotOrg(id, backup.PrefixOrgDelete, backup.KindOrgDelete)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/delete?err="+url.QueryEscape(err.Error()))
	}
	if err := h.repo.Update(id, org.OrgName, org.ShortName, org.Note, false); err != nil {
		return err
	}
	audit.Use(h.db)
	audit.LogWithReason(audit.ActionUpdate, "orgs", "org_id", id, org.OrgName, `{"is_active":1}`, `{"is_active":0}`, "조직 숨김 백업="+folder)
	return c.Redirect(http.StatusSeeOther, "/admin/orgs?ok="+url.QueryEscape("조직을 숨겼습니다. 백업: "+folder))
}

func (h *OrgHandler) PurgeForm(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	org, err := h.repo.Get(id)
	if err != nil {
		return err
	}
	if org == nil {
		return echo.ErrNotFound
	}
	counts, _ := h.repo.CountOrg(id)
	return c.Render(http.StatusOK, "admin/org_purge.html", map[string]interface{}{
		"Title": "조직 완전 삭제", "Active": NavOrgs, "Org": org, "Counts": counts,
		"FlashErr": strings.TrimSpace(c.QueryParam("err")),
	})
}

func (h *OrgHandler) Purge(c echo.Context) error {
	id := strings.TrimSpace(c.FormValue("org_id"))
	org, err := h.repo.Get(id)
	if err != nil {
		return err
	}
	if org == nil {
		return echo.ErrNotFound
	}
	if strings.TrimSpace(c.FormValue("confirm_name")) != org.OrgName {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/purge?err="+url.QueryEscape("조직명을 그대로 입력하세요"))
	}
	if !h.checkVisionPassword(c, c.FormValue("vision_password")) {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/purge?err="+url.QueryEscape("비밀번호가 올바르지 않습니다"))
	}
	if !h.hasOrgBackup(id) {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/purge?err="+url.QueryEscape("조직 백업이 없습니다. 먼저 숨기기를 하세요"))
	}
	if _, err := h.repo.PurgeOrg(id); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/purge?err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/orgs?ok="+url.QueryEscape("조직 데이터를 지웠습니다."))
}

func (h *OrgHandler) hasOrgBackup(orgID string) bool {
	items, err := backup.ListOrgSnapshots(h.dataDir)
	if err != nil {
		return false
	}
	for _, it := range items {
		b, err := os.ReadFile(filepath.Join(h.dataDir, "backups", it.Name, "backup.txt"))
		if err != nil {
			continue
		}
		if strings.Contains(string(b), "org_id="+orgID) {
			return true
		}
	}
	return false
}

func (h *OrgHandler) RestoreForm(c echo.Context) error {
	items, err := backup.ListOrgSnapshots(h.dataDir)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "admin/org_restore.html", map[string]interface{}{
		"Title": "조직 복구", "Active": NavOrgs, "Items": items,
		"FlashErr": strings.TrimSpace(c.QueryParam("err")),
		"FlashOK":  strings.TrimSpace(c.QueryParam("ok")),
	})
}

func (h *OrgHandler) Restore(c echo.Context) error {
	if !h.checkVisionPassword(c, c.FormValue("vision_password")) {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/restore?err="+url.QueryEscape("비밀번호가 올바르지 않습니다"))
	}
	name := filepath.Base(strings.TrimSpace(c.FormValue("folder")))
	if !backup.IsOrgSnapshotDir(name) {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/restore?err="+url.QueryEscape("잘못된 폴더입니다"))
	}
	path := filepath.Join(h.dataDir, "backups", name, "org.db")
	if err := h.repo.RestoreOrg(path); err != nil {
		if errors.Is(err, repository.ErrOrgAlive) {
			return c.Redirect(http.StatusSeeOther, "/admin/orgs/restore?err="+url.QueryEscape(err.Error()))
		}
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/restore?err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/orgs?ok="+url.QueryEscape("조직을 복구했습니다."))
}

func (h *OrgHandler) SplitForm(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	org, err := h.repo.Get(id)
	if err != nil || org == nil {
		return echo.ErrNotFound
	}
	cust, err := h.repo.ListOrgCustomers(id)
	if err != nil {
		return err
	}
	orgs, err := h.repo.ListActive()
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "admin/org_split.html", map[string]interface{}{
		"Title": "조직 분리", "Active": NavOrgs, "Org": org, "Customers": cust, "Orgs": orgs,
		"Preview":  nil,
		"Selected": map[string]bool{},
		"FlashErr": strings.TrimSpace(c.QueryParam("err")),
		"ToOrg":    strings.TrimSpace(c.QueryParam("to")),
	})
}

func (h *OrgHandler) SplitPreview(c echo.Context) error {
	id := strings.TrimSpace(c.FormValue("org_id"))
	to := strings.TrimSpace(c.FormValue("to_org_id"))
	ids := c.Request().Form["customer_id"]
	if len(ids) == 0 {
		_ = c.Request().ParseForm()
		ids = c.Request().PostForm["customer_id"]
	}
	p, err := h.repo.PreviewSplit(id, to, ids)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/split?err="+url.QueryEscape(err.Error())+"&to="+url.QueryEscape(to))
	}
	org, _ := h.repo.Get(id)
	cust, _ := h.repo.ListOrgCustomers(id)
	orgs, _ := h.repo.ListActive()
	return c.Render(http.StatusOK, "admin/org_split.html", map[string]interface{}{
		"Title": "조직 분리", "Active": NavOrgs, "Org": org, "Customers": cust, "Orgs": orgs,
		"Preview": p, "Selected": orgSelectedIDs(ids), "ToOrg": to,
	})
}

func orgSelectedIDs(ids []string) map[string]bool {
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func (h *OrgHandler) Split(c echo.Context) error {
	id := strings.TrimSpace(c.FormValue("org_id"))
	to := strings.TrimSpace(c.FormValue("to_org_id"))
	_ = c.Request().ParseForm()
	ids := c.Request().PostForm["customer_id"]
	if !h.checkVisionPassword(c, c.FormValue("vision_password")) {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/split?err="+url.QueryEscape("비밀번호가 올바르지 않습니다")+"&to="+url.QueryEscape(to))
	}
	if _, err := h.repo.PreviewSplit(id, to, ids); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/split?err="+url.QueryEscape(err.Error())+"&to="+url.QueryEscape(to))
	}
	if _, err := h.snapshotOrg(id, backup.PrefixOrgSplit, backup.KindOrgSplit); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/split?err="+url.QueryEscape(err.Error()))
	}
	got, err := h.repo.ApplySplit(id, to, ids)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs/"+url.PathEscape(id)+"/split?err="+url.QueryEscape(err.Error()))
	}
	msg := "옮겼습니다: 고객 " + strconv.Itoa(got.Customers)
	return c.Redirect(http.StatusSeeOther, "/admin/orgs?ok="+url.QueryEscape(msg))
}
