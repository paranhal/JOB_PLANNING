package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestDeliveriesHTTP_GoodsSplitSerialConsumableSkippedUnplanned(t *testing.T) {
	e, db := newSalesServerDB(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','세종시교육청','세종시교육청',1)`); err != nil {
		t.Fatal(err)
	}
	items := repository.NewSalesItemRepo(db)
	gate := &model.SalesItem{Name: "RFID 게이트", ItemKind: model.SalesItemKindGoods, Category: model.SalesItemCatRFID, Model: "EZ", Manufacturer: "앤로보틱스", IsActive: true}
	paper := &model.SalesItem{Name: "감열지", ItemKind: model.SalesItemKindGoods, Category: model.SalesItemCatConsumable, IsActive: true}
	if err := items.Create(gate); err != nil {
		t.Fatal(err)
	}
	if err := items.Create(paper); err != nil {
		t.Fatal(err)
	}

	form := quoteLineForm([]string{"RFID 게이트"}, []int{1000000})
	form.Set("customer_id", "c1")
	form.Set("line_item_id", gate.ItemID)
	form.Set("line_qty", "3")
	rec := doForm(t, e, "/quotes", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("견적 status=%d %s", rec.Code, rec.Body.String())
	}
	qid := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	conv := doForm(t, e, "/quotes/"+qid+"/order", url.Values{})
	if conv.Code != http.StatusSeeOther {
		t.Fatalf("수주 status=%d", conv.Code)
	}
	oid := orderIDFromRedirect(t, conv.Header().Get("Location"))
	o, err := repository.NewOrderRepo(db).Get(oid)
	if err != nil || len(o.Lines) != 1 {
		t.Fatalf("수주 %+v err=%v", o, err)
	}

	del := doForm(t, e, "/orders/"+oid+"/deliveries", url.Values{
		"line_id":          {o.Lines[0].LineID},
		"qty":              {"3"},
		"delivery_date":    {"2026-09-04"},
		"place":            {"로비"},
		"receiver_name":    {"김사서"},
		"installed_by":     {"양기헌"},
		"install_location": {"1층"},
		"serial":           {"SN-A", "SN-B", "SN-C"},
	})
	if del.Code != http.StatusSeeOther {
		t.Fatalf("납품 status=%d %s", del.Code, del.Body.String())
	}
	show := doGet(t, e, "/orders/"+oid)
	if show.Code != http.StatusOK {
		t.Fatalf("상세 %d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, "SN-A") || !strings.Contains(body, "SN-B") || !strings.Contains(body, "SN-C") {
		t.Fatalf("시리얼이 화면에 없다: %s", clipBody(body))
	}
	if !strings.Contains(body, "ref_type\" value=\"sales_delivery") && !strings.Contains(body, `value="sales_delivery"`) {
		t.Fatal("납품 첨부 ref_type 이 없다")
	}

	assets, _ := repository.NewAssetRepo(db).ListBySalesOrder(oid)
	if len(assets) != 3 {
		t.Fatalf("자산=%d", len(assets))
	}

	form2 := quoteLineForm([]string{"감열지"}, []int{150000})
	form2.Set("customer_id", "c1")
	form2.Set("line_item_id", paper.ItemID)
	rec2 := doForm(t, e, "/quotes", form2)
	qid2 := quoteIDFromRedirect(t, rec2.Header().Get("Location"))
	conv2 := doForm(t, e, "/quotes/"+qid2+"/order", url.Values{})
	oid2 := orderIDFromRedirect(t, conv2.Header().Get("Location"))
	o2, _ := repository.NewOrderRepo(db).Get(oid2)
	del2 := doForm(t, e, "/orders/"+oid2+"/deliveries", url.Values{
		"line_id": {o2.Lines[0].LineID}, "qty": {"2"}, "delivery_date": {"2026-09-04"}, "serial": {"X"},
	})
	if del2.Code != http.StatusSeeOther {
		t.Fatalf("소모품 납품 %d", del2.Code)
	}
	allPaper, _ := repository.NewAssetRepo(db).ListBySalesOrder(oid2)
	if len(allPaper) != 0 {
		t.Fatalf("소모품 자산=%d", len(allPaper))
	}

	labor := &model.SalesItem{Name: "응용SW개발", ItemKind: model.SalesItemKindLabor, IsActive: true}
	asjob := &model.SalesItem{Name: "AS 출장", ItemKind: model.SalesItemKindService, IsActive: true}
	if err := items.Create(labor); err != nil {
		t.Fatal(err)
	}
	if err := items.Create(asjob); err != nil {
		t.Fatal(err)
	}
	for _, it := range []*model.SalesItem{labor, asjob} {
		f := quoteLineForm([]string{it.Name}, []int{1})
		f.Set("customer_id", "c1")
		f.Set("line_item_id", it.ItemID)
		r := doForm(t, e, "/quotes", f)
		qidN := quoteIDFromRedirect(t, r.Header().Get("Location"))
		cN := doForm(t, e, "/quotes/"+qidN+"/order", url.Values{})
		oidN := orderIDFromRedirect(t, cN.Header().Get("Location"))
		oN, _ := repository.NewOrderRepo(db).Get(oidN)
		dN := doForm(t, e, "/orders/"+oidN+"/deliveries", url.Values{
			"line_id": {oN.Lines[0].LineID}, "qty": {"1"}, "delivery_date": {"2026-09-04"}, "serial": {"NO"},
		})
		if dN.Code != http.StatusSeeOther {
			t.Fatalf("%s 납품 %d", it.Name, dN.Code)
		}
		got, _ := repository.NewAssetRepo(db).ListBySalesOrder(oidN)
		if len(got) != 0 {
			t.Fatalf("%s 자산=%d", it.Name, len(got))
		}
	}

	empty := doForm(t, e, "/orders/"+oid+"/deliveries", url.Values{
		"line_id": {o.Lines[0].LineID}, "qty": {"1"}, "delivery_date": {"2026-09-05"},
	})
	if empty.Code != http.StatusSeeOther {
		t.Fatalf("빈 시리얼 납품 %d", empty.Code)
	}
	up := doGet(t, e, "/plan/unplanned")
	if up.Code != http.StatusOK {
		t.Fatalf("미계획 %d", up.Code)
	}
	if !strings.Contains(up.Body.String(), "자산 시리얼 미입력") {
		t.Fatalf("미계획함에 없음: %s", clipBody(up.Body.String()))
	}
}
