package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_120P_PhaseHTTP(t *testing.T) {
	t.Setenv("VIEW_AS_ENABLED", "1")
	e, repo, _, users := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"단계테스트사업"}, "short_name": {"단계사업"}, "plan_year": {"2026"},
		"is_paid": {"1"}, "contract_type": {"구축"}, "billing_type": {"일시불"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-01-10"},
		"status": {"active"}, "project_kind": {"build"}, "contract_amount": {"1000"}, "color": {"#3B82F6"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("등록 %d %s", rec.Code, rec.Body.String())
	}
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	before, _ := repo.ListPhases(id)
	prev := doGet(t, e, "/projects/"+id+"/phases/preview")
	if prev.Code != http.StatusSeeOther {
		t.Fatalf("미리보기 %d", prev.Code)
	}
	page := doGet(t, e, strings.Split(prev.Header().Get("Location"), "#")[0])
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "착수") || !strings.Contains(body, "저장 전 미리보기") || !strings.Contains(body, "data-phase-tab open") {
		t.Fatalf("미리보기 화면 %d", page.Code)
	}
	after, _ := repo.ListPhases(id)
	if len(after) != len(before) {
		t.Fatal("미리보기가 저장했다")
	}
	if !strings.Contains(body, "data-site-progress-tab") {
		t.Fatal("사이트·진행이 없다")
	}
	fill := doForm(t, e, "/projects/"+id+"/phases/fill", url.Values{
		"name":       {"착수", "분석", "설계"},
		"plan_start": {"2026-01-01", "2026-01-04", "2026-01-07"},
		"plan_end":   {"2026-01-03", "2026-01-06", "2026-01-10"},
	})
	if fill.Code != http.StatusSeeOther {
		t.Fatalf("채우기 %d", fill.Code)
	}
	list, err := repo.ListPhases(id)
	if err != nil || len(list) != 3 {
		t.Fatalf("저장 %d %v", len(list), err)
	}
	again := doForm(t, e, "/projects/"+id+"/phases/fill", url.Values{"name": {"착수", "새단계"}})
	if again.Code != http.StatusSeeOther {
		t.Fatal(again.Code)
	}
	list, _ = repo.ListPhases(id)
	if len(list) != 4 {
		t.Fatalf("중복은 건너뛰고 새 이름만 %d", len(list))
	}
	long := strings.Repeat("가", 101)
	bad := doForm(t, e, "/projects/"+id+"/phases", url.Values{"name": {long}})
	if bad.Code != http.StatusSeeOther || !strings.Contains(bad.Header().Get("Location"), "100") {
		t.Fatalf("긴 이름 %d %s", bad.Code, bad.Header().Get("Location"))
	}
	badDate := doForm(t, e, "/projects/"+id+"/phases", url.Values{
		"name": {"날짜"}, "plan_start": {"2026-05-02"}, "plan_end": {"2026-05-01"},
	})
	if badDate.Code != http.StatusSeeOther || !strings.Contains(badDate.Header().Get("Location"), "err=") {
		t.Fatal("날짜 순서")
	}
	second := list[1]
	st := doForm(t, e, "/projects/"+id+"/phases/"+second.PhaseID+"/status", url.Values{"status": {"active"}})
	if st.Code != http.StatusSeeOther || !strings.Contains(st.Header().Get("Location"), "phase_confirm=") {
		t.Fatalf("확인 화면 %s", st.Header().Get("Location"))
	}
	first, _ := repo.GetPhase(list[0].PhaseID)
	if first.Status != model.PhasePlanned {
		t.Fatal("확인 전 저장")
	}
	ok := doForm(t, e, "/projects/"+id+"/phases/"+second.PhaseID+"/status", url.Values{"status": {"active"}, "confirm_prior": {"1"}})
	if ok.Code != http.StatusSeeOther {
		t.Fatal(ok.Code)
	}
	first, _ = repo.GetPhase(list[0].PhaseID)
	secondNow, _ := repo.GetPhase(second.PhaseID)
	today := time.Now().Format("2006-01-02")
	if first.Status != model.PhaseDone || first.ActualEnd != today || secondNow.Status != model.PhaseActive || secondNow.ActualStart != today {
		t.Fatalf("확인 후 %+v %+v", first, secondNow)
	}
	show := doGet(t, e, "/projects/"+id)
	if !strings.Contains(show.Body.String(), "지연") && secondNow.PlanEnd < today {
		t.Fatal("지연 표시")
	}

	mnt := doForm(t, e, "/projects", url.Values{
		"name": {"유지보수단계"}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"maintenance"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"},
	})
	mid := strings.TrimPrefix(strings.Split(mnt.Header().Get("Location"), "?")[0], "/projects/")
	mb := doGet(t, e, "/projects/"+mid).Body.String()
	if !strings.Contains(mb, "사용 안 함") || !strings.Contains(mb, "data-site-progress-tab") || strings.Contains(mb, "data-phase-tab open") {
		t.Fatal("유지보수 단계는 접고 사이트·진행은 남긴다")
	}
	if strings.Contains(mb, "기본 단계로 채우기") {
		t.Fatal("유지보수에 기본 단계")
	}

	obs := jwtCookieUser(t, &model.User{UserID: "obs120", Username: "obs120", Role: model.RoleObserver, IsReadOnly: true, OrgID: model.OrgIDLibrary})
	deny := postPhase(t, e, "/projects/"+id+"/phases", url.Values{"name": {"옵저버"}}, obs)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("옵저버 %d", deny.Code)
	}
	viewed := &model.User{Username: "view120", PasswordHash: "x", FullName: "열람대상", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(viewed); err != nil {
		t.Fatal(err)
	}
	view := postPhase(t, e, "/projects/"+id+"/phases", url.Values{"name": {"열람"}}, jwtCookie(t), &http.Cookie{Name: viewAsCookie, Value: viewed.UserID, Path: "/"})
	if view.Code != http.StatusForbidden {
		t.Fatalf("view-as %d", view.Code)
	}
	clevel := jwtCookieUser(t, &model.User{UserID: "cl120", Username: "cl120", Role: model.RoleCLevel, OrgID: model.OrgIDLibrary})
	cl := postPhase(t, e, "/projects/"+id+"/phases", url.Values{"name": {"임원"}}, clevel)
	if cl.Code != http.StatusForbidden {
		t.Fatalf("임원 %d", cl.Code)
	}
	otherP := &model.WorkProject{Name: "타조직", Status: "active", OrgID: "O02"}
	if err := repo.Create(otherP); err != nil {
		t.Fatal(err)
	}
	other := doForm(t, e, "/projects/"+otherP.ProjectID+"/phases", url.Values{"name": {"타조직단계"}})
	if other.Code != http.StatusNotFound && other.Code != http.StatusForbidden {
		t.Fatalf("다른 조직 %d", other.Code)
	}

	dash := doGet(t, e, "/projects/dashboard")
	db := dash.Body.String()
	if dash.Code != http.StatusOK || !strings.Contains(db, "현재 단계") || !strings.Contains(db, "단계 진행") || !strings.Contains(db, "단계 지연") || !strings.Contains(db, `data-col="11"`) {
		t.Fatalf("대시보드 열 %d", dash.Code)
	}
	mntDash := doGet(t, e, "/projects/dashboard?kind=maintenance")
	if strings.Contains(mntDash.Body.String(), "단계 지연") {
		t.Fatal("유지보수 탭에 단계 지연 칩")
	}
	latePage := doGet(t, e, "/projects/dashboard?phase_late=1")
	lb := latePage.Body.String()
	if !strings.Contains(lb, "단계테스트사업") {
		t.Fatal("지연 필터에 지연 사업이 없다")
	}
	if strings.Contains(lb, "유지보수단계") {
		t.Fatal("지연이 아닌 사업이 필터에 있다")
	}
}

func postPhase(t *testing.T, e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}
