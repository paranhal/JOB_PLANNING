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

func TestDeletePlanThreeStepAndBackup(t *testing.T) {
	e, db, h, dir := newMntProtectApp(t)
	p := seedProtectPlan(t, db, h.Maintenance.repo, "2026-11-10")

	page := mntGet(t, e, "/maintenance/"+p.PlanID+"/delete")
	if page.Code != http.StatusOK {
		t.Fatalf("삭제 화면 status=%d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "방문 1건(완료 0건 포함)") {
		t.Fatalf("영향 범위 없음: %s", body)
	}

	wrongTitle := mntPost(t, e, "/maintenance/"+p.PlanID+"/delete", url.Values{
		"confirm_title":   {"다른제목"},
		"delete_password": {"del-pw"},
	})
	if !strings.Contains(locText(wrongTitle), "제목") {
		t.Fatalf("제목 불일치 안내: %s", locText(wrongTitle))
	}

	wrongPW := mntPost(t, e, "/maintenance/"+p.PlanID+"/delete", url.Values{
		"confirm_title":   {"2026년 정기점검"},
		"delete_password": {"wrong"},
	})
	if !strings.Contains(locText(wrongPW), "비밀번호") {
		t.Fatalf("비밀번호 거부 안내: %s", locText(wrongPW))
	}
	if got, _ := h.Maintenance.repo.GetPlan(p.PlanID); got == nil {
		t.Fatal("검증 실패인데 계획이 지워짐")
	}

	ok := mntPost(t, e, "/maintenance/"+p.PlanID+"/delete", url.Values{
		"confirm_title":   {"2026년 정기점검"},
		"delete_password": {"del-pw"},
	})
	if ok.Code != http.StatusSeeOther || ok.Header().Get("Location") != "/maintenance" {
		t.Fatalf("삭제 결과: status=%d loc=%s", ok.Code, ok.Header().Get("Location"))
	}
	if got, _ := h.Maintenance.repo.GetPlan(p.PlanID); got != nil {
		t.Fatal("계획이 남아 있음")
	}
	backupDir := filepath.Join(dir, "backups")
	ents, err := os.ReadDir(backupDir)
	if err != nil || len(ents) == 0 {
		t.Fatalf("백업 없음: %v", err)
	}
	found := false
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "계획삭제_"+p.PlanID+"_") && strings.HasSuffix(e.Name(), ".json") {
			found = true
			raw, _ := os.ReadFile(filepath.Join(backupDir, e.Name()))
			if !strings.Contains(string(raw), "2026-11-10") {
				t.Fatalf("백업에 방문이 없다: %s", raw)
			}
		}
	}
	if !found {
		t.Fatalf("백업 파일명: %v", ents)
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
		t.Fatalf("기간 보호 거부 없음: %s", locText(rec))
	}
	if got, _ := h.Maintenance.repo.GetPlan(p.PlanID); got == nil {
		t.Fatal("비밀번호로 당월 계획이 지워짐")
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
	if !strings.Contains(page.Body.String(), "완료된 방문 1건") {
		t.Fatalf("완료 차단 안내 없음: %s", page.Body.String())
	}
	rec := mntPost(t, e, "/maintenance/"+p.PlanID+"/delete", url.Values{
		"confirm_title":   {"2026년 정기점검"},
		"delete_password": {"del-pw"},
	})
	if !strings.Contains(locText(rec), "완료된 방문") {
		t.Fatalf("완료 거부 없음: %s", locText(rec))
	}
}

func TestGenerateAutoRequiresConfirm(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	p := seedProtectPlan(t, db, h.Maintenance.repo)
	if err := h.Maintenance.repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-11-10", CustomerID: "c1", ProductType: "KLAS", AutoGenerated: true,
	}); err != nil {
		t.Fatal(err)
	}

	page := mntGet(t, e, "/maintenance/"+p.PlanID+"/generate")
	if page.Code != http.StatusOK {
		t.Fatalf("확인 화면 status=%d", page.Code)
	}
	if !strings.Contains(page.Body.String(), "기존 자동 생성 방문") || !strings.Contains(page.Body.String(), "1건") {
		t.Fatalf("삭제 건수 안내 없음: %s", page.Body.String())
	}

	skip := mntPost(t, e, "/maintenance/"+p.PlanID+"/generate", url.Values{})
	if skip.Code != http.StatusSeeOther || !strings.Contains(locText(skip), "/generate") {
		t.Fatalf("확인 없이 실행: status=%d loc=%s", skip.Code, skip.Header().Get("Location"))
	}
	left, _ := h.Maintenance.repo.ListVisits(p.PlanID)
	if len(left) != 1 {
		t.Fatalf("확인 전에 자동 방문이 지워짐: %d", len(left))
	}
}
