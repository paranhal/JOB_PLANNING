package handler

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_67_ParentPageMap(t *testing.T) {
	cases := []struct{ path, want string }{
		{"/", ""},
		{"/workboard/board", "/"},
		{"/meeting/board", "/"},
		{"/workboard/register", "/workboard/board"},
		{"/workboard/tasks/WT-1", "/workboard/board"},
		{"/workboard/tasks/WT-1/action", "/workboard/board"},
		{"/as", "/"},
		{"/as/AS-1", "/as"},
		{"/as/AS-1/action", "/workboard/board"},
		{"/maintenance/visits/V1/action", "/workboard/board"},
		{"/maintenance/sites", "/maintenance"},
		{"/quotes/Q-1", "/quotes"},
		{"/orders/O-1", "/orders"},
		{"/contracts/C-1", "/contracts"},
		{"/sales/S-1", "/sales"},
		{"/sales/S-1/edit", "/sales"},
		{"/projects/P-1", "/projects"},
		{"/projects/P-1/sites", "/projects"},
		{"/admin/orgs", "/admin/users"},
		{"/admin/permissions", "/admin/users"},
		{"/admin/users/deleted", "/admin/users"},
		{"/spaces/1/edit", "/spaces"},
		{"/stats/list", "/stats"},
		{"/product-groups/import", "/product-groups"},
		{"/work-item-templates", "/"},
		{"/work-item-templates/x", "/work-item-templates"},
		{"/assets/import", "/assets"},
		{"/assets/A1/sw", "/assets/A1"},
		{"/as/AS-1/report", "/as/AS-1/action"},
		{"/as/work/W1/action", "/as"},
		{"/items/I1/edit", "/items"},
	}
	for _, tc := range cases {
		if got := ParentPage(tc.path); got != tc.want {
			t.Errorf("%s parent=%q want %q", tc.path, got, tc.want)
		}
	}
	if len(pageParentTable()) < 40 {
		t.Fatal("부모 표가 비었다")
	}
}

func Test95_67_SafePageBackAndResolveOrder(t *testing.T) {
	if SafePageBack("https://evil.example/x", "/ok") != "/ok" {
		t.Fatal("외부 URL")
	}
	if SafePageBack("//evil", "/ok") != "/ok" {
		t.Fatal("프로토콜 상대")
	}
	if SafePageBack("/login", "/ok") != "/ok" {
		t.Fatal("로그인")
	}
	if SafePageBack("/admin-work", "/") != "/admin-work" {
		t.Fatal("내부 경로")
	}
	if SafePageBack("/workboard/board?view=list", "/") != "/workboard/board?view=list" {
		t.Fatal("쿼리 유지")
	}

	href, kind := ResolvePageBack("/workboard/tasks/WT-1", "/admin-work", "http://localhost/as")
	if href != "/admin-work" || kind != pageBackKindBack {
		t.Fatalf("back 우선순위 href=%s kind=%s", href, kind)
	}
	href, kind = ResolvePageBack("/workboard/tasks/WT-1", "https://evil", "")
	if href != "/workboard/board" || kind != pageBackKindParent {
		t.Fatalf("불안전 back 이면 부모 href=%s kind=%s", href, kind)
	}
	href, kind = ResolvePageBack("/quotes/Q-1", "", "http://localhost/quotes")
	if href != "/quotes" || kind != pageBackKindBack {
		t.Fatalf("referer href=%s kind=%s", href, kind)
	}
	href, kind = ResolvePageBack("/", "", "")
	if href != "/" || kind != pageBackKindHistory {
		t.Fatalf("대시보드 href=%s kind=%s", href, kind)
	}
}

func Test95_67_ParentTableHasSidebarRoots(t *testing.T) {
	for _, p := range []string{"/as", "/maintenance", "/sales", "/quotes", "/workboard/board", "/stats"} {
		if ParentPage(p) != "/" {
			t.Errorf("%s parent=%s want /", p, ParentPage(p))
		}
	}
	if ParentPage("/orders") != "/quotes" {
		t.Fatal("수주 부모는 견적 허브")
	}
}

func Test95_67_LayoutPrevButtonOnBoard(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "pb67.db"))
	body := doGetWith(t, e, "/workboard/board", jwtCookieUser(t, me)).Body.String()
	if strings.Count(body, `id="page-back"`) != 1 {
		t.Fatal("공통 이전 버튼이 한 개가 아니다")
	}
	if !strings.Contains(body, "‹ 이전") {
		t.Fatal("‹ 이전 문구 없음")
	}
	if !strings.Contains(body, `href="/"`) {
		t.Fatal("일일 업무 부모는 대시보드")
	}
}

func Test95_67_TaskShowOnePrevButton(t *testing.T) {
	e, repo, _ := mountWorkboard95_30(t)
	tk := create30Task(t, repo, "이전통합67", model.WBWorkAdmin, "2026-10-24", "2026-10-24", model.OrgIDLibrary)
	body := doGet(t, e, "/workboard/tasks/"+tk.TaskID).Body.String()
	if strings.Count(body, "‹ 이전") != 1 {
		t.Fatalf("이전 버튼 %d개", strings.Count(body, "‹ 이전"))
	}
	if strings.Contains(body, "← 업무 관리") || strings.Contains(body, "‹ 작업 목록") {
		t.Fatal("개별 뒤로가기가 남았다")
	}
}
