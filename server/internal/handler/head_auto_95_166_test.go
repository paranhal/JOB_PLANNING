package handler

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/approval"
)

func Test95_166B3_AutoClearAndLeaveApproverText(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "head166b3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	head := &model.User{Username: "headb3", PasswordHash: HashPassword("pw"), FullName: "홍길동", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", Email: "h@ex.com", ProfileDone: true}
	admin := &model.User{Username: "adminb3", PasswordHash: HashPassword("pw"), FullName: "홍길순", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", Email: "a@ex.com", ProfileDone: true}
	staff := &model.User{Username: "staffb3", PasswordHash: HashPassword("pw"), FullName: "직원", Role: model.RoleTech, IsActive: true, OrgID: "O01", Mobile: "010-1", ProfileDone: true}
	for _, u := range []*model.User{head, admin, staff} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	h := New(db)
	eng := &approval.Engine{DB: db}
	actor := approval.Actor{UserID: admin.UserID, OrgID: "O01", Role: model.RoleOrgAdmin}
	if err := eng.SetOrgHead(actor, head.UserID, "O01"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO approval_requests(request_id, org_id, doc_type, doc_id, drafter_id, title, status)
		VALUES ('req-b3','O01','leave','doc-b3',?, '휴가','submitted')`, staff.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO approval_steps(step_id, request_id, step_no, approver_id, status)
		VALUES ('step-b3','req-b3',1,?,'waiting')`, head.UserID); err != nil {
		t.Fatal(err)
	}

	e := echoNew166(t, h)
	staffCk := jwtCookieUserClaims(t, staff)
	form := getAccount(t, e, "/leaves/new", staffCk)
	if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), "홍길동(팀장)") {
		t.Fatalf("팀장 안내 %d %s", form.Code, form.Body.String())
	}

	got, err := users.GetByID(head.UserID)
	if err != nil {
		t.Fatal(err)
	}
	got.IsActive = false
	if err := users.Update(got); err != nil {
		t.Fatal(err)
	}
	var flag int
	if err := db.QueryRow(`SELECT COALESCE(is_org_head,0) FROM employee_profiles WHERE user_id=?`, head.UserID).Scan(&flag); err != nil || flag != 0 {
		t.Fatalf("비활성 후 팀장 플래그 %d %v", flag, err)
	}
	var mode string
	if err := db.QueryRow(`SELECT pending_mode FROM org_head_history WHERE org_id='O01' AND prev_user_id=? ORDER BY rowid DESC LIMIT 1`, head.UserID).Scan(&mode); err != nil || mode != "auto" {
		t.Fatalf("자동 해제 이력 %s %v", mode, err)
	}
	var approver string
	_ = db.QueryRow(`SELECT approver_id FROM approval_steps WHERE step_id='step-b3'`).Scan(&approver)
	if approver != head.UserID {
		t.Fatal("자동 해제가 대기 문서 결재자를 바꿨다")
	}
	adminCk := jwtCookieUserClaims(t, admin)
	settings := getAccount(t, e, "/admin/approval-settings", adminCk)
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), "자동 해제") || !strings.Contains(settings.Body.String(), "홍길동 님은 팀장 후보에서 빠져") {
		t.Fatalf("관리자 알림 %d", settings.Code)
	}
	usersPage := getAccount(t, e, "/admin/users", adminCk)
	if !strings.Contains(usersPage.Body.String(), "data-head-auto-notice") {
		t.Fatal("사용자 목록 알림")
	}
	who, err := eng.OrgApprover("O01")
	if err != nil || who != admin.UserID {
		t.Fatalf("해제 후 상신 대상 %s %v", who, err)
	}
	form = getAccount(t, e, "/leaves/new", staffCk)
	if !strings.Contains(form.Body.String(), "조직관리자 홍길순 외 — 팀장 미지정") {
		t.Fatalf("미지정 안내 %s", form.Body.String())
	}

	for _, change := range []struct {
		name string
		fn   func(*model.User)
	}{
		{"readonly", func(u *model.User) { u.IsReadOnly = true }},
		{"move", func(u *model.User) { u.OrgID = "O02" }},
		{"role", func(u *model.User) { u.Role = model.RoleTech; u.Mobile = "010-2" }},
	} {
		fresh := &model.User{Username: "h" + change.name, PasswordHash: HashPassword("pw"), FullName: change.name, Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", Email: change.name + "@ex.com", ProfileDone: true}
		if err := users.Create(fresh); err != nil {
			t.Fatal(err)
		}
		if err := eng.SetOrgHead(actor, fresh.UserID, "O01"); err != nil {
			t.Fatal(change.name, err)
		}
		cur, err := users.GetByID(fresh.UserID)
		if err != nil {
			t.Fatal(err)
		}
		change.fn(cur)
		if err := users.Update(cur); err != nil {
			t.Fatal(change.name, err)
		}
		if err := db.QueryRow(`SELECT COALESCE(is_org_head,0) FROM employee_profiles WHERE user_id=?`, fresh.UserID).Scan(&flag); err != nil || flag != 0 {
			t.Fatalf("%s 후 팀장 플래그 %d %v", change.name, flag, err)
		}
	}
	gone := &model.User{Username: "hdel", PasswordHash: HashPassword("pw"), FullName: "삭제장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", Email: "d@ex.com", ProfileDone: true}
	if err := users.Create(gone); err != nil {
		t.Fatal(err)
	}
	if err := eng.SetOrgHead(actor, gone.UserID, "O01"); err != nil {
		t.Fatal(err)
	}
	if err := users.DeleteUserAccount(gone.UserID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COALESCE(is_org_head,0) FROM employee_profiles WHERE user_id=?`, gone.UserID).Scan(&flag); err != nil || flag != 0 {
		t.Fatalf("삭제 후 팀장 플래그 %d %v", flag, err)
	}
}

func echoNew166(t *testing.T, h *Handler) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.Renderer = NewRenderer()
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/users", h.Auth.UserList)
	g.GET("/admin/approval-settings", h.Approval.Page)
	g.GET("/leaves/new", h.Leave.New)
	return e
}
