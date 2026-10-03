package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

func TestRenameUsernameKeepsUserIDAndRejectsDuplicate(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "rename.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(db)
	r := NewUserRepo(db)
	a := &model.User{Username: "hjlee", PasswordHash: "x", FullName: "이해진", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-1"}
	b := &model.User{Username: "taken", PasswordHash: "x", FullName: "다른이", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-2"}
	if err := r.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(b); err != nil {
		t.Fatal(err)
	}
	id := a.UserID
	if err := r.RenameUsername(id, "taken"); err == nil {
		t.Fatal("중복 아이디를 받았다")
	}
	if err := r.RenameUsername(id, "haejin"); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByID(id)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.UserID != id {
		t.Fatalf("user_id 가 바뀌었다 %s", got.UserID)
	}
	if got.Username != "haejin" {
		t.Fatalf("username=%s", got.Username)
	}
	if got.UsernameChangedAt == "" {
		t.Fatal("username_changed_at 없음")
	}
}

func TestRewriteAssigneeNamesUpdatesLinkedRows(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "rename-name.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewUserRepo(db)
	u := &model.User{Username: "tech1", PasswordHash: "x", FullName: "옛이름", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-1"}
	if err := r.Create(u); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('c1','도서관','도서관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, assigned_to, assigned_user_id, status, org_id)
		VALUES ('AS1','R-1','c1','옛이름',?,'open','O01')`, u.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, assigned_to, assigned_user_id, status, org_id)
		VALUES ('AS2','R-2','c1','옛이름','','open','O01')`); err != nil {
		t.Fatal(err)
	}
	n, leftover, err := r.RewriteAssigneeNames(u.UserID, "옛이름", "새이름")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("갱신 %d want 1", n)
	}
	var linked string
	if err := db.QueryRow(`SELECT assigned_to FROM as_receipts WHERE as_id='AS1'`).Scan(&linked); err != nil || linked != "새이름" {
		t.Fatalf("연결된 행 %q err=%v", linked, err)
	}
	var orphan string
	if err := db.QueryRow(`SELECT assigned_to FROM as_receipts WHERE as_id='AS2'`).Scan(&orphan); err != nil || orphan != "옛이름" {
		t.Fatalf("user_id 없는 행이 바뀌면 안 됨 %q", orphan)
	}
	if len(leftover) == 0 || leftover[0].Count < 1 {
		t.Fatalf("못 찾은 행 목록 없음 %+v", leftover)
	}
}
