package repository

import (
	"database/sql"
	"log"
	"math"
	"strings"
	"time"

	"customer-support/internal/model"
)

// 95-217: 접수→방문 · 방문→조치완료 · 접수→조치완료 리드타임(영업일)을 일·주·월 단위로 계속 저장한다.
// 산식은 새로 만들지 않고 기존 avgASVisitSpans / avgASCompleteLeadTime 을 그대로 부른다(요구사항 v2 §4.2).
// 저장 값은 조직별 + 전체('all'). 표본이 0이면 avg_days 는 NULL(측정할 수 없으면 표시하지 않는다).

const (
	LeadKindDay   = "day"
	LeadKindWeek  = "week"
	LeadKindMonth = "month"

	LeadMetricReceiptToVisit = "receipt_to_visit"
	LeadMetricVisitToDone    = "visit_to_done"
	LeadMetricReceiptToDone  = "receipt_to_done"

	LeadOrgAll = "all"

	// LeadQualityBase 지표 기준일 이후·이관 데이터 제외(화면 카드와 같은 기준).
	LeadQualityBase = "base"
	// LeadQualityLegacy 지표 기준일 이전 구간. 이관 데이터를 포함해 참고용으로만 저장한다.
	LeadQualityLegacy = "legacy"

	// leadBackfillFrom 처음 채우는 날(요청: 7월 데이터부터).
	leadBackfillFrom = "2026-07-01"
	// leadFinalGraceDays 기간이 끝난 뒤 이 날수가 지나면 확정해 더는 덮어쓰지 않는다.
	leadFinalGraceDays = 7
)

const leadSnapshotDDL = `CREATE TABLE IF NOT EXISTS lead_time_snapshots (
	period_kind   TEXT NOT NULL CHECK (period_kind IN ('day','week','month')),
	period_start  TEXT NOT NULL,
	period_end_ex TEXT NOT NULL,
	org_id        TEXT NOT NULL,
	metric        TEXT NOT NULL CHECK (metric IN ('receipt_to_visit','visit_to_done','receipt_to_done')),
	avg_days      REAL,
	sample_n      INTEGER NOT NULL DEFAULT 0,
	quality       TEXT NOT NULL DEFAULT 'base',
	computed_at   TEXT NOT NULL,
	final_at      TEXT,
	PRIMARY KEY (period_kind, period_start, org_id, metric)
)`

func ensureLeadSnapshotTable(db *sql.DB) error {
	if _, err := db.Exec(leadSnapshotDDL); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE INDEX IF NOT EXISTS ix_lead_snap_org ON lead_time_snapshots(org_id, period_kind, period_start)`)
	return err
}

// LeadSnapshotRow 저장된 한 줄.
type LeadSnapshotRow struct {
	Kind, Start, EndEx, OrgID, Metric, Quality, ComputedAt, FinalAt string
	AvgDays                                                         *float64
	Sample                                                          int
}

type leadPeriod struct{ Kind, Start, EndEx string }

// leadPeriods from~today 를 덮는 일·주·월 기간(오늘이 든 기간까지).
func leadPeriods(from string, today time.Time) []leadPeriod {
	loc := model.KST()
	start, err := time.ParseInLocation("2006-01-02", from, loc)
	if err != nil {
		return nil
	}
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)
	var out []leadPeriod
	for d := start; !d.After(today); d = d.AddDate(0, 0, 1) {
		out = append(out, leadPeriod{LeadKindDay, d.Format("2006-01-02"), d.AddDate(0, 0, 1).Format("2006-01-02")})
	}
	for w := WeekMonday(start); !w.After(today); w = w.AddDate(0, 0, 7) {
		out = append(out, leadPeriod{LeadKindWeek, w.Format("2006-01-02"), w.AddDate(0, 0, 7).Format("2006-01-02")})
	}
	for m := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, loc); !m.After(today); m = m.AddDate(0, 1, 0) {
		out = append(out, leadPeriod{LeadKindMonth, m.Format("2006-01-02"), m.AddDate(0, 1, 0).Format("2006-01-02")})
	}
	return out
}

func (r *StatsRepo) leadOrgIDs() []string {
	out := []string{LeadOrgAll}
	rows, err := r.db.Query(`SELECT org_id FROM orgs WHERE COALESCE(is_active,1)=1 AND org_id<>'' ORDER BY sort_order, org_id`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && strings.TrimSpace(id) != "" && id != OrgAll && id != LeadOrgAll {
			out = append(out, strings.TrimSpace(id))
		}
	}
	return out
}

// leadQualityOf 기간 전체가 지표 기준일 이전이면 legacy, 아니면 base.
func (r *StatsRepo) leadQualityOf(p leadPeriod) string {
	pol := r.MetricsPolicy()
	base := ""
	for _, b := range []string{pol.BaseReceipt, pol.BaseVisit, pol.BaseComplete, pol.BaseDate} {
		if b > base {
			base = b
		}
	}
	if base != "" && p.EndEx <= base {
		return LeadQualityLegacy
	}
	return LeadQualityBase
}

// computeLead 한 기간·한 조직의 세 지표를 기존 산식으로 계산한다.
func (r *StatsRepo) computeLead(p leadPeriod, orgID, quality string) (map[string]*float64, map[string]int, error) {
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	if orgID != LeadOrgAll {
		f.OrgID = orgID
	}
	if quality == LeadQualityLegacy {
		f.IgnoreMetricsBase = true
		f.IncludeImport = true
	}
	a1, a2, n12, err := r.avgASVisitSpans(p.Start, p.EndEx, f)
	if err != nil {
		return nil, nil, err
	}
	a3, n3, err := r.avgASCompleteLeadTime(p.Start, p.EndEx, f)
	if err != nil {
		return nil, nil, err
	}
	val := func(v float64, n int) *float64 {
		if n == 0 {
			return nil
		}
		x := math.Round(v*100) / 100
		return &x
	}
	return map[string]*float64{
			LeadMetricReceiptToVisit: val(a1, n12), LeadMetricVisitToDone: val(a2, n12), LeadMetricReceiptToDone: val(a3, n3),
		}, map[string]int{
			LeadMetricReceiptToVisit: n12, LeadMetricVisitToDone: n12, LeadMetricReceiptToDone: n3,
		}, nil
}

// saveLead 확정되지 않은 줄만 덮어쓴다. final 이면 확정 시각을 함께 적는다.
func (r *StatsRepo) saveLead(p leadPeriod, orgID, quality string, avg map[string]*float64, n map[string]int, final bool, now time.Time) error {
	ts := now.Format("2006-01-02 15:04:05")
	finalAt := interface{}(nil)
	if final {
		finalAt = ts
	}
	for _, m := range []string{LeadMetricReceiptToVisit, LeadMetricVisitToDone, LeadMetricReceiptToDone} {
		var av interface{}
		if avg[m] != nil {
			av = *avg[m]
		}
		if _, err := r.db.Exec(`
			INSERT INTO lead_time_snapshots(period_kind,period_start,period_end_ex,org_id,metric,avg_days,sample_n,quality,computed_at,final_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(period_kind,period_start,org_id,metric) DO UPDATE SET
				period_end_ex=excluded.period_end_ex, avg_days=excluded.avg_days, sample_n=excluded.sample_n,
				quality=excluded.quality, computed_at=excluded.computed_at, final_at=excluded.final_at
			WHERE lead_time_snapshots.final_at IS NULL`,
			p.Kind, p.Start, p.EndEx, orgID, m, av, n[m], quality, ts, finalAt); err != nil {
			return err
		}
	}
	return nil
}

// RunLeadSnapshots 비어 있는 기간은 채우고(처음 실행하면 7월부터), 아직 확정되지 않은 기간은 다시 계산한다.
// 확정된 기간(끝난 지 7일 지남)은 건드리지 않는다. 오류는 기간 단위로 남기고 계속한다. 새 테이블에만 쓴다.
func (r *StatsRepo) RunLeadSnapshots(now time.Time) (computed, skipped int, err error) {
	if r == nil || r.db == nil {
		return 0, 0, nil
	}
	if err := ensureLeadSnapshotTable(r.db); err != nil {
		return 0, 0, err
	}
	now = now.In(model.KST())
	finalBefore := now.AddDate(0, 0, -leadFinalGraceDays).Format("2006-01-02")
	orgs := r.leadOrgIDs()
	for _, p := range leadPeriods(leadBackfillFrom, now) {
		final := p.EndEx <= finalBefore
		quality := r.leadQualityOf(p)
		for _, org := range orgs {
			var done int
			_ = r.db.QueryRow(`SELECT COUNT(*) FROM lead_time_snapshots WHERE period_kind=? AND period_start=? AND org_id=? AND final_at IS NOT NULL`,
				p.Kind, p.Start, org).Scan(&done)
			if done > 0 {
				skipped++
				continue
			}
			avg, n, e := r.computeLead(p, org, quality)
			if e != nil {
				log.Printf("리드타임 스냅샷 계산 실패 %s %s org=%s: %v", p.Kind, p.Start, org, e)
				err = e
				continue
			}
			if e := r.saveLead(p, org, quality, avg, n, final, now); e != nil {
				log.Printf("리드타임 스냅샷 저장 실패 %s %s org=%s: %v", p.Kind, p.Start, org, e)
				err = e
				continue
			}
			computed++
		}
	}
	return computed, skipped, err
}

// ListLeadSnapshots 저장된 값을 읽는다. orgID 가 비면 전체('all').
func (r *StatsRepo) ListLeadSnapshots(kind, orgID, from, toEx string) ([]LeadSnapshotRow, error) {
	if strings.TrimSpace(orgID) == "" || orgID == OrgAll {
		orgID = LeadOrgAll
	}
	rows, err := r.db.Query(`SELECT period_kind,period_start,period_end_ex,org_id,metric,avg_days,sample_n,quality,computed_at,COALESCE(final_at,'')
		FROM lead_time_snapshots WHERE period_kind=? AND org_id=? AND period_start>=? AND period_start<?
		ORDER BY period_start, metric`, kind, orgID, from, toEx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeadSnapshotRow
	for rows.Next() {
		var x LeadSnapshotRow
		var avg sql.NullFloat64
		if err := rows.Scan(&x.Kind, &x.Start, &x.EndEx, &x.OrgID, &x.Metric, &avg, &x.Sample, &x.Quality, &x.ComputedAt, &x.FinalAt); err != nil {
			return nil, err
		}
		if avg.Valid {
			v := avg.Float64
			x.AvgDays = &v
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// StartLeadSnapshots 서버가 뜰 때 한 번(7월부터 채우기 포함) + 매일 새벽 0시 30분 이후 하루 한 번.
func StartLeadSnapshots(r *StatsRepo) {
	if r == nil {
		return
	}
	go func() {
		last := ""
		run := func(now time.Time) {
			c, s, err := r.RunLeadSnapshots(now)
			log.Printf("리드타임 스냅샷: 계산 %d · 확정 건너뜀 %d err=%v", c, s, err)
			last = now.In(model.KST()).Format("2006-01-02")
		}
		run(time.Now())
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for now := range t.C {
			k := now.In(model.KST())
			if k.Format("2006-01-02") != last && (k.Hour() > 0 || k.Minute() >= 30) {
				run(now)
			}
		}
	}()
}
