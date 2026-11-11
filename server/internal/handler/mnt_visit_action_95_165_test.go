package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/repository"
)

func Test95_165D_VisitPickerCopiesAssetsAndRefusesSplit(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES ('v165','MP47','2026-09-18','c47',0,'normal','최혜영','KLAS',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, product_type, operation_status)
		VALUES ('A165-1','c47','서버','HW','operating'),
		       ('A165-2','c47','게이트','HW','operating')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, err := users.GetByUsername("oa47")
	if err != nil || oa == nil {
		t.Fatal(err)
	}
	ck := jwtCookieVerified(t, oa)

	split := doMnt47Form(t, e, "/maintenance/visits/v165/action", url.Values{
		"assignee": {"최혜영"}, "visit_date": {"2026-09-18"}, "status": {"open"},
		"target_asset_id": {"A165-1", "A165-2"},
		"split_mode":      {"each"},
	}, ck)
	if split.Code != http.StatusOK || !strings.Contains(split.Body.String(), "정기점검") {
		t.Fatalf("분리 거부 %d %s", split.Code, split.Body.String())
	}
	var tasks int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type='maintenance' AND source_id='v165'`).Scan(&tasks)
	if tasks != 0 {
		t.Fatalf("거부 후 업무 %d", tasks)
	}

	save := doMnt47Form(t, e, "/maintenance/visits/v165/action", url.Values{
		"assignee": {"최혜영"}, "visit_date": {"2026-09-18"}, "status": {"open"},
		"start_time": {"09:00"}, "end_time": {"10:00"},
		"target_asset_id":    {"A165-1", "A165-2"},
		"target_custom_name": {"직접장비"},
	}, ck)
	if save.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", save.Code, save.Body.String())
	}

	var visitN, customN int
	if err := db.QueryRow(`SELECT COUNT(*) FROM maintenance_visit_targets WHERE visit_id='v165' AND asset_id!=''`).Scan(&visitN); err != nil {
		t.Fatal(err)
	}
	if visitN != 2 {
		t.Fatalf("방문 자산 %d", visitN)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM maintenance_visit_targets WHERE visit_id='v165' AND name='직접장비'`).Scan(&customN); err != nil {
		t.Fatal(err)
	}
	if customN != 1 {
		t.Fatalf("직접 입력 %d", customN)
	}
	var taskID string
	if err := db.QueryRow(`SELECT task_id FROM work_tasks WHERE source_type='maintenance' AND source_id='v165'`).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT asset_id FROM work_task_assets WHERE task_id=? ORDER BY asset_id`, taskID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	if len(got) != 2 || got[0] != "A165-1" || got[1] != "A165-2" {
		t.Fatalf("업무 자산 %v", got)
	}
}
