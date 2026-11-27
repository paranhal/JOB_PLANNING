package repository

import (
	"database/sql"
	"log"
	"strings"

	"customer-support/internal/model"
)

// 95-224 §1: 업무 구분은 codes(code_group='work_category') 가 원본이다. code_value = work_tasks.activity_kind 키.

func applyWorkCategoryCodesV116(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := leaveLedgerSQL("116_work_category_codes.sql")
	if err != nil {
		log.Printf("116 work_category: %v", err)
		return
	}
	if _, err := db.Exec(up); err != nil {
		log.Printf("116 work_category apply: %v", err)
		return
	}
	reloadCodes(db)
}

// syncWorkCategoryKinds 캐시에 올라온 codes 에서 업무 구분 목록을 만들어 모델에 반영한다.
func syncWorkCategoryKinds(items []model.Code) {
	var list []model.WeeklyActivityKind
	for _, c := range items {
		if c.CodeGroup != model.WorkCategoryCodeGroup {
			continue
		}
		key := strings.TrimSpace(c.CodeValue)
		if key == "" {
			continue
		}
		list = append(list, model.WeeklyActivityKind{Key: key, Label: strings.TrimSpace(c.CodeName), Inactive: !c.IsActive})
	}
	model.SetWeeklyActivityKinds(list)
}
