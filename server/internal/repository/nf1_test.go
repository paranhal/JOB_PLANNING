package repository

import (
	"database/sql"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestNF1_FreshDBDropsLocCopies(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "nf1-fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if tableHasColumn(db, "assets", "loc_building_name") ||
		tableHasColumn(db, "assets", "loc_floor_name") ||
		tableHasColumn(db, "assets", "loc_room_name") {
		t.Fatal("신규 DB 에 loc_* 복사본이 남아 있다")
	}
	if !tableExists(db, "project_scope_product_keys") ||
		!tableExists(db, "project_scope_work_kinds") ||
		!tableExists(db, "work_task_tags") ||
		!tableExists(db, "sales_activity_members") {
		t.Fatal("연결 테이블이 없다")
	}
	if AppliedSchemaVersion(db) != AppSchemaVersion {
		t.Fatalf("schema=%d want=%d", AppliedSchemaVersion(db), AppSchemaVersion)
	}
}

func TestNF1_BackfillNameOnlyThenDrop(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "nf1-fill.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-nf1','위치도서관','위치도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customer_buildings (building_id, customer_id, building_name)
		VALUES ('b1','c-nf1','본관')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customer_floors (floor_id, building_id, floor_name)
		VALUES ('f1','b1','3층')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customer_rooms (room_id, floor_id, room_name)
		VALUES ('r1','f1','서버실')`); err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{"loc_building_name", "loc_floor_name", "loc_room_name"} {
		if _, err := db.Exec(`ALTER TABLE assets ADD COLUMN ` + col + ` TEXT`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO assets (asset_id, customer_id, product_name, loc_building_name, loc_floor_name, loc_room_name)
		VALUES ('a-nf1','c-nf1','게이트','본관','3층','서버실')`); err != nil {
		t.Fatal(err)
	}
	if n := countAssetLocNameOnly(db); n != 1 {
		t.Fatalf("이름만 있는 행 %d want 1", n)
	}
	applyNF1(db)
	if tableHasColumn(db, "assets", "loc_building_name") {
		t.Fatal("역채우기 뒤 loc_* 를 지워야 한다")
	}
	var bid, fid, rid string
	if err := db.QueryRow(`SELECT COALESCE(building_id,''), COALESCE(floor_id,''), COALESCE(room_id,'')
		FROM assets WHERE asset_id='a-nf1'`).Scan(&bid, &fid, &rid); err != nil {
		t.Fatal(err)
	}
	if bid != "b1" || fid != "f1" || rid != "r1" {
		t.Fatalf("역채우기 building=%s floor=%s room=%s", bid, fid, rid)
	}
	got, err := NewAssetRepo(db).GetByID("a-nf1")
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.BuildingName != "본관" || got.FloorName != "3층" || got.RoomName != "서버실" {
		t.Fatalf("JOIN 이름: %+v", got)
	}
}

func TestNF1_KeepLocIfUnmatched(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "nf1-keep.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-keep','이름만도서관','이름만도서관',1)`); err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{"loc_building_name", "loc_floor_name", "loc_room_name"} {
		if _, err := db.Exec(`ALTER TABLE assets ADD COLUMN ` + col + ` TEXT`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO assets (asset_id, customer_id, product_name, loc_building_name)
		VALUES ('a-keep','c-keep','게이트','없는건물')`); err != nil {
		t.Fatal(err)
	}
	applyNF1(db)
	if !tableHasColumn(db, "assets", "loc_building_name") {
		t.Fatal("매칭 실패 행이 있으면 loc_* 를 지우면 안 된다")
	}
	if n := countAssetLocNameOnly(db); n != 1 {
		t.Fatalf("남은 이름만 행 %d want 1", n)
	}
}

func TestNF1_JoinPrefersLiveBuildingName(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "nf1-join.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-join','조인도서관','조인도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customer_buildings (building_id, customer_id, building_name)
		VALUES ('b-join','c-join','옛이름')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO assets (asset_id, customer_id, product_name, building_id)
		VALUES ('a-join','c-join','게이트','b-join')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE customer_buildings SET building_name='새이름' WHERE building_id='b-join'`); err != nil {
		t.Fatal(err)
	}
	got, err := NewAssetRepo(db).GetByID("a-join")
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.BuildingName != "새이름" || got.LocBuildingName != "새이름" {
		t.Fatalf("복사본이 남으면 옛 이름이 나온다: building=%q loc=%q", got.BuildingName, got.LocBuildingName)
	}
}

func TestNF1_CSVJunctionsExactMatch(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "nf1-csv.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name) VALUES ('WP-F','범위시험')`); err != nil {
		t.Fatal(err)
	}
	repo := NewProjectRepo(db)
	if err := repo.ReplaceRules("WP-F", []model.ProjectScopeRule{{
		ParentCustomerID: "P1",
		ProductKeys:      "klas,sejong_klas,anrobotics",
		WorkKinds:        "as,maintenance",
	}}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM project_scope_product_keys WHERE rule_id IN
		(SELECT rule_id FROM project_scope_rules WHERE project_id='WP-F')`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("product_keys 행 %d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM project_scope_work_kinds WHERE work_kind='as' AND rule_id IN
		(SELECT rule_id FROM project_scope_rules WHERE project_id='WP-F')`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("work_kind as %d err=%v", n, err)
	}
	rules, err := repo.ListRules("WP-F")
	if err != nil || len(rules) != 1 {
		t.Fatalf("rules n=%d err=%v", len(rules), err)
	}
	if !rules[0].HasWorkKind(model.ScopeWorkAS) || !rules[0].HasWorkKind(model.ScopeWorkMaintenance) {
		t.Fatalf("work_kinds overlay: %q", rules[0].WorkKinds)
	}
	if !hasScopeKey(db, rules[0].RuleID, "klas") || !hasScopeKey(db, rules[0].RuleID, "sejong_klas") {
		t.Fatal("klas 와 sejong_klas 가 각각 행이어야 한다")
	}
	if matchProductKeys(model.ProductKeyKLAS, "세종KLAS") {
		t.Fatal("klas 키가 세종KLAS 에 오탐하면 안 된다")
	}
	if !matchProductKeys(model.ProductKeySejongKLAS, "세종KLAS") {
		t.Fatal("sejong_klas 는 세종KLAS 에 맞아야 한다")
	}
}

func TestNF1_TaskTagsSearch(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "nf1-tags.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	wb := NewWBRepo(db)
	t1 := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "자료 정리", DueDate: "2026-09-17",
		WorkDate: "2026-09-17", Tags: "꿀벌도서관, 예산안, 유지보수", Status: model.WBTaskWaiting,
	}
	if err := wb.CreateTask(t1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_tags WHERE task_id=?`, t1.TaskID).Scan(&n); err != nil || n != 3 {
		t.Fatalf("tags 행 %d err=%v", n, err)
	}
	found, err := wb.ListAdminWork("", "꿀벌도서관")
	if err != nil || len(found) != 1 {
		t.Fatalf("태그 검색 n=%d err=%v", len(found), err)
	}
	t1.Tags = "예산안"
	if err := wb.UpdateTask(t1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_tags WHERE task_id=?`, t1.TaskID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("수정 후 tags 행 %d", n)
	}
}

func TestNF1_SalesMembersFindMine(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "nf1-mem.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	sales := NewSalesRepo(db)
	p := &model.SalesProject{Name: "멤버시험"}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	act := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: "2026-09-17", StartTime: "10:00",
		DurationMin: 30, ActivityType: "visit", Title: "현장 미팅",
		OurMembers: "최혜영, 양기헌",
	}
	if err := sales.CreateActivity(act, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sales_activity_members WHERE activity_id=?`, act.ActivityID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("members 행 %d err=%v", n, err)
	}
	mine, err := sales.ListActivitiesFilter(SalesActivityFilter{Member: "양기헌"})
	if err != nil || len(mine) != 1 {
		t.Fatalf("내가 참여한 활동 n=%d err=%v", len(mine), err)
	}
	ids := sales.activitySearchLike("양기헌")
	if len(ids) != 1 || ids[0] != act.ActivityID {
		t.Fatalf("검색 ids=%v", ids)
	}
	nobody, err := sales.ListActivitiesFilter(SalesActivityFilter{Member: "김철수"})
	if err != nil || len(nobody) != 0 {
		t.Fatalf("다른 사람 n=%d err=%v", len(nobody), err)
	}
}

func TestNF1_AttachRefUnify(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "nf1-att.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if model.CanonicalAttachRef(model.AttachFormReceipt) != model.AttachRefAS {
		t.Fatalf("CanonicalAttachRef(%q)=%q", model.AttachFormReceipt, model.CanonicalAttachRef(model.AttachFormReceipt))
	}
	if _, err := db.Exec(`
		INSERT INTO attachments (attachment_id, ref_type, ref_id, file_name, file_path)
		VALUES ('att-old', ?, 'as-1', '증상.jpg', 'data/uploads/as/as-1/receipt/증상.jpg')`,
		model.AttachFormReceipt); err != nil {
		t.Fatal(err)
	}
	applyAttachRefUnify(db)
	var rt string
	if err := db.QueryRow(`SELECT ref_type FROM attachments WHERE attachment_id='att-old'`).Scan(&rt); err != nil {
		t.Fatal(err)
	}
	if rt != model.AttachRefAS {
		t.Fatalf("ref_type=%q want as", rt)
	}
	photos, err := NewAttachmentRepo(db).ListReceiptPhotos("as-1")
	if err != nil || len(photos) != 1 {
		t.Fatalf("접수 사진 n=%d err=%v", len(photos), err)
	}
}

func tableExists(db *sql.DB, name string) bool {
	if db == nil {
		return false
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return n > 0
}
