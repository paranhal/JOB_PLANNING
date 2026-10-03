package repository

import (
	"strings"
	"time"

	"customer-support/internal/model"
)

func WeekMonday(t time.Time) time.Time {
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return t.AddDate(0, 0, -(wd - 1))
}

func WeekRange(monday time.Time) (from, toEx string) {
	from = monday.Format("2006-01-02")
	toEx = monday.AddDate(0, 0, 7).Format("2006-01-02")
	return from, toEx
}

func (r *StatsRepo) upsertMilestone(row model.MilestoneRow) error {
	var st string
	err := r.db.QueryRow(`SELECT COALESCE(state,'') FROM milestones
		WHERE week_start=? AND org_id=? AND domain=? AND scope=? AND scope_key=?`,
		row.WeekStart, row.OrgID, row.Domain, row.Scope, row.ScopeKey).Scan(&st)
	if err == nil && st == model.MilestoneFixed {
		return nil
	}
	if row.State == "" {
		row.State = model.MilestoneDraft
	}
	_, err = r.db.Exec(`
		INSERT INTO milestones (
			week_start, org_id, domain, scope, scope_key,
			received, processed, carried, minutes,
			sales_new, sales_info, sales_act, sales_quote, sales_order, sales_contract,
			sales_open_discover, sales_open_propose, sales_open_bid,
			working_days, state, computed_at, fixed_at, fixed_by)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(week_start, org_id, domain, scope, scope_key) DO UPDATE SET
			received=excluded.received, processed=excluded.processed, carried=excluded.carried,
			minutes=excluded.minutes,
			sales_new=excluded.sales_new, sales_info=excluded.sales_info, sales_act=excluded.sales_act,
			sales_quote=excluded.sales_quote, sales_order=excluded.sales_order, sales_contract=excluded.sales_contract,
			sales_open_discover=excluded.sales_open_discover, sales_open_propose=excluded.sales_open_propose,
			sales_open_bid=excluded.sales_open_bid,
			working_days=excluded.working_days, state=excluded.state, computed_at=excluded.computed_at
			WHERE milestones.state != 'fixed'`,
		row.WeekStart, row.OrgID, row.Domain, row.Scope, row.ScopeKey,
		row.Received, row.Processed, row.Carried, row.Minutes,
		row.SalesNew, row.SalesInfo, row.SalesAct, row.SalesQuote, row.SalesOrder, row.SalesContract,
		row.SalesOpenDiscover, row.SalesOpenPropose, row.SalesOpenBid,
		row.WorkingDays, row.State, row.ComputedAt, row.FixedAt, row.FixedBy)
	return err
}

func baseFilter(orgID string) model.StatsMeetingFilter {
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	if strings.TrimSpace(orgID) == "" {
		orgID = OrgAll
	}
	f.OrgID = orgID
	return f
}

func (r *StatsRepo) ComputeMilestones(monday time.Time, orgID string) error {
	from, toEx := WeekRange(monday)
	week := monday.Format("2006-01-02")
	wd, _, _ := r.CountWorkingDays(from, toEx)
	now := time.Now().Format("2006-01-02 15:04:05")
	f := baseFilter(orgID)

	asS, err := r.countASSlice(from, toEx, f)
	if err != nil {
		return err
	}
	mntS, err := r.countMntSlice(from, toEx, f)
	if err != nil {
		return err
	}
	adminS, err := r.countAdminSlice(from, toEx, f)
	if err != nil {
		return err
	}
	salesS, err := r.countSalesSlice(from, toEx, f)
	if err != nil {
		return err
	}
	asCarry, err := r.countASCarry(toEx, toEx, f)
	if err != nil {
		return err
	}
	mntCarry, err := r.countMntCarry(toEx, toEx, f)
	if err != nil {
		return err
	}
	adminCarry, err := r.countAdminCarry(toEx, f)
	if err != nil {
		return err
	}
	adminDone, err := r.countAdminCompletedParents(from, toEx, f)
	if err != nil {
		return err
	}
	salesX, err := r.countSalesExtras(from, toEx, f)
	if err != nil {
		return err
	}

	orgRows := []model.MilestoneRow{
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainAS, Scope: model.MilestoneScopeOrg,
			Received: asS.Receipt, Processed: asS.Process, Carried: asCarry, WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainMnt, Scope: model.MilestoneScopeOrg,
			Received: mntS.Receipt, Processed: mntS.Process, Carried: mntCarry, WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainAdmin, Scope: model.MilestoneScopeOrg,
			Received: adminS.Receipt, Processed: adminDone, Carried: adminCarry, WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainSales, Scope: model.MilestoneScopeOrg,
			Received: salesS.Receipt, Processed: salesS.Process, Carried: 0,
			SalesNew: salesX.New, SalesInfo: salesX.Info, SalesAct: salesS.Receipt,
			SalesQuote: salesX.Quote, SalesOrder: salesX.Order, SalesContract: salesX.Contract,
			SalesOpenDiscover: salesX.OpenDiscover, SalesOpenPropose: salesX.OpenPropose, SalesOpenBid: salesX.OpenBid,
			WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
	}
	for _, row := range orgRows {
		if err := r.upsertMilestone(row); err != nil {
			return err
		}
	}

	names, err := r.weeklyActiveAssignees(from, toEx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, name := range names {
		seen[name] = true
	}
	if !seen[model.StatsUnassignedLabel] {
		names = append(names, model.StatsUnassignedLabel)
	}
	for _, name := range names {
		af := ParseMeetingFilter(model.StatsScopeAssignee, name, "")
		af.OrgID = orgID
		if err := r.computeAssigneeDomain(week, orgID, name, from, toEx, wd, now, af); err != nil {
			return err
		}
	}

	for i := 0; i < 7; i++ {
		d := monday.AddDate(0, 0, i)
		ds := d.Format("2006-01-02")
		nex := d.AddDate(0, 0, 1).Format("2006-01-02")
		if err := r.computeDateDomain(week, orgID, ds, ds, nex, wd, now, f); err != nil {
			return err
		}
	}
	return nil
}

func (r *StatsRepo) computeAssigneeDomain(week, orgID, name, from, toEx string, wd int, now string, f model.StatsMeetingFilter) error {
	asS, err := r.countASSlice(from, toEx, f)
	if err != nil {
		return err
	}
	mntS, err := r.countMntSlice(from, toEx, f)
	if err != nil {
		return err
	}
	adminS, err := r.countAdminSlice(from, toEx, f)
	if err != nil {
		return err
	}
	salesS, err := r.countSalesSlice(from, toEx, f)
	if err != nil {
		return err
	}
	asCarry, _ := r.countASCarry(toEx, toEx, f)
	mntCarry, _ := r.countMntCarry(toEx, toEx, f)
	adminCarry, _ := r.countAdminCarry(toEx, f)
	adminDone, _ := r.countAdminCompletedParents(from, toEx, f)
	salesX, _ := r.countSalesExtras(from, toEx, f)
	rows := []model.MilestoneRow{
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainAS, Scope: model.MilestoneScopeAssignee, ScopeKey: name,
			Received: asS.Receipt, Processed: asS.Process, Carried: asCarry, WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainMnt, Scope: model.MilestoneScopeAssignee, ScopeKey: name,
			Received: mntS.Receipt, Processed: mntS.Process, Carried: mntCarry, WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainAdmin, Scope: model.MilestoneScopeAssignee, ScopeKey: name,
			Received: adminS.Receipt, Processed: adminDone, Carried: adminCarry, WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainSales, Scope: model.MilestoneScopeAssignee, ScopeKey: name,
			Received: salesS.Receipt, Processed: salesS.Process, Carried: 0, SalesAct: salesS.Receipt,
			SalesNew: salesX.New, SalesInfo: salesX.Info, SalesQuote: salesX.Quote, SalesOrder: salesX.Order, SalesContract: salesX.Contract,
			WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
	}
	for _, row := range rows {
		if err := r.upsertMilestone(row); err != nil {
			return err
		}
	}
	return nil
}

func (r *StatsRepo) computeDateDomain(week, orgID, key, from, toEx string, wd int, now string, f model.StatsMeetingFilter) error {
	asS, err := r.countASSlice(from, toEx, f)
	if err != nil {
		return err
	}
	mntS, err := r.countMntSlice(from, toEx, f)
	if err != nil {
		return err
	}
	adminS, err := r.countAdminSlice(from, toEx, f)
	if err != nil {
		return err
	}
	salesS, err := r.countSalesSlice(from, toEx, f)
	if err != nil {
		return err
	}
	adminDone, _ := r.countAdminCompletedParents(from, toEx, f)
	rows := []model.MilestoneRow{
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainAS, Scope: model.MilestoneScopeDate, ScopeKey: key,
			Received: asS.Receipt, Processed: asS.Process, WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainMnt, Scope: model.MilestoneScopeDate, ScopeKey: key,
			Received: mntS.Receipt, Processed: mntS.Process, WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainAdmin, Scope: model.MilestoneScopeDate, ScopeKey: key,
			Received: adminS.Receipt, Processed: adminDone, WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
		{WeekStart: week, OrgID: orgID, Domain: model.MilestoneDomainSales, Scope: model.MilestoneScopeDate, ScopeKey: key,
			Received: salesS.Receipt, Processed: salesS.Process, Carried: 0, SalesAct: salesS.Receipt,
			WorkingDays: wd, State: model.MilestoneDraft, ComputedAt: now},
	}
	for _, row := range rows {
		if err := r.upsertMilestone(row); err != nil {
			return err
		}
	}
	return nil
}

type salesExtras struct {
	New, Info, Quote, Order, Contract  int
	OpenDiscover, OpenPropose, OpenBid int
}

func (r *StatsRepo) countSalesExtras(from, toEx string, f model.StatsMeetingFilter) (salesExtras, error) {
	var out salesExtras
	orgFrag, orgArgs := mustOrgSQL("s", f.OrgID)
	q := func(sql string, args ...interface{}) (int, error) {
		return r.countSQL(sql, args...)
	}
	var err error
	out.New, err = q(`SELECT COUNT(*) FROM sales_projects s WHERE s.created_at >= ? AND s.created_at < ?`+orgFrag,
		append([]interface{}{dayTimeStart(from), dayTimeStart(toEx)}, orgArgs...)...)
	if err != nil {
		return out, err
	}
	out.Info, err = q(`SELECT COUNT(DISTINCT h.sales_id) FROM sales_stage_history h
		JOIN sales_projects s ON s.sales_id=h.sales_id
		WHERE h.changed_at >= ? AND h.changed_at < ?`+orgFrag,
		append([]interface{}{dayTimeStart(from), dayTimeStart(toEx)}, orgArgs...)...)
	if err != nil {
		return out, err
	}
	qFrag, qArgs := mustOrgSQL("q", f.OrgID)
	out.Quote, err = q(`SELECT COUNT(*) FROM sales_quotes q WHERE q.quote_date >= ? AND q.quote_date < ?`+qFrag,
		append([]interface{}{from, toEx}, qArgs...)...)
	if err != nil {
		return out, err
	}
	oFrag, oArgs := mustOrgSQL("o", f.OrgID)
	out.Order, err = q(`SELECT COUNT(*) FROM sales_orders o WHERE o.order_date >= ? AND o.order_date < ?`+oFrag,
		append([]interface{}{from, toEx}, oArgs...)...)
	if err != nil {
		return out, err
	}
	out.Contract, err = q(`SELECT COUNT(*) FROM sales_projects s
		WHERE TRIM(COALESCE(s.contracted_at,'')) != '' AND date(s.contracted_at) >= date(?) AND date(s.contracted_at) < date(?)
		  AND COALESCE(s.close_reason,'')=?`+orgFrag,
		append([]interface{}{from, toEx, model.SalesCloseContracted}, orgArgs...)...)
	if err != nil {
		return out, err
	}
	countStage := func(stage string) (int, error) {
		return q(`SELECT COUNT(*) FROM sales_projects s WHERE s.stage=? AND COALESCE(s.status,'active') NOT IN ('dropped','promoted')`+orgFrag,
			append([]interface{}{stage}, orgArgs...)...)
	}
	out.OpenDiscover, err = countStage(model.SalesStage4Discover)
	if err != nil {
		return out, err
	}
	out.OpenPropose, err = countStage(model.SalesStage4Propose)
	if err != nil {
		return out, err
	}
	out.OpenBid, err = countStage(model.SalesStage4Bid)
	return out, err
}

func (r *StatsRepo) ListMilestones(week, orgID, domain, scope string) ([]model.MilestoneRow, error) {
	q := `SELECT week_start, org_id, domain, scope, COALESCE(scope_key,''),
		COALESCE(received,0), COALESCE(processed,0), COALESCE(carried,0), COALESCE(minutes,0),
		COALESCE(sales_new,0), COALESCE(sales_info,0), COALESCE(sales_act,0), COALESCE(sales_quote,0),
		COALESCE(sales_order,0), COALESCE(sales_contract,0),
		COALESCE(sales_open_discover,0), COALESCE(sales_open_propose,0), COALESCE(sales_open_bid,0),
		COALESCE(working_days,0), COALESCE(state,''), COALESCE(computed_at,''), COALESCE(fixed_at,''), COALESCE(fixed_by,'')
		FROM milestones WHERE week_start=?`
	args := []interface{}{week}
	if orgID != "" && orgID != OrgAll {
		q += ` AND org_id=?`
		args = append(args, orgID)
	}
	if domain != "" {
		q += ` AND domain=?`
		args = append(args, domain)
	}
	if scope != "" {
		q += ` AND scope=?`
		args = append(args, scope)
	}
	q += ` ORDER BY domain, scope, scope_key`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MilestoneRow
	for rows.Next() {
		var it model.MilestoneRow
		if err := rows.Scan(&it.WeekStart, &it.OrgID, &it.Domain, &it.Scope, &it.ScopeKey,
			&it.Received, &it.Processed, &it.Carried, &it.Minutes,
			&it.SalesNew, &it.SalesInfo, &it.SalesAct, &it.SalesQuote, &it.SalesOrder, &it.SalesContract,
			&it.SalesOpenDiscover, &it.SalesOpenPropose, &it.SalesOpenBid,
			&it.WorkingDays, &it.State, &it.ComputedAt, &it.FixedAt, &it.FixedBy); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *StatsRepo) MilestoneOrgState(week, orgID string) string {
	var st string
	_ = r.db.QueryRow(`SELECT state FROM milestones WHERE week_start=? AND org_id=? AND scope='org' AND domain='as' AND scope_key=''`,
		week, orgID).Scan(&st)
	return st
}

func (r *StatsRepo) FixMilestones(week, orgID, by string) error {
	_, err := r.db.Exec(`UPDATE milestones SET state='fixed', fixed_at=CURRENT_TIMESTAMP, fixed_by=?
		WHERE week_start=? AND org_id=? AND state='draft'`, by, week, orgID)
	return err
}

func (r *StatsRepo) UnfixMilestones(week, orgID, by string) error {
	_, err := r.db.Exec(`UPDATE milestones SET state='draft', fixed_at=NULL, fixed_by=''
		WHERE week_start=? AND org_id=? AND state='fixed'`, week, orgID)
	return err
}

func (r *StatsRepo) HasOrgMilestone(week, orgID string) bool {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM milestones WHERE week_start=? AND org_id=? AND scope='org'`, week, orgID).Scan(&n)
	return n > 0
}

func (r *StatsRepo) RecomputeDraftWeek(monday time.Time) {
	orgs, err := NewOrgRepo(r.db).ListActive()
	if err != nil {
		return
	}
	week := monday.Format("2006-01-02")
	for _, o := range orgs {
		if r.MilestoneOrgState(week, o.OrgID) == model.MilestoneFixed {
			continue
		}
		_ = r.ComputeMilestones(monday, o.OrgID)
	}
}

func (r *StatsRepo) ListMilestoneEvidence(week, orgID, domain, metric string) ([]model.MilestoneEvidenceItem, error) {
	mon, err := time.ParseInLocation("2006-01-02", week, time.Local)
	if err != nil {
		return nil, err
	}
	from, toEx := WeekRange(mon)
	events, err := r.ListWeeklyEventRows(from, toEx)
	if err != nil {
		return nil, err
	}
	var out []model.MilestoneEvidenceItem
	for _, ev := range events {
		if !matchEvidence(ev, domain, metric) {
			continue
		}
		href := "/workboard/tasks/?embed=1"
		switch ev.WorkType {
		case "AS":
			href = "/as/" + ev.WorkNo + "?embed=1"
		case "정기점검":
			href = "/maintenance/visits/" + ev.WorkNo + "?embed=1"
		}
		out = append(out, model.MilestoneEvidenceItem{
			ID: ev.WorkNo, Number: ev.WorkNo, Title: ev.Customer + " " + ev.Content, Date: ev.OccurDate, Href: href,
		})
	}
	return out, nil
}

func matchEvidence(ev model.WeeklyEventRow, domain, metric string) bool {
	switch domain {
	case model.MilestoneDomainAS:
		if ev.WorkType != "AS" {
			return false
		}
		if metric == "processed" {
			return ev.Kind == model.WeeklyEventComplete || ev.Kind == model.WeeklyEventAction
		}
		if metric == "carried" {
			return ev.Kind != model.WeeklyEventComplete
		}
		return ev.Kind == model.WeeklyEventReceipt
	case model.MilestoneDomainMnt:
		return ev.WorkType == "정기점검"
	case model.MilestoneDomainAdmin:
		return ev.Kind == model.WeeklyEventAdmin
	case model.MilestoneDomainSales:
		return ev.WorkType == "영업" || strings.Contains(ev.WorkType, "영업")
	}
	return true
}

func (r *StatsRepo) applyFixedMilestoneToWeekly(out *model.WeeklyReport, orgID string) {
	if out == nil {
		return
	}
	rows, err := r.ListMilestones(out.WeekFrom, orgID, "", model.MilestoneScopeOrg)
	if err != nil || len(rows) == 0 {
		out.MilestoneProvisional = true
		return
	}
	st := ""
	for _, row := range rows {
		st = row.State
		switch row.Domain {
		case model.MilestoneDomainSales:
			out.SalesNew = row.SalesNew
			out.SalesInfo = row.SalesInfo
			out.SalesAct = row.SalesAct
			out.SalesQuote = row.SalesQuote
			out.SalesOrder = row.SalesOrder
			out.SalesContract = row.SalesContract
		}
	}
	out.MilestoneState = st
	out.MilestoneProvisional = st != model.MilestoneFixed
	if st == model.MilestoneFixed && len(out.PersonRows) > 0 && out.PersonRows[0].IsTeam {
		var rec, done, carry int
		for _, row := range rows {
			if row.Domain == model.MilestoneDomainSales {
				continue
			}
			rec += row.Received
			done += row.Processed
			carry += row.Carried
		}
		out.PersonRows[0].Receipt = rec
		out.PersonRows[0].Completed = done
		out.PersonRows[0].CarryOut = carry
	}
}

func (r *StatsRepo) fillLiveSalesWeekly(out *model.WeeklyReport, orgID string) {
	if out == nil || (out.SalesAct+out.SalesNew+out.SalesQuote) > 0 {
		return
	}
	f := baseFilter(orgID)
	salesS, err := r.countSalesSlice(out.WeekFrom, out.WeekToEx, f)
	if err != nil {
		return
	}
	x, err := r.countSalesExtras(out.WeekFrom, out.WeekToEx, f)
	if err != nil {
		return
	}
	out.SalesAct = salesS.Receipt
	out.SalesNew = x.New
	out.SalesInfo = x.Info
	out.SalesQuote = x.Quote
	out.SalesOrder = x.Order
	out.SalesContract = x.Contract
	if out.MilestoneState == "" {
		out.MilestoneProvisional = true
	}
}

func NextMilestoneFire(now time.Time, deadline time.Time) time.Time {
	loc := now.Location()
	mon := WeekMonday(now)
	mondayMorning := time.Date(mon.Year(), mon.Month(), mon.Day(), 8, 0, 0, 0, loc)
	if now.Before(mondayMorning) {
		if deadline.Before(mondayMorning) && now.Before(deadline) {
			return deadline
		}
		return mondayMorning
	}
	if now.Before(deadline) {
		return deadline
	}
	nextMon := mon.AddDate(0, 0, 7)
	return time.Date(nextMon.Year(), nextMon.Month(), nextMon.Day(), 8, 0, 0, 0, loc)
}

func MilestoneWeekOptions(now time.Time, n int) []string {
	mon := WeekMonday(now)
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, mon.AddDate(0, 0, -7*i).Format("2006-01-02"))
	}
	return out
}
