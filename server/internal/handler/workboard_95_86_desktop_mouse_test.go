package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func jwtCookieProfileOK(t *testing.T, u *model.User) *http.Cookie {
	t.Helper()
	secret := []byte("cs-system-jwt-secret-2026")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": u.UserID, "username": u.Username, "role": u.Role,
		"name": u.FullName, "org_id": u.OrgID, "is_readonly": u.IsReadOnly,
		"profile_ok": true, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func Test95_86_BarMinutesAndMinHeight(t *testing.T) {
	zero := boardItem{PlaceDur: 0, HasTime: true, StartMin: 9 * 60, EndMin: 9*60 + 30, StartHHMM: "09:00", EndHHMM: "09:30"}
	if boardBarMinutes(zero) != 30 {
		t.Fatalf("PlaceDur=0 이면 시각으로 30분이어야 함 %d", boardBarMinutes(zero))
	}
	if _, h := boardDayBarHeight(9*60, 9*60+30, 30); h < boardDayBarMinShort {
		t.Fatalf("30분 바 최소 높이 %d", h)
	}
	if _, h := boardDayBarHeight(10*60, 11*60, 60); h < boardDayBarMinLong {
		t.Fatalf("60분 바 최소 높이 %d", h)
	}
}

func Test95_86_DayBarRenderSnapshot(t *testing.T) {
	e, db, _, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d86snap.db"))
	oa, err := repository.NewUserRepo(db).GetByUsername("oa01")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().AddDate(0, 0, 2).Format(dateLayout)
	mustCreateBoardTask(t, repo, "짧은바30", day, "09:00", "09:30", model.OrgIDLibrary, oa.FullName, oa.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "긴바60", day, "11:00", "12:00", model.OrgIDLibrary, oa.FullName, oa.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "제로분", day, "13:00", "13:30", model.OrgIDLibrary, oa.FullName, oa.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	if _, err := db.Exec(`UPDATE work_tasks SET duration_min=0 WHERE title=?`, "제로분"); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, oa)
	body := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0&mine=0", ck).Body.String()
	assertBarBox := func(title, wantDur string, minH int, short bool) {
		t.Helper()
		chunk := barHTMLChunk(body, title)
		if chunk == "" {
			t.Fatalf("%s 바 없음", title)
		}
		if !strings.Contains(chunk, `data-duration="`+wantDur+`"`) {
			t.Fatalf("%s duration 없음 %s", title, chunk[:min(400, len(chunk))])
		}
		if short && !strings.Contains(chunk, "board-bar-short") {
			t.Fatalf("%s 짧은 바 아님", title)
		}
		if !short && strings.Contains(chunk, "board-bar-short") {
			t.Fatalf("%s 가 짧은 바로 표시", title)
		}
		if !strings.Contains(chunk, title) || !strings.Contains(chunk, "조치") {
			t.Fatalf("%s 제목·버튼이 바 안에 없음", title)
		}
		h := attrInt(chunk, "data-h")
		if h < minH {
			t.Fatalf("%s 높이 %d < %d", title, h, minH)
		}
	}
	assertBarBox("짧은바30", "30", boardDayBarMinShort, true)
	assertBarBox("긴바60", "60", boardDayBarMinLong, false)
	assertBarBox("제로분", "30", boardDayBarMinShort, true)
	css := string(mustRead(t, filepath.Join(findTemplateRoot(t), "..", "css", "input.css")))
	if !strings.Contains(css, "width: 24px") || !strings.Contains(css, "height: 4px") {
		t.Fatal("손잡이 막대 없음")
	}
	if strings.Contains(css, ".board-day-ev:not(.board-bar-short)") {
		t.Fatal("긴 바 overflow:visible 상시 규칙이 남음")
	}
}

func Test95_86_DesktopMouse(t *testing.T) {
	e, db, _, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d86mouse.db"))
	oa, err := repository.NewUserRepo(db).GetByUsername("oa01")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().AddDate(0, 0, 3).Format(dateLayout)
	kim, err := repository.NewUserRepo(db).GetByUsername("kimhw")
	if err != nil || kim == nil {
		t.Fatal("kimhw")
	}
	mustCreateBoardTask(t, repo, "짧은바30", day, "09:00", "09:30", model.OrgIDLibrary, oa.FullName, oa.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "긴바60", day, "14:00", "15:00", model.OrgIDLibrary, oa.FullName, oa.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "완료고정", day, "16:00", "17:00", model.OrgIDLibrary, oa.FullName, oa.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "열동료", day, "11:00", "12:00", model.OrgIDLibrary, kim.FullName, kim.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	ck := jwtCookieProfileOK(t, oa)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	probe := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0&mine=0", ck)
	if probe.Code != http.StatusOK {
		t.Fatalf("Go GET board %d loc=%s", probe.Code, probe.Header().Get("Location"))
	}
	if !strings.Contains(probe.Body.String(), `data-title="짧은바30"`) {
		t.Fatalf("Go HTML에 바 없음")
	}
	if !strings.Contains(probe.Body.String(), `defer src="/static/js/board_schedule.js`) {
		t.Fatal("렌더 HTML에 board_schedule.js defer 없음")
	}
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	cmd := exec.Command("node", "95_86_desktop_mouse_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+ck.Value,
		"WORK_DATE="+day,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "is not found") {
			t.Skip(s)
		}
		t.Fatalf("데스크톱 마우스 실패: %v\n%s", err, out)
	}
}

func barHTMLChunk(body, title string) string {
	key := `data-title="` + title + `"`
	i := strings.Index(body, key)
	if i < 0 {
		return ""
	}
	lo := i - 400
	if lo < 0 {
		lo = 0
	}
	hi := i + 1800
	if hi > len(body) {
		hi = len(body)
	}
	return body[lo:hi]
}

func attrInt(chunk, name string) int {
	re := regexp.MustCompile(name + `="([0-9]+)"`)
	m := re.FindStringSubmatch(chunk)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
