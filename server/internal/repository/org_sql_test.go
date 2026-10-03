package repository

import "testing"

func TestAppendOrgSQL(t *testing.T) {
	if _, _, err := AppendOrgSQL("ar", ""); err == nil {
		t.Fatal("빈 org_id 는 오류여야 한다")
	}
	frag, args, err := AppendOrgSQL("ar", "O01")
	if err != nil || frag != ` AND ar.org_id = ?` || len(args) != 1 || args[0] != "O01" {
		t.Fatalf("O01 frag=%q args=%v err=%v", frag, args, err)
	}
	frag, args, err = AppendOrgSQL("ar", OrgAll)
	if err != nil || len(args) != 0 {
		t.Fatalf("OrgAll frag=%q args=%v err=%v", frag, args, err)
	}
	if VisibleInOrg("O02", "O01") || !VisibleInOrg("O01", "O01") {
		t.Fatal("VisibleInOrg")
	}
	if !VisibleInOrg("O02", OrgAll) {
		t.Fatal("OrgAll visibility")
	}
	if _, err := RequireInsertOrg(""); err == nil {
		t.Fatal("insert empty")
	}
	if _, err := RequireInsertOrg(OrgAll); err == nil {
		t.Fatal("insert OrgAll")
	}
}
