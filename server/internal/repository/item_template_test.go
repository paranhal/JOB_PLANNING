package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_83_MigrationUpDownNoProdPatch(t *testing.T) {
	up, err := itemTemplatesMigrationSQL()
	if err != nil {
		t.Fatal(err)
	}
	if err := itemTemplateSQLForbiddenProdPatch(up); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(up, "wb_item_templates") || !strings.Contains(up, "wb_item_template_links") {
		t.Fatal("up 표 부족")
	}
	if !strings.Contains(up, "wb_item_unit") {
		t.Fatal("단위 코드 없음")
	}
	down, err := itemTemplatesDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(down, "DROP TABLE") {
		t.Fatal("down 없음")
	}
	db, err := InitDB(filepath.Join(t.TempDir(), "m98.db"))
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
	if _, err := db.Exec(`INSERT INTO work_task_items(item_id,task_id,seq,title,status,note) VALUES('WI-old98',?,1,'기존','todo','')`, tid); err != nil {
		t.Fatal(err)
	}
	applyItemTemplatesV98(db)
	applyItemTemplatesV98(db)
	var tpl, setMain, unit string
	if err := db.QueryRow(`SELECT template_id, set_main_item_id, unit FROM work_task_items WHERE item_id='WI-old98'`).Scan(&tpl, &setMain, &unit); err != nil {
		t.Fatal(err)
	}
	if tpl != "" || setMain != "" || unit != "" {
		t.Fatalf("기존 행 기본값 tpl=%s set=%s unit=%s", tpl, setMain, unit)
	}
	if err := execWorkGroupSQL(db, down); err != nil {
		t.Fatal(err)
	}
}

func Test95_83_TemplateCRUDAndCycle(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "tpl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewItemTemplateRepo(db)
	a := &model.ItemTemplate{Name: "터파기", Kind: model.ItemKindWork, DefaultQty: 1, Unit: "식"}
	if err := r.Create(a, nil); err != nil {
		t.Fatal(err)
	}
	b := &model.ItemTemplate{Name: "야외 배관 작업", Kind: model.ItemKindWork, DefaultQty: 1, Unit: "식"}
	if err := r.Create(b, []model.ItemTemplateLink{{ChildTemplateID: a.TemplateID, DefaultChecked: true}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Update(a, []model.ItemTemplateLink{{ChildTemplateID: b.TemplateID, DefaultChecked: true}}); err == nil {
		t.Fatal("순환 허용")
	}
	if err := r.Update(b, []model.ItemTemplateLink{{ChildTemplateID: b.TemplateID, DefaultChecked: true}}); err == nil {
		t.Fatal("자기 연결 허용")
	}
	dup := &model.ItemTemplate{Name: "야외 배관 작업", Kind: model.ItemKindWork, DefaultQty: 1, Unit: "식"}
	if err := r.Create(dup, nil); err == nil {
		t.Fatal("이름 중복 허용")
	}
	zero := &model.ItemTemplate{Name: "수량0", Kind: model.ItemKindWork, DefaultQty: 0, Unit: "건"}
	if err := r.Create(zero, nil); err == nil {
		t.Fatal("수량 0 허용")
	}
	bad := &model.ItemTemplate{Name: "단위", Kind: model.ItemKindWork, DefaultQty: 1, Unit: "kg"}
	if err := r.Create(bad, nil); err == nil {
		t.Fatal("코드 외 단위 허용")
	}
}
