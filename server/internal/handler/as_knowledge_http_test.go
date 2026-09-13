package handler

import (
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

func TestASKnowledgePageSection416(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c2", "다라도서관", "다라도서관"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('A1','c1','무인예약기')`); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	asRepo := repository.NewASRepo(db)
	proc := repository.NewASProcessRepo(db)

	long := strings.Repeat("예약대출기 투입 목록이 안 나옵니다. ", 8)
	a := &model.ASReceipt{
		CustomerID: "c1", AssetID: "A1", Symptom: long,
		ReceiptDatetime: time.Date(2026, 8, 12, 9, 0, 0, 0, time.Local),
		Urgency:         "normal", Priority: "normal",
	}
	if err := asRepo.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := proc.Create(&model.ASProcess{ASID: a.ASID, WorkContent: "VNC 원격 접속 후 서비스 재시작 완료", TimeSpent: 40}); err != nil {
		t.Fatal(err)
	}
	done, _ := asRepo.GetByID(a.ASID)
	done.Status = "completed"
	ct := time.Date(2026, 8, 14, 16, 0, 0, 0, time.Local)
	done.CompleteDatetime = &ct
	if err := asRepo.Update(done); err != nil {
		t.Fatal(err)
	}

	b := &model.ASReceipt{
		CustomerID: "c2", Symptom: "홈페이지 로그인 오류",
		ReceiptDatetime: time.Date(2026, 8, 10, 9, 0, 0, 0, time.Local),
		Urgency:         "normal", Priority: "normal",
	}
	if err := asRepo.Create(b); err != nil {
		t.Fatal(err)
	}
	if err := proc.Create(&model.ASProcess{ASID: b.ASID, WorkContent: "예약대출기 설정 확인 후 재시작했습니다", TimeSpent: 25}); err != nil {
		t.Fatal(err)
	}

	empty := &model.ASReceipt{
		CustomerID: "c1", AssetID: "A1", Symptom: "예약대출기 전원만 문의",
		ReceiptDatetime: time.Now(), Urgency: "normal", Priority: "normal",
	}
	if err := asRepo.Create(empty); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/knowledge", h.AS.Knowledge)
	g.GET("/as/knowledge.xlsx", h.AS.KnowledgeExcel)
	g.GET("/customers/:id", h.Customer.Show)

	get := func(path string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
		req.AddCookie(jwtCookie(t))
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d %s", path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	home := get("/as/knowledge")
	if !strings.Contains(home, "조치 기록이 있는") || !strings.Contains(home, "건에서 찾습니다") {
		t.Fatal("한계 문구가 없다")
	}
	if !strings.Contains(home, "전체") || strings.Contains(home, `name="all" value="1" checked`) {
		t.Fatal("전체 토글 기본은 꺼짐이어야 한다")
	}
	if !strings.Contains(home, "아직 분류된 건이 없습니다") {
		t.Fatal("원인분류 0건 안내가 없다")
	}

	q := "/as/knowledge?q=" + url.QueryEscape("예약대출기")
	body := get(q)
	if !strings.Contains(body, "<mark>예약대출기</mark>") {
		t.Fatal("맞은 낱말이 진하지 않다")
	}
	if !strings.Contains(body, ">증상<") || !strings.Contains(body, ">조치<") {
		t.Fatal("증상·조치 뱃지가 없다")
	}
	if !strings.Contains(body, "line-clamp-3") || !strings.Contains(body, "펼치기") {
		t.Fatal("긴 증상을 접지 않았다")
	}
	if strings.Contains(body, "조치 기록 없음") {
		t.Fatal("기본 검색에 조치 없는 건이 나왔다")
	}
	if !strings.Contains(body, "영업일") {
		t.Fatal("소요 기간이 없다")
	}
	if !strings.Contains(body, "👍") {
		t.Fatal("도움이 된 사례 추천이 없다")
	}

	allBody := get(q + "&all=1")
	if !strings.Contains(allBody, "조치 기록 없음") && !strings.Contains(allBody, empty.Symptom[:8]) {
		t.Fatal("전체 토글이면 조치 없는 건도 나와야 한다")
	}

	site := get("/as/knowledge?tab=site&customer_id=c2")
	if strings.Contains(site, "미지정") {
		t.Fatal("사이트 건수에 미지정이 있다")
	}
	if !strings.Contains(site, "다라도서관") || !strings.Contains(site, "AS ") {
		t.Fatal("사이트 요약이 없다")
	}
	if !strings.Contains(site, "조치 기록") {
		t.Fatal("조치 기록 수가 없다")
	}

	xlsx := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/knowledge.xlsx?q="+url.QueryEscape("예약대출기"), nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(xlsx, req)
	if xlsx.Code != http.StatusOK {
		t.Fatalf("xlsx %d", xlsx.Code)
	}
	ctype := xlsx.Header().Get("Content-Type")
	if !strings.Contains(ctype, "spreadsheet") {
		t.Fatalf("xlsx type=%s", ctype)
	}
	if xlsx.Body.Len() < 100 {
		t.Fatal("xlsx 가 비었다")
	}

	cust := get("/customers/c1")
	if !strings.Contains(cust, "AS 이력") || !strings.Contains(cust, "/as/knowledge?tab=site") {
		t.Fatal("고객현황에 AS 이력이 없다")
	}
}

func TestCustomerTemplateHasASHistory(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("web", "templates", "customer", "show.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "AS 이력") {
		t.Fatal("고객 상세에 AS 이력 버튼이 없다")
	}
}
