package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-219: 마일스톤 화면의 리드타임 추이(저장된 스냅샷을 일·주·월로 보여 준다).
func TestMilestonesLeadTrend95_219(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "ms219.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	e.GET("/stats/milestones", h.Stats.Milestones, h.Auth.AuthMiddleware)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, model.OrgIDLibrary)
	get := func(q string) string {
		req := httptest.NewRequest(http.MethodGet, "/stats/milestones"+q, nil)
		req.AddCookie(oa)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status %d", q, rec.Code)
		}
		return rec.Body.String()
	}
	// 저장된 값이 없으면 안내한다
	if b := get(""); !strings.Contains(b, "data-lead-empty") {
		t.Fatal("저장된 값이 없을 때 안내가 없다")
	}
	seedLead219(t, db)

	b := get("")
	if strings.Contains(b, "data-lead-empty") {
		t.Fatal("값이 있는데 비어 있다고 나온다")
	}
	rows := strings.Count(b, "data-lead-row")
	if rows != 3 {
		t.Fatalf("주 단위 기본 3행 want 3 got %d", rows)
	}
	for _, want := range []string{"<b>0.8</b>", "17건", "<b>1.5</b>", "69건", "data-lead-legacy", "표본 적음", "집계 중", "—", `data-lead-tab="day"`, `data-lead-tab="week"`, `data-lead-tab="month"`, `data-lead-spark="0"`} {
		if !strings.Contains(b, want) {
			t.Errorf("화면에 %q 없음", want)
		}
	}
	if strings.Contains(b, "9.9") {
		t.Error("다른 조직(O03) 값이 이 조직 화면에 나옴")
	}
	// 표본 없는 지표는 0.0 이 아니라 「—」: 참고 주의 방문→조치완료 칸
	if !strings.Contains(b, ">—</td>") {
		t.Error("표본 0 칸이 「—」가 아니다")
	}
	// 월 단위·일 단위 전환(저장된 값이 없으면 안내)
	if m := get("?lead=month"); !strings.Contains(m, "data-lead-empty") {
		t.Error("월 단위에 값이 없으면 안내해야 한다")
	}
	// 탭 링크는 다른 검색 조건(영역·주)을 유지한다
	if tb := get("?domain=mnt&lead=day"); !strings.Contains(tb, "domain=mnt") || !strings.Contains(tb, "lead=week") {
		t.Error("탭 링크가 현재 조건을 유지하지 않는다")
	}
}

func seedLead219(t *testing.T, db *sql.DB) {
	t.Helper()
	now := time.Now().In(model.KST())
	mon := repository.WeekMonday(now)
	ins := func(kind, start, endEx, org, metric string, avg interface{}, n int, quality string, final interface{}) {
		if _, err := db.Exec(`INSERT INTO lead_time_snapshots(period_kind,period_start,period_end_ex,org_id,metric,avg_days,sample_n,quality,computed_at,final_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			kind, start, endEx, org, metric, avg, n, quality, "2026-11-20 00:30:00", final); err != nil {
			t.Fatal(err)
		}
	}
	w := func(back int) (string, string) {
		s := mon.AddDate(0, 0, -7*back)
		return s.Format("2006-01-02"), s.AddDate(0, 0, 7).Format("2006-01-02")
	}
	s1, e1 := w(2) // 확정·base
	ins("week", s1, e1, model.OrgIDLibrary, repository.LeadMetricReceiptToVisit, 0.8, 17, "base", "2026-11-20 00:30:00")
	ins("week", s1, e1, model.OrgIDLibrary, repository.LeadMetricVisitToDone, 0.0, 17, "base", "2026-11-20 00:30:00")
	ins("week", s1, e1, model.OrgIDLibrary, repository.LeadMetricReceiptToDone, 1.5, 69, "base", "2026-11-20 00:30:00")
	s2, e2 := w(1) // 확정·legacy·표본 적음
	ins("week", s2, e2, model.OrgIDLibrary, repository.LeadMetricReceiptToVisit, 1.2, 3, "legacy", "2026-11-20 00:30:00")
	ins("week", s2, e2, model.OrgIDLibrary, repository.LeadMetricVisitToDone, nil, 0, "legacy", "2026-11-20 00:30:00")
	ins("week", s2, e2, model.OrgIDLibrary, repository.LeadMetricReceiptToDone, 2.0, 9, "legacy", "2026-11-20 00:30:00")
	s3, e3 := w(0) // 집계 중
	ins("week", s3, e3, model.OrgIDLibrary, repository.LeadMetricReceiptToVisit, 0.5, 6, "base", nil)
	ins("week", s3, e3, model.OrgIDLibrary, repository.LeadMetricVisitToDone, 0.5, 6, "base", nil)
	ins("week", s3, e3, model.OrgIDLibrary, repository.LeadMetricReceiptToDone, 1.0, 12, "base", nil)
	// 다른 조직 값은 이 조직 화면에 나오면 안 된다
	ins("week", s1, e1, "O03", repository.LeadMetricReceiptToDone, 9.9, 99, "base", "2026-11-20 00:30:00")

}
