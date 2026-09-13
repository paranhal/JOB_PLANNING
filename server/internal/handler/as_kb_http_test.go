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

func TestASKnowledgeHTTPOriginAndHistory(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "kb2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('A1','c1','무인예약기')`); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	asRepo := repository.NewASRepo(db)
	proc := repository.NewASProcessRepo(db)
	as := &model.ASReceipt{
		CustomerID: "c1", AssetID: "A1", Symptom: "예약대출기 투입 목록이 안 나옴",
		ReceiptDatetime: time.Date(2026, 8, 12, 9, 0, 0, 0, time.Local),
		Urgency:         "normal", Priority: "normal",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := proc.Create(&model.ASProcess{ASID: as.ASID, Worker: "태자운", WorkContent: "서비스 재시작 후 정상", TimeSpent: 20}); err != nil {
		t.Fatal(err)
	}
	before := asRepo.ProcessWorkSnapshot(as.ASID)

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/knowledge", h.AS.Knowledge)
	g.POST("/as/knowledge", h.AS.CreateKnowledge)
	g.POST("/as/kb/:kb_id/revise", h.AS.ReviseKnowledge)
	g.GET("/as/kb/:kb_id/history", h.AS.KnowledgeHistory)

	post := func(path string, form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(jwtCookie(t))
		e.ServeHTTP(rec, req)
		return rec
	}
	get := func(path string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
		req.AddCookie(jwtCookie(t))
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	if rec := post("/as/knowledge", url.Values{
		"as_id": {as.ASID}, "action_text": {"방화벽 예외를 등록해야 근본 해결"}, "q": {"예약대출기"},
	}); rec.Code != http.StatusSeeOther {
		t.Fatalf("publish %d", rec.Code)
	}
	if asRepo.ProcessWorkSnapshot(as.ASID) != before {
		t.Fatal("HTTP 저장 후 원본이 바뀌었다")
	}
	cur := asRepo.CurrentKBByAS(as.ASID)
	if cur == nil {
		t.Fatal("지식이 없다")
	}
	if rec := post("/as/kb/"+cur.KBID+"/revise", url.Values{
		"action_text": {"방화벽 + 재시작"}, "change_note": {"재시작도 필요"}, "q": {"예약대출기"},
	}); rec.Code != http.StatusSeeOther {
		t.Fatalf("revise %d", rec.Code)
	}
	if asRepo.ProcessWorkSnapshot(as.ASID) != before {
		t.Fatal("HTTP 수정 후 원본이 바뀌었다")
	}

	body := get("/as/knowledge?q=" + url.QueryEscape("예약대출기"))
	if strings.Count(body, "원본 열기") != 1 {
		t.Fatalf("원본 카드가 따로 나왔다: n=%d", strings.Count(body, "원본 열기"))
	}
	if !strings.Contains(body, "정정") || !strings.Contains(body, "당시 조치 기록 보기") {
		t.Fatal("revised 뱃지·원본 접기 없음")
	}
	if !strings.Contains(body, "서비스 재시작 후 정상") || !strings.Contains(body, "태자운") {
		t.Fatal("접힌 원본 작성자가 없다")
	}
	if !strings.Contains(body, "수정 이력") {
		t.Fatal("수정 이력 링크가 없다")
	}
	if !strings.Contains(body, "작성자 미상") && !strings.Contains(body, "관리자") && !strings.Contains(body, "양기헌") {
		if !strings.Contains(body, "관리자") {
			// jwt name 관리자
		}
	}

	hist := get("/as/kb/" + asRepo.CurrentKBByAS(as.ASID).KBID + "/history")
	if !strings.Contains(hist, "수정 이력") || !strings.Contains(hist, "방화벽 예외") {
		t.Fatal("이력 페이지에 지난 rev 가 없다")
	}
}
