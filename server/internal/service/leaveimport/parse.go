// Package leaveimport loads opening annual-leave balances from the fiscal-year Excel sheet.
package leaveimport

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/service/leavecalc"
)

const SheetName = "회계연도 기준 (예비군훈련 포함-연차수조정)"

type Usage struct {
	On      string
	Minutes int
	Deduct  bool
	Raw     string
}

type StaffRow struct {
	ExcelRow     int
	Name         string
	HireDate     string
	GrantDays    float64
	GrantEmpty   bool
	MilitaryDays float64
	Usages       []Usage
	RemainDays   float64
	Errors       []string
	Notes        []string
}

func DaysToMinutes(d float64) int {
	return int(math.Round(d * float64(leavecalc.DefaultWorkMinutesPerDay)))
}

func Parse(raw []byte, year int) ([]StaffRow, error) {
	if year <= 0 {
		year = time.Now().Year()
	}
	xf, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("엑셀을 열 수 없습니다")
	}
	defer xf.Close()
	sheet := ""
	for _, n := range xf.GetSheetList() {
		if n == SheetName {
			sheet = n
			break
		}
	}
	if sheet == "" {
		return nil, fmt.Errorf("시트 「%s」가 없습니다", SheetName)
	}
	rows, err := xf.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	head := findHeader(rows)
	if head < 0 {
		return nil, fmt.Errorf("이름 헤더가 없습니다")
	}
	nameCol, hireCol := 0, 1
	grantCol, milCol, remainCol := 5, 6, 9
	usageCols := map[int]bool{}
	for i, h := range rows[head] {
		k := normHeader(h)
		switch {
		case k == "이름" || k == "성명" || k == "성 명":
			nameCol = i
		case strings.Contains(k, "입사"):
			hireCol = i
		case k == "부여" || strings.Contains(k, "연차일수") || k == "발생":
			grantCol = i
		case strings.Contains(k, "예비군") && !strings.Contains(k, "일자"):
			milCol = i
		case strings.Contains(k, "남은"):
			remainCol = i
		case isMonthHeader(k):
			usageCols[i] = true
		}
	}
	for i := 0; i < maxLen(rows); i++ {
		if i == nameCol || i == hireCol || i == grantCol || i == milCol || i == remainCol {
			continue
		}
		if i < 5 {
			continue
		}
		usageCols[i] = true
	}
	delete(usageCols, grantCol)
	delete(usageCols, milCol)
	delete(usageCols, remainCol)
	delete(usageCols, nameCol)
	delete(usageCols, hireCol)

	var out []StaffRow
	for r := head + 1; r < len(rows); r++ {
		line := rows[r]
		name := strings.TrimSpace(cellAt(line, nameCol))
		if name == "" || strings.HasPrefix(name, "합계") || strings.HasPrefix(name, "총") {
			continue
		}
		row := StaffRow{ExcelRow: r + 1, Name: name}
		grantRaw := strings.TrimSpace(cellAt(line, grantCol))
		if grantEmpty(grantRaw) {
			row.GrantEmpty = true
			row.Notes = append(row.Notes, "연차일수 없음 · 미등록")
			out = append(out, row)
			continue
		}
		gd, err := parseDays(grantRaw)
		if err != nil {
			row.Errors = append(row.Errors, "부여(F) "+err.Error())
		} else {
			row.GrantDays = gd
		}
		if hire := strings.TrimSpace(cellAt(line, hireCol)); hire != "" {
			if d, ok, perr := parseDateValue(hire, year); perr != "" {
				row.Errors = append(row.Errors, "입사일 확인 필요")
			} else if ok {
				row.HireDate = d
			}
		}
		if milRaw := strings.TrimSpace(cellAt(line, milCol)); milRaw != "" && !grantEmpty(milRaw) {
			md, err := parseDays(milRaw)
			if err != nil {
				row.Errors = append(row.Errors, "예비군 보전(G) "+err.Error())
			} else {
				row.MilitaryDays = md
			}
		}
		if remRaw := strings.TrimSpace(cellAt(line, remainCol)); remRaw != "" && !grantEmpty(remRaw) {
			rd, err := parseDays(remRaw)
			if err != nil {
				row.Errors = append(row.Errors, "남은연차(J) "+err.Error())
			} else {
				row.RemainDays = rd
			}
		}
		for i := range usageCols {
			rawU := strings.TrimSpace(cellAt(line, i))
			if rawU == "" {
				continue
			}
			us, errs := ParseUsageCell(rawU, year)
			row.Usages = append(row.Usages, us...)
			row.Errors = append(row.Errors, errs...)
		}
		out = append(out, row)
	}
	return out, nil
}

func findHeader(rows [][]string) int {
	for i, row := range rows {
		for _, c := range row {
			k := normHeader(c)
			if k == "이름" || k == "성명" || k == "성 명" {
				return i
			}
		}
	}
	return -1
}

func normHeader(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

func isMonthHeader(k string) bool {
	if k == "" {
		return false
	}
	if strings.HasSuffix(k, "월") {
		n := strings.TrimSuffix(k, "월")
		if _, err := strconv.Atoi(n); err == nil {
			return true
		}
	}
	if n, err := strconv.Atoi(k); err == nil && n >= 1 && n <= 12 {
		return true
	}
	return false
}

func cellAt(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

func maxLen(rows [][]string) int {
	m := 0
	for _, r := range rows {
		if len(r) > m {
			m = len(r)
		}
	}
	return m
}

func grantEmpty(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || s == "-" || s == "—" || s == "–" || strings.EqualFold(s, "n/a")
}

func parseDays(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "일")
	s = strings.ReplaceAll(s, ",", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("확인 필요(%s)", s)
	}
	return f, nil
}

var (
	reDateYMD = regexp.MustCompile(`(?i)(\d{4})[./-](\d{1,2})[./-](\d{1,2})`)
	reDateMD  = regexp.MustCompile(`(\d{1,2})[./-](\d{1,2})`)
	reSerial  = regexp.MustCompile(`^\d{5}(\.\d+)?$`)
)

func ParseUsageCell(s string, year int) ([]Usage, []string) {
	orig := strings.TrimSpace(s)
	if orig == "" {
		return nil, nil
	}
	var out []Usage
	var errs []string
	for _, p := range splitUsage(orig) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		u, bad := parseOneUsage(p, year)
		if bad {
			errs = append(errs, "확인 필요: "+p)
			continue
		}
		out = append(out, u)
	}
	return out, errs
}

func splitUsage(s string) []string {
	s = strings.ReplaceAll(s, "，", ",")
	s = strings.ReplaceAll(s, "、", ",")
	s = strings.ReplaceAll(s, ";", ",")
	s = strings.ReplaceAll(s, "\n", ",")
	s = strings.ReplaceAll(s, "\r", ",")
	return strings.Split(s, ",")
}

func parseOneUsage(p string, year int) (Usage, bool) {
	raw := p
	half := strings.Contains(p, "(반차)") || strings.Contains(p, "（반차）")
	mil := strings.Contains(p, "예비군") || strings.Contains(p, "공가")
	cleaned := p
	cleaned = strings.ReplaceAll(cleaned, "(반차)", " ")
	cleaned = strings.ReplaceAll(cleaned, "（반차）", " ")
	cleaned = strings.ReplaceAll(cleaned, "예비군", " ")
	cleaned = strings.ReplaceAll(cleaned, "공가", " ")
	cleaned = strings.ReplaceAll(cleaned, "훈련", " ")
	on, ok, _ := parseDateValue(strings.TrimSpace(cleaned), year)
	if !ok {
		return Usage{}, true
	}
	if stripDateNoise(cleaned) != "" {
		return Usage{}, true
	}
	min := leavecalc.DefaultWorkMinutesPerDay
	if half {
		min = leavecalc.DefaultHalfDayMinutes
	}
	return Usage{On: on, Minutes: min, Deduct: !mil, Raw: raw}, false
}

func stripDateNoise(s string) string {
	s = reDateYMD.ReplaceAllString(s, " ")
	s = reDateMD.ReplaceAllString(s, " ")
	s = reSerial.ReplaceAllString(s, " ")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) || r == '/' || r == '.' || r == '-' || r == '(' || r == ')' || r == '（' || r == '）' {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func parseDateValue(s string, year int) (string, bool, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false, ""
	}
	if reSerial.MatchString(s) {
		f, err := strconv.ParseFloat(s, 64)
		if err == nil && f > 20000 && f < 80000 {
			t, err := excelize.ExcelDateToTime(f, false)
			if err == nil {
				return t.Format("2006-01-02"), true, ""
			}
		}
	}
	for _, layout := range []string{"2006-01-02", "2006.01.02", "2006/01/02", "2006.1.2", "2006/1/2"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02"), true, ""
		}
	}
	if m := reDateYMD.FindStringSubmatch(s); m != nil && stripDateNoise(s) == "" {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
		if t.Month() != time.Month(mo) || t.Day() != d {
			return "", false, "날짜"
		}
		return t.Format("2006-01-02"), true, ""
	}
	if m := reDateMD.FindStringSubmatch(s); m != nil && stripDateNoise(s) == "" {
		mo, _ := strconv.Atoi(m[1])
		d, _ := strconv.Atoi(m[2])
		t := time.Date(year, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
		if t.Month() != time.Month(mo) || t.Day() != d {
			return "", false, "날짜"
		}
		return t.Format("2006-01-02"), true, ""
	}
	return "", false, "날짜"
}
