package repository

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_63_SeedIdempotentAndLabels(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "spc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	applyProductCategory95_63(db)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group=? AND code_id BETWEEN 'SPC007' AND 'SPC012'`, ProductCategoryGroup).Scan(&n); err != nil || n != 6 {
		t.Fatalf("시드 %d %v", n, err)
	}
	want := map[string]string{
		"SPC007": "방화벽", "SPC008": "보안L2스위치", "SPC009": "L2스위치",
		"SPC010": "집선스위치", "SPC011": "PoE스위치", "SPC012": "indoor",
	}
	for id, name := range want {
		var got string
		if err := db.QueryRow(`SELECT code_name FROM codes WHERE code_id=?`, id).Scan(&got); err != nil || got != name {
			t.Fatalf("%s %s %v", id, got, err)
		}
	}
	if ProductCategoryLabel("firewall") != "방화벽" || ProductCategoryLabel("") != "—" {
		t.Fatalf("label %s", ProductCategoryLabel("firewall"))
	}
	if ResolveProductCategoryValue("방화벽") != "firewall" || ResolveProductCategoryValue("PoE스위치") != "poe_switch" {
		t.Fatalf("resolve %s %s", ResolveProductCategoryValue("방화벽"), ResolveProductCategoryValue("PoE스위치"))
	}
	if MapStatsProductCategory("firewall", "", "") != "방화벽" {
		t.Fatalf("stats %s", MapStatsProductCategory("firewall", "", ""))
	}
	if mapImportProductCategory("indoor") != "indoor" {
		t.Fatalf("import indoor %s", mapImportProductCategory("indoor"))
	}
}

func Test95_63_SeqNoReuseDupNameKeyLockedInUse(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "spc2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewCodeRepo(db)
	a := &model.Code{CodeGroup: ProductCategoryGroup, CodeValue: "cam", CodeName: "카메라", IsActive: true}
	if err := r.Create(a); err != nil {
		t.Fatal(err)
	}
	if a.CodeID != "SPC013" {
		t.Fatalf("다음 번호 %s", a.CodeID)
	}
	if err := r.Delete(a.CodeID); err != nil {
		t.Fatal(err)
	}
	b := &model.Code{CodeGroup: ProductCategoryGroup, CodeValue: "cam2", CodeName: "카메라2", IsActive: true}
	if err := r.Create(b); err != nil {
		t.Fatal(err)
	}
	if b.CodeID != "SPC014" {
		t.Fatalf("재사용 %s", b.CodeID)
	}
	dup := &model.Code{CodeGroup: ProductCategoryGroup, CodeValue: "cam3", CodeName: "카메라2", IsActive: true}
	if err := r.Create(dup); !errors.Is(err, ErrProductCategoryDupName) {
		t.Fatalf("동명 %v", err)
	}
	b.CodeValue = "changed"
	if err := r.Update(b); !errors.Is(err, ErrProductCategoryKeyLocked) {
		t.Fatalf("키잠금 %v", err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, product_category) VALUES ('A1','C1','스위치','cam2')`); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(b.CodeID); !errors.Is(err, ErrProductCategoryInUse) {
		t.Fatalf("사용중 삭제 %v", err)
	}
	if err := r.DeactivateProductCategory(b.CodeID); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(b.CodeID)
	if err != nil || got.IsActive {
		t.Fatalf("사용중지 %+v %v", got, err)
	}
}

func Test95_63_NoHardcodedCategoryMaps(t *testing.T) {
	here := filepath.Dir(mustFile(t))
	files := []string{
		filepath.Join(here, "..", "handler", "renderer.go"),
		filepath.Join(here, "..", "handler", "asset_excel.go"),
		filepath.Join(here, "asset_import.go"),
		filepath.Join(here, "stats_repo.go"),
	}
	needle := `"homepage": "홈페이지"`
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), needle) {
			t.Fatalf("하드코딩 남음 %s", f)
		}
	}
}

func mustFile(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("caller")
	}
	return file
}
