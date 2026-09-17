package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestASListAndShowQueryPath(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("internal", "handler", "as.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	listFn := handlerMethod(text, "func (h *ASHandler) List")
	showFn := handlerMethod(text, "func (h *ASHandler) Show")
	actionFn := handlerMethod(text, "func (h *ASHandler) Action")
	if strings.Contains(listFn, "CloseOpen") || strings.Contains(listFn, ".Exec(") || strings.Contains(listFn, "UPDATE ") {
		t.Fatal("목록 조회 경로에서 쓴다")
	}
	if strings.Contains(showFn, "ListHistoryByAsset") {
		t.Fatal("상세 첫 페인트가 장비 이력을 읽는다")
	}
	if strings.Contains(actionFn, "loadSimilarCases") {
		t.Fatal("조치 첫 페인트가 비슷한 사례를 읽는다")
	}
}

func handlerMethod(src, sig string) string {
	i := strings.Index(src, sig)
	if i < 0 {
		return ""
	}
	rest := src[i:]
	if j := strings.Index(rest[1:], "func (h *ASHandler)"); j >= 0 {
		return rest[:j+1]
	}
	return rest
}

func TestASShowDefersAssetHistoryAndActionDefersSimilar(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "as_perf.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','성능도서관','성능도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('A1','c1','무인예약기')`); err != nil {
		t.Fatal(err)
	}
	asRepo := repository.NewASRepo(db)
	open := &model.ASReceipt{
		CustomerID: "c1", AssetID: "A1", Symptom: "무인예약이 안 됩니다",
		ReceiptDatetime: time.Now(), Urgency: "normal", Priority: "normal",
	}
	if err := asRepo.Create(open); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as", h.AS.List)
	g.GET("/as/:id", h.AS.Show)
	g.GET("/as/:id/action", h.AS.Action)
	g.GET("/as/:id/similar-panel", h.AS.SimilarPanel)

	list := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(list, req)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d", list.Code)
	}
	if !strings.Contains(list.Body.String(), open.ASNumber) {
		t.Fatal("목록에 접수가 없다")
	}

	show := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/as/"+open.ASID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req)
	if show.Code != http.StatusOK {
		t.Fatalf("show %d", show.Code)
	}
	sb := show.Body.String()
	if !strings.Contains(sb, "/api/as/asset-history/") {
		t.Fatal("상세가 이력 API를 부르지 않는다")
	}
	if !strings.Contains(sb, "버튼을 누르면 이력을 불러옵니다") {
		t.Fatal("상세 첫 HTML이 이력을 나중으로 미루지 않았다")
	}

	act := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/as/"+open.ASID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(act, req)
	if act.Code != http.StatusOK {
		t.Fatalf("action %d", act.Code)
	}
	ab := act.Body.String()
	if !strings.Contains(ab, "비슷한 사례") || !strings.Contains(ab, "/as/"+open.ASID+"/similar-panel") {
		t.Fatal("조치 화면에 비슷한 사례 지연 훅이 없다")
	}
}
