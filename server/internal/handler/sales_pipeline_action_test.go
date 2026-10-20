package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestSalesPipelineActionColumnAndDialogs(t *testing.T) {
	e, db := newSalesServerDB(t)
	disc := doForm(t, e, "/sales", url.Values{"name": {"발굴사업90"}, "expected_amount": {"1000000"}})
	discID := salesIDFromRedirect(t, disc.Header().Get("Location"))
	bid := doForm(t, e, "/sales", url.Values{"name": {"입찰대기90"}, "expected_amount": {"2000000"}})
	bidID := salesIDFromRedirect(t, bid.Header().Get("Location"))
	won := doForm(t, e, "/sales", url.Values{"name": {"수주사업90"}, "expected_amount": {"3000000"}})
	wonID := salesIDFromRedirect(t, won.Header().Get("Location"))
	if rec := doForm(t, e, "/sales/"+wonID+"/stage", url.Values{"stage": {"direct_won"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("수주 준비 loc=%s", rec.Header().Get("Location"))
	}
	done := createContractedSales(t, e, url.Values{"name": {"계약완료90"}, "expected_ym": {"2026-10"}})
	lost := doForm(t, e, "/sales", url.Values{"name": {"실주사업90"}})
	lostID := salesIDFromRedirect(t, lost.Header().Get("Location"))
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, bid_status=? WHERE sales_id=?`,
		model.SalesStage4Bid, model.SalesBidPending, bidID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, status=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseLost, model.SalesStatusLost, lostID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, sales_project_id, contract_amount)
		VALUES ('WP-90','전환됨','active',?, 30000000)`, done); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active)
		VALUES ('u-view90','view90','x','보기용','sales',1)`); err != nil {
		t.Fatal(err)
	}

	page := doGet(t, e, "/sales?view=pipeline&from=2026-09&to=2026-12")
	if page.Code != http.StatusOK {
		t.Fatalf("pipeline %d", page.Code)
	}
	body := page.Body.String()
	tpl, _ := os.ReadFile("web/templates/sales/pipeline.html")
	if !strings.Contains(string(tpl), `colspan="10"`) {
		t.Fatal("빈 목록 colspan=10 이 아니다")
	}
	if !strings.Contains(body, `aria-label="발굴사업90 수주"`) || !strings.Contains(body, ">수주<") {
		t.Fatal("발굴 수주 단추 없음")
	}
	if !strings.Contains(body, `aria-label="수주사업90 계약으로 전환"`) {
		t.Fatal("수주 건 계약 단추 없음")
	}
	if !strings.Contains(body, `aria-label="계약완료90 사업 보기"`) || !strings.Contains(body, "/projects/WP-90") {
		t.Fatal("전환 완료 사업 보기 없음")
	}
	if strings.Contains(body, `aria-label="실주사업90`) {
		t.Fatal("실주 건에 처리 단추가 있다")
	}
	if !strings.Contains(body, "data-sales-row-link") || !strings.Contains(body, "sales-next.js") {
		t.Fatal("포커스 복귀용 링크·스크립트 없음")
	}

	dlg := doGet(t, e, "/sales/"+discID+"/win-dialog")
	if dlg.Code != http.StatusOK || !strings.Contains(dlg.Body.String(), "수주 확정") || !strings.Contains(dlg.Body.String(), "발굴사업90") {
		t.Fatalf("수주 팝업 %d %s", dlg.Code, dlg.Body.String())
	}
	cdlg := doGet(t, e, "/sales/"+wonID+"/contract-dialog")
	if cdlg.Code != http.StatusOK || !strings.Contains(cdlg.Body.String(), "계약으로 전환") {
		t.Fatalf("계약 팝업 %d", cdlg.Code)
	}

	ro := doGetWith(t, e, "/sales?view=pipeline", jwtCookieReadOnly(t, "sales"))
	if strings.Contains(ro.Body.String(), `data-sales-next`) {
		t.Fatal("읽기 전용에 처리 단추가 있다")
	}
	obs := httptestGetAs(t, e, "/sales?view=pipeline&from=2026-09&to=2026-12", "observer")
	if strings.Contains(obs.Body.String(), `data-sales-next`) {
		t.Fatal("권한 없음에 처리 단추가 있다")
	}
	view := doGetWith(t, e, "/sales?view=pipeline", jwtCookie(t), &http.Cookie{Name: "view_as_user_id", Value: "u-view90", Path: "/"})
	if strings.Contains(view.Body.String(), `data-sales-next`) {
		t.Fatal("view-as에 처리 단추가 있다")
	}

	js, err := os.ReadFile("web/static/js/sales-next.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(js)
	for _, want := range []string{"Escape", "Tab", "back.focus", "data-sales-row-link"} {
		if !strings.Contains(src, want) {
			t.Fatalf("접근성 %s 없음", want)
		}
	}
}

func TestSalesWinConfirmPaths(t *testing.T) {
	e, db := newSalesServerDB(t)
	disc := doForm(t, e, "/sales", url.Values{"name": {"직접수주90"}, "expected_amount": {"5000000"}})
	discID := salesIDFromRedirect(t, disc.Header().Get("Location"))
	if sh := doGet(t, e, "/sales/"+discID); sh.Code != http.StatusOK {
		t.Fatalf("상세 id=%q status=%d", discID, sh.Code)
	}
	got := doFormJSON(t, e, "/sales/"+discID+"/win-confirm", url.Values{"won_at": {"2026-10-01"}, "amount": {"8000000"}})
	if got.Code != http.StatusOK {
		t.Fatalf("직접 수주 %d %s", got.Code, got.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(got.Body.Bytes(), &payload); err != nil || payload["ok"] != true {
		t.Fatalf("json %s", got.Body.String())
	}
	if !strings.Contains(payload["next"].(string), "ok=won") {
		t.Fatalf("next=%v", payload["next"])
	}
	p, _ := repository.NewSalesRepo(db).Get(discID)
	if p.BidStatus != model.SalesBidWon || p.WonAt != "2026-10-01" || p.AwardedAmount != 8000000 {
		t.Fatalf("직접 수주 결과 %+v", p)
	}
	again := doFormJSON(t, e, "/sales/"+discID+"/win-confirm", url.Values{"won_at": {"2026-10-02"}})
	if again.Code != http.StatusBadRequest || !strings.Contains(again.Body.String(), "이미 수주") {
		t.Fatalf("이중 제출 %d %s", again.Code, again.Body.String())
	}
	pipe := doGet(t, e, "/sales?view=pipeline&ok=won")
	if !strings.Contains(pipe.Body.String(), "수주로 처리했습니다") || !strings.Contains(pipe.Body.String(), "계약으로 전환") {
		t.Fatal("파이프라인 알림·단추 갱신 없음")
	}

	bid := doForm(t, e, "/sales", url.Values{"name": {"입찰수주90"}, "expected_amount": {"1000"}})
	bidID := salesIDFromRedirect(t, bid.Header().Get("Location"))
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, bid_status=? WHERE sales_id=?`,
		model.SalesStage4Bid, model.SalesBidPending, bidID); err != nil {
		t.Fatal(err)
	}
	neg := doFormJSON(t, e, "/sales/"+bidID+"/win-confirm", url.Values{"amount": {"-1"}, "won_at": {"2026-10-01"}})
	if neg.Code != http.StatusBadRequest || !strings.Contains(neg.Body.String(), "0 이상") {
		t.Fatalf("음수 %d %s", neg.Code, neg.Body.String())
	}
	lost := doFormJSON(t, e, "/sales/"+bidID+"/win-confirm", url.Values{"result": {"lost"}})
	if lost.Code != http.StatusBadRequest || !strings.Contains(lost.Body.String(), "실주 사유") {
		t.Fatalf("실주 사유 %d %s", lost.Code, lost.Body.String())
	}
	okLost := doFormJSON(t, e, "/sales/"+bidID+"/win-confirm", url.Values{"result": {"lost"}, "reason": {"가격"}})
	if okLost.Code != http.StatusOK {
		t.Fatalf("실주 %d %s", okLost.Code, okLost.Body.String())
	}

	bid2 := doForm(t, e, "/sales", url.Values{"name": {"입찰낙찰90"}})
	bid2ID := salesIDFromRedirect(t, bid2.Header().Get("Location"))
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, bid_status=? WHERE sales_id=?`,
		model.SalesStage4Bid, model.SalesBidPending, bid2ID); err != nil {
		t.Fatal(err)
	}
	wonBid := doFormJSON(t, e, "/sales/"+bid2ID+"/win-confirm", url.Values{"won_at": {"2026-10-03"}})
	if wonBid.Code != http.StatusOK {
		t.Fatalf("입찰 수주 %d %s", wonBid.Code, wonBid.Body.String())
	}

	keep := doForm(t, e, "/sales", url.Values{"name": {"금액유지90"}, "expected_amount": {"7770000"}})
	keepID := salesIDFromRedirect(t, keep.Header().Get("Location"))
	emptyAmt := doFormJSON(t, e, "/sales/"+keepID+"/win-confirm", url.Values{"won_at": {"2026-10-04"}})
	if emptyAmt.Code != http.StatusOK {
		t.Fatalf("금액 생략 %d %s", emptyAmt.Code, emptyAmt.Body.String())
	}
	kept, _ := repository.NewSalesRepo(db).Get(keepID)
	if kept.ExpectedAmount != 7770000 || kept.AwardedAmount != 0 {
		t.Fatalf("예상금액 유지 실패 exp=%d awarded=%d", kept.ExpectedAmount, kept.AwardedAmount)
	}

	missing := doFormJSON(t, e, "/sales/no-such/win-confirm", url.Values{"won_at": {"2026-10-01"}})
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("없는 건 %d", missing.Code)
	}

	other := doForm(t, e, "/sales", url.Values{"name": {"타조직90"}})
	otherID := salesIDFromRedirect(t, other.Header().Get("Location"))
	if _, err := db.Exec(`UPDATE sales_projects SET org_id='O99' WHERE sales_id=?`, otherID); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/sales/"+otherID+"/win-confirm", strings.NewReader(url.Values{"won_at": {"2026-10-01"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(jwtCookieOrg(t, model.RoleSales, "O01"))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("타 조직 %d %s", rec.Code, rec.Body.String())
	}

	anon := httptest.NewRecorder()
	areq := httptest.NewRequest(http.MethodPost, "http://localhost/sales/"+discID+"/win-confirm", strings.NewReader(url.Values{"won_at": {"2026-10-01"}}.Encode()))
	areq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	e.ServeHTTP(anon, areq)
	if anon.Code != http.StatusFound && anon.Code != http.StatusSeeOther && anon.Code != http.StatusUnauthorized && anon.Code != http.StatusForbidden {
		t.Fatalf("비로그인 CSRF/세션 %d", anon.Code)
	}
}

func TestSalesContractDialogSubmit(t *testing.T) {
	e, db := newSalesServerDB(t)
	rec := doForm(t, e, "/sales", url.Values{"name": {"계약전환90"}, "expected_amount": {"10000000"}})
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	if rec := doForm(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"direct_won"}}); rec.Code != http.StatusSeeOther {
		t.Fatal("수주 실패")
	}
	noDate := doFormJSON(t, e, "/sales/"+id+"/close-contract", url.Values{"contract_amount": {"0"}})
	if noDate.Code != http.StatusBadRequest || !strings.Contains(noDate.Body.String(), "계약일") {
		t.Fatalf("계약일 %d %s", noDate.Code, noDate.Body.String())
	}
	neg := doFormJSON(t, e, "/sales/"+id+"/close-contract", url.Values{"contracted_at": {"2026-10-10"}, "contract_amount": {"-3"}})
	if neg.Code != http.StatusBadRequest || !strings.Contains(neg.Body.String(), "0 이상") {
		t.Fatalf("음수 계약 %d %s", neg.Code, neg.Body.String())
	}
	ok := doForm(t, e, "/sales/"+id+"/close-contract", url.Values{"contracted_at": {"2026-10-10"}, "contract_amount": {"0"}})
	if ok.Code != http.StatusSeeOther || !strings.Contains(ok.Header().Get("Location"), "/sales/"+id+"/promote") {
		t.Fatalf("promote 303 %d loc=%s", ok.Code, ok.Header().Get("Location"))
	}
	p, _ := repository.NewSalesRepo(db).Get(id)
	if p.CloseReason != model.SalesCloseContracted || p.ContractAmount != 0 || p.ContractedAt != "2026-10-10" {
		t.Fatalf("계약 결과 %+v", p)
	}
	hist, _ := repository.NewSalesRepo(db).ListHistory(id)
	if len(hist) == 0 {
		t.Fatal("변경 이력 없음")
	}
	if wp, err := repository.NewProjectRepo(db).GetBySalesID(id); err == nil && wp != nil {
		t.Fatal("양식 저장 전에 사업관리 사업이 생겼다")
	}
	pipe := doGet(t, e, "/sales?view=pipeline")
	if !strings.Contains(pipe.Body.String(), `aria-label="계약전환90 계약으로 전환"`) || !strings.Contains(pipe.Body.String(), "/promote") {
		t.Fatal("미전환 건 promote 링크가 없다")
	}
	if !strings.Contains(pipe.Body.String(), "계약 확정") {
		t.Fatal("요약 칸이 없다")
	}
}

func TestSalesShowButtonsUnchanged(t *testing.T) {
	e := newSalesServer(t)
	rec := doForm(t, e, "/sales", url.Values{"name": {"상세회귀90"}})
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	show := doGet(t, e, "/sales/"+id).Body.String()
	if !strings.Contains(show, "바로 수주") {
		t.Fatal("상세 바로 수주 문구가 바뀌었다")
	}
	if rec := doForm(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"direct_won"}}); rec.Code != http.StatusSeeOther {
		t.Fatal("수주")
	}
	show = doGet(t, e, "/sales/"+id).Body.String()
	if !strings.Contains(show, "계약으로 종료") || !strings.Contains(show, "협상 시작") {
		t.Fatal("상세 수주 후 단추 문구가 바뀌었다")
	}
}
