package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestReportSignatureSettingsAndUserBox(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sig.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if !tableHasColumn(db, "users", "signature_box") {
		t.Fatal("users.signature_box 없음")
	}
	if !tableHasColumn(db, "users", "signature_path2") {
		t.Fatal("users.signature_path2 없음")
	}
	if !metaDone(db, reportSignatureV62MetaKey) {
		t.Fatal("metaDone 이 안 섰다")
	}
	st := NewSettingsRepo(db)
	box := st.ReportSignatureBox()
	if box.Size != 4600 || box.Gap != 900 || box.NY != 0 {
		t.Fatalf("확정값이 아니다 %+v", box)
	}
	if err := st.Set(model.SettingReportSignatureNudgeY, "-900"); err != nil {
		t.Fatal(err)
	}
	got := st.ReportSignatureBox()
	if got.NY != -900 {
		t.Fatalf("음수 보정을 막았다 %+v", got)
	}

	users := NewUserRepo(db)
	u := &model.User{Username: "siguser", PasswordHash: "x", FullName: "사인", Role: model.RoleTech, IsActive: true}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	gotEmpty, err := users.GetByID(u.UserID)
	if err != nil || gotEmpty == nil {
		t.Fatal(err)
	}
	if gotEmpty.SignatureBox != "" {
		t.Fatalf("기본은 빈 칸: %q", gotEmpty.SignatureBox)
	}
	merged := model.ParseReportSignatureBox(gotEmpty.SignatureBox, box)
	if merged.Size != box.Size {
		t.Fatal("빈 칸은 전사 기본값")
	}
	if err := users.UpdateSignatureBox(u.UserID, `{"size":5000,"gap":400,"ny":-200}`); err != nil {
		t.Fatal(err)
	}
	gotU, err := users.GetByID(u.UserID)
	if err != nil || gotU == nil {
		t.Fatal(err)
	}
	own := model.ParseReportSignatureBox(gotU.SignatureBox, box)
	if own.Size != 5000 || own.Gap != 400 || own.NY != -200 {
		t.Fatalf("%+v", own)
	}
}
