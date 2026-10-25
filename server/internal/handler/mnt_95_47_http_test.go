package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newMnt47Server(t *testing.T) (*echo.Echo, *sql.DB, *Handler) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "mnt47.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	a := &model.User{Username: "a47", PasswordHash: "x", FullName: "최혜영", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	b := &model.User{Username: "b47", PasswordHash: "x", FullName: "양기헌", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	c := &model.User{Username: "c47", PasswordHash: "x", FullName: "김서준", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	oa := &model.User{Username: "oa47", PasswordHash: HashPassword("x"), FullName: "도서관장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{a, b, c, oa} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c47','서산고등학교','서산고등학교',1,?),('c47b','타학교','타학교',1,?)`, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status, org_id)
		VALUES ('MP47', 2026, '2026 정기점검', 'approved', ?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/visits/:visit_id/action", h.Maintenance.VisitAction)
	g.POST("/maintenance/visits/:visit_id/action", h.Maintenance.UpdateVisitAction)
	g.GET("/workboard/board", h.Workboard.Board)
	return e, db, h
}

func seedMnt47Visit(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES (?, 'MP47', '2026-10-12', 'c47', 0, 'normal', '최혜영', 'KLAS', 0)`, id); err != nil {
		t.Fatal(err)
	}
}

func doMnt47Form(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	if ck == nil {
		ck = jwtCookie(t)
	}
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func doMnt47Get(t *testing.T, e *echo.Echo, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	if len(cookies) == 0 {
		req.AddCookie(jwtCookie(t))
	}
	for _, ck := range cookies {
		if ck != nil {
			req.AddCookie(ck)
		}
	}
	e.ServeHTTP(rec, req)
	return rec
}

func flashOKFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == flashOKCookie && ck.Value != "" && ck.MaxAge != -1 {
			return ck
		}
	}
	return nil
}

func Test95_47_SavePopupOnceAndErrorKeepsInput(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	seedMnt47Visit(t, db, "v47s")
	rec := doMnt47Form(t, e, "/maintenance/visits/v47s/action", url.Values{
		"assignee":   {"최혜영"},
		"visit_date": {"2026-10-12"},
		"status":     {"open"},
		"notes":      {"이상 없음"},
		"start_time": {"10:00"},
		"end_time":   {"11:30"},
	}, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Code == http.StatusBadRequest {
		t.Fatal("400 에러 페이지")
	}
	flash := flashOKFrom(rec)
	if flash == nil {
		t.Fatal("jp_flash_ok 없음")
	}
	got := doMnt47Get(t, e, rec.Header().Get("Location"), jwtCookie(t), flash)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d", got.Code)
	}
	body := got.Body.String()
	if !strings.Contains(body, "저장되었습니다.") {
		t.Fatal("저장 팝업 제목 없음")
	}
	if !strings.Contains(body, "서산고등학교") {
		t.Fatal("요약 사이트 없음")
	}
	again := doMnt47Get(t, e, rec.Header().Get("Location"), jwtCookie(t))
	if strings.Contains(again.Body.String(), "저장되었습니다.") {
		t.Fatal("새로고침에 팝업 재표시")
	}

	bad := doMnt47Form(t, e, "/maintenance/visits/v47s/action", url.Values{
		"assignee":   {"최혜영"},
		"visit_date": {"2026-10-12"},
		"status":     {"open"},
		"notes":      {"보존해야 할 비고"},
		"start_time": {"11:30"},
		"end_time":   {"10:00"},
	}, nil)
	if bad.Code == http.StatusBadRequest {
		t.Fatal("시간 오류가 400 페이지")
	}
	if bad.Code != http.StatusOK {
		t.Fatalf("오류 재표시 %d", bad.Code)
	}
	if !strings.Contains(bad.Body.String(), "확인해 주세요") {
		t.Fatal("오류 팝업 없음")
	}
	if !strings.Contains(bad.Body.String(), "보존해야 할 비고") {
		t.Fatal("입력값 미보존")
	}
}

func Test95_47_MarkDonePopupAndBackAndHideButton(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	seedMnt47Visit(t, db, "v47d")
	rec := doMnt47Form(t, e, "/maintenance/visits/v47d/action", url.Values{
		"assignee":   {"최혜영"},
		"visit_date": {"2026-10-12"},
		"mark_done":  {"1"},
		"back":       {"/workboard/board?view=list"},
	}, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("완료 %d", rec.Code)
	}
	v, err := repository.NewMaintenanceRepo(db).GetVisit("v47d")
	if err != nil || v == nil || !v.Completed {
		t.Fatalf("완료 미반영 %+v %v", v, err)
	}
	flash := flashOKFrom(rec)
	got := doMnt47Get(t, e, rec.Header().Get("Location"), jwtCookie(t), flash)
	body := got.Body.String()
	if !strings.Contains(body, "점검이 완료되었습니다.") {
		t.Fatal("완료 팝업 제목 없음")
	}
	if !strings.Contains(body, "/workboard/board?view=list") {
		t.Fatal("back 주소 없음")
	}
	if strings.Contains(body, "방문 완료 처리") && strings.Contains(body, `name="mark_done"`) {
		t.Fatal("완료 건에 방문 완료 처리 버튼")
	}
}

func Test95_47_TimesSyncToWorkTask(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	seedMnt47Visit(t, db, "v47t")
	if _, err := db.Exec(`INSERT INTO work_tasks
		(task_id, work_type, title, due_date, work_date, start_time, end_time, duration_min, status, priority, assignee, source_type, source_id, org_id)
		VALUES ('WT47','maintenance','[점검]서산고등학교_2026-10-12','2026-10-12','2026-10-12','','',30,'waiting','normal','최혜영','maintenance','v47t',?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	rec := doMnt47Form(t, e, "/maintenance/visits/v47t/action", url.Values{
		"assignee":   {"최혜영"},
		"visit_date": {"2026-10-12"},
		"status":     {"open"},
		"start_time": {"10:00"},
		"end_time":   {"11:30"},
		"travel_min": {"40"},
	}, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	v, _ := repository.NewMaintenanceRepo(db).GetVisit("v47t")
	if v.StartTime != "10:00" || v.EndTime != "11:30" || v.DurationMin != 90 || v.TravelMin != 40 {
		t.Fatalf("방문 시간 %+v", v)
	}
	task, err := repository.NewWBRepo(db).GetTaskBySource(model.WBSourceMaintenance, "v47t")
	if err != nil || task == nil {
		t.Fatal(err)
	}
	if task.StartTime != "10:00" || task.EndTime != "11:30" || task.DurationMin != 90 {
		t.Fatalf("업무 시간 %+v", task)
	}
	board := doMnt47Get(t, e, "/workboard/board?view=list&date=2026-10-12")
	if !strings.Contains(board.Body.String(), "10:00~11:30") && !strings.Contains(board.Body.String(), "10/12") {
		t.Fatalf("리스트 일정 없음: 일부=%s", board.Body.String()[:min(800, len(board.Body.String()))])
	}
}

func Test95_47_AssigneePeopleRowsWereBlockingLegacySave(t *testing.T) {
	// 원인: UpdateVisitAction 이 v:=*existing 후 assignee 문자열만 덮고
	// applyVisitAssigneesFromForm 을 호출하지 않아, People 행(또는 work_group_id)이
	// syncVisitAssigneeRows 에서 옛 담당자를 다시 썼다.
	e, db, h := newMnt47Server(t)
	seedMnt47Visit(t, db, "v47a")
	users := repository.NewUserRepo(db)
	u1, _ := users.GetByUsername("a47")
	u2, _ := users.GetByUsername("b47")
	if err := h.Maintenance.repo.ReplaceVisitAssignees("v47a", []model.MaintenanceVisitAssignee{
		{UserID: u1.UserID, AssigneeName: "최혜영", Role: model.WBMemberOwner},
		{UserID: u2.UserID, AssigneeName: "양기헌", Role: model.WBMemberSupport},
	}, ""); err != nil {
		t.Fatal(err)
	}
	oa, _ := users.GetByUsername("oa47")
	c47u, _ := users.GetByUsername("c47")
	rec := doMnt47Form(t, e, "/maintenance/visits/v47a/action", url.Values{
		"assignee":         {"김서준"},
		"visit_date":       {"2026-10-12"},
		"status":           {"open"},
		"assignee_multi":   {"1"},
		"assignee_user_id": {c47u.UserID},
	}, jwtCookieUser(t, oa))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	got, err := h.Maintenance.repo.ListVisitAssignees("v47a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].AssigneeName != "김서준" {
		t.Fatalf("담당 미반영 %+v", got)
	}
	v, _ := h.Maintenance.repo.GetVisit("v47a")
	if v.Assignee != "김서준" {
		t.Fatalf("대표 담당 %s", v.Assignee)
	}
}

func Test95_47_EquipmentRowsNotProductType(t *testing.T) {
	// 원인: product_type 은 슬롯 식별 문자열 1개뿐이라 장비 이름 여러 개를 저장할 칸이 없었다.
	// 같은 날 같은 슬롯이면 clearVisitSlotConflict 가 다른 방문을 지우거나 회차 가드가 400을 냈다.
	e, db, h := newMnt47Server(t)
	seedMnt47Visit(t, db, "v47e")
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, round_no)
		VALUES ('v47e2','MP47','2026-10-12','c47',1,'normal','최혜영','RFID',0,1)`); err != nil {
		t.Fatal(err)
	}
	rec := doMnt47Form(t, e, "/maintenance/visits/v47e/action", url.Values{
		"assignee":           {"최혜영"},
		"visit_date":         {"2026-10-12"},
		"status":             {"open"},
		"target_custom_name": {"게이트A", "게이트B", "데스크", "PDA"},
		"target_custom_memo": {"1층", "", "", ""},
	}, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	items, err := h.Maintenance.repo.ListVisitTargets("v47e")
	if err != nil {
		t.Fatal(err)
	}
	if line := model.FormatVisitTargetsLine(items); line != "게이트A, 게이트B, 데스크 외 1대" {
		t.Fatalf("표시 %q n=%d", line, len(items))
	}
	other, _ := h.Maintenance.repo.GetVisit("v47e2")
	if other == nil {
		t.Fatal("같은 슬롯 다른 방문이 지워지면 안 됨(장비 저장은 슬롯과 별개)")
	}
}

func Test95_47_AssetCheckboxAndForeignReject(t *testing.T) {
	e, db, h := newMnt47Server(t)
	seedMnt47Visit(t, db, "v47as")
	if err := h.Maintenance.assetRepo.Create(&model.Asset{
		CustomerID: "c47", ProductName: "K-LAS 서버", ModelName: "K1", OperationStatus: "operating",
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Maintenance.assetRepo.Create(&model.Asset{
		CustomerID: "c47b", ProductName: "타교 서버", OperationStatus: "operating",
	}); err != nil {
		t.Fatal(err)
	}
	own, _ := h.Maintenance.assetRepo.ListByCustomer("c47")
	other, _ := h.Maintenance.assetRepo.ListByCustomer("c47b")
	if len(own) == 0 || len(other) == 0 {
		t.Fatal("자산 없음")
	}
	ok := doMnt47Form(t, e, "/maintenance/visits/v47as/action", url.Values{
		"assignee":        {"최혜영"},
		"visit_date":      {"2026-10-12"},
		"status":          {"open"},
		"target_asset_id": {own[0].AssetID},
	}, nil)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("자산 저장 %d", ok.Code)
	}
	items, _ := h.Maintenance.repo.ListVisitTargets("v47as")
	if len(items) != 1 || items[0].AssetID != own[0].AssetID {
		t.Fatalf("%+v", items)
	}
	bad := doMnt47Form(t, e, "/maintenance/visits/v47as/action", url.Values{
		"assignee":        {"최혜영"},
		"visit_date":      {"2026-10-12"},
		"status":          {"open"},
		"target_asset_id": {other[0].AssetID},
	}, nil)
	if bad.Code == http.StatusBadRequest {
		t.Fatal("400")
	}
	if !strings.Contains(bad.Body.String(), "설치자산만") {
		t.Fatalf("타 사이트 거부 메시지 없음: %s", bad.Body.String()[:min(400, len(bad.Body.String()))])
	}
}

func Test95_47_GroupAssigneesAndRequiredPopup(t *testing.T) {
	e, db, h := newMnt47Server(t)
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES ('v47g', 'MP47', '2026-10-25', 'c47', 0, 'normal', '최혜영', 'KLAS', 0)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, _ := users.GetByUsername("oa47")
	a, _ := users.GetByUsername("a47")
	b, _ := users.GetByUsername("b47")
	wg := repository.NewWorkGroupRepo(db)
	g := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "서산팀"}
	if err := wg.CreateWithMembers(model.WorkGroupActor{UserID: oa.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleOrgAdmin}, g, []string{a.UserID, b.UserID}, a.UserID); err != nil {
		t.Fatal(err)
	}
	rec := doMnt47Form(t, e, "/maintenance/visits/v47g/action", url.Values{
		"visit_date":        {"2026-10-25"},
		"status":            {"open"},
		"assignee_multi":    {"1"},
		"assignee_group_id": {g.GroupID},
	}, jwtCookieUser(t, oa))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("그룹 %d %s", rec.Code, rec.Body.String())
	}
	got, _ := h.Maintenance.repo.ListVisitAssignees("v47g")
	if len(got) < 2 {
		t.Fatalf("구성원 전체 아님 %+v", got)
	}
	empty := doMnt47Form(t, e, "/maintenance/visits/v47g/action", url.Values{
		"visit_date":     {"2026-10-25"},
		"status":         {"open"},
		"assignee_multi": {"1"},
	}, jwtCookieUser(t, oa))
	if empty.Code == http.StatusBadRequest {
		t.Fatal("400")
	}
	if !strings.Contains(empty.Body.String(), msgAssigneeRequired) && !strings.Contains(empty.Body.String(), "확인해 주세요") {
		t.Fatal("담당자 필수 팝업 없음")
	}
}

func Test95_47_VisitActionCSSInBundle(t *testing.T) {
	root := testWebRoot(t)
	tpl, err := os.ReadFile(filepath.Join(root, "templates", "maintenance", "visit_action.html"))
	if err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile(filepath.Join(root, "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := string(css)
	for _, tok := range extractClassTokens(string(tpl)) {
		if !strings.Contains(tok, "[") {
			continue
		}
		esc := strings.ReplaceAll(tok, "[", `\[`)
		esc = strings.ReplaceAll(esc, "]", `\]`)
		if !strings.Contains(bundle, esc) && !strings.Contains(bundle, tok) {
			t.Fatalf("임의값 클래스 %q 가 번들에 없다 — npm run build:css 필요", tok)
		}
	}
}
