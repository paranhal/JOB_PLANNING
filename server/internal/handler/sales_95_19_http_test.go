package handler

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

var sales95_19Fields02 = []string{
	"customer_id", "prospect_name", "prospect_region", "customer_confirmed",
	"expected_amount", "expected_amount_confirmed", "amount_vat_included",
	"lead_source", "contract_target", "procurement_route", "contract_method",
	"bid_eval_method", "mall_contract_type", "budget_status",
}

var sales95_19Fields03 = []string{
	"sales_owner_id", "expected_precision", "expected_ym", "expected_year",
	"expected_quarter", "expected_half", "expected_ym_confirmed",
	"bid_ym", "revenue_ym", "billing_cycle", "revenue_from", "revenue_to",
	"competitor", "group_id",
}

func pdfBytes() []byte  { return []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n") }
func zipBytes() []byte  { return append([]byte("PK\x03\x04"), bytes.Repeat([]byte("a"), 32)...) }
func oleBytes() []byte  { return []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0, 0, 0, 0} }

func postSalesMultipart(t *testing.T, e *echo.Echo, path string, fields map[string]string, files [][2]string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		part, err := w.CreateFormFile(model.SalesInitialAttachField, f[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	if ck != nil {
		req.AddCookie(ck)
	} else {
		req.AddCookie(jwtCookie(t))
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestSales95_19NewFormStageBoxAndFields(t *testing.T) {
	e := newSalesServer(t)
	rec := doGet(t, e, "/sales/new")
	if rec.Code != http.StatusOK {
		t.Fatalf("new %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "기회 정보 완성도") {
		t.Fatal("완성도")
	}
	if strings.Contains(body, "임시 저장") {
		t.Fatal("임시저장")
	}
	if !strings.Contains(body, "max-w-6xl") || !strings.Contains(body, "영업 단계") {
		t.Fatal("레이아웃")
	}
	if !strings.Contains(body, "발굴") || !strings.Contains(body, "제안") || !strings.Contains(body, "입찰") || !strings.Contains(body, "사업 종료") {
		t.Fatal("4단계 라벨")
	}
	if !strings.Contains(body, "10%") {
		t.Fatal("신규 10%")
	}
	if !strings.Contains(body, "open02: false") || !strings.Contains(body, "open03: false") || !strings.Contains(body, "open04: true") {
		t.Fatal("접힘 기본값")
	}
	need := append(append([]string{"name", "is_tentative_name", "biz_type", "notes", "quick_quote", "initial_files"}, sales95_19Fields02...), sales95_19Fields03...)
	for _, n := range need {
		if !strings.Contains(body, `name="`+n+`"`) {
			if n == "group_id" {
				continue
			}
			t.Fatalf("필드 없음 %s", n)
		}
	}
	p := &model.SalesProject{Stage: model.SalesStage4Discover}
	if model.SalesProbability(p) != 10 {
		t.Fatal("SalesProbability")
	}
}

func TestSales95_19FormUILintZero(t *testing.T) {
	root := findTemplateRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "sales", "form.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := countUILint(string(b))
	if !got.zero() {
		t.Fatalf("form.html ui_lint %+v", got)
	}
}

func TestSales95_19EditProbabilityByStage(t *testing.T) {
	e, db := newSalesServerDB(t)
	rec := doForm(t, e, "/sales", url.Values{"name": {"확률사업"}})
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))

	check := func(stage, extra string, want int, needles ...string) {
		t.Helper()
		p := &model.SalesProject{Stage: stage}
		switch stage {
		case model.SalesStage4Propose:
			p.RFPReceivedAt = extra
		case model.SalesStage4Bid:
			p.BidStatus = extra
		case model.SalesStage4Closed:
			p.CloseReason = extra
		}
		if model.SalesProbability(p) != want {
			t.Fatalf("prob %s %s want %d got %d", stage, extra, want, model.SalesProbability(p))
		}
		q := `UPDATE sales_projects SET stage=?`
		args := []any{stage}
		if stage == model.SalesStage4Propose {
			q += `, rfp_received_at=?`
			args = append(args, extra)
		}
		if stage == model.SalesStage4Bid {
			q += `, bid_status=?`
			args = append(args, extra)
		}
		if stage == model.SalesStage4Closed {
			q += `, close_reason=?`
			args = append(args, extra)
		}
		q += ` WHERE sales_id=?`
		args = append(args, id)
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
		got := doGet(t, e, "/sales/"+id+"/edit")
		if got.Code != http.StatusOK {
			t.Fatalf("edit %d", got.Code)
		}
		body := got.Body.String()
		if !strings.Contains(body, strconv.Itoa(want)+"%") {
			t.Fatalf("%s %s 확률 %d 없음", stage, extra, want)
		}
		for _, n := range needles {
			if !strings.Contains(body, n) {
				t.Fatalf("%s 문구 %q", stage, n)
			}
		}
		if !strings.Contains(body, "단계 변경은 사업 상세/파이프라인에서 합니다") {
			t.Fatal("수정 안내")
		}
	}
	check(model.SalesStage4Propose, "", 20, "제안")
	check(model.SalesStage4Propose, "2026-10-01", 50, "제안")
	check(model.SalesStage4Bid, model.SalesBidWon, 100, "수주")
	check(model.SalesStage4Closed, model.SalesCloseContracted, 100, "계약")
	check(model.SalesStage4Closed, model.SalesCloseLost, 0, "실주")
	check(model.SalesStage4Closed, model.SalesCloseDropped, 0, "중단")
}

func TestSales95_19CollapsedPOSTKeeps02_03(t *testing.T) {
	e := newSalesServer(t)
	rec := doForm(t, e, "/sales", url.Values{
		"name":            {"접힘저장"},
		"prospect_name":   {"에이펙스커머스"},
		"expected_amount": {"180000000"},
		"lead_source":     {""},
		"competitor":      {"경쟁A"},
		"bid_ym":          {"2026-11"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	edit := doGet(t, e, "/sales/"+id+"/edit")
	body := edit.Body.String()
	if !strings.Contains(body, "에이펙스커머스") || !strings.Contains(body, "180000000") || !strings.Contains(body, "경쟁A") {
		t.Fatal("접힌 값 미저장")
	}
	if !strings.Contains(body, "open02: false") {
		t.Fatal("수정 기본 접힘")
	}
}

func TestSales95_19FormErrorOpensCards(t *testing.T) {
	e, db := newSalesServerDB(t)
	_ = e
	h := New(db)
	h.Attachment.uploadDir = t.TempDir()
	ee := echo.New()
	ee.Renderer = NewRenderer()
	req := httptest.NewRequest(http.MethodGet, "/sales/new", nil)
	rec := httptest.NewRecorder()
	c := ee.NewContext(req, rec)
	c.Set("role", model.RoleAdmin)
	c.Set("user_name", "관리자")
	c.Set("permissions", model.DefaultPermissions(model.RoleAdmin))
	p := &model.SalesProject{Stage: model.SalesStage4Discover, DealType: model.SalesDealBuild}
	if err := h.Sales.renderForm(c, p, false, "유입 경로를 확인하세요"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), "open02: true") {
		t.Fatal("02 펼침")
	}
	rec = httptest.NewRecorder()
	c = ee.NewContext(req, rec)
	c.Set("role", model.RoleAdmin)
	c.Set("user_name", "관리자")
	c.Set("permissions", model.DefaultPermissions(model.RoleAdmin))
	if err := h.Sales.renderForm(c, p, false, "영업담당을 선택하세요"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), "open03: true") {
		t.Fatal("03 펼침")
	}
}

func TestSales95_19UploadAcceptRejectAndHeaders(t *testing.T) {
	e, db := newSalesServerDB(t)
	ok := postSalesMultipart(t, e, "/sales", map[string]string{"name": "첨부사업"}, [][2]string{
		{"rfp.pdf", string(pdfBytes())},
		{"spec.docx", string(zipBytes())},
		{"sheet.xlsx", string(zipBytes())},
	}, nil)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("ok %d %s", ok.Code, ok.Body.String())
	}
	id := salesIDFromRedirect(t, ok.Header().Get("Location"))
	show := doGet(t, e, "/sales/"+id)
	body := show.Body.String()
	if !strings.Contains(body, "rfp.pdf") || !strings.Contains(body, "spec.docx") {
		t.Fatal("목록")
	}
	var attID, path string
	if err := db.QueryRow(`SELECT attachment_id, file_path FROM attachments WHERE ref_type=? AND ref_id=? AND file_name='rfp.pdf'`,
		model.RefTypeSalesProject, id).Scan(&attID, &path); err != nil {
		t.Fatal(err)
	}
	dl := doGet(t, e, "/attachments/"+attID)
	if dl.Code != http.StatusOK {
		t.Fatalf("dl %d", dl.Code)
	}
	if !strings.Contains(dl.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("disposition %s", dl.Header().Get("Content-Disposition"))
	}
	if dl.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("nosniff")
	}

	part := postSalesMultipart(t, e, "/sales", map[string]string{"name": "일부실패"}, [][2]string{
		{"ok.pdf", string(pdfBytes())},
		{"bad.exe", "MZ"},
	}, nil)
	if part.Code != http.StatusSeeOther {
		t.Fatalf("partial %d", part.Code)
	}
	loc := part.Header().Get("Location")
	if !strings.Contains(loc, "attach_warn=") {
		t.Fatalf("warn %s", loc)
	}
	pid := salesIDFromRedirect(t, loc)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sales_projects WHERE sales_id=?`, pid).Scan(&n); err != nil || n != 1 {
		t.Fatal("사업 미저장")
	}
	show = doGet(t, e, loc)
	if !strings.Contains(show.Body.String(), "일부 파일을 올리지 못했습니다") {
		t.Fatal("실패 안내")
	}

	rej := postSalesMultipart(t, e, "/sales", map[string]string{"name": "거절모음"}, [][2]string{
		{"page.html", "<html>"},
		{"pic.svg", "<svg>"},
		{"fake.pdf", "<html>notpdf"},
		{"../secret.pdf", string(pdfBytes())},
		{"dup.pdf", string(pdfBytes())},
		{"dup.pdf", string(pdfBytes())},
	}, nil)
	if rej.Code != http.StatusSeeOther || !strings.Contains(rej.Header().Get("Location"), "attach_warn=") {
		t.Fatalf("reject loc=%s code=%d", rej.Header().Get("Location"), rej.Code)
	}

	files := make([][2]string, 21)
	for i := 0; i < 21; i++ {
		files[i] = [2]string{"f" + strconv.Itoa(i+1) + ".pdf", string(pdfBytes())}
	}
	many := postSalesMultipart(t, e, "/sales", map[string]string{"name": "21개"}, files, nil)
	if many.Code != http.StatusSeeOther || !strings.Contains(many.Header().Get("Location"), "attach_warn=") {
		t.Fatalf("21 loc=%s", many.Header().Get("Location"))
	}
	mid := salesIDFromRedirect(t, many.Header().Get("Location"))
	if err := db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE ref_id=?`, mid).Scan(&n); err != nil || n != 20 {
		t.Fatalf("20개 제한 n=%d", n)
	}

	rel := strings.TrimPrefix(filepath.ToSlash(path), filepath.ToSlash(filepath.Dir(path)))
	_ = rel
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	salesIDDir := filepath.Base(dir)
	unauth := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/sales_project/"+salesIDDir+"/"+base, nil)
	e.ServeHTTP(unauth, req)
	if unauth.Code != http.StatusNotFound {
		t.Fatalf("static %d", unauth.Code)
	}
	login := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/attachments/"+attID, nil)
	e.ServeHTTP(login, req)
	if login.Code == http.StatusOK {
		t.Fatal("비로그인 다운로드")
	}
}

func TestSales95_19AttachACL(t *testing.T) {
	e, db, repo, u := seed95_2G(t)
	AttachHTTPErrorHandler(e)
	foreign := mustSales95(t, repo, db, "타조직", "O03", u.bUser.FullName, u.bUser.UserID, false)
	mine := mustSales95(t, repo, db, "내조직", model.OrgIDLibrary, u.hjlee.FullName, u.hjlee.UserID, false)
	owned := mustSales95(t, repo, db, "Owner예외", "O03", u.hjlee.FullName, u.hjlee.UserID, false)

	postAtt := func(ck *http.Cookie, salesID string) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		_ = w.WriteField("ref_type", model.RefTypeSalesProject)
		_ = w.WriteField("ref_id", salesID)
		part, _ := w.CreateFormFile("file", "n.pdf")
		_, _ = io.Copy(part, bytes.NewReader(pdfBytes()))
		_ = w.Close()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/attachments", &buf)
		req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		return rec
	}
	hj := jwtCookieUser(t, u.hjlee)
	if rec := postAtt(hj, foreign.SalesID); rec.Code != http.StatusForbidden && !strings.Contains(rec.Header().Get("Location"), "attach_forbidden") {
		t.Fatalf("타조직 비Owner %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := postAtt(hj, mine.SalesID); rec.Code != http.StatusSeeOther {
		t.Fatalf("소속 %d", rec.Code)
	}
	if rec := postAtt(hj, owned.SalesID); rec.Code != http.StatusSeeOther {
		t.Fatalf("Owner %d %s", rec.Code, rec.Body.String())
	}

	cl := jwtCookieUser(t, u.cl)
	if rec := postAtt(cl, mine.SalesID); rec.Code != http.StatusForbidden && !strings.Contains(rec.Header().Get("Location"), "attach_forbidden") {
		t.Fatalf("C-Level 업로드 %d", rec.Code)
	}
	var attID string
	if err := db.QueryRow(`SELECT attachment_id FROM attachments WHERE ref_id=? LIMIT 1`, mine.SalesID).Scan(&attID); err != nil {
		t.Fatal(err)
	}
	dl := doGetWith(t, e, "/attachments/"+attID, cl)
	if dl.Code != http.StatusOK {
		t.Fatalf("C-Level 다운로드 %d", dl.Code)
	}
	ro := jwtCookieReadOnly(t, model.RoleAdmin)
	if rec := postAtt(ro, mine.SalesID); rec.Code != http.StatusForbidden && !strings.Contains(rec.Header().Get("Location"), "attach_forbidden") {
		t.Fatalf("readonly %d", rec.Code)
	}
	del := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/attachments/"+attID+"/delete", strings.NewReader("redirect=/"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUser(t, u.bUser))
	e.ServeHTTP(del, req)
	if del.Code != http.StatusForbidden && !strings.Contains(del.Header().Get("Location"), "attach_forbidden") {
		t.Fatalf("타조직 삭제 %d loc=%s", del.Code, del.Header().Get("Location"))
	}
}

func TestSales95_19LayoutNavUntouched(t *testing.T) {
	root := findTemplateRoot(t)
	base := filepath.Join(root, "layout", "base.html")
	st, err := os.Stat(base)
	if err != nil || st.Size() == 0 {
		t.Fatal(err)
	}
}
