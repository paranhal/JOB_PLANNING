package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"database/sql"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const title155Dup = "2027년 충남교육청통합도서관 제안요청서 신구대조표 작성"

func seed155Task(t *testing.T, repo *repository.WBRepo, u *model.User, title, workType, assignee string, assigneeID string) *model.WorkTask {
	t.Helper()
	if assignee == "" {
		assignee = u.FullName
	}
	if assigneeID == "" {
		assigneeID = u.UserID
	}
	tk := &model.WorkTask{
		Title: title, OrgID: u.OrgID, Assignee: assignee, AssigneeUserID: assigneeID,
		Status: model.WBTaskWaiting, WorkType: workType, Priority: model.WBPriorityNormal,
		CustomerID: "c-jeongan", Description: "정안초등학교",
	}
	if err := repo.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
	return tk
}

func board155(t *testing.T, e *echo.Echo, u *model.User, view string) *httptest.ResponseRecorder {
	t.Helper()
	return doGetWith(t, e, "/workboard/board?view="+view+"&mine=0&auto=0", jwtCookieUser(t, u))
}

func postBulk155(t *testing.T, e *echo.Echo, path string, u *model.User, ids []string, reason string) *httptest.ResponseRecorder {
	t.Helper()
	ck, form := csrfPair()
	for _, id := range ids {
		form.Add("ids", id)
	}
	if reason != "" {
		form.Set("reason", reason)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	req.Header.Set("X-CSRF-Token", form.Get("csrf"))
	req.AddCookie(jwtCookieUser(t, u))
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func add155Users(t *testing.T, h *Handler) (tech, ro *model.User) {
	t.Helper()
	tech = &model.User{
		Username: "tech155", PasswordHash: HashPassword("password10"),
		FullName: "기술원155", Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true,
	}
	ro = &model.User{
		Username: "ro155", PasswordHash: HashPassword("password10"),
		FullName: "읽기155", Role: model.RoleTech, IsActive: true, OrgID: "O03",
		ProfileDone: true, IsReadOnly: true,
	}
	if err := h.Workboard.userRepo.Create(tech); err != nil {
		t.Fatal(err)
	}
	if err := h.Workboard.userRepo.Create(ro); err != nil {
		t.Fatal(err)
	}
	got, _ := h.Workboard.userRepo.GetByUsername("tech155")
	roGot, _ := h.Workboard.userRepo.GetByUsername("ro155")
	return got, roGot
}

func Test95_155_T1_UnplacedDeleteButtons(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	repo := h.Workboard.repo
	tech, ro := add155Users(t, h)
	admin := seed155Task(t, repo, si, "행정삭제가능", model.WBWorkAdmin, "", "")
	_ = seed155Task(t, repo, si, "영업삭제불가", model.WBWorkSales, "", "")
	parent := seed155Task(t, repo, si, "상위업무", model.WBWorkAdmin, "", "")
	child := seed155Task(t, repo, si, "하위업무", model.WBWorkAdmin, "", "")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET parent_task_id=? WHERE task_id=?`, parent.TaskID, child.TaskID); err != nil {
		t.Fatal(err)
	}
	child.ParentTaskID = parent.TaskID
	occ := seed155Task(t, repo, si, "반복실행분", model.WBWorkAdmin, "", "")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET recurrence_role=? WHERE task_id=?`, model.RecurrenceRoleOccurrence, occ.TaskID); err != nil {
		t.Fatal(err)
	}
	as := &model.ASReceipt{
		CustomerID: "c-jeongan", ReceiptChannel: "phone", Requester: "학교",
		Symptom: "미배치AS", AssignedTo: si.FullName, AssignedUserID: si.UserID,
		ReceivedBy: si.FullName, ReceiptDatetime: time.Date(2026, 10, 25, 9, 0, 0, 0, time.Local),
		OrgID: "O03",
	}
	if err := h.AS.repo.Create(as); err != nil {
		t.Fatal(err)
	}

	body := board155(t, e, si, "kanban").Body.String()
	if !strings.Contains(body, `data-unplaced-del="task:`+admin.TaskID+`"`) {
		t.Fatal("행정 삭제 버튼 없음")
	}
	if strings.Contains(body, "하위 업무가 있어 삭제할 수 없습니다") {
		t.Fatal("상위가 하위 때문에 막혀 있다")
	}
	if !strings.Contains(body, "실행 작업은 상위 업무에서 일정을 바꾸세요") {
		t.Fatal("실행분 사유 없음")
	}
	if !strings.Contains(body, "영업 활동은 영업 화면에서 관리합니다") {
		t.Fatal("영업 사유 없음")
	}
	if !strings.Contains(body, "AS 화면에서 삭제") {
		t.Fatal("AS 링크 없음")
	}
	if strings.Contains(body, `data-unplaced-del="as:`) {
		t.Fatal("AS 삭제 버튼이 있다")
	}
	day := board155(t, e, si, "day").Body.String()
	if !strings.Contains(day, `id="unplanned"`) || !strings.Contains(day, "선택한 0건 삭제") {
		t.Fatal("일정표가 같은 미배치 패널이 아님")
	}
	roBody := board155(t, e, ro, "kanban").Body.String()
	if strings.Contains(roBody, "data-unplaced-del") || strings.Contains(roBody, "선택한 0건 삭제") {
		t.Fatal("읽기 전용에 삭제 UI")
	}
	_ = tech
}

func Test95_155_T2_SingleDeleteAuditAndSkip(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	repo := h.Workboard.repo
	tech, _ := add155Users(t, h)
	tk := seed155Task(t, repo, si, "단건삭제", model.WBWorkAdmin, "", "")
	accessPath := filepath.Join(t.TempDir(), "access155.db")
	if err := auditlog.Init(accessPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auditlog.Close() })
	rec := postBulk155(t, e, "/workboard/tasks/bulk-delete", si, []string{"task:" + tk.TaskID}, unplacedDeleteAuditReason)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatal(rec.Body.String())
	}
	if fmt.Sprint(out["deleted"]) == "" || !strings.Contains(rec.Body.String(), tk.TaskID) {
		t.Fatalf("deleted=%v", out["deleted"])
	}
	got, _ := repo.GetTask(tk.TaskID)
	if got != nil {
		t.Fatal("DB에 남음")
	}
	adb, err := sql.Open("sqlite", accessPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { adb.Close() })
	row := adb.QueryRow(`SELECT reason FROM access_logs WHERE target_id=? AND action=? ORDER BY seq DESC LIMIT 1`, tk.TaskID, auditlog.ActionDelete)
	var reason string
	if err := row.Scan(&reason); err != nil || reason != unplacedDeleteAuditReason {
		t.Fatalf("audit %q %v", reason, err)
	}
	again := postBulk155(t, e, "/workboard/tasks/bulk-delete", si, []string{"task:" + tk.TaskID}, unplacedDeleteAuditReason)
	if !strings.Contains(again.Body.String(), msgBulkGone) {
		t.Fatalf("이미 삭제됨 없음 %s", again.Body.String())
	}
	other := seed155Task(t, repo, si, "타인건", model.WBWorkAdmin, "", "")
	noperm := postBulk155(t, e, "/workboard/tasks/bulk-delete", tech, []string{"task:" + other.TaskID}, unplacedDeleteAuditReason)
	if !strings.Contains(noperm.Body.String(), msgBulkNoPerm) {
		t.Fatalf("권한 없음 없음 %s", noperm.Body.String())
	}
}

func Test95_155_T3_BulkPreviewPartialAndCap(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	repo := h.Workboard.repo
	a := seed155Task(t, repo, si, "선택A", model.WBWorkAdmin, "", "")
	b := seed155Task(t, repo, si, "선택B", model.WBWorkAdmin, "", "")
	p := seed155Task(t, repo, si, "선택상위", model.WBWorkAdmin, "", "")
	ch := seed155Task(t, repo, si, "선택하위", model.WBWorkAdmin, "", "")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET parent_task_id=? WHERE task_id=?`, p.TaskID, ch.TaskID); err != nil {
		t.Fatal(err)
	}
	prev := postBulk155(t, e, "/workboard/tasks/bulk-delete/preview", si, []string{
		"task:" + a.TaskID, "task:" + b.TaskID, "task:" + p.TaskID,
	}, unplacedDeleteAuditReason)
	if prev.Code != http.StatusOK {
		t.Fatalf("preview %d %s", prev.Code, prev.Body.String())
	}
	if !strings.Contains(prev.Body.String(), a.TaskID) || strings.Contains(prev.Body.String(), msgBulkSubtasks) {
		t.Fatalf("N/M 미리보기 %s", prev.Body.String())
	}
	del := postBulk155(t, e, "/workboard/tasks/bulk-delete", si, []string{
		"task:" + a.TaskID, "task:" + b.TaskID, "task:" + p.TaskID,
	}, unplacedDeleteAuditReason)
	if !strings.Contains(del.Body.String(), a.TaskID) || strings.Contains(del.Body.String(), msgBulkSubtasks) {
		t.Fatalf("일부만 삭제 %s", del.Body.String())
	}
	if g, _ := repo.GetTask(p.TaskID); g != nil {
		t.Fatal("상위가 남음")
	}
	if g, _ := repo.GetTask(ch.TaskID); g != nil {
		t.Fatal("하위가 남음")
	}
	ids := make([]string, 0, 201)
	for i := 0; i < 201; i++ {
		ids = append(ids, fmt.Sprintf("task:x%d", i))
	}
	capRec := postBulk155(t, e, "/workboard/tasks/bulk-delete/preview", si, ids, unplacedDeleteAuditReason)
	if capRec.Code != http.StatusBadRequest || !strings.Contains(capRec.Body.String(), msgBulkTooMany) {
		t.Fatalf("201 %d %s", capRec.Code, capRec.Body.String())
	}
}

func Test95_155_T4_DupCleanup(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	repo := h.Workboard.repo
	tech, _ := add155Users(t, h)
	var ids []string
	for i := 0; i < 5; i++ {
		tk := seed155Task(t, repo, si, title155Dup, model.WBWorkAdmin, "", "")
		ids = append(ids, tk.TaskID)
		if _, err := repo.DB().Exec(`UPDATE work_tasks SET created_at=? WHERE task_id=?`,
			time.Date(2026, 1, 1, 0, 0, i, 0, time.Local), tk.TaskID); err != nil {
			t.Fatal(err)
		}
	}
	other := seed155Task(t, repo, si, title155Dup, model.WBWorkAdmin, tech.FullName, tech.UserID)
	body := board155(t, e, si, "kanban").Body.String()
	if !strings.Contains(body, "같은 업무 5건") {
		t.Fatal("5건 배지 없음")
	}
	if strings.Contains(body, "같은 업무 6건") {
		t.Fatal("담당 다른 건이 묶임")
	}
	if !strings.Contains(body, "중복 4건 정리") {
		t.Fatal("중복 정리 버튼 없음")
	}
	drop := ids[1:]
	var keys []string
	for _, id := range drop {
		keys = append(keys, "task:"+id)
	}
	prev := postBulk155(t, e, "/workboard/tasks/bulk-delete/preview", si, keys, unplacedDeleteAuditReason)
	if !strings.Contains(prev.Body.String(), `"deletable"`) {
		t.Fatal(prev.Body.String())
	}
	del := postBulk155(t, e, "/workboard/tasks/bulk-delete", si, keys, unplacedDeleteAuditReason)
	if del.Code != http.StatusOK {
		t.Fatal(del.Body.String())
	}
	keep, _ := repo.GetTask(ids[0])
	if keep == nil {
		t.Fatal("가장 오래된 건이 지워짐")
	}
	for _, id := range drop {
		if g, _ := repo.GetTask(id); g != nil {
			t.Fatalf("남음 %s", id)
		}
	}
	if g, _ := repo.GetTask(other.TaskID); g == nil {
		t.Fatal("다른 담당이 지워짐")
	}
	parent := seed155Task(t, repo, si, "중복하위부모", model.WBWorkAdmin, "", "")
	ch := seed155Task(t, repo, si, "중복하위부모", model.WBWorkAdmin, "", "")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET parent_task_id=? WHERE task_id=?`, parent.TaskID, ch.TaskID); err != nil {
		t.Fatal(err)
	}
	okPrev := postBulk155(t, e, "/workboard/tasks/bulk-delete/preview", si, []string{"task:" + parent.TaskID}, unplacedDeleteAuditReason)
	if strings.Contains(okPrev.Body.String(), msgBulkSubtasks) {
		t.Fatalf("하위 있는 건이 막힘 %s", okPrev.Body.String())
	}
}

func Test95_155_T5_PermissionRegression(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	repo := h.Workboard.repo
	_, ro := add155Users(t, h)
	done := seed155Task(t, repo, si, "완료건", model.WBWorkAdmin, "", "")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET status=? WHERE task_id=?`, model.WBTaskComplete, done.TaskID); err != nil {
		t.Fatal(err)
	}
	// 조직관리자는 완료 삭제 가능. 기술(읽기)은 읽기전용 메시지.
	roRec := postBulk155(t, e, "/workboard/tasks/bulk-delete/preview", ro, []string{"task:" + done.TaskID}, unplacedDeleteAuditReason)
	if roRec.Code != http.StatusForbidden || !strings.Contains(roRec.Body.String(), msgBulkReadOnly) {
		t.Fatalf("읽기전용 %d %s", roRec.Code, roRec.Body.String())
	}
	tech, _ := h.Workboard.userRepo.GetByUsername("tech155")
	viewReq := httptest.NewRecorder()
	ck, form := csrfPair()
	form.Add("ids", "task:"+done.TaskID)
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks/bulk-delete/preview", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-CSRF-Token", form.Get("csrf"))
	req.AddCookie(jwtCookieUser(t, si))
	req.AddCookie(ck)
	req.AddCookie(&http.Cookie{Name: "view_as_user_id", Value: tech.UserID, Path: "/"})
	e.ServeHTTP(viewReq, req)
	if viewReq.Code != http.StatusForbidden && !strings.Contains(viewReq.Body.String(), msgBulkReadOnly) {
		// viewAs 가 미들웨어에서 꺼지면 조직장이 완료 삭제가 가능하므로, 보드 HTML 로 확인
		body := doGetWith(t, e, "/workboard/board?view=kanban&mine=0&auto=0", jwtCookieUser(t, si), &http.Cookie{Name: "view_as_user_id", Value: tech.UserID, Path: "/"}).Body.String()
		if strings.Contains(body, `data-unplaced-del="task:`+done.TaskID+`"`) && !strings.Contains(body, "disabled") {
			t.Fatalf("viewAs 완료 삭제 활성 %d %s", viewReq.Code, viewReq.Body.String())
		}
	}
	occ := seed155Task(t, repo, si, "실행분T5", model.WBWorkAdmin, "", "")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET recurrence_role=? WHERE task_id=?`, model.RecurrenceRoleOccurrence, occ.TaskID); err != nil {
		t.Fatal(err)
	}
	occPrev := postBulk155(t, e, "/workboard/tasks/bulk-delete/preview", si, []string{"task:" + occ.TaskID}, "")
	if !strings.Contains(occPrev.Body.String(), "실행 작업은 상위 업무에서 일정을 바꾸세요") {
		t.Fatal(occPrev.Body.String())
	}
}

func Test95_155_T6_JSUnplacedDelete(t *testing.T) {
	js := filepath.Join(testWebRoot(t), "static", "js", "unplaced_delete.js")
	src, err := os.ReadFile(js)
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if strings.Contains(s, "confirm(") {
		t.Fatal("confirm() 사용")
	}
	if !strings.Contains(s, "role") && !strings.Contains(s, "unplaced-status") {
		t.Fatal("status 연동 없음")
	}
	cmd := exec.Command("node", "unplaced_delete_test.js")
	cmd.Dir = filepath.Join(testWebRoot(t), "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js: %v\n%s", err, out)
	}
}

func Test95_155_T7_BrowserUnplacedDelete(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, h, si, _ := new95_41App(t)
	repo := h.Workboard.repo
	for i := 0; i < 5; i++ {
		tk := seed155Task(t, repo, si, title155Dup, model.WBWorkAdmin, "", "")
		if _, err := repo.DB().Exec(`UPDATE work_tasks SET created_at=? WHERE task_id=?`,
			time.Date(2026, 1, 1, 0, 0, i, 0, time.Local), tk.TaskID); err != nil {
			t.Fatal(err)
		}
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-155")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_155_unplaced_delete_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieUser(t, si).Value,
		"SCREENSHOT_DIR="+shot,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") || (strings.Contains(s, "playwright") && strings.Contains(s, "install")) {
			t.Skip(s)
		}
		t.Fatalf("browser: %v\n%s", err, s)
	}
}
