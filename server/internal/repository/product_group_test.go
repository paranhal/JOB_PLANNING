package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func Test95_64_SeedIdempotentKPIAnd전선스위치(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "pg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	applyProductGroups95_64(db)
	applyProductGroups95_64(db)

	r := NewProductGroupRepo(db)
	items, err := r.List("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 16 {
		t.Fatalf("시드 %d", len(items))
	}
	kpi := r.KPI(items)
	if kpi.Total != 16 || kpi.Classes != 3 || kpi.Types != 12 || kpi.NoDetail != 11 {
		t.Fatalf("KPI %+v", kpi)
	}
	g, err := r.Get("PG-0003")
	if err != nil || g == nil || g.DetailName != "전선스위치" {
		t.Fatalf("PG-0003 %+v %v", g, err)
	}
	if g.TypeName != "네트워크장비" || g.ClassName != "HW" {
		t.Fatalf("PG-0003 분류 %+v", g)
	}
}

func Test95_64_SeqNoReuseInUseDeactivate(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "pg2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewProductGroupRepo(db)
	g := &model.ProductGroup{ClassID: "hw", TypeName: "테스트구분", DetailName: "세부A", IsActive: true}
	if err := r.Create(g); err != nil {
		t.Fatal(err)
	}
	if g.GroupID != "PG-0017" {
		t.Fatalf("다음번호 %s", g.GroupID)
	}
	if err := r.Delete(g.GroupID); err != nil {
		t.Fatal(err)
	}
	g2 := &model.ProductGroup{ClassID: "hw", TypeName: "테스트구분", DetailName: "세부B", IsActive: true}
	if err := r.Create(g2); err != nil {
		t.Fatal(err)
	}
	if g2.GroupID != "PG-0018" {
		t.Fatalf("재사용 %s", g2.GroupID)
	}

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, product_group_id) VALUES ('A1','C1','스위치','PG-0001')`); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete("PG-0001"); err != ErrProductGroupInUse {
		t.Fatalf("사용중 삭제 %v", err)
	}
	if err := r.Deactivate("PG-0001"); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get("PG-0001")
	if got == nil || got.IsActive {
		t.Fatal("사용중지 안 됨")
	}
}

func Test95_64_BackfillCategoryTypeNotName(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "pg3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`DELETE FROM id_sequences WHERE seq_key=?`, assetProductGroupMetaKey); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, product_type, product_category) VALUES
		('A-fw','C1','방화벽','NETWORK',''),
		('A-cat','C1','아무이름','HW','firewall'),
		('A-per','C1','주변','PERIPHERAL',''),
		('A-net','C1','스위치','NETWORK','')`); err != nil {
		t.Fatal(err)
	}
	backfillAssetProductGroups(db)

	var fw, cat, per, net string
	_ = db.QueryRow(`SELECT COALESCE(product_group_id,'') FROM assets WHERE asset_id='A-fw'`).Scan(&fw)
	_ = db.QueryRow(`SELECT COALESCE(product_group_id,'') FROM assets WHERE asset_id='A-cat'`).Scan(&cat)
	_ = db.QueryRow(`SELECT COALESCE(product_group_id,'') FROM assets WHERE asset_id='A-per'`).Scan(&per)
	_ = db.QueryRow(`SELECT COALESCE(product_group_id,'') FROM assets WHERE asset_id='A-net'`).Scan(&net)
	if fw != "" {
		t.Fatalf("이름 추론 금지 got %s", fw)
	}
	if cat != "PG-0014" {
		t.Fatalf("category 매핑 %s", cat)
	}
	if per != "PG-0006" {
		t.Fatalf("type 매핑 %s", per)
	}
	if net != "" {
		t.Fatalf("NETWORK 단독은 세부 여러 개라 비움 %s", net)
	}
}

func Test95_64_ApplyToAssetSyncsTypeCategory(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "pg4.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewProductGroupRepo(db)
	a := &model.Asset{ProductGroupID: "PG-0003", ProductType: "SW", ProductCategory: "homepage"}
	r.ApplyToAsset(a)
	if a.ProductType != "NETWORK" || a.ProductCategory != "aggregation_switch" {
		t.Fatalf("동기화 %+v", a)
	}
	g, _ := r.Get("PG-0001")
	a2 := &model.Asset{}
	SyncAssetLegacyFromGroup(a2, g)
	if a2.ProductType != "NETWORK" || a2.ProductCategory != "l2_switch" || a2.ProductGroupID != "PG-0001" {
		t.Fatalf("매핑 함수 %+v", a2)
	}
}

func Test95_64_FillUnregisteredThenNewIDTypeClassAndRename(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "pg5.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewProductGroupRepo(db)
	first := &model.ProductGroup{ClassID: "hw", TypeName: "주변장비", DetailName: "USB", IsActive: true}
	if err := r.Create(first); err != nil {
		t.Fatal(err)
	}
	if first.GroupID != "PG-0006" {
		t.Fatalf("미등록 채움 %s", first.GroupID)
	}
	got, _ := r.Get("PG-0006")
	if got.DetailName != "USB" {
		t.Fatalf("채움 내용 %+v", got)
	}
	second := &model.ProductGroup{ClassID: "hw", TypeName: "주변장비", DetailName: "허브", IsActive: true}
	if err := r.Create(second); err != nil {
		t.Fatal(err)
	}
	if second.GroupID != "PG-0017" {
		t.Fatalf("이후 새 번호 %s", second.GroupID)
	}
	bad := &model.ProductGroup{ClassID: "sw", TypeName: "네트워크장비", DetailName: "X", IsActive: true}
	if err := r.Create(bad); err != ErrProductTypeClassLocked {
		t.Fatalf("분류 잠금 %v", err)
	}
	u, _ := r.Get("PG-0007")
	u.TypeName = "키오스크장비"
	if err := r.Update(u); err != nil {
		t.Fatal(err)
	}
	again, _ := r.Get("PG-0007")
	if again.TypeName != "키오스크장비" {
		t.Fatalf("이름 반영 %+v", again)
	}
}

func Test95_64_BulkDeleteSkipsInUse(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "pg6.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, product_group_id) VALUES ('A1','C1','스위치','PG-0001')`); err != nil {
		t.Fatal(err)
	}
	r := NewProductGroupRepo(db)
	extra := &model.ProductGroup{ClassID: "hw", TypeName: "임시장비", DetailName: "", IsActive: true}
	if err := r.Create(extra); err != nil {
		t.Fatal(err)
	}
	del, skip, err := r.BulkDelete([]string{"PG-0001", extra.GroupID})
	if err != nil {
		t.Fatal(err)
	}
	if del != 1 || skip != 1 {
		t.Fatalf("일괄 del=%d skip=%d", del, skip)
	}
}

func Test95_64_AssetImportHeaderProductGroup(t *testing.T) {
	if normalizeAssetHeader("제품군") != "제품군" || normalizeAssetHeader("제품분류") != "제품군" {
		t.Fatal("헤더 정규화")
	}
	found := false
	for _, h := range assetImportDataHeaders {
		if h == "제품군" {
			found = true
		}
		if h == "제품분류" {
			t.Fatal("양식에 제품분류가 남아 있다")
		}
	}
	if !found {
		t.Fatal("양식에 제품군 열이 없다")
	}
}
