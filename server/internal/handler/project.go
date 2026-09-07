package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type ProjectHandler struct {
	repo         *repository.ProjectRepo
	wbRepo       *repository.WBRepo
	customerRepo *repository.CustomerRepo
	contactRepo  *repository.ContactRepo
	codeRepo     *repository.CodeRepo
	assetRepo    *repository.AssetRepo
}

func NewProjectHandler(
	repo *repository.ProjectRepo,
	wbRepo *repository.WBRepo,
	customerRepo *repository.CustomerRepo,
	contactRepo *repository.ContactRepo,
	codeRepo *repository.CodeRepo,
	assetRepo *repository.AssetRepo,
) *ProjectHandler {
	return &ProjectHandler{
		repo: repo, wbRepo: wbRepo,
		customerRepo: customerRepo, contactRepo: contactRepo, codeRepo: codeRepo,
		assetRepo: assetRepo,
	}
}

func canViewProjects(c echo.Context) bool {
	r := currentRole(c)
	return r == model.RoleAdmin || r == model.RoleOffice || r == model.RoleTech ||
		r == model.RoleObserver || hasPerm(c, model.PermWorkboard)
}

// canWriteProjects 등록·수정·보관/재개 — 일일업무와 동일
func canWriteProjects(c echo.Context) bool {
	if isObserverRole(c) {
		return false
	}
	r := currentRole(c)
	return r == model.RoleAdmin || r == model.RoleOffice || r == model.RoleTech ||
		hasPerm(c, model.PermWorkboard)
}

func canDeleteProjects(c echo.Context) bool {
	return isAdminRole(c)
}

func (h *ProjectHandler) List(c echo.Context) error {
	if !canViewProjects(c) {
		return echo.ErrForbidden
	}
	year, _ := strconv.Atoi(strings.TrimSpace(c.QueryParam("year")))
	status := strings.TrimSpace(c.QueryParam("status"))
	search := strings.TrimSpace(c.QueryParam("search"))
	items, err := h.repo.ListFiltered(search, year, status)
	if err != nil {
		return err
	}
	years, _ := h.repo.ListYears()
	return c.Render(http.StatusOK, "project/list.html", map[string]interface{}{
		"Title": "사업(프로젝트)관리", "Active": NavProjects,
		"Projects": items, "Years": years, "Year": year, "Status": status, "Search": search,
		"CanWrite": canWriteProjects(c),
		"FlashOK":  c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
	})
}

func (h *ProjectHandler) New(c echo.Context) error {
	if !canWriteProjects(c) {
		return echo.ErrForbidden
	}
	return h.renderForm(c, &model.WorkProject{IsPaid: true, Status: model.WBProjectActive, Color: "#3B82F6"}, nil, false)
}

func (h *ProjectHandler) Create(c echo.Context) error {
	if !canWriteProjects(c) {
		return echo.ErrForbidden
	}
	p, rules := h.parseForm(c)
	if p.Name == "" {
		return h.renderForm(c, p, rules, false, "사업명을 입력하세요.")
	}
	if err := h.repo.Create(p); err != nil {
		return err
	}
	if err := h.repo.ReplaceRules(p.ProjectID, rules); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?ok=created")
}

func (h *ProjectHandler) Show(c echo.Context) error {
	if !canViewProjects(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	p, err := h.repo.Get(id)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects?err=notfound")
	}
	asRows, asTotal, _ := h.repo.MatchAS(id, 20)
	mntRows, mntTotal, _ := h.repo.MatchMaintenance(id, 20)
	taskRows, taskTotal, _ := h.repo.MatchAdminTasks(id, 20)
	assetTotal, _ := h.assetRepo.CountByProject(id)
	p.ASCount, p.MntCount, p.TaskCount, p.AssetCount = asTotal, mntTotal, taskTotal, assetTotal
	return c.Render(http.StatusOK, "project/show.html", map[string]interface{}{
		"Title": p.DisplayName(), "Active": NavProjects,
		"Project": p, "ASRows": asRows, "MntRows": mntRows, "TaskRows": taskRows,
		"CanWrite":  canWriteProjects(c),
		"CanDelete": canDeleteProjects(c),
		"FlashOK":   c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
	})
}

func (h *ProjectHandler) Edit(c echo.Context) error {
	if !canWriteProjects(c) {
		return echo.ErrForbidden
	}
	p, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects?err=notfound")
	}
	return h.renderForm(c, p, p.ScopeRules, true)
}

func (h *ProjectHandler) Update(c echo.Context) error {
	if !canWriteProjects(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	existing, err := h.repo.Get(id)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects?err=notfound")
	}
	p, rules := h.parseForm(c)
	p.ProjectID = existing.ProjectID
	p.SalesProjectID = existing.SalesProjectID
	if p.Name == "" {
		return h.renderForm(c, p, rules, true, "사업명을 입력하세요.")
	}
	if err := h.repo.Update(p); err != nil {
		return err
	}
	if err := h.repo.ReplaceRules(p.ProjectID, rules); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?ok=updated")
}

func (h *ProjectHandler) Archive(c echo.Context) error {
	if !canWriteProjects(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	if _, err := h.repo.Get(id); err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects?err=notfound")
	}
	if err := h.wbRepo.SetProjectStatus(id, model.WBProjectArchived); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+id+"?ok=archived")
}

func (h *ProjectHandler) Activate(c echo.Context) error {
	if !canWriteProjects(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	if _, err := h.repo.Get(id); err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects?err=notfound")
	}
	if err := h.wbRepo.SetProjectStatus(id, model.WBProjectActive); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+id+"?ok=activated")
}

func (h *ProjectHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	if !canDeleteProjects(c) {
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionDelete,
			TargetTable: "work_projects",
			TargetID:    id,
			Detail:      "권한없음",
			Reason:      "권한없음",
			Result:      auditlog.ResultDeny,
		})
		return echo.ErrForbidden
	}
	p, _ := h.wbRepo.GetProject(id)
	subjectType, subjectID, subjectName := "project", id, id
	if p != nil {
		subjectName = p.Name
		if p.CustomerID != "" {
			subjectType, subjectID, subjectName = "customer", p.CustomerID, p.CustomerName
			if subjectName == "" {
				subjectName = p.Name
			}
		}
	}
	if err := h.wbRepo.DeleteProject(id); err != nil {
		result := auditlog.ResultDeny
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionDelete,
			TargetTable: "work_projects",
			TargetID:    id,
			SubjectType: subjectType,
			SubjectID:   subjectID,
			SubjectName: subjectName,
			Detail:      err.Error(),
			Reason:      accessReason(c, "사업 삭제"),
			Result:      result,
			BeforeJSON:  toJSON(p),
		})
		if strings.Contains(err.Error(), "연결") {
			return c.Redirect(http.StatusSeeOther, "/projects/"+id+"?err=linked")
		}
		return err
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionDelete,
		TargetTable: "work_projects",
		TargetID:    id,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		SubjectName: subjectName,
		Detail:      "사업 삭제",
		Reason:      accessReason(c, "사업 삭제"),
		Result:      auditlog.ResultOK,
		BeforeJSON:  toJSON(p),
	})
	return c.Redirect(http.StatusSeeOther, "/projects?ok=deleted")
}

func (h *ProjectHandler) renderForm(c echo.Context, p *model.WorkProject, rules []model.ProjectScopeRule, isEdit bool, errs ...string) error {
	customers, _ := h.customerRepo.ListAll()
	parents, _, _ := h.customerRepo.ListCategories()
	contractTypes, _ := h.codeRepo.ActiveByGroup("project_contract_type")
	billingTypes, _ := h.codeRepo.ActiveByGroup("project_billing_type")
	errMsg := ""
	if len(errs) > 0 {
		errMsg = errs[0]
	}
	title := "사업 등록"
	if isEdit {
		title = "사업 수정"
	}
	type ruleJS struct {
		ID       string   `json:"id"`
		Parent   string   `json:"parent"`
		Products []string `json:"products"`
		Kinds    []string `json:"kinds"`
		Notes    string   `json:"notes"`
	}
	jsRules := make([]ruleJS, 0, len(rules))
	for _, rule := range rules {
		jsRules = append(jsRules, ruleJS{
			ID: rule.RuleID, Parent: rule.ParentCustomerID,
			Products: rule.ProductKeySlice(),
			Kinds:    splitCSVLocal(rule.WorkKinds),
			Notes:    rule.Notes,
		})
	}
	rulesJSON, _ := json.Marshal(jsRules)
	return c.Render(http.StatusOK, "project/form.html", map[string]interface{}{
		"Title": title, "Active": NavProjects,
		"Project": p, "Rules": rules, "RulesJSON": string(rulesJSON), "IsEdit": isEdit,
		"Customers": customers, "Parents": parents,
		"ContractTypes": contractTypes, "BillingTypes": billingTypes,
		"ProductKeys": []map[string]string{
			{"Key": model.ProductKeyKLAS, "Label": model.ProductKeyLabel(model.ProductKeyKLAS)},
			{"Key": model.ProductKeySejongKLAS, "Label": model.ProductKeyLabel(model.ProductKeySejongKLAS)},
			{"Key": model.ProductKeyAnrobotics, "Label": model.ProductKeyLabel(model.ProductKeyAnrobotics)},
		},
		"WorkKinds": []map[string]string{
			{"Key": model.ScopeWorkAS, "Label": model.ScopeWorkKindLabel(model.ScopeWorkAS)},
			{"Key": model.ScopeWorkMaintenance, "Label": model.ScopeWorkKindLabel(model.ScopeWorkMaintenance)},
			{"Key": model.ScopeWorkAdmin, "Label": model.ScopeWorkKindLabel(model.ScopeWorkAdmin)},
		},
		"FormError": errMsg,
		"CanWrite":  true,
	})
}

func splitCSVLocal(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (h *ProjectHandler) parseForm(c echo.Context) (*model.WorkProject, []model.ProjectScopeRule) {
	return ParseWorkProjectForm(c), parseScopeRules(c)
}

func parseScopeRules(c echo.Context) []model.ProjectScopeRule {
	_ = c.Request().ParseForm()
	parents := c.Request().Form["rule_parent_id"]
	products := c.Request().Form["rule_product_keys"]
	kinds := c.Request().Form["rule_work_kinds"]
	notes := c.Request().Form["rule_notes"]
	ids := c.Request().Form["rule_id"]
	n := len(parents)
	if len(products) > n {
		n = len(products)
	}
	if len(kinds) > n {
		n = len(kinds)
	}
	var out []model.ProjectScopeRule
	for i := 0; i < n; i++ {
		parent := ""
		prod := ""
		kind := ""
		note := ""
		rid := ""
		if i < len(parents) {
			parent = strings.TrimSpace(parents[i])
		}
		if i < len(products) {
			prod = strings.TrimSpace(products[i])
		}
		if i < len(kinds) {
			kind = strings.TrimSpace(kinds[i])
		}
		if i < len(notes) {
			note = strings.TrimSpace(notes[i])
		}
		if i < len(ids) {
			rid = strings.TrimSpace(ids[i])
		}
		if parent == "" && prod == "" && kind == "" && note == "" {
			continue
		}
		out = append(out, model.ProjectScopeRule{
			RuleID:           rid,
			ParentCustomerID: parent,
			ProductKeys:      prod,
			WorkKinds:        kind,
			Notes:            note,
		})
	}
	return out
}
