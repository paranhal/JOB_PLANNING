package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *StatsHandler) ShowWeeklyReport(c echo.Context) error {
	now := time.Now()
	week := parseWeekMonday(c.QueryParam("week"), now)
	orgs := weeklyVisibleOrgs(c, h)
	if len(orgs) == 0 {
		return echo.ErrForbidden
	}
	orgID, err := weeklySelectedOrg(c, orgs)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "팀을 찾을 수 없습니다")
	}
	needOrg := orgID == "" || orgID == repository.OrgAll
	v := weeklyReportView{
		Week: week, WeekInfo: repository.WeekInfoOf(week), NeedOrg: needOrg,
	}
	tabs, teamsN, cardsN := weeklyTabs(h, orgs, week)
	v.Tabs, v.VisibleTeams, v.VisibleCards = tabs, teamsN, cardsN
	canEdit := canWriteCompanyWeekly(c) && orgID != "" && orgID != repository.OrgAll && weeklyOrgWritable(c, orgID)
	if !needOrg {
		built, err := buildWeeklyReportView(h.repo, orgID, week, canEdit)
		if err != nil {
			return err
		}
		built.Tabs, built.VisibleTeams, built.VisibleCards = tabs, teamsN, cardsN
		built.CanEdit, built.CanConfirm = canEdit, canEdit
		v = built
		markCardsEditable(&v, c.QueryParam("edit") == "1" && v.CanEdit)
		if c.QueryParam("debug") == "1" && currentRole(c) == model.RoleVisionAdmin && h.repo != nil {
			v.WeeklyDebug = true
			ex, _ := h.repo.DiagnoseCompanyWeeklyUnassigned(v.Anchor, orgID)
			v.Draft.DebugExclusions = ex
		}
	}
	var projects []model.WorkProject
	var assignees []model.User
	if !needOrg && h.repo != nil {
		projects, _ = h.repo.ListWeeklyRowProjects(orgID)
		assignees = listAssignableForPicker(h.userRepo, c)
	}
	return c.Render(http.StatusOK, "stats/company_weekly.html", map[string]interface{}{
		"Title": "주간 업무 보고서", "Active": NavStatsWeekly, "View": v,
		"Week": week.Format("2006-01-02"), "PrevWeek": week.AddDate(0, 0, -7).Format("2006-01-02"),
		"NextWeek": week.AddDate(0, 0, 7).Format("2006-01-02"),
		"ThisWeek": parseWeekMonday("", now).Format("2006-01-02"),
		"CanExportAll": weeklyCanExportAll(c),
		"Edit":         c.QueryParam("edit") == "1" && v.CanEdit,
		"WeeklyProjects":  projects,
		"WeeklyAssignees": assignees,
		"FlashOK":         c.QueryParam("ok"),
		"FlashErr":        c.QueryParam("err"),
		"NeedRow":         c.QueryParam("need_row"),
		"NeedReagg":       c.QueryParam("reagg") == "1",
	})
}

func markCardsEditable(v *weeklyReportView, edit bool) {
	if v == nil || !edit {
		return
	}
	for i := range v.Cards {
		v.Cards[i].Edit = true
	}
}

func (h *StatsHandler) GenerateWeeklyReport(c echo.Context) error {
	if !canWriteCompanyWeekly(c) {
		return echo.ErrForbidden
	}
	week, orgID, err := weeklyWriteTarget(c)
	if err != nil {
		return err
	}
	anchor := companyWeeklyAnchor(week)
	draft, err := h.repo.BuildCompanyWeeklyDraftOrg(anchor, orgID)
	if err != nil {
		return err
	}
	saved, _ := h.repo.GetCompanyWeeklyReport(orgID, draft.Period.Anchor)
	doc := repository.ParseCompanyWeeklyDoc(saved.ContentJSON)
	if doc.Rows == nil {
		doc.Rows = map[string]repository.CompanyWeeklySavedRow{}
	}
	for _, row := range draft.Rows {
		cur := doc.Rows[row.RowKey]
		if len(cur.DoneLines) == 0 && strings.TrimSpace(cur.Prev) == "" {
			cur.DoneLines = repository.SplitWeeklyLines(row.PrevText)
			cur.Prev = row.PrevText
		}
		if len(cur.PlanLines) == 0 && strings.TrimSpace(cur.Plan) == "" {
			cur.PlanLines = repository.SplitWeeklyLines(row.PlanText)
			cur.Plan = row.PlanText
		}
		doc.Rows[row.RowKey] = cur
	}
	if err := h.repo.UpsertCompanyWeeklyReport(orgID, draft.Period.Anchor, draft.Period.SheetNameDefault,
		repository.EncodeCompanyWeeklyDoc(doc), ctxString(c, "user_name"), repository.CompanyWeeklyUpsertSave); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, weeklyReportURL(week, orgID, false))
}

func (h *StatsHandler) SaveWeeklyReport(c echo.Context) error {
	if !canWriteCompanyWeekly(c) {
		return echo.ErrForbidden
	}
	week, orgID, err := weeklyWriteTarget(c)
	if err != nil {
		return err
	}
	anchor := companyWeeklyAnchor(week)
	draft, err := h.repo.BuildCompanyWeeklyDraftOrg(anchor, orgID)
	if err != nil {
		return err
	}
	saved, _ := h.repo.GetCompanyWeeklyReport(orgID, draft.Period.Anchor)
	doc := repository.ParseCompanyWeeklyDoc(saved.ContentJSON)
	if doc.Rows == nil {
		doc.Rows = map[string]repository.CompanyWeeklySavedRow{}
	}
	reagg := c.FormValue("reaggregate") == "1"
	_ = c.Request().ParseForm()
	for _, row := range draft.Rows {
		cur := doc.Rows[row.RowKey]
		doneRaw := c.FormValue("done_" + row.RowKey)
		planRaw := c.FormValue("plan_" + row.RowKey)
		autoDone := repository.AutoToLines(row.AutoDone)
		if len(autoDone) == 0 {
			autoDone = repository.SplitWeeklyLines(row.PrevText)
		}
		autoPlan := repository.AutoToLines(row.AutoPlan)
		if len(autoPlan) == 0 {
			autoPlan = repository.SplitWeeklyLines(row.PlanText)
		}
		if reagg {
			cur.DoneLines = repository.MergeWeeklyLinesKeyed(autoDone, keepManual(cur.DoneLines), cur.HiddenDone)
			cur.PlanLines = repository.MergeWeeklyLinesKeyed(autoPlan, keepManual(cur.PlanLines), cur.HiddenPlan)
		} else {
			if doneRaw != "" || c.FormValue("touch_"+row.RowKey) == "1" {
				cur.DoneLines, cur.HiddenDone = applyEditedLines(doneRaw, autoDone, cur.HiddenDone, c.Request().Form["restore_"+row.RowKey])
			}
			if planRaw != "" || c.FormValue("touch_"+row.RowKey) == "1" {
				cur.PlanLines, cur.HiddenPlan = applyEditedLines(planRaw, autoPlan, cur.HiddenPlan, c.Request().Form["restore_"+row.RowKey])
			}
		}
		if changed := lineChangeNote(c, row.RowKey); changed != "" {
			cur.LineLog = append(cur.LineLog, repository.CompanyWeeklyLog{
				At: time.Now().Format("2006-01-02 15:04:05"), By: ctxString(c, "user_name"), Text: changed,
			})
		}
		cur.Prev = repository.JoinWeeklyLines(cur.DoneLines)
		cur.Plan = repository.JoinWeeklyLines(cur.PlanLines)
		need := c.FormValue("need_" + row.RowKey)
		dec := c.FormValue("dec_" + row.RowKey)
		cur.Customer = strings.TrimSpace(c.FormValue("cust_" + row.RowKey))
		cur.Assignee = strings.TrimSpace(c.FormValue("assignee_" + row.RowKey))
		if saved.Status == repository.CompanyWeeklyStatusConfirmed {
			if strings.TrimSpace(need) != strings.TrimSpace(nz(cur.SupportNeeds, cur.Help)) {
				cur.DecisionLog = append(cur.DecisionLog, repository.CompanyWeeklyLog{
					At: time.Now().Format("2006-01-02 15:04:05"), By: ctxString(c, "user_name"), Text: "지원 필요사항",
				})
			}
			if strings.TrimSpace(dec) != strings.TrimSpace(cur.Decision) {
				cur.DecisionLog = append(cur.DecisionLog, repository.CompanyWeeklyLog{
					At: time.Now().Format("2006-01-02 15:04:05"), By: ctxString(c, "user_name"), Text: "주요 의사결정",
				})
			}
		}
		cur.SupportNeeds, cur.Help = need, need
		cur.Decision = dec
		doc.Rows[row.RowKey] = cur
	}
	mode := repository.CompanyWeeklyUpsertSave
	contentChanged := repository.EncodeCompanyWeeklyDoc(doc) != saved.ContentJSON
	onlyExtra := false
	if saved.Status == repository.CompanyWeeklyStatusConfirmed && contentChanged {
		onlyExtra = extrasOnlyChanged(saved.ContentJSON, doc)
		if onlyExtra {
			mode = ""
		}
	}
	if mode == "" {
		mode = repository.CompanyWeeklyUpsertSave
		if onlyExtra {
			if err := h.repo.UpsertCompanyWeeklyReport(orgID, draft.Period.Anchor, nz(saved.SheetName, draft.Period.SheetNameDefault),
				repository.EncodeCompanyWeeklyDoc(doc), ctxString(c, "user_name"), repository.CompanyWeeklyUpsertConfirm); err != nil {
				return err
			}
			return c.Redirect(http.StatusSeeOther, weeklyReportURL(week, orgID, false))
		}
	}
	if err := h.repo.UpsertCompanyWeeklyReport(orgID, draft.Period.Anchor, nz(saved.SheetName, draft.Period.SheetNameDefault),
		repository.EncodeCompanyWeeklyDoc(doc), ctxString(c, "user_name"), repository.CompanyWeeklyUpsertSave); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, weeklyReportURL(week, orgID, false))
}

func (h *StatsHandler) ConfirmWeeklyReportUI(c echo.Context) error {
	if !canWriteCompanyWeekly(c) {
		return echo.ErrForbidden
	}
	week, orgID, err := weeklyWriteTarget(c)
	if err != nil {
		return err
	}
	anchor := companyWeeklyAnchor(week)
	p := repository.CompanyWeeklyPeriod(anchor)
	saved, _ := h.repo.GetCompanyWeeklyReport(orgID, p.Anchor)
	if err := h.repo.UpsertCompanyWeeklyReport(orgID, p.Anchor, nz(saved.SheetName, p.SheetNameDefault),
		saved.ContentJSON, ctxString(c, "user_name"), repository.CompanyWeeklyUpsertConfirm); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, weeklyReportURL(week, orgID, false))
}

func (h *StatsHandler) SaveWeeklyProgress(c echo.Context) error {
	if !canWriteCompanyWeekly(c) {
		return echo.ErrForbidden
	}
	week, orgID, err := weeklyWriteTarget(c)
	if err != nil {
		return err
	}
	if h.repo.WeeklyProgressMode(orgID) == repository.WeeklyProgressModeAuto {
		return echo.NewHTTPError(http.StatusForbidden, "자동 집계 팀은 진행율을 입력할 수 없습니다")
	}
	done, err := repository.ParseProgressNumber(c.FormValue("done"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	total, err := repository.ParseProgressNumber(c.FormValue("total"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	pct, err := repository.ParseProgressPct(c.FormValue("pct"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if done != nil && total != nil && c.FormValue("pct") == "" {
		v := repository.ProgressPct(*done, *total)
		pct = &v
	}
	cycle := strings.TrimSpace(c.FormValue("cycle"))
	if cycle == "" {
		cycle = h.repo.WeeklyProgressCycle(orgID)
	}
	switch cycle {
	case "월", "분기", "반기", "연":
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "주기가 올바르지 않습니다")
	}
	anchor := companyWeeklyAnchor(week)
	p := repository.CompanyWeeklyPeriod(anchor)
	saved, _ := h.repo.GetCompanyWeeklyReport(orgID, p.Anchor)
	doc := repository.ParseCompanyWeeklyDoc(saved.ContentJSON)
	now := time.Now().Format("2006-01-02 15:04:05")
	by := ctxString(c, "user_name")
	pr := &repository.CompanyWeeklyProgress{
		Done: done, Total: total, Pct: pct, Cycle: cycle, Memo: strings.TrimSpace(c.FormValue("memo")),
		UpdatedBy: by, UpdatedAt: now,
	}
	if doc.Progress != nil {
		pr.Log = append([]repository.CompanyWeeklyLog{}, doc.Progress.Log...)
	}
	pr.Log = append(pr.Log, repository.CompanyWeeklyLog{At: now, By: by, Text: "진행율"})
	doc.Progress = pr
	mode := repository.CompanyWeeklyUpsertSave
	if saved.ReportID != "" && saved.Status == repository.CompanyWeeklyStatusConfirmed {
		mode = repository.CompanyWeeklyUpsertConfirm
	}
	if err := h.repo.UpsertCompanyWeeklyReport(orgID, p.Anchor, nz(saved.SheetName, p.SheetNameDefault),
		repository.EncodeCompanyWeeklyDoc(doc), by, mode); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, weeklyReportURL(week, orgID, c.FormValue("edit") == "1"))
}

func (h *StatsHandler) ExportWeeklyReportSheet(c echo.Context) error {
	now := time.Now()
	week := parseWeekMonday(c.QueryParam("week"), now)
	orgs := weeklyVisibleOrgs(c, h)
	if len(orgs) == 0 {
		return echo.ErrForbidden
	}
	scope := strings.TrimSpace(c.QueryParam("scope"))
	var views []weeklyReportView
	if scope == "all" {
		if !weeklyCanExportAll(c) {
			return echo.ErrForbidden
		}
		for _, o := range orgs {
			v, err := buildWeeklyReportView(h.repo, o.OrgID, week, false)
			if err != nil {
				return err
			}
			views = append(views, v)
		}
	} else {
		orgID, err := weeklySelectedOrg(c, orgs)
		if err != nil || orgID == "" || orgID == repository.OrgAll {
			return echo.NewHTTPError(http.StatusForbidden, "조직을 선택하세요")
		}
		v, err := buildWeeklyReportView(h.repo, orgID, week, false)
		if err != nil {
			return err
		}
		views = []weeklyReportView{v}
	}
	data, err := buildCompanyWeeklyTeamWorkbook(views)
	if err != nil {
		return err
	}
	ascii, utf8 := weeklyReportExportNames(views[0], scope == "all")
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionDownload,
		TargetTable: "company_weekly_report",
		TargetID:    utf8,
		SubjectType: "report",
		SubjectID:   week.Format("2006-01-02"),
		Reason:      fmt.Sprintf("주간업무보고 엑셀 team=%s week=%s", views[0].OrgShort, week.Format("2006-01-02")),
		Result:      auditlog.ResultOK,
	})
	return writeExcelFile(c, data, ascii, utf8)
}

func weeklyVisibleOrgs(c echo.Context, h *StatsHandler) []model.Org {
	role := currentRole(c)
	home := strings.TrimSpace(ctxString(c, "org_id"))
	var all []model.Org
	if h != nil && h.repo != nil {
		all, _ = repository.NewOrgRepo(h.repo.DB()).ListActive()
	}
	if role == model.RoleVisionAdmin || role == model.RoleCLevel {
		return all
	}
	var out []model.Org
	for _, o := range all {
		if o.OrgID == home {
			out = append(out, o)
		}
	}
	return out
}

func weeklySelectedOrg(c echo.Context, orgs []model.Org) (string, error) {
	want := strings.TrimSpace(c.QueryParam("org"))
	if want == "" {
		want = strings.TrimSpace(c.FormValue("org"))
	}
	if currentRole(c) == model.RoleVisionAdmin && (currentOrg(c) == repository.OrgAll) && want == "" {
		return "", nil
	}
	if want == "" {
		if len(orgs) == 1 {
			return orgs[0].OrgID, nil
		}
		cur := currentOrg(c)
		for _, o := range orgs {
			if o.OrgID == cur {
				return cur, nil
			}
		}
		if len(orgs) > 0 {
			return orgs[0].OrgID, nil
		}
		return "", fmt.Errorf("org")
	}
	for _, o := range orgs {
		if o.OrgID == want {
			return want, nil
		}
	}
	return "", fmt.Errorf("org")
}

func weeklyOrgWritable(c echo.Context, orgID string) bool {
	if currentRole(c) == model.RoleVisionAdmin {
		return orgID != "" && orgID != repository.OrgAll
	}
	return strings.TrimSpace(ctxString(c, "org_id")) == orgID
}

func weeklyCanExportAll(c echo.Context) bool {
	r := currentRole(c)
	return r == model.RoleVisionAdmin || r == model.RoleCLevel
}

func weeklyWriteTarget(c echo.Context) (time.Time, string, error) {
	week := parseWeekMonday(nz(c.FormValue("week"), c.QueryParam("week")), time.Now())
	orgID := strings.TrimSpace(nz(c.FormValue("org"), c.QueryParam("org")))
	if orgID == "" {
		id, err := resolveInsertOrg(c)
		if err != nil {
			return week, "", echo.ErrForbidden
		}
		orgID = id
	}
	if !weeklyOrgWritable(c, orgID) {
		return week, "", echo.ErrForbidden
	}
	return week, orgID, nil
}

func weeklyReportURL(week time.Time, orgID string, edit bool) string {
	u := "/stats/company-weekly?week=" + week.Format("2006-01-02") + "&org=" + orgID
	if edit {
		u += "&edit=1"
	}
	return u
}

func weeklyTabs(h *StatsHandler, orgs []model.Org, week time.Time) ([]weeklyReportTab, int, int) {
	var tabs []weeklyReportTab
	teams, cards := 0, 0
	for _, o := range orgs {
		v, err := buildWeeklyReportView(h.repo, o.OrgID, week, false)
		n := 0
		if err == nil {
			for _, c := range v.Cards {
				if c.HasContent {
					n++
				}
			}
		}
		tabs = append(tabs, weeklyReportTab{OrgID: o.OrgID, Name: o.NavLabel(), Count: n})
		if n > 0 {
			teams++
			cards += n
		}
	}
	return tabs, teams, cards
}

func keepManual(in []repository.CompanyWeeklyLine) []repository.CompanyWeeklyLine {
	var out []repository.CompanyWeeklyLine
	for _, l := range in {
		if l.Manual {
			out = append(out, l)
		}
	}
	return out
}

func linesFromEdit(raw, draft string) []repository.CompanyWeeklyLine {
	out, _ := applyEditedLines(raw, repository.SplitWeeklyLines(draft), nil, nil)
	return out
}

func applyEditedLines(raw string, auto []repository.CompanyWeeklyLine, oldHidden, restore []string) ([]repository.CompanyWeeklyLine, []string) {
	restoreSet := map[string]bool{}
	for _, k := range restore {
		if k = strings.TrimSpace(k); k != "" {
			restoreSet[k] = true
		}
	}
	var texts []string
	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(line)
		t = strings.TrimPrefix(t, "·")
		t = strings.TrimPrefix(t, "*")
		t = strings.TrimPrefix(t, "+")
		t = strings.TrimSpace(t)
		if t != "" {
			texts = append(texts, t)
		}
	}
	used := make([]bool, len(auto))
	var out []repository.CompanyWeeklyLine
	var leftover []string
	for _, t := range texts {
		hit := -1
		for i, a := range auto {
			if used[i] {
				continue
			}
			if strings.TrimSpace(a.Text) == t {
				hit = i
				break
			}
		}
		if hit >= 0 {
			used[hit] = true
			out = append(out, auto[hit])
			continue
		}
		leftover = append(leftover, t)
	}
	var leftoverAuto []repository.CompanyWeeklyLine
	for i, a := range auto {
		if !used[i] {
			leftoverAuto = append(leftoverAuto, a)
		}
	}
	if len(leftover) == 1 && len(leftoverAuto) == 1 {
		a := leftoverAuto[0]
		out = append(out, repository.CompanyWeeklyLine{Text: leftover[0], Manual: true, Key: a.Key})
		leftover, leftoverAuto = nil, nil
	} else {
		for _, t := range leftover {
			out = append(out, repository.CompanyWeeklyLine{Text: t, Manual: true})
		}
	}
	hide := map[string]bool{}
	for _, k := range oldHidden {
		k = strings.TrimSpace(k)
		if k != "" && !restoreSet[k] {
			hide[k] = true
		}
	}
	for _, a := range leftoverAuto {
		if a.Key != "" && !restoreSet[a.Key] {
			hide[a.Key] = true
		}
	}
	var hidden []string
	for k := range hide {
		hidden = append(hidden, k)
	}
	sort.Strings(hidden)
	return out, hidden
}

func lineChangeNote(c echo.Context, rowKey string) string {
	if c.FormValue("reaggregate") == "1" {
		return "다시 집계"
	}
	if c.FormValue("touch_"+rowKey) == "1" {
		return "줄 수정"
	}
	return ""
}

func extrasOnlyChanged(before string, doc repository.CompanyWeeklyDoc) bool {
	old := repository.ParseCompanyWeeklyDoc(before)
	if len(old.Rows) != len(doc.Rows) {
		return false
	}
	for k, n := range doc.Rows {
		o := old.Rows[k]
		if repository.JoinWeeklyLines(n.DoneLines) != nz(repository.JoinWeeklyLines(o.DoneLines), o.Prev) {
			return false
		}
		if repository.JoinWeeklyLines(n.PlanLines) != nz(repository.JoinWeeklyLines(o.PlanLines), o.Plan) {
			return false
		}
	}
	return true
}

func nz(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
