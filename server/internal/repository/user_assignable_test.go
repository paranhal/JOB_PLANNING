package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestListAssignableOrgScopedAndHomonym(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "assignable.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := NewUserRepo(db)
	mustUser := func(u *model.User) {
		t.Helper()
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	mustUser(&model.User{Username: "o01a", PasswordHash: "x", FullName: "김철수", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary})
	mustUser(&model.User{Username: "o02a", PasswordHash: "x", FullName: "김철수", Role: model.RoleTech, IsActive: true, OrgID: "O02"})
	mustUser(&model.User{Username: "o02b", PasswordHash: "x", FullName: "미디어담당B", Role: model.RoleTech, IsActive: true, OrgID: "O02"})
	mustUser(&model.User{Username: "o03a", PasswordHash: "x", FullName: "SI담당C", Role: model.RoleTech, IsActive: true, OrgID: "O03"})
	if _, err := db.Exec(`INSERT INTO users (user_id,username,password_hash,full_name,role,is_active,org_id,created_at)
		VALUES ('USR_OBS','obs1','x','옵저버계정','observer',1,'O02','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	mustUser(&model.User{Username: "ceo1", PasswordHash: "x", FullName: "경영진", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, OrgID: ""})
	mustUser(&model.User{Username: "vs1", PasswordHash: "x", FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, OrgID: ""})
	mustUser(&model.User{Username: "off1", PasswordHash: "x", FullName: "비활성", Role: model.RoleTech, IsActive: false, OrgID: "O02"})

	if _, err := users.ListAssignable(""); err == nil {
		t.Fatal("empty orgID must fail")
	}
	got, err := users.ListAssignable("O02")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]int{}
	for _, u := range got {
		names[u.FullName]++
		if u.OrgID != "O02" {
			t.Fatalf("O02 list leaked %s org=%s", u.FullName, u.OrgID)
		}
		switch u.Role {
		case model.RoleObserver, model.RoleCLevel, model.RoleVisionAdmin, model.RoleTester:
			t.Fatalf("excluded role leaked %s", u.Role)
		}
	}
	if names["미디어담당B"] != 1 || names["김철수"] != 1 {
		t.Fatalf("O02 names %+v", names)
	}
	if names["SI담당C"] != 0 || names["비활성"] != 0 || names["경영진"] != 0 || names["비젼"] != 0 {
		t.Fatalf("O02 extra %+v", names)
	}

	u := users.FindAssignable("O02", "김철수")
	if u == nil || u.OrgID != "O02" || u.Username != "o02a" {
		t.Fatalf("homonym %+v", u)
	}
	if users.FindAssignable("O01", "미디어담당B") != nil {
		t.Fatal("O01 must not find O02 user")
	}
}
