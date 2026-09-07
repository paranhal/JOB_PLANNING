package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func TestASCreateThenShow(t *testing.T) {
	e, _, asRepo, _, _ := newReceiptPhotoFixture(t)
	form := url.Values{
		"customer_id":      {"cust_a"},
		"symptom":          {"게이트 오류"},
		"received_by":      {"테스터"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/as/R") {
		t.Fatalf("redirect: %s", loc)
	}
	got, err := asRepo.GetByID(strings.TrimPrefix(loc, "/as/"))
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.ReceiptGroupID != "" {
		t.Fatalf("1건은 묶음이 없어야 한다: %s", got.ReceiptGroupID)
	}

	show := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost"+loc, nil)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req2)
	if show.Code != http.StatusOK {
		t.Fatalf("show status=%d body=%s", show.Code, show.Body.String())
	}
	if strings.Contains(show.Body.String(), "Internal Server Error") {
		t.Fatal("상세가 JSON 500이다")
	}
	if strings.Contains(show.Body.String(), "함께 접수된 건") {
		t.Fatal("1건 상세에 묶음이 보이면 안 된다")
	}
}

func TestASCreateInvalidCustomerRedirectsNot500(t *testing.T) {
	e, _, _, _, _ := newReceiptPhotoFixture(t)
	form := url.Values{
		"customer_id":      {"NO_SUCH"},
		"symptom":          {"게이트 오류"},
		"received_by":      {"테스터"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("JSON 500이면 안 된다: %s", rec.Body.String())
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/as/new") || !strings.Contains(loc, "err=") {
		t.Fatalf("redirect: %s", loc)
	}
}

func TestASNewFormHasEquipmentAdd(t *testing.T) {
	e, _, _, _, _ := newReceiptPhotoFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/new", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "＋장비 추가") {
		t.Fatal("신규 화면에 ＋장비 추가가 없다")
	}
	if !strings.Contains(body, `name="eq_asset_id"`) {
		t.Fatal("신규 화면에 eq_asset_id가 없다")
	}
	if !strings.Contains(body, `name="eq_symptom"`) {
		t.Fatal("신규 화면에 eq_symptom이 없다")
	}
	if !strings.Contains(body, `id="as-customer-search"`) {
		t.Fatal("기관 검색란이 없다")
	}
	if !strings.Contains(body, `name="eq_urgency_reason"`) {
		t.Fatal("긴급 사유가 없다")
	}
	if !strings.Contains(body, "이번 건 연락처") {
		t.Fatal("이번 건 연락처가 없다")
	}
	if !strings.Contains(body, "원청") || !strings.Contains(body, "내부") {
		t.Fatal("접수 채널에 원청·내부가 없다")
	}
	if strings.Contains(body, "요청주체") {
		t.Fatal("요청주체 필드가 남아 있다")
	}
	if strings.Contains(body, `name="eq_urgency"`) {
		t.Fatal("eq_urgency 입력란이 남아 있다")
	}
	if strings.Contains(body, `name="schedule_confirmed"`) {
		t.Fatal("일정 확정 체크박스가 남아 있다")
	}
}

func TestASCreateMultiEquipmentGroup(t *testing.T) {
	e, h, asRepo, _, _ := newReceiptPhotoFixture(t)
	a1 := &model.Asset{CustomerID: "cust_a", ProductName: "자가대출기"}
	if err := h.assetRepo.Create(a1); err != nil {
		t.Fatal(err)
	}
	a2 := &model.Asset{CustomerID: "cust_a", ProductName: "사서대출기"}
	if err := h.assetRepo.Create(a2); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"customer_id":        {"cust_a"},
		"received_by":        {"테스터"},
		"receipt_datetime":   {time.Now().Format("2006-01-02T15:04")},
		"confirm_contact":    {"010-1111-2222"},
		"eq_asset_id":        {a1.AssetID, a2.AssetID},
		"eq_symptom":         {"카드 인식 안 됨", "영수증 용지 걸림"},
		"eq_urgency":         {"high", "normal"},
		"eq_visit_date":      {"", ""},
		"eq_confirmed":       {"0", "0"},
		"eq_assigned_code":   {"", ""},
		"eq_assigned_custom": {"", ""},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/as/R") {
		t.Fatalf("redirect: %s", loc)
	}
	first, err := asRepo.GetByID(strings.TrimPrefix(loc, "/as/"))
	if err != nil || first == nil {
		t.Fatalf("get: %v", err)
	}
	if first.ReceiptGroupID == "" {
		t.Fatal("2건은 receipt_group_id가 같아야 한다")
	}
	if first.ConfirmContact != "010-1111-2222" {
		t.Fatalf("연락처=%q", first.ConfirmContact)
	}
	mates, err := asRepo.ListByReceiptGroup(first.ReceiptGroupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mates) != 2 {
		t.Fatalf("묶음 %d건", len(mates))
	}
	if mates[0].ASNumber == mates[1].ASNumber {
		t.Fatal("접수번호가 각각 달라야 한다")
	}
	symptoms := mates[0].Symptom + mates[1].Symptom
	if !strings.Contains(symptoms, "카드 인식") || !strings.Contains(symptoms, "영수증") {
		t.Fatalf("증상: %q / %q", mates[0].Symptom, mates[1].Symptom)
	}

	list := httptest.NewRecorder()
	reqList := httptest.NewRequest(http.MethodGet, "http://localhost/as", nil)
	reqList.AddCookie(jwtCookie(t))
	e.ServeHTTP(list, reqList)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d", list.Code)
	}
	if !strings.Contains(list.Body.String(), "🔗") {
		t.Fatal("목록에 🔗 뱃지가 없다")
	}

	show := httptest.NewRecorder()
	reqShow := httptest.NewRequest(http.MethodGet, "http://localhost"+loc, nil)
	reqShow.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, reqShow)
	if show.Code != http.StatusOK {
		t.Fatalf("show status=%d body=%s", show.Code, show.Body.String())
	}
	body := show.Body.String()
	if !strings.Contains(body, "함께 접수된 건 2건") {
		t.Fatal("상세에 함께 접수된 건이 없다")
	}
	var other string
	for _, m := range mates {
		if m.ASID != first.ASID {
			other = m.ASNumber
			break
		}
	}
	if other == "" || !strings.Contains(body, other) {
		t.Fatalf("상대 접수번호 %s 가 상세에 없다", other)
	}
}

func TestASCreateUrgencyReasonSetsHighAndConfirmsDate(t *testing.T) {
	e, _, asRepo, _, _ := newReceiptPhotoFixture(t)
	form := url.Values{
		"customer_id":       {"cust_a"},
		"received_by":       {"테스터"},
		"receipt_datetime":  {time.Now().Format("2006-01-02T15:04")},
		"eq_asset_id":       {""},
		"eq_symptom":        {"게이트 정지"},
		"eq_urgency_reason": {"rfid_ops_stop"},
		"eq_urgency_note":   {""},
		"eq_visit_date":     {"2026-09-01"},
		"eq_assigned_code":  {""},
		"eq_assigned_custom": {""},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	got, err := asRepo.GetByID(strings.TrimPrefix(rec.Header().Get("Location"), "/as/"))
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.Urgency != "high" {
		t.Fatalf("urgency=%q", got.Urgency)
	}
	if got.UrgencyReason != "rfid_ops_stop" {
		t.Fatalf("reason=%q", got.UrgencyReason)
	}
	if !got.ScheduleConfirmed {
		t.Fatal("예정일이 있으면 일정 확정이어야 한다")
	}
	if got.VisitScheduledDate != "2026-09-01" {
		t.Fatalf("visit=%q", got.VisitScheduledDate)
	}

	show := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost"+rec.Header().Get("Location"), nil)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req2)
	if !strings.Contains(show.Body.String(), "🔴") {
		t.Fatal("상세에 긴급 뱃지가 없다")
	}
}

func TestASCreateNoneReasonLeavesNormalUnconfirmed(t *testing.T) {
	e, _, asRepo, _, _ := newReceiptPhotoFixture(t)
	form := url.Values{
		"customer_id":       {"cust_a"},
		"received_by":       {"테스터"},
		"receipt_datetime":  {time.Now().Format("2006-01-02T15:04")},
		"eq_asset_id":       {""},
		"eq_symptom":        {"문의"},
		"eq_urgency_reason": {"none"},
		"eq_visit_date":     {""},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	got, err := asRepo.GetByID(strings.TrimPrefix(rec.Header().Get("Location"), "/as/"))
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.Urgency != "normal" {
		t.Fatalf("urgency=%q", got.Urgency)
	}
	if got.ScheduleConfirmed {
		t.Fatal("예정일 없으면 미확정")
	}
}

func TestASNewOmitsPartnerAndOwn(t *testing.T) {
	e, h, _, _, _ := newReceiptPhotoFixture(t)
	if err := h.customerRepo.Create(&model.Customer{
		OrgName: "채움씨앤아이", OfficialName: "채움씨앤아이", IsActive: true, PartyKind: model.PartyKindPartner,
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.customerRepo.Create(&model.Customer{
		OrgName: "비젼아이티", OfficialName: "비젼아이티", IsActive: true, PartyKind: model.PartyKindOwn,
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/new", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "채움씨앤아이") || strings.Contains(body, "비젼아이티") {
		t.Fatal("접수 화면에 협력사·자사가 보인다")
	}
	if !strings.Contains(body, "가나도서관") {
		t.Fatal("고객 가나도서관이 접수 JSON에 없다")
	}
}

func TestCustomerListPartyKindFilter(t *testing.T) {
	e, h, _, _, _ := newReceiptPhotoFixture(t)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/customers", h.Customer.List)
	if err := h.customerRepo.Create(&model.Customer{
		OrgName: "비젼아이티", OfficialName: "비젼아이티", IsActive: true, PartyKind: model.PartyKindOwn,
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/customers?party_kind=own", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "비젼아이티") {
		t.Fatal("own 필터에 자사가 없다")
	}
	if !strings.Contains(body, "전체 1개 기관") {
		t.Fatal("own 필터 건수가 1이 아니다")
	}
	if !strings.Contains(body, `name="party_kind"`) {
		t.Fatal("구분 필터가 없다")
	}
}

