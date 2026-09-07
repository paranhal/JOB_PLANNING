package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestRegisterKanbanHTTPDayAndWeek(t *testing.T) {
	e, repo := newWorkboardServer(t, "reg-kanban.db")
	today := time.Now().Format("2006-01-02")
	if err := repo.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "칸반행정", Status: model.WBTaskInProgress,
		Assignee: "관리자", DueDate: today, WorkDate: today,
	}); err != nil {
		t.Fatal(err)
	}

	day := doGet(t, e, "/workboard/register?view=day&date="+today+"&display=kanban")
	if day.Code != http.StatusOK {
		t.Fatalf("일일 칸반 status=%d", day.Code)
	}
	body := day.Body.String()
	for _, want := range []string{"미계획업무", "오늘 예정", "진행중", "지연", "오늘 완료", ">일정표<", ">칸반<"} {
		if !strings.Contains(body, want) {
			t.Errorf("일일 칸반에 %q 없음", want)
		}
	}

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	prev := doGet(t, e, "/workboard/register?view=day&date="+yesterday+"&display=kanban")
	if prev.Code != http.StatusOK {
		t.Fatalf("어제 칸반 status=%d", prev.Code)
	}
	wantLabel := model.FormatMonthDay(yesterday) + " 예정"
	if !strings.Contains(prev.Body.String(), wantLabel) {
		t.Errorf("어제 열 이름 %q 없음", wantLabel)
	}

	week := doGet(t, e, "/workboard/register?view=week&date="+today+"&display=kanban")
	if week.Code != http.StatusOK {
		t.Fatalf("주간 status=%d", week.Code)
	}
	if strings.Contains(week.Body.String(), ">칸반<") {
		t.Fatal("주간 보기에 칸반 버튼이 있다")
	}

	block := doForm(t, e, "/workboard/register/kanban-move", url.Values{
		"date":     {today},
		"from":     {model.WorkBucketToday},
		"to":       {model.WorkBucketUnplanned},
		"item_key": {"task:x"},
		"redirect": {"/workboard/register?view=day&date=" + today + "&display=kanban"},
	})
	if block.Code != http.StatusSeeOther {
		t.Fatalf("미계획 이동 status=%d", block.Code)
	}
	if loc := block.Header().Get("Location"); !strings.Contains(loc, "err=kanban_unplanned") {
		t.Fatalf("미계획 이동을 막아야 함: %s", loc)
	}
}
