package repository

import (
	"path/filepath"
	"testing"
)

func TestASCauseCategoriesV2Seed(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cause_v2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_cause_categories`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 55 {
		t.Fatalf("행수=%d want 55", n)
	}
	var serverL2 int
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_cause_categories WHERE parent_code='server' AND COALESCE(is_active,1)=1`).Scan(&serverL2); err != nil {
		t.Fatal(err)
	}
	if serverL2 != 5 {
		t.Fatalf("서버 2차=%d want 5", serverL2)
	}

	var label, cmap string
	var fault int
	if err := db.QueryRow(`SELECT label, cause_type_map, is_fault FROM as_cause_categories WHERE code='server.patch'`).Scan(&label, &cmap, &fault); err != nil {
		t.Fatal(err)
	}
	if label != "패치" || cmap != "sw" || fault != 0 {
		t.Fatalf("패치: label=%q map=%q fault=%d", label, cmap, fault)
	}
	if err := db.QueryRow(`SELECT label FROM as_cause_categories WHERE code='server.hw'`).Scan(&label); err != nil {
		t.Fatal(err)
	}
	if label != "HW 장애" {
		t.Fatalf("server.hw 라벨=%q", label)
	}
	if err := db.QueryRow(`SELECT label FROM as_cause_categories WHERE code='web.post_edit'`).Scan(&label); err != nil {
		t.Fatal(err)
	}
	if label != "게시물 수정" {
		t.Fatalf("web.post_edit 라벨=%q", label)
	}
}
