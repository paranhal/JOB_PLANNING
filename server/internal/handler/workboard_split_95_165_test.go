package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_165_SplitEachTaskAndToken(t *testing.T) {
	e, repo, _, db := newProjectWorkHarness(t)
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, model_name, install_location, operation_status) VALUES
		('A-165','C-high','스위치','SW-100','본관','operating'),
		('A-166','C-high','발급기','M1','창구','operating')`); err != nil {
		t.Fatal(err)
	}
	page := doGet(t, e, "/workboard/register?view=day")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "1건의 작업으로 처리") || !strings.Contains(page.Body.String(), "js-target-split hidden") {
		t.Fatalf("split ui %d", page.Code)
	}
	asEcho, _, asWB, _ := new45DServer(t)
	asPage := doGet(t, asEcho, "/as/new")
	if asPage.Code != http.StatusOK || strings.Contains(asPage.Body.String(), "split_mode") {
		t.Fatalf("AS 접수 %d", asPage.Code)
	}
	adminSplit := doForm(t, asEcho, "/admin-work", url.Values{
		"work_type": {"admin"}, "title": {"행정분할"}, "due_date": {"2026-11-30"},
		"work_date": {"2026-11-20"}, "status": {model.WBTaskWaiting}, "customer_id": {"C1"},
		"split_mode": {"each"}, "asset_id": {"A1", "A2"},
		"split_label_asset_A1": {"발급기"}, "split_label_asset_A2": {"반납기"},
	})
	if adminSplit.Code != http.StatusSeeOther {
		t.Fatalf("admin split %d %s", adminSplit.Code, adminSplit.Body.String())
	}
	adminTasks, err := asWB.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	adminTitles := map[string]bool{}
	for _, tk := range adminTasks {
		adminTitles[tk.Title] = true
	}
	if !adminTitles["행정분할 - 발급기"] || !adminTitles["행정분할 - 반납기"] || len(adminTasks) != 2 {
		t.Fatalf("admin split %+v", adminTitles)
	}

	one := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"한건"}, "work_date": {"2026-11-20"},
		"start_time": {"09:00"}, "end_time": {"10:00"}, "assignee": {"관리자"},
		"split_mode": {"single"}, "asset_id": {"A-165", "A-166"},
	})
	if one.Code != http.StatusSeeOther {
		t.Fatalf("single %d %s", one.Code, one.Body.String())
	}

	form := url.Values{
		"work_type": {"project_work"}, "title": {"현장분할"}, "work_subtype": {"other"},
		"work_date": {"2026-11-21"}, "assignee": {"관리자"}, "project_id": {"WP-O01"},
		"split_mode": {"each"}, "asset_id": {"A-165", "A-166"},
		"split_label_asset_A-165": {"스위치"}, "split_label_asset_A-166": {"발급기"},
		"site_id": {"C-high"}, "submit_token": {"tok-165c-asset"},
	}
	first := doForm(t, e, "/workboard/tasks", form)
	if first.Code != http.StatusSeeOther {
		t.Fatalf("each %d %s", first.Code, first.Body.String())
	}
	again := doForm(t, e, "/workboard/tasks", form)
	if again.Code != http.StatusSeeOther {
		t.Fatalf("replay %d %s", again.Code, again.Body.String())
	}

	sites := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"사이트분할"}, "work_subtype": {"other"},
		"work_date": {"2026-11-22"}, "assignee": {"관리자"}, "project_id": {"WP-O01"},
		"split_mode": {"each"}, "split_kind": {"site"},
		"site_id":                 {"C-high", "C-mid"},
		"split_label_site_C-high": {"계룡고"}, "split_label_site_C-mid": {"계룡중"},
		"submit_token": {"tok-165c-site"},
	})
	if sites.Code != http.StatusSeeOther {
		t.Fatalf("sites %d %s", sites.Code, sites.Body.String())
	}

	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]model.WorkTask{}
	for _, tk := range tasks {
		byTitle[tk.Title] = tk
	}
	single, ok := byTitle["한건"]
	if !ok {
		t.Fatal("한건 없음")
	}
	got, err := repo.ListTaskAssets(single.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("single links %d", len(got))
	}
	for _, title := range []string{"현장분할 - 스위치", "현장분할 - 발급기"} {
		if _, ok := byTitle[title]; !ok {
			t.Fatalf("missing %s in %v", title, keysOf(byTitle))
		}
	}
	a := byTitle["현장분할 - 스위치"]
	b := byTitle["현장분할 - 발급기"]
	if a.Assignee != b.Assignee || a.WorkDate != b.WorkDate || a.ProjectID != b.ProjectID {
		t.Fatalf("inherit %s %s / %s %s / %s %s", a.Assignee, b.Assignee, a.WorkDate, b.WorkDate, a.ProjectID, b.ProjectID)
	}
	alink, _ := repo.ListTaskAssets(a.TaskID)
	blink, _ := repo.ListTaskAssets(b.TaskID)
	if len(alink) != 1 || alink[0].AssetID != "A-165" || len(blink) != 1 || blink[0].AssetID != "A-166" {
		t.Fatalf("asset links %+v %+v", alink, blink)
	}
	if _, ok := byTitle["현장분할"]; ok {
		t.Fatal("원제가 그대로 남음")
	}
	hi := byTitle["사이트분할 - 계룡고"]
	mid := byTitle["사이트분할 - 계룡중"]
	if hi.CustomerID != "C-high" || mid.CustomerID != "C-mid" {
		t.Fatalf("site customer %s %s", hi.CustomerID, mid.CustomerID)
	}
	hs, _ := repo.ListTaskSites(hi.TaskID)
	ms, _ := repo.ListTaskSites(mid.TaskID)
	if len(hs) != 1 || hs[0].CustomerID != "C-high" || len(ms) != 1 || ms[0].CustomerID != "C-mid" {
		t.Fatalf("sites %+v %+v", hs, ms)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE title LIKE '현장분할%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("token replay rows %d", n)
	}
}

func keysOf(m map[string]model.WorkTask) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
