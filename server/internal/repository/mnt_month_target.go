package repository

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"

	"customer-support/internal/model"
)

func applyMaintenanceMonthTargetsV89(db *sql.DB) {
	if db == nil {
		return
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS maintenance_month_targets (
			target_id TEXT PRIMARY KEY,
			plan_id TEXT NOT NULL,
			org_id TEXT NOT NULL DEFAULT '',
			customer_id TEXT NOT NULL,
			slot_key TEXT NOT NULL DEFAULT 'generic',
			year INTEGER NOT NULL,
			month INTEGER NOT NULL,
			round_no INTEGER NOT NULL DEFAULT 1,
			source TEXT NOT NULL DEFAULT 'site',
			region TEXT NOT NULL DEFAULT '',
			created_by_user_id TEXT NOT NULL DEFAULT '',
			created_by_name TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			cancelled_at TEXT
		);
		CREATE UNIQUE INDEX IF NOT EXISTS uq_mnt_month_targets_active
			ON maintenance_month_targets(plan_id, customer_id, slot_key, year, month, round_no)
			WHERE cancelled_at IS NULL OR cancelled_at = '';
		CREATE INDEX IF NOT EXISTS idx_mnt_month_targets_plan_ym
			ON maintenance_month_targets(plan_id, year, month);`)
	if err != nil {
		log.Printf("089 month targets: %v", err)
	}
}

func ApplyMaintenanceMonthTargetsV89ForTest(db *sql.DB) { applyMaintenanceMonthTargetsV89(db) }

func (r *MaintenanceRepo) ListActiveMonthTargets(planID string, year int) ([]model.MaintenanceMonthTarget, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" || year < 1 {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT target_id, plan_id, org_id, customer_id, slot_key, year, month, round_no,
		       source, region, created_by_user_id, created_by_name, created_at, COALESCE(cancelled_at,'')
		  FROM maintenance_month_targets
		 WHERE plan_id=? AND year=? AND (cancelled_at IS NULL OR cancelled_at='')
		 ORDER BY month, customer_id, slot_key, round_no`, planID, year)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.MaintenanceMonthTarget
	for rows.Next() {
		var t model.MaintenanceMonthTarget
		if err := rows.Scan(&t.TargetID, &t.PlanID, &t.OrgID, &t.CustomerID, &t.SlotKey, &t.Year, &t.Month, &t.RoundNo,
			&t.Source, &t.Region, &t.CreatedByUserID, &t.CreatedByName, &t.CreatedAt, &t.CancelledAt); err != nil {
			return nil, err
		}
		t.SlotKey = model.NormalizeMonthTargetSlot(t.SlotKey)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *MaintenanceRepo) openPeriodSlotsFrom(planID string, year, month int, configs []model.MaintenanceSiteConfig, visits []model.MaintenanceVisit, targets []model.MaintenanceMonthTarget, lookup func(customerID, product string) string) ([]model.UnassignedMonthSlot, error) {
	if year < 1 || month < 1 || month > 12 {
		return nil, nil
	}
	if lookup == nil {
		lookup = func(string, string) string { return "" }
	}
	var out []model.UnassignedMonthSlot
	for _, cycle := range []string{"quarterly", "semi", "yearly"} {
		p, ok := model.CyclePeriodOf(cycle, month)
		if !ok {
			continue
		}
		skip, err := r.quarterSkipSet(planID, year, p.Index)
		if err != nil {
			return nil, err
		}
		out = append(out, periodMonthOpenSlots(configs, visits, targets, skip, lookup, year, month, p)...)
	}
	return out, nil
}

func periodMonthOpenSlots(configs []model.MaintenanceSiteConfig, visits []model.MaintenanceVisit, targets []model.MaintenanceMonthTarget, skip map[string]bool, lookup func(string, string) string, year, month int, p model.CyclePeriod) []model.UnassignedMonthSlot {
	cfgBy := map[string]model.MaintenanceSiteConfig{}
	for _, cfg := range configs {
		cfgBy[cfg.CustomerID] = cfg
	}
	visitN := map[string]int{}
	for _, v := range visits {
		if !model.VisitInCyclePeriod(v.VisitDate, year, p) {
			continue
		}
		key := model.VisitProductSlotKeyFor(v.ProductType, cfgBy[v.CustomerID].Products)
		visitN[v.CustomerID+"|"+key]++
		if key != "generic" {
			visitN[v.CustomerID+"|generic"] += 0
		}
	}
	type tlist []model.MaintenanceMonthTarget
	bySlot := map[string]tlist{}
	for _, t := range targets {
		if t.Year != year || !t.Active() {
			continue
		}
		if t.Month < p.StartMonth || t.Month > p.EndMonth {
			continue
		}
		k := t.CustomerID + "|" + model.NormalizeMonthTargetSlot(t.SlotKey)
		bySlot[k] = append(bySlot[k], t)
	}
	for k := range bySlot {
		sort.Slice(bySlot[k], func(i, j int) bool {
			if bySlot[k][i].Month != bySlot[k][j].Month {
				return bySlot[k][i].Month < bySlot[k][j].Month
			}
			return bySlot[k][i].RoundNo < bySlot[k][j].RoundNo
		})
	}
	var out []model.UnassignedMonthSlot
	for _, cfg := range configs {
		if strings.TrimSpace(cfg.InspectionCycle) != p.Cycle {
			continue
		}
		need := cfg.SiteRounds()
		for _, s := range model.SiteProductSlots(cfg) {
			sk := cfg.CustomerID + "|" + s.SlotKey
			if skip[sk] {
				continue
			}
			nVis := visitN[sk]
			if s.SlotKey != "generic" && nVis == 0 {
				nVis = visitN[cfg.CustomerID+"|generic"]
			}
			if nVis >= need {
				continue
			}
			tgs := bySlot[sk]
			unful := tgs
			if nVis > 0 && nVis <= len(tgs) {
				unful = tgs[nVis:]
			} else if nVis > len(tgs) {
				unful = nil
			}
			var show *model.MaintenanceMonthTarget
			for i := range unful {
				if unful[i].Month == month {
					show = &unful[i]
					break
				}
			}
			if show == nil {
				for i := range unful {
					if unful[i].Month < month {
						show = &unful[i]
						break
					}
				}
			}
			s.LastAssignee = lookup(s.CustomerID, s.ProductType)
			if show != nil {
				s.TargetMonth = show.Month
				s.TargetCarry = show.Month < month
				s.UnconfirmedQuarter = false
				out = append(out, s)
				continue
			}
			if month == p.EndMonth && nVis == 0 && len(tgs) == 0 {
				s.UnconfirmedQuarter = true
				out = append(out, s)
			}
		}
	}
	return out
}

type MonthTargetRegionStat struct {
	Region     string
	Key        string
	Label      string
	Total      int
	Registered int
	Open       int
	Children   []MonthTargetRegionStat `json:",omitempty"`
}

type MonthTargetSiteRow struct {
	CustomerID  string
	Name        string
	Region      string
	Sigungu     string
	RegionLabel string
	Cycle       string
	Registered  bool
	SkipReason  string
}

type monthTargetParentAcc struct {
	st      MonthTargetRegionStat
	kids    map[string]*MonthTargetRegionStat
	emptyGu int
	namedGu int
}

func bumpMonthTargetStat(st *MonthTargetRegionStat, registered bool, reason string) {
	st.Total++
	if registered {
		st.Registered++
	} else if reason == "" {
		st.Open++
	} else if strings.Contains(reason, "이미") {
		st.Registered++
	} else {
		st.Open++
	}
}

func (r *MaintenanceRepo) MonthTargetPanel(plan *model.MaintenancePlan, month int, q, industry string) ([]MonthTargetRegionStat, []MonthTargetSiteRow, error) {
	return r.MonthTargetPanelFilter(plan, month, q, industry, "", "")
}

func (r *MaintenanceRepo) MonthTargetPanelFilter(plan *model.MaintenancePlan, month int, q, industry, sidoFilter, sigunguFilter string) ([]MonthTargetRegionStat, []MonthTargetSiteRow, error) {
	if plan == nil {
		return nil, nil, fmt.Errorf("계획이 없습니다")
	}
	configs, err := r.ListSiteConfigs()
	if err != nil {
		return nil, nil, err
	}
	visits, err := r.ListVisits(plan.PlanID)
	if err != nil {
		return nil, nil, err
	}
	targets, err := r.ListActiveMonthTargets(plan.PlanID, plan.PlanYear)
	if err != nil {
		return nil, nil, err
	}
	dist, _ := r.ListRegionDistanceOrder()
	rank := regionDistanceRank(dist)
	q = strings.ToLower(strings.TrimSpace(q))
	industry = strings.TrimSpace(industry)
	sidoFilter = strings.TrimSpace(sidoFilter)
	sigunguFilter = strings.TrimSpace(sigunguFilter)
	unsetGu := sigunguFilter == model.RegionSigunguUnsetQuery
	indBy := map[string]string{}
	if industry != "" {
		rows, err := r.db.Query(`SELECT customer_id, COALESCE(industry,'') FROM customers`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, ind string
				if rows.Scan(&id, &ind) == nil {
					indBy[id] = ind
				}
			}
		}
	}
	parents := map[string]*monthTargetParentAcc{}
	var sites []MonthTargetSiteRow
	for _, cfg := range configs {
		if strings.TrimSpace(plan.OrgID) != "" && strings.TrimSpace(plan.OrgID) != OrgAll &&
			strings.TrimSpace(cfg.OrgID) != "" && cfg.OrgID != plan.OrgID {
			continue
		}
		if !model.IsPeriodCycle(cfg.InspectionCycle) {
			continue
		}
		if err := model.PeriodMonthTargetAllowed(cfg.InspectionCycle, month); err != nil {
			continue
		}
		sido, gu := model.SiteRegion(cfg)
		parentKey := model.SidoRegionKey(sido)
		parentLab := model.RegionLabel(sido)
		acc := parents[parentKey]
		if acc == nil {
			acc = &monthTargetParentAcc{
				st:   MonthTargetRegionStat{Region: parentLab, Key: parentKey, Label: parentLab},
				kids: map[string]*MonthTargetRegionStat{},
			}
			parents[parentKey] = acc
		}
		name := cfg.ShortName
		if name == "" {
			name = cfg.OrgName
		}
		reason := r.monthTargetSkipReason(cfg, visits, targets, plan.PlanYear, month)
		registered := reason == "" && r.hasMonthTarget(cfg, targets, plan.PlanYear, month)
		bumpMonthTargetStat(&acc.st, registered, reason)
		if parentKey != model.RegionUnspecified {
			ck := model.FormatRegionKey(sido, gu)
			ch := acc.kids[ck]
			if ch == nil {
				lab := gu
				if lab == "" {
					lab = model.RegionSigunguUnsetLabel
				}
				ch = &MonthTargetRegionStat{Region: lab, Key: ck, Label: lab}
				acc.kids[ck] = ch
			}
			bumpMonthTargetStat(ch, registered, reason)
			if gu == "" {
				acc.emptyGu++
			} else {
				acc.namedGu++
			}
		}
		if industry != "" && !strings.EqualFold(indBy[cfg.CustomerID], industry) {
			continue
		}
		if q != "" {
			blob := strings.ToLower(name + " " + cfg.OrgName + " " + cfg.CustomerID + " " + cfg.Region + " " + cfg.AddrSigungu + " " + gu)
			if !strings.Contains(blob, q) {
				continue
			}
		}
		if sidoFilter != "" {
			want := sidoFilter
			if want == model.RegionUnspecified {
				if sido != "" {
					continue
				}
			} else if sido != want {
				continue
			}
		}
		if unsetGu {
			if gu != "" {
				continue
			}
		} else if sigunguFilter != "" && gu != sigunguFilter {
			continue
		}
		sites = append(sites, MonthTargetSiteRow{
			CustomerID: cfg.CustomerID, Name: name, Region: parentLab, Sigungu: gu,
			RegionLabel: model.SiteRegionLabel(cfg), Cycle: cfg.InspectionCycle,
			Registered: registered || strings.Contains(reason, "이미"), SkipReason: reason,
		})
	}
	var stats []MonthTargetRegionStat
	for _, acc := range parents {
		st := acc.st
		if acc.namedGu > 0 {
			var kids []MonthTargetRegionStat
			for _, ch := range acc.kids {
				kids = append(kids, *ch)
			}
			sort.Slice(kids, func(i, j int) bool {
				ri, rj := rank[kids[i].Key], rank[kids[j].Key]
				if ri == 0 {
					ri = 1 << 30
				}
				if rj == 0 {
					rj = 1 << 30
				}
				if ri != rj {
					return ri < rj
				}
				return kids[i].Label < kids[j].Label
			})
			st.Children = kids
		}
		stats = append(stats, st)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Region < stats[j].Region })
	sort.Slice(sites, func(i, j int) bool { return sites[i].Name < sites[j].Name })
	return stats, sites, nil
}

func regionDistanceRank(items []model.RegionDistanceOrder) map[string]int {
	out := map[string]int{}
	for _, it := range items {
		ord := it.SortOrder + 1
		sido := model.ShortRegionName(it.Sido)
		if sido == "" {
			sido = strings.TrimSpace(it.Sido)
		}
		key := model.FormatRegionKey(sido, it.Sigungu)
		if _, ok := out[key]; !ok {
			out[key] = ord
		}
		raw := model.FormatRegionKey(strings.TrimSpace(it.Sido), it.Sigungu)
		if _, ok := out[raw]; !ok {
			out[raw] = ord
		}
	}
	return out
}

func (r *MaintenanceRepo) hasMonthTarget(cfg model.MaintenanceSiteConfig, targets []model.MaintenanceMonthTarget, year, month int) bool {
	for _, t := range targets {
		if t.CustomerID == cfg.CustomerID && t.Year == year && t.Month == month && t.Active() {
			return true
		}
	}
	return false
}

func (r *MaintenanceRepo) monthTargetSkipReason(cfg model.MaintenanceSiteConfig, visits []model.MaintenanceVisit, targets []model.MaintenanceMonthTarget, year, month int) string {
	if err := model.PeriodMonthTargetAllowed(cfg.InspectionCycle, month); err != nil {
		return err.Error()
	}
	p, ok := model.CyclePeriodOf(cfg.InspectionCycle, month)
	if !ok {
		return "기간 주기만 등록합니다"
	}
	need := cfg.SiteRounds()
	slots := model.SiteProductSlots(cfg)
	for _, s := range slots {
		nVis, nTgt, sameMonth := periodSlotCommit(cfg, s, visits, targets, year, p, month)
		if sameMonth {
			return fmt.Sprintf("이미 %d월에 등록됨", month)
		}
		if max(nVis, nTgt) >= need {
			other := firstTargetMonth(cfg.CustomerID, s.SlotKey, targets, year, p)
			if other > 0 {
				return fmt.Sprintf("이미 %d월에 등록됨", other)
			}
			return "이 기간 점검을 이미 채웠습니다"
		}
	}
	return ""
}

func firstTargetMonth(customerID, slot string, targets []model.MaintenanceMonthTarget, year int, p model.CyclePeriod) int {
	for _, t := range targets {
		if t.CustomerID == customerID && model.NormalizeMonthTargetSlot(t.SlotKey) == slot && t.Year == year && t.Active() &&
			t.Month >= p.StartMonth && t.Month <= p.EndMonth {
			return t.Month
		}
	}
	return 0
}

func periodSlotCommit(cfg model.MaintenanceSiteConfig, s model.UnassignedMonthSlot, visits []model.MaintenanceVisit, targets []model.MaintenanceMonthTarget, year int, p model.CyclePeriod, month int) (nVis, nTgt int, sameMonth bool) {
	for _, v := range visits {
		if v.CustomerID != cfg.CustomerID {
			continue
		}
		if !model.VisitInCyclePeriod(v.VisitDate, year, p) {
			continue
		}
		key := model.VisitProductSlotKeyFor(v.ProductType, cfg.Products)
		if key == s.SlotKey || key == "generic" {
			nVis++
		}
	}
	for _, t := range targets {
		if t.CustomerID != cfg.CustomerID || !t.Active() || t.Year != year {
			continue
		}
		if model.NormalizeMonthTargetSlot(t.SlotKey) != s.SlotKey {
			continue
		}
		if t.Month < p.StartMonth || t.Month > p.EndMonth {
			continue
		}
		nTgt++
		if t.Month == month {
			sameMonth = true
		}
	}
	return
}

func (r *MaintenanceRepo) RegisterMonthTargets(plan *model.MaintenancePlan, month int, source, region string, customerIDs []string, actorID, actorName string) (model.MonthTargetRegisterResult, error) {
	var res model.MonthTargetRegisterResult
	if plan == nil {
		return res, fmt.Errorf("계획이 없습니다")
	}
	if err := model.ValidateMonthTargetMonth(plan.PlanYear, month); err != nil {
		return res, err
	}
	source = model.NormalizeMonthTargetSource(source)
	orgID, err := RequireInsertOrg(plan.OrgID)
	if err != nil {
		return res, fmt.Errorf("계획 조직이 없습니다")
	}
	configs, err := r.ListSiteConfigs()
	if err != nil {
		return res, err
	}
	cfgBy := map[string]model.MaintenanceSiteConfig{}
	for _, c := range configs {
		cfgBy[c.CustomerID] = c
	}
	if source == model.MonthTargetSourceRegion {
		regionKey := strings.TrimSpace(region)
		for _, c := range configs {
			if strings.TrimSpace(c.OrgID) != "" && c.OrgID != orgID {
				continue
			}
			if !model.IsPeriodCycle(c.InspectionCycle) {
				continue
			}
			if !model.MatchRegionKey(c, regionKey) {
				continue
			}
			customerIDs = append(customerIDs, c.CustomerID)
		}
	}
	seen := map[string]bool{}
	visits, err := r.ListVisits(plan.PlanID)
	if err != nil {
		return res, err
	}
	targets, err := r.ListActiveMonthTargets(plan.PlanID, plan.PlanYear)
	if err != nil {
		return res, err
	}
	now := r.nowTime().Format("2006-01-02 15:04:05")
	for _, cid := range customerIDs {
		cid = strings.TrimSpace(cid)
		if cid == "" || seen[cid] {
			continue
		}
		seen[cid] = true
		cfg, ok := cfgBy[cid]
		name := cid
		if ok {
			name = cfg.ShortName
			if name == "" {
				name = cfg.OrgName
			}
		}
		if !ok {
			res.Skipped = append(res.Skipped, model.MonthTargetSkip{CustomerID: cid, Name: name, Reason: "점검 사이트가 아닙니다", Month: month})
			continue
		}
		if strings.TrimSpace(cfg.OrgID) != "" && cfg.OrgID != orgID {
			res.Skipped = append(res.Skipped, model.MonthTargetSkip{CustomerID: cid, Name: name, Reason: "다른 조직 사이트입니다", Month: month})
			continue
		}
		reason := r.monthTargetSkipReason(cfg, visits, targets, plan.PlanYear, month)
		if reason != "" {
			res.Skipped = append(res.Skipped, model.MonthTargetSkip{CustomerID: cid, Name: name, Reason: reason, Month: month})
			continue
		}
		p, _ := model.CyclePeriodOf(cfg.InspectionCycle, month)
		for _, s := range model.SiteProductSlots(cfg) {
			nVis, nTgt, same := periodSlotCommit(cfg, s, visits, targets, plan.PlanYear, p, month)
			if same || max(nVis, nTgt) >= cfg.SiteRounds() {
				continue
			}
			round := nTgt + 1
			id := randomID("mmt_")
			_, err := r.db.Exec(`
				INSERT INTO maintenance_month_targets
				(target_id, plan_id, org_id, customer_id, slot_key, year, month, round_no, source, region,
				 created_by_user_id, created_by_name, created_at)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				id, plan.PlanID, orgID, cid, s.SlotKey, plan.PlanYear, month, round, source, strings.TrimSpace(cfg.Region),
				strings.TrimSpace(actorID), strings.TrimSpace(actorName), now)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "unique") {
					res.Skipped = append(res.Skipped, model.MonthTargetSkip{CustomerID: cid, Name: name, Reason: fmt.Sprintf("이미 %d월에 등록됨", month), Month: month})
					continue
				}
				return res, err
			}
			targets = append(targets, model.MaintenanceMonthTarget{
				TargetID: id, PlanID: plan.PlanID, OrgID: orgID, CustomerID: cid, SlotKey: s.SlotKey,
				Year: plan.PlanYear, Month: month, RoundNo: round, Source: source, Region: cfg.Region,
			})
			res.Added++
		}
	}
	if res.Added > 0 {
		r.invalidateOpenUnassignedSlotsCache()
	}
	return res, nil
}

func (r *MaintenanceRepo) GetMonthTarget(id string) (*model.MaintenanceMonthTarget, error) {
	var t model.MaintenanceMonthTarget
	err := r.db.QueryRow(`
		SELECT target_id, plan_id, org_id, customer_id, slot_key, year, month, round_no,
		       source, region, created_by_user_id, created_by_name, created_at, COALESCE(cancelled_at,'')
		  FROM maintenance_month_targets WHERE target_id=?`, strings.TrimSpace(id)).
		Scan(&t.TargetID, &t.PlanID, &t.OrgID, &t.CustomerID, &t.SlotKey, &t.Year, &t.Month, &t.RoundNo,
			&t.Source, &t.Region, &t.CreatedByUserID, &t.CreatedByName, &t.CreatedAt, &t.CancelledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.SlotKey = model.NormalizeMonthTargetSlot(t.SlotKey)
	return &t, nil
}

func (r *MaintenanceRepo) periodHasVisitForSlot(planID, customerID, slotKey string, year int, p model.CyclePeriod) (bool, error) {
	visits, err := r.ListVisits(planID)
	if err != nil {
		return false, err
	}
	cfg, _ := r.GetSiteConfig(customerID)
	prod := []model.InspectionProduct(nil)
	if cfg != nil {
		prod = cfg.Products
	}
	for _, v := range visits {
		if v.CustomerID != customerID {
			continue
		}
		if !model.VisitInCyclePeriod(v.VisitDate, year, p) {
			continue
		}
		key := model.VisitProductSlotKeyFor(v.ProductType, prod)
		if key == slotKey || key == "generic" {
			return true, nil
		}
	}
	return false, nil
}

func (r *MaintenanceRepo) CancelMonthTarget(id string) error {
	t, err := r.GetMonthTarget(id)
	if err != nil || t == nil {
		return fmt.Errorf("등록을 찾을 수 없습니다")
	}
	if !t.Active() {
		return nil
	}
	cfg, _ := r.GetSiteConfig(t.CustomerID)
	cycle := "yearly"
	if cfg != nil {
		cycle = cfg.InspectionCycle
	}
	p, ok := model.CyclePeriodOf(cycle, t.Month)
	if ok {
		has, err := r.periodHasVisitForSlot(t.PlanID, t.CustomerID, t.SlotKey, t.Year, p)
		if err != nil {
			return err
		}
		if has {
			return fmt.Errorf("방문이 있어 등록을 취소할 수 없습니다")
		}
	}
	now := r.nowTime().Format("2006-01-02 15:04:05")
	_, err = r.db.Exec(`UPDATE maintenance_month_targets SET cancelled_at=? WHERE target_id=? AND (cancelled_at IS NULL OR cancelled_at='')`, now, t.TargetID)
	if err == nil {
		r.invalidateOpenUnassignedSlotsCache()
	}
	return err
}

func (r *MaintenanceRepo) MoveMonthTarget(id string, newMonth int) error {
	t, err := r.GetMonthTarget(id)
	if err != nil || t == nil {
		return fmt.Errorf("등록을 찾을 수 없습니다")
	}
	if !t.Active() {
		return fmt.Errorf("이미 취소된 등록입니다")
	}
	cfg, err := r.GetSiteConfig(t.CustomerID)
	if err != nil || cfg == nil {
		return fmt.Errorf("사이트를 찾을 수 없습니다")
	}
	if err := model.PeriodMonthTargetAllowed(cfg.InspectionCycle, newMonth); err != nil {
		return err
	}
	p, _ := model.CyclePeriodOf(cfg.InspectionCycle, t.Month)
	has, err := r.periodHasVisitForSlot(t.PlanID, t.CustomerID, t.SlotKey, t.Year, p)
	if err != nil {
		return err
	}
	if has {
		return fmt.Errorf("방문이 있어 월을 옮길 수 없습니다")
	}
	_, err = r.db.Exec(`UPDATE maintenance_month_targets SET month=? WHERE target_id=?`, newMonth, t.TargetID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("이미 %d월에 등록됨", newMonth)
		}
		return err
	}
	r.invalidateOpenUnassignedSlotsCache()
	return nil
}

func (r *MaintenanceRepo) EnsureYearPlan(orgID string, year int) (*model.MaintenancePlan, error) {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return nil, fmt.Errorf("조직을 선택한 뒤 계획을 만드세요")
	}
	p, err := r.GetPlanByYearInOrg(orgID, year)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return p, nil
	}
	title := fmt.Sprintf("%d년 정기점검", year)
	return r.CreatePlanForOrg(orgID, year, title)
}
