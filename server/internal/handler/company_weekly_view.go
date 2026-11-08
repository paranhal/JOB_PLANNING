package handler

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type weeklyReportKPI struct {
	Label    string
	Value    string
	Hint     string
	Mnt      bool
	Auto     bool
	Empty    bool
	Warn     bool
	Source   string
}

type weeklyReportCard struct {
	RowKey     string
	No         int
	Title      string
	Customer   string
	Assignee   string
	Kind       string
	KindLabel  string
	Support    bool
	Missing    bool
	Done       []repository.CompanyWeeklyLine
	Plan       []repository.CompanyWeeklyLine
	Need       string
	Decision   string
	HasContent bool
	Edit       bool
	HiddenN    int
	HiddenKeys []string
	StatBoxes  []model.CompanyWeeklyStatBox
}

type weeklyReportTab struct {
	OrgID string
	Name  string
	Count int
}

type weeklyReportView struct {
	Week          time.Time
	WeekInfo      model.WeekInfo
	Anchor        time.Time
	OrgID         string
	OrgName       string
	OrgShort      string
	OrgNote       string
	Status        string
	HasReport     bool
	CanEdit       bool
	CanConfirm    bool
	NeedOrg       bool
	MissingN      int
	Tabs          []weeklyReportTab
	VisibleTeams  int
	VisibleCards  int
	KPI           []weeklyReportKPI
	Cards         []weeklyReportCard
	Progress         *repository.CompanyWeeklyProgress
	ProgressMode     string
	ProgressCycle    string
	ProgressFromPrev bool
	ProgressDone     string
	ProgressTotal    string
	ProgressPct      string
	ProgressMemo     string
	ProgressCycleSel string
	NeedSalesRow     bool
	Draft            model.CompanyWeeklyDraft
	WeeklyDebug      bool
}

func companyWeeklyAnchor(weekMon time.Time) time.Time {
	return repository.WeekInfoOf(weekMon).Monday.AddDate(0, 0, 7)
}

func parseWeekMonday(s string, now time.Time) time.Time {
	loc := now.Location()
	if t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), loc); err == nil {
		return repository.WeekInfoOf(t).Monday
	}
	return repository.WeekInfoOf(now).Monday
}

func buildWeeklyReportView(repo *repository.StatsRepo, orgID string, weekMon time.Time, canEdit bool) (weeklyReportView, error) {
	wi := repository.WeekInfoOf(weekMon)
	weekMon = wi.Monday
	anchor := companyWeeklyAnchor(weekMon)
	v := weeklyReportView{
		Week: weekMon, WeekInfo: wi, Anchor: anchor, OrgID: orgID,
		CanEdit: canEdit, CanConfirm: canEdit,
		ProgressMode:  repository.WeeklyProgressModeManual,
		ProgressCycle: "반기",
	}
	if repo == nil {
		return v, fmt.Errorf("repo")
	}
	short, name := repo.OrgShortAndName(orgID)
	v.OrgShort, v.OrgName = short, name
	v.ProgressMode = repo.WeeklyProgressMode(orgID)
	v.ProgressCycle = repo.WeeklyProgressCycle(orgID)

	draft, err := repo.BuildCompanyWeeklyDraftOrg(anchor, orgID)
	if err != nil {
		return v, err
	}
	savedRep, _ := repo.GetCompanyWeeklyReport(orgID, draft.Period.Anchor)
	doc := repository.ParseCompanyWeeklyDoc(savedRep.ContentJSON)
	if savedRep.ReportID != "" {
		v.HasReport = true
		v.Status = savedRep.Status
		draft.Rows = repository.ApplyCompanyWeeklySaved(draft.Rows, doc.Rows)
	} else {
		v.Status = repository.CompanyWeeklyStatusDraft
	}
	v.Draft = draft
	v.Progress = doc.Progress
	v.ProgressCycleSel = v.ProgressCycle
	if v.Progress == nil || progressEmpty(v.Progress) {
		prevAnchor := draft.Period.Anchor
		if t, err := time.ParseInLocation("2006-01-02", draft.Period.Anchor, weekMon.Location()); err == nil {
			prev, _ := repo.GetCompanyWeeklyReport(orgID, t.AddDate(0, 0, -7).Format("2006-01-02"))
			prevDoc := repository.ParseCompanyWeeklyDoc(prev.ContentJSON)
			if prevDoc.Progress != nil && !progressEmpty(prevDoc.Progress) {
				cp := *prevDoc.Progress
				cp.UpdatedBy, cp.UpdatedAt, cp.Log = "", "", nil
				v.Progress = &cp
				v.ProgressFromPrev = savedRep.ReportID == "" || progressEmpty(doc.Progress)
				_ = prevAnchor
			}
		}
	}

	statItems, _, err := repo.ListCompanyWeeklyStatItems(draft.Period, orgID)
	if err != nil {
		return v, err
	}
	statByRow := repository.GroupCompanyWeeklyStatBoxes(draft.Rows, statItems)

	var cards []weeklyReportCard
	mntDone, asDone := 0, 0
	projN, actN := 0, 0
	hasTech := false
	var projNames []string
	actKinds := map[string]int{}
	for _, row := range draft.Rows {
		saved := doc.Rows[row.RowKey]
		autoDone := repository.AutoToLines(row.AutoDone)
		if len(autoDone) == 0 {
			autoDone = repository.SplitWeeklyLines(row.PrevText)
		}
		autoPlan := repository.AutoToLines(row.AutoPlan)
		if len(autoPlan) == 0 {
			autoPlan = repository.SplitWeeklyLines(row.PlanText)
		}
		done := repository.MergeWeeklyLinesKeyed(autoDone, saved.DoneLines, saved.HiddenDone)
		plan := repository.MergeWeeklyLinesKeyed(autoPlan, saved.PlanLines, saved.HiddenPlan)
		if row.ActivityKind == "tech" {
			hasTech = true
		}
		need := strings.TrimSpace(saved.SupportNeeds)
		if need == "" {
			need = strings.TrimSpace(saved.Help)
		}
		if need == "" {
			need = row.HelpText
		}
		dec := strings.TrimSpace(saved.Decision)
		if dec == "" {
			dec = row.DecisionText
		}
		cust := strings.TrimSpace(saved.Customer)
		card := weeklyReportCard{
			RowKey: row.RowKey, Title: row.DisplayName, Customer: cust, Assignee: saved.Assignee,
			Kind: row.RowKind, Missing: row.MissingProject,
			Done: done, Plan: plan, Need: need, Decision: dec,
			StatBoxes: statByRow[row.RowKey],
		}
		if row.RowKind == "activity" {
			card.KindLabel = activityBadge(row.ActivityKind, row.DisplayName)
			actN++
			actKinds[card.KindLabel]++
		} else {
			if hasLineText(done) || hasLineText(plan) {
				projN++
				if len(projNames) < 3 {
					projNames = append(projNames, row.DisplayName)
				}
			}
		}
		card.HasContent = hasLineText(done) || hasLineText(plan) || need != "" || dec != ""
		card.HiddenN = len(saved.HiddenDone) + len(saved.HiddenPlan)
		card.HiddenKeys = append(append([]string{}, saved.HiddenDone...), saved.HiddenPlan...)
		if row.MissingProject {
			v.MissingN++
		}
		mntDone += row.MntDoneSites
		asDone += row.ASDoneCount
		cards = append(cards, card)
	}
	for i := range cards {
		cards[i].No = i + 1
	}
	v.Cards = cards
	v.NeedSalesRow = !hasTech

	mntHint, mntEmpty, mntWarn, src := weeklyProgressHint(repo, orgID, weekMon, v.ProgressMode, v.ProgressCycle, v.Progress, mntDone)
	v.KPI = []weeklyReportKPI{
		{Label: "정기점검", Value: fmt.Sprintf("%d건", mntDone), Hint: mntHint, Mnt: true, Auto: v.ProgressMode == repository.WeeklyProgressModeAuto, Empty: mntEmpty, Warn: mntWarn, Source: src},
		{Label: "A/S 처리", Value: fmt.Sprintf("%d건", asDone), Hint: asHint(asDone, projN)},
		{Label: "사업 업무", Value: fmt.Sprintf("%d건", projN), Hint: strings.Join(projNames, " · ")},
		{Label: "영업·지원", Value: fmt.Sprintf("%d건", actN), Hint: actHint(actKinds)},
	}
	if v.KPI[2].Hint == "" {
		v.KPI[2].Hint = "—"
	}
	if v.KPI[1].Hint == "" {
		v.KPI[1].Hint = "—"
	}
	if v.KPI[3].Hint == "" {
		v.KPI[3].Hint = "—"
	}
	fillProgressForm(&v)
	return v, nil
}

func fillProgressForm(v *weeklyReportView) {
	v.ProgressCycleSel = v.ProgressCycle
	if v.Progress == nil {
		return
	}
	if v.Progress.Cycle != "" {
		v.ProgressCycleSel = v.Progress.Cycle
	}
	if v.Progress.Done != nil {
		v.ProgressDone = fmt.Sprintf("%d", *v.Progress.Done)
	}
	if v.Progress.Total != nil {
		v.ProgressTotal = fmt.Sprintf("%d", *v.Progress.Total)
	}
	if v.Progress.Pct != nil {
		v.ProgressPct = formatViewPct(*v.Progress.Pct)
	}
	v.ProgressMemo = v.Progress.Memo
}

func progressEmpty(p *repository.CompanyWeeklyProgress) bool {
	if p == nil {
		return true
	}
	return p.Done == nil && p.Total == nil && p.Pct == nil && strings.TrimSpace(p.Memo) == ""
}

func hasLineText(lines []repository.CompanyWeeklyLine) bool {
	for _, l := range lines {
		if strings.TrimSpace(l.Text) != "" {
			return true
		}
	}
	return false
}

func activityBadge(kind, name string) string {
	switch kind {
	case "tech":
		return "영업"
	case "unlinked":
		if strings.TrimSpace(name) != "" {
			return name
		}
		return "미연결"
	case "asset":
		return "지원"
	case "mnt_as":
		return "지원"
	}
	if strings.Contains(name, "영업") {
		return "영업"
	}
	if strings.Contains(name, "타팀") || strings.Contains(name, "타 팀") {
		return "타팀 지원"
	}
	return "지원"
}

func asHint(as, projects int) string {
	if as == 0 {
		return "—"
	}
	if projects > 0 {
		return fmt.Sprintf("%d개 유지관리 사업 합계", projects)
	}
	return "완료 AS"
}

func actHint(m map[string]int) string {
	var parts []string
	for _, k := range []string{"영업", "지원", "타팀 지원", "미연결"} {
		if m[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d건", k, m[k]))
		}
	}
	return strings.Join(parts, " · ")
}

func weeklyProgressHint(repo *repository.StatsRepo, orgID string, weekMon time.Time, mode, cycle string, saved *repository.CompanyWeeklyProgress, weekDone int) (hint string, empty bool, warn bool, src string) {
	if mode == repository.WeeklyProgressModeAuto {
		src = "자동 집계"
		quota, cum, month, err := repo.CompanyMntMonthProgress(orgID, weekMon)
		if err != nil {
			return "—", true, false, src
		}
		if quota <= 0 {
			if weekDone == 0 {
				return "해당 없음", true, false, src
			}
			return "—", true, false, src
		}
		pct := repository.ProgressPct(cum, quota)
		return fmt.Sprintf("%d월 · 누적 %d/%d건 · 달성률 %s%%", month, cum, quota, formatViewPct(pct)), false, cum > quota, src
	}
	src = "작성자 입력"
	if saved == nil {
		return "진행율 미입력", true, false, src
	}
	label, emp, w := saved.Display(cycle)
	return label, emp, w, src
}

func formatViewPct(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0")
}
