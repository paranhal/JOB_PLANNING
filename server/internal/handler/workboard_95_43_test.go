package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_43_PollutedASTaskDeletesByReceiptOrg(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	asRepo := h.AS.repo
	listed, _, err := asRepo.ListFiltered("O03", "", "", "", nil, "", "", 1, 20)
	if err != nil || len(listed) < 6 {
		t.Fatalf("list %v n=%d", err, len(listed))
	}
	asID := listed[0].ASID
	tk, err := h.Workboard.repo.GetTaskBySource(model.WBSourceAS, asID)
	if err != nil || tk == nil {
		t.Fatal(err)
	}
	if _, err := h.AS.db.Exec(`UPDATE work_tasks SET org_id=? WHERE task_id=?`, model.OrgIDLibrary, tk.TaskID); err != nil {
		t.Fatal(err)
	}
	prev := postBulk42(t, e, "/workboard/tasks/bulk-delete/preview", si, []string{bulkKeyAS + asID})
	if prev.Code != http.StatusOK {
		t.Fatalf("preview %d %s", prev.Code, prev.Body.String())
	}
	if strings.Contains(prev.Body.String(), "타 조직") {
		t.Fatalf("오염된 업무 OrgID 로 거부: %s", prev.Body.String())
	}
	var out struct {
		Deletable []bulkPreviewRow `json:"deletable"`
	}
	if json.Unmarshal(prev.Body.Bytes(), &out) != nil || len(out.Deletable) != 1 {
		t.Fatalf("body=%s", prev.Body.String())
	}
	del := postBulk42(t, e, "/workboard/tasks/bulk-delete", si, []string{bulkKeyAS + asID})
	if del.Code != http.StatusOK {
		t.Fatalf("delete %d %s", del.Code, del.Body.String())
	}
	gone, _ := asRepo.GetByID(repository.OrgAll, asID)
	if gone != nil {
		t.Fatal("AS 남음")
	}
}

func Test95_43_OtherOrgNamedReason(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	asRepo := h.AS.repo
	lib := &model.ASReceipt{
		CustomerID: "c-jeongan", ReceiptChannel: "phone", Requester: "도서관", Symptom: "O01건",
		AssignedTo: si.FullName, AssignedUserID: si.UserID, ReceiptDatetime: time.Now(),
		VisitScheduledDate: "2026-10-25", OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(lib); err != nil {
		t.Fatal(err)
	}
	if err := h.Workboard.repo.SyncASDailyTask(lib); err != nil {
		t.Fatal(err)
	}
	prev := postBulk42(t, e, "/workboard/tasks/bulk-delete/preview", si, []string{bulkKeyAS + lib.ASID})
	if prev.Code != http.StatusOK {
		t.Fatalf("%d %s", prev.Code, prev.Body.String())
	}
	if !strings.Contains(prev.Body.String(), "타 조직") || !strings.Contains(prev.Body.String(), "도서관") {
		t.Fatalf("조직명 없음: %s", prev.Body.String())
	}
}

func Test95_43_OrgAdminDeletesOthersCompleteWithProcess(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	users := repository.NewUserRepo(h.AS.db)
	tech := &model.User{Username: "park43", PasswordHash: "x", FullName: "박창훈", Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true}
	if err := users.Create(tech); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByUsername("park43")
	as := &model.ASReceipt{
		CustomerID: "c-jeongan", ReceiptChannel: "phone", Requester: "학교", Symptom: "완료건",
		AssignedTo: got.FullName, AssignedUserID: got.UserID, ReceiptDatetime: time.Now(),
		VisitScheduledDate: "2026-10-25", OrgID: "O03", Status: "completed",
	}
	if err := h.AS.repo.Create(as); err != nil {
		t.Fatal(err)
	}
	if _, err := h.AS.db.Exec(`UPDATE as_receipts SET status='completed' WHERE as_id=?`, as.ASID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.AS.db.Exec(`INSERT INTO as_processes (process_id, as_id, process_datetime, worker, work_content, time_spent)
		VALUES ('P43-1', ?, '2026-10-25 10:00:00', ?, '조치', 30)`, as.ASID, got.FullName); err != nil {
		t.Fatal(err)
	}
	if err := h.Workboard.repo.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	prev := postBulk42(t, e, "/workboard/tasks/bulk-delete/preview", si, []string{bulkKeyAS + as.ASID})
	if prev.Code != http.StatusOK || !strings.Contains(prev.Body.String(), `"deletable"`) {
		t.Fatalf("%d %s", prev.Code, prev.Body.String())
	}
	var out struct {
		Deletable []bulkPreviewRow `json:"deletable"`
		Skipped   []bulkSkipItem   `json:"skipped"`
	}
	if json.Unmarshal(prev.Body.Bytes(), &out) != nil || len(out.Deletable) != 1 {
		t.Fatalf("조직장이 완료·조치 이력을 못 지움: %s", prev.Body.String())
	}
	techPrev := postBulk42(t, e, "/workboard/tasks/bulk-delete/preview", got, []string{bulkKeyAS + as.ASID})
	if strings.Contains(techPrev.Body.String(), `"deletable":[{`) {
		t.Fatalf("담당 tech 가 삭제 가능: %s", techPrev.Body.String())
	}
}

func Test95_43_ASListSelectMarkupAndDelete(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	e.GET("/as", h.AS.List, h.Auth.AuthMiddleware)
	page := doGetWith(t, e, "/as", jwtCookieUser(t, si))
	if page.Code != http.StatusOK {
		t.Fatalf("as list %d", page.Code)
	}
	body := page.Body.String()
	for _, need := range []string{
		`id="as-list-select"`, `js-bulk-select`, `js-wl-all`, `js-wl-check`,
		`workListSelect(`, `선택 삭제`, `/workboard/tasks/bulk-delete`,
	} {
		if !strings.Contains(body, need) {
			t.Fatalf("목록에 %s 없음", need)
		}
	}
	if strings.Contains(body, `class="js-wl-check" name="ids" value=""`) {
		t.Fatal("빈 체크박스")
	}
	listed, _, _ := h.AS.repo.ListFiltered("O03", "", "", "", nil, "", "", 1, 20)
	if len(listed) == 0 {
		t.Fatal("as")
	}
	key := bulkKeyAS + listed[0].ASID
	if !strings.Contains(body, `value="`+key+`"`) {
		t.Fatalf("체크박스 %s 없음", key)
	}
	del := postBulk42(t, e, "/workboard/tasks/bulk-delete", si, []string{key})
	if del.Code != http.StatusOK && del.Code != http.StatusSeeOther {
		t.Fatalf("delete %d %s", del.Code, del.Body.String())
	}
}

func Test95_43_SyncSixSITasksOrgO03(t *testing.T) {
	_, h, _, _ := new95_41App(t)
	listed, _, err := h.AS.repo.ListFiltered("O03", "", "", "", nil, "", "", 1, 20)
	if err != nil || len(listed) < 6 {
		t.Fatalf("%v n=%d", err, len(listed))
	}
	for _, as := range listed[:6] {
		tk, err := h.Workboard.repo.GetTaskBySource(model.WBSourceAS, as.ASID)
		if err != nil || tk == nil {
			t.Fatalf("%s: %v", as.ASID, err)
		}
		if tk.OrgID != "O03" {
			t.Fatalf("%s org=%s", as.ASID, tk.OrgID)
		}
	}
}

func Test95_43_CreateTaskMissingOrgOnASSource(t *testing.T) {
	_, h, _, _ := new95_41App(t)
	err := h.Workboard.repo.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAS, Title: fmt.Sprintf("없음"), Status: model.WBTaskWaiting,
		SourceType: model.WBSourceAS, SourceID: "missing-as",
	})
	if err == nil {
		t.Fatal("출처 AS 인데 org 없이 생성")
	}
}
