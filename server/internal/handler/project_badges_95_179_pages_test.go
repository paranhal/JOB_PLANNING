package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// seedBadgeData 사업에 사이트 둘(홍성고·천안고)과 배지가 있는 하부 업무 둘(GB·FW)을 만들고, 홍성고는 둘 다, 천안고는 GB 만 완료로 둔다.
func seedBadgeData(t *testing.T, db *sql.DB, projectID, org string) []string {
	t.Helper()
	repo := repository.NewProjectRepo(db)
	ph := &model.ProjectPhase{ProjectID: projectID, OrgID: org, Name: "전처리 업무", Status: model.PhaseActive}
	if err := repo.CreatePhase(ph); err != nil {
		t.Fatal(err)
	}
	var sids, cids []string
	for i, nm := range []string{"홍성고등학교", "천안고등학교"} {
		cid, sid := fmt.Sprintf("C179P%s%d", projectID, i), fmt.Sprintf("S179P%s%d", projectID, i)
		if _, err := db.Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,?)`, cid, nm, nm, org); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO project_sites(id, project_id, site_name, region, status, org_id, customer_id) VALUES (?,?,?,?, 'planned',?,?)`, sid, projectID, nm, "충남", org, cid); err != nil {
			t.Fatal(err)
		}
		sids, cids = append(sids, sid), append(cids, cid)
	}
	if _, err := repo.AddPhaseSites(projectID, ph.PhaseID, sids, false, "", "", "u1"); err != nil {
		t.Fatal(err)
	}
	pk := map[string]string{}
	for _, b := range []struct{ name, badge string }{{"GBIC 수량 산정", "사전GB"}, {"방화벽/L2 SW 수량 산정", "사전FW"}} {
		id, err := repo.CreatePackage(ph.PhaseID, b.name, model.TrackModeSite)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdatePackageMeta(projectID, id, repository.PackageMeta{Badge: b.badge}, repository.DateChangeCtx{}); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.AddPackageSites(projectID, id, sids, false, "", "", "u1"); err != nil {
			t.Fatal(err)
		}
		pk[b.badge] = id
	}
	for _, d := range []struct{ badge, site, day string }{{"사전GB", sids[0], "2026-10-20"}, {"사전FW", sids[0], "2026-10-24"}, {"사전GB", sids[1], "2026-10-21"}} {
		if _, err := db.Exec(`UPDATE package_sites SET status='done', updated_at=? WHERE package_id=? AND project_site_id=?`, d.day+" 10:00:00", pk[d.badge], d.site); err != nil {
			t.Fatal(err)
		}
	}
	return cids
}

// 주간 업무 보고서 사업 카드에 「완료 배지」 요약이 읽기 전용으로 보인다.
func Test95_179D_BadgesOnWeeklyReport(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "w179d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	seedWeekly32(t, db)
	seedBadgeData(t, db, "WP-P2", "O03")
	e := echo.New()
	e.Renderer = NewRenderer()
	mountWeekly28(e, New(db))
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	week := parseWeekMonday("", time.Now()).AddDate(0, 0, -7).Format("2006-01-02")

	add := httptest.NewRecorder()
	areq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows/from-projects", strings.NewReader(url.Values{"project_id": {"WP-P2"}, "org": {"O03"}, "week": {week}}.Encode()))
	areq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	areq.AddCookie(ck)
	e.ServeHTTP(add, areq)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/company-weekly?week="+week+"&org=O03", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "data-weekly-badges") {
		t.Fatalf("주간 보고서 %d: 배지 요약이 없다", rec.Code)
	}
	if !strings.Contains(body, `data-badge-count="사전GB"`) || !strings.Contains(body, "사전GB 2곳") || !strings.Contains(body, "사전FW 1곳") {
		t.Fatal("배지별 사이트 수가 맞지 않다(GB 2곳, FW 1곳)")
	}
	if strings.Index(body, "사전GB 2곳") > strings.Index(body, "사전FW 1곳") {
		t.Fatal("많은 순으로 보여야 한다")
	}
}

// 업무 회의 「사업별」 보기의 사업 머리에 같은 요약이 보인다.
func Test95_179D_BadgesOnMeetingBoard(t *testing.T) {
	e, _, u := newMeeting970Server(t)
	seedBadgeData(t, meeting970DB, "WP-A", model.OrgIDLibrary)
	rec := weeklyGet(t, e, "/meeting/board?from=2026-10-21&to=2026-10-21&view=project", jwtCookieUserClaims(t, u.oa01))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "data-meeting-badges") || !strings.Contains(body, "사전GB 2곳") || !strings.Contains(body, "사전FW 1곳") {
		t.Fatalf("업무 회의 사업 머리에 배지 요약이 없다 (%d)", rec.Code)
	}
}

// 작업 조치 화면의 「대상 사이트」 목록에서 사이트 이름 옆에 그 사이트의 완료 배지가 쌓여 보인다(홍성고: GB → FW).
func Test95_179D_BadgesOnTaskActionPage(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	cids := seedBadgeData(t, repo.DB(), "WP-O01", model.OrgIDLibrary)
	id := pwCreateTask(t, e, repo, "배지확인")
	if rec := doForm(t, e, "/workboard/tasks/"+id+"/action/project", url.Values{"project_id": {"WP-O01"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("사업 연결 %d", rec.Code)
	}
	if err := repo.ReplaceTaskSites(id, cids); err != nil {
		t.Fatal(err)
	}
	body := doGet(t, e, "/workboard/tasks/"+id+"/action").Body.String()
	row := func(name string) string {
		i := strings.Index(body, "<p>"+name)
		if i < 0 {
			t.Fatalf("대상 사이트에 %s 가 없다", name)
		}
		return body[i : i+strings.Index(body[i:], "</p>")]
	}
	hs, ca := row("홍성고등학교"), row("천안고등학교")
	if strings.Count(hs, "data-site-badge") != 2 || strings.Index(hs, "사전GB") > strings.Index(hs, "사전FW") || strings.Count(ca, "data-site-badge") != 1 || strings.Contains(ca, "사전FW") {
		t.Fatalf("배지가 맞지 않다:\n홍성=%s\n천안=%s", hs, ca)
	}
}

// seed181 수량만 하부 업무(환경조사 120식 중 30식)가 있는 사업 WP-P2 의 주간 보고서 줄을 만들고, 서버·쿠키·주를 돌려준다.
func seed181(t *testing.T) (*echo.Echo, *http.Cookie, string) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "w181.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	seedWeekly32(t, db)
	repo := repository.NewProjectRepo(db)
	ph := &model.ProjectPhase{ProjectID: "WP-P2", OrgID: "O03", Name: "전처리 업무", Status: model.PhaseActive}
	if err := repo.CreatePhase(ph); err != nil {
		t.Fatal(err)
	}
	pkg, err := repo.CreatePackage(ph.PhaseID, "환경조사", model.TrackModePhase)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetPackageQtyTarget("WP-P2", pkg, "식", 120); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RecordPackageQty("WP-P2", pkg, "", 30, "u1", ""); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	mountWeekly28(e, New(db))
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	week := parseWeekMonday("", time.Now()).AddDate(0, 0, -7).Format("2006-01-02")
	areq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows/from-projects", strings.NewReader(url.Values{"project_id": {"WP-P2"}, "org": {"O03"}, "week": {week}}.Encode()))
	areq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	areq.AddCookie(ck)
	e.ServeHTTP(httptest.NewRecorder(), areq)
	return e, ck, week
}

// 95-181 주간 보고서: 수행 구조 진행 줄을 「금주에 넣기」·「차주에 넣기」 버튼의 데이터로 준다. 편집 모드에서만 보이고, 구조가 없는 사업에는 없다.
func Test95_181_WeeklyInsertButtons(t *testing.T) {
	e, ck, week := seed181(t)
	get := func(q string) string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/stats/company-weekly?week="+week+"&org=O03"+q, nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("주간 보고서 %d", rec.Code)
		}
		return rec.Body.String()
	}
	edit := get("&edit=1")
	if !strings.Contains(edit, "data-insert-target=\"done_") || !strings.Contains(edit, "금주에 넣기") || !strings.Contains(edit, "차주에 넣기") {
		t.Fatal("편집 모드에 넣기 버튼이 없다")
	}
	if !strings.Contains(edit, "- 환경조사 120식 중 30식 완료(25%)") || !strings.Contains(edit, "- 환경조사 남은 90식 진행") {
		t.Fatal("버튼에 담긴 줄이 맞지 않다")
	}
	// 95-190: edit 값이 없으면 작성 중 보고서는 기본 편집이므로 읽기 모드는 edit=0 으로 명시한다.
	if strings.Contains(get("&edit=0"), `data-insert-target="`) {
		t.Fatal("읽기 모드에는 넣기 버튼이 없어야 한다")
	}
}
