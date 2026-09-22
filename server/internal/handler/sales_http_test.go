package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newSalesServer(t *testing.T) *echo.Echo {
	e, _ := newSalesServerDB(t)
	return e
}

func newSalesServerDB(t *testing.T) (*echo.Echo, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "sales_http.db"))
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
	g.GET("/sales", h.Sales.List)
	g.GET("/sales/dashboard", h.Sales.Dashboard)
	g.GET("/sales/new", h.Sales.New)
	g.POST("/sales", h.Sales.Create)
	g.GET("/sales/activities", h.Sales.Activities)
	g.POST("/sales/activities", h.Sales.CreateActivity)
	g.POST("/sales/activities/:aid/move", h.Sales.MoveActivity)
	g.POST("/sales/activities/:aid/delete", h.Sales.DeleteActivity)
	g.POST("/sales/activities/:aid", h.Sales.UpdateActivity)
	g.POST("/sales/from-task", h.Sales.FromTask)
	g.GET("/sales/pipeline", h.Sales.Pipeline)
	g.GET("/sales/:id/promote", h.Sales.PromoteForm)
	g.POST("/sales/:id/promote", h.Sales.PromoteSave)
	g.POST("/sales/:id/promote/customer", h.Sales.PromoteCustomer)
	g.GET("/sales/:id/party-hints", h.Sales.PartyHints)
	g.GET("/sales/:id", h.Sales.Show)
	g.GET("/sales/:id/edit", h.Sales.Edit)
	g.POST("/sales/:id", h.Sales.Update)
	g.POST("/sales/:id/stage", h.Sales.ChangeStage)
	g.POST("/sales/:id/bid-result", h.Sales.SetBidResult)
	g.POST("/sales/:id/negotiate", h.Sales.StartNegotiation)
	g.POST("/sales/:id/close-contract", h.Sales.CloseContracted)
	g.POST("/sales/:id/close-failed", h.Sales.CloseNegotiationFailed)
	g.POST("/sales/:id/win-prob", h.Sales.SetWinProb)
	g.POST("/sales/:id/rfp", h.Sales.SetRFPReceived)
	g.POST("/sales/:id/migrated-checked", h.Sales.MarkMigratedChecked)
	g.GET("/sales/:id/drop.json", h.Sales.DropForm)
	g.POST("/sales/:id/drop", h.Sales.Drop)
	g.POST("/sales/:id/activities", h.Sales.CreateActivity)
	g.POST("/sales/:id/parties", h.Sales.CreateParty)
	g.POST("/sales/:id/parties/:pid/replace", h.Sales.ReplaceParty)
	g.POST("/sales/:id/parties/:pid/link", h.Sales.LinkParty)
	g.POST("/sales/:id/memos", h.Sales.CreateMemo)
	g.POST("/sales/:id/memos/:mid/delete", h.Sales.DeleteMemo)
	g.GET("/workboard/register", h.Workboard.Register)
	g.GET("/projects/new", h.Project.New)
	g.GET("/projects/:id", h.Project.Show)
	g.GET("/contracts", h.Project.Contracts)
	g.GET("/customers", h.Customer.List)
	g.GET("/items", h.Items.List)
	g.GET("/items/new", h.Items.New)
	g.POST("/items", h.Items.Create)
	g.GET("/items/suggest", h.Items.Suggest)
	g.POST("/items/quote-line", h.Items.QuoteLine)
	g.GET("/items/:id/edit", h.Items.Edit)
	g.POST("/items/:id/kind", h.Items.SetKind)
	g.POST("/items/:id/confirm", h.Items.Confirm)
	g.POST("/items/:id", h.Items.Update)
	return e, db
}

func salesIDFromRedirect(t *testing.T, loc string) string {
	t.Helper()
	path := strings.Split(loc, "?")[0]
	id := strings.TrimPrefix(path, "/sales/")
	if id == "" || strings.Contains(id, "/") {
		t.Fatalf("sales id 없음: %q", loc)
	}
	return id
}

func TestSalesHTTP_NameOnlyStageOverrideWon(t *testing.T) {
	e := newSalesServer(t)

	rec := doForm(t, e, "/sales", url.Values{
		"name":              {"세종시 도서관 RFID 증설(가칭)"},
		"is_tentative_name": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("사업명만 저장 실패 status=%d body=%s", rec.Code, rec.Body.String())
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))

	show := doGet(t, e, "/sales/"+id)
	if show.Code != http.StatusOK {
		t.Fatalf("상세 status=%d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, "발굴") || !strings.Contains(body, "10%") {
		t.Fatalf("기본 단계·확도 미표시: %s", body[0:min(400, len(body))])
	}
	if !strings.Contains(body, "정보 확정도 0/4") {
		t.Fatalf("확정도 미표시")
	}
	if !strings.Contains(body, "영업 단계") || !strings.Contains(body, "pickStage(") {
		t.Fatal("단계 pill 이 없다")
	}
	if strings.Contains(body, "삭제") && strings.Contains(body, "/sales/"+id+"/delete") {
		t.Fatal("삭제 버튼이 남았다")
	}
	if !strings.Contains(body, "사업 정보") || !strings.Contains(body, ">확률<") {
		t.Fatal("사업 정보·확률 행이 없다")
	}

	rec = doForm(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"propose"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=stage") {
		t.Fatalf("단계 변경: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	show = doGet(t, e, "/sales/"+id)
	body = show.Body.String()
	if !strings.Contains(body, "제안") || !strings.Contains(body, "20%") {
		t.Fatalf("단계 변경 후 확도 미반영: %s", body[0:min(500, len(body))])
	}
	if strings.Contains(body, "수동 조정") {
		t.Fatal("수동 조정 UI가 남았다")
	}

	rec = doForm(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"discover"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("후퇴 사유 없음: loc=%q", rec.Header().Get("Location"))
	}
	rec = doForm(t, e, "/sales/"+id+"/stage", url.Values{
		"stage":  {"discover"},
		"reason": {"내년으로 이연"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=stage") {
		t.Fatalf("후퇴 실패: loc=%q", rec.Header().Get("Location"))
	}
	show = doGet(t, e, "/sales/"+id)
	if !strings.Contains(show.Body.String(), "내년으로 이연") {
		t.Fatal("후퇴 사유가 이력에 없다")
	}

	rec = doForm(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"direct_won"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=stage") {
		t.Fatalf("바로 수주 실패: loc=%q", rec.Header().Get("Location"))
	}
	show = doGet(t, e, "/sales/"+id)
	body = show.Body.String()
	if !strings.Contains(body, "수주") {
		t.Fatal("수주 상태가 없다")
	}

	del := doForm(t, e, "/sales/"+id+"/delete", url.Values{})
	if del.Code != http.StatusNotFound && del.Code != http.StatusMethodNotAllowed {
		t.Fatalf("삭제 라우트 status=%d", del.Code)
	}

	list := doGet(t, e, "/sales")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "영업") {
		t.Fatalf("목록 status=%d", list.Code)
	}
	act := doGet(t, e, "/sales/activities")
	if act.Code != http.StatusOK || !strings.Contains(act.Body.String(), "영업 활동") {
		t.Fatalf("활동 화면: %d", act.Code)
	}
}

func TestSalesHTTP_ProjectFormUnchanged(t *testing.T) {
	e := newSalesServer(t)
	rec := doGet(t, e, "/projects/new")
	if rec.Code != http.StatusOK {
		t.Fatalf("사업 등록 status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "유상/무상") || !strings.Contains(body, "계약방식") {
		t.Fatal("유지보수 사업 등록 화면이 바뀌었다")
	}
	if strings.Contains(body, "임시 사업명") {
		t.Fatal("사업 등록에 영업 필드가 섞였다")
	}
}

func TestSalesHTTP_TechCannotWrite(t *testing.T) {
	e := newSalesServer(t)
	rec := httptestPostAs(t, e, "/sales", url.Values{"name": {"기술은 조회만"}}, "tech")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("기술 등록 status=%d", rec.Code)
	}
}

func httptestPostAs(t *testing.T, e *echo.Echo, path string, form url.Values, role string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieRole(t, role))
	e.ServeHTTP(rec, req)
	return rec
}

func TestSalesHTTP_ActivityAppearsOnRegisterAndEmptySalesColumn(t *testing.T) {
	e, db := newSalesServerDB(t)
	if _, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active)
		VALUES ('u-sales1','hyeyoung','x','최혜영','sales',1), ('u-sales2','tae','x','태자운','sales',1)`); err != nil {
		t.Fatal(err)
	}

	rec := doForm(t, e, "/sales", url.Values{
		"name": {"세종 RFID 증설"}, "is_tentative_name": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("사업 등록 status=%d", rec.Code)
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))

	rec = doForm(t, e, "/sales/"+id+"/activities", url.Values{
		"activity_date": {"2026-08-19"},
		"start_time":    {"10:00"},
		"duration_min":  {"60"},
		"activity_type": {"visit"},
		"title":         {"방문미팅 · 세종 RFID"},
		"our_members":   {"최혜영"},
		"next_action":   {"견적 요청"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=activity") {
		t.Fatalf("활동 등록: status=%d loc=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}

	show := doGet(t, e, "/sales/"+id)
	if show.Code != http.StatusOK || !strings.Contains(show.Body.String(), "방문미팅") {
		t.Fatalf("상세 타임라인에 활동 없음: %d", show.Code)
	}

	var n, emptyDate, emptyAssignee int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type='sales_activity'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks
		WHERE source_type='sales_activity' AND TRIM(COALESCE(work_date,''))=''`).Scan(&emptyDate); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks
		WHERE source_type='sales_activity' AND TRIM(COALESCE(assignee,''))=''`).Scan(&emptyAssignee); err != nil {
		t.Fatal(err)
	}
	unplaced, err := repository.NewWBRepo(db).ListUnplacedAdminTasks()
	if err != nil {
		t.Fatal(err)
	}
	paletteSales := 0
	for _, tk := range unplaced {
		if tk.SourceType == model.WBSourceSalesActivity {
			paletteSales++
		}
	}
	t.Logf("§39.6 (1) source_type=sales_activity COUNT=%d", n)
	t.Logf("§39.6 (2) empty work_date COUNT=%d", emptyDate)
	t.Logf("§39.6 (3) empty assignee COUNT=%d", emptyAssignee)
	t.Logf("§39.6 (4) WBCategory(sales)=%q label=%q; WBCategory(sales_activity)=%q label=%q",
		model.WBCategory(model.WorkPrefixSales), model.WBCategoryLabel(model.WBCategory(model.WorkPrefixSales)),
		model.WBCategory(model.WBSourceSalesActivity), model.WBCategoryLabel(model.WBCategory(model.WBSourceSalesActivity)))
	t.Logf("§39.6 (5) ListUnplacedAdminTasks sales_activity COUNT=%d (source_type!='' 제외)", paletteSales)

	if n == 0 {
		t.Fatal("(1) work_tasks 에 sales_activity 가 없다 — syncWorkTask 가 안 불린다")
	}
	if emptyDate != 0 {
		t.Fatalf("(2) work_date 빈 행 %d건 — 일정표에 자리가 없다", emptyDate)
	}
	if emptyAssignee != 0 {
		t.Fatalf("(3) assignee 빈 행 %d건 — 담당자 열에 안 들어간다", emptyAssignee)
	}
	if model.WBCategoryLabel(model.WBCategory(model.WorkPrefixSales)) != "영업" {
		t.Fatalf("(4) WBCategory(sales) 가 행정으로 떨어진다: %q", model.WBCategory(model.WorkPrefixSales))
	}
	if paletteSales != 0 {
		t.Fatalf("(5) 팔레트에 sales_activity 가 %d건 있다 — 일정표가 아니라 대기 목록 쪽", paletteSales)
	}

	reg := doGet(t, e, "/workboard/register?view=day&date=2026-08-19")
	if reg.Code != http.StatusOK {
		t.Fatalf("일정표 status=%d", reg.Code)
	}
	regBody := reg.Body.String()
	if !strings.Contains(regBody, "방문미팅 · 세종 RFID") {
		t.Fatalf("일일 업무 등록 일정표에 활동이 없다: %s", clipBody(regBody))
	}
	if !strings.Contains(regBody, "[영업]") {
		t.Fatalf("일정표에 「영업」 라벨이 없다(행정으로 보인다): %s", clipBody(regBody))
	}
	if !strings.Contains(regBody, `data-assignee="최혜영"`) {
		t.Fatalf("담당자 열(최혜영)에 안 들어갔다: %s", clipBody(regBody))
	}

	act := doGet(t, e, "/sales/activities?date=2026-08-19")
	if act.Code != http.StatusOK {
		t.Fatalf("영업 활동 status=%d", act.Code)
	}
	body := act.Body.String()
	if !strings.Contains(body, "최혜영") || !strings.Contains(body, "태자운") {
		t.Fatalf("영업 담당자 열이 없다: %s", clipBody(body))
	}
	if !strings.Contains(body, "활동 없음") {
		t.Fatalf("빈 열이 '활동 없음'을 보여주지 않는다: %s", clipBody(body))
	}
}

func TestSalesHTTP_FromTaskLinksExistingWork(t *testing.T) {
	e, db := newSalesServerDB(t)
	wb := repository.NewWBRepo(db)
	task := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "현장 미팅 준비",
		WorkDate: "2026-08-19", DueDate: "2026-08-19",
		StartTime: "10:00", EndTime: "11:00", DurationMin: 60,
		Status: model.WBTaskWaiting, Assignee: "최혜영",
	}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}

	rec := doForm(t, e, "/sales", url.Values{"name": {"세종 RFID 증설"}, "is_tentative_name": {"1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("사업 등록 status=%d", rec.Code)
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))

	list := doGet(t, e, "/sales?from_task="+task.TaskID)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "영업 활동으로 만들 사업") {
		t.Fatalf("사업 고르기 안내 없음 status=%d", list.Code)
	}

	show := doGet(t, e, "/sales/"+id+"?tab=timeline&from_task="+task.TaskID)
	if show.Code != http.StatusOK || !strings.Contains(show.Body.String(), `name="from_task"`) {
		t.Fatalf("활동 등록에 from_task 없음 status=%d", show.Code)
	}

	rec = doForm(t, e, "/sales/"+id+"/activities", url.Values{
		"from_task":     {task.TaskID},
		"activity_date": {"2026-08-19"},
		"start_time":    {"10:00"},
		"duration_min":  {"60"},
		"activity_type": {"visit"},
		"title":         {task.Title},
		"our_members":   {"최혜영"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=activity") {
		t.Fatalf("연결 활동 등록 loc=%q", rec.Header().Get("Location"))
	}
	got, _ := wb.GetTask(task.TaskID)
	if got == nil || got.SourceType != model.WBSourceSalesActivity || got.SourceID == "" {
		t.Fatalf("업무가 sales_activities에 연결되지 않음 %+v", got)
	}
}

func TestSalesHTTP_ChangeHistoryAndPartyReplace(t *testing.T) {
	e := newSalesServer(t)

	rec := doForm(t, e, "/sales", url.Values{
		"name":              {"세종 RFID 증설"},
		"is_tentative_name": {"1"},
		"expected_amount":   {"30000000"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("사업 등록 status=%d", rec.Code)
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))

	show := doGet(t, e, "/sales/"+id)
	if show.Code != http.StatusOK {
		t.Fatalf("상세 status=%d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, "활동 로그") || !strings.Contains(body, "관계자") {
		t.Fatalf("상세 탭 없음: %s", clipBody(body))
	}
	if strings.Contains(body, `@click="tab='changes'"`) {
		t.Fatal("변경 이력을 별 탭으로 나눴다")
	}

	rec = doForm(t, e, "/sales/"+id, url.Values{
		"name":              {"세종 RFID 증설"},
		"is_tentative_name": {"1"},
		"expected_amount":   {"180000000"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("금액 수정 status=%d", rec.Code)
	}
	chg := doGet(t, e, "/sales/"+id+"?tab=timeline")
	if chg.Code != http.StatusOK {
		t.Fatalf("활동 로그 status=%d", chg.Code)
	}
	chgBody := chg.Body.String()
	if !strings.Contains(chgBody, "30,000,000") || !strings.Contains(chgBody, "180,000,000") {
		t.Fatalf("금액 이전 값이 활동 로그에 없다: %s", clipBody(chgBody))
	}

	rec = doForm(t, e, "/sales/"+id+"/parties", url.Values{
		"party_type":  {"customer"},
		"org_name":    {"세종시립도서관"},
		"person_name": {"김담당"},
		"party_role":  {"working"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=party") {
		t.Fatalf("관계자 추가: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}

	parties := doGet(t, e, "/sales/"+id+"?tab=parties")
	pid := partyIDFromBody(t, parties.Body.String())
	rec = doForm(t, e, "/sales/"+id+"/parties/"+pid+"/replace", url.Values{
		"party_type":      {"customer"},
		"org_name":        {"세종시립도서관"},
		"person_name":     {"이후임"},
		"party_role":      {"working"},
		"replaced_reason": {"전배"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=replaced") {
		t.Fatalf("교체: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}

	after := doGet(t, e, "/sales/"+id+"?tab=parties")
	afterBody := after.Body.String()
	if !strings.Contains(afterBody, "김담당") || !strings.Contains(afterBody, "이후임") {
		t.Fatalf("교체 후 옛 이름이 없다: %s", clipBody(afterBody))
	}
	if !strings.Contains(afterBody, "비활성") {
		t.Fatal("옛 관계자가 비활성으로 표시되지 않는다")
	}

	del := doForm(t, e, "/sales/"+id+"/parties/"+pid+"/delete", url.Values{})
	if del.Code == http.StatusOK || del.Code == http.StatusSeeOther {
		t.Fatalf("관계자 삭제 경로가 있다 status=%d", del.Code)
	}
}

func TestSalesHTTP_KanbanTimelinePipelineAndActivityBoard(t *testing.T) {
	e := newSalesServer(t)

	rec := doForm(t, e, "/sales", url.Values{
		"name":              {"세종 RFID 증설"},
		"is_tentative_name": {"1"},
		"expected_amount":   {"30000000"},
		"expected_ym":       {"2026-09"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("등록 status=%d", rec.Code)
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))

	list := doGet(t, e, "/sales")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "단계 칸반") || !strings.Contains(list.Body.String(), "예정월 타임라인") {
		t.Fatalf("보기 전환 없음: %s", clipBody(list.Body.String()))
	}

	kanban := doGet(t, e, "/sales?view=kanban")
	if kanban.Code != http.StatusOK {
		t.Fatalf("칸반 status=%d", kanban.Code)
	}
	kb := kanban.Body.String()
	if !strings.Contains(kb, "발굴") || !strings.Contains(kb, "제안") ||
		!strings.Contains(kb, "입찰") || !strings.Contains(kb, "사업 종료") {
		t.Fatalf("4단계 열이 없다: %s", clipBody(kb))
	}
	if strings.Contains(kb, "제안서 제출") || strings.Contains(kb, "계약완료") {
		t.Fatal("폐 8단계 열이 남았다")
	}
	if !strings.Contains(kb, "data-collapsed=\"1\"") {
		t.Fatal("실주 열이 기본 접힘이 아니다")
	}
	if !strings.Contains(kb, "min-w-[260px]") {
		t.Fatal("칸반 최소 폭 260px 가 없다")
	}

	tl := doGet(t, e, "/sales?view=timeline")
	if tl.Code != http.StatusOK || !strings.Contains(tl.Body.String(), "2026-09") {
		t.Fatalf("예정월 타임라인: %s", clipBody(tl.Body.String()))
	}

	rec = doForm(t, e, "/sales/"+id+"/stage", url.Values{
		"stage":  {"proposal"},
		"return": {"/sales?view=kanban"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "view=kanban") {
		t.Fatalf("칸반 단계 이동 복귀: loc=%q", rec.Header().Get("Location"))
	}

	pipe := doGet(t, e, "/sales/pipeline")
	if pipe.Code != http.StatusMovedPermanently {
		t.Fatalf("파이프라인 리다이렉트 status=%d", pipe.Code)
	}
	if loc := pipe.Header().Get("Location"); !strings.Contains(loc, "view=kanban") {
		t.Fatalf("파이프라인 Location=%q", loc)
	}

	rec = doForm(t, e, "/sales/"+id+"/activities", url.Values{
		"activity_date": {"2026-08-19"},
		"start_time":    {"10:00"},
		"duration_min":  {"30"},
		"activity_type": {"visit"},
		"title":         {"방문미팅 · 칸반"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("활동 등록 status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}

	actBoard := doGet(t, e, "/sales/activities?view=kanban&group=type")
	if actBoard.Code != http.StatusOK {
		t.Fatalf("활동 칸반 status=%d", actBoard.Code)
	}
	ab := actBoard.Body.String()
	if !strings.Contains(ab, "방문미팅") || !strings.Contains(ab, "view=kanban&group=type") {
		t.Fatalf("활동 유형 칸반 없음: %s", clipBody(ab))
	}
	if !strings.Contains(ab, "방문미팅 · 칸반") {
		t.Fatal("활동 카드가 칸반에 없다")
	}

	showKanban := doGet(t, e, "/sales/"+id+"?tab=timeline&actview=kanban&group=type")
	if showKanban.Code != http.StatusOK || !strings.Contains(showKanban.Body.String(), "칸반 · 유형") {
		t.Fatal("상세 활동 칸반이 없다")
	}

	aid := activityIDFromBody(t, ab)
	rec = doForm(t, e, "/sales/activities/"+aid+"/move", url.Values{
		"group":  {"type"},
		"value":  {"call"},
		"return": {"/sales/activities?view=kanban&group=type"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("활동 이동 status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	moved := doGet(t, e, "/sales/activities?view=kanban&group=type")
	if !strings.Contains(moved.Body.String(), "방문미팅 · 칸반") {
		t.Fatal("유형을 옮긴 카드가 사라졌다")
	}
}

func partyIDFromBody(t *testing.T, body string) string {
	t.Helper()
	const marker = "/parties/PT-"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("party id 없음: %s", clipBody(body))
	}
	rest := body[i+len("/parties/"):]
	end := strings.Index(rest, "/replace")
	if end < 0 {
		t.Fatalf("replace 경로 없음: %s", clipBody(rest))
	}
	id := rest[:end]
	if !strings.HasPrefix(id, "PT-") {
		t.Fatalf("party id 형식: %q", id)
	}
	return id
}

func activityIDFromBody(t *testing.T, body string) string {
	t.Helper()
	const marker = "dragAct($event, '"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("activity id 없음: %s", clipBody(body))
	}
	rest := body[i+len(marker):]
	end := strings.Index(rest, "'")
	if end < 0 {
		t.Fatalf("activity id 끝 없음")
	}
	id := rest[:end]
	if !strings.HasPrefix(id, "SA-") {
		t.Fatalf("activity id 형식: %q", id)
	}
	return id
}

func clipBody(s string) string {
	if len(s) > 600 {
		return s[:600]
	}
	return s
}

func httptestGetAs(t *testing.T, e *echo.Echo, path, role string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(jwtCookieRole(t, role))
	e.ServeHTTP(rec, req)
	return rec
}

func createContractedSales(t *testing.T, e *echo.Echo, vals url.Values) string {
	t.Helper()
	if vals.Get("name") == "" {
		vals.Set("name", "세종시 도서관 RFID")
	}
	vals.Set("customer_confirmed", "1")
	vals.Set("expected_ym_confirmed", "1")
	vals.Set("expected_amount_confirmed", "1")
	if vals.Get("expected_ym") == "" {
		vals.Set("expected_ym", "2027-01")
	}
	if vals.Get("expected_amount") == "" {
		vals.Set("expected_amount", "30000000")
	}
	rec := doForm(t, e, "/sales", vals)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("영업 등록 status=%d body=%s", rec.Code, rec.Body.String())
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	rec = doForm(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"direct_won"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=stage") {
		t.Fatalf("바로 수주: loc=%q", rec.Header().Get("Location"))
	}
	rec = doForm(t, e, "/sales/"+id+"/close-contract", url.Values{
		"contracted_at":    {"2026-09-01"},
		"contract_amount":  {"30000000"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("계약 종료: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	return id
}

func TestSalesHTTP_PromoteOnlyWhenContracted(t *testing.T) {
	e := newSalesServer(t)
	rec := doForm(t, e, "/sales", url.Values{"name": {"아직 계약 전"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("등록 status=%d", rec.Code)
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	show := doGet(t, e, "/sales/"+id)
	if strings.Contains(show.Body.String(), "사업관리로 등록") {
		t.Fatal("수주 전에 승격 버튼이 열렸다")
	}
	rec = doGet(t, e, "/sales/"+id+"/promote")
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=not_contracted") {
		t.Fatalf("미계약 승격: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestSalesHTTP_PromoteProspectThenKeepActivities(t *testing.T) {
	e, db := newSalesServerDB(t)
	id := createContractedSales(t, e, url.Values{
		"name":          {"세종시 도서관 RFID 증설"},
		"prospect_name": {"세종시립도서관"},
	})
	rec := doForm(t, e, "/sales/"+id+"/activities", url.Values{
		"activity_date": {"2026-08-19"},
		"activity_type": {"visit"},
		"title":         {"방문미팅 · 승격 전"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("활동 등록 status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}

	show := doGet(t, e, "/sales/"+id)
	if !strings.Contains(show.Body.String(), "사업관리로 등록") {
		t.Fatal("수주인데 승격 버튼이 없다")
	}

	form := doGet(t, e, "/sales/"+id+"/promote")
	if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), "고객 마스터에 등록") {
		t.Fatalf("미등록 고객 단계가 아니다: %d %s", form.Code, clipBody(form.Body.String()))
	}

	rec = doForm(t, e, "/sales/"+id+"/promote/customer", url.Values{
		"org_name":      {"세종시립도서관"},
		"official_name": {"세종특별자치시립도서관"},
		"industry":      {"도서관"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/promote") {
		t.Fatalf("고객 등록: loc=%q", rec.Header().Get("Location"))
	}

	var custID string
	if err := db.QueryRow(`SELECT customer_id FROM sales_projects WHERE sales_id=?`, id).Scan(&custID); err != nil || custID == "" {
		t.Fatalf("sales.customer_id 없음: %v %q", err, custID)
	}

	projForm := doGet(t, e, "/sales/"+id+"/promote")
	body := projForm.Body.String()
	if projForm.Code != http.StatusOK || !strings.Contains(body, "유상/무상") || !strings.Contains(body, "계약방식") {
		t.Fatalf("§22.1 폼이 아니다: %d %s", projForm.Code, clipBody(body))
	}
	if strings.Contains(body, "임시 사업명") {
		t.Fatal("승격 폼에 영업 전용 필드가 섞였다")
	}

	rec = doForm(t, e, "/sales/"+id+"/promote", url.Values{
		"name":          {"세종시 도서관 RFID 증설"},
		"customer_id":   {custID},
		"plan_year":     {"2027"},
		"is_paid":       {"1"},
		"contract_type": {"private"},
		"billing_type":  {"yearly"},
		"start_date":    {"2027-01-01"},
		"end_date":      {"2027-12-31"},
	})
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "/projects/WP-") || !strings.Contains(loc, "ok=promoted") {
		t.Fatalf("승격 저장: status=%d loc=%q", rec.Code, loc)
	}

	var wpID, linked, status string
	if err := db.QueryRow(`SELECT project_id, COALESCE(sales_project_id,''), status FROM work_projects WHERE sales_project_id=?`, id).
		Scan(&wpID, &linked, &status); err != nil {
		t.Fatalf("work_projects 없음: %v", err)
	}
	if linked != id {
		t.Fatalf("sales_project_id=%q want %s", linked, id)
	}

	var salesStatus string
	var salesCount int
	if err := db.QueryRow(`SELECT status FROM sales_projects WHERE sales_id=?`, id).Scan(&salesStatus); err != nil {
		t.Fatal(err)
	}
	if salesStatus != "promoted" {
		t.Fatalf("status=%q want promoted", salesStatus)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sales_projects WHERE sales_id=?`, id).Scan(&salesCount); err != nil || salesCount != 1 {
		t.Fatalf("영업 건이 지워졌다 count=%d err=%v", salesCount, err)
	}
	var actCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sales_activities WHERE sales_id=?`, id).Scan(&actCount); err != nil || actCount < 1 {
		t.Fatalf("활동 로그가 없다 count=%d err=%v", actCount, err)
	}

	after := doGet(t, e, "/sales/"+id)
	afterBody := after.Body.String()
	if strings.Contains(afterBody, ">사업관리로 등록<") || strings.Contains(afterBody, "사업관리로 등록</a>") {
		t.Fatal("승격 후에도 등록 버튼이 남아 있다")
	}
	if !strings.Contains(afterBody, "사업관리에서 보기") || !strings.Contains(afterBody, "방문미팅 · 승격 전") {
		t.Fatalf("사업 링크 또는 활동이 없다: %s", clipBody(afterBody))
	}

	listed := doGet(t, e, "/sales?status=promoted")
	if !strings.Contains(listed.Body.String(), "세종시 도서관 RFID 증설") {
		t.Fatal("승격완료 필터에 안 나온다")
	}

	again := doGet(t, e, "/sales/"+id+"/promote")
	if again.Code != http.StatusSeeOther || !strings.Contains(again.Header().Get("Location"), "/projects/"+wpID) {
		t.Fatalf("두 번째 승격: loc=%q", again.Header().Get("Location"))
	}

	unchanged := doGet(t, e, "/projects/new")
	if strings.Contains(unchanged.Body.String(), "임시 사업명") {
		t.Fatal("일반 사업 등록에 영업 필드가 섞였다")
	}
}

func TestSalesHTTP_PromoteRenewalSameCustomer(t *testing.T) {
	e, db := newSalesServerDB(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, main_phone, industry, is_active)
		VALUES ('C041-26-001','충남교육청','충청남도교육청','041-123-4567','공공기관',1)`); err != nil {
		t.Fatal(err)
	}
	id1 := createContractedSales(t, e, url.Values{
		"name":        {"2026년 충남교육청"},
		"customer_id": {"C041-26-001"},
	})
	form1 := doGet(t, e, "/sales/"+id1+"/promote")
	if form1.Code != http.StatusOK || !strings.Contains(form1.Body.String(), "유상/무상") {
		t.Fatalf("기존 고객인데 고객 등록을 다시 물었다: %d", form1.Code)
	}
	if strings.Contains(form1.Body.String(), "고객 마스터에 등록") {
		t.Fatal("재계약 1차에서 고객 마스터 화면이 나왔다")
	}
	rec := doForm(t, e, "/sales/"+id1+"/promote", url.Values{
		"name":        {"2026년 충남교육청"},
		"customer_id": {"C041-26-001"},
		"plan_year":   {"2026"},
		"is_paid":     {"1"},
		"start_date":  {"2026-01-01"},
		"end_date":    {"2026-12-31"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/projects/") {
		t.Fatalf("1차 승격 loc=%q", rec.Header().Get("Location"))
	}

	id2 := createContractedSales(t, e, url.Values{
		"name":        {"2027년 충남교육청"},
		"customer_id": {"C041-26-001"},
		"expected_ym": {"2027-03"},
	})
	form2 := doGet(t, e, "/sales/"+id2+"/promote")
	if form2.Code != http.StatusOK || strings.Contains(form2.Body.String(), "고객 마스터에 등록") {
		t.Fatalf("차년도 재계약에서 고객을 다시 등록하라 한다: %d", form2.Code)
	}
	rec = doForm(t, e, "/sales/"+id2+"/promote", url.Values{
		"name":        {"2027년 충남교육청"},
		"customer_id": {"C041-26-001"},
		"plan_year":   {"2027"},
		"is_paid":     {"1"},
		"start_date":  {"2027-01-01"},
		"end_date":    {"2027-12-31"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("2차 승격 status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE customer_id='C041-26-001'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("재계약 work_projects=%d err=%v", n, err)
	}
	var s1, s2 string
	_ = db.QueryRow(`SELECT sales_project_id FROM work_projects WHERE name='2026년 충남교육청'`).Scan(&s1)
	_ = db.QueryRow(`SELECT sales_project_id FROM work_projects WHERE name='2027년 충남교육청'`).Scan(&s2)
	if s1 != id1 || s2 != id2 {
		t.Fatalf("원본 연결 s1=%s/%s s2=%s/%s", s1, id1, s2, id2)
	}
}

func TestSalesHTTP_TechCannotPromote(t *testing.T) {
	e := newSalesServer(t)
	id := createContractedSales(t, e, url.Values{
		"name":          {"기술은 승격 불가"},
		"prospect_name": {"테스트기관"},
	})
	rec := httptestGetAs(t, e, "/sales/"+id+"/promote", "tech")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("기술 승격 GET status=%d", rec.Code)
	}
	rec = httptestPostAs(t, e, "/sales/"+id+"/promote", url.Values{"name": {"기술은 승격 불가"}}, "tech")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("기술 승격 POST status=%d", rec.Code)
	}
}

func TestSalesHTTP_PipelineKanbanSharedWithList(t *testing.T) {
	e := newSalesServer(t)
	rec := doForm(t, e, "/sales", url.Values{
		"name":              {"세종 RFID 증설"},
		"is_tentative_name": {"1"},
		"expected_amount":   {"30000000"},
		"expected_ym":       {"2026-09"},
		"prospect_name":     {"세종시립도서관"},
		"sales_owner":       {"최혜영"},
	})
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))

	list := doGet(t, e, "/sales?display=kanban")
	pipe := doGet(t, e, "/sales/pipeline")
	if list.Code != http.StatusOK {
		t.Fatalf("status list=%d", list.Code)
	}
	if pipe.Code != http.StatusMovedPermanently {
		t.Fatalf("파이프라인 리다이렉트 status=%d", pipe.Code)
	}
	if loc := pipe.Header().Get("Location"); !strings.Contains(loc, "/sales") || !strings.Contains(loc, "view=kanban") {
		t.Fatalf("파이프라인 Location=%q", loc)
	}
	pb := doGet(t, e, "/sales?view=kanban").Body.String()
	lb := list.Body.String()
	if strings.Count(lb, "flex-1 basis-0") != 4 || strings.Count(pb, "flex-1 basis-0") != 4 {
		t.Fatalf("열 수가 다르다 list=%d pipe=%d", strings.Count(lb, "flex-1 basis-0"), strings.Count(pb, "flex-1 basis-0"))
	}
	if !strings.Contains(lb, "세종 RFID 증설 (가칭)") || !strings.Contains(pb, "세종 RFID 증설 (가칭)") {
		t.Fatal("임시명 (가칭) 이 카드에 없다")
	}
	if !strings.Contains(lb, "0건") {
		t.Fatal("빈 열이 사라졌다")
	}
	if !strings.Contains(lb, "영업담당") || !strings.Contains(pb, "영업담당") ||
		!strings.Contains(lb, "금액 확정") || !strings.Contains(pb, "금액 확정") {
		t.Fatal("리스트와 파이프라인이 필터를 공유하지 않는다")
	}

	lost := doFormJSON(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"closed"}})
	if lost.Code != http.StatusBadRequest {
		t.Fatalf("종료 끌어놓기: %d %s", lost.Code, lost.Body.String())
	}
	won := doFormJSON(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"won"}})
	if won.Code != http.StatusBadRequest {
		t.Fatalf("옛 수주 단계: %d %s", won.Code, won.Body.String())
	}
}

func countSalesCustomerParties(t *testing.T, db *sql.DB, salesID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sales_parties WHERE sales_id=? AND party_type='customer'`, salesID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSalesHTTP_ActivityAutoParty(t *testing.T) {
	e, db := newSalesServerDB(t)

	rec := doForm(t, e, "/sales", url.Values{
		"name":              {"세종 RFID 증설"},
		"prospect_name":     {"세종시립도서관"},
		"is_tentative_name": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("사업 등록 status=%d", rec.Code)
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))

	rec = doForm(t, e, "/sales/"+id+"/activities", url.Values{
		"activity_date": {"2026-08-19"},
		"activity_type": {"visit"},
		"title":         {"담당자 미팅"},
		"counterparts":  {"김철수"},
		"our_members":   {"관리자"},
		"next_action":   {"후속 방문"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=activity") {
		t.Fatalf("활동 저장 loc=%q", rec.Header().Get("Location"))
	}

	parties := doGet(t, e, "/sales/"+id+"?tab=parties")
	body := parties.Body.String()
	if !strings.Contains(body, "김철수") {
		t.Fatalf("관계자 탭에 자동 생성 이름이 없다: %s", clipBody(body))
	}
	if !strings.Contains(body, "보완 필요") {
		t.Fatalf("보완 필요 표시가 없다: %s", clipBody(body))
	}
	if !strings.Contains(body, "당사 참석자") {
		t.Fatalf("당사 참석자 드롭다운이 없다: %s", clipBody(body))
	}
	if n := countSalesCustomerParties(t, db, id); n != 1 {
		t.Fatalf("첫 저장 후 고객 관계자=%d", n)
	}

	rec = doForm(t, e, "/sales/"+id+"/activities", url.Values{
		"activity_date": {"2026-08-20"},
		"activity_type": {"call"},
		"title":         {"같은 사람 전화"},
		"counterparts":  {"김철수"},
		"our_members":   {"관리자"},
		"next_action":   {"메일"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("두 번째 활동 status=%d", rec.Code)
	}
	if n := countSalesCustomerParties(t, db, id); n != 1 {
		t.Fatalf("같은 이름을 또 쳤는데 행이 늘었다 n=%d", n)
	}

	rec = doForm(t, e, "/sales/"+id+"/activities", url.Values{
		"activity_date": {"2026-08-21"},
		"activity_type": {"visit"},
		"title":         {"두 명 미팅"},
		"counterparts":  {"김철수, 이영희"},
		"our_members":   {"관리자"},
		"next_action":   {"제안"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("쉼표 두 명 status=%d", rec.Code)
	}
	if n := countSalesCustomerParties(t, db, id); n != 2 {
		t.Fatalf("김철수, 이영희 후 고객 관계자=%d", n)
	}
	two := doGet(t, e, "/sales/"+id+"?tab=parties")
	if !strings.Contains(two.Body.String(), "이영희") {
		t.Fatalf("이영희가 관계자 탭에 없다: %s", clipBody(two.Body.String()))
	}

	hints := doGet(t, e, "/sales/"+id+"/party-hints")
	if hints.Code != http.StatusOK || !strings.Contains(hints.Body.String(), "김철수") {
		t.Fatalf("자동완성 힌트: %d %s", hints.Code, hints.Body.String())
	}

	pid := partyIDFromBody(t, two.Body.String())
	rec = doForm(t, e, "/sales/"+id+"/parties/"+pid+"/replace", url.Values{
		"party_type":      {"customer"},
		"org_name":        {"세종시립도서관"},
		"person_name":     {"이후임"},
		"party_role":      {"working"},
		"replaced_reason": {"전배"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=replaced") {
		t.Fatalf("자동 생성 관계자 교체 loc=%q", rec.Header().Get("Location"))
	}
	after := doGet(t, e, "/sales/"+id+"?tab=parties")
	afterBody := after.Body.String()
	if !strings.Contains(afterBody, "이후임") {
		t.Fatalf("새 담당자가 없다: %s", clipBody(afterBody))
	}
	if !strings.Contains(afterBody, "비활성") {
		t.Fatal("옛 관계자가 비활성으로 남지 않았다")
	}
	if n := countSalesCustomerParties(t, db, id); n < 2 {
		t.Fatalf("교체 후 옛 행이 사라진 것 같다 n=%d", n)
	}
	var inactive int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sales_parties WHERE sales_id=? AND COALESCE(is_active,1)=0`, id).Scan(&inactive); err != nil || inactive < 1 {
		t.Fatalf("비활성 행 inactive=%d err=%v", inactive, err)
	}
}

func TestSalesHTTP_DashboardAndMemo(t *testing.T) {
	e := newSalesServer(t)
	dash := doGet(t, e, "/sales/dashboard")
	if dash.Code != http.StatusOK {
		t.Fatalf("dashboard status=%d", dash.Code)
	}
	body := dash.Body.String()
	if !strings.Contains(body, "이번 달 수주") || !strings.Contains(body, "가중 파이프라인") || !strings.Contains(body, "오늘 내 업무") {
		t.Fatalf("대시보드 문구 없음: %s", clipBody(body))
	}
	if strings.Contains(body, "이번 달 계약") {
		t.Fatal("이번 달 계약 라벨이 남아 있다")
	}
	rec := doForm(t, e, "/sales", url.Values{"name": {"대시보드 사업"}, "deal_type": {"build"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("등록 status=%d", rec.Code)
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	rec = doForm(t, e, "/sales/"+id+"/memos", url.Values{"content": {"핵심 전략 한 줄"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("메모 status=%d", rec.Code)
	}
	show := doGet(t, e, "/sales/"+id)
	if show.Code != http.StatusOK {
		t.Fatalf("상세 status=%d", show.Code)
	}
	sb := show.Body.String()
	if !strings.Contains(sb, "핵심 전략 한 줄") {
		t.Fatal("메모가 안 보인다")
	}
	if !strings.Contains(sb, "견적 작성") || !strings.Contains(sb, "/quotes/new?sales_id="+id) {
		t.Fatal("견적 작성 링크가 없다")
	}
	pipe := doGet(t, e, "/sales/pipeline")
	if pipe.Code != http.StatusMovedPermanently {
		t.Fatalf("파이프라인 리다이렉트 status=%d", pipe.Code)
	}
	ct := doGet(t, e, "/contracts")
	if ct.Code != http.StatusOK || !strings.Contains(ct.Body.String(), "만료임박") {
		t.Fatalf("계약 목록 status=%d", ct.Code)
	}
}
