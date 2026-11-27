package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *ASHandler) attachASProjectPicker(c echo.Context, as *model.ASReceipt, data map[string]interface{}) {
	if h == nil || as == nil || data == nil {
		return
	}
	// AS 조회(GetByID)는 project_id 를 읽지 않아 모델의 ProjectID 가 항상 비어 있다. 연결 여부를 화면에 맞게 보이려면 접수에서 직접 읽는다.
	if strings.TrimSpace(as.ProjectID) == "" && h.wbRepo != nil && h.wbRepo.DB() != nil {
		var pid string
		_ = h.wbRepo.DB().QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id=?`, as.ASID).Scan(&pid)
		as.ProjectID = strings.TrimSpace(pid)
	}
	name := ""
	if h.wbRepo != nil {
		if p, err := h.wbRepo.GetProject(as.ProjectID); err == nil && p != nil {
			name = strings.TrimSpace(p.Name)
		}
	}
	year := 0
	if !as.ReceiptDatetime.IsZero() {
		year = as.ReceiptDatetime.Year()
	}
	orgName := strings.TrimSpace(as.OrgName)
	if orgName == "" {
		if cust, ok := data["Customer"].(*model.Customer); ok && cust != nil {
			orgName = strings.TrimSpace(cust.OfficialName)
			if orgName == "" {
				orgName = strings.TrimSpace(cust.OrgName)
			}
		}
	}
	if h.wbRepo != nil {
		attachProjectPicker(c, pickerFromRepo(h.wbRepo), data, projectPickerOptsForAS(
			as.CustomerID, orgName, as.ProjectID, name, year,
		))
	}
	data["ProjectName"] = name
	data["CanLinkProject"] = canLinkASProject(c, as)
	h.attachASProjectCandidates(as, data)
}

// ASProjectCand 사업 연결 후보 한 건(한 번 클릭 연결 + 추천 표시). 95-228
type ASProjectCand struct {
	ID, Name, Why string
	Recommended   bool
}

// attachASProjectCandidates 사업 미연결 접수의 연결 후보와 업무 구분 선택지를 화면에 준다.
// 후보는 기존 연결 규칙(사이트 등록·범위 규칙)과 같고, 추천은 같은 담당자의 과거 연결 이력에서 읽는다.
func (h *ASHandler) attachASProjectCandidates(as *model.ASReceipt, data map[string]interface{}) {
	data["ASNoProject"] = strings.TrimSpace(as.ProjectID) == ""
	if h.wbRepo == nil || h.wbRepo.DB() == nil {
		return
	}
	db := h.wbRepo.DB()
	var kind string
	_ = db.QueryRow(`SELECT COALESCE(activity_kind,'') FROM work_tasks WHERE source_type IN ('as','as_work') AND (source_id=? OR source_id IN (SELECT work_id FROM as_work_items WHERE as_id=?)) AND COALESCE(activity_kind,'')<>'' LIMIT 1`,
		as.ASID, as.ASID).Scan(&kind)
	data["ASActivityLabel"] = model.WeeklyActivityLabel(kind)
	if !data["ASNoProject"].(bool) {
		return
	}
	pr := repository.NewProjectRepo(db)
	cands, err := pr.VisitProjectCandidates(as.CustomerID, "", model.ScopeWorkAS)
	if err != nil || len(cands) == 0 {
		data["ASCategoryKinds"] = model.SelectableWeeklyActivityKinds()
		return
	}
	ids := make([]string, 0, len(cands))
	for _, cd := range cands {
		ids = append(ids, cd.ProjectID)
	}
	recID, why := pr.RecommendASProject(as.CustomerID, as.AssignedTo, ids)
	out := make([]ASProjectCand, 0, len(cands))
	for _, cd := range cands {
		it := ASProjectCand{ID: cd.ProjectID, Name: cd.Name}
		if cd.ProjectID == recID {
			it.Recommended, it.Why = true, why
		}
		out = append(out, it)
	}
	data["ASProjectCands"] = out
	data["ASCategoryKinds"] = model.SelectableWeeklyActivityKinds()
}

// UpdateCategory POST /as/:id/category — 사업과 무관한 AS 를 업무 구분(사업 부문 등)으로 지정한다. 사업이 이미 연결돼 있으면 거절한다.
// 접수에는 구분 컬럼이 없어 그 접수에서 나온 업무(접수·하부업무)에 구분을 붙인다(사후 지정 이력이 남는다).
func (h *ASHandler) UpdateCategory(c echo.Context) error {
	id := c.Param("id")
	as, err := h.asInScope(c, id)
	if err != nil {
		return err
	}
	if !canLinkASProject(c, as) {
		return echo.ErrForbidden
	}
	back := asOpenHref(as.ASID)
	fail := func(msg string) error {
		return c.Redirect(http.StatusSeeOther, back+"?err="+url.QueryEscape(msg)+"#project")
	}
	kind := strings.TrimSpace(c.FormValue("activity_kind"))
	if !model.IsWeeklyActivitySelectable(kind) {
		return fail("업무 구분을 선택하세요")
	}
	var curPID string
	if h.wbRepo != nil && h.wbRepo.DB() != nil {
		_ = h.wbRepo.DB().QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id=?`, as.ASID).Scan(&curPID)
	}
	if strings.TrimSpace(curPID) != "" {
		return fail("사업이 연결된 접수에는 업무 구분을 지정하지 않습니다(사업과 구분은 함께 둘 수 없습니다)")
	}
	if h.wbRepo == nil || h.wbRepo.DB() == nil {
		return fail("업무를 찾을 수 없습니다")
	}
	rows, err := h.wbRepo.DB().Query(`SELECT task_id, COALESCE(org_id,'') FROM work_tasks
		WHERE (source_type='as' AND source_id=?) OR (source_type='as_work' AND source_id IN (SELECT work_id FROM as_work_items WHERE as_id=?))`, as.ASID, as.ASID)
	if err != nil {
		return fail("업무를 찾을 수 없습니다")
	}
	type tk struct{ id, org string }
	var tasks []tk
	for rows.Next() {
		var t tk
		if rows.Scan(&t.id, &t.org) == nil {
			tasks = append(tasks, t)
		}
	}
	rows.Close()
	if len(tasks) == 0 {
		return fail("이 접수에서 만든 업무가 없어 구분을 지정할 수 없습니다")
	}
	for _, t := range tasks {
		org := t.org
		if org == "" {
			org = as.OrgID
		}
		if _, _, err := h.wbRepo.SetTaskActivity(org, t.id, kind); err != nil {
			return fail(err.Error())
		}
	}
	return c.Redirect(http.StatusSeeOther, back+"?ok="+url.QueryEscape("업무 구분을 지정했습니다")+"#project")
}

func canLinkASProject(c echo.Context, as *model.ASReceipt) bool {
	if as == nil {
		return false
	}
	return canLinkASProjectDefault(c, as.AssignedTo, as.AssignedUserID, as.OrgID)
}

func canLinkASProjectDefault(c echo.Context, assignedTo, assignedUID, orgID string) bool {
	if isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	if currentRole(c) == model.RoleVisionAdmin {
		org := strings.TrimSpace(nz(c.FormValue("org"), currentOrg(c)))
		return org != "" && org != repository.OrgAll && org == orgID
	}
	if currentRole(c) == model.RoleOrgAdmin && loginHomeOrg(c) == orgID {
		return true
	}
	if currentRole(c) == model.RoleAdmin || currentRole(c) == model.RoleSupport {
		return true
	}
	return assigneeIsMine(c, assignedTo, assignedUID)
}

// UpdateProject 조치 내용과 별개로 사업만 연결한다. 완료·종결이어도 잠금을 요구하지 않는다.
func (h *ASHandler) UpdateProject(c echo.Context) error {
	id := c.Param("id")
	as, err := h.asInScope(c, id)
	if err != nil {
		return err
	}
	if !canLinkASProject(c, as) {
		return echo.ErrForbidden
	}
	pid := strings.TrimSpace(c.FormValue("project_id"))
	if h.wbRepo != nil {
		if err := requireProjectLink(c, h.wbRepo.GetProject, pid, false); err != nil {
			return err
		}
		if err := requireVisitSiteProjectFor(h.wbRepo, as.CustomerID, "", model.ScopeWorkAS, pid, as.ProjectID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	if _, _, err := h.repo.SetASReceiptProject(as.OrgID, as.ASID, pid); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	q := url.Values{}
	q.Set("ok", "project")
	return c.Redirect(http.StatusSeeOther, asOpenHref(as.ASID)+"?"+q.Encode()+"#project")
}

// checkReceiptProject 접수 화면에서 고른 사업이 연결 가능한지(열람 권한·이 기관이 사업 사이트이거나 범위 규칙이 가리키는 사업) 확인한다.
func (h *ASHandler) checkReceiptProject(c echo.Context, customerID, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	if h.wbRepo == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 확인하지 못했습니다")
	}
	if err := requireProjectLink(c, h.wbRepo.GetProject, projectID, false); err != nil {
		return err
	}
	if err := requireVisitSiteProjectFor(h.wbRepo, customerID, "", model.ScopeWorkAS, projectID, ""); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return nil
}

// httpErrMessage echo.HTTPError 의 사용자용 문구만 꺼낸다.
func httpErrMessage(err error) string {
	if he, ok := err.(*echo.HTTPError); ok {
		if m, ok := he.Message.(string); ok {
			return m
		}
	}
	return err.Error()
}
