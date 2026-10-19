package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestContractsHTTP_UnionAndAmountInput(t *testing.T) {
	e, db := newSalesServerDB(t)
	srepo := repository.NewSalesRepo(db)
	p := &model.SalesProject{Name: "미전환계약", ProspectName: "세종", ExpectedAmount: 220000}
	if err := srepo.Create(p); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, contract_amount=0, contracted_at=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, "2026-09-10", p.SalesID); err != nil {
		t.Fatal(err)
	}

	list := doGet(t, e, "/contracts")
	if list.Code != http.StatusOK {
		t.Fatalf("status=%d", list.Code)
	}
	b := list.Body.String()
	if !strings.Contains(b, "미전환계약") || !strings.Contains(b, "영업 계약") || !strings.Contains(b, "계약으로 전환") {
		t.Fatalf("합집합 목록 없음: %s", clipBody(b))
	}
	if !strings.Contains(b, "금액 미입력") {
		t.Fatal("금액 미입력 없음")
	}
	show := doGet(t, e, "/contracts/"+p.SalesID)
	if show.Code == http.StatusOK && strings.Contains(show.Body.String(), "미전환계약") {
		t.Fatal("영업 행이 계약 상세로 열리면 안 된다")
	}

	rec := doForm(t, e, "/sales/"+p.SalesID+"/contract-amount", url.Values{
		"contract_amount": {"220000"},
		"contracted_at":   {"2026-09-10"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("금액 입력 status=%d body=%s", rec.Code, rec.Body.String())
	}
	got, err := srepo.Get(p.SalesID)
	if err != nil || got.ContractAmount != 220000 {
		t.Fatalf("금액 저장 %+v %v", got, err)
	}

	roRec := httptest.NewRecorder()
	roReq := httptest.NewRequest(http.MethodPost, "http://localhost/sales/"+p.SalesID+"/contract-amount", strings.NewReader(url.Values{"contract_amount": {"1"}}.Encode()))
	roReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	roReq.AddCookie(jwtCookieReadOnly(t, model.RoleTech))
	e.ServeHTTP(roRec, roReq)
	if roRec.Code != http.StatusForbidden {
		t.Fatalf("읽기 전용 POST status=%d", roRec.Code)
	}

	wp := &model.WorkProject{Name: "상세회귀", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 100}
	if err := repository.NewProjectRepo(db).Create(wp); err != nil {
		t.Fatal(err)
	}
	detail := doGet(t, e, "/contracts/"+wp.ProjectID)
	if detail.Code != http.StatusOK {
		t.Fatalf("계약 상세 %d", detail.Code)
	}
}

func TestContractsHTTP_LinkSuggest(t *testing.T) {
	e, db := newSalesServerDB(t)
	srepo := repository.NewSalesRepo(db)
	prepo := repository.NewProjectRepo(db)
	s := &model.SalesProject{Name: "연결확인사업"}
	if err := srepo.Create(s); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, contract_amount=?, sales_no=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, 1000, "S2609-099", s.SalesID); err != nil {
		t.Fatal(err)
	}
	w := &model.WorkProject{Name: "연결확인사업", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 2000}
	if err := prepo.Create(w); err != nil {
		t.Fatal(err)
	}
	list := doGet(t, e, "/contracts")
	b := list.Body.String()
	if !strings.Contains(b, "연결 필요") || !strings.Contains(b, "금액 불일치") {
		t.Fatalf("제안 칩 없음: %s", clipBody(b))
	}
	form := doGet(t, e, "/contracts/"+w.ProjectID+"/link-sales?sales_id="+s.SalesID)
	if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), "연결확인사업") {
		t.Fatalf("확인 화면 %d", form.Code)
	}
	denied := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/contracts/"+w.ProjectID+"/link-sales", strings.NewReader(url.Values{"sales_id": {s.SalesID}, "confirm": {"1"}}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieReadOnly(t, model.RoleTech))
	e.ServeHTTP(denied, req)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("읽기 전용 연결 %d", denied.Code)
	}
	saved := doForm(t, e, "/contracts/"+w.ProjectID+"/link-sales", url.Values{"sales_id": {s.SalesID}, "confirm": {"1"}})
	if saved.Code != http.StatusSeeOther {
		t.Fatalf("연결 status=%d", saved.Code)
	}
	got, err := prepo.Get(w.ProjectID)
	if err != nil || got.SalesProjectID != s.SalesID {
		t.Fatalf("연결 결과 %+v %v", got, err)
	}
}
