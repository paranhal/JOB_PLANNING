package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-224: 업무 구분 코드(work_category) — 시드 7종, 삭제 금지, 값·이름 검증, 비활성 반영, 사후 지정 이력.
func TestWorkCategoryCodes(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "wc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); model.SetWeeklyActivityKinds(nil) })
	repo := NewCodeRepo(db)
	codes, _ := repo.ListByGroup(model.WorkCategoryCodeGroup)
	if len(codes) != 7 {
		t.Fatalf("시드 %d종 want 7", len(codes))
	}
	want := map[string]string{"mgmt": "경영지원", "asset": "자산관리", "tech": "영업", "strategy": "전략및혁신", "service": "사업 부문", "biz_support": "업무지원", "growth": "역량강화"}
	for _, c := range codes {
		if want[c.CodeValue] != c.CodeName {
			t.Fatalf("%s=%q want %q", c.CodeValue, c.CodeName, want[c.CodeValue])
		}
	}
	// 두 번 시드해도 늘지 않는다(멱등)
	applyWorkCategoryCodesV116(db)
	if again, _ := repo.ListByGroup(model.WorkCategoryCodeGroup); len(again) != 7 {
		t.Fatalf("재시드 뒤 %d종", len(again))
	}
	// 삭제 금지, 이상한 값·중복 거절
	if err := repo.Delete(codes[0].CodeID); err == nil || !strings.Contains(err.Error(), "삭제") {
		t.Fatalf("삭제가 막히지 않았다: %v", err)
	}
	if err := repo.Create(&model.Code{CodeGroup: model.WorkCategoryCodeGroup, CodeValue: "Bad Key", CodeName: "x", IsActive: true}); err == nil {
		t.Fatal("잘못된 코드 값이 통과했다")
	}
	if err := repo.Create(&model.Code{CodeGroup: model.WorkCategoryCodeGroup, CodeValue: "mgmt", CodeName: "다른이름", IsActive: true}); err == nil {
		t.Fatal("중복 코드 값이 통과했다")
	}
	// 새 구분 추가 → 선택 목록에 나타난다. 비활성으로 바꾸면 선택 목록에서 빠지고 표시 이름은 남는다.
	nc := &model.Code{CodeGroup: model.WorkCategoryCodeGroup, CodeValue: "field_ops", CodeName: "현장지원", SortOrder: 80, IsActive: true}
	if err := repo.Create(nc); err != nil {
		t.Fatal(err)
	}
	if !model.IsWeeklyActivitySelectable("field_ops") {
		t.Fatal("추가한 구분을 고를 수 없다")
	}
	nc.IsActive = false
	if err := repo.Update(nc); err != nil {
		t.Fatal(err)
	}
	if model.IsWeeklyActivitySelectable("field_ops") || model.WeeklyActivityLabel("field_ops") != "현장지원" {
		t.Fatal("비활성 처리가 반영되지 않았다")
	}
	// 사후 지정 이력
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id) VALUES('WT-224L','admin','이력 시험','waiting','O01')`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewWBRepo(db).SetTaskActivity("O01", "WT-224L", "mgmt"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE entity_id='WT-224L' AND reason LIKE '%category_code%'`).Scan(&n)
	if n != 1 {
		t.Fatalf("사후 지정 이력 %d건 want 1", n)
	}
}
