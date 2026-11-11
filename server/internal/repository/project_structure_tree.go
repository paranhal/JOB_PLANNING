package repository

import (
	"fmt"
	"sort"
	"strings"

	"customer-support/internal/model"
)

type PlanNode struct {
	ID, Name, Status, Display, PlanEnd, Pace, Progress string
	TaskN                                              int
	Current, Hold, Done                                bool
	Seq                                                int
}

type PkgNode struct {
	ID, Name, Track, Display, Progress string
	Unassigned, TaskN                  int
	Plans                              []PlanNode
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
	for _, ph := range phases {
		node := PhaseNode{ID: ph.PhaseID, Name: ph.Name, Current: ph.PhaseID == current}
		childActivity := false
		for _, pk := range pkgsByPhase[ph.PhaseID] {
			pn := buildPkg(pk, plansByPkg[pk.id], byPkgPlan[pk.id], taskByPlan, taskByPkg, today, biz, node.Current)
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
	id, phase, name, track, status string
	weight                         float64
}
type planRow struct {
	id, pkg, name, status, planEnd, basis, unit string
	seq                                         int
	active                                      int
}

func (r *ProjectRepo) listPackages(projectID string) ([]pkgRow, error) {
	rows, err := r.db.Query(`
		SELECT pk.package_id, pk.phase_id, pk.name, pk.track_mode, pk.status, pk.weight
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
		if err := rows.Scan(&p.id, &p.phase, &p.name, &p.track, &p.status, &p.weight); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) listPlans(projectID string) ([]planRow, error) {
	rows, err := r.db.Query(`
		SELECT pl.plan_id, pl.package_id, pl.name, pl.status, pl.plan_end, pl.progress_basis, pl.qty_unit, pl.seq, pl.is_active
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
		if err := rows.Scan(&p.id, &p.pkg, &p.name, &p.status, &p.planEnd, &p.basis, &p.unit, &p.seq, &p.active); err != nil {
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

func buildPkg(pk pkgRow, plans []planRow, sites []siteRow, byPlan, byPkg map[string]int, today string, biz func(string) bool, current bool) PkgNode {
	node := PkgNode{ID: pk.id, Name: pk.name, Track: pk.track, TaskN: byPkg[pk.id]}
	sd, st, qd, qt := tally(sites, "")
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
		pact := byPlan[pl.id] > 0
		for _, s := range mine {
			if s.status == model.SiteInProgress || s.status == model.SiteDone {
				pact = true
			}
		}
		disp := model.EffectiveStatus(pl.status, pact, pst > 0 && psd == pst)
		pn := PlanNode{
			ID: pl.id, Name: pl.name, Status: pl.status, Display: statusLabel(disp),
			PlanEnd: pl.planEnd, TaskN: byPlan[pl.id], Seq: pl.seq,
			Progress: model.BothRateText(psd, pst, pqd, pqt, pl.unit),
			Pace:     model.PlanPace(pl.planEnd, today, psd, pst, 0, 0, biz),
			Hold:     pl.status == model.PkgHold, Done: pl.status == model.PkgDone,
			Current: current && disp == model.PhaseActive,
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
		SELECT ph.project_id, pk.package_id, pk.track_mode, pk.weight, COALESCE(ps.status,''), COALESCE(ps.target_qty,0), COALESCE(ps.done_qty,0)
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
		weight      float64
		done, total int
		seen        bool
	}
	packs := map[string]*pack{}
	order := map[string][]string{}
	for rows.Next() {
		var pid, pkg, track, st string
		var weight float64
		var target, done int
		if err := rows.Scan(&pid, &pkg, &track, &weight, &st, &target, &done); err != nil {
			return nil, err
		}
		k := pid + "\t" + pkg
		p := packs[k]
		if p == nil {
			p = &pack{track: track, weight: weight}
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
		var wSum, wRate float64
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
			w := p.weight
			if w <= 0 {
				w = 1
			}
			wSum += w
			wRate += w * float64(p.done) / float64(p.total)
			sp.SiteDone += p.done
			sp.SiteTotal += p.total
		}
		sp.Reflected = ref
		if wSum > 0 {
			sp.Pct = int(wRate / wSum * 100)
			sp.Label = model.SiteCheckRateLabel(sp.Pct, 100)
			if sp.SiteTotal > 0 {
				sp.Label = model.SiteCheckRateLabel(sp.SiteDone, sp.SiteTotal)
				sp.Pct = sp.SiteDone * 100 / sp.SiteTotal
			}
		}
		out[id] = sp
	}
	return out, rows.Err()
}

func (r *ProjectRepo) CopyStructure(toID, fromID, orgID string) (int, error) {
	src, err := r.ListPhases(fromID)
	if err != nil {
		return 0, err
	}
	if len(src) == 0 {
		return 0, fmt.Errorf("복사할 구조가 없습니다")
	}
	dst, err := r.ListPhases(toID)
	if err != nil {
		return 0, err
	}
	have := map[string]string{}
	for _, p := range dst {
		have[model.NormalizePhaseMergeName(p.Name)] = p.PhaseID
	}
	n := 0
	for _, p := range src {
		if !p.IsActive {
			continue
		}
		key := model.NormalizePhaseMergeName(p.Name)
		phaseID := have[key]
		if phaseID == "" {
			row := &model.ProjectPhase{ProjectID: toID, OrgID: orgID, Name: p.Name, Status: model.PhasePlanned}
			if err := r.CreatePhase(row); err != nil {
				return n, err
			}
			phaseID = row.PhaseID
			have[key] = phaseID
			n++
		}
		pkgs, err := r.listPackages(fromID)
		if err != nil {
			return n, err
		}
		exist, _ := r.listPackages(toID)
		names := map[string]bool{}
		for _, e := range exist {
			if e.phase == phaseID {
				names[model.NormalizePlanMergeName(e.name)] = true
			}
		}
		for _, pk := range pkgs {
			if pk.phase != p.PhaseID {
				continue
			}
			if names[model.NormalizePlanMergeName(pk.name)] {
				continue
			}
			if _, err := r.CreatePackage(phaseID, pk.name, pk.track, pk.weight); err != nil {
				return n, err
			}
			n++
		}
	}
	sort.Slice(src, func(i, j int) bool { return src[i].Seq < src[j].Seq })
	return n, nil
}
