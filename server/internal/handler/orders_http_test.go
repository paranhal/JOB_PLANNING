package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func orderIDFromRedirect(t *testing.T, loc string) string {
	t.Helper()
	path := strings.Split(loc, "?")[0]
	id := strings.TrimPrefix(path, "/orders/")
	id = strings.TrimSuffix(id, "/edit")
	if id == "" || strings.Contains(id, "/") {
		t.Fatalf("order id 없음: %q", loc)
	}
	return id
}

func doGetAs(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func doGetJSONAs(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) map[string]interface{} {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("json GET %s status=%d %s", path, rec.Code, rec.Body.String())
	}
	var d map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestOrdersHTTP_FromQuoteCopyDiffAndHideCostFromTech(t *testing.T) {
	e, quotes := newQuoteServer(t)
	form := quoteLineForm([]string{"감열지"}, []int{150000})
	form.Set("line_qty", "2")
	rec := doForm(t, e, "/quotes", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("견적 저장 status=%d %s", rec.Code, rec.Body.String())
	}
	qid := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	q, err := quotes.Get(qid)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Lines) != 1 || q.Lines[0].Qty != 2 {
		t.Fatalf("견적 라인 %+v", q.Lines)
	}

	conv := doForm(t, e, "/quotes/"+qid+"/order", url.Values{})
	if conv.Code != http.StatusSeeOther {
		t.Fatalf("수주 전환 status=%d %s", conv.Code, conv.Body.String())
	}
	oid := orderIDFromRedirect(t, conv.Header().Get("Location"))

	q2, _ := quotes.Get(qid)
	if q2.Status != model.QuoteStatusWon {
		t.Fatalf("견적 상태=%s", q2.Status)
	}

	adminCK := jwtCookie(t)
	d := doGetJSONAs(t, e, "/orders/"+oid, adminCK)
	lines, _ := d["lines"].([]interface{})
	if len(lines) != 1 {
		t.Fatalf("복사 라인=%v", d["lines"])
	}
	ln, _ := lines[0].(map[string]interface{})
	if ln["name"] != "감열지" || ln["qty"] != 2.0 || ln["unit_price"] != 150000.0 {
		t.Fatalf("라인 복사가 다르다 %+v", ln)
	}
	if ln["differs"] == true {
		t.Fatal("그대로인데 견적과 다름")
	}

	show := doGet(t, e, "/orders/"+oid)
	if show.Code != http.StatusOK {
		t.Fatalf("상세 status=%d", show.Code)
	}
	body := show.Body.String()
	if strings.Contains(body, "견적과 다름") {
		t.Fatal("복사 직후 견적과 다름이 떴다")
	}
	if strings.Contains(body, "billing_status") || strings.Contains(body, "invoice_no") ||
		strings.Contains(body, "paid_amount") || strings.Contains(body, "invoiced_at") {
		t.Fatal("청구 컬럼이 화면에 있다")
	}

	upd := doForm(t, e, "/orders/"+oid, url.Values{
		"line_id":    {strMap(ln, "line_id")},
		"line_qty":   {"3"},
		"line_price": {"150000"},
		"line_unit":  {"EA"},
		"title":      {"감열지 수주"},
	})
	if upd.Code != http.StatusSeeOther {
		t.Fatalf("수정 status=%d %s", upd.Code, upd.Body.String())
	}
	after := doGet(t, e, "/orders/"+oid)
	if !strings.Contains(after.Body.String(), "견적과 다름") {
		t.Fatalf("수량 고쳤는데 견적과 다름이 없다: %s", clipBody(after.Body.String()))
	}

	buy := doForm(t, e, "/orders/"+oid+"/purchases", url.Values{
		"line_id":       {strMap(ln, "line_id")},
		"supplier_name": {"앤로보틱스"},
		"qty":           {"1"},
		"cost_price":    {"9876543"},
		"purchase_date": {"2026-09-01"},
		"status":        {"ordered"},
	})
	if buy.Code != http.StatusSeeOther {
		t.Fatalf("발주 status=%d %s", buy.Code, buy.Body.String())
	}

	adminShow := doGet(t, e, "/orders/"+oid)
	ab := adminShow.Body.String()
	if !strings.Contains(ab, "9,876,543") {
		t.Fatalf("관리자 화면에 매입가가 없다: %s", clipBody(ab))
	}
	if !strings.Contains(ab, "매입가") || !strings.Contains(ab, "마진") {
		t.Fatal("관리자에게 매입가·마진 라벨이 없다")
	}

	adminJSON := doGetJSONAs(t, e, "/orders/"+oid, adminCK)
	if adminJSON["cost_total"] == nil || adminJSON["margin"] == nil {
		t.Fatalf("관리자 JSON에 마진이 없다 %+v", adminJSON)
	}
	purch, _ := adminJSON["purchases"].([]interface{})
	if len(purch) != 1 {
		t.Fatalf("발주=%v", adminJSON["purchases"])
	}
	prow, _ := purch[0].(map[string]interface{})
	if prow["cost_price"] != 9876543.0 {
		t.Fatalf("매입가=%v", prow["cost_price"])
	}

	techCK := jwtCookieRole(t, model.RoleTech)
	techHTML := doGetAs(t, e, "/orders/"+oid, techCK)
	if techHTML.Code != http.StatusOK {
		t.Fatalf("기술담당 상세 status=%d %s", techHTML.Code, techHTML.Body.String())
	}
	tb := techHTML.Body.String()
	if strings.Contains(tb, "9876543") || strings.Contains(tb, "9,876,543") {
		t.Fatal("기술담당 HTML에 매입가 숫자가 있다")
	}
	if strings.Contains(tb, "매입가") {
		t.Fatal("기술담당 화면에 매입가 라벨이 있다")
	}

	techJSON := doGetJSONAs(t, e, "/orders/"+oid, techCK)
	if _, ok := techJSON["cost_total"]; ok {
		t.Fatal("기술담당 JSON에 cost_total이 있다")
	}
	if _, ok := techJSON["margin"]; ok {
		t.Fatal("기술담당 JSON에 margin이 있다")
	}
	raw := mustJSON(t, techJSON)
	if strings.Contains(raw, "9876543") || strings.Contains(raw, "cost_price") || strings.Contains(raw, `"cost"`) {
		t.Fatalf("기술담당 JSON에 매입가 키가 있다: %s", raw)
	}
}

func TestOrdersHTTP_KanbanFourColumns(t *testing.T) {
	e, _ := newQuoteServer(t)
	kanban := doGet(t, e, "/orders?display=kanban")
	if kanban.Code != http.StatusOK {
		t.Fatalf("칸반 status=%d", kanban.Code)
	}
	kb := kanban.Body.String()
	if strings.Count(kb, "flex-1 basis-0") != 4 {
		t.Fatalf("수주 칸반 열=%d", strings.Count(kb, "flex-1 basis-0"))
	}
	for _, title := range []string{"수주", "발주", "입고", "납품 완료"} {
		if !strings.Contains(kb, title) {
			t.Fatalf("열 %s 없음", title)
		}
	}
}

func strMap(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
