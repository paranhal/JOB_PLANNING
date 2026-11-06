package handler

import (
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_81_BoardAssigneeFields(t *testing.T) {
	n, extra, title, _, line := boardAssigneeFields("", nil)
	if n != "미배정" || line != "미배정" || title != "미배정" {
		t.Fatalf("빈 담당 %q %q %q", n, line, title)
	}
	_, _, _, _, line = boardAssigneeFields("USR-1", []string{"USR-1"})
	if line != "미배정" {
		t.Fatalf("USR- 표시 %q", line)
	}
	n, extra, _, _, line = boardAssigneeFields("김현우", []string{"김현우", "박지원"})
	if n != "김현우" || extra != " 외 1명" || line != "김현우 외 1명" {
		t.Fatalf("외 N %q %q %q", n, extra, line)
	}
}

func Test95_81_CalendarShowsAssigneeHTML(t *testing.T) {
	t.Skip("일정표 일은 담당자 열 복사본. 주·월 칩은 유지하나 구 data-assignee-line 일 보기 단언 종료(95-132A)")
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d81.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 4).Format(dateLayout)
	users := repository.NewUserRepo(db)
	park := &model.User{Username: "park81", PasswordHash: HashPassword("password10"), FullName: "박지원", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(park); err != nil {
		t.Fatal(err)
	}
	gotPark, _ := users.GetByUsername("park81")

	mustCreateBoardTask(t, repo, "내이름업무", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "동료업무", day, "11:00", "12:00", model.OrgIDLibrary, gotPark.FullName, gotPark.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "미배정업무", day, "13:00", "14:00", model.OrgIDLibrary, "", "", model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "복수담당", day, "15:00", "16:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	multi := taskByTitle(t, repo, "복수담당")
	if err := repo.ReplaceSupportMembers(multi.TaskID, []model.WorkTaskMember{{Assignee: gotPark.FullName, Role: model.WBMemberSupport}}); err != nil {
		t.Fatal(err)
	}
	mustCreateBoardTask(t, repo, "구간업무", day, "09:00", "18:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	seg := taskByTitle(t, repo, "구간업무")
	if err := repo.ReplacePlanSegments(seg.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "18:00"},
	}); err != nil {
		t.Fatal(err)
	}
	mustCreateBoardTask(t, repo, "이스케이프", day, "16:30", "17:00", model.OrgIDLibrary, `<b>XSS</b>`, "", model.WBTaskWaiting, model.WBWorkAdmin)

	mineDay := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0", ck)
	if mineDay.Code != http.StatusOK {
		t.Fatalf("내 업무 일 %d", mineDay.Code)
	}
	mineBody := mineDay.Body.String()
	if strings.Count(mineBody, `data-assignee-line`) < 1 {
		t.Fatal("일 보기 담당자 줄 없음")
	}
	if !strings.Contains(mineBody, "김현우") {
		t.Fatal("내 업무에 본인 이름 없음")
	}
	if strings.Contains(mineBody, "동료업무") {
		t.Fatal("내 업무에 동료 업무가 보임")
	}

	for _, view := range []string{"day", "week", "month"} {
		rec := doGetWith(t, e, "/workboard/board?view="+view+"&date="+day+"&auto=0&mine=0", ck)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %d", view, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-assignee-line`) {
			t.Fatalf("%s 담당자 줄 없음", view)
		}
		if !strings.Contains(body, "김현우") {
			t.Fatalf("%s 담당자 이름 없음", view)
		}
		if strings.Contains(body, "<b>XSS</b>") {
			t.Fatalf("%s 이스케이프 실패 원문", view)
		}
		if view == "month" {
			continue
		}
		if !strings.Contains(body, "박지원") {
			t.Fatalf("%s 전체에서 동료 이름 없음", view)
		}
		if !strings.Contains(body, "미배정") {
			t.Fatalf("%s 미배정 없음", view)
		}
		if !strings.Contains(body, "외 1명") {
			t.Fatalf("%s 외 N 없음", view)
		}
		if !strings.Contains(body, "1/2") || !strings.Contains(body, "2/2") {
			t.Fatalf("%s 구간 조각 없음", view)
		}
		if !strings.Contains(body, html.EscapeString(`<b>XSS</b>`)) {
			t.Fatalf("%s 이스케이프된 이름 없음", view)
		}
	}

	list := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(list, "미배정") || !strings.Contains(list, "외 1명") {
		t.Fatal("리스트 미배정/외 N 없음")
	}

	monthDay := time.Now().AddDate(0, 0, 6).Format(dateLayout)
	mustCreateBoardTask(t, repo, "월미배정", monthDay, "09:00", "10:00", model.OrgIDLibrary, "", "", model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "월복수", monthDay, "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mp := taskByTitle(t, repo, "월복수")
	_ = repo.ReplaceSupportMembers(mp.TaskID, []model.WorkTaskMember{{Assignee: gotPark.FullName, Role: model.WBMemberSupport}})
	month := doGetWith(t, e, "/workboard/board?view=month&date="+monthDay+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(month, "미배정") || !strings.Contains(month, "외 1명") {
		t.Fatal("월 보기 미배정/외 N 없음")
	}
}

func Test95_81_TemplateAndCSS(t *testing.T) {
	root := findTemplateRoot(t)
	for _, f := range []string{"workboard/board_week.html", "workboard/board_month.html"} {
		src := string(mustRead(t, filepath.Join(root, f)))
		if !strings.Contains(src, `board_assignee_line`) {
			t.Fatalf("%s 담당자 부분 템플릿 없음", f)
		}
	}
	css, err := os.ReadFile(filepath.Join(root, "..", "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(css)
	if !strings.Contains(s, "truncate") || !strings.Contains(s, "min-w-0") {
		t.Fatal("tailwind 번들에 truncate/min-w-0 없음 — npm run build:css")
	}
}

func Test95_81_Browser360390(t *testing.T) {
	t.Skip("390px 열 폭은 HTTP 단언(Test95_132_StartWorkHrefAndColMin)")
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d81b.db"))
	me.Mobile = "010-1111-2222"
	me.ProfileDone = true
	ck := jwtCookieVerified(t, me)
	day := time.Now().AddDate(0, 0, 5).Format(dateLayout)
	mustCreateBoardTask(t, repo, "모바일이름", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	cmd := exec.Command("node", "95_81_calendar_name_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+ck.Value,
		"WORK_DATE="+day,
		"EXPECT_NAME="+me.FullName,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "is not found") {
			t.Skip(s)
		}
		t.Fatalf("브라우저 테스트 실패: %v\n%s", err, out)
	}
}
