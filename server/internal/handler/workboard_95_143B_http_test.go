package handler

import (
	"html"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_143B_MonthDoneChipAndDayOpenHref(t *testing.T) {
	e, db, me, other, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d143b.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 3).Format(dateLayout)
	mustCreateBoardTask(t, repo, "완료143B월", day, "07:00", "08:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkProjectWork)
	mustCreateBoardTask(t, repo, "예정143B월", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkProjectWork)
	mustCreateBoardTask(t, repo, "완료143B일", day, "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "예정143B일", day, "13:00", "14:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "지원본143B", day, "15:00", "16:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	sup := taskByTitle(t, repo, "지원본143B")
	if err := repo.ReplaceSupportMembers(sup.TaskID, []model.WorkTaskMember{{UserID: other.UserID, Assignee: other.FullName, Role: model.WBMemberSupport}}); err != nil {
		t.Fatal(err)
	}

	q := "&auto=0&show=plan,act&mine=0"
	month := doGetWith(t, e, "/workboard/board?view=sched&range=month&date="+day+q, ck).Body.String()
	week := doGetWith(t, e, "/workboard/board?view=sched&range=week&date="+day+q, ck).Body.String()
	dayHTML := doGetWith(t, e, "/workboard/board?view=sched&range=day&date="+day+q, ck).Body.String()

	// T1 월 완료 칩
	mDone := strings.Index(month, `title="완료143B월"`)
	mPlan := strings.Index(month, `title="예정143B월"`)
	if mDone < 0 || mPlan < 0 {
		t.Fatalf("월 칩 없음 done=%d plan=%d", mDone, mPlan)
	}
	mDoneChunk := monthChipChunk(month, mDone)
	mPlanChunk := monthChipChunk(month, mPlan)
	if !strings.Contains(mDoneChunk, "board-done-badge") || !strings.Contains(mDoneChunk, "board-done-text") {
		t.Fatalf("월 완료 칩 클래스 없음 %s", mDoneChunk)
	}
	if strings.Contains(mDoneChunk, ">✓<") || strings.Contains(mDoneChunk, "✓</span>") {
		t.Fatal("월 완료 칩에 ✓")
	}
	if strings.Contains(mPlanChunk, "board-done-badge") || strings.Contains(mPlanChunk, "board-done-text") {
		t.Fatalf("예정 칩에 완료 표시 %s", mPlanChunk)
	}

	// T2 주 보기 95-143과 동일
	wAt := strings.Index(week, "완료143B월")
	if wAt < 0 {
		t.Fatal("주 완료 카드 없음")
	}
	wChunk := sliceAround(week, wAt, 200, 2500)
	if !strings.Contains(wChunk, "board-done-badge") || !strings.Contains(wChunk, "board-done-text") || !strings.Contains(wChunk, "board-done-head") {
		t.Fatalf("주 완료 머리줄 없음 %s", wChunk)
	}
	if !strings.Contains(wChunk, "board-b-work") {
		t.Fatalf("주 작업 배경 없음 %s", wChunk)
	}
	if !strings.Contains(wChunk, `text-ui font-bold leading-snug text-ink`) {
		t.Fatal("주 제목 클래스 변경")
	}

	// T3 일 보기 제목 링크
	doneTask := taskByTitle(t, repo, "완료143B일")
	planTask := taskByTitle(t, repo, "예정143B일")
	assertDayTitleLink(t, dayHTML, "완료143B일", "/workboard/tasks/"+doneTask.TaskID)
	assertDayTitleLink(t, dayHTML, "예정143B일", "/workboard/tasks/"+planTask.TaskID)
	pwDone := taskByTitle(t, repo, "완료143B월")
	assertDayTitleLink(t, dayHTML, "[작업]완료143B월", projectWorkActionPath(pwDone.TaskID))
	if !strings.Contains(dayHTML, `data-bar-open="1"`) || !strings.Contains(dayHTML, `@click.stop`) {
		t.Fatal("일 보기 제목 링크 stop 없음")
	}
	if !strings.Contains(dayHTML, `@dblclick.stop="openWork($event.currentTarget)"`) {
		t.Fatal("일 보기 더블클릭 열기 없음")
	}
	assertDayTitleLink(t, dayHTML, "지원본143B", "/workboard/tasks/"+sup.TaskID)

	users := repository.NewUserRepo(db)
	obs := &model.User{Username: "obs143b", PasswordHash: HashPassword("password10"), FullName: "옵저버143B", Role: model.RoleObserver, IsActive: true, IsReadOnly: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(obs); err != nil {
		t.Fatal(err)
	}
	gotObs, _ := users.GetByUsername("obs143b")
	mustCreateBoardTask(t, repo, "열람143B일", day, "17:00", "18:00", model.OrgIDLibrary, gotObs.FullName, gotObs.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	roHTML := doGetWith(t, e, "/workboard/board?view=sched&range=day&date="+day+q, jwtCookieUser(t, gotObs)).Body.String()
	if rec := doGetWith(t, e, "/workboard/board?view=sched&range=day&date="+day+q, jwtCookieUser(t, gotObs)); rec.Code != http.StatusOK {
		t.Fatalf("열람 일 보기 %d", rec.Code)
	}
	roTask := taskByTitle(t, repo, "열람143B일")
	assertDayTitleLink(t, roHTML, "열람143B일", "/workboard/tasks/"+roTask.TaskID)
}

func monthChipChunk(body string, at int) string {
	if at < 0 {
		return ""
	}
	chunk := body[at:]
	if nxt := strings.Index(chunk[1:], `data-month-chip`); nxt >= 0 {
		chunk = chunk[:1+nxt]
	}
	return chunk
}

func assertDayTitleLink(t *testing.T, body, title, wantPath string) {
	t.Helper()
	key := `data-title="` + title + `"`
	at := strings.Index(body, key)
	if at < 0 {
		t.Fatalf("일 보기 제목 없음 %s", title)
	}
	chunk := body[at:]
	if nxt := strings.Index(chunk[len(key):], `data-title="`); nxt >= 0 {
		chunk = chunk[:len(key)+nxt]
	}
	re := regexp.MustCompile(`<a href="([^"]+)"[^>]*data-bar-open="1"`)
	m := re.FindStringSubmatch(chunk)
	if m == nil {
		t.Fatalf("제목 링크 없음 %s %s", title, chunk)
	}
	got := hrefPathOnly(html.UnescapeString(m[1]))
	if got != wantPath {
		t.Fatalf("제목 주소 %s got=%s want=%s", title, got, wantPath)
	}
}
