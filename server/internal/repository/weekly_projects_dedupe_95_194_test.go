package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

// 95-194: 같은 조직·같은 이름(공백·괄호 무시)의 사업은 대표 1개만. 실적 → 영업 연결 → 생성일 → ID.
func TestDedupeWeeklyProjects95_194(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "d194.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO work_projects(project_id,name,status,org_id,created_at) VALUES
			('T-A','가 사업(2026)','active','O01','2026-09-01'),('T-B','가 사업 2026','active','O01','2026-08-01'),
			('T-C','나 사업','active','O01','2026-08-01'),('T-D','나 사업','active','O01','2026-08-01'),
			('T-E','다 사업','active','O01','2026-09-01'),('T-F','다 사업','active','O01','2026-10-01'),
			('T-G','가 사업(2026)','active','O02','2026-09-01')`,
		`UPDATE work_projects SET sales_project_id='SP-X' WHERE project_id='T-F'`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id) VALUES ('T1','admin','a','complete','T-A','O01')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	r := NewStatsRepo(db)
	in := []model.WorkProject{
		{ProjectID: "T-A", Name: "가 사업(2026)", OrgID: "O01"}, {ProjectID: "T-B", Name: "가 사업 2026", OrgID: "O01"},
		{ProjectID: "T-C", Name: "나 사업", OrgID: "O01"}, {ProjectID: "T-D", Name: "나 사업", OrgID: "O01"},
		{ProjectID: "T-E", Name: "다 사업", OrgID: "O01"}, {ProjectID: "T-F", Name: "다 사업", OrgID: "O01"},
		{ProjectID: "T-G", Name: "가 사업(2026)", OrgID: "O02"},
	}
	got := map[string]bool{}
	for _, p := range r.dedupeWeeklyProjects(in) {
		got[p.ProjectID] = true
	}
	// 가: 실적이 있는 T-A (생성일은 T-B 가 빠르지만 실적이 우선) / 나: 실적·영업 같고 생성일 같음 → ID 순 T-C
	// 다: 영업 연결이 있는 T-F (생성일은 T-E 가 빠르지만 영업 연결이 우선) / 다른 조직 T-G 는 그대로
	for id, want := range map[string]bool{"T-A": true, "T-B": false, "T-C": true, "T-D": false, "T-E": false, "T-F": true, "T-G": true} {
		if got[id] != want {
			t.Fatalf("%s 표시=%v want %v (%v)", id, got[id], want, got)
		}
	}
	if len(r.dedupeWeeklyProjects(in[:1])) != 1 {
		t.Fatal("1개 목록은 그대로")
	}
}
