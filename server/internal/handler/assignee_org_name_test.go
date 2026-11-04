package handler

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestUserDisplayName(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "udn.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("adminadmin")); err != nil {
		t.Fatal(err)
	}
	alive := &model.User{Username: "alive111", PasswordHash: "x", FullName: "최혜영", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	gone := &model.User{Username: "gone111", PasswordHash: "x", FullName: "퇴사자김", Role: model.RoleTech, IsActive: false, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(alive); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(gone); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET deleted_at=CURRENT_TIMESTAMP WHERE user_id=?`, gone.UserID); err != nil {
		t.Fatal(err)
	}

	if got := userDisplayName(users, alive.UserID, "다른이름"); got != "최혜영" {
		t.Fatalf("ID 있음 want 최혜영 got %s", got)
	}
	missingID := "USR-0000000000000000000"
	if got := userDisplayName(users, missingID, "지원자박"); got != "지원자박" {
		t.Fatalf("목록에 없음 stored want 지원자박 got %s", got)
	}
	if got := userDisplayName(users, gone.UserID, "무시"); got != "퇴사자김" {
		t.Fatalf("삭제 계정 want 퇴사자김 got %s", got)
	}
	if got := userDisplayName(users, "", "문성진"); got != "문성진" {
		t.Fatalf("이름만 want 문성진 got %s", got)
	}
	if got := userDisplayName(users, "", ""); got != assigneeUnspecified {
		t.Fatalf("둘 다 없음 want %s got %s", assigneeUnspecified, got)
	}
	if got := userDisplayName(users, missingID, "USR-999"); got != assigneeUnspecified {
		t.Fatalf("USR 저장값 want %s got %s", assigneeUnspecified, got)
	}
}
