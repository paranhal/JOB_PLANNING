package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func Test95_166B2_PreviewPendingAndHistory(t *testing.T) {
	e, users, db := newHead166(t)
	oa := &model.User{Username: "oa2a", PasswordHash: HashPassword("pw"), FullName: "일팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", Email: "a@ex.com", ProfileDone: true}
	oa2 := &model.User{Username: "oa2b", PasswordHash: HashPassword("pw"), FullName: "이팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", Email: "b@ex.com", ProfileDone: true}
	for _, u := range []*model.User{oa, oa2} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	ck := jwtCookieUserClaims(t, oa)
	page := getAccount(t, e, "/admin/users", ck)
	csrfCk, csrf := csrfFromRec(t, page)

	postHead := func(vals url.Values) *httptest.ResponseRecorder {
		t.Helper()
		vals.Set("csrf", csrf)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin/approval-settings/head", strings.NewReader(vals.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(ck)
		req.AddCookie(csrfCk)
		e.ServeHTTP(rec, req)
		return rec
	}
	if rec := postHead(url.Values{"head_user_id": {oa.UserID}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("지정 %d %s", rec.Code, rec.Body.String())
	}
	insertStep := func(id, status, approver string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO approval_requests(request_id, org_id, doc_type, doc_id, drafter_id, title, status)
			VALUES (?,?,?,?,?,?,?)`, "req-"+id, "O01", "leave", "doc-"+id, oa2.UserID, "휴가", "submitted"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO approval_steps(step_id, request_id, step_no, approver_id, status)
			VALUES (?,?,1,?,?)`, "step-"+id, "req-"+id, approver, status); err != nil {
			t.Fatal(err)
		}
	}
	insertStep("w1", "waiting", oa.UserID)
	insertStep("w2", "waiting", oa.UserID)
	insertStep("done", "done", oa.UserID)
	insertStep("rej", "rejected", oa.UserID)
	insertStep("other", "waiting", oa2.UserID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/approval-settings/head-preview", strings.NewReader(url.Values{
		"csrf": {csrf}, "head_user_id": {oa2.UserID},
	}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	req.AddCookie(csrfCk)
	e.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "이팀장(팀장)") || !strings.Contains(body, ">2</strong>") || !strings.Contains(body, `value="keep" checked`) {
		t.Fatalf("미리보기 %d %s", rec.Code, body)
	}

	moved := postHead(url.Values{"head_user_id": {oa2.UserID}, "pending_mode": {"move"}})
	if moved.Code != http.StatusSeeOther {
		t.Fatalf("이관 %d %s", moved.Code, moved.Body.String())
	}
	loc, _ := url.QueryUnescape(moved.Header().Get("Location"))
	if !strings.Contains(loc, "2건") {
		t.Fatalf("이관 안내 %s", loc)
	}
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count(`SELECT COUNT(*) FROM approval_steps WHERE step_id IN ('step-w1','step-w2') AND status='waiting' AND approver_id=?`, oa2.UserID) != 2 {
		t.Fatal("대기 문서가 새 팀장에게 넘어가지 않았다")
	}
	if count(`SELECT COUNT(*) FROM approval_steps WHERE step_id='step-done' AND status='done' AND approver_id=?`, oa.UserID) != 1 {
		t.Fatal("완료 문서가 바뀌었다")
	}
	if count(`SELECT COUNT(*) FROM approval_steps WHERE step_id='step-rej' AND status='rejected' AND approver_id=?`, oa.UserID) != 1 {
		t.Fatal("반려 문서가 바뀌었다")
	}
	if count(`SELECT COUNT(*) FROM approval_steps WHERE step_id='step-other' AND approver_id=?`, oa2.UserID) != 1 {
		t.Fatal("다른 결재자의 대기 문서를 건드렸다")
	}
	waiting := count(`SELECT COUNT(*) FROM approval_steps WHERE status='waiting' AND approver_id=?`, oa2.UserID)
	if rec := postHead(url.Values{"head_user_id": {oa2.UserID}, "pending_mode": {"move"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("재실행 %d", rec.Code)
	}
	if count(`SELECT COUNT(*) FROM approval_steps WHERE status='waiting' AND approver_id=?`, oa2.UserID) != waiting {
		t.Fatal("두 번째 저장이 대기 문서를 바꿨다")
	}
	if count(`SELECT COUNT(*) FROM org_head_history WHERE org_id='O01'`) < 2 {
		t.Fatal("이력 없음")
	}
	settings := getAccount(t, e, "/admin/approval-settings", ck)
	if !strings.Contains(settings.Body.String(), "팀장 변경 이력") || !strings.Contains(settings.Body.String(), "이팀장") {
		t.Fatal("이력 화면")
	}
	if rec := postHead(url.Values{"head_user_id": {""}, "pending_mode": {"move"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("해제 이관 %d %s", rec.Code, rec.Body.String())
	}
	if count(`SELECT COUNT(*) FROM approval_steps WHERE step_id IN ('step-w1','step-w2') AND status='waiting' AND approver_id=?`, oa.UserID) != 2 {
		t.Fatal("해제 후 조직관리자에게 넘어가지 않았다")
	}
}
