package handler

import (
	"strings"

	"customer-support/internal/model"
)

func (c MenuCtx) isCLevel() bool { return c.Role == model.RoleCLevel }

func (c MenuCtx) visExec() bool { return c.isCLevel() || c.isVision() }

func execNavDefinition(orgs []model.Org) []navDefGroup {
	show := func(MenuCtx) bool { return true }
	biz, support := splitExecOrgs(orgs)
	orgItems := make([]navDefItem, 0, len(biz)+len(support))
	for _, o := range biz {
		orgItems = append(orgItems, navDefItem{
			ID: "exec_org_" + o.OrgID, Label: o.NavLabel(),
			Href: "/?view_org_id=" + o.OrgID, Active: []string{NavDashboard}, Show: show,
		})
	}
	for _, o := range support {
		orgItems = append(orgItems, navDefItem{
			ID: "exec_org_" + o.OrgID, Label: o.NavLabel(),
			Href: "/exec/org/" + o.OrgID, Active: []string{NavExecOrg}, Show: show,
		})
	}
	return []navDefGroup{
		{
			ID: "exec_ws", Label: "경영 워크스페이스", Show: show, Default: true,
			Items: []navDefItem{
				{ID: "exec_home", Label: "경영 통합", Href: "/exec", Active: []string{NavExec}, Show: show},
				{ID: "exec_contract", Label: "사업·계약", Show: show, Children: []navDefItem{
					{ID: "exec_proj_dash", Label: "사업 대시보드", Href: "/projects/dashboard", Active: []string{NavProjectsDashboard}, Show: show},
					{ID: "exec_projects", Label: "사업 목록", Href: "/projects", Active: []string{NavProjects}, Show: show},
					{ID: "exec_quotes", Label: "견적·수주·계약", Href: "/quotes", Active: []string{NavQuotes, NavOrders, NavContracts, NavDocsHub}, Show: show},
					{ID: "exec_registry", Label: "사업 등록 현황", Href: "/projects/registry", Active: []string{NavProjectsRegistry}, Show: show},
				}},
				{ID: "exec_pipe", Label: "영업 파이프라인", Show: show, Children: []navDefItem{
					{ID: "exec_sales_dash", Label: "영업 대시보드", Href: "/sales/dashboard", Active: []string{NavSalesDashboard}, Show: show},
					{ID: "exec_sales_pipe", Label: "파이프라인", Href: "/sales?view=pipeline", Active: []string{NavSalesPipeline, NavSales}, Show: show},
					{ID: "exec_sales_act", Label: "영업 활동", Href: "/sales/activities", Active: []string{NavSalesActivities}, Show: show},
				}},
				{ID: "exec_work", Label: "업무 관리", Show: show, Children: []navDefItem{
					{ID: "exec_work_org", Label: "조직별 업무", Href: "/exec/work", Active: []string{NavExecWork}, Show: show},
					{ID: "exec_work_all", Label: "전체 업무", Href: "/work/all", Active: []string{NavWorkAll}, Show: show},
					{ID: "exec_work_groups", Label: "업무 그룹", Href: "/work-groups", Active: []string{NavWorkGroups}, Show: show},
					{ID: "exec_weekly", Label: "주간 회의 (팀 자료)", Href: "/meeting/weekly", Active: []string{NavMeetingWeekly}, Show: show},
				}},
				{ID: "exec_as", Label: "AS·점검", Show: show, Children: []navDefItem{
					{ID: "exec_as_list", Label: "AS 현황", Href: "/as", Active: []string{NavAS}, Show: show},
					{ID: "exec_mnt", Label: "정기점검", Href: "/maintenance", Active: []string{NavMaintenance}, Show: show},
					{ID: "exec_customers", Label: "고객현황", Href: "/customers", Active: []string{NavCustomers}, Show: show},
					{ID: "exec_assets", Label: "설치자산", Href: "/assets", Active: []string{NavAssets}, Show: show},
				}},
				{ID: "exec_appr", Label: "결재·휴가", Show: show, Children: []navDefItem{
					{ID: "exec_inbox", Label: "결재 현황", Href: "/approvals/inbox", Active: []string{NavApprovalsInbox, NavApprovalsHub}, Show: show},
					{ID: "exec_leaves", Label: "휴가 현황", Href: "/leaves", Active: []string{NavLeaves}, Show: show},
				}},
				{ID: "exec_stats", Label: "통계·보고서", Show: show, Children: []navDefItem{
					{ID: "exec_stats", Label: "통계", Href: "/stats", Active: []string{NavStats}, Show: show},
					{ID: "exec_ms", Label: "마일스톤", Href: "/stats/milestones", Active: []string{NavStatsMilestones}, Show: show},
					{ID: "exec_reports", Label: "보고서", Href: "/stats/reports", Active: []string{NavStatsReports, NavStatsDetail}, Show: show},
				}},
			},
		},
		{
			ID: "exec_orgs", Label: "조직 대시보드", Show: func(MenuCtx) bool { return len(orgItems) > 0 },
			Items: orgItems,
		},
		{
			ID: "exec_help", Label: "설정 및 도움말", Show: show,
			Items: []navDefItem{
				{ID: "exec_account", Label: "설정 및 도움말", Href: "/account", Active: []string{NavAccount}, Show: show},
			},
		},
	}
}

func splitExecOrgs(orgs []model.Org) (biz, support []model.Org) {
	for _, o := range orgs {
		if strings.TrimSpace(o.OrgID) == "" {
			continue
		}
		if model.NormalizeOrgKind(o.Kind) == model.OrgKindSupport {
			support = append(support, o)
			continue
		}
		biz = append(biz, o)
	}
	return biz, support
}

func prependVisionExecHome(defs []navDefGroup) []navDefGroup {
	home := navDefGroup{
		ID: "exec_ws", Label: "경영 워크스페이스",
		Items: []navDefItem{
			{ID: "exec_home", Label: "경영 통합", Href: "/exec", Active: []string{NavExec}},
		},
	}
	out := make([]navDefGroup, 0, len(defs)+1)
	out = append(out, home)
	return append(out, defs...)
}

func navDefsFor(ctx MenuCtx) []navDefGroup {
	if ctx.isCLevel() {
		return execNavDefinition(ctx.ExecOrgs)
	}
	defs := navDefinition()
	if ctx.isVision() {
		return prependVisionExecHome(defs)
	}
	return defs
}

func cLevelHiddenPath(path string) bool {
	p := strings.TrimSpace(path)
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	if p == "/admin/orgs/switch" {
		return false
	}
	switch {
	case p == "/work",
		p == "/meeting",
		p == "/meeting/weekly/me",
		strings.HasPrefix(p, "/meeting/weekly/me/"),
		strings.HasPrefix(p, "/admin"),
		strings.HasPrefix(p, "/workboard"),
		strings.HasPrefix(p, "/as/knowledge"),
		strings.HasPrefix(p, "/contacts"),
		strings.HasPrefix(p, "/contact-history"),
		strings.HasPrefix(p, "/spaces"),
		strings.HasPrefix(p, "/relations"),
		strings.HasPrefix(p, "/items"),
		strings.HasPrefix(p, "/maintenance/sites"),
		strings.HasPrefix(p, "/codes"),
		strings.HasPrefix(p, "/sales/new"),
		strings.HasPrefix(p, "/sales/groups"),
		strings.HasPrefix(p, "/projects/new"),
		strings.HasPrefix(p, "/projects/docs"),
		strings.HasPrefix(p, "/projects/diary"),
		strings.HasPrefix(p, "/quotes/new"),
		strings.HasPrefix(p, "/analysis"),
		strings.HasPrefix(p, "/admin-work"),
		strings.HasPrefix(p, "/plan"),
		strings.HasPrefix(p, "/work-status"),
		strings.HasPrefix(p, "/leaves/new"):
		return true
	}
	return false
}

func cLevelWriteExempt(path string) bool {
	p := strings.TrimSpace(path)
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	if strings.HasPrefix(p, "/account") {
		return true
	}
	if p == "/logout" || p == "/admin/orgs/switch" {
		return true
	}
	if strings.HasPrefix(p, "/approvals/") {
		return true
	}
	if strings.HasPrefix(p, "/view-as") {
		return true
	}
	return false
}

func sampleExecOrgs() []model.Org {
	return []model.Org{
		{OrgID: "O01", OrgName: "도서관사업팀", ShortName: "도서관", Kind: model.OrgKindBusiness, IsActive: true},
		{OrgID: "O02", OrgName: "미디어사업팀", ShortName: "미디어", Kind: model.OrgKindBusiness, IsActive: true},
		{OrgID: "O03", OrgName: "SI사업팀", ShortName: "SI", Kind: model.OrgKindBusiness, IsActive: true},
		{OrgID: "O04", OrgName: "경영지원", Kind: model.OrgKindSupport, IsActive: true},
		{OrgID: "O05", OrgName: "경영기획", Kind: model.OrgKindSupport, IsActive: true},
	}
}
