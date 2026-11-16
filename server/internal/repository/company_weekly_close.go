package repository

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"customer-support/internal/model"
)

// 95-212: 주 1차 자동 마감. 마감 시각은 여기 상수 한 곳에서만 정한다(설정 화면 없음).
// 금요일 24:00 = 토요일 00:00 KST.
const (
	weeklyCloseWeekday = time.Friday
	weeklyCloseHour    = 24

	weeklyCloseAtKey = "close1_at"
	weeklyCloseByKey = "close1_by"
	weeklyCloseBy    = "system"

	// 마감 결과(감사·로그)
	WeeklyCloseCreated = "created" // 보고서가 없어 자동 집계로 새로 만들고 마감 기록
	WeeklyCloseMarked  = "marked"  // 이미 있는 보고서: 내용·상태 불변, 마감 시각만 기록
	WeeklyCloseSkipped = "skipped" // 이미 마감 기록이 있음
	WeeklyCloseFailed  = "failed"
)

// WeeklyCloseTime 실적 주(월요일 week)의 1차 마감 시각. 예: 11/9 주 → 11/14(토) 00:00 KST.
func WeeklyCloseTime(weekMonday time.Time) time.Time {
	mon := WeekInfoOf(weekMonday).Monday
	days := int(weeklyCloseWeekday - time.Monday)
	return time.Date(mon.Year(), mon.Month(), mon.Day()+days, weeklyCloseHour, 0, 0, 0, mon.Location())
}

// WeeklyCloseLabel 「11/13(금) 24:00」 — 화면 표기. 24시는 토 00:00 이 아니라 마감 요일의 24:00 로 적는다.
func WeeklyCloseLabel(weekMonday time.Time) string {
	mon := WeekInfoOf(weekMonday).Monday
	d := mon.AddDate(0, 0, int(weeklyCloseWeekday-time.Monday))
	wd := []string{"일", "월", "화", "수", "목", "금", "토"}[d.Weekday()]
	return fmt.Sprintf("%d/%d(%s) %d:00", int(d.Month()), d.Day(), wd, weeklyCloseHour)
}

// Close1At 보고서에 기록된 1차 마감 시각(없으면 빈 값).
func (d CompanyWeeklyDoc) Close1At() string {
	raw, ok := d.Extra[weeklyCloseAtKey]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

func (d *CompanyWeeklyDoc) setClose1(at time.Time) {
	if d.Extra == nil {
		d.Extra = map[string]json.RawMessage{}
	}
	a, _ := json.Marshal(at.Format(time.RFC3339))
	b, _ := json.Marshal(weeklyCloseBy)
	d.Extra[weeklyCloseAtKey] = a
	d.Extra[weeklyCloseByKey] = b
}

// FillDocFromDraft 자동 집계 결과를 비어 있는 칸에만 채운다(「생성」과 마감이 같은 규칙).
func FillDocFromDraft(doc *CompanyWeeklyDoc, draft model.CompanyWeeklyDraft) {
	if doc.Rows == nil {
		doc.Rows = map[string]CompanyWeeklySavedRow{}
	}
	for _, row := range draft.Rows {
		cur := doc.Rows[row.RowKey]
		if len(cur.DoneLines) == 0 && strings.TrimSpace(cur.Prev) == "" {
			cur.DoneLines = SplitWeeklyLines(row.PrevText)
			cur.Prev = row.PrevText
		}
		if len(cur.PlanLines) == 0 && strings.TrimSpace(cur.Plan) == "" {
			cur.PlanLines = SplitWeeklyLines(row.PlanText)
			cur.Plan = row.PlanText
		}
		doc.Rows[row.RowKey] = cur
	}
}

type WeeklyCloseResult struct {
	OrgID, Result, Err string
}

// RunWeeklyClose1 now 가 이번 주 1차 마감 시각 이후면, 이번 주 보고서에 마감을 기록한다.
// 이번 주만 대상(지난 주는 건드리지 않음). 이미 close1_at 이 있으면 건너뛴다(재시작·중복 실행 안전).
func (r *StatsRepo) RunWeeklyClose1(now time.Time) []WeeklyCloseResult {
	loc := model.KST()
	now = now.In(loc)
	monday := WeekInfoOf(now).Monday
	closeAt := WeeklyCloseTime(monday)
	if now.Before(closeAt) {
		return nil
	}
	rows, err := r.db.Query(`SELECT org_id FROM orgs WHERE COALESCE(is_active,0)=1 ORDER BY sort_order, org_id`)
	if err != nil {
		log.Printf("주간보고 1차 마감: 조직 조회 실패: %v", err)
		return nil
	}
	var orgs []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && strings.TrimSpace(id) != "" && id != OrgAll {
			orgs = append(orgs, id)
		}
	}
	rows.Close()
	var out []WeeklyCloseResult
	for _, id := range orgs {
		res, err := r.closeOrgWeek1(id, monday, closeAt)
		item := WeeklyCloseResult{OrgID: id, Result: res}
		if err != nil {
			item.Result, item.Err = WeeklyCloseFailed, err.Error()
			log.Printf("주간보고 1차 마감 실패 org=%s: %v", id, err)
		} else if res != WeeklyCloseSkipped {
			log.Printf("주간보고 1차 마감 org=%s week=%s result=%s", id, monday.Format("2006-01-02"), res)
		}
		out = append(out, item)
	}
	return out
}

func (r *StatsRepo) closeOrgWeek1(orgID string, monday, closeAt time.Time) (string, error) {
	anchor := monday.AddDate(0, 0, 7)
	existing, err := r.GetCompanyWeeklyReport(orgID, anchor.Format("2006-01-02"))
	if err != nil {
		return "", err
	}
	label := "주간보고 1차 마감 " + orgID + " " + anchor.Format("2006-01-02")
	if existing.ReportID == "" {
		draft, err := r.BuildCompanyWeeklyDraftOrg(anchor, orgID)
		if err != nil {
			return "", err
		}
		doc := CompanyWeeklyDoc{Rows: map[string]CompanyWeeklySavedRow{}}
		FillDocFromDraft(&doc, draft)
		doc.setClose1(closeAt)
		if err := r.UpsertCompanyWeeklyReport(orgID, draft.Period.Anchor, draft.Period.SheetNameDefault,
			EncodeCompanyWeeklyDoc(doc), weeklyCloseBy, CompanyWeeklyUpsertSave); err != nil {
			return "", err
		}
		if cur, e := r.GetCompanyWeeklyReport(orgID, draft.Period.Anchor); e == nil && cur.ReportID != "" {
			logCreateWithReason(r.db, "company_weekly_reports", "report_id", cur.ReportID, label, WeeklyCloseCreated)
		}
		return WeeklyCloseCreated, nil
	}
	doc := ParseCompanyWeeklyDoc(existing.ContentJSON)
	if doc.Close1At() != "" {
		return WeeklyCloseSkipped, nil
	}
	doc.setClose1(closeAt)
	// 내용·상태·확정자·수정자·수정시각은 건드리지 않고 content_json 에 마감 키만 더한다.
	// 사이에 누가 저장했으면(내용이 달라졌으면) 0건 갱신 → 다음 주기에 다시 시도한다.
	res, err := r.db.Exec(`UPDATE company_weekly_reports SET content_json=? WHERE report_id=? AND org_id=? AND content_json=?`,
		EncodeCompanyWeeklyDoc(doc), existing.ReportID, orgID, existing.ContentJSON)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return WeeklyCloseSkipped, nil
	}
	logUpdateWithReason(r.db, "company_weekly_reports", "report_id", existing.ReportID, label, rowJSONStr(existing), WeeklyCloseMarked)
	return WeeklyCloseMarked, nil
}

func rowJSONStr(rep CompanyWeeklyReport) string {
	b, _ := json.Marshal(map[string]string{"status": rep.Status, "content_json": rep.ContentJSON})
	return string(b)
}

// StartWeeklyClose1 기동 직후 1회(따라잡기) + 1분마다 마감 시각을 확인한다.
func StartWeeklyClose1(r *StatsRepo) {
	if r == nil {
		return
	}
	go func() {
		r.RunWeeklyClose1(time.Now())
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for now := range t.C {
			r.RunWeeklyClose1(now)
		}
	}()
}
