package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func kst212(y int, m time.Month, d, h, mi int) time.Time {
	return time.Date(y, m, d, h, mi, 0, 0, model.KST())
}

func closeRepo212(t *testing.T) *StatsRepo {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "c212.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// 이 시험은 조직 O03(보고서 없음)·O04(작성됨)·O05(확정됨)만 활성으로 본다.
	if _, err := db.Exec(`UPDATE orgs SET is_active=0`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"O03", "O04", "O05"} {
		if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES (?,?,?,?,1,3)
			ON CONFLICT(org_id) DO UPDATE SET is_active=1`, id, id, id, id); err != nil {
			t.Fatal(err)
		}
	}
	return NewStatsRepo(db)
}

const closeWeek212 = "2026-11-16" // 보고 주 11/9 의 저장 anchor

func resOf(rs []WeeklyCloseResult, org string) string {
	for _, r := range rs {
		if r.OrgID == org {
			return r.Result
		}
	}
	return ""
}

func TestWeeklyClose95_212_Time(t *testing.T) {
	mon := kst212(2026, 11, 9, 0, 0)
	if got := WeeklyCloseTime(mon); !got.Equal(kst212(2026, 11, 14, 0, 0)) {
		t.Fatalf("마감 시각 %v (금 24:00 = 토 00:00)", got)
	}
	if l := WeeklyCloseLabel(mon); l != "11/13(금) 24:00" {
		t.Fatalf("라벨 %q", l)
	}
}

func TestWeeklyClose95_212_Scenarios(t *testing.T) {
	r := closeRepo212(t)
	// O04: 작성된 보고서(내용 있음, 팀 전체 포함). O05: 확정된 보고서.
	written := EncodeCompanyWeeklyDoc(CompanyWeeklyDoc{
		Rows:     map[string]CompanyWeeklySavedRow{"R1": {Prev: "·직접 쓴 금주", Plan: "·직접 쓴 차주", Help: "지원 필요"}},
		TeamDone: SplitWeeklyLines("·팀 금주"),
	})
	if err := r.UpsertCompanyWeeklyReport("O04", closeWeek212, "s", written, "팀장", CompanyWeeklyUpsertSave); err != nil {
		t.Fatal(err)
	}
	if err := r.UpsertCompanyWeeklyReport("O05", closeWeek212, "s", written, "팀장", CompanyWeeklyUpsertConfirm); err != nil {
		t.Fatal(err)
	}
	before4, _ := r.GetCompanyWeeklyReport("O04", closeWeek212)
	before5, _ := r.GetCompanyWeeklyReport("O05", closeWeek212)

	// 금 23:59 — 실행 안 함
	if rs := r.RunWeeklyClose1(kst212(2026, 11, 13, 23, 59)); len(rs) != 0 {
		t.Fatalf("23:59 에는 실행하지 않는다: %v", rs)
	}
	if g, _ := r.GetCompanyWeeklyReport("O03", closeWeek212); g.ReportID != "" {
		t.Fatal("23:59 에 보고서가 생겼다")
	}
	// 토 00:00 — 실행
	rs := r.RunWeeklyClose1(kst212(2026, 11, 14, 0, 0))
	if resOf(rs, "O03") != WeeklyCloseCreated || resOf(rs, "O04") != WeeklyCloseMarked || resOf(rs, "O05") != WeeklyCloseMarked {
		t.Fatalf("결과 %+v", rs)
	}
	after4, _ := r.GetCompanyWeeklyReport("O04", closeWeek212)
	after5, _ := r.GetCompanyWeeklyReport("O05", closeWeek212)
	d4, d5 := ParseCompanyWeeklyDoc(after4.ContentJSON), ParseCompanyWeeklyDoc(after5.ContentJSON)
	if d4.Close1At() == "" || d5.Close1At() == "" || !strings.HasPrefix(d4.Close1At(), "2026-11-14T00:00:00+09:00") {
		t.Fatalf("close1_at: %q %q", d4.Close1At(), d5.Close1At())
	}
	// 마감 키를 빼면 마감 전과 바이트 단위로 동일
	strip := func(js string) string {
		d := ParseCompanyWeeklyDoc(js)
		delete(d.Extra, weeklyCloseAtKey)
		delete(d.Extra, weeklyCloseByKey)
		return EncodeCompanyWeeklyDoc(d)
	}
	if strip(after4.ContentJSON) != before4.ContentJSON || strip(after5.ContentJSON) != before5.ContentJSON {
		t.Fatalf("마감이 내용을 바꿨다\n전=%s\n후=%s", before4.ContentJSON, after4.ContentJSON)
	}
	if after5.Status != CompanyWeeklyStatusConfirmed || after5.ConfirmedBy != before5.ConfirmedBy || after5.ConfirmedAt != before5.ConfirmedAt {
		t.Fatalf("확정 상태가 바뀌었다: %+v", after5)
	}
	if after4.Status != before4.Status || after4.UpdatedBy != before4.UpdatedBy || after4.UpdatedAt != before4.UpdatedAt {
		t.Fatalf("상태·수정자가 바뀌었다: %+v", after4)
	}
	// 보고서 없던 조직: 자동 생성, 작성 중, system
	c3, _ := r.GetCompanyWeeklyReport("O03", closeWeek212)
	if c3.ReportID == "" || c3.Status != CompanyWeeklyStatusDraft || c3.UpdatedBy != "system" || ParseCompanyWeeklyDoc(c3.ContentJSON).Close1At() == "" {
		t.Fatalf("자동 생성 보고서: %+v", c3)
	}
	// 중복 실행 → 건너뜀, 내용 불변
	snap := after4.ContentJSON
	rs = r.RunWeeklyClose1(kst212(2026, 11, 14, 0, 1))
	for _, id := range []string{"O03", "O04", "O05"} {
		if resOf(rs, id) != WeeklyCloseSkipped {
			t.Fatalf("중복 실행은 건너뛴다 %s: %+v", id, rs)
		}
	}
	if g, _ := r.GetCompanyWeeklyReport("O04", closeWeek212); g.ContentJSON != snap {
		t.Fatal("중복 실행이 내용을 바꿨다")
	}
	// 마감 뒤 저장해도 close1_at 은 지워지지 않는다(저장 경로는 Extra 를 되돌려 쓴다)
	d := ParseCompanyWeeklyDoc(after4.ContentJSON)
	d.TeamDone = SplitWeeklyLines("·마감 뒤 수정")
	if err := r.UpsertCompanyWeeklyReport("O04", closeWeek212, "s", EncodeCompanyWeeklyDoc(d), "팀장", CompanyWeeklyUpsertSave); err != nil {
		t.Fatal(err)
	}
	g, _ := r.GetCompanyWeeklyReport("O04", closeWeek212)
	if ParseCompanyWeeklyDoc(g.ContentJSON).Close1At() == "" {
		t.Fatal("마감 뒤 저장이 close1_at 을 지웠다")
	}
}

// 재시작 따라잡기는 이번 주만. 다음 주 월요일에 기동하면 지난 주는 건드리지 않는다.
func TestWeeklyClose95_212_CatchUpThisWeekOnly(t *testing.T) {
	r := closeRepo212(t)
	// 토요일 오전에 기동 → 이번 주(11/9주) 마감을 따라잡는다
	if rs := r.RunWeeklyClose1(kst212(2026, 11, 14, 9, 30)); resOf(rs, "O03") != WeeklyCloseCreated {
		t.Fatalf("따라잡기 %+v", rs)
	}
	// 월요일(다음 주) 기동 → 마감 시각 전이라 아무것도 안 함, 지난 주 보고서는 그대로
	if rs := r.RunWeeklyClose1(kst212(2026, 11, 16, 8, 0)); len(rs) != 0 {
		t.Fatalf("월요일에는 지난 주를 건드리지 않는다 %+v", rs)
	}
	if g, _ := r.GetCompanyWeeklyReport("O03", "2026-11-23"); g.ReportID != "" {
		t.Fatal("다음 주 보고서가 만들어졌다")
	}
	// 마감을 놓친 지난 주(11/2주)는 일요일/월요일 따라잡기 대상이 아니다
	if g, _ := r.GetCompanyWeeklyReport("O03", "2026-11-09"); g.ReportID != "" {
		t.Fatal("지난 주 보고서가 만들어졌다")
	}
}

// 한 조직이 실패해도 나머지는 성공한다.
func TestWeeklyClose95_212_OrgFailureIsolated(t *testing.T) {
	r := closeRepo212(t)
	if _, err := r.db.Exec(`INSERT INTO company_weekly_reports(report_id,org_id,week_anchor,sheet_name,status,content_json,updated_by,updated_at)
		VALUES ('bad','O04',?, 's','draft','{"rows":{}}','x','2026-11-13 10:00:00')`, closeWeek212); err != nil {
		t.Fatal(err)
	}
	// O04 의 UPDATE 만 실패하게 트리거로 막는다
	if _, err := r.db.Exec(`CREATE TRIGGER fail212 BEFORE UPDATE ON company_weekly_reports WHEN NEW.org_id='O04'
		BEGIN SELECT RAISE(ABORT,'막음'); END`); err != nil {
		t.Fatal(err)
	}
	rs := r.RunWeeklyClose1(kst212(2026, 11, 14, 0, 0))
	if resOf(rs, "O04") != WeeklyCloseFailed || resOf(rs, "O03") != WeeklyCloseCreated || resOf(rs, "O05") != WeeklyCloseCreated {
		t.Fatalf("조직별 격리 실패 %+v", rs)
	}
}
