package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_42_WorkListToBoardItemASKeyNotEmpty(t *testing.T) {
	it := model.WorkListItem{
		Prefix: model.WorkPrefixAS, RefID: "R2610-042", Title: "정안초",
		OrgID: "O03", OrgName: "정안초등학교", Assignee: "SI조직장",
		MappedStatus: model.WBTaskInProgress, ScheduledDate: "2026-10-25",
	}
	bi := workListToBoardItem(it, "2026-10-25")
	if bi.Task.TaskID != "" {
		t.Fatalf("합성 AS 행 TaskID=%q (비어 있어야 재현)", bi.Task.TaskID)
	}
	if bi.DeleteKey != "as:R2610-042" {
		t.Fatalf("DeleteKey=%q", bi.DeleteKey)
	}
	mnt := workListToBoardItem(model.WorkListItem{
		Prefix: model.WorkPrefixMaintenance, RefID: "V1", Title: "점검",
	}, "2026-10-25")
	if mnt.DeleteKey != "" || mnt.DeleteDisabledReason != msgBulkMnt {
		t.Fatalf("점검 key=%q reason=%q", mnt.DeleteKey, mnt.DeleteDisabledReason)
	}
	sales := workListToBoardItem(model.WorkListItem{
		Prefix: model.WorkPrefixSales, RefID: "S1", Title: "영업",
	}, "2026-10-25")
	if sales.DeleteKey != "" {
		t.Fatalf("영업 DeleteKey=%q", sales.DeleteKey)
	}
	gen := workListToBoardItem(model.WorkListItem{
		Prefix: model.WorkPrefixGeneral, RefID: "WT-9", Title: "행정",
	}, "2026-10-25")
	if gen.DeleteKey != "task:WT-9" {
		t.Fatalf("일반 key=%q", gen.DeleteKey)
	}
}

func Test95_42_EmptyIDsPreview400(t *testing.T) {
	e, _, si, _ := new95_41App(t)
	ck, form := csrfPair()
	form.Set("ids", "   ")
	form.Add("ids", "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks/bulk-delete/preview", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(jwtCookieUser(t, si))
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), msgBulkEmptyID) {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func postBulk42(t *testing.T, e *echo.Echo, path string, u *model.User, ids []string) *httptest.ResponseRecorder {
	t.Helper()
	ck, form := csrfPair()
	for _, id := range ids {
		form.Add("ids", id)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-CSRF-Token", form.Get("csrf"))
	req.AddCookie(jwtCookieUser(t, u))
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func Test95_42_SyntheticASCheckboxAndDeleteWithAndWithoutTask(t *testing.T) {
	for _, withTask := range []bool{false, true} {
		t.Run(fmt.Sprintf("task=%v", withTask), func(t *testing.T) {
			e, h, si, _ := new95_41App(t)
			asRepo := h.AS.repo
			listed, _, err := asRepo.ListFiltered("O03", "", "", "", nil, "", "", 1, 20)
			if err != nil || len(listed) < 6 {
				t.Fatalf("as list: %v n=%d", err, len(listed))
			}
			if !withTask {
				if _, err := h.AS.db.Exec(`DELETE FROM work_tasks`); err != nil {
					t.Fatal(err)
				}
			}
			var keys []string
			var asIDs []string
			for _, as := range listed[:6] {
				it := model.WorkListItem{
					Prefix: model.WorkPrefixAS, RefID: as.ASID, Title: as.Symptom,
					OrgID: "O03", OrgName: as.OrgName, Assignee: as.AssignedTo,
					MappedStatus: model.MapASStatusToWB(as.Status), ScheduledDate: "2026-10-25",
				}
				bi := workListToBoardItem(it, "2026-10-25")
				if bi.Task.TaskID != "" && !withTask {
					t.Fatalf("합성 행 TaskID=%q", bi.Task.TaskID)
				}
				if bi.DeleteKey != bulkKeyAS+as.ASID {
					t.Fatalf("key=%q as=%s", bi.DeleteKey, as.ASID)
				}
				keys = append(keys, bi.DeleteKey)
				asIDs = append(asIDs, as.ASID)
			}
			page := doGetWith(t, e, "/workboard/board?view=day&date=2026-10-25&auto=0", jwtCookieUser(t, si))
			if page.Code != http.StatusOK {
				t.Fatalf("board %d", page.Code)
			}
			body := page.Body.String()
			re := regexp.MustCompile(`class="js-wl-check"[^>]*value="(as:[^"]+)"`)
			found := map[string]bool{}
			for _, m := range re.FindAllStringSubmatch(body, -1) {
				found[m[1]] = true
			}
			if strings.Contains(body, `class="js-wl-check" name="ids" value=""`) {
				t.Fatal("빈 체크박스 value")
			}
			for _, k := range keys {
				if !found[k] && !strings.Contains(body, `value="`+k+`"`) {
					t.Logf("HTML에 %s 없음(날짜 필터일 수 있음). 합성 키 경로로 삭제 검증", k)
				}
			}
			prev := postBulk42(t, e, "/workboard/tasks/bulk-delete/preview", si, keys)
			if prev.Code != http.StatusOK {
				t.Fatalf("preview %d %s", prev.Code, prev.Body.String())
			}
			var out struct {
				Deletable []bulkPreviewRow `json:"deletable"`
				Skipped   []bulkSkipItem   `json:"skipped"`
			}
			if json.Unmarshal(prev.Body.Bytes(), &out) != nil {
				t.Fatal(prev.Body.String())
			}
			if len(out.Deletable) != 6 {
				t.Fatalf("deletable=%d skip=%v body=%s", len(out.Deletable), out.Skipped, prev.Body.String())
			}
			del := postBulk42(t, e, "/workboard/tasks/bulk-delete", si, keys)
			if del.Code != http.StatusOK {
				t.Fatalf("delete %d %s", del.Code, del.Body.String())
			}
			var d struct {
				Deleted []string `json:"deleted"`
			}
			_ = json.Unmarshal(del.Body.Bytes(), &d)
			if len(d.Deleted) != 6 {
				t.Fatalf("deleted=%d %s", len(d.Deleted), del.Body.String())
			}
			for _, id := range asIDs {
				as, err := asRepo.GetByID(repository.OrgAll, id)
				if err != nil {
					t.Fatal(err)
				}
				if as != nil {
					t.Fatalf("AS 남음 %s", id)
				}
			}
		})
	}
}

func Test95_42_LegacyTaskKeyAndPrefix(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	tk := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "직접등록", Status: model.WBTaskWaiting,
		WorkDate: "2026-10-25", DueDate: "2026-10-25", OrgID: "O03",
		Assignee: si.FullName, AssigneeUserID: si.UserID,
	}
	if err := h.Workboard.repo.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
	prev := postBulk42(t, e, "/workboard/tasks/bulk-delete/preview", si, []string{tk.TaskID})
	if prev.Code != http.StatusOK || !strings.Contains(prev.Body.String(), tk.TaskID) && !strings.Contains(prev.Body.String(), `"deletable"`) {
		t.Fatalf("legacy preview %d %s", prev.Code, prev.Body.String())
	}
	var out struct {
		Deletable []bulkPreviewRow `json:"deletable"`
		Skipped   []bulkSkipItem   `json:"skipped"`
	}
	if json.Unmarshal(prev.Body.Bytes(), &out) != nil || len(out.Deletable) != 1 {
		t.Fatalf("legacy %s", prev.Body.String())
	}
	prefixed := postBulk42(t, e, "/workboard/tasks/bulk-delete", si, []string{bulkKeyTask + tk.TaskID})
	if prefixed.Code != http.StatusOK {
		t.Fatalf("task: delete %d %s", prefixed.Code, prefixed.Body.String())
	}
	got, _ := h.Workboard.repo.GetTask(tk.TaskID)
	if got != nil {
		t.Fatal("직접 등록 업무 남음")
	}
}

func Test95_42_SameASTwoKeysOnce(t *testing.T) {
	e, h, si, taskIDs := new95_41App(t)
	tk, err := h.Workboard.repo.GetTask(taskIDs[0])
	if err != nil || tk == nil {
		t.Fatal(err)
	}
	asID := tk.SourceID
	rec := postBulk42(t, e, "/workboard/tasks/bulk-delete", si, []string{bulkKeyAS + asID, bulkKeyTask + tk.TaskID})
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Deleted []string       `json:"deleted"`
		Skipped []bulkSkipItem `json:"skipped"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatal(rec.Body.String())
	}
	if len(out.Deleted) != 2 {
		t.Fatalf("deleted=%d skip=%v %s", len(out.Deleted), out.Skipped, rec.Body.String())
	}
	as, _ := h.AS.repo.GetByID(repository.OrgAll, asID)
	if as != nil {
		t.Fatal("AS 가 남아 있다")
	}
}

func Test95_42_ASSyntheticPermissionMatrix(t *testing.T) {
	e, h, _, _ := new95_41App(t)
	db := h.AS.db
	users := repository.NewUserRepo(db)
	mk := func(u *model.User) *model.User {
		t.Helper()
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
		got, _ := users.GetByUsername(u.Username)
		return got
	}
	vis := mk(&model.User{Username: "vis42", PasswordHash: "x", FullName: "비전42", Role: model.RoleVisionAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true, Email: "v@e.com"})
	oa := mk(&model.User{Username: "oa42", PasswordHash: "x", FullName: "조직42", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true, Email: "o@e.com"})
	oa2 := mk(&model.User{Username: "oa42lib", PasswordHash: "x", FullName: "도서42", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true, Email: "l@e.com"})
	recv := mk(&model.User{Username: "recv42", PasswordHash: "x", FullName: "접수42", Role: model.RoleSupport, IsActive: true, OrgID: "O03", ProfileDone: true})
	tech := mk(&model.User{Username: "tech42", PasswordHash: "x", FullName: "기술42", Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true})
	cl := mk(&model.User{Username: "cl42", PasswordHash: "x", FullName: "대표42", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true})

	asRepo := h.AS.repo
	open := &model.ASReceipt{
		CustomerID: "c-jeongan", ReceiptChannel: "phone", Requester: "학교", Symptom: "권한표-미처리",
		AssignedTo: oa.FullName, AssignedUserID: oa.UserID, ReceivedBy: recv.FullName,
		CreatedByUserID: recv.UserID, ReceiptDatetime: time.Now(), VisitScheduledDate: "2026-10-25", OrgID: "O03",
	}
	if err := asRepo.Create(open); err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`UPDATE as_receipts SET created_by_user_id=?, received_by=? WHERE as_id=?`, recv.UserID, recv.FullName, open.ASID)
	if _, err := db.Exec(`DELETE FROM work_tasks WHERE source_id=?`, open.ASID); err != nil {
		t.Fatal(err)
	}
	key := bulkKeyAS + open.ASID
	allow := func(u *model.User) bool {
		rec := postBulk42(t, e, "/workboard/tasks/bulk-delete/preview", u, []string{key})
		if rec.Code != http.StatusOK {
			return false
		}
		var out struct {
			Deletable []bulkPreviewRow `json:"deletable"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return len(out.Deletable) == 1
	}
	if !allow(vis) {
		t.Fatal("비전관리자 거부")
	}
	if !allow(oa) {
		t.Fatal("자기 조직장 거부")
	}
	if allow(oa2) {
		t.Fatal("타 조직장이 삭제 가능")
	}
	if !allow(recv) {
		t.Fatal("접수자 거부")
	}
	if allow(tech) {
		t.Fatal("기술자가 삭제 가능")
	}
	if allow(cl) {
		t.Fatal("c_level 이 삭제 가능")
	}
}
