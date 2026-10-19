package handler

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestProjectDashboardHasFigmaItems(t *testing.T) {
	e, _, _ := newProjectServer(t)
	page := doGet(t, e, "/projects/dashboard")
	if page.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", page.Code, clipBody(page.Body.String()))
	}
	body := page.Body.String()
	for _, want := range []string{
		"사업 대시보드",
		"업무 리포트",
		"공정→기성→준공",
		"구축→개통→검수",
		"접수→조치→완료",
		"사업 운영 현황",
		"kind=other",
		"계약 사업",
		"진행 중 사업",
		"완료 확인 필요",
		"보기 조건",
		`role="separator"`,
		"열 너비 초기화",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("화면에 %q 없음", want)
		}
	}
	for _, leak := range []string{"김현우", "A동 골조공사", "data-node-id", "화면 설계를 위한 예시", "계약 미확인"} {
		if strings.Contains(body, leak) {
			t.Fatalf("샘플/원본 흔적 %q", leak)
		}
	}
}

func TestProjectsSectorVisibleOnLocalProductionEnv(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SHOW_PROJECTS_SECTOR", "")
	e, _, _ := newProjectServer(t)
	home := doGet(t, e, "/projects")
	if home.Code != http.StatusOK {
		t.Fatalf("list=%d", home.Code)
	}
	if !strings.Contains(home.Body.String(), "사업 대시보드") {
		t.Fatal("로컬에서는 APP_ENV=production 이어도 사업관리 메뉴가 있어야 한다")
	}
	dash := doGet(t, e, "/projects/dashboard")
	if dash.Code != http.StatusOK {
		t.Fatalf("로컬 대시보드 status=%d", dash.Code)
	}
	if strings.Contains(dash.Body.String(), "김현우") || strings.Contains(dash.Body.String(), "A동 골조공사") {
		t.Fatal("예시 데이터가 나왔다")
	}
}

func TestProjectsSectorHiddenWhenProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SHOW_PROJECTS_SECTOR", "0")
	e, _, _ := newProjectServer(t)
	home := doGet(t, e, "/projects")
	if home.Code != http.StatusOK {
		t.Fatalf("list=%d", home.Code)
	}
	body := home.Body.String()
	if strings.Contains(body, ">사업관리<") {
		t.Fatal("운영 홈에 사업관리 초안 메뉴가 보인다")
	}
	if !strings.Contains(body, "수행관계") {
		t.Fatal("운영에서도 수행관계는 남아야 한다")
	}
	dash := doGet(t, e, "/projects/dashboard")
	if dash.Code != http.StatusOK {
		t.Fatalf("운영에서도 URL은 열려야 한다 status=%d", dash.Code)
	}
	if strings.Contains(dash.Body.String(), "김현우") {
		t.Fatal("운영 URL에 예시 데이터가 있다")
	}
}

func TestProjectExampleDataBlankOnProductionSector(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SHOW_PROJECTS_SECTOR", "1")
	e, _, _ := newProjectServer(t)
	for _, path := range []string{"/projects", "/projects/dashboard", "/projects/docs", "/projects/diary"} {
		page := doGet(t, e, path)
		if page.Code != http.StatusOK {
			t.Fatalf("%s status=%d", path, page.Code)
		}
		body := page.Body.String()
		for _, leak := range []string{"김현우", "A동 골조공사", "충남세종 RFID", "I-2026-1012"} {
			if strings.Contains(body, leak) {
				t.Fatalf("운영 화면 %s 에 예시 데이터 %q 가 있다", path, leak)
			}
		}
	}
}

func TestTemplatesHaveNoUnconfirmedContractArea(t *testing.T) {
	root := findTemplateRoot(t)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "계약 미확인") {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s 에 계약 미확인", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProjectDashboardContractedKindAndAct(t *testing.T) {
	e, repo, wb := newProjectServer(t)
	hidden := &model.WorkProject{Name: "계약전숨김", PlanYear: 2026, Status: model.WBProjectActive}
	if err := repo.Create(hidden); err != nil {
		t.Fatal(err)
	}
	build := &model.WorkProject{
		Name: "구축계약", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 1000,
		ProjectKind: model.ProjectKindBuild, ProjectKindSet: 1, StartDate: "2020-01-01", EndDate: "2099-01-01",
	}
	if err := repo.Create(build); err != nil {
		t.Fatal(err)
	}
	need := &model.WorkProject{
		Name: "종료확인", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 500,
		ProjectKind: model.ProjectKindConstruction, ProjectKindSet: 1, StartDate: "2020-01-01", EndDate: "2020-02-01",
	}
	if err := repo.Create(need); err != nil {
		t.Fatal(err)
	}
	tk := &model.WorkTask{WorkType: model.WBWorkAdmin, ProjectID: build.ProjectID, Title: "대기", Status: model.WBTaskWaiting, DueDate: "2026-10-20", WorkDate: "2026-10-19"}
	if err := wb.CreateTask(tk); err != nil {
		t.Fatal(err)
	}

	all := doGet(t, e, "/projects/dashboard")
	b := all.Body.String()
	if all.Code != http.StatusOK {
		t.Fatalf("dash %d", all.Code)
	}
	if strings.Contains(b, "계약전숨김") {
		t.Fatal("비계약이 목록에 있다")
	}
	if !strings.Contains(b, "구축계약") || !strings.Contains(b, "종료확인") {
		t.Fatal("계약 사업이 없다")
	}
	if !strings.Contains(b, `/contracts/`+build.ProjectID) {
		t.Fatal("사업명 계약 링크 없음")
	}
	if !strings.Contains(b, "완료 확인 필요") {
		t.Fatal("종료 경과 상태 없음")
	}
	if strings.Contains(b, "kind=unclassified") {
		t.Fatal("확정만 있는데 미분류 탭")
	}
	task := doGet(t, e, "/projects/dashboard?act=task")
	if !strings.Contains(task.Body.String(), "구축계약") {
		t.Fatal("일반 업무 필터")
	}
	if strings.Contains(task.Body.String(), "종료확인") {
		t.Fatal("업무 없는 행이 필터에 남음")
	}
}

func TestProjectDashboardHidesUnclassifiedTabWhenEmpty(t *testing.T) {
	e, repo, _ := newProjectServer(t)
	p := &model.WorkProject{
		Name: "확정만", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 10,
		ProjectKind: model.ProjectKindDev, ProjectKindSet: 1,
	}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	b := doGet(t, e, "/projects/dashboard").Body.String()
	if strings.Contains(b, "kind=unclassified") || strings.Contains(b, ">미분류 ") {
		t.Fatal("미분류 0건인데 탭이 있다")
	}
	if !strings.Contains(b, "확정만") {
		t.Fatal("목록 없음")
	}
	dev := doGet(t, e, "/projects/dashboard?kind=dev")
	if !strings.Contains(dev.Body.String(), "확정만") {
		t.Fatal("개발 탭 목록 없음")
	}
	con := doGet(t, e, "/projects/dashboard?kind=construction")
	if strings.Contains(con.Body.String(), "확정만") {
		t.Fatal("공사 탭에 개발 건")
	}
	if !strings.Contains(dev.Body.String(), "진행 중 사업") {
		t.Fatal("KPI 없음")
	}
}

func TestProjectDashboardActFilterAndConvertCopy(t *testing.T) {
	e, db := newSalesServerDB(t)
	prepo := repository.NewProjectRepo(db)
	p := &model.WorkProject{
		Name: "필터대상", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 10,
		ProjectKind: model.ProjectKindOther, ProjectKindSet: 1,
	}
	if err := prepo.Create(p); err != nil {
		t.Fatal(err)
	}
	srepo := repository.NewSalesRepo(db)
	s := &model.SalesProject{Name: "전환대기"}
	if err := srepo.Create(s); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, contract_amount=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, 100, s.SalesID); err != nil {
		t.Fatal(err)
	}
	page := doGet(t, e, "/projects/dashboard")
	body := page.Body.String()
	if page.Code != http.StatusOK {
		t.Fatalf("%d %s", page.Code, clipBody(body))
	}
	if !strings.Contains(body, "전환대기") || !strings.Contains(body, "계약으로 전환") {
		t.Fatal("미전환 행 없음")
	}
	if !strings.Contains(body, "/sales/"+s.SalesID) {
		t.Fatal("영업 링크 없음")
	}
	if strings.Contains(body, "사업관리로 전환") {
		t.Fatal("옛 단추 문구")
	}
	act := doGet(t, e, "/projects/dashboard?act=task")
	if strings.Contains(act.Body.String(), "전환대기") {
		t.Fatal("일반 업무 필터에 영업 행")
	}
	modal := doGet(t, e, "/contracts/"+p.ProjectID+"?modal=1")
	if modal.Code != http.StatusOK || strings.Contains(modal.Body.String(), "<html") {
		t.Fatalf("모달 부분템플릿 %d", modal.Code)
	}
	if !strings.Contains(modal.Body.String(), `role="dialog"`) || !strings.Contains(modal.Body.String(), "전체 화면으로 열기") {
		t.Fatal("모달 내용")
	}
	sm := doGet(t, e, "/sales/"+s.SalesID+"?modal=1")
	if !strings.Contains(sm.Body.String(), "계약으로 전환") || strings.Contains(sm.Body.String(), "<html") {
		t.Fatal("영업 모달")
	}
}

func TestProjectDashboardNamePlainWithoutSalesView(t *testing.T) {
	e, repo, _ := newProjectServer(t)
	p := &model.WorkProject{
		Name: "링크숨김사업", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 1,
		ProjectKind: model.ProjectKindOther, ProjectKindSet: 1,
	}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookiePerms(t, model.RoleTester, model.PermWorkboard)
	page := doGetWith(t, e, "/projects/dashboard", ck)
	if page.Code != http.StatusOK {
		t.Fatalf("%d", page.Code)
	}
	if strings.Contains(page.Body.String(), `href="/contracts/`+p.ProjectID+`"`) {
		t.Fatal("영업 조회 없이 계약 링크")
	}
	if !strings.Contains(page.Body.String(), "링크숨김사업") {
		t.Fatal("텍스트 이름 없음")
	}
}
