package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

type ProgressBrief struct {
	Text string
	Rate string
}

func seedMilestonesTx(tx *sql.Tx, packageID, planID string) error {
	for _, pct := range []int{25, 50, 75, 100} {
		var n int
		err := tx.QueryRow(`SELECT COUNT(*) FROM package_milestones WHERE package_id=? AND plan_id=? AND threshold_pct=?`, packageID, planID, pct).Scan(&n)
		if err != nil {
			if strings.Contains(err.Error(), "no such table") {
				return nil
			}
			return err
		}
		if n > 0 {
			continue
		}
		seq, err := nextSeqTx(tx, "package_milestone")
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO package_milestones(milestone_id, package_id, plan_id, threshold_pct, label)
			VALUES (?,?,?,?,?)`,
			fmt.Sprintf("MS-%03d", seq), packageID, planID, pct, fmt.Sprintf("%d%%", pct)); err != nil {
			return err
		}
	}
	return nil
}

func syncMilestonesTx(tx *sql.Tx, packageID, day string) error {
	packageID = strings.TrimSpace(packageID)
	day = strings.TrimSpace(day)
	if len(day) >= 10 {
		day = day[:10]
	}
	if packageID == "" || day == "" {
		return nil
	}
	basis := map[string]string{}
	brows, err := tx.Query(`SELECT plan_id, progress_basis FROM package_plans WHERE package_id=?`, packageID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	for brows.Next() {
		var id, b string
		if err := brows.Scan(&id, &b); err != nil {
			brows.Close()
			return err
		}
		basis[id] = b
	}
	brows.Close()
	type agg struct{ done, total int }
	stat := map[string]*agg{}
	add := func(plan string) *agg {
		if stat[plan] == nil {
			stat[plan] = &agg{}
		}
		return stat[plan]
	}
	srows, err := tx.Query(`SELECT COALESCE(plan_id,''), status, target_qty, done_qty FROM package_sites WHERE package_id=?`, packageID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	for srows.Next() {
		var plan, status string
		var target, done int
		if err := srows.Scan(&plan, &status, &target, &done); err != nil {
			srows.Close()
			return err
		}
		if status == model.SiteExcluded {
			continue
		}
		for _, key := range []string{"", plan} {
			a := add(key)
			if key != "" && basis[plan] == model.BasisQty {
				a.total += target
				a.done += model.QtyCredit(status, target, done)
				continue
			}
			if key != "" && basis[plan] != model.BasisQty {
				a.total++
				if status == model.SiteDone {
					a.done++
				}
				continue
			}
			a.total++
			if status == model.SiteDone {
				a.done++
			}
		}
	}
	srows.Close()
	mrows, err := tx.Query(`SELECT milestone_id, COALESCE(plan_id,''), threshold_pct, reached_at, dropped_at FROM package_milestones WHERE package_id=?`, packageID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	defer mrows.Close()
	for mrows.Next() {
		var id, plan, reached, dropped string
		var pct int
		if err := mrows.Scan(&id, &plan, &pct, &reached, &dropped); err != nil {
			return err
		}
		a := stat[plan]
		rate := 0
		if a != nil && a.total > 0 {
			rate = a.done * 100 / a.total
		}
		if rate >= pct {
			if strings.TrimSpace(reached) == "" {
				reached = day
			}
			dropped = ""
		} else if strings.TrimSpace(reached) != "" && strings.TrimSpace(dropped) == "" {
			dropped = day
		}
		if _, err := tx.Exec(`UPDATE package_milestones SET reached_at=?, dropped_at=? WHERE milestone_id=?`, reached, dropped, id); err != nil {
			return err
		}
	}
	return mrows.Err()
}

func (r *ProjectRepo) SeedMilestones(projectID, packageID, planID string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("저장소가 없습니다")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := packagePhaseTx(tx, packageID, projectID); err != nil {
		return err
	}
	planID = strings.TrimSpace(planID)
	if planID != "" {
		var pkg string
		if err := tx.QueryRow(`SELECT package_id FROM package_plans WHERE plan_id=?`, planID).Scan(&pkg); err != nil || pkg != packageID {
			return fmt.Errorf("계획이 없습니다")
		}
	}
	if err := seedMilestonesTx(tx, packageID, planID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ProjectRepo) SetMilestoneConfirm(projectID, id, userID, note string, on bool) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("저장소가 없습니다")
	}
	var pkg string
	err := r.db.QueryRow(`
		SELECT m.package_id
		FROM package_milestones m
		JOIN phase_packages pk ON pk.package_id=m.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE m.milestone_id=? AND ph.project_id=?`, id, projectID).Scan(&pkg)
	if err != nil {
		return fmt.Errorf("마일스톤이 없습니다")
	}
	when, by := "", ""
	if on {
		when = time.Now().Format("2006-01-02 15:04:05")
		by = strings.TrimSpace(userID)
	}
	_, err = r.db.Exec(`UPDATE package_milestones SET confirmed_at=?, confirmed_by=?, note=? WHERE milestone_id=?`, when, by, strings.TrimSpace(note), id)
	return err
}

func (r *ProjectRepo) LoadStructurePeriod(projectID, today, which string, biz func(string) bool) (StructureTree, error) {
	tree, err := r.LoadStructure(projectID, today, biz)
	if err != nil || !tree.Has {
		return tree, err
	}
	from, to, label := model.ReportSpan(today, which)
	if err := r.decorateTree(&tree, projectID, from, to, label, today, biz); err != nil {
		return tree, err
	}
	return tree, nil
}

func (r *ProjectRepo) decorateTree(tree *StructureTree, projectID, from, to, label, today string, biz func(string) bool) error {
	events, err := r.projectEvents(projectID, paceFloor(today, from))
	if err != nil {
		return err
	}
	days, periodSites, periodQty := foldEvents(events, from, to)
	for pi := range tree.Phases {
		for ki := range tree.Phases[pi].Packages {
			pk := &tree.Phases[pi].Packages[ki]
			for ni := range pk.Plans {
				pl := &pk.Plans[ni]
				pl.PeriodSites = len(periodSites[pl.ID])
				pl.PeriodQty = periodQty[pl.ID]
				done, total := pl.DoneN, pl.TotalN
				unit := "곳"
				if pl.Basis == model.BasisQty {
					done, total = pl.QtyDone, pl.QtyTotal
					unit = strings.TrimSpace(pl.Unit)
					if unit == "" {
						unit = "식"
					}
				}
				facts := model.WindowFacts(today, pl.PlanEnd, pl.Basis, done, total, pl.Confirmed, days[pl.ID], biz)
				facts.Unit = unit
				pace := model.AssessPace(facts)
				pl.Pace = pace.Label
				pl.PaceNote = pace.Note()
				pl.PaceView = pace
			}
		}
	}
	tree.Report = reportFromTree(*tree, label)
	miles, err := r.projectMilestones(projectID)
	if err != nil {
		return err
	}
	for pi := range tree.Phases {
		for ki := range tree.Phases[pi].Packages {
			pk := &tree.Phases[pi].Packages[ki]
			pk.Milestones = miles[pk.ID+"|"]
			for ni := range pk.Plans {
				pl := &pk.Plans[ni]
				pl.Milestones = miles[pk.ID+"|"+pl.ID]
			}
		}
	}
	pts, err := r.projectChart(projectID)
	if err != nil {
		return err
	}
	tree.ChartSVG = model.ProgressChartSVG(pts)
	return nil
}

func (r *ProjectRepo) ProgressLines(ids []string, from, to, today string, biz func(string) bool) (map[string]ProgressBrief, error) {
	out := map[string]ProgressBrief{}
	if r == nil || r.db == nil || len(ids) == 0 {
		return out, nil
	}
	trees, err := r.lightTrees(ids)
	if err != nil {
		return nil, err
	}
	events, err := r.eventsFor(ids, paceFloor(today, from))
	if err != nil {
		return nil, err
	}
	byProject := map[string][]progEvent{}
	for _, e := range events {
		byProject[e.project] = append(byProject[e.project], e)
	}
	for id, tree := range trees {
		if tree == nil || !tree.Has {
			continue
		}
		days, ps, pq := foldEvents(byProject[id], from, to)
		for pi := range tree.Phases {
			for ki := range tree.Phases[pi].Packages {
				for ni := range tree.Phases[pi].Packages[ki].Plans {
					pl := &tree.Phases[pi].Packages[ki].Plans[ni]
					pl.PeriodSites = len(ps[pl.ID])
					pl.PeriodQty = pq[pl.ID]
					done, total := pl.DoneN, pl.TotalN
					unit := "곳"
					if pl.Basis == model.BasisQty {
						done, total = pl.QtyDone, pl.QtyTotal
						unit = strings.TrimSpace(pl.Unit)
						if unit == "" {
							unit = "식"
						}
					}
					facts := model.WindowFacts(today, pl.PlanEnd, pl.Basis, done, total, pl.Confirmed, days[pl.ID], biz)
					facts.Unit = unit
					pace := model.AssessPace(facts)
					pl.Pace = pace.Label
					pl.PaceView = pace
				}
			}
		}
		rep := reportFromTree(*tree, model.PeriodWord(from, to, today))
		lines := model.BuildProjectProgressReport(rep)
		if len(lines) == 0 {
			continue
		}
		brief := ProgressBrief{Text: strings.Join(lines, " ")}
		if len(rep.Plans) > 0 {
			p := rep.Plans[0]
			if p.Basis == model.BasisQty && p.QtyTotal > 0 {
				brief.Rate = fmt.Sprintf("%s/%s (%s)", model.FormatIntComma(p.QtyDone), model.FormatIntComma(p.QtyTotal), model.SiteCheckRateLabel(p.QtyDone, p.QtyTotal))
			} else if p.SitesTotal > 0 {
				brief.Rate = fmt.Sprintf("%d/%d (%s)", p.SitesDone, p.SitesTotal, model.SiteCheckRateLabel(p.SitesDone, p.SitesTotal))
			}
		}
		out[id] = brief
	}
	return out, nil
}

func reportFromTree(tree StructureTree, label string) model.ProgressReport {
	rep := model.ProgressReport{Has: tree.Has, PeriodLabel: label}
	if !tree.Has {
		return rep
	}
	var current *PhaseNode
	for i := range tree.Phases {
		if tree.Phases[i].Current {
			current = &tree.Phases[i]
			break
		}
	}
	if current == nil {
		for i := range tree.Phases {
			if tree.Phases[i].Display != "완료" {
				current = &tree.Phases[i]
				break
			}
		}
	}
	if current == nil && len(tree.Phases) > 0 {
		current = &tree.Phases[0]
	}
	if current == nil || len(current.Packages) == 0 {
		rep.Has = false
		return rep
	}
	rep.Phase = current.Name
	var cur *PkgNode
	for i := range current.Packages {
		if current.Packages[i].Display == "진행 중" || current.Packages[i].Display == "완료 후보" {
			cur = &current.Packages[i]
			break
		}
	}
	if cur == nil {
		cur = &current.Packages[0]
	}
	rep.Package = cur.Name
	seen := false
	for _, pk := range current.Packages {
		if seen {
			rep.NextPackage = pk.Name
			break
		}
		if pk.ID == cur.ID {
			seen = true
		}
	}
	if rep.NextPackage == "" {
		passed := false
		for _, ph := range tree.Phases {
			if passed && len(ph.Packages) > 0 {
				rep.NextPackage = ph.Packages[0].Name
				break
			}
			if ph.ID == current.ID {
				passed = true
			}
		}
	}
	var active []model.ReportPlan
	for _, ph := range tree.Phases {
		for _, pk := range ph.Packages {
			for _, pl := range pk.Plans {
				if pl.Hold {
					continue
				}
				if pl.Display == "진행 중" || pl.Display == "완료 후보" {
					active = append(active, toReportPlan(pl))
				}
			}
		}
	}
	if len(active) == 0 {
		for _, pl := range cur.Plans {
			if pl.Hold {
				continue
			}
			active = append(active, toReportPlan(pl))
			break
		}
	}
	rep.Plans = active
	return rep
}

func toReportPlan(pl PlanNode) model.ReportPlan {
	return model.ReportPlan{
		Name: pl.Name, Package: pl.PackageName, NextNote: pl.NextNote, PackageNote: pl.PkgNote,
		Confirmed: pl.Confirmed, Basis: pl.Basis, Unit: pl.Unit,
		SitesDone: pl.DoneN, SitesTotal: pl.TotalN, Doing: pl.Doing,
		QtyDone: pl.QtyDone, QtyTotal: pl.QtyTotal,
		PeriodSites: pl.PeriodSites, PeriodQty: pl.PeriodQty,
		PlanEnd: pl.PlanEnd, Pace: pl.PaceView,
	}
}

type progEvent struct {
	project, plan, site, date, typ string
	delta                          int
}

func foldEvents(events []progEvent, from, to string) (map[string][]model.DayAmt, map[string]map[string]bool, map[string]int) {
	type key struct{ plan, date string }
	acc := map[key]*model.DayAmt{}
	seen := map[string]bool{}
	periodSites := map[string]map[string]bool{}
	periodQty := map[string]int{}
	for _, e := range events {
		k := key{e.plan, e.date}
		d := acc[k]
		if d == nil {
			d = &model.DayAmt{Date: e.date}
			acc[k] = d
		}
		if e.typ == "complete" {
			sk := e.plan + "|" + e.date + "|" + e.site
			if !seen[sk] {
				seen[sk] = true
				d.Sites++
			}
		}
		if e.delta > 0 {
			d.Qty += e.delta
		}
		if e.typ == "complete" && e.date >= from && e.date <= to {
			if periodSites[e.plan] == nil {
				periodSites[e.plan] = map[string]bool{}
			}
			periodSites[e.plan][e.site] = true
			if e.delta > 0 {
				periodQty[e.plan] += e.delta
			}
		}
	}
	out := map[string][]model.DayAmt{}
	for k, d := range acc {
		out[k.plan] = append(out[k.plan], *d)
	}
	return out, periodSites, periodQty
}

func (r *ProjectRepo) projectEvents(projectID, since string) ([]progEvent, error) {
	return r.eventsFor([]string{projectID}, since)
}

func (r *ProjectRepo) eventsFor(ids []string, since string) ([]progEvent, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph, idsArgs := inArgs(ids)
	args := append([]interface{}{since}, idsArgs...)
	rows, err := r.db.Query(`
		SELECT ph.project_id, COALESCE(e.plan_id,''), e.package_site_id, e.event_date, e.event_type, e.qty_delta
		FROM package_events e
		JOIN phase_packages pk ON pk.package_id=e.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE e.event_date>=? AND ph.project_id IN (`+ph+`)`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []progEvent
	for rows.Next() {
		var e progEvent
		if err := rows.Scan(&e.project, &e.plan, &e.site, &e.date, &e.typ, &e.delta); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) projectMilestones(projectID string) (map[string][]model.MilestoneView, error) {
	out := map[string][]model.MilestoneView{}
	rows, err := r.db.Query(`
		SELECT m.milestone_id, m.package_id, COALESCE(m.plan_id,''), m.threshold_pct,
		       COALESCE(m.reached_at,''), COALESCE(m.dropped_at,''), COALESCE(m.confirmed_at,''), COALESCE(m.note,'')
		FROM package_milestones m
		JOIN phase_packages pk ON pk.package_id=m.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=?
		ORDER BY m.threshold_pct, m.milestone_id`, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m model.MilestoneView
		var reached, dropped, confirmed string
		if err := rows.Scan(&m.ID, &m.PackageID, &m.PlanID, &m.Pct, &reached, &dropped, &confirmed, &m.Note); err != nil {
			return nil, err
		}
		m.State, m.When = milestoneState(reached, dropped, confirmed)
		out[m.PackageID+"|"+m.PlanID] = append(out[m.PackageID+"|"+m.PlanID], m)
	}
	return out, rows.Err()
}

func milestoneState(reached, dropped, confirmed string) (string, string) {
	if strings.TrimSpace(confirmed) != "" {
		return "확인 완료", dayOf(confirmed)
	}
	if strings.TrimSpace(dropped) != "" {
		return "재하락", dayOf(dropped)
	}
	if strings.TrimSpace(reached) != "" {
		return "도달", dayOf(reached)
	}
	return "미도달", ""
}

func dayOf(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func (r *ProjectRepo) projectChart(projectID string) ([]model.ChartDay, error) {
	rows, err := r.db.Query(`
		SELECT d.snap_date, SUM(d.done_sites), SUM(d.day_done_sites)
		FROM package_progress_daily d
		JOIN phase_packages pk ON pk.package_id=d.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=? AND d.plan_id=''
		GROUP BY d.snap_date
		ORDER BY d.snap_date`, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.ChartDay
	for rows.Next() {
		var d model.ChartDay
		if err := rows.Scan(&d.Date, &d.Done, &d.Day); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if len(out) > 30 {
		out = out[len(out)-30:]
	}
	return out, rows.Err()
}

func (r *ProjectRepo) lightTrees(ids []string) (map[string]*StructureTree, error) {
	out := map[string]*StructureTree{}
	for _, id := range ids {
		out[id] = &StructureTree{}
	}
	ph, args := inArgs(ids)
	prows, err := r.db.Query(`
		SELECT project_id, phase_id, name, status FROM project_phases
		WHERE project_id IN (`+ph+`) AND is_active=1
		ORDER BY project_id, seq, phase_id`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	currentSeen := map[string]bool{}
	for prows.Next() {
		var pid, id, name, status string
		if err := prows.Scan(&pid, &id, &name, &status); err != nil {
			prows.Close()
			return nil, err
		}
		tree := out[pid]
		node := PhaseNode{ID: id, Name: name, Display: statusLabel(status)}
		if status != model.PhaseDone && !currentSeen[pid] {
			node.Current = true
			currentSeen[pid] = true
		}
		tree.Phases = append(tree.Phases, node)
	}
	prows.Close()
	if err := prows.Err(); err != nil {
		return nil, err
	}
	krows, err := r.db.Query(`
		SELECT ph.project_id, pk.package_id, pk.phase_id, pk.name, pk.track_mode, pk.status, COALESCE(pk.next_note,'')
		FROM phase_packages pk
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id IN (`+ph+`) AND pk.is_active=1
		ORDER BY ph.project_id, pk.seq, pk.package_id`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	type pkgLoc struct {
		pid string
		pi  int
		ki  int
	}
	pkgAt := map[string]pkgLoc{}
	for krows.Next() {
		var pid, id, phase, name, track, status, note string
		if err := krows.Scan(&pid, &id, &phase, &name, &track, &status, &note); err != nil {
			krows.Close()
			return nil, err
		}
		tree := out[pid]
		for i := range tree.Phases {
			if tree.Phases[i].ID != phase {
				continue
			}
			tree.Has = true
			tree.Phases[i].Packages = append(tree.Phases[i].Packages, PkgNode{ID: id, Name: name, Track: track, NextNote: note, Display: status})
			pkgAt[id] = pkgLoc{pid: pid, pi: i, ki: len(tree.Phases[i].Packages) - 1}
		}
	}
	krows.Close()
	if err := krows.Err(); err != nil {
		return nil, err
	}
	lrows, err := r.db.Query(`
		SELECT ph.project_id, pl.plan_id, pl.package_id, pl.name, pl.status, pl.plan_end, pl.progress_basis, pl.qty_unit, COALESCE(pl.next_note,'')
		FROM package_plans pl
		JOIN phase_packages pk ON pk.package_id=pl.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id IN (`+ph+`) AND pl.is_active=1
		ORDER BY ph.project_id, pl.seq, pl.plan_id`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	for lrows.Next() {
		var pid, id, pkg, name, status, end, basis, unit, note string
		if err := lrows.Scan(&pid, &id, &pkg, &name, &status, &end, &basis, &unit, &note); err != nil {
			lrows.Close()
			return nil, err
		}
		loc, ok := pkgAt[pkg]
		if !ok {
			continue
		}
		node := &out[loc.pid].Phases[loc.pi].Packages[loc.ki]
		node.Plans = append(node.Plans, PlanNode{
			ID: id, Name: name, Status: status, PlanEnd: end, Basis: basis, Unit: unit,
			NextNote: note, PackageName: node.Name, Confirmed: status == model.PkgDone,
			Hold: status == model.PkgHold, Done: status == model.PkgDone,
		})
	}
	lrows.Close()
	if err := lrows.Err(); err != nil {
		return nil, err
	}
	srows, err := r.db.Query(`
		SELECT pk.package_id, COALESCE(ps.plan_id,''), ps.status, ps.target_qty, ps.done_qty
		FROM package_sites ps
		JOIN phase_packages pk ON pk.package_id=ps.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id IN (`+ph+`)`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	sites := map[string][]siteRow{}
	for srows.Next() {
		var s siteRow
		if err := srows.Scan(&s.pkg, &s.plan, &s.status, &s.target, &s.done); err != nil {
			srows.Close()
			return nil, err
		}
		sites[s.pkg] = append(sites[s.pkg], s)
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		return nil, err
	}
	for _, tree := range out {
		for pi := range tree.Phases {
			child := false
			for ki := range tree.Phases[pi].Packages {
				pk := &tree.Phases[pi].Packages[ki]
				built := buildPkg(pkgRow{id: pk.ID, name: pk.Name, track: pk.Track, status: "auto", next: pk.NextNote}, plansOf(pk.Plans), sites[pk.ID], nil, nil, "", bizAlways, tree.Phases[pi].Current)
				built.Milestones = pk.Milestones
				*pk = built
				if pk.Display == "진행 중" || pk.Display == "완료 후보" {
					child = true
				}
			}
			if tree.Phases[pi].Display != "완료" && child {
				tree.Phases[pi].Display = "진행 중"
			}
		}
	}
	return out, nil
}

func plansOf(nodes []PlanNode) []planRow {
	var out []planRow
	for _, n := range nodes {
		active := 1
		out = append(out, planRow{
			id: n.ID, name: n.Name, status: n.Status, planEnd: n.PlanEnd,
			basis: n.Basis, unit: n.Unit, next: n.NextNote, active: active,
		})
	}
	return out
}

func bizAlways(string) bool { return true }

func paceFloor(today, from string) string {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(today))
	if err != nil {
		return from
	}
	floor := t.AddDate(0, 0, -60).Format("2006-01-02")
	if strings.TrimSpace(from) != "" && from < floor {
		return from
	}
	return floor
}
