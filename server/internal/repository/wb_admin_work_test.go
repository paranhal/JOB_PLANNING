package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestAdminWorkCustomerOnWeeklyAndList(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "admin_cust.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-chungnam','충남교육청','충청남도교육청',1)`); err != nil {
		t.Fatal(err)
	}

	wb := NewWBRepo(db)
	title := "충남교육청통합도서관 27년 유지보수 계약을 위한 과업심의 안건 자료 제출"
	free := &model.WorkTask{
		WorkType:     model.WBWorkAdmin,
		Title:        title,
		Description:  "과업심의 안건 자료",
		DueDate:      "2026-08-14",
		WorkDate:     "2026-08-14",
		CustomerName: "충남교육청",
		Status:       model.WBTaskWaiting,
	}
	if err := wb.CreateTask(free); err != nil {
		t.Fatal(err)
	}

	linked := &model.WorkTask{
		WorkType:   model.WBWorkAdmin,
		Title:      "계약 관련 회신",
		DueDate:    "2026-08-14",
		WorkDate:   "2026-08-14",
		CustomerID: "c-chungnam",
		Status:     model.WBTaskWaiting,
	}
	if err := wb.CreateTask(linked); err != nil {
		t.Fatal(err)
	}

	asTask := &model.WorkTask{
		WorkType:   model.WBWorkAS,
		Title:      "[AS]테스트",
		DueDate:    "2026-08-14",
		WorkDate:   "2026-08-14",
		SourceType: model.WBSourceAS,
		SourceID:   "as-1",
	}
	if err := wb.CreateTask(asTask); err != nil {
		t.Fatal(err)
	}

	items, err := wb.ListAdminWork("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("행정 목록=%d want 2 (AS 제외)", len(items))
	}
	foundFree, foundLinked := false, false
	for _, it := range items {
		if it.TaskID == free.TaskID {
			foundFree = true
			if it.CustomerLabel() != "충남교육청" {
				t.Fatalf("직접입력 거래처=%q", it.CustomerLabel())
			}
			if it.Title != title {
				t.Fatalf("업무명=%q", it.Title)
			}
		}
		if it.TaskID == linked.TaskID {
			foundLinked = true
			if it.CustomerLabel() != "충남교육청" {
				t.Fatalf("마스터 거래처=%q org=%q", it.CustomerLabel(), it.OrgName)
			}
		}
	}
	if !foundFree || !foundLinked {
		t.Fatal("등록 건이 목록에 없음")
	}

	searched, err := wb.ListAdminWork("", "충남교육청")
	if err != nil || len(searched) != 2 {
		t.Fatalf("거래처 검색 len=%d err=%v", len(searched), err)
	}

	st, err := wb.CountAdminWorkStats()
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 2 || st.Waiting != 2 {
		t.Fatalf("현황 %+v", st)
	}

	weekly, err := NewStatsRepo(db).ListWeeklyEventRows("2026-08-10", "2026-08-17")
	if err != nil {
		t.Fatal(err)
	}
	var adminRows []model.WeeklyEventRow
	for _, r := range weekly {
		if r.Kind == model.WeeklyEventAdmin {
			adminRows = append(adminRows, r)
		}
	}
	if len(adminRows) != 2 {
		t.Fatalf("주간 행정=%d want 2", len(adminRows))
	}
	for _, r := range adminRows {
		if r.Customer != "충남교육청" {
			t.Fatalf("주간 거래처=%q want 충남교육청 (내용=%q)", r.Customer, r.Content)
		}
		if r.Customer == r.Content {
			t.Fatal("거래처 칸에 업무명이 들어감")
		}
	}
	foundTitle := false
	for _, r := range adminRows {
		if r.Content == title {
			foundTitle = true
		}
	}
	if !foundTitle {
		t.Fatalf("주간 접수내용(업무명) 미반영 %+v", adminRows)
	}

	cases, err := NewStatsRepo(db).listAdminCases("2026-08-10", "2026-08-17",
		ParseMeetingFilter(model.StatsScopeTeam, "", ""), statsCaseTimeline, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("통계 행정=%d", len(cases))
	}
	for _, c := range cases {
		if c.CustomerName != "충남교육청" {
			t.Fatalf("통계 거래처=%q symptom=%q", c.CustomerName, c.Symptom)
		}
	}
}
