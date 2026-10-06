package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestMigrateRoleFlagsV57RoundTrip(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "v57.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	_, _ = db.Exec(`DELETE FROM id_sequences WHERE seq_key=?`, roleFlagsV57MetaKey)
	_, _ = db.Exec(`UPDATE users SET v57_role=NULL, v57_base=NULL, v57_perms=NULL`)

	_, err = db.Exec(`INSERT INTO users (user_id,username,password_hash,full_name,role,permissions,is_active,org_id,base_role,is_test)
		VALUES ('U1','obs1','x','옵1','observer','as.view',1,'O01','',0),
		       ('U2','obs2','x','옵2','observer','',1,'O01','tech',0),
		       ('U3','tst1','x','테1','tester','',1,'O01','sales',0),
		       ('U4','tech1','x','기1','tech','',1,'O01','',0)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateRoleFlagsV57(db); err != nil {
		t.Fatal(err)
	}
	var role, base, perms string
	var ro, test int
	_ = db.QueryRow(`SELECT role, COALESCE(base_role,''), COALESCE(permissions,''), is_readonly, is_test FROM users WHERE user_id='U1'`).Scan(&role, &base, &perms, &ro, &test)
	if role != model.RoleOrgAdmin || ro != 1 || perms != "" {
		t.Fatalf("빈 base 옵저버: role=%s ro=%d perms=%s", role, ro, perms)
	}
	_ = db.QueryRow(`SELECT role, is_readonly FROM users WHERE user_id='U2'`).Scan(&role, &ro)
	if role != model.RoleTech || ro != 1 {
		t.Fatalf("tech 옵저버: %s %d", role, ro)
	}
	_ = db.QueryRow(`SELECT role, is_test FROM users WHERE user_id='U3'`).Scan(&role, &test)
	if role != model.RoleSales || test != 1 {
		t.Fatalf("테스터: %s %d", role, test)
	}
	_ = db.QueryRow(`SELECT role FROM users WHERE user_id='U4'`).Scan(&role)
	if role != model.RoleTech {
		t.Fatalf("업무 손댐 %s", role)
	}

	_, err = db.Exec(`UPDATE users SET role=v57_role, base_role=v57_base, permissions=v57_perms,
		is_readonly=0, is_test=CASE WHEN v57_role='tester' THEN 1 ELSE 0 END
		WHERE v57_role IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT role, COALESCE(base_role,''), COALESCE(permissions,'') FROM users WHERE user_id='U1'`).Scan(&role, &base, &perms)
	if role != "observer" || perms != "as.view" {
		t.Fatalf("되돌리기 U1 %s %s %s", role, base, perms)
	}
	_ = db.QueryRow(`SELECT role, COALESCE(base_role,'') FROM users WHERE user_id='U2'`).Scan(&role, &base)
	if role != "observer" || base != "tech" {
		t.Fatalf("되돌리기 U2 %s %s", role, base)
	}
}
