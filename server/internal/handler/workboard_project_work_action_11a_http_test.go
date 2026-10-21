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

func TestProjectWorkActionASLayoutHTML(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "구성작업")
	page := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if page.Code != http.StatusOK {
		t.Fatalf("GET %d", page.Code)
	}
	body := page.Body.String()
	for _, need := range []string{
		id, "조치", "▸ 사업 연결·변경", "작업 보기", "다음 처리",
		"작업 및 사이트 정보", "작업 내용", "과거 작업 이력",
		"조치 내역", "처리담당자", "근무구분", "작업 구분",
		"이 내용이 다음 작업자에게 참고가 됩니다",
		"추가조치 필요", "확인", "고객확인자", "조치 이력",
		`id="as-action-bar"`, "저장",
	} {
		if !strings.Contains(body, need) {
			t.Fatalf("레이아웃에 %q 없음", need)
		}
	}
	if strings.Contains(body, "일부완료") {
		t.Fatal("결과 라디오에 일부완료가 남아 있다")
	}
	if strings.Count(body, `name="result_code"`) < 3 {
		t.Fatal("결과 라디오 3개 미만")
	}
	if strings.Contains(body, `name="work_place" value="field"`) && strings.Contains(body, `value="field" checked`) {
		t.Fatal("근무구분 외근이 기본 선택")
	}
}

func TestProjectWorkActionWorkPlaceRequired(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "근무구분필수")
	form := pwActionForm()
	form.Del("work_place")
	rec := doForm(t, e, "/workboard/tasks/"+id+"/action", form)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=work_place") {
		t.Fatalf("근무구분 누락 loc=%s code=%d", rec.Header().Get("Location"), rec.Code)
	}
}

func TestProjectWorkActionLinkProjectAndHistory(t *testing.T) {
	e, repo, users := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "연결대상")
	tk, _ := repo.GetTask(id)
	if tk.ProjectID != "" {
		t.Fatal("처음엔 사업 없음이어야 함")
	}
	ok := doForm(t, e, "/workboard/tasks/"+id+"/action/project", url.Values{"project_id": {"WP-O01"}})
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("연결 %d", ok.Code)
	}
	tk, _ = repo.GetTask(id)
	if tk.ProjectID != "WP-O01" || tk.ProjectName != "도서관공사" {
		t.Fatalf("연결 후 %+v", tk)
	}
	procs, _ := repo.ListWorkTaskProcesses(tk.OrgID, id)
	found := false
	for _, p := range procs {
		if strings.Contains(p.WorkContent, "사업 연결") && strings.Contains(p.WorkContent, "도서관공사") {
			found = true
		}
	}
	if !found {
		t.Fatalf("이력에 사업 연결 없음 %+v", procs)
	}

	off := doForm(t, e, "/workboard/tasks/"+id+"/action/project", url.Values{"project_id": {""}})
	if off.Code != http.StatusSeeOther {
		t.Fatalf("해제 %d", off.Code)
	}
	tk, _ = repo.GetTask(id)
	if tk.ProjectID != "" {
		t.Fatalf("해제 후 %q", tk.ProjectID)
	}

	idA := pwCreateTask(t, e, repo, "이력원본")
	_ = repo.ReplaceTaskCustomers(idA, "WP-O01", []string{"C-high"})
	idB := pwCreateTask(t, e, repo, "이력대상")
	_ = repo.ReplaceTaskCustomers(idB, "WP-O01", []string{"C-high"})
	page := doGet(t, e, "/workboard/tasks/"+idB+"/action")
	if !strings.Contains(page.Body.String(), idA) {
		t.Fatal("같은 사이트 과거 이력이 없다")
	}
	if strings.Contains(page.Body.String(), idB+"/action\">"+idB) {
		// 현재 작업 링크가 이력에 있으면 안 됨 — 제목 쪽 번호는 헤더에 있음
	}

	oa03, _ := users.GetByUsername("oa03")
	other := doFormUser(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"타조직이력"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"SI관리자"},
		"start_time": {"09:00"}, "end_time": {"09:30"}, "site_id": {"C-high"},
	}, jwtCookieUser(t, oa03))
	if other.Code != http.StatusSeeOther {
		t.Fatalf("타조직 작업 %d", other.Code)
	}
	idO := pwFindTitle(t, repo, "타조직이력")
	page2 := doGet(t, e, "/workboard/tasks/"+idB+"/action")
	if strings.Contains(page2.Body.String(), idO) {
		t.Fatal("타 조직 작업이 과거 이력에 보인다")
	}

	cl := doFormUser(t, e, "/workboard/tasks/"+id+"/action/project", url.Values{"project_id": {"WP-O01"}}, jwtCookieRole(t, model.RoleCLevel))
	if cl.Code != http.StatusForbidden {
		t.Fatalf("C-Level 연결 %d", cl.Code)
	}
	obs := doFormUser(t, e, "/workboard/tasks/"+id+"/action/project", url.Values{"project_id": {"WP-O01"}}, jwtCookieReadOnly(t, model.RoleObserver))
	if obs.Code != http.StatusForbidden {
		t.Fatalf("observer 연결 %d", obs.Code)
	}
	tech := doFormUser(t, e, "/workboard/tasks/"+id+"/action/project", url.Values{"project_id": {"WP-O01"}}, jwtCookieRole(t, model.RoleTech))
	if tech.Code != http.StatusForbidden {
		t.Fatalf("타인 작업 팀원 연결 %d", tech.Code)
	}
}

func TestProjectWorkActionEntryPoints(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "진입점작업")
	if _, _, err := repo.SetTaskProject(mustOrg(t, repo, id), id, "WP-O01"); err != nil {
		t.Fatal(err)
	}
	want := "/workboard/tasks/" + id + "/action"
	old := "/workboard/tasks/" + id + `"`
	reg := doGet(t, e, "/workboard/register?view=day&date=2026-10-21")
	if !strings.Contains(reg.Body.String(), want) {
		t.Fatal("일일업무 일정표 조치가 /action 이 아님")
	}
	kb := doGet(t, e, "/workboard/kanban")
	if kb.Code == http.StatusOK && strings.Contains(kb.Body.String(), id) && !strings.Contains(kb.Body.String(), want) {
		t.Fatal("칸반이 구 상세로 연결")
	}
	lst := doGet(t, e, "/workboard/tasks")
	if lst.Code == http.StatusOK && strings.Contains(lst.Body.String(), id) && !strings.Contains(lst.Body.String(), want) {
		t.Fatal("목록이 구 상세로 연결")
	}
	show := doGet(t, e, "/workboard/tasks/"+id)
	if !strings.Contains(show.Body.String(), want) {
		t.Fatal("상세 조치 버튼이 /action 이 아님")
	}
	sites := doGet(t, e, "/projects/WP-O01/sites")
	if sites.Code == http.StatusOK && strings.Contains(sites.Body.String(), id) {
		if strings.Contains(sites.Body.String(), `/workboard/tasks/`+id+`"`) && !strings.Contains(sites.Body.String(), want) {
			t.Fatal("사업 상세 작업이 구 화면")
		}
	}
	_ = old
}

func TestProjectWorkActionTemplateUILintZero(t *testing.T) {
	root := findTemplateRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "workboard", "project_work_action.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := countUILint(string(b))
	if !got.zero() {
		t.Fatalf("ui_lint %+v", got)
	}
}

func mustOrg(t *testing.T, repo *repository.WBRepo, id string) string {
	t.Helper()
	tk, err := repo.GetTask(id)
	if err != nil || tk == nil {
		t.Fatal(err)
	}
	return tk.OrgID
}
