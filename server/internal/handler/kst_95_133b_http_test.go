package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// KST 00:30 에 만든 업무의 「업무 등록일」은 그날(2026-11-06)이어야 한다. UTC 기본값일 때는 전날로 보였다.
func Test95_133B_TaskShowRegisteredDateIsKSTDay(t *testing.T) {
	prev := time.Local
	defer func() { time.Local = prev }()
	time.Local = time.UTC // 서버 TZ 가 UTC 여도 한국 날짜
	restore := model.SetNowForTest(func() time.Time { return time.Date(2026, 11, 5, 15, 30, 0, 0, time.UTC) })
	defer restore()

	e, repo := newGTDServer(t)
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "새벽등록", DueDate: "2026-11-06",
		WorkDate: "2026-11-06", StartTime: "09:00", EndTime: "09:30", Status: model.WBTaskWaiting}
	if err := repo.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	show := doGet(t, e, "/workboard/tasks/"+task.TaskID+"?back=/admin-work")
	if show.Code != http.StatusOK {
		t.Fatalf("status=%d", show.Code)
	}
	body := show.Body.String()
	i := strings.Index(body, "업무 등록일")
	if i < 0 {
		t.Fatal("업무 등록일 칸이 없다")
	}
	seg := body[i:min(len(body), i+260)]
	if !strings.Contains(seg, "2026-11-06") || strings.Contains(seg, "2026-11-05") {
		t.Fatalf("업무 등록일이 한국 날짜가 아니다: %s", seg)
	}
}

// 팀장 변경 이력의 시각은 한국 시각(분 단위)으로 저장·표시된다.
func Test95_133B_HeadHistoryShowsKST(t *testing.T) {
	restore := model.SetNowForTest(func() time.Time { return time.Date(2026, 11, 6, 7, 48, 45, 0, time.UTC) })
	defer restore()

	e, users, db := newHead166(t)
	oa := &model.User{Username: "oa133b", PasswordHash: HashPassword("pw"), FullName: "일팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", Email: "a@ex.com", ProfileDone: true}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUserClaims(t, oa)
	page := getAccount(t, e, "/admin/users", ck)
	csrfCk, csrf := csrfFromRec(t, page)
	vals := url.Values{"head_user_id": {oa.UserID}, "csrf": {csrf}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/approval-settings/head", strings.NewReader(vals.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	req.AddCookie(csrfCk)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("지정 %d %s", rec.Code, rec.Body.String())
	}
	var raw string
	if err := db.QueryRow(`SELECT strftime('%Y-%m-%d %H:%M:%S', acted_at) FROM org_head_history WHERE org_id='O01' ORDER BY rowid DESC LIMIT 1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != "2026-11-06 16:48:45" {
		t.Fatalf("acted_at=%s (KST 16:48:45 기대)", raw)
	}
	body := getAccount(t, e, "/admin/approval-settings", ck).Body.String()
	if !strings.Contains(body, "2026-11-06 16:48 ·") {
		t.Fatal("팀장 변경 이력이 분 단위 한국 시각으로 안 나온다")
	}
	if strings.Contains(body, "16:48:45") || strings.Contains(body, "07:48") {
		t.Fatal("초가 보이거나 UTC 시각이 보인다")
	}
}

// 업무 홈의 기본 날짜(날짜가 없거나 틀릴 때)는 한국의 오늘이다.
func Test95_133C_WorkboardHomeDateIsKSTToday(t *testing.T) {
	prev := time.Local
	defer func() { time.Local = prev }()
	restore := model.SetNowForTest(func() time.Time { return time.Date(2026, 11, 5, 15, 30, 0, 0, time.UTC) })
	defer restore()
	for _, local := range []*time.Location{time.UTC, model.KST(), time.FixedZone("X", -5*3600)} {
		time.Local = local
		if got := workboardHomeDate(""); got != "2026-11-06" {
			t.Fatalf("Local=%s 기본 날짜 %s", local, got)
		}
		if got := workboardHomeDate("2026-11-02"); got != "2026-11-02" {
			t.Fatalf("지정 날짜 %s", got)
		}
	}
}
