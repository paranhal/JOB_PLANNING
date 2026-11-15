package repository

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

// 95-205: 주간보고 프로젝트 행의 업무 줄 묶음. 줄 생성은 여기 한 곳(화면·엑셀·HWPX 가 같은 뷰모델을 쓴다).
//
// 묶음 키 = (사업, 묶음 ID). 사업은 조회가 이미 사업별이라 같은 사업끼리만 들어온다.
// 묶음 ID: parent_task_id 가 있으면 그 값(반복·하위 일정), 같은 목록에 자신을 상위로 가진 건이 있으면 자기 ID,
// 아니면 정규화한 제목. 제목이 다르고 상위도 다르면 묶지 않는다(유사도 매칭 없음).

const (
	weeklySecDone  = "done"
	weeklySecPlan  = "plan"
	weeklySecCarry = "carry"
)

// normWeeklyTitle 공백 연속→1칸, 앞뒤 공백 제거, 대소문자 무시.
func normWeeklyTitle(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// formatWorkDateRange 날짜 목록을 줄 끝 괄호 안 표기로 만든다.
//   - 1개: 11/9 · 연속(다음 날, 또는 토·일만 건너뛴 평일 연속): 11/9~11/11
//   - 연속이 아니면 11/9·11/11 (3개 초과 시 앞 3개 + 「 외 N일」)
func formatWorkDateRange(dates []string) string {
	seen := map[string]bool{}
	var ds []time.Time
	for _, s := range dates {
		s = firstISODay(strings.TrimSpace(s))
		if s == "" || seen[s] {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02", s, time.Local)
		if err != nil {
			continue
		}
		seen[s] = true
		ds = append(ds, t)
	}
	if len(ds) == 0 {
		return ""
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].Before(ds[j]) })
	md := func(t time.Time) string { return fmt.Sprintf("%d/%d", int(t.Month()), t.Day()) }
	if len(ds) == 1 {
		return md(ds[0])
	}
	consecutive := true
	for i := 1; i < len(ds); i++ {
		if !weeklyNextWorkDay(ds[i-1], ds[i]) {
			consecutive = false
			break
		}
	}
	if consecutive {
		return md(ds[0]) + "~" + md(ds[len(ds)-1])
	}
	shown, extra := ds, 0
	if len(shown) > 3 {
		extra = len(shown) - 3
		shown = shown[:3]
	}
	parts := make([]string, 0, len(shown))
	for _, t := range shown {
		parts = append(parts, md(t))
	}
	out := strings.Join(parts, "·")
	if extra > 0 {
		out += fmt.Sprintf(" 외 %d일", extra)
	}
	return out
}

// weeklyNextWorkDay b 가 a 다음 날이거나, a 와 b 사이가 모두 토·일이면 true.
func weeklyNextWorkDay(a, b time.Time) bool {
	for d := a.AddDate(0, 0, 1); d.Before(b); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			return false
		}
	}
	return b.After(a)
}

func isTaskAdminLine(a adminLine) bool { return a.WorkType != "as" && a.WorkType != "mnt" }

// groupWeeklyAdminLines 업무 줄(work_tasks)을 묶음 단위 1줄로 만든다. AS 후속·정기점검 이월 줄은 그대로 둔다.
// 반환 줄의 Count·Dates·GroupKey 를 compose 가 문구로 만든다. 순서는 각 묶음의 첫 등장 순.
func groupWeeklyAdminLines(section string, lines []adminLine) []adminLine {
	hasChild := map[string]bool{}
	for _, a := range lines {
		if isTaskAdminLine(a) && strings.TrimSpace(a.ParentID) != "" {
			hasChild[strings.TrimSpace(a.ParentID)] = true
		}
	}
	type grp struct {
		gid     string
		members []adminLine
	}
	var order []string
	bag := map[string]*grp{}
	out := make([]adminLine, 0, len(lines))
	idx := map[string]int{} // 묶음 → out 위치
	for _, a := range lines {
		if !isTaskAdminLine(a) {
			out = append(out, a)
			continue
		}
		gid := strings.TrimSpace(a.ParentID)
		switch {
		case gid != "":
			gid = "p:" + gid
		case hasChild[strings.TrimSpace(a.ID)]:
			gid = "p:" + strings.TrimSpace(a.ID)
		default:
			gid = "t:" + normWeeklyTitle(a.Title)
		}
		g, ok := bag[gid]
		if !ok {
			g = &grp{gid: gid}
			bag[gid] = g
			order = append(order, gid)
			idx[gid] = len(out)
			out = append(out, adminLine{})
		}
		g.members = append(g.members, a)
	}
	for _, gid := range order {
		g := bag[gid]
		out[idx[gid]] = mergeWeeklyGroup(section, g.gid, g.members)
	}
	return out
}

func mergeWeeklyGroup(section, gid string, ms []adminLine) adminLine {
	first := ms[0]
	var dates []string
	maxDelay := 0
	for _, m := range ms {
		if d := firstISODay(strings.TrimSpace(m.Date)); d != "" {
			dates = append(dates, d)
		}
		if m.DelayN > maxDelay {
			maxDelay = m.DelayN
		}
	}
	// 날짜가 가장 이른 건을 대표로(같으면 입력 순서)
	rep := first
	for _, m := range ms[1:] {
		if d := firstISODay(strings.TrimSpace(m.Date)); d != "" && (firstISODay(strings.TrimSpace(rep.Date)) == "" || d < firstISODay(strings.TrimSpace(rep.Date))) {
			rep = m
		}
	}
	title := strings.TrimSpace(rep.Title)
	if strings.HasPrefix(gid, "p:") {
		pid := strings.TrimPrefix(gid, "p:")
		for _, m := range ms { // 상위 업무가 이 목록에 있으면 그 제목
			if strings.TrimSpace(m.ID) == pid {
				title = strings.TrimSpace(m.Title)
				break
			}
		}
		if pt := strings.TrimSpace(rep.ParentTitle); pt != "" && rep.WorkType != model.WBWorkProjectWork && strings.TrimSpace(rep.ID) != pid {
			title = pt
		}
	}
	title = strings.Join(strings.Fields(title), " ")
	g := rep
	g.Title, g.Dates, g.Count, g.DelayN = title, dates, len(ms), maxDelay
	g.ID = strings.TrimSpace(rep.ID)
	// 묶이지 않은 낱줄(상위 없음·한 건)은 기존 Key(task:ID)를 그대로 쓴다.
	if len(ms) > 1 || strings.HasPrefix(gid, "p:") {
		g.GroupKey = "grp:" + section + ":" + gid
	}
	return g
}

// weeklyDatedTitle 제목 + (기간). 날짜가 없으면 제목만.
func weeklyDatedTitle(a adminLine) string {
	if r := formatWorkDateRange(a.Dates); r != "" {
		return a.Title + "(" + r + ")"
	}
	return a.Title
}

// weeklyCarryTitle 이월 줄: 제목(N주 지연) N회(이월). 현행 형식 유지.
func weeklyCarryTitle(a adminLine) string {
	title := strings.TrimSpace(a.Title)
	if a.DelayN >= 1 {
		title += fmt.Sprintf("(%d주 지연)", a.DelayN)
	}
	if a.Count > 1 {
		title = strings.TrimPrefix(model.FormatRecurrenceTimes(title, a.Count), "·")
	}
	return carryTitle(title)
}

// dedupeAutoLines 같은 구역에서 문구가 완전히 같은 자동 줄은 1줄만 남긴다(공백 정규화 후 비교, 먼저 나온 줄 유지).
func dedupeAutoLines(in []model.CompanyWeeklyAutoLine) []model.CompanyWeeklyAutoLine {
	seen := map[string]bool{}
	out := make([]model.CompanyWeeklyAutoLine, 0, len(in))
	for _, a := range in {
		k := normWeeklyTitle(strings.TrimPrefix(strings.TrimSpace(a.Text), "·"))
		if k != "" {
			if seen[k] {
				continue
			}
			seen[k] = true
		}
		out = append(out, a)
	}
	return out
}
