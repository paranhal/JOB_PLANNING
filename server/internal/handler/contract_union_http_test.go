package handler

import (
	"database/sql"
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
	p := &model.SalesProject{OrgID: model.OrgIDLibrary, Name: "미전환계약", ProspectName: "세종", ExpectedAmount: 220000}
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

	wp := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "상세회귀", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 100}
	if err := repository.NewProjectRepo(db).Create(wp); err != nil {
		t.Fatal(err)
	}
	detail := doGet(t, e, "/contracts/"+wp.ProjectID)
	if detail.Code != http.StatusOK {
		t.Fatalf("계약 상세 %d", detail.Code)
	}
}

func TestSalesHTTP_UnpromotedBadge(t *testing.T) {
	e, db := newSalesServerDB(t)
	srepo := repository.NewSalesRepo(db)
	p := &model.SalesProject{OrgID: model.OrgIDLibrary, Name: "배지계약"}
	if err := srepo.Create(p); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, contract_amount=0 WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, p.SalesID); err != nil {
		t.Fatal(err)
	}
	show := doGet(t, e, "/sales/"+p.SalesID)
	if !strings.Contains(show.Body.String(), "영업 계약 완료 · 계약 전환 필요") {
		t.Fatal("상세 배지 없음")
	}
	dash := doGet(t, e, "/sales/dashboard")
	if !strings.Contains(dash.Body.String(), "사업관리 미전환") || !strings.Contains(dash.Body.String(), "배지계약") {
		t.Fatal("대시보드 배지 없음")
	}
}

func TestContractsHTTP_LinkSuggest(t *testing.T) {
	e, db := newSalesServerDB(t)
	srepo := repository.NewSalesRepo(db)
	prepo := repository.NewProjectRepo(db)
	s := &model.SalesProject{OrgID: model.OrgIDLibrary, Name: "연결확인사업"}
	if err := srepo.Create(s); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, contract_amount=?, sales_no=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, 1000, "S2609-099", s.SalesID); err != nil {
		t.Fatal(err)
	}
	w := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "연결확인사업", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 2000}
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

func seed137ClosedSales(t *testing.T, db *sql.DB, srepo *repository.SalesRepo, name, deal, status string) *model.SalesProject {
	t.Helper()
	p := &model.SalesProject{OrgID: model.OrgIDLibrary, Name: name, DealType: deal}
	if err := srepo.Create(p); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, deal_type=?, status=?, contract_amount=150000, contracted_at=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, deal, status, "2026-11-01", p.SalesID); err != nil {
		t.Fatal(err)
	}
	got, err := srepo.Get(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func Test95_137_T1toT6_SupplyPromoteAndDeny(t *testing.T) {
	e, db := newSalesServerDB(t)
	srepo := repository.NewSalesRepo(db)
	supply := seed137ClosedSales(t, db, srepo, "단품감열지", model.SalesDealSupply, model.SalesStatusContracted)
	build := seed137ClosedSales(t, db, srepo, "구축전환대상", model.SalesDealBuild, model.SalesStatusContracted)
	orphan := seed137ClosedSales(t, db, srepo, "전환기록만", model.SalesDealBuild, model.SalesStatusPromoted)
	lead := &model.SalesProject{OrgID: model.OrgIDLibrary, Name: "아직발굴"}
	if err := srepo.Create(lead); err != nil {
		t.Fatal(err)
	}

	t.Run("T1상세칩", func(t *testing.T) {
		show := doGet(t, e, "/sales/"+supply.SalesID).Body.String()
		if strings.Contains(show, "계약 전환 필요") || strings.Contains(show, "계약으로 전환") {
			t.Fatal("단품 상세에 전환 안내가 있다")
		}
		if !strings.Contains(show, "단품 납품 · 영업에서 종료") {
			t.Fatal("단품 칩 없음")
		}
		bshow := doGet(t, e, "/sales/"+build.SalesID).Body.String()
		if !strings.Contains(bshow, "영업 계약 완료 · 계약 전환 필요") || !strings.Contains(bshow, "계약으로 전환") {
			t.Fatal("구축 상세에 전환이 없다")
		}
	})
	t.Run("T2대시보드경고", func(t *testing.T) {
		dash := doGet(t, e, "/sales/dashboard").Body.String()
		if !strings.Contains(dash, "구축전환대상") || !strings.Contains(dash, "사업관리 미전환") {
			t.Fatal("구축이 경고 박스에 없다")
		}
		if strings.Contains(dash, "단품감열지") && strings.Contains(dash, "/sales/"+supply.SalesID+"/promote") {
			t.Fatal("단품이 경고 박스 전환 대상이다")
		}
		if strings.Contains(dash, "/sales/"+orphan.SalesID+"/promote") {
			t.Fatal("미연결 승격이 경고 박스에 있다")
		}
	})
	t.Run("T3계약목록", func(t *testing.T) {
		list := doGet(t, e, "/contracts").Body.String()
		if !strings.Contains(list, "단품감열지") {
			t.Fatal("계약 목록에서 단품 행이 빠졌다")
		}
		if !strings.Contains(list, "단품 납품 · 영업에서 종료") {
			t.Fatal("목록 단품 칩 없음")
		}
		if strings.Contains(list, `/sales/`+supply.SalesID+`/promote`) {
			t.Fatal("계약 목록에 단품 전환 버튼이 있다")
		}
		if !strings.Contains(list, `/sales/`+build.SalesID+`/promote`) {
			t.Fatal("구축 전환 버튼 없음")
		}
		if !strings.Contains(list, "전환기록만") || !strings.Contains(list, "전환 기록 있음, 연결 없음") {
			t.Fatal("미연결 승격 칩 없음")
		}
		if strings.Contains(list, `/sales/`+orphan.SalesID+`/promote`) {
			t.Fatal("미연결 승격에 전환 버튼이 있다")
		}
	})
	t.Run("T4직접주소거부", func(t *testing.T) {
		loc := doGet(t, e, "/sales/"+supply.SalesID+"/promote").Header().Get("Location")
		if !strings.Contains(loc, "err=supply") {
			t.Fatalf("단품 loc=%q", loc)
		}
		denied := doGet(t, e, "/sales/"+supply.SalesID+"?err=supply").Body.String()
		if !strings.Contains(denied, "단품 납품은 영업에서 종료") {
			t.Fatal("단품 거부 문구 없음")
		}
		loc = doGet(t, e, "/sales/"+lead.SalesID+"/promote").Header().Get("Location")
		if !strings.Contains(loc, "err=not_contracted") {
			t.Fatalf("상태아님 loc=%q", loc)
		}
		loc = doGet(t, e, "/sales/"+orphan.SalesID+"/promote").Header().Get("Location")
		if !strings.Contains(loc, "err=promoted_unlinked") {
			t.Fatalf("미연결 loc=%q", loc)
		}
		msg := doGet(t, e, "/sales/"+orphan.SalesID+"?err=promoted_unlinked").Body.String()
		if !strings.Contains(msg, "전환 기록은 있으나 사업관리 연결이 없습니다") {
			t.Fatal("미연결 거부 문구 없음")
		}
	})
	t.Run("T5파이프라인처리열", func(t *testing.T) {
		pipe := doGet(t, e, "/sales?view=pipeline").Body.String()
		if strings.Contains(pipe, `/sales/`+supply.SalesID+`/promote`) {
			t.Fatal("파이프라인에 단품 전환이 있다")
		}
		if !strings.Contains(pipe, `/sales/`+build.SalesID+`/promote`) {
			t.Fatal("파이프라인에 구축 전환이 없다")
		}
	})
	t.Run("T6이미전환연결있음", func(t *testing.T) {
		prepo := repository.NewProjectRepo(db)
		wp := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "이미전환연결", PlanYear: 2026, Status: model.WBProjectActive, SalesProjectID: build.SalesID}
		if err := prepo.Create(wp); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE sales_projects SET status=? WHERE sales_id=?`, model.SalesStatusPromoted, build.SalesID); err != nil {
			t.Fatal(err)
		}
		loc := doGet(t, e, "/sales/"+build.SalesID+"/promote").Header().Get("Location")
		if !strings.Contains(loc, "err=already_promoted") {
			t.Fatalf("이미전환 loc=%q", loc)
		}
		msg := doGet(t, e, "/sales/"+build.SalesID+"?err=already_promoted").Body.String()
		if !strings.Contains(msg, "이미 계약으로 전환된 영업 사업입니다") {
			t.Fatal("이미 전환 문구 없음")
		}
	})
}
