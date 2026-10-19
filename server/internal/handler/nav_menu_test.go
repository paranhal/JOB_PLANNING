package handler

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

func visionMenuCtx() MenuCtx {
	return MenuCtx{
		Role: model.RoleVisionAdmin, LoginRole: model.RoleVisionAdmin,
		ShowProjects: true, NavMail: true, CanViewOrgs: true,
		Perms: model.DefaultPermissions(model.RoleVisionAdmin),
	}
}

func TestNavMenuHrefsUniqueAndParentsHaveNoHref(t *testing.T) {
	if dups := navDuplicateHrefs(); len(dups) > 0 {
		t.Fatalf("중복 href: %v", dups)
	}
	var walk func([]navDefItem)
	walk = func(items []navDefItem) {
		for _, it := range items {
			if len(it.Children) > 0 && strings.TrimSpace(it.Href) != "" {
				t.Fatalf("부모 %s 에 href=%s", it.ID, it.Href)
			}
			if len(it.Children) == 0 && strings.TrimSpace(it.Href) == "" {
				t.Fatalf("잎 %s 에 href 없음", it.ID)
			}
			walk(it.Children)
		}
	}
	for _, g := range navDefinition() {
		walk(g.Items)
	}
}

func TestNavRoleVisibilityTable(t *testing.T) {
	type row struct {
		name string
		ctx  MenuCtx
		want []string
		hide []string
	}
	rows := []row{
		{
			name: "비전",
			ctx:  visionMenuCtx(),
			want: []string{"/", "/work", "/meeting/weekly", "/approvals/inbox", "/leaves", "/as", "/sales/dashboard", "/quotes", "/projects", "/stats", "/admin/users", "/admin/system", "/admin/test-stats"},
			hide: []string{"/account", "/admin-work", "/plan/unplanned", "/work-status"},
		},
		{
			name: "조직관리자",
			ctx: MenuCtx{Role: model.RoleOrgAdmin, LoginRole: model.RoleOrgAdmin, ShowProjects: true,
				Perms: model.DefaultPermissions(model.RoleOrgAdmin)},
			want: []string{"/", "/work", "/as", "/admin/users", "/admin/holidays"},
			hide: []string{"/admin/system", "/admin/data", "/admin/permissions"},
		},
		{
			name: "영업",
			ctx: MenuCtx{Role: model.RoleSales, LoginRole: model.RoleSales, ShowProjects: true,
				Perms: model.DefaultPermissions(model.RoleSales)},
			want: []string{"/", "/work", "/as", "/sales", "/items"},
			hide: []string{"/admin/users", "/admin/system"},
		},
		{
			name: "기술",
			ctx: MenuCtx{Role: model.RoleTech, LoginRole: model.RoleTech, ShowProjects: true,
				Perms: model.DefaultPermissions(model.RoleTech)},
			want: []string{"/", "/work", "/as", "/maintenance"},
			hide: []string{"/admin/users", "/maintenance/sites"},
		},
		{
			name: "읽기전용",
			ctx:  MenuCtx{Role: model.RoleTech, LoginRole: model.RoleTech, IsReadOnly: true, ShowProjects: true},
			want: []string{"/", "/work", "/as"},
			hide: []string{"/admin/users"},
		},
		{
			name: "view-as 기술",
			ctx: MenuCtx{Role: model.RoleTech, LoginRole: model.RoleVisionAdmin, ShowProjects: true,
				Perms: model.DefaultPermissions(model.RoleTech)},
			want: []string{"/", "/work"},
			hide: []string{"/admin/users", "/admin/system"},
		},
	}
	for _, r := range rows {
		res := FilterNav(r.ctx, NavDashboard, NavBadges{})
		got := strings.Join(collectNavHrefs(res.Groups), "\n")
		for _, h := range r.want {
			if !strings.Contains(got, h) {
				t.Errorf("%s: 보여야 할 %s 없음", r.name, h)
			}
		}
		for _, h := range r.hide {
			if containsHref(collectNavHrefs(res.Groups), h) {
				t.Errorf("%s: 숨겨야 할 %s 가 있다", r.name, h)
			}
		}
	}
}

func containsHref(list []string, h string) bool {
	for _, x := range list {
		if x == h {
			return true
		}
	}
	return false
}

func TestNavActiveExactlyOnePerMenuKey(t *testing.T) {
	ctx := visionMenuCtx()
	var keys []string
	var walk func([]navDefItem)
	walk = func(items []navDefItem) {
		for _, it := range items {
			if len(it.Active) > 0 && it.Href != "" {
				keys = append(keys, it.Active[0])
			}
			walk(it.Children)
		}
	}
	for _, g := range navDefinition() {
		walk(g.Items)
	}
	for _, k := range keys {
		res := FilterNav(ctx, k, NavBadges{})
		n := countNavActive(res.Groups)
		if n != 1 {
			t.Errorf("active=%s 개수=%d", k, n)
		}
	}
	none := FilterNav(ctx, "not_a_nav_key", NavBadges{})
	if countNavActive(none.Groups) != 0 {
		t.Fatal("없는 키인데 활성이 있다")
	}
}

func TestNavAutoExpandCurrentMoreAndParent(t *testing.T) {
	ctx := visionMenuCtx()
	rel := FilterNav(ctx, NavRelations, NavBadges{})
	if rel.CurGroup != "customer" {
		t.Fatalf("수행관계 그룹=%s", rel.CurGroup)
	}
	if !containsHref(rel.CurMore, "customer") {
		t.Fatal("수행관계는 더 보기 자동 펼침이어야 한다")
	}
	work := FilterNav(ctx, NavWorkAll, NavBadges{})
	if !containsHref(work.CurParents, "work_review") {
		t.Fatal("전체 업무는 업무 확인 자동 펼침")
	}
	if work.CurGroup != "work" {
		t.Fatalf("전체 업무 그룹=%s", work.CurGroup)
	}
	meet := FilterNav(ctx, NavMeetingWeekly, NavBadges{})
	if !containsHref(meet.CurParents, "work_meeting") {
		t.Fatal("주간 회의는 업무 회의 자동 펼침")
	}
}

func TestNavShowProjectsFlagOnlyChangesProjectGroup(t *testing.T) {
	on := visionMenuCtx()
	off := on
	off.ShowProjects = false
	a := FilterNav(on, NavDashboard, NavBadges{})
	b := FilterNav(off, NavDashboard, NavBadges{})
	if !containsHref(collectNavHrefs(a.Groups), "/projects") {
		t.Fatal("플래그 on 에 사업관리가 없다")
	}
	if containsHref(collectNavHrefs(b.Groups), "/projects") {
		t.Fatal("플래그 off 에 사업관리가 있다")
	}
	ha := collectNavHrefs(a.Groups)
	hb := collectNavHrefs(b.Groups)
	onlyA := 0
	for _, h := range ha {
		if !containsHref(hb, h) {
			onlyA++
			if !strings.HasPrefix(h, "/projects") {
				t.Errorf("플래그로 %s 가 사라지면 안 된다", h)
			}
		}
	}
	if onlyA == 0 {
		t.Fatal("플래그 on/off 차이가 없다")
	}
}

func TestOldSidebarHrefsClassified(t *testing.T) {
	missing := oldSidebarUnreachable(visionMenuCtx())
	if len(missing) > 0 {
		t.Fatalf("도달 분류 누락: %v", missing)
	}
}

func TestNavAssignBadgeOnWorkReview(t *testing.T) {
	ctx := visionMenuCtx()
	res := FilterNav(ctx, NavDashboard, NavBadges{Assign: 3, Approvals: 2, Leaves: 1})
	var review, today, appr NavItemView
	for _, g := range res.Groups {
		if g.ID != "work" {
			continue
		}
		for _, it := range g.Items {
			if it.ID == "work_review" {
				review = it
				for _, ch := range it.Children {
					if ch.ID == "work_today" {
						today = ch
					}
				}
			}
			if it.ID == "approval" {
				appr = it
			}
		}
	}
	if review.Badge != 3 || today.Badge != 3 {
		t.Fatalf("미배정 배지 review=%d today=%d", review.Badge, today.Badge)
	}
	if appr.Badge != 3 {
		t.Fatalf("결재 합산 배지=%d", appr.Badge)
	}
}
