package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_126QuickQuoteFlow(t *testing.T) {
	e, db := newSalesServerDB(t)
	if _, err := db.Exec(`UPDATE users SET mobile='010-1111-2222' WHERE username='admin'`); err != nil {
		t.Fatal(err)
	}
	cust := &model.Customer{OrgName: "빠른견적고객", OfficialName: "빠른견적고객", IsActive: true, OrgID: model.OrgIDLibrary}
	if err := repository.NewCustomerRepo(db).Create(cust); err != nil {
		t.Fatal(err)
	}
	ct := &model.Contact{CustomerID: cust.CustomerID, FullName: "김수신", Mobile: "010-3333-4444", Email: "k@ex.kr", Status: "active"}
	if err := repository.NewContactRepo(db).Create(ct); err != nil {
		t.Fatal(err)
	}

	page := doGet(t, e, "/sales/new")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "등록 유형") || !strings.Contains(page.Body.String(), "빠른 견적") {
		t.Fatalf("T1 /sales/new: %d", page.Code)
	}
	pipe := doGet(t, e, "/sales?view=pipeline")
	body := pipe.Body.String()
	iDeal := strings.Index(body, "영업 사업 등록")
	iQQ := strings.Index(body[iDeal+1:], "빠른 견적")
	if iDeal < 0 || iQQ < 0 {
		t.Fatalf("T1 파이프라인 버튼 순서")
	}

	qq := doGet(t, e, "/sales/quick-quote")
	if qq.Code != http.StatusOK || !strings.Contains(qq.Body.String(), "신규 거래처 등록") {
		t.Fatalf("빠른 견적 화면 %d", qq.Code)
	}

	miss := doFormJSON(t, e, "/sales/quick-quote", url.Values{
		"title": {"x"},
	})
	if miss.Code != http.StatusBadRequest {
		t.Fatalf("T2 필수누락 status=%d %s", miss.Code, miss.Body.String())
	}

	form := url.Values{
		"customer_id":  {cust.CustomerID},
		"customer_name": {cust.OrgName},
		"attn_name":    {ct.FullName},
		"contact_id":   {ct.ContactID},
		"contact_line": {"010-3333-4444 / k@ex.kr"},
		"title":        {"리더기 외 1건"},
		"title_manual": {"1"},
		"line_name":    {"리더기", "카드"},
		"line_spec":    {"A", "B"},
		"line_qty":     {"1", "1"},
		"line_unit":    {"EA", "EA"},
		"line_price":   {"37035", "9999"},
		"vat_mode":     {"excluded"},
		"round_rule":   {"thousand"},
		"cut_scope":    {"total"},
	}
	rec := doFormJSON(t, e, "/sales/quick-quote", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("저장 %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	qid, _ := out["quote_id"].(string)
	sid, _ := out["sales_id"].(string)
	if qid == "" || sid == "" {
		t.Fatalf("ids %+v", out)
	}
	q, err := repository.NewQuoteRepo(db).Get(qid)
	if err != nil {
		t.Fatal(err)
	}
	if q.Total != 51000 {
		t.Fatalf("T7 합계=%d", q.Total)
	}
	if q.Status != model.QuoteStatusDraft {
		t.Fatalf("draft=%s", q.Status)
	}
	p, err := repository.NewSalesRepo(db).Get(sid)
	if err != nil {
		t.Fatal(err)
	}
	if p.DealType != model.SalesDealSupply || p.Stage != model.SalesStageInquiry || p.CreatedVia != model.SalesCreatedViaQuickQuote {
		t.Fatalf("사업 type=%s stage=%s via=%s", p.DealType, p.Stage, p.CreatedVia)
	}
	if p.ExpectedAmount != q.Total {
		t.Fatalf("예상금액 %d != %d", p.ExpectedAmount, q.Total)
	}
	show := doGet(t, e, "/sales/"+sid)
	if !strings.Contains(show.Body.String(), "빠른 견적으로 등록") {
		t.Fatal("사업 상세 안내 없음")
	}

	badSend := doFormJSON(t, e, "/quotes/"+qid+"/mark-sent", url.Values{})
	if badSend.Code != http.StatusBadRequest {
		t.Fatalf("다운로드 전 발송 %d %s", badSend.Code, badSend.Body.String())
	}

	dl := doGet(t, e, "/quotes/"+qid+"/xlsx")
	if dl.Code != http.StatusOK {
		t.Fatalf("xlsx %d", dl.Code)
	}
	q2, _ := repository.NewQuoteRepo(db).Get(qid)
	if strings.TrimSpace(q2.DownloadedAt) == "" {
		t.Fatal("downloaded_at 없음")
	}
	okSend := doFormJSON(t, e, "/quotes/"+qid+"/mark-sent", url.Values{})
	if okSend.Code != http.StatusOK {
		t.Fatalf("발송 %d %s", okSend.Code, okSend.Body.String())
	}
	q3, _ := repository.NewQuoteRepo(db).Get(qid)
	if q3.Status != model.QuoteStatusSent {
		t.Fatalf("sent=%s", q3.Status)
	}
	p2, _ := repository.NewSalesRepo(db).Get(sid)
	if p2.Stage != model.SalesStageQuoted {
		t.Fatalf("견적 제출=%s", p2.Stage)
	}

	dup := doFormJSON(t, e, "/sales/quick-quote", url.Values{
		"new_customer": {"1"},
		"customer_name": {cust.OrgName},
		"attn_name":    {"새담당"},
		"contact_line": {"010-5555-6666"},
		"line_name":    {"품"},
		"line_qty":     {"1"},
		"line_price":   {"1000"},
	})
	if dup.Code != http.StatusBadRequest || !strings.Contains(dup.Body.String(), "이미 등록된 거래처") {
		t.Fatalf("중복 거래처 %d %s", dup.Code, dup.Body.String())
	}

	newc := doFormJSON(t, e, "/sales/quick-quote", url.Values{
		"new_customer": {"1"},
		"customer_name": {"신규거래처126"},
		"attn_name":    {"담당신규"},
		"contact_line": {"010-7777-8888 / n@ex.kr"},
		"line_name":    {"스캐너"},
		"line_qty":     {"1"},
		"line_price":   {"10000"},
		"vat_mode":     {"excluded"},
		"round_rule":   {"none"},
	})
	if newc.Code != http.StatusOK {
		t.Fatalf("신규 거래처 %d %s", newc.Code, newc.Body.String())
	}

	item := doFormJSON(t, e, "/items/quote-line", url.Values{
		"name": {"QQ마스터품"}, "spec": {"S1"}, "unit": {"EA"}, "unit_price": {"5000"}, "register": {"1"},
	})
	if item.Code != http.StatusOK || !strings.Contains(item.Body.String(), `"registered":true`) && !strings.Contains(item.Body.String(), "item_id") {
		t.Fatalf("마스터 등록 %s", item.Body.String())
	}
	dupItem := doFormJSON(t, e, "/items/quote-line", url.Values{
		"name": {"QQ마스터품"}, "spec": {"S1"}, "unit": {"EA"}, "unit_price": {"9000"}, "register": {"1"},
	})
	if !strings.Contains(dupItem.Body.String(), "이미 등록된 품목") {
		t.Fatalf("중복 품목 %s", dupItem.Body.String())
	}
}

func Test95_126QuickQuoteRollback(t *testing.T) {
	e, db := newSalesServerDB(t)
	_ = e
	if _, err := db.Exec(`UPDATE users SET mobile='010-1111-2222' WHERE username='admin'`); err != nil {
		t.Fatal(err)
	}
	in := repository.QuickQuoteInput{
		OrgID:        model.OrgIDLibrary,
		NewCustomer:  true,
		CustomerName: "롤백거래처",
		AttnName:     "롤백담당",
		ContactLine:  "010-1234-5678",
		AddContact:   true,
		Project:      &model.SalesProject{Name: "롤백사업", OrgID: model.OrgIDLibrary, SalesOwner: "관리자"},
		Quote: &model.SalesQuote{
			Title: "롤백", OwnerName: "관리자", OwnerPhone: "",
			Lines: []model.SalesQuoteLine{{Name: "x", Qty: 1, UnitPrice: 100}},
			VATMode: model.QuoteVATExcluded, RoundRule: model.QuoteRoundNone,
		},
	}
	_, err := repository.SaveQuickQuote(db, in)
	if err == nil {
		t.Fatal("연락처 없이 저장되면 안 됨")
	}
	hits, err := repository.NewCustomerRepo(db).ListActiveNameMatches("롤백거래처", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("롤백 안 됨 %+v", hits)
	}
}
