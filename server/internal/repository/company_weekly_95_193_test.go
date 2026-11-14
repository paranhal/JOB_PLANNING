package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func seed193(t *testing.T) (*StatsRepo, string) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "cw193.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	exec := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES ('O03','O03','SI사업팀','SI',1,3)`)
	exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active) VALUES ('C193','제품학교','제품학교',1)`)
	exec(`INSERT INTO assets(asset_id,customer_id,product_name,product_type,product_category,model_name) VALUES
		('A193-1','C193','전자도서관','SW','rfid','LX-1'),
		('A193-2','C193','무인대출반납기','HW','rfid','')`)
	exec(`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id) VALUES ('MP193',2026,'t','approved','O03')`)
	exec(`INSERT INTO as_receipts(as_id,as_number,customer_id,receipt_datetime,complete_datetime,status,org_id,asset_id) VALUES
		('AS193-A','R193A','C193','2026-08-12 10:00:00','2026-08-12 14:00:00','completed','O03','A193-1'),
		('AS193-N','R193N','C193','2026-08-12 10:00:00','2026-08-12 15:00:00','completed','O03',NULL)`)
	exec(`INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,product_type,completed,completed_date,assignee) VALUES
		('V193-K','MP193','2026-08-12','C193','KLAS',1,'2026-08-12','담당'),
		('V193-T','MP193','2026-08-11','C193','',1,'2026-08-11','담당'),
		('V193-N','MP193','2026-08-13','C193','',1,'2026-08-13','담당')`)
	exec(`INSERT INTO maintenance_visit_targets(target_id,visit_id,asset_id,name,sort_order,org_id) VALUES ('T1','V193-T','A193-2','',0,'O03')`)
	exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date,source_type) VALUES
		('T193-M','admin','자산 여러 개','complete',NULL,'O03','2026-08-12','2026-08-12',''),
		('T193-N','admin','자산 없음','complete',NULL,'O03','2026-08-12','2026-08-12','')`)
	exec(`INSERT INTO work_task_assets(task_id,asset_id) VALUES ('T193-M','A193-1'),('T193-M','A193-2')`)
	return NewStatsRepo(db), "O03"
}

// 95-193: 사업 미연결 업무에 제품 라벨 — 정기점검 제품구분, AS 연결 자산, 점검 대상, 행정/지원 연결 자산(여러 개는 「첫 개 외 n」). 모르면 비운다.
func TestCompanyWeekly95_193_UnassignedProductLabel(t *testing.T) {
	repo, org := seed193(t)
	d, err := repo.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), org)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]model.CompanyWeeklyUnassigned{}
	for _, u := range d.UnassignedNoProject {
		by[u.ID] = u
	}
	want := map[string]string{
		"V193-K":  "KLAS",
		"AS193-A": "전자도서관 · LX-1",
		"V193-T":  "무인대출반납기",
		"T193-M":  "전자도서관 · LX-1", // + 「 외 1」
		"AS193-N": "", "V193-N": "", "T193-N": "",
	}
	for id, w := range want {
		u, ok := by[id]
		if !ok {
			t.Fatalf("미연결 목록에 %s 없음 (%d건)", id, len(d.UnassignedNoProject))
		}
		switch id {
		case "AS193-A", "T193-M", "V193-T":
			if len(u.ProductLabel) < len(w) || u.ProductLabel[:len(w)] != w {
				t.Fatalf("%s 라벨 %q want %q 로 시작", id, u.ProductLabel, w)
			}
		default:
			if u.ProductLabel != w {
				t.Fatalf("%s 라벨 %q want %q", id, u.ProductLabel, w)
			}
		}
	}
	if l := by["T193-M"].ProductLabel; l[len(l)-len(" 외 1"):] != " 외 1" {
		t.Fatalf("여러 자산은 「첫 개 외 n」: %q", l)
	}
	// 연결·분류는 그대로: 라벨과 무관하게 사유는 사업 없음
	for _, u := range d.UnassignedNoProject {
		if u.Reason != model.UnassignedReasonNoProject {
			t.Fatalf("%s 사유 %q", u.ID, u.Reason)
		}
	}
}

func TestAssetProductLabelFormats(t *testing.T) {
	cases := []struct{ n, m, c, want string }{
		{"전자도서관", "LX-1", "", "전자도서관 · LX-1"},
		{"전자도서관", "", "", "전자도서관"},
		{"", "LX-1", "", "LX-1"},
		{"", "", "rfid", ""},
	}
	for _, c := range cases {
		if got := assetProductLabel(c.n, c.m, c.c); got != c.want {
			t.Fatalf("%+v → %q", c, got)
		}
	}
	if got := joinProductLabels([]string{"A", "A", "B", "C"}); got != "A 외 2" {
		t.Fatalf("join %q", got)
	}
	if joinProductLabels(nil) != "" {
		t.Fatal("빈 목록")
	}
}

// 400건을 넘는 ID 도 쿼리 길이 제한 없이 청크로 나눠 조회한다(N+1 이 아니라 종류별 청크 수만큼만 쿼리).
func TestCompanyWeekly95_193_ManyIDsChunked(t *testing.T) {
	repo, _ := seed193(t)
	var items []model.CompanyWeeklyUnassigned
	for i := 0; i < 1000; i++ {
		items = append(items, model.CompanyWeeklyUnassigned{Kind: model.ScopeWorkAS, ID: "none-" + time.Now().Format("150405") + string(rune('a'+i%26)) + string(rune('A'+i/26%26))})
	}
	items = append(items, model.CompanyWeeklyUnassigned{Kind: model.ScopeWorkAS, ID: "AS193-A"})
	repo.fillUnassignedProductLabels(items)
	if !strings.HasPrefix(items[len(items)-1].ProductLabel, "전자도서관 · LX-1") {
		t.Fatalf("청크 조회 실패 %q", items[len(items)-1].ProductLabel)
	}
}
