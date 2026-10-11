package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestAccountLifecycleMigrationAndReserve(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "acc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := NewUserRepo(db)
	u := &model.User{Username: "KeepMe", PasswordHash: "x", FullName: "유지", Role: model.RoleTech, IsActive: true}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	taken, err := users.UsernameTaken("keepme", "")
	if err != nil || !taken {
		t.Fatalf("예약 taken=%v err=%v", taken, err)
	}
	if err := users.SoftDelete(u.UserID, "actor", "퇴사"); err != nil {
		t.Fatal(err)
	}
	if live, _ := users.GetByUsername("KeepMe"); live != nil {
		t.Fatal("삭제 후 로그인 조회")
	}
	got, _ := users.GetByID(u.UserID)
	if got == nil || !got.IsDeleted() {
		t.Fatal("소프트 삭제 아님")
	}
	if err := users.Restore(u.UserID, "actor", model.RoleSales, "O01", "복구"); err != nil {
		t.Fatal(err)
	}
	back, _ := users.GetByID(u.UserID)
	if back.IsDeleted() || back.Role != model.RoleSales {
		t.Fatalf("%+v", back)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_lifecycle_events WHERE user_id=? AND kind=?`, u.UserID, model.LifeLeaveExpire).Scan(&n); err != nil || n != 1 {
		t.Fatalf("연차 소멸 이벤트 n=%d err=%v", n, err)
	}
	up, err := os.ReadFile(filepath.Join("..", "..", "migrations", "057_account_lifecycle.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(up), "username_reservations") {
		t.Fatal("057 내용")
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "057_account_lifecycle.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(down)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(up)); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Fatal(err)
	}
}
