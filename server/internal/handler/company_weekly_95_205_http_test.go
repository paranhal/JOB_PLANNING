package handler

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-205: 연속·반복 업무 1줄 축약(기간 표시) · 같은 문구 중복 제거 · 「## ##」 이중 접두.
// 주(週) 2026-10-19 → 금주 추진내역 10/19~10/25, 차주 추진계획 10/26~11/1.

const w205 = "/stats/company-weekly?week=2026-10-19&org=O03"

func setup205(t *testing.T) env190 {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw205.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ex := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	ex(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES ('O03','O03','SI사업팀','SI',1,3)`)
	ex(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,is_test) VALUES ('WP-A','충남도서관 사업','active','O03','project',0)`)
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,recurrence_role) VALUES ('P1','admin','제안서 작성','open','WP-A','O03','parent')`)
	for i, d := range []string{"2026-10-20", "2026-10-21", "2026-10-22"} {
		ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,parent_task_id,recurrence_role,work_date,complete_date) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			"D"+string(rune('0'+i)), "admin", "제안서 작성", "complete", "WP-A", "O03", "P1", "occurrence", d, d)
	}
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,recurrence_role) VALUES ('P2','admin','신구대조표 작성','open','WP-A','O03','parent'),('P3','admin','신구대조표 작성','open','WP-A','O03','parent')`)
	for i, d := range []string{"2026-10-21", "2026-10-22", "2026-10-23"} {
		ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,parent_task_id,recurrence_role,work_date) VALUES (?,?,?,?,?,?,?,?,?)`,
			"C"+string(rune('0'+i)), "admin", "신구대조표 작성", "open", "WP-A", "O03", "P2", "occurrence", d)
		ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,parent_task_id,recurrence_role,work_date) VALUES (?,?,?,?,?,?,?,?,?)`,
			"E"+string(rune('0'+i)), "admin", "신구대조표 작성", "open", "WP-A", "O03", "P3", "occurrence", d)
	}
	// 기술영업활동(활동 행): 완료 영업 업무 1건
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,work_date,complete_date) VALUES ('SA1','sales','제안 자료 정리','complete','O03','2026-10-21','2026-10-21')`)

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeeklyUI(e, h)
	e.GET("/stats/company-weekly/export-hwpx", h.Stats.ExportWeeklyReportSummary, h.Auth.AuthMiddleware)
	v := env190{e: e, h: h}
	k, err := h.Stats.repo.InsertWeeklyReportRow(repository.WeeklyReportRowInput{OrgID: "O03", DisplayName: "충남도서관", ProjectID: "WP-A", Kind: "project", SortOrder: 1, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	v.key = k
	if _, err := h.Stats.repo.InsertWeeklyReportRow(repository.WeeklyReportRowInput{OrgID: "O03", DisplayName: "##기술영업활동", Kind: "activity", ActivityKind: "tech", SortOrder: 2, Active: true}); err != nil {
		t.Fatal(err)
	}
	return v
}

func (v env190) view205(t *testing.T) weeklyReportView {
	t.Helper()
	wk := time.Date(2026, 10, 19, 0, 0, 0, 0, time.Local)
	view, err := buildWeeklyReportView(v.h.Stats.repo, "O03", wk, true)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func card205(t *testing.T, v weeklyReportView, title string) weeklyReportCard {
	t.Helper()
	for _, c := range v.Cards {
		if c.Title == title {
			return c
		}
	}
	t.Fatalf("카드 %q 없음", title)
	return weeklyReportCard{}
}

func zipSection205(t *testing.T, b []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	for _, zf := range zr.File {
		if zf.Name == "Contents/section0.xml" {
			rc, _ := zf.Open()
			d, _ := io.ReadAll(rc)
			rc.Close()
			return string(d)
		}
	}
	t.Fatal("section0.xml 없음")
	return ""
}

// 화면(뷰모델)·엑셀·HWPX 가 같은 줄을 쓴다. 묶음은 기간 표기, 같은 문구는 1줄.
func TestHTTP95_205_ScreenExcelHWPXSameLines(t *testing.T) {
	v := setup205(t)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	view := v.view205(t)
	proj := card205(t, view, "충남도서관")
	done, plan := weeklyLineTexts(proj.Done), weeklyLineTexts(proj.Plan)
	if len(done) != 1 || done[0] != "제안서 작성(10/20~10/22)" {
		t.Fatalf("금주 %v", done)
	}
	if len(plan) != 1 || plan[0] != "신구대조표 작성 3회(이월)" {
		t.Fatalf("차주(같은 문구 중복 제거) %v", plan)
	}
	// 화면(읽기 모드)
	body := v.do(t, "GET", w205+"&edit=0", nil, oa).Body.String()
	if strings.Count(body, "신구대조표 작성 3회(이월)") != 1 || !strings.Contains(body, "제안서 작성(10/20~10/22)") {
		t.Fatalf("화면 줄 수 오류: 이월 %d", strings.Count(body, "신구대조표 작성 3회(이월)"))
	}
	// 엑셀
	x := v.do(t, "GET", "/stats/company-weekly/export?week=2026-10-19&org=O03&scope=team", nil, oa)
	f, err := excelize.OpenReader(bytes.NewReader(x.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	all := ""
	var names []string
	for _, r := range rows {
		all += strings.Join(r, "|") + "\n"
		for _, c := range r {
			if strings.Contains(c, "기술영업활동") {
				names = append(names, c)
			}
		}
	}
	if strings.Count(all, "신구대조표 작성 3회(이월)") != 1 || !strings.Contains(all, "제안서 작성(10/20~10/22)") {
		t.Fatalf("엑셀 줄 오류:\n%s", all)
	}
	// HWPX
	h := v.do(t, "GET", "/stats/company-weekly/export-hwpx?week=2026-10-19&org=O03&scope=team", nil, oa)
	sec := zipSection205(t, h.Body.Bytes())
	if strings.Count(sec, "신구대조표 작성 3회(이월)") != 1 || !strings.Contains(sec, "제안서 작성(10/20~10/22)") {
		t.Fatalf("HWPX 줄 오류 이월=%d", strings.Count(sec, "신구대조표 작성 3회(이월)"))
	}
	// 「## ##」: 활동 행 이름에 ## 접두가 한 번만
	if strings.Contains(sec, "## ##") || strings.Contains(all, "## ##") {
		t.Fatal("「## ##」 이중 접두가 남았다")
	}
	if !strings.Contains(sec, "## 기술영업활동") {
		t.Fatalf("HWPX 활동 행 이름이 「## 기술영업활동」이 아니다")
	}
	if len(names) != 1 || names[0] != "## 기술영업활동" {
		t.Fatalf("엑셀 활동 행 이름 %v", names)
	}
}

func TestWeeklyActivityName95_205(t *testing.T) {
	for in, want := range map[string]string{
		"##기술영업활동":   "## 기술영업활동",
		"## 기술영업활동":  "## 기술영업활동",
		"기술영업활동":     "## 기술영업활동",
		"  ##  자산관리활동": "## 자산관리활동",
	} {
		if got := weeklyActivityName(in); got != want {
			t.Errorf("%q → %q want %q", in, got, want)
		}
	}
}

// 직접 입력 전환·삭제 뒤 「다시 집계」해도 묶음 줄 동작이 유지된다(묶음 Key 안정성). 직접 입력 줄은 불변.
func TestHTTP95_205_ReaggregateKeepsManualAndHidden(t *testing.T) {
	v := setup205(t)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	save := func(extra url.Values) {
		t.Helper()
		form := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "edit": {"1"}}
		for k, vs := range extra {
			form[k] = vs
		}
		if r := v.do(t, "POST", "/stats/company-weekly/save", form, oa); r.Code != http.StatusSeeOther {
			t.Fatalf("저장 %d %s", r.Code, r.Body.String())
		}
	}
	texts := func() (done, plan []string, c weeklyReportCard) {
		c = card205(t, v.view205(t), "충남도서관")
		return weeklyLineTexts(c.Done), weeklyLineTexts(c.Plan), c
	}
	// 1) 묶음 줄을 고치고 직접 입력 줄 추가 → 직접 입력으로 저장
	save(url.Values{"touch_" + v.key: {"1"}, "done_" + v.key: {"제안서 작성(10/20~10/22) 수정\n직접 쓴 줄"}, "plan_" + v.key: {"신구대조표 작성 3회(이월)"}})
	done, plan, _ := texts()
	if len(done) != 2 || done[0] != "제안서 작성(10/20~10/22) 수정" || done[1] != "직접 쓴 줄" || len(plan) != 1 {
		t.Fatalf("저장 뒤 %v / %v", done, plan)
	}
	// 2) 다시 집계 → 직접 입력(수정된 묶음 줄·직접 줄)은 그대로, 자동 줄은 갱신만
	save(url.Values{"reaggregate": {"1"}})
	done, plan, _ = texts()
	if len(done) != 2 || done[0] != "제안서 작성(10/20~10/22) 수정" || done[1] != "직접 쓴 줄" || len(plan) != 1 || plan[0] != "신구대조표 작성 3회(이월)" {
		t.Fatalf("재집계 뒤 %v / %v", done, plan)
	}
	// 3) 차주 자동 줄을 지운다(숨김) → 재집계해도 다시 나오지 않는다
	save(url.Values{"touch_" + v.key: {"1"}, "done_" + v.key: {"제안서 작성(10/20~10/22) 수정\n직접 쓴 줄"}, "plan_" + v.key: {""}})
	save(url.Values{"reaggregate": {"1"}})
	_, plan, _ = texts()
	if len(plan) != 0 {
		t.Fatalf("지운 묶음 줄이 재집계로 되살아났다 %v", plan)
	}
}

// 팀원(읽기 전용)이 봐도 같은 줄이 보인다.
func TestHTTP95_205_ReadOnlyViewSameLines(t *testing.T) {
	v := setup205(t)
	for _, ck := range []*http.Cookie{jwtCookieOrg(t, model.RoleTech, "O03")} {
		body := v.do(t, "GET", w205, nil, ck).Body.String()
		if !strings.Contains(body, "제안서 작성(10/20~10/22)") {
			t.Fatal("팀원 화면에 묶음 줄이 없다")
		}
	}
	_ = repository.CompanyWeeklyStatusDraft
}
