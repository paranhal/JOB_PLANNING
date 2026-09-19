package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestKnowledgeGapGroupHTTPPartial(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "gap-group.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('A1','c1','KLAS')`); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	asRepo := repository.NewASRepo(db)
	proc := repository.NewASProcessRepo(db)
	var lastID string
	for i := 0; i < 22; i++ {
		as := &model.ASReceipt{
			CustomerID: "c1", AssetID: "A1", Symptom: "홈페이지가 열리지 않습니다",
			ReceiptDatetime: time.Date(2026, 6, 1+i, 9, 0, 0, 0, time.Local),
			Urgency:         "normal", Priority: "normal",
		}
		if err := asRepo.Create(as); err != nil {
			t.Fatal(err)
		}
		lastID = as.ASID
	}
	if err := proc.Create(&model.ASProcess{ASID: lastID, WorkContent: "완료", TimeSpent: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := asRepo.RebuildKeywordLinks(nil); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/knowledge/gaps", h.AS.KnowledgeGaps)
	g.GET("/as/knowledge/gaps/group", h.AS.KnowledgeGapGroup)

	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/knowledge/gaps", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, req)
	if page.Code != http.StatusOK {
		t.Fatalf("gaps %d %s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), `hx-get="/as/knowledge/gaps/group?keyword_id=KW001"`) {
		t.Fatal("묶음 펼침 버튼이 없다")
	}
	if !strings.Contains(page.Body.String(), "기록 없음 21") || !strings.Contains(page.Body.String(), "10자 미만 1") {
		t.Fatalf("묶음 줄 숫자가 안 나뉜다\n%s", page.Body.String())
	}

	start := time.Now()
	rec := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodGet, "http://localhost/as/knowledge/gaps/group?keyword_id=KW001", nil)
	greq.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, greq)
	if dur := time.Since(start); dur > 200*time.Millisecond {
		t.Fatalf("응답 %s > 200ms", dur)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("group %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `"message":"Internal Server Error"`) {
		t.Fatal("부분 템플릿이 죽었다")
	}
	if strings.Count(body, `id="gap-as-`) != 20 {
		t.Fatalf("20건만 와야 한다: %d\n%s", strings.Count(body, `id="gap-as-`), body)
	}
	for _, col := range []string{"접수번호", "접수일", "사이트", "증상"} {
		if !strings.Contains(body, col) {
			t.Fatalf("열 %s 가 없다", col)
		}
	}
	if !strings.Contains(body, "22건 중 1–20") {
		t.Fatal("총계 구간이 없다")
	}
	assertSortHeaderChrome(t, body, "gap-sort-KW001", true)
	if !strings.Contains(body, "더 보기") {
		t.Fatal("더 보기 없다")
	}
	if !strings.Contains(body, "10자 미만") || !strings.Contains(body, "기존 기록: 「완료」") {
		t.Fatal("10자 미만 원문이 없다")
	}
	if !strings.Contains(body, ">완료</textarea>") {
		t.Fatal("짧은 조치가 입력란에 안 채워졌다")
	}

	more := httptest.NewRecorder()
	mreq := httptest.NewRequest(http.MethodGet, "http://localhost/as/knowledge/gaps/group?keyword_id=KW001&offset=20", nil)
	mreq.AddCookie(jwtCookie(t))
	e.ServeHTTP(more, mreq)
	if more.Code != http.StatusOK {
		t.Fatalf("offset %d %s", more.Code, more.Body.String())
	}
	if strings.Count(more.Body.String(), `id="gap-as-`) != 2 {
		t.Fatalf("21~22번째 n=%d %s", strings.Count(more.Body.String(), `id="gap-as-`), more.Body.String())
	}
}

func TestKnowledgeGapIndexEmptyMessage(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "gap-empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	asRepo := repository.NewASRepo(db)
	as := &model.ASReceipt{
		CustomerID: "c1", Symptom: "홈페이지가 열리지 않습니다",
		ReceiptDatetime: time.Date(2026, 6, 1, 9, 0, 0, 0, time.Local),
		Urgency:         "normal", Priority: "normal",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM as_keyword_links`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/knowledge/gaps", h.AS.KnowledgeGaps)
	g.GET("/as/knowledge/gaps/group", h.AS.KnowledgeGapGroup)

	page := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, "http://localhost/as/knowledge/gaps", nil)
	preq.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, preq)
	if strings.Contains(page.Body.String(), `hx-get="/as/knowledge/gaps/group?keyword_id=KW001"`) {
		t.Fatal("색인이 비었는데 키워드 묶음 건수가 있다")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/knowledge/gaps/group?keyword_id=KW001", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("group %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "묶음 색인이 비어 있습니다") || !strings.Contains(rec.Body.String(), "지금 다시 묶기") {
		t.Fatalf("빈 색인 안내가 없다\n%s", rec.Body.String())
	}
}
