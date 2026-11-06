package handler

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_140_WeeklyLinkMntConnectsAndHidesAssignee(t *testing.T) {
	root := findTemplateRoot(t)
	html := string(mustRead(t, filepath.Join(root, "stats", "_weekly_unassigned.html")))
	js := string(mustRead(t, filepath.Join(findStaticRoot(t), "js", "weekly-link.js")))
	src := string(mustRead(t, filepath.Join(findHandlerRoot(t), "stats_company_weekly_link.go")))
	plan := string(mustRead(t, filepath.Join(root, "maintenance", "plan_show.html")))

	if strings.Contains(html, "정기점검에서 사업 지정") || strings.Contains(html, "data-mnt-goto") {
		t.Fatal("T8 135A 차단 안내가 남아 있다")
	}
	if strings.Contains(js, "이번 연결에서 제외됩니다") || strings.Contains(js, "if (isMnt(it)) return") {
		t.Fatal("T2 점검을 POST에서 빼면 안 된다")
	}
	if !strings.Contains(js, "asgBox.hidden = onlyMnt") {
		t.Fatal("T1 정기점검만이면 담당자를 숨겨야 한다")
	}
	if strings.Contains(js, "submit.hidden = onlyMnt") {
		t.Fatal("T1 연결 버튼을 숨기면 안 된다")
	}
	if strings.Contains(js, "projBox.hidden = onlyMnt") {
		t.Fatal("T1 사업 선택을 숨기면 안 된다")
	}
	if !strings.Contains(js, "담당자 변경은 일반 업무·AS에만 적용됩니다") {
		t.Fatal("T2 혼합 담당자 안내 없음")
	}
	if !strings.Contains(src, "SetVisitProject") {
		t.Fatal("T4 SetVisitProject 호출 없음")
	}
	if strings.Contains(src, "점검 일정에서 사업 확인") {
		t.Fatal("T8 서버가 점검을 건너뛴다")
	}
	if !strings.Contains(plan, "사업 없음 · 연결") || !strings.Contains(plan, "focusEditProject") {
		t.Fatal("T6 방문 카드·목록 사업 없음 연결 없음")
	}
	if !strings.Contains(plan, "openEdit($event.currentTarget, true)") {
		t.Fatal("T6 사업 칸 포커스 없음")
	}
}

func Test95_140_WeeklyMntIgnoresAssigneeAndKeepsFields(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw140.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	seedWeekly32(t, db)

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeekly28(e, h)
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	rec := postWeeklyLink(t, e, url.Values{
		"item": {"maintenance_visit:V32"}, "project_id": {"WP-P1"},
		"org": {"O03"}, "primary_user_id": {userIDByName(t, db, "si-tech")},
	}, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(loc, "연결했습니다") {
		t.Fatalf("loc %s", loc)
	}
	var pid, asg, date, cust string
	var done int
	if err := db.QueryRow(`SELECT COALESCE(project_id,''), COALESCE(assignee,''), visit_date, customer_id, COALESCE(completed,0)
		FROM maintenance_visits WHERE visit_id='V32'`).Scan(&pid, &asg, &date, &cust, &done); err != nil {
		t.Fatal(err)
	}
	if pid != "WP-P1" {
		t.Fatalf("project %s", pid)
	}
	if asg != "김기술" || date != "2026-08-12" || cust != "C32" || done != 1 {
		t.Fatalf("T3/T5 방문 필드 asg=%s date=%s cust=%s done=%d", asg, date, cust, done)
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatal("95-36 담당 필수로 거절되면 안 된다")
	}
}

func Test95_140_MixedAssigneeAppliesOnlyToAS(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw140mix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	seedWeekly32(t, db)
	other := &model.User{Username: "si-other", PasswordHash: HashPassword("password10"), FullName: "이기술",
		Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true}
	if err := repository.NewUserRepo(db).Create(other); err != nil {
		t.Fatal(err)
	}
	techID := userIDByName(t, db, "si-tech")
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, assigned_to, assigned_user_id, org_id, symptom, created_at, updated_at)
		VALUES ('AS-MIX','AS-MIX','C32','2026-08-12 10:00:00','in_progress','김기술',?,'O03','게이트',datetime('now'),datetime('now'))`, techID); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeekly28(e, h)
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	rec := postWeeklyLink(t, e, url.Values{
		"item": {"maintenance_visit:V32", "as_receipt:AS-MIX"},
		"project_id": {"WP-P2"}, "org": {"O03"}, "primary_user_id": {other.UserID},
	}, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var vasg, vpid string
	if err := db.QueryRow(`SELECT COALESCE(assignee,''), COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='V32'`).Scan(&vasg, &vpid); err != nil {
		t.Fatal(err)
	}
	if vpid != "WP-P2" || vasg != "김기술" {
		t.Fatalf("T2 방문 asg=%s pid=%s", vasg, vpid)
	}
	n, uid := asAssignee(t, db, "AS-MIX")
	if n != "이기술" || uid != other.UserID {
		t.Fatalf("T2 AS 담당 %s %s", n, uid)
	}
	if asProjectID(t, db, "AS-MIX") != "WP-P2" {
		t.Fatal("T2 AS 사업")
	}
}
