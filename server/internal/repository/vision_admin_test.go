package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/passwd"
)

func TestApplyVisionAdminPasswordInitOnceThenIgnore(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "vision.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewSettingsRepo(db)
	note, err := ApplyVisionAdminPassword(r, "init-secret1", "", passwd.Hash)
	if err != nil || note == "" {
		t.Fatalf("초기 저장 note=%q err=%v", note, err)
	}
	first, _ := r.Get(SettingVisionAdminPassword)
	if first == "" || first == "init-secret1" {
		t.Fatal("평문이 저장되면 안 된다")
	}
	note, err = ApplyVisionAdminPassword(r, "other-secret1", "", passwd.Hash)
	if err != nil || note != "" {
		t.Fatalf("두 번째는 무시해야 한다 note=%q err=%v", note, err)
	}
	second, _ := r.Get(SettingVisionAdminPassword)
	if second != first {
		t.Fatal("기존 해시를 덮었다")
	}
	if !passwd.Verify(first, "init-secret1") {
		t.Fatal("초기 비밀번호 검증 실패")
	}
}

func TestApplyVisionAdminPasswordResetForcesChange(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "vision-reset.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewSettingsRepo(db)
	_, _ = ApplyVisionAdminPassword(r, "init-secret1", "", passwd.Hash)
	note, err := ApplyVisionAdminPassword(r, "", "reset-secret", passwd.Hash)
	if err != nil || !strings.Contains(note, "초기화") {
		t.Fatalf("reset note=%q err=%v", note, err)
	}
	h, _ := r.Get(SettingVisionAdminPassword)
	if !passwd.Verify(h, "reset-secret") {
		t.Fatal("초기화 비밀번호가 안 들어갔다")
	}
	if !r.VisionAdminMustChange() {
		t.Fatal("새 비밀번호를 정하라는 표시가 없다")
	}
}

func TestApplyVisionAdminPasswordRejectsShort(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "vision-short.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewSettingsRepo(db)
	if _, err := ApplyVisionAdminPassword(r, "short", "", passwd.Hash); err == nil {
		t.Fatal("짧은 초기 비밀번호를 받았다")
	}
}
