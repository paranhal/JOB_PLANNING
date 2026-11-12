package handler

import (
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 작업 계획 수정: 이름·일자·다음 할 일, 마감 연장은 사유 필수·이력, 완료된 계획도 수정, 다른 사업 계획은 거절. 95-180.
func Test95_180_PlanEditExtendHistory(t *testing.T) {
	e, repo, _, _ := newProjectServerWithDB(t)
	mk := func(name string) string {
		rec := doForm(t, e, "/projects", url.Values{
			"name": {name}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"build"},
			"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"},
			"org_id": {model.OrgIDLibrary}, "is_paid": {"1"}, "contract_amount": {"1500000"},
		})
		return strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	}
	id, other := mk("계획수정사업"), mk("다른사업")
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: id, OrgID: model.OrgIDLibrary, Name: "전처리", Status: model.PhaseActive}); err != nil {
		t.Fatal(err)
	}
	phases, _ := repo.ListPhases(id)
	pkgID, err := repo.CreatePackage(phases[0].PhaseID, "GBIC", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	planID, err := repo.CreatePlan(id, repoPlanDraft(pkgID, "홍천 50곳", "2026-11-01", "2026-11-30"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreatePlan(id, repoPlanDraft(pkgID, "횡성 30곳", "", "")); err != nil {
		t.Fatal(err)
	}
	post := func(proj, plan string, v url.Values) string {
		t.Helper()
		r := postPhase(t, e, "/projects/"+proj+"/plans/"+plan, v, jwtCookie(t))
		loc, _ := url.QueryUnescape(r.Header().Get("Location"))
		if r.Code != 303 {
			t.Fatalf("status=%d", r.Code)
		}
		return loc
	}
	base := func() url.Values {
		return url.Values{"name": {"홍천 50곳"}, "plan_start": {"2026-11-01"}, "plan_end": {"2026-11-30"}, "next_note": {""}}
	}
	// 사유 없이 연장 → 거절, 값 불변
	v := base()
	v.Set("plan_end", "2026-12-15")
	if loc := post(id, planID, v); !strings.Contains(loc, "err=") || !strings.Contains(loc, "연장 사유") {
		t.Fatalf("사유 없는 연장: %s", loc)
	}
	if ch, _ := repo.ListDateChanges(id, 50); len(ch) != 0 {
		t.Fatal("거절됐는데 이력이 남았다")
	}
	// 사유와 함께 연장 + 이름·다음 할 일 변경
	v.Set("extend_reason", "현장 일정 지연")
	v.Set("name", "홍천 지역 50곳")
	v.Set("next_note", "장비 반입")
	if loc := post(id, planID, v); !strings.Contains(loc, "ok=plan") {
		t.Fatalf("연장: %s", loc)
	}
	ch, _ := repo.ListDateChanges(id, 50)
	if len(ch) != 1 || ch[0].Level != "plan" || ch[0].Kind != model.EndChangeExtend || ch[0].OldValue != "2026-11-30" || ch[0].NewValue != "2026-12-15" || ch[0].Reason != "현장 일정 지연" || ch[0].TargetName != "홍천 지역 50곳" {
		t.Fatalf("이력 %+v", ch)
	}
	tree, _ := repo.LoadStructure(id, "2026-11-12", model.WeekendBiz)
	var got *struct{ n, e, s, x string }
	for _, pl := range tree.Phases[0].Packages[0].Plans {
		if pl.ID == planID {
			got = &struct{ n, e, s, x string }{pl.Name, pl.PlanEnd, pl.PlanStart, pl.NextNote}
		}
	}
	if got == nil || got.n != "홍천 지역 50곳" || got.e != "2026-12-15" || got.s != "2026-11-01" || got.x != "장비 반입" {
		t.Fatalf("저장 값 %+v", got)
	}
	// 앞당김은 사유 없이 허용, 이력 남김
	v = base()
	v.Set("name", "홍천 지역 50곳")
	v.Set("plan_end", "2026-12-01")
	if loc := post(id, planID, v); !strings.Contains(loc, "ok=plan") {
		t.Fatalf("앞당김: %s", loc)
	}
	if ch, _ = repo.ListDateChanges(id, 50); len(ch) != 2 || ch[0].Kind != model.EndChangeShorten {
		t.Fatalf("앞당김 이력 %+v", ch)
	}
	// 같은 이름 거절, 이름 그대로면 통과
	v = base()
	v.Set("name", "횡성 30곳")
	v.Set("plan_end", "2026-12-01")
	if loc := post(id, planID, v); !strings.Contains(loc, "같은 이름") {
		t.Fatalf("중복 이름: %s", loc)
	}
	// 완료된 계획도 수정 허용(이력 남김)
	if err := repo.SetPlanStatus(id, planID, model.PkgDone, "t"); err != nil {
		t.Fatal(err)
	}
	v = base()
	v.Set("name", "홍천 지역 50곳")
	v.Set("plan_end", "2026-12-20")
	v.Set("extend_reason", "추가 점검")
	if loc := post(id, planID, v); !strings.Contains(loc, "ok=plan") {
		t.Fatalf("완료 계획 수정: %s", loc)
	}
	if ch, _ = repo.ListDateChanges(id, 50); len(ch) != 3 {
		t.Fatalf("이력 %d건", len(ch))
	}
	// 다른 사업의 계획은 수정 불가
	if loc := post(other, planID, v); !strings.Contains(loc, "err=") {
		t.Fatalf("다른 사업 계획: %s", loc)
	}
	// 변경내역 화면과 사업 화면
	page := doGet(t, e, "/projects/"+id+"/changes").Body.String()
	if !strings.Contains(page, "홍천 지역 50곳") || !strings.Contains(page, "현장 일정 지연") {
		t.Fatal("변경내역 화면에 작업 계획 이력이 없다")
	}
	show := doGet(t, e, "/projects/"+id).Body.String()
	if !strings.Contains(show, "data-plan-edit") || !strings.Contains(show, `name="extend_reason"`) {
		t.Fatal("사업 화면에 계획 수정 폼이 없다")
	}
}

func repoPlanDraft(pkgID, name, start, end string) repository.PlanDraft {
	return repository.PlanDraft{PackageID: pkgID, Name: name, PlanStart: start, PlanEnd: end, Basis: model.BasisSite, Actor: "t"}
}
