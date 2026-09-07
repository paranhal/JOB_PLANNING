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

func newASSearchHTTP(t *testing.T) (*echo.Echo, *repository.ASRepo, *repository.ASProcessRepo, *repository.ASKeywordRepo) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "search.db"))
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
		CustomerID: "c1", AssetID: "A1", Symptom: "무인예약이 안 됩니다",
		ReceiptDatetime: time.Date(2020, 4, 2, 9, 0, 0, 0, time.Local),
		Urgency:         "normal", Priority: "normal",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := proc.Create(&model.ASProcess{ASID: as.ASID, WorkContent: "설정 확인 후 재시작", TimeSpent: 15}); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/search", h.AS.Search)
	g.GET("/as/similar", h.AS.Similar)
	g.GET("/as/keywords/suggest", h.AS.KeywordSuggest)
	g.GET("/as/keywords", h.AS.KeywordList)
	g.GET("/as/:id/action", h.AS.Action)
	g.GET("/as/new", h.AS.New)
	return e, asRepo, proc, repository.NewASKeywordRepo(db)
}

func TestASSearchPageShowsSymptomAndAction(t *testing.T) {
	e, _, _, _ := newASSearchHTTP(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/search?q="+url.QueryEscape("무인예약"), nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<mark>무인예약</mark>") {
		t.Fatal("검색어 강조가 없다")
	}
	if !strings.Contains(body, "재시작") {
		t.Fatal("조치가 결과에 함께 보여야 한다")
	}
	if !strings.Contains(body, "2020-04-02") {
		t.Fatal("기준일 이전 건도 찾아야 한다")
	}
}

func TestASSimilarJSONSameAsset(t *testing.T) {
	e, asRepo, _, _ := newASSearchHTTP(t)
	src, _, err := asRepo.SearchAS(model.ASSearchFilter{Query: "무인예약", PageSize: 5})
	if err != nil || len(src) == 0 {
		t.Fatalf("seed: %v n=%d", err, len(src))
	}
	done := src[0]
	got, _ := asRepo.GetByID(done.ASID)
	got.Status = "completed"
	_ = asRepo.Update(got)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"http://localhost/as/similar?q="+url.QueryEscape("무인예약")+"&customer_id=c1&asset_id=A1", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []model.ASSimilarCase `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) == 0 {
		t.Fatal("비슷한 사례 JSON이 비었다")
	}
	if !out.Items[0].SameAsset {
		t.Fatalf("같은 자산이 위: %+v", out.Items[0])
	}
	if !out.Items[0].CanReopen {
		t.Fatal("완료+같은 자산이면 재접수 제안")
	}
}

func TestASActionShowsSimilarWhenPresent(t *testing.T) {
	e, asRepo, _, _ := newASSearchHTTP(t)
	open := &model.ASReceipt{
		CustomerID: "c1", AssetID: "A1", Symptom: "무인예약이 또 안 됩니다",
		ReceiptDatetime: time.Now(), Urgency: "normal", Priority: "normal",
	}
	if err := asRepo.Create(open); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+open.ASID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "비슷한 사례") {
		t.Fatal("조치 화면에 비슷한 사례가 있어야 한다")
	}
}

func TestASNewHasSimilarHook(t *testing.T) {
	e, _, _, _ := newASSearchHTTP(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/new", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "as-similar-panel") || !strings.Contains(body, "/as/similar") {
		t.Fatal("접수 화면에 비슷한 사례 훅이 없다")
	}
}
