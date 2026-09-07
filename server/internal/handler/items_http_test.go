package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

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
	if item["last_price"] != float64(220000) || item["last_customer"] != "공주시립도서관" {
		t.Fatalf("최근 단가: %+v", item)
	}

	after := doGet(t, e, "/items/"+itemID+"/edit")
	if after.Code != http.StatusOK {
		t.Fatalf("수정 폼 status=%d", after.Code)
	}
	ab := after.Body.String()
	if !strings.Contains(ab, "150000") || !strings.Contains(ab, "세종시립도서관") {
		t.Fatalf("고른 품목의 최근 단가가 안 남았다: %s", clipBody(ab))
	}

	rec := doForm(t, e, "/sales", url.Values{
		"name":      {"감열지 단품"},
		"deal_type": {"supply"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("단품 등록 status=%d", rec.Code)
	}
	sid := salesIDFromRedirect(t, rec.Header().Get("Location"))
	show := doGet(t, e, "/sales/"+sid)
	if show.Code != http.StatusOK {
		t.Fatalf("단품 상세 status=%d %s", show.Code, clipBody(show.Body.String()))
	}
	sb := show.Body.String()
	if !strings.Contains(sb, "품목으로 등록할까요?") || !strings.Contains(sb, "없는 품목은 그대로 쳐도") {
		t.Fatalf("단품 상세에 견적 라인 피커가 없다: %s", clipBody(sb))
	}
}
