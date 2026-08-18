package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// parseDurationForm 폼의 start/end 또는 duration_min 으로 소요분·종료시각을 정한다.
func parseDurationForm(start, end, durationStr string, fallback int) (endOut string, durationMin int) {
	durationMin = fallback
	if durationMin <= 0 {
		durationMin = 30
	}
	if d := strings.TrimSpace(durationStr); d != "" {
		var n int
		fmtScanInt(d, &n)
		if n > 0 {
			durationMin = model.NormalizeDurationMin(n)
		}
	}
	if strings.TrimSpace(end) != "" && strings.TrimSpace(start) != "" {
		durationMin = model.DurationFromTimes(start, end)
		return end, durationMin
	}
	sm := model.ParseHHMMMinutes(start)
	if sm < 0 {
		sm = workdayStartHour * 60
	}
	em := sm + durationMin
	if max := workdayEndHour * 60; em > max {
		em = max
	}
	return model.FormatHHMMMinutes(em), durationMin
}

// buildASTitleMap 미배치 AS 업무명 — "[AS]고객_접수번호"
func buildASTitleMap(items []model.ASListItem) map[string]string {
	titles := map[string]string{}
	for _, it := range items {
		titles[it.ASID] = model.FormatASWorkTitle(it.OrgName, it.ASNumber)
	}
	return titles
}

func mntVisitHref(v model.MaintenanceVisit) string {
	href := "/maintenance/" + v.PlanID
	if len(v.VisitDate) >= 7 {
		// 월 쿼리
		if m, err := strconv.Atoi(v.VisitDate[5:7]); err == nil {
			href += fmt.Sprintf("?month=%d&view=list", m)
		}
	}
	return href
}

// mntVisitActionHref 일일업무「조치」용 — 해당 방문 조치 화면
func mntVisitActionHref(v model.MaintenanceVisit) string {
	if strings.TrimSpace(v.VisitID) == "" {
		return ""
	}
	return "/maintenance/visits/" + v.VisitID + "/action"
}

func mntVisitNumber(v model.MaintenanceVisit) string {
	// 화면용 점검 번호 — 방문일·점검대상으로 식별
	num := model.FormatMaintenanceVisitNumber(v.VisitDate, v.ProductType)
	if num == "" {
		return v.VisitID
	}
	return num
}

// taskCardFromWork 배치된 업무 → 시간표 카드
func taskCardFromWork(t model.WorkTask) model.WBCard {
	cat := model.WBCategory(t.SourceType)
	if t.SourceType == "" {
		if t.WorkType == model.WBWorkAS {
			cat = model.WBSourceAS
		} else if t.WorkType == model.WBWorkMaintenance {
			cat = model.WBSourceMaintenance
		}
	}
	card := model.WBCard{
		Kind:         "task",
		RefID:        t.TaskID,
		TaskID:       t.TaskID,
		Category:     cat,
		Title:        t.Title,
		SubTitle:     t.Description,
		Assignee:     t.Assignee,
		WorkDate:     t.WorkDate,
		StartTime:    t.StartTime,
		EndTime:      t.EndTime,
		DurationMin:  t.DurationMin,
		ParentTaskID: t.ParentTaskID,
		ProjectID:    t.ProjectID,
		Status:       t.Status,
		StatusLabel:  model.WBTaskStatusLabel(t.Status),
	}
	switch t.SourceType {
	case model.WBSourceAS:
		card.SourceNumber = t.SourceID
		card.SourceHref = "/as/" + t.SourceID
	case model.WBSourceMaintenance:
		card.SourceNumber = t.SourceID
		// 상세는 Show에서 plan 링크로 보강. 기본은 업무 상세.
		card.SourceHref = "/workboard/tasks/" + t.TaskID
	}
	if cat == model.WBSourceMaintenance {
		card.ProductType = productFromMaintDesc(t.Description)
	}
	if card.SourceHref == "" {
		card.SourceHref = "/workboard/tasks/" + t.TaskID
	}
	return card
}

// productFromMaintDesc "[제품]고객 정기점검" / 구형 "{제품} 정기 점검" 에서 점검 대상을 뽑는다.
func productFromMaintDesc(desc string) string {
	desc = strings.TrimSpace(desc)
	if strings.HasPrefix(desc, "[") {
		if i := strings.Index(desc, "]"); i > 1 {
			return strings.TrimSpace(desc[1:i])
		}
	}
	for _, suf := range []string{" 정기점검", " 정기 점검"} {
		if strings.HasSuffix(desc, suf) {
			return strings.TrimSpace(strings.TrimSuffix(desc, suf))
		}
	}
	// "2026-08-05 · KLAS" 형태
	if i := strings.LastIndex(desc, "·"); i >= 0 {
		p := strings.TrimSpace(desc[i+len("·"):])
		if p != "" {
			return p
		}
	}
	return ""
}

// collectDatesFromForm 하위업무 날짜 — daily_from/to 또는 dates(줄바꿈·쉼표)
func collectDatesFromForm(mode, dailyFrom, dailyTo, datesRaw string) ([]string, error) {
	mode = strings.TrimSpace(mode)
	if mode == "daily" || (dailyFrom != "" && dailyTo != "") {
		return repository.ExpandDailyDates(dailyFrom, dailyTo)
	}
	var out []string
	for _, part := range strings.FieldsFunc(datesRaw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';' || r == ' '
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, err := time.ParseInLocation(dateLayout, part, time.Local); err != nil {
			return nil, fmt.Errorf("날짜 형식 오류: %s", part)
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("하위업무 날짜를 입력하세요")
	}
	return out, nil
}
