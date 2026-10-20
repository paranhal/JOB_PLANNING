package handler

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

func cLevelMenuCtx(orgs []model.Org) MenuCtx {
	return MenuCtx{
		Role: model.RoleCLevel, LoginRole: model.RoleCLevel, IsReadOnly: true,
		ShowProjects: true, ExecOrgs: orgs,
		Perms: model.DefaultPermissions(model.RoleCLevel),
	}
}

func TestCLevelMenuHrefsOrder(t *testing.T) {
	orgs := sampleExecOrgs()
	res := FilterNav(cLevelMenuCtx(orgs), NavExec, NavBadges{})
	got := collectNavHrefs(res.Groups)
	want := []string{
		"/exec",
		"/projects/dashboard", "/projects", "/quotes", "/projects/registry",
		"/sales/dashboard", "/sales?view=pipeline", "/sales/activities",
		"/exec/work", "/work/all", "/meeting/weekly",
		"/as", "/maintenance", "/customers", "/assets",
		"/approvals/inbox", "/leaves",
		"/stats", "/stats/milestones", "/stats/reports",
		"/?view_org_id=O01", "/?view_org_id=O02", "/?view_org_id=O03",
		"/exec/org/O04", "/exec/org/O05",
		"/account",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("C-Level 메뉴 순서\n got=%v\nwant=%v", got, want)
	}
	hide := []string{
		"/admin/users", "/workboard/register", "/work", "/meeting",
		"/meeting/weekly/me", "/as/knowledge", "/items", "/contacts",
		"/maintenance/sites", "/codes",
	}
	for _, h := range hide {
		if containsHref(got, h) {
			t.Errorf("숨겨야 할 %s 가 있다", h)
		}
	}
}

func TestCLevelMenuAddsBusinessOrg(t *testing.T) {
	orgs := sampleExecOrgs()
	orgs = append(orgs, model.Org{OrgID: "O09", OrgName: "신규사업", Kind: model.OrgKindBusiness, IsActive: true})
	res := FilterNav(cLevelMenuCtx(orgs), NavExec, NavBadges{})
	if !containsHref(collectNavHrefs(res.Groups), "/?view_org_id=O09") {
		t.Fatal("추가 business 조직이 메뉴에 없다")
	}
}

func TestOrgObserverMenuHasNoExec(t *testing.T) {
	rows := []MenuCtx{
		{Role: model.RoleOrgAdmin, LoginRole: model.RoleOrgAdmin, ShowProjects: true, Perms: model.DefaultPermissions(model.RoleOrgAdmin)},
		{Role: model.RoleSales, LoginRole: model.RoleSales, ShowProjects: true, Perms: model.DefaultPermissions(model.RoleSales)},
		{Role: model.RoleTech, LoginRole: model.RoleTech, IsReadOnly: true, ShowProjects: true},
	}
	for _, ctx := range rows {
		got := collectNavHrefs(FilterNav(ctx, NavDashboard, NavBadges{}).Groups)
		if containsHref(got, "/exec") || containsHref(got, "/exec/work") {
			t.Errorf("%s 메뉴에 /exec 가 있다", ctx.Role)
		}
	}
}

func TestVisionMenuKeepsExistingAndAddsExecHome(t *testing.T) {
	res := FilterNav(visionMenuCtx(), NavDashboard, NavBadges{})
	got := collectNavHrefs(res.Groups)
	if got[0] != "/exec" {
		t.Fatalf("비전 첫 메뉴=%s", got[0])
	}
	for _, h := range []string{"/", "/work", "/admin/users", "/admin/system"} {
		if !containsHref(got, h) {
			t.Errorf("비전 기존 항목 누락 %s", h)
		}
	}
}

func TestExecNavParentsHaveNoHref(t *testing.T) {
	var walk func([]navDefItem)
	walk = func(items []navDefItem) {
		for _, it := range items {
			if len(it.Children) > 0 && strings.TrimSpace(it.Href) != "" {
				t.Fatalf("부모 %s 에 href=%s", it.ID, it.Href)
			}
			walk(it.Children)
		}
	}
	for _, g := range execNavDefinition(sampleExecOrgs()) {
		walk(g.Items)
	}
}

func TestCLevelHiddenPathList(t *testing.T) {
	for _, p := range []string{"/admin/users", "/workboard/register", "/work", "/meeting", "/meeting/weekly/me", "/as/knowledge"} {
		if !cLevelHiddenPath(p) {
			t.Errorf("%s 가 숨김이 아니다", p)
		}
	}
	for _, p := range []string{"/exec", "/exec/work", "/work/all", "/meeting/weekly", "/as", "/account"} {
		if cLevelHiddenPath(p) {
			t.Errorf("%s 가 숨김으로 막혔다", p)
		}
	}
}
