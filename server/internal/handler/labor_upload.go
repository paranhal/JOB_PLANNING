package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

type laborUploadRow struct {
	Row        int
	Name       string
	Monthly    int
	Daily      int
	Hourly     int
	JobCode    string
	Verdict    string
	OldMonthly int
	Unmatched  bool
}

func (h *QuotesHandler) SaveLaborRows(c echo.Context) error {
	if !canWriteSales(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	year := atoiQuiet(c.FormValue("year"))
	if year <= 0 {
		year = time.Now().Year()
	}
	_ = c.Request().ParseForm()
	codes := c.Request().PostForm["job_code"]
	names := c.Request().PostForm["job_name"]
	monthlies := c.Request().PostForm["monthly"]
	dailies := c.Request().PostForm["daily"]
	hourlies := c.Request().PostForm["hourly"]
	var items []model.LaborRate
	n := len(codes)
	if len(names) > n {
		n = len(names)
	}
	for i := 0; i < n; i++ {
		code, name := "", ""
		if i < len(codes) {
			code = strings.TrimSpace(codes[i])
		}
		if i < len(names) {
			name = model.CleanJobName(names[i])
		}
		if code == "" && name == "" {
			continue
		}
		if code == "" {
			code = fmt.Sprintf("%02d", i+1)
		}
		it := model.LaborRate{Year: year, JobCode: code, JobName: name}
		if i < len(monthlies) {
			it.Monthly = atoiQuiet(strings.ReplaceAll(monthlies[i], ",", ""))
		}
		if i < len(dailies) {
			it.Daily = atoiQuiet(strings.ReplaceAll(dailies[i], ",", ""))
		}
		if i < len(hourlies) {
			it.Hourly = atoiQuiet(strings.ReplaceAll(hourlies[i], ",", ""))
		}
		items = append(items, it)
	}
	added, changed, err := h.repo.SaveLaborRates(year, items)
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/labor-rates?year=%d&ok=rows&added=%d&changed=%d", year, added, changed))
}

func (h *QuotesHandler) CopyLaborRates(c echo.Context) error {
	if !canWriteSales(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	from := atoiQuiet(c.FormValue("from"))
	to := atoiQuiet(c.FormValue("to"))
	if from <= 0 || to <= 0 {
		return c.Redirect(http.StatusSeeOther, "/labor-rates?err=year")
	}
	n, err := h.repo.CopyLaborRates(from, to)
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/labor-rates?year=%d&ok=copy&copied=%d", to, n))
}

func (h *QuotesHandler) LaborUploadForm(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	year := atoiQuiet(c.QueryParam("year"))
	if year <= 0 {
		year = time.Now().Year()
	}
	return c.Render(http.StatusOK, "quotes/labor_upload.html", map[string]interface{}{
		"Title": "노임단가 올리기", "Active": NavQuotes, "Year": year, "CanWrite": canWriteSales(c) || isAdminRole(c),
	})
}

func (h *QuotesHandler) LaborUploadPreview(c echo.Context) error {
	if !canWriteSales(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	year := atoiQuiet(c.FormValue("year"))
	if year <= 0 {
		year = time.Now().Year()
	}
	file, err := c.FormFile("file")
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/labor-rates/upload?err=file")
	}
	if file.Size > 5<<20 {
		return c.Redirect(http.StatusSeeOther, "/labor-rates/upload?err=size")
	}
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".xlsx") {
		return c.Redirect(http.StatusSeeOther, "/labor-rates/upload?err=ext")
	}
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	raw, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	xf, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer xf.Close()
	sheets := xf.GetSheetList()
	sheet := strings.TrimSpace(c.FormValue("sheet"))
	if sheet == "" && len(sheets) > 0 {
		sheet = sheets[0]
	}
	existing, _ := h.repo.ListLaborRates(year)
	parsed := parseLaborExcel(xf, sheet, existing, laborColsFromForm(c))
	return c.Render(http.StatusOK, "quotes/labor_upload.html", map[string]interface{}{
		"Title": "노임단가 올리기", "Active": NavQuotes, "Year": year,
		"FileName": file.Filename, "Sheets": sheets, "Sheet": sheet, "Rows": parsed.Rows,
		"CanWrite": true, "Preview": true, "Existing": existing,
		"NeedPick": parsed.NeedPick, "Reason": parsed.Reason,
		"ColLetters": parsed.ColLetters,
		"JobCol": parsed.JobLetter, "MonthlyCol": parsed.MonthlyLetter,
		"DailyCol": parsed.DailyLetter, "HourlyCol": parsed.HourlyLetter,
	})
}

func (h *QuotesHandler) LaborUploadApply(c echo.Context) error {
	if !canWriteSales(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	year := atoiQuiet(c.FormValue("year"))
	fileName := strings.TrimSpace(c.FormValue("file_name"))
	note := fmt.Sprintf("%s · %s · %s", fileName, time.Now().Format("2006-01-02"), ctxString(c, "user_name"))
	codes := c.Request().PostForm["job_code"]
	names := c.Request().PostForm["job_name"]
	monthlies := c.Request().PostForm["monthly"]
	dailies := c.Request().PostForm["daily"]
	hourlies := c.Request().PostForm["hourly"]
	skips := map[int]bool{}
	for _, s := range c.Request().PostForm["skip"] {
		skips[atoiQuiet(s)] = true
	}
	var items []model.LaborRate
	for i := range names {
		if skips[i] {
			continue
		}
		name := model.CleanJobName(names[i])
		if name == "" {
			continue
		}
		code := ""
		if i < len(codes) {
			code = strings.TrimSpace(codes[i])
		}
		if code == "" {
			code = fmt.Sprintf("%02d", i+1)
		}
		it := model.LaborRate{Year: year, JobCode: code, JobName: name, SourceNote: note}
		if i < len(monthlies) {
			it.Monthly = atoiQuiet(strings.ReplaceAll(monthlies[i], ",", ""))
		}
		if i < len(dailies) {
			it.Daily = atoiQuiet(strings.ReplaceAll(dailies[i], ",", ""))
		}
		if i < len(hourlies) {
			it.Hourly = atoiQuiet(strings.ReplaceAll(hourlies[i], ",", ""))
		}
		items = append(items, it)
	}
	added, changed, err := h.repo.SaveLaborRates(year, items)
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/labor-rates?year=%d&ok=upload&added=%d&changed=%d", year, added, changed))
}

type laborCols struct {
	Head, Job, Monthly, Daily, Hourly int
	NeedPick                          bool
	Reason                            string
}

type laborParseResult struct {
	Rows           []laborUploadRow
	NeedPick       bool
	Reason         string
	ColLetters     []string
	JobLetter      string
	MonthlyLetter  string
	DailyLetter    string
	HourlyLetter   string
}

func parseLaborExcel(xf *excelize.File, sheet string, existing []model.LaborRate, override *laborCols) laborParseResult {
	rows, err := xf.GetRows(sheet)
	if err != nil {
		return laborParseResult{NeedPick: true, Reason: "시트를 읽지 못했습니다.", ColLetters: excelColLetters(8)}
	}
	return parseLaborSheet(rows, existing, override)
}

func parseLaborSheet(rows [][]string, existing []model.LaborRate, override *laborCols) laborParseResult {
	maxCols := 8
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	if maxCols < 4 {
		maxCols = 4
	}
	letters := excelColLetters(maxCols)
	cols := DetectLaborColumns(rows)
	if override != nil && override.Job >= 0 {
		cols = *override
		cols.NeedPick = false
		cols.Reason = ""
	}
	out := laborParseResult{
		ColLetters:    letters,
		JobLetter:     colLetter(cols.Job),
		MonthlyLetter: colLetter(cols.Monthly),
		DailyLetter:   colLetter(cols.Daily),
		HourlyLetter:  colLetter(cols.Hourly),
	}
	if cols.NeedPick || cols.Job < 0 {
		out.NeedPick = true
		out.Reason = cols.Reason
		if out.Reason == "" {
			out.Reason = "머리글을 찾지 못했고, 값으로도 직무 열을 찾지 못했습니다."
		}
		if out.JobLetter == "" {
			out.JobLetter = "A"
			out.MonthlyLetter = "B"
			out.DailyLetter = "C"
			out.HourlyLetter = "D"
		}
		return out
	}
	byNorm := map[string]model.LaborRate{}
	for _, it := range existing {
		byNorm[normLaborName(it.JobName)] = it
	}
	start := cols.Head + 1
	if start < 0 {
		start = 0
	}
	var parsed []laborUploadRow
	for i := start; i < len(rows); i++ {
		row := rows[i]
		raw := cellAt(row, cols.Job)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if looksLaborHeader(raw) {
			continue
		}
		name := model.CleanJobName(raw)
		if name == "" {
			continue
		}
		ur := laborUploadRow{
			Row: i + 1, Name: name,
			Monthly: parseWonInt(cellAt(row, cols.Monthly)),
			Daily:   parseWonInt(cellAt(row, cols.Daily)),
			Hourly:  parseWonInt(cellAt(row, cols.Hourly)),
		}
		if ur.Monthly == 0 && ur.Daily == 0 && ur.Hourly == 0 && !looksNumber(cellAt(row, cols.Monthly)) {
			continue
		}
		if hit, ok := byNorm[normLaborName(name)]; ok {
			ur.JobCode = hit.JobCode
			ur.OldMonthly = hit.Monthly
			if hit.Monthly == ur.Monthly {
				ur.Verdict = "같음"
			} else {
				ur.Verdict = fmt.Sprintf("바뀜 %s → %s", formatIntComma(hit.Monthly), formatIntComma(ur.Monthly))
			}
		} else {
			ur.Unmatched = true
			ur.Verdict = "새 직무"
		}
		parsed = append(parsed, ur)
	}
	out.Rows = parsed
	if len(parsed) == 0 {
		out.NeedPick = true
		if cols.Monthly < 0 && cols.Daily < 0 && cols.Hourly < 0 {
			out.Reason = "숫자 칸(월·일·시간 임금)을 찾지 못했습니다."
		} else {
			out.Reason = "숫자 칸 없음 — 직무 행을 한 줄도 읽지 못했습니다. 열을 직접 고르세요."
		}
		if out.JobLetter == "" {
			out.JobLetter = "A"
			out.MonthlyLetter = "B"
			out.DailyLetter = "C"
			out.HourlyLetter = "D"
		}
	}
	return out
}

// DetectLaborColumns 머리글 이름 → 값 패턴 순으로 직무·금액 열을 찾는다. §47.20.1
func DetectLaborColumns(rows [][]string) laborCols {
	cols := laborCols{Head: -1, Job: -1, Monthly: -1, Daily: -1, Hourly: -1}
	for i, row := range rows {
		for j, cell := range row {
			s := compactHeader(cell)
			if s == "" {
				continue
			}
			if strings.Contains(s, "구분") || strings.Contains(s, "직무") || strings.Contains(s, "직종") {
				if cols.Job < 0 {
					cols.Head = i
					cols.Job = j
				}
			}
			if strings.Contains(s, "월평균") || strings.Contains(s, "월임금") {
				cols.Monthly = j
				if cols.Head < 0 {
					cols.Head = i
				}
			}
			if strings.Contains(s, "일평균") || strings.Contains(s, "일임금") {
				cols.Daily = j
			}
			if strings.Contains(s, "시간평균") || strings.Contains(s, "시간당") {
				cols.Hourly = j
			}
		}
		if cols.Job >= 0 && cols.Monthly >= 0 {
			break
		}
	}
	if cols.Job >= 0 {
		if cols.Monthly < 0 {
			cols.Monthly = cols.Job + 1
		}
		if cols.Daily < 0 {
			cols.Daily = cols.Job + 2
		}
		if cols.Hourly < 0 {
			cols.Hourly = cols.Job + 3
		}
		return cols
	}
	if job, head := detectLaborJobByValues(rows); job >= 0 {
		cols.Job = job
		cols.Head = head
		cols.Monthly = job + 1
		cols.Daily = job + 2
		cols.Hourly = job + 3
		return cols
	}
	cols.NeedPick = true
	cols.Reason = "머리글을 찾지 못했고, 값으로도 직무 열을 찾지 못했습니다."
	return cols
}

func detectLaborJobByValues(rows [][]string) (jobCol, headBefore int) {
	limit := 30
	if len(rows) < limit {
		limit = len(rows)
	}
	maxCols := 0
	for i := 0; i < limit; i++ {
		if len(rows[i]) > maxCols {
			maxCols = len(rows[i])
		}
	}
	bestJob, bestStreak, bestStart := -1, 0, 0
	for job := 0; job < maxCols; job++ {
		streak, start := 0, 0
		for i := 0; i < limit; i++ {
			if laborValuePatternRow(rows[i], job) {
				if streak == 0 {
					start = i
				}
				streak++
				if streak > bestStreak {
					bestStreak = streak
					bestJob = job
					bestStart = start
				}
			} else {
				streak = 0
			}
		}
	}
	if bestJob >= 0 && bestStreak >= 3 {
		return bestJob, bestStart - 1
	}
	return -1, -1
}

func laborValuePatternRow(row []string, jobCol int) bool {
	name := cellAt(row, jobCol)
	if name == "" || looksNumber(name) || looksLaborHeader(name) {
		return false
	}
	return looksNumber(cellAt(row, jobCol+1)) && looksNumber(cellAt(row, jobCol+2)) && looksNumber(cellAt(row, jobCol+3))
}

func looksLaborHeader(s string) bool {
	c := compactHeader(s)
	return strings.Contains(c, "구분") || strings.Contains(c, "직무") || strings.Contains(c, "직종") ||
		strings.Contains(c, "월평균") || strings.Contains(c, "일평균") || strings.Contains(c, "시간평균")
}

func compactHeader(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) || r == '(' || r == ')' || r == '（' || r == '）' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func looksNumber(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "원", "")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

func laborColsFromForm(c echo.Context) *laborCols {
	job := colLetterToIndex(c.FormValue("job_col"))
	if job < 0 {
		return nil
	}
	m := colLetterToIndex(c.FormValue("monthly_col"))
	d := colLetterToIndex(c.FormValue("daily_col"))
	h := colLetterToIndex(c.FormValue("hourly_col"))
	if m < 0 {
		m = job + 1
	}
	if d < 0 {
		d = job + 2
	}
	if h < 0 {
		h = job + 3
	}
	return &laborCols{Head: -1, Job: job, Monthly: m, Daily: d, Hourly: h}
}

func colLetterToIndex(s string) int {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return -1
	}
	n := 0
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return -1
		}
		n = n*26 + int(r-'A'+1)
	}
	return n - 1
}

func colLetter(i int) string {
	if i < 0 {
		return ""
	}
	return excelColLetters(i + 1)[i]
}

func excelColLetters(n int) []string {
	if n < 4 {
		n = 4
	}
	if n > 26 {
		n = 26
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = string(rune('A' + i))
	}
	return out
}

func normLaborName(s string) string {
	s = model.CleanJobName(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) || r == '(' || r == ')' || r == '/' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func cellAt(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func parseWonInt(s string) int {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "원", "")
	s = strings.ReplaceAll(s, " ", "")
	n, _ := strconv.Atoi(s)
	return n
}

func formatIntComma(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + formatIntComma(-n)
	}
	var out []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}
