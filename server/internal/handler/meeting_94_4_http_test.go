package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestMeeting94_4SalesCoopOnMeetingAndExcel(t *testing.T) {
	e, db, u := newMeeting943Server(t)
	day := "2026-10-21"
	yday := "2026-10-20"
	last := "2026-10-14"
	if _, err := db.Exec(`INSERT INTO work_tasks (
		task_id, work_type, title, work_date, due_date, status, assignee, complete_date, source_type, org_id
	) VALUES
		('WT-s-yday','sales','전일 영업',?,?, 'complete','도서관영업',?,'sales_activity',?),
		('WT-s-done','sales','지난주 영업',?,?, 'complete','도서관영업',?,'sales_activity',?),
		('WT-s-plan','sales','이번주 영업',?,?, 'waiting','도서관영업',NULL,'sales_activity',?),
		('WT-s-other','sales','타인 영업',?,?, 'waiting','도서관기술',NULL,'sales_activity',?)`,
		yday, yday, yday, model.OrgIDLibrary,
		last, last, last, model.OrgIDLibrary,
		day, day, model.OrgIDLibrary,
		day, day, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}

	ckSales := jwtCookieUserClaims(t, u.sales)
	daily := weeklyGet(t, e, "/meeting?date="+day, ckSales)
	if daily.Code != http.StatusOK {
		t.Fatalf("daily %d", daily.Code)
	}
	body := daily.Body.String()
	if !strings.Contains(body, "이번주 영업") {
		t.Fatal("일일 예정에 영업 없음")
	}
	if !strings.Contains(body, "전일 영업") {
		t.Fatal("일일 전일 실적에 영업 없음")
	}
	if strings.Contains(body, "타인 영업") {
		t.Fatal("본인 일일에 타인 영업")
	}

	ckOA := jwtCookieUserClaims(t, u.oa01)
	prev := weeklyGet(t, e, "/meeting?date="+day+"&assignee="+url.QueryEscape("도서관영업"), ckOA)
	if prev.Code != http.StatusOK || !strings.Contains(prev.Body.String(), "전일 영업") {
		t.Fatal("팀 일일 전일 실적에 영업 완료 없음")
	}

	coop := repository.NewCoopRepo(db)
	row := &model.Cooperation{TargetOrgID: "O02", Kind: model.CoopAdmin, Title: "회의협조업무", Body: "본문", WantStart: day, WantEnd: day}
	req := model.CoopActor{UserID: u.oa01.UserID, OrgID: u.oa01.OrgID, Role: u.oa01.Role}
	if err := coop.Create(req, row); err != nil {
		t.Fatal(err)
	}
	acc := model.CoopActor{UserID: u.oa02.UserID, OrgID: u.oa02.OrgID, Role: u.oa02.Role}
	if err := coop.Accept(acc, row.CoopID, ""); err != nil {
		t.Fatal(err)
	}
	if err := coop.Assign(acc, row.CoopID, day, day, "지시", "", []string{u.o02tech.UserID}); err != nil {
		t.Fatal(err)
	}

	sent := weeklyGet(t, e, "/meeting/weekly?date=2026-10-19", ckOA)
	if sent.Code != http.StatusOK {
		t.Fatalf("weekly %d", sent.Code)
	}
	if !strings.Contains(sent.Body.String(), "타 조직 지원 요청") || !strings.Contains(sent.Body.String(), "회의협조업무") {
		t.Fatal("요청 팀 주간에 보낸 협조 구역 없음")
	}
	dailySent := weeklyGet(t, e, "/meeting?date="+day, ckOA)
	if dailySent.Code != http.StatusOK || !strings.Contains(dailySent.Body.String(), "타 조직 지원 요청") {
		t.Fatal("일일 팀 자료에 보낸 협조 구역 없음")
	}

	ckT2 := jwtCookieUserClaims(t, u.o02tech)
	me := weeklyGet(t, e, "/meeting/weekly/me?date=2026-10-19", ckT2)
	if me.Code != http.StatusOK {
		t.Fatalf("me %d", me.Code)
	}
	if !strings.Contains(me.Body.String(), "협조") || !strings.Contains(me.Body.String(), "회의협조업무") {
		t.Fatal("배정 담당 내 자료에 협조 배지 없음")
	}
	libTech := weeklyGet(t, e, "/meeting/weekly/me?date=2026-10-19", jwtCookieUserClaims(t, u.tech))
	if libTech.Code != http.StatusOK {
		t.Fatalf("lib tech %d", libTech.Code)
	}
	if strings.Contains(libTech.Body.String(), "회의협조업무") {
		t.Fatal("타 조직 팀원에게 협조 업무가 보임")
	}

	xlsx := weeklyGet(t, e, "/meeting/weekly/me.xlsx?date=2026-10-19", ckSales)
	if xlsx.Code != http.StatusOK {
		t.Fatalf("xlsx %d", xlsx.Code)
	}
	f, err := excelize.OpenReader(xlsx.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(1)
	if sheet == "" {
		sheet = f.GetSheetName(0)
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, r := range rows {
		joined += strings.Join(r, "\t") + "\n"
	}
	if !strings.Contains(joined, "영업") && !strings.Contains(xlsx.Body.String(), "영업") {
		// 시트 이름이 통계일 수 있어 전체 시트를 본다.
		found := false
		for _, name := range f.GetSheetList() {
			rr, _ := f.GetRows(name)
			s := ""
			for _, r := range rr {
				s += strings.Join(r, "\t") + "\n"
			}
			if strings.Contains(s, "영업") {
				found = true
			}
		}
		if !found {
			t.Fatal("내 주간 엑셀에 영업 구분 없음")
		}
	}
}
