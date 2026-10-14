package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

func TestApplyD44ConvertsOnlyReadonlyOrgAdmin(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "d44.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := NewUserRepo(db)
	roAdmin := &model.User{Username: "obs_hj", PasswordHash: "x", FullName: "옵저버일", Role: model.RoleOrgAdmin, IsActive: true, IsReadOnly: true, OrgID: "O01"}
	realAdmin := &model.User{Username: "hjlee", PasswordHash: "x", FullName: "이해진", Role: model.RoleOrgAdmin, IsActive: true, IsReadOnly: false, OrgID: "O01"}
	roTech := &model.User{Username: "techro", PasswordHash: "x", FullName: "기술읽기", Role: model.RoleTech, IsActive: true, IsReadOnly: true, OrgID: "O01"}
	for _, u := range []*model.User{roAdmin, realAdmin, roTech} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if err := ApplyD44ReadonlyOrgAdminToTech(db); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByUsername("obs_hj")
	if got.Role != model.RoleTech || !got.IsReadOnly {
		t.Fatalf("대상 role=%s ro=%v", got.Role, got.IsReadOnly)
	}
	keep, _ := users.GetByUsername("hjlee")
	if keep.Role != model.RoleOrgAdmin || keep.IsReadOnly {
		t.Fatalf("팀장 role=%s ro=%v", keep.Role, keep.IsReadOnly)
	}
	tech, _ := users.GetByUsername("techro")
	if tech.Role != model.RoleTech || !tech.IsReadOnly {
		t.Fatalf("기술읽기 role=%s ro=%v", tech.Role, tech.IsReadOnly)
	}
	logs, err := audit.ListLogs(20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if l.UserName == "시스템" && strings.Contains(l.Summary, "D-44") {
			found = true
			if strings.Contains(strings.ToLower(l.BeforeJSON+l.AfterJSON), "password") {
				t.Fatal("비밀번호가 로그에 있다")
			}
		}
	}
	if !found {
		t.Fatal("시스템 D-44 로그가 없다")
	}
	if err := ApplyD44ReadonlyOrgAdminToTech(db); err != nil {
		t.Fatal(err)
	}
	again, _ := users.GetByUsername("obs_hj")
	if again.Role != model.RoleTech {
		t.Fatal("두 번째 실행이 대상을 바꿨다")
	}
}

func TestApplyD44StopsIfHjleeReadonly(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "d44-stop.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := NewUserRepo(db)
	if err := users.Create(&model.User{Username: "hjlee", PasswordHash: "x", FullName: "이해진", Role: model.RoleOrgAdmin, IsActive: true, IsReadOnly: true, OrgID: "O01"}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyD44ReadonlyOrgAdminToTech(db); err == nil || !strings.Contains(err.Error(), "hjlee") {
		t.Fatalf("중단해야 한다: %v", err)
	}
	got, _ := users.GetByUsername("hjlee")
	if got.Role != model.RoleOrgAdmin {
		t.Fatalf("중단 후 role=%s", got.Role)
	}
}
