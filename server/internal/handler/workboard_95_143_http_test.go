package handler

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_143_BoardDoneHeadReadableAllTypes(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d143.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 3).Format(dateLayout)
	types := []struct {
		title string
		wtype string
		start string
		end   string
	}{
		{"완료AS143", model.WBWorkAS, "07:00", "08:00"},
		{"완료MNT143", model.WBWorkMaintenance, "08:00", "09:00"},
		{"완료PW143", model.WBWorkProjectWork, "09:00", "10:00"},
		{"완료판매143", model.WBWorkSales, "10:00", "11:00"},
		{"완료행정143", model.WBWorkAdmin, "11:00", "12:00"},
		{"완료지원143", model.WBWorkSupport, "12:00", "13:00"},
	}
	for _, row := range types {
		mustCreateBoardTask(t, repo, row.title, day, row.start, row.end, model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, row.wtype)
	}
	mustCreateBoardTask(t, repo, "예정143", day, "14:00", "15:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkProjectWork)

	q := "&auto=0&show=plan,act&mine=0"
	week := doGetWith(t, e, "/workboard/board?view=sched&range=week&date="+day+q, ck).Body.String()
	dayHTML := doGetWith(t, e, "/workboard/board?view=sched&range=day&date="+day+q, ck).Body.String()
	check := func(label, body, titleClass, bgHint string) {
		t.Helper()
		for _, row := range types {
			at := strings.Index(body, row.title)
			if at < 0 {
				t.Fatalf("%s 완료 카드 없음 %s", label, row.title)
			}
			chunk := sliceAround(body, at, 200, 2500)
			if !strings.Contains(chunk, "board-done-badge") || !strings.Contains(chunk, "board-done-text") || !strings.Contains(chunk, "board-done-head") {
				t.Fatalf("%s 완료 머리줄 클래스 없음 %s %s", label, row.title, chunk)
			}
			if !strings.Contains(chunk, titleClass) {
				t.Fatalf("%s 제목 클래스 변경됨 %s", label, row.title)
			}
			if bgHint != "" && !strings.Contains(chunk, bgHint) {
				t.Fatalf("%s 배경 없음 %s %s", label, row.title, chunk)
			}
		}
		key := `data-title="[작업]예정143"`
		planAt := strings.Index(body, key)
		if planAt < 0 {
			key = `data-title="예정143"`
			planAt = strings.Index(body, key)
		}
		if planAt < 0 {
			t.Fatalf("%s 예정 카드 없음", label)
		}
		planChunk := body[planAt:]
		if nxt := strings.Index(planChunk[len(key):], `data-title="`); nxt >= 0 {
			planChunk = planChunk[:len(key)+nxt]
		}
		if strings.Contains(planChunk, "board-done-badge") || strings.Contains(planChunk, "board-done-head") {
			t.Fatalf("%s 예정 카드에 완료 머리줄", label)
		}
	}
	check("주", week, `text-ui font-bold leading-snug text-ink`, "board-b-")
	check("일", dayHTML, `truncate font-semibold`, "background-color:")
	pwAt := strings.Index(week, "완료PW143")
	pwChunk := sliceAround(week, pwAt, 200, 2500)
	if !strings.Contains(pwChunk, "board-b-work") {
		t.Fatalf("작업 수행 배경 없음 %s", pwChunk)
	}
}
