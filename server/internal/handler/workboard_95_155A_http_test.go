package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"database/sql"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func unplacedPanelHTML(body string) string {
	i := strings.Index(body, `id="unplanned"`)
	if i < 0 {
		return ""
	}
	j := strings.Index(body[i:], `id="unplaced-del-mask"`)
	if j < 0 {
		return body[i:]
	}
	return body[i : i+j]
}

func seed155AFamily(t *testing.T, repo *repository.WBRepo, u *model.User) (parent *model.WorkTask, kids []*model.WorkTask) {
	t.Helper()
	parent = seed155Task(t, repo, u, title155Dup, model.WBWorkAdmin, "", "")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET created_at=? WHERE task_id=?`,
		time.Date(2026, 1, 1, 8, 0, 0, 0, time.Local), parent.TaskID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		ch := seed155Task(t, repo, u, title155Dup, model.WBWorkAdmin, "", "")
		if _, err := repo.DB().Exec(`UPDATE work_tasks SET parent_task_id=?, created_at=? WHERE task_id=?`,
			parent.TaskID, time.Date(2026, 1, 1, 9, 0, i, 0, time.Local), ch.TaskID); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetTask(ch.TaskID)
		if err != nil || got == nil {
			t.Fatal(err)
		}
		kids = append(kids, got)
	}
	p, err := repo.GetTask(parent.TaskID)
	if err != nil || p == nil {
		t.Fatal(err)
	}
	return p, kids
}

func Test95_155A_T1_ChildDeleteReasonAndPanel(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	parent, kids := seed155AFamily(t, h.Workboard.repo, si)
	ctx := echoCtxOrg(t, si)
	ctx.Set("permissions", model.DefaultPermissions(si.Role))

	if why := h.Workboard.listDeleteBlockReason(ctx, parent); why != "" {
		t.Fatalf("부모 사유 %q", why)
	}
	n, err := h.Workboard.repo.CountSubtasks(parent.TaskID)
	if err != nil || n != 4 {
		t.Fatalf("부모 하위수 %d %v", n, err)
	}
	for i, k := range kids {
		if k.ParentTaskID != parent.TaskID {
			t.Fatalf("child%d parent %q", i, k.ParentTaskID)
		}
		if k.RecurrenceRole != "" {
			t.Fatalf("child%d role %q", i, k.RecurrenceRole)
		}
		if cid := h.Workboard.repo.TaskCooperationID(k.TaskID); cid != "" {
			t.Fatalf("child%d coop %q", i, cid)
		}
		cn, err := h.Workboard.repo.CountSubtasks(k.TaskID)
		if err != nil || cn != 0 {
			t.Fatalf("child%d subN %d %v", i, cn, err)
		}
		if why := h.Workboard.listDeleteBlockReason(ctx, k); why != "" {
			t.Fatalf("하위가 막힘 %d %q", i+1, why)
		}
	}

	panel := unplacedPanelHTML(board155(t, e, si, "kanban").Body.String())
	if panel == "" {
		t.Fatal("미배치 패널 없음")
	}
	if !strings.Contains(panel, `data-unplaced-del="task:`+parent.TaskID+`"`) {
		t.Fatal("부모 삭제 버튼 없음")
	}
	if strings.Contains(panel, `disabled title="`+msgBulkSubtasks) || strings.Contains(panel, msgBulkSubtasks) {
		t.Fatal("부모가 하위 때문에 막혀 있다")
	}
	if !strings.Contains(panel, "같은 업무 5건") {
		t.Fatal("부모+하위가 한 묶음이 아님")
	}
	if !strings.Contains(panel, "중복 4건 정리") {
		t.Fatal("하위 4건 정리 버튼 없음")
	}
	for _, k := range kids {
		key := `data-unplaced-del="task:` + k.TaskID + `"`
		if !strings.Contains(panel, key) {
			t.Fatalf("하위 삭제 버튼 없음 %s", k.TaskID)
		}
	}
	day := unplacedPanelHTML(board155(t, e, si, "day").Body.String())
	if !strings.Contains(day, "중복 4건 정리") {
		t.Fatal("일정표 패널이 칸반과 다름")
	}
}

func Test95_155A_T2_ChildSingleDeleteAudit(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	parent, kids := seed155AFamily(t, h.Workboard.repo, si)
	accessPath := filepath.Join(t.TempDir(), "access155A.db")
	if err := auditlog.Init(accessPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auditlog.Close() })

	rec := postBulk155(t, e, "/workboard/tasks/bulk-delete", si, []string{"task:" + kids[0].TaskID}, unplacedDeleteAuditReason)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}
	if g, _ := h.Workboard.repo.GetTask(kids[0].TaskID); g != nil {
		t.Fatal("하위가 DB에 남음")
	}
	if g, _ := h.Workboard.repo.GetTask(parent.TaskID); g == nil {
		t.Fatal("부모가 지워짐")
	}
	adb, err := sql.Open("sqlite", accessPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { adb.Close() })
	var reason string
	row := adb.QueryRow(`SELECT reason FROM access_logs WHERE target_id=? AND action=? ORDER BY seq DESC LIMIT 1`, kids[0].TaskID, auditlog.ActionDelete)
	if err := row.Scan(&reason); err != nil || reason != unplacedDeleteAuditReason {
		t.Fatalf("audit %q %v", reason, err)
	}
}

func Test95_155A_T3_BulkChildrenKeepParent(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	parent, kids := seed155AFamily(t, h.Workboard.repo, si)
	keys := []string{"task:" + parent.TaskID}
	for _, k := range kids {
		keys = append(keys, "task:"+k.TaskID)
	}
	prev := postBulk155(t, e, "/workboard/tasks/bulk-delete/preview", si, keys, unplacedDeleteAuditReason)
	if strings.Contains(prev.Body.String(), msgBulkSubtasks) {
		t.Fatalf("부모가 skip 됨 %s", prev.Body.String())
	}
	for _, k := range kids {
		if !strings.Contains(prev.Body.String(), k.TaskID) {
			t.Fatalf("미리보기에 하위 없음 %s", k.TaskID)
		}
	}
	del := postBulk155(t, e, "/workboard/tasks/bulk-delete", si, keys, unplacedDeleteAuditReason)
	if del.Code != http.StatusOK {
		t.Fatal(del.Body.String())
	}
	if g, _ := h.Workboard.repo.GetTask(parent.TaskID); g != nil {
		t.Fatal("부모가 남음")
	}
	for _, k := range kids {
		if g, _ := h.Workboard.repo.GetTask(k.TaskID); g != nil {
			t.Fatalf("하위 남음 %s", k.TaskID)
		}
	}
}

func Test95_155A_T4_DupCleanupChildren(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	parent, kids := seed155AFamily(t, h.Workboard.repo, si)
	panel := unplacedPanelHTML(board155(t, e, si, "kanban").Body.String())
	if !strings.Contains(panel, "중복 4건 정리") {
		t.Fatal(panel)
	}
	var keys []string
	for _, k := range kids {
		keys = append(keys, "task:"+k.TaskID)
	}
	del := postBulk155(t, e, "/workboard/tasks/bulk-delete", si, keys, unplacedDeleteAuditReason)
	if del.Code != http.StatusOK {
		t.Fatal(del.Body.String())
	}
	if g, _ := h.Workboard.repo.GetTask(parent.TaskID); g == nil {
		t.Fatal("부모가 지워짐")
	}
	after := unplacedPanelHTML(board155(t, e, si, "kanban").Body.String())
	if strings.Contains(after, "중복 ") {
		t.Fatal("정리 후에도 중복 버튼")
	}
	if !strings.Contains(after, `data-unplaced-del="task:`+parent.TaskID+`"`) {
		t.Fatal("부모 카드 없음")
	}
}

func Test95_155A_T5_CoopAndOtherOrg(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	repo := h.Workboard.repo
	coop := seed155Task(t, repo, si, "협조삭제불가", model.WBWorkAdmin, "", "")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET cooperation_id=? WHERE task_id=?`, "COOP-155A", coop.TaskID); err != nil {
		t.Fatal(err)
	}
	other := seed155Task(t, repo, si, "타조직삭제불가", model.WBWorkAdmin, "", "")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET org_id=? WHERE task_id=?`, "O01", other.TaskID); err != nil {
		t.Fatal(err)
	}
	prev := postBulk155(t, e, "/workboard/tasks/bulk-delete/preview", si, []string{
		"task:" + coop.TaskID, "task:" + other.TaskID,
	}, unplacedDeleteAuditReason)
	if !strings.Contains(prev.Body.String(), msgBulkCoop) {
		t.Fatalf("협조 사유 없음 %s", prev.Body.String())
	}
	if !strings.Contains(prev.Body.String(), msgBulkOtherOrg) {
		t.Fatalf("타 조직 사유 없음 %s", prev.Body.String())
	}
	panel := unplacedPanelHTML(board155(t, e, si, "kanban").Body.String())
	if !strings.Contains(panel, msgBulkCoop) && !strings.Contains(panel, "협조") {
		t.Fatal("패널에 협조 사유 없음")
	}
}

func Test95_155A_T6_DupSkipHint(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	repo := h.Workboard.repo
	a := seed155Task(t, repo, si, "중복불가묶음", model.WBWorkAdmin, "", "")
	b := seed155Task(t, repo, si, "중복불가묶음", model.WBWorkAdmin, "", "")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET cooperation_id=? WHERE task_id IN (?,?)`, "COOP-155A2", a.TaskID, b.TaskID); err != nil {
		t.Fatal(err)
	}
	panel := unplacedPanelHTML(board155(t, e, si, "kanban").Body.String())
	if strings.Contains(panel, "중복 ") && strings.Contains(panel, "건 정리") {
		t.Fatal("지울 수 없는데 정리 버튼")
	}
	if !strings.Contains(panel, "지울 수 있는 중복이 없습니다") {
		t.Fatal("중복 불가 안내 없음")
	}
	if !strings.Contains(panel, msgBulkCoop) {
		t.Fatal("불가 사유가 안내에 없음")
	}
}

func Test95_155A_T7_Browser(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, h, si, _ := new95_41App(t)
	seed155AFamily(t, h.Workboard.repo, si)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-155A")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_155A_unplaced_delete_browser.mjs")
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

func Test95_155A_T8_JSONPreviewCounts(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	parent, kids := seed155AFamily(t, h.Workboard.repo, si)
	keys := []string{"task:" + parent.TaskID}
	for _, k := range kids {
		keys = append(keys, "task:"+k.TaskID)
	}
	prev := postBulk155(t, e, "/workboard/tasks/bulk-delete/preview", si, keys, unplacedDeleteAuditReason)
	var out struct {
		Deletable []map[string]any `json:"deletable"`
		Skipped   []map[string]any `json:"skipped"`
	}
	if json.Unmarshal(prev.Body.Bytes(), &out) != nil {
		t.Fatal(prev.Body.String())
	}
	if len(out.Deletable) != 5 || len(out.Skipped) != 0 {
		t.Fatalf("deletable=%d skipped=%d %s", len(out.Deletable), len(out.Skipped), prev.Body.String())
	}
}

func Test95_155A_T9_ForceDeleteCompleteParent(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	parent, kids := seed155AFamily(t, h.Workboard.repo, si)
	ids := []string{parent.TaskID}
	for _, k := range kids {
		ids = append(ids, k.TaskID)
	}
	q := "UPDATE work_tasks SET status=? WHERE task_id IN (?" + strings.Repeat(",?", len(ids)-1) + ")"
	args := []any{model.WBTaskComplete}
	for _, id := range ids {
		args = append(args, id)
	}
	if _, err := h.Workboard.repo.DB().Exec(q, args...); err != nil {
		t.Fatal(err)
	}
	ctx := echoCtxOrg(t, si)
	ctx.Set("permissions", model.DefaultPermissions(si.Role))
	if why := h.Workboard.listDeleteBlockReason(ctx, parent); why != "" {
		t.Fatalf("완료 부모 막힘 %q", why)
	}
	panel := unplacedPanelHTML(board155(t, e, si, "kanban").Body.String())
	if strings.Contains(panel, msgBulkSubtasks) {
		t.Fatal("완료 부모가 하위 때문에 막힘")
	}
	del := postBulk155(t, e, "/workboard/tasks/bulk-delete", si, []string{"task:" + parent.TaskID}, unplacedDeleteAuditReason)
	if del.Code != http.StatusOK {
		t.Fatal(del.Body.String())
	}
	if g, _ := h.Workboard.repo.GetTask(parent.TaskID); g != nil {
		t.Fatal("완료 부모가 남음")
	}
	for _, k := range kids {
		if g, _ := h.Workboard.repo.GetTask(k.TaskID); g != nil {
			t.Fatalf("완료 하위 남음 %s", k.TaskID)
		}
	}
}
