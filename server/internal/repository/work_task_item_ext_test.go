package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_82_MigrationUpDownNoProdPatch(t *testing.T) {
	up, err := workTaskItemExtMigrationSQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(up, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		u := strings.ToUpper(trim)
		if strings.Contains(u, "UPDATE ") && strings.Contains(u, "WORK_TASK_ITEMS") {
			t.Fatal("운영 행 UPDATE 금지")
		}
		if strings.Contains(u, "INSERT INTO WORK_TASK_ITEMS") {
			t.Fatal("work_task_items INSERT 금지")
		}
	}
	if !strings.Contains(up, "work_task_item_assignees") || !strings.Contains(up, "work_task_item_assets") {
		t.Fatal("up 표 부족")
	}
	if !strings.Contains(up, "wb_item_kind") {
		t.Fatal("코드 그룹 없음")
	}
	down, err := workTaskItemExtDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(down, "DROP TABLE") {
		t.Fatal("down 없음")
	}
	db, err := InitDB(filepath.Join(t.TempDir(), "m97.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)
	if err := wb.CreateTask(&model.WorkTask{Title: "기존작업", WorkDate: "2026-11-02", DueDate: "2026-11-02",
		OrgID: model.OrgIDLibrary, Assignee: "관리자", Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin}); err != nil {
		t.Fatal(err)
	}
	tid := ""
	_ = db.QueryRow(`SELECT task_id FROM work_tasks LIMIT 1`).Scan(&tid)
	if _, err := db.Exec(`INSERT INTO work_task_items(item_id,task_id,seq,title,status,note) VALUES('WI-old',?,1,'기존','todo','')`, tid); err != nil {
		t.Fatal(err)
	}
	applyWorkTaskItemExtV97(db)
	applyWorkTaskItemExtV97(db)
	var kind, product string
	var qty int
	if err := db.QueryRow(`SELECT kind, product_name, qty FROM work_task_items WHERE item_id='WI-old'`).Scan(&kind, &product, &qty); err != nil {
		t.Fatal(err)
	}
	if kind != "etc" || product != "" || qty != 1 {
		t.Fatalf("기존 행 기본값 kind=%s product=%s qty=%d", kind, product, qty)
	}
	if err := execWorkGroupSQL(db, down); err != nil {
		t.Fatal(err)
	}
}

func Test95_82_ItemKindSummaryQuery(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "s97.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)
	if err := wb.CreateTask(&model.WorkTask{Title: "집계작업", WorkDate: "2026-11-02", DueDate: "2026-11-02",
		OrgID: model.OrgIDLibrary, Assignee: "관리자", Status: model.WBTaskWaiting, WorkType: model.WBWorkProjectWork}); err != nil {
		t.Fatal(err)
	}
	tasks, _ := wb.ListTasks()
	id := tasks[0].TaskID
	if err := wb.ReplaceItems(id, []model.WorkTaskItem{
		{Kind: model.ItemKindInstall, Title: "카메라", Qty: 3, Status: model.ItemDone},
		{Kind: model.ItemKindAS, Title: "DVR", Qty: 1, Status: model.ItemTodo},
	}); err != nil {
		t.Fatal(err)
	}
	sum, err := wb.ItemKindSummary([]string{id, id})
	if err != nil {
		t.Fatal(err)
	}
	got := sum[id]
	if got.Line != "장착 1건 · AS 1건" {
		t.Fatalf("line %q", got.Line)
	}
	if got.LineQty != "장착 1건(3대) · AS 1건" {
		t.Fatalf("qty %q", got.LineQty)
	}
}
