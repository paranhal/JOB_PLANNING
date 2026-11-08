package handler

import (
	"strings"
	"testing"

	"customer-support/internal/repository"
)

func Test95_149A_TaskShowDelayHistoryRenders(t *testing.T) {
	e, repo := newWorkboardServer(t, "d149a.db")
	tk := seed148Task(t, repo, "지연이력업무", "", "", "2026-11-08")
	if err := repo.InsertWorkTaskDelay(repository.WorkTaskDelay{
		TaskID: tk.TaskID, Kind: repository.DelayKindReason,
		Reason: "일정 조정", CreatedByName: "최혜영",
	}); err != nil {
		t.Fatal(err)
	}
	rec := doGetWith(t, e, "/workboard/tasks/"+tk.TaskID, jwtCookie(t))
	if rec.Code != 200 {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String()[:minInt(400, rec.Body.Len())])
	}
	body := rec.Body.String()
	if strings.Contains(body, "Internal Server Error") || strings.Contains(body, "function \"formatLocalDateTime\"") {
		t.Fatal(body[:minInt(500, len(body))])
	}
	if !strings.Contains(body, "지연·이월 이력") || !strings.Contains(body, "일정 조정") {
		t.Fatal("이력 없음")
	}
}
