package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

var quoteTestSalesID string

func newQuoteServer(t *testing.T) (*echo.Echo, *repository.QuoteRepo) {
	t.Helper()
	e, db := newSalesServerDB(t)
	h := New(db)
	p := &model.SalesProject{Name: "견적연결"}
	if err := repository.NewSalesRepo(db).Create(p); err != nil {
		t.Fatal(err)
	}
	quoteTestSalesID = p.SalesID
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/quotes", h.Quotes.List)
	g.GET("/quotes/new", h.Quotes.New)
	g.POST("/quotes", h.Quotes.Create)
	g.POST("/quotes/preview", h.Quotes.Preview)
	g.GET("/quotes/:id", h.Quotes.Show)
	g.GET("/quotes/:id/edit", h.Quotes.Edit)
	g.POST("/quotes/:id", h.Quotes.Update)
	g.POST("/quotes/:id/copy", h.Quotes.Copy)
	g.POST("/quotes/:id/revise", h.Quotes.Revise)
	g.POST("/quotes/:id/order", h.Orders.FromQuote)
	g.POST("/quotes/:id/status", h.Quotes.SetStatus)
	g.GET("/quotes/:id/xlsx", h.Quotes.Download)
	g.GET("/labor-rates", h.Quotes.LaborRates)
	g.POST("/labor-rates", h.Quotes.SaveStandardRates)
	g.GET("/orders", h.Orders.List)
	g.POST("/orders/from-quote", h.Orders.FromQuote)
	g.GET("/orders/:id", h.Orders.Show)
	g.GET("/orders/:id/edit", h.Orders.Edit)
	g.POST("/orders/:id", h.Orders.Update)
	g.POST("/orders/:id/status", h.Orders.SetStatus)
	g.POST("/orders/:id/purchases", h.Orders.AddPurchase)
	g.POST("/orders/:id/deliveries", h.Orders.AddDelivery)
	g.GET("/plan/unplanned", h.Work.UnplannedList)
	return e, repository.NewQuoteRepo(db)
}

func quoteIDFromRedirect(t *testing.T, loc string) string {
	t.Helper()
	path := strings.Split(loc, "?")[0]
	id := strings.TrimPrefix(path, "/quotes/")
	id = strings.TrimSuffix(id, "/edit")
	if id == "" || strings.Contains(id, "/") {
		t.Fatalf("quote id 없음: %q", loc)
	}
	return id
}

func quoteLineForm(names []string, prices []int) url.Values {
	v := url.Values{
		"title":          {"테스트 견적"},
		"owner_name":     {"최혜영"},
		"owner_phone":    {"010-1111-2222"},
		"form_type":      {"A"},
		"vat_mode":       {"excluded"},
		"round_rule":     {"none"},
		"recipient_name": {"세종시교육청"},
		"sales_id":       {quoteTestSalesID},
	}
	for i, name := range names {
		v.Add("line_name", name)
		v.Add("line_qty", "1")
		v.Add("line_unit", "EA")
		price := 0
		if i < len(prices) {
			price = prices[i]
		}
		v.Add("line_price", strconv.Itoa(price))
		v.Add("line_spec", "")
		v.Add("line_group", "")
		v.Add("line_mm", "")
		v.Add("line_disc", "")
		v.Add("line_note", "")
		v.Add("line_item_id", "")
		v.Add("line_gov", "")
	}
	return v
}

func TestQuotesHTTP_CreateElevenLinesAndFormGuards(t *testing.T) {
	e, repo := newQuoteServer(t)

	page := doGet(t, e, "/quotes/new")
	if page.Code != http.StatusOK {
		t.Fatalf("/quotes/new status=%d %s", page.Code, clipBody(page.Body.String()))
	}
	body := page.Body.String()
	if strings.Contains(body, `name="quote_no"`) {
		t.Fatal("견적번호 입력칸이 있다")
	}
	if !strings.Contains(body, `name="quote_date"`) {
		t.Fatal("견적일 칸이 없다")
	}
	if !strings.Contains(body, `name="sales_id"`) {
		t.Fatal("사업 선택이 없다")
	}
	if strings.Contains(body, `name="subtotal"`) {
		t.Fatal("소계 입력칸이 있다")
	}
	if !strings.Contains(body, "소계") {
		t.Fatal("소계 표시가 없다")
	}
	if !strings.Contains(body, "담당자") || !strings.Contains(body, "연락처") {
		t.Fatal("담당자·연락처가 없다")
	}

	names := make([]string, 11)
	prices := make([]int, 11)
	want := 0
	for i := 0; i < 11; i++ {
		names[i] = "품목" + strconv.Itoa(i+1)
		prices[i] = (i + 1) * 100000
		want += prices[i]
	}
	rec := doForm(t, e, "/quotes", quoteLineForm(names, prices))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 status=%d body=%s", rec.Code, rec.Body.String())
	}
	id := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	q, err := repo.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Lines) != 11 {
		t.Fatalf("라인=%d", len(q.Lines))
	}
	if q.Subtotal != want {
		t.Fatalf("소계=%d want=%d", q.Subtotal, want)
	}
	today := time.Now().Format("2006-01-02")
	if q.QuoteDate != today {
		t.Fatalf("견적일=%s", q.QuoteDate)
	}
	if err := model.AssertQuoteNoMatchesDate(q.QuoteNo, q.QuoteDate, false); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(q.QuoteNo, "VI-견적-") {
		t.Fatalf("번호=%s", q.QuoteNo)
	}

	show := doGet(t, e, "/quotes/"+id)
	if show.Code != http.StatusOK {
		t.Fatalf("상세 status=%d", show.Code)
	}
	sb := show.Body.String()
	if !strings.Contains(sb, strconv.Itoa(want)) && !strings.Contains(sb, formatSalesWon(want)) {
		t.Fatalf("화면에 소계가 없다: %s", clipBody(sb))
	}

	upd := quoteLineForm([]string{"남긴 품목", "둘째"}, []int{5000, 7000})
	rec = doForm(t, e, "/quotes/"+id, upd)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("수정 status=%d", rec.Code)
	}
	q, _ = repo.Get(id)
	if q.Subtotal != 12000 || len(q.Lines) != 2 {
		t.Fatalf("줄 삭제 후 소계=%d lines=%d", q.Subtotal, len(q.Lines))
	}
	if q.QuoteNo == "" || q.QuoteDate != today {
		t.Fatal("수정 때 번호·날짜가 바뀌었다")
	}
}

func TestQuotesHTTP_OwnerRequiredAndDuplicateBlocked(t *testing.T) {
	e, repo := newQuoteServer(t)
	form := quoteLineForm([]string{"감열지"}, []int{150000})
	form.Del("owner_name")
	form.Set("owner_name", "")
	rec := doForm(t, e, "/quotes", form)
	if rec.Code == http.StatusSeeOther {
		t.Fatal("담당자 없이 저장됐다")
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "담당자") {
		t.Fatalf("담당자 오류 화면이 아니다 status=%d", rec.Code)
	}

	form = quoteLineForm([]string{"감열지"}, []int{150000})
	form.Set("owner_phone", "")
	rec = doForm(t, e, "/quotes", form)
	if rec.Code == http.StatusSeeOther {
		t.Fatal("연락처 없이 저장됐다")
	}

	ok := quoteLineForm([]string{"감열지"}, []int{150000})
	rec = doForm(t, e, "/quotes", ok)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("정상 저장 실패 status=%d %s", rec.Code, rec.Body.String())
	}
	id := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	q, _ := repo.Get(id)

	err := repo.Create(&model.SalesQuote{
		QuoteNo: q.QuoteNo, QuoteDate: q.QuoteDate, IsLegacy: true,
		SalesID: quoteTestSalesID,
		OwnerName: "a", OwnerPhone: "1",
		Lines: []model.SalesQuoteLine{{Name: "x", Qty: 1, UnitPrice: 1}},
	})
	if err != model.ErrQuoteNoDuplicate {
		t.Fatalf("중복 저장: %v", err)
	}
}

func TestQuotesHTTP_CopyRenumbersToday(t *testing.T) {
	e, repo := newQuoteServer(t)
	rec := doForm(t, e, "/quotes", quoteLineForm([]string{"복사원"}, []int{1000}))
	id := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	src, _ := repo.Get(id)
	rec = doForm(t, e, "/quotes/"+id+"/copy", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("복사 status=%d", rec.Code)
	}
	dstID := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	dst, err := repo.Get(dstID)
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format("2006-01-02")
	if dst.QuoteNo == src.QuoteNo {
		t.Fatal("복사했는데 번호가 같다")
	}
	if dst.QuoteDate != today {
		t.Fatalf("복사 견적일=%s", dst.QuoteDate)
	}
	if err := model.AssertQuoteNoMatchesDate(dst.QuoteNo, dst.QuoteDate, false); err != nil {
		t.Fatal(err)
	}
}

func TestQuotesHTTP_VATIncluded5700000AndRoundHundred(t *testing.T) {
	e, repo := newQuoteServer(t)
	form := quoteLineForm([]string{"장서점검기"}, []int{5_700_000})
	form.Set("form_type", "B")
	form.Set("vat_mode", "included")
	form.Set("round_rule", "none")
	rec := doForm(t, e, "/quotes", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 status=%d %s", rec.Code, rec.Body.String())
	}
	id := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	q, _ := repo.Get(id)
	if !strings.HasPrefix(q.QuoteNo, "VI-견적-") {
		t.Fatalf("양식 B 번호=%s", q.QuoteNo)
	}
	tot := q.Totals()
	if tot.Supply != 5_181_818 || tot.VAT != 518_182 || tot.Total != 5_700_000 {
		t.Fatalf("공급가=%d 부가세=%d 합계=%d", tot.Supply, tot.VAT, tot.Total)
	}
	show := doGet(t, e, "/quotes/"+id)
	sb := show.Body.String()
	if !strings.Contains(sb, "5,181,818") || !strings.Contains(sb, "518,182") || !strings.Contains(sb, "5,700,000") {
		t.Fatalf("세 숫자가 화면에 없다: %s", clipBody(sb))
	}

	form = quoteLineForm([]string{"절사"}, []int{25_222_587})
	form.Set("vat_mode", "included")
	form.Set("round_rule", "hundred")
	rec = doForm(t, e, "/quotes", form)
	id2 := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	q2, _ := repo.Get(id2)
	if q2.Total != 25_222_500 {
		t.Fatalf("100원 절사 합계=%d", q2.Total)
	}
	show = doGet(t, e, "/quotes/"+id2)
	if !strings.Contains(show.Body.String(), "100원 단위 절사") {
		t.Fatal("절사 라벨이 없다")
	}
	if strings.Contains(show.Body.String(), "1,000원 단위 절사") {
		t.Fatal("100원 절사인데 천원 라벨이 있다")
	}
}

func TestQuotesHTTP_XlsxValuesMatchScreenNoFormula(t *testing.T) {
	e, repo := newQuoteServer(t)
	names := make([]string, 11)
	prices := make([]int, 11)
	sum := 0
	for i := 0; i < 11; i++ {
		names[i] = "L" + strconv.Itoa(i+1)
		prices[i] = 100000 * (i + 1)
		sum += prices[i]
	}
	form := quoteLineForm(names, prices)
	form.Set("vat_mode", "excluded")
	rec := doForm(t, e, "/quotes", form)
	id := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	q, _ := repo.Get(id)
	tot := q.Totals()

	dl := doGet(t, e, "/quotes/"+id+"/xlsx")
	if dl.Code != http.StatusOK {
		t.Fatalf("xlsx status=%d", dl.Code)
	}
	ct := dl.Header().Get("Content-Type")
	if !strings.Contains(ct, "spreadsheetml") {
		t.Fatalf("content-type=%s", ct)
	}
	f, err := excelize.OpenReader(bytes.NewReader(dl.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	visN := 11
	subRow, vatRow, totRow := QuotePSumRows(visN)
	gotSupply, _ := f.GetCellValue(sheet, cellI(subRow))
	gotVAT, _ := f.GetCellValue(sheet, cellI(vatRow))
	gotTotal, _ := f.GetCellValue(sheet, "H"+strconv.Itoa(totRow))
	if atoiCell(gotSupply) != tot.Supply || atoiCell(gotVAT) != tot.VAT || atoiCell(gotTotal) != tot.Total {
		t.Fatalf("xlsx 공급가=%s 부가세=%s 합계=%s 화면=%d/%d/%d", gotSupply, gotVAT, gotTotal, tot.Supply, tot.VAT, tot.Total)
	}
	for _, addr := range []string{cellI(subRow), cellI(vatRow), "H" + strconv.Itoa(totRow), "B13"} {
		formula, _ := f.GetCellFormula(sheet, addr)
		if strings.TrimSpace(formula) != "" {
			t.Fatalf("%s 에 산식이 있다: %s", addr, formula)
		}
	}
	foot, _ := f.GetCellValue(sheet, "A"+strconv.Itoa(quoteFormP.footerRow()+(visN-quoteFormP.DefaultLines)))
	if !strings.Contains(foot, "QEP-710-01") {
		t.Fatalf("푸터=%q", foot)
	}
}

func TestQuotesHTTP_KanbanFiveColumns(t *testing.T) {
	e, _ := newQuoteServer(t)
	kanban := doGet(t, e, "/quotes?display=kanban")
	if kanban.Code != http.StatusOK {
		t.Fatalf("칸반 status=%d", kanban.Code)
	}
	kb := kanban.Body.String()
	if strings.Count(kb, "flex-1 basis-0") != 5 {
		t.Fatalf("견적 칸반 열=%d", strings.Count(kb, "flex-1 basis-0"))
	}
	for _, title := range []string{"작성중", "제출", "수주", "실주", "만료"} {
		if !strings.Contains(kb, title) {
			t.Fatalf("열 %s 없음", title)
		}
	}
}

func TestQuotesHTTP_PreviewJSON(t *testing.T) {
	e, _ := newQuoteServer(t)
	form := quoteLineForm([]string{"장서점검기"}, []int{5_700_000})
	form.Set("vat_mode", "included")
	rec := doFormJSON(t, e, "/quotes/preview", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status=%d", rec.Code)
	}
	var d map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if intFromJSON(d["supply"]) != 5_181_818 || intFromJSON(d["vat"]) != 518_182 || intFromJSON(d["total"]) != 5_700_000 {
		t.Fatalf("preview=%v", d)
	}
}

func cellI(row int) string {
	return "I" + strconv.Itoa(row)
}

func atoiCell(s string) int {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "₩", "")
	s = strings.ReplaceAll(s, "원", "")
	s = strings.ReplaceAll(s, " ", "")
	n, _ := strconv.Atoi(s)
	return n
}

func intFromJSON(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	default:
		return 0
	}
}

func TestQuotesHTTP_FormB2GroupLaborRevisePurposeReverse(t *testing.T) {
	e, repo := newQuoteServer(t)

	b2 := quoteLineForm([]string{"서버", "단말"}, []int{1_000_000, 2_000_000})
	b2.Set("form_type", "B2")
	b2.Set("vat_mode", "included")
	b2.Del("line_group")
	b2.Add("line_group", "서버묶음")
	b2.Add("line_group", "단말묶음")
	rec := doForm(t, e, "/quotes", b2)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("B-2 저장 status=%d %s", rec.Code, rec.Body.String())
	}
	id := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	q, _ := repo.Get(id)
	if model.NormalizeQuoteForm(q.FormType) != model.QuoteFormB2 {
		t.Fatalf("form=%s", q.FormType)
	}
	if err := model.AssertGroupSumEquals(q.Lines, q.Subtotal); err != nil {
		t.Fatal(err)
	}

	labor := quoteLineForm([]string{"응용SW개발자"}, []int{7_754_124})
	labor.Set("form_type", "A2")
	labor.Set("vat_mode", "excluded")
	labor.Set("overhead_rate", "110")
	labor.Set("tech_fee_rate", "20")
	labor.Del("line_mm")
	labor.Add("line_mm", "0.7")
	labor.Del("line_unit")
	labor.Add("line_unit", "M/M")
	labor.Del("line_labor_year")
	labor.Add("line_labor_year", "2025")
	rec = doForm(t, e, "/quotes", labor)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("용역 저장 status=%d %s", rec.Code, rec.Body.String())
	}
	lid := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	lq, _ := repo.Get(lid)
	direct := model.LineAmount(lq.Lines[0])
	if direct != 5_427_886 {
		t.Fatalf("직접인건비=%d", direct)
	}
	lab := model.LaborAddons(direct, 110, 20)
	if lq.Subtotal != lab.Subtotal {
		t.Fatalf("용역 소계=%d want=%d 제경=%d 기술=%d", lq.Subtotal, lab.Subtotal, lab.Overhead, lab.TechFee)
	}
	show := doGet(t, e, "/quotes/"+lid)
	sb := show.Body.String()
	if !strings.Contains(sb, "2025년 단가") {
		t.Fatalf("연도 단가 표시 없음: %s", clipBody(sb))
	}
	if !strings.Contains(sb, "직접인건비") || !strings.Contains(sb, "제경비") || !strings.Contains(sb, "기술료") {
		t.Fatal("용역 산식 표시가 없다")
	}

	labor.Set("overhead_rate", "100")
	rec = doForm(t, e, "/quotes", labor)
	id100 := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	q100, _ := repo.Get(id100)
	savedTotal := q100.Total
	stdOH, _ := repo.StandardRates()
	if stdOH != 110 {
		t.Fatalf("표준이 견적과 같이 바뀌었다: %g", stdOH)
	}
	show = doGet(t, e, "/quotes/"+id100)
	if !strings.Contains(show.Body.String(), "표준 110% → 100%") {
		t.Fatalf("요율 차이 표시 없음: %s", clipBody(show.Body.String()))
	}
	if err := repo.SetStandardRates(90, 15); err != nil {
		t.Fatal(err)
	}
	again, _ := repo.Get(id100)
	if again.Total != savedTotal || again.OverheadRate != 100 {
		t.Fatalf("소급됨 total=%d rate=%g", again.Total, again.OverheadRate)
	}
	stdOH, _ = repo.StandardRates()
	if stdOH != 90 {
		t.Fatalf("설정 표준=%g", stdOH)
	}

	revForm := quoteLineForm([]string{"버스"}, []int{10_000_000})
	revForm.Set("vat_mode", "included")
	revForm.Set("target_total", "19990000")
	rec = doForm(t, e, "/quotes", revForm)
	rid := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	rq, _ := repo.Get(rid)
	if !rq.IsReverseCalc || rq.Total != 19_990_000 {
		t.Fatalf("역산 flag=%v total=%d", rq.IsReverseCalc, rq.Total)
	}
	show = doGet(t, e, "/quotes/"+rid)
	if !strings.Contains(show.Body.String(), "역산") {
		t.Fatal("역산 뱃지가 없다")
	}

	srcID := id
	rec = doForm(t, e, "/quotes/"+srcID+"/revise", url.Values{"rev_reason": {"단가 조정"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("개정 status=%d %s", rec.Code, rec.Body.String())
	}
	newID := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	oldQ, err := repo.Get(srcID)
	if err != nil {
		t.Fatal("이전 개정이 지워졌다")
	}
	newQ, _ := repo.Get(newID)
	if newQ.QuoteNo != oldQ.QuoteNo || newQ.Rev != oldQ.Rev+1 {
		t.Fatalf("번호/rev old=%s r%d new=%s r%d", oldQ.QuoteNo, oldQ.Rev, newQ.QuoteNo, newQ.Rev)
	}
	if !strings.HasSuffix(newQ.DisplayNo(), "-1") {
		t.Fatalf("화면 번호=%s", newQ.DisplayNo())
	}
	oldQ.Title = "고치면 안 됨"
	if err := repo.Update(oldQ); err != model.ErrQuoteReadOnly {
		t.Fatalf("이전 개정 수정: %v", err)
	}
	edit := doGet(t, e, "/quotes/"+srcID+"/edit")
	if edit.Code != http.StatusSeeOther {
		t.Fatal("이전 개정 수정 화면이 열렸다")
	}

	budget := quoteLineForm([]string{"출입통제"}, []int{37_950_000})
	budget.Set("purpose", "budget")
	budget.Set("budget_year", "2027")
	rec = doForm(t, e, "/quotes", budget)
	bid := quoteIDFromRedirect(t, rec.Header().Get("Location"))
	bq, _ := repo.Get(bid)
	if bq.InPipeline() {
		t.Fatal("예산용이 파이프라인 대상이다")
	}
	list := doGet(t, e, "/quotes?purpose=budget")
	if !strings.Contains(list.Body.String(), bid) && !strings.Contains(list.Body.String(), bq.DisplayNo()) {
		t.Fatal("예산 반영 예정 목록에 없다")
	}
	show = doGet(t, e, "/quotes/"+bid)
	if !strings.Contains(show.Body.String(), "예산용") {
		t.Fatal("용도 뱃지가 없다")
	}

	page := doGet(t, e, "/labor-rates")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "UI/UX기획/개발자") {
		t.Fatalf("노임단가 화면 status=%d", page.Code)
	}
	if !strings.Contains(page.Body.String(), "6,901,660") || !strings.Contains(page.Body.String(), "7,754,124") {
		t.Fatal("2026 시드 금액이 없다")
	}
}
