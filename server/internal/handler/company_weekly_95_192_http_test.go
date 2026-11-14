package handler

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-192: 진행율 폼은 화면에서 빼고, 팀 전체 금주 추진내역·차주 추진계획을 저장·표시·내보낸다.

func (v env190) saveTeam(t *testing.T, ck *http.Cookie, done, plan string, extra url.Values) *http.Cookie {
	t.Helper()
	form := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "edit": {"1"}, "touch_team": {"1"}, "team_done": {done}, "team_plan": {plan}}
	for k, vs := range extra {
		form[k] = vs
	}
	if r := v.do(t, "POST", "/stats/company-weekly/save", form, ck); r.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", r.Code, r.Body.String())
	}
	return ck
}

func TestHTTP95_192_TeamFieldsSavedAndProgressFormGone(t *testing.T) {
	v := setup190(t, true)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	body := v.do(t, "GET", w190, nil, oa).Body.String()
	if strings.Contains(body, `action="/stats/company-weekly/progress"`) || strings.Contains(body, "진행율 저장") || strings.Contains(body, "정기점검 진행율 (작성자 입력)") {
		t.Fatal("진행율 폼이 화면에 남아 있다")
	}
	if !strings.Contains(body, "data-weekly-team") || !strings.Contains(body, `name="team_done"`) || !strings.Contains(body, `name="team_plan"`) {
		t.Fatal("팀 전체 입력칸이 없다")
	}
	v.saveTeam(t, oa, "팀 금주 1\n팀 금주 2", "팀 차주 1", nil)
	body = v.do(t, "GET", w190, nil, oa).Body.String()
	for _, want := range []string{"팀 금주 1", "팀 금주 2", "팀 차주 1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("저장한 %q 가 편집 화면에 없다", want)
		}
	}
	// 읽기 모드에도 보인다
	ro := v.do(t, "GET", w190+"&edit=0", nil, oa).Body.String()
	if !strings.Contains(ro, "data-weekly-team-read") || !strings.Contains(ro, "팀 금주 2") {
		t.Fatal("읽기 모드에 팀 전체가 없다")
	}
	// 다른 저장 경로(진행율 저장 /progress, 사업 행 저장)를 거쳐도 팀 전체가 사라지지 않는다
	if r := v.do(t, "POST", "/stats/company-weekly/progress", url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "done": {"1"}, "total": {"4"}, "cycle": {"반기"}}, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("진행율 저장 경로(남겨 둠) %d", r.Code)
	}
	v.do(t, "POST", "/stats/company-weekly/save", url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "touch_" + v.key: {"1"}, "done_" + v.key: {"행 금주"}}, oa)
	saved, _ := v.h.Stats.repo.GetCompanyWeeklyReport("O03", "2026-10-26")
	doc := repository.ParseCompanyWeeklyDoc(saved.ContentJSON)
	if len(doc.TeamDone) != 2 || len(doc.TeamPlan) != 1 || doc.Progress == nil || doc.Progress.Done == nil || *doc.Progress.Done != 1 {
		t.Fatalf("다른 저장 경로 뒤 팀 전체/진행율 손실 %+v", doc)
	}
}

func TestHTTP95_192_ConfirmedTeamEditBackToDraft(t *testing.T) {
	v := setup190(t, true)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	v.saveTeam(t, oa, "팀 금주", "팀 차주", url.Values{"touch_" + v.key: {"1"}, "done_" + v.key: {"행 금주"}, "need_" + v.key: {"지원1"}})
	if r := v.do(t, "POST", "/stats/company-weekly/ui-confirm", url.Values{"week": {"2026-10-19"}, "org": {"O03"}}, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("확정 %d", r.Code)
	}
	status := func() string {
		s, _ := v.h.Stats.repo.GetCompanyWeeklyReport("O03", "2026-10-26")
		return s.Status
	}
	if status() != repository.CompanyWeeklyStatusConfirmed {
		t.Fatalf("확정 상태가 아니다 %s", status())
	}
	same := url.Values{"touch_" + v.key: {"1"}, "done_" + v.key: {"행 금주"}, "need_" + v.key: {"지원1"}}
	// 지원 필요사항만 고치면 확정 유지(기존 규칙)
	v.saveTeam(t, oa, "팀 금주", "팀 차주", url.Values{"touch_" + v.key: {"1"}, "done_" + v.key: {"행 금주"}, "need_" + v.key: {"지원2"}})
	if status() != repository.CompanyWeeklyStatusConfirmed {
		t.Fatalf("지원만 바꿨는데 확정이 풀렸다 %s", status())
	}
	// 팀 전체를 고치면 작성 중으로
	v.saveTeam(t, oa, "팀 금주(수정)", "팀 차주", same)
	if status() == repository.CompanyWeeklyStatusConfirmed {
		t.Fatal("팀 전체를 고쳤는데 확정이 유지됐다")
	}
}

func TestHTTP95_192_ExcelAndHWPXTeamFirst(t *testing.T) {
	v := setup190(t, true)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	v.saveTeam(t, oa, "팀 금주 A", "팀 차주 B", url.Values{"touch_" + v.key: {"1"}, "done_" + v.key: {"행 금주"}, "plan_" + v.key: {"행 차주"}})
	x := v.do(t, "GET", "/stats/company-weekly/export?week=2026-10-19&org=O03&scope=team", nil, oa)
	f, err := excelize.OpenReader(bytes.NewReader(x.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 3 {
		t.Fatalf("엑셀 행 %d", len(rows))
	}
	line := strings.Join(rows[1], "|")
	if !strings.Contains(line, "팀 전체") || !strings.Contains(line, "팀 금주 A") || !strings.Contains(line, "팀 차주 B") {
		t.Fatalf("엑셀 첫 데이터 행이 팀 전체가 아니다: %s", line)
	}
	if !strings.Contains(strings.Join(rows[2], "|"), "시험 사업") {
		t.Fatalf("그 다음 행이 사업이 아니다: %v", rows[2])
	}
	h := v.do(t, "GET", "/stats/company-weekly/export-hwpx?week=2026-10-19&org=O03&scope=team", nil, oa)
	zr, err := zip.NewReader(bytes.NewReader(h.Body.Bytes()), int64(h.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var sec string
	for _, zf := range zr.File {
		if zf.Name == "Contents/section0.xml" {
			rc, _ := zf.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			sec = string(b)
		}
	}
	iTeam, iBiz := strings.Index(sec, "▶ 팀 전체"), strings.Index(sec, "시험 사업")
	if iTeam < 0 || iBiz < 0 || iTeam > iBiz || !strings.Contains(sec, "팀 금주 A") || !strings.Contains(sec, "팀 차주 B") {
		t.Fatalf("HWPX 팀 전체 항목/순서 오류 team=%d biz=%d", iTeam, iBiz)
	}
	// 팀 전체만 있고 사업 줄이 없어도 내보낼 수 있다
	v2 := setup190(t, false)
	v2.saveTeam(t, oa, "팀만 금주", "팀만 차주", nil)
	if r := v2.do(t, "GET", "/stats/company-weekly/export-hwpx?week=2026-10-19&org=O03&scope=team", nil, oa); r.Code != 200 {
		t.Fatalf("팀 전체만 있는 HWPX %d %s", r.Code, r.Body.String())
	}
}
