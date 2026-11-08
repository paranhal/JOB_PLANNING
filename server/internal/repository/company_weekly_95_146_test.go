package repository

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_146_T1T2_StatBoxesNumbersAndSum(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw146.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active) VALUES
		('C146','해미초','해미초',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES
		('WP146','본사업','active','O03'),('WP146B','다른사업','active','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id)
		VALUES ('MP146',2026,'t','approved','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id,plan_id,visit_date,customer_id,product_type,completed,completed_date,assignee,project_id)
		VALUES ('V146D','MP146','2026-08-12','C146','KLAS',1,'2026-08-12','담당','WP146'),
		       ('V146O','MP146','2026-08-13','C146','KLAS',0,'','담당','WP146')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts(as_id,as_number,customer_id,receipt_datetime,complete_datetime,status,org_id,project_id)
		VALUES ('AS146','R146','C146','2026-08-12 10:00:00','2026-08-12 14:00:00','completed','O03','WP146'),
		       ('AS146C','RC','C146','2026-08-12 10:00:00','','cancelled','O03','WP146')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date,source_type,assignee) VALUES
		('T-WRK','project_work','작업수행','complete','WP146','O03','2026-08-12','2026-08-12','','김작업'),
		('T-ADM','admin','행정','complete','WP146','O03','2026-08-12','2026-08-12','','김행정'),
		('T-SAL','sales','영업건','complete','WP146','O03','2026-08-12','2026-08-12','','김영업'),
		('T-CAN','admin','취소됨','cancelled','WP146','O03','2026-08-12','2026-08-12','',''),
		('T-OTH','admin','다른사업행정','complete','WP146B','O03','2026-08-12','2026-08-12','',''),
		('T-UNL','admin','미연결행정','complete',NULL,'O03','2026-08-12','2026-08-12','',''),
		('T-SA','admin','영업원본','complete',NULL,'O03','2026-08-12','2026-08-12','sales_activity','')`); err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	if _, err := repo.InsertWeeklyReportRow(WeeklyReportRowInput{
		OrgID: "O03", DisplayName: "본사업", Kind: "project", ProjectID: "WP146", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertWeeklyReportRow(WeeklyReportRowInput{
		OrgID: "O03", DisplayName: "사업 미연결 업무", Kind: "activity", ActivityKind: "unlinked", Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	p := CompanyWeeklyPeriod(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	items, qn, err := repo.ListCompanyWeeklyStatItems(p, "O03")
	if err != nil {
		t.Fatal(err)
	}
	if qn < 5 {
		t.Fatalf("쿼리 배치가 너무 적다 %d", qn)
	}
	draft, err := repo.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	boxes := GroupCompanyWeeklyStatBoxes(draft.Rows, items)

	var projKey, unKey string
	for _, row := range draft.Rows {
		if row.ProjectID == "WP146" {
			projKey = row.RowKey
		}
		if row.ActivityKind == "unlinked" {
			unKey = row.RowKey
		}
	}
	if projKey == "" || unKey == "" {
		t.Fatalf("행 키 없음 %+v", draft.Rows)
	}
	proj := boxes[projKey]
	if len(proj) != 5 {
		t.Fatalf("박스 %d", len(proj))
	}
	by := map[string]model.CompanyWeeklyStatBox{}
	for _, b := range proj {
		by[b.Key] = b
	}
	if by["mnt"].Done != 1 || by["mnt"].Total != 2 || by["mnt"].Rate != "50%" {
		t.Fatalf("정기점검 %+v", by["mnt"])
	}
	if by["as"].Done != 1 || by["as"].Total != 1 {
		t.Fatalf("AS 취소 제외 실패 %+v", by["as"])
	}
	if by["work"].Total != 1 || by["work"].Done != 1 || by["work"].Items[0].WorkType != model.WBWorkProjectWork {
		t.Fatalf("작업 방문 %+v", by["work"])
	}
	if by["admin"].Total != 1 || by["sales"].Total != 1 {
		t.Fatalf("행정/영업 %+v %+v", by["admin"], by["sales"])
	}
	sum := 0
	for _, b := range proj {
		sum += b.Total
	}
	if sum != 6 {
		t.Fatalf("카드 합 %d want 6", sum)
	}
	un := boxes[unKey]
	unSum := 0
	var unAdmin model.CompanyWeeklyStatBox
	for _, b := range un {
		unSum += b.Total
		if b.Key == "admin" {
			unAdmin = b
		}
		if b.Total == 0 && b.Rate != "—" {
			t.Fatalf("총 0 진행률 %s %s", b.Key, b.Rate)
		}
	}
	if unAdmin.Total != 1 || unAdmin.Items[0].ID != "T-UNL" {
		t.Fatalf("통칭 행정 %+v", unAdmin)
	}
	if unSum != 1 {
		t.Fatalf("통칭 합 %d (sales_activity·다른사업 제외)", unSum)
	}
	for _, it := range items {
		if it.ID == "T-CAN" || it.ID == "AS146C" || it.ID == "T-SA" {
			t.Fatalf("제외돼야 할 항목 %s", it.ID)
		}
	}
}

func Test95_146_T4_QueryCountStable(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw146q.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	insertN := func(from, n int) {
		for i := from; i < from+n; i++ {
			pid := fmt.Sprintf("WPQ%d", i)
			if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES (?,?, 'active','O03')`, pid, pid); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.InsertWeeklyReportRow(WeeklyReportRowInput{
				OrgID: "O03", DisplayName: pid, Kind: "project", ProjectID: pid, Active: true,
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	insertN(0, 5)
	p := CompanyWeeklyPeriod(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	_, n5, err := repo.ListCompanyWeeklyStatItems(p, "O03")
	if err != nil {
		t.Fatal(err)
	}
	insertN(5, 45)
	_, n50, err := repo.ListCompanyWeeklyStatItems(p, "O03")
	if err != nil {
		t.Fatal(err)
	}
	if n5 != n50 {
		t.Fatalf("쿼리 수 5카드=%d 50카드=%d", n5, n50)
	}
}

func Test95_146_BoxKeysMatchExistingKinds(t *testing.T) {
	if companyWeeklyStatBoxKey(model.ScopeWorkMaintenance, "") != weeklyStatBoxMnt {
		t.Fatal("mnt")
	}
	if companyWeeklyStatBoxKey(model.ScopeWorkAS, "") != weeklyStatBoxAS {
		t.Fatal("as")
	}
	if companyWeeklyStatBoxKey(model.ScopeWorkAdmin, model.WBWorkProjectWork) != weeklyStatBoxWork {
		t.Fatal("work")
	}
	if companyWeeklyStatBoxKey(model.ScopeWorkAdmin, model.WBWorkSales) != weeklyStatBoxSales {
		t.Fatal("sales")
	}
	if companyWeeklyStatBoxKey(model.ScopeWorkAdmin, model.WBWorkAdmin) != weeklyStatBoxAdmin {
		t.Fatal("admin")
	}
	if strings.Contains(companyWeeklyTaskKindLabel(model.WBWorkProjectWork), "작업") == false {
		t.Fatal("기존 종류 라벨")
	}
}
