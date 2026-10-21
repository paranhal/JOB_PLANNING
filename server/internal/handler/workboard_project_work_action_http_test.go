package handler

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

var pwSlot int

func pwCreateTask(t *testing.T, e *echo.Echo, repo *repository.WBRepo, title string) string {
	t.Helper()
	pwSlot++
	startMin := 8*60 + pwSlot*30
	start := fmt.Sprintf("%02d:%02d", startMin/60, startMin%60)
	endMin := startMin + 30
	end := fmt.Sprintf("%02d:%02d", endMin/60, endMin%60)
	rec := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {title}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"},
		"start_time": {start}, "end_time": {end},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("create %s status=%d loc=%s body=%s", title, rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Title == title {
			return tk.TaskID
		}
	}
	t.Fatalf("task %s 없음", title)
	return ""
}

func pwActionForm() url.Values {
	return url.Values{
		"assignee":           {"관리자"},
		"work_place":         {"field"},
		"work_date":          {"2026-10-21"},
		"action_taken":       {"현장 배선 정리 및 점검 완료함"},
		"complete_datetime":  {"2026-10-21T15:00"},
		"result_code":        {model.WorkResultDone},
		"customer_confirmer_custom": {"김선생"},
		"action_short_ok":    {"1"},
	}
}

func TestProjectWorkActionRoutesAuthAndNonWork(t *testing.T) {
	e, repo, users := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "조치대상작업")

	ok := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if ok.Code != http.StatusOK {
		t.Fatalf("GET action %d body=%s", ok.Code, ok.Body.String())
	}
	body := ok.Body.String()
	if !strings.Contains(body, "자재 사용") {
		t.Fatal("자재 사용 없음")
	}
	for _, bad := range []string{"단가", "금액", "unit_price", "정산"} {
		if strings.Contains(body, bad) {
			t.Fatalf("가격 필드 %q", bad)
		}
	}

	cl := httptest.NewRequest(http.MethodGet, "/workboard/tasks/"+id+"/action", nil)
	cl.AddCookie(jwtCookieRole(t, model.RoleCLevel))
	clRec := httptest.NewRecorder()
	e.ServeHTTP(clRec, cl)
	if clRec.Code != http.StatusOK {
		t.Fatalf("C-Level GET %d", clRec.Code)
	}
	if strings.Contains(clRec.Body.String(), `form="as-action-form"`) && strings.Contains(clRec.Body.String(), "저장") &&
		strings.Contains(clRec.Body.String(), `type="submit"`) {
		if strings.Contains(clRec.Body.String(), `CanProcess`) {
			t.Log("readonly 템플릿")
		}
	}
	clPost := doFormUser(t, e, "/workboard/tasks/"+id+"/action", pwActionForm(), jwtCookieRole(t, model.RoleCLevel))
	if clPost.Code != http.StatusForbidden {
		t.Fatalf("C-Level POST %d", clPost.Code)
	}

	roPost := doFormUser(t, e, "/workboard/tasks/"+id+"/action", pwActionForm(), jwtCookieReadOnly(t, "tech"))
	if roPost.Code != http.StatusForbidden {
		t.Fatalf("읽기전용 POST %d", roPost.Code)
	}

	oa03, err := users.GetByUsername("oa03")
	if err != nil || oa03 == nil {
		t.Fatal(err)
	}
	other := httptest.NewRequest(http.MethodGet, "/workboard/tasks/"+id+"/action", nil)
	other.AddCookie(jwtCookieUser(t, oa03))
	othRec := httptest.NewRecorder()
	e.ServeHTTP(othRec, other)
	if othRec.Code != http.StatusNotFound && othRec.Code != http.StatusForbidden {
		t.Fatalf("타 조직 GET %d", othRec.Code)
	}

	admin := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"행정만"}, "work_date": {"2026-10-21"},
		"assignee": {"관리자"}, "start_time": {"11:00"}, "end_time": {"11:30"},
	})
	if admin.Code != http.StatusSeeOther {
		t.Fatalf("admin create %d %s", admin.Code, admin.Body.String())
	}
	var adminID string
	tasks, _ := repo.ListTasks()
	for _, tk := range tasks {
		if tk.Title == "행정만" {
			adminID = tk.TaskID
		}
	}
	if adminID == "" {
		t.Fatal("행정 업무 없음")
	}
	redir := doGet(t, e, "/workboard/tasks/"+adminID+"/action")
	if redir.Code != http.StatusSeeOther && redir.Code != http.StatusNotFound {
		t.Fatalf("비작업 action %d", redir.Code)
	}
	if redir.Code == http.StatusSeeOther && !strings.Contains(redir.Header().Get("Location"), "/workboard/tasks/"+adminID) {
		t.Fatalf("redirect %s", redir.Header().Get("Location"))
	}
}

func TestProjectWorkActionCompletePartialHoldTransferCancel(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "완료작업")

	miss := doForm(t, e, "/workboard/tasks/"+id+"/action", url.Values{
		"assignee": {"관리자"}, "work_place": {"field"}, "work_date": {"2026-10-21"},
		"result_code": {model.WorkResultDone}, "complete_datetime": {"2026-10-21T15:00"},
	})
	if miss.Code != http.StatusSeeOther || !strings.Contains(miss.Header().Get("Location"), "err=content") {
		t.Fatalf("빈 조치 loc=%s code=%d", miss.Header().Get("Location"), miss.Code)
	}

	ok := doForm(t, e, "/workboard/tasks/"+id+"/action", pwActionForm())
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("완료 저장 %d", ok.Code)
	}
	tk, err := repo.GetTask(id)
	if err != nil {
		t.Fatal(err)
	}
	_ = repo.LoadTaskActionFields(tk)
	if tk.Status != model.WBTaskComplete || tk.ResultCode != model.WorkResultDone {
		t.Fatalf("complete status=%s result=%s", tk.Status, tk.ResultCode)
	}
	if tk.CompleteNote == "" || tk.ActionTaken == "" {
		t.Fatal("complete_note/action_taken")
	}
	procs, _ := repo.ListWorkTaskProcesses(tk.OrgID, id)
	if len(procs) != 1 {
		t.Fatalf("이력 %d", len(procs))
	}

	id2 := pwCreateTask(t, e, repo, "일부작업")
	part := pwActionForm()
	part.Set("result_code", model.WorkResultPartial)
	part.Set("followup_note", "자재 부족")
	part.Set("next_date", "2026-10-28")
	part.Set("create_followup", "1")
	if rec := doForm(t, e, "/workboard/tasks/"+id2+"/action", part); rec.Code != http.StatusSeeOther {
		t.Fatalf("일부완료 %d", rec.Code)
	}
	src, _ := repo.GetTask(id2)
	_ = repo.LoadTaskActionFields(src)
	if src.Status != model.StatusPartialComplete || src.ResultCode != model.WorkResultPartial {
		t.Fatalf("일부완료 status=%s result=%s", src.Status, src.ResultCode)
	}
	n, err := repo.CountPartialFollowups(src.OrgID, id2, "2026-10-28")
	if err != nil || n != 1 {
		t.Fatalf("후속 n=%d err=%v", n, err)
	}
	if rec := doForm(t, e, "/workboard/tasks/"+id2+"/action", part); rec.Code != http.StatusSeeOther {
		t.Fatalf("후속 중복 저장 %d", rec.Code)
	}
	n2, _ := repo.CountPartialFollowups(src.OrgID, id2, "2026-10-28")
	if n2 != 1 {
		t.Fatalf("후속 중복 %d", n2)
	}

	id3 := pwCreateTask(t, e, repo, "대기업무")
	hold := doForm(t, e, "/workboard/tasks/"+id3+"/action/hold", url.Values{
		"work_place": {"field"}, "action_taken": {"대기 처리 내용입니다"},
		"hold_next_action": {"action"}, "hold_reason": {"고객 부재"},
	})
	if hold.Code != http.StatusSeeOther {
		t.Fatalf("hold %d", hold.Code)
	}
	h3, _ := repo.GetTask(id3)
	if h3.Status != model.WBTaskHold {
		t.Fatalf("hold status=%s", h3.Status)
	}
	rel := doForm(t, e, "/workboard/tasks/"+id3+"/action/hold/release", url.Values{})
	if rel.Code != http.StatusSeeOther {
		t.Fatalf("release %d", rel.Code)
	}
	h3, _ = repo.GetTask(id3)
	if h3.Status != model.WBTaskInProgress {
		t.Fatalf("release status=%s", h3.Status)
	}

	id4 := pwCreateTask(t, e, repo, "이관업무")
	tr := doForm(t, e, "/workboard/tasks/"+id4+"/action/transfer", url.Values{
		"transfer_to": {"협력사A"}, "transfer_reason": {"현장 이관"},
	})
	if tr.Code != http.StatusSeeOther {
		t.Fatalf("transfer %d", tr.Code)
	}
	h4, _ := repo.GetTask(id4)
	_ = repo.LoadTaskActionFields(h4)
	if h4.Status != model.WBTaskTransfer || h4.TransferTo != "협력사A" {
		t.Fatalf("transfer %+v", h4)
	}

	id5 := pwCreateTask(t, e, repo, "취소업무")
	bad := doForm(t, e, "/workboard/tasks/"+id5+"/action/cancel", url.Values{
		"cancel_date": {"2026-10-21"},
	})
	if bad.Code != http.StatusSeeOther || !strings.Contains(bad.Header().Get("Location"), "err=cancel_reason") {
		t.Fatalf("cancel 사유 loc=%s", bad.Header().Get("Location"))
	}
	cn := doForm(t, e, "/workboard/tasks/"+id5+"/action/cancel", url.Values{
		"cancel_date": {"2026-10-21"}, "cancel_reason": {"발주 취소"},
	})
	if cn.Code != http.StatusSeeOther {
		t.Fatalf("cancel %d", cn.Code)
	}
	h5, _ := repo.GetTask(id5)
	if h5.Status != model.WBTaskCancelled {
		t.Fatalf("cancel status=%s", h5.Status)
	}
}

func TestProjectWorkActionPartsAndCardLink(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "자재작업")
	form := pwActionForm()
	form.Set("result_code", model.WorkResultPartial)
	form.Set("followup_note", "자재 기록")
	form.Set("parts_form", "1")
	form.Set("parts_enabled", "1")
	form.Add("part_name", "케이블")
	form.Add("part_code", "C01")
	form.Add("part_qty", "2")
	form.Add("part_item_id", "")
	if rec := doForm(t, e, "/workboard/tasks/"+id+"/action", form); rec.Code != http.StatusSeeOther {
		t.Fatalf("parts save %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	tk, _ := repo.GetTask(id)
	parts, err := repo.ListWorkTaskParts(tk.OrgID, id)
	if err != nil || len(parts) != 1 || parts[0].PartName != "케이블" || parts[0].Qty != 2 {
		t.Fatalf("parts %+v err=%v", parts, err)
	}

	off := pwActionForm()
	off.Set("result_code", model.WorkResultPartial)
	off.Set("followup_note", "자재 해제")
	off.Set("parts_form", "1")
	if rec := doForm(t, e, "/workboard/tasks/"+id+"/action", off); rec.Code != http.StatusSeeOther {
		t.Fatalf("parts off %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	parts, _ = repo.ListWorkTaskParts(tk.OrgID, id)
	if len(parts) != 0 {
		t.Fatalf("해제 후 %d", len(parts))
	}

	reg := doGet(t, e, "/workboard/register?view=day&date=2026-10-21")
	if !strings.Contains(reg.Body.String(), "/workboard/tasks/"+id+"/action") {
		t.Fatal("일정표 조치 링크 없음")
	}
	show := doGet(t, e, "/workboard/tasks/"+id)
	if !strings.Contains(show.Body.String(), "/workboard/tasks/"+id+"/action") {
		t.Fatal("상세 조치 링크 없음")
	}
	if strings.Contains(show.Body.String(), "complete-remaining") {
		t.Fatal("작업 상세에 complete-remaining")
	}

	adminID := ""
	_ = doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"일반카드"}, "work_date": {"2026-10-21"},
		"assignee": {"관리자"}, "start_time": {"13:00"}, "end_time": {"13:30"},
	})
	tasks, _ := repo.ListTasks()
	for _, tk := range tasks {
		if tk.Title == "일반카드" {
			adminID = tk.TaskID
		}
	}
	reg2 := doGet(t, e, "/workboard/register?view=day&date=2026-10-21")
	if adminID != "" && strings.Contains(reg2.Body.String(), "/workboard/tasks/"+adminID+"/action") {
		t.Fatal("행정 카드가 작업 조치 화면으로 감")
	}
}

func TestProjectWorkActionAttachmentsOrg(t *testing.T) {
	e, repo, users := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "첨부작업")

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("ref_type", model.RefTypeWorkTask)
	_ = w.WriteField("ref_id", id)
	_ = w.WriteField("redirect", "/workboard/tasks/"+id+"/action")
	fw, err := w.CreateFormFile("file", "현장사진.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(fw, "photo")
	_ = w.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/attachments", &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("upload %d %s", rec.Code, rec.Body.String())
	}
	page := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if !strings.Contains(page.Body.String(), "현장사진") {
		t.Fatalf("조치 화면에 첨부 없음 %s", page.Body.String())
	}
	m := regexp.MustCompile(`/attachments/([^"/]+)`).FindStringSubmatch(page.Body.String())
	if len(m) < 2 {
		t.Fatal("첨부 id 없음")
	}
	oa03, _ := users.GetByUsername("oa03")
	dl := httptest.NewRequest(http.MethodGet, "/attachments/"+m[1], nil)
	dl.AddCookie(jwtCookieUser(t, oa03))
	dlRec := httptest.NewRecorder()
	e.ServeHTTP(dlRec, dl)
	if dlRec.Code != http.StatusNotFound && dlRec.Code != http.StatusForbidden {
		t.Fatalf("타조직 다운로드 %d", dlRec.Code)
	}
}
