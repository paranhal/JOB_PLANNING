package model

import "strings"

const (
	ScheduleShowPlan = "plan"
	ScheduleShowAct  = "act"
	ScheduleShowKey  = "jp.schedule.show"
)

// ParseScheduleShow URL show 값. 비거나 둘 다 꺼짐이면 접수만.
func ParseScheduleShow(raw string) (plan, act bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return true, false
	}
	for _, p := range strings.Split(raw, ",") {
		switch strings.TrimSpace(p) {
		case ScheduleShowPlan, "receipt", "p":
			plan = true
		case ScheduleShowAct, "done", "exec", "x":
			act = true
		}
	}
	if !plan && !act {
		return true, false
	}
	return plan, act
}

func EncodeScheduleShow(plan, act bool) string {
	if plan && act {
		return ScheduleShowPlan + "," + ScheduleShowAct
	}
	if act && !plan {
		return ScheduleShowAct
	}
	return ScheduleShowPlan
}

func ToggleScheduleShow(raw, which string) string {
	plan, act := ParseScheduleShow(raw)
	switch strings.TrimSpace(strings.ToLower(which)) {
	case ScheduleShowPlan:
		plan = !plan
	case ScheduleShowAct:
		act = !act
	}
	return EncodeScheduleShow(plan, act)
}

func TaskPlanDate(t WorkTask) string {
	d := strings.TrimSpace(t.WorkDate)
	if d == "" {
		d = strings.TrimSpace(t.DueDate)
	}
	return ymdPrefix(d)
}

func TaskExecDate(t WorkTask) string {
	d := ymdPrefix(t.CompleteDate)
	if d != "" {
		return d
	}
	if strings.TrimSpace(t.Status) == WBTaskComplete {
		return TaskPlanDate(t)
	}
	return ""
}

func ymdPrefix(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
