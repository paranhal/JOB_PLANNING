package handler

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

// 정기점검 계획 화면의 보기 방식
const (
	mntViewCalendar = "calendar"
	mntViewKanban   = "kanban"
	mntViewList     = "list"
)

func mntView(v string) string {
	switch v {
	case mntViewKanban, mntViewList:
		return v
	default:
		return mntViewCalendar
	}
}

func mntViewLabel(v string) string {
	switch v {
	case mntViewKanban:
		return "칸반"
	case mntViewList:
		return "목록"
	default:
		return "캘린더"
	}
}

// MntCalDay 달력 한 칸
type MntCalDay struct {
	Date    string // YYYY-MM-DD (빈 칸이면 "")
	Day     int
	Weekend bool
	Today   bool
	Visits  []model.MaintenanceVisit
}

// MntVisitBoard 칸반 열 묶음
type MntVisitBoard struct {
	Overdue  []model.MaintenanceVisit // 예정일 지남 · 미방문
	Today    []model.MaintenanceVisit
	Upcoming []model.MaintenanceVisit
	Done     []model.MaintenanceVisit
}

// defaultPlanMonth 캘린더·칸반의 기본 달 — 올해면 이번 달, 아니면 방문이 있는 첫 달.
func defaultPlanMonth(year int, visits []model.MaintenanceVisit, now time.Time) int {
	if now.Year() == year {
		return int(now.Month())
	}
	for m := 1; m <= 12; m++ {
		if len(filterVisitsByMonth(visits, year, m)) > 0 {
			return m
		}
	}
	return 1
}

func filterVisitsByMonth(visits []model.MaintenanceVisit, year, month int) []model.MaintenanceVisit {
	prefix := fmt.Sprintf("%04d-%02d-", year, month)
	out := make([]model.MaintenanceVisit, 0, len(visits))
	for _, v := range visits {
		if len(v.VisitDate) >= len(prefix) && v.VisitDate[:len(prefix)] == prefix {
			out = append(out, v)
		}
	}
	return out
}

// filterVisitsByAssignee 담당자명·로그인 ID와 일치하는 방문만 남긴다(기술담당 기본 범위).
func filterVisitsByAssignee(visits []model.MaintenanceVisit, keys []string) []model.MaintenanceVisit {
	want := map[string]struct{}{}
	for _, k := range keys {
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "" {
			want[k] = struct{}{}
		}
	}
	if len(want) == 0 {
		return nil
	}
	out := make([]model.MaintenanceVisit, 0, len(visits))
	for _, v := range visits {
		a := strings.ToLower(strings.TrimSpace(v.Assignee))
		if _, ok := want[a]; ok {
			out = append(out, v)
		}
	}
	return out
}

// buildVisitCalendar 월간 달력(일~토 6주) 격자. month가 0이면 만들지 않는다.
func buildVisitCalendar(year, month int, visits []model.MaintenanceVisit, today string) [][]MntCalDay {
	if month < 1 || month > 12 {
		return nil
	}
	byDate := map[string][]model.MaintenanceVisit{}
	for _, v := range visits {
		byDate[v.VisitDate] = append(byDate[v.VisitDate], v)
	}

	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	offset := int(first.Weekday())
	lastDay := first.AddDate(0, 1, -1).Day()

	weeks := make([][]MntCalDay, 0, 6)
	for w := 0; w < 6; w++ {
		week := make([]MntCalDay, 7)
		filled := false
		for col := 0; col < 7; col++ {
			day := w*7 + col - offset + 1
			cell := MntCalDay{Weekend: col == 0 || col == 6}
			if day >= 1 && day <= lastDay {
				cell.Day = day
				cell.Date = fmt.Sprintf("%04d-%02d-%02d", year, month, day)
				cell.Today = cell.Date == today
				cell.Visits = byDate[cell.Date]
				filled = true
			}
			week[col] = cell
		}
		if !filled {
			break // 6주째가 통째로 비면 줄을 만들지 않는다
		}
		weeks = append(weeks, week)
	}
	return weeks
}

// buildVisitBoard 칸반 열 — 완료 / 예정일 경과 / 오늘 / 예정.
func buildVisitBoard(visits []model.MaintenanceVisit, today string) MntVisitBoard {
	var b MntVisitBoard
	for _, v := range visits {
		switch {
		case v.Completed:
			b.Done = append(b.Done, v)
		case v.VisitDate < today:
			b.Overdue = append(b.Overdue, v)
		case v.VisitDate == today:
			b.Today = append(b.Today, v)
		default:
			b.Upcoming = append(b.Upcoming, v)
		}
	}
	return b
}

// visitLabel 화면에 쓰는 방문 이름 — 같은 사이트라도 점검 대상이 다르면 구분해야 한다.
func visitLabel(v model.MaintenanceVisit) string {
	name := v.ShortName
	if name == "" {
		name = v.OrgName
	}
	if v.ProductType != "" {
		return name + " · " + v.ProductType
	}
	return name
}

// mntProductKind 점검 대상 색 구분 — anrobotics(초록) / klas(파랑) / sejong_klas(보라)
func mntProductKind(product string) string {
	raw := strings.TrimSpace(product)
	if raw == "" {
		return ""
	}
	// K-LAS · KLAS 를 같은 키로 본다.
	compact := strings.ToUpper(raw)
	compact = strings.ReplaceAll(compact, " ", "")
	compact = strings.ReplaceAll(compact, "-", "")
	compact = strings.ReplaceAll(compact, "_", "")
	hasKlas := strings.Contains(compact, "KLAS")
	hasSejong := strings.Contains(raw, "세종") || strings.Contains(compact, "SEJONG")
	if hasSejong && hasKlas {
		return "sejong_klas"
	}
	if strings.Contains(raw, "앤로보") || strings.Contains(compact, "ANROBO") {
		return "anrobotics"
	}
	if hasKlas {
		return "klas"
	}
	return ""
}

// mntProductClass 캘린더·칸반·목록 카드용 클래스(왼쪽 강조선 + 배경).
// CDN Tailwind가 유틸을 못 잡아도 보이도록 style은 mntProductStyle로 병행한다.
func mntProductClass(product string) string {
	switch mntProductKind(product) {
	case "anrobotics":
		return "mnt-prod-anrobotics border-l-4 border-l-green-600 border border-green-400 bg-green-100 text-green-950"
	case "klas":
		return "mnt-prod-klas border-l-4 border-l-blue-600 border border-blue-400 bg-blue-100 text-blue-950"
	case "sejong_klas":
		return "mnt-prod-sejong border-l-4 border-l-purple-600 border border-purple-400 bg-purple-100 text-purple-950"
	default:
		return "border border-gray-200 bg-white text-gray-700"
	}
}

// mntProductStyle 인라인 스타일 — 색이 항상 보이게 한다.
func mntProductStyle(product string) string {
	switch mntProductKind(product) {
	case "anrobotics":
		return "background-color:#bbf7d0;border-color:#4ade80;border-left:4px solid #16a34a;color:#14532d"
	case "klas":
		return "background-color:#bfdbfe;border-color:#60a5fa;border-left:4px solid #2563eb;color:#1e3a8a"
	case "sejong_klas":
		return "background-color:#e9d5ff;border-color:#c084fc;border-left:4px solid #9333ea;color:#581c87"
	default:
		return "background-color:#ffffff;border-color:#e5e7eb;color:#374151"
	}
}

// mntProductBadgeClass 목록·뱃지용 작은 색 칩
func mntProductBadgeClass(product string) string {
	switch mntProductKind(product) {
	case "anrobotics":
		return "bg-green-200 text-green-900"
	case "klas":
		return "bg-blue-200 text-blue-900"
	case "sejong_klas":
		return "bg-purple-200 text-purple-900"
	default:
		return "bg-gray-100 text-gray-600"
	}
}

// mntProductBadgeStyle 뱃지 인라인 스타일
func mntProductBadgeStyle(product string) string {
	switch mntProductKind(product) {
	case "anrobotics":
		return "background-color:#bbf7d0;color:#14532d"
	case "klas":
		return "background-color:#bfdbfe;color:#1e3a8a"
	case "sejong_klas":
		return "background-color:#e9d5ff;color:#581c87"
	default:
		return "background-color:#f3f4f6;color:#4b5563"
	}
}

// excelProductFontColor 엑셀 글자색 — 초록/파랑/보라
func excelProductFontColor(product string) string {
	switch mntProductKind(product) {
	case "anrobotics":
		return "FF00B050" // 초록
	case "klas":
		return "FF0070C0" // 파랑
	case "sejong_klas":
		return "FF7030A0" // 보라
	default:
		return "FF333333"
	}
}
