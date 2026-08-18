package handler

import (
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

func newReopenFixture(t *testing.T) (*echo.Echo, *Handler, *repository.ASRepo, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "reopen.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"cust_a", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	asRepo := repository.NewASRepo(db)
	src := &model.ASReceipt{
		CustomerID: "cust_a", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "게이트 오작동", Urgency: "high", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now().AddDate(0, 0, -20),
	}
	if err := asRepo.Create(src); err != nil {
		t.Fatal(err)
	}
	// 완료 처리
	src.Status = "completed"
	src.ResultCode = "done"
	src.ActionTaken = "센서 교체"
	if err := asRepo.Update(src); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/as/:id/reopen", h.AS.Reopen)
	g.GET("/as/:id", h.AS.Show)
	return e, h, asRepo, src.ASID
}

func postReopen(t *testing.T, e *echo.Echo, asID string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/reopen",
		strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

// 완료된 건을 같은 증상으로 재접수하면 새 접수번호가 생기고 원 건과 이어져야 한다.
func TestASReopenCreatesLinkedReceipt(t *testing.T) {
	e, _, asRepo, srcID := newReopenFixture(t)

	rec := postReopen(t, e, srcID, url.Values{
		"reopen_reason":        {"조치 후 같은 증상 재발"},
		"visit_scheduled_date": {"2026-08-20"},
		"schedule_confirmed":   {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("재접수 실패: status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	newID := strings.TrimPrefix(loc, "/as/")
	if newID == "" || newID == srcID {
		t.Fatalf("새 접수번호가 발급되지 않았다: %q", loc)
	}

	got, err := asRepo.GetByID(newID)
	if err != nil || got == nil {
		t.Fatalf("새 접수 조회 실패: %v", err)
	}
	if !got.IsReopen || !got.IsRecurrence {
		t.Fatalf("재접수·재발 표시: reopen=%v recurrence=%v", got.IsReopen, got.IsRecurrence)
	}
	if got.ParentASID != srcID {
		t.Fatalf("원 접수 연결: %q", got.ParentASID)
	}
	if got.ReopenReason != "조치 후 같은 증상 재발" {
		t.Fatalf("재접수 사유: %q", got.ReopenReason)
	}
	if got.Symptom != "게이트 오작동" {
		t.Fatalf("증상이 그대로 넘어와야 한다: %q", got.Symptom)
	}
	if got.AssignedTo != "양기헌" {
		t.Fatalf("담당자 기본값: %q", got.AssignedTo)
	}
	if got.Status != "in_progress" {
		t.Fatalf("일정 확정이면 진행중이어야 한다: %q", got.Status)
	}
	if got.VisitScheduledDate != "2026-08-20" {
		t.Fatalf("방문예정일: %q", got.VisitScheduledDate)
	}

	// 원 건은 완료 상태 그대로 남고, 재접수 목록에서 새 건이 보여야 한다.
	src, _ := asRepo.GetByID(srcID)
	if src.Status != "completed" {
		t.Fatalf("원 건 상태가 바뀌었다: %q", src.Status)
	}
	reopens, err := asRepo.ListReopens(srcID)
	if err != nil || len(reopens) != 1 || reopens[0].ASID != newID {
		t.Fatalf("재접수 목록: %+v err=%v", reopens, err)
	}

	// 상세 화면에 연결이 보여야 한다.
	show := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+newID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req)
	if show.Code != http.StatusOK {
		t.Fatalf("상세 조회: status=%d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, "재접수 연결") || !strings.Contains(body, srcID) {
		t.Error("재접수 배너에 원 접수가 보이지 않는다")
	}
}

// 사유가 없거나 아직 진행 중인 건은 재접수하지 않는다.
func TestASReopenRejectsInvalidCases(t *testing.T) {
	e, _, asRepo, srcID := newReopenFixture(t)

	rec := postReopen(t, e, srcID, url.Values{"reopen_reason": {"  "}})
	if rec.Code != http.StatusSeeOther ||
		!strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("사유 없이 재접수됐다: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	if list, _ := asRepo.ListReopens(srcID); len(list) != 0 {
		t.Fatalf("사유 없는 재접수가 생성됐다: %+v", list)
	}

	open := &model.ASReceipt{
		CustomerID: "cust_a", ReceiptChannel: "phone",
		Symptom: "진행중 건", Urgency: "normal", Priority: "normal",
	}
	if err := asRepo.Create(open); err != nil {
		t.Fatal(err)
	}
	rec2 := postReopen(t, e, open.ASID, url.Values{"reopen_reason": {"재발"}})
	if rec2.Code != http.StatusSeeOther ||
		!strings.Contains(rec2.Header().Get("Location"), "err=") {
		t.Fatalf("진행중 건이 재접수됐다: status=%d loc=%q", rec2.Code, rec2.Header().Get("Location"))
	}
	if list, _ := asRepo.ListReopens(open.ASID); len(list) != 0 {
		t.Fatalf("진행중 건의 재접수가 생성됐다: %+v", list)
	}
}

func TestCanReopenAS(t *testing.T) {
	for _, s := range []string{"completed", "closed"} {
		if !model.CanReopenAS(s) {
			t.Errorf("%s 는 재접수할 수 있어야 한다", s)
		}
	}
	for _, s := range []string{"received", "assigned", "in_progress", "hold", "transfer", "cancelled"} {
		if model.CanReopenAS(s) {
			t.Errorf("%s 는 재접수 대상이 아니다", s)
		}
	}
}
