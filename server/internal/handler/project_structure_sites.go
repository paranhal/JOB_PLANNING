package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-179B: 1단계 업무·2단계 하부 업무 사이트 등록 화면. 검색 · 지역 필터 · 결과 전체 선택(체크박스)으로 고른다.
// 1단계 후보 = 사업 사이트 풀, 2단계 후보 = 그 1단계 업무에 등록된 사이트.

type siteRegTarget struct {
	level string // phase | package
	id    string
	path  string // 이 화면의 경로(리다이렉트용)
}

func (t siteRegTarget) url(p *model.WorkProject, q url.Values) string {
	u := "/projects/" + url.PathEscape(p.ProjectID) + "/" + t.path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func siteRegBack(c echo.Context) url.Values {
	q := url.Values{}
	if v := strings.TrimSpace(c.FormValue("q")); v != "" {
		q.Set("q", v)
	}
	if v := strings.TrimSpace(c.FormValue("region")); v != "" {
		q.Set("region", v)
	}
	if v := strings.TrimSpace(c.FormValue("page")); v != "" {
		q.Set("page", v)
	}
	return q
}

func (h *ProjectHandler) siteRegPage(c echo.Context, level string) error {
	p, err := h.phaseContext(c, false)
	if err != nil {
		return err
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	var (
		res  repository.SiteCandPage
		perr error
		path string
		id   string
	)
	if level == "phase" {
		id = c.Param("phaseID")
		path = "phases/" + url.PathEscape(id) + "/sites"
		res, perr = h.repo.PhaseSiteCandidates(p.ProjectID, id, c.QueryParam("q"), c.QueryParam("region"), page)
	} else {
		id = c.Param("pkgID")
		path = "packages/" + url.PathEscape(id) + "/sites"
		res, perr = h.repo.PackageSiteCandidates(p.ProjectID, id, c.QueryParam("q"), c.QueryParam("region"), page)
	}
	if perr != nil {
		if strings.Contains(perr.Error(), "찾을 수 없습니다") {
			return echo.ErrNotFound
		}
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(perr.Error())))
	}
	flashKind, flashN := "", ""
	if ok := c.QueryParam("ok"); strings.Contains(ok, ":") {
		parts := strings.SplitN(ok, ":", 2)
		if parts[0] == "add" || parts[0] == "remove" {
			flashKind, flashN = parts[0], parts[1]
		}
	}
	return c.Render(http.StatusOK, "project/structure_sites.html", map[string]interface{}{
		"FlashKind": flashKind, "FlashN": flashN,
		"Title": p.DisplayName() + " · 사이트 등록", "Active": NavProjects, "Project": p,
		"Level": level, "TargetID": id, "Action": "/projects/" + url.PathEscape(p.ProjectID) + "/" + path,
		"Res": res, "CanEdit": canEditProjectPhase(c, h, p),
		"FlashOK": c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
		"SitesRegisterURL": "/projects/" + url.PathEscape(p.ProjectID) + "/sites",
		"PrevPage":         res.Page - 1, "NextPage": res.Page + 1,
	})
}

func (h *ProjectHandler) PhaseSitesPage(c echo.Context) error   { return h.siteRegPage(c, "phase") }
func (h *ProjectHandler) PackageSitesPage(c echo.Context) error { return h.siteRegPage(c, "package") }

func (h *ProjectHandler) siteRegAct(c echo.Context, level, op string) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	var t siteRegTarget
	if level == "phase" {
		t = siteRegTarget{level: level, id: c.Param("phaseID"), path: "phases/" + url.PathEscape(c.Param("phaseID")) + "/sites"}
	} else {
		t = siteRegTarget{level: level, id: c.Param("pkgID"), path: "packages/" + url.PathEscape(c.Param("pkgID")) + "/sites"}
	}
	back := siteRegBack(c)
	_ = c.Request().ParseForm()
	ids := c.Request().Form["site_id"]
	var n int
	var opErr error
	actor := currentUserID(c)
	switch {
	case level == "phase" && op == "add":
		n, opErr = h.repo.AddPhaseSites(p.ProjectID, t.id, ids, c.FormValue("all") == "1", c.FormValue("q"), c.FormValue("region"), actor)
	case level == "phase" && op == "remove":
		n, opErr = h.repo.RemovePhaseSites(p.ProjectID, t.id, ids, c.FormValue("reason"), actor)
	case level == "package" && op == "add":
		n, opErr = h.repo.AddPackageSites(p.ProjectID, t.id, ids, c.FormValue("all") == "1", c.FormValue("q"), c.FormValue("region"), actor)
	default:
		n, opErr = h.repo.RemovePackageSites(p.ProjectID, t.id, ids, c.FormValue("reason"), actor)
	}
	if opErr != nil {
		if strings.Contains(opErr.Error(), "찾을 수 없습니다") {
			return echo.ErrNotFound
		}
		back.Set("err", opErr.Error())
		return c.Redirect(http.StatusSeeOther, t.url(p, back))
	}
	table, idcol := "phase_sites", "phase_id"
	if level == "package" {
		table, idcol = "package_sites", "package_id"
	}
	label := map[string]string{"add": "사이트 등록", "remove": "사이트 제외"}[op]
	audit.LogWithReason(audit.ActionUpdate, table, idcol, t.id, label, "", strconv.Itoa(n), label+" "+strconv.Itoa(n)+"곳")
	back.Set("ok", op+":"+strconv.Itoa(n))
	return c.Redirect(http.StatusSeeOther, t.url(p, back))
}

func (h *ProjectHandler) PhaseSitesAdd(c echo.Context) error { return h.siteRegAct(c, "phase", "add") }
func (h *ProjectHandler) PhaseSitesRemove(c echo.Context) error {
	return h.siteRegAct(c, "phase", "remove")
}
func (h *ProjectHandler) PackageSitesAdd(c echo.Context) error {
	return h.siteRegAct(c, "package", "add")
}
func (h *ProjectHandler) PackageSitesRemove(c echo.Context) error {
	return h.siteRegAct(c, "package", "remove")
}
