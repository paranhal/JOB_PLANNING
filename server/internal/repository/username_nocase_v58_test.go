package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestUsernameNocaseLookupAndTaken(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "nocase.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := NewUserRepo(db)
	u := &model.User{
		Username: "LeeKim", PasswordHash: "x", FullName: "이김",
		Role: model.RoleTech, IsActive: true, OrgID: "O01",
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	got, err := users.GetByUsername("leekim")
	if err != nil || got == nil || got.UserID != u.UserID {
		t.Fatalf("nocase get %+v %v", got, err)
	}
	taken, err := users.UsernameTaken(" LEEKIM ", "")
	if err != nil || !taken {
		t.Fatalf("taken=%v err=%v", taken, err)
	}
	dup := &model.User{
		Username: "leekim", PasswordHash: "x", FullName: "중복",
		Role: model.RoleSales, IsActive: true, OrgID: "O01",
	}
	if err := users.Create(dup); err == nil {
		t.Fatal("대소문자만 다른 아이디가 들어갔다")
	}
}
