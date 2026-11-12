package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 수량만 하부 업무: 목표 저장 → 처리 수량 입력 → 사업 상세 「수행 구조」에 N/M 단위(%)가 나온다.
func Test95_176_QtyOnlyPackageHTTP(t *testing.T) {
	e, repo, _, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"수량사업"}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("등록 %d", rec.Code)
	}
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: id, OrgID: model.OrgIDLibrary, Name: "전처리 업무", Status: model.PhaseActive}); err != nil {
		t.Fatal(err)
	}
	phases, _ := repo.ListPhases(id)
	qtyPkg, err := repo.CreatePackage(phases[0].PhaseID, "환경조사", model.TrackModePhase, 1)
	if err != nil {
		t.Fatal(err)
	}
	sitePkg, err := repo.CreatePackage(phases[0].PhaseID, "현장작업", model.TrackModeSite, 1)
	if err != nil {
		t.Fatal(err)
	}

	page := doGet(t, e, "/projects/"+id).Body.String()
	if !strings.Contains(page, "data-qty-target") || strings.Contains(page, "data-qty-record") {
		t.Fatal("목표 전에는 목표 저장 칸만 있어야 한다")
	}
	if !strings.Contains(page, "목표 수량 미설정") {
		t.Fatal("목표 미설정 표시가 없다")
	}
	// 사이트 하부 업무에는 수량 칸이 없고 「+ 작업 계획」이 있다
	if strings.Count(page, "data-qty-line") != 1 || !strings.Contains(page, `data-add-plan="`+sitePkg+`"`) || strings.Contains(page, `data-add-plan="`+qtyPkg+`"`) {
		t.Fatal("수량만/사이트 하부 업무 표시가 섞였다")
	}

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
	if loc := post("/packages/"+qtyPkg+"/qty", url.Values{"qty": {"10"}}); !strings.Contains(loc, "err=") || !strings.Contains(loc, "목표 수량을 먼저") {
		t.Fatalf("목표 전 입력: %s", loc)
	}
	if loc := post("/packages/"+qtyPkg+"/qty-target", url.Values{"qty_total": {"120"}, "qty_unit": {"식"}}); !strings.Contains(loc, "ok=qty-target") {
		t.Fatalf("목표 저장: %s", loc)
	}
	if loc := post("/packages/"+qtyPkg+"/qty", url.Values{"qty": {"30"}, "date": {"2026-11-12"}}); !strings.Contains(loc, "ok=qty") {
		t.Fatalf("수량 입력: %s", loc)
	}
	if loc := post("/packages/"+qtyPkg+"/qty", url.Values{"qty": {"abc"}}); !strings.Contains(loc, "정수로 입력") {
		t.Fatalf("정수 아님: %s", loc)
	}
	if loc := post("/packages/"+qtyPkg+"/qty", url.Values{"qty": {"91"}}); !strings.Contains(loc, "넘습니다") {
		t.Fatalf("목표 초과: %s", loc)
	}
	if loc := post("/packages/"+sitePkg+"/qty", url.Values{"qty": {"1"}}); !strings.Contains(loc, "수량만 하부 업무에서만") {
		t.Fatalf("사이트 하부 업무: %s", loc)
	}
	page = doGet(t, e, "/projects/"+id).Body.String()
	if !strings.Contains(page, "30/120식 (25%)") || !strings.Contains(page, "data-qty-record") {
		t.Fatal("진행 수량이 화면에 없다")
	}

	// 임원(C-Level)은 조회만: 입력 칸이 없고 POST 는 403
	clevel := jwtCookieUser(t, &model.User{UserID: "cl176q", Username: "cl176q", Role: model.RoleCLevel, OrgID: model.OrgIDLibrary})
	for _, path := range []string{"/packages/" + qtyPkg + "/qty", "/packages/" + qtyPkg + "/qty-target"} {
		r := postPhase(t, e, "/projects/"+id+path, url.Values{"qty": {"1"}, "qty_total": {"500"}}, clevel)
		if r.Code != http.StatusForbidden {
			t.Fatalf("임원 %s → %d", path, r.Code)
		}
	}
	cpage := doGetWith(t, e, "/projects/"+id, clevel).Body.String()
	if strings.Contains(cpage, "data-qty-target") || strings.Contains(cpage, "data-qty-record") || !strings.Contains(cpage, "30/120식 (25%)") {
		t.Fatal("임원 화면: 조회만 되고 입력 칸은 없어야 한다")
	}
}
