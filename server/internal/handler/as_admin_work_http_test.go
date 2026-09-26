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

func new45DServer(t *testing.T) (*echo.Echo, *sql.DB, *repository.WBRepo, *repository.ASRepo) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "45d_http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','해미도서관','해미도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, model_name, operation_status) VALUES
		('A1','C1','발급기','M1','operating'), ('A2','C1','반납기','M2','operating')`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/new", h.AS.New)
	g.GET("/as/:id", h.AS.Show)
	g.POST("/as/:id/to-admin-work", h.AS.ToAdminWork)
	g.GET("/admin-work/new", h.AdminWork.New)
	g.POST("/admin-work", h.AdminWork.Create)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.GET("/assets/:id", h.Asset.Show)
	g.GET("/api/assets/:customer_id", h.Asset.APIAssetsByCustomer)
	return e, db, repository.NewWBRepo(db), repository.NewASRepo(db)
}

func TestClassifyHintOnBothForms(t *testing.T) {
	e, db, _, _ := new45DServer(t)
	asForm := doGet(t, e, "/as/new")
	awForm := doGet(t, e, "/admin-work/new")
	if asForm.Code != http.StatusOK || awForm.Code != http.StatusOK {
		t.Fatalf("forms %d %d", asForm.Code, awForm.Code)
	}
	if !strings.Contains(asForm.Body.String(), "행정/지원") || !strings.Contains(awForm.Body.String(), "행정/지원") {
		t.Fatal("분류 안내가 두 화면에 없음")
	}
	if _, err := db.Exec(`UPDATE codes SET code_name='안내문구테스트' WHERE code_id='WCH001'`); err != nil {
		t.Fatal(err)
	}
	repository.LoadLookupCache(db)
	as2 := doGet(t, e, "/as/new")
	if !strings.Contains(as2.Body.String(), "안내문구테스트") {
		t.Fatal("codes 변경이 화면에 반영되지 않음")
	}
}

func TestAdminWorkAssetLinkAndASMoveHTTP(t *testing.T) {
	e, db, wb, asRepo := new45DServer(t)
	rec := doForm(t, e, "/admin-work", url.Values{
		"work_type":   {"admin"},
		"title":       {"DB 서버 IP 수정"},
		"due_date":    {"2026-09-30"},
		"work_date":   {"2026-09-26"},
		"customer_id": {"C1"},
		"asset_id":    {"A1", "A2"},
		"status":      {model.WBTaskWaiting},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("create status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	items, err := wb.ListAdminWork("", "")
	if err != nil || len(items) == 0 {
		t.Fatalf("list %+v err=%v", items, err)
	}
	id := items[0].TaskID
	got, _ := wb.GetTask(id)
	if got == nil || len(got.LinkedAssets) != 2 {
		t.Fatalf("saved assets %+v", got)
	}
	show := doGet(t, e, "/workboard/tasks/"+id)
	body := show.Body.String()
	if !strings.Contains(body, "/assets/A1") || !strings.Contains(body, "/assets/A2") {
		t.Fatal("업무 상세에 자산 링크 없음")
	}
	assetShow := doGet(t, e, "/assets/A1")
	if assetShow.Code != http.StatusOK || !strings.Contains(assetShow.Body.String(), id) {
		t.Fatal("자산 지원 이력에 업무가 없음")
	}

	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, asset_id, receipt_datetime, symptom, status, assigned_to, data_origin)
		VALUES ('R9','R2609-012','C1','A1','2026-09-15 10:00:00','웹 접근성 오류','received','관리자','app')`); err != nil {
		t.Fatal(err)
	}
	bad := doForm(t, e, "/as/R9/to-admin-work", url.Values{})
	if bad.Code != http.StatusSeeOther || !strings.Contains(bad.Header().Get("Location"), "err=move_reason") {
		t.Fatalf("no reason loc=%s", bad.Header().Get("Location"))
	}
	ok := doForm(t, e, "/as/R9/to-admin-work", url.Values{"reason": {"AS가 아니라 지원 요청"}})
	if ok.Code != http.StatusSeeOther || !strings.Contains(ok.Header().Get("Location"), "ok=moved") {
		t.Fatalf("move loc=%s", ok.Header().Get("Location"))
	}
	as, _ := asRepo.GetByID("R9")
	if as == nil || as.Status != model.StatusAdminWork || as.MovedTaskID == "" {
		t.Fatalf("as %+v", as)
	}
	asShow := doGet(t, e, "/as/R9")
	if !strings.Contains(asShow.Body.String(), as.MovedTaskID+" 로 옮김") {
		t.Fatal("AS 상세 링크 없음")
	}
	taskShow := doGet(t, e, "/workboard/tasks/"+as.MovedTaskID)
	if !strings.Contains(taskShow.Body.String(), "R2609-012 에서 옮겨옴") {
		t.Fatal("업무 상세 링크 없음")
	}
}

func TestAPIAssetsFollowsCustomer(t *testing.T) {
	e, db, _, _ := new45DServer(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C2','다른곳','다른곳',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, operation_status) VALUES ('A9','C2','다른자산','operating')`); err != nil {
		t.Fatal(err)
	}
	c1 := doGet(t, e, "/api/assets/C1")
	c2 := doGet(t, e, "/api/assets/C2")
	if !strings.Contains(c1.Body.String(), "A1") || strings.Contains(c1.Body.String(), "A9") {
		t.Fatalf("c1 assets %s", c1.Body.String())
	}
	if !strings.Contains(c2.Body.String(), "A9") {
		t.Fatalf("c2 assets %s", c2.Body.String())
	}
}
