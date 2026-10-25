package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func csrfPair() (*http.Cookie, url.Values) {
	tok := "testcsrf0123456789ab"
	return &http.Cookie{Name: csrfCookieName, Value: tok, Path: "/"}, url.Values{"csrf": {tok}}
}

func copySetCookies(rec *httptest.ResponseRecorder, req *http.Request) {
	for _, ck := range rec.Result().Cookies() {
		if ck != nil && ck.Value != "" && ck.MaxAge != -1 {
			req.AddCookie(ck)
		}
	}
}

func extractSubmitToken(body string) string {
	m := regexp.MustCompile(`name="submit_token" value="([^"]+)"`).FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func Test95_38_CreateFlashPopupAndRefreshOnce(t *testing.T) {
	e, _, asRepo, _, _ := newReceiptPhotoFixture(t)
	sym := "일이삼사오육칠팔구십하나"
	form := url.Values{
		"customer_id":      {"cust_a"},
		"symptom":          {sym},
		"received_by":      {"테스터"},
		"assigned_to":      {"최혜영"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/action") {
		t.Fatalf("loc=%s", loc)
	}
	var flash *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == flashOKCookie && ck.Value != "" && ck.MaxAge != -1 {
			flash = ck
		}
	}
	if flash == nil {
		t.Fatal("jp_flash_ok 없음")
	}
	got := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodGet, "http://localhost"+loc, nil)
	greq.AddCookie(jwtCookie(t))
	greq.AddCookie(flash)
	e.ServeHTTP(got, greq)
	if got.Code != http.StatusOK {
		t.Fatalf("action %d", got.Code)
	}
	body := got.Body.String()
	as, _ := asRepo.GetByID(repository.OrgAll, asIDFromHref(loc))
	if as == nil {
		t.Fatal("as nil")
	}
	if !strings.Contains(body, as.ASNumber) || !strings.Contains(body, "가나도서관") {
		t.Fatal("팝업에 접수번호·기관 없음")
	}
	if !strings.Contains(body, "일이삼사오육칠팔구십…") {
		t.Fatalf("10자+… 없음: %s", asSymptomSnippet(sym))
	}
	if !strings.Contains(body, "저장되었습니다.") {
		t.Fatal("저장되었습니다 없음")
	}
	again := httptest.NewRecorder()
	areq := httptest.NewRequest(http.MethodGet, "http://localhost"+loc, nil)
	areq.AddCookie(jwtCookie(t))
	e.ServeHTTP(again, areq)
	if strings.Contains(again.Body.String(), "저장되었습니다.") {
		t.Fatal("새로고침에 팝업이 다시 뜸")
	}
}

func Test95_38_ShortSymptomNoEllipsis(t *testing.T) {
	if asSymptomSnippet("짧은증상") != "짧은증상" {
		t.Fatalf("%q", asSymptomSnippet("짧은증상"))
	}
}

func Test95_38_MultiEquipmentGoesToListPopup(t *testing.T) {
	e, h, asRepo, _, _ := newReceiptPhotoFixture(t)
	a1 := &model.Asset{CustomerID: "cust_a", ProductName: "자가대출기"}
	a2 := &model.Asset{CustomerID: "cust_a", ProductName: "사서대출기"}
	if err := h.assetRepo.Create(a1); err != nil {
		t.Fatal(err)
	}
	if err := h.assetRepo.Create(a2); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"customer_id":        {"cust_a"},
		"received_by":        {"테스터"},
		"assigned_to":        {"최혜영"},
		"receipt_datetime":   {time.Now().Format("2006-01-02T15:04")},
		"eq_asset_id":        {a1.AssetID, a2.AssetID},
		"eq_symptom":         {"카드 인식 안 됨", "영수증 용지 걸림"},
		"eq_assigned_code":   {"", ""},
		"eq_assigned_custom": {"", ""},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Header().Get("Location") != "/as" {
		t.Fatalf("loc=%s", rec.Header().Get("Location"))
	}
	list := httptest.NewRecorder()
	lreq := httptest.NewRequest(http.MethodGet, "http://localhost/as", nil)
	lreq.AddCookie(jwtCookie(t))
	copySetCookies(rec, lreq)
	e.ServeHTTP(list, lreq)
	body := list.Body.String()
	if !strings.Contains(body, "저장되었습니다.") {
		t.Fatal("목록 팝업 없음")
	}
	mates, _, _ := asRepo.ListFiltered(repository.OrgAll, "", "", "", nil, "", "", 1, 20)
	n := 0
	for _, it := range mates {
		if strings.Contains(body, it.ASNumber) {
			n++
		}
	}
	if n < 2 {
		t.Fatalf("팝업 2건 표시 n=%d", n)
	}
}

func Test95_38_SubmitTokenIdempotentAndDuplicateWarn(t *testing.T) {
	e, _, asRepo, _, _ := newReceiptPhotoFixture(t)
	newPage := doGetWith(t, e, "/as/new", jwtCookie(t))
	tok := extractSubmitToken(newPage.Body.String())
	if tok == "" {
		t.Fatal("submit_token 없음")
	}
	if !strings.Contains(newPage.Body.String(), `data-submit-guard="1"`) || !strings.Contains(newPage.Body.String(), "저장 중…") {
		t.Fatal("이중 클릭 방지 마크업 없음")
	}
	form := url.Values{
		"customer_id":      {"cust_a"},
		"symptom":          {"중복방지증상-38"},
		"received_by":      {"테스터"},
		"assigned_to":      {"최혜영"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
		"submit_token":     {tok},
	}
	post := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(jwtCookie(t))
		e.ServeHTTP(rec, req)
		return rec
	}
	first := post()
	if first.Code != http.StatusSeeOther {
		t.Fatalf("1st %d", first.Code)
	}
	id1 := asIDFromHref(first.Header().Get("Location"))
	second := post()
	if second.Code != http.StatusSeeOther {
		t.Fatalf("2nd %d", second.Code)
	}
	if asIDFromHref(second.Header().Get("Location")) != id1 {
		t.Fatal("토큰 재사용이 새 접수를 만들었다")
	}
	items, _, _ := asRepo.ListFiltered(repository.OrgAll, "", "중복방지증상-38", "", nil, "", "", 1, 50)
	if len(items) != 1 {
		t.Fatalf("토큰 후 건수 %d", len(items))
	}

	form.Set("submit_token", newASSubmitToken())
	dup := post()
	if dup.Code != http.StatusUnprocessableEntity {
		t.Fatalf("중복 경고 status=%d", dup.Code)
	}
	if !strings.Contains(dup.Body.String(), "방금 같은 내용이 접수되었습니다") {
		t.Fatal("중복 안내 없음")
	}
	form.Set("confirm_dup", "1")
	form.Set("submit_token", newASSubmitToken())
	forced := post()
	if forced.Code != http.StatusSeeOther {
		t.Fatalf("강제 생성 %d %s", forced.Code, forced.Body.String())
	}
	items, _, _ = asRepo.ListFiltered(repository.OrgAll, "", "중복방지증상-38", "", nil, "", "", 1, 50)
	if len(items) != 2 {
		t.Fatalf("강제 후 건수 %d", len(items))
	}
}

func Test95_38_AssigneeRejectHasNoSavePopup(t *testing.T) {
	e, h, _, _, _ := newReceiptPhotoFixture(t)
	users := repository.NewUserRepo(h.AS.db)
	oa := &model.User{Username: "oa38", PasswordHash: HashPassword("pw"), FullName: "조직38", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	oa, _ = users.GetByUsername("oa38")
	form := url.Values{
		"customer_id":        {"cust_a"},
		"received_by":        {"조직38"},
		"receipt_datetime":   {time.Now().Format("2006-01-02T15:04")},
		"eq_symptom":         {"담당자없음38"},
		"eq_assigned_code":   {""},
		"eq_assigned_custom": {""},
		"eq_work_group_id":   {""},
	}
	rec := postForm(t, e, "/as", form, jwtCookieUser(t, oa))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "저장되었습니다.") {
		t.Fatal("거부인데 저장 팝업")
	}
}

func Test95_38_DeleteCSRFAndModalMarkup(t *testing.T) {
	e, _, asRepo, _, _ := newReceiptPhotoFixture(t)
	as := &model.ASReceipt{CustomerID: "cust_a", Symptom: "삭제대상38", Status: "received", ReceiptDatetime: time.Now(), OrgID: model.OrgIDLibrary}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	bad := httptest.NewRecorder()
	breq := httptest.NewRequest(http.MethodPost, "/as/"+as.ASID+"/delete", strings.NewReader("x=1"))
	breq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	breq.AddCookie(jwtCookie(t))
	e.ServeHTTP(bad, breq)
	if bad.Code != http.StatusForbidden && bad.Code != http.StatusSeeOther {
		t.Fatalf("csrf 없는 POST %d", bad.Code)
	}
	if bad.Code == http.StatusSeeOther {
		t.Fatal("CSRF 없이 삭제되면 안 된다")
	}
	list := doGetWith(t, e, "/as", jwtCookie(t))
	if !strings.Contains(list.Body.String(), "js-as-delete") || !strings.Contains(list.Body.String(), "as-delete-title") {
		t.Fatal("삭제 모달/버튼 마크업 없음")
	}
	if !strings.Contains(list.Body.String(), "삭제하면 연결된 일일 업무도 정리됩니다") {
		t.Fatal("확인 문구 없음")
	}
}

func Test95_38_DeletePermissionMatrix(t *testing.T) {
	e, h, asRepo, _, _ := newReceiptPhotoFixture(t)
	db := h.AS.db
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES ('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('cust_si','SI학교','SI학교',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	mk := func(u *model.User) *model.User {
		t.Helper()
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
		got, _ := users.GetByUsername(u.Username)
		return got
	}
	vis := mk(&model.User{Username: "vis38", PasswordHash: "x", FullName: "비전38", Role: model.RoleVisionAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true})
	oa := mk(&model.User{Username: "oa38b", PasswordHash: "x", FullName: "조직38b", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true})
	oa2 := mk(&model.User{Username: "oa38si", PasswordHash: "x", FullName: "SI장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true})
	recv := mk(&model.User{Username: "recv38", PasswordHash: "x", FullName: "접수38", Role: model.RoleSupport, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true})
	other := mk(&model.User{Username: "recv38b", PasswordHash: "x", FullName: "다른접수", Role: model.RoleSupport, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true})
	tech := mk(&model.User{Username: "tech38", PasswordHash: "x", FullName: "기술38", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true})
	cl := mk(&model.User{Username: "cl38", PasswordHash: "x", FullName: "대표38", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true})

	newAS := func(status, created, received, org string) *model.ASReceipt {
		as := &model.ASReceipt{
			CustomerID: "cust_a", Symptom: "권한표-" + created + status, Status: status,
			ReceiptDatetime: time.Now(), OrgID: org, CreatedByUserID: created, ReceivedBy: received,
			AssignedTo: "기술38", AssignedUserID: tech.UserID,
		}
		if org == "O03" {
			as.CustomerID = "cust_si"
		}
		if err := asRepo.Create(as); err != nil {
			t.Fatal(err)
		}
		as.CreatedByUserID = created
		as.ReceivedBy = received
		if err := asRepo.UpdateReceipt(as); err != nil {
			_, _ = db.Exec(`UPDATE as_receipts SET created_by_user_id=?, received_by=? WHERE as_id=?`, created, received, as.ASID)
		}
		return as
	}
	open := newAS("received", recv.UserID, recv.FullName, model.OrgIDLibrary)
	withProc := newAS("assigned", recv.UserID, recv.FullName, model.OrgIDLibrary)
	if err := h.AS.processRepo.Create(&model.ASProcess{ASID: withProc.ASID, Worker: "기술38", Notes: "조치", WorkContent: "확인"}); err != nil {
		t.Fatal(err)
	}
	done := newAS("completed", recv.UserID, recv.FullName, model.OrgIDLibrary)
	otherOrg := newAS("received", oa2.UserID, oa2.FullName, "O03")

	try := func(u *model.User, as *model.ASReceipt) int {
		t.Helper()
		ck, form := csrfPair()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/as/"+as.ASID+"/delete", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(jwtCookieUser(t, u))
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := try(vis, open); c != http.StatusSeeOther {
		t.Fatalf("비전 미처리 %d", c)
	}
	open2 := newAS("received", recv.UserID, recv.FullName, model.OrgIDLibrary)
	if c := try(oa, open2); c != http.StatusSeeOther {
		t.Fatalf("자기 조직장 미처리 %d", c)
	}
	if c := try(oa2, open); c != http.StatusNotFound && c != http.StatusForbidden {
		t.Fatalf("타 조직장 %d", c)
	}
	if c := try(oa, otherOrg); c != http.StatusNotFound && c != http.StatusForbidden {
		t.Fatalf("타조직 건 %d", c)
	}
	open3 := newAS("received", recv.UserID, recv.FullName, model.OrgIDLibrary)
	if c := try(recv, open3); c != http.StatusSeeOther {
		t.Fatalf("접수자 미처리 %d", c)
	}
	if c := try(other, withProc); c != http.StatusForbidden && c != http.StatusSeeOther {
		t.Fatalf("다른 접수자 %d", c)
	}
	still, _ := asRepo.GetByID(repository.OrgAll, withProc.ASID)
	if still == nil {
		t.Fatal("다른 접수자가 조치건을 지움")
	}
	if c := try(recv, withProc); c != http.StatusForbidden {
		t.Fatalf("접수자 조치건 %d", c)
	}
	openTech := newAS("received", recv.UserID, recv.FullName, model.OrgIDLibrary)
	if c := try(tech, openTech); c != http.StatusForbidden {
		t.Fatalf("담당 tech %d", c)
	}
	openCL := newAS("received", recv.UserID, recv.FullName, model.OrgIDLibrary)
	if c := try(cl, openCL); c != http.StatusForbidden && c != http.StatusSeeOther {
		t.Fatalf("c_level %d", c)
	}
	if c := try(oa, done); c != http.StatusSeeOther && c != http.StatusForbidden {
		// 잠금이면 303 목록/잠금 안내 가능
		if c == http.StatusOK {
			t.Fatalf("완료 건 조직장 %d", c)
		}
	}
	act := doGetWith(t, e, asOpenHref(withProc.ASID), jwtCookieUser(t, recv))
	if strings.Contains(act.Body.String(), `data-as-id="`+withProc.ASID+`"`) && strings.Contains(act.Body.String(), "js-as-delete") {
		t.Fatal("조치 이력 있는 건에 접수자 삭제 버튼")
	}
}

func Test95_38_DeleteCascadeAndLinkedRefuse(t *testing.T) {
	e, h, asRepo, attachRepo, _ := newReceiptPhotoFixture(t)
	as := &model.ASReceipt{
		CustomerID: "cust_a", Symptom: "연쇄38", Status: "received", AssignedTo: "최혜영",
		ReceiptDatetime: time.Now(), OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	h.AS.syncASPlannedDailyTask(as.ASID)
	if h.AS.partsRepo != nil {
		_ = h.AS.partsRepo.ReplaceByAS(as.ASID, "", []model.ASPartUsed{{PartName: "나사", Qty: 1}})
	}
	if attachRepo != nil {
		if err := attachRepo.Create(&model.Attachment{RefType: model.AttachRefAS, RefID: as.ASID, FileName: "a.jpg", FilePath: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.AS.db.Exec(`INSERT INTO work_assign_notices (notice_id, user_id, source_type, source_id, assigned_by, assigned_at)
		VALUES ('WAN-38','admin-id','as',?, '관리자', datetime('now'))`, as.ASID); err != nil {
		t.Log(err)
	}
	child := &model.ASReceipt{
		CustomerID: "cust_a", Symptom: "후속38", Status: "received", ParentASID: as.ASID, IsReopen: true,
		ReceiptDatetime: time.Now(), OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(child); err != nil {
		t.Fatal(err)
	}
	ck, form := csrfPair()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/as/"+as.ASID+"/delete", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("연결 거부 status=%d", rec.Code)
	}
	if still, _ := asRepo.GetByID(repository.OrgAll, as.ASID); still == nil {
		t.Fatal("연결 건이 삭제됨")
	}
	_, _ = h.AS.db.Exec(`UPDATE as_receipts SET parent_as_id='' WHERE as_id=?`, child.ASID)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/as/"+as.ASID+"/delete", strings.NewReader(form.Encode()))
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req2.AddCookie(jwtCookie(t))
	req2.AddCookie(ck)
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusSeeOther {
		t.Fatalf("삭제 %d loc=%s body=%s", rec2.Code, rec2.Header().Get("Location"), rec2.Body.String())
	}
	if gone, _ := asRepo.GetByID(repository.OrgAll, as.ASID); gone != nil {
		t.Fatal("접수 남음")
	}
	var n int
	_ = h.AS.db.QueryRow(`SELECT COUNT(*) FROM as_processes WHERE as_id=?`, as.ASID).Scan(&n)
	if n != 0 {
		t.Fatalf("process %d", n)
	}
	_ = h.AS.db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type='as' AND source_id=?`, as.ASID).Scan(&n)
	if n != 0 {
		t.Fatalf("work_tasks %d", n)
	}
	_ = h.AS.db.QueryRow(`SELECT COUNT(*) FROM as_parts_used WHERE as_id=?`, as.ASID).Scan(&n)
	if n != 0 {
		t.Fatalf("parts %d", n)
	}
	_ = h.AS.db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE ref_id=?`, as.ASID).Scan(&n)
	if n != 0 {
		t.Fatalf("attach %d", n)
	}
	_ = h.AS.db.QueryRow(`SELECT COUNT(*) FROM work_assign_notices WHERE source_id=?`, as.ASID).Scan(&n)
	if n != 0 {
		t.Fatalf("notices %d", n)
	}
	list := httptest.NewRecorder()
	lreq := httptest.NewRequest(http.MethodGet, "/as", nil)
	lreq.AddCookie(jwtCookie(t))
	copySetCookies(rec2, lreq)
	e.ServeHTTP(list, lreq)
	if !strings.Contains(list.Body.String(), "삭제되었습니다.") {
		t.Fatal("삭제 팝업 없음")
	}
}

