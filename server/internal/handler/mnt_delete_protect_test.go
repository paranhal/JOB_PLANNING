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
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newMntProtectApp(t *testing.T) (*echo.Echo, *sql.DB, *Handler, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "protect.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if err := repository.NewSettingsRepo(db).Set(repository.SettingMaintenanceDeletePassword, HashPassword("del-pw")); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	h.Maintenance.dataDir = dir
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/:id", h.Maintenance.ShowPlan)
	g.GET("/maintenance/:id/generate", h.Maintenance.GenerateConfirm)
	g.POST("/maintenance/:id/generate", h.Maintenance.GenerateAuto)
	g.GET("/maintenance/:id/delete", h.Maintenance.DeletePlanPage)
	g.POST("/maintenance/:id/delete", h.Maintenance.DeletePlan)
	g.GET("/maintenance/:id/duplicates", h.Maintenance.DuplicatesPreview)
	g.POST("/maintenance/:id/duplicates", h.Maintenance.CollapseDuplicates)
	g.POST("/maintenance/visits/:visit_id/delete", h.Maintenance.DeleteVisit)
	return e, db, h, dir
}

func seedProtectPlan(t *testing.T, db *sql.DB, repo *repository.MaintenanceRepo, dates ...string) *model.MaintenancePlan {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','가나도서관','가나도서관',1)`); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "2026년 정기점검")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dates {
		if err := repo.InsertVisitFull(model.MaintenanceVisit{
			PlanID: p.PlanID, VisitDate: d, CustomerID: "c1", ProductType: "KLAS",
		}); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func mntGet(t *testing.T, e *echo.Echo, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func mntPost(t *testing.T, e *echo.Echo, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func locText(rec *httptest.ResponseRecorder) string {
	loc, err := url.QueryUnescape(rec.Header().Get("Location"))
	if err != nil {
		return rec.Header().Get("Location")
	}
	return loc
}

func TestShowPlanGetDoesNotCollapseDuplicates(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	p := seedProtectPlan(t, db, h.Maintenance.repo, "2026-11-10", "2026-11-14")

	rec := mntGet(t, e, "/maintenance/"+p.PlanID+"?view=list")
	if rec.Code != http.StatusOK {
		t.Fatalf("조회 status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "중복 1건 발견") {
		t.Fatalf("중복 안내 없음: %s", body)
	}
	if !strings.Contains(body, "정리하기") {
		t.Fatal("정리하기 링크 없음")
	}
	visits, _ := h.Maintenance.repo.ListVisits(p.PlanID)
	if len(visits) != 2 {
		t.Fatalf("GET이 방문을 지움: %d", len(visits))
	}
}

func TestCollapseDuplicatesRequiresConfirm(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	p := seedProtectPlan(t, db, h.Maintenance.repo, "2026-11-10", "2026-11-14")

	preview := mntGet(t, e, "/maintenance/"+p.PlanID+"/duplicates?month=11")
	if preview.Code != http.StatusOK {
		t.Fatalf("미리보기 status=%d", preview.Code)
	}
	if !strings.Contains(preview.Body.String(), "지워질 방문") {
		t.Fatalf("목록 없음: %s", preview.Body.String())
	}

	skip := mntPost(t, e, "/maintenance/"+p.PlanID+"/duplicates", url.Values{"month": {"11"}})
	if skip.Code != http.StatusSeeOther || !strings.Contains(locText(skip), "/duplicates") {
		t.Fatalf("확인 없이 실행됨: status=%d loc=%s", skip.Code, skip.Header().Get("Location"))
	}
	if n, _ := h.Maintenance.repo.ListVisits(p.PlanID); len(n) != 2 {
		t.Fatalf("확인 전에 삭제됨: %d", len(n))
	}

	ok := mntPost(t, e, "/maintenance/"+p.PlanID+"/duplicates", url.Values{"confirm": {"1"}, "month": {"11"}})
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("정리 status=%d", ok.Code)
	}
	left, _ := h.Maintenance.repo.ListVisits(p.PlanID)
	if len(left) != 1 {
		t.Fatalf("정리 후 건수=%d", len(left))
	}
}

func TestDeleteVisitHTTPRejectsCurrentMonth(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	p := seedProtectPlan(t, db, h.Maintenance.repo, "2026-08-10")
	visits, _ := h.Maintenance.repo.ListVisits(p.PlanID)
	rec := mntPost(t, e, "/maintenance/visits/"+visits[0].VisitID+"/delete?plan_id="+p.PlanID, url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(locText(rec), "당월") && !strings.Contains(locText(rec), "삭제할 수 없습니다") {
		t.Fatalf("보호 오류가 없다: %s", locText(rec))
	}
	left, _ := h.Maintenance.repo.ListVisits(p.PlanID)
	if len(left) != 1 {
		t.Fatalf("당월 방문이 지워짐: %d", len(left))
	}
}

func TestDeletePlanRemoved(t *testing.T) {
	e, db, h, dir := newMntProtectApp(t)
	p := seedProtectPlan(t, db, h.Maintenance.repo, "2026-11-10")

	page := mntGet(t, e, "/maintenance/"+p.PlanID+"/delete")
	if page.Code != http.StatusOK {
		t.Fatalf("삭제 화면 status=%d", page.Code)
	}
	if !strings.Contains(page.Body.String(), "계획은 삭제할 수 없습니다") {
		t.Fatalf("삭제 불가 안내 없음: %s", page.Body.String())
	}

	ok := mntPost(t, e, "/maintenance/"+p.PlanID+"/delete", url.Values{
		"confirm_title":   {"2026년 정기점검"},
		"delete_password": {"del-pw"},
	})
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", ok.Code)
	}
	if !strings.Contains(locText(ok), "삭제할 수 없습니다") {
		t.Fatalf("거부 안내: %s", locText(ok))
	}
	if got, _ := h.Maintenance.repo.GetPlan(p.PlanID); got == nil {
		t.Fatal("계획이 지워졌다")
	}
	backupDir := filepath.Join(dir, "backups")
	if ents, err := os.ReadDir(backupDir); err == nil && len(ents) > 0 {
		t.Fatalf("삭제 백업이 생기면 안 됨: %v", ents)
	}
}

func TestDeletePlanPasswordCannotBypassProtected(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	p := seedProtectPlan(t, db, h.Maintenance.repo, "2026-08-10")
	rec := mntPost(t, e, "/maintenance/"+p.PlanID+"/delete", url.Values{
		"confirm_title":   {"2026년 정기점검"},
		"delete_password": {"del-pw"},
	})
	if !strings.Contains(locText(rec), "삭제할 수 없습니다") {
		t.Fatalf("삭제 거부 없음: %s", locText(rec))
	}
	if got, _ := h.Maintenance.repo.GetPlan(p.PlanID); got == nil {
		t.Fatal("계획이 지워짐")
	}
}

func TestDeletePlanRejectsCompletedVisit(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	p := seedProtectPlan(t, db, h.Maintenance.repo, "2026-11-10")
	visits, _ := h.Maintenance.repo.ListVisits(p.PlanID)
	if err := h.Maintenance.repo.SetVisitCompleted(visits[0].VisitID, true, "2026-11-10"); err != nil {
		t.Fatal(err)
	}
	page := mntGet(t, e, "/maintenance/"+p.PlanID+"/delete")
	if !strings.Contains(page.Body.String(), "계획은 삭제할 수 없습니다") {
		t.Fatalf("삭제 불가 안내 없음: %s", page.Body.String())
	}
	rec := mntPost(t, e, "/maintenance/"+p.PlanID+"/delete", url.Values{
		"confirm_title":   {"2026년 정기점검"},
		"delete_password": {"del-pw"},
	})
	if !strings.Contains(locText(rec), "삭제할 수 없습니다") {
		t.Fatalf("완료 거부 없음: %s", locText(rec))
	}
	if got, _ := h.Maintenance.repo.GetPlan(p.PlanID); got == nil {
		t.Fatal("계획이 지워짐")
	}
}

func TestGenerateAutoRequiresConfirm(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	h.Maintenance.repo.Now = func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.Local) }
	p := seedProtectPlan(t, db, h.Maintenance.repo)
	if err := h.Maintenance.repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-11-10", CustomerID: "c1", ProductType: "KLAS", AutoGenerated: true,
	}); err != nil {
		t.Fatal(err)
	}

	page := mntGet(t, e, "/maintenance/"+p.PlanID+"/generate?month=11")
	if page.Code != http.StatusOK {
		t.Fatalf("확인 화면 status=%d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "2026년 11월 자동 배정") {
		t.Fatalf("월 제목 없음: %s", body)
	}
	if !strings.Contains(body, "11월 자동 생성 방문") || !strings.Contains(body, "1건") {
		t.Fatalf("삭제 건수 안내 없음: %s", body)
	}
	if !strings.Contains(body, "전월") {
		t.Fatal("자동 배정 기본값 전월이 없다")
	}

	skip := mntPost(t, e, "/maintenance/"+p.PlanID+"/generate", url.Values{"month": {"11"}})
	if skip.Code != http.StatusSeeOther || !strings.Contains(locText(skip), "/generate") {
		t.Fatalf("확인 없이 실행: status=%d loc=%s", skip.Code, skip.Header().Get("Location"))
	}
	left, _ := h.Maintenance.repo.ListVisits(p.PlanID)
	if len(left) != 1 {
		t.Fatalf("확인 전에 자동 방문이 지워짐: %d", len(left))
	}
}

func TestGenerateConfirmPastMonthRejected(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	h.Maintenance.repo.Now = func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.Local) }
	p := seedProtectPlan(t, db, h.Maintenance.repo)
	if err := h.Maintenance.repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-10", CustomerID: "c1", ProductType: "KLAS", AutoGenerated: true,
	}); err != nil {
		t.Fatal(err)
	}

	page := mntGet(t, e, "/maintenance/"+p.PlanID+"/generate?month=8")
	if page.Code != http.StatusOK {
		t.Fatalf("status=%d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "2026년 8월 자동 배정") {
		t.Fatalf("월 제목 없음: %s", body)
	}
	if !strings.Contains(body, "지난 달은 배정할 수 없습니다") {
		t.Fatalf("지난 달 거부 문구 없음: %s", body)
	}
	if strings.Contains(body, "확인하고 자동 배정") {
		t.Fatal("지난 달에 실행 버튼이 있다")
	}

	run := mntPost(t, e, "/maintenance/"+p.PlanID+"/generate", url.Values{
		"confirm": {"1"}, "month": {"8"},
	})
	if run.Code != http.StatusSeeOther || !strings.Contains(locText(run), "지난 달은 배정할 수 없습니다") {
		t.Fatalf("실행 거부 없음: %s", locText(run))
	}
}
