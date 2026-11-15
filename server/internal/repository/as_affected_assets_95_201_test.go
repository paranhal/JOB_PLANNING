package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func newDB201(t *testing.T) *ASRepo {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "aff201.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO customers(customer_id,org_name,official_name,is_active) VALUES ('C201','성환도서관','성환도서관',1),('C201B','타기관','타기관',1)`,
		`INSERT OR IGNORE INTO product_groups(group_id,class_id,type_name,is_active) VALUES ('PG-RFID','CL','RFID자동화',1),('PG-MAT','CL','자료관리',1)`,
		`INSERT INTO assets(asset_id,customer_id,product_name,product_type,product_category,model_name,product_group_id) VALUES
			('A-MAIN','C201','KLAS 서버','SW','klas','',NULL),
			('A-PC','C201','회원가입PC','HW','materials','','PG-MAT'),
			('A-SRP','C201','EZ-SRP 프린터','HW','rfid','','PG-RFID'),
			('A-NOGRP','C201','그룹 없는 장비','HW','','',NULL),
			('A-OTHER','C201B','타기관 PC','HW','materials','','PG-MAT')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return NewASRepo(db)
}

func insertAS201(t *testing.T, r *ASRepo, id, asset string) {
	t.Helper()
	if _, err := r.db.Exec(`INSERT INTO as_receipts(as_id,as_number,customer_id,asset_id,receipt_datetime,status,symptom,org_id)
		VALUES (?,?,?,?,?, 'in_progress','증상 '||?, 'O01')`, id, id, "C201", nullIfEmpty(asset), "2026-11-10 09:00:00", id); err != nil {
		t.Fatal(err)
	}
}

func TestAffected201_MigrationAndKDeviceSeed(t *testing.T) {
	r := newDB201(t)
	db := r.db
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='as_receipt_affected_assets'`).Scan(&n)
	if n != 1 {
		t.Fatal("영향 장비 테이블이 없다")
	}
	var legacy int
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_cause_categories WHERE code LIKE 'klas.kdevice%'`).Scan(&legacy)
	if legacy != 5 {
		t.Fatalf("KDevice 분류 %d건(기대 5)", legacy)
	}
	rows := map[string]struct {
		level  int
		parent string
		label  string
		fault  int
	}{}
	rr, _ := db.Query(`SELECT code, level, COALESCE(parent_code,''), label, is_fault FROM as_cause_categories WHERE code LIKE 'klas.kdevice%'`)
	for rr.Next() {
		var code string
		var v struct {
			level  int
			parent string
			label  string
			fault  int
		}
		_ = rr.Scan(&code, &v.level, &v.parent, &v.label, &v.fault)
		rows[code] = v
	}
	rr.Close()
	want := map[string][3]interface{}{
		"klas.kdevice":              {2, "KDevice(클라이언트 프로그램)", 1},
		"klas.kdevice.install":      {3, "설치·재설치", 0},
		"klas.kdevice.login_config": {3, "설정·자동 로그인", 1},
		"klas.kdevice.device_link":  {3, "장비 연동 오류", 1},
		"klas.kdevice.etc":          {3, "기타", 1},
	}
	for code, w := range want {
		g, ok := rows[code]
		if !ok || g.level != w[0].(int) || g.label != w[1].(string) || g.fault != w[2].(int) {
			t.Fatalf("%s: %+v (기대 %v)", code, g, w)
		}
	}
	// 정렬: opac(30) 뒤쪽, 기존 분류는 그대로
	var so, opac int
	_ = db.QueryRow(`SELECT sort_order FROM as_cause_categories WHERE code='klas.kdevice'`).Scan(&so)
	_ = db.QueryRow(`SELECT sort_order FROM as_cause_categories WHERE code='klas.opac'`).Scan(&opac)
	if so <= opac {
		t.Fatalf("KDevice 정렬 %d 가 opac %d 뒤가 아니다", so, opac)
	}
	// 멱등: 다시 적용해도 중복이 생기지 않는다
	applyASAffectedAssetsV114(db)
	applyASAffectedAssetsV114(db)
	var again int
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_cause_categories WHERE code LIKE 'klas.kdevice%'`).Scan(&again)
	if again != 5 {
		t.Fatalf("재적용 뒤 %d건", again)
	}
	// 같은 자산을 같은 접수에 두 번 넣을 수 없다
	insertAS201(t, r, "R1", "A-MAIN")
	if _, err := db.Exec(`INSERT INTO as_receipt_affected_assets(as_id,asset_id) VALUES ('R1','A-PC'),('R1','A-PC')`); err == nil {
		t.Fatal("같은 접수에 같은 자산 중복이 들어갔다")
	}
}

func TestAffected201_OrgTableSeed(t *testing.T) {
	r := newDB201(t)
	db := r.db
	// 조직 O03 은 자기 표를 가지고 있고 KLAS 가 있다 / 조직 O04 는 자기 표는 있지만 KLAS 가 없다
	l1 := model.CauseOrgPathCode("O03", "KLAS")
	l2 := model.CauseOrgPathCode("O03", "KLAS", "자료검색대")
	for _, q := range []string{
		`INSERT INTO as_cause_category_orgs(org_id,code,level,parent_code,label,sort_order) VALUES ('O03','` + l1 + `',1,'','KLAS',10)`,
		`INSERT INTO as_cause_category_orgs(org_id,code,level,parent_code,label,sort_order) VALUES ('O03','` + l2 + `',2,'` + l1 + `','자료검색대',30)`,
		`INSERT INTO as_cause_category_orgs(org_id,code,level,parent_code,label,sort_order) VALUES ('O04','` + model.CauseOrgPathCode("O04", "서버") + `',1,'','서버',10)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	applyASAffectedAssetsV114(db)
	applyASAffectedAssetsV114(db) // 멱등
	count := func(org string) int {
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM as_cause_category_orgs WHERE org_id=? AND (label=? OR parent_code=?)`, org, "KDevice(클라이언트 프로그램)",
			model.CauseOrgPathCode(org, "KLAS", "KDevice(클라이언트 프로그램)")).Scan(&n)
		return n
	}
	if n := count("O03"); n != 5 {
		t.Fatalf("O03 KDevice 행 %d(기대 5)", n)
	}
	if n := count("O04"); n != 0 {
		t.Fatalf("KLAS 가 없는 조직 O04 에 %d 행이 들어갔다", n)
	}
	var sortKD int
	_ = db.QueryRow(`SELECT sort_order FROM as_cause_category_orgs WHERE org_id='O03' AND label='KDevice(클라이언트 프로그램)'`).Scan(&sortKD)
	if sortKD != 35 {
		t.Fatalf("O03 정렬 %d(opac 30 + 5)", sortKD)
	}
	// 조직 화면(조치 화면)에서 3단계까지 보인다
	cats, err := r.ListCauseCategoriesForOrg("O03")
	if err != nil {
		t.Fatal(err)
	}
	kd := map[string]int{}
	for _, c := range cats {
		if strings.HasPrefix(c.Label, "KDevice") || c.Label == "설치·재설치" || c.Label == "설정·자동 로그인" || c.Label == "장비 연동 오류" {
			kd[c.Label] = c.Level
		}
	}
	if kd["KDevice(클라이언트 프로그램)"] != 2 || kd["설치·재설치"] != 3 {
		t.Fatalf("조직표 노출 %v", kd)
	}
}

func TestAffected201_DownMigration(t *testing.T) {
	r := newDB201(t)
	b, err := os.ReadFile("../../migrations/114_as_affected_assets.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(string(b)); err != nil {
		t.Fatalf("down 실행: %v", err)
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='as_receipt_affected_assets'`).Scan(&n)
	var kd int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_cause_categories WHERE code LIKE 'klas.kdevice%'`).Scan(&kd)
	if n != 0 || kd != 0 {
		t.Fatalf("down 뒤 테이블 %d·분류 %d", n, kd)
	}
	var base int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_cause_categories WHERE code LIKE 'klas.%'`).Scan(&base)
	if base == 0 {
		t.Fatal("down 이 기존 KLAS 분류까지 지웠다")
	}
	// up 을 다시 적용할 수 있다
	applyASAffectedAssetsV114(r.db)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_cause_categories WHERE code LIKE 'klas.kdevice%'`).Scan(&kd)
	if kd != 5 {
		t.Fatalf("up 재적용 %d", kd)
	}
}

func TestAffected201_CleanAndReplace(t *testing.T) {
	r := newDB201(t)
	insertAS201(t, r, "R1", "A-MAIN")
	// 주 대상 자산·설치자산과 같은 자산은 빠지고 skipped 로 돌아온다
	ids, labels, skipped, err := r.CleanAffectedInput("C201", []string{"A-MAIN", "A-PC", "A-SRP", "A-PC", ""}, []string{"열람실 PC"}, []string{"A-MAIN", "A-SRP"})
	if err != nil || len(ids) != 1 || ids[0] != "A-PC" || len(skipped) != 2 || len(labels) != 1 {
		t.Fatalf("정리 %v %v %v %v", ids, labels, skipped, err)
	}
	// 다른 고객 자산 거절
	if _, _, _, err := r.CleanAffectedInput("C201", []string{"A-OTHER"}, nil, nil); err != ErrAffectedCust {
		t.Fatalf("다른 고객 자산 %v", err)
	}
	if _, _, _, err := r.CleanAffectedInput("C201", []string{"NOPE"}, nil, nil); err != ErrAffectedCust {
		t.Fatalf("없는 자산 %v", err)
	}
	// 21건·항목당 80자 초과
	many := make([]string, 21)
	for i := range many {
		many[i] = "L" + string(rune('a'+i))
	}
	if _, _, _, err := r.CleanAffectedInput("C201", nil, many, nil); err != ErrAffectedLimit {
		t.Fatalf("라벨 21줄 %v", err)
	}
	ids21 := make([]string, 21)
	for i := range ids21 {
		ids21[i] = "A" + string(rune('a'+i))
	}
	if _, _, _, err := r.CleanAffectedInput("C201", ids21, nil, nil); err != ErrAffectedLimit {
		t.Fatalf("자산 21건 %v", err)
	}
	long := strings.Repeat("가", 81)
	if _, _, _, err := r.CleanAffectedInput("C201", nil, []string{long}, nil); err != ErrAffectedLabelLn {
		t.Fatalf("81자 %v", err)
	}
	if _, _, _, err := r.CleanAffectedInput("C201", nil, []string{strings.Repeat("가", 80)}, nil); err != nil {
		t.Fatalf("80자는 통과해야 한다 %v", err)
	}
	// 줄바꿈·쉼표 분리, 빈 항목·중복 제거
	if got := ParseAffectedLabels("열람실 PC, 자료검색PC\n\n열람실 PC\r\n 회원가입PC "); len(got) != 3 || got[0] != "열람실 PC" || got[2] != "회원가입PC" {
		t.Fatalf("라벨 분리 %v", got)
	}
	// 교체: 저장 → 바꿔 저장 → 모두 비우면 행이 지워진다
	if err := r.ReplaceAffectedAssets("R1", "U1", []string{"A-PC", "A-SRP"}, []string{"열람실 PC"}); err != nil {
		t.Fatal(err)
	}
	items, _ := r.ListAffectedAssets("R1")
	if len(items) != 3 || items[2].AssetID != "" || items[2].Name != "열람실 PC" {
		t.Fatalf("저장 %+v", items)
	}
	if got := AffectedLine(items); !strings.HasSuffix(got, "외 2대") {
		t.Fatalf("한 줄 %q", got)
	}
	if err := r.ReplaceAffectedAssets("R1", "U1", []string{"A-SRP"}, nil); err != nil {
		t.Fatal(err)
	}
	items, _ = r.ListAffectedAssets("R1")
	if len(items) != 1 || items[0].AssetID != "A-SRP" || AffectedLine(items) != "EZ-SRP 프린터" {
		t.Fatalf("교체 %+v", items)
	}
	if err := r.ReplaceAffectedAssets("R1", "U1", nil, nil); err != nil {
		t.Fatal(err)
	}
	if items, _ = r.ListAffectedAssets("R1"); len(items) != 0 || AffectedLine(items) != "" {
		t.Fatalf("비우기 %+v", items)
	}
}

func TestAffected201_HistoryBadgeAndStats(t *testing.T) {
	r := newDB201(t)
	insertAS201(t, r, "R-A", "A-MAIN") // 영향: A-PC, A-SRP, 직접 입력 1
	insertAS201(t, r, "R-B", "A-PC")   // 주 대상이 A-PC 인 별개 접수
	insertAS201(t, r, "R-C", "")       // 자산 없는 접수
	_ = r.ReplaceAffectedAssets("R-A", "U", []string{"A-PC", "A-SRP"}, []string{"열람실 PC"})
	_ = r.ReplaceAffectedAssets("R-C", "U", []string{"A-NOGRP"}, nil)

	// 자산 이력: A-PC 이력에 R-B(주 대상, 배지 없음)와 R-A(영향, 배지) 가 한 번씩
	items, err := r.ListHistoryByAsset("A-PC", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, it := range items {
		if _, dup := got[it.ASID]; dup {
			t.Fatalf("%s 가 두 번 나온다", it.ASID)
		}
		got[it.ASID] = it.Affected
	}
	if len(items) != 2 || got["R-A"] != true || got["R-B"] != false {
		t.Fatalf("A-PC 이력 %v", got)
	}
	// excludeASID 유지
	if ex, _ := r.ListHistoryByAsset("A-PC", "R-A", 50); len(ex) != 1 || ex[0].ASID != "R-B" {
		t.Fatalf("exclude %+v", ex)
	}
	// 주 대상 자산 A-MAIN 의 이력에는 R-A 가 배지 없이
	if m, _ := r.ListHistoryByAsset("A-MAIN", "", 50); len(m) != 1 || m[0].Affected {
		t.Fatalf("A-MAIN 이력 %+v", m)
	}
	// 영향 장비가 없는 자산(A-NOGRP 는 R-C 의 영향)과 전혀 없는 자산
	if n, _ := r.ListHistoryByAsset("A-OTHER", "", 50); len(n) != 0 {
		t.Fatal("관계 없는 자산 이력이 늘었다")
	}

	// 건수 불변: 접수·조치 건수는 영향 장비로 늘지 않는다
	var cnt int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&cnt)
	if cnt != 3 {
		t.Fatalf("접수 건수 %d", cnt)
	}
	// 통계: 영향 장비가 있는 접수 2건(R-A, R-C), 영향 장비 4대(A-PC·A-SRP·직접 1·A-NOGRP)
	st, err := r.AffectedStats("", "", "O01")
	if err != nil {
		t.Fatal(err)
	}
	if st.ReceiptN != 2 || st.AssetN != 4 {
		t.Fatalf("통계 %+v", st)
	}
	groups := map[string]int{}
	for _, g := range st.ByGroup {
		groups[g.Group] = g.N
	}
	if groups["자료관리"] != 1 || groups["RFID자동화"] != 1 || groups["직접 입력"] != 1 || groups["(제품군 없음)"] != 1 {
		t.Fatalf("제품군별 %v", groups)
	}
	// 기간·조직 필터
	if st, _ := r.AffectedStats("2026-12-01", "", "O01"); st.ReceiptN != 0 {
		t.Fatalf("기간 밖 %+v", st)
	}
	if st, _ := r.AffectedStats("2026-11-01", "2026-11-30", "O01"); st.ReceiptN != 2 {
		t.Fatalf("기간 안 %+v", st)
	}
	if st, _ := r.AffectedStats("", "", "O09"); st.ReceiptN != 0 {
		t.Fatalf("다른 조직 %+v", st)
	}
}

// AS 가 지워지면(초기화·조직 정리) 영향 장비 행도 함께 지워진다.
func TestAffected201_PrunedWithAS(t *testing.T) {
	r := newDB201(t)
	insertAS201(t, r, "R-KEEP", "A-MAIN")
	insertAS201(t, r, "R-GONE", "A-MAIN")
	_ = r.ReplaceAffectedAssets("R-KEEP", "u", []string{"A-PC"}, []string{"열람실 PC"})
	_ = r.ReplaceAffectedAssets("R-GONE", "u", []string{"A-PC", "A-SRP"}, nil)
	if _, err := r.db.Exec(`DELETE FROM as_receipts WHERE as_id='R-GONE'`); err != nil {
		t.Fatal(err)
	}
	tx, err := r.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := pruneOrphansTx(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var gone, keep int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipt_affected_assets WHERE as_id='R-GONE'`).Scan(&gone)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipt_affected_assets WHERE as_id='R-KEEP'`).Scan(&keep)
	if gone != 0 || keep != 2 {
		t.Fatalf("정리 뒤 지운 접수 %d행·남긴 접수 %d행", gone, keep)
	}
	// 전체 초기화에서도 영향 장비 행이 남지 않는다
	if _, err := ExecuteReset(r.db, ResetScopeAll, ""); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipt_affected_assets`).Scan(&left)
	if left != 0 {
		t.Fatalf("초기화 뒤 영향 장비 %d행이 남았다", left)
	}
}
