package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func new95_41App(t *testing.T) (*echo.Echo, *Handler, *model.User, []string) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "wb41.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("adminadmin")); err != nil {
		t.Fatal(err)
	}
	si := &model.User{
		Username: "si_admin", PasswordHash: HashPassword("password10"),
		FullName: "SI조직장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03",
		ProfileDone: true, Email: "si@example.com", Mobile: "010-0000-0000",
	}
	if err := users.Create(si); err != nil {
		t.Fatal(err)
	}
	got, err := users.GetByUsername("si_admin")
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-jeongan','정안초등학교','정안초등학교',1,'O03')`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	e.GET("/csrf", CSRFJSON)
	e.GET("/login", h.Auth.LoginPage)
	e.POST("/login", h.Auth.Login)
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/board", h.Workboard.Board)
	g.POST("/workboard/tasks/bulk-delete/preview", h.Workboard.BulkDeletePreview)
	g.POST("/workboard/tasks/bulk-delete", h.Workboard.BulkDelete)

	asRepo := repository.NewASRepo(db)
	wb := h.Workboard.repo
	gid := repository.NewReceiptGroupID()
	at := time.Date(2026, 10, 25, 9, 0, 0, 0, time.Local)
	var taskIDs []string
	for i := 0; i < 6; i++ {
		as := &model.ASReceipt{
			CustomerID: "c-jeongan", ReceiptChannel: "phone", Requester: "학교",
			Symptom: fmt.Sprintf("정안초 증상 %d", i+1), AssignedTo: got.FullName, AssignedUserID: got.UserID,
			ReceivedBy: got.FullName, ReceiptDatetime: at, VisitScheduledDate: "2026-10-25",
			OrgID: "O03", ReceiptGroupID: gid,
		}
		if err := asRepo.Create(as); err != nil {
			t.Fatal(err)
		}
		if err := wb.SyncASDailyTask(as); err != nil {
			t.Fatal(err)
		}
		tk, err := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
		if err != nil || tk == nil {
			t.Fatalf("task: %v", err)
		}
		if _, err := db.Exec(`UPDATE work_tasks SET org_id=? WHERE task_id=?`, "O03", tk.TaskID); err != nil {
			t.Fatal(err)
		}
		taskIDs = append(taskIDs, tk.TaskID)
	}
	return e, h, got, taskIDs
}

func extractWorkListCSRF(body string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`name="csrf-token" content="([^"]+)"`),
		regexp.MustCompile(`workListSelect\("([a-f0-9]+)"\)`),
		regexp.MustCompile(`workListSelect\(&#34;([a-f0-9]+)&#34;\)`),
	} {
		m := re.FindStringSubmatch(body)
		if len(m) >= 2 && strings.TrimSpace(m[1]) != "" {
			return m[1]
		}
	}
	return ""
}

func Test95_41_BoardListSetsCSRFCookieMatchingToken(t *testing.T) {
	e, _, si, _ := new95_41App(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/workboard/board?view=day&date=2026-10-25&auto=0", nil)
	req.AddCookie(jwtCookieUser(t, si))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("board %d", rec.Code)
	}
	tok := extractWorkListCSRF(rec.Body.String())
	if tok == "" {
		t.Fatal("HTML 토큰 없음")
	}
	var ck *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == csrfCookieName {
			ck = c
		}
	}
	if ck == nil || ck.Value == "" {
		t.Fatalf("Set-Cookie jp_csrf 없음 %v", rec.Result().Header.Values("Set-Cookie"))
	}
	if ck.Value != tok {
		t.Fatalf("쿠키=%s HTML=%s", ck.Value, tok)
	}
	if ck.HttpOnly {
		t.Fatal("jp_csrf 는 JS가 읽어야 해서 HttpOnly 이면 안 된다")
	}
	if ck.Path != "/" || ck.SameSite != http.SameSiteLaxMode {
		t.Fatalf("path/samesite %s %v", ck.Path, ck.SameSite)
	}
	if ck.Secure {
		t.Fatal("HTTP 테스트에서 Secure 가 켜져 있다")
	}
}

func Test95_41_PreviewCSRFMismatch403JSON(t *testing.T) {
	e, _, si, ids := new95_41App(t)
	form := url.Values{"csrf": {"wrongtoken01234567"}}
	for _, id := range ids {
		form.Add("ids", id)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks/bulk-delete/preview", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-CSRF-Token", "wrongtoken01234567")
	req.AddCookie(jwtCookieUser(t, si))
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "goodtoken0123456789", Path: "/"})
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if json.Unmarshal(rec.Body.Bytes(), &out) != nil || out["error"] == "" {
		t.Fatalf("json=%s", rec.Body.String())
	}
	if !strings.Contains(out["error"], "새로고침") {
		t.Fatalf("안내 없음: %s", out["error"])
	}
}

func Test95_41_CookieJarLoginPreviewDelete(t *testing.T) {
	e, h, _, ids := new95_41App(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	client := &http.Client{Jar: jar}

	login, err := client.PostForm(srv.URL+"/login", url.Values{
		"username": {"si_admin"}, "password": {"password10"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, login.Body)
	login.Body.Close()

	list, err := client.Get(srv.URL + "/workboard/board?view=day&date=2026-10-25&auto=0")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(list.Body)
	list.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if list.StatusCode != http.StatusOK {
		t.Fatalf("list %d %s", list.StatusCode, body)
	}
	tok := extractWorkListCSRF(string(body))
	if tok == "" {
		t.Fatal("토큰 없음")
	}
	u, _ := url.Parse(srv.URL)
	var csrfCk *http.Cookie
	for _, ck := range jar.Cookies(u) {
		if ck.Name == csrfCookieName {
			csrfCk = ck
		}
	}
	if csrfCk == nil || csrfCk.Value != tok {
		t.Fatalf("jar csrf=%v html=%s", csrfCk, tok)
	}

	form := url.Values{"csrf": {tok}}
	for _, id := range ids {
		form.Add("ids", id)
	}
	preq, err := http.NewRequest(http.MethodPost, srv.URL+"/workboard/tasks/bulk-delete/preview", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	preq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	preq.Header.Set("Accept", "application/json")
	preq.Header.Set("X-CSRF-Token", tok)
	presp, err := client.Do(preq)
	if err != nil {
		t.Fatal(err)
	}
	pbody, _ := io.ReadAll(presp.Body)
	presp.Body.Close()
	if presp.StatusCode != http.StatusOK {
		t.Fatalf("preview %d %s", presp.StatusCode, pbody)
	}
	var prev struct {
		Deletable []bulkPreviewRow `json:"deletable"`
		Skipped   []bulkSkipItem   `json:"skipped"`
		Error     string           `json:"error"`
	}
	if json.Unmarshal(pbody, &prev) != nil {
		t.Fatalf("preview json %s", pbody)
	}
	if prev.Error != "" || len(prev.Deletable) != 6 {
		t.Fatalf("deletable=%d skip=%d err=%s body=%s", len(prev.Deletable), len(prev.Skipped), prev.Error, pbody)
	}

	var asIDs []string
	for _, id := range ids {
		tk, err := h.Workboard.repo.GetTask(id)
		if err != nil || tk == nil {
			t.Fatalf("삭제 전 업무 %s: %v", id, err)
		}
		asIDs = append(asIDs, tk.SourceID)
	}

	dreq, err := http.NewRequest(http.MethodPost, srv.URL+"/workboard/tasks/bulk-delete", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	dreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	dreq.Header.Set("Accept", "application/json")
	dreq.Header.Set("X-CSRF-Token", tok)
	dresp, err := client.Do(dreq)
	if err != nil {
		t.Fatal(err)
	}
	dbody, _ := io.ReadAll(dresp.Body)
	dresp.Body.Close()
	if dresp.StatusCode != http.StatusOK {
		t.Fatalf("delete %d %s", dresp.StatusCode, dbody)
	}
	var del struct {
		Deleted []string       `json:"deleted"`
		Skipped []bulkSkipItem `json:"skipped"`
	}
	if json.Unmarshal(dbody, &del) != nil {
		t.Fatalf("delete json %s", dbody)
	}
	if len(del.Deleted) != 6 {
		t.Fatalf("deleted=%d skip=%v body=%s", len(del.Deleted), del.Skipped, dbody)
	}
	for _, id := range ids {
		tk, err := h.Workboard.repo.GetTask(id)
		if err != nil {
			t.Fatal(err)
		}
		if tk != nil {
			t.Fatalf("업무 남음 %s", id)
		}
	}
	for _, asID := range asIDs {
		as, err := h.AS.repo.GetByID(repository.OrgAll, asID)
		if err != nil {
			t.Fatal(err)
		}
		if as != nil {
			t.Fatalf("AS 남음 %s", asID)
		}
	}
}

func Test95_41_JSPreviewErrorNotZeroModal(t *testing.T) {
	js := filepath.Join(testWebRoot(t), "static", "js", "work_list_select.js")
	src, err := os.ReadFile(js)
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, "credentials: 'same-origin'") && !strings.Contains(s, `credentials: "same-origin"`) {
		t.Fatal("fetch credentials 없음")
	}
	if !strings.Contains(s, "d.error") && !strings.Contains(s, "!res.ok") {
		t.Fatal("오류 JSON 을 보지 않는다")
	}
	cmd := exec.Command("node", "work_list_select_test.js")
	cmd.Dir = filepath.Join(testWebRoot(t), "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js 테스트 실패: %v\n%s", err, out)
	}
}

func Test95_41_CSRFJSONIssuesCookie(t *testing.T) {
	e, _, _, _ := new95_41App(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/csrf", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if json.Unmarshal(rec.Body.Bytes(), &out) != nil || len(out["csrf"]) < 16 {
		t.Fatalf("%s", rec.Body.String())
	}
	found := false
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == csrfCookieName && ck.Value == out["csrf"] && !ck.HttpOnly {
			found = true
		}
	}
	if !found {
		t.Fatalf("Set-Cookie 불일치 %v body=%s", rec.Result().Header.Values("Set-Cookie"), rec.Body.String())
	}
}
