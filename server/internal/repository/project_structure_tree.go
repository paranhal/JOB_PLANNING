package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

type PlanNode struct {
	ID, Name, Status, Display, PlanEnd, Pace, PaceNote, Progress    string
	TaskN                                                           int
	Current, Hold, Done, Confirmed                                  bool
	Seq                                                             int
	DoneN, TotalN, Doing, QtyDone, QtyTotal, PeriodSites, PeriodQty int
	Basis, Unit, NextNote, PkgNote, PackageName                     string
	PaceView                                                        model.PaceResult
	Milestones                                                      []model.MilestoneView
}

type PkgNode struct {
	ID, Name, Track, Display, Progress, NextNote string
	Unassigned, TaskN, DoneN, TotalN             int
	// 수량만(track_mode=phase) 하부 업무: 목표·누적 수량. 사이트·계획 없이 몇 개 중 몇 개만 센다. 95-176 질문 1.
	QtyMode           bool
	QtyDone, QtyTotal int
	Unit, QtyLabel    string
	// 하부 업무 일자·배지(95-179). ActualStart 는 수동 입력값이 있으면 그 값, 없으면 연결 작업의 첫 완료일(자동)이다.
	PlanStart, PlanEnd, ActualStart string
	ActualAuto                      bool
	Badge, BadgeTone                string
	Plans                           []PlanNode
	Milestones                      []model.MilestoneView
}

type PhaseNode struct {
	ID, Name, Display string
	Current           bool
	Packages          []PkgNode
}

type StructureTree struct {
	Has          bool
	Unclassified int
	Phases       []PhaseNode
	Options      []PlanOption
	Report       model.ProgressReport
	ChartSVG     string
	// 단계별 실제 시작일(수동 입력값 우선, 없으면 하부 업무 시작일 중 가장 이른 날). 95-179C.
	PhaseStart map[string]EffStart
}

// EffStart 실제 시작일과 자동 계산 여부.
type EffStart struct {
	Value string
	Auto  bool
}

type PlanOption struct {
	PackageID, PlanID, Label, Status string
	Done, Hold                       bool
}

type siteRow struct {
	pkg, plan, status string
	target, done      int
}

func (r *ProjectRepo) LoadStructure(projectID, today string, biz func(string) bool) (StructureTree, error) {
	var out StructureTree
	if r == nil || strings.TrimSpace(projectID) == "" {
		return out, nil
	}
	phases, err := r.ListPhases(projectID)
	if err != nil {
		return out, err
	}
	pkgs, err := r.listPackages(projectID)
	if err != nil {
		return out, err
	}
	plans, err := r.listPlans(projectID)
	if err != nil {
		return out, err
	}
	sites, err := r.listSiteRows(projectID)
	if err != nil {
		return out, err
	}
	taskByPlan, taskByPkg, unclassified, err := r.taskCounts(projectID)
	if err != nil {
		return out, err
	}
	pkgQty, err := r.packageQtyDone(projectID)
	if err != nil {
		return out, err
	}
	out.Unclassified = unclassified
	out.Has = len(pkgs) > 0
	byPkgPlan := map[string][]siteRow{}
	for _, s := range sites {
		byPkgPlan[s.pkg] = append(byPkgPlan[s.pkg], s)
	}
	plansByPkg := map[string][]planRow{}
	for _, p := range plans {
		plansByPkg[p.pkg] = append(plansByPkg[p.pkg], p)
	}
	pkgsByPhase := map[string][]pkgRow{}
	for _, p := range pkgs {
		pkgsByPhase[p.phase] = append(pkgsByPhase[p.phase], p)
	}
	current := ""
	for _, ph := range phases {
		if ph.IsActive && ph.Status != model.PhaseDone {
			current = ph.PhaseID
			break
		}
	}
	autoStart, err := r.autoPackageStarts(projectID)
	if err != nil {
		return out, err
	}
	out.PhaseStart = map[string]EffStart{}
	for _, ph := range phases {
		node := PhaseNode{ID: ph.PhaseID, Name: ph.Name, Current: ph.PhaseID == current}
		childActivity := false
		earliest := ""
		for _, pk := range pkgsByPhase[ph.PhaseID] {
			pn := buildPkg(pk, plansByPkg[pk.id], byPkgPlan[pk.id], taskByPlan, taskByPkg, pkgQty[pk.id], today, biz, node.Current)
			// 실제 시작일: 수동 입력값이 있으면 그 값, 없으면 연결 작업의 첫 완료일(자동). 95-179C.
			if pn.ActualStart == "" && autoStart[pk.id] != "" {
				pn.ActualStart, pn.ActualAuto = autoStart[pk.id], true
			}
			if pn.ActualStart != "" && (earliest == "" || pn.ActualStart < earliest) {
				earliest = pn.ActualStart
			}
			if pn.Display == "진행 중" || pn.Display == "완료 후보" {
				childActivity = true
			}
			node.Packages = append(node.Packages, pn)
			for _, pl := range pn.Plans {
				out.Options = append(out.Options, PlanOption{
					PackageID: pk.id, PlanID: pl.ID,
					Label:  ph.Name + " › " + pk.name + " › " + pl.Name,
					Status: pl.Status, Done: pl.Done, Hold: pl.Hold,
				})
			}
			if pk.track == model.TrackModePhase {
				out.Options = append(out.Options, PlanOption{
					PackageID: pk.id, Label: ph.Name + " › " + pk.name, Status: pk.status,
				})
			}
		}
		node.Display = phaseDisplay(ph.Status, childActivity)
		if strings.TrimSpace(ph.ActualStart) != "" {
			out.PhaseStart[ph.PhaseID] = EffStart{Value: ph.ActualStart}
		} else if earliest != "" {
			out.PhaseStart[ph.PhaseID] = EffStart{Value: earliest, Auto: true}
		}
		out.Phases = append(out.Phases, node)
	}
	return out, nil
}

func phaseDisplay(stored string, child bool) string {
	switch stored {
	case model.PhaseDone:
		return "완료"
	case model.PhaseActive:
		return "진행 중"
	}
	if child {
		return "진행 중"
	}
	return "예정"
}

type pkgRow struct {
	id, phase, name, track, status, next string
	seq                                  int
	qtyUnit                              string
	qtyTotal                             int
	planStart, planEnd, actualStart      string
	badge                                string
}
type planRow struct {
	id, pkg, name, status, planEnd, basis, unit, next string
	seq                                               int
	active                                            int
}

func (r *ProjectRepo) listPackages(projectID string) ([]pkgRow, error) {
	rows, err := r.db.Query(`
		SELECT pk.package_id, pk.phase_id, pk.name, pk.track_mode, pk.status, COALESCE(pk.next_note,''), pk.seq,
		       COALESCE(pk.qty_unit,''), COALESCE(pk.qty_total,0),
		       COALESCE(pk.plan_start,''), COALESCE(pk.plan_end,''), COALESCE(pk.actual_start,''), COALESCE(pk.badge,'')
		FROM phase_packages pk
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=? AND pk.is_active=1
		ORDER BY pk.seq, pk.package_id`, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []pkgRow
	for rows.Next() {
		var p pkgRow
		if err := rows.Scan(&p.id, &p.phase, &p.name, &p.track, &p.status, &p.next, &p.seq, &p.qtyUnit, &p.qtyTotal, &p.planStart, &p.planEnd, &p.actualStart, &p.badge); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) listPlans(projectID string) ([]planRow, error) {
	rows, err := r.db.Query(`
		SELECT pl.plan_id, pl.package_id, pl.name, pl.status, pl.plan_end, pl.progress_basis, pl.qty_unit, pl.seq, pl.is_active, COALESCE(pl.next_note,'')
		FROM package_plans pl
		JOIN phase_packages pk ON pk.package_id=pl.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=?
		ORDER BY pl.seq, pl.plan_id`, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []planRow
	for rows.Next() {
		var p planRow
		if err := rows.Scan(&p.id, &p.pkg, &p.name, &p.status, &p.planEnd, &p.basis, &p.unit, &p.seq, &p.active, &p.next); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) listSiteRows(projectID string) ([]siteRow, error) {
	rows, err := r.db.Query(`
		SELECT ps.package_id, ps.plan_id, ps.status, ps.target_qty, ps.done_qty
		FROM package_sites ps
		JOIN phase_packages pk ON pk.package_id=ps.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=?`, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []siteRow
	for rows.Next() {
		var s siteRow
		if err := rows.Scan(&s.pkg, &s.plan, &s.status, &s.target, &s.done); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) taskCounts(projectID string) (map[string]int, map[string]int, int, error) {
	byPlan, byPkg := map[string]int{}, map[string]int{}
	rows, err := r.db.Query(`
		SELECT COALESCE(plan_id,''), COALESCE(package_id,''), COUNT(*)
		FROM work_tasks WHERE project_id=? GROUP BY plan_id, package_id`, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no such column") {
			return byPlan, byPkg, 0, nil
		}
		return nil, nil, 0, err
	}
	defer rows.Close()
	un := 0
	for rows.Next() {
		var plan, pkg string
		var n int
		if err := rows.Scan(&plan, &pkg, &n); err != nil {
			return nil, nil, 0, err
		}
		if plan != "" {
			byPlan[plan] += n
		}
		if pkg != "" {
			byPkg[pkg] += n
		}
		if plan == "" && pkg == "" {
			un += n
		}
	}
	return byPlan, byPkg, un, rows.Err()
}

func buildPkg(pk pkgRow, plans []planRow, sites []siteRow, byPlan, byPkg map[string]int, qtyDone int, today string, biz func(string) bool, current bool) PkgNode {
	node := PkgNode{ID: pk.id, Name: pk.name, Track: pk.track, TaskN: byPkg[pk.id], NextNote: pk.next,
		PlanStart: pk.planStart, PlanEnd: pk.planEnd, ActualStart: pk.actualStart, Badge: pk.badge}
	if pk.badge != "" {
		node.BadgeTone = model.BadgeTone(pk.badge)
	}
	sd, st, qd, qt := tally(sites, "")
	node.DoneN, node.TotalN = sd, st
	node.Progress = model.BothRateText(sd, st, qd, qt, "")
	for _, s := range sites {
		if s.plan == "" && s.status != model.SiteExcluded {
			node.Unassigned++
		}
	}
	act := byPkg[pk.id] > 0
	for _, s := range sites {
		if s.status == model.SiteInProgress || s.status == model.SiteDone {
			act = true
		}
	}
	all := st > 0 && sd == st
	if pk.track == model.TrackModePhase {
		unit := strings.TrimSpace(pk.qtyUnit)
		if unit == "" {
			unit = "식"
		}
		node.QtyMode, node.QtyDone, node.QtyTotal, node.Unit = true, qtyDone, pk.qtyTotal, unit
		if pk.qtyTotal > 0 {
			node.QtyLabel = fmt.Sprintf("%s/%s%s (%s)", model.FormatIntComma(qtyDone), model.FormatIntComma(pk.qtyTotal), unit, model.SiteCheckRateLabel(qtyDone, pk.qtyTotal))
		} else {
			node.QtyLabel = "목표 수량 미설정"
		}
		node.Progress = node.QtyLabel
		act = act || qtyDone > 0
		all = pk.qtyTotal > 0 && qtyDone >= pk.qtyTotal
	}
	node.Display = statusLabel(model.EffectiveStatus(pk.status, act, all))
	for _, pl := range plans {
		if pl.active == 0 {
			continue
		}
		var mine []siteRow
		for _, s := range sites {
			if s.plan == pl.id {
				mine = append(mine, s)
			}
		}
		psd, pst, pqd, pqt := tally(mine, pl.id)
		doing := 0
		pact := byPlan[pl.id] > 0
		for _, s := range mine {
			if s.status == model.SiteInProgress || s.status == model.SiteDone {
				pact = true
			}
			if s.status == model.SiteInProgress {
				doing++
			}
		}
		disp := model.EffectiveStatus(pl.status, pact, pst > 0 && psd == pst)
		pn := PlanNode{
			ID: pl.id, Name: pl.name, Status: pl.status, Display: statusLabel(disp),
			PlanEnd: pl.planEnd, TaskN: byPlan[pl.id], Seq: pl.seq,
			Progress: model.BothRateText(psd, pst, pqd, pqt, pl.unit),
			Pace:     model.PlanPace(pl.planEnd, today, psd, pst, 0, 0, biz),
			Hold:     pl.status == model.PkgHold, Done: pl.status == model.PkgDone,
			Confirmed: pl.status == model.PkgDone,
			Current:   current && disp == model.PhaseActive,
			DoneN:     psd, TotalN: pst, Doing: doing, QtyDone: pqd, QtyTotal: pqt,
			Basis: pl.basis, Unit: pl.unit, NextNote: pl.next, PkgNote: pk.next, PackageName: pk.name,
		}
		node.Plans = append(node.Plans, pn)
	}
	return node
}

func tally(rows []siteRow, planID string) (done, total, qtyDone, qtyTotal int) {
	for _, s := range rows {
		if planID != "" && s.plan != planID {
			continue
		}
		if s.status == model.SiteExcluded {
			continue
		}
		total++
		if s.status == model.SiteDone {
			done++
		}
		qtyTotal += s.target
		qtyDone += model.QtyCredit(s.status, s.target, s.done)
	}
	return
}

func statusLabel(code string) string {
	switch code {
	case model.PhaseDone:
		return "완료"
	case model.PkgHold:
		return "보류"
	case model.PhaseActive:
		return "진행 중"
	case model.StatusCandidate:
		return "완료 후보"
	default:
		return "예정"
	}
}

type StructureProgress struct {
	Label, Current        string
	Pct, Reflected, Total int
	SiteDone, SiteTotal   int
}

func (r *ProjectRepo) StructureProgress(projectIDs []string) (map[string]StructureProgress, error) {
	if r == nil || len(projectIDs) == 0 {
		return map[string]StructureProgress{}, nil
	}
	return r.structureProgressFold(projectIDs)
}

func (r *ProjectRepo) structureProgressFold(projectIDs []string) (map[string]StructureProgress, error) {
	out := map[string]StructureProgress{}
	ph := make([]string, len(projectIDs))
	args := make([]interface{}, len(projectIDs))
	for i, id := range projectIDs {
		ph[i] = "?"
		args[i] = id
		out[id] = StructureProgress{Label: "—", Current: "—"}
	}
	prows, err := r.db.Query(`
		SELECT project_id, name, status FROM project_phases
		WHERE project_id IN (`+strings.Join(ph, ",")+`) AND is_active=1
		ORDER BY project_id, seq`, args...)
	if err != nil {
		return nil, err
	}
	cur := map[string]string{}
	for prows.Next() {
		var id, name, st string
		if err := prows.Scan(&id, &name, &st); err != nil {
			prows.Close()
			return nil, err
		}
		if _, ok := cur[id]; !ok && st != model.PhaseDone {
			cur[id] = name
		}
	}
	prows.Close()
	rows, err := r.db.Query(`
		SELECT ph.project_id, pk.package_id, pk.track_mode, COALESCE(ps.status,''), COALESCE(ps.target_qty,0), COALESCE(ps.done_qty,0)
		FROM phase_packages pk
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		LEFT JOIN package_sites ps ON ps.package_id=pk.package_id
		WHERE ph.project_id IN (`+strings.Join(ph, ",")+`) AND pk.is_active=1 AND ph.is_active=1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type pack struct {
		track       string
		done, total int
		seen        bool
	}
	packs := map[string]*pack{}
	order := map[string][]string{}
	for rows.Next() {
		var pid, pkg, track, st string
		var target, done int
		if err := rows.Scan(&pid, &pkg, &track, &st, &target, &done); err != nil {
			return nil, err
		}
		k := pid + "\t" + pkg
		p := packs[k]
		if p == nil {
			p = &pack{track: track}
			packs[k] = p
			order[pid] = append(order[pid], k)
		}
		if st == "" {
			continue
		}
		if st == model.SiteExcluded {
			continue
		}
		p.total++
		if st == model.SiteDone {
			p.done++
		}
		_ = target
	}
	for id, sp := range out {
		sp.Current = "—"
		if c := cur[id]; c != "" {
			sp.Current = c
		}
		ref := 0
		for _, k := range order[id] {
			p := packs[k]
			sp.Total++
			if p.track == model.TrackModePhase {
				continue
			}
			if p.total == 0 {
				continue
			}
			ref++
			sp.SiteDone += p.done
			sp.SiteTotal += p.total
		}
		sp.Reflected = ref
		// 사업 진행률은 사이트 합계(완료 ÷ 대상)다. 하부 업무 비중으로 가중평균하던 값은 어차피 이 값으로 덮어써져
		// 화면에 남지 않았다(95-176 비중 정리). 수량만(track_mode=phase)·사이트 0곳 하부 업무는 반영에서 빠진다.
		if sp.SiteTotal > 0 {
			sp.Label = model.SiteCheckRateLabel(sp.SiteDone, sp.SiteTotal)
			sp.Pct = sp.SiteDone * 100 / sp.SiteTotal
		}
		out[id] = sp
	}
	return out, rows.Err()
}
