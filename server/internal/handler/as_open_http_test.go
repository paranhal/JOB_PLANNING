package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestASOpenBannerAPIAndPullAndPastDate(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "open-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c1','도서관','도서관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	asRepo := repository.NewASRepo(db)
	mine := &model.ASReceipt{
		CustomerID: "c1", Symptom: "내 건", AssignedTo: "관리자", Status: "in_progress",
		ReceiptDatetime: time.Now(), OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(mine); err != nil {
		t.Fatal(err)
	}
	other := &model.ASReceipt{
		CustomerID: "c1", Symptom: "남의 건", AssignedTo: "김기술", Status: "partial_complete",
		ReceiptDatetime: time.Now(), OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(other); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE as_receipts SET status='partial_complete' WHERE as_id=?`, other.ASID); err != nil {
		t.Fatal(err)
	}
	done := &model.ASReceipt{
		CustomerID: "c1", Symptom: "완료", AssignedTo: "관리자", Status: "completed",
		ReceiptDatetime: time.Now(), OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(done); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE as_receipts SET status='completed' WHERE as_id=?`, done.ASID); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/api/as/open/:customer_id", h.AS.APIOpenByCustomer)
	g.POST("/as/open/pull", h.AS.PullOpen)
	g.POST("/as/open/assign", h.AS.AssignOpenDates)
	g.GET("/as/new", h.AS.New)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/as/open/c1", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("open api %d", rec.Code)
	}
	var items []model.CustomerOpenItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("띠 건수=%d want 2", len(items))
	}

	form := url.Values{"item_key": {"as:" + other.ASID}}
	pull := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodPost, "/as/open/pull", strings.NewReader(form.Encode()))
	preq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	preq.AddCookie(jwtCookie(t))
	e.ServeHTTP(pull, preq)
	if !strings.Contains(pull.Body.String(), `"need_reason":true`) {
		t.Fatalf("사유 없이 가져오기: %s", pull.Body.String())
	}

	form.Set("takeover_reason", "현장 대행")
	pull2 := httptest.NewRecorder()
	preq2 := httptest.NewRequest(http.MethodPost, "/as/open/pull", strings.NewReader(form.Encode()))
	preq2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	preq2.AddCookie(jwtCookie(t))
	e.ServeHTTP(pull2, preq2)
	if !strings.Contains(pull2.Body.String(), `"assigned":1`) {
		t.Fatalf("가져오기 실패: %s", pull2.Body.String())
	}

	past := url.Values{
		"item_key":   {"as:" + mine.ASID},
		"visit_date": {"2020-01-01"},
		"assignee":   {"관리자"},
	}
	bad := httptest.NewRecorder()
	breq := httptest.NewRequest(http.MethodPost, "/as/open/assign", strings.NewReader(past.Encode()))
	breq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	breq.AddCookie(jwtCookie(t))
	e.ServeHTTP(bad, breq)
	if !strings.Contains(bad.Body.String(), "지난 날짜") {
		t.Fatalf("지난 날짜: %s", bad.Body.String())
	}

	page := httptest.NewRecorder()
	q := httptest.NewRequest(http.MethodGet, "/as/new", nil)
	q.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, q)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "as-open-banner") {
		t.Fatalf("접수 폼에 띠가 없다 %d", page.Code)
	}
}

func TestASBehalfProcessKeepsOwner(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"work_place":   {"office"},
		"process_type": {"remote"},
		"cause_type":   {"hw"},
		"action_taken": {"원격으로 재시작 안내함"},
		"result_code":  {model.ResultDone},
		"time_spent":   {"20"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("조치 저장 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil || len(procs) == 0 {
		t.Fatalf("이력 err=%v n=%d", err, len(procs))
	}
	p := procs[len(procs)-1]
	if p.Worker != "양기헌" {
		t.Fatalf("worker=%s want 양기헌", p.Worker)
	}
	if !p.OnBehalf || p.ActedByName != "관리자" {
		t.Fatalf("on_behalf=%v acted=%s", p.OnBehalf, p.ActedByName)
	}
	if p.ActorLabel() != "양기헌 (대신: 관리자)" {
		t.Fatalf("표시=%s", p.ActorLabel())
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got == nil || (got.Status != "completed" && got.Status != "closed") {
		t.Fatalf("완료 상태=%v", got)
	}
	page := getASActionPage(t, e, asID)
	if !strings.Contains(page, "대신: 관리자") && !strings.Contains(page, "대신 처리합니다") {
		t.Fatal("대신 표시가 없다")
	}
}
