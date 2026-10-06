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

func TestItemsHTTP_SeedSuggestQuoteLineKanban(t *testing.T) {
	e, db := newSalesServerDB(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	for _, row := range [][]string{
		{"AS1", "자동대출반납기용 감열지", "59x80mm", "나이콤", "materials", "SN1"},
		{"AS2", "자동대출반납기용 감열지", "59x80mm", "나이콤", "materials", "SN2"},
	} {
		if _, err := db.Exec(`
			INSERT INTO assets (asset_id, customer_id, product_name, model_name, manufacturer, product_category, serial_number)
			VALUES (?,?,?,?,?,?,?)`, row[0], "c1", row[1], row[2], row[3], row[4], row[5]); err != nil {
			t.Fatal(err)
		}
	}
	if err := repository.SeedSalesItemsFromAssets(db); err != nil {
		t.Fatal(err)
	}

	list := doGet(t, e, "/items")
	if list.Code != http.StatusOK {
		t.Fatalf("/items status=%d %s", list.Code, clipBody(list.Body.String()))
	}
	lb := list.Body.String()
	if !strings.Contains(lb, "확인 필요") || !strings.Contains(lb, "자동대출반납기용 감열지") {
		t.Fatalf("시드 품목이 없다: %s", clipBody(lb))
	}
	if !strings.Contains(lb, "품목으로 등록할까요?") {
		t.Fatal("견적 라인 자유 입력 안내가 없다")
	}

	kanban := doGet(t, e, "/items?display=kanban")
	if kanban.Code != http.StatusOK {
		t.Fatalf("칸반 status=%d", kanban.Code)
	}
	kb := kanban.Body.String()
	if strings.Count(kb, "flex-1 basis-0") != 4 {
		t.Fatalf("품목 칸반 열=%d", strings.Count(kb, "flex-1 basis-0"))
	}
	if !strings.Contains(kb, "물품") || !strings.Contains(kb, "AS·작업") ||
		!strings.Contains(kb, "개발 용역") || !strings.Contains(kb, "기타") {
		t.Fatalf("item_kind 열이 없다: %s", clipBody(kb))
	}

	sug := doGet(t, e, "/items/suggest?q="+url.QueryEscape("감열지"))
	if sug.Code != http.StatusOK {
		t.Fatalf("suggest status=%d", sug.Code)
	}
	var hits []map[string]interface{}
	if err := json.Unmarshal(sug.Body.Bytes(), &hits); err != nil || len(hits) < 1 {
		t.Fatalf("suggest: %s", sug.Body.String())
	}
	if hits[0]["spec"] != "59x80mm" || hits[0]["unit"] != "EA" {
		t.Fatalf("자동 채움 값: %+v", hits[0])
	}
	itemID, _ := hits[0]["item_id"].(string)
	if itemID == "" {
		t.Fatal("item_id 없음")
	}

	picked := doFormJSON(t, e, "/items/quote-line", url.Values{
		"item_id":    {itemID},
		"name":       {"자동대출반납기용 감열지"},
		"spec":       {"59x80mm"},
		"unit":       {"EA"},
		"unit_price": {"150000"},
		"customer":   {"세종시립도서관"},
		"quoted_at":  {"2026-08-16"},
	})
	if picked.Code != http.StatusOK {
		t.Fatalf("고른 라인 status=%d %s", picked.Code, picked.Body.String())
	}
	var pickResp map[string]interface{}
	if err := json.Unmarshal(picked.Body.Bytes(), &pickResp); err != nil {
		t.Fatal(err)
	}
	if pickResp["ok"] != true || pickResp["ask_register"] == true {
		t.Fatalf("고른 품목이 등록을 묻는다: %+v", pickResp)
	}

	free := doFormJSON(t, e, "/items/quote-line", url.Values{
		"name":       {"나이콤 감열지"},
		"spec":       {"1 Box(50Roll)"},
		"unit":       {"식"},
		"unit_price": {"220000"},
		"customer":   {"공주시립도서관"},
		"quoted_at":  {"2026-03-30"},
	})
	if free.Code != http.StatusOK {
		t.Fatalf("자유 입력 status=%d %s", free.Code, free.Body.String())
	}
	var freeResp map[string]interface{}
	if err := json.Unmarshal(free.Body.Bytes(), &freeResp); err != nil {
		t.Fatal(err)
	}
	if freeResp["ask_register"] != true || freeResp["prompt"] != "품목으로 등록할까요?" {
		t.Fatalf("등록을 안 묻는다: %+v", freeResp)
	}

	reg := doFormJSON(t, e, "/items/quote-line", url.Values{
		"name":       {"나이콤 감열지"},
		"spec":       {"1 Box(50Roll)"},
		"unit":       {"식"},
		"unit_price": {"220000"},
		"customer":   {"공주시립도서관"},
		"quoted_at":  {"2026-03-30"},
		"register":   {"1"},
	})
	if reg.Code != http.StatusOK {
		t.Fatalf("등록 status=%d %s", reg.Code, reg.Body.String())
	}
	var regResp map[string]interface{}
	if err := json.Unmarshal(reg.Body.Bytes(), &regResp); err != nil {
		t.Fatal(err)
	}
	if regResp["registered"] != true {
		t.Fatalf("등록 안 됨: %+v", regResp)
	}
	item, _ := regResp["item"].(map[string]interface{})
	if item["name"] != "나이콤 감열지" {
		t.Fatalf("등록 품목: %+v", item)
	}

	after := doGet(t, e, "/items/"+itemID+"/edit")
	if after.Code != http.StatusOK {
		t.Fatalf("수정 폼 status=%d", after.Code)
	}
	ab := after.Body.String()
	if strings.Contains(ab, "최근 견적 단가") || strings.Contains(ab, "최근 견적일") || strings.Contains(ab, "최근 견적 고객") {
		t.Fatalf("최근 견적 칸이 남았다: %s", clipBody(ab))
	}
	if strings.Contains(ab, "세종시립도서관") && strings.Contains(ab, "150000") && strings.Contains(ab, "최근") {
		t.Fatalf("고른 품목에 최근 견적이 복사됐다: %s", clipBody(ab))
	}
	if !strings.Contains(ab, "이 품목이 들어간 견적") {
		t.Fatalf("견적 목록이 없다: %s", clipBody(ab))
	}

	var lastPrice int
	_ = db.QueryRow(`SELECT last_price FROM sales_items WHERE item_id=?`, itemID).Scan(&lastPrice)
	if lastPrice != 0 {
		t.Fatalf("quote-line 이 last_price 를 바꿨다: %d", lastPrice)
	}

	listAfter := doGet(t, e, "/items")
	if !strings.Contains(listAfter.Body.String(), "없는 품목은 그대로 쳐도") {
		t.Fatalf("품목 목록에 견적 라인 피커가 없다: %s", clipBody(listAfter.Body.String()))
	}
}

func TestItemsHTTP_ManufacturerSearchAndPartner(t *testing.T) {
	e, db := newSalesServerDB(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, party_kind)
		VALUES ('C00-26-010','앤로보틱스','앤로보틱스',1,'partner')`); err != nil {
		t.Fatal(err)
	}
	search := doGet(t, e, "/customers/search?q="+url.QueryEscape("앤로"))
	if search.Code != http.StatusOK {
		t.Fatalf("search status=%d %s", search.Code, search.Body.String())
	}
	if !strings.Contains(search.Body.String(), "C00-26-010") {
		t.Fatalf("검색 실패: %s", search.Body.String())
	}

	created := doFormJSON(t, e, "/items/partners", url.Values{"name": {"나이콤"}})
	if created.Code != http.StatusOK {
		t.Fatalf("partner status=%d %s", created.Code, created.Body.String())
	}
	var cr map[string]interface{}
	if err := json.Unmarshal(created.Body.Bytes(), &cr); err != nil {
		t.Fatal(err)
	}
	cust, _ := cr["customer"].(map[string]interface{})
	if cr["created"] != true || cust["party_kind"] != "partner" || cust["customer_id"] == "" {
		t.Fatalf("새 거래처: %+v", cr)
	}

	dup := doFormJSON(t, e, "/items/partners", url.Values{"name": {"나이콤"}})
	var dr map[string]interface{}
	_ = json.Unmarshal(dup.Body.Bytes(), &dr)
	if dr["ask_confirm"] != true || dr["existed"] != true {
		t.Fatalf("같은 이름 확인 없음: %+v", dr)
	}
	use := doFormJSON(t, e, "/items/partners", url.Values{"name": {"나이콤"}, "confirm": {"1"}})
	var ur map[string]interface{}
	_ = json.Unmarshal(use.Body.Bytes(), &ur)
	existCust, _ := ur["customer"].(map[string]interface{})
	if existCust["customer_id"] != cust["customer_id"] {
		t.Fatalf("기존을 안 골랐다: %+v vs %+v", ur, cr)
	}

	rec := doForm(t, e, "/items", url.Values{
		"name":            {"RFID 게이트"},
		"item_kind":       {"goods"},
		"unit":            {"EA"},
		"is_active":       {"1"},
		"manufacturer_id": {"C00-26-010"},
		"manufacturer":    {"앤로보틱스"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("품목 등록 status=%d %s", rec.Code, rec.Body.String())
	}
	var itemID, mfrID string
	_ = db.QueryRow(`SELECT item_id, manufacturer_id FROM sales_items WHERE name='RFID 게이트'`).Scan(&itemID, &mfrID)
	if mfrID != "C00-26-010" {
		t.Fatalf("manufacturer_id=%q", mfrID)
	}
	form := doGet(t, e, "/items/"+itemID+"/edit")
	if !strings.Contains(form.Body.String(), "C00-26-010") {
		t.Fatalf("상세에 고객번호 없음: %s", clipBody(form.Body.String()))
	}
}

func TestItemsHTTP_SpecsCatalogRangeAndQuoteGuard(t *testing.T) {
	e, db := newSalesServerDB(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES
		('SUP1','공급A','공급A',1),('SUP2','공급B','공급B',1)`); err != nil {
		t.Fatal(err)
	}
	rec := doForm(t, e, "/items", url.Values{
		"name":       {"클리너카드"},
		"item_kind":  {"goods"},
		"is_active":  {"1"},
		"spec":       {"A.D형", "T형", "Box"},
		"spec_unit":  {"EA", "EA", "Box"},
		"spec_price": {"24000", "39600", "79200"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("3규격 저장 status=%d %s", rec.Code, rec.Body.String())
	}
	var itemID string
	_ = db.QueryRow(`SELECT item_id FROM sales_items WHERE name='클리너카드'`).Scan(&itemID)
	if itemID == "" {
		t.Fatal("품목 없음")
	}
	edit := doGet(t, e, "/items/"+itemID+"/edit")
	eb := edit.Body.String()
	if !strings.Contains(eb, "A.D형") || !strings.Contains(eb, "T형") || !strings.Contains(eb, "Box") {
		t.Fatalf("규격이 다시 안 열림: %s", clipBody(eb))
	}
	if !strings.Contains(eb, "공급사별 단가(매입)") || !strings.Contains(eb, "+ 규격 추가") {
		t.Fatalf("카탈로그 화면이 없다: %s", clipBody(eb))
	}

	zero := doForm(t, e, "/items", url.Values{"name": {"빈규격"}, "item_kind": {"goods"}, "is_active": {"1"}})
	if zero.Code != http.StatusSeeOther {
		t.Fatalf("0규격 저장 status=%d", zero.Code)
	}

	list := doGet(t, e, "/items?search="+url.QueryEscape("A.D형"))
	lb := list.Body.String()
	if !strings.Contains(lb, "클리너카드") || !strings.Contains(lb, "3개 규격 · 24,000~79,200원") {
		t.Fatalf("목록 범위/검색: %s", clipBody(lb))
	}

	var specIDs []string
	rows, err := db.Query(`SELECT spec_id FROM sales_item_specs WHERE item_id=? ORDER BY sort_order`, itemID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		specIDs = append(specIDs, id)
	}
	rows.Close()
	if len(specIDs) != 3 {
		t.Fatalf("spec_id=%v", specIDs)
	}
	supForm := url.Values{
		"name": {"클리너카드"}, "item_kind": {"goods"}, "is_active": {"1"},
		"spec_id": {specIDs[0], specIDs[1], specIDs[2]},
		"spec":    {"A.D형", "T형", "Box"}, "spec_unit": {"EA", "EA", "Box"},
		"spec_price": {"24000", "39600", "79200"},
		"sup_id":     {"SUP1", "SUP2"},
		"sp_group":   {"0", "0", "1", "1"},
		"sp_spec_id": {specIDs[0], specIDs[1], specIDs[0], specIDs[1]},
		"sp_unit":    {"", "", "", ""},
		"sp_price":   {"18000", "25000", "19000", "26000"},
	}
	upd := doForm(t, e, "/items/"+itemID, supForm)
	if upd.Code != http.StatusSeeOther {
		t.Fatalf("공급사 저장 status=%d %s", upd.Code, upd.Body.String())
	}
	var nPrice int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_item_supplier_prices WHERE item_id=?`, itemID).Scan(&nPrice)
	if nPrice != 4 {
		t.Fatalf("공급사 줄=%d", nPrice)
	}
	detail := doGet(t, e, "/items/"+itemID+"/edit")
	if !strings.Contains(detail.Body.String(), "규격별 이익") {
		t.Fatalf("이익이 없다: %s", clipBody(detail.Body.String()))
	}

	sug := doGet(t, e, "/items/suggest?q="+url.QueryEscape("클리너"))
	var hits []map[string]interface{}
	if err := json.Unmarshal(sug.Body.Bytes(), &hits); err != nil || len(hits) < 1 {
		t.Fatalf("suggest: %s", sug.Body.String())
	}
	specs, _ := hits[0]["specs"].([]interface{})
	if len(specs) != 3 {
		t.Fatalf("suggest specs=%v", hits[0]["specs"])
	}

	sales := repository.NewSalesRepo(db)
	p := &model.SalesProject{Name: "견적연결"}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	q := &model.SalesQuote{
		SalesID: p.SalesID, FormType: model.QuoteFormA, VATMode: model.QuoteVATExcluded,
		RoundRule: model.QuoteRoundNone, QuoteDate: "2026-09-27",
		Title: "클리너", OwnerName: "최혜영", OwnerPhone: "010-1111-2222", RecipientName: "도서관",
		Lines: []model.SalesQuoteLine{
			{Name: "클리너카드", SpecID: specIDs[0], Qty: 1, Unit: "EA", UnitPrice: 24000},
		},
	}
	if err := repository.NewQuoteRepo(db).Create(q); err != nil {
		t.Fatal(err)
	}
	offForm := url.Values{
		"name": {"클리너카드"}, "item_kind": {"goods"}, "is_active": {"1"},
		"spec_id": {specIDs[1], specIDs[2]},
		"spec":    {"T형", "Box"}, "spec_unit": {"EA", "Box"},
		"spec_price": {"39600", "79200"},
	}
	off := doForm(t, e, "/items/"+itemID, offForm)
	loc := off.Header().Get("Location")
	if off.Code != http.StatusSeeOther || !strings.Contains(loc, "ok=spec_off") {
		t.Fatalf("끄기 안내: status=%d loc=%s body=%s", off.Code, loc, off.Body.String())
	}
	flash := doGet(t, e, loc)
	if !strings.Contains(flash.Body.String(), "견적에 쓰여 끕니다") {
		t.Fatalf("안내 문구 없음: %s", clipBody(flash.Body.String()))
	}
	var active int
	_ = db.QueryRow(`SELECT COALESCE(is_active,1) FROM sales_item_specs WHERE spec_id=?`, specIDs[0]).Scan(&active)
	if active != 0 {
		t.Fatalf("견적 규격 is_active=%d", active)
	}
}
