package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestASShowReportButtonByHistory(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, req)
	if page.Code != http.StatusOK {
		t.Fatalf("%d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "조치완료보고서 생성") {
		t.Fatal("이력 없는 단추가 생성 글자가 아니다")
	}
	if strings.Contains(body, "보고서 재발급") {
		t.Fatal("이력 없는데 재발급이다")
	}

	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	draft := h.AS.buildASReportDraft(as, time.Now())
	if rec := postASReport(t, e, asID, draft); rec.Code != http.StatusOK {
		t.Fatalf("발급 %d", rec.Code)
	}
	page = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, req)
	body = page.Body.String()
	if !strings.Contains(body, "보고서 재발급") {
		t.Fatal("이력 있는데 재발급이 없다")
	}
}

func TestASReportIssueRedirectsWithSavedMessage(t *testing.T) {
	e, h, asRepo, attachRepo, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	draft := h.AS.buildASReportDraft(as, time.Now())
	form := reportForm(draft)
	form.Del("return_file")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/report", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 뒤 이동이어야 한다 %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/as/"+asID) || !strings.Contains(loc, "ok=report_saved") {
		t.Fatalf("Location %s", loc)
	}
	items, err := attachRepo.ListByRef(model.RefTypeASReport, asID)
	if err != nil || len(items) != 1 {
		t.Fatalf("이력 %d %v", len(items), err)
	}
	if _, err := os.Stat(items[0].FilePath); err != nil {
		t.Fatal("파일이 없다")
	}
	show := httptest.NewRecorder()
	sreq := httptest.NewRequest(http.MethodGet, "http://localhost"+loc, nil)
	sreq.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, sreq)
	if !strings.Contains(show.Body.String(), "저장했습니다") {
		t.Fatal("저장했습니다 가 없다")
	}
}

func TestASReportSignaturesByAssignableName(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	a := &model.User{Username: "choi-a", PasswordHash: HashPassword("pw"), FullName: "최혜영", Role: model.RoleTech, IsActive: true, OrgID: admin.OrgID}
	b := &model.User{Username: "choi-b", PasswordHash: HashPassword("pw"), FullName: "최혜영", Role: model.RoleTech, IsActive: true, OrgID: admin.OrgID}
	if err := h.AS.userRepo.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := h.AS.userRepo.Create(b); err != nil {
		t.Fatal(err)
	}
	writeUserSig(t, h, a.UserID, inkPNGAt(t, 8, 28))
	writeUserSig(t, h, b.UserID, inkPNGAt(t, 50, 70))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/report/signatures?inspector_id="+url.QueryEscape(a.UserID), nil)
	req.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out map[string]reportSigJSONSet
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out["inspector"].Has || len(out["inspector"].Items) != 1 {
		t.Fatalf("%+v", out["inspector"])
	}

	form := reportForm(h.AS.buildASReportDraft(mustAS(t, asRepo, asID), time.Now()))
	form.Set("inspector", "최혜영")
	form.Set("inspector_id", a.UserID)
	form.Set("use_saved_sig", "1")
	irec := httptest.NewRecorder()
	ireq := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/report", strings.NewReader(form.Encode()))
	ireq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ireq.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(irec, ireq)
	if irec.Code != http.StatusOK {
		t.Fatalf("발급 %d %s", irec.Code, irec.Header().Get("Location"))
	}
	sec := string(zipFileBytes(t, irec.Body.Bytes(), "Contents/section0.xml"))
	if !strings.Contains(sec, "<hp:pic") {
		t.Fatal("ID 로 맞춘 사인이 없다")
	}
}

func TestASReportPeopleComeFromAction(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	tech := &model.User{Username: "insp-act", PasswordHash: HashPassword("pw"), FullName: "조치담당", Role: model.RoleTech, IsActive: true, OrgID: admin.OrgID}
	if err := h.AS.userRepo.Create(tech); err != nil {
		t.Fatal(err)
	}
	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	as.AssignedTo = tech.FullName
	as.AssignedUserID = tech.UserID
	as.CustomerConfirmer = "한채운"
	if err := asRepo.Update(as); err != nil {
		t.Fatal(err)
	}
	conf := &model.User{Username: "conf-act", PasswordHash: HashPassword("pw"), FullName: "한채운", Role: model.RoleTech, IsActive: true, OrgID: admin.OrgID}
	if err := h.AS.userRepo.Create(conf); err != nil {
		t.Fatal(err)
	}

	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/report", nil)
	req.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(page, req)
	body := page.Body.String()
	if !strings.Contains(body, `id="report-inspector-name" value="조치담당"`) {
		t.Fatal("점검자가 조치 처리담당자가 아니다")
	}
	if !strings.Contains(body, `id="report-inspector-id" value="`+tech.UserID+`"`) {
		t.Fatal("점검자 ID 가 조치 배정이 아니다")
	}
	if !strings.Contains(body, `id="report-confirmer-name" value="한채운"`) {
		t.Fatal("확인자가 조치 고객확인자가 아니다")
	}
	if !strings.Contains(body, `id="report-confirmer-id" value="`+conf.UserID+`"`) {
		t.Fatal("확인자 이름이 ID 로 안 맞았다")
	}
	if !strings.Contains(body, `type="hidden" name="inspector"`) || !strings.Contains(body, `type="hidden" name="confirmer"`) {
		t.Fatal("점검자·확인자를 조치 값으로 고정하지 않았다")
	}
}

func TestASReportConfirmerMatchesAssignableNameToID(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	han := &model.User{Username: "han-c", PasswordHash: HashPassword("pw"), FullName: "한채운", Role: model.RoleTech, IsActive: true, OrgID: admin.OrgID}
	if err := h.AS.userRepo.Create(han); err != nil {
		t.Fatal(err)
	}
	writeUserSig(t, h, han.UserID, inkPNGAt(t, 8, 28))

	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	as.CustomerConfirmer = "한채운"
	if err := asRepo.Update(as); err != nil {
		t.Fatal(err)
	}
	page := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/report", nil)
	preq.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(page, preq)
	if page.Code != http.StatusOK {
		t.Fatalf("미리보기 %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, `id="report-confirmer-id" value="`+han.UserID+`"`) {
		t.Fatalf("확인자 이름이 ID 로 안 맞았다: %s", han.UserID)
	}
	if !strings.Contains(body, `id="btn-report-sig-confirmer" data-slot="confirmer" data-has-account="1"`) {
		t.Fatal("확인자 계정 매칭이 없다")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/report/signatures?confirmer="+url.QueryEscape("한채운"), nil)
	req.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("조회 %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]reportSigJSONSet
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["confirmer"].UserID != han.UserID || !out["confirmer"].Has {
		t.Fatalf("이름→ID %+v want %s", out["confirmer"], han.UserID)
	}

	form := reportForm(h.AS.buildASReportDraft(mustAS(t, asRepo, asID), time.Now()))
	form.Set("confirmer", "한채운")
	form.Del("confirmer_id")
	form.Set("use_saved_sig", "1")
	irec := httptest.NewRecorder()
	ireq := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/report", strings.NewReader(form.Encode()))
	ireq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ireq.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(irec, ireq)
	if irec.Code != http.StatusOK {
		t.Fatalf("발급 %d %s", irec.Code, irec.Header().Get("Location"))
	}
	sec := string(zipFileBytes(t, irec.Body.Bytes(), "Contents/section0.xml"))
	if !strings.Contains(sec, "한채운") {
		t.Fatal("확인자 이름이 없다")
	}
	if !strings.Contains(sec, "<hp:pic") {
		t.Fatal("확인자 이름→ID 사인이 없다")
	}
}

func TestASReportSignaturesForbiddenWithoutProcess(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	sales := &model.User{Username: "sales-no", PasswordHash: HashPassword("pw"), FullName: "영업", Role: model.RoleSales, IsActive: true, IsReadOnly: true, OrgID: model.OrgIDLibrary}
	if err := h.AS.userRepo.Create(sales); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/report/signatures", nil)
	req.AddCookie(jwtCookieUserClaims(t, sales))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("권한 없이 열렸다 %d", rec.Code)
	}
}

func TestAccountSecondSignatureSlot(t *testing.T) {
	_, users, _, up := newSignatureApp(t)
	u := &model.User{
		Username: "two-sig", PasswordHash: HashPassword("pw"), FullName: "두사인",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-1",
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	if _, err := storeSignaturePNGAt(users, up, u.UserID, "", inkPNG(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := storeSignaturePNGAt(users, up, u.UserID, "", inkPNG(t)); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByID(u.UserID)
	if got == nil || !strings.Contains(got.SignaturePath, u.UserID+".png") {
		t.Fatalf("1번 %q", got.SignaturePath)
	}
	if !strings.Contains(got.SignaturePath2, u.UserID+"_2.png") {
		t.Fatalf("2번 %q", got.SignaturePath2)
	}
	if _, err := os.Stat(filepath.Join(up, "signatures", u.UserID+"_2.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := storeSignaturePNGAt(users, up, u.UserID, "", inkPNG(t)); err == nil {
		t.Fatal("둘 다 찼는데 말없이 덮었다")
	}
}

func mustAS(t *testing.T, asRepo *repository.ASRepo, asID string) *model.ASReceipt {
	t.Helper()
	as, err := asRepo.GetByID(repository.OrgAll, asID)
	if err != nil || as == nil {
		t.Fatal(err)
	}
	return as
}
