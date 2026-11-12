package handler

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-179E 대시보드 마감 임박 칩: 일자 있는 미완료 대상만, 일자 없으면 칩 없음, 완료되면 사라진다.
func Test95_179E_DashboardDeadlineChip(t *testing.T) {
	e, repo, _, _ := newProjectServerWithDB(t)
	mk := func(name string) string {
		rec := doForm(t, e, "/projects", url.Values{
			"name": {name}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"build"},
			"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"}, "org_id": {model.OrgIDLibrary}, "is_paid": {"1"}, "contract_amount": {"1500000"},
		})
		return strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	}
	withDate, noDate := mk("마감있는사업"), mk("마감없는사업")
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: withDate, OrgID: model.OrgIDLibrary, Name: "지난단계", Status: model.PhaseActive, PlanEnd: "2020-01-05"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: noDate, OrgID: model.OrgIDLibrary, Name: "일자없음", Status: model.PhaseActive}); err != nil {
		t.Fatal(err)
	}
	body := doGet(t, e, "/projects/dashboard").Body.String()
	if !strings.Contains(body, "data-deadline-soon") || !strings.Contains(body, "마감 지남") || !strings.Contains(body, "지난단계") {
		t.Fatalf("마감 지난 단계 칩이 없다 사업표시=%v 단계표시=%v", strings.Contains(body, "마감있는사업"), strings.Contains(body, "data-phase-current"))
	}
	if strings.Count(body, "data-deadline-soon") != 1 {
		t.Fatalf("일자 없는 사업에는 칩이 없어야 한다: %d", strings.Count(body, "data-deadline-soon"))
	}
	// 완료하면 칩이 사라진다
	phases, _ := repo.ListPhases(withDate)
	p := phases[0]
	p.Status = model.PhaseDone
	if err := repo.UpdatePhase(&p); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doGet(t, e, "/projects/dashboard").Body.String(), "data-deadline-soon") {
		t.Fatal("완료된 단계는 칩이 없어야 한다")
	}
}

func Test95_179E_DeadlineSoonByProject(t *testing.T) {
	_, repo, _, _ := newProjectServerWithDB(t)
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: "PX", OrgID: model.OrgIDLibrary, Name: "A", Status: model.PhaseActive, PlanEnd: "2026-11-20"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		today string
		days  int
		want  bool
		n     int
	}{
		{"2026-11-12", 7, false, 0}, // 8일 남음 → 기준 밖
		{"2026-11-13", 7, true, 7},
		{"2026-11-20", 7, true, 0},
		{"2026-11-25", 7, true, -5},
	} {
		got, err := repo.DeadlineSoonByProject([]string{"PX"}, c.today, c.days)
		if err != nil {
			t.Fatal(err)
		}
		d, ok := got["PX"]
		if ok != c.want || (ok && d.Days != c.n) {
			t.Fatalf("%s: ok=%v %+v", c.today, ok, d)
		}
	}
}

// 95-179F 사이트 등록 안내: 수정 화면 카드, 수행 구조 바로가기, 「등록 N곳 중 연결 M곳」.
func Test95_179F_SiteRegistrationGuide(t *testing.T) {
	e, _, wb, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"안내사업"}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"},
		"org_id": {model.OrgIDLibrary}, "is_paid": {"1"}, "contract_amount": {"1500000"},
	})
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	for i, n := range []string{"가학교", "나학교", "다학교"} {
		if _, err := wb.DB().Exec(`INSERT INTO project_sites (id, project_id, site_name, region, status, is_active, org_id) VALUES (?,?,?,?,?,1,?)`,
			fmt.Sprintf("PS-G%d", i), id, n, "강원", map[bool]string{true: "excluded", false: "planned"}[i == 2], model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	edit := doGet(t, e, "/projects/"+id+"/edit").Body.String()
	if !strings.Contains(edit, "data-site-reg-card") || !strings.Contains(edit, "등록된 사이트 2곳") || !strings.Contains(edit, "/projects/"+id+"/sites") {
		t.Fatal("수정 화면에 사이트 등록 안내 카드가 없다(제외 상태는 세지 않는다)")
	}
	show := doGet(t, e, "/projects/"+id).Body.String()
	if !strings.Contains(show, "등록 2곳 중 작업 연결 0곳") {
		t.Fatal("사이트·진행에 「등록 N곳 중 작업 연결 M곳」이 없다")
	}
	if !strings.Contains(show, "data-register-shortcut") && strings.Contains(show, "data-plan-sites") {
		t.Fatal("작업 계획 사이트 선택에 등록 바로가기가 없다")
	}
	// 새 사업 등록 화면에는 카드가 없다
	if strings.Contains(doGet(t, e, "/projects/new").Body.String(), "data-site-reg-card") {
		t.Fatal("등록 화면에는 사이트 카드가 없어야 한다")
	}
}
