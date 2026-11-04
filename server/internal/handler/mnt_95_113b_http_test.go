package handler

import (
	"bytes"
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_113BUpdateVisitKeepsProjectWhenOmitted(t *testing.T) {
	e, db := newMntSyncServer(t, "mnt_keep_proj.db")
	seedVisit(t, db, "mvs_keep", "2026-08-12", "최혜영", "KLAS", "종촌동도서관")
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid)
		VALUES ('WP-KEEP','유지사업','active',?,0,0)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE maintenance_visits SET project_id='WP-KEEP', notes='원래' WHERE visit_id='mvs_keep'`); err != nil {
		t.Fatal(err)
	}

	rec := doForm(t, e, "/maintenance/visits/mvs_keep/update", url.Values{
		"plan_id":      {"MP1"},
		"visit_date":   {"2026-08-12"},
		"customer_id":  {"C_mvs_keep"},
		"assignee":     {"최혜영"},
		"product_type": {"KLAS"},
		"notes":        {"비고만 수정"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	var pid, notes string
	if err := db.QueryRow(`SELECT COALESCE(project_id,''), COALESCE(notes,'') FROM maintenance_visits WHERE visit_id='mvs_keep'`).Scan(&pid, &notes); err != nil {
		t.Fatal(err)
	}
	if pid != "WP-KEEP" {
		t.Fatalf("project_id=%q want WP-KEEP", pid)
	}
	if notes != "비고만 수정" {
		t.Fatalf("notes=%q", notes)
	}
}

func Test95_113BPlanShowEditHasProjectNameAndHydrate(t *testing.T) {
	files := planShowParseFiles(t)
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		t.Fatal(err)
	}
	visits := sampleVisits()
	visits[0].ProjectID = "WP-KEEP"
	visits[0].ProjectName = "유지사업"
	data := planViewData("calendar", 8, visits)
	data["CanEdit"] = true
	data["ProjectPicker"] = ProjectPickerView{Selected: "", SelectedName: "", Own: []projectPickerItem{{ProjectID: "WP-KEEP", Name: "유지사업"}}}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	if !strings.Contains(body, `data-project-name="유지사업"`) && !strings.Contains(body, `:data-project-name="card.project_name"`) {
		t.Fatal("수정 버튼에 사업명 없음")
	}
	if !strings.Contains(body, "hydrateEditProject") {
		t.Fatal("피커 초기값 동기 없음")
	}
	if !strings.Contains(body, "data-mnt-edit-picker") {
		t.Fatal("수정 모달 피커 영역 없음")
	}
	if !strings.Contains(body, "선택:") {
		t.Fatal("선택: 표시 없음")
	}
}

func Test95_113BDashboardMatchPendingField(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "dash113b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	prepo := repository.NewProjectRepo(db)
	got, err := prepo.CountProjectActivities()
	if err != nil {
		t.Fatal(err)
	}
	if got.MatchPending != 0 {
		t.Fatalf("빈 DB 미매칭=%d", got.MatchPending)
	}
}
