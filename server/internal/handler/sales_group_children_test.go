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
	"customer-support/internal/repository"
)

func TestSalesGroupBulkChildrenAndAutoClose(t *testing.T) {
	e, db := newSalesServerDB(t)
	if _, err := db.Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id)
		VALUES ('SC1','천안시공공도서관','천안시공공도서관',1,?),
		       ('SC2','아산시공공도서관','아산시공공도서관',1,?),
		       ('SC3','공주시공공도서관','공주시공공도서관',1,?)`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewSalesRepo(db)
	g := &model.SalesGroup{Name: "윈세프 360 영업", Year: 2026, GroupKind: model.SalesGroupManual, OwnerID: model.OrgIDLibrary}
	if err := repo.CreateGroup(g); err != nil {
		t.Fatal(err)
	}
	form := url.Values{}
	form.Set("name_template", "{고객명} 윈세프 360 구매 사업")
	form.Add("customer_id", "SC1")
	form.Add("customer_id", "SC2")
	form.Add("customer_id", "SC3")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/sales/groups/"+g.GroupID+"/children", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d %s", rec.Code, rec.Body.String())
	}
	var out salesChildBatchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Created) != 3 {
		t.Fatalf("created %d skipped %v failed %v %s", len(out.Created), out.Skipped, out.Failed, rec.Body.String())
	}
	ps, _ := repo.ResolveGroupProjects(g)
	if len(ps) != 3 {
		t.Fatalf("members %d", len(ps))
	}
	for _, p := range ps {
		if p.OrgID != model.OrgIDLibrary {
			t.Fatalf("org %s", p.OrgID)
		}
		if !strings.Contains(p.Name, "윈세프 360 구매 사업") {
			t.Fatalf("name %s", p.Name)
		}
	}

	bad := url.Values{}
	bad.Set("name_template", "이름만")
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://localhost/sales/groups/"+g.GroupID+"/children", strings.NewReader(bad.Encode()))
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("template 400 want, got %d", rec2.Code)
	}

	again := url.Values{}
	again.Set("name_template", "{고객명} 윈세프 360 구매 사업")
	again.Add("customer_id", "SC1")
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "http://localhost/sales/groups/"+g.GroupID+"/children", strings.NewReader(again.Encode()))
	req3.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req3.Header.Set("Accept", "application/json")
	req3.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec3, req3)
	var out3 salesChildBatchResult
	_ = json.Unmarshal(rec3.Body.Bytes(), &out3)
	if len(out3.Skipped) == 0 {
		t.Fatalf("중복 건너뜀 없음 %+v", out3)
	}

	for i, p := range ps {
		if i == 2 {
			break
		}
		if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, status=? WHERE sales_id=?`,
			model.SalesStage4Closed, model.SalesCloseDropped, model.SalesStatusDropped, p.SalesID); err != nil {
			t.Fatal(err)
		}
	}
	g2, _ := repo.GetGroup(g.GroupID)
	if g2.Status != model.SalesGroupActive {
		t.Fatalf("2건만 결정인데 종료됨")
	}
	last := ps[2]
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, status=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseDropped, model.SalesStatusDropped, last.SalesID); err != nil {
		t.Fatal(err)
	}
	h := New(db)
	msg := h.Sales.afterSalesChildChanged(last.SalesID)
	if msg == "" {
		t.Fatal("3번째 결정 후 자동 종료 메시지 없음")
	}
	g3, _ := repo.GetGroup(g.GroupID)
	if g3.Status != model.SalesGroupClosed {
		t.Fatalf("status %s", g3.Status)
	}
}
