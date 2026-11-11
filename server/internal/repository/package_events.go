package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

type PlanProgressView struct {
	Chip, PlanID, ProjectID string
	Done, Total             int
	AllDone                 bool
}

type OpenSite struct {
	CustomerID string `json:"customer_id"`
	Name       string `json:"name"`
	Region     string `json:"region"`
	Status     string `json:"status"`
	LastDate   string `json:"last_date"`
	LastUser   string `json:"last_user"`
	Assigned   string `json:"assigned"`
	Continue   bool   `json:"continue"`
}

type OpenSitePage struct {
	Sites     []OpenSite `json:"sites"`
	Done      int        `json:"done"`
	Total     int        `json:"total"`
	Name      string     `json:"name"`
	Preview   string     `json:"preview"`
	Truncated bool       `json:"truncated"`
}

type openRow struct {
	OpenSite
	psid string
}

func (r *WBRepo) HasExecutionDuty(projectID, userID string) bool {
	if r == nil || r.db == nil {
		return false
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_assignments WHERE project_id=? AND user_id=? AND duty=?`,
		strings.TrimSpace(projectID), strings.TrimSpace(userID), model.ProjectDutyExecution).Scan(&n)
	return n > 0
}

func (r *WBRepo) PlanProgressView(taskID string) PlanProgressView {
	var v PlanProgressView
	if r == nil || r.db == nil {
		return v
	}
	var plan, pkg, project string
	err := r.db.QueryRow(`SELECT COALESCE(plan_id,''), COALESCE(package_id,''), COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&plan, &pkg, &project)
	if err != nil {
		return v
	}
	v.PlanID = plan
	v.ProjectID = project
	if plan == "" && pkg == "" {
		return v
	}
	done, total, _, _ := countPackageSites(r.db, pkg, plan)
	v.Done, v.Total = done, total
	var planName, pkgName, status string
	if plan != "" {
		_ = r.db.QueryRow(`
			SELECT pl.name, pk.name, pl.status
			FROM package_plans pl JOIN phase_packages pk ON pk.package_id=pl.package_id
			WHERE pl.plan_id=?`, plan).Scan(&planName, &pkgName, &status)
		v.Chip = formatPlanChip(planName, pkgName, done, total)
	} else {
		_ = r.db.QueryRow(`SELECT name, status FROM phase_packages WHERE package_id=?`, pkg).Scan(&pkgName, &status)
		v.Chip = formatPlanChip(pkgName, "", done, total)
	}
	v.AllDone = total > 0 && done == total && status != model.PkgDone
	return v
}

func formatPlanChip(plan, pkg string, done, total int) string {
	label := strings.TrimSpace(plan)
	if label != "" && strings.TrimSpace(pkg) != "" {
		label = label + " " + strings.TrimSpace(pkg)
	}
	if label == "" {
		label = strings.TrimSpace(pkg)
	}
	if total <= 0 {
		if label == "" {
			return "현재 —"
		}
		return label + " · 현재 —"
	}
	return fmt.Sprintf("%s · 현재 %d/%d (%s)", label, done, total, model.SiteCheckRateLabel(done, total))
}

func (r *WBRepo) SitesOffPlan(taskID string) map[string]bool {
	out := map[string]bool{}
	if r == nil || r.db == nil {
		return out
	}
	var plan, project string
	if err := r.db.QueryRow(`SELECT COALESCE(plan_id,''), COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&plan, &project); err != nil || plan == "" {
		return out
	}
	rows, err := r.db.Query(`
		SELECT w.customer_id FROM work_task_sites w
		WHERE w.task_id=? AND TRIM(COALESCE(w.customer_id,''))!=''
		  AND NOT EXISTS (
		    SELECT 1 FROM package_sites ps
		    JOIN project_sites s ON s.id=ps.project_site_id
		    WHERE s.project_id=? AND s.customer_id=w.customer_id AND ps.plan_id=? AND ps.status!=?
		  )`, taskID, project, plan, model.SiteExcluded)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil && id != "" {
			out[id] = true
		}
	}
	return out
}

func (r *WBRepo) PlanSiteCheckRate(planID string) (done, total int, label string) {
	planID = strings.TrimSpace(planID)
	if r == nil || r.db == nil || planID == "" {
		return 0, 0, "—"
	}
	rows, err := r.db.Query(`
		SELECT s.customer_id, ps.status
		FROM package_sites ps
		JOIN project_sites s ON s.id=ps.project_site_id
		WHERE ps.plan_id=? AND TRIM(COALESCE(s.customer_id,''))!=''`, planID)
	if err != nil {
		return 0, 0, "—"
	}
	type st struct{ status string }
	sites := map[string]st{}
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			rows.Close()
			return 0, 0, "—"
		}
		sites[id] = st{status: status}
	}
	rows.Close()
	checks, err := r.db.Query(`
		SELECT p.customer_id, p.status
		FROM work_task_site_progress p
		JOIN work_tasks t ON t.task_id=p.task_id
		WHERE t.plan_id=?`, planID)
	if err != nil {
		return 0, 0, "—"
	}
	doneSite := map[string]bool{}
	naSite := map[string]bool{}
	other := map[string]bool{}
	for checks.Next() {
		var id, status string
		if err := checks.Scan(&id, &status); err != nil {
			checks.Close()
			return 0, 0, "—"
		}
		switch status {
		case model.SiteCheckDone:
			doneSite[id] = true
		case model.SiteCheckNA:
			naSite[id] = true
		default:
			other[id] = true
		}
	}
	checks.Close()
	for id, s := range sites {
		if s.status == model.SiteExcluded {
			continue
		}
		if doneSite[id] {
			done++
			total++
			continue
		}
		if naSite[id] && !other[id] {
			continue
		}
		total++
	}
	return done, total, model.SiteCheckRateLabel(done, total)
}

func (r *WBRepo) RebuildPackageProgress(packageID string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("저장소가 없습니다")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := rebuildPackageSnapsTx(tx, packageID); err != nil {
		return err
	}
	return tx.Commit()
}

func reflectPackageSiteTx(tx *sql.Tx, taskID, customerID, workDate, check, userID string, qty int) error {
	id, pkg, plan, oldStatus, target, oldDone, ok, err := findPackageSiteTx(tx, taskID, customerID)
	if err != nil || !ok {
		return err
	}
	newStatus, newDone, ev, apply := nextSiteState(check, qty, target, oldDone, oldStatus)
	if !apply || (newStatus == oldStatus && newDone == oldDone) {
		return nil
	}
	reason := ""
	if newStatus == model.SiteExcluded {
		reason = "해당없음"
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := tx.Exec(`UPDATE package_sites SET status=?, done_qty=?, exclude_reason=?, updated_at=? WHERE package_site_id=?`,
		newStatus, newDone, reason, now, id); err != nil {
		return err
	}
	delta := model.QtyCredit(newStatus, target, newDone) - model.QtyCredit(oldStatus, target, oldDone)
	var n int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM package_events
		WHERE work_task_id=? AND package_site_id=? AND event_date=? AND event_type=? AND qty_delta=?`,
		taskID, id, workDate, ev, delta).Scan(&n); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	if n == 0 {
		seq, err := nextSeqTx(tx, "package_event")
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO package_events(
				event_id, package_id, package_site_id, plan_id, event_date, event_type,
				qty_delta, user_id, work_task_id, source, note, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,'work','',?)`,
			fmt.Sprintf("EV-%03d", seq), pkg, id, plan, workDate, ev, delta, strings.TrimSpace(userID), taskID, now); err != nil {
			return err
		}
	}
	return rebuildPackageSnapsTx(tx, pkg)
}

func findPackageSiteTx(tx *sql.Tx, taskID, customerID string) (id, pkg, plan, status string, target, done int, ok bool, err error) {
	var project, taskPkg, taskPlan string
	err = tx.QueryRow(`SELECT COALESCE(project_id,''), COALESCE(package_id,''), COALESCE(plan_id,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&project, &taskPkg, &taskPlan)
	if err == sql.ErrNoRows {
		return "", "", "", "", 0, 0, false, nil
	}
	if err != nil {
		return "", "", "", "", 0, 0, false, err
	}
	if project == "" || (taskPlan == "" && taskPkg == "") {
		return "", "", "", "", 0, 0, false, nil
	}
	q := `
		SELECT ps.package_site_id, ps.package_id, COALESCE(ps.plan_id,''), ps.status, ps.target_qty, ps.done_qty
		FROM project_sites s
		JOIN package_sites ps ON ps.project_site_id=s.id
		WHERE s.project_id=? AND s.customer_id=? AND s.is_active=1`
	args := []any{project, customerID}
	if taskPlan != "" {
		q += ` AND ps.plan_id=?`
		args = append(args, taskPlan)
	} else {
		q += ` AND ps.package_id=? AND TRIM(COALESCE(ps.plan_id,''))=''`
		args = append(args, taskPkg)
	}
	q += ` LIMIT 1`
	err = tx.QueryRow(q, args...).Scan(&id, &pkg, &plan, &status, &target, &done)
	if err == sql.ErrNoRows {
		return "", "", "", "", 0, 0, false, nil
	}
	if err != nil {
		if strings.Contains(err.Error(), "no such") {
			return "", "", "", "", 0, 0, false, nil
		}
		return "", "", "", "", 0, 0, false, err
	}
	return id, pkg, plan, status, target, done, true, nil
}

func nextSiteState(check string, qty, target, oldDone int, oldStatus string) (status string, done int, ev string, apply bool) {
	switch check {
	case model.SiteCheckDone:
		if target < 0 {
			target = 0
		}
		return model.SiteDone, target, "complete", true
	case model.SiteCheckNA:
		return model.SiteExcluded, 0, "exclude", true
	default:
		if qty > 0 {
			d := qty
			if target > 0 && d > target {
				d = target
			}
			ev = "progress"
			if oldStatus == model.SitePlanned || oldStatus == "" {
				ev = "start"
			}
			return model.SiteInProgress, d, ev, true
		}
		if oldStatus == model.SitePlanned || oldStatus == "" {
			return oldStatus, oldDone, "", false
		}
		return model.SitePlanned, 0, "revert", true
	}
}

type snapSite struct {
	id, plan, status string
	target, done     int
	touched          bool
}

type snapEvent struct {
	site, plan, date, typ string
	delta                 int
}

func rebuildPackageSnapsTx(tx *sql.Tx, packageID string) error {
	packageID = strings.TrimSpace(packageID)
	if packageID == "" {
		return nil
	}
	rows, err := tx.Query(`SELECT package_site_id, COALESCE(plan_id,''), status, target_qty, done_qty FROM package_sites WHERE package_id=?`, packageID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	var sites []snapSite
	for rows.Next() {
		var s snapSite
		if err := rows.Scan(&s.id, &s.plan, &s.status, &s.target, &s.done); err != nil {
			rows.Close()
			return err
		}
		sites = append(sites, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	evRows, err := tx.Query(`
		SELECT package_site_id, COALESCE(plan_id,''), event_date, event_type, qty_delta
		FROM package_events WHERE package_id=?
		ORDER BY event_date, created_at, event_id`, packageID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	var events []snapEvent
	for evRows.Next() {
		var e snapEvent
		if err := evRows.Scan(&e.site, &e.plan, &e.date, &e.typ, &e.delta); err != nil {
			evRows.Close()
			return err
		}
		events = append(events, e)
	}
	evRows.Close()
	if err := evRows.Err(); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM package_progress_daily WHERE package_id=?`, packageID); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	if len(events) == 0 {
		return nil
	}
	touched := map[string]bool{}
	for _, e := range events {
		touched[e.site] = true
	}
	cur := map[string]*snapSite{}
	for i := range sites {
		s := sites[i]
		if touched[s.id] {
			s.status = model.SitePlanned
			s.done = 0
		}
		cur[s.id] = &s
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	i := 0
	for i < len(events) {
		date := events[i].date
		daySites := map[string]int{}
		dayQty := map[string]int{}
		for i < len(events) && events[i].date == date {
			e := events[i]
			i++
			if s := cur[e.site]; s != nil {
				applySnapEvent(s, e)
			}
			if e.typ == "complete" {
				daySites[e.plan]++
				daySites[""]++
			}
			if e.delta > 0 {
				dayQty[e.plan] += e.delta
				dayQty[""] += e.delta
			}
		}
		plans := map[string]bool{"": true}
		for _, s := range cur {
			plans[s.plan] = true
		}
		list := make([]snapSite, 0, len(cur))
		for _, s := range cur {
			list = append(list, *s)
		}
		for plan := range plans {
			total, done, doing, qtyTotal, qtyDone := countSnap(list, plan)
			if _, err := tx.Exec(`
				INSERT INTO package_progress_daily(
					package_id, plan_id, snap_date, total_sites, done_sites, doing_sites,
					total_qty, done_qty, day_done_sites, day_done_qty, updated_at)
				VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
				packageID, plan, date, total, done, doing, qtyTotal, qtyDone, daySites[plan], dayQty[plan], now); err != nil {
				return err
			}
		}
	}
	return nil
}

func applySnapEvent(s *snapSite, e snapEvent) {
	switch e.typ {
	case "complete":
		s.status = model.SiteDone
		if s.target > 0 {
			s.done = s.target
		}
	case "exclude":
		s.status = model.SiteExcluded
		s.done = 0
	case "revert":
		s.status = model.SitePlanned
		s.done = 0
	default:
		s.status = model.SiteInProgress
		next := s.done + e.delta
		if next < 0 {
			next = 0
		}
		if s.target > 0 && next > s.target {
			next = s.target
		}
		s.done = next
	}
}

func countSnap(list []snapSite, plan string) (total, done, doing, qtyTotal, qtyDone int) {
	for _, s := range list {
		if plan != "" && s.plan != plan {
			continue
		}
		if s.status == model.SiteExcluded {
			continue
		}
		total++
		if s.status == model.SiteDone {
			done++
		}
		if s.status == model.SiteInProgress {
			doing++
		}
		qtyTotal += s.target
		qtyDone += model.QtyCredit(s.status, s.target, s.done)
	}
	return
}

func countPackageSites(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, pkg, plan string) (done, total, qtyDone, qtyTotal int) {
	sqlText := `SELECT status, target_qty, done_qty FROM package_sites WHERE plan_id=?`
	arg := any(plan)
	if plan == "" {
		sqlText = `SELECT status, target_qty, done_qty FROM package_sites WHERE package_id=? AND TRIM(COALESCE(plan_id,''))=''`
		arg = pkg
	}
	rows, err := q.Query(sqlText, arg)
	if err != nil {
		return 0, 0, 0, 0
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var target, doneQty int
		if err := rows.Scan(&status, &target, &doneQty); err != nil {
			return done, total, qtyDone, qtyTotal
		}
		if status == model.SiteExcluded {
			continue
		}
		total++
		if status == model.SiteDone {
			done++
		}
		qtyTotal += target
		qtyDone += model.QtyCredit(status, target, doneQty)
	}
	return done, total, qtyDone, qtyTotal
}

func (r *ProjectRepo) ListOpenSites(projectID, packageID, planID, q, region, today string, limit int) (OpenSitePage, error) {
	var page OpenSitePage
	if r == nil || r.db == nil {
		return page, fmt.Errorf("저장소가 없습니다")
	}
	projectID = strings.TrimSpace(projectID)
	packageID = strings.TrimSpace(packageID)
	planID = strings.TrimSpace(planID)
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var name string
	var err error
	if planID != "" {
		err = r.db.QueryRow(`
			SELECT pl.name FROM package_plans pl
			JOIN phase_packages pk ON pk.package_id=pl.package_id
			JOIN project_phases ph ON ph.phase_id=pk.phase_id
			WHERE pl.plan_id=? AND ph.project_id=? AND pl.is_active=1`, planID, projectID).Scan(&name)
	} else if packageID != "" {
		err = r.db.QueryRow(`
			SELECT pk.name FROM phase_packages pk
			JOIN project_phases ph ON ph.phase_id=pk.phase_id
			WHERE pk.package_id=? AND ph.project_id=? AND pk.is_active=1`, packageID, projectID).Scan(&name)
	} else {
		return page, nil
	}
	if err != nil {
		return page, nil
	}
	page.Name = name
	page.Done, page.Total, _, _ = countPackageSites(r.db, packageID, planID)
	sqlText := `
		SELECT s.customer_id, COALESCE(NULLIF(TRIM(s.site_name),''), s.customer_id), COALESCE(s.region,''), ps.status, ps.package_site_id
		FROM package_sites ps
		JOIN project_sites s ON s.id=ps.project_site_id
		WHERE s.project_id=? AND s.is_active=1 AND TRIM(COALESCE(s.customer_id,''))!=''
		  AND ps.status IN (?, ?)`
	args := []any{projectID, model.SitePlanned, model.SiteInProgress}
	if planID != "" {
		sqlText += ` AND ps.plan_id=?`
		args = append(args, planID)
	} else {
		sqlText += ` AND ps.package_id=? AND TRIM(COALESCE(ps.plan_id,''))=''`
		args = append(args, packageID)
	}
	rows, err := r.db.Query(sqlText, args...)
	if err != nil {
		return page, err
	}
	var all []openRow
	for rows.Next() {
		var it openRow
		if err := rows.Scan(&it.CustomerID, &it.Name, &it.Region, &it.Status, &it.psid); err != nil {
			rows.Close()
			return page, err
		}
		if !model.SiteImportMatch(it.Name, "", "", q, "") || !model.RegionAuxMatch(it.Region, region) {
			continue
		}
		it.Continue = it.Status == model.SiteInProgress
		all = append(all, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return page, err
	}
	sortOpen(all)
	if len(all) > limit {
		page.Truncated = true
		all = all[:limit]
	}
	if len(all) > 0 {
		fillOpenMeta(r.db, all, today)
	}
	for _, it := range all {
		page.Sites = append(page.Sites, it.OpenSite)
	}
	return page, nil
}

func sortOpen(all []openRow) {
	// in_progress first, then name. Local type alias via index sort.
	for i := 1; i < len(all); i++ {
		j := i
		for j > 0 {
			a, b := all[j-1], all[j]
			swap := false
			if a.Continue != b.Continue {
				swap = b.Continue
			} else if a.Name > b.Name {
				swap = true
			}
			if !swap {
				break
			}
			all[j-1], all[j] = all[j], all[j-1]
			j--
		}
	}
}

func fillOpenMeta(db *sql.DB, all []openRow, today string) {
	var ph []string
	var args []any
	var cph []string
	var cargs []any
	for i := range all {
		ph = append(ph, "?")
		args = append(args, all[i].psid)
		cph = append(cph, "?")
		cargs = append(cargs, all[i].CustomerID)
	}
	rows, err := db.Query(`
		SELECT e.package_site_id, e.event_date, COALESCE(u.full_name,'')
		FROM package_events e
		LEFT JOIN users u ON u.user_id=e.user_id
		WHERE e.package_site_id IN (`+strings.Join(ph, ",")+`)
		  AND e.event_date=(SELECT MAX(event_date) FROM package_events WHERE package_site_id=e.package_site_id)`, args...)
	if err == nil {
		defer rows.Close()
		last := map[string][2]string{}
		for rows.Next() {
			var id, date, name string
			if err := rows.Scan(&id, &date, &name); err == nil {
				last[id] = [2]string{date, name}
			}
		}
		for i := range all {
			if v, ok := last[all[i].psid]; ok {
				all[i].LastDate = v[0]
				all[i].LastUser = v[1]
			}
		}
	}
	if strings.TrimSpace(today) == "" {
		return
	}
	cargs = append([]any{today}, cargs...)
	arows, err := db.Query(`
		SELECT w.customer_id, COALESCE(NULLIF(TRIM(u.full_name),''), NULLIF(TRIM(t.assignee),''), '담당')
		FROM work_task_sites w
		JOIN work_tasks t ON t.task_id=w.task_id
		LEFT JOIN users u ON u.user_id=t.assignee_user_id
		WHERE t.work_date=? AND w.customer_id IN (`+strings.Join(cph, ",")+`)
		  AND t.status NOT IN ('cancelled','done','completed')`, cargs...)
	if err != nil {
		return
	}
	defer arows.Close()
	assigned := map[string]string{}
	for arows.Next() {
		var id, name string
		if err := arows.Scan(&id, &name); err == nil && assigned[id] == "" {
			assigned[id] = name
		}
	}
	for i := range all {
		all[i].Assigned = assigned[all[i].CustomerID]
	}
}
