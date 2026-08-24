package handler

import (
	"database/sql"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newProjectServer(t *testing.T) (*echo.Echo, *repository.ProjectRepo, *repository.WBRepo, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/projects", h.Project.List)
	g.GET("/projects/new", h.Project.New)
	g.POST("/projects", h.Project.Create)
	g.GET("/projects/:id", h.Project.Show)
	g.GET("/projects/:id/edit", h.Project.Edit)
	g.POST("/projects/:id", h.Project.Update)
	g.POST("/projects/:id/promote-customer", h.Project.PromoteCustomer)
	g.POST("/projects/:id/stage", h.Project.SetStage)
	g.POST("/projects/:id/follow-up", h.Project.AddFollowup)
	g.POST("/projects/:id/archive", h.Project.Archive)
	g.POST("/projects/:id/activate", h.Project.Activate)
	g.POST("/projects/:id/delete", h.Project.Delete)
	return e, repository.NewProjectRepo(db), repository.NewWBRepo(db), db
}

func TestProjectCRUDArchiveSmoke(t *testing.T) {
	e, repo, wb, _ := newProjectServer(t)

	// 등록
	rec := doForm(t, e, "/projects", url.Values{
		"name":          {"스모크 테스트 사업"},
		"short_name":    {"스모크사업"},
		"plan_year":     {"2026"},
		"is_paid":       {"1"},
		"contract_type": {"유지보수"},
		"billing_type":  {"월정액"},
		"start_date":    {"2026-01-01"},
		"end_date":      {"2026-12-31"},
		"status":        {"active"},
		"color":         {"#3B82F6"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("등록 status=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/projects/") || !strings.Contains(loc, "ok=created") {
		t.Fatalf("등록 redirect=%q", loc)
	}
	id := strings.TrimPrefix(strings.Split(loc, "?")[0], "/projects/")
	if id == "" {
		t.Fatal("project id 없음")
	}

	// 목록 노출
	list := doGet(t, e, "/projects?search="+url.QueryEscape("스모크"))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "스모크사업") {
		t.Fatalf("목록 미노출 status=%d", list.Code)
	}

	// 수정
	rec = doForm(t, e, "/projects/"+id, url.Values{
		"name":       {"스모크 테스트 사업 수정"},
		"short_name": {"스모크수정"},
		"plan_year":  {"2026"},
		"is_paid":    {"1"},
		"status":     {"active"},
		"color":      {"#10B981"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=updated") {
		t.Fatalf("수정: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	got, err := repo.Get(id)
	if err != nil || got.ShortName != "스모크수정" {
		t.Fatalf("수정 반영 실패: %+v err=%v", got, err)
	}

	// 보관 → 활성 콤보 제외
	rec = doForm(t, e, "/projects/"+id+"/archive", url.Values{})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=archived") {
		t.Fatalf("보관: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	active, err := wb.ListProjects(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range active {
		if p.ProjectID == id {
			t.Fatal("보관 사업이 활성 콤보에 남음")
		}
	}
	p, err := wb.GetProject(id)
	if err != nil || p.Status != model.WBProjectArchived {
		t.Fatalf("보관 상태: %+v err=%v", p, err)
	}

	// 재개
	rec = doForm(t, e, "/projects/"+id+"/activate", url.Values{})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=activated") {
		t.Fatalf("재개: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	filtered, err := wb.ListProjectsFiltered("스모크수정", model.WBProjectActive)
	if err != nil || len(filtered) != 1 {
		t.Fatalf("재개 후 필터: len=%d err=%v", len(filtered), err)
	}

	// 연결 업무 없이 삭제
	rec = doForm(t, e, "/projects/"+id+"/delete", url.Values{})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=deleted") {
		t.Fatalf("삭제: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestWBRepoProjectHelpers(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "wb_proj.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := repository.NewWBRepo(db)
	p := &model.WorkProject{Name: "헬퍼사업", Status: model.WBProjectActive, Color: "#3B82F6", IsPaid: true}
	if err := wb.CreateProject(p); err != nil {
		t.Fatal(err)
	}
	got, err := wb.GetProject(p.ProjectID)
	if err != nil || got.Name != "헬퍼사업" {
		t.Fatalf("GetProject: %+v err=%v", got, err)
	}
	got.Notes = "메모"
	if err := wb.UpdateProject(got); err != nil {
		t.Fatal(err)
	}
	n, err := wb.CountTasksByProject(p.ProjectID)
	if err != nil || n != 0 {
		t.Fatalf("CountTasks: %d err=%v", n, err)
	}
	if err := wb.SetProjectStatus(p.ProjectID, model.WBProjectArchived); err != nil {
		t.Fatal(err)
	}
	list, err := wb.ListProjectsFiltered("헬퍼", model.WBProjectArchived)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListProjectsFiltered: %d err=%v", len(list), err)
	}
	if err := wb.DeleteProject(p.ProjectID); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceFormUnchanged(t *testing.T) {
	e, _, _, _ := newProjectServer(t)
	rec := doGet(t, e, "/projects/new")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "가등록 기관명") || strings.Contains(body, "영업담당") {
		t.Fatal("유지보수 등록 화면에 영업 칸이 보인다")
	}
	if strings.Contains(body, "영업 건 —") {
		t.Fatal("유지보수 등록 안내 문구가 바뀌었다")
	}
	if !strings.Contains(body, "기본 정보와 AS/정기점검") {
		t.Fatal("유지보수 등록 안내 문구가 빠졌다")
	}
	if !strings.Contains(body, "범위 규칙") {
		t.Fatal("유지보수 등록에서 범위 규칙이 빠졌다")
	}
}

func TestSalesProjectProspectAndWonGuard(t *testing.T) {
	e, repo, _, db := newProjectServer(t)
	users, err := repository.NewUserRepo(db).ListAssignable()
	if err != nil || len(users) == 0 {
		t.Fatalf("users: %v n=%d", err, len(users))
	}
	owner := users[0].UserID

	newSales := doGet(t, e, "/projects/new?kind=build")
	if newSales.Code != http.StatusOK || !strings.Contains(newSales.Body.String(), "가등록 기관명") {
		t.Fatalf("영업 등록 화면 없음 status=%d", newSales.Code)
	}

	rec := doForm(t, e, "/projects", url.Values{
		"name":                {"충남 과업심의"},
		"project_kind":        {model.ProjectKindBuild},
		"sales_stage":         {model.SalesStageLead},
		"expected_ym":         {"2027-03"},
		"expected_precision":  {model.ExpectedPrecisionQuarter},
		"sales_owner_id":      {owner},
		"prospect_name":       {"충청남도교육청"},
		"prospect_contact_name": {"김담당"},
		"status":              {"active"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("가등록 저장 status=%d body=%s", rec.Code, rec.Body.String())
	}
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	got, err := repo.Get(id)
	if err != nil || got.CustomerID != "" || got.ProspectName != "충청남도교육청" {
		t.Fatalf("가등록 저장 결과: %+v err=%v", got, err)
	}
	if got.ExpectedYM != "2027-03" {
		t.Fatalf("expected_ym=%q", got.ExpectedYM)
	}
	show := doGet(t, e, "/projects/"+id)
	if !strings.Contains(show.Body.String(), "2027년 1분기") {
		t.Fatal("정밀도 표시가 2027년 1분기가 아니다")
	}
	if !strings.Contains(show.Body.String(), "가등록") {
		t.Fatal("가등록 뱃지가 없다")
	}
	if !strings.Contains(show.Body.String(), "고객으로 등록") {
		t.Fatal("고객으로 등록 버튼이 없다")
	}

	list := doGet(t, e, "/projects")
	if strings.Contains(list.Body.String(), "충남 과업심의") {
		t.Fatal("기본 유지보수 목록에 영업 건이 보인다")
	}
	salesList := doGet(t, e, "/projects?kind=sales")
	if !strings.Contains(salesList.Body.String(), "충남") && !strings.Contains(salesList.Body.String(), "충청남도교육청") {
		t.Fatalf("영업 목록에 없음: %s", salesList.Body.String()[:min(400, len(salesList.Body.String()))])
	}

	rec = doForm(t, e, "/projects/"+id, url.Values{
		"name":           {"충남 과업심의"},
		"project_kind":   {model.ProjectKindBuild},
		"sales_stage":    {model.SalesStageWon},
		"expected_ym":    {"2027-03"},
		"sales_owner_id": {owner},
		"prospect_name":  {"충청남도교육청"},
		"status":         {"active"},
	})
	if rec.Code == http.StatusSeeOther {
		t.Fatal("won 을 고객 없이 저장했다")
	}
	if !strings.Contains(rec.Body.String(), "고객을 등록") && !strings.Contains(rec.Body.String(), "계약기간") {
		body := rec.Body.String()
		if len(body) > 500 {
			body = body[:500]
		}
		t.Fatalf("won 가드 메시지 없음: %s", body)
	}

	promo := doForm(t, e, "/projects/"+id+"/promote-customer", url.Values{})
	if promo.Code != http.StatusSeeOther || !strings.Contains(promo.Header().Get("Location"), "ok=promoted") {
		t.Fatalf("승격: status=%d loc=%q", promo.Code, promo.Header().Get("Location"))
	}
	got, err = repo.Get(id)
	if err != nil || got.CustomerID == "" || got.ProspectName != "충청남도교육청" {
		t.Fatalf("승격 후 prospect 유지: %+v err=%v", got, err)
	}

	rec = doForm(t, e, "/projects/"+id, url.Values{
		"name":           {"충남 과업심의"},
		"project_kind":   {model.ProjectKindBuild},
		"sales_stage":    {model.SalesStageWon},
		"expected_ym":    {"2027-03"},
		"sales_owner_id": {owner},
		"customer_id":    {got.CustomerID},
		"prospect_name":  {got.ProspectName},
		"start_date":     {"2027-01-01"},
		"end_date":       {"2027-12-31"},
		"contract_type":  {"private"},
		"is_paid":        {"1"},
		"status":         {"active"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("won 저장 status=%d", rec.Code)
	}
}

func TestLostSalesHiddenFromDefaultList(t *testing.T) {
	e, repo, _, db := newProjectServer(t)
	users, _ := repository.NewUserRepo(db).ListAssignable()
	p := &model.WorkProject{
		Name: "실주건", ProjectKind: model.ProjectKindBuild, SalesStage: model.SalesStageLost,
		ExpectedYM: "2027-04", SalesOwner: "관리자", SalesOwnerID: users[0].UserID,
		ProspectName: "실주기관", Status: model.WBProjectActive, Color: "#3B82F6",
	}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	hidden := doGet(t, e, "/projects?kind=sales")
	if strings.Contains(hidden.Body.String(), "실주건") {
		t.Fatal("기본 영업 목록에 실주가 보인다")
	}
	shown := doGet(t, e, "/projects?kind=sales&lost=1")
	if !strings.Contains(shown.Body.String(), "실주건") {
		t.Fatal("lost=1 에도 실주가 안 보인다")
	}
}

func TestSalesTabsFollowupAndUnplanned(t *testing.T) {
	e, repo, wb, db := newProjectServer(t)
	users, err := repository.NewUserRepo(db).ListAssignable()
	if err != nil || len(users) == 0 {
		t.Fatal(err)
	}
	home := doGet(t, e, "/projects")
	if home.Code != http.StatusOK {
		t.Fatalf("status=%d", home.Code)
	}
	body := home.Body.String()
	if !strings.Contains(body, "영업(수주 전)") || !strings.Contains(body, "진행") || !strings.Contains(body, "종료") {
		t.Fatal("탭이 없다")
	}

	rec := doForm(t, e, "/projects", url.Values{
		"name":           {"타임라인건"},
		"project_kind":   {model.ProjectKindBuild},
		"sales_stage":    {model.SalesStageLead},
		"expected_ym":    {"2027-03"},
		"sales_owner_id": {users[0].UserID},
		"prospect_name":  {"타임라인기관"},
		"status":         {"active"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 status=%d", rec.Code)
	}
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")

	tl := doGet(t, e, "/projects?tab=sales")
	if !strings.Contains(tl.Body.String(), "예정월 타임라인") || !strings.Contains(tl.Body.String(), "타임라인건") {
		t.Fatal("타임라인에 영업 건이 없다")
	}
	if !strings.Contains(tl.Body.String(), "이번 달") || !strings.Contains(tl.Body.String(), "다음 달") {
		t.Fatal("이번 달·다음 달 표시가 없다")
	}
	kb := doGet(t, e, "/projects?tab=sales&view=kanban")
	if !strings.Contains(kb.Body.String(), "발굴") || !strings.Contains(kb.Body.String(), "타임라인건") {
		t.Fatal("칸반에 없다")
	}

	board := repository.NewWorkBoardRepo(db)
	items, counts, err := board.ListUnplanned("", nil, model.UnplannedSalesFollowup)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range items {
		if it.RefID == id {
			found = true
		}
	}
	if !found || counts.SalesFollowup < 1 {
		t.Fatalf("영업 후속없음 없음 n=%d counts=%+v", len(items), counts)
	}

	fu := doForm(t, e, "/projects/"+id+"/follow-up", url.Values{
		"followup_title": {"과업심의 자료"},
		"followup_due":   {"2026-09-01"},
	})
	if fu.Code != http.StatusSeeOther {
		t.Fatalf("다음 행동 status=%d body=%s", fu.Code, fu.Body.String())
	}
	task, err := wb.GetTaskBySource(model.WBSourceProject, id)
	if err != nil || task == nil || task.SourceType != model.WBSourceProject {
		t.Fatalf("source_type=project 업무 없음: %+v err=%v", task, err)
	}
	items2, _, err := board.ListUnplanned("", nil, model.UnplannedSalesFollowup)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items2 {
		if it.RefID == id {
			t.Fatal("다음 행동을 붙였는데 후속없음에 남았다")
		}
	}

	st := doForm(t, e, "/projects/"+id+"/stage", url.Values{
		"sales_stage": {model.SalesStageProposal},
		"back":        {"/projects?tab=sales&view=kanban"},
	})
	if st.Code != http.StatusSeeOther {
		t.Fatalf("단계 이동 status=%d", st.Code)
	}
	got, _ := repo.Get(id)
	if got.SalesStage != model.SalesStageProposal {
		t.Fatalf("stage=%q", got.SalesStage)
	}

	over := &model.WorkProject{
		Name: "지난예정", ProjectKind: model.ProjectKindSupply, SalesStage: model.SalesStageQuote,
		ExpectedYM: "2020-01", SalesOwner: "관리자", ProspectName: "지난기관",
		Status: model.WBProjectActive, Color: "#3B82F6",
	}
	if err := repo.Create(over); err != nil {
		t.Fatal(err)
	}
	rev, _, err := board.ListUnplanned("", nil, model.UnplannedReview)
	if err != nil {
		t.Fatal(err)
	}
	foundRev := false
	for _, it := range rev {
		if it.RefID == over.ProjectID {
			foundRev = true
		}
	}
	if !foundRev {
		t.Fatal("예정월이 지난 영업 건이 재검토에 없다")
	}

	won := doForm(t, e, "/projects/"+id+"/stage", url.Values{
		"sales_stage": {model.SalesStageWon},
	})
	if won.Code == http.StatusSeeOther && strings.Contains(won.Header().Get("Location"), "view=kanban") && !strings.Contains(won.Header().Get("Location"), "edit") {
		t.Fatal("칸반에서 고객 없이 won 으로 옮겼다")
	}
}

