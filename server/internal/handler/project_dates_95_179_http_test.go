package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 하부 업무에 배지·예정·마감을 주고, 마감 연장은 사유가 있어야 하며, 「변경내역」에 이력이 보인다. 95-179A.
func Test95_179A_PackageDatesBadgeExtendAndChanges(t *testing.T) {
	e, repo, wb, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"일자사업"}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"},
	})
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: id, OrgID: model.OrgIDLibrary, Name: "전처리 업무", Status: model.PhaseActive, PlanEnd: "2026-12-01"}); err != nil {
		t.Fatal(err)
	}
	phases, _ := repo.ListPhases(id)
	phaseID := phases[0].PhaseID
	post := func(path string, v url.Values, cookies ...*http.Cookie) string {
		t.Helper()
		if len(cookies) == 0 {
			cookies = []*http.Cookie{jwtCookie(t)}
		}
		r := postPhase(t, e, "/projects/"+id+path, v, cookies...)
		if r.Code != http.StatusSeeOther {
			t.Fatalf("%s → %d", path, r.Code)
		}
		loc, _ := url.QueryUnescape(r.Header().Get("Location"))
		return loc
	}

	// 만들 때 배지·일자 (8자 초과 배지는 하부 업무가 만들어지지 않는다)
	if loc := post("/phases/"+phaseID+"/packages", url.Values{"name": {"나쁜배지"}, "track_mode": {"site"}, "badge": {"아홉글자배지이름임"}}); !strings.Contains(loc, "err=") || !strings.Contains(loc, "8자") {
		t.Fatalf("긴 배지: %s", loc)
	}
	if pk, _ := repo.LoadStructure(id, "2026-11-12", model.WeekendBiz); len(pk.Phases[0].Packages) != 0 {
		t.Fatal("검사에 실패했는데 하부 업무가 만들어졌다")
	}
	if loc := post("/phases/"+phaseID+"/packages", url.Values{"name": {"GBIC 수량 산정"}, "track_mode": {"site"}, "badge": {"사전GB"}, "plan_start": {"2026-11-01"}, "plan_end": {"2026-12-20"}}); !strings.Contains(loc, "ok=pkg") {
		t.Fatalf("만들기: %s", loc)
	}
	tree, _ := repo.LoadStructure(id, "2026-11-12", model.WeekendBiz)
	pkg := tree.Phases[0].Packages[0]
	if pkg.Badge != "사전GB" || pkg.PlanEnd != "2026-12-20" {
		t.Fatalf("저장 값 %+v", pkg)
	}
	page := doGet(t, e, "/projects/"+id).Body.String()
	if !strings.Contains(page, "data-pkg-badge") || !strings.Contains(page, "사전GB") || !strings.Contains(page, "예정 2026-11-01 ~ 2026-12-20") || !strings.Contains(page, "data-changes-link") {
		t.Fatal("화면에 배지·예정일·변경내역 링크가 없다")
	}
	// 다른 사업의 단계에는 하부 업무를 만들 수 없다
	if _, err := wb.DB().Exec(`INSERT INTO work_projects (project_id, name, status, org_id) VALUES ('WPO','타사업','active',?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if r := postPhase(t, e, "/projects/WPO/phases/"+phaseID+"/packages", url.Values{"name": {"침투"}}, jwtCookie(t)); r.Code != http.StatusNotFound {
		t.Fatalf("다른 사업 단계에 생성 %d", r.Code)
	}

	// 마감 연장: 사유 없으면 거절, 있으면 저장
	ext := url.Values{"plan_start": {"2026-11-01"}, "plan_end": {"2027-01-10"}, "badge": {"사전GB"}}
	if loc := post("/packages/"+pkg.ID, ext); !strings.Contains(loc, "err=") || !strings.Contains(loc, "연장 사유") {
		t.Fatalf("사유 없는 연장: %s", loc)
	}
	ext.Set("extend_reason", "발주처 일정 변경")
	if loc := post("/packages/"+pkg.ID, ext); !strings.Contains(loc, "ok=pkg-meta") {
		t.Fatalf("연장: %s", loc)
	}
	// 1단계 마감 연장도 같은 규칙
	if loc := post("/phases/"+phaseID, url.Values{"name": {"전처리 업무"}, "status": {"active"}, "plan_end": {"2026-12-31"}, "is_active": {"1"}}); !strings.Contains(loc, "err=") || !strings.Contains(loc, "연장 사유") {
		t.Fatalf("단계 사유 없는 연장: %s", loc)
	}
	if loc := post("/phases/"+phaseID, url.Values{"name": {"전처리 업무"}, "status": {"active"}, "plan_end": {"2026-12-31"}, "is_active": {"1"}, "extend_reason": {"검수 지연"}}); !strings.Contains(loc, "ok=phase") {
		t.Fatalf("단계 연장: %s", loc)
	}

	// 변경내역 탭: 이력 두 건, 사유·이전→이후·종류가 보인다. 임원(C-Level)도 조회는 된다
	body := doGet(t, e, "/projects/"+id+"/changes").Body.String()
	for _, want := range []string{"data-changes-table", "발주처 일정 변경", "검수 지연", "2026-12-20 → 2027-01-10", "2026-12-01 → 2026-12-31", "GBIC 수량 산정", "1단계 업무", "2단계 하부 업무"} {
		if !strings.Contains(body, want) {
			t.Fatalf("변경내역에 %q 가 없다", want)
		}
	}
	if strings.Count(body, `data-change-kind="extend"`) != 2 {
		t.Fatal("연장 이력이 2건이어야 한다")
	}
	clevel := jwtCookieUser(t, &model.User{UserID: "cl179", Username: "cl179", Role: model.RoleCLevel, OrgID: model.OrgIDLibrary})
	if b := doGetWith(t, e, "/projects/"+id+"/changes", clevel); b.Code != http.StatusOK || !strings.Contains(b.Body.String(), "발주처 일정 변경") {
		t.Fatalf("임원 조회 %d", b.Code)
	}
	// 쓰기는 막힌다
	if r := postPhase(t, e, "/projects/"+id+"/packages/"+pkg.ID, ext, clevel); r.Code != http.StatusForbidden {
		t.Fatalf("임원 수정 %d", r.Code)
	}
	// 이력 없는 사업
	empty := doGet(t, e, "/projects/WPO/changes").Body.String()
	if !strings.Contains(empty, "data-changes-empty") {
		t.Fatal("이력이 없을 때 안내가 없다")
	}
}

// 실제 시작일은 연결 작업이 처음 완료된 날로 자동 표시되고(「자동」), 수동값이 있으면 그 값이 보인다. 95-179C.
func Test95_179C_ActualStartAutoShownOnPage(t *testing.T) {
	e, repo, wb, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"시작일사업"}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"},
	})
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: id, OrgID: model.OrgIDLibrary, Name: "전처리 업무", Status: model.PhaseActive}); err != nil {
		t.Fatal(err)
	}
	phases, _ := repo.ListPhases(id)
	pkg, err := repo.CreatePackage(phases[0].PhaseID, "GBIC 수량 산정", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wb.DB().Exec(`INSERT INTO work_tasks(task_id, work_type, title, status, project_id, org_id, package_id, complete_date, work_date)
		VALUES ('W179C','project_work','완료작업','complete',?,?,?,'2026-11-05','2026-11-05')`, id, model.OrgIDLibrary, pkg); err != nil {
		t.Fatal(err)
	}
	page := doGet(t, e, "/projects/"+id).Body.String()
	if !strings.Contains(page, "실제 시작 2026-11-05 (자동)") || !strings.Contains(page, "data-phase-auto-start") || !strings.Contains(page, "실제 시작(자동): 2026-11-05") {
		t.Fatal("자동 실제 시작일이 화면에 없다")
	}
	// 수동 입력
	r := postPhase(t, e, "/projects/"+id+"/packages/"+pkg, url.Values{"actual_start": {"2026-11-01"}}, jwtCookie(t))
	if r.Code != http.StatusSeeOther {
		t.Fatalf("수동 저장 %d", r.Code)
	}
	page = doGet(t, e, "/projects/"+id).Body.String()
	if !strings.Contains(page, "실제 시작 2026-11-01") || strings.Contains(page, "실제 시작 2026-11-01 (자동)") {
		t.Fatal("수동값이 우선해서 (자동) 없이 보여야 한다")
	}
}
