package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_60_ScheduleShowToggleBoard(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b60.db"))
	ck := jwtCookieUser(t, me)
	planTk := &model.WorkTask{
		Title: "예정만60", WorkDate: "2026-10-10", DueDate: "2026-10-10", StartTime: "09:00", EndTime: "10:00",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(planTk); err != nil {
		t.Fatal(err)
	}
	actTk := &model.WorkTask{
		Title: "완료일만60", WorkDate: "2026-09-01", DueDate: "2026-09-01", StartTime: "11:00", EndTime: "12:00",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(actTk); err != nil {
		t.Fatal(err)
	}
	actTk.Status = model.WBTaskComplete
	actTk.CompleteDate = "2026-10-10"
	if err := repo.UpdateTask(actTk); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertWorkTaskProcess(model.WorkTaskProcess{
		TaskID: actTk.TaskID, OrgID: actTk.OrgID, ProcessAt: "2026-10-10 11:00:00",
		Worker: me.FullName, WorkerUserID: me.UserID, WorkContent: "조치60",
	}); err != nil {
		t.Fatal(err)
	}
	def := doGetWith(t, e, "/workboard/board?view=sched&range=day&date=2026-10-10&auto=0", ck)
	if def.Code != http.StatusOK {
		t.Fatalf("default %d", def.Code)
	}
	body := def.Body.String()
	if !strings.Contains(body, "접수(계획)") || !strings.Contains(body, "완료") {
		t.Fatal("토글 문구 없음")
	}
	if !strings.Contains(body, "접수(계획) 1건") || !strings.Contains(body, "완료 1건") {
		t.Fatalf("건수 없음: %s", snippetShow(body))
	}
	if !strings.Contains(body, `data-show-bit="plan"`) || !strings.Contains(body, `data-show-bit="act"`) {
		t.Fatal("토글 비트 없음")
	}
	if !strings.Contains(body, `class="on" data-show-bit="plan"`) {
		t.Fatal("기본은 접수 on")
	}
	if strings.Contains(body, `class="on" data-show-bit="act"`) {
		t.Fatal("기본에 조치가 켜져 있음")
	}
	if !strings.Contains(body, "예정만60") {
		t.Fatal("기본(접수)에 계획 업무 없음")
	}
	if strings.Contains(body, "완료일만60") {
		t.Fatal("기본(접수)에 완료일만 업무가 보임")
	}
	page := doGetWith(t, e, "/workboard/board?view=sched&range=day&date=2026-10-10&auto=0", ck)
	b := page.Body.String()
	if !strings.Contains(b, "id=\"schedule-show\"") {
		t.Fatal("일정표 일 토글 없음")
	}
	if !strings.Contains(b, "show=") {
		t.Fatal("링크에 show 없음")
	}

	actOnly := doGetWith(t, e, "/workboard/board?view=sched&range=day&date=2026-10-10&auto=0&show=act", ck)
	ab := actOnly.Body.String()
	if strings.Contains(ab, "예정만60") {
		t.Fatal("조치만인데 계획 업무가 보임")
	}
	if !strings.Contains(ab, "완료일만60") {
		t.Fatal("조치만인데 완료 업무 없음")
	}
	if !strings.Contains(ab, "접수(계획) 1건") || !strings.Contains(ab, "완료 1건") {
		t.Fatal("꺼진 쪽 건수가 사라짐")
	}

	both := doGetWith(t, e, "/workboard/board?view=sched&range=day&date=2026-10-10&auto=0&show=plan,act", ck)
	bb := both.Body.String()
	if !strings.Contains(bb, "예정만60") || !strings.Contains(bb, "완료일만60") {
		t.Fatal("둘 다 켜기에서 누락")
	}

	empty := doGetWith(t, e, "/workboard/board?view=sched&range=day&date=2026-10-10&auto=0&show=", ck)
	if !strings.Contains(empty.Body.String(), "예정만60") || strings.Contains(empty.Body.String(), "완료일만60") {
		t.Fatal("빈 show 는 접수만이어야 함")
	}

	css, err := os.ReadFile(filepath.Join(findTemplateRoot(t), "..", "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".sched-show") {
		t.Fatal("tailwind 번들에 .sched-show 없음 — npm run build:css")
	}
	js, err := os.ReadFile(filepath.Join(findTemplateRoot(t), "..", "static", "js", "schedule-show.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "jp.schedule.show") {
		t.Fatal("localStorage 키 없음")
	}
}

func snippetShow(body string) string {
	i := strings.Index(body, "접수(계획)")
	if i < 0 {
		return body[:min(len(body), 200)]
	}
	end := i + 80
	if end > len(body) {
		end = len(body)
	}
	return body[i:end]
}
