package repository

import (
	"net/url"
	"strings"

	"customer-support/internal/model"
)

type LeaveToday struct {
	Name string
	Kind string
}

type ExecOrgToday struct {
	Org        model.Org
	Headcount  int
	Total      int
	Done       int
	InProgress int
	NotStarted int
	Delayed    int
	DonePct    int
	OnLeave    []LeaveToday
	LeaveMore  int
	Href       string
	BarDone    int
	BarProg    int
	BarWait    int
	BarDelay   int
}

type ExecTotals struct {
	Total, Done, InProgress, Delayed, OnLeave int
}

type ExecDashRepo struct {
	orgs   *OrgRepo
	work   *WorkBoardRepo
	users  *UserRepo
	leaves *StaffLeaveRepo
}

func NewExecDashRepo(orgs *OrgRepo, work *WorkBoardRepo, users *UserRepo, leaves *StaffLeaveRepo) *ExecDashRepo {
	return &ExecDashRepo{orgs: orgs, work: work, users: users, leaves: leaves}
}

func ExecMeetingHref(orgID, date string) string {
	q := url.Values{}
	q.Set("view_org_id", strings.TrimSpace(orgID))
	if d := strings.TrimSpace(date); d != "" {
		q.Set("date", d)
	}
	q.Set("from", "exec")
	return "/meeting?" + q.Encode()
}

func execHeadcountOK(u model.User) bool {
	if !u.IsActive {
		return false
	}
	if strings.TrimSpace(u.DeletedAt) != "" {
		return false
	}
	role := model.NormalizeRole(u.Role)
	if role == model.RoleVisionAdmin || role == model.RoleObserver {
		return false
	}
	if u.IsReadOnly {
		return false
	}
	return true
}

func (r *ExecDashRepo) Today(date string, orgIDs []string) ([]ExecOrgToday, ExecTotals, error) {
	date = strings.TrimSpace(date)
	var totals ExecTotals
	if r == nil || r.orgs == nil || r.work == nil {
		return nil, totals, nil
	}
	orgs, err := r.orgs.ListActive()
	if err != nil {
		return nil, totals, err
	}
	allow := map[string]bool{}
	restrict := len(orgIDs) > 0
	for _, id := range orgIDs {
		id = strings.TrimSpace(id)
		if id != "" && id != OrgAll {
			allow[id] = true
		} else if id == OrgAll {
			restrict = false
		}
	}
	heads := map[string]int{}
	userOrg := map[string]string{}
	if r.users != nil {
		list, err := r.users.ListAll()
		if err != nil {
			return nil, totals, err
		}
		for _, u := range list {
			oid := strings.TrimSpace(u.OrgID)
			userOrg[strings.TrimSpace(u.UserID)] = oid
			if execHeadcountOK(u) {
				heads[oid]++
			}
		}
	}
	done, sched, _, err := r.work.MeetingTodaySets(date, "", nil, 300)
	if err != nil {
		return nil, totals, err
	}
	// 오늘 업무 건수 = 회의 PlannedN(오늘 완료∪오늘 예정). 진행중만 있는 건은 회의 요약에 안 넣는다.
	planned := model.UniqueWorkList(done, sched)
	byOrg := map[string][]model.WorkListItem{}
	for _, it := range planned {
		oid := strings.TrimSpace(it.OrgID)
		if oid == "" {
			continue
		}
		byOrg[oid] = append(byOrg[oid], it)
	}
	leavesByOrg := map[string][]LeaveToday{}
	leaveSeen := map[string]bool{}
	if r.leaves != nil {
		rows, err := r.leaves.ListInRange(date, date)
		if err != nil {
			return nil, totals, err
		}
		for _, lv := range rows {
			uid := strings.TrimSpace(lv.UserID)
			key := uid
			if key == "" {
				key = strings.TrimSpace(lv.UserName)
			}
			if key == "" || leaveSeen[key] {
				continue
			}
			leaveSeen[key] = true
			oid := userOrg[uid]
			name := strings.TrimSpace(lv.UserName)
			if name == "" {
				name = uid
			}
			leavesByOrg[oid] = append(leavesByOrg[oid], LeaveToday{Name: name, Kind: lv.Kind})
		}
	}
	const leaveCap = 5
	var out []ExecOrgToday
	for _, o := range orgs {
		if restrict && !allow[o.OrgID] {
			continue
		}
		hc := heads[o.OrgID]
		if hc <= 0 {
			continue
		}
		cls := model.ClassifyExecToday(byOrg[o.OrgID], date)
		allLeave := leavesByOrg[o.OrgID]
		shown := allLeave
		more := 0
		if len(shown) > leaveCap {
			more = len(shown) - leaveCap
			shown = shown[:leaveCap]
		}
		row := ExecOrgToday{
			Org: o, Headcount: hc,
			Total: cls.Total, Done: cls.Done, InProgress: cls.InProgress,
			NotStarted: cls.NotStarted, Delayed: cls.Delayed, DonePct: cls.DonePct,
			OnLeave: shown, LeaveMore: more,
			Href:     ExecMeetingHref(o.OrgID, date),
			BarDone:  model.ExecBarPct(cls.Done, cls.Total),
			BarProg:  model.ExecBarPct(cls.InProgress, cls.Total),
			BarWait:  model.ExecBarPct(cls.NotStarted, cls.Total),
			BarDelay: model.ExecBarPct(cls.Delayed, cls.Total),
		}
		out = append(out, row)
		totals.Total += cls.Total
		totals.Done += cls.Done
		totals.InProgress += cls.InProgress
		totals.Delayed += cls.Delayed
		totals.OnLeave += len(allLeave)
	}
	return out, totals, nil
}
