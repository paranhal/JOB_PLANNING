package handler

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestProjectMatchPendingBadgeHelper(t *testing.T) {
	if got := projectMatchPendingBadge(false); got != "" {
		t.Fatalf("비pending=%q", got)
	}
	got := string(projectMatchPendingBadge(true))
	if got != projectMatchPendingBadgeHTML {
		t.Fatalf("문구·스타일 불일치 %q", got)
	}
	if !strings.Contains(got, `class="badge badge-warning text-xs"`) || !strings.Contains(got, "사업명 매칭 필요") {
		t.Fatal("task_show 배지와 다름")
	}
	show := string(mustRead(t, filepath.Join(findTemplateRoot(t), "workboard", "task_show.html")))
	if !strings.Contains(show, "{{projectMatchPendingBadge $t.ProjectMatchPending}}") {
		t.Fatal("상세 화면이 공용 헬퍼를 쓰지 않음")
	}
}

func Test95_113_PendingBadgeOnCalendarBarAndKanbanRender(t *testing.T) {
	root := findTemplateRoot(t)
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(
		filepath.Join(root, "workboard", "_board_bar.html"),
		filepath.Join(root, "workboard", "_board_assignee.html"),
		filepath.Join(root, "workboard", "_board_kanban_card.html"),
	)
	if err != nil {
		t.Fatal(err)
	}
	page := map[string]interface{}{"View": "week", "CanWrite": true, "ScheduleRedirect": "/workboard/board"}
	pend := boardItem{
		Task: model.WorkTask{TaskID: "T-PEND", Title: "매칭대기", ProjectMatchPending: true, StartTime: "09:00", EndTime: "09:30"},
		Type: boardTypeStyle{Label: "지원", Text: "text-ink", Bg: "bg-canvas"},
		Href: "/workboard/tasks/T-PEND", CanAction: true, ActionHref: "/workboard/tasks/T-PEND",
		StartHHMM: "09:00", EndHHMM: "09:30", DayPosHeight: 48, CanMove: true,
	}
	ok := pend
	ok.Task.TaskID = "T-OK"
	ok.Task.Title = "사업있음"
	ok.Task.ProjectMatchPending = false
	ok.Href = "/workboard/tasks/T-OK"
	ok.ActionHref = "/workboard/tasks/T-OK"

	barPend := execNamed(t, tmpl, "board_cal_bar", map[string]interface{}{"Item": pend, "Page": page})
	barOK := execNamed(t, tmpl, "board_cal_bar", map[string]interface{}{"Item": ok, "Page": page})
	if !strings.Contains(barPend, projectMatchPendingBadgeHTML) {
		t.Fatal("캘린더 바 pending 배지 없음")
	}
	if strings.Contains(barOK, projectMatchPendingBadgeHTML) || strings.Contains(barOK, "사업명 매칭 필요") {
		t.Fatal("캘린더 바에 비pending 배지")
	}
	if !strings.Contains(barPend, `data-board-bar="1"`) || !strings.Contains(barPend, `draggable="true"`) {
		t.Fatal("바 드래그 속성 깨짐")
	}
	if !strings.Contains(barPend, `class="board-bar-act"`) || !strings.Contains(barPend, ">조치<") {
		t.Fatal("바 조치 버튼 깨짐")
	}
	if !strings.Contains(barPend, `data-h="48"`) {
		t.Fatal("바 높이 속성 깨짐")
	}

	kanPend := execNamed(t, tmpl, "board_kanban_card", pend)
	kanOK := execNamed(t, tmpl, "board_kanban_card", ok)
	if !strings.Contains(kanPend, projectMatchPendingBadgeHTML) {
		t.Fatal("칸반 pending 배지 없음")
	}
	if strings.Contains(kanOK, projectMatchPendingBadgeHTML) || strings.Contains(kanOK, "사업명 매칭 필요") {
		t.Fatal("칸반에 비pending 배지")
	}
	if !strings.Contains(kanPend, `data-kanban-card="1"`) || !strings.Contains(kanPend, ">조치<") {
		t.Fatal("칸반 카드 동작 깨짐")
	}
}

func Test95_113_PendingBadgeOnBoardHTTP(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p113badge.db"))
	ck := jwtCookieUser(t, me)
	const day = "2026-11-10"
	pend := &model.WorkTask{
		Title: "매칭대기카드", WorkDate: day, DueDate: day, StartTime: "09:00", EndTime: "09:30",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkSupport, Priority: model.WBPriorityNormal,
		ProjectMatchPending: true,
	}
	ok := &model.WorkTask{
		Title: "연결됨카드", WorkDate: day, DueDate: day, StartTime: "10:00", EndTime: "10:30",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(pend); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ok); err != nil {
		t.Fatal(err)
	}
	week := doGetWith(t, e, "/workboard/board?view=week&date="+day+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(week, "매칭대기카드") {
		t.Fatal("주간 캘린더에 pending 업무 없음")
	}
	if !strings.Contains(week, projectMatchPendingBadgeHTML) {
		t.Fatal("주간 캘린더 배지 없음")
	}
	if strings.Count(week, projectMatchPendingBadgeHTML) != 1 {
		t.Fatal("주간에 pending이 아닌 업무에도 배지가 있음")
	}
	dayBody := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(dayBody, projectMatchPendingBadgeHTML) {
		t.Fatal("일간 캘린더 배지 없음")
	}
	if strings.Count(dayBody, projectMatchPendingBadgeHTML) != 1 {
		t.Fatal("일간에 pending이 아닌 업무에도 배지가 있음")
	}
	show := doGetWith(t, e, "/workboard/tasks/"+pend.TaskID, ck).Body.String()
	if !strings.Contains(show, projectMatchPendingBadgeHTML) {
		t.Fatal("상세 배지 헬퍼 불일치")
	}
}

func execNamed(t *testing.T, tmpl *template.Template, name string, data interface{}) string {
	t.Helper()
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return buf.String()
}
