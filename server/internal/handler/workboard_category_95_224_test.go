package handler

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func with224(f url.Values, kv ...string) url.Values {
	for i := 0; i+1 < len(kv); i += 2 {
		f.Set(kv[i], kv[i+1])
	}
	return f
}

// 95-224 §1: 연결 대상(link_target)은 사업 또는 업무 구분 중 정확히 하나. 위반은 400이고 업무가 만들어지지 않는다.
func TestHTTP95_224_LinkTargetXOR(t *testing.T) {
	v := setup191(t)
	reject := func(title string, form url.Values) {
		t.Helper()
		if r := v.do("POST", "/workboard/tasks", form, v.admin); r.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d want 400 (%s)", title, r.Code, r.Header().Get("Location"))
		}
		if taskIDByTitle198(t, v, title) != "" {
			t.Fatalf("%s: 거절됐는데 업무가 만들어졌다", title)
		}
	}
	reject("연결없음-사업", with224(createForm198(v, "연결없음-사업", ""), "link_target", "project"))
	reject("둘다-사업구분", with224(createForm198(v, "둘다-사업구분", "mgmt"), "link_target", "project"))
	reject("둘다-구분사업", with224(createForm198(v, "둘다-구분사업", "mgmt"), "link_target", "category", "project_id", "WP-NOPE"))
	reject("구분없음", with224(createForm198(v, "구분없음", ""), "link_target", "category"))
	reject("나중에-값있음", with224(createForm198(v, "나중에-값있음", "mgmt"), "link_target", "later"))
	reject("이상한-대상", with224(createForm198(v, "이상한-대상", ""), "link_target", "zzz"))

	// 업무 구분만: 저장되고 구분이 붙는다
	if r := v.do("POST", "/workboard/tasks", with224(createForm198(v, "구분 등록", "service"), "link_target", "category"), v.admin); r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("구분 등록 %d %s", r.Code, r.Header().Get("Location"))
	}
	if id := taskIDByTitle198(t, v, "구분 등록"); id == "" || v.wb.TaskActivityKind(id) != "service" {
		t.Fatal("구분이 저장되지 않았다")
	}
	// 나중에 지정: 비워 둔 채 저장된다(분류 필요)
	if r := v.do("POST", "/workboard/tasks", with224(createForm198(v, "나중에 지정", ""), "link_target", "later"), v.admin); r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("나중에 지정 %d %s", r.Code, r.Header().Get("Location"))
	}
	later := taskIDByTitle198(t, v, "나중에 지정")
	if later == "" || v.wb.TaskActivityKind(later) != "" {
		t.Fatal("나중에 지정 업무가 잘못 저장됐다")
	}
	// 회귀: link_target 을 안 보내는 옛 요청은 종전대로 저장된다
	if r := v.do("POST", "/workboard/tasks", createForm198(v, "옛 요청", ""), v.admin); r.Code != http.StatusSeeOther {
		t.Fatalf("옛 요청 %d", r.Code)
	}
	// 사후 지정: 이후에 구분을 붙이면 이력(변경 로그)에 category_code 사유가 남는다
	if _, _, err := v.wb.SetTaskActivity(model.OrgIDLibrary, later, "asset"); err != nil {
		t.Fatal(err)
	}
	if v.wb.TaskActivityKind(later) != "asset" {
		t.Fatal("사후 지정이 안 됐다")
	}
}

// 목록: 사업도 구분도 없는 일반 업무에만 「분류 필요」, 구분이 있으면 구분 이름. 원본 기반 업무는 대상이 아니다.
func TestListBadge95_224(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "b224.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id) VALUES('B1','admin','분류 없음','waiting','O01')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,activity_kind) VALUES('B2','admin','구분 있음','waiting','O01','tech')`,
		`INSERT INTO work_projects(project_id,name,status,org_id) VALUES('PB','사업','active','O01')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,project_id) VALUES('B3','admin','사업 있음','waiting','O01','PB')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,source_type,source_id) VALUES('B4','admin','[AS]원본','waiting','O01','as','AS-X')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	h := New(db)
	tasks := []model.WorkTask{
		{TaskID: "B1", WorkType: model.WBWorkAdmin},
		{TaskID: "B2", WorkType: model.WBWorkAdmin},
		{TaskID: "B3", WorkType: model.WBWorkAdmin, ProjectID: "PB"},
		{TaskID: "B4", WorkType: model.WBWorkAdmin, SourceType: "as"},
	}
	h.Workboard.fillCategoryMarks(tasks)
	if !tasks[0].NeedCategory || tasks[0].ActivityLabel != "" {
		t.Fatalf("분류 없는 업무: %+v", tasks[0])
	}
	if tasks[1].NeedCategory || tasks[1].ActivityLabel != "영업" {
		t.Fatalf("구분 있는 업무: %+v", tasks[1])
	}
	if tasks[2].NeedCategory || tasks[3].NeedCategory {
		t.Fatalf("사업 있는 업무·원본 기반 업무에 배지가 붙었다: %+v %+v", tasks[2], tasks[3])
	}
	raw, err := os.ReadFile("web/templates/workboard/list.html")
	if err != nil || !strings.Contains(string(raw), "data-need-category") || !strings.Contains(string(raw), "분류 필요") {
		t.Fatal("목록 템플릿에 「분류 필요」 배지가 없다")
	}
}

// 비활성 구분은 새로 고를 수 없고, 이미 붙은 업무의 표시는 남는다.
func TestHTTP95_224_InactiveCategory(t *testing.T) {
	v := setup191(t)
	if _, _, err := v.wb.SetTaskActivity(model.OrgIDLibrary, v.progress, "strategy"); err != nil {
		t.Fatal(err)
	}
	defer model.SetWeeklyActivityKinds(nil)
	list := model.DefaultWeeklyActivityKinds()
	for i := range list {
		if list[i].Key == "strategy" {
			list[i].Inactive = true
		}
	}
	model.SetWeeklyActivityKinds(list)
	if r := v.do("POST", "/workboard/tasks", with224(createForm198(v, "비활성 신규", "strategy"), "link_target", "category"), v.admin); r.Code != http.StatusBadRequest {
		t.Fatalf("비활성 구분 신규 선택 %d want 400", r.Code)
	}
	if _, _, err := v.wb.SetTaskActivity(model.OrgIDLibrary, v.done, "strategy"); err == nil {
		t.Fatal("비활성 구분으로 사후 지정이 됐다")
	}
	if got := model.WeeklyActivityLabel("strategy"); got != "전략및혁신" {
		t.Fatalf("비활성이어도 기존 표시 이름은 남아야 한다: %q", got)
	}
	if v.wb.TaskActivityKind(v.progress) != "strategy" {
		t.Fatal("이미 붙은 업무의 구분이 사라졌다")
	}
}
