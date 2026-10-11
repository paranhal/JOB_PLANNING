package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestUpdateAuditedRollsBackWhenLogFails(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "audited.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := NewUserRepo(db)
	u := &model.User{Username: "rollme", PasswordHash: "x", FullName: "롤백", Role: model.RoleSales, IsActive: true, OrgID: "O01"}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE data_change_logs`); err != nil {
		t.Fatal(err)
	}
	u.Role = model.RoleTech
	if err := users.UpdateAudited(u, "", "등급·업무 영업 → 기술", `{"role":"sales"}`, `{"role":"tech"}`); err == nil {
		t.Fatal("감사 실패를 통과했다")
	}
	got, err := users.GetByID(u.UserID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.Role != model.RoleSales {
		t.Fatalf("롤백 안 됨 role=%s", got.Role)
	}
}
