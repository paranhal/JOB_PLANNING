package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-183: 하부 업무(2단계) 순서·상태·삭제·복구, 계획(3단계) 사이트·수량 화면, 「변경 내용 저장」.

func (h *ProjectHandler) pkgRedirect(p *model.WorkProject, pkgID, msg string) string {
	return h.phaseRedirectAt(p, msg, "package-"+pkgID)
}

// PackageMove 하부 업무 순서(같은 단위 업무 안). dir=up|down
func (h *ProjectHandler) PackageMove(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	id := c.Param("pkgID")
	if _, _, err := h.repo.PackageMetaOf(p.ProjectID, id); err != nil {
		return echo.ErrNotFound
	}
	if err := h.repo.MovePackage(p.ProjectID, id, c.FormValue("dir")); err != nil {
		return c.Redirect(http.StatusSeeOther, h.pkgRedirect(p, id, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "phase_packages", "package_id", id, c.FormValue("dir"), "", "", "하부 업무 순서")
	return c.Redirect(http.StatusSeeOther, h.pkgRedirect(p, id, "ok=pkg-move"))
}

// PackageStatus 하부 업무 상태 auto|done|hold. 완료인데 끝나지 않은 사이트가 있으면 확인을 한 번 더 받는다.
func (h *ProjectHandler) PackageStatus(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	id := c.Param("pkgID")
	if _, _, err := h.repo.PackageMetaOf(p.ProjectID, id); err != nil {
		return echo.ErrNotFound
	}
	status := strings.TrimSpace(c.FormValue("status"))
	err = h.repo.SetPackageStatus(p.ProjectID, id, status, currentUserID(c), c.FormValue("confirm_open") == "1")
	var open *repository.OpenSitesError
	if errors.As(err, &open) {
		return c.Redirect(http.StatusSeeOther, h.pkgRedirect(p, id, "pkg_confirm="+url.QueryEscape(id)+"&open_n="+strconv.Itoa(open.N)))
	}
	if err != nil {
		return c.Redirect(http.StatusSeeOther, h.pkgRedirect(p, id, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "phase_packages", "package_id", id, status, "", status, "하부 업무 상태")
	return c.Redirect(http.StatusSeeOther, h.pkgRedirect(p, id, "ok=pkg-status"))
}

// PackageDelete 실적이 없으면 삭제, 있으면 비활성.
func (h *ProjectHandler) PackageDelete(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	id := c.Param("pkgID")
	_, name, err := h.repo.PackageMetaOf(p.ProjectID, id)
	if err != nil {
		return echo.ErrNotFound
	}
	off, im, err := h.repo.DeletePackage(p.ProjectID, id)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, h.pkgRedirect(p, id, "err="+url.QueryEscape(err.Error())))
	}
	if off {
		audit.LogWithReason(audit.ActionUpdate, "phase_packages", "package_id", id, name, "", "", fmt.Sprintf("하부 업무 비활성(작업 %d·계획 %d·사이트 %d·기록 %d)", im.Tasks, im.Plans, im.PlanSites, im.Events))
		return c.Redirect(http.StatusSeeOther, h.phaseRedirectAt(p, "ok=pkg-off", "project-phases"))
	}
	audit.LogWithReason(audit.ActionDelete, "phase_packages", "package_id", id, name, "", "", "하부 업무 삭제")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirectAt(p, "ok=pkg-del", "project-phases"))
}

// PackageRestore 비활성 하부 업무를 되돌린다.
func (h *ProjectHandler) PackageRestore(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	id := c.Param("pkgID")
	if err := h.repo.RestorePackage(p.ProjectID, id); err != nil {
		if strings.Contains(err.Error(), "찾을 수 없습니다") {
			return echo.ErrNotFound
		}
		return c.Redirect(http.StatusSeeOther, h.phaseRedirectAt(p, "err="+url.QueryEscape(err.Error()), "project-phases"))
	}
	audit.LogWithReason(audit.ActionUpdate, "phase_packages", "package_id", id, "", "", "", "하부 업무 되돌리기")
	return c.Redirect(http.StatusSeeOther, h.pkgRedirect(p, id, "ok=pkg-restore"))
}

// ---- 계획 사이트·수량 관리 화면 ----

func planSitesURL(p *model.WorkProject, planID string, q url.Values) string {
	u := "/projects/" + url.PathEscape(p.ProjectID) + "/plans/" + url.PathEscape(planID) + "/sites"
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// PlanSitesPage GET /projects/:id/plans/:planID/sites — 이 계획의 사이트(목표 수량 포함)와 하부 업무에 등록된 후보.
func (h *ProjectHandler) PlanSitesPage(c echo.Context) error {
	p, err := h.planContext(c, false)
	if err != nil {
		return err
	}
	planID := c.Param("planID")
	view, err := h.repo.PlanSites(p.ProjectID, planID)
	if err != nil {
		return echo.ErrNotFound
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	q, region := strings.TrimSpace(c.QueryParam("q")), strings.TrimSpace(c.QueryParam("region"))
	cands, total, err := h.repo.PlanSiteCandidates(p.ProjectID, planID, q, region, (page-1)*100, 100)
	if err != nil {
		return err
	}
	pages := (total + 99) / 100
	if pages < 1 {
		pages = 1
	}
	return c.Render(http.StatusOK, "project/plan_sites.html", map[string]interface{}{
		"Title": p.DisplayName() + " · 계획 사이트·수량", "Active": NavProjects, "Project": p,
		"View": view, "Cands": cands, "Total": total, "Q": q, "Region": region,
		"Page": page, "Pages": pages, "PrevPage": page - 1, "NextPage": page + 1,
		"CanManage": canManagePlans(c, h, p) && view.Active,
		"Base":      "/projects/" + url.PathEscape(p.ProjectID) + "/plans/" + url.PathEscape(planID),
		"Qty":       view.Basis == model.BasisQty,
		"FlashText": planSitesFlash(c.QueryParam("ok")), "FlashErr": c.QueryParam("err"),
	})
}

func planSitesFlash(ok string) string {
	parts := strings.SplitN(ok, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	switch parts[0] {
	case "add":
		return "사이트 " + parts[1] + "곳을 담았습니다."
	case "exclude":
		return "사이트 " + parts[1] + "곳을 제외했습니다."
	case "qty":
		return parts[1] + "곳의 목표 수량을 저장했습니다."
	}
	return ""
}

func formQty(c echo.Context, ids []string) map[string]int {
	qty := map[string]int{}
	for _, id := range ids {
		if n, err := strconv.Atoi(strings.TrimSpace(c.FormValue("qty_" + id))); err == nil && n > 0 {
			qty[id] = n
		}
	}
	return qty
}

// PlanMoveSites POST /plans/:planID/sites — 하부 업무에 등록된 사이트를 이 계획에 담거나 옮긴다(완료 사이트는 옮길 수 없다).
func (h *ProjectHandler) PlanMoveSites(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	_ = c.Request().ParseForm()
	planID := c.Param("planID")
	ids := c.Request().Form["site_id"]
	back := c.FormValue("back") == "sites"
	if err := h.repo.MovePlanSitesQty(p.ProjectID, planID, ids, formQty(c, ids), currentUserID(c)); err != nil {
		if back {
			return c.Redirect(http.StatusSeeOther, planSitesURL(p, planID, url.Values{"err": {err.Error()}}))
		}
		return c.Redirect(http.StatusSeeOther, h.phaseRedirectAt(p, "err="+url.QueryEscape(err.Error()), "plan-"+planID))
	}
	audit.LogWithReason(audit.ActionUpdate, "package_plans", "plan_id", planID, "옮기기", "", strconv.Itoa(len(ids)), "이 계획으로 옮기기")
	if back {
		return c.Redirect(http.StatusSeeOther, planSitesURL(p, planID, url.Values{"ok": {"add:" + strconv.Itoa(len(ids))}}))
	}
	return c.Redirect(http.StatusSeeOther, h.phaseRedirectAt(p, "ok=plan", "plan-"+planID))
}

// PlanExcludeSite POST /plans/:planID/exclude — 선택한 사이트를 계획에서 제외한다(사유 필수, 완료 사이트는 확인).
func (h *ProjectHandler) PlanExcludeSite(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	_ = c.Request().ParseForm()
	planID := c.Param("planID")
	ids := c.Request().Form["site_id"]
	back := c.FormValue("back") == "sites"
	if len(ids) == 0 {
		ids = []string{c.FormValue("site_id")}
	}
	confirm := c.FormValue("confirm_done") == "1"
	done := 0
	for _, id := range ids {
		if err := h.repo.ExcludePlanSite(p.ProjectID, planID, id, c.FormValue("reason"), confirm); err != nil {
			msg := err.Error()
			if done > 0 {
				msg = fmt.Sprintf("%d곳을 제외한 뒤 멈췄습니다: %s", done, msg)
			}
			if back {
				return c.Redirect(http.StatusSeeOther, planSitesURL(p, planID, url.Values{"err": {msg}}))
			}
			return c.Redirect(http.StatusSeeOther, h.phaseRedirectAt(p, "err="+url.QueryEscape(msg), "plan-"+planID))
		}
		done++
		audit.LogWithReason(audit.ActionUpdate, "package_sites", "plan_id", planID, id, "", c.FormValue("reason"), "사이트 제외")
	}
	if back {
		return c.Redirect(http.StatusSeeOther, planSitesURL(p, planID, url.Values{"ok": {"exclude:" + strconv.Itoa(done)}}))
	}
	return c.Redirect(http.StatusSeeOther, h.phaseRedirectAt(p, "ok=plan", "plan-"+planID))
}

// PlanSiteQty POST /plans/:planID/qty — 사이트별 목표 수량 저장. same_qty 와 선택한 사이트가 있으면 그 사이트에 같은 수량을 적용한다.
func (h *ProjectHandler) PlanSiteQty(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	_ = c.Request().ParseForm()
	planID := c.Param("planID")
	fail := func(msg string) error {
		return c.Redirect(http.StatusSeeOther, planSitesURL(p, planID, url.Values{"err": {msg}}))
	}
	qty := map[string]int{}
	if c.FormValue("apply_same") == "1" {
		n, convErr := strconv.Atoi(strings.TrimSpace(c.FormValue("same_qty")))
		ids := c.Request().Form["site_id"]
		if convErr != nil || n < 1 {
			return fail("같은 수량은 1 이상의 정수로 입력하세요")
		}
		if len(ids) == 0 {
			return fail("수량을 적용할 사이트를 고르세요")
		}
		for _, id := range ids {
			qty[id] = n
		}
	} else {
		view, err := h.repo.PlanSites(p.ProjectID, planID)
		if err != nil {
			return echo.ErrNotFound
		}
		for _, s := range view.Sites {
			raw := strings.TrimSpace(c.FormValue("qty_" + s.ID))
			if raw == "" {
				continue
			}
			n, convErr := strconv.Atoi(raw)
			if convErr != nil || n < 1 {
				return fail("목표 수량은 1 이상의 정수로 입력하세요")
			}
			qty[s.ID] = n
		}
	}
	pkgID, err := h.repo.SetPlanSiteQty(p.ProjectID, planID, qty)
	if err != nil {
		return fail(err.Error())
	}
	if h.wbRepo != nil {
		_ = h.wbRepo.RebuildPackageProgress(pkgID)
	}
	audit.LogWithReason(audit.ActionUpdate, "package_sites", "plan_id", planID, "목표 수량", "", strconv.Itoa(len(qty)), "사이트별 목표 수량")
	return c.Redirect(http.StatusSeeOther, planSitesURL(p, planID, url.Values{"ok": {"qty:" + strconv.Itoa(len(qty))}}))
}

// ---- 변경 내용 저장(C-2) ----

type batchReqItem struct {
	Kind   string            `json:"kind"`
	ID     string            `json:"id"`
	Fields map[string]string `json:"fields"`
}

// StructureBatch POST /projects/:id/structure/batch — 수정한 1단계·하부 업무·계획을 한 번에 저장한다(전부 성공 또는 전부 취소).
// 항목 종류마다 개별 저장과 같은 권한을 다시 검사한다: 1단계·하부 업무는 설계 권한자, 계획은 계획 권한자.
func (h *ProjectHandler) StructureBatch(c echo.Context) error {
	p, err := h.loadVisibleProject(c, true)
	if err != nil || !canViewProjects(c) {
		if err == nil {
			err = echo.ErrForbidden
		}
		return err
	}
	var req struct {
		Items []batchReqItem `json:"items"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "message": "요청을 읽을 수 없습니다"})
	}
	items := make([]repository.BatchItem, 0, len(req.Items))
	for i, it := range req.Items {
		if !repository.BatchKindAllowed(it.Kind) {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "index": i, "message": "알 수 없는 항목입니다"})
		}
		allowed := canEditProjectPhase(c, h, p)
		if it.Kind == repository.BatchPlan {
			allowed = canManagePlans(c, h, p)
		}
		if !allowed {
			return echo.ErrForbidden
		}
		items = append(items, repository.BatchItem{Kind: it.Kind, ID: it.ID, F: it.Fields})
	}
	done, err := h.repo.ApplyStructureBatch(p.ProjectID, items, currentUserID(c))
	if err != nil {
		var be *repository.BatchError
		if errors.As(err, &be) {
			return c.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
				"ok": false, "index": be.Index, "kind": be.Kind, "id": be.ID, "message": be.Msg,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, map[string]interface{}{"ok": false, "message": err.Error()})
	}
	for _, r := range done {
		table, pk := "project_phases", "phase_id"
		switch r.Kind {
		case repository.BatchPackage:
			table, pk = "phase_packages", "package_id"
		case repository.BatchPlan:
			table, pk = "package_plans", "plan_id"
		}
		audit.LogWithReason(audit.ActionUpdate, table, pk, r.ID, r.Label, "", "", r.Reason+"(변경 내용 저장)")
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "n": len(done)})
}

// structOKText 수행 구조 화면 위에 보여 줄 저장 결과 문구.
func structOKText(ok string) string {
	switch {
	case ok == "pkg":
		return "하부 업무를 추가했습니다."
	case ok == "pkg-meta":
		return "하부 업무를 저장했습니다."
	case ok == "pkg-move":
		return "하부 업무 순서를 바꿨습니다."
	case ok == "pkg-status":
		return "하부 업무 상태를 바꿨습니다."
	case ok == "pkg-del":
		return "하부 업무를 삭제했습니다."
	case ok == "pkg-off":
		return "실적(연결 작업·계획·사이트·기록)이 있어 삭제하지 않고 비활성으로 두었습니다. 아래 「비활성 하부 업무」에서 되돌릴 수 있습니다."
	case ok == "pkg-restore":
		return "하부 업무를 되돌렸습니다."
	case ok == "plan":
		return "작업 계획을 저장했습니다."
	case strings.HasPrefix(ok, "batch:"):
		return strings.TrimPrefix(ok, "batch:") + "곳을 저장했습니다."
	}
	return ""
}

// attachStructureEdit 95-183 화면 자료: 비활성 하부 업무, 열어 둘 수정 접힘, 완료 확인, 저장 결과 문구.
func (h *ProjectHandler) attachStructureEdit(c echo.Context, p *model.WorkProject, data map[string]interface{}) {
	data["StructOK"] = structOKText(c.QueryParam("ok"))
	data["OpenID"] = strings.TrimSpace(c.QueryParam("open"))
	data["PkgConfirmID"] = strings.TrimSpace(c.QueryParam("pkg_confirm"))
	data["PkgOpenN"] = c.QueryParam("open_n")
	// 하부 업무 추가 패널: 오류 뒤에 열어 두고 입력값을 되돌려 채운다. 95-185.
	data["AddOpenID"] = strings.TrimSpace(c.QueryParam("add_open"))
	data["AddName"] = c.QueryParam("add_name")
	data["AddTrack"] = c.QueryParam("add_track")
	data["AddBadge"] = c.QueryParam("add_badge")
	data["AddStart"] = c.QueryParam("add_start")
	data["AddEnd"] = c.QueryParam("add_end")
	inactive := map[string][]repository.InactivePackage{}
	if h.repo != nil {
		if list, err := h.repo.ListInactivePackages(p.ProjectID); err == nil {
			for _, x := range list {
				inactive[x.PhaseID] = append(inactive[x.PhaseID], x)
			}
		}
	}
	data["InactivePkgs"] = inactive
}
